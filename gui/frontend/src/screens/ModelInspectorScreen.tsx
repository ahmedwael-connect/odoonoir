import { useEffect, useState, useCallback } from "react"
import { ModelInfo, Databases, ValidateDBConfig } from "../../bindings/github.com/ahmed/odoonoir/gui/app"
import type { DatabaseView } from "../../bindings/github.com/ahmed/odoonoir/internal/service/models"
import { useToast } from "../App"
import { Button } from "../components/atoms/Button"

export function ModelInspectorScreen({ name }: { name: string }) {
  const { toast } = useToast()
  const [dbs, setDbs] = useState<DatabaseView[]>([])
  const [dbName, setDbName] = useState("")
  const [dbError, setDbError] = useState<string | null>(null)
  const [configWarns, setConfigWarns] = useState<string[]>([])
  const [model, setModel] = useState("res.partner")
  const [info, setInfo] = useState<any>(null)
  const [loading, setLoading] = useState(false)
  const [tab, setTab] = useState<"fields"|"access"|"views"|"actions">("fields")

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
    try { const r = await ModelInfo(name, dbName, model); setInfo(r) } catch (e) { toast(String(e), "error") } finally { setLoading(false) }
  }, [name, dbName, model, toast])

  useEffect(() => { if (dbName) load() }, [load])

  const fields: any[] = info?.Fields || info?.fields || []
  const access: any[] = info?.Access || info?.access || []
  const views: any[] = info?.Views || info?.views || []
  const actions: any[] = info?.Actions || info?.actions || []

  return (
    <div className="screen">
      <div className="card">
        <div className="card-header">
          <h2>Inspector — {name}</h2>
          <div className="action-row">
            <select className="field-input" value={dbName} onChange={e => setDbName(e.target.value)} style={{ minWidth: 140 }}>{dbs.map(d => <option key={d.name} value={d.name}>{d.name}</option>)}</select>
            <input className="search-input mono" value={model} onChange={e => setModel(e.target.value)} placeholder="res.partner" />
            <Button variant="primary" onClick={load} disabled={loading}>{loading ? "Loading…" : "Inspect"}</Button>
          </div>
        </div>
        {configWarns.length > 0 && (
          <div className="warning-banner mx-4 mt-3 p-3 rounded-lg border border-warning bg-warning/10 text-sm"><div className="font-medium">Config drift</div>{configWarns.map((w,i)=><div key={i} className="text-muted-foreground">• {w}</div>)}</div>
        )}
        {dbError && (
          <div className="error mx-4 mt-3 p-3 rounded-lg border border-destructive bg-destructive/10 text-sm flex flex-col gap-2"><div className="font-medium">Database error</div><div className="mono text-xs whitespace-pre-wrap break-all">{dbError}</div><div className="flex gap-2"><Button variant="outline" size="sm" onClick={loadDbs}>Retry</Button><Button variant="ghost" size="sm" onClick={load}>Inspect</Button></div></div>
        )}

        <div className="tabs p-2 border-b">
          <button className={`tab ${tab==="fields"?"active":""}`} onClick={()=>setTab("fields")}>Fields ({fields.length})</button>
          <button className={`tab ${tab==="access"?"active":""}`} onClick={()=>setTab("access")}>Access ({access.length})</button>
          <button className={`tab ${tab==="views"?"active":""}`} onClick={()=>setTab("views")}>Views ({views.length})</button>
          <button className={`tab ${tab==="actions"?"active":""}`} onClick={()=>setTab("actions")}>Actions ({actions.length})</button>
        </div>

        <div className="p-4">
          {tab==="fields" && (
            fields.length ? <table><thead><tr><th>Name</th><th>Type</th><th>Relation</th><th>Required</th><th>String</th></tr></thead><tbody>{fields.map((f:any)=><tr key={f.Name||f.name}><td className="mono">{f.Name||f.name}</td><td>{f.Type||f.type}</td><td>{f.Relation||f.relation||f.ComodelName||""}</td><td>{(f.Required||f.required)?"yes":""}</td><td>{f.String||f.string||""}</td></tr>)}</tbody></table> : <div className="empty">No fields</div>
          )}
          {tab==="access" && (
            access.length ? <table><thead><tr><th>Name</th><th>Group</th><th>Read</th><th>Write</th><th>Create</th><th>Unlink</th></tr></thead><tbody>{access.map((a:any)=><tr key={a.Name||a.name}><td>{a.Name||a.name}</td><td>{a.Group||a.group||""}</td><td>{a.PermRead||a.permRead?"✓":""}</td><td>{a.PermWrite||a.permWrite?"✓":""}</td><td>{a.PermCreate||a.permCreate?"✓":""}</td><td>{a.PermUnlink||a.permUnlink?"✓":""}</td></tr>)}</tbody></table> : <div className="empty">No access rules</div>
          )}
          {tab==="views" && (
            views.length ? <table><thead><tr><th>Name</th><th>Type</th><th>Arch</th></tr></thead><tbody>{views.map((v:any)=><tr key={v.Name||v.name}><td className="mono">{v.Name||v.name}</td><td>{v.Type||v.type}</td><td className="mono text-xs max-w-[400px] truncate">{(v.Arch||v.arch||"").slice(0,120)}</td></tr>)}</tbody></table> : <div className="empty">No views</div>
          )}
          {tab==="actions" && (
            actions.length ? <table><thead><tr><th>Name</th><th>Res Model</th><th>View Mode</th></tr></thead><tbody>{actions.map((a:any)=><tr key={a.Name||a.name}><td>{a.Name||a.name}</td><td>{a.ResModel||a.resModel}</td><td>{a.ViewMode||a.viewMode}</td></tr>)}</tbody></table> : <div className="empty">No actions</div>
          )}
        </div>
      </div>
    </div>
  )
}