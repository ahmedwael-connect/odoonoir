package service

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/ahmed/odoonoir/internal/checker"
	"github.com/ahmed/odoonoir/internal/db"
	"github.com/ahmed/odoonoir/internal/installer"
	"github.com/ahmed/odoonoir/internal/instance"
)

// CreateOptions drives Create. Empty values fall back to the CLI defaults
// (db user "odoo", db password = user, port auto-detection...).
type CreateOptions struct {
	Name        string
	Version     string // "16", "17", "18", "19"
	GitRef      string
	Source      string // git URL
	Python      string
	Port        int
	DBName      string
	DBUser      string
	DBPass      string
	Root        string // per-instance override of the instances dir
	Description string
	Workers     int
	LogLevel    string
	Dev         bool
	Force       bool // replace an existing target folder
	InitDB      bool // run -i base after install
	Extra       map[string]string
}

// CreateResult reports what Create produced.
type CreateResult struct {
	Instance *instance.Instance
	Database string
	Port     int
}

// Create builds a full instance: postgres role, port checks, source clone,
// venv/requirements, conf, registry entry and optional database init. On
// failure everything is rolled back (folder, database, registry entry), so
// a failed create never leaves a broken instance behind. Progress streams
// through emit.
func (s *Service) Create(ctx context.Context, opts CreateOptions, emit Sink) (*CreateResult, error) {
	inst, err := s.reg.Get(opts.Name)
	if err == nil {
		return nil, fmt.Errorf("instance %q already exists (registry has it with version %s)", opts.Name, inst.Version)
	} else if !IsNotFound(err) {
		return nil, err
	}

	// ---- pre-flight checks ------------------------------------------------
	if err := db.IsValidName(opts.Name); err != nil {
		return nil, fmt.Errorf("invalid instance name %q", opts.Name)
	}
	major := strings.Split(opts.Version, ".")[0]
	if major == "" {
		return nil, fmt.Errorf("version is required (e.g. 16, 17, 18, 19)")
	}
	// Pre-flight: verify a compatible Python is available before starting install.
	pyBin := opts.Python
	if pyBin == "" {
		pyBin = installer.ResolvePython(opts.Version)
	}
	if err := installer.CheckPythonCompat(pyBin, opts.Version); err != nil {
		return nil, err
	}
	instPath := s.cfg.InstancePath(opts.Name)
	if opts.Root != "" {
		instPath = filepath.Join(opts.Root, opts.Name)
	}
	if _, err := os.Stat(instPath); err == nil {
		if !opts.Force {
			return nil, fmt.Errorf("target path already exists: %s (pass Force to remove and recreate)", instPath)
		}
		if err := os.RemoveAll(instPath); err != nil {
			return nil, fmt.Errorf("remove existing target path %s: %w", instPath, err)
		}
		emit(Event{Kind: LogLine, Instance: opts.Name, Message: "removed existing path " + instPath + " (force)"})
	}
	folderCreated := true
	defer func() {
		if folderCreated {
			_ = os.RemoveAll(instPath)
		}
	}()

	if err := s.pg.ServerRunning(); err != nil {
		return nil, err
	}
	admin, err := s.pg.DetectAdminUser()
	if err != nil {
		return nil, fmt.Errorf("postgres admin user not found: %w", err)
	}
	s.mu.Lock()
	origPostgresUser := s.cfg.PostgresUser
	origOdooUser := s.cfg.OdooUser
	origOdooPassword := s.cfg.OdooPassword
	s.cfg.PostgresUser = admin
	s.cfg.OdooUser = ""
	s.cfg.OdooPassword = ""
	s.mu.Unlock()

	dbUser := opts.DBUser
	if dbUser == "" {
		dbUser = "odoo"
	}
	if err := db.IsValidName(dbUser); err != nil {
		return nil, fmt.Errorf("invalid db user %q", dbUser)
	}
	dbPass := opts.DBPass
	if dbPass == "" {
		dbPass = dbUser
	}
	created, err := s.pg.EnsureRole(dbUser, true)
	if err != nil {
		return nil, err
	}
	if created {
		emit(Event{Kind: LogLine, Instance: opts.Name, Message: "postgres role " + dbUser + " created"})
	} else {
		emit(Event{Kind: LogLine, Instance: opts.Name, Message: "postgres role " + dbUser + " already exists"})
	}

	port := opts.Port
	if port == 0 {
		port, err = nextFreePort(8069)
		if err != nil {
			return nil, err
		}
	}
	if port > 65532 {
		return nil, fmt.Errorf("port %d is too high — the longpoll port (port+3, %d) would exceed the valid range", port, port+3)
	}
	if checker.PortInUse(port) {
		return nil, fmt.Errorf("port %d is already in use", port)
	}
	if err := s.registryPortFree(port); err != nil {
		return nil, err
	}
	longpoll := port + 3
	if checker.PortInUse(longpoll) {
		return nil, fmt.Errorf("longpoll port %d (http_port + 3) is already in use — pass Port to pick a different HTTP port", longpoll)
	}

	dbName := opts.DBName
	if dbName == "" {
		dbName = db.SuggestDBName(opts.Name)
	}
	if err := db.IsValidName(dbName); err != nil {
		return nil, fmt.Errorf("invalid database name %q", dbName)
	}
	s.mu.Lock()
	s.cfg.OdooUser = dbUser
	s.cfg.OdooPassword = dbPass
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.cfg.PostgresUser = origPostgresUser
		s.cfg.OdooUser = origOdooUser
		s.cfg.OdooPassword = origOdooPassword
		s.mu.Unlock()
	}()

	inst = &instance.Instance{
		Name:         opts.Name,
		Version:      major,
		Branch:       installer.BranchFor(major),
		SourceURL:    opts.Source,
		Port:         port,
		LongpollPort: longpoll,
		DBName:       dbName,
		DBUser:       dbUser,
		Root:         opts.Root,
		Description:  opts.Description,
		Workers:      opts.Workers,
		LogLevel:     opts.LogLevel,
		GitRef:       opts.GitRef,
		PythonBin:    opts.Python,
	}

	// ---- install pipeline -------------------------------------------------
	emit(Event{Kind: StepStart, Instance: opts.Name, Step: "installing source and python environment"})
	err = installer.Install(ctx, s.cfg, inst, installer.Options{
		Version:  major,
		Port:     port,
		LongPoll: longpoll,
		DBUser:   dbUser,
		DBPass:   dbPass,
		DBName:   dbName,
		DevMode:  opts.Dev,
		Workers:  opts.Workers,
		LogLevel: opts.LogLevel,
		GitRef:   opts.GitRef,
		Python:   opts.Python,
		Extra:    opts.Extra,
	}, LineSink(opts.Name, "install", emit))
	if err != nil {
		emit(Event{Kind: StepFail, Instance: opts.Name, Message: err.Error()})
		return nil, err
	}
	emit(Event{Kind: StepDone, Instance: opts.Name, Step: "installing source and python environment"})

	emit(Event{Kind: StepStart, Instance: opts.Name, Step: "creating database " + dbName})
	if err := s.pg.CreateDatabase(dbName, dbUser); err != nil {
		emit(Event{Kind: StepFail, Instance: opts.Name, Message: err.Error()})
		return nil, err
	}
	dbCreated := true
	defer func() {
		if dbCreated {
			_ = s.pg.DropDatabase(dbName)
		}
	}()
	emit(Event{Kind: StepDone, Instance: opts.Name, Step: "creating database " + dbName})

	// ---- registry entry ---------------------------------------------------
	if err := s.reg.Put(inst); err != nil {
		return nil, err
	}
	registered := true
	defer func() {
		if registered {
			_ = s.reg.Delete(opts.Name)
		}
	}()

	// ---- database init (optional) ------------------------------------------
	if opts.InitDB {
		if err := s.InitDB(ctx, opts.Name, dbName, emit); err != nil {
			return nil, err
		}
	}

	folderCreated, dbCreated, registered = false, false, false
	return &CreateResult{Instance: inst, Database: dbName, Port: port}, nil
}

// nextFreePort finds a free HTTP port (with its longpoll sibling free too).
func nextFreePort(start int) (int, error) {
	for p := start; p < start+100; p++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err != nil {
			continue
		}
		_ = ln.Close()
		ln2, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p+3))
		if err != nil {
			continue
		}
		_ = ln2.Close()
		return p, nil
	}
	return 0, fmt.Errorf("no free port found from %d", start)
}

// registryPortFree verifies no other registered instance uses the port.
func (s *Service) registryPortFree(port int) error {
	if port < 1 || port > 65532 {
		return fmt.Errorf("invalid port %d", port)
	}
	all, err := s.reg.All()
	if err != nil {
		return err
	}
	for _, inst := range all {
		if inst.Port == port || inst.LongpollPort == port {
			return fmt.Errorf("port %d is used by instance %q", port, inst.Name)
		}
	}
	return nil
}
