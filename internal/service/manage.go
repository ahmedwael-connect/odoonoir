package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ahmed/odoonoir/internal/adopt"
	"github.com/ahmed/odoonoir/internal/checker"
	"github.com/ahmed/odoonoir/internal/instance"
	"github.com/ahmed/odoonoir/internal/installer"
	"github.com/ahmed/odoonoir/internal/logmon"
	"github.com/ahmed/odoonoir/internal/proc"
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
	mgr := proc.New(p, installer.PythonFor(inst, p), p.Conf)
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
}

// ModuleList queries ir_module_module for the modules of a database.
func (s *Service) ModuleList(name, dbName string) ([]ModuleView, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return nil, err
	}
	if dbName == "" {
		dbName = inst.DBName
	}
	query := "SELECT name, state, COALESCE(shortdesc::text, ''), COALESCE(author, ''), COALESCE(latest_version, '') FROM ir_module_module ORDER BY name"
	out, err := s.pg.Query(dbName, query)
	if err != nil {
		return nil, err
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
		modules = append(modules, ModuleView{
			Name:      parts[0],
			State:     parts[1],
			Installed: parts[1] == "installed",
			Shortdesc: safeIdx(parts, 2),
			Author:    safeIdx(parts, 3),
			Version:   safeIdx(parts, 4),
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
