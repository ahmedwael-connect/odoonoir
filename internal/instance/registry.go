package instance

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"
)

var ErrNotFound = errors.New("instance not found")

// Status mirrors the runtime state of an instance process.
type Status string

const (
	StatusStopped Status = "stopped"
	StatusRunning Status = "running"
	StatusUnknown Status = "unknown"
)

// Instance describes one managed Odoo installation.
type Instance struct {
	Name         string    `json:"name"`
	Version      string    `json:"version"` // e.g. "18.0"
	Branch       string    `json:"branch"`  // git branch/tag the source is pinned to
	SourceURL    string    `json:"source_url"`
	Port         int       `json:"port"`
	DBName       string    `json:"db_name"`
	DBUser       string    `json:"db_user"`
	Databases    []string  `json:"databases,omitempty"` // additional databases served by this instance (beyond DBName)
	LongpollPort int       `json:"longpoll_port,omitempty"`
	Root         string    `json:"root,omitempty"` // per-instance storage root override
	Description  string    `json:"description,omitempty"`
	Workers      int       `json:"workers,omitempty"`
	LogLevel     string    `json:"log_level,omitempty"`
	GitRef       string    `json:"git_ref,omitempty"`
	PythonBin    string    `json:"python_bin,omitempty"` // python used for the venv
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	// Adopted marks an instance that was imported from an existing
	// installation (see `odoonoir adopt`). Its paths may live anywhere
	// and are resolved through the overrides below.
	Adopted     bool     `json:"adopted,omitempty"`
	SourcePath  string   `json:"source_path,omitempty"`  // dir containing odoo-bin
	VenvPath    string   `json:"venv_path,omitempty"`    // python venv dir
	ConfPath    string   `json:"conf_path,omitempty"`    // existing odoo.conf
	AddonsPaths []string `json:"addons_paths,omitempty"` // custom addon dirs
	DataDirPath string   `json:"data_dir_path,omitempty"`
	LogPath     string   `json:"log_path,omitempty"` // conf logfile
	PIDPath     string   `json:"pid_path,omitempty"` // conf pidfile

// Backup scheduling
	BackupScheduleEnabled  bool       `json:"backup_schedule_enabled,omitempty"`
	BackupScheduleCron     string     `json:"backup_schedule_cron,omitempty"`   // cron expression
	BackupScheduleDBs      []string   `json:"backup_schedule_dbs,omitempty"`    // databases to backup (empty = all)
	BackupScheduleRetain   int        `json:"backup_schedule_retain,omitempty"` // number of backups to retain
	BackupScheduleCompress bool       `json:"backup_schedule_compress,omitempty"` // gzip compress
	BackupScheduleCustom   bool       `json:"backup_schedule_custom,omitempty"`   // custom format
	BackupScheduleLastRun  *time.Time `json:"backup_schedule_last_run,omitempty"`
	BackupScheduleNextRun  *time.Time `json:"backup_schedule_next_run,omitempty"`
	BackupScheduleID       int        `json:"backup_schedule_id,omitempty"`     // cron entry ID

	// Enterprise integration (Odoo.sh like)
	EnterprisePath   string `json:"enterprise_path,omitempty"`   // cloned enterprise root (contains enterprise addons)
	EnterpriseRepo   string `json:"enterprise_repo,omitempty"`   // owner/repo e.g. odoo/enterprise
	EnterpriseBranch string `json:"enterprise_branch,omitempty"` // branch e.g. 16.0

	// Auto-update on run: modules to -u on every start
	AutoUpdateModules []string `json:"auto_update_modules,omitempty"`
	AutoUpdateOnRun   bool     `json:"auto_update_on_run,omitempty"`
}

// Paths resolves the on-disk layout of an instance.
type Paths struct {
	Root    string
	Source  string
	Addons  string
	Venv    string
	Conf    string
	Log     string
	PIDFile string
	DataDir string
	Scripts string
}

// AllDBs returns the full list of databases served by the instance:
// the primary DBName plus any additional Databases (no duplicates).
func (i *Instance) AllDBs() []string {
	out := make([]string, 0, 1+len(i.Databases))
	if i.DBName != "" {
		out = append(out, i.DBName)
	}
	for _, d := range i.Databases {
		if d != "" && d != i.DBName {
			out = append(out, d)
		}
	}
	return out
}

// AddDB registers an additional database served by the instance.
func (i *Instance) AddDB(name string) bool {
	if name == "" || name == i.DBName {
		return false
	}
	for _, d := range i.Databases {
		if d == name {
			return false
		}
	}
	i.Databases = append(i.Databases, name)
	return true
}

// RemoveDB unregisters an additional database.
func (i *Instance) RemoveDB(name string) bool {
	for idx, d := range i.Databases {
		if d == name {
			i.Databases = append(i.Databases[:idx], i.Databases[idx+1:]...)
			return true
		}
	}
	return false
}

// ResolvePaths computes the on-disk layout for an instance.
// The root parameter is the instances root; the instance's own folder
// is derived from its name. Adopted instances may override every path
// with the location of an existing installation.
func (i *Instance) ResolvePaths(root string) Paths {
	base := filepath.Join(root, i.Name)
	source := filepath.Join(base, "src", "odoo")
	venv := filepath.Join(base, "venv")
	conf := filepath.Join(base, "etc", "odoo.conf")
	logf := filepath.Join(base, "logs", "odoo.log")
	pidf := filepath.Join(base, "etc", "odoo.pid")
	datadir := filepath.Join(base, "data")
	addons := filepath.Join(base, "custom_addons")
	if i.SourcePath != "" {
		source = i.SourcePath
	}
	if i.VenvPath != "" {
		venv = i.VenvPath
	}
	if i.ConfPath != "" {
		conf = i.ConfPath
	}
	if i.LogPath != "" {
		logf = i.LogPath
	}
	if i.PIDPath != "" {
		pidf = i.PIDPath
	}
	if i.DataDirPath != "" {
		datadir = i.DataDirPath
	}
	if len(i.AddonsPaths) > 0 {
		addons = i.AddonsPaths[0]
	}
	return Paths{
		Root:    base,
		Source:  source,
		Addons:  addons,
		Venv:    venv,
		Conf:    conf,
		Log:     logf,
		PIDFile: pidf,
		DataDir: datadir,
		Scripts: filepath.Join(base, "etc"),
	}
}

// Registry is a JSON-backed store of known instances.
type Registry struct {
	path string
}

// NewRegistry loads (or initializes) the registry file.
func NewRegistry(path string) (*Registry, error) {
	r := &Registry{path: path}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return r, r.save(map[string]*Instance{})
	}
	return r, nil
}

// lock guards the registry against concurrent writers (a future GUI and a
// CLI terminal editing the same file must never lose updates). The lock
// lives on a sibling .lock file so the atomic temp+rename of the registry
// itself never invalidates the inode being locked.
func (r *Registry) lock(exclusive bool) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return nil, err
	}
	lf, err := os.OpenFile(r.path+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open registry lock: %w", err)
	}
	how := syscall.LOCK_SH
	if exclusive {
		how = syscall.LOCK_EX
	}
	if err := syscall.Flock(int(lf.Fd()), how); err != nil {
		lf.Close()
		return nil, fmt.Errorf("lock registry: %w", err)
	}
	return func() {
		syscall.Flock(int(lf.Fd()), syscall.LOCK_UN)
		lf.Close()
	}, nil
}

// All returns instances sorted by name.
func (r *Registry) All() ([]*Instance, error) {
	unlock, err := r.lock(false)
	if err != nil {
		return nil, err
	}
	defer unlock()
	m, err := r.load()
	if err != nil {
		return nil, err
	}
	out := make([]*Instance, 0, len(m))
	for _, inst := range m {
		out = append(out, inst)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	return out, nil
}

// Get returns a single instance by name.
func (r *Registry) Get(name string) (*Instance, error) {
	unlock, err := r.lock(false)
	if err != nil {
		return nil, err
	}
	defer unlock()
	m, err := r.load()
	if err != nil {
		return nil, err
	}
	inst, ok := m[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	return inst, nil
}

// Put creates or updates an instance.
func (r *Registry) Put(inst *Instance) error {
	unlock, err := r.lock(true)
	if err != nil {
		return err
	}
	defer unlock()
	m, err := r.load()
	if err != nil {
		return err
	}
	inst.UpdatedAt = time.Now()
	if inst.CreatedAt.IsZero() {
		inst.CreatedAt = inst.UpdatedAt
	}
	m[inst.Name] = inst
	return r.save(m)
}

// Delete removes an instance from the registry.
func (r *Registry) Delete(name string) error {
	unlock, err := r.lock(true)
	if err != nil {
		return err
	}
	defer unlock()
	m, err := r.load()
	if err != nil {
		return err
	}
	if _, ok := m[name]; !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	delete(m, name)
	return r.save(m)
}

func (r *Registry) load() (map[string]*Instance, error) {
	data, err := os.ReadFile(r.path)
	if err != nil {
		return nil, fmt.Errorf("read registry: %w", err)
	}
	m := map[string]*Instance{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse registry %s: %w", r.path, err)
	}
	return m, nil
}

func (r *Registry) save(m map[string]*Instance) error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, r.path)
}
