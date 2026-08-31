import React, { useRef } from "react"
import * as DropdownMenu from "@radix-ui/react-dropdown-menu"
import { Play, Square, RotateCw, MoreHorizontal, ChevronDown, Database, Package, ScrollText, Terminal, Settings2, Stethoscope, Trash2, HardDrive, Code2, Building2, Folder } from "lucide-react"
import { Button } from "../atoms/Button"
import { Badge } from "../atoms/Badge"
import { instanceNavTabs, moreInstanceTabs, type Screen } from "../../hooks/useInstanceNav"
import type { StatusView } from "../../../bindings/github.com/ahmed/odoonoir/internal/service/models"

const iconMap: Record<string, React.ElementType> = {
  LayoutDashboard: Database,
  Database,
  Package,
  ScrollText,
  Terminal,
  RefreshCw: RotateCw,
  Settings2,
  Stethoscope,
}

export function InstanceTopNav({
  selected,
  status,
  screen,
  setScreen,
  busy,
  onAct,
  onRemove,
  onConfirm,
  onEnterprise,
  onAddons,
}: {
  selected: string | null
  status: StatusView | null
  screen: Screen
  setScreen: (s: Screen) => void
  busy: string | null
  onAct: (n: string, op: "start" | "stop" | "restart") => void
  onRemove: (n: string, keepData: boolean) => void
  onConfirm: (c: { title: string; message: string; danger?: boolean; onConfirm: () => void }) => void
  onEnterprise?: () => void
  onAddons?: () => void
}) {
  const scrollRef = useRef<HTMLDivElement>(null)
  if (!selected || !status) return null
  const running = status.Instance?.status === "running"
  const inst = status.Instance

  return (
    <div className="sticky top-0 z-20 bg-card border flex items-center h-[48px] px-4 gap-4 overflow-hidden rounded-[16px] shadow-sm">
      <div className="flex items-center gap-3 min-w-0 shrink-0 border-r pr-4">
        <div className="h-2 w-2 rounded-full shrink-0" style={{ background: running ? "var(--success)" : "var(--destructive)", boxShadow: `0 0 6px ${running ? "var(--success)" : "var(--destructive)"}` }} />
        <div className="min-w-0">
          <div className="text-sm font-semibold leading-none truncate">{inst.name}</div>
          <div className="text-xs text-muted-foreground truncate">v{inst.version} • {inst.dbName} {status.Serving ? `→ ${status.Serving}` : ""}</div>
        </div>
        <Badge variant={running ? "success" : "destructive"} className="hidden sm:inline-flex ml-1">{running ? "running" : "stopped"}{inst.pid ? ` #${inst.pid}` : ""}</Badge>
      </div>

      <div ref={scrollRef} className="flex items-center gap-1 overflow-x-auto scrollbar-none flex-1 min-w-0 scroll-smooth snap-x
        [mask-image:linear-gradient(to_right,transparent,black_12px,black_calc(100%-12px),transparent)] lg:[mask-image:none]">
        {instanceNavTabs.map(t => {
          const active = screen === t.id
          const Icon = iconMap[t.icon] || Database
          return (
            <button
              key={t.id}
              onClick={() => setScreen(t.id as Screen)}
              aria-current={active ? "page" : undefined}
              className={`inline-flex items-center gap-2 px-4 py-2 rounded-[14px] text-sm font-semibold whitespace-nowrap transition-all border ${active ? "bg-primary text-primary-foreground border-primary shadow-[0_4px_12px_rgba(52,211,153,0.3)] scale-[1.02]" : "bg-card border-border hover:bg-accent text-muted-foreground hover:text-foreground hover:shadow-sm"}`}
            >
              <Icon className="h-4 w-4" /> {t.label}
            </button>
          )
        })}
        <DropdownMenu.Root>
          <DropdownMenu.Trigger asChild>
            <Button variant="ghost" size="sm" className="ml-1 shrink-0">
              More <ChevronDown className="h-4 w-4" />
            </Button>
          </DropdownMenu.Trigger>
          <DropdownMenu.Portal>
            <DropdownMenu.Content className="min-w-[180px] bg-popover border rounded-lg shadow-lg p-1 z-50">
              {moreInstanceTabs.map(m => (
                <DropdownMenu.Item key={m.id} className="px-3 py-2 text-sm rounded-md hover:bg-accent cursor-pointer" onClick={() => setScreen(m.id as Screen)}>
                  {m.label}
                </DropdownMenu.Item>
              ))}
            </DropdownMenu.Content>
          </DropdownMenu.Portal>
        </DropdownMenu.Root>
      </div>

      <div className="flex items-center gap-2 shrink-0 ml-auto">
        <div className="hidden lg:flex items-center gap-2">
          {running ? (
            <>
              <Button size="sm" variant="secondary" disabled={busy === selected} onClick={() => onAct(selected!, "restart")}><RotateCw className="h-4 w-4" /> Restart</Button>
              <Button size="sm" variant="destructive" disabled={busy === selected} onClick={() => onAct(selected!, "stop")}><Square className="h-4 w-4" /> Stop</Button>
            </>
          ) : (
            <Button size="sm" variant="primary" disabled={busy === selected} onClick={() => onAct(selected!, "start")}><Play className="h-4 w-4" /> Start</Button>
          )}
        </div>
        <DropdownMenu.Root>
          <DropdownMenu.Trigger asChild>
            <Button variant="ghost" size="icon" aria-label="Instance actions"><MoreHorizontal className="h-4 w-4" /></Button>
          </DropdownMenu.Trigger>
          <DropdownMenu.Portal>
            <DropdownMenu.Content className="min-w-[200px] bg-popover border rounded-lg shadow-lg p-1 z-50" align="end">
              <DropdownMenu.Item className="px-3 py-2 text-sm rounded-md hover:bg-accent cursor-pointer flex gap-2" onClick={async()=>{ try{ const { OpenInVSCode } = await import("../../../bindings/github.com/ahmed/odoonoir/gui/app"); await OpenInVSCode(selected!, ""); } catch{}}}><Code2 className="h-4 w-4" /> Open in VS Code</DropdownMenu.Item>
              <DropdownMenu.Item className="px-3 py-2 text-sm rounded-md hover:bg-accent cursor-pointer flex gap-2" onClick={()=>onEnterprise?.()}><Building2 className="h-4 w-4" /> Load Enterprise</DropdownMenu.Item>
              <DropdownMenu.Item className="px-3 py-2 text-sm rounded-md hover:bg-accent cursor-pointer flex gap-2" onClick={()=>onAddons?.()}><Folder className="h-4 w-4" /> Addon Path Manager</DropdownMenu.Item>
              <DropdownMenu.Item className="px-3 py-2 text-sm rounded-md hover:bg-accent cursor-pointer flex gap-2" onClick={() => setScreen("databases" as Screen)}><HardDrive className="h-4 w-4" /> Manage databases</DropdownMenu.Item>
              <DropdownMenu.Separator className="h-px bg-border my-1" />
              <DropdownMenu.Item className="px-3 py-2 text-sm rounded-md hover:bg-destructive/10 text-destructive cursor-pointer flex gap-2" onClick={() => onConfirm({ title: `Remove "${selected}"?`, message: "Deletes files and primary DB.", danger: true, onConfirm: () => onRemove(selected!, false) })}>
                <Trash2 className="h-4 w-4" /> Remove instance
              </DropdownMenu.Item>
            </DropdownMenu.Content>
          </DropdownMenu.Portal>
        </DropdownMenu.Root>
      </div>
    </div>
  )
}
