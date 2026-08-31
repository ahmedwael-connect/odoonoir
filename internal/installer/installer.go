package installer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ahmed/odoonoir/internal/checker"
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
	Version    string // "16", "17", "18", "19"
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
	// Validate that the resolved Python is compatible with the target Odoo version.
	if err := CheckPythonCompat(py, opts.Version); err != nil {
		return err
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
	// Patch pinned versions that won't compile on modern Python/setuptools.
	if err := PatchRequirements(reqFile, opts.Version, stdout); err != nil {
		return fmt.Errorf("patch requirements: %w", err)
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

// PatchRequirements rewrites known broken pinned versions in requirements.txt
// for older Odoo versions running on modern Python.
var versionOverrides = map[string]map[string]string{
	"16": {
		"gevent==21.8.0":      "gevent==22.10.2",
		"greenlet==1.1.2":     "greenlet>=2.0.0",
		"cryptography==3.4.8": "cryptography>=3.4.8,<42",
		"docutils==0.16":      "docutils>=0.16,<0.21",
		"Pillow==9.0.1":       "Pillow>=9.4.0,<11",
		"Pillow==9.4.0":       "Pillow>=9.4.0,<11",
		"psycopg2==2.9.2":     "psycopg2>=2.9.5,<3",
		"psycopg2==2.9.5":     "psycopg2>=2.9.5,<3",
		"simplejson==3.19.1":  "simplejson>=3.19.1,<4",
		"PyYAML==6.0":         "PyYAML>=6.0,<7",
		"structlog==21.5.0":   "structlog>=21.5.0,<24",
		"libsass==0.20.1":     "libsass>=0.22.0,<1",
		"libsass==0.22.0":     "libsass>=0.22.0,<1",
		"MarkupSafe==1.1.1":   "MarkupSafe>=2.0,<3",
		"Jinja2==2.11.3":      "Jinja2>=2.11.3,<4",
		"Werkzeug==2.0.2":     "Werkzeug>=2.0.2,<3",
		"lxml==4.6.5":         "lxml>=4.6.5,<6",
		"psutil==5.8.0":       "psutil>=5.8.0,<6",
		"reportlab==3.5.59":   "reportlab>=3.5.59,<4",
		"urllib3==1.26.5":      "urllib3>=1.26.5,<2",
		"requests==2.25.1":    "requests>=2.25.1,<3",
		"decorator==4.4.2":    "decorator>=4.4.2,<5",
		"Babel==2.9.1":        "Babel>=2.9.1,<3",
		"chardet==4.0.0":      "chardet>=4.0.0,<6",
		"idna==2.10":          "idna>=2.10,<4",
		"isodate==0.6.0":      "isodate>=0.6.0,<1",
		"pytz==2021.3":        "pytz>=2021.3,<2025",
		"zeep==4.1.0":         "zeep>=4.1.0,<5",
	},
}

func PatchRequirements(reqFile, odooVersion string, stdout func(string)) error {
	major := strings.Split(odooVersion, ".")[0]
	overrides, ok := versionOverrides[major]
	if !ok {
		return nil // no overrides needed for this version
	}

	data, err := os.ReadFile(reqFile)
	if err != nil {
		return err
	}
	original := string(data)
	patched := original
	applied := []string{}

	for old, replacement := range overrides {
		if strings.Contains(patched, old) {
			patched = strings.ReplaceAll(patched, old, replacement)
			applied = append(applied, old+" -> "+replacement)
		}
	}

	if len(applied) == 0 {
		return nil // nothing to patch
	}

	// Log what was patched.
	for _, a := range applied {
		stdout("patched " + a)
	}

	// Backup original and write patched version.
	backup := reqFile + ".orig"
	if err := os.WriteFile(backup, []byte(original), 0o644); err != nil {
		return fmt.Errorf("backup requirements: %w", err)
	}
	return os.WriteFile(reqFile, []byte(patched), 0o644)
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
	return ResolvePython(version)
}

// ResolvePython finds the best available Python binary for an Odoo version.
// It scans PATH and common system directories (deadsnakes, pyenv, etc.).
func ResolvePython(version string) string {
	major := strings.Split(version, ".")[0]
	switch major {
	case "19":
		if py := findPython("python3.13", "python3.12", "python3.11"); py != "" {
			return py
		}
	case "18":
		if py := findPython("python3.13", "python3.12", "python3.11", "python3.10"); py != "" {
			return py
		}
	case "16":
		if py := findPython("python3.11", "python3.10", "python3.9", "python3.8"); py != "" {
			return py
		}
	default:
		if py := findPython("python3.12", "python3.11", "python3.10"); py != "" {
			return py
		}
	}
	return "python3"
}

// findPython searches for a python binary by trying PATH lookup first,
// then scanning common system directories. This handles GUI environments
// where PATH may be restricted.
func findPython(candidates ...string) string {
	for _, name := range candidates {
		// Try PATH first.
		if _, err := exec.LookPath(name); err == nil {
			return name
		}
		// Scan common system directories (deadsnakes, pyenv, system).
		for _, dir := range []string{"/usr/bin", "/usr/local/bin", os.ExpandEnv("$HOME/.pyenv/versions"), "/opt"} {
			path := dir + "/" + name
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
	}
	return ""
}

// CheckPythonCompat verifies that the given python binary is compatible
// with the target Odoo major version. Returns a detailed, actionable error
// message when the Python version is too new or too old.
func CheckPythonCompat(pythonBin string, odooVersion string) error {
	major := strings.Split(odooVersion, ".")[0]
	allowed, ok := checker.PythonCompat[major]
	if !ok {
		return nil // unknown version — skip check, let Odoo decide
	}

	// Get the Python version string (e.g. "Python 3.12.3").
	out, err := exec.Command(pythonBin, "--version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("cannot detect version of %s: %w", pythonBin, err)
	}
	pyVer := strings.TrimSpace(string(out)) // "Python 3.12.3"
	parts := strings.Fields(pyVer)
	if len(parts) < 2 {
		return nil
	}
	pyVersion := parts[1] // "3.12.3"
	pyMinor := strings.Join(strings.Split(pyVersion, ".")[:2], ".") // "3.12"

	for _, a := range allowed {
		if pyMinor == a || strings.HasPrefix(pyMinor, a+".") {
			return nil // compatible
		}
	}

	// Check which Pythons are actually installed (to suggest the right one).
	installed := detectInstalledPythons()
	var availableCompatible []string
	for _, a := range allowed {
		candidate := "python" + a
		for _, ip := range installed {
			if strings.Contains(ip, a) {
				availableCompatible = append(availableCompatible, candidate)
				break
			}
		}
	}

	suggestedPkg := "python" + allowed[len(allowed)-1]

	// Build the install command section.
	installSection := ""
	if len(availableCompatible) > 0 {
		// A compatible Python is installed but not on PATH — suggest activating or symlinking.
		installSection = fmt.Sprintf(
			"  A compatible Python was found on your system but is not on PATH.\n"+
				"  Try running:\n"+
				"    export PATH=\"/usr/bin:$PATH\"\n"+
				"  or pass it explicitly with:\n"+
				"    odoonoir create <name> -v %s --python %s\n",
			odooVersion, availableCompatible[0],
		)
	} else {
		// No compatible Python found — provide full install instructions.
		installSection = fmt.Sprintf(
			"  Option 1 — deadsnakes PPA (recommended for Ubuntu 22.04+):\n"+
				"    sudo add-apt-repository ppa:deadsnakes/ppa\n"+
				"    sudo apt update\n"+
				"    sudo apt install %s %s-venv %s-dev\n\n"+
				"  Option 2 — direct install (if package is available):\n"+
				"    sudo apt install %s %s-venv %s-dev\n\n"+
				"  Option 3 — use pyenv:\n"+
				"    curl https://pyenv.run | bash\n"+
				"    pyenv install %s\n"+
				"    pyenv local %s\n",
			suggestedPkg, suggestedPkg, suggestedPkg,
			suggestedPkg, suggestedPkg, suggestedPkg,
			allowed[len(allowed)-1],
			allowed[len(allowed)-1],
		)
	}

	return fmt.Errorf(
		"Incompatible Python version for Odoo %s.\n\n"+
			"  Found:    %s (Python %s)\n"+
			"  Required: Python %s\n\n"+
			"To fix this, install a compatible Python version:\n\n"+
			"%s"+
			"After installing, try creating the instance again.",
		major,
		pythonBin, pyVersion,
		strings.Join(allowed, ", "),
		installSection,
	)
}

// detectInstalledPythons scans common paths for python3.x binaries.
func detectInstalledPythons() []string {
	var found []string
	for _, v := range []string{"3.13", "3.12", "3.11", "3.10", "3.9", "3.8"} {
		candidate := "python" + v
		if _, err := exec.LookPath(candidate); err == nil {
			found = append(found, candidate)
			continue
		}
		// Check absolute paths for deadsnakes installs.
		for _, prefix := range []string{"/usr/bin", "/usr/local/bin"} {
			path := prefix + "/" + candidate
			if _, err := os.Stat(path); err == nil {
				found = append(found, path)
				break
			}
		}
	}
	return found
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
