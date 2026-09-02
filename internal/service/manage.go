package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ahmed/odoonoir/internal/adopt"
	"github.com/ahmed/odoonoir/internal/checker"
	"github.com/ahmed/odoonoir/internal/db"
	"github.com/ahmed/odoonoir/internal/instance"
	"github.com/ahmed/odoonoir/internal/installer"
	"github.com/ahmed/odoonoir/internal/logmon"
	"github.com/ahmed/odoonoir/internal/proc"
	"github.com/sergi/go-diff/diffmatchpatch"
)

var validModuleName = regexp.MustCompile(`^[a-zA-Z0-9_.]+$`)

// Remove stops, deletes files and drops the primary database of an instance.
// Adopted instances are only unregistered. keepData skips the DB drop.
func (s *Service) Remove(name string, keepData bool) error {
	inst, err := s.reg.Get(name)
	if err != nil {
		return err
	}
	if inst.Adopted {
		if err := s.reg.Delete(inst.Name); err != nil {
			return err
		}
		return nil
	}
	p := inst.ResolvePaths(s.rootFor(inst))
	mgr := proc.New(p, installer.PythonFor(inst, p), p.Conf, inst.LongpollPort)
	_ = mgr.Stop()
	if err := os.RemoveAll(p.Root); err != nil {
		return fmt.Errorf("remove %s: %w", p.Root, err)
	}
	if !keepData {
		others, err := s.reg.All()
		if err != nil {
			return err
		}
		for _, o := range others {
			if o.Name != inst.Name && o.DBName == inst.DBName {
				return fmt.Errorf("database %s is also used by instance %s", inst.DBName, o.Name)
			}
		}
		if err := s.pg.DropDatabase(inst.DBName); err != nil {
			return fmt.Errorf("remove instance OK but drop database %s failed: %w", inst.DBName, err)
		}
	}
	return s.reg.Delete(inst.Name)
}

// AdoptOptions carries the inputs for adopting an existing installation.
type AdoptOptions struct {
	Name        string
	Source      string
	Conf        string
	Venv        string
	Python      string
	Addons      []string
	DBName      string
	DBUser      string
	Port        int
	Version     string
	Description string
}

// AdoptResult carries what was detected.
type AdoptResult struct {
	Name      string   `json:"name"`
	Source    string   `json:"source"`
	Conf      string   `json:"conf"`
	Port      int      `json:"port"`
	DBName    string   `json:"dbName"`
	Version   string   `json:"version"`
	Databases []string `json:"databases"`
	Warnings  []string `json:"warnings"`
}

// Adopt imports an existing Odoo installation into the registry.
func (s *Service) Adopt(opts AdoptOptions) (*AdoptResult, error) {
	dopts := adopt.Options{
		Name:        opts.Name,
		Source:      opts.Source,
		Conf:        opts.Conf,
		Venv:        opts.Venv,
		Python:      opts.Python,
		Addons:      opts.Addons,
		DBName:      opts.DBName,
		DBUser:      opts.DBUser,
		Port:        opts.Port,
		Version:     opts.Version,
		Description: opts.Description,
	}
	res, err := adopt.Detect(s.cfg, s.reg, dopts)
	if err != nil {
		return nil, err
	}
	inst := res.Inst
	if err := s.reg.Put(inst); err != nil {
		return nil, err
	}
	var dbs []string
	for _, d := range res.DBs {
		dbs = append(dbs, d.Name)
	}
	return &AdoptResult{
		Name:      inst.Name,
		Source:    inst.SourcePath,
		Conf:      inst.ConfPath,
		Port:      inst.Port,
		DBName:    inst.DBName,
		Version:   inst.Version,
		Databases: dbs,
		Warnings:  res.Warn,
	}, nil
}

// CloneResult carries what was produced by Clone.
type CloneResult struct {
	Name string `json:"name"`
	Port int    `json:"port"`
	DB   string `json:"db"`
}

// Clone creates a new instance by copying source, addons, data and database.
func (s *Service) Clone(ctx context.Context, name, newName string, port int, emit Sink) (*CloneResult, error) {
	src, err := s.reg.Get(name)
	if err != nil {
		return nil, err
	}
	if src.Adopted {
		return nil, fmt.Errorf("cannot clone an adopted instance")
	}
	if _, err := s.reg.Get(newName); err == nil {
		return nil, fmt.Errorf("instance %q already exists", newName)
	}

	sp := src.ResolvePaths(s.rootFor(src))

	if port == 0 {
		if !checker.PortInUse(src.Port) && s.portFree(src.Port) == nil {
			port = src.Port
		} else if p, err := s.findFreePort(8069); err == nil {
			port = p
		}
	}

	newInst := &instance.Instance{
		Name:        newName,
		Version:     src.Version,
		Branch:      src.Branch,
		SourceURL:   src.SourceURL,
		Port:        port,
		DBName:      newName,
		DBUser:      src.DBUser,
		Root:        src.Root,
		Description: "clone of " + name,
		Workers:     src.Workers,
		LogLevel:    src.LogLevel,
		PythonBin:   src.PythonBin,
	}
	np := newInst.ResolvePaths(s.rootFor(newInst))

	emit(Event{Kind: StepStart, Instance: newName, Step: "cloning addons"})
	_ = copyDir(sp.Addons, np.Addons)
	_ = copyDir(sp.DataDir, np.DataDir)
	emit(Event{Kind: StepDone, Instance: newName, Step: "cloning addons"})

	emit(Event{Kind: StepStart, Instance: newName, Step: "installing"})
	err = installer.Install(ctx, s.cfg, newInst, installer.Options{
		Version:  src.Version,
		Port:     port,
		DBUser:   src.DBUser,
		DBName:   newName,
		Workers:  src.Workers,
		LogLevel: src.LogLevel,
		Python:   src.PythonBin,
	}, LineSink(newName, "install", emit))
	if err != nil {
		emit(Event{Kind: StepFail, Instance: newName, Message: err.Error()})
		return nil, err
	}
	emit(Event{Kind: StepDone, Instance: newName, Step: "installing"})

	emit(Event{Kind: StepStart, Instance: newName, Step: "cloning database"})
	tmpFile, err := os.CreateTemp("", "odoonoir-clone-*.sql")
	if err != nil {
		emit(Event{Kind: StepFail, Instance: newName, Message: err.Error()})
		return nil, fmt.Errorf("create temp dump: %w", err)
	}
	tmpDump := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpDump)
	if err := s.pg.Backup(ctx, src.DBName, tmpDump, true, true); err != nil {
		emit(Event{Kind: StepFail, Instance: newName, Message: err.Error()})
		return nil, err
	}
	if err := s.pg.CreateDatabase(newName, src.DBUser); err != nil {
		emit(Event{Kind: StepFail, Instance: newName, Message: err.Error()})
		return nil, err
	}
	if err := s.pg.Restore(ctx, newName, tmpDump); err != nil {
		emit(Event{Kind: StepFail, Instance: newName, Message: err.Error()})
		return nil, err
	}
	emit(Event{Kind: StepDone, Instance: newName, Step: "cloning database"})

	if err := s.reg.Put(newInst); err != nil {
		return nil, err
	}
	return &CloneResult{Name: newName, Port: port, DB: newName}, nil
}

// ModuleView represents a single module from ir_module_module.
type ModuleView struct {
	Name      string `json:"name"`
	State     string `json:"state"`
	Installed bool   `json:"installed"`
	Shortdesc string `json:"shortdesc"`
	Author    string `json:"author"`
	Version   string `json:"version"`
	Custom    bool   `json:"custom"`
}

// ModuleList queries ir_module_module for the modules of a database.
func (s *Service) ModuleList(name, dbName string) ([]ModuleView, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return nil, err
	}
	if dbName, err = s.ResolveDB(inst, dbName, true); err != nil {
		return nil, err
	}
	query := "SELECT name, state, COALESCE(shortdesc::text, ''), COALESCE(author, ''), COALESCE(latest_version, '') FROM ir_module_module ORDER BY name"
	out, err := s.pg.Query(dbName, query)
	if err != nil {
		return nil, s.wrapPsqlErr(dbName, err)
	}
	// determine custom modules via disk scan
	p := inst.ResolvePaths(s.rootFor(inst))
	addonsPaths := []string{p.Addons}
	if len(inst.AddonsPaths) > 0 {
		addonsPaths = inst.AddonsPaths
	}
	customSet := map[string]bool{}
	if diskMods, _ := scanDiskModules(addonsPaths); diskMods != nil {
		for n := range diskMods {
			customSet[n] = true
		}
	}
	var modules []ModuleView
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 5)
		if len(parts) < 2 {
			continue
		}
		nm := parts[0]
		modules = append(modules, ModuleView{
			Name:      nm,
			State:     parts[1],
			Installed: parts[1] == "installed",
			Shortdesc: safeIdx(parts, 2),
			Author:    safeIdx(parts, 3),
			Version:   safeIdx(parts, 4),
			Custom:    customSet[nm],
		})
	}
	return modules, nil
}

func safeIdx(parts []string, i int) string {
	if i < len(parts) {
		return parts[i]
	}
	return ""
}

// ModuleUninstall uninstalls a module via the odoo shell.
func (s *Service) ModuleUninstall(ctx context.Context, name, moduleName, dbName string, emit Sink) error {
	inst, err := s.reg.Get(name)
	if err != nil {
		return err
	}
	if dbName == "" {
		dbName = inst.DBName
	}
	if !validModuleName.MatchString(moduleName) {
		return fmt.Errorf("invalid module name %q: must contain only alphanumerics, underscores, or dots", moduleName)
	}
	step := "uninstall " + moduleName
	emit(Event{Kind: StepStart, Instance: name, Step: step})
	p := inst.ResolvePaths(s.rootFor(inst))
	py := installer.PythonFor(inst, p)
	script := fmt.Sprintf("mods = env['ir.module.module'].search([('name', '=', '%s')])\nif not mods:\n    raise Exception('module not found: %s')\nmods.button_immediate_uninstall()\nprint('OK')\n", moduleName, moduleName)
	shell := exec.Command(py, "-m", "odoo", "shell", "-c", p.Conf, "-d", dbName)
	shell.Stdin = strings.NewReader(script)
	shell.Dir = p.Source
	out, err := shell.CombinedOutput()
	if err != nil {
		emit(Event{Kind: StepFail, Instance: name, Message: err.Error()})
		return fmt.Errorf("uninstall %s: %w\n%s", moduleName, err, strings.TrimSpace(string(out)))
	}
	emit(Event{Kind: StepDone, Instance: name, Step: step})
	return nil
}

// DoctorIssue is a detected problem from the log scan.
type DoctorIssue struct {
	Severity string `json:"severity"`
	Pattern  string `json:"pattern"`
	Message  string `json:"message"`
	Line     string `json:"line"`
	LineNo   int    `json:"lineNo"`
	Hint     string `json:"hint"`
}

// Doctor scans the instance log for known failure signatures.
func (s *Service) Doctor(name string) ([]DoctorIssue, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return nil, err
	}
	p := inst.ResolvePaths(s.rootFor(inst))
	if _, err := os.Stat(p.Log); os.IsNotExist(err) {
		return nil, nil
	}
	issues, err := logmon.Scan(p.Log)
	if err != nil {
		return nil, err
	}
	issues = logmon.Dedupe(issues)
	var out []DoctorIssue
	for _, it := range issues {
		out = append(out, DoctorIssue{
			Severity: it.Severity,
			Pattern:  it.Pattern,
			Message:  it.Message,
			Line:     it.Line,
			LineNo:   it.LineNo,
			Hint:     it.Hint,
		})
	}
	return out, nil
}

func (s *Service) portFree(port int) error {
	all, err := s.reg.All()
	if err != nil {
		return err
	}
	for _, inst := range all {
		if inst.Port == port {
			return fmt.Errorf("port %d is used by %s", port, inst.Name)
		}
	}
	return nil
}

func (s *Service) findFreePort(start int) (int, error) {
	for p := start; p < 65535; p++ {
		if !checker.PortInUse(p) && s.portFree(p) == nil {
			return p, nil
		}
	}
	return 0, fmt.Errorf("no free port found")
}

func copyDir(src, dst string) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	cmd := exec.Command("cp", "-r", src, dst)
	return cmd.Run()
}

// ═══════════════════════════════════════════════════════════════════════
// Module Diff
// ═══════════════════════════════════════════════════════════════════════

// DBModule represents a module from ir_module_module.
type DBModule struct {
	Name        string
	State       string
	Version     string
	Shortdesc   string
	Author      string
	Depends     []string
	Models      []string
	Views       []string
}

// DiskModule represents a module found on disk.
type DiskModule struct {
	Name        string
	Path        string
	Version     string
	Summary     string
	Author      string
	Depends     []string
	Models      []DiskModel
	Views       []DiskView
}

// DiskModel represents a model file on disk.
type DiskModel struct {
	Name        string
	Fields      []string
}

// DiskView represents a view XML on disk.
type DiskView struct {
	Name        string
	Type        string // list, form, search, etc.
	Model       string
}

// ModuleDiffResult represents differences between DB and disk.
type ModuleDiffResult struct {
	ModuleName    string   `json:"moduleName"`
	InDB          bool     `json:"inDB"`
	OnDisk        bool     `json:"onDisk"`
	DBState       string   `json:"dbState"`       // installed, uninstalled, to upgrade, to remove
	DiskVersion   string   `json:"diskVersion"`   // from __manifest__.py
	DBVersion     string   `json:"dbVersion"`     // from ir_module_module
	FieldsAdded   []string `json:"fieldsAdded"`   // fields on disk not in DB
	FieldsRemoved []string `json:"fieldsRemoved"` // fields in DB not on disk
	ModelsAdded   []string `json:"modelsAdded"`   // models on disk not in DB
	ModelsRemoved []string `json:"modelsRemoved"` // models in DB not on disk
	ViewsAdded    []string `json:"viewsAdded"`    // view XMLs on disk not in DB
	ViewsRemoved  []string `json:"viewsRemoved"`  // view XMLs in DB not on disk
	ManifestDiff  string   `json:"manifestDiff"`  // unified diff of __manifest__.py
}

// ModuleDiff compares installed modules with their disk manifests.
func (s *Service) ModuleDiff(name, dbName string) ([]ModuleDiffResult, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return nil, err
	}
	if dbName == "" {
		dbName = inst.DBName
	}
	p := inst.ResolvePaths(s.rootFor(inst))

	// 1. Get all modules from DB
	dbModules, err := s.getDBModules(dbName)
	if err != nil {
		return nil, err
	}

	// 2. Scan addons_path for modules with __manifest__.py
	addonsPaths := append([]string{p.Source}, inst.AddonsPaths...)
	diskModules, err := scanDiskModules(addonsPaths)
	if err != nil {
		return nil, err
	}

	// 3. Compare and compute diffs
	return computeModuleDiff(dbModules, diskModules), nil
}

// getDBModules queries ir_module_module with full field list.
func (s *Service) getDBModules(dbName string) (map[string]DBModule, error) {
	query := `SELECT name, state, COALESCE(latest_version, ''), COALESCE(shortdesc::text, ''),
		COALESCE(author, ''), COALESCE(depends::text, ''), COALESCE(models::text, '')
	FROM ir_module_module ORDER BY name`
	out, err := s.pg.Query(dbName, query)
	if err != nil {
		return nil, err
	}

	modules := make(map[string]DBModule)
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 7)
		if len(parts) < 2 {
			continue
		}
		name := parts[0]
		depends := []string{}
		if len(parts) > 5 && parts[5] != "" {
			// depends is a text array representation like {base,sale}
			depStr := strings.Trim(parts[5], "{}")
			if depStr != "" {
				depends = strings.Split(depStr, ",")
			}
		}
		models := []string{}
		if len(parts) > 6 && parts[6] != "" {
			modelStr := strings.Trim(parts[6], "{}")
			if modelStr != "" {
				models = strings.Split(modelStr, ",")
			}
		}
		modules[name] = DBModule{
			Name:    name,
			State:   parts[1],
			Version: parts[2],
			Shortdesc: parts[3],
			Author:  parts[4],
			Depends: depends,
			Models:  models,
		}
	}
	return modules, nil
}

// scanDiskModules scans addons directories for modules with __manifest__.py.
func scanDiskModules(addonsPaths []string) (map[string]DiskModule, error) {
	modules := make(map[string]DiskModule)
	for _, addonsPath := range addonsPaths {
		if _, err := os.Stat(addonsPath); os.IsNotExist(err) {
			continue
		}
		entries, err := os.ReadDir(addonsPath)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			manifestPath := filepath.Join(addonsPath, entry.Name(), "__manifest__.py")
			if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
				continue
			}
			dm, err := parseDiskModule(manifestPath, addonsPath, entry.Name())
			if err != nil {
				continue
			}
			modules[dm.Name] = dm
		}
	}
	return modules, nil
}

// parseDiskModule parses a __manifest__.py and scans for models/views.
func parseDiskModule(manifestPath, addonsPath, moduleName string) (DiskModule, error) {
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		return DiskModule{}, err
	}
	// Simple extraction of key fields from manifest
	// In production, use a proper Python AST parser
	dm := DiskModule{
		Name: moduleName,
		Path: filepath.Dir(manifestPath),
	}
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "'version'") || strings.HasPrefix(line, "\"version\"") {
			dm.Version = extractManifestValue(line)
		} else if strings.HasPrefix(line, "'summary'") || strings.HasPrefix(line, "\"summary\"") {
			dm.Summary = extractManifestValue(line)
		} else if strings.HasPrefix(line, "'author'") || strings.HasPrefix(line, "\"author\"") {
			dm.Author = extractManifestValue(line)
		} else if strings.HasPrefix(line, "'depends'") || strings.HasPrefix(line, "\"depends\"") {
			dm.Depends = extractManifestList(line)
		}
	}
	// Scan models/ directory
	modelsDir := filepath.Join(dm.Path, "models")
	if entries, err := os.ReadDir(modelsDir); err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".py") && !strings.HasPrefix(e.Name(), "__") {
				modelName := strings.TrimSuffix(e.Name(), ".py")
				dm.Models = append(dm.Models, DiskModel{Name: modelName})
			}
		}
	}
	// Scan views/ directory
	viewsDir := filepath.Join(dm.Path, "views")
	if entries, err := os.ReadDir(viewsDir); err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".xml") {
				dm.Views = append(dm.Views, DiskView{Name: e.Name()})
			}
		}
	}
	return dm, nil
}

func extractManifestValue(line string) string {
	// Extract value from 'key': 'value', or "key": "value",
	if idx := strings.Index(line, ":"); idx >= 0 {
		val := strings.TrimSpace(line[idx+1:])
		val = strings.Trim(val, ",")
		val = strings.Trim(val, "'\"")
		return val
	}
	return ""
}

func extractManifestList(line string) []string {
	// Extract list from 'depends': ['base', 'sale'],
	if idx := strings.Index(line, "["); idx >= 0 {
		end := strings.Index(line[idx:], "]")
		if end >= 0 {
			listStr := line[idx+1 : idx+end]
			items := strings.Split(listStr, ",")
			result := make([]string, 0, len(items))
			for _, item := range items {
				item = strings.TrimSpace(item)
				item = strings.Trim(item, "'\"")
				if item != "" {
					result = append(result, item)
				}
			}
			return result
		}
	}
	return nil
}

// computeModuleDiff compares DB and disk modules.
func computeModuleDiff(dbModules map[string]DBModule, diskModules map[string]DiskModule) []ModuleDiffResult {
	allNames := make(map[string]bool)
	for n := range dbModules {
		allNames[n] = true
	}
	for n := range diskModules {
		allNames[n] = true
	}

	var results []ModuleDiffResult
	for name := range allNames {
		dbMod, inDB := dbModules[name]
		diskMod, onDisk := diskModules[name]

		res := ModuleDiffResult{
			ModuleName: name,
			InDB:       inDB,
			OnDisk:     onDisk,
		}
		if inDB {
			res.DBState = dbMod.State
			res.DBVersion = dbMod.Version
		}
		if onDisk {
			res.DiskVersion = diskMod.Version
		}

		// Compare versions
		if inDB && onDisk && dbMod.Version != diskMod.Version {
			res.DBState = "to upgrade"
		}

		// Compare depends
		if inDB && onDisk {
			dbDepMap := make(map[string]bool)
			for _, d := range dbMod.Depends {
				dbDepMap[d] = true
			}
			for _, d := range diskMod.Depends {
				if !dbDepMap[d] {
					res.FieldsAdded = append(res.FieldsAdded, "depends:+"+d)
				}
			}
			for _, d := range dbMod.Depends {
				found := false
				for _, dd := range diskMod.Depends {
					if dd == d {
						found = true
						break
					}
				}
				if !found {
					res.FieldsRemoved = append(res.FieldsRemoved, "depends:-"+d)
				}
			}
		}

		// Compare models
		if inDB && onDisk {
			dbModelMap := make(map[string]bool)
			for _, m := range dbMod.Models {
				dbModelMap[m] = true
			}
			for _, m := range diskMod.Models {
				if !dbModelMap[m.Name] {
					res.ModelsAdded = append(res.ModelsAdded, m.Name)
				}
			}
			for _, m := range dbMod.Models {
				found := false
				for _, dm := range diskMod.Models {
					if dm.Name == m {
						found = true
						break
					}
				}
				if !found {
					res.ModelsRemoved = append(res.ModelsRemoved, m)
				}
			}
		}

		// Compare views
		if inDB && onDisk {
			dbViewMap := make(map[string]bool)
			// Views not directly in DB module, would need separate query
			for _, v := range diskMod.Views {
				if !dbViewMap[v.Name] {
					res.ViewsAdded = append(res.ViewsAdded, v.Name)
				}
			}
		}

		// Manifest diff
		if inDB && onDisk {
			dbManifest := fmt.Sprintf("name: %s\nversion: %s\ndepends: %v\n", dbMod.Name, dbMod.Version, dbMod.Depends)
			diskManifest := fmt.Sprintf("name: %s\nversion: %s\ndepends: %v\n", diskMod.Name, diskMod.Version, diskMod.Depends)
			dmp := diffmatchpatch.New()
			diffs := dmp.DiffMain(dbManifest, diskManifest, false)
			res.ManifestDiff = dmp.DiffPrettyText(diffs)
		}

		results = append(results, res)
	}

	// Sort by module name
	sort.Slice(results, func(i, j int) bool {
		return results[i].ModuleName < results[j].ModuleName
	})
	return results
}

// ═══════════════════════════════════════════════════════════════════════
// Model Inspector
// ═══════════════════════════════════════════════════════════════════════

// FieldInfo represents a field's metadata.
type FieldInfo struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	String        string `json:"string"`
	Required      bool   `json:"required"`
	Readonly      bool   `json:"readonly"`
	Index         bool   `json:"index"`
	Store         bool   `json:"store"`
	Relation      string `json:"relation"`       // for relational fields
	RelationTable string `json:"relationTable"`  // for many2many
	ComodelName   string `json:"comodelName"`    // for many2one
	Domain        string `json:"domain"`
	Default       string `json:"default"`
	Groups        string `json:"groups"`
	Help          string `json:"help"`
}

// ACLInfo represents ir.model.access rules.
type ACLInfo struct {
	Name       string `json:"name"`
	Group      string `json:"group"`
	PermRead   bool   `json:"permRead"`
	PermWrite  bool   `json:"permWrite"`
	PermCreate bool   `json:"permCreate"`
	PermUnlink bool   `json:"permUnlink"`
}

// ViewInfo represents ir.ui.view records.
type ViewInfo struct {
	Name    string `json:"name"`
	Type    string `json:"type"` // tree, form, search, etc.
	Model   string `json:"model"`
	Arch    string `json:"arch"`
	Active  bool   `json:"active"`
	Priority int   `json:"priority"`
}

// ActionInfo represents ir.actions.act_window records.
type ActionInfo struct {
	Name     string `json:"name"`
	ResModel string `json:"resModel"`
	ViewMode string `json:"viewMode"`
	Domain   string `json:"domain"`
	Context  string `json:"context"`
}

// ModelInfo represents a model's full metadata.
type ModelInfo struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Table       string      `json:"table"`
	RecName     string      `json:"recName"`
	Order       string      `json:"order"`
	Fields      []FieldInfo `json:"fields"`
	Access      []ACLInfo   `json:"access"`
	Views       []ViewInfo  `json:"views"`
	Actions     []ActionInfo `json:"actions"`
}

// ModelInfo fetches complete model metadata from the database.
func (s *Service) ModelInfo(name, dbName, model string) (*ModelInfo, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return nil, err
	}
	var resolveErr error
	if dbName, resolveErr = s.ResolveDB(inst, dbName, true); resolveErr != nil {
		return nil, resolveErr
	}

	// Query ir_model for model metadata
	modelQuery := fmt.Sprintf(`SELECT model, name, info, table_name, rec_name, "order"
		FROM ir_model WHERE model = '%s'`, db.PgEscapeLiteral(model))
	modelRows, err := s.pg.Query(dbName, modelQuery)
	if err != nil {
		return nil, s.wrapPsqlErr(dbName, err)
	}
	mi := &ModelInfo{Name: model}
	modelLines := strings.Split(modelRows, "\n")
	if len(modelLines) > 0 && modelLines[0] != "" {
		parts := strings.SplitN(modelLines[0], "|", 6)
		if len(parts) >= 6 {
			mi.Description = parts[1]
			mi.Table = parts[3]
			mi.RecName = parts[4]
			mi.Order = parts[5]
		}
	}

	// Query ir_model_fields for fields
	fieldsQuery := fmt.Sprintf(`SELECT name, ttype, field_description, required, readonly,
		index, store, relation, relation_table, comodel_name,
		domain, default, groups, help
		FROM ir_model_fields WHERE model = '%s' ORDER BY name`, db.PgEscapeLiteral(model))
	fieldsRows, err := s.pg.Query(dbName, fieldsQuery)
	if err == nil {
		for _, line := range strings.Split(fieldsRows, "\n") {
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "|", 14)
			if len(parts) >= 14 {
				mi.Fields = append(mi.Fields, FieldInfo{
					Name:          parts[0],
					Type:          parts[1],
					String:        parts[2],
					Required:      parts[3] == "t",
					Readonly:      parts[4] == "t",
					Index:         parts[5] == "t",
					Store:         parts[6] == "t",
					Relation:      parts[7],
					RelationTable: parts[8],
					ComodelName:   parts[9],
					Domain:        parts[10],
					Default:       parts[11],
					Groups:        parts[12],
					Help:          parts[13],
				})
			}
		}
	}

	// Query ir_model_access for ACLs
	accessQuery := fmt.Sprintf(`SELECT name, group_id:id, perm_read, perm_write, perm_create, perm_unlink
		FROM ir_model_access WHERE model_id:id = (SELECT id FROM ir_model WHERE model = '%s')`, db.PgEscapeLiteral(model))
	accessRows, err := s.pg.Query(dbName, accessQuery)
	if err == nil {
		for _, line := range strings.Split(accessRows, "\n") {
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "|", 6)
			if len(parts) >= 6 {
				mi.Access = append(mi.Access, ACLInfo{
					Name:       parts[0],
					Group:      parts[1],
					PermRead:   parts[2] == "t",
					PermWrite:  parts[3] == "t",
					PermCreate: parts[4] == "t",
					PermUnlink: parts[5] == "t",
				})
			}
		}
	}

	// Query ir_ui_view for views
	viewsQuery := fmt.Sprintf(`SELECT name, type, model, arch_db, active, priority
		FROM ir_ui_view WHERE model = '%s' ORDER BY priority`, db.PgEscapeLiteral(model))
	viewsRows, err := s.pg.Query(dbName, viewsQuery)
	if err == nil {
		for _, line := range strings.Split(viewsRows, "\n") {
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "|", 6)
			if len(parts) >= 6 {
				mi.Views = append(mi.Views, ViewInfo{
					Name:     parts[0],
					Type:     parts[1],
					Model:    parts[2],
					Arch:     parts[3],
					Active:   parts[4] == "t",
					Priority: parseInt(parts[5]),
				})
			}
		}
	}

	// Query ir_actions_act_window for actions
	actionsQuery := fmt.Sprintf(`SELECT name, res_model, view_mode, domain, context
		FROM ir_actions_act_window WHERE res_model = '%s'`, db.PgEscapeLiteral(model))
	actionsRows, err := s.pg.Query(dbName, actionsQuery)
	if err == nil {
		for _, line := range strings.Split(actionsRows, "\n") {
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "|", 5)
			if len(parts) >= 5 {
				mi.Actions = append(mi.Actions, ActionInfo{
					Name:     parts[0],
					ResModel: parts[1],
					ViewMode: parts[2],
					Domain:   parts[3],
					Context:  parts[4],
				})
			}
		}
	}

	return mi, nil
}

func parseInt(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}

// ═══════════════════════════════════════════════════════════════════════
// Cron / Scheduled Actions
// ═══════════════════════════════════════════════════════════════════════

// CronEntry represents an ir.cron record.
type CronEntry struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Model        string `json:"model"`
	Function     string `json:"function"`
	Args         string `json:"args"`
	IntervalType string `json:"intervalType"` // minutes, hours, days, weeks, months
	IntervalNum  int    `json:"intervalNumber"`
	NextCall     string `json:"nextCall"`
	NumberCall   int    `json:"numberCall"` // -1 = infinite
	DoAll        bool   `json:"doAll"`
	Active       bool   `json:"active"`
	Priority     int    `json:"priority"`
}

// CronList returns all scheduled actions for a database.
func (s *Service) CronList(name, dbName string) ([]CronEntry, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return nil, err
	}
	dbName, err = s.ResolveDB(inst, dbName, true)
	if err != nil {
		return nil, err
	}
	query := `SELECT id, name, model, function, args, interval_type, interval_number,
		nextcall, numbercall, doall, active, priority
		FROM ir_cron ORDER BY nextcall`
	out, err := s.pg.Query(dbName, query)
	if err != nil {
		return nil, s.wrapPsqlErr(dbName, err)
	}

	var entries []CronEntry
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 12)
		if len(parts) < 12 {
			continue
		}
		entries = append(entries, CronEntry{
			ID:           parseInt(parts[0]),
			Name:         parts[1],
			Model:        parts[2],
			Function:     parts[3],
			Args:         parts[4],
			IntervalType: parts[5],
			IntervalNum:  parseInt(parts[6]),
			NextCall:     parts[7],
			NumberCall:   parseInt(parts[8]),
			DoAll:        parts[9] == "t",
			Active:       parts[10] == "t",
			Priority:     parseInt(parts[11]),
		})
	}
	return entries, nil
}

// ═══════════════════════════════════════════════════════════════════════
// Module Dependency Graph
// ═══════════════════════════════════════════════════════════════════════

// ModuleDepNode represents a node in the dependency graph.
type ModuleDepNode struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"displayName"`
	State       string   `json:"state"`       // installed, uninstalled, to upgrade, to remove
	Version     string   `json:"version"`     // from __manifest__.py
	Category    string   `json:"category"`
	Author      string   `json:"author"`
	Summary     string   `json:"summary"`
	Depends     []string `json:"depends"`     // direct dependencies
	DependedBy  []string `json:"dependedBy"`  // modules that depend on this
	OnDisk      bool     `json:"onDisk"`      // has __manifest__.py
	InDB        bool     `json:"inDB"`        // in ir_module_module
}

// ModuleDepLink represents an edge in the dependency graph.
type ModuleDepLink struct {
	Source string `json:"source"` // module name
	Target string `json:"target"` // module name
	Type   string `json:"type"`   // "depends", "reverse"
}

// ModuleDepGraph is the full dependency graph.
type ModuleDepGraph struct {
	Nodes []ModuleDepNode `json:"nodes"`
	Links []ModuleDepLink `json:"links"`
}

// ModuleDepGraphOptions configures the graph generation.
type ModuleDepGraphOptions struct {
	IncludeUninstalled bool     `json:"includeUninstalled"` // show uninstalled modules
	OnlyInstalled      bool     `json:"onlyInstalled"`      // only show installed modules
	Categories         []string `json:"categories"`         // filter by category
}

// ModuleDepGraph returns the module dependency graph for an instance.
func (s *Service) ModuleDepGraph(name, dbName string, opts ModuleDepGraphOptions) (*ModuleDepGraph, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return nil, err
	}
	if dbName, err = s.ResolveDB(inst, dbName, true); err != nil {
		return nil, err
	}
	p := inst.ResolvePaths(s.rootFor(inst))

	// Get modules from DB
	dbModules, err := s.getDBModules(dbName)
	if err != nil {
		return nil, s.wrapPsqlErr(dbName, err)
	}

	// Get modules from disk
	addonsPaths := append([]string{p.Source}, inst.AddonsPaths...)
	diskModules, err := scanDiskModules(addonsPaths)
	if err != nil {
		return nil, err
	}

	// Build combined module map
	allNames := make(map[string]bool)
	for n := range dbModules {
		allNames[n] = true
	}
	for n := range diskModules {
		allNames[n] = true
	}

	// Filter by options
	filteredNames := make(map[string]bool)
	for name := range allNames {
		dbMod, inDB := dbModules[name]
		diskMod, onDisk := diskModules[name]

		if opts.OnlyInstalled && (!inDB || dbMod.State != "installed") {
			continue
		}
		if !opts.IncludeUninstalled && !inDB && onDisk {
			continue
		}
		if len(opts.Categories) > 0 {
			catMatch := false
			for _, cat := range opts.Categories {
				if (inDB && strings.Contains(strings.ToLower(dbMod.Shortdesc), strings.ToLower(cat))) ||
					(onDisk && strings.Contains(strings.ToLower(diskMod.Summary), strings.ToLower(cat))) {
					catMatch = true
					break
				}
			}
			if !catMatch {
				continue
			}
		}
		filteredNames[name] = true
	}

	// Build nodes
	nodes := make([]ModuleDepNode, 0, len(filteredNames))
	nameToNode := make(map[string]*ModuleDepNode)
	for name := range filteredNames {
		dbMod, inDB := dbModules[name]
		diskMod, onDisk := diskModules[name]

		state := "not_installed"
		if inDB {
			state = dbMod.State
		}

		node := ModuleDepNode{
			Name:        name,
			DisplayName: name,
			State:       state,
			OnDisk:      onDisk,
			InDB:        inDB,
			Depends:     []string{},
		}

		if inDB {
			node.Version = dbMod.Version
			node.Depends = dbMod.Depends
			// Extract display name from shortdesc or use name
			if dbMod.Shortdesc != "" {
				node.DisplayName = dbMod.Shortdesc
			}
			node.Author = dbMod.Author
		}
		if onDisk {
			if diskMod.Version != "" {
				node.Version = diskMod.Version
			}
			if diskMod.Summary != "" {
				node.DisplayName = diskMod.Summary
			}
			if diskMod.Author != "" {
				node.Author = diskMod.Author
			}
			// Merge depends
			depMap := make(map[string]bool)
			for _, d := range node.Depends {
				depMap[d] = true
			}
			for _, d := range diskMod.Depends {
				depMap[d] = true
			}
			node.Depends = make([]string, 0, len(depMap))
			for d := range depMap {
				node.Depends = append(node.Depends, d)
			}
		}

		nameToNode[name] = &node
		nodes = append(nodes, node)
	}

	// Build reverse dependencies (dependedBy)
	for _, node := range nodes {
		for _, dep := range node.Depends {
			if target, ok := nameToNode[dep]; ok {
				target.DependedBy = append(target.DependedBy, node.Name)
			}
		}
	}

	// Build links
	links := make([]ModuleDepLink, 0)
	for _, node := range nodes {
		for _, dep := range node.Depends {
			if _, ok := nameToNode[dep]; ok {
				links = append(links, ModuleDepLink{
					Source: node.Name,
					Target: dep,
					Type:   "depends",
				})
			}
		}
	}

	// Sort nodes by name for consistent output
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Name < nodes[j].Name
	})

	return &ModuleDepGraph{
		Nodes: nodes,
		Links: links,
	}, nil
}
