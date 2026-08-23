// Package adopt imports an already-installed Odoo instance into odoonoir.
// Detection is strictly read-only: no file outside the odoonoir metadata
// directory is ever created, moved or rewritten.
package adopt

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ahmed/odoonoir/internal/checker"
	"github.com/ahmed/odoonoir/internal/config"
	"github.com/ahmed/odoonoir/internal/db"
	"github.com/ahmed/odoonoir/internal/instance"
	"github.com/ahmed/odoonoir/internal/odoconf"
	"github.com/ahmed/odoonoir/internal/proc"
)

// Options are the user-supplied inputs for adopting an existing instance.
// Empty fields are auto-detected; anything still unknown after detection
// must be resolved by the caller (the interactive wizard) before Commit.
type Options struct {
	Name        string
	Source      string // dir that contains (or nests) odoo-bin
	Conf        string // existing odoo.conf
	Venv        string // python venv dir
	Python      string // interpreter override (venv wins over this)
	Addons      []string
	DBName      string
	DBUser      string
	Port        int
	Version     string
	Description string
	Root        string // odoonoir metadata root (default: config instances dir)
}

// Result carries the detected facts and any warnings.
type Result struct {
	Inst    *instance.Instance
	Warn    []string
	Running int // pid of a live odoo process using the conf (0 = not running)
	DBs     []DBInfo
}

// DBInfo describes a database discovered on the existing installation.
type DBInfo struct {
	Name    string
	Primary bool
	Size    int64
	Init    bool
}

var nameRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// ValidName reports whether s is a valid instance name.
func ValidName(s string) bool {
	return nameRe.MatchString(s)
}

// Detect locates and inspects an existing Odoo installation without
// modifying anything. It returns an error when a required fact cannot be
// determined; the caller decides whether to prompt the user.
func Detect(cfg *config.Config, reg *instance.Registry, o Options) (*Result, error) {
	if !nameRe.MatchString(o.Name) {
		return nil, fmt.Errorf("invalid instance name %q (use lowercase letters, digits and underscores)", o.Name)
	}
	res := &Result{Inst: &instance.Instance{Name: o.Name}}
	inst := res.Inst
	var conf *odoconf.OdooConf

	// 1. Locate the source directory (dir containing odoo-bin).
	source, err := detectSource(o.Source, o.Conf)
	if err != nil {
		return nil, err
	}
	inst.SourcePath = source

	// 2. Locate the conf.
	confPath := o.Conf
	if confPath == "" {
		confPath = detectConf(source)
	}
	if confPath != "" {
		c, err := odoconf.Load(confPath)
		if err != nil {
			return nil, fmt.Errorf("conf %s: %w", confPath, err)
		}
		conf = c
		inst.ConfPath = confPath
	}

	// 3. Version from release.py, then git branch.
	inst.Version = o.Version
	if inst.Version == "" {
		inst.Version = versionFromSource(source)
	}
	if inst.Version == "" && conf != nil {
		inst.Version = versionFromConf(conf)
	}
	inst.Branch = branchOf(source)
	if inst.SourceURL == "" {
		if url := gitRemote(source); url != "" {
			inst.SourceURL = url
		}
	}

	// 4. Running process facts (interpreter + port), conf-backed.
	pid, py, runningPort := proc.ScanOdoo(confPath)
	res.Running = pid

	// 5. Venv / python.
	venv := o.Venv
	if venv == "" {
		venv = detectVenv(source)
	}
	if venv != "" {
		inst.VenvPath = venv
		inst.PythonBin = filepath.Join(venv, "bin", "python")
	} else if o.Python != "" {
		inst.PythonBin = o.Python
	} else if py != "" {
		inst.PythonBin = py
	}

	// 6. Port: flag > conf > running process.
	port := o.Port
	if port == 0 && conf != nil {
		port = intFromConf(conf, "http_port")
	}
	if port == 0 {
		port = runningPort
	}
	inst.Port = port
	if conf != nil {
		inst.LongpollPort = intFromConf(conf, "longpolling_port")
	}

	// 7. Database.
	inst.DBName = o.DBName
	if inst.DBName == "" && conf != nil {
		if v, ok := conf.Get("db_name"); ok && v != "" && v != "False" {
			inst.DBName = v
		}
	}
	inst.DBUser = o.DBUser
	if inst.DBUser == "" {
		if conf != nil {
			if v, ok := conf.Get("db_user"); ok && v != "" {
				inst.DBUser = v
			}
		}
	}
	if inst.DBUser == "" {
		inst.DBUser = "odoo"
	}

	// 8. Addons: flag > conf addons_path > source probe.
	inst.AddonsPaths = o.Addons
	if len(inst.AddonsPaths) == 0 && conf != nil {
		if v, ok := conf.Get("addons_path"); ok && v != "" {
			inst.AddonsPaths = splitList(v)
		}
	}
	if len(inst.AddonsPaths) == 0 {
		inst.AddonsPaths = probeSourceAddons(source)
	}

	// 9. Conf-owned paths: data_dir, logfile, pidfile.
	if conf != nil {
		if v, ok := conf.Get("data_dir"); ok && v != "" {
			inst.DataDirPath = v
		}
		if v, ok := conf.Get("logfile"); ok && v != "" {
			inst.LogPath = v
		}
		if v, ok := conf.Get("pidfile"); ok && v != "" {
			inst.PIDPath = v
		}
		inst.Workers = intFromConf(conf, "workers")
		if inst.Workers == 0 {
			if _, ok := conf.Get("workers"); ok {
				inst.Workers = 0 // explicitly configured: keep
			} else {
				inst.Workers = 2
			}
		}
		if v, ok := conf.Get("log_level"); ok && v != "" {
			inst.LogLevel = v
		}
	}

	// 10. Metadata root.
	inst.Root = o.Root
	if inst.Root == "" {
		inst.Root = cfg.InstancesDir()
	}
	inst.Description = o.Description
	inst.Adopted = true

	// 11. Database discovery: every database owned by the instance role
	// that no other registered instance claims.
	discoverDatabases(cfg, reg, res)

	// 12. Warnings.
	if inst.Version == "" {
		res.Warn = append(res.Warn, "version could not be detected (no release.py / git branch) — set it with --version")
	}
	if inst.Port == 0 {
		res.Warn = append(res.Warn, "port could not be detected — set it with --port")
	}
	if inst.DBName == "" {
		res.Warn = append(res.Warn, "database could not be detected — set it with --db")
	}
	if inst.PythonBin == "" {
		res.Warn = append(res.Warn, "no python found (no venv, --python, or running process) — start will not work until set")
	}
	if isGitRepo(source) == "" {
		res.Warn = append(res.Warn, "source is not a git repo — the update command will fail at the pull step")
	}
	if conf == nil {
		res.Warn = append(res.Warn, "no odoo.conf found — start will run with Odoo defaults")
	}
	if len(inst.AddonsPaths) == 0 {
		res.Warn = append(res.Warn, "no addons paths found — module commands will not see any modules")
	}
	return res, nil
}

// discoverDatabases finds every database owned by the instance role and
// records it on the result. Databases already claimed by another registered
// instance are skipped; uninitialized databases are reported but still
// tracked. A dead postgres server degrades to a warning, not an error.
func discoverDatabases(cfg *config.Config, reg *instance.Registry, res *Result) {
	inst := res.Inst
	pg := db.New(cfg)
	if err := pg.ServerRunning(); err != nil {
		res.Warn = append(res.Warn, "postgres not reachable — databases could not be discovered")
		return
	}
	claimed := map[string]bool{}
	if all, err := reg.All(); err == nil {
		for _, o := range all {
			if o.Name == inst.Name {
				continue
			}
			for _, d := range o.AllDBs() {
				claimed[d] = true
			}
		}
	}
	owned, err := pg.DatabasesForRole(inst.DBUser)
	if err != nil {
		res.Warn = append(res.Warn, "could not list databases for role "+inst.DBUser+": "+err.Error())
		return
	}
	inst.Databases = nil
	res.DBs = nil
	if inst.DBName != "" {
		primary := DBInfo{Name: inst.DBName, Primary: true}
		if size, err := pg.DatabaseSize(inst.DBName); err == nil {
			primary.Size = size
		}
		if ok, err := pg.IsInitialized(inst.DBName); err == nil {
			primary.Init = ok
		}
		res.DBs = append(res.DBs, primary)
		if !primary.Init {
			res.Warn = append(res.Warn, fmt.Sprintf("database %s is not initialized (no Odoo tables) — initialize with: odoonoir init %s %s", inst.DBName, inst.Name, inst.DBName))
		}
	}
	for _, name := range owned {
		if name == inst.DBName {
			continue
		}
		if claimed[name] {
			continue
		}
		info := DBInfo{Name: name}
		if size, err := pg.DatabaseSize(name); err == nil {
			info.Size = size
		}
		if ok, err := pg.IsInitialized(name); err == nil {
			info.Init = ok
		}
		res.DBs = append(res.DBs, info)
		inst.AddDB(name)
		if !info.Init {
			res.Warn = append(res.Warn, fmt.Sprintf("database %s is not initialized (no Odoo tables) — initialize with: odoonoir init %s %s", name, inst.Name, name))
		}
	}
}

// detectSource finds the dir containing odoo-bin, starting from an explicit
// source dir or the conf path.
func detectSource(source, confPath string) (string, error) {
	candidates := []string{}
	if source != "" {
		abs, err := filepath.Abs(source)
		if err != nil {
			return "", err
		}
		candidates = append(candidates, abs, filepath.Join(abs, "odoo"))
	}
	if confPath != "" {
		dir := filepath.Dir(confPath)
		candidates = append(candidates,
			dir,
			filepath.Join(dir, ".."),
			filepath.Join(dir, "..", ".."),
		)
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		for _, bin := range []string{filepath.Join(c, "odoo-bin"), filepath.Join(c, "odoo", "odoo-bin"), filepath.Join(c, "src", "odoo", "odoo-bin")} {
			if fi, err := os.Stat(bin); err == nil && !fi.IsDir() {
				return c, nil
			}
		}
	}
	return "", fmt.Errorf("could not locate the Odoo source (no odoo-bin) — pass --source <dir>")
}

// detectConf looks for an odoo.conf near the source dir.
func detectConf(source string) string {
	for _, c := range []string{
		filepath.Join(source, "..", "etc", "odoo.conf"),
		filepath.Join(source, "..", "conf", "odoo.conf"),
		filepath.Join(source, "..", "odoo.conf"),
		filepath.Join(source, "odoo.conf"),
	} {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	return ""
}

// detectVenv looks for a python venv near the source dir.
func detectVenv(source string) string {
	for _, c := range []string{
		filepath.Join(source, "..", "venv"),
		filepath.Join(source, "venv"),
		filepath.Join(source, "..", ".venv"),
		filepath.Join(source, ".venv"),
	} {
		if fi, err := os.Stat(filepath.Join(c, "bin", "python")); err == nil && !fi.IsDir() {
			return c
		}
	}
	return ""
}

var releaseVersionRe = regexp.MustCompile(`(?m)info\s*\[[^\]]*['"]version['"]\s*\]\s*=\s*['"]([^'"]+)`)

// versionFromSource reads the Odoo version from release.py.
func versionFromSource(source string) string {
	for _, rp := range []string{
		filepath.Join(source, "odoo", "release.py"),
		filepath.Join(source, "release.py"),
	} {
		data, err := os.ReadFile(rp)
		if err != nil {
			continue
		}
		if m := releaseVersionRe.FindSubmatch(data); m != nil {
			return string(m[1])
		}
	}
	return ""
}

func versionFromConf(conf *odoconf.OdooConf) string {
	// some setups pin the version in the conf (e.g. Odoo's enterprise image)
	if v, ok := conf.Get("server_version"); ok && v != "" {
		return v
	}
	return ""
}

// branchOf returns the checked-out git branch of the source dir.
func branchOf(source string) string {
	out, err := exec.Command("git", "-C", source, "symbolic-ref", "--short", "HEAD").Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	data, err := os.ReadFile(filepath.Join(source, ".git", "HEAD"))
	if err == nil {
		head := strings.TrimSpace(string(data))
		if strings.HasPrefix(head, "ref: refs/heads/") {
			return strings.TrimPrefix(head, "ref: refs/heads/")
		}
		return head
	}
	return ""
}

func isGitRepo(source string) string {
	out, _ := exec.Command("git", "-C", source, "rev-parse", "--is-inside-work-tree").Output()
	return strings.TrimSpace(string(out))
}

func gitRemote(source string) string {
	out, err := exec.Command("git", "-C", source, "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func intFromConf(conf *odoconf.OdooConf, key string) int {
	v, ok := conf.Get(key)
	if !ok {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0
	}
	return n
}

func splitList(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func probeSourceAddons(source string) []string {
	for _, p := range []string{
		filepath.Join(source, "addons"),
		filepath.Join(source, "odoo", "addons"),
	} {
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			return []string{p}
		}
	}
	return nil
}

// Issue is a single audit finding for an adopted instance.
type Issue struct {
	Check string
	OK    bool
	Hint  string
}

// Audit verifies that an adopted instance can actually run, without
// changing anything. Used by `odoonoir adopt --check`.
func Audit(cfg *config.Config, inst *instance.Instance, paths instance.Paths) []Issue {
	var issues []Issue
	add := func(check string, ok bool, hint string) {
		issues = append(issues, Issue{Check: check, OK: ok, Hint: hint})
	}

	bin := filepath.Join(paths.Source, "odoo-bin")
	if _, err := os.Stat(bin); err != nil {
		add("source odoo-bin", false, "not found at "+bin+" — pass --source")
	} else if fi, err := os.Stat(bin); err == nil && fi.Mode()&0o111 == 0 {
		add("source odoo-bin", false, "not executable at "+bin)
	} else {
		add("source odoo-bin", true, "")
	}

	if inst.PythonBin == "" {
		add("python interpreter", false, "no venv/--python resolved — start will fail")
	} else {
		if _, err := os.Stat(inst.PythonBin); err != nil {
			add("python interpreter", false, inst.PythonBin+" does not exist")
		} else if _, err := exec.Command(inst.PythonBin, "-c", "import odoo; print(odoo.release.version_info[0])").Output(); err != nil {
			add("python imports odoo", false, inst.PythonBin+" cannot import odoo (wrong venv?) — "+
				strings.TrimSpace(err.Error()))
		} else {
			add("python imports odoo", true, "")
		}
	}

	if inst.ConfPath == "" {
		add("odoo.conf", false, "no conf resolved — Odoo defaults will be used")
	} else if _, err := odoconf.Load(inst.ConfPath); err != nil {
		add("odoo.conf", false, inst.ConfPath+": "+err.Error())
	} else {
		add("odoo.conf", true, "")
	}

	pg := db.New(cfg)
	if err := pg.ServerRunning(); err != nil {
		add("postgres", false, err.Error())
	} else {
		add("postgres", true, "")
		exists := true
		for _, d := range inst.AllDBs() {
			ok, err := pg.DatabaseExists(d)
			if err != nil || !ok {
				exists = false
				break
			}
		}
		if !exists {
			add("databases", false, "one or more registered databases are missing from the server")
		} else {
			add("databases", true, "")
		}
	}

	if inst.Port > 0 {
		if checker.PortInUse(inst.Port) {
			add("http port "+fmt.Sprint(inst.Port), true, "in use — assumed to be this instance")
		} else {
			add("http port "+fmt.Sprint(inst.Port), true, "free — ready to start")
		}
	}
	return issues
}

// Candidate describes one discovered Odoo installation for `adopt --scan`.
type Candidate struct {
	Source string // dir containing odoo-bin ("" when only a conf was found)
	Conf   string // odoo.conf path ("" when only a source was found)
	Port   int
	Python string
	Hint   string
}

// ScanCandidates probes running processes and common install locations for
// existing Odoo installations. When scanRoot is non-empty, the search is
// limited to that directory (walked recursively) instead.
func ScanCandidates(scanRoot string) []Candidate {
	if scanRoot != "" {
		return scanRootCandidates(scanRoot)
	}
	seen := map[string]bool{}
	var out []Candidate
	add := func(c Candidate) {
		key := c.Source + "|" + c.Conf
		if key == "|" || seen[key] {
			return
		}
		seen[key] = true
		out = append(out, c)
	}

	// 1. live odoo processes
	for _, p := range proc.ScanAllOdoo("") {
		c := Candidate{Conf: p.Conf, Port: p.Port, Python: p.Python,
			Hint: fmt.Sprintf("running (pid %d)", p.PID)}
		if p.Conf != "" {
			if src, err := detectSource("", p.Conf); err == nil {
				c.Source = src
			}
		}
		add(c)
	}

	// 2. common install roots
	roots := []string{"/srv", "/opt", "/usr/lib/odoo", "/usr/share/odoo"}
	if home := os.Getenv("HOME"); home != "" {
		roots = append(roots, home)
	}
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			dir := filepath.Join(root, e.Name())
			if src, err := detectSource(dir, ""); err == nil {
				add(Candidate{Source: src, Conf: detectConf(src),
					Hint: "found at " + dir})
				continue
			}
			if conf := detectConf(dir); conf != "" {
				add(Candidate{Conf: conf, Hint: "found at " + dir})
			}
		}
	}

	// 3. /etc/odoo
	if _, err := os.Stat("/etc/odoo/odoo.conf"); err == nil {
		add(Candidate{Conf: "/etc/odoo/odoo.conf", Hint: "system install (deb)"})
	}
	return out
}

// scanSkipDirs are directory names never descended into during a scan walk
// (virtualenvs, build artifacts and VCS internals are huge and never hold
// an install root).
var scanSkipDirs = map[string]bool{
	"venv": true, ".venv": true, "node_modules": true, "__pycache__": true,
	".git": true, ".hg": true, "build": true, "dist": true,
}

// scanRootCandidates walks root recursively (bounded depth) looking for
// directories that contain odoo-bin and for odoo.conf files.
func scanRootCandidates(root string) []Candidate {
	seen := map[string]bool{}
	var out []Candidate
	add := func(c Candidate) {
		key := c.Source + "|" + c.Conf
		if key == "|" || seen[key] {
			return
		}
		seen[key] = true
		out = append(out, c)
	}

	const maxDepth = 5
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}
	_ = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			if d.Name() == "odoo.conf" {
				if src, serr := detectSource("", path); serr == nil {
					add(Candidate{Source: src, Conf: path, Hint: "found at " + filepath.Dir(path)})
				} else {
					add(Candidate{Conf: path, Hint: "conf at " + path})
				}
			}
			return nil
		}
		depth := strings.Count(strings.TrimPrefix(path, absRoot), string(filepath.Separator))
		if depth >= maxDepth {
			return filepath.SkipDir
		}
		if path != absRoot && scanSkipDirs[d.Name()] {
			return filepath.SkipDir
		}
		if src, serr := detectSource(path, ""); serr == nil {
			conf := detectConf(src)
			if conf == "" {
				// look for a conf one level down, e.g. <src>/etc/odoo.conf
				for _, c := range []string{filepath.Join(path, "etc", "odoo.conf"), filepath.Join(path, "conf", "odoo.conf")} {
					if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
						conf = c
						break
					}
				}
			}
			add(Candidate{Source: src, Conf: conf, Hint: "found at " + path})
			return filepath.SkipDir
		}
		return nil
	})
	return out
}
