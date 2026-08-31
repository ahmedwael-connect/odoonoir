// Package updater implements the instance update pipeline (git pull, conf
// refresh, requirements reinstall, module install/upgrade) with streaming
// progress, shared by the CLI `update` command and the TUI dashboard.
package updater

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ahmed/odoonoir/internal/config"
	"github.com/ahmed/odoonoir/internal/installer"
	"github.com/ahmed/odoonoir/internal/instance"
	"github.com/ahmed/odoonoir/internal/odoconf"
)

// Options mirrors the update CLI flags.
type Options struct {
	InstallMods []string
	UpdateMods  []string
	UpgradeAll  bool
	DB          string // database to run module operations on (empty = primary)
}

// ProgressKind describes the kind of progress event.
type ProgressKind int

const (
	// StepStart marks a pipeline step beginning.
	StepStart ProgressKind = iota
	// StepDone marks a pipeline step completing successfully.
	StepDone
	// Line streams one line of command output.
	Line
)

// Progress is a single update progress event.
type Progress struct {
	Kind  ProgressKind
	Index int
	Name  string
	Line  string
}

func stepNames() []string {
	return []string{
		"pull latest source",
		"refresh addons_path",
		"reinstall requirements",
		"install/upgrade modules",
	}
}

// Run executes the full update pipeline for an instance, streaming progress
// through on. Module install/upgrade is skipped when no modules are requested.
// Cancelling ctx aborts the running pipeline (and the underlying command).
func Run(ctx context.Context, cfg *config.Config, inst *instance.Instance, opts Options, on func(Progress)) error {
	root := inst.Root
	if root == "" {
		root = cfg.InstancesDir()
	}
	p := inst.ResolvePaths(root)
	names := stepNames()

	emit := func(kind ProgressKind, i int, line string) {
		on(Progress{Kind: kind, Index: i, Name: names[i], Line: line})
	}

	emit(StepStart, 0, "")
	if err := gitPull(ctx, p.Source, func(l string) { emit(Line, 0, l) }); err != nil {
		return fmt.Errorf("git pull: %w", err)
	}
	emit(StepDone, 0, "")

	emit(StepStart, 1, "")
	if inst.Adopted {
		// the conf belongs to an existing installation: never rewrite it
		on(Progress{Kind: Line, Index: 1, Name: names[1], Line: "(skipped — adopted instance, conf left untouched)"})
	} else if err := refreshAddonsPath(p); err != nil {
		return fmt.Errorf("refresh addons_path: %w", err)
	}
	emit(StepDone, 1, "")

	emit(StepStart, 2, "")
	py := installer.PythonFor(inst, p)
	reqFile := filepath.Join(p.Source, "requirements.txt")
	if err := installer.PatchRequirements(reqFile, inst.Version, func(l string) { emit(Line, 2, l) }); err != nil {
		return fmt.Errorf("patch requirements: %w", err)
	}
	if err := pipInstall(ctx, py, p.Source, func(l string) { emit(Line, 2, l) }); err != nil {
		return fmt.Errorf("pip install: %w", err)
	}
	emit(StepDone, 2, "")

	runMods := len(opts.InstallMods) > 0 || len(opts.UpdateMods) > 0 || opts.UpgradeAll
	if !runMods {
		return nil
	}
	emit(StepStart, 3, "")
	if err := InstallModules(ctx, inst, root, opts.DB, opts.InstallMods, opts.UpdateMods,
		func(l string) { emit(Line, 3, l) }); err != nil {
		return fmt.Errorf("module install/upgrade: %w", err)
	}
	emit(StepDone, 3, "")
	return nil
}

// InitDatabase creates the base schema of a database by running odoo-bin
// with -i base in stop-after-init mode. Used by `odoonoir init`.
func InitDatabase(ctx context.Context, inst *instance.Instance, root, dbName string, on func(string)) error {
	p := inst.ResolvePaths(root)
	py := installer.PythonFor(inst, p)
	args := []string{filepath.Join(p.Source, "odoo-bin"), "-c", p.Conf,
		"-d", dbName, "-i", "base", "--stop-after-init", "--no-http"}
	return runStream(ctx, exec.CommandContext(ctx, py, args...), p.Source, on)
}

// InstallModules runs odoo-bin -i/-u against an instance database in
// stop-after-init mode, streaming output line by line. An empty dbName
// targets the instance's primary database.
func InstallModules(ctx context.Context, inst *instance.Instance, root, dbName string, install, update []string, on func(string)) error {
	p := inst.ResolvePaths(root)
	py := installer.PythonFor(inst, p)
	if dbName == "" {
		dbName = inst.DBName
	}
	args := []string{filepath.Join(p.Source, "odoo-bin"), "-c", p.Conf,
		"-d", dbName, "--stop-after-init", "--no-http"}
	if len(install) > 0 {
		args = append(args, "-i", strings.Join(install, ","))
	}
	if len(update) > 0 {
		args = append(args, "-u", strings.Join(update, ","))
	}
	return runStream(ctx, exec.CommandContext(ctx, py, args...), p.Source, on)
}

// RunTests runs the test suite of the given modules against an instance
// database (odoo --test-enable -u <mods> --stop-after-init), streaming
// output line by line. An empty dbName targets the instance's primary
// database.
func RunTests(ctx context.Context, inst *instance.Instance, root, dbName string, modules []string, on func(string)) error {
	p := inst.ResolvePaths(root)
	py := installer.PythonFor(inst, p)
	if dbName == "" {
		dbName = inst.DBName
	}
	args := []string{filepath.Join(p.Source, "odoo-bin"), "-c", p.Conf,
		"-d", dbName, "--stop-after-init", "--no-http",
		"--test-enable", "-u", strings.Join(modules, ",")}
	return runStream(ctx, exec.CommandContext(ctx, py, args...), p.Source, on)
}

// refreshAddonsPath makes sure the conf's addons_path covers the current
// source layout (handles the odoo/addons -> addons/ repo restructure).
// User-made entries and their order are preserved: only missing built-in
// paths are appended, so `odoonoir config addons` reordering survives
// updates.
func refreshAddonsPath(p instance.Paths) error {
	paths := installer.BuildAddonsPath(p)
	c, err := odoconf.Load(p.Conf)
	if err != nil {
		return err
	}
	cur, err := c.AddonsPath()
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, x := range cur {
		have[x] = true
	}
	changed := false
	for _, x := range paths {
		if !have[x] {
			cur = append(cur, x)
			have[x] = true
			changed = true
		}
	}
	if !changed {
		return nil
	}
	if err := c.SetAddonsPath(cur); err != nil {
		return err
	}
	return c.Save()
}

func gitPull(ctx context.Context, dir string, on func(string)) error {
	return runStream(ctx, exec.CommandContext(ctx, "git", "-C", dir, "pull", "--ff-only"), "", on)
}

func pipInstall(ctx context.Context, py, src string, on func(string)) error {
	cmd := exec.CommandContext(ctx, py, "-m", "pip", "install", "-r",
		filepath.Join(src, "requirements.txt"))
	return runStream(ctx, cmd, src, on)
}

// runStream runs cmd with combined output streamed line by line through on.
// The command is killed when ctx is cancelled.
func runStream(ctx context.Context, cmd *exec.Cmd, dir string, on func(string)) error {
	pr, pw, err := os.Pipe()
	if err != nil {
		return err
	}
	defer pr.Close()
	cmd.Stdout = pw
	cmd.Stderr = pw
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		pw.Close()
		return err
	}
	pw.Close()
	sc := bufio.NewScanner(pr)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		on(sc.Text())
	}
	err = cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return err
	}
	return sc.Err()
}
