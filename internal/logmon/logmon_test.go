package logmon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeLog(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "odoo.log")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestScanDetectsPortConflict(t *testing.T) {
	log := writeLog(t, "INFO module loaded\nERROR odoo.service.server: Address already in use\n")
	issues, err := Scan(log)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, it := range issues {
		if it.Severity == "error" && strings.Contains(it.Message, "already in use") {
			found = true
		}
	}
	if !found {
		t.Errorf("port conflict not detected: %+v", issues)
	}
}

func TestScanDetectsDbAuth(t *testing.T) {
	log := writeLog(t, "ERROR FATAL:  password authentication failed for user \"odoo\"\n")
	issues, _ := Scan(log)
	if len(issues) == 0 {
		t.Fatal("expected issues")
	}
	if issues[0].Hint == "" {
		t.Error("expected a fix hint")
	}
}

func TestScanDetectsMissingModule(t *testing.T) {
	log := writeLog(t, "Module `sale_ext` was not found on addons path\n")
	issues, _ := Scan(log)
	found := false
	for _, it := range issues {
		if strings.Contains(it.Message, "missing") {
			found = true
		}
	}
	if !found {
		t.Errorf("missing module not detected: %+v", issues)
	}
}

func TestScanMissingFile(t *testing.T) {
	issues, err := Scan("/nonexistent/odoo.log")
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Errorf("expected no issues for missing log, got %d", len(issues))
	}
}

func TestDedupe(t *testing.T) {
	log := writeLog(t, strings.Repeat("ERROR odoo.service.server: Address already in use\n", 20))
	issues, _ := Scan(log)
	if len(issues) != 20 {
		t.Fatalf("expected 20 raw issues, got %d", len(issues))
	}
	deduped := Dedupe(issues)
	if len(deduped) >= 20 {
		t.Errorf("dedupe ineffective: %d", len(deduped))
	}
}

func TestErrorsOnly(t *testing.T) {
	log := writeLog(t, "WARNING deprecation detected\nERROR FATAL: role does not exist\n")
	issues, _ := Scan(log)
	only := ErrorsOnly(issues)
	for _, it := range only {
		if it.Severity != "error" {
			t.Errorf("non-error survived filter: %+v", it)
		}
	}
}

func TestTail(t *testing.T) {
	log := writeLog(t, "a\nb\nc\nd\ne\n")
	lines, err := Tail(log, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 || lines[0] != "d" || lines[1] != "e" {
		t.Errorf("tail = %v", lines)
	}
}
