package installer

import (
	"testing"

	"github.com/ahmed/odoonoir/internal/config"
	"github.com/ahmed/odoonoir/internal/instance"
)

func TestRootFor(t *testing.T) {
	cfg := &config.Config{InstancesRoot: "/data/instances"}
	inst := &instance.Instance{Name: "myapp"}

	if got := RootFor(cfg, inst); got != "/data/instances" {
		t.Fatalf("default root: got %q, want %q", got, "/data/instances")
	}

	inst.Root = "/custom/root"
	if got := RootFor(cfg, inst); got != "/custom/root" {
		t.Fatalf("override root: got %q, want %q", got, "/custom/root")
	}

	if p := inst.ResolvePaths(RootFor(cfg, inst)); p.Source != "/custom/root/myapp/src/odoo" {
		t.Fatalf("ResolvePaths must honour the per-instance root, got %q", p.Source)
	}
}
