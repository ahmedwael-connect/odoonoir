# Sprint 3: Module Diff, Model Inspector & Odoo Shell
**Target**: v0.9.0 | **Theme**: Developer Power Tools

---

## 📋 Sprint 3 Goals

| Feature | Impact | Effort |
|---------|--------|--------|
| **Module Diff** — Compare installed vs disk manifest | 🔴 Critical | M |
| **Model Inspector** — Field/relation/ACL explorer | 🔴 Critical | M |
| **Odoo Shell/REPL** — WebSocket PTY terminal in GUI | 🔴 Critical | L |
| **Cron/Scheduled Actions Viewer** | 🟡 High | S |
| **Module dependency graph** | 🟡 High | M |
| **Record browser** — List/view/edit any model | 🟢 Medium | L |

---

## 🔧 Backend Changes

### 1. ModuleDiff (`internal/service/manage.go`)

Compare `ir_module_module` state vs `__manifest__.py` on disk:

```go
// ModuleDiffResult represents differences between DB and disk.
type ModuleDiffResult struct {
	ModuleName    string   `json:"moduleName"`
	InDB          bool     `json:"inDB"`
	OnDisk        bool     `json:"onDisk"`
	DBState       string   `json:"dbState"`       // installed, uninstalled, to upgrade, to remove
	DiskVersion   string   `json:"diskVersion"`   // from __manifest__.py
	DBVersion     string   `json:"dbVersion"`     // from ir_module_module
	FieldsAdded   []string `json:"fieldsAdded"`   // fields on disk not in DB
	FieldsRemoved []string `json:"fieldsRemoved"` // fields in DB not on disk
	ModelsAdded   []string `json:"modelsAdded"`   // models on disk not in DB
	ModelsRemoved []string `json:"modelsRemoved"` // models in DB not on disk
	ViewsAdded    []string `json:"viewsAdded"`    // view XMLs on disk not in DB
	ViewsRemoved  []string `json:"viewsRemoved"`  // view XMLs in DB not on disk
	ManifestDiff  string   `json:"manifestDiff"`  // unified diff of __manifest__.py
}

// ModuleDiff compares installed modules with their disk manifests.
func (s *Service) ModuleDiff(name, dbName string) ([]ModuleDiffResult, error) {
	inst, err := s.reg.Get(name)
	if err != nil { return nil, err }
	if dbName == "" { dbName = inst.DBName }
	p := inst.ResolvePaths(s.rootFor(inst))

	// 1. Get all modules from DB
	dbModules, err := s.getDBModules(dbName)
	if err != nil { return nil, err }

	// 2. Scan addons_path for modules with __manifest__.py
	diskModules, err := scanDiskModules(p.Source, inst.AddonsPaths)
	if err != nil { return nil, err }

	// 3. Compare and compute diffs
	return computeModuleDiff(dbModules, diskModules), nil
}

// getDBModules queries ir_module_module with full field list.
func (s *Service) getDBModules(dbName string) (map[string]DBModule, error) {
	query := `SELECT name, state, latest_version, COALESCE(shortdesc::text, ''),
		COALESCE(author, ''), COALESCE(depends::text, ''),
		COALESCE(menus::text, ''), COALESCE(models::text, '')
	FROM ir_module_module`
	// parse results into map[name]DBModule
}
```

### 2. Model Inspector (`internal/service/manage.go`)

```go
// ModelInfo represents a model's full metadata.
type ModelInfo struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Table       string      `json:"table"`
	RecName     string      `json:"recName"`
	Order       string      `json:"order"`
	Fields      []FieldInfo `json:"fields"`
	Access      []ACLInfo   `json:"access"`
	Views       []ViewInfo  `json:"views"`
	Actions     []ActionInfo `json:"actions"`
}

// FieldInfo represents a field's metadata.
type FieldInfo struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	String      string `json:"string"`
	Required    bool   `json:"required"`
	Readonly    bool   `json:"readonly"`
	Index       bool   `json:"index"`
	Store       bool   `json:"store"`
	Relation    string `json:"relation"`    // for relational fields
	RelationTable string `json:"relationTable"` // for many2many
	ComodelName string `json:"comodelName"`   // for many2one
	Domain      string `json:"domain"`
	Default     string `json:"default"`
	Groups      string `json:"groups"`
	Help        string `json:"help"`
}

// ACLInfo represents ir.model.access rules.
type ACLInfo struct {
	Name        string `json:"name"`
	Group       string `json:"group"`
	PermRead    bool   `json:"permRead"`
	PermWrite   bool   `json:"permWrite"`
	PermCreate  bool   `json:"permCreate"`
	PermUnlink  bool   `json:"permUnlink"`
}

// ModelInfo fetches complete model metadata from the database.
func (s *Service) ModelInfo(name, dbName, model string) (*ModelInfo, error) {
	inst, err := s.reg.Get(name)
	if err != nil { return nil, err }
	if dbName == "" { dbName = inst.DBName }

	// Query ir_model for model metadata
	modelQuery := `SELECT model, name, info, table_name, rec_name, "order"
		FROM ir_model WHERE model = $1`
	// Query ir_model_fields for fields
	fieldsQuery := `SELECT name, ttype, field_description, required, readonly, 
		index, store, relation, relation_table, comodel_name, 
		domain, default, groups, help
		FROM ir_model_fields WHERE model = $1 ORDER BY name`
	// Query ir_model_access for ACLs
	// Query ir_ui_view for views
	// Query ir_actions_act_window for actions
}
```

### 3. Odoo Shell/REPL (`internal/proc/proc.go` + new `internal/proc/shell.go`)

```go
// Shell starts an interactive `odoo shell` session and returns a WebSocket endpoint.
// The frontend connects via WebSocket for a real PTY experience.
type ShellSession struct {
	conn   *websocket.Conn
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
	done   chan struct{}
}

// NewShellSession spawns `odoo shell -c conf -d db` with PTY.
func NewShellSession(inst *instance.Instance, p instance.Paths, py string, dbName string) (*ShellSession, error) {
	// Use gorilla/websocket + creack/pty for true PTY
}

// HandleWS upgrades HTTP to WebSocket and pipes stdin/stdout/stderr.
func (s *ShellSession) HandleWS(w http.ResponseWriter, r *http.Request) error {
	// Upgrade to WS, then:
	// - Read from WS → write to pty stdin
	// - Read from pty stdout/stderr → write to WS
	// - Handle resize messages (rows/cols)
}
```

**Dependencies**: `github.com/gorilla/websocket`, `github.com/creack/pty`

### 4. Cron/Scheduled Actions Viewer (`internal/service/manage.go`)

```go
// CronEntry represents an ir.cron record.
type CronEntry struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Model        string `json:"model"`
	Function     string `json:"function"`
	Args         string `json:"args"`
	IntervalType string `json:"intervalType"` // minutes, hours, days, weeks, months
	IntervalNum  int    `json:"intervalNumber"`
	NextCall     string `json:"nextCall"`
	NumberCall   int    `json:"numberCall"` // -1 = infinite
	DoAll        bool   `json:"doAll"`
	Active       bool   `json:"active"`
	Priority     int    `json:"priority"`
}

// CronList returns all scheduled actions for a database.
func (s *Service) CronList(name, dbName string) ([]CronEntry, error) {
	inst, err := s.reg.Get(name)
	if err != nil { return nil, err }
	if dbName == "" { dbName = inst.DBName }
	query := `SELECT id, name, model, function, args, interval_type, interval_number,
		nextcall, numbercall, doall, active, priority
		FROM ir_cron ORDER BY nextcall`
}
```

---

## 🎨 Frontend Changes

### 1. Modules Screen → Add Diff Tab

```
Modules Screen
├── [Installed] [Uninstalled] [To Upgrade] [Diff] ← NEW TAB
├── Search / Filter
├── Table: Module | DB State | Disk Version | DB Version | Status | Actions
└── Row Actions: [View Diff] [Open Manifest] [Upgrade] [Uninstall]
```

**Diff Modal** — Side-by-side comparison:
- Left: DB state (from `ir_module_module`)
- Right: Disk state (from `__manifest__.py` + model/view files)
- Highlight: added/removed fields, models, views, dependencies

### 2. Model Inspector Panel (Click module → right drawer)

```
┌─────────────────────────────────────┐
│ Model: library.book          [✕]   │
├─────────────────────────────────────┤
│ Fields                              │
│ ▼ name        char        required  │
│ ▼ author_id   many2one    res.partner│
│ ▼ pages       integer                │
│ ▼ tags        many2many   library.tag│
├─────────────────────────────────────┤
│ Access Rules (ir.model.access)      │
│ user  | read write create unlink    │
├─────────────────────────────────────┤
│ Views (ir.ui.view)                  │
│ list  | library.book.list           │
│ form  | library.book.form           │
├─────────────────────────────────────┤
│ Actions (ir.actions.act_window)     │
│ Library Books | tree,form           │
└─────────────────────────────────────┘
```

### 3. Terminal Tab in Instance Detail

```
┌─────────────────────────────────────┐
│ Instance: myapp (running)     [✕]  │
├─────────────────────────────────────┤
│ [Info] [Databases] [Logs] [Update] │
│ [Modules] [Config] [Terminal] ← NEW │
└─────────────────────────────────────┘

$ odoo shell -c /path/to/odoo.conf -d myapp
Python 3.10.12 | Odoo 18.0
>>> env['res.partner'].search([])
res.partner(1, 2, 3)
>>> 
```

**Implementation**: xterm.js + WebSocket to backend `/shell` endpoint

### 4. Cron Tab in Instance Detail

```
┌─────────────────────────────────────┐
│ Cron Jobs                    [✕]   │
├─────────────────────────────────────┤
│ Name              | Next Run | Act  │
│ Generate Invoices | 2024-01-15│ ✓   │
│ Cleanup Sessions  | 2024-01-14│ ✓   │
│ [Run Now] [Edit] [Toggle]           │
└─────────────────────────────────────┘
```

---

## 📦 New Dependencies

```bash
go get github.com/gorilla/websocket
go get github.com/creack/pty
go get github.com/sergi/go-diff/diffmatchpatch  # for manifest diff
```

```json
// package.json
"xterm": "^5.3.0",
"xterm-addon-fit": "^0.8.0",
"xterm-addon-web-links": "^0.9.0"
```

---

## 📁 Files to Modify

| File | Changes |
|------|---------|
| `internal/service/manage.go` | Add `ModuleDiff`, `ModelInfo`, `CronList` |
| `internal/service/service.go` | Bind new methods |
| `internal/proc/proc.go` | Add `NewShellSession`, admin route `/shell` |
| `internal/proc/shell.go` | NEW: WebSocket PTY handler |
| `gui/app.go` | Bind `ModuleDiff`, `ModelInfo`, `CronList`, `Shell` |
| `gui/frontend/src/App.tsx` | Add Terminal tab, ModelInspector component |
| `gui/frontend/src/Screens.tsx` | Add Diff tab to ModulesScreen |
| `gui/frontend/src/styles.css` | Terminal, diff view, model inspector styles |
| `gui/linux/nfpm/nfpm.yaml` | Version `0.9.0` |

---

## ✅ Definition of Done

- [ ] **ModuleDiff** works: shows side-by-side DB vs disk comparison
- [ ] **Model Inspector** opens from Modules screen, shows fields/ACLs/views/actions
- [ ] **Odoo Shell** terminal works in GUI with full PTY (color, resize, history)
- [ ] **Cron tab** lists scheduled actions with run/edit/toggle
- [ ] All tests pass, deb builds clean

---

## 🎯 Sprint 4 Preview (Post v0.9.0)

- **Record Browser** — Generic list/form view for any model
- **Module Dependency Graph** — Visual graphviz/D3.js
- **Backup Scheduler** — Cron-based auto-backup with retention
- **Multi-instance Dashboard** — Aggregate view across projects