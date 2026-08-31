package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ahmed/odoonoir/internal/db"
	"github.com/ahmed/odoonoir/internal/installer"
	"github.com/ahmed/odoonoir/internal/instance"
	"github.com/ahmed/odoonoir/internal/logmon"
	"github.com/ahmed/odoonoir/internal/proc"
	"github.com/ahmed/odoonoir/internal/updater"
	"github.com/fsnotify/fsnotify"
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

// SearchLogs searches instance logs with regex, level filter, time range.
// Returns matched log lines with metadata.
func (s *Service) SearchLogs(name, query string, regex bool, level string, since int64, limit int) ([]logmon.SearchResult, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return nil, err
	}
	p := inst.ResolvePaths(s.rootFor(inst))
	if _, err := os.Stat(p.Log); err != nil {
		return []logmon.SearchResult{}, nil
	}
	return logmon.Search(p.Log, logmon.SearchOptions{
		Query:   query,
		Regex:   regex,
		Level:   level,
		Since:   since,
		Limit:   limit,
	})
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

// DevStart launches an instance in development mode with file watching.
// It watches the addons_path and source directory for changes to .py, .xml, .js, .css files
// and automatically restarts the Odoo server when changes are detected.
func (s *Service) DevStart(ctx context.Context, name, dbName string, emit Sink) error {
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
	p := inst.ResolvePaths(s.rootFor(inst))
	mgr := proc.New(p, installer.PythonFor(inst, p), p.Conf, inst.LongpollPort)

	// Initial start
	if err := mgr.Start(dbName); err != nil {
		return err
	}
	if err := waitForStable(ctx, mgr, s.logPath(inst)); err != nil {
		return err
	}
	emit(Event{Kind: StatusChanged, Instance: name, Message: fmt.Sprintf("instance %s started in dev mode serving %q", name, dbName)})

	// Set up file watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create file watcher: %w", err)
	}
	defer watcher.Close()

	// Collect directories to watch
	watchDirs := []string{p.Source}
	if len(inst.AddonsPaths) > 0 {
		watchDirs = append(watchDirs, inst.AddonsPaths...)
	} else {
		addons := installer.BuildAddonsPath(p)
		watchDirs = append(watchDirs, addons...)
	}

	// Add existing directories to watcher
	for _, dir := range watchDirs {
		if err := addWatchRecursive(watcher, dir); err != nil {
			emit(Event{Kind: LogLine, Instance: name, Message: fmt.Sprintf("watch warning: %v", err)})
		}
	}

	// Debounce restarts
	restartCh := make(chan struct{}, 1)
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				select {
				case <-restartCh:
					// Debounced restart
					emit(Event{Kind: LogLine, Instance: name, Message: "File change detected, restarting..."})
					if err := mgr.Stop(); err != nil {
						emit(Event{Kind: LogLine, Instance: name, Message: fmt.Sprintf("stop error: %v", err)})
					}
					if err := mgr.Start(dbName); err != nil {
						emit(Event{Kind: LogLine, Instance: name, Message: fmt.Sprintf("start error: %v", err)})
					} else if err := waitForStable(ctx, mgr, s.logPath(inst)); err != nil {
						emit(Event{Kind: LogLine, Instance: name, Message: fmt.Sprintf("stabilize error: %v", err)})
					} else {
						emit(Event{Kind: StatusChanged, Instance: name, Message: fmt.Sprintf("instance %s restarted (dev mode)", name)})
					}
				default:
				}
			}
		}
	}()

	// Watch for events
	for {
		select {
		case <-ctx.Done():
			_ = mgr.Stop()
			return ctx.Err()
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			// Filter for relevant file types
			if shouldWatchFile(event.Name) {
				select {
				case restartCh <- struct{}{}:
				default:
				}
			}
			// If a new directory is created, watch it
			if event.Op&fsnotify.Create == fsnotify.Create {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					_ = addWatchRecursive(watcher, event.Name)
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			emit(Event{Kind: LogLine, Instance: name, Message: fmt.Sprintf("watch error: %v", err)})
		}
	}
}

// shouldWatchFile returns true if the file extension is relevant for Odoo development.
func shouldWatchFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".py", ".xml", ".js", ".css", ".scss", ".sass", ".less", ".json", ".csv", ".po", ".pot":
		return true
	}
	// Also watch __manifest__.py and __openerp__.py
	base := strings.ToLower(filepath.Base(path))
	return base == "__manifest__.py" || base == "__openerp__.py"
}

// addWatchRecursive adds a directory and all its subdirectories to the watcher.
func addWatchRecursive(watcher *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors, just don't watch this path
		}
		if d.IsDir() {
			// Skip hidden directories and common ignore patterns
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "__pycache__" || name == "node_modules" || name == "venv" || name == ".git" {
				return filepath.SkipDir
			}
			return watcher.Add(path)
		}
		return nil
	})
}

// GenerateLaunchConfig creates a VS Code/Cursor launch.json for debugging an instance.
// It configures debugpy to attach to the running Odoo instance.
func (s *Service) GenerateLaunchConfig(name string) (string, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return "", err
	}
	p := inst.ResolvePaths(s.rootFor(inst))
	py := installer.PythonFor(inst, p)

	// Find the debugpy path
	debugpyPath := filepath.Join(filepath.Dir(py), "debugpy")
	if _, err := os.Stat(debugpyPath); os.IsNotExist(err) {
		// Try to find it in site-packages
		debugpyPath = ""
	}

	config := map[string]interface{}{
		"version": "0.2.0",
		"configurations": []map[string]interface{}{
			{
				"name":         "Odoo: Attach to " + name,
				"type":         "debugpy",
				"request":      "attach",
				"connect":      map[string]interface{}{"host": "localhost", "port": 5678},
				"pathMappings": []map[string]string{{"localRoot": p.Source, "remoteRoot": p.Source}},
				"justMyCode":   false,
			},
			{
				"name":         "Odoo: Launch " + name + " (debugpy)",
				"type":         "debugpy",
				"request":      "launch",
				"module":       "odoo",
				"args":         []string{"-c", p.Conf, "-d", inst.DBName, "--dev=all"},
				"python":       py,
				"cwd":          p.Source,
				"justMyCode":   false,
				"env":          map[string]string{"PYTHONPATH": strings.Join(installer.BuildAddonsPath(p), ":")},
			},
		},
	}

	// Write to .vscode/launch.json in the instance source directory
	vscodeDir := filepath.Join(p.Source, ".vscode")
	if err := os.MkdirAll(vscodeDir, 0o755); err != nil {
		return "", fmt.Errorf("create .vscode dir: %w", err)
	}
	launchPath := filepath.Join(vscodeDir, "launch.json")
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal launch.json: %w", err)
	}
	if err := os.WriteFile(launchPath, data, 0o644); err != nil {
		return "", fmt.Errorf("write launch.json: %w", err)
	}
	return launchPath, nil
}

// ═══════════════════════════════════════════════════════════════════════
// Record Browser
// ═══════════════════════════════════════════════════════════════════════

// RecordField represents a field definition for a model.
type RecordField struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	String      string `json:"string"`
	Required    bool   `json:"required"`
	Readonly    bool   `json:"readonly"`
	Relation    string `json:"relation"`
	RelationTable string `json:"relationTable"`
	ComodelName string `json:"comodelName"`
	Domain      string `json:"domain"`
	Default     string `json:"default"`
	Groups      string `json:"groups"`
	Help        string `json:"help"`
}

// Record represents a single record from a model.
type Record struct {
	ID      int64                  `json:"id"`
	Values  map[string]interface{} `json:"values"`
	Display string                 `json:"display"` // name_get result
}

// RecordBrowserOptions configures record search.
type RecordBrowserOptions struct {
	Model       string
	Domain      []interface{} // Odoo domain format
	Fields      []string
	Limit       int
	Offset      int
	Order       string
	Context     map[string]interface{}
}

// RecordBrowserResult contains records and metadata.
type RecordBrowserResult struct {
	Records    []Record     `json:"records"`
	TotalCount int64        `json:"totalCount"`
	Fields     []RecordField `json:"fields"`
	Limit      int          `json:"limit"`
	Offset     int          `json:"offset"`
}

// RecordCreateInput for creating new records.
type RecordCreateInput struct {
	Model  string                 `json:"model"`
	Values map[string]interface{} `json:"values"`
	Context map[string]interface{} `json:"context,omitempty"`
}

// RecordUpdateInput for updating existing records.
type RecordUpdateInput struct {
	Model  string                 `json:"model"`
	ID     int64                  `json:"id"`
	Values map[string]interface{} `json:"values"`
	Context map[string]interface{} `json:"context,omitempty"`
}

// RecordDeleteInput for deleting records.
type RecordDeleteInput struct {
	Model  string   `json:"model"`
	IDs    []int64  `json:"ids"`
	Context map[string]interface{} `json:"context,omitempty"`
}

// BrowseRecords searches and returns records for a model.
func (s *Service) BrowseRecords(name, dbName string, opts RecordBrowserOptions) (*RecordBrowserResult, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return nil, err
	}
	if dbName, err = s.ResolveDB(inst, dbName, true); err != nil {
		return nil, err
	}

	// Get model fields first
	fields, err := s.getModelFields(dbName, opts.Model)
	if err != nil {
		return nil, s.wrapPsqlErr(dbName, err)
	}

	// Build field list for query
	fieldList := opts.Fields
	if len(fieldList) == 0 {
		for _, f := range fields {
			fieldList = append(fieldList, f.Name)
		}
	}
	// Always include id and display_name/name
	hasID := false
	for _, f := range fieldList {
		if f == "id" {
			hasID = true
			break
		}
	}
	if !hasID {
		fieldList = append([]string{"id"}, fieldList...)
	}

	// Build domain
	domain := "[]"
	if len(opts.Domain) > 0 {
		domainBytes, _ := json.Marshal(opts.Domain)
		domain = string(domainBytes)
	}

	// Build context
	ctx := "{}"
	if opts.Context != nil {
		ctxBytes, _ := json.Marshal(opts.Context)
		ctx = string(ctxBytes)
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 80
	}
	offset := opts.Offset
	if offset < 0 {
		offset = 0
	}
	order := opts.Order
	if order == "" {
		order = "id DESC"
	}

	// Use odoo shell to search_read
	p := inst.ResolvePaths(s.rootFor(inst))
	py := installer.PythonFor(inst, p)

	script := fmt.Sprintf(`
import json
result = env['%s'].search_read(
    domain=%s,
    fields=%s,
    limit=%d,
    offset=%d,
    order=%s,
    context=%s
)
print(json.dumps(result))
`, opts.Model, domain, jsonMarshal(fieldList), limit, offset, jsonMarshal(order), ctx)

	if err := validatePythonAndPaths(py, p); err != nil {
		return nil, err
	}
	cmd := exec.Command(py, "-m", "odoo", "shell", "-c", p.Conf, "-d", dbName)
	cmd.Dir = p.Source
	cmd.Stdin = strings.NewReader(script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		raw := fmt.Sprintf("browse records: %v\n%s", err, strings.TrimSpace(string(out)))
		return nil, translatePsqlError(raw, fmt.Errorf("%s", raw))
	}

	var records []Record
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &records); err != nil {
		return nil, fmt.Errorf("parse records: %w", err)
	}

	// Get total count
	countScript := fmt.Sprintf(`
count = env['%s'].search_count(%s, context=%s)
print(count)
`, opts.Model, domain, ctx)
	cmd2 := exec.Command(py, "-m", "odoo", "shell", "-c", p.Conf, "-d", dbName)
	cmd2.Dir = p.Source
	cmd2.Stdin = strings.NewReader(countScript)
	countOut, _ := cmd2.CombinedOutput()
	var totalCount int64
	fmt.Sscanf(strings.TrimSpace(string(countOut)), "%d", &totalCount)

	return &RecordBrowserResult{
		Records:    records,
		TotalCount: totalCount,
		Fields:     fields,
		Limit:      limit,
		Offset:     offset,
	}, nil
}

// CreateRecord creates a new record.
func (s *Service) CreateRecord(name, dbName string, input RecordCreateInput) (int64, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return 0, err
	}
	if dbName, err = s.ResolveDB(inst, dbName, true); err != nil {
		return 0, err
	}
	p := inst.ResolvePaths(s.rootFor(inst))
	py := installer.PythonFor(inst, p)
	if err := validatePythonAndPaths(py, p); err != nil {
		return 0, err
	}

	valuesJSON, _ := json.Marshal(input.Values)
	ctx := "{}"
	if input.Context != nil {
		ctxBytes, _ := json.Marshal(input.Context)
		ctx = string(ctxBytes)
	}

	script := fmt.Sprintf(`
import json
vals = %s
record = env['%s'].with_context(%s).create(vals)
print(record.id)
`, string(valuesJSON), input.Model, ctx)

	cmd := exec.Command(py, "-m", "odoo", "shell", "-c", p.Conf, "-d", dbName)
	cmd.Dir = p.Source
	cmd.Stdin = strings.NewReader(script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		raw := fmt.Sprintf("create record: %v\n%s", err, strings.TrimSpace(string(out)))
		return 0, translatePsqlError(raw, fmt.Errorf("%s", raw))
	}
	var id int64
	fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &id)
	return id, nil
}

// UpdateRecord updates an existing record.
func (s *Service) UpdateRecord(name, dbName string, input RecordUpdateInput) error {
	inst, err := s.reg.Get(name)
	if err != nil {
		return err
	}
	if dbName, err = s.ResolveDB(inst, dbName, true); err != nil {
		return err
	}
	p := inst.ResolvePaths(s.rootFor(inst))
	py := installer.PythonFor(inst, p)
	if err := validatePythonAndPaths(py, p); err != nil {
		return err
	}

	valuesJSON, _ := json.Marshal(input.Values)
	ctx := "{}"
	if input.Context != nil {
		ctxBytes, _ := json.Marshal(input.Context)
		ctx = string(ctxBytes)
	}

	script := fmt.Sprintf(`
vals = %s
record = env['%s'].with_context(%s).browse(%d)
record.write(vals)
print('OK')
`, string(valuesJSON), input.Model, ctx, input.ID)

	cmd := exec.Command(py, "-m", "odoo", "shell", "-c", p.Conf, "-d", dbName)
	cmd.Dir = p.Source
	cmd.Stdin = strings.NewReader(script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		raw := fmt.Sprintf("update record: %v\n%s", err, strings.TrimSpace(string(out)))
		return translatePsqlError(raw, fmt.Errorf("%s", raw))
	}
	return nil
}

// DeleteRecord deletes records.
func (s *Service) DeleteRecord(name, dbName string, input RecordDeleteInput) error {
	inst, err := s.reg.Get(name)
	if err != nil {
		return err
	}
	if dbName, err = s.ResolveDB(inst, dbName, true); err != nil {
		return err
	}
	p := inst.ResolvePaths(s.rootFor(inst))
	py := installer.PythonFor(inst, p)
	if err := validatePythonAndPaths(py, p); err != nil {
		return err
	}
	idsJSON, _ := json.Marshal(input.IDs)
	ctx := "{}"
	if input.Context != nil {
		ctxBytes, _ := json.Marshal(input.Context)
		ctx = string(ctxBytes)
	}

	script := fmt.Sprintf(`
ids = %s
records = env['%s'].with_context(%s).browse(ids)
records.unlink()
print('OK')
`, string(idsJSON), input.Model, ctx)

	cmd := exec.Command(py, "-m", "odoo", "shell", "-c", p.Conf, "-d", dbName)
	cmd.Dir = p.Source
	cmd.Stdin = strings.NewReader(script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		raw := fmt.Sprintf("delete record: %v\n%s", err, strings.TrimSpace(string(out)))
		return translatePsqlError(raw, fmt.Errorf("%s", raw))
	}
	return nil
}

// getModelFields fetches field definitions for a model.
func (s *Service) getModelFields(dbName, model string) ([]RecordField, error) {
	query := fmt.Sprintf(`SELECT name, ttype, field_description, required, readonly,
		relation, relation_table, comodel_name, domain, default, groups, help
		FROM ir_model_fields WHERE model = '%s' ORDER BY name`, db.PgEscapeLiteral(model))
	out, err := s.pg.Query(dbName, query)
	if err != nil {
		return nil, err
	}
	var fields []RecordField
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 12)
		if len(parts) >= 12 {
			fields = append(fields, RecordField{
				Name:          parts[0],
				Type:          parts[1],
				String:        parts[2],
				Required:      parts[3] == "t",
				Readonly:      parts[4] == "t",
				Relation:      parts[5],
				RelationTable: parts[6],
				ComodelName:   parts[7],
				Domain:        parts[8],
				Default:       parts[9],
				Groups:        parts[10],
				Help:          parts[11],
			})
		}
	}
	return fields, nil
}

func jsonMarshal(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func validatePythonAndPaths(py string, p instance.Paths) error {
	if _, err := os.Stat(p.Source); err != nil {
		return &DBError{Code: DBErrConfig, Message: fmt.Sprintf("odoo source not found at %s", p.Source), Hint: "check instance source path — re-adopt or re-create", Err: err}
	}
	if _, err := os.Stat(p.Conf); err != nil {
		return &DBError{Code: DBErrConfig, Message: fmt.Sprintf("odoo.conf not found at %s", p.Conf), Hint: "run odoonoir config set or odoonoir create", Err: err}
	}
	if _, err := exec.LookPath(py); err != nil {
		// fallback check for venv python file
		if _, err2 := os.Stat(py); err2 != nil {
			return &DBError{Code: DBErrPython, Message: fmt.Sprintf("python not found: %s", py), Hint: "check venv — odoonoir update <instance> or set python bin", Err: err}
		}
	}
	return nil
}
