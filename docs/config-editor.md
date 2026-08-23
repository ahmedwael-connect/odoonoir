# Config Editor Engine — Design & Plan

Status: implemented (v0.7.0)
Owner: odoonoir
Scope: the `internal/odoconf` package (rewritten) + the `odoonoir config` CLI surface
+ every other consumer of `odoconf` (installer, adopt, clone, rename, updater, dash).

---

## 1. Goals

1. **Full control of the conf file** of every instance — including *adopted* ones,
   whose conf is owned by the user and must never be rewritten destructively.
2. **Comment / uncomment any option** without losing its value or its comments.
3. **`addons_path` as a first-class array**: list it numbered, add, delete and —
   most importantly — **reorder paths** (order matters: Odoo resolves modules
   first-match-wins along `addons_path`).
4. **Lossless edits**: comments, blank lines, section headers, key ordering and
   even unknown lines survive every round-trip.
5. Safe: atomic writes, a backup copy before each mutation, clear "restart to
   apply" guidance, and registry synchronisation for keys the rest of the tool
   reads.

## 2. Why a new engine (current limitations)

The current `internal/odoconf` (v0.5/v0.6) is a lossy INI parser:

| Limitation | Consequence |
|---|---|
| Comments are dropped at parse time | Editing destroys the user's comments; `Edit()` uses regex |
| Key order is lost (sorted on write) | Reordering `addons_path` is impossible; file diff churn |
| Bare keys are folded into `[options]` | No way to round-trip a conf without a section header |
| No "commented key" concept | Can't disable an option while keeping its value |
| `addons_path` is a plain string | No add/remove/reorder operations; human error on CSV |
| `Edit()` is regex-based | Fragile with special chars; no section awareness |

The engine below replaces the internals while **keeping the existing
`OdooConf` API surface** (`Load/New/Get/Set/Unset/Keys/Write/Save/Path`), so all
current call sites compile and behave as before — then gains the new
capabilities on top.

## 3. Document model (lossless, line-based)

The file is a slice of **lines**, each classified once, in file order.
Everything not understood is preserved byte-for-byte.

```
type Kind int
const (
    KindSection       // [options]
    KindKey           // key = value           (active)
    KindCommentedKey  // # key = value         (inactive, value kept)
    KindComment       // # or ; free comment
    KindBlank         // empty line
    KindContinuation  // indented line attached to the previous key (configparser multi-line)
    KindUnknown       // anything else — preserved verbatim
)

type Line struct {
    Kind  Kind
    Raw   string   // the original line (byte-exact, minus the trailing \n)
    Key   string   // KindKey / KindCommentedKey
    Value string   // raw value text
}
```

Rules:

- **Section tracking.** A key belongs to the section declared before it.
  Keys before any `[section]` belong to the implicit `options` section — the
  engine reads/writes *options* keys wherever they live (top-of-file or under
  `[options]`) and never adds a section header to a conf that does not have one.
  Newly created confs (`installer`) keep the generated `[options]` header.
- **Get semantics** = configparser: the **last active** `key = value` wins.
  Commented keys are inactive but visible via `Keys()`.
- **Attached comments.** The block of `KindComment`/`KindBlank` lines directly
  above a key is its attached block; `Delete` removes them together, `Set`
  keeps them.
- **Multi-line values** (indented continuation lines, used by some deployments)
  are attached to the preceding key and written back unchanged.
- **Write** re-emits every line: mutated lines only for changed keys, the rest
  byte-exact. A `Save()` never reformats the file.

## 4. Engine API (`internal/odoconf`)

```go
// existing surface (kept, now lossless)
func Load(path string) (*OdooConf, error)
func New() *OdooConf                     // generated-conf defaults (installer), deterministic order
func (c *OdooConf) Get(key string) (string, bool)   // last active occurrence wins
func (c *OdooConf) Set(key, value string)           // set in place, re-enable commented, or append
func (c *OdooConf) Unset(key string)                // delete key + attached comment block
func (c *OdooConf) Keys() []string                  // active options keys in file order
func (c *OdooConf) AllKeys() []Entry                // active + commented; Entry{Name, Active}
func (c *OdooConf) Write(path string) error         // atomic temp+rename
func (c *OdooConf) Save() error                     // .bak once per session, only when dirty
func (c *OdooConf) Path() string

// new: comment / uncomment
func (c *OdooConf) Comment(key string) error     // disable, keep the value
func (c *OdooConf) Uncomment(key string) error   // re-enable in place (absent key → Set)

// new: addons_path as an array
func (c *OdooConf) AddonsPath() ([]string, error)    // order = priority (first match wins)
func (c *OdooConf) SetAddonsPath(paths []string) error
func (c *OdooConf) AddonsAdd(path string, at int) error  // at<0 → append; dup → error
func (c *OdooConf) AddonsRemove(path string) error        // absent → error
func (c *OdooConf) AddonsMove(from, to int) error         // 0-based; clamped to bounds

// new: introspection + safety
func (c *OdooConf) Lines() []Line                      // raw line model (viewers)
func (c *OdooConf) Dirty() bool                        // mutated since Load
func (c *OdooConf) Backup() (string, error)            // <conf>.bak copy before first mutation
```

`addons_path` value grammar: comma-separated, whitespace-trimmed, empty entries
ignored. Quotes are *not* used by Odoo for this key; if a path contains a comma
it is rejected with a clear error (documented limitation). `AddonsMove` is the
reorder primitive: the CLI builds `up/down/move` on top of it.

## 5. Comment / uncomment semantics

- `Comment(key)` — the active `key = value` line becomes `# key = value`.
  Its value, position and attached comments stay. `Get` no longer returns it.
  If the key is already commented: no-op. If absent: appended as a commented
  line so the option is *documented* without being enabled.
- `Uncomment(key)` — strips `# ` / `; ` from the commented line, restoring it
  in place. If absent: behaves like `Set`.
- These ops are the safe way to e.g. disable `workers` or `logfile` instead of
  deleting them, and they round-trip adopted confs perfectly.

## 6. Safety

1. **Atomic write**: `Save()` writes `<conf>.tmp` in the same directory then
   renames over the target — a crash never leaves a truncated conf.
2. **Backup**: the first mutation of a session writes `<conf>.bak` (previous
   content preserved; `Backup()` returns its path). Restore = `cp conf.bak conf`.
3. **Running-instance warning**: every mutating command prints
   "restart the instance for changes to apply: odoonoir restart <name>"
   (identical to today's `set`).
4. **Registry sync**: keys the rest of the tool reads (`http_port`, `db_name`,
   `db_user`, `db_password`, `addons_path`) are mirrored into the instance
   registry on every edit (extending today's `syncRegistryKey` to
   `addons_path`), so `list`, `module new`, `update` and the dash stay correct.
5. **Adopted instances**: same engine, zero special-casing — edits are
   in-place, line-preserving, and the conf is never regenerated.

## 7. CLI surface (`odoonoir config`)

```
odoonoir config list <name>            # active keys in file order
odoonoir config list <name> --all      # + commented keys, marked "# key = value"
odoonoir config get <name> <key>       # unchanged
odoonoir config set <name> <key> <val> # unchanged behaviour
odoonoir config unset <name> <key>     # unchanged

odoonoir config comment <name> <key>       # disable, keep the value
odoonoir config uncomment <name> <key>     # re-enable

# addons_path editor — the array
odoonoir config addons <name>                  # alias of list
odoonoir config addons list <name>             # numbered list (1 = highest priority)
odoonoir config addons list <name> --edit      # interactive TUI editor
odoonoir config addons add <name> <path>            # append
odoonoir config addons add <name> <path> --at 2     # insert at position (1-based)
odoonoir config addons remove <name> <path>         # delete a path
odoonoir config addons move <name> <from> <to>      # e.g. "3 1" reorders (1-based)
odoonoir config addons up <name> <n>                # one step up (higher priority)
odoonoir config addons down <name> <n>              # one step down
```

**Interactive editor** (`--edit`): a bubbletea TUI (`internal/tui/addons_edit.go`)
with real reorder support:

- `↑`/`k` `↓`/`j` — select an entry
- `u` / `d` — move the selected entry up / down (reorders the list)
- `a` — append a path (inline input), `e` — edit the selected path
- `x` — remove the selected entry
- `q` / `esc` — save & quit (`.bak` + restart hint), `ctrl+c` — abort

Examples:

```
$ odoonoir config addons myapp
1  /home/ahmed/.odoonoir/myapp/custom_addons     <- your modules
2  /home/ahmed/.odoonoir/myapp/src/odoo/addons   <- core addons
3  /home/ahmed/.odoonoir/myapp/src/odoo/odoo/addons

$ odoonoir config addons move myapp 3 1
addons_path of myapp updated:
1  /home/ahmed/.odoonoir/myapp/src/odoo/odoo/addons
2  /home/ahmed/.odoonoir/myapp/custom_addons
3  /home/ahmed/.odoonoir/myapp/src/odoo/addons
restart the instance for changes to apply: odoonoir restart myapp

$ odoonoir config addons remove myapp /home/ahmed/.odoonoir/myapp/src/odoo/odoo/addons
$ odoonoir config comment myapp workers
commented workers
restart the instance for changes to apply: odoonoir restart myapp
```

## 8. Integration points (all existing consumers keep working)

| Consumer | Today | With the engine |
|---|---|---|
| `installer` (create) | `odoconf.New` + `Set` + `Write` | unchanged API; deterministic key order |
| `cli/config.go` | `Load/Get/Set/Unset/Keys` | same + comment/uncomment/addons |
| `clone.go` / `rename.go` | `odoconf.Edit` (regex) | removed; `applyConfEdits` (load, Set all, Save once) |
| `adopt.go` | `Load` (validation) | unchanged |
| `updater.go` | `refreshAddonsPath` replaced the whole list | now *appends* missing built-in paths only — user reordering via `config addons` survives `update` |
| `tui/dash.go` | `Load` + `Keys` (conf viewer) | unchanged (viewer shows active keys; comments visible via `config list --all`) |

The engine deliberately does **not** mirror `addons_path` into the instance
registry: the registry's `AddonsPaths` field means "custom module dirs"
(scaffold target for `module new`) and is populated by detection; the conf
is the single source of truth for the full ordered list.

## 9. Implementation plan (v0.7.0)

### Phase 1 — engine rewrite (`internal/odoconf`) — done
- Line model (`Kind`/`Line`), section-aware lossless `Load`/`Write`/`Save`
- `Get/Set/Unset/Keys/New` semantics kept; `Keys()` file order; `AllKeys()`
- Atomic write + `.bak` backup + `Dirty()`
- Unit tests: round-trip preserves comments/order/unknown lines; last-wins
  `Get`; attached-block delete; multiline values; other-section isolation

### Phase 2 — addons_path array ops — done
- `AddonsPath/SetAddonsPath/AddonsAdd/AddonsRemove/AddonsMove`
- Unit tests: parse/re-serialize, dedupe, move clamping, error cases

### Phase 3 — CLI (`odoonoir config`) — done
- `comment` / `uncomment`
- `config addons` (list/add/remove/move/up/down, `--at`) with numbered output
- `list --all` (shows commented keys); db_password masking kept
- Restart hints on every mutation

### Phase 3b — interactive TUI editor — done
- `config addons list <name> --edit`: bubbletea editor with real reorder
  (`u`/`d`), inline add/edit input, remove, save-on-quit with `.bak`

### Phase 4 — migration of remaining consumers — done
- `clone.go` / `rename.go`: `odoconf.Edit` removed, `applyConfEdits`
- `updater.go`: refresh *appends* missing built-in paths (reorder survives)
- README: commands table + "Config editor" section; `docs/config-editor.md`
- Root help: config editor examples

### Phase 5 — verification — done
- `go vet` + full `go test ./...`
- pty e2e: addons reorder/add/remove, comment/uncomment, `--edit` TUI,
  restart applies, adopted conf lossless round-trip

## 10. Out of scope (future)

- `db_password` encryption at rest (only masked in output, as today)
- Multi-key `set` transactions / `config apply <file>` bulk edits
- Templated confs (per-host includes)
- Global conf templates for new instances