import { useEffect, useState, useCallback, useMemo } from "react"
import { GetDashboardMetrics, GetSlowQueries, GetFlameGraph, Instances } from "../../bindings/github.com/ahmed/odoonoir/gui/app"
import { useToast } from "../App"
import { Button } from "../components/atoms/Button"

export function DashboardScreen() {
  const { toast } = useToast()
  const [data, setData] = useState<any>(null)
  const [loading, setLoading] = useState(false)
  const [history, setHistory] = useState<{t:number,cpu:number,mem:number}[]>([])
  const [slow, setSlow] = useState<any[]>([])
  const [slowDb, setSlowDb] = useState("")
  const [flame, setFlame] = useState<string>("")
  const [flameBusy, setFlameBusy] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const m:any = await GetDashboardMetrics()
      setData(m)
      const now = Date.now()
      const cpu = m.hostCPU ?? m.HostCPU ?? 0
      const mem = m.hostMemPercent ?? m.HostMemPercent ?? 0
      setHistory(h=> [...h.slice(-19), {t: now, cpu, mem}])
    } catch (e) { toast(String(e), "error") } finally { setLoading(false) }
  }, [toast])

  useEffect(()=>{ load(); const id=setInterval(load, 5000); return ()=>clearInterval(id) }, [load])

  const loadSlow = useCallback(async (inst?: string, db?: string) => {
    const targetInst = inst || data?.InstanceMetrics?.[0]?.Name || data?.instanceMetrics?.[0]?.name
    if (!targetInst) return
    const targetDb = db || slowDb
    try {
      const res:any = await GetSlowQueries(targetInst, targetDb, 10)
      setSlow(res as any)
    } catch (e) { /* ignore if no pg_stat_statements */ }
  }, [data, slowDb])
  useEffect(()=>{ if (data) loadSlow() }, [data, loadSlow])

  const runFlame = async () => {
    const inst = data?.InstanceMetrics?.[0]?.Name || data?.instanceMetrics?.[0]?.name
    if (!inst) { toast("No running instance for flame", "error"); return }
    setFlameBusy(true)
    try { const p:any = await GetFlameGraph(inst, 10); setFlame(p as any); toast(`Flame: ${p}`, "success") } catch (e) { toast(String(e), "error") } finally { setFlameBusy(false) }
  }

  if (loading && !data) return <div className="screen"><div className="card"><div className="p-6">Loading dashboard…</div></div></div>
  if (!data) return <div className="screen"><div className="card"><div className="empty">No data — <Button variant="primary" onClick={load}>Load</Button></div></div></div>

  const cpuHistory = useMemo(() => history.map(h => h.cpu), [history])
  const memHistory = useMemo(() => history.map(h => h.mem), [history])

  const spark = useCallback((vals: number[], max = 100) => {
    if (vals.length < 2) return null
    const w = 80, h = 24, step = w / (vals.length - 1)
    const pts = vals.map((v, i) => `${i * step},${h - (v / max) * h}`).join(" ")
    return <svg width={w} height={h} className="mt-1"><polyline fill="none" stroke="var(--primary)" strokeWidth="1.5" points={pts} /></svg>
  }, [])

  const cpuSpark = useMemo(() => spark(cpuHistory), [cpuHistory, spark])
  const memSpark = useMemo(() => spark(memHistory), [memHistory, spark])

  return (
    <div className="screen space-y-4">
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        <div className="card p-4"><div className="text-xs text-muted-foreground">Instances</div><div className="text-2xl font-bold">{data.TotalInstances ?? data.totalInstances ?? 0}</div><div className="text-xs">{data.RunningInstances ?? data.runningInstances ?? 0} running • {data.StoppedInstances ?? data.stoppedInstances ?? 0} stopped</div></div>
        <div className="card p-4"><div className="text-xs text-muted-foreground">Databases</div><div className="text-2xl font-bold">{data.TotalDatabases ?? data.totalDatabases ?? 0}</div><div className="text-xs">{((data.TotalSizeBytes ?? data.totalSizeBytes ?? 0) / 1024/1024).toFixed(1)} MB total</div></div>
        <div className="card p-4"><div className="text-xs text-muted-foreground">Alerts</div><div className="text-2xl font-bold">{(data.Alerts ?? data.alerts ?? []).length}</div><div className="text-xs">health & backup • live</div>{cpuSpark}</div>
        <div className="card p-4"><div className="text-xs text-muted-foreground">Health</div><div className="text-2xl font-bold">{data.InstanceMetrics ? Math.round(data.InstanceMetrics.reduce((a:number,c:any)=>a+(c.HealthScore||c.healthScore||0),0) / Math.max(1,data.InstanceMetrics.length)) : 0}%</div><div className="text-xs">avg score {memSpark}</div></div>
      </div>
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <div className="card p-3"><div className="text-xs text-muted-foreground">Host CPU</div><div className="text-lg font-bold">{(data.hostCPU ?? data.HostCPU ?? 0).toFixed(1)}%</div>{cpuSpark}</div>
        <div className="card p-3"><div className="text-xs text-muted-foreground">Host Mem</div><div className="text-lg font-bold">{(data.hostMemPercent ?? data.HostMemPercent ?? 0).toFixed(1)}%</div>{memSpark}</div>
        <div className="card p-3"><div className="text-xs text-muted-foreground">Host Disk</div><div className="text-lg font-bold">{(data.hostDiskPercent ?? data.HostDiskPercent ?? 0).toFixed(1)}%</div><div className="text-xs">{data.TotalInstances ?? 0} instances</div></div>
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
        <div className="card-header"><h3>Instances — live</h3><div className="flex gap-2"><span className="text-xs text-muted-foreground hidden md:inline">auto-refresh 5s</span><Button variant="outline" onClick={load}>Refresh</Button></div></div>
        <table>
          <thead><tr><th>Name</th><th>Status</th><th>CPU</th><th>Mem</th><th>Score</th><th>DBs</th><th>Uptime</th></tr></thead>
          <tbody>
            {(data.InstanceMetrics || data.instanceMetrics || []).map((m:any)=>(
              <tr key={m.Name||m.name}>
                <td className="mono">{m.Name||m.name} <span className="text-muted-foreground">v{m.Version||m.version}</span></td>
                <td><span className={`pill ${m.Status==="running"||m.status==="running" ? "success" : "stopped"}`}>{m.Status||m.status}</span></td>
                <td className="text-xs">{(m.CPUPercent ?? m.cpuPercent ?? 0).toFixed(1)}%</td>
                <td className="text-xs">{(m.MemoryMB ?? m.memoryMB ?? 0).toFixed(0)} MB</td>
                <td>{m.HealthScore ?? m.healthScore ?? 0}</td>
                <td>{m.Databases ?? m.databases ?? 0}</td>
                <td className="text-xs">{m.Uptime ? `${(m.Uptime/3600).toFixed(1)}h` : "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="card">
        <div className="card-header"><h3>Profiler — slow queries</h3><div className="flex gap-2"><input className="field-input text-xs" placeholder="db (empty=primary)" value={slowDb} onChange={e=>setSlowDb(e.target.value)} style={{width:140}} /><Button variant="outline" size="sm" onClick={()=>loadSlow()}>Load</Button><Button variant="primary" size="sm" loading={flameBusy} onClick={runFlame}>Flame 10s (py-spy)</Button></div></div>
        {slow.length===0 ? <div className="p-4 text-sm text-muted-foreground">No slow queries — need pg_stat_statements extension or no traffic yet. {flame && <span className="mono">Flame: {flame}</span>}</div> : (
          <table><thead><tr><th>Query</th><th>Calls</th><th>Mean ms</th><th>Total ms</th></tr></thead><tbody>{slow.map((q:any,i:number)=><tr key={i}><td className="mono text-xs max-w-[600px] truncate" title={q.query||q.Query}>{(q.query||q.Query||"").slice(0,120)}</td><td>{q.calls||q.Calls}</td><td>{(q.meanTime||q.MeanTime||0).toFixed(1)}</td><td>{(q.totalTime||q.TotalTime||0).toFixed(0)}</td></tr>)}</tbody></table>
        )}
        {flame && <div className="p-3 text-xs">Flame: <span className="mono">{flame}</span> — open with <span className="mono">xdg-open {flame}</span></div>}
      </div>
    </div>
  )
}