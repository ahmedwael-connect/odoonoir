import React, { useEffect, useState } from "react"
import { Command, Bell, Moon, Sun, Menu, GitBranch } from "lucide-react"
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
  return (
    <header className="h-[56px] flex items-center gap-3 px-4 border bg-card sticky top-0 z-30 rounded-[20px] shadow-sm">
      <Button variant="ghost" size="icon" className="lg:hidden" onClick={onToggleLift} aria-label="Toggle lift">
        <Menu className="h-5 w-5" />
      </Button>
      <div className="font-bold tracking-tight">OdooNoir</div>
      <span className="hidden sm:inline text-xs text-muted-foreground border rounded-full px-2 py-0.5">
        {instanceCount} instance{instanceCount !== 1 ? "s" : ""}
      </span>

      <button
        onClick={onOpenPalette}
        className="hidden md:flex items-center gap-2 ml-6 bg-muted/60 backdrop-blur border rounded-[14px] px-4 py-2 text-sm text-muted-foreground hover:text-foreground hover:bg-accent transition-all flex-1 max-w-[320px] shadow-sm hover:shadow-md"
        aria-label="Open command palette"
      >
        <Command className="h-4 w-4" /> <span className="flex-1 text-left font-medium">Search or jump…</span> <kbd className="hidden lg:inline bg-background border rounded-[8px] px-2 py-0.5 text-xs font-mono">⌘K</kbd>
      </button>

      <div className="ml-auto flex items-center gap-1">
        <GitHubTokenBadge />
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
  )
}
