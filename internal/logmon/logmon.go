package logmon

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

// Issue is a detected problem in the instance log.
type Issue struct {
	Severity string // "error" | "warning"
	Pattern  string
	Message  string
	Line     string
	LineNo   int
	Hint     string
}

// knownPatterns maps regex -> human explanation + fix hint.
var knownPatterns = []struct {
	re      *regexp.Regexp
	sev     string
	message string
	hint    string
}{
	{regexp.MustCompile(`Address already in use`), "error",
		"HTTP port is already in use by another process",
		"stop the other process or change http_port in the conf: odoonoir config set <name> http_port 8070"},
	{regexp.MustCompile(`could not connect to server`), "error",
		"Odoo cannot reach PostgreSQL",
		"check postgres is running and db_host/db_port in conf are correct"},
	{regexp.MustCompile(`FATAL:  (password authentication failed|role ".*" does not exist)`), "error",
		"PostgreSQL authentication/role problem",
		"check db_user and db_password in conf; the role must exist: odoonoir create (or odoonoir init)"},
	{regexp.MustCompile(`Module \` + "`" + `([a-z0-9_]+)` + "`" + ` was not found`), "error",
		"A required module is missing from addons_path",
		"check the module name and that addons_path includes the folder containing it"},
	{regexp.MustCompile(`No module named 'odoo\.addons\.web'|Failed to load server-wide module .web.`), "error",
		"The web addon is missing from addons_path",
		"Odoo 18.0+ keeps addons in a top-level addons/ dir — run: odoonoir update <name>"},
	{regexp.MustCompile(`MissingSectionHeaderError.*File contains no section headers`), "error",
		"odoo.conf is missing the [options] section header",
		"rewrite the conf with: odoonoir config set <name> http_port <port> (or re-run create)"},
	{regexp.MustCompile(`Database .* not initialized, you can force it with`), "error",
		"Instance database has no Odoo tables yet",
		"initialize it with: odoonoir update -i base <name>"},
	{regexp.MustCompile(`No module named '?([a-z0-9_]+)'?`), "error",
		"A python dependency is missing",
		"install it in the instance venv: <instance-root>/<name>/venv/bin/pip install <module>"},
	{regexp.MustCompile(`Module .* requires .* which depends on`), "error",
		"Module dependency chain is broken",
		"install the missing dependency module or remove it from addons_path"},
	{regexp.MustCompile(`PermissionError|Permission denied`), "error",
		"Filesystem permission problem",
		"check ownership of the instance data dir and log file"},
	{regexp.MustCompile(`Filestore location .* does not exist`), "error",
		"Filestore data_dir is missing",
		"create the data dir: odoonoir config set <name> data_dir <path>"},
	{regexp.MustCompile(`ERROR|CRITICAL`), "warning",
		"General ERROR/CRITICAL log entries detected",
		"review the log lines above for the specific cause"},
	{regexp.MustCompile(`WARNING .*(deprecated|Deprecated|psycopg2|Imported module|DateTimeFormat)`), "warning",
		"Deprecation or compatibility warnings",
		"usually harmless; verify on next minor upgrade"},
}

// Scan inspects the log file and returns detected issues with hints.
func Scan(logPath string) ([]Issue, error) {
	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var issues []Issue
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := sc.Text()
		matched := false
		for _, p := range knownPatterns {
			if !p.re.MatchString(line) {
				continue
			}
			if matched {
				continue // a more specific error already explains this line
			}
			issues = append(issues, Issue{
				Severity: p.sev, Pattern: p.re.String(),
				Message: p.message, Line: line, LineNo: lineNo, Hint: p.hint,
			})
			if p.sev == "error" {
				matched = true
			}
		}
	}
	return issues, sc.Err()
}

// Dedupe collapses issues sharing the same message within a window.
func Dedupe(issues []Issue) []Issue {
	seen := map[string]int{}
	out := make([]Issue, 0, len(issues))
	for _, it := range issues {
		key := it.Severity + "|" + it.Message + "|" + it.Hint
		if prev, ok := seen[key]; ok {
			if it.LineNo-prev > 500 { // spread out
				seen[key] = it.LineNo
				out = append(out, it)
			}
			continue
		}
		seen[key] = it.LineNo
		out = append(out, it)
	}
	return out
}

// Tail reads the last n lines of the log file.
func Tail(logPath string, n int) ([]string, error) {
	f, err := os.Open(logPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	var lines []string
	for sc.Scan() {
		lines = append(lines, sc.Text())
		if len(lines) > n {
			lines = lines[1:]
		}
	}
	return lines, sc.Err()
}

// TailFrom reads new lines appended to a file since lastOffset (seek-based,
// so huge logs stay cheap). The first call returns the last n lines. On
// rotation (file shrunk) it restarts from the current end and returns the
// last n lines of the new file. After a call lastOffset tracks the file end.
// The returned bool is true when rotation was detected.
func TailFrom(path string, n int, lastOffset *int64) ([]string, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, false, err
	}
	if *lastOffset == 0 {
		if fi.Size() == 0 {
			*lastOffset = 0
			return nil, false, nil
		}
		// first read: seek back up to 1 MiB and keep the last n lines
		if fi.Size() > 1024*1024 {
			if _, err := f.Seek(-1024*1024, io.SeekEnd); err != nil {
				return nil, false, err
			}
		}
		data, err := io.ReadAll(f)
		if err != nil {
			return nil, false, err
		}
		*lastOffset = fi.Size()
		all := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		if len(all) > n {
			all = all[len(all)-n:]
		}
		return all, false, nil
	}
	if fi.Size() < *lastOffset {
		// log was rotated — restart from the current end
		*lastOffset = 0
		lines, _, err := TailFrom(path, n, lastOffset)
		return lines, true, err
	}
	if fi.Size() == *lastOffset {
		return nil, false, nil
	}
	if _, err := f.Seek(*lastOffset, io.SeekStart); err != nil {
		return nil, false, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, false, err
	}
	*lastOffset = fi.Size()
	line := strings.TrimRight(string(data), "\n")
	if line == "" {
		return nil, false, nil
	}
	return strings.Split(line, "\n"), false, nil
}

// ErrorsOnly filters to error-severity issues.
func ErrorsOnly(issues []Issue) []Issue {
	var out []Issue
	for _, it := range issues {
		if it.Severity == "error" {
			out = append(out, it)
		}
	}
	return out
}

// HasCritical reports whether the log shows a hard failure (DB, port, missing dep).
func HasCritical(issues []Issue) bool {
	return len(ErrorsOnly(issues)) > 0
}

// Format issues as human-readable report lines.
func Format(issues []Issue) []string {
	var out []string
	for _, it := range issues {
		prefix := "ERROR"
		if it.Severity == "warning" {
			prefix = "WARN "
		}
		out = append(out, fmt.Sprintf("%s [line %d] %s", prefix, it.LineNo, it.Message))
		if it.Hint != "" {
			out = append(out, "      fix: "+it.Hint)
		}
		out = append(out, "      log: "+strings.TrimSpace(it.Line))
	}
	return out
}
