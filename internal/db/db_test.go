package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateDump(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name    string
		content []byte
		wantErr bool
	}{
		{"gzip", []byte{0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00}, false},
		{"custom", []byte("PGDMP\x00\x01"), false},
		{"plain sql", []byte("--\nCREATE TABLE x (i int);\n"), false},
		{"plain sql set", []byte("SET statement_timeout = 0;"), false},
		{"plain sql copy", []byte("COPY res_partner FROM stdin;"), false},
		{"empty", []byte{}, true},
		{"binary junk", []byte{0x00, 0x01, 0x02, 0x03, 0xff, 0xfe}, true},
		{"random text", []byte("this is not a dump file at all"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name+".dump")
			if err := os.WriteFile(path, tc.content, 0o644); err != nil {
				t.Fatal(err)
			}
			err := ValidateDump(path)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
	t.Run("missing", func(t *testing.T) {
		if err := ValidateDump(filepath.Join(dir, "nope.dump")); err == nil {
			t.Fatalf("expected error for missing file")
		}
	})
	t.Run("directory", func(t *testing.T) {
		if err := ValidateDump(dir); err == nil {
			t.Fatalf("expected error for directory")
		}
	})
}

func TestIsGzipAndCustom(t *testing.T) {
	if !isGzip([]byte{0x1f, 0x8b, 0x08}) {
		t.Fatalf("expected gzip magic to be detected")
	}
	if isGzip([]byte("PGDMP")) {
		t.Fatalf("custom dump must not be gzip")
	}
	if !isCustomDump([]byte("PGDMP\x00")) {
		t.Fatalf("expected custom dump magic to be detected")
	}
	if isCustomDump([]byte("-- SQL")) {
		t.Fatalf("plain SQL must not be custom")
	}
}
