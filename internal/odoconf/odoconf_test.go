package odoconf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "odoo.conf")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestNewWriteLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "odoo.conf")

	c := New()
	c.Set("addons_path", "/a,/b")
	c.Set("http_port", "8080")
	c.Set("db_password", "secret")
	if err := c.Write(path); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := loaded.Get("addons_path"); v != "/a,/b" {
		t.Errorf("addons_path = %q", v)
	}
	if v, _ := loaded.Get("http_port"); v != "8080" {
		t.Errorf("http_port = %q", v)
	}
	if v, _ := loaded.Get("db_password"); v != "secret" {
		t.Errorf("db_password = %q", v)
	}
}

func TestLosslessRoundtrip(t *testing.T) {
	content := `# odoonoir generated odoo.conf
; a semicolon comment

[options]
addons_path = /a,/b
data_dir = /data

# the port people type
http_port = 8069
logfile = /tmp/x.log
`
	path := write(t, content)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	c.Set("http_port", "8090")
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	got := read(t, path)
	want := strings.Replace(content, "http_port = 8069", "http_port = 8090", 1)
	if got != want {
		t.Errorf("round-trip changed unrelated content:\n%s", got)
	}
	if !strings.Contains(got, "# the port people type") {
		t.Error("comment lost")
	}
	if !strings.Contains(got, "; a semicolon comment") {
		t.Error("semicolon comment lost")
	}
	if !strings.Contains(got, "\n\n") {
		t.Error("blank line lost")
	}
}

func TestGetLastWins(t *testing.T) {
	path := write(t, "http_port = 8069\nhttp_port = 8070\n")
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := c.Get("http_port"); v != "8070" {
		t.Errorf("http_port = %q, want 8070", v)
	}
}

func TestKeysFileOrder(t *testing.T) {
	path := write(t, "b = 1\na = 2\nhttp_port = 3\n")
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	keys := c.Keys()
	if len(keys) != 3 || keys[0] != "b" || keys[1] != "a" || keys[2] != "http_port" {
		t.Errorf("Keys = %v", keys)
	}
}

func TestUnknownLinesPreserved(t *testing.T) {
	content := "[options]\nsome weird line with no equals\nalso [not a section\nhttp_port = 8069\n"
	path := write(t, content)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	c.Set("http_port", "8070")
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	got := read(t, path)
	for _, want := range []string{"some weird line with no equals", "also [not a section", "http_port = 8070"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestCommentUncomment(t *testing.T) {
	path := write(t, "# keep this\nworkers = 2\nlogfile = /tmp/x.log\n")
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Comment("workers"); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get("workers"); ok {
		t.Error("workers still active after Comment")
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	got := read(t, path)
	if !strings.Contains(got, "# workers = 2") {
		t.Errorf("workers not commented:\n%s", got)
	}
	if !strings.Contains(got, "# keep this") {
		t.Error("attached comment lost")
	}

	c2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := c2.Uncomment("workers"); err != nil {
		t.Fatal(err)
	}
	if v, ok := c2.Get("workers"); !ok || v != "2" {
		t.Errorf("workers = %q, %v", v, ok)
	}
	if err := c2.Save(); err != nil {
		t.Fatal(err)
	}
	got = read(t, path)
	if !strings.Contains(got, "workers = 2") {
		t.Errorf("workers not restored:\n%s", got)
	}
}

func TestCommentAbsentKeyAppendsDocumented(t *testing.T) {
	c := New()
	if err := c.Comment("proxy_mode"); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get("proxy_mode"); ok {
		t.Error("proxy_mode should be inactive")
	}
	found := false
	for _, l := range c.Lines() {
		if l.Kind == KindCommentedKey && l.Key == "proxy_mode" {
			found = true
		}
	}
	if !found {
		t.Error("commented line not appended")
	}
}

func TestAllKeysKeepsCommentedValues(t *testing.T) {
	path := write(t, "workers = 2\nhttp_port = 8069\n")
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Comment("workers"); err != nil {
		t.Fatal(err)
	}
	entries := c.AllKeys()
	if len(entries) != 2 {
		t.Fatalf("AllKeys = %+v", entries)
	}
	for _, e := range entries {
		if e.Name == "workers" && (e.Active || e.Value != "2") {
			t.Errorf("workers entry = %+v, want inactive with value 2", e)
		}
	}
}

func TestUnsetRemovesAttachedComments(t *testing.T) {
	content := "http_port = 8069\n\n# the workers count\nworkers = 2\nlogfile = /tmp/x.log\n"
	path := write(t, content)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	c.Unset("workers")
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	got := read(t, path)
	if strings.Contains(got, "workers") {
		t.Errorf("workers block not removed:\n%s", got)
	}
	if !strings.Contains(got, "http_port = 8069") || !strings.Contains(got, "logfile") {
		t.Error("other keys damaged")
	}
}

func TestAddonsPathOps(t *testing.T) {
	c := New()
	if err := c.SetAddonsPath([]string{"/a", "/b", "/c"}); err != nil {
		t.Fatal(err)
	}
	got, err := c.AddonsPath()
	if err != nil || len(got) != 3 || got[0] != "/a" || got[2] != "/c" {
		t.Fatalf("AddonsPath = %v, %v", got, err)
	}

	if err := c.AddonsAdd("/d", -1); err != nil {
		t.Fatal(err)
	}
	got, _ = c.AddonsPath()
	if got[len(got)-1] != "/d" {
		t.Errorf("append failed: %v", got)
	}
	if err := c.AddonsAdd("/d", -1); err == nil {
		t.Error("duplicate add should fail")
	}

	if err := c.AddonsMove(3, 0); err != nil { // /d to front
		t.Fatal(err)
	}
	got, _ = c.AddonsPath()
	want := []string{"/d", "/a", "/b", "/c"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("after move: %v, want %v", got, want)
		}
	}

	if err := c.AddonsRemove("/b"); err != nil {
		t.Fatal(err)
	}
	got, _ = c.AddonsPath()
	if len(got) != 3 {
		t.Fatalf("after remove: %v", got)
	}
	for _, p := range got {
		if p == "/b" {
			t.Error("/b still present")
		}
	}
	if err := c.AddonsRemove("/nope"); err == nil {
		t.Error("removing an absent path should fail")
	}
}

func TestAddonsMoveBounds(t *testing.T) {
	c := New()
	_ = c.SetAddonsPath([]string{"/a", "/b"})
	if err := c.AddonsMove(5, 0); err != nil {
		t.Fatalf("clamped move should succeed: %v", err)
	}
	got, _ := c.AddonsPath()
	if got[0] != "/b" {
		t.Errorf("got %v", got)
	}
}

func TestSaveIdempotent(t *testing.T) {
	path := write(t, "http_port = 8069\n")
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	if c.Dirty() {
		t.Error("Save marked the doc dirty")
	}
	if got := read(t, path); got != "http_port = 8069\n" {
		t.Errorf("unexpected rewrite: %q", got)
	}
}

func TestBackupBeforeSave(t *testing.T) {
	path := write(t, "http_port = 8069\n")
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	c.Set("http_port", "8070")
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	bak, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if string(bak) != "http_port = 8069\n" {
		t.Errorf("backup = %q", string(bak))
	}
}

func TestOtherSectionsUntouched(t *testing.T) {
	content := "[options]\nhttp_port = 8069\n\n[websocket]\nport = 8081\n"
	path := write(t, content)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get("port"); ok {
		t.Error("Get leaked into the websocket section")
	}
	c.Set("http_port", "8070")
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	got := read(t, path)
	if !strings.Contains(got, "[websocket]\nport = 8081\n") {
		t.Errorf("websocket section damaged:\n%s", got)
	}
}
