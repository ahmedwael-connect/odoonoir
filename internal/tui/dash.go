package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ahmed/odoonoir/internal/config"
	"github.com/ahmed/odoonoir/internal/db"
	"github.com/ahmed/odoonoir/internal/installer"
	"github.com/ahmed/odoonoir/internal/instance"
	"github.com/ahmed/odoonoir/internal/logmon"
	"github.com/ahmed/odoonoir/internal/notify"
	"github.com/ahmed/odoonoir/internal/odoconf"
	"github.com/ahmed/odoonoir/internal/proc"
	"github.com/ahmed/odoonoir/internal/ui"
	"github.com/ahmed/odoonoir/internal/updater"
)

// Deps carries the shared runtime dependencies into the dashboard.
type Deps struct {
	Cfg *config.Config
	Reg *instance.Registry
	Th  *ui.Theme
}

// Views
const (
	viewList   = "list"
	viewDetail = "detail"
	viewLogs   = "logs"
	viewUpdate = "update"
	viewConf   = "conf"
	viewDoctor = "doctor"
	viewHelp   = "help"
)

type model struct {
	deps     Deps
	insts    []*instance.Instance
	status   map[string]instance.Status
	pids     map[string]int
	cursor   int
	view     string
	spinner  spinner.Model
	table    table.Model
	err      string
	notice   string
	logErr   string
	logLines []string
	logPath  string
	paused   bool
	logErrs  bool
	conf     confData
	doctor   doctorData
	db       dbData
	update   *updateState
}

// dbData holds the async detail facts (db size, latest backup).
type dbData struct {
	size   int64
	backup string
	dbs    []dbRow
}

// dbRow is one database served by the instance.
type dbRow struct {
	name string
	size int64
	init bool
	pri  bool
}

// confData holds a loaded odoo.conf for the conf view.
type confData struct {
	path  string
	keys  []string
	lines []string
	err   string
}

// doctorData holds the log scan results for the doctor view.
type doctorData struct {
	lines []string
	err   string
}

// Run launches the dashboard and blocks until quit.
func Run(d Deps) error {
	p := tea.NewProgram(newModel(d), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func newModel(d Deps) model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("13"))
	cols := []table.Column{
		{Title: "NAME", Width: 16},
		{Title: "VERSION", Width: 9},
		{Title: "STATUS", Width: 18},
		{Title: "PORT", Width: 6},
		{Title: "DATABASE", Width: 16},
	}
	m := model{
		deps:    d,
		status:  map[string]instance.Status{},
		pids:    map[string]int{},
		view:    viewList,
		spinner: s,
		table:   table.New(table.WithColumns(cols), table.WithFocused(true)),
	}
	m.table.SetStyles(table.Styles{
		Header:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13")).Padding(0, 1),
		Cell:     lipgloss.NewStyle().Padding(0, 1),
		Selected: lipgloss.NewStyle().Background(lipgloss.Color("5")).Foreground(lipgloss.Color("0")),
	})
	return m
}

// tickMsg triggers periodic refresh.
type tickMsg time.Time

type instancesMsg []*instance.Instance
type statusMsg struct {
	status map[string]instance.Status
	pids   map[string]int
}
type opDoneMsg struct {
	op, name string
	err      error
}
type logsMsg struct {
	lines []string
	path  string
}

type confMsg confData

type doctorMsg doctorData

type dbInfoMsg dbData

// updateState is the shared, mutex-guarded state of a running update.
type updateState struct {
	mu      sync.Mutex
	running bool
	curStep int
	err     error
	lines   []string
}

func (s *updateState) snapshot() (running bool, curStep int, err error, lines []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running, s.curStep, s.err, append([]string(nil), s.lines...)
}

func (s *updateState) isRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		refreshInstances(m.deps),
		refreshStatuses(m.deps),
		tickCmd(2*time.Second),
		m.spinner.Tick,
	)
}

func tickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func refreshInstances(d Deps) tea.Cmd {
	return func() tea.Msg {
		insts, err := d.Reg.All()
		if err != nil {
			return instancesMsg{}
		}
		return instancesMsg(insts)
	}
}

func refreshStatuses(d Deps) tea.Cmd {
	return func() tea.Msg {
		insts, err := d.Reg.All()
		if err != nil {
			return statusMsg{}
		}
		out := map[string]instance.Status{}
		pids := map[string]int{}
		for _, inst := range insts {
			p := inst.ResolvePaths(instRoot(d, inst))
			mgr := proc.New(p, installer.PythonFor(inst, p), p.Conf)
			st, pid, err := mgr.Status()
			if err != nil {
				continue
			}
			out[inst.Name] = st
			pids[inst.Name] = pid
		}
		return statusMsg{status: out, pids: pids}
	}
}

func instRoot(d Deps, inst *instance.Instance) string {
	if inst.Root != "" {
		return inst.Root
	}
	return d.Cfg.InstancesDir()
}

// tailLog tails the selected instance's log file for the live view.
func tailLog(d Deps, inst *instance.Instance) tea.Cmd {
	return func() tea.Msg {
		p := inst.ResolvePaths(instRoot(d, inst))
		lines, _ := logmon.Tail(p.Log, 300)
		return logsMsg{lines: lines, path: p.Log}
	}
}

// loadConf reads the instance's odoo.conf for the conf view.
func loadConf(d Deps, inst *instance.Instance) tea.Cmd {
	return func() tea.Msg {
		p := inst.ResolvePaths(instRoot(d, inst))
		c, err := odoconf.Load(p.Conf)
		if err != nil {
			return confMsg{path: p.Conf, err: "cannot read conf: " + err.Error()}
		}
		keys := c.Keys()
		sort.Strings(keys)
		lines := make([]string, 0, len(keys))
		for _, k := range keys {
			v, _ := c.Get(k)
			if k == "db_password" && v != "" {
				v = "********"
			}
			lines = append(lines, fmt.Sprintf("%s = %s", k, v))
		}
		return confMsg{path: p.Conf, keys: keys, lines: lines}
	}
}

// runDoctor scans the instance log for known issues.
func runDoctor(d Deps, inst *instance.Instance) tea.Cmd {
	return func() tea.Msg {
		p := inst.ResolvePaths(instRoot(d, inst))
		if _, err := os.Stat(p.Log); os.IsNotExist(err) {
			return doctorMsg{err: "no log yet — the instance has never been started"}
		}
		issues, err := logmon.Scan(p.Log)
		if err != nil {
			return doctorMsg{err: "scan failed: " + err.Error()}
		}
		issues = logmon.Dedupe(issues)
		if len(issues) == 0 {
			return doctorMsg{lines: []string{d.Th.Success.Render("no issues detected — the log looks clean")}}
		}
		lines := logmon.Format(issues)
		colored := make([]string, 0, len(lines))
		for _, l := range lines {
			colored = append(colored, colorIssueLine(d.Th, l))
		}
		return doctorMsg{lines: colored}
	}
}

// loadDbInfo gathers db sizes and the latest backup for the detail view.
func loadDbInfo(d Deps, inst *instance.Instance) tea.Cmd {
	return func() tea.Msg {
		var info dbData
		pg := db.New(d.Cfg)
		for _, name := range inst.AllDBs() {
			row := dbRow{name: name, pri: name == inst.DBName}
			if size, err := pg.DatabaseSize(name); err == nil {
				row.size = size
			}
			if ok, err := pg.IsInitialized(name); err == nil {
				row.init = ok
			}
			if name == inst.DBName {
				info.size = row.size
			}
			info.dbs = append(info.dbs, row)
		}
		p := inst.ResolvePaths(instRoot(d, inst))
		matches, _ := filepath.Glob(filepath.Join(p.Root, "backups", "*"))
		if len(matches) > 0 {
			var newest string
			var newestMod time.Time
			for _, mf := range matches {
				if fi, err := os.Stat(mf); err == nil && fi.ModTime().After(newestMod) {
					newestMod = fi.ModTime()
					newest = mf
				}
			}
			if newest != "" {
				if fi, err := os.Stat(newest); err == nil {
					info.backup = fmt.Sprintf("%s (%s, %s)",
						filepath.Base(newest), humanSize(fi.Size()), newestMod.Format("2006-01-02 15:04"))
				}
			}
		}
		return dbInfoMsg(info)
	}
}

// openBrowser opens the instance URL in the default browser.
func openBrowser(d Deps, inst *instance.Instance) tea.Cmd {
	return func() tea.Msg {
		if _, err := exec.LookPath("xdg-open"); err != nil {
			return opDoneMsg{op: "browser", name: inst.Name, err: fmt.Errorf("xdg-open not found")}
		}
		err := exec.Command("xdg-open", fmt.Sprintf("http://localhost:%d", inst.Port)).Run()
		return opDoneMsg{op: "browser", name: inst.Name, err: err}
	}
}

// startUpdate runs the update pipeline in a goroutine, streaming progress
// into the shared state.
func startUpdate(d Deps, inst *instance.Instance, st *updateState) tea.Cmd {
	return func() tea.Msg {
		st.mu.Lock()
		st.running = true
		st.curStep = -1
		st.mu.Unlock()
		err := updater.Run(context.Background(), d.Cfg, inst, updater.Options{}, func(p updater.Progress) {
			st.mu.Lock()
			defer st.mu.Unlock()
			switch p.Kind {
			case updater.StepStart:
				st.curStep = p.Index
			case updater.StepDone:
				st.curStep = -1
			case updater.Line:
				st.lines = append(st.lines, p.Line)
			}
		})
		st.mu.Lock()
		st.running = false
		st.err = err
		st.mu.Unlock()
		if err != nil {
			_ = notify.Notify(d.Cfg, "update.failed", "update "+inst.Name+" failed", err.Error())
		} else {
			_ = notify.Notify(d.Cfg, "update.done", "update "+inst.Name+" complete", "restart to apply")
		}
		return updateDoneMsg{}
	}
}

type updateDoneMsg struct{}

// updateCmd returns a 500ms re-render tick while the update view is active.
func updateCmd() tea.Cmd { return tickCmd(500 * time.Millisecond) }

// actionMsg runs an instance action in a goroutine.
func runAction(d Deps, inst *instance.Instance, op string) tea.Cmd {
	return func() tea.Msg {
		p := inst.ResolvePaths(instRoot(d, inst))
		var err error
		switch op {
		case "start":
			mgr := proc.New(p, installer.PythonFor(inst, p), p.Conf)
			err = mgr.Start("")
		case "stop":
			mgr := proc.New(p, installer.PythonFor(inst, p), p.Conf)
			err = mgr.Stop()
		case "restart":
			mgr := proc.New(p, installer.PythonFor(inst, p), p.Conf)
			err = mgr.Restart()
		case "backup":
			pg := db.New(d.Cfg)
			stamp := time.Now().Format("20060102_150405")
			out := filepath.Join(p.Root, "backups", inst.DBName+"_"+stamp+".dump")
			err = pg.Backup(context.Background(), inst.DBName, out, true, false)
			if err == nil {
				_ = notify.Notify(d.Cfg, "backup.done", "backup "+inst.Name+" complete", "dumped to "+out)
			}
		}
		return opDoneMsg{op: op, name: inst.Name, err: err}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	inListOrDetail := m.view == viewList || m.view == viewDetail
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "?":
			if m.view != viewHelp {
				m.view = viewHelp
				return m, nil
			}
		case "esc":
			if m.view == viewHelp {
				m.view = viewList
				return m, nil
			}
			if m.view != viewList {
				m.view = viewList
				m.err = ""
				m.logErr = ""
				m.notice = ""
				return m, nil
			}
		case "enter":
			if m.view == viewList && len(m.insts) > 0 {
				m.view = viewDetail
				m.err = ""
				m.notice = ""
				return m, tea.Batch(loadDbInfo(m.deps, m.insts[m.cursor]))
			}
		case "d":
			if m.view == viewDetail && len(m.insts) > 0 {
				m.view = viewDoctor
				m.err = ""
				return m, runDoctor(m.deps, m.insts[m.cursor])
			}
		case "c":
			if m.view == viewDetail && len(m.insts) > 0 {
				m.view = viewConf
				m.err = ""
				return m, loadConf(m.deps, m.insts[m.cursor])
			}
		case "o":
			if m.view == viewDetail && len(m.insts) > 0 {
				m.notice = ""
				return m, openBrowser(m.deps, m.insts[m.cursor])
			}
		case "l":
			if inListOrDetail && len(m.insts) > 0 {
				m.view = viewLogs
				m.logErr = ""
				return m, tea.Batch(
					tailLog(m.deps, m.insts[m.cursor]),
					tickCmd(time.Second),
				)
			}
		case "e":
			if m.view == viewLogs {
				m.logErrs = !m.logErrs
			}
		case "p":
			if m.view == viewLogs {
				m.paused = !m.paused
			}
		case "s", "S":
			if inListOrDetail && len(m.insts) > 0 {
				m.err = ""
				m.notice = ""
				return m, runAction(m.deps, m.insts[m.cursor], "start")
			}
		case "x":
			if inListOrDetail && len(m.insts) > 0 {
				m.notice = ""
				return m, runAction(m.deps, m.insts[m.cursor], "stop")
			}
		case "r":
			if inListOrDetail && len(m.insts) > 0 {
				m.notice = ""
				return m, runAction(m.deps, m.insts[m.cursor], "restart")
			}
		case "u":
			if inListOrDetail && len(m.insts) > 0 {
				inst := m.insts[m.cursor]
				if m.update == nil || !m.update.isRunning() {
					st := &updateState{}
					m.update = st
					m.view = viewUpdate
					return m, tea.Batch(
						startUpdate(m.deps, inst, st),
						updateCmd(),
					)
				}
			}
		case "b":
			if inListOrDetail && len(m.insts) > 0 {
				m.notice = ""
				return m, runAction(m.deps, m.insts[m.cursor], "backup")
			}
		case "down", "j":
			if m.view == viewList && len(m.insts) > 0 {
				m.cursor = (m.cursor + 1) % len(m.insts)
			}
		case "up", "k":
			if m.view == viewList && len(m.insts) > 0 {
				m.cursor = (m.cursor - 1 + len(m.insts)) % len(m.insts)
			}
		}
	case instancesMsg:
		m.insts = msg
		if m.cursor >= len(m.insts) && len(m.insts) > 0 {
			m.cursor = len(m.insts) - 1
		}
		m.renderTable()
	case statusMsg:
		m.status = msg.status
		m.pids = msg.pids
		m.renderTable()
	case tickMsg:
		if m.view == viewLogs && !m.paused && len(m.insts) > 0 {
			cmds = append(cmds, tailLog(m.deps, m.insts[m.cursor]), tickCmd(time.Second))
		} else if m.view == viewUpdate && m.update != nil {
			cmds = append(cmds, updateCmd())
		} else {
			cmds = append(cmds, refreshStatuses(m.deps), tickCmd(2*time.Second))
		}
	case logsMsg:
		m.logLines = msg.lines
		m.logPath = msg.path
		m.logErr = ""
	case confMsg:
		m.conf = confData(msg)
	case doctorMsg:
		m.doctor = doctorData(msg)
	case dbInfoMsg:
		m.db = dbData(msg)
	case updateDoneMsg:
		// state is read directly from m.update; nothing to do
	case opDoneMsg:
		if msg.err != nil {
			m.err = msg.op + " " + msg.name + ": " + msg.err.Error()
			m.notice = ""
		} else {
			m.err = ""
			m.notice = m.deps.Th.Success.Render("  " + msg.op + " " + msg.name + " — done")
		}
		cmds = append(cmds, refreshStatuses(m.deps))
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}
	// keep the table cursor in sync
	if len(m.insts) > 0 && m.cursor < len(m.insts) {
		m.table.SetCursor(m.cursor)
	}
	return m, tea.Batch(cmds...)
}

// renderTable rebuilds the table rows from the current instance state.
func (m *model) renderTable() {
	rows := []table.Row{}
	for _, inst := range m.insts {
		st := m.status[inst.Name]
		statusStr := string(st)
		if st == instance.StatusRunning {
			if pid, ok := m.pids[inst.Name]; ok && pid > 0 {
				statusStr = fmt.Sprintf("running (pid %d)", pid)
			}
		}
		rows = append(rows, table.Row{
			inst.Name, inst.Version, statusStr,
			fmt.Sprint(inst.Port), inst.DBName,
		})
	}
	m.table.SetRows(rows)
}

func (m model) View() string {
	switch m.view {
	case viewLogs:
		return m.logsView()
	case viewDetail:
		return m.detailView()
	case viewUpdate:
		return m.updateView()
	case viewConf:
		return m.confView()
	case viewDoctor:
		return m.doctorView()
	case viewHelp:
		return m.helpView()
	}
	return m.listView()
}

func (m model) listView() string {
	var b strings.Builder
	b.WriteString(m.header())
	if m.notice != "" {
		b.WriteString("\n" + m.notice)
	}
	b.WriteString("\n\n")
	if len(m.insts) == 0 {
		b.WriteString(m.deps.Th.Warning.Render("  no instances yet — press q and run: odoonoir create <name>"))
		b.WriteString("\n")
	} else {
		b.WriteString(m.table.View())
		b.WriteString("\n")
	}
	if m.err != "" {
		b.WriteString(m.deps.Th.Error.Render("  "+m.err) + "\n")
	}
	b.WriteString("\n")
	b.WriteString(m.keyhints())
	return b.String()
}

func (m model) detailView() string {
	if m.cursor >= len(m.insts) {
		return m.listView()
	}
	inst := m.insts[m.cursor]
	p := inst.ResolvePaths(instRoot(m.deps, inst))
	st := m.status[inst.Name]
	lines := []string{
		fmt.Sprintf("name:       %s", inst.Name),
		fmt.Sprintf("type:       %s", instType(inst)),
		fmt.Sprintf("version:    %s  (%s)", inst.Version, inst.Branch),
		fmt.Sprintf("status:     %s", m.deps.Th.StatusStyle(string(st)).Render(string(st))),
		fmt.Sprintf("url:        http://localhost:%d", inst.Port),
		fmt.Sprintf("database:   %s (owner %s)", inst.DBName, inst.DBUser),
		fmt.Sprintf("location:   %s", p.Root),
		fmt.Sprintf("source:     %s", p.Source),
		fmt.Sprintf("workers:    %d", inst.Workers),
		fmt.Sprintf("log level:  %s", inst.LogLevel),
		fmt.Sprintf("conf:       %s", p.Conf),
		fmt.Sprintf("log:        %s", p.Log),
	}
	if m.db.size > 0 {
		lines = append(lines, fmt.Sprintf("db size:    %s", humanSize(m.db.size)))
	}
	if len(m.db.dbs) > 1 {
		rows := make([]string, 0, len(m.db.dbs))
		for _, d := range m.db.dbs {
			mark := " "
			if d.pri {
				mark = "*"
			}
			init := "-"
			if d.init {
				init = "ok"
			}
			rows = append(rows, fmt.Sprintf("    %s %-16s %10s  init %s", mark, d.name, humanSize(d.size), init))
		}
		lines = append(lines, append([]string{"databases:"}, rows...)...)
	}
	if m.db.backup != "" {
		lines = append(lines, "last backup: "+m.db.backup)
	}
	if inst.Description != "" {
		lines = append(lines, "", "description:", "  "+inst.Description)
	}
	var b strings.Builder
	b.WriteString(m.header())
	if m.notice != "" {
		b.WriteString("\n" + m.notice)
	}
	b.WriteString("\n\n")
	b.WriteString(m.deps.Th.Box("instance "+inst.Name, strings.Join(lines, "\n")))
	b.WriteString("\n\n")
	b.WriteString(m.deps.Th.Hint.Render("  [esc] back   [l] logs   [c] conf   [d] doctor   [o] open   [u] update   [s] start   [x] stop   [r] restart   [b] backup   [q] quit\n"))
	return b.String()
}

func (m model) logsView() string {
	if m.cursor >= len(m.insts) {
		return m.listView()
	}
	var b strings.Builder
	b.WriteString(m.header())
	b.WriteString("\n\n")
	status := "live"
	if m.paused {
		status = m.deps.Th.Warning.Render("paused")
	} else {
		status = m.deps.Th.Success.Render("live")
	}
	if m.logErrs {
		status += "  " + m.deps.Th.Warning.Render("errors only")
	}
	if m.logPath != "" {
		b.WriteString(m.deps.Th.Accent.Render("  log — "+m.logPath+"  ["+status+"]") + "\n\n")
	}
	if m.logErr != "" {
		b.WriteString(m.deps.Th.Warning.Render("  "+m.logErr) + "\n")
	} else if len(m.logLines) == 0 {
		b.WriteString(m.deps.Th.Warning.Render("  no log output yet") + "\n")
	} else {
		for _, l := range m.logLines {
			if m.logErrs && !isErrorLine(l) {
				continue
			}
			b.WriteString(colorLogLine(m.deps.Th, l))
			b.WriteString("\n")
		}
	}
	b.WriteString(m.deps.Th.Hint.Render("  [esc] back   [e] errors only   [p] pause   [q] quit\n"))
	return b.String()
}

// isErrorLine reports whether an odoo log line carries error severity.
func isErrorLine(line string) bool {
	return strings.Contains(line, " ERROR ") || strings.Contains(line, " CRITICAL ")
}

func (m model) confView() string {
	var b strings.Builder
	b.WriteString(m.header())
	b.WriteString("\n\n")
	if m.conf.err != "" {
		b.WriteString(m.deps.Th.Warning.Render("  "+m.conf.err) + "\n")
	} else {
		if m.conf.path != "" {
			b.WriteString(m.deps.Th.Accent.Render("  "+m.conf.path) + "\n\n")
		}
		for _, l := range m.conf.lines {
			b.WriteString("  " + l + "\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(m.deps.Th.Hint.Render("  [esc] back   [q] quit   (edit keys with: odoonoir config set <name> <key> <value>)"))
	return b.String()
}

func (m model) doctorView() string {
	var b strings.Builder
	b.WriteString(m.header())
	b.WriteString("\n\n")
	b.WriteString(m.deps.Th.Accent.Render("  doctor — log scan") + "\n\n")
	if m.doctor.err != "" {
		b.WriteString(m.deps.Th.Warning.Render("  "+m.doctor.err) + "\n")
	} else {
		for _, l := range m.doctor.lines {
			b.WriteString("  " + l + "\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(m.deps.Th.Hint.Render("  [esc] back   [q] quit"))
	return b.String()
}

func (m model) helpView() string {
	var b strings.Builder
	b.WriteString(m.header())
	b.WriteString("\n\n")
	b.WriteString(m.deps.Th.Box("keyboard", strings.Join([]string{
		"",
		"list view",
		"  ↑ / ↓, j / k   move between instances",
		"  enter           instance details",
		"  l               live log",
		"  u               update instance (git pull + modules)",
		"  s, x, r         start / stop / restart",
		"  b               backup database",
		"",
		"detail view",
		"  l               live log",
		"  c               odoo.conf keys",
		"  d               doctor (log scan)",
		"  o               open in browser",
		"  u, s, x, r, b   update / start / stop / restart / backup",
		"",
		"log view",
		"  e               toggle errors-only filter",
		"  p               pause / resume live tail",
		"",
		"everywhere",
		"  esc             back to the instance list",
		"  ?               this help",
		"  q, ctrl+c       quit",
	}, "\n")))
	b.WriteString("\n")
	b.WriteString(m.deps.Th.Hint.Render("  [esc] back   [q] quit"))
	return b.String()
}

// humanSize renders bytes as a compact human string.
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// instType describes how the instance is managed.
func instType(inst *instance.Instance) string {
	if inst.Adopted {
		return "adopted (existing installation)"
	}
	return "odoonoir-managed"
}

// colorIssueLine colors the ERROR/WARN prefix of a doctor report line.
func colorIssueLine(th *ui.Theme, line string) string {
	if strings.HasPrefix(line, "ERROR") {
		return th.Error.Render(line)
	}
	if strings.HasPrefix(line, "WARN ") {
		return th.Warning.Render(line)
	}
	return line
}

func (m model) updateView() string {
	if m.cursor >= len(m.insts) {
		return m.listView()
	}
	inst := m.insts[m.cursor]
	running, curStep, uerr, lines := m.update.snapshot()
	names := []string{
		"pull latest source",
		"refresh addons_path",
		"reinstall requirements",
		"install/upgrade modules",
	}

	var b strings.Builder
	b.WriteString(m.header())
	b.WriteString("\n\n")
	b.WriteString(m.deps.Th.Accent.Render("  updating "+inst.Name) + "\n\n")
	for i, n := range names {
		switch {
		case running && i == curStep:
			b.WriteString("  " + m.spinner.View() + " " + n + "\n")
		case running && i < curStep, !running && uerr == nil && i < 3:
			b.WriteString("  " + m.deps.Th.Success.Render(ui.CheckMark+" "+n) + "\n")
		default:
			b.WriteString("  " + m.deps.Th.Muted.Render("  "+n) + "\n")
		}
	}
	b.WriteString("\n")
	if uerr != nil {
		b.WriteString(m.deps.Th.Error.Render("  update failed: "+uerr.Error()) + "\n")
	} else if !running {
		b.WriteString(m.deps.Th.Success.Render("  update complete — restart from the list view ([r])") + "\n")
	}
	if len(lines) > 0 {
		b.WriteString("\n" + m.deps.Th.Box("output", strings.Join(tailN(lines, 12), "\n")) + "\n")
	}
	b.WriteString("\n")
	b.WriteString(m.deps.Th.Hint.Render("  [esc] back   [q] quit\n"))
	return b.String()
}

// tailN returns the last n items of lines.
func tailN(lines []string, n int) []string {
	if len(lines) <= n {
		return lines
	}
	return lines[len(lines)-n:]
}

// colorLogLine colors the severity token of an odoo log line.
func colorLogLine(th *ui.Theme, line string) string {
	if strings.HasPrefix(line, "202") || strings.Contains(line, " INFO ") ||
		strings.Contains(line, " DEBUG ") || strings.Contains(line, " WARNING ") ||
		strings.Contains(line, " ERROR ") || strings.Contains(line, " CRITICAL ") {
		fields := strings.SplitN(line, " ", 6)
		if len(fields) >= 6 {
			sev := fields[4]
			fields[4] = th.SeverityStyle(sev).Render(sev)
			rest := strings.Join(fields[5:], " ")
			fields[5] = rest
			return strings.Join(fields, " ")
		}
	}
	return line
}

func (m model) header() string {
	total := len(m.insts)
	running := 0
	for _, st := range m.status {
		if st == instance.StatusRunning {
			running++
		}
	}
	title := m.deps.Th.Header.Render("odoo noir")
	stats := fmt.Sprintf(" %s  %d running / %d total",
		m.spinner.View(), running, total)
	return title + m.deps.Th.Muted.Render(stats)
}

func (m model) keyhints() string {
	hints := [][2]string{
		{"↑↓", "navigate"}, {"enter", "details"}, {"l", "logs"}, {"u", "update"},
		{"s", "start"}, {"x", "stop"}, {"r", "restart"}, {"b", "backup"},
		{"?", "help"}, {"q", "quit"},
	}
	var parts []string
	for _, h := range hints {
		parts = append(parts, m.deps.Th.Accent.Render(h[0])+" "+m.deps.Th.Muted.Render(h[1]))
	}
	return strings.Join(parts, "   ")
}
