import { useEffect, useState, useCallback } from "react"
import { BrowseRecords, Databases, ValidateDBConfig } from "../../bindings/github.com/ahmed/odoonoir/gui/app"
import type { DatabaseView } from "../../bindings/github.com/ahmed/odoonoir/internal/service/models"
import { useToast } from "../App"
import { SkeletonTable } from "../Skeleton"
import { Button } from "../components/atoms/Button"

export function RecordsScreen({ name }: { name: string }) {
  const { toast } = useToast()
  const [dbs, setDbs] = useState<DatabaseView[]>([])
  const [dbName, setDbName] = useState("")
  const [dbError, setDbError] = useState<string | null>(null)
  const [configWarns, setConfigWarns] = useState<string[]>([])
  const [model, setModel] = useState("res.partner")
  const [domain, setDomain] = useState("[]")
  const [limit, setLimit] = useState(20)
  const [offset, setOffset] = useState(0)
  const [result, setResult] = useState<any>(null)
  const [loading, setLoading] = useState(false)
  const [filter, setFilter] = useState("")

  const loadDbs = useCallback(async () => {
    setDbError(null)
    try {
      const list:any = await Databases(name)
      setDbs(list)
      if (list?.length && !dbName) {
        const primary = (list as any[]).find((d:any)=> d.primary||d.Primary) || (list as any[])[0]
        setDbName(primary.name || primary.Name)
      }
      try { const warns:any = await ValidateDBConfig(name); if (warns?.length) setConfigWarns(warns) } catch {}
    } catch (e) { const msg=String(e); setDbError(msg); toast(msg,"error") }
  }, [name, dbName, toast])
  useEffect(() => { loadDbs() }, [loadDbs])

  const load = useCallback(async () => {
    if (!dbName || !model) return
    setLoading(true)
    try {
      let dom: any = []
      try { dom = JSON.parse(domain) } catch { dom = [] }
      const r = await BrowseRecords(name, dbName, { Model: model, Domain: dom, Limit: limit, Offset: offset, Order: "id desc" } as any)
      setResult(r)
    } catch (e) { toast(String(e), "error") } finally { setLoading(false) }
  }, [name, dbName, model, domain, limit, offset, toast])

  useEffect(() => { if (dbName) load() }, [load])

  const records: any[] = result?.Records || result?.records || []
  const fields: any[] = result?.Fields || result?.fields || []
  const total: number = result?.TotalCount ?? result?.totalCount ?? records.length

  const filtered = filter ? records.filter((r: any) => JSON.stringify(r).toLowerCase().includes(filter.toLowerCase())) : records

  return (
    <div className="screen">
      <div className="card">
        <div className="card-header">
          <h2>Records — {name}</h2>
          <div className="action-row">
            <select className="field-input" value={dbName} onChange={e => { setDbName(e.target.value); setOffset(0) }} style={{ minWidth: 140 }}>
              {dbs.map(d => <option key={d.name} value={d.name}>{d.name}</option>)}
            </select>
            <Button variant="outline" onClick={load} disabled={loading}>{loading ? "Loading…" : "Refresh"}</Button>
          </div>
        </div>
        {configWarns.length > 0 && (
          <div className="warning-banner mx-4 mt-3 p-3 rounded-lg border border-warning bg-warning/10 text-sm"><div className="font-medium">Config drift</div>{configWarns.map((w,i)=><div key={i} className="text-muted-foreground">• {w}</div>)}</div>
        )}
        {dbError && (
          <div className="error mx-4 mt-3 p-3 rounded-lg border border-destructive bg-destructive/10 text-sm flex flex-col gap-2"><div className="font-medium">Database connection failed</div><div className="mono text-xs whitespace-pre-wrap break-all">{dbError}</div><div className="flex gap-2"><Button variant="outline" size="sm" onClick={loadDbs}>Retry</Button><Button variant="ghost" size="sm" onClick={load}>Search</Button></div></div>
        )}

        <div className="p-4 border-b bg-muted/20 flex flex-wrap gap-3 items-end">
          <div className="flex flex-col gap-1">
            <label className="text-xs font-medium">Model</label>
            <input className="field-input mono" value={model} onChange={e => setModel(e.target.value)} placeholder="res.partner" style={{ minWidth: 200 }} />
          </div>
          <div className="flex flex-col gap-1 flex-1 min-w-[200px]">
            <label className="text-xs font-medium">Domain (JSON)</label>
            <input className="field-input mono" value={domain} onChange={e => setDomain(e.target.value)} placeholder='[] or [["name","ilike","test"]]' />
          </div>
          <div className="flex flex-col gap-1">
            <label className="text-xs font-medium">Limit</label>
            <input type="number" className="field-input" value={limit} onChange={e => setLimit(parseInt(e.target.value) || 20)} style={{ width: 90 }} />
          </div>
          <Button variant="primary" onClick={() => { setOffset(0); load() }} disabled={loading}>Search</Button>
        </div>

        <div className="p-3 flex gap-2 border-b">
          <input className="search-input flex-1" placeholder="Filter results…" value={filter} onChange={e => setFilter(e.target.value)} />
          <span className="search-count">{filtered.length} / {total} {total !== filtered.length ? "filtered" : ""}</span>
          <Button variant="ghost" size="sm" onClick={() => setOffset(Math.max(0, offset - limit))} disabled={offset === 0}>Prev</Button>
          <Button variant="ghost" size="sm" onClick={() => setOffset(offset + limit)} disabled={filtered.length < limit}>Next</Button>
        </div>

        {loading ? <SkeletonTable rows={4} cols={5} /> : filtered.length === 0 ? <div className="empty">No records {filter ? `matching "${filter}"` : ""}</div> : (
          <div className="overflow-x-auto">
            <table>
              <thead><tr>{fields.slice(0, 6).map((f: any) => <th key={f.Name || f.name}>{f.Name || f.name}<span className="text-muted-foreground font-normal ml-1">({f.Type || f.type})</span></th>)}<th>ID</th></tr></thead>
              <tbody>
                {filtered.map((r: any) => (
                  <tr key={r.ID || r.id}>
                    {fields.slice(0, 6).map((f: any) => {
                      const fn = f.Name || f.name
                      const val = r.Values?.[fn] ?? r.values?.[fn] ?? r[fn] ?? ""
                      const s = typeof val === "object" ? JSON.stringify(val) : String(val ?? "")
                      return <td key={fn} className="mono text-xs max-w-[200px] truncate" title={s}>{s.slice(0, 80)}</td>
                    })}
                    <td className="mono">{r.ID || r.id}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        {fields.length > 6 && <div className="p-2 text-xs text-muted-foreground">Showing 6 of {fields.length} fields. Use domain/limit to narrow.</div>}
      </div>
    </div>
  )
}