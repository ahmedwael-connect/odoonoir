package db

import (
	"testing"

	"github.com/ahmed/odoonoir/internal/config"
)

func TestIsValidName(t *testing.T) {
	cases := []struct {
		name string
		ok   bool
	}{
		{"myapp", true},
		{"my_app_123", true},
		{"MyApp", false},
		{"123bad", false},
		{"bad-name", false},
		{"", false},
	}
	for _, c := range cases {
		err := IsValidName(c.name)
		if (err == nil) != c.ok {
			t.Fatalf("IsValidName(%q) ok=%v err=%v", c.name, c.ok, err)
		}
	}
}

func TestPgEscapeLiteral(t *testing.T) {
	if got := PgEscapeLiteral("a'b"); got != "a''b" {
		t.Fatalf("PgEscapeLiteral: %q", got)
	}
	if got := PgEscapeLiteral("noquote"); got != "noquote" {
		t.Fatalf("PgEscapeLiteral: %q", got)
	}
}

func TestSuggestDBName(t *testing.T) {
	if got := SuggestDBName("myapp"); got != "myapp" {
		t.Fatalf("SuggestDBName: %q", got)
	}
}

func TestIsGzipAndCustomExtra(t *testing.T) {
	if !isGzip([]byte{0x1f, 0x8b, 0x00}) {
		t.Fatal("isGzip should be true")
	}
	if isGzip([]byte{0x00, 0x00}) {
		t.Fatal("isGzip false")
	}
	if !isCustomDump([]byte("PGDMP extra")) {
		t.Fatal("isCustomDump")
	}
	if isCustomDump([]byte("plain")) {
		t.Fatal("isCustomDump false")
	}
}

func TestLooksLikeSQL(t *testing.T) {
	if !looksLikeSQL([]byte("-- comment")) {
		t.Fatal("looksLikeSQL --")
	}
	if !looksLikeSQL([]byte("CREATE TABLE")) {
		t.Fatal("CREATE")
	}
	if looksLikeSQL([]byte("binary\x00data")) {
		t.Fatal("binary false")
	}
}

func TestBatchDatabaseInfoInvalid(t *testing.T) {
	p := New(&config.Config{PostgresHost: "localhost", PostgresPort: 5432})
	_, err := p.BatchDatabaseInfo([]string{"bad-name"})
	if err == nil {
		t.Fatal("expected invalid name error")
	}
	_, err = p.BatchDatabaseInfo([]string{})
	if err != nil {
		t.Fatalf("empty should not error: %v", err)
	}
}
