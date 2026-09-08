import React from "react"
import { Search, Plus, ChevronLeft, ChevronRight, MoreHorizontal, Play, Square, RotateCw } from "lucide-react"
import * as DropdownMenu from "@radix-ui/react-dropdown-menu"
import { Button } from "../atoms/Button"
import { StatusDot } from "../atoms/Badge"
import type { InstanceView, StatusView } from "../../../bindings/github.com/ahmed/odoonoir/internal/service/models"

function statusHealth(running: boolean) {
  return running ? "var(--success)" : "var(--destructive)"
}

export function InstanceLift({
  instances,
  statuses,
  selected,
  busy,
  initialLoading,
  collapsed,
  onToggle,
  onSelect,
  onAct,
  filter,
  setFilter,
  onCreate,
}: {
  instances: InstanceView[]
  statuses: Record<string, StatusView>
  selected: string | null
  busy: string | null
  initialLoading: boolean
  collapsed: boolean
  onToggle: () => void
  onSelect: (name: string) => void
  onAct: (name: string, op: "start" | "stop" | "restart") => void
  filter: string
  setFilter: (v: string) => void
  onCreate: () => void
}) {
  const filtered = filter
    ? instances.filter(i => i.name.toLowerCase().includes(filter.toLowerCase()) || i.version.includes(filter))
    : instances

  return (
    <>
      {/* backdrop for mobile drawer */}
      {!collapsed && (
        <div
          className="fixed inset-0 bg-black/40 backdrop-blur-sm z-30 lg:hidden"
          onClick={onToggle}
          aria-hidden="true"
        />
      )}
      <aside className={`flex flex-col bg-card overflow-hidden transition-all duration-200 z-40 rounded-[16px] border shadow-sm
        ${collapsed
          ? "w-[64px] min-w-[64px] -translate-x-full lg:translate-x-0 fixed lg:static inset-y-0 left-0 lg:inset-auto"
          : "w-[260px] min-w-[260px] fixed lg:static inset-y-0 left-0 lg:inset-auto translate-x-0 shadow-lg lg:shadow-sm"
        }`} role="navigation" aria-label="Instance lift">
      <div className="flex items-center gap-2 p-3 border-b h-[56px] shrink-0">
        {!collapsed && <div className="font-bold text-sm tracking-tight flex-1">OdooNoir</div>}
        <Button variant="ghost" size="icon-sm" onClick={onToggle} aria-label={collapsed ? "Expand lift" : "Collapse lift"}>
          {collapsed ? <ChevronRight className="h-4 w-4" /> : <ChevronLeft className="h-4 w-4" />}
        </Button>
      </div>

      {!collapsed && (
        <div className="p-3 flex gap-2">
          <div className="relative flex-1">
            <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
            <input
              placeholder="Filter instances…"
              value={filter}
              onChange={e => setFilter(e.target.value)}
              className="w-full bg-muted border border-input rounded-lg pl-8 pr-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              aria-label="Filter instances"
            />
          </div>
          <Button size="icon" variant="primary" onClick={onCreate} aria-label="Create instance">
            <Plus className="h-4 w-4" />
          </Button>
        </div>
      )}

      <div className="flex-1 overflow-y-auto py-2 space-y-1 px-2">
        {!collapsed && <div className="px-2 py-1 text-xs font-semibold uppercase tracking-wider text-muted-foreground">Instances • {instances.length}</div>}
        {initialLoading ? (
          <div className="px-3 py-4 text-sm text-muted-foreground">Loading…</div>
        ) : filtered.length === 0 ? (
          <div className="px-3 py-6 text-center text-sm text-muted-foreground">
            {filter ? "No match" : "No instances yet"}
            {!filter && !collapsed && <div><Button variant="link" size="sm" onClick={onCreate}>Create first</Button></div>}
          </div>
        ) : filtered.map(inst => {
          const running = statuses[inst.name]?.Instance?.status === "running"
          const active = selected === inst.name
          return (
            <div
              key={inst.name}
              role="button"
              tabIndex={0}
              onClick={() => onSelect(inst.name)}
              onKeyDown={e => e.key === "Enter" && onSelect(inst.name)}
              aria-current={active ? "true" : undefined}
              aria-label={`${inst.name} v${inst.version} ${running ? "running" : "stopped"}`}
              className={`group flex items-center gap-3 px-3 py-3 rounded-[16px] cursor-pointer transition-all duration-200 border ${active ? "bg-primary text-primary-foreground border-primary shadow-[0_4px_16px_rgba(52,211,153,0.3)] scale-[1.02]" : "border-transparent hover:bg-accent text-muted-foreground hover:text-foreground hover:shadow-sm hover:scale-[1.01] bg-card/50"}`}
            >
              <StatusDot running={running} healthColor={statusHealth(running)} />
              {!collapsed && (
                <>
                  <div className="flex-1 min-w-0">
                    <div className="text-sm font-medium truncate leading-none">{inst.name}</div>
                    <div className="text-xs text-muted-foreground truncate">v{inst.version} • {inst.dbName}</div>
                  </div>
                  <DropdownMenu.Root>
                    <DropdownMenu.Trigger asChild>
                      <Button variant="ghost" size="icon-sm" className="opacity-0 group-hover:opacity-100 data-[state=open]:opacity-100 h-7 w-7" onClick={e => e.stopPropagation()} aria-label={`Actions for ${inst.name}`}>
                        <MoreHorizontal className="h-4 w-4" />
                      </Button>
                    </DropdownMenu.Trigger>
                    <DropdownMenu.Portal>
                      <DropdownMenu.Content className="min-w-[160px] bg-popover border rounded-lg shadow-lg p-1 z-50" side="right">
                        <DropdownMenu.Item className="flex items-center gap-2 px-3 py-2 text-sm rounded-md hover:bg-accent cursor-pointer" onClick={() => onAct(inst.name, running ? "restart" : "start")} disabled={busy === inst.name}>
                          {running ? <RotateCw className="h-4 w-4" /> : <Play className="h-4 w-4" />} {running ? "Restart" : "Start"}
                        </DropdownMenu.Item>
                        {running && (
                          <DropdownMenu.Item className="flex items-center gap-2 px-3 py-2 text-sm rounded-md hover:bg-accent cursor-pointer" onClick={() => onAct(inst.name, "stop")} disabled={busy === inst.name}>
                            <Square className="h-4 w-4" /> Stop
                          </DropdownMenu.Item>
                        )}
                      </DropdownMenu.Content>
                    </DropdownMenu.Portal>
                  </DropdownMenu.Root>
                </>
              )}
            </div>
          )
        })}
      </div>

      {!collapsed && (
        <div className="p-3 border-t text-xs text-muted-foreground flex flex-col gap-2">
          <button onClick={()=>window.dispatchEvent(new CustomEvent("odoonoir-nav-dashboard"))} className="w-full flex items-center gap-2 px-2 py-1.5 rounded-lg border hover:bg-accent text-xs font-medium">
            <span>📊</span> Dashboard
          </button>
          <div className="flex items-center justify-between">
            <span>{instances.length} instance{instances.length !== 1 ? "s" : ""}</span>
            <span className="hidden xl:inline">Lift</span>
          </div>
        </div>
      )}
    </aside>
    </>
  )
}
