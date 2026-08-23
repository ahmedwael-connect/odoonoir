package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ahmed/odoonoir/internal/logmon"
)

func TestHumanDuration(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, "-"},
		{5 * time.Second, "5s"},
		{2*time.Minute + 3*time.Second, "2m3s"},
		{3*time.Hour + 5*time.Minute, "3h5m"},
	}
	for _, tc := range cases {
		if got := humanDuration(tc.in); got != tc.want {
			t.Fatalf("humanDuration(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestReadProcInfo(t *testing.T) {
	// wait at least one clock tick so a fresh process shows uptime > 0
	time.Sleep(30 * time.Millisecond)
	info := readProcInfo(os.Getpid())
	if info.pid != os.Getpid() {
		t.Fatalf("pid mismatch: %d", info.pid)
	}
	if info.uptime <= 0 {
		t.Fatalf("expected positive uptime for self, got %v", info.uptime)
	}
	if info.memKB <= 0 {
		t.Fatalf("expected positive VmRSS for self, got %d", info.memKB)
	}
}

func TestTailFrom(t *testing.T) {
	path := filepath.Join(t.TempDir(), "odoo.log")
	write := func(s string) {
		if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	append := func(s string) {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if _, err := f.WriteString(s); err != nil {
			t.Fatal(err)
		}
	}

	var offset int64
	write("1\n2\n3\n4\n5\n6\n")
	lines, rotated, err := logmon.TailFrom(path, 3, &offset)
	if err != nil {
		t.Fatal(err)
	}
	if rotated {
		t.Fatal("expected no rotation on first call")
	}
	if !reflect.DeepEqual(lines, []string{"4", "5", "6"}) {
		t.Fatalf("first tail: %v", lines)
	}

	append("7\n8\n")
	lines, rotated, err = logmon.TailFrom(path, 3, &offset)
	if err != nil {
		t.Fatal(err)
	}
	if rotated {
		t.Fatal("expected no rotation on incremental")
	}
	if !reflect.DeepEqual(lines, []string{"7", "8"}) {
		t.Fatalf("incremental tail: %v", lines)
	}

	// rotation: file replaced -> restart from the new end
	write("a\nb\n")
	lines, rotated, err = logmon.TailFrom(path, 3, &offset)
	if err != nil {
		t.Fatal(err)
	}
	if !rotated {
		t.Fatal("expected rotation=true")
	}
	if !reflect.DeepEqual(lines, []string{"a", "b"}) {
		t.Fatalf("rotation tail: %v", lines)
	}

	// no new data -> nothing
	lines, rotated, err = logmon.TailFrom(path, 3, &offset)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 0 {
		t.Fatalf("expected no lines, got %v", lines)
	}
}
