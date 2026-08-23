package adopt

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahmed/odoonoir/internal/config"
	"github.com/ahmed/odoonoir/internal/instance"
)

// buildFakeTree creates a plausible existing Odoo installation with a
// non-standard layout:
//
//	/tmp/x/install/odoo-bin
//	/tmp/x/install/odoo/release.py       (version 17.0)
//	/tmp/x/install/mods/custom            (addons)
//	/tmp/x/venv/bin/python
//	/tmp/x/conf/odoo.conf
func buildFakeTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	inst := filepath.Join(root, "install")
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(inst, "odoo-bin"), "#!/bin/sh\necho fake\n")
	write(filepath.Join(inst, "odoo", "release.py"),
		"import sys\ninfo = {}\ninfo['version'] = '17.0'\n")
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatal(err)
		}
	}
	run(inst, "git", "init", "-q", "-b", "17.0")
	run(inst, "git", "config", "user.email", "t@t")
	run(inst, "git", "config", "user.name", "t")
	run(inst, "git", "add", "-A")
	run(inst, "git", "commit", "-qm", "init")
	write(filepath.Join(inst, "mods", "custom", "README"), "custom addons\n")
	write(filepath.Join(root, "venv", "bin", "python"), "#!/bin/sh\necho py\n")
	write(filepath.Join(root, "conf", "odoo.conf"), `[options]
addons_path = /tmp/x/mods/custom,/tmp/x/mods/extra
http_port = 8085
longpolling_port = 8091
db_name = legacy_db
db_user = legacydb
workers = 4
log_level = warning
logfile = /var/log/legacy/odoo.log
pidfile = /run/legacy/odoo.pid
data_dir = /var/lib/legacy
`)
	return root
}

func TestDetectStandardFake(t *testing.T) {
	root := buildFakeTree(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.InstancesRoot = filepath.Join(root, "meta")

	reg, _ := instance.NewRegistry(filepath.Join(t.TempDir(), "instances.json"))
	res, err := Detect(cfg, reg, Options{Name: "legacy", Source: filepath.Join(root, "install")})
	if err != nil {
		t.Fatal(err)
	}
	inst := res.Inst
	if inst.SourcePath != filepath.Join(root, "install") {
		t.Errorf("SourcePath = %q", inst.SourcePath)
	}
	if inst.Version != "17.0" {
		t.Errorf("Version = %q", inst.Version)
	}
	if inst.ConfPath != filepath.Join(root, "conf", "odoo.conf") {
		t.Errorf("ConfPath = %q (detectConf failed)", inst.ConfPath)
	}
	if inst.VenvPath != filepath.Join(root, "venv") {
		t.Errorf("VenvPath = %q", inst.VenvPath)
	}
	if inst.Port != 8085 {
		t.Errorf("Port = %d", inst.Port)
	}
	if inst.LongpollPort != 8091 {
		t.Errorf("LongpollPort = %d", inst.LongpollPort)
	}
	if inst.DBName != "legacy_db" {
		t.Errorf("DBName = %q", inst.DBName)
	}
	if inst.DBUser != "legacydb" {
		t.Errorf("DBUser = %q", inst.DBUser)
	}
	if len(inst.AddonsPaths) != 2 || inst.AddonsPaths[0] != "/tmp/x/mods/custom" {
		t.Errorf("AddonsPaths = %v", inst.AddonsPaths)
	}
	if inst.LogPath != "/var/log/legacy/odoo.log" {
		t.Errorf("LogPath = %q", inst.LogPath)
	}
	if inst.PIDPath != "/run/legacy/odoo.pid" {
		t.Errorf("PIDPath = %q", inst.PIDPath)
	}
	if inst.DataDirPath != "/var/lib/legacy" {
		t.Errorf("DataDirPath = %q", inst.DataDirPath)
	}
	if inst.Workers != 4 {
		t.Errorf("Workers = %d", inst.Workers)
	}
	if inst.LogLevel != "warning" {
		t.Errorf("LogLevel = %q", inst.LogLevel)
	}
	if !inst.Adopted {
		t.Error("Adopted = false")
	}
	for _, w := range res.Warn {
		if !strings.Contains(w, "not initialized") {
			t.Errorf("unexpected warning: %s", w)
		}
	}
}

func TestDetectMissingSource(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	reg, _ := instance.NewRegistry(filepath.Join(t.TempDir(), "instances.json"))
	if _, err := Detect(cfg, reg, Options{Name: "ghost"}); err == nil {
		t.Error("expected error when no source can be located")
	}
}

func TestDetectInvalidName(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	reg, _ := instance.NewRegistry(filepath.Join(t.TempDir(), "instances.json"))
	if _, err := Detect(cfg, reg, Options{Name: "Bad Name!"}); err == nil {
		t.Error("expected error for invalid name")
	}
}
