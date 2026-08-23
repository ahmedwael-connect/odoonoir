package proc

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ahmed/odoonoir/internal/instance"
)

// Manager starts/stops/checks Odoo processes via PID files.
type Manager struct {
	pidFile string
	dbFile  string
	logFile string
	venvPy  string
	conf    string
	source  string
}

// New creates a process manager for the given instance paths.
func New(p instance.Paths, venvPython, confPath string) *Manager {
	return &Manager{
		pidFile: p.PIDFile,
		dbFile:  p.PIDFile + ".db",
		logFile: p.Log,
		venvPy:  venvPython,
		conf:    confPath,
		source:  p.Source,
	}
}

// ServingDB returns the database the running process was started with
// ("" when it was started without an explicit -d, i.e. the conf db_name).
func (m *Manager) ServingDB() string {
	if data, err := os.ReadFile(m.dbFile); err == nil {
		return strings.TrimSpace(string(data))
	}
	return ""
}

// Status reports the instance runtime state and PID.
// The PID file is consulted first (it is authoritative for instances
// started by this tool); when it is missing or stale, the process table
// is scanned for an odoo process running with this instance's conf — this
// covers adopted instances and externally-managed startups.
func (m *Manager) Status() (instance.Status, int, error) {
	pid, err := readPID(m.pidFile)
	if err == nil && pid > 0 && processAlive(pid) {
		return instance.StatusRunning, pid, nil
	}
	if err == nil && pid > 0 {
		os.Remove(m.pidFile) // stale pid file
	}
	if pid, _, _ := ScanOdoo(m.conf); pid > 0 {
		return instance.StatusRunning, pid, nil
	}
	return instance.StatusStopped, 0, nil
}

// OdooProc describes an odoo-bin process found in the process table.
type OdooProc struct {
	PID    int
	Python string
	Conf   string
	Port   int
}

// ScanOdoo scans the process table for an odoo-bin process started with
// the given conf path ("-c <conf>" or "--config <conf>"). It returns the
// process pid, the interpreter used to launch it, and the --http-port it
// was told to listen on (0 when absent). The conf path may be empty to
// match any odoo-bin process.
func ScanOdoo(confPath string) (pid int, python string, httpPort int) {
	for _, p := range ScanAllOdoo(confPath) {
		return p.PID, p.Python, p.Port
	}
	return 0, "", 0
}

// ScanAllOdoo lists every odoo-bin process (optionally restricted to one
// conf path), newest-first is not guaranteed — order follows ps output.
func ScanAllOdoo(confPath string) []OdooProc {
	out, err := exec.Command("ps", "-eo", "pid=,args=").Output()
	if err != nil {
		return nil
	}
	var procs []OdooProc
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.Contains(line, "odoo-bin") {
			continue
		}
		p, err := strconv.Atoi(fields[0])
		if err != nil || p <= 0 {
			continue
		}
		if !hasConfFlag(p, fields[1:], confPath) {
			continue
		}
		interp := fields[1]
		if strings.HasSuffix(interp, "odoo-bin") {
			interp = ""
		}
		procs = append(procs, OdooProc{PID: p, Python: interp, Port: portFromArgs(fields[1:]), Conf: confFromArgs(fields[1:])})
	}
	return procs
}

// confFromArgs extracts the -c/--config argument from a command line.
func confFromArgs(args []string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-c" || args[i] == "--config" {
			return args[i+1]
		}
	}
	return ""
}

// hasConfFlag reports whether the args reference the given conf path.
// Relative conf args are resolved against the process cwd; a bare
// basename match is intentionally not used, because every Odoo install
// calls its conf odoo.conf and that would match unrelated instances.
func hasConfFlag(pid int, args []string, confPath string) bool {
	if confPath == "" {
		return true
	}
	for i := 0; i < len(args)-1; i++ {
		if args[i] != "-c" && args[i] != "--config" {
			continue
		}
		arg := args[i+1]
		if arg == confPath {
			return true
		}
		if !filepath.IsAbs(arg) {
			if cwd, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", pid)); err == nil {
				if filepath.Join(cwd, arg) == confPath {
					return true
				}
			}
		}
	}
	return false
}

// portFromArgs extracts -p/--http-port from a command line.
func portFromArgs(args []string) int {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-p" || args[i] == "--http-port" {
			if n, err := strconv.Atoi(args[i+1]); err == nil && n > 0 && n < 65536 {
				return n
			}
		}
	}
	return 0
}

// Start launches `odoo [-d db] -c conf` with output appended to the log file.
// An empty dbName leaves the database selection to the conf's db_name.
func (m *Manager) Start(dbName string) error {
	status, pid, err := m.Status()
	if err != nil {
		return err
	}
	if status == instance.StatusRunning {
		return fmt.Errorf("instance already running (pid %d)", pid)
	}
	if err := os.MkdirAll(dirOf(m.pidFile), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(dirOf(m.logFile), 0o755); err != nil {
		return err
	}
	log, err := os.OpenFile(m.logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer log.Close()

	args := []string{filepath.Join(m.source, "odoo-bin"), "-c", m.conf}
	if dbName != "" {
		args = append(args, "-d", dbName)
	}
	cmd := exec.Command(m.venvPy, args...)
	cmd.Dir = m.source
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start odoo: %w", err)
	}
	if err := os.WriteFile(m.pidFile, []byte(strconv.Itoa(cmd.Process.Pid)+"\n"), 0o644); err != nil {
		// The conf may point the pidfile at a directory we cannot write
		// (e.g. /var/run for adopted instances): skip the pid file — the
		// process-table scan in Status keeps tracking working.
		return nil
	}
	if dbName != "" {
		_ = os.WriteFile(m.dbFile, []byte(dbName+"\n"), 0o644)
	} else {
		_ = os.Remove(m.dbFile)
	}
	return nil
}

// Stop terminates the instance (process group), escalating SIGTERM -> SIGKILL.
func (m *Manager) Stop() error {
	status, pid, err := m.Status()
	if err != nil {
		return err
	}
	if status != instance.StatusRunning {
		return nil // already stopped
	}
	// odoo spawns worker processes in the same process group (Setpgid in
	// Start); signal the whole group so nothing is left orphaned.
	if !processAlive(pid) {
		os.Remove(m.pidFile)
		os.Remove(m.dbFile)
		return nil
	}
	if err := killGroup(pid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("send SIGTERM: %w", err)
	}
	for i := 0; i < 20; i++ { // wait up to ~10s for graceful shutdown
		if !processAlive(pid) {
			os.Remove(m.pidFile)
			os.Remove(m.dbFile)
			return nil
		}
		waitMs(500)
	}
	_ = killGroup(pid, syscall.SIGKILL)
	for i := 0; i < 4; i++ { // wait up to ~2s for the group to actually die
		if !processAlive(pid) {
			os.Remove(m.pidFile)
			os.Remove(m.dbFile)
			return nil
		}
		waitMs(500)
	}
	os.Remove(m.pidFile)
	os.Remove(m.dbFile)
	return fmt.Errorf("process %d (and its workers) did not die after SIGKILL", pid)
}

// killGroup signals every process in the group led by pid (negative pid).
func killGroup(pid int, sig syscall.Signal) error {
	return syscall.Kill(-pid, sig)
}

// Restart stops then starts the instance, keeping the database it was
// started with (from the serving-db marker).
func (m *Manager) Restart() error {
	dbName := m.ServingDB()
	if err := m.Stop(); err != nil {
		return err
	}
	return m.Start(dbName)
}

func readPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("corrupt pid file %s: %w", path, err)
	}
	return pid, nil
}

// processAlive reports whether the pid is a live (non-zombie) process.
// Zombies pass kill(pid, 0) but are dead as far as an instance is concerned,
// so the process state is checked via /proc (Linux).
func processAlive(pid int) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		// non-Linux or process gone: fall back to signal 0
		proc, err := os.FindProcess(pid)
		if err != nil {
			return false
		}
		return proc.Signal(syscall.Signal(0)) == nil
	}
	// comm (field 2) may contain spaces and parens; the state is the first
	// field after the last ')'.
	if i := strings.LastIndex(string(data), ")"); i >= 0 {
		rest := strings.Fields(string(data)[i+1:])
		if len(rest) > 0 && rest[0] == "Z" {
			return false
		}
	}
	return true
}

func waitMs(ms int) {
	time.Sleep(time.Duration(ms) * time.Millisecond)
}

func dirOf(path string) string {
	idx := strings.LastIndex(path, "/")
	if idx < 0 {
		return "."
	}
	return path[:idx]
}
