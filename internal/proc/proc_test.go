package proc

import (
	"path/filepath"
	"testing"

	"github.com/ahmed/odoonoir/internal/instance"
)

func TestManagerServingDB(t *testing.T) {
	p := instance.Paths{Root: t.TempDir(), Conf: filepath.Join(t.TempDir(), "odoo.conf"), Log: "/tmp/x.log"}
	m := New(p, "/usr/bin/python3", p.Conf, 8072)
	if got := m.ServingDB(); got != "" {
		t.Fatalf("expected empty serving db, got %q", got)
	}
}

func TestDirOf(t *testing.T) {
	if got := dirOf("/a/b/c"); got != "/a/b" {
		t.Fatalf("dirOf %q", got)
	}
	if got := dirOf("noSlash"); got != "." {
		t.Fatalf("dirOf %q", got)
	}
}
