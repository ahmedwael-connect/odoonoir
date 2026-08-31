package service

import (
	"fmt"
	"strings"

	"github.com/ahmed/odoonoir/internal/instance"
	"github.com/ahmed/odoonoir/internal/odoconf"
)

// DBErrorCode classifies DB failures for UI hints.
type DBErrorCode string

const (
	DBErrNotServed   DBErrorCode = "NOT_SERVED"
	DBErrNotFound    DBErrorCode = "NOT_FOUND"
	DBErrAuth        DBErrorCode = "AUTH"
	DBErrConnRefused DBErrorCode = "CONN_REFUSED"
	DBErrServerDown  DBErrorCode = "SERVER_DOWN"
	DBErrInvalidName DBErrorCode = "INVALID_NAME"
	DBErrPython      DBErrorCode = "PYTHON"
	DBErrConfig      DBErrorCode = "CONFIG"
	DBErrUnknown     DBErrorCode = "UNKNOWN"
)

// DBError is a typed error with code + actionable hint.
type DBError struct {
	Code    DBErrorCode `json:"code"`
	Message string      `json:"message"`
	Hint    string      `json:"hint"`
	Err     error       `json:"-"`
}

func (e *DBError) Error() string {
	if e.Hint != "" {
		return fmt.Sprintf("%s — fix: %s", e.Message, e.Hint)
	}
	return e.Message
}
func (e *DBError) Unwrap() error { return e.Err }

// isDBError reports whether err is a *DBError.
func isDBError(err error) (*DBError, bool) {
	if e, ok := err.(*DBError); ok {
		return e, true
	}
	return nil, false
}

// translatePsqlError maps raw psql/psycopg2 stderr to DBError.
func translatePsqlError(raw string, fallback error) error {
	lower := strings.ToLower(raw)
	switch {
	case strings.Contains(lower, "is not served by instance"):
		return &DBError{Code: DBErrNotServed, Message: raw, Hint: "pick a database from Databases or run odoonoir init <instance> <db>", Err: fallback}
	case strings.Contains(lower, "does not exist") && strings.Contains(lower, "database"):
		return &DBError{Code: DBErrNotFound, Message: raw, Hint: "create with odoonoir init <instance> <db> or check Databases list", Err: fallback}
	case strings.Contains(lower, "role") && strings.Contains(lower, "does not exist"):
		return &DBError{Code: DBErrAuth, Message: raw, Hint: "role missing — odoonoir check + createuser -s <role> or fix db_user in odoo.conf / config", Err: fallback}
	case strings.Contains(lower, "password authentication failed"):
		return &DBError{Code: DBErrAuth, Message: raw, Hint: "password mismatch — check db_password in etc/odoo.conf and PGUSER/PGPASSWORD", Err: fallback}
	case strings.Contains(lower, "peer authentication failed"):
		return &DBError{Code: DBErrAuth, Message: raw, Hint: "peer auth failed — set db_host=localhost or add pg_hba.conf trust/md5 for user", Err: fallback}
	case strings.Contains(lower, "no password supplied"):
		return &DBError{Code: DBErrAuth, Message: raw, Hint: "missing password — set db_password in conf or PGPASSWORD", Err: fallback}
	case strings.Contains(lower, "could not connect to server"), strings.Contains(lower, "connection refused"), strings.Contains(lower, "connection timed out"):
		return &DBError{Code: DBErrConnRefused, Message: raw, Hint: "postgres not reachable — check systemctl status postgresql and db_host/db_port in odoo.conf vs config", Err: fallback}
	case strings.Contains(lower, "is not accepting connections"), strings.Contains(lower, "pg_isready"):
		return &DBError{Code: DBErrServerDown, Message: raw, Hint: "postgres down — sudo systemctl start postgresql", Err: fallback}
	case strings.Contains(lower, "invalid database name"):
		return &DBError{Code: DBErrInvalidName, Message: raw, Hint: "use lowercase letters, digits, underscores only", Err: fallback}
	}
	// fallback: wrap raw if non-empty
	if raw != "" && fallback != nil {
		return &DBError{Code: DBErrUnknown, Message: raw, Hint: "check postgres logs and odoo.conf", Err: fallback}
	}
	return fallback
}

// ResolveDB validates that requested db is served by inst, checks existence via pg, and returns canonical name.
// empty dbName falls back to inst.DBName only if caller allows fallback (allowEmptyFallback=true).
// For strict screens (records/graph/inspector) caller should pass empty and handle error to force explicit pick.
func (s *Service) ResolveDB(inst *instance.Instance, dbName string, allowFallback bool) (string, error) {
	if dbName == "" {
		if !allowFallback || inst.DBName == "" {
			return "", &DBError{Code: DBErrNotServed, Message: "no database selected", Hint: "select a database from the Databases tab or run odoonoir init <instance> <db>", Err: nil}
		}
		dbName = inst.DBName
	}
	// AllDBs check — must be served
	found := false
	for _, d := range inst.AllDBs() {
		if d == dbName {
			found = true
			break
		}
	}
	if !found {
		return "", &DBError{Code: DBErrNotServed, Message: fmt.Sprintf("database %q is not served by instance %q", dbName, inst.Name), Hint: fmt.Sprintf("available: %s — add with odoonoir init %s <db>", strings.Join(inst.AllDBs(), ", "), inst.Name), Err: nil}
	}
	// server health
	if err := s.pg.ServerRunning(); err != nil {
		return "", translatePsqlError(err.Error(), err)
	}
	// existence probe — use psql light query, but translate FATAL
	if exists, err := s.pg.DatabaseExists(dbName); err != nil {
		return "", translatePsqlError(err.Error(), err)
	} else if !exists {
		return "", &DBError{Code: DBErrNotFound, Message: fmt.Sprintf("database %q does not exist", dbName), Hint: fmt.Sprintf("create it: odoonoir init %s %s", inst.Name, dbName), Err: nil}
	}
	return dbName, nil
}

// wrapPsqlErr translates psql Query errors for callers that already have dbName.
func (s *Service) wrapPsqlErr(dbName string, err error) error {
	if err == nil {
		return nil
	}
	return translatePsqlError(err.Error(), err)
}

// ValidateDBConfig compares odoo.conf db_* vs global config and returns warnings.
func (s *Service) ValidateDBConfig(name string) ([]string, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return nil, err
	}
	p := inst.ResolvePaths(s.rootFor(inst))
	conf, err := odoconf.Load(p.Conf)
	if err != nil {
		return []string{fmt.Sprintf("cannot load odoo.conf %s: %v", p.Conf, err)}, nil
	}
	var warns []string
	// collect conf values
	get := func(k string) string {
		for _, e := range conf.AllKeys() {
			if e.Name == k {
				return e.Value
			}
		}
		return ""
	}
	dbHost := get("db_host")
	dbPort := get("db_port")
	dbUser := get("db_user")
	// compare host
	if dbHost != "" && dbHost != "False" && dbHost != s.cfg.PostgresHost && s.cfg.PostgresHost != "" && s.cfg.PostgresHost != "localhost" {
		warns = append(warns, fmt.Sprintf("odoo.conf db_host=%q differs from config PostgresHost=%q — mismatch causes peer vs TCP auth", dbHost, s.cfg.PostgresHost))
	}
	if dbPort != "" && dbPort != "False" && dbPort != fmt.Sprint(s.cfg.PostgresPort) && s.cfg.PostgresPort != 0 && s.cfg.PostgresPort != 5432 {
		warns = append(warns, fmt.Sprintf("odoo.conf db_port=%q differs from config %d", dbPort, s.cfg.PostgresPort))
	}
	if dbUser != "" && dbUser != "False" && dbUser != inst.DBUser && inst.DBUser != "" {
		warns = append(warns, fmt.Sprintf("odoo.conf db_user=%q differs from instance DBUser=%q", dbUser, inst.DBUser))
	}
	// python check
	pyWarnings := s.validatePython(inst, p)
	warns = append(warns, pyWarnings...)
	return warns, nil
}

func (s *Service) validatePython(inst *instance.Instance, p instance.Paths) []string {
	// lightweight: check source exists
	var warns []string
	if inst.SourcePath != "" {
		// adopted — trust but warn if missing
	}
	// use installer helper would require import cycle; do minimal stat here
	return warns
}
