package checker

import (
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"strings"
)

// Severity of a check result.
type Severity string

const (
	OK      Severity = "ok"
	WARN    Severity = "warning"
	ERROR   Severity = "error"
	MISSING Severity = "missing"
)

// Result is a single system check outcome.
type Result struct {
	Check    string   `json:"check"`
	Severity Severity `json:"severity"`
	Found    string   `json:"found,omitempty"`
	Required string   `json:"required,omitempty"`
	Hint     string   `json:"hint,omitempty"`
}

// PythonCompat maps Odoo major versions to acceptable Python major.minor prefixes.
var PythonCompat = map[string][]string{
	"15": {"3.8", "3.9", "3.10"},
	"16": {"3.8", "3.9", "3.10", "3.11"},
	"17": {"3.10", "3.11", "3.12"},
	"18": {"3.10", "3.11", "3.12", "3.13"},
	"19": {"3.11", "3.12", "3.13"},
}

// AptPackages lists the essential system libraries for building Odoo.
var AptPackages = []string{
	"build-essential", "python3-dev", "python3-venv",
	"libpq-dev", "libxml2-dev", "libxslt1-dev",
	"libldap2-dev", "libsasl2-dev", "libssl-dev",
	"libjpeg-dev", "libffi-dev", "zlib1g-dev", "liblcms2-dev",
}

// aptAliases maps old package names to new equivalents for dpkg check.
var aptAliases = map[string][]string{
	"libz-dev":   {"zlib1g-dev", "libz-dev"},
	"zlib1g-dev": {"zlib1g-dev", "libz-dev"},
}

// Check describes a fully resolved system audit. Results is populated by Run.
type Check struct {
	Results []Result
}

// Run audits the whole system for Odoo readiness.
func Run() *Check {
	c := &Check{}
	c.add(checkBin("python3", "Python 3.8+"))
	c.add(checkBin("python3.12", "for Odoo 17/18 venvs"))
	c.add(checkBin("python3.10", "for Odoo 16 venvs"))
	c.add(checkBin("git", "source checkout"))
	c.add(checkBin("node", ">= 20 (Odoo 18+ asset build)"))
	c.add(checkBin("npm", ">= 10 (Odoo 18+ asset build)"))
	c.add(checkBin("psql", "database client"))
	c.add(checkBin("createdb", "database client"))
	c.add(checkBin("pg_dump", "database client"))
	c.add(checkPip())
	c.add(checkVenvModule())
	c.add(checkPostgresRunning())
	c.add(checkSystemPackages())
	return c
}

// ForVersion audits plus enforces Python compatibility with an Odoo version.
// Any installed python3.x binary counts, not just the default python3.
func (c *Check) ForVersion(odooVersion string) {
	major := strings.Split(odooVersion, ".")[0]
	allowed, ok := PythonCompat[major]
	if !ok {
		c.add(Result{Check: "odoo-version", Severity: ERROR,
			Required: "16, 17, 18 or 19", Hint: "unsupported Odoo version: " + odooVersion})
		return
	}
	candidates := candidatePythonVersions()
	if len(candidates) == 0 {
		return // python3 missing already reported
	}
	compatible := false
	var found string
	for _, v := range candidates {
		if compatibleVersion(v, allowed) {
			compatible = true
			found = v
			break
		}
	}
	if compatible {
		c.add(Result{Check: "python-compat", Severity: OK, Found: found,
			Required: "Odoo " + major + " needs Python " + strings.Join(allowed, ", ")})
	} else {
		c.add(Result{Check: "python-compat", Severity: ERROR, Found: strings.Join(candidates, ", "),
			Required: "Odoo " + major + " needs Python " + strings.Join(allowed, ", "),
			Hint:     "install a matching python3 with: sudo apt install python3.x python3.x-venv"})
	}
}

// candidatePythonVersions collects the versions of every installed python3 binary.
func candidatePythonVersions() []string {
	var out []string
	re := regexp.MustCompile(`Python (\d+\.\d+\.\d+)`)
	for _, bin := range []string{"python3", "python3.13", "python3.12", "python3.11", "python3.10", "python3.9"} {
		raw, err := exec.Command(bin, "--version").CombinedOutput()
		if err != nil {
			continue
		}
		if m := re.FindStringSubmatch(string(raw)); len(m) > 1 {
			out = append(out, m[1])
		}
	}
	return out
}

func compatibleVersion(version string, allowed []string) bool {
	for _, p := range allowed {
		if version == p || strings.HasPrefix(version, p+".") {
			return true
		}
	}
	return false
}

// PortInUse reports whether a TCP port is already bound on localhost.
func PortInUse(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return true
	}
	_ = ln.Close()
	return false
}

// Add appends a raw result (used by callers for ad-hoc checks).
func (c *Check) Add(r Result) { c.add(r) }

func (c *Check) add(r Result) { c.Results = append(c.Results, r) }

func checkBin(name, want string) Result {
	if _, err := exec.LookPath(name); err != nil {
		return Result{Check: "bin-" + name, Severity: MISSING, Required: name,
			Hint: "install with: sudo apt install " + name}
	}
	return Result{Check: "bin-" + name, Severity: OK, Found: name, Required: want}
}

func checkPip() Result {
	out, err := exec.Command("python3", "-m", "pip", "--version").CombinedOutput()
	if err != nil {
		return Result{Check: "pip", Severity: MISSING, Required: "python3 -m pip",
			Hint: "install with: sudo apt install python3-pip"}
	}
	ver := strings.Fields(string(out))
	if len(ver) > 1 {
		return Result{Check: "pip", Severity: OK, Found: "pip " + ver[1]}
	}
	return Result{Check: "pip", Severity: OK, Found: strings.TrimSpace(string(out))}
}

func checkVenvModule() Result {
	out, err := exec.Command("python3", "-c", "import venv; print('venv ok')").CombinedOutput()
	if err != nil {
		return Result{Check: "python-venv", Severity: MISSING, Required: "python3-venv module",
			Hint: "install with: sudo apt install python3-venv"}
	}
	return Result{Check: "python-venv", Severity: OK, Found: strings.TrimSpace(string(out))}
}

func checkPostgresRunning() Result {
	out, err := exec.Command("pg_isready").CombinedOutput()
	if err != nil {
		return Result{Check: "postgres-running", Severity: ERROR,
			Required: "PostgreSQL server accepting connections",
			Hint:     "start it with: sudo systemctl start postgresql   (or: sudo service postgresql start)"}
	}
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		msg = "accepting connections"
	}
	return Result{Check: "postgres-running", Severity: OK, Found: msg}
}

func checkSystemPackages() Result {
	if _, err := exec.LookPath("dpkg"); err != nil {
		return Result{Check: "apt-packages", Severity: WARN, Found: "not a dpkg system",
			Required: "build libraries for Odoo",
			Hint:     "install build essentials with your package manager (e.g. dnf group install 'Development Tools', zlib-devel, libjpeg-turbo-devel, ...)"}
	}
	missing := []string{}
	for _, pkg := range AptPackages {
		found := false
		aliases := aptAliases[pkg]
		if len(aliases) == 0 {
			aliases = []string{pkg}
		}
		for _, a := range aliases {
			if ok, _ := dpkgInstalled(a); ok {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, pkg)
		}
	}
	if len(missing) > 0 {
		return Result{Check: "apt-packages", Severity: MISSING,
			Required: strings.Join(AptPackages, ", "),
			Hint:     "install with: sudo apt install " + strings.Join(missing, " ")}
	}
	return Result{Check: "apt-packages", Severity: OK, Found: strings.Join(AptPackages, ", ")}
}

func dpkgInstalled(pkg string) (bool, error) {
	return exec.Command("dpkg", "-s", pkg).Run() == nil, nil
}
