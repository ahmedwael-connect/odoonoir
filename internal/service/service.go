// Package service is the toolkit-agnostic engine shared by the CLI and the
// future Wails GUI. It owns the registry/config/DB access and exposes
// typed operations that emit Event streams for long-running work, so a
// UI layer never touches os/exec or instance internals directly.
package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ahmed/odoonoir/internal/sudo"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/process"

	"github.com/ahmed/odoonoir/internal/ci"
	"github.com/ahmed/odoonoir/internal/config"
	"github.com/ahmed/odoonoir/internal/db"
	ghinternal "github.com/ahmed/odoonoir/internal/github"
	"github.com/ahmed/odoonoir/internal/installer"
	"github.com/ahmed/odoonoir/internal/instance"
	"github.com/ahmed/odoonoir/internal/marketplace"
	"github.com/ahmed/odoonoir/internal/odoconf"
	"github.com/ahmed/odoonoir/internal/proc"
	"github.com/ahmed/odoonoir/internal/ssh"
	"github.com/google/go-github/v60/github"
	"github.com/robfig/cron/v3"
)

// Service is the application engine. Create one per process; it is safe to
// call from multiple goroutines (registry access is file-locked).
type Service struct {
	mu        sync.Mutex
	cfg       *config.Config
	reg       *instance.Registry
	pg        *db.PG
	scheduler *cron.Cron
	github    *ghinternal.Client
	sshClients map[string]*ssh.Client
	sshMu     sync.Mutex
	marketplace *marketplace.Service
	ci         *ci.CIService
}

// New loads the global config and registry and returns a Service.
func New(cfg *config.Config) (*Service, error) {
	reg, err := instance.NewRegistry(cfg.RegistryPath())
	if err != nil {
		return nil, err
	}
	s := &Service{cfg: cfg, reg: reg, pg: db.New(cfg)}
	s.scheduler = cron.New(cron.WithLocation(time.Local))
	s.scheduler.Start()
	s.sshClients = make(map[string]*ssh.Client)

	// Initialize marketplace
	store, err := marketplace.NewFileStore(filepath.Join(cfg.InstancesDir(), "marketplace"))
	if err != nil {
		return nil, fmt.Errorf("create marketplace store: %w", err)
	}
	s.marketplace = marketplace.NewService(store, nil)

	// Initialize GitHub client if token is available
	tokenStore, _ := ghinternal.NewFileTokenStore()
	if token, _ := tokenStore.Get(); token != "" {
		s.github = ghinternal.NewClient(token)
		// Also update marketplace with GitHub client
		if s.marketplace != nil {
			s.marketplace.SetGitHubClient(s.github)
		}
	}

	// Initialize CI service (needs underlying go-github client)
	var goClient *github.Client
	if s.github != nil {
		goClient = s.github.GoClient()
	}
	s.ci = ci.NewCIService(goClient)

	// Re-hydrate backup schedules after restart
	if err := s.restoreBackupSchedules(); err != nil {
		// non-fatal: log but don't fail startup
		_ = err
	}
	return s, nil
}

// restoreBackupSchedules re-adds cron entries for instances with enabled schedules.
func (s *Service) restoreBackupSchedules() error {
	all, err := s.reg.All()
	if err != nil {
		return err
	}
	for _, inst := range all {
		if !inst.BackupScheduleEnabled || inst.BackupScheduleCron == "" {
			continue
		}
		name := inst.Name
		entryID, err := s.scheduler.AddFunc(inst.BackupScheduleCron, func() {
			s.runScheduledBackup(name)
		})
		if err != nil {
			continue
		}
		inst.BackupScheduleID = int(entryID)
		entry := s.scheduler.Entry(cron.EntryID(entryID))
		next := entry.Next
		inst.BackupScheduleNextRun = &next
		_ = s.reg.Put(inst)
	}
	return nil
}

// InstanceView is a flat, GUI-friendly snapshot of an instance.
type InstanceView struct {
	Name    string          `json:"name"`
	Version string          `json:"version"`
	Status  instance.Status `json:"status"`
	PID     int             `json:"pid"`
	Port    int             `json:"port"`
	DBName  string          `json:"dbName"`
	DBs     int             `json:"dbs"`
	Path    string          `json:"path"`
	Adopted bool            `json:"adopted"`
}

// Instances returns every registered instance with its live status.
func (s *Service) Instances() ([]InstanceView, error) {
	all, err := s.reg.All()
	if err != nil {
		return nil, err
	}
	out := make([]InstanceView, 0, len(all))
	for _, inst := range all {
		p := inst.ResolvePaths(s.rootFor(inst))
		mgr := proc.New(p, installer.PythonFor(inst, p), p.Conf, inst.LongpollPort)
		status, pid, err := mgr.Status()
		if err != nil {
			status = instance.StatusUnknown
		}
		out = append(out, InstanceView{
			Name:    inst.Name,
			Version: inst.Version,
			Status:  status,
			PID:     pid,
			Port:    inst.Port,
			DBName:  inst.DBName,
			DBs:     len(inst.AllDBs()),
			Path:    p.Root,
			Adopted: inst.Adopted,
		})
	}
	return out, nil
}

// StatusView is the runtime detail of a single instance.
type StatusView struct {
	Instance InstanceView `json:"Instance"`
	Serving  string       `json:"Serving"` // database currently served, if any
	Conf     string       `json:"Conf"`
	Log      string       `json:"Log"`
}

// Status returns the live status of one instance.
func (s *Service) Status(name string) (StatusView, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return StatusView{}, err
	}
	p := inst.ResolvePaths(s.rootFor(inst))
	mgr := proc.New(p, installer.PythonFor(inst, p), p.Conf, inst.LongpollPort)
	status, pid, err := mgr.Status()
	if err != nil {
		status = instance.StatusUnknown
	}
	return StatusView{
		Instance: InstanceView{
			Name:    inst.Name,
			Version: inst.Version,
			Status:  status,
			PID:     pid,
			Port:    inst.Port,
			DBName:  inst.DBName,
			DBs:     len(inst.AllDBs()),
			Path:    p.Root,
			Adopted: inst.Adopted,
		},
		Serving: mgr.ServingDB(),
		Conf:    p.Conf,
		Log:     p.Log,
	}, nil
}

// Start launches an instance. db selects the database to serve (empty =
// primary). emit receives StatusChanged events.
// If instance has AutoUpdateOnRun set, modules are passed as -u.
func (s *Service) Start(ctx context.Context, name, dbName string, emit Sink) error {
	inst, err := s.reg.Get(name)
	if err != nil {
		return err
	}
	if dbName != "" {
		if err := s.requireDB(inst, dbName); err != nil {
			return err
		}
	} else {
		dbName = inst.DBName
	}
	mgr := s.procFor(inst)
	var auto []string
	if inst.AutoUpdateOnRun && len(inst.AutoUpdateModules) > 0 {
		auto = inst.AutoUpdateModules
		emit(Event{Kind: LogLine, Instance: name, Message: fmt.Sprintf("auto-update on run: -u %s", strings.Join(auto, ","))})
	}
	if err := mgr.StartWithUpdate(dbName, auto); err != nil {
		return err
	}
	if err := waitForStable(ctx, mgr, s.logPath(inst)); err != nil {
		return err
	}
	emit(Event{Kind: StatusChanged, Instance: name, Message: fmt.Sprintf("instance %s started serving %q", name, dbName)})
	return nil
}

// Stop stops an instance, emitting StatusChanged on success.
func (s *Service) Stop(ctx context.Context, name string, emit Sink) error {
	inst, err := s.reg.Get(name)
	if err != nil {
		return err
	}
	if err := s.procFor(inst).Stop(); err != nil {
		return err
	}
	emit(Event{Kind: StatusChanged, Instance: name, Message: "instance " + name + " stopped"})
	return nil
}

// Restart restarts an instance, optionally switching the served database.
func (s *Service) Restart(ctx context.Context, name, dbName string, emit Sink) error {
	inst, err := s.reg.Get(name)
	if err != nil {
		return err
	}
	mgr := s.procFor(inst)
	var auto []string
	if inst.AutoUpdateOnRun && len(inst.AutoUpdateModules) > 0 {
		auto = inst.AutoUpdateModules
	}
	if dbName != "" {
		if err := s.requireDB(inst, dbName); err != nil {
			return err
		}
		if err := mgr.Stop(); err != nil {
			return err
		}
		if len(auto) > 0 {
			emit(Event{Kind: LogLine, Instance: name, Message: fmt.Sprintf("auto-update on run: -u %s", strings.Join(auto, ","))})
			if err := mgr.StartWithUpdate(dbName, auto); err != nil {
				return err
			}
		} else if err := mgr.Start(dbName); err != nil {
			return err
		}
	} else {
		if len(auto) > 0 {
			// auto-update requires Stop+Start with -u, not just Restart
			emit(Event{Kind: LogLine, Instance: name, Message: fmt.Sprintf("auto-update on run: -u %s", strings.Join(auto, ","))})
			if err := mgr.Stop(); err != nil {
				return err
			}
			// use current serving DB or primary
			curDB := mgr.ServingDB()
			if curDB == "" {
				curDB = inst.DBName
			}
			if err := mgr.StartWithUpdate(curDB, auto); err != nil {
				return err
			}
		} else if err := mgr.Restart(); err != nil {
			return err
		}
	}
	if err := waitForStable(ctx, mgr, s.logPath(inst)); err != nil {
		return err
	}
	emit(Event{Kind: StatusChanged, Instance: name, Message: "instance " + name + " restarted"})
	return nil
}

type AutoUpdateConfig struct {
	Modules []string `json:"modules"`
	Enabled bool     `json:"enabled"`
}

// SetAutoUpdate configures modules to -u on every Start/Restart.
func (s *Service) SetAutoUpdate(name string, modules []string, enabled bool) error {
	inst, err := s.reg.Get(name)
	if err != nil {
		return err
	}
	// normalize
	var norm []string
	for _, m := range modules {
		m = strings.TrimSpace(m)
		if m != "" {
			norm = append(norm, m)
		}
	}
	inst.AutoUpdateModules = norm
	inst.AutoUpdateOnRun = enabled && len(norm) > 0
	return s.reg.Put(inst)
}

// GetAutoUpdate returns auto-update config.
func (s *Service) GetAutoUpdate(name string) (*AutoUpdateConfig, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return nil, err
	}
	return &AutoUpdateConfig{Modules: inst.AutoUpdateModules, Enabled: inst.AutoUpdateOnRun}, nil
}

// SetPrimaryDatabase changes the primary DB (DBName) for an instance.
// It validates dbName is in AllDBs, moves old primary to Databases list, updates odoo.conf db_name.
func (s *Service) SetPrimaryDatabase(name, dbName string) error {
	inst, err := s.reg.Get(name)
	if err != nil {
		return err
	}
	if err := s.requireDB(inst, dbName); err != nil {
		// allow discovered DBs not yet tracked: check if DB exists and is not tracked elsewhere
		if exists, _ := s.pg.DatabaseExists(dbName); !exists {
			return err
		}
		// auto-track discovered
		inst.AddDB(dbName)
		_ = s.reg.Put(inst)
		// re-validate
		if err := s.requireDB(inst, dbName); err != nil {
			return err
		}
	}
	if dbName == inst.DBName {
		return nil // already primary
	}
	oldPrimary := inst.DBName
	// Remove new primary from additional list if present
	inst.RemoveDB(dbName)
	inst.DBName = dbName
	// Keep old primary as additional if not already (set DBName first: AddDB refuses name==DBName)
	if oldPrimary != "" && oldPrimary != dbName {
		inst.AddDB(oldPrimary)
	}
	if err := s.reg.Put(inst); err != nil {
		return err
	}
	// Update odoo.conf db_name
	p := inst.ResolvePaths(s.rootFor(inst))
	conf, err := odoconf.Load(p.Conf)
	if err == nil {
		conf.Set("db_name", dbName)
		_ = conf.Save()
	}
	return nil
}

// TrackDatabase adds an existing discovered DB to the instance's tracked list.
func (s *Service) TrackDatabase(name, dbName string) error {
	inst, err := s.reg.Get(name)
	if err != nil {
		return err
	}
	if err := db.IsValidName(dbName); err != nil {
		return err
	}
	if exists, err := s.pg.DatabaseExists(dbName); err != nil || !exists {
		return fmt.Errorf("database %q does not exist", dbName)
	}
	// check not already tracked by another instance
	if all, _ := s.reg.All(); all != nil {
		for _, other := range all {
			if other.Name == name {
				continue
			}
			for _, d := range other.AllDBs() {
				if d == dbName {
					return fmt.Errorf("database %q already tracked by instance %q", dbName, other.Name)
				}
			}
		}
	}
	if inst.AddDB(dbName) {
		return s.reg.Put(inst)
	}
	return nil // already tracked
}

// Conf returns the instance's odoo.conf (lossless view/edit).
func (s *Service) Conf(name string) (*odoconf.OdooConf, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return nil, err
	}
	p := inst.ResolvePaths(s.rootFor(inst))
	return odoconf.Load(p.Conf)
}

// ConfPath resolves the conf path of an instance without loading it.
func (s *Service) ConfPath(name string) (string, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return "", err
	}
	return inst.ResolvePaths(s.rootFor(inst)).Conf, nil
}

// requireStopped verifies an instance is not currently running (database
// operations and restores must not race a live server).
func (s *Service) requireStopped(name string) error {
	inst, err := s.reg.Get(name)
	if err != nil {
		return err
	}
	p := inst.ResolvePaths(s.rootFor(inst))
	mgr := proc.New(p, installer.PythonFor(inst, p), p.Conf, inst.LongpollPort)
	status, _, err := mgr.Status()
	if err != nil {
		return err
	}
	if status == instance.StatusRunning {
		return fmt.Errorf("instance %q is running — stop it first (odoonoir stop %s)", name, name)
	}
	return nil
}

func (s *Service) requireDB(inst *instance.Instance, name string) error {
	for _, d := range inst.AllDBs() {
		if d == name {
			return nil
		}
	}
	return fmt.Errorf("database %q is not served by instance %q", name, inst.Name)
}

func (s *Service) rootFor(inst *instance.Instance) string {
	if inst.Root != "" {
		return inst.Root
	}
	return s.cfg.InstancesDir()
}

func (s *Service) procFor(inst *instance.Instance) *proc.Manager {
	p := inst.ResolvePaths(s.rootFor(inst))
	return proc.New(p, installer.PythonFor(inst, p), p.Conf, inst.LongpollPort)
}

func (s *Service) logPath(inst *instance.Instance) string {
	return inst.ResolvePaths(s.rootFor(inst)).Log
}

// ReadConf reads all key-value pairs from the instance's odoo.conf.
func (s *Service) ReadConf(name string) ([]odoconf.Entry, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return nil, err
	}
	p := inst.ResolvePaths(s.rootFor(inst))
	c, err := odoconf.Load(p.Conf)
	if err != nil {
		return nil, err
	}
	return c.AllKeys(), nil
}

// SetConf updates a key in the instance's odoo.conf and saves it.
func (s *Service) SetConf(name string, key string, value string) error {
	inst, err := s.reg.Get(name)
	if err != nil {
		return err
	}
	p := inst.ResolvePaths(s.rootFor(inst))
	c, err := odoconf.Load(p.Conf)
	if err != nil {
		return err
	}
	c.Set(key, value)
	return c.Save()
}

// CreateConf generates a fresh odoo.conf for an instance from its registry data.
// It populates defaults and fills values from the instance record (port, db, paths).
func (s *Service) CreateConf(name string) error {
	inst, err := s.reg.Get(name)
	if err != nil {
		return err
	}
	p := inst.ResolvePaths(s.rootFor(inst))

	c := odoconf.New()
	c.Set("addons_path", "")
	if len(inst.AddonsPaths) > 0 {
		c.Set("addons_path", joinPaths(inst.AddonsPaths))
	} else {
		addons := installer.BuildAddonsPath(p)
		if len(addons) > 0 {
			c.Set("addons_path", joinPaths(addons))
		}
	}
	if p.DataDir != "" {
		c.Set("data_dir", p.DataDir)
	}
	c.Set("db_host", "localhost")
	c.Set("db_port", "5432")
	if inst.DBUser != "" {
		c.Set("db_user", inst.DBUser)
	}
	if inst.DBName != "" {
		c.Set("db_name", inst.DBName)
	}
	if inst.Port > 0 {
		c.Set("http_port", fmt.Sprint(inst.Port))
	}
	if inst.LongpollPort > 0 {
		c.Set("longpolling_port", fmt.Sprint(inst.LongpollPort))
	}
	if p.Log != "" {
		c.Set("logfile", p.Log)
	}
	if inst.Workers > 0 {
		c.Set("workers", fmt.Sprint(inst.Workers))
	}
	if inst.LogLevel != "" {
		c.Set("log_level", inst.LogLevel)
	}
	return c.Write(p.Conf)
}

func joinPaths(paths []string) string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = p
	}
	return strings.Join(out, ",")
}

// GitHub returns the GitHub client (may be nil if not configured).
func (s *Service) GitHub() *ghinternal.Client { return s.github }

// Instance returns the raw registry record for a name.
func (s *Service) Instance(name string) (*instance.Instance, error) {
	return s.reg.Get(name)
}

// PutInstance persists an instance record back to the registry.
func (s *Service) PutInstance(inst *instance.Instance) error {
	return s.reg.Put(inst)
}

// IsNotFound reports whether err is a registry miss.
func IsNotFound(err error) bool { return errors.Is(err, instance.ErrNotFound) }

// ═══════════════════════════════════════════════════════════════════════
// Backup Scheduler
// ══════════════════════════════════════════════════════════════════════

// BackupScheduleOptions configures automatic backup scheduling.
type BackupScheduleOptions struct {
	Enabled    bool     `json:"enabled"`
	Cron       string   `json:"cron"`        // cron expression (e.g. "0 2 * * *" for 2 AM daily)
	Databases  []string `json:"databases"`   // databases to backup (empty = all)
	Retain     int      `json:"retain"`      // number of backups to retain per DB
	Compress   bool     `json:"compress"`    // gzip compress
	Custom     bool     `json:"custom"`      // custom format (pg_dump -Fc)
}

// SetBackupSchedule enables/disables and configures automatic backup for an instance.
func (s *Service) SetBackupSchedule(name string, opts BackupScheduleOptions) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	inst, err := s.reg.Get(name)
	if err != nil {
		return err
	}

	// Remove existing job if any
	if inst.BackupScheduleEnabled {
		s.scheduler.Remove(cron.EntryID(inst.BackupScheduleID))
	}

	inst.BackupScheduleEnabled = opts.Enabled
	inst.BackupScheduleCron = opts.Cron
	inst.BackupScheduleDBs = opts.Databases
	inst.BackupScheduleRetain = opts.Retain
	inst.BackupScheduleCompress = opts.Compress
	inst.BackupScheduleCustom = opts.Custom

	if opts.Enabled && opts.Cron != "" {
		entryID, err := s.scheduler.AddFunc(opts.Cron, func() {
			s.runScheduledBackup(name)
		})
		if err != nil {
			return fmt.Errorf("invalid cron expression: %w", err)
		}
		inst.BackupScheduleID = int(entryID)
		// Calculate next run
		entry := s.scheduler.Entry(cron.EntryID(entryID))
		next := entry.Next
		inst.BackupScheduleNextRun = &next
	} else {
		inst.BackupScheduleID = 0
		inst.BackupScheduleNextRun = nil
	}

	now := time.Now()
	inst.UpdatedAt = now
	return s.reg.Put(inst)
}

// GetBackupSchedule returns the current backup schedule for an instance.
func (s *Service) GetBackupSchedule(name string) (*BackupScheduleOptions, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return nil, err
	}
	return &BackupScheduleOptions{
		Enabled:   inst.BackupScheduleEnabled,
		Cron:      inst.BackupScheduleCron,
		Databases: inst.BackupScheduleDBs,
		Retain:    inst.BackupScheduleRetain,
		Compress:  inst.BackupScheduleCompress,
		Custom:    inst.BackupScheduleCustom,
	}, nil
}

// runScheduledBackup executes the backup for all configured databases.
func (s *Service) runScheduledBackup(name string) {
	s.mu.Lock()
	inst, err := s.reg.Get(name)
	s.mu.Unlock()
	if err != nil {
		return
	}

	if !inst.BackupScheduleEnabled || inst.BackupScheduleCron == "" {
		return
	}

	dbs := inst.BackupScheduleDBs
	if len(dbs) == 0 {
		dbs = inst.AllDBs()
	}

	retain := inst.BackupScheduleRetain
	if retain <= 0 {
		retain = 7
	}

	p := inst.ResolvePaths(s.rootFor(inst))
	for _, dbName := range dbs {
		// Create backup filename
		timestamp := time.Now().Format("20060102_150405")
		ext := ".sql"
		if inst.BackupScheduleCustom {
			ext = ".dump"
		}
		if inst.BackupScheduleCompress {
			ext += ".gz"
		}
		backupDir := filepath.Join(p.Root, "backups", "scheduled")
		if err := os.MkdirAll(backupDir, 0o755); err != nil {
			continue
		}
		backupPath := filepath.Join(backupDir, fmt.Sprintf("%s_%s%s", dbName, timestamp, ext))

		// Run backup
		ctx := context.Background()
		err := s.pg.Backup(ctx, dbName, backupPath, inst.BackupScheduleCustom, inst.BackupScheduleCompress)
		if err != nil {
			continue
		}

		// Cleanup old backups
		s.cleanupOldBackups(backupDir, dbName, retain)
	}

	// Update last run time
	s.mu.Lock()
	inst2, err := s.reg.Get(name)
	if err != nil {
		s.mu.Unlock()
		return
	}
	now := time.Now()
	inst2.BackupScheduleLastRun = &now
	// Update next run
	if inst2.BackupScheduleEnabled && inst2.BackupScheduleCron != "" {
		entry := s.scheduler.Entry(cron.EntryID(inst2.BackupScheduleID))
		next := entry.Next
		inst2.BackupScheduleNextRun = &next
	}
	inst2.UpdatedAt = now
	_ = s.reg.Put(inst2)
	s.mu.Unlock()
}

// cleanupOldBackups removes old backup files, keeping only the most recent N.
func (s *Service) cleanupOldBackups(backupDir, dbPrefix string, retain int) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return
	}
	var files []fs.DirEntry
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), dbPrefix+"_") {
			files = append(files, e)
		}
	}
	if len(files) <= retain {
		return
	}
	sort.Slice(files, func(i, j int) bool {
		ii, _ := files[i].Info()
		jj, _ := files[j].Info()
		return ii.ModTime().After(jj.ModTime())
	})
	for _, f := range files[retain:] {
		_ = os.Remove(filepath.Join(backupDir, f.Name()))
	}
}

// BackupScheduleStatus represents the status of scheduled backups.
type BackupScheduleStatus struct {
	Enabled     bool       `json:"enabled"`
	Cron        string     `json:"cron"`
	Databases   []string   `json:"databases"`
	Retain      int        `json:"retain"`
	Compress    bool       `json:"compress"`
	Custom      bool       `json:"custom"`
	LastRun     *time.Time `json:"lastRun"`
	NextRun     *time.Time `json:"nextRun"`
	EntryID     int        `json:"entryId"`
}

// GetBackupScheduleStatus returns the current status of scheduled backups.
func (s *Service) GetBackupScheduleStatus(name string) (*BackupScheduleStatus, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return nil, err
	}
	return &BackupScheduleStatus{
		Enabled:   inst.BackupScheduleEnabled,
		Cron:      inst.BackupScheduleCron,
		Databases: inst.BackupScheduleDBs,
		Retain:    inst.BackupScheduleRetain,
		Compress:  inst.BackupScheduleCompress,
		Custom:    inst.BackupScheduleCustom,
		LastRun:   inst.BackupScheduleLastRun,
		NextRun:   inst.BackupScheduleNextRun,
		EntryID:   inst.BackupScheduleID,
	}, nil
}

// ListScheduledBackups lists backup files for an instance.
func (s *Service) ListScheduledBackups(name string) ([]BackupFileInfo, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return nil, err
	}
	p := inst.ResolvePaths(s.rootFor(inst))
	backupDir := filepath.Join(p.Root, "backups", "scheduled")
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []BackupFileInfo{}, nil
		}
		return nil, err
	}
	var files []BackupFileInfo
	for _, e := range entries {
		info, _ := e.Info()
		files = append(files, BackupFileInfo{
			Name:    e.Name(),
			Path:    filepath.Join(backupDir, e.Name()),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime.After(files[j].ModTime)
	})
	return files, nil
}

// BackupFileInfo represents a backup file.
type BackupFileInfo struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modTime"`
}

// RunBackupNow triggers an immediate backup for the scheduled databases.
func (s *Service) RunBackupNow(name string) error {
	s.mu.Lock()
	inst, err := s.reg.Get(name)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if !inst.BackupScheduleEnabled {
		return fmt.Errorf("backup scheduling not enabled for %s", name)
	}
	go s.runScheduledBackup(name)
	return nil
}

// ═══════════════════════════════════════════════════════════════════════
// Multi-instance Dashboard
// ══════════════════════════════════════════════════════════════════════

// DashboardMetrics represents aggregate metrics across all instances.
type DashboardMetrics struct {
	TotalInstances     int                    `json:"totalInstances"`
	RunningInstances   int                    `json:"runningInstances"`
	StoppedInstances   int                    `json:"stoppedInstances"`
	TotalDatabases     int                    `json:"totalDatabases"`
	TotalSizeBytes     int64                  `json:"totalSizeBytes"`
	HostCPU            float64                `json:"hostCPU"`
	HostMemPercent     float64                `json:"hostMemPercent"`
	HostDiskPercent    float64                `json:"hostDiskPercent"`
	InstanceMetrics    []InstanceMetric       `json:"instanceMetrics"`
	Alerts             []DashboardAlert       `json:"alerts"`
}

// InstanceMetric represents metrics for a single instance.
type InstanceMetric struct {
	Name         string     `json:"name"`
	Version      string     `json:"version"`
	Status       string     `json:"status"`
	Port         int        `json:"port"`
	Databases    int        `json:"databases"`
	SizeBytes    int64      `json:"sizeBytes"`
	Uptime       float64    `json:"uptime"`       // seconds
	CPUPercent   float64    `json:"cpuPercent"`   // placeholder
	MemoryMB     float64    `json:"memoryMB"`     // placeholder
	LastBackup   *time.Time `json:"lastBackup"`
	NextBackup   *time.Time `json:"nextBackup"`
	HealthScore  int        `json:"healthScore"`  // 0-100
}

// DashboardAlert represents an alert for the dashboard.
type DashboardAlert struct {
	Severity   string    `json:"severity"`   // "critical", "warning", "info"
	Instance   string    `json:"instance"`
	Message    string    `json:"message"`
	Timestamp  time.Time `json:"timestamp"`
}

// GetDashboardMetrics returns aggregate metrics across all instances.
func (s *Service) GetDashboardMetrics() (*DashboardMetrics, error) {
	instances, err := s.reg.All()
	if err != nil {
		return nil, err
	}

	var metrics DashboardMetrics
	metrics.TotalInstances = len(instances)
	metrics.InstanceMetrics = make([]InstanceMetric, 0, len(instances))
	// Host live metrics
	if percents, err := cpu.Percent(0, false); err == nil && len(percents) > 0 {
		metrics.HostCPU = percents[0]
	}
	if vm, err := mem.VirtualMemory(); err == nil {
		metrics.HostMemPercent = vm.UsedPercent
	}
	if du, err := disk.Usage("/"); err == nil {
		metrics.HostDiskPercent = du.UsedPercent
	}

	for _, inst := range instances {
		p := inst.ResolvePaths(s.rootFor(inst))
		mgr := proc.New(p, installer.PythonFor(inst, p), p.Conf, inst.LongpollPort)
		status, pid, _ := mgr.Status()

		dbCount := len(inst.AllDBs())
		var sizeBytes int64
		for _, dbName := range inst.AllDBs() {
			if sz, err := s.pg.DatabaseSize(dbName); err == nil {
				sizeBytes += sz
			}
		}
		metrics.TotalDatabases += dbCount
		metrics.TotalSizeBytes += sizeBytes

		running := status == instance.StatusRunning
		if running {
			metrics.RunningInstances++
		} else {
			metrics.StoppedInstances++
		}

		// Calculate uptime
		var uptime float64
		if running {
			// Try to get process start time from /proc
			if pid > 0 {
				if data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)); err == nil {
					fields := strings.Fields(string(data))
					if len(fields) > 21 {
						if startTime, err := strconv.ParseInt(fields[21], 10, 64); err == nil {
							// Convert to seconds since boot
							uptime = time.Since(time.Unix(startTime/100, 0)).Seconds()
						}
					}
				}
			}
		}

		// Live CPU/Memory per instance (if running)
		var cpuPct float64
		var memMB float64
		if pid > 0 {
			if proc, err := process.NewProcess(int32(pid)); err == nil {
				if pct, err := proc.CPUPercent(); err == nil {
					cpuPct = pct
				}
				if mi, err := proc.MemoryInfo(); err == nil && mi != nil {
					memMB = float64(mi.RSS) / 1024 / 1024
				}
			}
		}

		// Health score calculation
		healthScore := 100
		if !running {
			healthScore = 0
		} else if pid == 0 {
			healthScore = 50
		}
		// Reduce score if no recent backup
		if inst.BackupScheduleEnabled && inst.BackupScheduleLastRun != nil {
			if time.Since(*inst.BackupScheduleLastRun) > 24*time.Hour {
				healthScore -= 20
			}
		} else if inst.BackupScheduleEnabled {
			healthScore -= 30
		}
		// live thresholds
		if cpuPct > 80 {
			healthScore -= 10
		}
		if memMB > 800 {
			healthScore -= 10
		}
		if healthScore < 0 {
			healthScore = 0
		}

	im := InstanceMetric{
		Name:        inst.Name,
		Version:     inst.Version,
		Status:      string(status),
		Port:        inst.Port,
		Databases:   dbCount,
		SizeBytes:   sizeBytes,
		Uptime:      uptime,
		CPUPercent:  cpuPct,
		MemoryMB:    memMB,
		LastBackup:  inst.BackupScheduleLastRun,
		NextBackup:  inst.BackupScheduleNextRun,
		HealthScore: healthScore,
	}
		metrics.InstanceMetrics = append(metrics.InstanceMetrics, im)
	}

	// Generate alerts (live)
	if metrics.HostCPU > 80 {
		metrics.Alerts = append(metrics.Alerts, DashboardAlert{Severity: "warning", Instance: "_host", Message: fmt.Sprintf("Host CPU high %.1f%%", metrics.HostCPU), Timestamp: time.Now()})
	}
	if metrics.HostMemPercent > 80 {
		metrics.Alerts = append(metrics.Alerts, DashboardAlert{Severity: "warning", Instance: "_host", Message: fmt.Sprintf("Host memory high %.1f%%", metrics.HostMemPercent), Timestamp: time.Now()})
	}
	if metrics.HostDiskPercent > 80 {
		metrics.Alerts = append(metrics.Alerts, DashboardAlert{Severity: "critical", Instance: "_host", Message: fmt.Sprintf("Host disk high %.1f%%", metrics.HostDiskPercent), Timestamp: time.Now()})
	}
	for _, im := range metrics.InstanceMetrics {
		if im.HealthScore < 30 {
			metrics.Alerts = append(metrics.Alerts, DashboardAlert{
				Severity:  "critical",
				Instance:  im.Name,
				Message:   fmt.Sprintf("Instance %s is unhealthy (score: %d)", im.Name, im.HealthScore),
				Timestamp: time.Now(),
			})
		} else if im.HealthScore < 70 {
			metrics.Alerts = append(metrics.Alerts, DashboardAlert{
				Severity:  "warning",
				Instance:  im.Name,
				Message:   fmt.Sprintf("Instance %s needs attention (score: %d)", im.Name, im.HealthScore),
				Timestamp: time.Now(),
			})
		}
		if im.CPUPercent > 80 {
			metrics.Alerts = append(metrics.Alerts, DashboardAlert{Severity: "warning", Instance: im.Name, Message: fmt.Sprintf("CPU high %.1f%%", im.CPUPercent), Timestamp: time.Now()})
		}
		if im.MemoryMB > 800 {
			metrics.Alerts = append(metrics.Alerts, DashboardAlert{Severity: "warning", Instance: im.Name, Message: fmt.Sprintf("Memory high %.0f MB", im.MemoryMB), Timestamp: time.Now()})
		}
		// Check for no recent backup
		if im.LastBackup != nil && time.Since(*im.LastBackup) > 24*time.Hour {
			metrics.Alerts = append(metrics.Alerts, DashboardAlert{
				Severity:  "warning",
				Instance:  im.Name,
				Message:   fmt.Sprintf("Instance %s hasn't been backed up in over 24 hours", im.Name),
				Timestamp: time.Now(),
			})
		} else if im.NextBackup != nil && time.Until(*im.NextBackup) < 0 {
			metrics.Alerts = append(metrics.Alerts, DashboardAlert{
				Severity:  "warning",
				Instance:  im.Name,
				Message:   fmt.Sprintf("Instance %s backup is overdue", im.Name),
				Timestamp: time.Now(),
			})
		}
	}

	return &metrics, nil
}

// ═══════════════════════════════════════════════════════════════════════
// GitHub Module Support
// ═══════════════════════════════════════════════════════════════════════

// GitHubSearchResult represents the result of a module search
type GitHubSearchResult struct {
	Modules    []ghinternal.Module       `json:"modules"`
	TotalCount int                   `json:"totalCount"`
	Page       int                   `json:"page"`
	PerPage    int                   `json:"perPage"`
	HasMore    bool                  `json:"hasMore"`
}

// GitHubModuleDetail represents detailed module information
type GitHubModuleDetail struct {
	Module       ghinternal.Module           `json:"module"`
	Manifest     *ghinternal.Manifest        `json:"manifest"`
	SubModules   []ghinternal.SubModule      `json:"subModules"`
	Readme       string                  `json:"readme"`
	Releases     []ghinternal.Release        `json:"releases"`
	Dependencies []ghinternal.Dependency     `json:"dependencies"`
}

// GitHubSearchOptions configures module search
type GitHubSearchOptions struct {
	Query        string
	OdooVersion  string
	MinStars     int
	Category     string
	Language     string
	UpdatedAfter string
	Sort         string
	Order        string
	Page         int
	PerPage      int
}

// GitHubInstallOptions configures module installation
type GitHubInstallOptions struct {
	Owner     string
	Repo      string
	Branch    string
	Instance  string
	DBName    string
	AutoDeps  bool
	RunTests  bool
}

// GitHubPublishOptions configures module publishing
type GitHubPublishOptions struct {
	ModulePath    string
	Owner         string
	Repo          string
	Private       bool
	License       string
	Description   string
	Topics        []string
	CreateActions bool
}

// SearchGitHubModules searches for Odoo modules on GitHub
func (s *Service) SearchGitHubModules(ctx context.Context, opts GitHubSearchOptions) (*GitHubSearchResult, error) {
	s.mu.Lock()
	gh := s.github
	s.mu.Unlock()
	if gh == nil {
		return nil, fmt.Errorf("GitHub client not configured - please connect your GitHub account")
	}
	query := ghinternal.SearchQuery{
		Query:        opts.Query,
		OdooVersion:  opts.OdooVersion,
		MinStars:     opts.MinStars,
		Category:     opts.Category,
		Language:     opts.Language,
		UpdatedAfter: opts.UpdatedAfter,
		Sort:         opts.Sort,
		Order:        opts.Order,
		Page:         opts.Page,
		PerPage:      opts.PerPage,
	}
	result, err := gh.SearchModules(ctx, query)
	if err != nil {
		return nil, err
	}
	return &GitHubSearchResult{
		Modules:    result.Modules,
		TotalCount: result.TotalCount,
		Page:       result.Page,
		PerPage:    result.PerPage,
		HasMore:    result.HasMore,
	}, nil
}

// GetGitHubModuleDetail fetches detailed information about a module
func (s *Service) GetGitHubModuleDetail(ctx context.Context, owner, repo string) (*GitHubModuleDetail, error) {
	s.mu.Lock()
	gh := s.github
	s.mu.Unlock()
	if gh == nil {
		return nil, fmt.Errorf("GitHub client not configured")
	}
	detail, err := gh.GetModule(context.Background(), owner, repo)
	if err != nil {
		return nil, err
	}
	return &GitHubModuleDetail{
		Module:       detail.Module,
		Manifest:     detail.Manifest,
		SubModules:   detail.SubModules,
		Readme:       detail.Readme,
		Releases:     detail.Releases,
		Dependencies: detail.Dependencies,
	}, nil
}

// InstallGitHubModule installs a module from GitHub
func (s *Service) InstallGitHubModule(ctx context.Context, opts GitHubInstallOptions) (*ghinternal.InstallResult, error) {
	s.mu.Lock()
	gh := s.github
	s.mu.Unlock()
	if gh == nil {
		return nil, fmt.Errorf("GitHub client not configured")
	}
	inst, err := s.reg.Get(opts.Instance)
	if err != nil {
		return nil, err
	}
	spec := ghinternal.ModuleSpec{
		Owner:     opts.Owner,
		Repo:      opts.Repo,
		Branch:    opts.Branch,
		TargetDir: filepath.Join(s.cfg.InstancesDir(), "github_modules", opts.Owner, opts.Repo),
		Instance:  inst,
		DBName:    opts.DBName,
		AutoDeps:  opts.AutoDeps,
		RunTests:  opts.RunTests,
	}
	return gh.InstallModule(ctx, spec)
}

// PublishGitHubModule publishes a local module to GitHub
func (s *Service) PublishGitHubModule(ctx context.Context, opts GitHubPublishOptions) error {
	s.mu.Lock()
	gh := s.github
	s.mu.Unlock()
	if gh == nil {
		return fmt.Errorf("GitHub client not configured")
	}
	return gh.PublishModule(ctx, ghinternal.PublishOptions{
		ModulePath:   opts.ModulePath,
		Owner:        opts.Owner,
		Repo:         opts.Repo,
		Private:      opts.Private,
		License:      opts.License,
		Description:  opts.Description,
		Topics:       opts.Topics,
		CreateActions: opts.CreateActions,
	})
}

// SyncGitHubModule syncs a module with its GitHub repository
func (s *Service) SyncGitHubModule(ctx context.Context, instanceName, owner, repo, branch string) (*ghinternal.SyncResult, error) {
	s.mu.Lock()
	gh := s.github
	s.mu.Unlock()
	if gh == nil {
		return nil, fmt.Errorf("GitHub client not configured")
	}
	inst, err := s.reg.Get(instanceName)
	if err != nil {
		return nil, err
	}
	spec := ghinternal.ModuleSpec{
		Owner:    owner,
		Repo:     repo,
		Branch:   branch,
		Instance: inst,
	}
	return gh.SyncModule(ctx, spec)
}

// SetGitHubToken sets the GitHub OAuth token
func (s *Service) SetGitHubToken(ctx context.Context, token string) error {
	tokenStore, err := ghinternal.NewFileTokenStore()
	if err != nil {
		return fmt.Errorf("open token store: %w", err)
	}
	if err := tokenStore.Set(token); err != nil {
		return err
	}
	s.mu.Lock()
	s.github = ghinternal.NewClient(token)
	s.mu.Unlock()
	return nil
}

// GetGitHubToken returns the current GitHub token
func (s *Service) GetGitHubToken(ctx context.Context) (string, error) {
	tokenStore, err := ghinternal.NewFileTokenStore()
	if err != nil {
		return "", fmt.Errorf("open token store: %w", err)
	}
	return tokenStore.Get()
}

// ClearGitHubToken removes the GitHub token
func (s *Service) ClearGitHubToken(ctx context.Context) error {
	tokenStore, err := ghinternal.NewFileTokenStore()
	if err != nil {
		return fmt.Errorf("open token store: %w", err)
	}
	if err := tokenStore.Delete(); err != nil {
		return err
	}
	s.mu.Lock()
	s.github = nil
	s.mu.Unlock()
	return nil
}

// ValidateGitHubToken validates the current GitHub token
func (s *Service) ValidateGitHubToken(ctx context.Context) (interface{}, error) {
	s.mu.Lock()
	gh := s.github
	s.mu.Unlock()
	if gh == nil {
		return nil, fmt.Errorf("no GitHub token configured")
	}
	return gh.ValidateToken(ctx)
}

// GetGitHubTokenScopes returns the scopes of the current GitHub token
func (s *Service) GetGitHubTokenScopes(ctx context.Context) ([]string, error) {
	s.mu.Lock()
	gh := s.github
	s.mu.Unlock()
	if gh == nil {
		return nil, fmt.Errorf("no GitHub token configured")
	}
	return gh.GetTokenScopes(ctx)
}

// ════════════════════════════════════════════════════════════════════════
// Remote Instance Management (SSH)
// ═══════════════════════════════════════════════════════════════════════

// SSHConfig holds SSH connection configuration for a remote instance
type SSHConfig struct {
	Name     string
	Host     string
	Port     int
	User     string
	KeyPath  string
	Password string
	Timeout  time.Duration
}

// ConnectSSH establishes an SSH connection to a remote instance
func (s *Service) ConnectSSH(ctx context.Context, cfg SSHConfig) (*ssh.Client, error) {
	s.sshMu.Lock()
	defer s.sshMu.Unlock()

	if existing, ok := s.sshClients[cfg.Name]; ok {
		// Test if connection is still alive
		if existing.IsAlive() {
			return existing, nil
		}
		existing.Close()
		delete(s.sshClients, cfg.Name)
	}

	client, err := ssh.NewClient(ssh.Config{
		Host:     cfg.Host,
		Port:      cfg.Port,
		User:      cfg.User,
		KeyPath:   cfg.KeyPath,
		Password:  cfg.Password,
		Timeout:   cfg.Timeout,
	})
	if err != nil {
		return nil, err
	}

	s.sshClients[cfg.Name] = client
	return client, nil
}

// DisconnectSSH closes the SSH connection to a remote instance
func (s *Service) DisconnectSSH(name string) error {
	s.sshMu.Lock()
	defer s.sshMu.Unlock()

	if client, ok := s.sshClients[name]; ok {
		client.Close()
		delete(s.sshClients, name)
	}
	return nil
}

// GetSSHClient returns an existing SSH client
func (s *Service) GetSSHClient(name string) (*ssh.Client, bool) {
	s.sshMu.Lock()
	defer s.sshMu.Unlock()
	client, ok := s.sshClients[name]
	return client, ok
}

// ListSSHConnections returns all active SSH connections
func (s *Service) ListSSHConnections() []string {
	s.sshMu.Lock()
	defer s.sshMu.Unlock()

	names := make([]string, 0, len(s.sshClients))
	for name := range s.sshClients {
		names = append(names, name)
	}
	return names
}

// ExecuteRemoteCommand executes a command on a remote instance
func (s *Service) ExecuteRemoteCommand(ctx context.Context, name, cmd string) (string, error) {
	client, ok := s.GetSSHClient(name)
	if !ok {
		return "", fmt.Errorf("no SSH connection to %s", name)
	}
	return client.RunCommand(ctx, cmd)
}

// GetRemoteInstanceInfo gathers information about a remote instance
func (s *Service) GetRemoteInstanceInfo(ctx context.Context, name string) (*ssh.RemoteInfo, error) {
	client, ok := s.GetSSHClient(name)
	if !ok {
		return nil, fmt.Errorf("no SSH connection to %s", name)
	}
	return client.GetRemoteInfo(ctx)
}

// StartRemoteTunnel starts an SSH tunnel to a remote instance
func (s *Service) StartRemoteTunnel(ctx context.Context, name string, localPort int, remoteHost string, remotePort int) error {
	client, ok := s.GetSSHClient(name)
	if !ok {
		return fmt.Errorf("no SSH connection to %s", name)
	}
	return client.StartTunnel(ctx, localPort, remoteHost, remotePort)
}

// StopRemoteTunnel stops an SSH tunnel
func (s *Service) StopRemoteTunnel(name string, localPort int, remoteHost string, remotePort int) error {
	client, ok := s.GetSSHClient(name)
	if !ok {
		return fmt.Errorf("no SSH connection to %s", name)
	}
	return client.StopTunnel(localPort, remoteHost, remotePort)
}

// ExecuteRemoteCommandWithOutput executes a command and streams output
func (s *Service) ExecuteRemoteCommandWithOutput(ctx context.Context, name, cmd string, stdout, stderr io.Writer) error {
	client, ok := s.GetSSHClient(name)
	if !ok {
		return fmt.Errorf("no SSH connection to %s", name)
	}
	return client.RunCommandWithOutput(ctx, cmd, stdout, stderr)
}

// CopyFileToRemote copies a file to a remote instance
func (s *Service) CopyFileToRemote(ctx context.Context, name, src, dst string) error {
	client, ok := s.GetSSHClient(name)
	if !ok {
		return fmt.Errorf("no SSH connection to %s", name)
	}
	return client.CopyFile(ctx, src, dst, true)
}

// CopyFileFromRemote copies a file from a remote instance
func (s *Service) CopyFileFromRemote(ctx context.Context, name, src, dst string) error {
	client, ok := s.GetSSHClient(name)
	if !ok {
		return fmt.Errorf("no SSH connection to %s", name)
	}
	return client.CopyFile(ctx, src, dst, false)
}

// ReadRemoteFile reads a file from a remote instance
func (s *Service) ReadRemoteFile(ctx context.Context, name, path string) (string, error) {
	client, ok := s.GetSSHClient(name)
	if !ok {
		return "", fmt.Errorf("no SSH connection to %s", name)
	}
	return client.ReadFile(ctx, path)
}

// WriteRemoteFile writes a file to a remote instance
func (s *Service) WriteRemoteFile(ctx context.Context, name, path, content string) error {
	client, ok := s.GetSSHClient(name)
	if !ok {
		return fmt.Errorf("no SSH connection to %s", name)
	}
	return client.WriteFile(ctx, path, content)
}

// RemoteFileExists checks if a file exists on a remote instance
func (s *Service) RemoteFileExists(ctx context.Context, name, path string) (bool, error) {
	client, ok := s.GetSSHClient(name)
	if !ok {
		return false, fmt.Errorf("no SSH connection to %s", name)
	}
	return client.FileExists(ctx, path)
}

// ListRemoteDir lists a directory on a remote instance
func (s *Service) ListRemoteDir(ctx context.Context, name, path string) ([]string, error) {
	client, ok := s.GetSSHClient(name)
	if !ok {
		return nil, fmt.Errorf("no SSH connection to %s", name)
	}
	return client.ListDir(ctx, path)
}

// ============================================
// Marketplace
// ════════════════════════════════════════════════════════════════════════

// SearchMarketplaceModules searches for modules in the marketplace
func (s *Service) SearchMarketplaceModules(ctx context.Context, query string, opts marketplace.ModuleFilter) ([]marketplace.MarketplaceModule, error) {
	return s.marketplace.SearchModules(query, opts)
}

// GetMarketplaceModule returns a module by ID
func (s *Service) GetMarketplaceModule(id string) (*marketplace.MarketplaceModule, error) {
	return s.marketplace.GetModule(id)
}

// ListMarketplaceModules lists modules with filtering
func (s *Service) ListMarketplaceModules(opts marketplace.ModuleFilter) ([]marketplace.MarketplaceModule, error) {
	return s.marketplace.ListModules(opts)
}

// GetMarketplaceModuleReviews returns reviews for a module
func (s *Service) GetMarketplaceModuleReviews(moduleID string) ([]marketplace.Review, error) {
	return s.marketplace.GetModuleReviews(moduleID)
}

// AddMarketplaceReview adds a review to a module
func (s *Service) AddMarketplaceReview(review *marketplace.Review) error {
	return s.marketplace.AddReview(review)
}

// GetMarketplaceModuleRating returns rating summary for a module
func (s *Service) GetMarketplaceModuleRating(moduleID string) (*marketplace.RatingSummary, error) {
	return s.marketplace.GetModuleRating(moduleID)
}

// GetMarketplaceStats returns marketplace statistics
func (s *Service) GetMarketplaceStats() (*marketplace.MarketplaceStats, error) {
	return s.marketplace.GetStats()
}

// GetFeaturedModules returns featured modules
func (s *Service) GetFeaturedModules(limit int) ([]marketplace.MarketplaceModule, error) {
	return s.marketplace.GetFeaturedModules(limit)
}

// GetVerifiedModules returns verified modules
func (s *Service) GetVerifiedModules(limit int) ([]marketplace.MarketplaceModule, error) {
	return s.marketplace.GetVerifiedModules(limit)
}

// GetTopRatedModules returns top-rated modules
func (s *Service) GetTopRatedModules(limit int) ([]marketplace.MarketplaceModule, error) {
	return s.marketplace.GetTopRatedModules(limit)
}

// GetMostInstalledModules returns most installed modules
func (s *Service) GetMostInstalledModules(limit int) ([]marketplace.MarketplaceModule, error) {
	return s.marketplace.GetMostInstalledModules(limit)
}

// IncrementMarketplaceInstallCount increments the install count for a module
func (s *Service) IncrementMarketplaceInstallCount(moduleID string) error {
	return s.marketplace.IncrementInstallCount(moduleID)
}

// IndexMarketplaceModule indexes a module from GitHub
func (s *Service) IndexMarketplaceModule(ctx context.Context, owner, repo string) (*marketplace.MarketplaceModule, error) {
	return s.marketplace.IndexModule(ctx, owner, repo)
}

// ════════════════════════════════════════════════════════════════════════
// CI/CD Integration
// ═══════════════════════════════════════════════════════════════════════

// ListWorkflows lists all workflow files in a repository
func (s *Service) ListWorkflows(ctx context.Context, owner, repo string) ([]ci.WorkflowFile, error) {
	if s.ci == nil {
		return nil, fmt.Errorf("CI service not configured")
	}
	return s.ci.ListWorkflows(ctx, owner, repo)
}

// GetWorkflow gets a specific workflow by ID
func (s *Service) GetWorkflow(ctx context.Context, owner, repo string, workflowID int64) (*ci.WorkflowFile, error) {
	if s.ci == nil {
		return nil, fmt.Errorf("CI service not configured")
	}
	return s.ci.GetWorkflow(ctx, owner, repo, workflowID)
}

// EnableWorkflow enables a workflow
func (s *Service) EnableWorkflow(ctx context.Context, owner, repo string, workflowID int64) error {
	if s.ci == nil {
		return fmt.Errorf("CI service not configured")
	}
	return s.ci.EnableWorkflow(ctx, owner, repo, workflowID)
}

// DisableWorkflow disables a workflow
func (s *Service) DisableWorkflow(ctx context.Context, owner, repo string, workflowID int64) error {
	if s.ci == nil {
		return fmt.Errorf("CI service not configured")
	}
	return s.ci.DisableWorkflow(ctx, owner, repo, workflowID)
}

// CreateWorkflowDispatch creates a workflow dispatch event
func (s *Service) CreateWorkflowDispatch(ctx context.Context, owner, repo string, workflowID int64, ref string, inputs map[string]interface{}) error {
	if s.ci == nil {
		return fmt.Errorf("CI service not configured")
	}
	return s.ci.CreateWorkflowDispatch(ctx, owner, repo, workflowID, ref, inputs)
}

// ListWorkflowRuns lists recent workflow runs
func (s *Service) ListWorkflowRuns(ctx context.Context, owner, repo string, workflowID int64, opts *github.ListWorkflowRunsOptions) ([]ci.WorkflowRun, error) {
	if s.ci == nil {
		return nil, fmt.Errorf("CI service not configured")
	}
	return s.ci.ListWorkflowRuns(ctx, owner, repo, workflowID, opts)
}

// ListRepositoryWorkflowRuns lists all recent workflow runs for a repository
func (s *Service) ListRepositoryWorkflowRuns(ctx context.Context, owner, repo string, opts *github.ListWorkflowRunsOptions) ([]ci.WorkflowRun, error) {
	if s.ci == nil {
		return nil, fmt.Errorf("CI service not configured")
	}
	return s.ci.ListRepositoryWorkflowRuns(ctx, owner, repo, opts)
}

// GetWorkflowRun gets a specific workflow run
func (s *Service) GetWorkflowRun(ctx context.Context, owner, repo string, runID int64) (*ci.WorkflowRun, error) {
	if s.ci == nil {
		return nil, fmt.Errorf("CI service not configured")
	}
	return s.ci.GetWorkflowRun(ctx, owner, repo, runID)
}

// ReRunWorkflow re-runs a workflow (creates a new dispatch)
func (s *Service) ReRunWorkflow(ctx context.Context, owner, repo string, runID int64) error {
	if s.ci == nil {
		return fmt.Errorf("CI service not configured")
	}
	return s.ci.ReRunWorkflow(ctx, owner, repo, runID)
}

// CancelWorkflowRun cancels a workflow run
func (s *Service) CancelWorkflowRun(ctx context.Context, owner, repo string, runID int64) error {
	if s.ci == nil {
		return fmt.Errorf("CI service not configured")
	}
	return s.ci.CancelWorkflowRun(ctx, owner, repo, runID)
}

// ListJobsForWorkflowRun lists jobs for a workflow run
func (s *Service) ListJobsForWorkflowRun(ctx context.Context, owner, repo string, runID int64, opts *github.ListWorkflowJobsOptions) ([]ci.Job, error) {
	if s.ci == nil {
		return nil, fmt.Errorf("CI service not configured")
	}
	return s.ci.ListJobsForWorkflowRun(ctx, owner, repo, runID, opts)
}

// GetJobLogs gets logs for a job
func (s *Service) GetJobLogs(ctx context.Context, owner, repo string, jobID int64, maxRedirects int) (string, error) {
	if s.ci == nil {
		return "", fmt.Errorf("CI service not configured")
	}
	return s.ci.GetJobLogs(ctx, owner, repo, jobID, maxRedirects)
}

// GetRepositoryStatus gets the overall CI/CD status for a repository
func (s *Service) GetRepositoryStatus(ctx context.Context, owner, repo string) (*ci.RepositoryStatus, error) {
	if s.ci == nil {
		return nil, fmt.Errorf("CI service not configured")
	}
	return s.ci.GetRepositoryStatus(ctx, owner, repo)
}

// GetWorkflowFileContent gets the content of a workflow file
func (s *Service) GetWorkflowFileContent(ctx context.Context, owner, repo, path string) (string, error) {
	if s.ci == nil {
		return "", fmt.Errorf("CI service not configured")
	}
	return s.ci.GetWorkflowFileContent(ctx, owner, repo, path)
}

// OpenInVSCode opens the instance root in VS Code (or Cursor) if available.
func (s *Service) OpenInVSCode(name string, editor string) (string, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return "", err
	}
	p := inst.ResolvePaths(s.rootFor(inst))
	// Prefer instance root, fallback to custom_addons
	target := p.Root
	if _, err := os.Stat(target); err != nil {
		target = p.Addons
	}
	candidates := []string{}
	if editor != "" {
		candidates = append(candidates, editor)
	}
	candidates = append(candidates, "code", "code-insiders", "cursor", "codium")
	var bin string
	for _, c := range candidates {
		if _, err := exec.LookPath(c); err == nil {
			bin = c
			break
		}
	}
	if bin == "" {
		return "", fmt.Errorf("no editor found (tried %s) — install VS Code or set editor explicitly", strings.Join(candidates, ", "))
	}
	cmd := exec.Command(bin, target)
	cmd.Dir = target
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("open in %s: %w", bin, err)
	}
	return bin + " " + target, nil
}

// SudoAptInstall installs apt packages using sudo password from wizard.
func (s *Service) SudoAptInstall(password string, pkgs []string) (string, error) {
	if strings.TrimSpace(password) == "" {
		return "", fmt.Errorf("sudo password required")
	}
	// basic validation: only allow known apt packages to avoid command injection
	allowed := map[string]bool{}
	for _, p := range checkerAptPackages() {
		allowed[p] = true
	}
	// also allow common deps that pip failures suggest
	for _, p := range []string{"zlib1g-dev", "libz-dev", "python3-dev", "build-essential"} {
		allowed[p] = true
	}
	for _, p := range pkgs {
		if !allowed[p] && !strings.HasPrefix(p, "python3") && !strings.HasPrefix(p, "lib") {
			return "", fmt.Errorf("package %q not allowed", p)
		}
	}
	return sudo.AptInstall(password, pkgs...)
}

// checkerAptPackages returns the list from checker.AptPackages (avoid import cycle).
func checkerAptPackages() []string {
	return []string{"build-essential", "python3-dev", "python3-venv", "libpq-dev", "libxml2-dev", "libxslt1-dev", "libldap2-dev", "libsasl2-dev", "libssl-dev", "libjpeg-dev", "libffi-dev", "zlib1g-dev", "liblcms2-dev"}
}

// ShellCheck validates python/odoo-bin/conf/db for shell.
func (s *Service) ShellCheck(name, dbName string) (string, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return "", err
	}
	p := inst.ResolvePaths(s.rootFor(inst))
	py := installer.PythonFor(inst, p)
	// check db
	if dbName != "" {
		if err := s.requireDB(inst, dbName); err != nil {
			return "", err
		}
	} else if inst.DBName != "" {
		if err := s.requireDB(inst, inst.DBName); err != nil {
			return "", err
		}
	}
	// check paths
	if _, err := os.Stat(p.Source); err != nil {
		return "", fmt.Errorf("odoo source not found at %s", p.Source)
	}
	if _, err := os.Stat(filepath.Join(p.Source, "odoo-bin")); err != nil {
		return "", fmt.Errorf("odoo-bin not found at %s/odoo-bin", p.Source)
	}
	if _, err := os.Stat(p.Conf); err != nil {
		return "", fmt.Errorf("odoo.conf not found at %s", p.Conf)
	}
	if _, err := exec.LookPath(py); err != nil {
		if _, err2 := os.Stat(py); err2 != nil {
			return "", fmt.Errorf("python not found: %s (venv missing? try odoo16: python3.10)", py)
		}
	}
	return py, nil
}

// GetShellCommand returns the exact command to run odoo shell in system terminal.
func (s *Service) GetShellCommand(name, dbName string) (string, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return "", err
	}
	p := inst.ResolvePaths(s.rootFor(inst))
	py, err := s.ShellCheck(name, dbName)
	if err != nil {
		return "", err
	}
	if dbName == "" {
		dbName = inst.DBName
	}
	cmd := fmt.Sprintf("%s %s shell -c %s -d %s", py, filepath.Join(p.Source, "odoo-bin"), p.Conf, dbName)
	return cmd, nil
}
