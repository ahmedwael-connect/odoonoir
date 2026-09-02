package installer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPatchRequirements15(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "requirements.txt")
	_ = os.WriteFile(f, []byte("gevent==21.8.0\nlxml==4.6.5\ncryptography==3.4.8\n"), 0o644)
	if err := PatchRequirements(f, "15.0", func(s string) {}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(f)
	s := string(data)
	if !strings.Contains(s, "gevent==22.10.2") || !strings.Contains(s, "lxml>=4.6.5") || !strings.Contains(s, "cryptography==38.0.4") {
		t.Fatalf("patch for 15 not applied: %s", s)
	}
	if _, err := os.Stat(f + ".orig"); err != nil {
		t.Fatal("backup missing")
	}
}

func TestPatchRequirementsNoop(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "requirements.txt")
	_ = os.WriteFile(f, []byte("requests==2.25.1\n"), 0o644)
	if err := PatchRequirements(f, "19.0", func(s string) {}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(f)
	if string(data) != "requests==2.25.1\n" {
		t.Fatalf("should not patch 19: %s", string(data))
	}
}

func TestBranchFor15(t *testing.T) {
	if got := BranchFor("15"); got != "15.0" {
		t.Fatalf("branch 15: %q", got)
	}
}
