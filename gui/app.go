package main

import (
	"context"
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/ahmed/odoonoir/internal/service"
)

// App is the Wails-bound bridge over the service layer. Every exported
// method becomes callable from the frontend as a Promise.
type App struct {
	svc *service.Service
	ctx context.Context
}

func NewApp(svc *service.Service) *App {
	return &App{svc: svc}
}

// ServiceStartup runs when the app starts; ctx lives for the app lifetime.
func (a *App) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	a.ctx = ctx
	return nil
}

// Instances lists all managed instances with live status.
func (a *App) Instances() ([]service.InstanceView, error) {
	return a.svc.Instances()
}

// Status returns the runtime detail of one instance.
func (a *App) Status(name string) (service.StatusView, error) {
	return a.svc.Status(name)
}

// Databases lists the databases served by an instance.
func (a *App) Databases(name string) ([]service.DatabaseView, error) {
	return a.svc.Databases(name)
}

// Start launches an instance, emitting "status" events.
func (a *App) Start(name string) error {
	return a.svc.Start(a.ctx, name, "", a.emit)
}

// StartDB launches an instance serving a specific database.
func (a *App) StartDB(name string, dbName string) error {
	return a.svc.Start(a.ctx, name, dbName, a.emit)
}

// Stop stops an instance, emitting "status" events.
func (a *App) Stop(name string) error {
	return a.svc.Stop(a.ctx, name, a.emit)
}

// SwitchDB stops the instance and restarts it serving a different database.
func (a *App) SwitchDB(name string, dbName string) error {
	// Stop is idempotent — if already stopped, proceed to start.
	_ = a.svc.Stop(a.ctx, name, a.emit)
	return a.svc.Start(a.ctx, name, dbName, a.emit)
}

// Restart restarts an instance, emitting "status" events.
func (a *App) Restart(name string) error {
	return a.svc.Restart(a.ctx, name, "", a.emit)
}

// Update runs the full update pipeline (pull/pip/modules) with progress
// events streamed to the frontend.
func (a *App) Update(name string, install []string, update []string) error {
	return a.svc.Update(a.ctx, name, service.UpdateOptions{
		InstallMods: install,
		UpdateMods:  update,
	}, a.emit)
}

// LogsSince returns new log lines appended since an offset.
func (a *App) LogsSince(name string, n int, offset int64) ([]string, int64, bool, error) {
	lines, rotated, err := a.svc.LogsSince(name, n, &offset)
	return lines, offset, rotated, err
}

// LogPath resolves the instance's log file path.
func (a *App) LogPath(name string) (string, error) {
	return a.svc.LogPath(name)
}

// Backup dumps a database to a file.
func (a *App) Backup(name string, dbName string, out string) error {
	return a.svc.Backup(a.ctx, name, dbName, out, true, false, a.emit)
}

// Restore loads a dump into a database.
func (a *App) Restore(name string, dbName string, dump string, force bool) error {
	return a.svc.Restore(a.ctx, name, dbName, dump, service.RestoreOptions{Force: force}, a.emit)
}

// DropDB drops a non-primary database.
func (a *App) DropDB(name string, dbName string) error {
	return a.svc.DropDB(name, dbName)
}

// InitDB initializes a database (runs -i base).
func (a *App) InitDB(name string, dbName string) error {
	return a.svc.InitDB(a.ctx, name, dbName, a.emit)
}

// CreateResult reports what Create produced.
type CreateResult struct {
	Name     string `json:"name"`
	Database string `json:"database"`
	Port     int    `json:"port"`
}

// Create installs a new Odoo instance and registers it.
func (a *App) Create(name, version, port, dbUser, dbPass, dbName string) (CreateResult, error) {
	portNum := 0
	fmt.Sscanf(port, "%d", &portNum)
	res, err := a.svc.Create(a.ctx, service.CreateOptions{
		Name:    name,
		Version: version,
		Port:    portNum,
		DBUser:  dbUser,
		DBPass:  dbPass,
		DBName:  dbName,
		InitDB:  true,
	}, a.emit)
	if err != nil {
		return CreateResult{}, err
	}
	return CreateResult{
		Name:     res.Instance.Name,
		Database: res.Database,
		Port:     res.Port,
	}, nil
}

// ConfEntry is a single config key/value pair for the Settings screen.
type ConfEntry struct {
	Key    string `json:"key"`
	Active bool   `json:"active"`
	Value  string `json:"value"`
}

// ReadConf reads all key-value pairs from the instance's odoo.conf.
func (a *App) ReadConf(name string) ([]ConfEntry, error) {
	inst, err := a.svc.Instance(name)
	if err != nil {
		return nil, err
	}
	confPath := inst.ConfPath
	if confPath == "" {
		return nil, fmt.Errorf("no conf file for instance %q", name)
	}
	entries, err := a.svc.ReadConf(name)
	if err != nil {
		return nil, err
	}
	var out []ConfEntry
	for _, e := range entries {
		out = append(out, ConfEntry{Key: e.Name, Active: e.Active, Value: e.Value})
	}
	return out, nil
}

// SetConf updates a key in the instance's odoo.conf and saves it.
func (a *App) SetConf(name string, key string, value string) error {
	return a.svc.SetConf(name, key, value)
}

// Remove stops, deletes files and optionally drops the database.
func (a *App) Remove(name string, keepData bool) error {
	return a.svc.Remove(name, keepData)
}

// AdoptResult carries what Adopt detected.
type AdoptResult = service.AdoptResult

// Adopt imports an existing Odoo installation.
func (a *App) Adopt(name, source, conf, port, dbName, dbUser, version string) (AdoptResult, error) {
	portNum := 0
	fmt.Sscanf(port, "%d", &portNum)
	res, err := a.svc.Adopt(service.AdoptOptions{
		Name:    name,
		Source:  source,
		Conf:    conf,
		Port:    portNum,
		DBName:  dbName,
		DBUser:  dbUser,
		Version: version,
	})
	if err != nil {
		return AdoptResult{}, err
	}
	return *res, nil
}

// CloneResult carries what Clone produced.
type CloneResult = service.CloneResult

// Clone creates a copy of an instance.
func (a *App) Clone(name, newName, port string) (CloneResult, error) {
	portNum := 0
	fmt.Sscanf(port, "%d", &portNum)
	res, err := a.svc.Clone(a.ctx, name, newName, portNum, a.emit)
	if err != nil {
		return CloneResult{}, err
	}
	return *res, nil
}

// ModuleView represents a module from ir_module_module.
type ModuleView = service.ModuleView

// ModuleList lists modules in a database.
func (a *App) ModuleList(name string, db string) ([]ModuleView, error) {
	return a.svc.ModuleList(name, db)
}

// ModuleUninstall uninstalls a module from a database.
func (a *App) ModuleUninstall(name string, module string, db string) error {
	return a.svc.ModuleUninstall(a.ctx, name, module, db, a.emit)
}

// DoctorIssue is a detected log problem.
type DoctorIssue = service.DoctorIssue

// Doctor scans the instance log for known issues.
func (a *App) Doctor(name string) ([]DoctorIssue, error) {
	return a.svc.Doctor(name)
}

// emit forwards service events to the frontend as Wails "event" messages.
func (a *App) emit(e service.Event) {
	application.Get().Event.Emit("odoonoir-event", e)
}
