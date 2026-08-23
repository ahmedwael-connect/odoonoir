package installer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ahmed/odoonoir/internal/config"
	"github.com/ahmed/odoonoir/internal/instance"
	"github.com/ahmed/odoonoir/internal/odoconf"
)

// Version describes a supported Odoo release.
type Version struct {
	Major       string
	Branch      string // git branch, e.g. "18.0"
	Enterprise  bool
	Python      string // python binary to use
	RepoDefault string
}

// Options controls an install run.
type Options struct {
	Version    string // "17", "18", "19"
	Enterprise bool
	Port       int
	LongPoll   int
	DBUser     string
	DBPass     string
	DBName     string
	DevMode    bool              // install dev requirements + set dev_mode conf
	Workers    int               // odoo worker count
	LogLevel   string            // debug, info, warning, error
	GitRef     string            // pin source to a tag/commit instead of the branch
	Python     string            // python interpreter for the venv
	Extra      map[string]string // extra odoo.conf key=value pairs
}

// BranchFor maps a major version to its git branch name.
func BranchFor(major string) string { return major + ".0" }

// RootFor returns the instance's storage root: the per-instance override
// when set, otherwise the global instances directory.
func RootFor(cfg *config.Config, inst *instance.Instance) string {
	if inst.Root != "" {
		return inst.Root
	}
	return cfg.InstancesDir()
}

// Install clones the Odoo source, creates the venv, installs requirements,
// generates the conf and scaffolds the custom_addons folder. Cancelling ctx
// aborts the install (and the running clone/pip command).
func Install(ctx context.Context, cfg *config.Config, inst *instance.Instance, opts Options, stdout func(string)) error {
	p := inst.ResolvePaths(RootFor(cfg, inst))
	steps := []struct {
		name string
		fn   func() error
	}{
		{"clone source", func() error { return cloneSource(ctx, p.Source, inst.SourceURL, branchFor(inst, opts), stdout) }},
		{"create venv", func() error { return createVenv(ctx, p.Venv, opts, stdout) }},
		{"install python requirements", func() error { return installRequirements(ctx, p, opts, stdout) }},
		{"write odoo.conf", func() error { return writeConf(p, inst, opts) }},
		{"scaffold custom_addons", func() error { return scaffoldAddons(p.Addons, inst.Name) }},
		{"create data dir", func() error { return os.MkdirAll(p.DataDir, 0o755) }},
	}
	for _, s := range steps {
		stdout("==> " + s.name)
		if err := s.fn(); err != nil {
			return fmt.Errorf("%s: %w", s.name, err)
		}
	}
	return nil
}

// branchFor returns the git ref to clone: explicit --git-ref wins, else the
// version branch. Falls back to the branch stored on the instance.
func branchFor(inst *instance.Instance, opts Options) string {
	if opts.GitRef != "" {
		return opts.GitRef
	}
	if inst.Branch != "" {
		return inst.Branch
	}
	return BranchFor(strings.Split(opts.Version, ".")[0])
}

func cloneSource(ctx context.Context, src, url, ref string, stdout func(string)) error {
	if _, err := os.Stat(filepath.Join(src, ".git")); err == nil {
		stdout("source already present, skipping clone")
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		return err
	}
	if url == "" {
		url = "https://github.com/odoo/odoo.git"
	}
	if err := runLive(ctx, stdout, "git", "clone", "--depth", "1", "-b", ref, url, src); err != nil {
		return fmt.Errorf("git clone %s (ref %s): %w", url, ref, err)
	}
	return nil
}

func createVenv(ctx context.Context, venvDir string, opts Options, stdout func(string)) error {
	if _, err := os.Stat(filepath.Join(venvDir, "bin", "python")); err == nil {
		stdout("venv already exists")
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(venvDir), 0o755); err != nil {
		return err
	}
	py := opts.Python
	if py == "" {
		py = pythonFor(opts.Version)
	}
	if err := runLive(ctx, stdout, py, "-m", "venv", venvDir); err != nil {
		return fmt.Errorf("python venv: %w", err)
	}
	return nil
}

// PythonBin returns the venv python binary path.
func PythonBin(venvDir string) string { return filepath.Join(venvDir, "bin", "python") }

// PythonFor resolves the python interpreter for an instance: an explicit
// override (adopted instances, or --python at create time) wins, otherwise
// the venv binary. When neither exists, the system python3 is used, so
// venv-less adopted instances still work.
func PythonFor(inst *instance.Instance, p instance.Paths) string {
	if inst.PythonBin != "" {
		return inst.PythonBin
	}
	py := PythonBin(p.Venv)
	if _, err := os.Stat(py); err == nil {
		return py
	}
	return "python3"
}

func installRequirements(ctx context.Context, p instance.Paths, opts Options, stdout func(string)) error {
	reqFile := filepath.Join(p.Source, "requirements.txt")
	if _, err := os.Stat(reqFile); err != nil {
		return fmt.Errorf("requirements.txt not found in source")
	}
	py := PythonBin(p.Venv)
	if err := runLive(ctx, stdout, py, "-m", "pip", "install", "--upgrade", "pip", "setuptools", "wheel"); err != nil {
		return fmt.Errorf("pip bootstrap: %w", err)
	}
	args := []string{"-m", "pip", "install", "-r", reqFile}
	if err := runLive(ctx, stdout, py, args...); err != nil {
		return fmt.Errorf("pip install requirements: %w", err)
	}
	return nil
}

// WriteConf generates and persists the instance's odoo.conf from options.
func WriteConf(p instance.Paths, inst *instance.Instance, opts Options) error {
	return writeConf(p, inst, opts)
}

func writeConf(p instance.Paths, inst *instance.Instance, opts Options) error {
	c := odoconf.New()
	c.Set("addons_path", strings.Join(BuildAddonsPath(p), ","))
	c.Set("data_dir", p.DataDir)
	c.Set("db_host", "localhost")
	c.Set("db_port", "5432")
	c.Set("db_user", opts.DBUser)
	if opts.DBPass != "" {
		c.Set("db_password", opts.DBPass)
	}
	c.Set("db_name", opts.DBName)
	c.Set("http_port", fmt.Sprint(opts.Port))
	if opts.LongPoll > 0 {
		c.Set("longpolling_port", fmt.Sprint(opts.LongPoll))
	}
	c.Set("logfile", p.Log)
	if opts.Workers > 0 {
		c.Set("workers", fmt.Sprint(opts.Workers))
	}
	if opts.LogLevel != "" {
		c.Set("log_level", opts.LogLevel)
	}
	if opts.DevMode {
		c.Set("dev_mode", "all")
		c.Set("log_level", "debug")
	}
	for k, v := range opts.Extra {
		c.Set(k, v)
	}
	return c.Write(p.Conf)
}

func scaffoldAddons(addonsDir, name string) error {
	if err := os.MkdirAll(addonsDir, 0o755); err != nil {
		return err
	}
	readme := filepath.Join(addonsDir, "README.md")
	if _, err := os.Stat(readme); os.IsNotExist(err) {
		content := fmt.Sprintf("# %s custom addons\n\nModules for instance **%s**.\n\n"+
			"Create a module with:\n\n    odoonoir module new --instance %s <module_name>\n",
			name, name, name)
		if err := os.WriteFile(readme, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// BuildAddonsPath probes the checked-out source for its addon directories.
// Odoo restructured its repo (18.0, 2026+): community addons moved from
// odoo/addons/ to a top-level addons/. Both layouts are detected.
func BuildAddonsPath(p instance.Paths) []string {
	paths := []string{}
	if dirExists(filepath.Join(p.Source, "addons")) {
		paths = append(paths, filepath.Join(p.Source, "addons"))
	}
	if dirExists(filepath.Join(p.Source, "odoo", "addons")) {
		paths = append(paths, filepath.Join(p.Source, "odoo", "addons"))
	}
	paths = append(paths, p.Addons)
	return paths
}

func dirExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func pythonFor(version string) string {
	major := strings.Split(version, ".")[0]
	switch major {
	case "19":
		for _, py := range []string{"python3.13", "python3.12", "python3.11"} {
			if _, err := exec.LookPath(py); err == nil {
				return py
			}
		}
	case "18":
		for _, py := range []string{"python3.13", "python3.12", "python3.11", "python3.10"} {
			if _, err := exec.LookPath(py); err == nil {
				return py
			}
		}
	default:
		for _, py := range []string{"python3.12", "python3.11", "python3.10"} {
			if _, err := exec.LookPath(py); err == nil {
				return py
			}
		}
	}
	return "python3"
}

func runLive(ctx context.Context, stdout func(string), name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = lineWriter{fn: stdout}
	cmd.Stderr = lineWriter{fn: stdout}
	cmd.Env = append(os.Environ(), "PIP_DISABLE_PIP_VERSION_CHECK=1")
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	return nil
}

type lineWriter struct{ fn func(string) }

func (l lineWriter) Write(p []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		if strings.TrimSpace(line) != "" {
			l.fn("    " + line)
		}
	}
	return len(p), nil
}

// Touch updates instance metadata after install.
func Touch(inst *instance.Instance) { inst.UpdatedAt = time.Now() }
