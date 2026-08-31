# OdooNoir v0.11 & v0.12 Roadmap: UI/UX Overhaul + GitHub Module Support

---

## 🎯 v0.11: "Polaris" — Massive UI/UX Overhaul
**Theme**: Modern, accessible, responsive, developer-first design system  
**Target**: Q1 2025 | **Scope**: Complete frontend rewrite with new design system

---

## 🎨 Design System v2.0

### Color Palette (Semantic Tokens)
```css
:root {
  /* Brand */
  --brand-50: #f0fdf4;   --brand-500: #22c55e;   --brand-900: #14532d;
  --brand-50: #f0fdf4;   --brand-500: #22c55e;   --brand-900: #14532d;
  
  /* Neutral (Dark-first) */
  --bg-0: #0a0c10;       --bg-100: #111318;      --bg-200: #1a1d24;
  --bg-300: #242832;     --bg-400: #2d3240;
  
  /* Surface */
  --surface-1: #151820;  --surface-2: #1e2128;   --surface-3: #2a2e38;
  
  /* Border */
  --border-1: #1e2128;   --border-2: #2a2e38;    --border-3: #3b82f6;  --focus
  
  /* Text */
  --text-0: #ffffff;     --text-1: #e0e0e0;      --text-2: #a0a8b8;
  --text-3: #6b7280;     --text-4: #4b5563;
  
  /* Semantic */
  --success: #34d399;    --success-bg: #064e3b;  --success-border: #065f46;
  --warning: #fbbf24;    --warning-bg: #443300;  --warning-border: #854d0e;
  --danger: #fca5a5;     --danger-bg: #450a0a;   --danger-border: #7f1d1d;
  --info: #60a5fa;       --info-bg: #1e3a5f;     --info-border: #3b82f6;
  
  /* Spacing (4px grid) */
  --space-1: 4px; --space-2: 8px; --space-3: 12px; --space-4: 16px;
  --space-5: 20px; --space-6: 24px; --space-8: 32px; --space-10: 40px;
  --space-12: 48px; --space-16: 64px;
  
  /* Radii */
  --radius-sm: 4px; --radius-md: 8px; --radius-lg: 12px; --radius-xl: 16px;
  --radius-full: 9999px;
  
  /* Shadows */
  --shadow-sm: 0 1px 2px rgba(0,0,0,0.3);
  --shadow-md: 0 4px 12px rgba(0,0,0,0.4);
  --shadow-lg: 0 8px 24px rgba(0,0,0,0.5);
  --shadow-xl: 0 16px 48px rgba(0,0,0,0.6);
  
  /* Transitions */
  --ease: cubic-bezier(0.16, 1, 0.3, 1);
  --duration-fast: 100ms; --duration-normal: 200ms; --duration-slow: 300ms;
  
  /* Typography */
  --font-sans: "Inter", "JetBrains Mono", system-ui, sans-serif;
  --font-mono: "JetBrains Mono", "Fira Code", monospace;
  --text-xs: 0.6875rem; --text-sm: 0.8125rem; --text-base: 0.875rem;
  --text-lg: 1rem; --text-xl: 1.125rem; --text-2xl: 1.25rem;
  --text-3xl: 1.5rem; --text-4xl: 2rem;
  
  /* Layout */
  --sidebar-w: 280px; --sidebar-collapsed: 64px;
  --header-h: 56px; --footer-h: 40px;
  --container-max: 1400px;
}
```

### Light Mode (Auto/Manual)
```css
@media (prefers-color-scheme: light) {
  :root:not(.dark) {
    --bg-0: #ffffff; --bg-100: #f8fafc; --bg-200: #f1f5f9;
    --bg-300: #e2e8f0; --bg-400: #cbd5e1;
    --surface-1: #ffffff; --surface-2: #f8fafc; --surface-3: #f1f5f9;
    --border-1: #e2e8f0; --border-2: #cbd5e1;
    --text-0: #0f172a; --text-1: #1e293b; --text-2: #475569;
    --text-3: #64748b; --text-4: #94a3b8;
    --success: #16a34a; --success-bg: #dcfce7;
    --warning: #ca8a04; --warning-bg: #fef9c3;
    --danger: #ef4444; --danger-bg: #fef2f2;
    --info: #2563eb; --info-bg: #dbeafe;
  }
}
```

---

## 🧩 Component Library (Atomic Design)

### Atoms
| Component | Variants | States |
|-----------|----------|--------|
| `Button` | primary, secondary, ghost, danger, success, outline | default, hover, active, disabled, loading |
| `Input` | text, password, search, number | default, focus, error, disabled, filled |
| `Select` | single, multi, searchable | default, focus, error, disabled |
| `Checkbox` / `Radio` / `Switch` | — | default, checked, indeterminate, disabled |
| `Badge` | neutral, success, warning, danger, info | default, dot, removable |
| `Avatar` | xs, sm, md, lg, xl | default, online, offline, busy |
| `Icon` | 24px grid | default, muted, interactive |
| `Tooltip` / `Popover` | top, bottom, left, right | — |
| `Spinner` / `Skeleton` | sm, md, lg | — |

### Molecules
| Component | Description |
|-----------|-------------|
| `SearchInput` | Input + clear + loading + recent searches |
| `FilterChips` | Removable filter pills with counts |
| `DataTable` | Sortable, filterable, paginated, selectable rows |
| `Card` | Header, body, footer; elevated, outlined, filled |
| `Tabs` | Line, enclosed, soft; keyboard navigation |
| `DropdownMenu` | Groups, dividers, icons, shortcuts, separators |
| `Breadcrumb` | Collapsible, icons, current page |
| `Progress` | Linear, circular, indeterminate, labeled |
| `Alert` | Inline, toast, banner; dismissible, actions |
| `EmptyState` | Illustration, title, description, primary action |
| `ConfirmDialog` | Title, message, danger variant, loading state |

### Organisms
| Component | Description |
|-----------|-------------|
| `Sidebar` | Collapsible, sections, badges, keyboard shortcuts |
| `Header` | Global search, notifications, user menu, theme toggle |
| `InstanceList` | Virtualized, grouped, drag-reorder, quick actions |
| `InstanceDetail` | Tabbed: Overview, Metrics, Logs, Config, Backups |
| `ModuleManager` | Tabs: List, Diff, Graph; bulk actions |
| `LogViewer` | Virtualized, regex search, level filter, follow tail |
| `RecordBrowser` | Toolbar, columns, inline edit, bulk actions |
| `DepGraph` | Force-directed, zoom/pan, node detail drawer |
| `Dashboard` | Grid layout, widgets, drag-resize, persist layout |
| `SettingsPanel` | Sections, search, dirty tracking, Ctrl+S save |

---

## 📐 Layout System

### Responsive Breakpoints
```css
--bp-xs: 480px;   --bp-sm: 640px;   --bp-md: 768px;
--bp-lg: 1024px;  --bp-xl: 1280px;  --bp-2xl: 1536px;
```

### Layout Patterns
```tsx
// App Shell
<AppShell>
  <Sidebar />
  <Main>
    <Header />
    <Content />
    <Footer />
  </Main>
  <RightDrawer />
  <EventLog />
</AppShell>

// Page Layout
<PageLayout>
  <PageHeader title="" actions={} breadcrumbs={} />
  <PageContent>
    <Grid columns={12} gap={4}>
      <Card colSpan={8} />
      <Card colSpan={4} />
    </Grid>
  </PageContent>
</PageLayout>
```

---

## ♿ Accessibility (WCAG 2.1 AA)

- **Keyboard**: Full tab order, focus visible, skip links, escape to close
- **Screen Readers**: ARIA labels, live regions, landmarks, headings
- **Color**: 4.5:1 contrast, not color-only info
- **Motion**: Respect `prefers-reduced-motion`
- **Focus Management**: Trap in modals, restore on close

---

## 🎭 Animation & Micro-interactions

| Interaction | Animation |
|-------------|-----------|
| Page transition | Fade + slide (200ms) |
| Sidebar collapse | Width + opacity (200ms) |
| Modal open | Scale + fade (150ms) |
| Toast appear | Slide up + fade (200ms) |
| Hover card | Lift + shadow (150ms) |
| Button press | Scale 0.98 (50ms) |
| Tab switch | Cross-fade (100ms) |
| Skeleton → content | Shimmer → fade (300ms) |

---

## 🛠 Technical Implementation

### Stack
- **React 18** + **TypeScript 5** + **Vite 5**
- **Tailwind CSS 3.4** (JIT, custom design tokens)
- **Radix UI** (headless accessible primitives)
- **Framer Motion** (animations)
- **TanStack Query** (server state)
- **Zustand** (client state)
- **React Hook Form** + **Zod** (forms)
- **D3.js** (visualizations)
- **xterm.js** (terminal)

### Project Structure
```
gui/frontend/src/
├── design-system/          # Design tokens, theme provider
├── components/
│   ├── atoms/              # Button, Input, Badge, etc.
│   ├── molecules/          # SearchInput, DataTable, Card, etc.
│   ├── organisms/          # Sidebar, InstanceList, Dashboard, etc.
│   └── patterns/           # PageLayout, FormLayout, etc.
├── hooks/                  # useTheme, useBreakpoint, useDebounce, etc.
├── screens/                # Route-level components
├── services/               # API client, WebSocket, storage
├── stores/                 # Zustand stores (ui, instances, user)
├── types/                  # Shared TypeScript types
└── utils/                  # Formatters, validators, helpers
```

### CSS Strategy
- **Tailwind** for utilities + **CSS Variables** for theming
- **CSS Modules** for component-scoped styles
- **Zero runtime CSS-in-JS**

---

## 📱 Mobile-First Responsive

| Screen | Sidebar | Layout |
|--------|---------|--------|
| < 640px | Drawer (hamburger) | Stack cards, full-width |
| 640-1024px | Collapsible rail | 2-col grid |
| > 1024px | Full sidebar | 3-col grid, persistent |

---

## 📦 Deliverables v0.11

- [ ] Design system documentation (Storybook)
- [ ] All atom/molecule/organism components
- [ ] Theme provider (light/dark/system)
- [ ] Responsive app shell
- [ ] Migrated screens: Instances, Databases, Modules, Logs, Config, Terminal
- [ ] New screens: Dashboard, Record Browser, DepGraph
- [ ] Keyboard shortcuts + command palette
- [ ] Accessibility audit
- [ ] Visual regression tests (Chromatic)
- [ ] Performance budget (<100KB gzipped CSS, <500KB JS)

---

## 🔮 v0.12: "Forge" — GitHub Module Support

**Theme**: Seamless GitHub integration for module discovery, installation, and development

---

## 🎯 Core Features

### 1. Module Discovery & Search
```
┌─────────────────────────────────────────────────────────────┐
│  🔍  Search GitHub Modules                    [Sort ▼] [★]  │
├─────────────────────────────────────────────────────────────┤
│  ☑  Stars ≥ 10    ☑  Updated < 1yr    ☑  Odoo 17/18/19     │
│  📦  Category: [All ▼]  Language: [Python ▼]               │
├─────────────────────────────────────────────────────────────┤
│  📦  odoo/odoo                    ★ 45k  🍴 18k             │
│     The official Odoo repository. Core modules included.    │
│     [Install] [View] [Fork]  Updated 2h ago  v18.0          │
├─────────────────────────────────────────────────────────────┤
│  📦  OCA/account-financial        ★ 320  🍴 180             │
│     Financial accounting modules for Odoo.                  │
│     [Install] [View] [Fork]  Updated 3d ago  v17.0          │
├─────────────────────────────────────────────────────────────┤
│  📦  akretion/odoo-pos-hr         ★ 45   🍴 12              │
│     HR integration for Point of Sale.                       │
│     [Install] [View] [Fork]  Updated 1w ago  v18.0          │
└─────────────────────────────────────────────────────────────┘
```

### 2. Module Detail View
```
┌─────────────────────────────────────────────────────────────┐
│  OCA/account-financial                         [★ Star]    │
├─────────────────────────────────────────────────────────────┤
│  ★ 320  🍴 180  👁 1.2k  🏷 v18.0  📅 Updated 3d ago        │
│  📝 Financial accounting modules for Odoo                   │
│  🔗 github.com/OCA/account-financial                        │
│  📄 License: LGPL-3.0  🐛 12 open issues  🔀 5 PRs         │
├─────────────────────────────────────────────────────────────┤
│  📦 Modules (12)    📁 Structure    📋 Changelog    🔒 Security│
│  ┌─────────────────────────────────────────────────────────┐ │
│  │ account_move_extended     │ Extends account.move      │ │
│  │ account_payment_sepa      │ SEPA payment support      │ │
│  │ account_bank_statement    │ Bank statement import     │ │
│  │ ...                                                                    │
│  └─────────────────────────────────────────────────────────┘ │
├─────────────────────────────────────────────────────────────┤
│  [Install to Instance ▼]  [Download ZIP]  [Clone Locally]   │
└─────────────────────────────────────────────────────────────┘
```

### 3. One-Click Install
```
┌─────────────────────────────────────────────────────────────┐
│  Install OCA/account-financial                              │
├─────────────────────────────────────────────────────────────┤
│  Target Instance: [myapp ▼]                                 │
│  Database:        [myapp_db ▼]                              │
│  Branch:          [18.0 ▼]  (auto-detect from instance)     │
│  ☑  Auto-resolve dependencies                               │
│  ☑  Run tests after install                                 │
│  ☐  Enable auto-updates                                     │
├─────────────────────────────────────────────────────────────┤
│  Dependencies to install:                                   │
│  ☑  base                    (already installed)             │
│  ☑  account                 (already installed)             │
│  ☐  account_fr_rib          (not installed)  [Auto-install] │
│  ☐  l10n_fr                 (not installed)  [Auto-install] │
├─────────────────────────────────────────────────────────────┤
│  [Cancel]                    [Install]                      │
└─────────────────────────────────────────────────────────────┘
```

### 4. Dependency Resolution
- Parse `__manifest__.py` for `depends`
- Check against instance's installed modules
- Fetch missing deps recursively from same org/user
- Show conflict resolution UI

### 5. Module Development Workflow
```
┌─────────────────────────────────────────────────────────────┐
│  🛠  Create New Module                                      │
├─────────────────────────────────────────────────────────────┤
│  Name:           my_custom_module          [Validate]       │
│  Display Name:   My Custom Module                           │
│  GitHub Repo:    myorg/my_custom_module   [Create on GH]    │
│  Template:       [Basic ▼]  [Full ▼]  [Connector ▼]         │
│  Add to Instance: [myapp ▼]                                 │
├─────────────────────────────────────────────────────────────┤
│  [Create Locally]  [Create on GitHub & Clone]  [Cancel]    │
└─────────────────────────────────────────────────────────────┘
```

### 6. Sync & Update
```
┌─────────────────────────────────────────────────────────────┐
│  🔄  Sync with GitHub                                       │
├─────────────────────────────────────────────────────────────┤
│  Instance: myapp                                            │
│  Module:  OCA/account-financial                             │
│  Current: 18.0.1.0.0    →   Latest: 18.0.2.1.0  [▲]        │
│  Changelog:                                                 │
│  • Fix SEPA payment validation                              │
│  • Add Italian localization                                 │
│  • Improve bank statement reconciliation                    │
├─────────────────────────────────────────────────────────────┤
│  ☑  Backup before update    ☑  Run tests    ☐  Dry run     │
│  [Cancel]                    [Update Now]                   │
└─────────────────────────────────────────────────────────────┘
```

### 7. Publish to GitHub
```
┌─────────────────────────────────────────────────────────────┐
│  📤  Publish Module to GitHub                               │
├─────────────────────────────────────────────────────────────┤
│  Module:        my_custom_module                            │
│  Owner:         [myorg ▼]  [Create new org]                 │
│  Repo Name:     my_custom_module                            │
│  Visibility:    ☐ Private  ☑ Public                         │
│  License:       [LGPL-3 ▼]  [MIT]  [AGPL-3]  [OPL-1]        │
│  Description:   Custom module for...                        │
│  Topics:        odoo, odoo-18, custom-module                │
│  ☑  Initialize with README, LICENSE, .gitignore            │
│  ☑  Add GitHub Actions (lint, test, release)               │
├─────────────────────────────────────────────────────────────┤
│  [Cancel]                    [Publish]                      │
└─────────────────────────────────────────────────────────────┘
```

---

## 🔐 Authentication & Security

### GitHub OAuth Flow
```
┌─────────────────────────────────────────────────────────────┐
│  🔐  Connect GitHub Account                                 │
├─────────────────────────────────────────────────────────────┤
│  OdooNoir needs access to:                                  │
│  ☑  Read public repositories                                │
│  ☑  Read private repositories (for install)                │
│  ☑  Write repositories (for publish)                       │
│  ☑  Read user/org membership                                │
│  ☐  Admin repo access (optional)                            │
│                                                             │
│  Scopes: repo, read:org, user:email                         │
│  [Cancel]                    [Authorize]                    │
└─────────────────────────────────────────────────────────────┘
```

### Token Management
- Store encrypted in keyring/OS credential store
- Auto-refresh with refresh token
- Per-instance GitHub token override
- Audit log of GitHub actions

---

## 🏗 Technical Architecture

### Backend (Go)
```go
// GitHub Service
type GitHubService struct {
    client *github.Client
    tokenStore TokenStore
}

func (s *GitHubService) SearchModules(query SearchQuery) ([]Module, error)
func (s *GitHubService) GetModule(owner, repo string) (*ModuleDetail, error)
func (s *GitHubService) GetManifest(owner, repo, branch string) (*Manifest, error)
func (s *GitHubService) ResolveDependencies(manifest *Manifest) ([]Dependency, error)
func (s *GitHubService) InstallModule(inst *Instance, spec ModuleSpec) error
func (s *GitHubService) PublishModule(localPath string, opts PublishOptions) error
func (s *GitHubService) SyncModule(inst *Instance, spec ModuleSpec) error
```

### Frontend API
```typescript
// GitHub API Client
interface GitHubAPI {
  searchModules(query: SearchQuery): Promise<ModuleSearchResult>;
  getModule(owner: string, repo: string): Promise<ModuleDetail>;
  getManifest(owner: string, repo: string, branch: string): Promise<Manifest>;
  installModule(instance: string, spec: InstallSpec): Promise<InstallResult>;
  publishModule(localPath: string, opts: PublishOptions): Promise<PublishResult>;
  syncModule(instance: string, spec: SyncSpec): Promise<SyncResult>;
}
```

### Data Models
```typescript
interface Module {
  owner: string;
  repo: string;
  name: string;           // technical name from manifest
  displayName: string;
  description: string;
  stars: number;
  forks: number;
  license: string;
  topics: string[];
  updatedAt: Date;
  defaultBranch: string;
  odooVersions: string[]; // parsed from manifest
}

interface ModuleDetail extends Module {
  manifest: Manifest;
  modules: ModuleInfo[];  // sub-modules in repo
  readme: string;
  issues: number;
  pullRequests: number;
  releases: Release[];
  dependencies: Dependency[];
  dependents: Module[];
}

interface Manifest {
  name: string;
  version: string;
  depends: string[];
  author: string;
  license: string;
  category: string;
  summary: string;
  description: string;
  data: string[];
  demo: string[];
  installable: boolean;
  auto_install: boolean;
}
```

---

## 🎨 UI Components for v0.12

### New Components
| Component | Description |
|-----------|-------------|
| `GitHubSearch` | Debounced search with facets, infinite scroll |
| `ModuleCard` | Preview with stars, license, version badges |
| `ModuleDetailDrawer` | Full detail: readme, modules, deps, changelog |
| `InstallWizard` | Multi-step: select instance → deps → options → confirm |
| `DependencyGraph` | Visual dep resolution, conflicts highlighted |
| `PublishWizard` | Repo creation, license picker, Actions template |
| `SyncDialog` | Version compare, changelog, dry-run toggle |
| `GitHubAuth` | OAuth connect, token status, scopes management |

### Integration Points
- **Modules Screen**: New "GitHub" tab alongside "List"/"Diff"
- **Instance Detail**: "Install from GitHub" button
- **Command Palette**: "Install from GitHub", "Publish Module"
- **Module Scaffold**: "Push to GitHub" button

---

## 📦 Deliverables v0.12

- [ ] GitHub OAuth integration
- [ ] Module search & discovery UI
- [ ] Module detail view with manifest parsing
- [ ] One-click install with dependency resolution
- [ ] Sync/update workflow with changelog
- [ ] Publish wizard (repo creation, Actions)
- [ ] Module scaffold → GitHub push
- [ ] Token management & audit log
- [ ] Rate limit handling & caching
- [ ] Private repo support
- [ ] Org/team filtering

---

## 🗓 Timeline

| Phase | v0.11 (Polaris) | v0.12 (Forge) |
|-------|-----------------|---------------|
| Design | Week 1-2 | Week 1 |
| Core Components | Week 3-5 | Week 2-3 |
| Screen Migration | Week 6-8 | Week 4-5 |
| New Screens | Week 9-10 | Week 6-7 |
| Polish & A11y | Week 11-12 | Week 8 |
| Testing | Week 13 | Week 9 |
| Release | **Week 14** | **Week 10** |

---

## 🎯 Success Metrics

### v0.11
- Lighthouse: Performance >90, Accessibility >95, Best Practices >90
- Bundle size: <500KB JS, <100KB CSS (gzipped)
- Time to Interactive: <2s on 3G
- Zero critical a11y violations

### v0.12
- Module install success rate >95%
- Avg install time <60s (including deps)
- GitHub API error rate <1%
- User satisfaction >4.5/5

---

## 🔄 Migration Strategy

### v0.10 → v0.11
1. Parallel component library (new + legacy)
2. Screen-by-screen migration
3. Feature flags per screen
4. Visual regression baseline
5. Gradual rollout

### v0.11 → v0.12
1. GitHub service as separate module
2. Feature flag: `github_modules`
3. Backward compatible (local modules still work)
3. Progressive enhancement

---

## 💡 Future Considerations (v0.13+)

- **Marketplace**: Curated module registry with ratings
- **CI/CD Integration**: GitHub Actions status in UI
- **Collaborative Dev**: Real-time module editing
- **AI Assist**: Generate models/views from description
- **Multi-cloud**: GitLab, Bitbucket, Gitea support
- **Enterprise**: SSO, RBAC, audit compliance