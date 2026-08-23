package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
)

// Theme holds the tool-wide color and style definitions. Every rendering
// method degrades to plain text when Color is false (piped output,
// NO_COLOR, --no-color), so the same content is produced either way.
type Theme struct {
	Color bool

	// status colors
	Running lipgloss.Style
	Stopped lipgloss.Style
	Warning lipgloss.Style
	Error   lipgloss.Style
	Info    lipgloss.Style
	Accent  lipgloss.Style
	Muted   lipgloss.Style

	// structural
	Header   lipgloss.Style
	BoxStyle lipgloss.Style
	BoxDim   lipgloss.Style
	Success  lipgloss.Style
	Hint     lipgloss.Style

	// modern kit
	Primary lipgloss.Style
	Dim     lipgloss.Style
}

const (
	CheckMark = "✔"
	CrossMark = "✘"
	WarnMark  = "⚠"
	Bullet    = "▸"
)

// ColorEnabled reports whether ANSI colors should be emitted for the given
// writer: it must be a terminal, TERM must not be "dumb" and NO_COLOR must
// be unset. When override is set it wins over everything else.
func ColorEnabled(w io.Writer, override *bool) bool {
	if override != nil {
		return *override
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(uintptr(f.Fd()))
}

// DefaultTheme builds a theme for the process stdout/stderr, honoring
// NO_COLOR and non-TTY output.
func DefaultTheme() *Theme {
	return NewTheme(ColorEnabled(os.Stdout, nil))
}

// NewTheme builds the standard odoonoir palette; color=false yields plain
// styles so every Render call is a no-op.
func NewTheme(color bool) *Theme {
	t := &Theme{Color: color}
	t.Running = t.st(lipgloss.Color("10"), true)
	t.Stopped = t.st(lipgloss.Color("9"), true)
	t.Warning = t.st(lipgloss.Color("11"), false)
	t.Error = t.st(lipgloss.Color("9"), true)
	t.Info = t.st(lipgloss.Color("12"), false)
	t.Accent = t.st(lipgloss.Color("13"), true)
	t.Muted = t.st(lipgloss.Color("8"), false)
	t.Header = t.st(lipgloss.Color("13"), true)
	t.BoxStyle = t.st(lipgloss.Color("13"), false).Border(lipgloss.RoundedBorder()).Padding(0, 1)
	t.BoxDim = t.st(lipgloss.Color("8"), false).Border(lipgloss.RoundedBorder()).Padding(0, 1)
	t.Success = t.st(lipgloss.Color("10"), true)
	t.Hint = t.st(lipgloss.Color("8"), false).Italic(true)
	t.Primary = t.st(lipgloss.Color("14"), true)
	t.Dim = t.st(lipgloss.Color("8"), false)
	return t
}

// st builds a colored style, or a plain style when colors are off.
func (t *Theme) st(c lipgloss.Color, bold bool) lipgloss.Style {
	if !t.Color {
		return lipgloss.NewStyle()
	}
	s := lipgloss.NewStyle().Foreground(c)
	if bold {
		s = s.Bold(true)
	}
	return s
}

// StatusStyle maps a runtime status to its colored label.
func (t *Theme) StatusStyle(status string) lipgloss.Style {
	switch status {
	case "running":
		return t.Running
	case "stopped":
		return t.Stopped
	case "unknown":
		return t.Warning
	}
	return t.Info
}

// SeverityStyle maps a log severity to a color.
func (t *Theme) SeverityStyle(sev string) lipgloss.Style {
	switch strings.ToUpper(sev) {
	case "CRITICAL":
		return t.Error
	case "ERROR":
		return t.Error
	case "WARNING":
		return t.Warning
	case "INFO":
		return t.Info
	case "DEBUG":
		return t.Muted
	}
	return t.Muted
}

// StatusPill renders a status as a colored chip (● running) or plain text.
func (t *Theme) StatusPill(status string) string {
	if !t.Color {
		return status
	}
	var dot string
	bg := lipgloss.Color("11")
	switch status {
	case "running":
		dot, bg = "●", lipgloss.Color("10")
	case "stopped":
		dot, bg = "○", lipgloss.Color("9")
	default:
		dot = "?"
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("0")).
		Background(bg).
		Padding(0, 1).
		Render(" " + dot + " " + status + " ")
}

// SeverityChip renders a log severity as a chip, or plain text.
func (t *Theme) SeverityChip(sev string) string {
	if !t.Color {
		return sev
	}
	bg := lipgloss.Color("8")
	switch strings.ToUpper(sev) {
	case "ERROR", "CRITICAL":
		bg = lipgloss.Color("9")
	case "WARNING":
		bg = lipgloss.Color("11")
	case "INFO":
		bg = lipgloss.Color("12")
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("0")).
		Background(bg).
		Padding(0, 1).
		Render(" " + sev + " ")
}

// Successf, Errorf, Warningf, Infof return a prefixed, styled line.
func (t *Theme) Successf(format string, args ...any) string {
	return t.Success.Render(CheckMark + " " + fmt.Sprintf(format, args...))
}

func (t *Theme) Errorf(format string, args ...any) string {
	return t.Error.Render(CrossMark + " " + fmt.Sprintf(format, args...))
}

func (t *Theme) Warningf(format string, args ...any) string {
	return t.Warning.Render(WarnMark + " " + fmt.Sprintf(format, args...))
}

func (t *Theme) Infof(format string, args ...any) string {
	return t.Primary.Render(Bullet + " " + fmt.Sprintf(format, args...))
}

func (t *Theme) Hintf(format string, args ...any) string {
	return t.Hint.Render(fmt.Sprintf(format, args...))
}

// Panel renders a titled box: ╭─ title ───╮ … ╰─────────╯.
func (t *Theme) Panel(title, body string) string {
	lines := strings.Split(body, "\n")
	if !t.Color {
		out := []string{}
		if title != "" {
			out = append(out, title)
		}
		return strings.Join(append(out, lines...), "\n")
	}
	w := 0
	for _, l := range lines {
		if n := lipgloss.Width(l); n > w {
			w = n
		}
	}
	titlePart := ""
	if title != "" {
		titlePart = " " + title + " "
	}
	top := "╭─" + titlePart + strings.Repeat("─", w+2-lipgloss.Width(titlePart)) + "╮"
	bottom := "╰" + strings.Repeat("─", w+2) + "╯"
	var b strings.Builder
	b.WriteString(t.Primary.Render(top))
	b.WriteString("\n")
	for _, l := range lines {
		b.WriteString(t.Primary.Render("│ "))
		b.WriteString(l)
		b.WriteString(t.Primary.Render(strings.Repeat(" ", w-lipgloss.Width(l)) + " │"))
		b.WriteString("\n")
	}
	b.WriteString(t.Primary.Render(bottom))
	return b.String()
}

// KV renders aligned key: value pairs (the "status" layout).
func (t *Theme) KV(rows [][2]string) string {
	w := 0
	for _, r := range rows {
		if n := len(r[0]); n > w {
			w = n
		}
	}
	var b strings.Builder
	for _, r := range rows {
		b.WriteString(t.Dim.Render(fmt.Sprintf("%-*s:", w+1, r[0])))
		b.WriteString(" ")
		b.WriteString(r[1])
		b.WriteString("\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// TableOpt configures Table rendering.
type TableOpt func(*tableSpec)

type tableSpec struct {
	title    string
	empty    string
	maxWidth int
	maxDepth int // how many rows to render before "..."
	lastCol  bool
	truncate bool
}

// WithTitle gives the table a panel-style title.
func WithTitle(title string) TableOpt {
	return func(s *tableSpec) { s.title = title }
}

// WithEmptyText is shown instead of rows when there are none.
func WithEmptyText(text string) TableOpt {
	return func(s *tableSpec) { s.empty = text }
}

// WithMaxWidth caps the rendered width (default: terminal width - 2).
func WithMaxWidth(w int) TableOpt {
	return func(s *tableSpec) { s.maxWidth = w }
}

// WithMaxDepth limits the number of rendered rows (ellipsis after).
func WithMaxDepth(n int) TableOpt {
	return func(s *tableSpec) { s.maxDepth = n }
}

// Table renders a bordered modern table (or aligned plain rows when colors
// are off / output is piped). Columns are sized to content, cells are
// truncated with an ellipsis when the terminal is narrow.
func (t *Theme) Table(headers []string, rows [][]string, opts ...TableOpt) string {
	spec := tableSpec{}
	for _, o := range opts {
		o(&spec)
	}
	if len(rows) == 0 {
		if spec.empty != "" {
			return spec.empty
		}
		return ""
	}
	if spec.maxWidth <= 0 {
		if w, _, err := term.GetSize(uintptr(os.Stdout.Fd())); err == nil && w > 0 {
			spec.maxWidth = w - 2
		}
	}
	if spec.maxWidth < 20 {
		spec.maxWidth = 20
	}
	spec.truncate = t.Color && spec.maxWidth > 0

	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = lipgloss.Width(h)
	}
	ansiCols := map[int]bool{}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && lipgloss.Width(cell) > widths[i] {
				widths[i] = lipgloss.Width(cell)
			}
			if i < len(widths) && strings.Contains(cell, "\x1b[") {
				ansiCols[i] = true
			}
		}
	}
	// shrink columns to fit the available width (proportionally, min 6).
	// Columns carrying pre-styled cells (pills/chips) keep their width.
	if spec.truncate {
		sep := 2 // two spaces between columns
		total := 0
		for _, w := range widths {
			total += w
		}
		overflow := total + sep*(len(widths)-1) - spec.maxWidth
		if overflow > 0 {
			remain := overflow
			for i := range widths {
				if widths[i] <= 6 || ansiCols[i] {
					continue
				}
				cut := overflow * widths[i] / total
				if widths[i]-cut < 6 {
					cut = widths[i] - 6
				}
				widths[i] -= cut
				remain -= cut
			}
			// trim the widest columns again if still overflowing
			for remain > 0 {
				big := -1
				for i, w := range widths {
					if w > 6 && !ansiCols[i] && (big < 0 || w > widths[big]) {
						big = i
					}
				}
				if big < 0 {
					break
				}
				widths[big]--
				remain--
			}
		}
	}

	body := t.renderTableBody(headers, rows, widths, spec)
	return body
}

func (t *Theme) renderTableBody(headers []string, rows [][]string, widths []int, spec tableSpec) string {
	sep := "  "
	if t.Color {
		sep = " │ "
		inner := sumWidths(widths) + sepWidth(sep)*(len(headers)-1)
		titlePart := ""
		if spec.title != "" {
			titlePart = "─ " + spec.title + " "
		}
		top := "╭" + titlePart + strings.Repeat("─", inner+2-lipgloss.Width(titlePart)) + "╮"
		bot := "╰" + strings.Repeat("─", inner+2) + "╯"
		head := make([]string, len(headers))
		for i, h := range headers {
			head[i] = t.Header.Bold(true).Underline(true).Render(pad(h, widths[i]))
		}
		var b strings.Builder
		b.WriteString(t.Primary.Render(top))
		b.WriteString("\n")
		b.WriteString(t.Primary.Render("│ "))
		b.WriteString(strings.Join(head, sep))
		b.WriteString(t.Primary.Render(" │"))
		b.WriteString("\n")
		limited := rows
		if spec.maxDepth > 0 && len(rows) > spec.maxDepth {
			limited = rows[:spec.maxDepth]
		}
		for _, row := range limited {
			cells := make([]string, len(row))
			for i, cell := range row {
				cells[i] = pad(ansi.Truncate(cell, widths[i], "…"), widths[i])
			}
			b.WriteString(t.Primary.Render("│ "))
			b.WriteString(strings.Join(cells, sep))
			b.WriteString(t.Primary.Render(" │"))
			b.WriteString("\n")
		}
		if spec.maxDepth > 0 && len(rows) > spec.maxDepth {
			b.WriteString(t.Dim.Render(fmt.Sprintf("│ … %d more │", len(rows)-spec.maxDepth)))
			b.WriteString("\n")
		}
		b.WriteString(t.Primary.Render(bot))
		return b.String()
	}

	var b strings.Builder
	for i, h := range headers {
		if i > 0 {
			b.WriteString(sep)
		}
		if i == len(headers)-1 {
			b.WriteString(h)
		} else {
			b.WriteString(pad(h, widths[i]))
		}
	}
	b.WriteString("\n")
	for _, row := range rows {
		for i, cell := range row {
			if i > 0 {
				b.WriteString(sep)
			}
			if i == len(row)-1 {
				b.WriteString(cell)
			} else {
				b.WriteString(pad(cell, widths[i]))
			}
		}
		b.WriteString("\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func sepWidth(sep string) int { return lipgloss.Width(sep) }

func sumWidths(w []int) int {
	s := 0
	for _, x := range w {
		s += x
	}
	return s
}

// pad pads s (ANSI-aware) to width with spaces.
func pad(s string, width int) string {
	if n := lipgloss.Width(s); n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return s
}

// ellipsize cuts s (plain text) to max runes keeping both ends.
func ellipsize(s string, max int) string {
	if len(s) <= max || max < 3 {
		return s
	}
	half := (max - 1) / 2
	return s[:half] + "…" + s[len(s)-(max-1-half):]
}

// Progress renders a spinner-backed step line on a TTY; on non-TTY output
// it stays silent until the step completes, then prints a check line.
type Progress struct {
	w      io.Writer
	label  string
	active bool
	mu     sync.Mutex
	stop   chan struct{}
	once   sync.Once
}

var frames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// NewProgress starts a step labeled label on writer w.
func NewProgress(w io.Writer, label string) *Progress {
	p := &Progress{w: w, label: label, stop: make(chan struct{})}
	p.active = term.IsTerminal(uintptr(os.Stdout.Fd()))
	if p.active {
		go p.animate()
	}
	return p
}

func (p *Progress) animate() {
	i := 0
	for {
		select {
		case <-p.stop:
			return
		default:
		}
		p.mu.Lock()
		fmt.Fprintf(p.w, "\r\033[K%s %s", frames[i%len(frames)], p.label)
		p.mu.Unlock()
		i++
		select {
		case <-p.stop:
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// Line prints a sub-line under the step (indented; cleared of the spinner).
func (p *Progress) Line(s string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.active {
		fmt.Fprintf(p.w, "\r\033[K  %s\n", s)
	} else {
		fmt.Fprintf(p.w, "  %s\n", s)
	}
}

// Done marks the step complete with a check.
func (p *Progress) Done() {
	p.once.Do(func() { close(p.stop) })
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.active {
		fmt.Fprintf(p.w, "\r\033[K%s %s\n", CheckMark, p.label)
	} else {
		fmt.Fprintf(p.w, "%s %s\n", CheckMark, p.label)
	}
}

// Fail marks the step failed with a cross.
func (p *Progress) Fail() {
	p.once.Do(func() { close(p.stop) })
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.active {
		fmt.Fprintf(p.w, "\r\033[K%s %s\n", CrossMark, p.label)
	} else {
		fmt.Fprintf(p.w, "%s %s\n", CrossMark, p.label)
	}
}

// Step runs fn under a spinner and marks the step done (or failed on error).
func Step(w io.Writer, label string, fn func() error) error {
	p := NewProgress(w, label)
	if err := fn(); err != nil {
		p.Fail()
		return err
	}
	p.Done()
	return nil
}

// Box renders a titled rounded box.
func (t *Theme) Box(title, content string) string {
	if title == "" {
		return t.BoxStyle.Render(content)
	}
	return t.BoxStyle.Render(title + "\n" + content)
}
