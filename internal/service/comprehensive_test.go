package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ahmed/odoonoir/internal/instance"
	"github.com/ahmed/odoonoir/internal/odoconf"
)

func newTestServiceWithInstance(t *testing.T) (*Service, string) {
	t.Helper()
	svc := newTestService(t)
	inst := &instance.Instance{
		Name:         "myapp",
		Version:      "18.0",
		Port:         8069,
		LongpollPort: 8072,
		DBName:       "myapp",
		DBUser:       "odoo",
		Databases:    []string{"myapp"},
	}
	if err := svc.reg.Put(inst); err != nil {
		t.Fatal(err)
	}
	p := inst.ResolvePaths(svc.rootFor(inst))
	_ = os.MkdirAll(filepath.Dir(p.Conf), 0o755)
	_ = os.WriteFile(p.Conf, []byte("[options]\naddons_path = /a,/b\nhttp_port = 8069\nworkers = 2\n"), 0o644)
	return svc, inst.Name
}

// ── ReadConf / SetConf / CreateConf ───────────────────────────────────────

func TestReadConf(t *testing.T) {
	svc, name := newTestServiceWithInstance(t)
	entries, err := svc.ReadConf(name)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 2 {
		t.Fatalf("expected at least 2 entries, got %d", len(entries))
	}
	found := false
	for _, e := range entries {
		if e.Name == "http_port" && e.Value == "8069" {
			found = true
		}
	}
	if !found {
		t.Fatal("http_port=8069 not found in ReadConf output")
	}
}

func TestReadConfNotFound(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.ReadConf("nope")
	if err == nil {
		t.Fatal("expected error for missing instance")
	}
}

func TestSetConf(t *testing.T) {
	svc, name := newTestServiceWithInstance(t)
	if err := svc.SetConf(name, "workers", "4"); err != nil {
		t.Fatal(err)
	}
	entries, _ := svc.ReadConf(name)
	for _, e := range entries {
		if e.Name == "workers" && e.Value == "4" {
			return
		}
	}
	t.Fatal("workers=4 not found after SetConf")
}

func TestSetConfNotFound(t *testing.T) {
	svc := newTestService(t)
	err := svc.SetConf("nope", "key", "val")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateConf(t *testing.T) {
	svc, name := newTestServiceWithInstance(t)
	if err := svc.CreateConf(name); err != nil {
		t.Fatal(err)
	}
	inst, _ := svc.reg.Get(name)
	p := inst.ResolvePaths(svc.rootFor(inst))
	c, err := odoconf.Load(p.Conf)
	if err != nil {
		t.Fatal(err)
	}
	v, _ := c.Get("db_name")
	if v != "myapp" {
		t.Fatalf("expected db_name=myapp, got %q", v)
	}
}

func TestCreateConfNotFound(t *testing.T) {
	svc := newTestService(t)
	err := svc.CreateConf("nope")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ── Statuses (batch) ──────────────────────────────────────────────────────

func TestStatusesEmpty(t *testing.T) {
	svc := newTestService(t)
	list, err := svc.Statuses()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty, got %d", len(list))
	}
}

func TestStatusesMultiple(t *testing.T) {
	svc := newTestService(t)
	for i, name := range []string{"a", "b", "c"} {
		inst := &instance.Instance{
			Name:         name,
			Version:      "18.0",
			Port:         8069 + i*2,
			LongpollPort: 8072 + i*2,
			DBName:       name,
			DBUser:       "odoo",
			Databases:    []string{name},
		}
		_ = svc.reg.Put(inst)
	}
	list, err := svc.Statuses()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 statuses, got %d", len(list))
	}
	names := map[string]bool{}
	for _, s := range list {
		names[s.Instance.Name] = true
	}
	for _, want := range []string{"a", "b", "c"} {
		if !names[want] {
			t.Fatalf("missing status for %s", want)
		}
	}
}

// ── Instances ─────────────────────────────────────────────────────────────

func TestInstancesPopulated(t *testing.T) {
	svc := newTestService(t)
	inst := &instance.Instance{
		Name: "test-inst", Version: "17.0", Port: 8080,
		LongpollPort: 8083, DBName: "test-inst", DBUser: "odoo",
		Databases: []string{"test-inst"},
	}
	_ = svc.reg.Put(inst)
	list, err := svc.Instances()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1, got %d", len(list))
	}
	if list[0].Name != "test-inst" {
		t.Fatalf("unexpected name %q", list[0].Name)
	}
}

// ── TrackDatabase ─────────────────────────────────────────────────────────

func TestTrackDatabaseNotFound(t *testing.T) {
	svc := newTestService(t)
	err := svc.TrackDatabase("nope", "db")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestTrackDatabaseInvalidName(t *testing.T) {
	svc, name := newTestServiceWithInstance(t)
	err := svc.TrackDatabase(name, "bad name!")
	if err == nil {
		t.Fatal("expected error for invalid name")
	}
}

func TestTrackDatabaseAlreadyTracked(t *testing.T) {
	svc, name := newTestServiceWithInstance(t)
	err := svc.TrackDatabase(name, "myapp") // already in Databases list
	if err == nil {
		t.Fatal("expected error for already tracked")
	}
}

// ── Databases (batch info) ────────────────────────────────────────────────

func TestDatabasesNoPostgres(t *testing.T) {
	svc, name := newTestServiceWithInstance(t)
	_, err := svc.Databases(name)
	// Expected to fail because PG is not running in test
	if err == nil {
		t.Log("Databases returned successfully (PG may be running)")
	} else {
		t.Logf("Databases error (expected without PG): %v", err)
	}
}

func TestDatabasesNotFound(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.Databases("nope")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ── GetBackupScheduleStatus / ListScheduledBackups ────────────────────────

func TestGetBackupScheduleStatus(t *testing.T) {
	svc, name := newTestServiceWithInstance(t)
	status, err := svc.GetBackupScheduleStatus(name)
	if err != nil {
		t.Fatal(err)
	}
	if status.Enabled {
		t.Fatal("expected disabled by default")
	}
}

func TestGetBackupScheduleStatusNotFound(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.GetBackupScheduleStatus("nope")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListScheduledBackupsNotFound(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.ListScheduledBackups("nope")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ── ValidateDBConfig ──────────────────────────────────────────────────────

func TestValidateDBConfig(t *testing.T) {
	svc, name := newTestServiceWithInstance(t)
	_, err := svc.ValidateDBConfig(name)
	// May fail because PG not running, but shouldn't panic
	if err != nil {
		t.Logf("ValidateDBConfig error (expected without PG): %v", err)
	}
}

// ── Clone ─────────────────────────────────────────────────────────────────

func TestCloneNotFound(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	_, err := svc.Clone(ctx, "nope", "newname", 8090, NopSink)
	if err == nil {
		t.Fatal("expected error")
	}
}

// ── Remove ────────────────────────────────────────────────────────────────

func TestRemoveNotFound(t *testing.T) {
	svc := newTestService(t)
	err := svc.Remove("nope", false)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRemoveAlreadyStopped(t *testing.T) {
	svc, name := newTestServiceWithInstance(t)
	err := svc.Remove(name, false)
	// Should not panic; may fail due to missing directory but shouldn't be "not found"
	if err != nil {
		t.Logf("Remove error (expected without dirs): %v", err)
	}
}

// ── Databases ─────────────────────────────────────────────────────────────

func TestDatabasesWithDiscovered(t *testing.T) {
	svc := newTestService(t)
	inst := &instance.Instance{
		Name: "app", Version: "18.0", Port: 8069,
		LongpollPort: 8072, DBName: "app", DBUser: "odoo",
		Databases: []string{"app"},
	}
	_ = svc.reg.Put(inst)
	// Will fail because PG not running, but tests the path
	_, err := svc.Databases("app")
	if err != nil {
		t.Logf("Databases error (expected without PG): %v", err)
	}
}

// ── Update ────────────────────────────────────────────────────────────────

func TestUpdateNotFound(t *testing.T) {
	svc := newTestService(t)
	err := svc.Update(nil, "nope", UpdateOptions{}, NopSink)
	if err == nil {
		t.Fatal("expected error")
	}
}

// ── SearchGitHubModules ───────────────────────────────────────────────────

func TestSearchGitHubModulesNoToken(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.SearchGitHubModules(nil, GitHubSearchOptions{Query: "test"})
	// Should fail gracefully (no token configured)
	if err != nil {
		t.Logf("SearchGitHubModules error (expected without token): %v", err)
	}
}

// ── GetSSHClient ──────────────────────────────────────────────────────────

func TestGetSSHClientMissing(t *testing.T) {
	svc := newTestService(t)
	_, ok := svc.GetSSHClient("nope")
	if ok {
		t.Fatal("expected false for missing SSH client")
	}
}

func TestListSSHConnectionsEmpty(t *testing.T) {
	svc := newTestService(t)
	list := svc.ListSSHConnections()
	if len(list) != 0 {
		t.Fatalf("expected empty, got %d", len(list))
	}
}
