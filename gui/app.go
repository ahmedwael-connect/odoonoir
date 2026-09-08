package main

import (
	"context"
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/ahmed/odoonoir/internal/checker"
	ghinternal "github.com/ahmed/odoonoir/internal/github"
	"github.com/ahmed/odoonoir/internal/logmon"
	"github.com/ahmed/odoonoir/internal/marketplace"
	"github.com/ahmed/odoonoir/internal/odoomod"
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

// CreateConf generates a fresh odoo.conf for an instance from its registry data.
func (a *App) CreateConf(name string) error {
	return a.svc.CreateConf(name)
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

// SystemCheck runs a full system audit for Odoo readiness.
func (a *App) SystemCheck() []checker.Result {
	return checker.Run().Results
}

// CheckVersion runs the system audit plus Python version compatibility for an Odoo version.
func (a *App) CheckVersion(version string) []checker.Result {
	c := checker.Run()
	c.ForVersion(version)
	return c.Results
}

// ScaffoldModule generates a complete Odoo module tree from a declarative definition.
func (a *App) ScaffoldModule(name, displayName, summary, author, license_, version, category, addonsDir string, depends []string, models []odoomod.Model, menus, wizard, tests, controllers, demo, security, mailThread, activity bool) error {
	m := &odoomod.Module{
		Name:        name,
		DisplayName: displayName,
		Summary:     summary,
		Author:      author,
		License:     license_,
		Version:     version,
		Category:    category,
		Depends:     depends,
		Models:      models,
		Menus:       menus,
		Wizard:      wizard,
		Tests:       tests,
		Controllers: controllers,
		DemoData:    demo,
		Security:    security,
		MailThread:  mailThread,
		Activity:    activity,
		AddonsDir:   addonsDir,
	}
	return odoomod.Scaffold(m)
}

// RunTests runs the test suite of the given modules against an instance database.
func (a *App) RunTests(name string, dbName string, modules []string) error {
	return a.svc.Test(a.ctx, name, dbName, modules, a.emit)
}

// ModuleInstall installs modules into an instance database (lighter than Update — no git/pip).
func (a *App) ModuleInstall(name string, dbName string, modules []string) error {
	return a.svc.ModuleOps(a.ctx, name, dbName, modules, nil, a.emit)
}

// UpdateInstance edits metadata fields of a registered instance.
func (a *App) UpdateInstance(name string, description string, workers int, logLevel string, pythonBin string) error {
	inst, err := a.svc.Instance(name)
	if err != nil {
		return err
	}
	inst.Description = description
	inst.Workers = workers
	inst.LogLevel = logLevel
	inst.PythonBin = pythonBin
	return a.svc.PutInstance(inst)
}

// DevStart launches an instance in development mode with file watching and auto-restart.
func (a *App) DevStart(name string, dbName string) error {
	return a.svc.DevStart(a.ctx, name, dbName, a.emit)
}

// GenerateLaunchConfig creates a VS Code/Cursor launch.json for debugging an instance.
func (a *App) GenerateLaunchConfig(name string) (string, error) {
	return a.svc.GenerateLaunchConfig(name)
}

// SearchLogs searches instance logs with regex, level filter, time range.
func (a *App) SearchLogs(name, query string, regex bool, level string, since int64, limit int) ([]logmon.SearchResult, error) {
	return a.svc.SearchLogs(name, query, regex, level, since, limit)
}

// ModuleDiff compares installed modules with their disk manifests.
func (a *App) ModuleDiff(name, dbName string) ([]service.ModuleDiffResult, error) {
	return a.svc.ModuleDiff(name, dbName)
}

// ModelInfo fetches complete model metadata from the database.
func (a *App) ModelInfo(name, dbName, model string) (*service.ModelInfo, error) {
	return a.svc.ModelInfo(name, dbName, model)
}

// CronList returns all scheduled actions for a database.
func (a *App) CronList(name, dbName string) ([]service.CronEntry, error) {
	return a.svc.CronList(name, dbName)
}

// ShellURL returns the WebSocket URL for the odoo shell with pre-flight checks.
func (a *App) ShellURL(name string) (string, error) {
	inst, err := a.svc.Instance(name)
	if err != nil {
		return "", err
	}
	// pre-check python/odoo-bin/conf to give immediate hint instead of generic ws error
	if _, err := a.svc.ShellCheck(name, ""); err != nil {
		return "", err
	}
	return fmt.Sprintf("ws://localhost:%d/shell", inst.LongpollPort+1), nil
}

// GetShellCommand returns the exact odoo shell command to run in a system terminal.
func (a *App) GetShellCommand(name, dbName string) (string, error) {
	return a.svc.GetShellCommand(name, dbName)
}

// ShellCheck validates python/odoo-bin/conf/db for shell.
func (a *App) ShellCheck(name, dbName string) error {
	_, err := a.svc.ShellCheck(name, dbName)
	return err
}

// BrowseRecords searches and returns records for a model.
func (a *App) BrowseRecords(name, dbName string, opts service.RecordBrowserOptions) (*service.RecordBrowserResult, error) {
	return a.svc.BrowseRecords(name, dbName, opts)
}

// CreateRecord creates a new record.
func (a *App) CreateRecord(name, dbName string, input service.RecordCreateInput) (int64, error) {
	return a.svc.CreateRecord(name, dbName, input)
}

// UpdateRecord updates an existing record.
func (a *App) UpdateRecord(name, dbName string, input service.RecordUpdateInput) error {
	return a.svc.UpdateRecord(name, dbName, input)
}

// DeleteRecord deletes records.
func (a *App) DeleteRecord(name, dbName string, input service.RecordDeleteInput) error {
	return a.svc.DeleteRecord(name, dbName, input)
}

// ModuleDepGraph returns the module dependency graph for an instance.
func (a *App) ModuleDepGraph(name, dbName string, opts service.ModuleDepGraphOptions) (*service.ModuleDepGraph, error) {
	return a.svc.ModuleDepGraph(name, dbName, opts)
}

// SetBackupSchedule enables/disables and configures automatic backup for an instance.
func (a *App) SetBackupSchedule(name string, opts service.BackupScheduleOptions) error {
	return a.svc.SetBackupSchedule(name, opts)
}

// GetBackupSchedule returns the current backup schedule for an instance.
func (a *App) GetBackupSchedule(name string) (*service.BackupScheduleOptions, error) {
	return a.svc.GetBackupSchedule(name)
}

// GetBackupScheduleStatus returns the current status of scheduled backups.
func (a *App) GetBackupScheduleStatus(name string) (*service.BackupScheduleStatus, error) {
	return a.svc.GetBackupScheduleStatus(name)
}

// ListScheduledBackups lists backup files for an instance.
func (a *App) ListScheduledBackups(name string) ([]service.BackupFileInfo, error) {
	return a.svc.ListScheduledBackups(name)
}

// RunBackupNow triggers an immediate backup for the scheduled databases.
func (a *App) RunBackupNow(name string) error {
	return a.svc.RunBackupNow(name)
}

// GetDashboardMetrics returns aggregate metrics across all instances.
func (a *App) GetDashboardMetrics() (*service.DashboardMetrics, error) {
	return a.svc.GetDashboardMetrics()
}

// ============================================
// Marketplace
// ============================================

// SearchMarketplaceModules searches for modules in the marketplace
func (a *App) SearchMarketplaceModules(query string, opts marketplace.ModuleFilter) ([]marketplace.MarketplaceModule, error) {
	return a.svc.SearchMarketplaceModules(a.ctx, query, opts)
}

// GetMarketplaceModule returns a module by ID
func (a *App) GetMarketplaceModule(id string) (*marketplace.MarketplaceModule, error) {
	return a.svc.GetMarketplaceModule(id)
}

// ListMarketplaceModules lists modules with filtering
func (a *App) ListMarketplaceModules(opts marketplace.ModuleFilter) ([]marketplace.MarketplaceModule, error) {
	return a.svc.ListMarketplaceModules(opts)
}

// GetMarketplaceModuleReviews returns reviews for a module
func (a *App) GetMarketplaceModuleReviews(moduleID string) ([]marketplace.Review, error) {
	return a.svc.GetMarketplaceModuleReviews(moduleID)
}

// AddMarketplaceReview adds a review to a module
func (a *App) AddMarketplaceReview(review *marketplace.Review) error {
	return a.svc.AddMarketplaceReview(review)
}

// GetMarketplaceModuleRating returns rating summary for a module
func (a *App) GetMarketplaceModuleRating(moduleID string) (*marketplace.RatingSummary, error) {
	return a.svc.GetMarketplaceModuleRating(moduleID)
}

// GetMarketplaceStats returns marketplace statistics
func (a *App) GetMarketplaceStats() (*marketplace.MarketplaceStats, error) {
	return a.svc.GetMarketplaceStats()
}

// GetFeaturedModules returns featured modules
func (a *App) GetFeaturedModules(limit int) ([]marketplace.MarketplaceModule, error) {
	return a.svc.GetFeaturedModules(limit)
}

// GetVerifiedModules returns verified modules
func (a *App) GetVerifiedModules(limit int) ([]marketplace.MarketplaceModule, error) {
	return a.svc.GetVerifiedModules(limit)
}

// GetTopRatedModules returns top-rated modules
func (a *App) GetTopRatedModules(limit int) ([]marketplace.MarketplaceModule, error) {
	return a.svc.GetTopRatedModules(limit)
}

// GetMostInstalledModules returns most installed modules
func (a *App) GetMostInstalledModules(limit int) ([]marketplace.MarketplaceModule, error) {
	return a.svc.GetMostInstalledModules(limit)
}

// IncrementMarketplaceInstallCount increments the install count for a module
func (a *App) IncrementMarketplaceInstallCount(moduleID string) error {
	return a.svc.IncrementMarketplaceInstallCount(moduleID)
}

// IndexMarketplaceModule indexes a module from GitHub
func (a *App) IndexMarketplaceModule(owner, repo string) (*marketplace.MarketplaceModule, error) {
	return a.svc.IndexMarketplaceModule(context.Background(), owner, repo)
}

// SetGitHubToken stores the GitHub OAuth token
func (a *App) SetGitHubToken(token string) error {
	return a.svc.SetGitHubToken(a.ctx, token)
}

// GetGitHubToken returns the stored GitHub token
func (a *App) GetGitHubToken() (string, error) {
	return a.svc.GetGitHubToken(a.ctx)
}

// ValidateGitHubToken checks the stored token against GitHub API
func (a *App) ValidateGitHubToken() (interface{}, error) {
	return a.svc.ValidateGitHubToken(a.ctx)
}

// ClearGitHubToken removes the stored token
func (a *App) ClearGitHubToken() error {
	return a.svc.ClearGitHubToken(a.ctx)
}

// InstallGitHubModule clones a GitHub repo and installs it into an instance
func (a *App) InstallGitHubModule(opts service.GitHubInstallOptions) (*ghinternal.InstallResult, error) {
	return a.svc.InstallGitHubModule(a.ctx, opts)
}

// SyncGitHubModule syncs a local module with its GitHub repo
func (a *App) SyncGitHubModule(owner, repo, branch, instance string) (*ghinternal.SyncResult, error) {
	return a.svc.SyncGitHubModule(a.ctx, instance, owner, repo, branch)
}

// ValidateDBConfig checks odoo.conf vs global config for drift
func (a *App) ValidateDBConfig(name string) ([]string, error) {
	return a.svc.ValidateDBConfig(name)
}

// ListGitHubBranches lists remote branches for a repo (Odoo.sh like)
func (a *App) ListGitHubBranches(owner, repo string) ([]string, error) {
	if a.svc.GitHub() == nil {
		return nil, fmt.Errorf("GitHub not configured")
	}
	return a.svc.GitHub().ListBranches(a.ctx, owner, repo)
}

// PublishStandardModule validates manifest, ensures structure, pushes to GitHub and creates release (standard Odoo publish)
func (a *App) PublishStandardModule(opts ghinternal.PublishStandardOptions) (*ghinternal.PublishStandardResult, error) {
	if a.svc.GitHub() == nil {
		return nil, fmt.Errorf("GitHub not configured — set token in Marketplace")
	}
	return a.svc.GitHub().PublishStandard(a.ctx, opts)
}

// DBErrorInfo exposes DBError details to frontend (for toast hints)
type DBErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

// DetectEnterprise checks if instance is enterprise (addons_path contains enterprise)
func (a *App) DetectEnterprise(name string) (*service.EnterpriseStatus, error) {
	return a.svc.DetectEnterprise(name)
}

// LoadEnterprise clones enterprise repo at branch and wires addons_path (position: default|last)
func (a *App) LoadEnterprise(name string, repo string, branch string, position string) (*service.EnterpriseStatus, error) {
	return a.svc.LoadEnterprise(a.ctx, name, service.LoadEnterpriseOptions{Repo: repo, Branch: branch, Position: position}, a.emit)
}

// UnloadEnterprise removes enterprise from addons_path
func (a *App) UnloadEnterprise(name string) (*service.EnterpriseStatus, error) {
	return a.svc.UnloadEnterprise(name)
}

// OpenInVSCode opens the instance folder in VS Code / Cursor
func (a *App) OpenInVSCode(name string, editor string) (string, error) {
	return a.svc.OpenInVSCode(name, editor)
}

// AddonPathManager
func (a *App) ListAddonPaths(name string) ([]service.AddonPathEntry, error) {
	return a.svc.ListAddonPaths(name)
}
func (a *App) AddAddonPath(name, path string, position int) ([]service.AddonPathEntry, error) {
	return a.svc.AddAddonPath(name, path, position)
}
func (a *App) RemoveAddonPath(name, path string) ([]service.AddonPathEntry, error) {
	return a.svc.RemoveAddonPath(name, path)
}
func (a *App) ToggleAddonPath(name, path string, enable bool) ([]service.AddonPathEntry, error) {
	return a.svc.ToggleAddonPath(name, path, enable)
}
func (a *App) MoveAddonPath(name string, from, to int) ([]service.AddonPathEntry, error) {
	return a.svc.MoveAddonPath(name, from, to)
}

func (a *App) SudoAptInstall(password string, pkgs []string) (string, error) {
	return a.svc.SudoAptInstall(password, pkgs)
}

func (a *App) GetSlowQueries(name, dbName string, limit int) ([]service.SlowQuery, error) {
	return a.svc.GetSlowQueries(name, dbName, limit)
}

func (a *App) GetFlameGraph(name string, seconds int) (string, error) {
	return a.svc.GetFlameGraph(name, seconds)
}

func (a *App) GetAutoUpdate(name string) (*service.AutoUpdateConfig, error) {
	return a.svc.GetAutoUpdate(name)
}

func (a *App) SetPrimaryDatabase(name, dbName string) error {
	return a.svc.SetPrimaryDatabase(name, dbName)
}

func (a *App) TrackDatabase(name, dbName string) error {
	return a.svc.TrackDatabase(name, dbName)
}

func (a *App) SetAutoUpdate(name string, modules []string, enabled bool) error {
	return a.svc.SetAutoUpdate(name, modules, enabled)
}

// emit forwards service events to the frontend as Wails "event" messages.
func (a *App) emit(e service.Event) {
	application.Get().Event.Emit("odoonoir-event", e)
}
