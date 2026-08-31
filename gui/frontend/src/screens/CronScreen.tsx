import { useEffect, useState, useCallback } from "react"
import { CronList, Databases } from "../../bindings/github.com/ahmed/odoonoir/gui/app"
import type { DatabaseView } from "../../bindings/github.com/ahmed/odoonoir/internal/service/models"
import { useToast } from "../App"
import { SkeletonTable } from "../Skeleton"
import { Button } from "../components/atoms/Button"

export function CronScreen({ name }: { name: string }) {
  const { toast } = useToast()
  const [dbs, setDbs] = useState<DatabaseView[]>([])
  const [dbName, setDbName] = useState("")
  const [crons, setCrons] = useState<any[]>([])
  const [loading, setLoading] = useState(false)
  const [filter, setFilter] = useState("")

  useEffect(() => { Databases(name).then(list => { setDbs(list); if (list.length && !dbName) setDbName(list[0].name) }).catch(() => {}) }, [name])

  const load = useCallback(async () => {
    if (!dbName) return
    setLoading(true)
    try { const list = await CronList(name, dbName); setCrons(list) } catch (e) { toast(String(e), "error") } finally { setLoading(false) }
  }, [name, dbName, toast])

  useEffect(() => { load() }, [load])

  const filtered = filter ? crons.filter(c => (c.Name||c.name||"").toLowerCase().includes(filter.toLowerCase()) || (c.Model||c.model||"").toLowerCase().includes(filter.toLowerCase())) : crons

  return (
    <div className="screen">
      <div className="card">
        <div className="card-header">
          <h2>Cron — {name}</h2>
          <div className="action-row">
            <select className="field-input" value={dbName} onChange={e => setDbName(e.target.value)} style={{ minWidth: 160 }}>
              {dbs.map(d => <option key={d.name} value={d.name}>{d.name}</option>)}
            </select>
            <input className="search-input" placeholder="Filter cron…" value={filter} onChange={e => setFilter(e.target.value)} />
            <Button variant="outline" onClick={load} disabled={loading}>{loading ? "Loading…" : "Refresh"}</Button>
          </div>
        </div>
        {loading ? <SkeletonTable rows={4} cols={6} /> : filtered.length === 0 ? <div className="empty">No cron jobs {filter ? `matching "${filter}"` : ""}</div> : (
          <table>
            <thead><tr><th>Name</th><th>Model</th><th>Method</th><th>Interval</th><th>Next Call</th><th>Active</th></tr></thead>
            <tbody>
              {filtered.map((c: any) => (
                <tr key={c.ID || c.id || c.Name}>
                  <td><span className="mono">{c.Name || c.name}</span></td>
                  <td>{c.Model || c.model || "—"}</td>
                  <td className="mono">{c.Function || c.function || "—"}</td>
                  <td>{c.IntervalNum || c.intervalNumber || 0} {c.IntervalType || c.intervalType || ""}</td>
                  <td className="mono text-xs">{c.NextCall || c.nextCall || "—"}</td>
                  <td><span className={`pill ${c.Active ?? c.active ? "running" : "stopped"}`}>{(c.Active ?? c.active) ? "active" : "inactive"}</span></td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}