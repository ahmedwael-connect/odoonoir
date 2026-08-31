# NAV Fix — Blank Tabs Root Cause & Recovery Plan

**Symptom `App.tsx:392`:** `effectiveScreen` = `terminal|config|cron|records|depgraph|scaffold|clone|modelinspector|backups|systemcheck|dashboard` → no branch in `App.tsx:463-520` → empty `<main>` (white). Reproduced both `running`/`stopped`.

**Why:** `gui/frontend/src/hooks/useInstanceNav.ts:4` defines 15 `InstanceScreen` + 4 `GlobalScreen`, `InstanceTopNav.tsx:1` renders 8 primary + 7 in `More`, but `App.tsx` (restored to v0.5 1418 lines) only renders 7 screens. Revert `git checkout HEAD -- Screens.tsx App.tsx` deleted Sprint 3/4 components (`TerminalScreen`, `ModelInspectorScreen`, `CronScreen`, `RecordBrowserScreen`, `DepGraphScreen`, `ScaffoldScreen`, `CloneScreen`, `SystemCheckScreen`, `Dashboard`) that `gui/app.go:303-477` still exposes (61 bindings).

**Mapping audit (TopNav → Main → Component → Running?:**

| Top Nav `id` | Main check `App.tsx:463` | Component | File | Binding `gui/app.go` | Needs running? | Blanks if missing |
|---|---|---|---|---|---:|---|
| `overview` | `overview\|instances` → `InstanceDetail` | `InstanceDetail` | `App.tsx:666` | `Status` | no | — |
| `databases` | `databases` → `DatabasesScreen` | `DatabasesScreen` | `App.tsx:750` | `Databases` | no | — |
| `modules` | `modules` → `ModulesScreen` | `ModulesScreen` | `Screens.tsx:22` | `ModuleList` | no (but `Install` needs stopped? no) | — |
| `logs` | `logs` → `LogsScreen` | `LogsScreen` | `App.tsx:944` | `LogsSince` | no | — |
| `terminal` | **MISSING** | **MISSING** `TerminalScreen` | — | `ShellURL` | yes/no? should show `not running` hint | blank |
| `update` | `update` → `UpdateScreen` | `UpdateScreen` | `App.tsx:1078` | `Update` | no | — |
| `config` | **MISMATCH** `settings` | `SettingsScreen` | `App.tsx:1313` | `ReadConf` | no | blank (`config`≠`settings`) |
| `doctor` | `doctor` → `DoctorScreen` | `DoctorScreen` | `Screens.tsx:333` | `Doctor` | no | — |
| `cron` | **MISSING** | **MISSING** `CronScreen` | — | `CronList` | needs DB, not running | blank |
| `records` | **MISSING** | **MISSING** `RecordBrowserScreen` | — | `BrowseRecords` | needs DB, not running | blank |
| `depgraph` | **MISSING** | **MISSING** `DepGraphScreen` | — | `ModuleDepGraph` | no | blank |
| `scaffold` | **MISSING** | **MISSING** `ScaffoldScreen` | — | `ScaffoldModule` | no | blank |
| `clone` | **MISSING** | **MISSING** `CloneScreen` | — | `Clone` | no | blank |
| `modelinspector` | **MISSING** | **MISSING** `ModelInspectorScreen` | — | `ModelInfo` | needs DB | blank |
| `backups` | **CONFLICT** | `DatabasesScreen` has internal `backups` tab `App.tsx:556?` but TopNav expects separate screen | `ListScheduledBackups` | no | blank (duplicate vs missing) |
| `systemcheck` | **MISSING global** | **MISSING** `SystemCheckScreen` | — | `SystemCheck` | no | blank |
| `dashboard` | **MISSING global** | **MISSING** `DashboardScreen` | — | `GetDashboardMetrics` | no | blank |

**Secondary problems found while debugging:**
- `DatabasesScreen` internal `tab` state (`databases|backups`) collides with top `backups` → user must click twice, confusing.
- `SettingsScreen` expects `selected` but TopNav `config` never routes.
- All missing screens need `Databases(name)` to pick `dbName` → empty when instance has no DB; currently shows "No databases found" forever with no retry.
- `TerminalScreen` needs `xterm` + `xterm-addon-fit` already in `package.json:1` but not imported; `ShellURL` requires `LongpollPort` → adopted instances have `LogPath` override, may 404.
- `ScaffoldScreen`/`CloneScreen` previously in `App.tsx:1200+` deleted; their bindings `ScaffoldModule`, `Clone`, `SystemCheck`, `CheckVersion` still exist.
- Buttons: `InstanceDetail` now uses new `Button` atom, but `DatabasesScreen`, `LogsScreen`, `UpdateScreen`, `SettingsScreen`, `Screens.tsx` still use legacy `.btn` → visual inconsistency, no `loading` state, no icons.
- `useInstanceNav` guard `App.tsx:392` normalizes `instances→overview` but `Scaffold` etc still use old `"instances"` string; `1-9` shortcuts `App.tsx:372` still reference 9 old screens, not 15.
- `AppShell` event-log kept as right strip, but new header has bell with `eventLog.length` → duplication.

---

## Fix Plan — 3 Phases, Minimal Risk (keep Lift+TopNav)

**Phase 0 — Hotfix (30 min, restores functionality):**
- Alias `config` → `settings`: in `useInstanceNav.ts:17` rename `config` to `settings` or make `App.tsx:516` handle both `effectiveScreen==="config"||effectiveScreen==="settings"`.
- Add missing branches in `App.tsx:463-520` as lazy placeholders that reuse existing components or show `EmptyState` + `Create` CTA, so no blank:
  ```tsx
  {selected && effectiveScreen==="terminal" && <TerminalScreen name={selected} />}
  {selected && effectiveScreen==="cron" && <CronScreen name={selected} />}
  {selected && effectiveScreen==="records" && <RecordBrowserScreen name={selected} />}
  {selected && effectiveScreen==="depgraph" && <DepGraphScreen name={selected} />}
  {selected && ["scaffold","clone","modelinspector"].includes(effectiveScreen) && <ScaffoldCloneInspectorRouter .../>}
  {selected && effectiveScreen==="backups" && <DatabasesScreen name={selected} ... initialTab="backups" />}
  {screen==="systemcheck" && <SystemCheckScreen />}
  {screen==="dashboard" && <DashboardScreen />}
  ```
  Even if component is minimal `EmptyState`, tab won't be blank.

**Phase 1 — Restore Sprint 3/4 Screens (2 days, copy from git history + adapt to new shell):**
- `git show HEAD~1:gui/frontend/src/App.tsx` or `SPRINT_3_PLAN.md` → extract `TerminalScreen` (xterm), `ModelInspectorScreen` (tabs fields/access/views/actions), `CronScreen` (table), `RecordBrowserScreen` (filter + table + CRUD modals), `DepGraphScreen` (d3 force), `ScaffoldScreen` (form + `ScaffoldModel`), `CloneScreen` (progress), `SystemCheckScreen` (severity list), `DashboardScreen` (metrics cards). Move each to `src/screens/<Name>.tsx` and lazy-load (`React.lazy`).
- Update `Screens.tsx:22` to re-export `ScaffoldScreen`, `CloneScreen`, `SystemCheckScreen` or keep in `App.tsx` but import from `screens/`.
- Ensure each screen handles **both** running/stopped: show banner `Instance must be stopped for ...` if `requireStopped` else allow; for `Terminal` show `Start instance to enable shell` but still allow `Logs`. Add `Skeleton` while `Databases(name)` loading, `EmptyState` with retry.
- Fix `Backups` conflict: remove internal `DatabasesScreen` tab, make it pure `databases`; top `backups` renders `BackupSchedulerScreen` (extracted from `DatabasesScreen:556` backup-scheduler UI) that calls `ListScheduledBackups`, `GetBackupScheduleStatus`, `SetBackupSchedule`.

**Phase 2 — Polish & Consolidate (1 day):**
- Normalize `Screen` type single source: `hooks/useInstanceNav.ts:4` is source, `App.tsx:35` should `import type {Screen} from "./hooks/useInstanceNav"` not redeclare.
- Migrate remaining `.btn` to `<Button>` in `DatabasesScreen`, `LogsScreen`, `UpdateScreen`, `SettingsScreen`, `Screens.tsx` (add `loading`, `iconLeft`).
- Fix `1-9` shortcuts to map to `instanceNavTabs` order + `More` (or `Cmd+K` only for `More`).
- Remove dead code: old `SidebarItem` `App.tsx:146` (still defined but unused, 60 lines), old `.sidebar-item` CSS not needed for lift, dedupe `styles.css` (currently 1076 lines + 38k tailwind).
- Responsive: `InstanceTopNav` scroll fade, `Lift` drawer already handles `collapsed` via `AppShell`; ensure `main` padding accounts for sticky topnav (`top-[100px]`).
- `wails3 generate bindings` → verify 61→ ~70 methods, `vite build` 1940 modules, `go vet` 0.

**Verification:**
- For each of 15 tabs, test: instance `myapp` stopped → open tab → see expected empty/CTA or data; start instance → refresh → data appears; no `blank`. Check console no `Could not resolve`.
- Build deb `0.14.1` and `sudo dpkg -i`.

---

## Immediate Workaround (before code fix)
Clicking `terminal` etc shows blank because route missing. Use old sidebar: temporarily set `selected==null` → global nav shows `Create/Adopt` only, not affected. Or navigate via `Cmd+K` not yet wired.
