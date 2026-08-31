# Sprint 2: Health, Metrics & Structured Logs
**Target**: v0.8.0 | **Theme**: Observability & Reliability

---

## 📋 Sprint 2 Goals

| Feature | Impact | Effort |
|---------|--------|--------|
| **Admin HTTP Server** (`/health`, `/metrics`, `/logs`) | 🔴 Critical | M |
| **Prometheus `/metrics` endpoint** | 🔴 Critical | S |
| **Health badge in sidebar** | 🟡 High | S |
| **Structured JSON logging** | 🟡 High | M |
| **Regex log search + saved queries** | 🟡 High | M |
| **Metrics sparklines in Instance Detail** | 🟢 Medium | M |

---

## 🔧 Backend Changes

### 1. Admin HTTP Server (`internal/proc/proc.go`)

Add lightweight HTTP server on `longpolling_port + 1` (or dedicated admin port):

```go
// AdminServer exposes health, metrics, logs for monitoring
type AdminServer struct {
    mux      *http.ServeMux
    server   *http.Server
    inst     *instance.Instance
    mgr      *Manager
    logPath  string
}

func (s *AdminServer) Start() error {
    s.mux = http.NewServeMux()
    s.mux.HandleFunc("/health", s.handleHealth)
    s.mux.HandleFunc("/metrics", s.handleMetrics)
    s.mux.HandleFunc("/logs", s.handleLogs)
    
    s.server = &http.Server{
        Addr:    fmt.Sprintf("127.0.0.1:%d", s.inst.LongpollPort+1),
        Handler: s.mux,
    }
    return s.server.ListenAndServe()
}

func (s *AdminServer) handleHealth(w http.ResponseWriter, r *http.Request) {
    status, pid, _ := s.mgr.Status()
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]any{
        "status":  status.String(),
        "pid":     pid,
        "uptime":  time.Since(s.startTime).Seconds(),
        "version": s.inst.Version,
    })
}

func (s *AdminServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
    // Prometheus format
    w.Header().Set("Content-Type", "text/plain; version=0.0.4")
    fmt.Fprintf(w, `odoo_uptime_seconds %.2f\n`, time.Since(s.startTime).Seconds())
    fmt.Fprintf(w, `odoo_workers %d\n`, s.inst.Workers)
    fmt.Fprintf(w, `odoo_requests_total %d\n`, s.requestCount)
    fmt.Fprintf(w, `odoo_db_connections %d\n`, s.dbPool.Stat().AcquiredConns())
    // ... more metrics
}

func (s *AdminServer) handleLogs(w http.ResponseWriter, r *http.Request) {
    // Support ?format=json&since=timestamp&level=error&limit=100
    // Returns JSON lines or plain text
}
```

### 2. Prometheus Metrics (`internal/proc/proc.go`)

Use `github.com/prometheus/client_golang`:

```go
import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    httpRequestsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "odoo_http_requests_total",
            Help: "Total HTTP requests",
        },
        []string{"method", "path", "status"},
    )
    dbQueryDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "odoo_db_query_duration_seconds",
            Help:    "Database query latency",
            Buckets: prometheus.DefBuckets,
        },
        []string{"query_type"},
    )
    workerBusy = promauto.NewGauge(
        prometheus.GaugeOpts{
            Name: "odoo_workers_busy",
            Help: "Number of busy workers",
        },
    )
)
```

### 3. Service Layer: SearchLogs (`internal/service/ops.go`)

```go
// SearchLogs searches instance logs with regex, level filter, time range
func (s *Service) SearchLogs(name, query string, regex bool, level string, since int64, limit int) ([]LogMatch, error) {
    inst, err := s.reg.Get(name)
    if err != nil { return nil, err }
    p := inst.ResolvePaths(s.rootFor(inst))
    
    // Use logmon with enhanced search
    return logmon.Search(p.Log, logmon.SearchOptions{
        Query:   query,
        Regex:   regex,
        Level:   level,
        Since:   since,
        Limit:   limit,
    })
}

// LogMatch represents a matched log line with context
type LogMatch struct {
    LineNo   int    `json:"lineNo"`
    Timestamp string `json:"timestamp"`
    Level    string `json:"level"`
    Message  string `json:"message"`
    Context  []string `json:"context,omitempty"` // ±2 lines
}
```

### 4. Structured JSON Logging (`internal/logmon/logmon.go`)

```go
// ParseLogLine parses a single log line into structured fields
func ParseLogLine(line string) (LogEntry, bool) {
    // Odoo log format: 2026-08-28 12:34:56,789 12345 INFO db module: message
    // Return structured entry with: timestamp, pid, level, db, module, message
}
```

---

## 🎨 Frontend Changes

### 1. Health Badge in Sidebar (`gui/frontend/src/App.tsx`)

```tsx
// In SidebarItem - add health indicator
const healthColor = statuses[inst.name]?.Instance?.status === "running" 
  ? "var(--success)" 
  : "var(--fg-dim)";

<span 
  className="health-dot" 
  style={{ background: healthColor }}
  title={statuses[inst.name]?.Instance?.status || "unknown"}
/>
```

### 2. Metrics Sparklines in Instance Detail (`gui/frontend/src/App.tsx`)

```tsx
// New component: MetricsSparkline
function MetricsSparkline({ name }: { name: string }) {
  const [metrics, setMetrics] = useState<MetricPoint[]>([]);
  
  useEffect(() => {
    // Poll /metrics every 10s
    const fetch = async () => {
      try {
        const res = await fetch(`http://localhost:${port+1}/metrics`);
        const text = await res.text();
        // Parse Prometheus format, extract key metrics
        setMetrics(parseMetrics(text));
      } catch {}
    };
    fetch();
    const t = setInterval(fetch, 10000);
    return () => clearInterval(t);
  }, [name]);
  
  return (
    <svg className="sparkline" viewBox="0 0 100 30">
      <polyline 
        fill="none" 
        stroke="var(--success)" 
        strokeWidth="1.5"
        points={metrics.map((m, i) => `${i*5},${30-m.value*30}`).join(" ")}
      />
    </svg>
  );
}
```

### 3. Enhanced Logs Screen (`gui/frontend/src/Screens.tsx` → `LogsScreen`)

```tsx
function LogsScreen({ name }: { name: string }) {
  const [regexMode, setRegexMode] = useState(false);
  const [savedQueries, setSavedQueries] = useState<string[]>([]);
  const [selectedQuery, setSelectedQuery] = useState<string>("");
  
  // Search with backend regex support
  const handleSearch = async () => {
    const matches = await SearchLogs(name, search, regexMode, levelFilter, 0, 500);
    setLines(matches.map(m => m.Message));
  };
  
  // Saved queries dropdown
  return (
    <div className="logs-toolbar">
      <select value={selectedQuery} onChange={e => { setSelectedQuery(e.target.value); setSearch(e.target.value); }}>
        <option value="">Saved queries…</option>
        {savedQueries.map(q => <option key={q} value={q}>{q}</option>)}
      </select>
      <button onClick={() => { /* save current query */ }}>💾 Save</button>
      <label><input type="checkbox" checked={regexMode} onChange={e => setRegexMode(e.target.checked)} /> Regex</label>
    </div>
  );
}
```

---

## 📦 New Dependencies

```bash
go get github.com/prometheus/client_golang
```

---

## 📁 Files to Modify

| File | Changes |
|------|---------|
| `internal/proc/proc.go` | Add `AdminServer`, `/health`, `/metrics`, `/logs` handlers |
| `internal/service/ops.go` | Add `SearchLogs`, structured log parsing |
| `internal/logmon/logmon.go` | Add `Search`, `ParseLogLine` |
| `gui/app.go` | Bind `SearchLogs` |
| `gui/frontend/src/App.tsx` | Health badge, metrics sparkline component |
| `gui/frontend/src/Screens.tsx` | Enhanced `LogsScreen` with regex, saved queries |
| `gui/frontend/src/styles.css` | Health dot, sparkline, logs toolbar styles |
| `gui/linux/nfpm/nfpm.yaml` | Version `0.8.0` |

---

## ✅ Definition of Done

- [ ] `/health` returns JSON with status, pid, uptime
- [ ] `/metrics` scrapable by Prometheus (test with `curl`)
- [ ] `/logs?format=json` returns structured JSON lines
- [ ] Sidebar shows green/red health dot per instance
- [ ] Instance Detail shows uptime, request rate, DB connections sparklines
- [ ] Logs screen: regex toggle, saved queries dropdown, level filter
- [ ] All tests pass, deb builds clean

---

## 🎯 Next: Sprint 3 (Module Diff + Model Inspector)

After Sprint 2, focus on developer productivity:
- `ModuleDiff`: compare installed modules vs disk manifests
- `ModelInfo`: field explorer for any model
- Odoo Shell/REPL terminal in GUI