package odoconf

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUnsetNonExistent(t *testing.T) {
	c := New()
	c.Set("http_port", "8069")
	c.Unset("nonexistent")
	if _, ok := c.Get("nonexistent"); ok {
		t.Error("nonexistent should not exist")
	}
	if v, _ := c.Get("http_port"); v != "8069" {
		t.Errorf("http_port changed: %q", v)
	}
}

func TestSetOverwrite(t *testing.T) {
	c := New()
	c.Set("http_port", "8069")
	c.Set("http_port", "8070")
	v, _ := c.Get("http_port")
	if v != "8070" {
		t.Errorf("expected 8070, got %q", v)
	}
}

func TestAddonsPathEmpty(t *testing.T) {
	c := New()
	paths, err := c.AddonsPath()
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 0 {
		t.Fatalf("expected empty, got %v", paths)
	}
}

func TestAddonsMoveInvalidIndices(t *testing.T) {
	c := New()
	_ = c.SetAddonsPath([]string{"/a", "/b", "/c"})
	// negative indices are clamped to 0 — should not error
	if err := c.AddonsMove(-1, 0); err != nil {
		t.Errorf("clamped move should succeed: %v", err)
	}
	if err := c.AddonsMove(0, -1); err != nil {
		t.Errorf("clamped move should succeed: %v", err)
	}
	// same index after clamping — should be no-op
	if err := c.AddonsMove(1, 1); err != nil {
		t.Errorf("same index should be no-op: %v", err)
	}
}

func TestAddonsRemoveAll(t *testing.T) {
	c := New()
	_ = c.SetAddonsPath([]string{"/a", "/b"})
	_ = c.AddonsRemove("/a")
	_ = c.AddonsRemove("/b")
	paths, _ := c.AddonsPath()
	if len(paths) != 0 {
		t.Fatalf("expected empty after removing all, got %v", paths)
	}
}

func TestCommentUncommentNonExistent(t *testing.T) {
	c := New()
	c.Set("http_port", "8069")
	// Comment non-existent key — should append
	if err := c.Comment("proxy_mode"); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get("proxy_mode"); ok {
		t.Error("proxy_mode should not be active after Comment")
	}
	// Uncomment it
	if err := c.Uncomment("proxy_mode"); err != nil {
		t.Fatal(err)
	}
}

func TestSaveCreatesBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "odoo.conf")
	if err := os.WriteFile(path, []byte("http_port = 8069\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	c.Set("http_port", "8070")
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	bak := path + ".bak"
	if _, err := os.Stat(bak); os.IsNotExist(err) {
		t.Error("backup file not created")
	}
}

func TestAllKeysIncludesCommented(t *testing.T) {
	c := New()
	c.Set("http_port", "8069")
	_ = c.Comment("workers")
	entries := c.AllKeys()
	foundHTTPPort := false
	foundWorkers := false
	for _, e := range entries {
		if e.Name == "http_port" && e.Active {
			foundHTTPPort = true
		}
		if e.Name == "workers" && !e.Active {
			foundWorkers = true
		}
	}
	if !foundHTTPPort {
		t.Error("http_port active not found")
	}
	if !foundWorkers {
		t.Error("workers inactive not found")
	}
}

func TestDirtyTracking(t *testing.T) {
	c := New()
	if c.Dirty() {
		t.Error("new conf should not be dirty")
	}
	c.Set("http_port", "8069")
	if !c.Dirty() {
		t.Error("should be dirty after Set")
	}
}

func TestLinesPreserveOrder(t *testing.T) {
	content := "a = 1\nb = 2\nc = 3\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "odoo.conf")
	_ = os.WriteFile(path, []byte(content), 0o644)
	c, _ := Load(path)
	lines := c.Lines()
	keys := make([]string, 0)
	for _, l := range lines {
		if l.Kind == KindKey {
			keys = append(keys, l.Key)
		}
	}
	if len(keys) != 3 || keys[0] != "a" || keys[1] != "b" || keys[2] != "c" {
		t.Errorf("order lost: %v", keys)
	}
}
