package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ahmed/odoonoir/internal/instance"
)

func TestDetectEnterpriseNotEnterprise(t *testing.T) {
	svc := newTestService(t)
	inst := &instance.Instance{Name: "app", Version: "16.0", DBName: "app", Port: 8069, LongpollPort: 8072}
	_ = svc.reg.Put(inst)
	p := inst.ResolvePaths(svc.rootFor(inst))
	_ = os.MkdirAll(filepath.Dir(p.Conf), 0o755)
	_ = os.WriteFile(p.Conf, []byte("[options]\naddons_path = /a,/b\n"), 0o644)
	st, err := svc.DetectEnterprise("app")
	if err != nil {
		t.Fatal(err)
	}
	if st.IsEnterprise {
		t.Fatalf("expected not enterprise %+v", st)
	}
}

func TestDetectEnterpriseWithPath(t *testing.T) {
	svc := newTestService(t)
	inst := &instance.Instance{Name: "app", Version: "16.0", DBName: "app", Port: 8069, LongpollPort: 8072}
	_ = svc.reg.Put(inst)
	p := inst.ResolvePaths(svc.rootFor(inst))
	_ = os.MkdirAll(filepath.Dir(p.Conf), 0o755)
	_ = os.WriteFile(p.Conf, []byte("[options]\naddons_path = /a,/opt/enterprise,/b\n"), 0o644)
	st, err := svc.DetectEnterprise("app")
	if err != nil {
		t.Fatal(err)
	}
	if !st.IsEnterprise || st.Path != "/opt/enterprise" {
		t.Fatalf("expected enterprise /opt/enterprise %+v", st)
	}
}

func TestLoadEnterprisePositions(t *testing.T) {
	svc := newTestService(t)
	inst := &instance.Instance{Name: "app", Version: "16.0", Branch: "16.0", DBName: "app", Port: 8069, LongpollPort: 8072}
	_ = svc.reg.Put(inst)
	p := inst.ResolvePaths(svc.rootFor(inst))
	_ = os.MkdirAll(filepath.Dir(p.Conf), 0o755)
	_ = os.WriteFile(p.Conf, []byte("[options]\naddons_path = /a,/b\n"), 0o644)
	// create fake enterprise dir to avoid git clone (will be used as dest)
	dest := filepath.Join(svc.rootFor(inst), "app", "enterprise")
	_ = os.MkdirAll(dest, 0o755)
	_ = os.WriteFile(filepath.Join(dest, ".git"), []byte{}, 0o644) // fake .git to trigger fetch branch
	// We cannot test full git clone without network, but test Detect after manual set
	inst.EnterprisePath = dest
	inst.EnterpriseRepo = "odoo/enterprise"
	inst.EnterpriseBranch = "16.0"
	_ = svc.reg.Put(inst)
	st, _ := svc.DetectEnterprise("app")
	if !st.IsEnterprise {
		t.Fatalf("expected enterprise after manual set")
	}
}
