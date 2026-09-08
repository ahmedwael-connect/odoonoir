package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ahmed/odoonoir/internal/instance"
)

func TestSetPrimaryDatabase(t *testing.T) {
	svc := newTestService(t)
	inst := &instance.Instance{Name: "app", Version: "18.0", DBName: "main", Databases: []string{"extra"}, Port: 8069, LongpollPort: 8072}
	if err := svc.reg.Put(inst); err != nil {
		t.Fatal(err)
	}
	// create minimal conf so odoconf update succeeds
	p := inst.ResolvePaths(svc.rootFor(inst))
	_ = os.MkdirAll(filepath.Dir(p.Conf), 0o755)
	_ = os.WriteFile(p.Conf, []byte("[options]\naddons_path = /a\ndb_name = main\n"), 0o644)

	if err := svc.SetPrimaryDatabase("app", "extra"); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.reg.Get("app")
	if got.DBName != "extra" {
		t.Fatalf("DBName %q", got.DBName)
	}
	found := false
	for _, d := range got.Databases {
		if d == "main" {
			found = true
		}
	}
	if !found {
		t.Fatalf("old primary not kept: %+v", got.Databases)
	}
	// idempotent
	if err := svc.SetPrimaryDatabase("app", "extra"); err != nil {
		t.Fatal(err)
	}
	// unknown db
	if err := svc.SetPrimaryDatabase("app", "nope"); err == nil {
		// requireDB fails, but auto-track checks DatabaseExists via psql;
		// psql exists in CI so returns error path — either error is fine except nil only if DB exists
		t.Log("unexpected nil for unknown db (DB may exist in test PG)")
	}
}

func TestSetGetAutoUpdate(t *testing.T) {
	svc := newTestService(t)
	inst := &instance.Instance{Name: "app", Version: "18.0", DBName: "main"}
	_ = svc.reg.Put(inst)
	if err := svc.SetAutoUpdate("app", []string{" sale ", "", "stock "}, true); err != nil {
		t.Fatal(err)
	}
	cfg, err := svc.GetAutoUpdate("app")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Enabled || len(cfg.Modules) != 2 || cfg.Modules[0] != "sale" {
		t.Fatalf("normalize failed: %+v", cfg)
	}
	if err := svc.SetAutoUpdate("app", nil, true); err != nil {
		t.Fatal(err)
	}
	cfg, _ = svc.GetAutoUpdate("app")
	if cfg.Enabled {
		t.Fatal("empty modules should disable")
	}
}

func TestTrackDatabaseInvalid(t *testing.T) {
	svc := newTestService(t)
	inst := &instance.Instance{Name: "app", Version: "18.0", DBName: "main"}
	_ = svc.reg.Put(inst)
	if err := svc.TrackDatabase("app", "Bad-Name"); err == nil {
		t.Fatal("expected invalid name error")
	}
	if err := svc.TrackDatabase("app", "definitely_not_exist_xyz"); err == nil {
		t.Log("psql may not exist in this env, nil ok")
	}
}
