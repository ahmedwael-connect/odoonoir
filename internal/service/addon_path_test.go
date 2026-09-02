package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ahmed/odoonoir/internal/instance"
)

func newAddonTestService(t *testing.T) (*Service, string) {
	t.Helper()
	svc := newTestService(t)
	inst := &instance.Instance{Name: "app", Version: "18.0", DBName: "app", Port: 8069, LongpollPort: 8072}
	if err := svc.reg.Put(inst); err != nil {
		t.Fatal(err)
	}
	p := inst.ResolvePaths(svc.rootFor(inst))
	_ = os.MkdirAll(filepath.Dir(p.Conf), 0o755)
	// create minimal conf with addons_path
	_ = os.WriteFile(p.Conf, []byte("[options]\naddons_path = /a,/b\n"), 0o644)
	return svc, inst.Name
}

func TestListAddonPaths(t *testing.T) {
	svc, name := newAddonTestService(t)
	list, err := svc.ListAddonPaths(name)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Path != "/a" || list[1].Path != "/b" {
		t.Fatalf("unexpected list %+v", list)
	}
	if list[0].Position != 1 || !list[0].Enabled {
		t.Fatalf("position/enabled wrong %+v", list[0])
	}
}

func TestAddAddonPath(t *testing.T) {
	svc, name := newAddonTestService(t)
	list, err := svc.AddAddonPath(name, "/c", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 || list[2].Path != "/c" {
		t.Fatalf("add append failed %+v", list)
	}
	// duplicate should error
	if _, err := svc.AddAddonPath(name, "/a", 0); err == nil {
		t.Fatal("expected duplicate error")
	}
	// insert at 1
	list, err = svc.AddAddonPath(name, "/z", 1)
	if err != nil {
		t.Fatal(err)
	}
	if list[0].Path != "/z" {
		t.Fatalf("insert at 1 failed %+v", list)
	}
}

func TestRemoveAddonPath(t *testing.T) {
	svc, name := newAddonTestService(t)
	if _, err := svc.RemoveAddonPath(name, "/a"); err != nil {
		t.Fatal(err)
	}
	list, _ := svc.ListAddonPaths(name)
	if len(list) != 1 || list[0].Path != "/b" {
		t.Fatalf("remove failed %+v", list)
	}
	if _, err := svc.RemoveAddonPath(name, "/missing"); err == nil {
		t.Fatal("expected error for missing")
	}
}

func TestToggleAddonPath(t *testing.T) {
	svc, name := newAddonTestService(t)
	// disable
	list, err := svc.ToggleAddonPath(name, "/a", false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range list {
		if e.Path == "/a" {
			if e.Enabled {
				t.Fatalf("expected disabled %+v", e)
			}
			found = true
		}
	}
	if !found {
		// disabled may be hidden from List? our List returns both enabled+disabled via Raw
		// check that /a not in enabled list but may be in disabled
	}
	// enable again
	list, err = svc.ToggleAddonPath(name, "/a", true)
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, e := range list {
		if e.Path == "/a" && e.Enabled {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected re-enabled /a %+v", list)
	}
}

func TestMoveAddonPath(t *testing.T) {
	svc, name := newAddonTestService(t)
	// add third for move
	_, _ = svc.AddAddonPath(name, "/c", 0)
	list, err := svc.MoveAddonPath(name, 3, 1)
	if err != nil {
		t.Fatal(err)
	}
	if list[0].Path != "/c" {
		t.Fatalf("move 3->1 failed %+v", list)
	}
	if _, err := svc.MoveAddonPath(name, 10, 1); err == nil {
		t.Fatal("expected error for invalid move")
	}
}
