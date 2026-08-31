import { useEffect, useState, useCallback } from "react"
import { GetDashboardMetrics } from "../../bindings/github.com/ahmed/odoonoir/gui/app"
import { useToast } from "../App"
import { Button } from "../components/atoms/Button"

export function DashboardScreen() {
  const { toast } = useToast()
  const [data, setData] = useState<any>(null)
  const [loading, setLoading] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try { const m = await GetDashboardMetrics(); setData(m) } catch (e) { toast(String(e), "error") } finally { setLoading(false) }
  }, [toast])

  useEffect(()=>{ load() }, [load])

  if (loading) return <div className="screen"><div className="card"><div className="p-6">Loading dashboard…</div></div></div>
  if (!data) return <div className="screen"><div className="card"><div className="empty">No data — <Button variant="primary" onClick={load}>Load</Button></div></div></div>

  return (
    <div className="screen space-y-4">
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        <div className="card p-4"><div className="text-xs text-muted-foreground">Instances</div><div className="text-2xl font-bold">{data.TotalInstances ?? data.totalInstances ?? 0}</div><div className="text-xs">{data.RunningInstances ?? data.runningInstances ?? 0} running • {data.StoppedInstances ?? data.stoppedInstances ?? 0} stopped</div></div>
        <div className="card p-4"><div className="text-xs text-muted-foreground">Databases</div><div className="text-2xl font-bold">{data.TotalDatabases ?? data.totalDatabases ?? 0}</div><div className="text-xs">{((data.TotalSizeBytes ?? data.totalSizeBytes ?? 0) / 1024/1024).toFixed(1)} MB total</div></div>
        <div className="card p-4"><div className="text-xs text-muted-foreground">Alerts</div><div className="text-2xl font-bold">{(data.Alerts ?? data.alerts ?? []).length}</div><div className="text-xs">health & backup</div></div>
        <div className="card p-4"><div className="text-xs text-muted-foreground">Health</div><div className="text-2xl font-bold">{data.InstanceMetrics ? Math.round(data.InstanceMetrics.reduce((a:number,c:any)=>a+(c.HealthScore||c.healthScore||0),0) / Math.max(1,data.InstanceMetrics.length)) : 0}%</div><div className="text-xs">avg score</div></div>
      </div>

      {(data.Alerts || data.alerts || []).length > 0 && (
        <div className="card">
          <div className="card-header"><h3>Alerts</h3></div>
          <div className="divide-y">
            {(data.Alerts || data.alerts).map((a:any,i:number)=>(
              <div key={i} className="p-3 flex gap-2">
                <span className={`pill ${a.Severity==="critical"||a.severity==="critical" ? "destructive" : "warning"}`}>{a.Severity || a.severity}</span>
                <span className="flex-1">{a.Message || a.message} <span className="text-muted-foreground">— {a.Instance || a.instance}</span></span>
                <span className="text-xs text-muted-foreground">{a.Timestamp ? new Date(a.Timestamp).toLocaleString() : ""}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      <div className="card">
        <div className="card-header"><h3>Instances</h3><Button variant="outline" onClick={load}>Refresh</Button></div>
        <table>
          <thead><tr><th>Name</th><th>Status</th><th>Score</th><th>DBs</th><th>Uptime</th></tr></thead>
          <tbody>
            {(data.InstanceMetrics || data.instanceMetrics || []).map((m:any)=>(
              <tr key={m.Name||m.name}>
                <td className="mono">{m.Name||m.name} <span className="text-muted-foreground">v{m.Version||m.version}</span></td>
                <td><span className={`pill ${m.Status==="running"||m.status==="running" ? "success" : "stopped"}`}>{m.Status||m.status}</span></td>
                <td>{m.HealthScore ?? m.healthScore ?? 0}</td>
                <td>{m.Databases ?? m.databases ?? 0}</td>
                <td className="text-xs">{m.Uptime ? `${(m.Uptime/3600).toFixed(1)}h` : "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}