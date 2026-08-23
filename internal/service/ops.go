package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ahmed/odoonoir/internal/db"
	"github.com/ahmed/odoonoir/internal/installer"
	"github.com/ahmed/odoonoir/internal/instance"
	"github.com/ahmed/odoonoir/internal/logmon"
	"github.com/ahmed/odoonoir/internal/updater"
)

// DatabaseView is a GUI-friendly database row.
type DatabaseView struct {
	Name        string `json:"name"`
	SizeBytes   int64  `json:"sizeBytes"`
	Owner       string `json:"owner"`
	Initialized bool   `json:"initialized"`
	Primary     bool   `json:"primary"`
}

// Databases lists the databases served by an instance with live metadata.
func (s *Service) Databases(name string) ([]DatabaseView, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return nil, err
	}
	if err := s.pg.ServerRunning(); err != nil {
		return nil, err
	}
	names := inst.AllDBs()
	out := make([]DatabaseView, 0, len(names))
	for _, n := range names {
		dv := DatabaseView{Name: n, Primary: n == inst.DBName}
		if sz, err := s.pg.DatabaseSize(n); err == nil {
			dv.SizeBytes = sz
		}
		if o, err := s.pg.DatabaseOwner(n); err == nil {
			dv.Owner = o
		}
		if ok, err := s.pg.IsInitialized(n); err == nil {
			dv.Initialized = ok
		}
		out = append(out, dv)
	}
	return out, nil
}

// InitDB creates a database and runs -i base (installing core modules).
// An existing-but-uninitialized database is initialized in place; an
// initialized database is refused. Newly created databases are registered
// with the instance.
func (s *Service) InitDB(ctx context.Context, name, dbName string, emit Sink) error {
	inst, err := s.reg.Get(name)
	if err != nil {
		return err
	}
	if dbName == "" {
		dbName = inst.DBName
	}
	if err := s.requireStopped(name); err != nil {
		return err
	}
	exists, err := s.pg.DatabaseExists(dbName)
	if err != nil {
		return err
	}
	if exists {
		init, err := s.pg.IsInitialized(dbName)
		if err != nil {
			return err
		}
		if init {
			return fmt.Errorf("database %q already exists and is initialized", dbName)
		}
		emit(Event{Kind: LogLine, Instance: name, Message: "database " + dbName + " already exists"})
	} else {
		if err := s.pg.CreateDatabase(dbName, inst.DBUser); err != nil {
			return err
		}
	}
	created := !exists
	tracked := false
	defer func() {
		if created && !tracked {
			// roll back the empty database if init fails
			_ = s.pg.DropDatabase(dbName)
		}
	}()
	emit(Event{Kind: StepStart, Instance: name, Step: "initializing " + dbName})
	err = updater.InitDatabase(ctx, inst, s.rootFor(inst), dbName, LineSink(name, "init "+dbName, emit))
	if err != nil {
		emit(Event{Kind: StepFail, Instance: name, Message: err.Error()})
		return err
	}
	emit(Event{Kind: StepDone, Instance: name, Step: "initializing " + dbName})
	inst.AddDB(dbName)
	if err := s.reg.Put(inst); err != nil {
		return err
	}
	tracked = true
	return nil
}

// Backup dumps a database to out (or the instance's default backups dir
// when out is empty). custom/compress map to pg_dump -Fc and gzip.
func (s *Service) Backup(ctx context.Context, name, dbName, out string, custom, compress bool, emit Sink) error {
	inst, err := s.reg.Get(name)
	if err != nil {
		return err
	}
	if dbName == "" {
		dbName = inst.DBName
	}
	if err := s.requireDB(inst, dbName); err != nil {
		return err
	}
	if out == "" {
		p := inst.ResolvePaths(s.rootFor(inst))
		out = defaultBackupPath(p, dbName, custom, compress)
	}
	emit(Event{Kind: StepStart, Instance: name, Step: "backing up " + dbName})
	err = s.pg.Backup(ctx, dbName, out, custom, compress)
	if err != nil {
		emit(Event{Kind: StepFail, Instance: name, Message: err.Error()})
		return err
	}
	emit(Event{Kind: StepDone, Instance: name, Step: "backing up " + dbName})
	emit(Event{Kind: LogLine, Instance: name, Message: "backup written to " + out})
	return nil
}

// RestoreOptions drives Restore.
type RestoreOptions struct {
	// Force drops and recreates the database when it already exists.
	Force bool
}

// Restore loads a dump into an instance database. The dump is validated
// first, so a bad dump never costs an existing database. With Force the
// existing database is dropped and recreated; otherwise it must not exist.
// Fresh database names are tracked with the instance after success.
func (s *Service) Restore(ctx context.Context, name, dbName, dump string, opts RestoreOptions, emit Sink) error {
	inst, err := s.reg.Get(name)
	if err != nil {
		return err
	}
	if dbName == "" {
		dbName = inst.DBName
	}
	if err := s.requireStopped(name); err != nil {
		return err
	}
	if err := db.ValidateDump(dump); err != nil {
		return err
	}
	exists, err := s.pg.DatabaseExists(dbName)
	if err != nil {
		return err
	}
	if exists && !opts.Force {
		return fmt.Errorf("database %s already exists — pass --force to drop and recreate", dbName)
	}
	created := false
	if exists {
		emit(Event{Kind: StepStart, Instance: name, Step: "dropping existing database " + dbName})
		if err := s.pg.DropDatabase(dbName); err != nil {
			emit(Event{Kind: StepFail, Instance: name, Message: err.Error()})
			return err
		}
		emit(Event{Kind: StepDone, Instance: name, Step: "dropping existing database " + dbName})
	}
	emit(Event{Kind: StepStart, Instance: name, Step: "creating database " + dbName})
	if err := s.pg.CreateDatabase(dbName, inst.DBUser); err != nil {
		emit(Event{Kind: StepFail, Instance: name, Message: err.Error()})
		return err
	}
	emit(Event{Kind: StepDone, Instance: name, Step: "creating database " + dbName})
	created = true
	restored := false
	defer func() {
		if created && !restored {
			_ = s.pg.DropDatabase(dbName)
		}
	}()
	emit(Event{Kind: StepStart, Instance: name, Step: "restoring " + dump})
	err = s.pg.Restore(ctx, dbName, dump)
	if err != nil {
		emit(Event{Kind: StepFail, Instance: name, Message: err.Error()})
		return err
	}
	emit(Event{Kind: StepDone, Instance: name, Step: "restoring " + dump})
	restored = true
	inst.AddDB(dbName)
	return s.reg.Put(inst)
}

// DropDB drops a database (the primary database cannot be dropped). The
// instance must be stopped first.
func (s *Service) DropDB(name, dbName string) error {
	inst, err := s.reg.Get(name)
	if err != nil {
		return err
	}
	if dbName == "" {
		dbName = inst.DBName
	}
	if dbName == inst.DBName {
		return fmt.Errorf("cannot drop the primary database %q", dbName)
	}
	if err := s.requireStopped(name); err != nil {
		return err
	}
	if err := s.pg.DropDatabase(dbName); err != nil {
		return err
	}
	inst.RemoveDB(dbName)
	return s.reg.Put(inst)
}

// UpdateOptions mirrors the CLI update flags.
type UpdateOptions struct {
	InstallMods []string
	UpdateMods  []string
	UpgradeAll  bool
	DB          string
}

// Update runs the full update pipeline (pull, addons refresh, pip,
// module install/upgrade) with progress events.
func (s *Service) Update(ctx context.Context, name string, opts UpdateOptions, emit Sink) error {
	inst, err := s.reg.Get(name)
	if err != nil {
		return err
	}
	err = updater.Run(ctx, s.cfg, inst, updater.Options{
		InstallMods: opts.InstallMods,
		UpdateMods:  opts.UpdateMods,
		UpgradeAll:  opts.UpgradeAll,
		DB:          opts.DB,
	}, UpdaterSink(name, emit))
	if err != nil {
		emit(Event{Kind: StepFail, Instance: name, Message: err.Error()})
		return err
	}
	emit(Event{Kind: StatusChanged, Instance: name, Message: "update complete"})
	return nil
}

// ModuleOps runs -i/-u module operations against a database without the
// git/pip steps of Update.
func (s *Service) ModuleOps(ctx context.Context, name, dbName string, install, update []string, emit Sink) error {
	inst, err := s.reg.Get(name)
	if err != nil {
		return err
	}
	if dbName == "" {
		dbName = inst.DBName
	}
	step := "module " + strings.Join(append(append([]string{}, install...), update...), ",")
	emit(Event{Kind: StepStart, Instance: name, Step: step})
	err = updater.InstallModules(ctx, inst, s.rootFor(inst), dbName, install, update, LineSink(name, step, emit))
	if err != nil {
		emit(Event{Kind: StepFail, Instance: name, Message: err.Error()})
		return err
	}
	emit(Event{Kind: StepDone, Instance: name, Step: step})
	return nil
}

// Test runs the test suite of the given modules against a database.
func (s *Service) Test(ctx context.Context, name, dbName string, modules []string, emit Sink) error {
	inst, err := s.reg.Get(name)
	if err != nil {
		return err
	}
	if dbName == "" {
		dbName = inst.DBName
	}
	step := "test " + strings.Join(modules, ",")
	emit(Event{Kind: StepStart, Instance: name, Step: step})
	err = updater.RunTests(ctx, inst, s.rootFor(inst), dbName, modules, LineSink(name, step, emit))
	if err != nil {
		emit(Event{Kind: StepFail, Instance: name, Message: err.Error()})
		return err
	}
	emit(Event{Kind: StepDone, Instance: name, Step: step})
	return nil
}

// LogsSince returns new log lines appended since offset (see logmon.TailFrom:
// first call returns the last n lines). GUI screens poll this.
func (s *Service) LogsSince(name string, n int, offset *int64) ([]string, bool, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return nil, false, err
	}
	p := inst.ResolvePaths(s.rootFor(inst))
	if _, err := os.Stat(p.Log); err != nil {
		return nil, false, nil
	}
	return logmon.TailFrom(p.Log, n, offset)
}

// LogPath resolves the log file of an instance.
func (s *Service) LogPath(name string) (string, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return "", err
	}
	return inst.ResolvePaths(s.rootFor(inst)).Log, nil
}

// defaultBackupPath derives the CLI's default backup location:
// <instance>/backups/<db>_<timestamp>.<ext>.
func defaultBackupPath(p instance.Paths, dbName string, custom, compress bool) string {
	ext := ".sql"
	if custom {
		ext = ".dump"
	}
	if compress {
		ext += ".gz"
	}
	dir := filepath.Join(p.Root, "backups")
	return fmt.Sprintf("%s/%s_%s%s", dir, dbName, time.Now().Format("20060102_150405"), ext)
}

// installOptions mirrors the CLI create flags relevant to the engine.
type installOptions struct {
	Version  string
	GitRef   string
	Python   string
	Port     int
	LongPoll int
	DBUser   string
	DBPass   string
	DBName   string
}

// Install runs the installer pipeline (clone/venv/requirements/conf) for an
// instance with progress events.
func (s *Service) Install(ctx context.Context, name string, opts installOptions, emit Sink) error {
	inst, err := s.reg.Get(name)
	if err != nil {
		return err
	}
	emit(Event{Kind: StepStart, Instance: name, Step: "install"})
	err = installer.Install(ctx, s.cfg, inst, installer.Options{
		Version:  opts.Version,
		GitRef:   opts.GitRef,
		Python:   opts.Python,
		Port:     opts.Port,
		LongPoll: opts.LongPoll,
		DBUser:   opts.DBUser,
		DBPass:   opts.DBPass,
		DBName:   opts.DBName,
	}, LineSink(name, "install", emit))
	if err != nil {
		emit(Event{Kind: StepFail, Instance: name, Message: err.Error()})
		return err
	}
	emit(Event{Kind: StepDone, Instance: name, Step: "install"})
	return nil
}
