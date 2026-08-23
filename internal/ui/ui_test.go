package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestNewThemePlain(t *testing.T) {
	th := NewTheme(false)
	if got := th.Successf("x %d", 1); got != "✔ x 1" {
		t.Errorf("plain success = %q", got)
	}
	if got := th.StatusPill("running"); got != "running" {
		t.Errorf("plain pill = %q", got)
	}
	if got := th.Errorf("boom"); got != "✘ boom" {
		t.Errorf("plain error = %q", got)
	}
	if got := th.Warningf("w"); got != "⚠ w" {
		t.Errorf("plain warning = %q", got)
	}
}

func TestNewThemeColor(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	th := NewTheme(true)
	if got := th.Successf("x"); !strings.Contains(got, "✔ x") {
		t.Errorf("color success = %q", got)
	}
	if got := th.StatusPill("running"); !strings.Contains(got, "\x1b[") {
		t.Errorf("color pill has no ansi: %q", got)
	}
}

func TestTablePlain(t *testing.T) {
	th := NewTheme(false)
	got := th.Table([]string{"NAME", "STATUS"}, [][]string{{"myapp", "running"}}, WithTitle("instances"))
	want := "NAME   STATUS\nmyapp  running"
	if got != want {
		t.Errorf("plain table:\n%q\nwant:\n%q", got, want)
	}
}

func TestTableEmpty(t *testing.T) {
	th := NewTheme(false)
	if got := th.Table([]string{"A"}, nil, WithEmptyText("nothing here")); got != "nothing here" {
		t.Errorf("empty table = %q", got)
	}
}

func TestTableColor(t *testing.T) {
	th := NewTheme(true)
	got := th.Table([]string{"NAME", "VALUE"}, [][]string{{"myapp", "x"}}, WithTitle("t"))
	for _, want := range []string{"╭", "╮", "│", "NAME", "myapp"} {
		if !strings.Contains(got, want) {
			t.Errorf("color table missing %q:\n%s", want, got)
		}
	}
}

func TestKV(t *testing.T) {
	th := NewTheme(false)
	got := th.KV([][2]string{{"status", "running"}, {"port", "8069"}})
	want := "status : running\nport   : 8069"
	if got != want {
		t.Errorf("KV = %q want %q", got, want)
	}
}

func TestProgressNonTTY(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf, "working")
	if buf.String() != "" {
		t.Errorf("non-tty progress printed before any line: %q", buf.String())
	}
	p.Line("sub line")
	p.Done()
	want := "  sub line\n✔ working\n"
	if got := buf.String(); got != want {
		t.Errorf("output = %q want %q", got, want)
	}
}

func TestProgressFail(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf, "working")
	p.Fail()
	if !strings.Contains(buf.String(), "✘ working") {
		t.Errorf("fail output = %q", buf.String())
	}
}

func TestColorEnabledEnv(t *testing.T) {
	// piped writer: no color even without NO_COLOR
	if ColorEnabled(&bytes.Buffer{}, nil) {
		t.Error("pipe must be colorless")
	}
	// explicit override wins
	if !ColorEnabled(&bytes.Buffer{}, boolPtr(true)) {
		t.Error("override true must win")
	}
	if ColorEnabled(&bytes.Buffer{}, boolPtr(false)) {
		t.Error("override false must win")
	}
}

func boolPtr(b bool) *bool { return &b }

func TestPanelPlain(t *testing.T) {
	th := NewTheme(false)
	got := th.Panel("title", "body")
	if got != "title\nbody" {
		t.Errorf("plain panel = %q", got)
	}
}
