package service

import (
	"testing"

	"github.com/ahmed/odoonoir/internal/instance"
)

func TestIsNotFound(t *testing.T) {
	if !IsNotFound(instance.ErrNotFound) {
		t.Fatal("IsNotFound")
	}
	if IsNotFound(nil) {
		t.Fatal("IsNotFound nil")
	}
}

func TestRootFor(t *testing.T) {
	svc := newTestService(t)
	inst := &instance.Instance{Name: "app", Root: "/custom"}
	if got := svc.rootFor(inst); got != "/custom" {
		t.Fatalf("rootFor custom %q", got)
	}
	inst2 := &instance.Instance{Name: "app2"}
	if got := svc.rootFor(inst2); got != svc.cfg.InstancesDir() {
		t.Fatalf("rootFor default %q", got)
	}
}

func TestRequireDB(t *testing.T) {
	svc := newTestService(t)
	inst := &instance.Instance{Name: "app", DBName: "main", Databases: []string{"extra"}}
	_ = svc.reg.Put(inst)
	gotInst, _ := svc.reg.Get("app")
	if err := svc.requireDB(gotInst, "main"); err != nil {
		t.Fatalf("require main %v", err)
	}
	if err := svc.requireDB(gotInst, "extra"); err != nil {
		t.Fatalf("require extra %v", err)
	}
	if err := svc.requireDB(gotInst, "missing"); err == nil {
		t.Fatal("expected error for missing")
	}
}

func TestRequireStopped(t *testing.T) {
	svc := newTestService(t)
	inst := &instance.Instance{Name: "app", DBName: "app", Port: 8069, LongpollPort: 8072}
	_ = svc.reg.Put(inst)
	// should be stopped initially (no proc)
	if err := svc.requireStopped("app"); err != nil {
		t.Fatalf("requireStopped stopped %v", err)
	}
	if err := svc.requireStopped("nope"); err == nil {
		t.Fatal("expected error for missing")
	}
}
