# NAV Refactor — Odoo.sh Concept: Instance Lift + Top Instance Nav

**Goal:** Redesign left nav into a pure "Instance Lift" (Odoo.sh projects pane) and move all instance-context navigation to a top horizontal bar. Modernize buttons with design-system v2. Full refactor plan for `gui/frontend/src/App.tsx`, `Screens.tsx`, `styles.css`, bindings and state.

---

## 1. Current Analysis ( `gui/frontend/src/App.tsx:35`, `App.tsx:398`, `App.tsx:386`, `styles.css:156` )

**Current layout `App.tsx:385-600`:**
```
shell
 ├─ topbar (static: Odoonoir + count + kbd hints)  [App.tsx:386]
 └─ layout
     ├─ sidebar [App.tsx:398] (240px, collapsible)
     │   ├─ Instances section (SidebarItem [App.tsx:148] with pill + qa-btns)
     │   └─ Screens section (9 items: instances/databases/modules/logs/update/settings/create/adopt/doctor) [App.tsx:445]
     ├─ main (InstanceDetail | DatabasesScreen | LogsScreen …) [App.tsx:470-561]
     └─ event-log (280px strip) [App.tsx:565]
```

**Problems:**
- Sidebar mixes two concerns: instance selection + instance navigation → crowded, 9+ items, not scalable for future screens (scaffold/clone/systemcheck/terminal/modelinspector/cron/recordbrowser/depgraph already overflow 1-9 shortcuts).
- Topbar is dead space (only title). No instance context.
- `Screen` type `App.tsx:35` is flat, not grouped (global vs instance).
- Buttons in `InstanceDetail [App.tsx:722]` use old `.btn` variants (`.primary`, `.danger`, `.success`, `.warning`) but not Radix/variants, no icons, no size scale, inconsistent `qa-btn` vs `btn small`.
- Accessibility: sidebar items are `<button>` but nested quick-action buttons violate HTML (button-in-button). No `aria-current="page"` consistency.
- Responsive: sidebar collapses to 40px, topbar hints hide at 900px, but top nav would overflow.

**Odoo.sh reference:**
- Left: Project list (like our Instances) with search, status dot, branch/version.
- Top: When project selected, sticky horizontal nav: `Overview | Branches | Builds | Shell | Logs | Backups | Settings | ...` + project status/actions (start/stop) on right.
- Content below top nav is tab content. Global actions (Create Project) live outside instance context.

---

## 2. Target Information Architecture

**Two nav layers:**

1. **Lift (left, 280px → 72px collapsed, drawer on <768px) — Instance-only**
   - Header: `OdooNoir` logo + search input (filter instances) + `+` create button (primary, icon `Plus`)
   - List: `InstanceLiftItem` (status pill-dot, name, version, health dot, adopted badge) — no quick actions inside row
   - Footer: instance count, collapsed toggle
   - No Screens section. Lift only selects `selected`.

2. **Top Instance Nav (sticky, 44px, appears only when `selected != null`) — Like Odoo.sh**
   - Left: `InstanceTitle` (name + version + status pill + `Serving` db select) + divider
   - Center: Horizontal `InstanceNavTabs` (scrollable, underline indicator):
     - `Overview` (instances) | `Databases` (count) | `Modules` | `Logs` (live dot) | `Terminal` | `Update` | `Config` | `Doctor` | `Metrics` (if added) | `Backups` (under Databases) | `Cron` | `Records` | `Graph` …
     - Grouped: `Dev: Scaffold, ModelInspect, Clone, SystemCheck` in `⋯ More` dropdown (Radix Dropdown)
   - Right: Primary actions `Start/Stop/Restart` (split button) + `•••` Danger dropdown (Remove, Backup, Launch Config, Dev Mode)
   - When `selected==null`, top bar shows **Global Nav**: `Instances` (empty state), `Create`, `Adopt`, `System Check`, `Dashboard` (future) — centered.

3. **App Header (48px, always) — Global**
   - Left: hamburger (mobile lift drawer), logo, `Cmd+K` command palette trigger (like Odoo.sh search)
   - Right: theme toggle, notifications (event-log bell with count), user avatar placeholder.

4. **Main + EventLog**
   - `main` now has no sidebar offset logic for instance nav; top nav is sticky.
   - Event log becomes a drawer/bottom sheet or stays as right 280px on desktop, hidden on <1024px behind bell.

**Screen type refactor `App.tsx:35`:**
```ts
type GlobalScreen = "create" | "adopt" | "systemcheck" | "dashboard";
type InstanceScreen = "overview" | "databases" | "modules" | "logs" | "terminal" | "update" | "config" | "doctor" | "cron" | "records" | "depgraph" | "scaffold" | "clone" | "modelinspector";
type Screen = GlobalScreen | InstanceScreen;
// helper: isInstanceScreen(s: Screen) => InstanceScreen[]
```
- Keep single `screen` state but validate: when `selected==null` force `GlobalScreen`, when `selected` changes auto-reset to `overview`.

---

## 3. Visual Design (Odoo.sh + Polaris tokens)

**Top Instance Nav `src/styles/design-tokens.css` + `src/styles.css`:**
- `height: 44px`, `bg: --surface-1`, `border-bottom: 1px solid --border-1`, `position: sticky; top: var(--header-height)`
- Tabs: `font-medium text-sm`, `px-3 py-2`, `border-bottom: 2px solid transparent` → active: `border-color: --brand-500`, `text: --text-0`. Hover: `bg --surface-2`.
- Scrollable with `scrollbar hidden`, left/right fade, `overflow-x: auto`, `snap`.
- Badge counts inside tab (e.g., `Databases 3`).
- Mobile: horizontal scroll, active tab indicator animates (`framer-motion` layoutId).

**Lift Item `gui/frontend/src/App.tsx:148`:**
- `height 56px`, `px-3 gap-3`, `rounded-lg mx-2`, `hover: --surface-2`, `active: --brand-50 border`.
- Status dot `8px` with `box-shadow` glow (existing `.sidebar-health-dot` kept).
- No nested buttons. Hover reveals `Dropdown` (⋯) with `Start/Stop/Restart` to fix a11y button-in-button violation.

**Buttons modernization:**
- Replace `.btn/.qa-btn` with `components/atoms/Button.tsx` (cva variants: `primary, secondary, ghost, outline, destructive, success` × `sm/md/lg/icon` + `loading` + `iconLeft/Right` using `lucide-react`).
- `InstanceDetail` Process card: `SplitButton` (primary action `Start`/`Restart` + chevron dropdown for other).
- `Danger Zone` → `AlertDialog` (Radix) with destructive variant.
- All icon-only buttons have `aria-label`, `tooltip`.

---

## 4. Component / File Plan

**New files:**
- `src/components/atoms/Button.tsx` (cva + Radix Slot) — replaces `.btn` usages in `App.tsx:728`, `Screens.tsx`
- `src/components/atoms/Badge.tsx`, `StatusDot.tsx`
- `src/components/molecules/InstanceNavTabs.tsx` — top nav tabs, scroll, indicator, Radix Tabs primitive
- `src/components/molecules/SearchInput.tsx` (for lift filter)
- `src/components/organisms/InstanceLift.tsx` (extracted from `App.tsx:406` sidebar-section)
- `src/components/organisms/InstanceTopNav.tsx` (new top nav, holds tabs + actions)
- `src/components/organisms/AppHeader.tsx` (global header with Cmd+K, theme, bell)
- `src/layouts/AppShell.tsx` (composes `Header + Lift + TopNav + Main + EventDrawer`)
- `src/hooks/useInstanceNav.ts` (screen grouping, validation, keyboard 1-9 mapping)
- Move large screen components out of `App.tsx` (>1400 lines) → `src/screens/InstanceOverview.tsx`, `DatabasesScreen.tsx` etc., re-export via `src/screens/index.ts` to keep `App.tsx` <400 lines.

**Modified files:**
- `src/App.tsx:35` → new `Screen` types + `useInstanceNav`; `App.tsx:398` → replace sidebar Screens section with `<InstanceLift>`; `App.tsx:386` → replace topbar with `<AppHeader>` + `<InstanceTopNav>` conditional; `App.tsx:445` → delete Screens mapping; `App.tsx:617` → `InstanceDetail` trimmed (remove Operations card nav buttons, keep only overview stats); `App.tsx:728` → new Button variants.
- `src/Screens.tsx` → split, no change in logic, just imports new Button.
- `src/styles.css` → remove `.sidebar-item`, `.topbar-hints`, `.action-card` old nav styles; add `.instance-lift`, `.instance-topnav`, `.instance-topnav-tab`, `.app-header`.
- `gui/app.go` + `internal/service/service.go` — no change (already exposes `CreateConf`, `Clone`, etc. for Instances nav).
- `gui/build/config.yml:47` — already fixed `task build`, keep.

**State & Routing:**
- Keep `selected: string|null` + `screen: Screen` in `App.tsx` but add effect:
  ```ts
  useEffect(()=>{ if(selected && isGlobalScreen(screen)) setScreen("overview"); if(!selected && isInstanceScreen(screen)) setScreen("create"); }, [selected])
  ```
- Persist `selected` in `localStorage` (Odoo.sh remembers last project).
- URL hash optional (`#inst=myapp&tab=databases`) for deep link, no full router needed.

**Keyboard:**
- `Cmd/Ctrl+K` → command palette (already planned) includes `Go to Databases for <instance>`
- `1-9` → top nav tabs (not sidebar screens)
- `g i` → lift search focus, `Esc` → clear selection

---

## 5. Implementation Phases (2 weeks)

**Phase 1 — Lift + Tokens (2 days):**
- Create `InstanceLift.tsx`, `SearchInput.tsx`, `StatusDot.tsx`, `Button.tsx` with Storybook.
- Update `design-tokens.css` already present → add `--header-height`, `--topnav-height`.

**Phase 2 — Top Nav Shell (3 days):**
- Create `AppHeader.tsx`, `InstanceTopNav.tsx` (Radix Tabs, scroll, dropdown for More).
- Refactor `App.tsx` layout: `<AppShell>` with lift (left) + header (top global) + topnav (sticky under header) + main. Delete `App.tsx:426` Screens section. Add `useInstanceNav` hook.

**Phase 3 — Button & Card Polish (2 days):**
- Replace all `.btn` in `App.tsx:722` and `Screens.tsx` with `<Button>` variants + `lucide-react` icons (`Play`, `Square`, `RotateCw`, `Database`, `Trash2`).
- Add `SplitButton` for Process actions, `AlertDialog` for Remove/Drop.

**Phase 4 — Responsive & A11y (2 days):**
- Lift → drawer on <768px (`Dialog` overlay), top nav → horizontal scroll with arrows, event-log → bottom sheet.
- Audit: `aria-current="page"` on active top tab, `aria-selected` on lift, focus trap, contrast 4.5:1.

**Phase 5 — Cleanup & Docs (1 day):**
- Delete dead `.sidebar-item`, `.topbar-hints` CSS, update `Taskfile.yml` `build` task already fixed, bump `build/config.yml` version to `0.14.0`, rebuild `0.14.0` deb.

---

## 6. Risks & Mitigations

- **Too many instance tabs overflow** → scroll + `More` dropdown (Radix) + counts hidden on <1024px.
- **Button-in-button a11y** already fixed by removing `qa-btn` inside `sidebar-item` button → use div + dropdown.
- **State sync** `selected` + `screen` could desync → `useInstanceNav` guard.
- **Tailwind migration** already partially done (`@tailwind base/components/utilities` + tokens) → keep old `.sidebar` styles until new lift/topnav fully replace, then delete.

---

## 7. Deliverable

`NAV_REFACTOR_PLAN.md` (this file) + incremental PRs per phase. When approved, start Phase 1 with `InstanceLift` extraction (no visual regression — lift looks identical, just isolated).
