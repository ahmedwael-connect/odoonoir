package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppDirDefault(t *testing.T) {
	t.Setenv("ODOONOIR_HOME", "")
	home, _ := os.UserHomeDir()
	dir, err := AppDir()
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(home, ".odoonoir") {
		t.Fatalf("AppDir %q", dir)
	}
}

func TestAppDirEnv(t *testing.T) {
	t.Setenv("ODOONOIR_HOME", "/tmp/custom")
	dir, _ := AppDir()
	if dir != "/tmp/custom" {
		t.Fatalf("env %q", dir)
	}
}

func TestInstancesDir(t *testing.T) {
	cfg := &Config{InstancesRoot: "/tmp/inst"}
	if got := cfg.InstancesDir(); got != "/tmp/inst" {
		t.Fatalf("InstancesDir %q", got)
	}
	if got := cfg.InstancePath("myapp"); got != "/tmp/inst/myapp" {
		t.Fatalf("InstancePath %q", got)
	}
	if got := cfg.RegistryPath(); got != "/tmp/inst/instances.json" {
		t.Fatalf("RegistryPath %q", got)
	}
}
