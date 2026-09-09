package db

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahmed/odoonoir/internal/config"
)

// mockPSQL installs a fake psql script that responds to known queries.
func mockPSQL(t *testing.T, responses map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	// Write each response to a file
	for query, resp := range responses {
		safe := strings.ReplaceAll(query, " ", "_")
		if len(safe) > 40 {
			safe = safe[:40]
		}
		if err := os.WriteFile(filepath.Join(dir, safe+".txt"), []byte(resp), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Build a shell script with hardcoded paths
	var lines []string
	lines = append(lines, "#!/bin/sh", "QUERY=\"$4\"")
	for query := range responses {
		safe := strings.ReplaceAll(query, " ", "_")
		if len(safe) > 40 {
			safe = safe[:40]
		}
		respPath := filepath.Join(dir, safe+".txt")
		lines = append(lines, fmt.Sprintf("if echo \"$QUERY\" | grep -qF %q; then cat %q; exit 0; fi", query, respPath))
	}
	lines = append(lines, `echo "mock psql: unknown query: [$QUERY]" >&2`, "exit 1")
	script := strings.Join(lines, "\n")
	if err := os.WriteFile(filepath.Join(dir, "psql"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestBatchDatabaseInfoMock(t *testing.T) {
	responses := map[string]string{
		"SELECT datname, pg_database_size(datname), pg_get_userbyid(datdba) FROM pg_database WHERE datname IN": "mydb|1024000|odoo\notherdb|2048000|odoo",
	}
	dir := mockPSQL(t, responses)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	cfg := &config.Config{PostgresUser: "odoo", PostgresHost: "localhost", PostgresPort: 5432}
	pg := New(cfg)
	batch, err := pg.BatchDatabaseInfo([]string{"mydb", "otherdb"})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(batch))
	}
	if batch["mydb"].Size != 1024000 {
		t.Errorf("mydb size = %d", batch["mydb"].Size)
	}
	if batch["mydb"].Owner != "odoo" {
		t.Errorf("mydb owner = %q", batch["mydb"].Owner)
	}
}

func TestBatchDatabaseInfoEmpty(t *testing.T) {
	cfg := &config.Config{PostgresUser: "odoo", PostgresHost: "localhost", PostgresPort: 5432}
	pg := New(cfg)
	batch, err := pg.BatchDatabaseInfo([]string{})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) != 0 {
		t.Fatalf("expected empty, got %d", len(batch))
	}
}

func TestBatchDatabaseInfoInvalidName(t *testing.T) {
	cfg := &config.Config{PostgresUser: "odoo", PostgresHost: "localhost", PostgresPort: 5432}
	pg := New(cfg)
	_, err := pg.BatchDatabaseInfo([]string{"bad name!"})
	if err == nil {
		t.Fatal("expected error for invalid name")
	}
}

func TestListDatabasesMock(t *testing.T) {
	responses := map[string]string{
		"SELECT datname FROM pg_database WHERE datistemplate = false AND datname != 'postgres' ORDER BY datname": "db1\ndb2\ndb3",
	}
	dir := mockPSQL(t, responses)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	cfg := &config.Config{PostgresUser: "odoo", PostgresHost: "localhost", PostgresPort: 5432}
	pg := New(cfg)
	dbs, err := pg.ListDatabases()
	if err != nil {
		t.Fatal(err)
	}
	if len(dbs) != 3 {
		t.Fatalf("expected 3, got %d", len(dbs))
	}
}

func TestServerRunningMock(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "pg_isready"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	cfg := &config.Config{PostgresUser: "odoo", PostgresHost: "localhost", PostgresPort: 5432}
	pg := New(cfg)
	if err := pg.ServerRunning(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestServerNotRunningMock(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "pg_isready"), []byte("#!/bin/sh\nexit 2\n"), 0o755)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	cfg := &config.Config{PostgresUser: "odoo", PostgresHost: "localhost", PostgresPort: 5432}
	pg := New(cfg)
	if err := pg.ServerRunning(); err == nil {
		t.Fatal("expected error when server not running")
	}
}

func TestIsInitializedMock(t *testing.T) {
	responses := map[string]string{
		"SELECT 1 FROM information_schema.tables WHERE table_name = 'ir_module_module' AND table_schema = 'public'": "1",
	}
	dir := mockPSQL(t, responses)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	cfg := &config.Config{PostgresUser: "odoo", PostgresHost: "localhost", PostgresPort: 5432}
	pg := New(cfg)
	ok, err := pg.IsInitialized("mydb")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected initialized")
	}
}
