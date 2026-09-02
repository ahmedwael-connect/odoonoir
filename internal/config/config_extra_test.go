package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSave(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ODOONOIR_HOME", dir)
	cfg := &Config{InstancesRoot: filepath.Join(dir, "inst"), PostgresHost: "myhost", PostgresPort: 5433}
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.InstancesRoot != cfg.InstancesRoot {
		t.Fatalf("InstancesRoot %q", loaded.InstancesRoot)
	}
	if loaded.PostgresHost != "myhost" || loaded.PostgresPort != 5433 {
		t.Fatalf("pg host/port %+v", loaded)
	}
	// check file exists
	if _, err := os.Stat(filepath.Join(dir, "config.json")); err != nil {
		t.Fatal(err)
	}
}
