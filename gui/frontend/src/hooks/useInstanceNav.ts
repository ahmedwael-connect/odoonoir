import { useEffect, useMemo } from "react"

export type GlobalScreen = "create" | "adopt" | "systemcheck" | "dashboard" | "marketplace"
export type InstanceScreen = "overview" | "databases" | "modules" | "logs" | "terminal" | "update" | "config" | "doctor" | "cron" | "records" | "depgraph" | "scaffold" | "clone" | "modelinspector" | "backups" | "deploy"
export type Screen = GlobalScreen | InstanceScreen

const globalScreens: GlobalScreen[] = ["dashboard", "create", "adopt", "systemcheck", "marketplace"]
const instanceScreens: InstanceScreen[] = ["overview","databases","modules","logs","terminal","update","config","doctor","cron","records","depgraph","scaffold","clone","modelinspector","backups","deploy"]

export function isGlobalScreen(s: string): s is GlobalScreen {
  return (globalScreens as string[]).includes(s)
}
export function isInstanceScreen(s: string): s is InstanceScreen {
  return (instanceScreens as string[]).includes(s)
}

export const instanceNavTabs: { id: InstanceScreen; label: string; icon: string; hint?: string }[] = [
  { id: "overview", label: "Overview", icon: "LayoutDashboard" },
  { id: "databases", label: "Databases", icon: "Database" },
  { id: "modules", label: "Modules", icon: "Package" },
  { id: "logs", label: "Logs", icon: "ScrollText" },
  { id: "terminal", label: "Terminal", icon: "Terminal" },
  { id: "update", label: "Update", icon: "RefreshCw" },
  { id: "config", label: "Config", icon: "Settings2" },
  { id: "doctor", label: "Doctor", icon: "Stethoscope" },
]

export const moreInstanceTabs: { id: InstanceScreen; label: string }[] = [
  { id: "cron", label: "Cron" },
  { id: "records", label: "Records" },
  { id: "depgraph", label: "Graph" },
  { id: "scaffold", label: "Scaffold" },
  { id: "clone", label: "Clone" },
  { id: "modelinspector", label: "Inspector" },
  { id: "backups", label: "Backups" },
  { id: "deploy", label: "Deploy" },
]

export function useInstanceNav(selected: string | null, screen: Screen, setScreen: (s: Screen)=>void) {
  useEffect(()=>{
    // Allow global screens (create/adopt) even when an instance is selected — don't force overview
    if (!selected && isInstanceScreen(screen)) setScreen("create")
  }, [selected, screen, setScreen])

  const currentTabs = useMemo(()=> isInstanceScreen(screen) || selected ? instanceNavTabs : [], [screen, selected])
  return { currentTabs, moreInstanceTabs, isGlobalScreen, isInstanceScreen }
}
