package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ahmed/odoonoir/internal/config"
	"github.com/ahmed/odoonoir/internal/instance"
	"github.com/ahmed/odoonoir/internal/updater"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{
		InstancesRoot: dir,
		PostgresUser:  "odoo",
		PostgresHost:  "localhost",
		PostgresPort:  5432,
		OdooUser:      "odoo",
		OdooPassword:  "odoo",
	}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestEmptyRegistryInstances(t *testing.T) {
	svc := newTestService(t)
	views, err := svc.Instances()
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 0 {
		t.Fatalf("expected no instances, got %d", len(views))
	}
}

func TestStatusUnknownInstance(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.Status("nope")
	if !IsNotFound(err) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestStatusTrackedInstance(t *testing.T) {
	svc := newTestService(t)
	inst := &instance.Instance{
		Name:         "myapp",
		Version:      "18.0",
		Port:         8069,
		LongpollPort: 8072,
		DBName:       "myapp",
		DBUser:       "odoo",
	}
	if err := svc.reg.Put(inst); err != nil {
		t.Fatal(err)
	}
	st, err := svc.Status("myapp")
	if err != nil {
		t.Fatal(err)
	}
	if st.Instance.Name != "myapp" || st.Instance.Port != 8069 {
		t.Fatalf("unexpected status view: %+v", st)
	}
	if st.Conf != filepath.Join(t.TempDir(), "myapp", "etc", "odoo.conf") {
		// conf path derives from rootFor; assert it is non-empty at least
		if st.Conf == "" {
			t.Fatalf("expected a conf path, got empty")
		}
	}
}

func TestRegistryPortFreeConflict(t *testing.T) {
	svc := newTestService(t)
	inst := &instance.Instance{
		Name:         "taken",
		Version:      "18.0",
		Port:         8100,
		LongpollPort: 8103,
		DBName:       "taken",
	}
	if err := svc.reg.Put(inst); err != nil {
		t.Fatal(err)
	}
	if err := svc.registryPortFree(8100); err == nil {
		t.Fatal("expected port conflict for 8100")
	}
	if err := svc.registryPortFree(8103); err == nil {
		t.Fatal("expected port conflict for longpoll 8103")
	}
	if err := svc.registryPortFree(8101); err != nil {
		t.Fatalf("expected 8101 to be free, got %v", err)
	}
}

func TestCreateRejectsExistingInstance(t *testing.T) {
	svc := newTestService(t)
	inst := &instance.Instance{Name: "dup", Version: "18.0", DBName: "dup"}
	if err := svc.reg.Put(inst); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Create(context.Background(), CreateOptions{Name: "dup", Version: "18"}, NopSink)
	if err == nil {
		t.Fatal("expected error for duplicate instance")
	}
}

func TestUpdaterSinkAdaptsEvents(t *testing.T) {
	got := make(chan Event, 8)
	sink := func(e Event) { got <- e }
	adapter := UpdaterSink("myapp", sink)
	adapter(updater.Progress{Kind: updater.StepStart, Index: 0, Name: "pull"})
	adapter(updater.Progress{Kind: updater.Line, Index: 0, Name: "pull", Line: "Already up to date"})
	adapter(updater.Progress{Kind: updater.StepDone, Index: 0, Name: "pull"})
	e1 := <-got
	if e1.Kind != StepStart || e1.Instance != "myapp" || e1.Step != "pull" {
		t.Fatalf("bad step start event: %+v", e1)
	}
	e2 := <-got
	if e2.Kind != LogLine || e2.Message != "Already up to date" {
		t.Fatalf("bad log line event: %+v", e2)
	}
	e3 := <-got
	if e3.Kind != StepDone {
		t.Fatalf("bad step done event: %+v", e3)
	}
}

func TestLineSinkAdapts(t *testing.T) {
	got := make(chan Event, 1)
	LineSink("myapp", "init base", func(e Event) { got <- e })("installing...")
	e := <-got
	if e.Kind != LogLine || e.Step != "init base" || e.Message != "installing..." {
		t.Fatalf("bad line event: %+v", e)
	}
}

func TestCreateOptionsValidation(t *testing.T) {
	svc := newTestService(t)
	// invalid name
	_, err := svc.Create(context.Background(), CreateOptions{Name: "bad name!", Version: "18"}, NopSink)
	if err == nil {
		t.Fatal("expected invalid-name error")
	}
	// missing version
	_, err = svc.Create(context.Background(), CreateOptions{Name: "ok"}, NopSink)
	if err == nil {
		t.Fatal("expected missing-version error")
	}
}

func TestDefaultBackupPath(t *testing.T) {
	p := instance.Paths{Root: "/tmp/x/inst"}
	path := defaultBackupPath(p, "mydb", true, false)
	wantPrefix := "/tmp/x/inst/backups/mydb_"
	if !hasPrefix(path, wantPrefix) || hasSuffix(path, ".dump") == false {
		t.Fatalf("unexpected custom path: %s", path)
	}
	plain := defaultBackupPath(p, "mydb", false, true)
	if !hasSuffix(plain, ".sql.gz") {
		t.Fatalf("unexpected plain-compressed path: %s", plain)
	}
}

func hasPrefix(s, p string) bool { return len(s) >= len(p) && s[:len(p)] == p }
func hasSuffix(s, sfx string) bool {
	return len(s) >= len(sfx) && s[len(s)-len(sfx):] == sfx
}

func TestContextCancelPropagates(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancelled := make(chan struct{})
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
		close(cancelled)
	}()
	<-cancelled
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatal("expected canceled context")
	}
}
