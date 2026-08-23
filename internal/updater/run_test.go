package updater

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahmed/odoonoir/internal/instance"
)

func TestRefreshAddonsPreservesUserOrder(t *testing.T) {
	root := t.TempDir()
	core := filepath.Join(root, "src", "odoo")
	for _, d := range []string{
		filepath.Join(core, "addons"),
		filepath.Join(core, "odoo", "addons"),
		filepath.Join(root, "custom_addons"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	p := instance.Paths{
		Source: core,
		Addons: filepath.Join(root, "custom_addons"),
		Conf:   filepath.Join(root, "etc", "odoo.conf"),
	}
	userPath := filepath.Join(root, "srv", "tui-added")
	if err := os.MkdirAll(filepath.Join(root, "etc"), 0o755); err != nil {
		t.Fatal(err)
	}
	conf := "addons_path = " + filepath.Join(root, "src/odoo/addons") + "," +
		filepath.Join(root, "custom_addons") + "," + userPath + "\n"
	if err := os.WriteFile(p.Conf, []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := refreshAddonsPath(p); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(p.Conf)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	missing := filepath.Join(root, "src", "odoo", "odoo", "addons")
	if !strings.Contains(got, missing) {
		t.Fatalf("missing core entry %q after refresh:\n%s", missing, got)
	}
	if strings.Index(got, userPath) < 0 {
		t.Fatalf("user entry lost:\n%s", got)
	}
	if strings.Index(got, filepath.Join(root, "custom_addons")) > strings.Index(got, userPath) {
		t.Fatalf("user order not preserved:\n%s", got)
	}
	if strings.Index(got, userPath) > strings.Index(got, missing) {
		t.Fatalf("core entry should be appended after user entries:\n%s", got)
	}
}
