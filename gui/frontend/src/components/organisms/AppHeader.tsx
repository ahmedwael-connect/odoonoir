import React, { useEffect, useState, useCallback } from "react"
import { Command, Bell, Moon, Sun, Menu, GitBranch, Info, ExternalLink } from "lucide-react"
import { Button } from "../atoms/Button"

function GitHubTokenBadge() {
  const [status, setStatus] = useState<"unknown"|"saved"|"valid"|"invalid"|"missing">("unknown")
  useEffect(() => {
    import("../../../bindings/github.com/ahmed/odoonoir/gui/app").then(async (m) => {
      try { const t = await m.GetGitHubToken(); if (!t) setStatus("missing"); else { setStatus("saved"); try { await m.ValidateGitHubToken(); setStatus("valid") } catch { setStatus("invalid") } } } catch { setStatus("unknown") }
    }).catch(()=>{})
    const h = () => setStatus("unknown")
    window.addEventListener("odoonoir-token-changed" as any, h)
    return () => window.removeEventListener("odoonoir-token-changed" as any, h)
  }, [])
  const color = status==="valid" ? "bg-success text-success-foreground" : status==="saved" ? "bg-primary/20 text-primary" : status==="invalid" ? "bg-destructive text-destructive-foreground" : "bg-muted text-muted-foreground"
  const title = status==="valid" ? "GitHub token valid" : status==="saved" ? "GitHub token saved (not validated)" : status==="invalid" ? "GitHub token invalid" : status==="missing" ? "No GitHub token" : "GitHub token unknown"
  return (
    <span title={title} className={`hidden sm:inline-flex items-center gap-1 text-xs border rounded-full px-2 py-0.5 ${color}`}>
      <GitBranch className="h-3 w-3" /> {status==="valid"?"GH ✓":status==="saved"?"GH •":status==="invalid"?"GH ✗":status==="missing"?"GH —":"GH"}
    </span>
  )
}

const CHANGELOG = [
  { version: "0.24.0", date: "2026-09-09", changes: ["Error toasts with hints", "Skeleton card/grid variants", "Global keyboard shortcuts (⌘1-7, ⌘D, ⌘N)", "Version check on startup", "About dialog with changelog", "Responsive toast on mobile", "Dashboard grid responsive"] },
  { version: "0.23.0", date: "2026-09-09", changes: ["Test coverage expanded to 202 tests", "DB mock for unit testing without PostgreSQL", "Command palette tests", "DeployScreen tests"] },
  { version: "0.22.0", date: "2026-09-09", changes: ["Dashboard N+1 fix — batch DB sizes", "BrowseRecords single odoo shell", "Memoize sparklines", "DepGraph debounce filter", "Statuses batch endpoint"] },
  { version: "0.21.0", date: "2026-09-09", changes: ["DeployScreen with Docker Compose generation", "Command Palette (⌘K)", "Config editor with categories", "Terminal auto-reconnect", "Log search with regex"] },
  { version: "0.20.1", date: "2026-09-09", changes: ["Error handling hardening", "Enterprise checkout errors returned", "All conf.Save() calls return errors", "ScreenBoundary crash isolation"] },
  { version: "0.20.0", date: "2026-09-09", changes: ["Security: Python injection fix", "Security: Shell command injection fix", "Security: WebSocket CSRF protection", "Security: Marketplace deadlock fix", "Security: Data race fixes"] },
  { version: "0.19.0", date: "2026-09-08", changes: ["Performance profiler (py-spy)", "Dashboard with live metrics", "Module scaffold Pro", "Terminal xterm integration", "Database discovery"] },
]

export function AppHeader({
  instanceCount,
  eventCount,
  onToggleLift,
  onOpenPalette,
  onToggleEventLog,
}: {
  instanceCount: number
  eventCount: number
  onToggleLift: () => void
  onOpenPalette: () => void
  onToggleEventLog: () => void
}) {
  const [showAbout, setShowAbout] = useState(false)
  const [version, setVersion] = useState("")
  const [updateAvailable, setUpdateAvailable] = useState(false)

  useEffect(() => {
    import("../../../bindings/github.com/ahmed/odoonoir/gui/app").then(async (m) => {
      try {
        const v: any[] = await m.CheckVersion("")
        if (v?.length) {
          const latest = v[0]?.found || ""
          setVersion(latest)
          const hasUpdate = v.some((r: any) => r.severity === "warning")
          setUpdateAvailable(hasUpdate)
        }
      } catch {}
    }).catch(() => {})
  }, [])

  return (
    <>
    <header className="h-[56px] flex items-center gap-3 px-4 border bg-card sticky top-0 z-30 rounded-[20px] shadow-sm">
      <Button variant="ghost" size="icon" className="lg:hidden" onClick={onToggleLift} aria-label="Toggle lift">
        <Menu className="h-5 w-5" />
      </Button>
      <div className="font-bold tracking-tight">OdooNoir</div>
      <button onClick={()=>window.dispatchEvent(new CustomEvent("odoonoir-nav-dashboard"))} className="hidden sm:inline text-xs text-muted-foreground border rounded-full px-2 py-0.5 hover:bg-accent hover:text-foreground transition-colors">
        {instanceCount} instance{instanceCount !== 1 ? "s" : ""} • Dashboard
      </button>

      <button
        onClick={onOpenPalette}
        className="hidden md:flex items-center gap-2 ml-6 bg-muted/60 backdrop-blur border rounded-[14px] px-4 py-2 text-sm text-muted-foreground hover:text-foreground hover:bg-accent transition-all flex-1 max-w-[320px] shadow-sm hover:shadow-md"
        aria-label="Open command palette"
      >
        <Command className="h-4 w-4" /> <span className="flex-1 text-left font-medium">Search or jump…</span> <kbd className="hidden lg:inline bg-background border rounded-[8px] px-2 py-0.5 text-xs font-mono">⌘K</kbd>
      </button>

      <div className="ml-auto flex items-center gap-1">
        <GitHubTokenBadge />
        {updateAvailable && (
          <span className="hidden sm:inline-flex items-center gap-1 text-xs bg-warning/20 text-warning border border-warning/30 rounded-full px-2 py-0.5 cursor-pointer" onClick={() => setShowAbout(true)} title="Update available">
            ↑ Update
          </span>
        )}
        <Button variant="ghost" size="icon" aria-label="About" onClick={() => setShowAbout(true)} title="About OdooNoir">
          <Info className="h-4 w-4" />
        </Button>
        <Button variant="ghost" size="icon" aria-label="Toggle theme" onClick={() => document.documentElement.classList.toggle("light")} title="Toggle theme">
          <Sun className="h-4 w-4 hidden dark:block" />
          <Moon className="h-4 w-4 block dark:hidden" />
          <Sun className="h-4 w-4 block light:hidden dark:hidden" />
        </Button>
        <Button variant="ghost" size="icon" onClick={onToggleEventLog} aria-label="Toggle event log" className="relative">
          <Bell className="h-4 w-4" />
          {eventCount > 0 && <span className="absolute -top-1 -right-1 bg-primary text-primary-foreground text-[10px] leading-none rounded-full px-1.5 py-0.5">{Math.min(eventCount, 99)}</span>}
        </Button>
      </div>
    </header>

    {showAbout && (
      <div className="fixed inset-0 z-50 flex items-center justify-center" onClick={() => setShowAbout(false)}>
        <div className="fixed inset-0 bg-black/50 backdrop-blur-sm" />
        <div className="relative bg-card border rounded-2xl shadow-2xl max-w-lg w-full mx-4 max-h-[80vh] overflow-hidden" onClick={e => e.stopPropagation()}>
          <div className="p-6">
            <div className="flex items-center gap-3 mb-4">
              <div className="w-10 h-10 rounded-xl bg-primary/10 flex items-center justify-center text-lg font-bold text-primary">ON</div>
              <div>
                <h2 className="text-lg font-bold">OdooNoir</h2>
                <p className="text-xs text-muted-foreground">v{version || "0.23.0"} • Developer toolkit for Odoo</p>
              </div>
            </div>
            <p className="text-sm text-muted-foreground mb-4">
              A desktop GUI and CLI for managing Odoo 15–19 instances. Install, develop, scaffold modules, manage databases, and deploy — all from one app.
            </p>
            <div className="flex gap-2 mb-4">
              <a href="https://github.com/ahmedwael-connect/odoonoir" target="_blank" rel="noopener" className="inline-flex items-center gap-1 text-xs border rounded-lg px-3 py-1.5 hover:bg-accent transition-colors">
                <ExternalLink className="h-3 w-3" /> GitHub
              </a>
              {updateAvailable && (
                <span className="inline-flex items-center gap-1 text-xs bg-warning/20 text-warning border border-warning/30 rounded-lg px-3 py-1.5">
                  ↑ Update available
                </span>
              )}
            </div>
          </div>
          <div className="border-t max-h-[50vh] overflow-y-auto">
            <div className="p-4">
              <h3 className="text-sm font-semibold mb-3">Changelog</h3>
              {CHANGELOG.map(entry => (
                <div key={entry.version} className="mb-4 last:mb-0">
                  <div className="flex items-center gap-2 mb-1">
                    <span className="text-sm font-mono font-bold">v{entry.version}</span>
                    <span className="text-xs text-muted-foreground">{entry.date}</span>
                  </div>
                  <ul className="text-xs text-muted-foreground space-y-0.5 ml-1">
                    {entry.changes.map((c, i) => (
                      <li key={i}>• {c}</li>
                    ))}
                  </ul>
                </div>
              ))}
            </div>
          </div>
          <div className="border-t p-3 flex justify-end">
            <Button variant="ghost" size="sm" onClick={() => setShowAbout(false)}>Close</Button>
          </div>
        </div>
      </div>
    )}
    </>
  )
}
