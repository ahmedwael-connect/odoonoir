package db

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/ahmed/odoonoir/internal/config"
)

// PG wraps postgres client access using the odoonoir config for credentials.
type PG struct {
	cfg *config.Config
}

// New creates a PG helper bound to the global config.
func New(cfg *config.Config) *PG { return &PG{cfg: cfg} }

// pgEnv builds the environment for psql/createdb/pg_dump calls.
// Defaults (localhost:5432) are omitted so tools use the unix socket and
// peer auth; explicit hosts are passed through.
func (p *PG) pgEnv() []string {
	env := os.Environ()
	if p.cfg.PostgresUser != "" {
		env = append(env, "PGUSER="+p.cfg.PostgresUser)
	}
	if p.cfg.PostgresHost != "" && p.cfg.PostgresHost != "localhost" {
		env = append(env, "PGHOST="+p.cfg.PostgresHost)
	}
	if p.cfg.PostgresPort != 0 && p.cfg.PostgresPort != 5432 {
		env = append(env, fmt.Sprintf("PGPORT=%d", p.cfg.PostgresPort))
	}
	return env
}

// psql runs a query and returns the output.
func (p *PG) psql(dbname, query string) (string, error) {
	cmd := exec.Command("psql", "-d", dbname, "-tAc", query)
	cmd.Env = p.pgEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("psql: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// Query runs a SQL query against a database and returns the unaligned rows
// (fields separated by "|", rows by newline). Use for read-only inspection
// (module states, sizes, ...).
func (p *PG) Query(dbname, sql string) (string, error) {
	return p.psql(dbname, sql)
}

// ServerRunning checks psql responds on the configured host/port.
func (p *PG) ServerRunning() error {
	cmd := exec.Command("pg_isready", "-h", p.cfg.PostgresHost, "-p", fmt.Sprint(p.cfg.PostgresPort))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("postgres is not accepting connections at %s:%d: %s",
			p.cfg.PostgresHost, p.cfg.PostgresPort, strings.TrimSpace(string(out)))
	}
	return nil
}

// DetectAdminUser tries to find a working postgres admin role.
// Order: configured user -> odoo -> postgres -> current OS user (peer auth).
func (p *PG) DetectAdminUser() (string, error) {
	candidates := []string{}
	if p.cfg.PostgresUser != "" {
		candidates = append(candidates, p.cfg.PostgresUser)
	}
	candidates = append(candidates, "odoo", "postgres", currentUser())

	for _, u := range candidates {
		cmd := exec.Command("psql", "-d", "postgres", "-tAc", "SELECT 1")
		cmd.Env = append(os.Environ(), "PGUSER="+u)
		if cmd.Run() == nil {
			return u, nil
		}
	}
	return "", fmt.Errorf("no working postgres admin role found; tried: %s\n"+
		"create one with: sudo -u postgres createuser -s $(whoami)", strings.Join(candidates, ", "))
}

// RoleExists checks whether a role exists.
func (p *PG) RoleExists(role string) (bool, error) {
	if err := dbIdentValid(role); err != nil {
		return false, err
	}
	out, err := p.psql("postgres", "SELECT 1 FROM pg_roles WHERE rolname = '"+role+"'")
	return out == "1", err
}

// DatabaseExists checks whether a database exists.
func (p *PG) DatabaseExists(name string) (bool, error) {
	if err := dbIdentValid(name); err != nil {
		return false, err
	}
	out, err := p.psql("postgres", "SELECT 1 FROM pg_database WHERE datname = '"+name+"'")
	return out == "1", err
}

// EnsureRole creates the role if missing and returns whether it was created.
func (p *PG) EnsureRole(role string, withPassword bool) (bool, error) {
	if err := dbIdentValid(role); err != nil {
		return false, err
	}
	exists, err := p.RoleExists(role)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	var q string
	if withPassword {
		q = "CREATE ROLE " + role + " LOGIN CREATEDB PASSWORD '" + PgEscapeLiteral(role) + "'"
	} else {
		q = "CREATE ROLE " + role + " LOGIN CREATEDB"
	}
	cmd := exec.Command("psql", "-d", "postgres", "-c", q)
	cmd.Env = p.pgEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		return false, fmt.Errorf("create role %s: %w\n%s", role, err, strings.TrimSpace(string(out)))
	}
	return true, nil
}

// CreateDatabase creates a database owned by the given role (idempotent).
func (p *PG) CreateDatabase(name, owner string) error {
	exists, err := p.DatabaseExists(name)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	cmd := exec.Command("createdb", "-O", owner, name)
	cmd.Env = p.pgEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("create database %s: %w\n%s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// DropDatabase removes a database (never touches postgres/template*).
func (p *PG) DropDatabase(name string) error {
	exists, err := p.DatabaseExists(name)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if name == "postgres" || strings.HasPrefix(name, "template") {
		return fmt.Errorf("refusing to drop system database %q", name)
	}
	cmd := exec.Command("dropdb", name)
	cmd.Env = p.pgEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("drop database %s: %w\n%s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Backup dumps a database to a file (plain SQL by default, -Fc with custom=true).
// compress gzips the output (pg_dump --compress is only honored by -Fc, so we
// gzip plain dumps ourselves). Cancelling ctx kills pg_dump.
func (p *PG) Backup(ctx context.Context, name, outPath string, custom, compress bool) error {
	if err := os.MkdirAll(dirOf(outPath), 0o755); err != nil {
		return err
	}
	if compress && !custom && !strings.HasSuffix(outPath, ".gz") {
		outPath += ".gz"
	}
	args := []string{"--dbname", name}
	pipeToGzip := compress && !custom
	if custom {
		args = append(args, "--format=custom")
		if compress {
			args = append(args, "--compress=9")
		}
		args = append(args, "--file", outPath)
	} else {
		args = append(args, "--format=plain", "--no-owner")
		if !pipeToGzip {
			args = append(args, "--file", outPath)
		}
	}
	cmd := exec.CommandContext(ctx, "pg_dump", args...)
	cmd.Env = p.pgEnv()
	if !pipeToGzip {
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("pg_dump %s: %w\n%s", name, err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	// plain SQL piped through gzip
	gz := exec.Command("gzip", "-9")
	pr, pw, err := os.Pipe()
	if err != nil {
		return err
	}
	defer pr.Close()
	gz.Stdin = pr
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz.Stdout = f
	cmd.Stdout = pw
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := gz.Start(); err != nil {
		return err
	}
	pw.Close()
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("pg_dump %s: %w\n%s", name, err, strings.TrimSpace(stderr.String()))
	}
	return gz.Wait()
}

// Restore loads a dump into a database. The input is streamed (never fully
// loaded into memory): gzip inputs are decompressed on the fly, custom dumps
// go to pg_restore via stdin, plain SQL streams through psql. Cancelling ctx
// aborts the running restore tool.
func (p *PG) Restore(ctx context.Context, name, inPath string) error {
	exists, err := p.DatabaseExists(name)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("database %s does not exist — create it with: odoonoir init <instance> %s (or odoonoir create <instance>)", name, name)
	}
	if err := ValidateDump(inPath); err != nil {
		return err
	}
	f, err := os.Open(inPath)
	if err != nil {
		return err
	}
	defer f.Close()
	head := make([]byte, 5)
	n, err := io.ReadFull(f, head)
	if err != nil && err != io.ErrUnexpectedEOF {
		return err
	}
	head = head[:n]
	// Feed the peeked bytes back into the stream so classification never
	// consumes content.
	var src io.Reader = io.MultiReader(bytes.NewReader(head), f)
	if isGzip(head) {
		zr, err := gzip.NewReader(src)
		if err != nil {
			return fmt.Errorf("not a valid gzip file: %w", err)
		}
		defer zr.Close()
		src = zr
	}
	// Classify custom vs plain: a plain dump is decided by the outer bytes;
	// a gzipped dump must be classified after decompression.
	if isCustomDump(head) {
		return p.restoreCustom(ctx, name, src)
	}
	if isGzip(head) {
		inner := make([]byte, 5)
		n, err := io.ReadFull(src, inner)
		if err != nil && err != io.ErrUnexpectedEOF {
			return err
		}
		inner = inner[:n]
		if isCustomDump(inner) {
			return p.restoreCustom(ctx, name, io.MultiReader(bytes.NewReader(inner), src))
		}
		return p.restoreSQL(ctx, name, io.MultiReader(bytes.NewReader(inner), src))
	}
	return p.restoreSQL(ctx, name, src)
}

// restoreCustom streams a pg_dump custom-format dump into pg_restore.
func (p *PG) restoreCustom(ctx context.Context, name string, src io.Reader) error {
	cmd := exec.CommandContext(ctx, "pg_restore", "--dbname", name, "--no-owner", "--exit-on-error")
	cmd.Env = p.pgEnv()
	cmd.Stdin = src
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("pg_restore: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// restoreSQL streams plain SQL through psql with ON_ERROR_STOP so syntax
// errors abort the restore (psql would otherwise exit 0).
func (p *PG) restoreSQL(ctx context.Context, name string, src io.Reader) error {
	cmd := exec.CommandContext(ctx, "psql", "-d", name, "-q", "--set", "ON_ERROR_STOP=1")
	cmd.Env = p.pgEnv()
	cmd.Stdin = src
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("psql restore: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ValidateDump checks that a dump file exists, is non-empty and starts with
// a recognizable signature (gzip, pg_dump custom or plain SQL text). Call it
// BEFORE destructive steps so a bad dump never costs a database.
func ValidateDump(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("dump file not found: %s", path)
	}
	if fi.IsDir() {
		return fmt.Errorf("dump path is a directory: %s", path)
	}
	if fi.Size() == 0 {
		return fmt.Errorf("dump file is empty: %s", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	head := make([]byte, 5)
	n, err := io.ReadFull(f, head)
	if err != nil && err != io.ErrUnexpectedEOF {
		return err
	}
	head = head[:n]
	if len(head) == 0 {
		return fmt.Errorf("dump file is empty: %s", path)
	}
	if isGzip(head) {
		return nil
	}
	if isCustomDump(head) {
		return nil
	}
	if !looksLikeSQL(head) {
		return fmt.Errorf("dump file has an unknown format: %s (expected gzip, pg_dump custom or plain SQL)", path)
	}
	return nil
}

// looksLikeSQL heuristically checks whether the start of a file is plain
// SQL text (pg_dump plain output): a comment, a COPY/SET/ALTER/CREATE/BEGIN
// statement or a SQL keyword.
func looksLikeSQL(head []byte) bool {
	text := strings.ToUpper(strings.TrimSpace(string(head)))
	for _, kw := range []string{"--", "CREATE", "ALTER", "SET", "BEGIN", "COPY", "INSERT", "SELECT", "DROP", "COMMENT", "GRANT", "REVOKE"} {
		if strings.HasPrefix(text, kw) {
			return true
		}
	}
	return false
}

func isText(b []byte) bool {
	for _, c := range b {
		if c < 0x20 && c != '\n' && c != '\r' && c != '\t' {
			return false
		}
	}
	return true
}

// RenameDatabase renames a database, terminating lingering connections
// first (sessions from a stopped instance). Fails if the target exists.
func (p *PG) RenameDatabase(oldName, newName string) error {
	if err := dbIdentValid(oldName); err != nil {
		return err
	}
	if err := dbIdentValid(newName); err != nil {
		return err
	}
	exists, err := p.DatabaseExists(newName)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("database %s already exists", newName)
	}
	exists, err = p.DatabaseExists(oldName)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("database %s does not exist", oldName)
	}
	term := exec.Command("psql", "-d", "postgres", "-tAc",
		"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '"+PgEscapeLiteral(oldName)+"' AND pid <> pg_backend_pid()")
	term.Env = p.pgEnv()
	if out, err := term.CombinedOutput(); err != nil {
		return fmt.Errorf("terminate connections to %s: %w\n%s", oldName, err, strings.TrimSpace(string(out)))
	}
	rename := exec.Command("psql", "-d", "postgres", "-c",
		"ALTER DATABASE \""+oldName+"\" RENAME TO \""+newName+"\"")
	rename.Env = p.pgEnv()
	if out, err := rename.CombinedOutput(); err != nil {
		return fmt.Errorf("rename database %s -> %s: %w\n%s", oldName, newName, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func dbIdentValid(name string) error {
	if !regexp.MustCompile(`^[a-z][a-z0-9_]*$`).MatchString(name) {
		return fmt.Errorf("invalid database name %q: use lowercase letters, digits and underscores only", name)
	}
	return nil
}

// PgEscapeLiteral escapes a string for use as a SQL literal (single-quote safe).
func PgEscapeLiteral(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// DatabaseSize returns the on-disk size of a database in bytes.
func (p *PG) DatabaseSize(name string) (int64, error) {
	if err := dbIdentValid(name); err != nil {
		return 0, err
	}
	out, err := p.psql("postgres", "SELECT pg_database_size('"+name+"')")
	if err != nil {
		return 0, err
	}
	size, err := strconv.ParseInt(out, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse size: %w", err)
	}
	return size, nil
}

// ListDatabases returns non-system databases.
func (p *PG) ListDatabases() ([]string, error) {
	out, err := p.psql("postgres",
		"SELECT datname FROM pg_database WHERE datistemplate = false AND datname != 'postgres' ORDER BY datname")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return []string{}, nil
	}
	return strings.Split(out, "\n"), nil
}

// DatabasesForRole returns non-system databases owned by the given role.
func (p *PG) DatabasesForRole(role string) ([]string, error) {
	if err := dbIdentValid(role); err != nil {
		return nil, err
	}
	out, err := p.psql("postgres",
		"SELECT datname FROM pg_database d JOIN pg_roles r ON r.oid = d.datdba WHERE datistemplate = false AND datname != 'postgres' AND r.rolname = '"+role+"' ORDER BY datname")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return []string{}, nil
	}
	return strings.Split(out, "\n"), nil
}

// DatabaseOwner returns the role that owns a database.
func (p *PG) DatabaseOwner(name string) (string, error) {
	if err := dbIdentValid(name); err != nil {
		return "", err
	}
	out, err := p.psql("postgres",
		"SELECT r.rolname FROM pg_database d JOIN pg_roles r ON r.oid = d.datdba WHERE d.datname = '"+name+"'")
	if err != nil {
		return "", err
	}
	return out, nil
}

// IsInitialized reports whether a database has Odoo tables (ir_module_module).
func (p *PG) IsInitialized(name string) (bool, error) {
	if err := dbIdentValid(name); err != nil {
		return false, err
	}
	out, err := p.psql(name,
		"SELECT 1 FROM information_schema.tables WHERE table_name = 'ir_module_module' AND table_schema = 'public'")
	if err != nil {
		return false, nil // unreadable / not created — treat as uninitialized
	}
	return out == "1", nil
}

// IsValidName validates a database/role identifier.
func IsValidName(name string) error { return dbIdentValid(name) }

// SuggestDBName derives a database name from an instance name.
func SuggestDBName(instanceName string) string {
	return instanceName
}

func currentUser() string {
	out, err := exec.Command("whoami").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// CheckPasswordAccess verifies password auth against the odoo role.
func (p *PG) CheckPasswordAccess(role, password string) error {
	cmd := exec.Command("psql", "-h", p.cfg.PostgresHost, "-p", fmt.Sprint(p.cfg.PostgresPort),
		"-U", role, "-d", "postgres", "-tAc", "SELECT 1")
	cmd.Env = append(os.Environ(), "PGPASSWORD="+password)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("auth as %s failed: %w\n%s", role, err, strings.TrimSpace(string(out)))
	}
	if strings.TrimSpace(string(out)) != "1" {
		return fmt.Errorf("unexpected auth result for %s", role)
	}
	return nil
}

// isGzip reports whether data starts with the gzip magic bytes (0x1f 0x8b).
func isGzip(data []byte) bool {
	return len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b
}

var customDumpMagic = []byte("PGDMP")

func isCustomDump(data []byte) bool {
	return bytes.HasPrefix(data, customDumpMagic)
}

func dirOf(path string) string {
	idx := strings.LastIndex(path, "/")
	if idx < 0 {
		return "."
	}
	return path[:idx]
}
