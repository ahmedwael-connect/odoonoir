package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ahmed/odoonoir/internal/ui"
)

// AddonsEditorDeps carries the state into the interactive addons_path editor.
type AddonsEditorDeps struct {
	Instance string
	Paths    []string
	Save     func(paths []string) error
}

// AddonsEditorResult reports how the editor session ended.
type AddonsEditorResult struct {
	Saved bool
}

type addonsEditMode int

const (
	modeList addonsEditMode = iota
	modeInput
)

type addonsEditor struct {
	deps     AddonsEditorDeps
	paths    []string
	cursor   int
	mode     addonsEditMode
	input    textinput.Model
	inputAt  int // -1 = append at end, >= 0 = replace index
	err      string
	saved    bool
	quitting bool
	theme    *ui.Theme
	width    int
	height   int
}

// RunAddonsEditor runs the interactive addons_path list editor.
func RunAddonsEditor(deps AddonsEditorDeps) (AddonsEditorResult, error) {
	if deps.Save == nil {
		return AddonsEditorResult{}, fmt.Errorf("editor misconfigured: no save function")
	}
	ti := textinput.New()
	ti.Placeholder = "path to add (e.g. /srv/modules)"
	ti.CharLimit = 500
	m := &addonsEditor{
		deps:    deps,
		paths:   append([]string(nil), deps.Paths...),
		input:   ti,
		inputAt: -1,
		theme:   ui.DefaultTheme(),
		width:   80,
		height:  24,
	}
	p := tea.NewProgram(m, tea.WithInputTTY())
	model, err := p.Run()
	if err != nil {
		return AddonsEditorResult{}, err
	}
	final := model.(*addonsEditor)
	return AddonsEditorResult{Saved: final.saved}, nil
}

func (m *addonsEditor) Init() tea.Cmd {
	return nil
}

func (m *addonsEditor) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			m.quitting = true
			return m, tea.Quit
		}
		if m.mode == modeInput {
			return m.updateInput(msg)
		}
		return m.updateList(msg)
	}
	return m, nil
}

func (m *addonsEditor) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		value := strings.TrimSpace(m.input.Value())
		if value != "" {
			if m.inputAt >= 0 && m.inputAt < len(m.paths) {
				m.paths[m.inputAt] = value
			} else {
				m.paths = append(m.paths, value)
			}
		}
		m.mode = modeList
		m.input.SetValue("")
		m.input.Blur()
	case tea.KeyEsc:
		m.mode = modeList
		m.input.SetValue("")
		m.input.Blur()
	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *addonsEditor) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp, tea.KeyCtrlP:
		if m.cursor > 0 {
			m.cursor--
		}
	case tea.KeyDown, tea.KeyCtrlN:
		if m.cursor < len(m.paths)-1 {
			m.cursor++
		}
	case tea.KeyRunes:
		switch msg.String() {
		case "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "j":
			if m.cursor < len(m.paths)-1 {
				m.cursor++
			}
		case "u":
			if m.cursor > 0 {
				m.swap(m.cursor, m.cursor-1)
			}
		case "d":
			if m.cursor < len(m.paths)-1 {
				m.swap(m.cursor, m.cursor+1)
			}
		case "a":
			m.inputAt = -1
			m.input.Placeholder = "path to add (e.g. /srv/modules)"
			m.input.SetValue("")
			m.input.Focus()
			m.mode = modeInput
		case "e":
			if len(m.paths) > 0 {
				m.inputAt = m.cursor
				m.input.Placeholder = "edit path"
				m.input.SetValue(m.paths[m.cursor])
				m.input.Focus()
				m.mode = modeInput
			}
		case "x":
			if len(m.paths) > 0 {
				m.paths = append(m.paths[:m.cursor], m.paths[m.cursor+1:]...)
				if m.cursor >= len(m.paths) {
					m.cursor = len(m.paths) - 1
				}
			}
		case "q":
			return m.saveAndQuit()
		}
	case tea.KeyEsc:
		return m.saveAndQuit()
	}
	return m, nil
}

func (m *addonsEditor) saveAndQuit() (tea.Model, tea.Cmd) {
	if err := m.deps.Save(m.paths); err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.saved = true
	m.quitting = true
	return m, tea.Quit
}

func (m *addonsEditor) swap(a, b int) {
	m.paths[a], m.paths[b] = m.paths[b], m.paths[a]
	m.cursor = b
}

func (m *addonsEditor) View() string {
	var b strings.Builder

	title := m.theme.Accent.Render(fmt.Sprintf("addons_path — %s", m.deps.Instance))
	b.WriteString(title)
	b.WriteString("\n\n")

	for i, p := range m.paths {
		num := fmt.Sprintf("%2d ", i+1)
		if i == m.cursor {
			b.WriteString(m.theme.Accent.Render(num + p))
		} else {
			b.WriteString(num + p)
		}
		b.WriteString("\n")
	}
	if len(m.paths) == 0 {
		b.WriteString(m.theme.Muted.Render("  (empty — press a to add a path)"))
		b.WriteString("\n")
	}

	b.WriteString("\n")

	if m.mode == modeInput {
		b.WriteString(m.theme.Muted.Render("enter: confirm    esc: cancel"))
		b.WriteString("\n")
		b.WriteString(m.input.View())
		b.WriteString("\n")
	} else {
		if m.err != "" {
			b.WriteString(m.theme.Error.Render(m.err))
			b.WriteString("\n")
		}
		b.WriteString(m.theme.Muted.Render("↑/k ↓/j: select   u/d: move   a: add   e: edit   x: remove   q: save & quit   ctrl+c: abort"))
		b.WriteString("\n")
	}
	return b.String()
}
