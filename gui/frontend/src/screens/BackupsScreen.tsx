import { useEffect, useState, useCallback } from "react"
import { GetBackupScheduleStatus, ListScheduledBackups, SetBackupSchedule, RunBackupNow, Databases } from "../../bindings/github.com/ahmed/odoonoir/gui/app"
import type { DatabaseView } from "../../bindings/github.com/ahmed/odoonoir/internal/service/models"
import { useToast } from "../App"
import { Button } from "../components/atoms/Button"

export function BackupsScreen({ name }: { name: string }) {
  const { toast } = useToast()
  const [dbs, setDbs] = useState<DatabaseView[]>([])
  const [status, setStatus] = useState<any>(null)
  const [files, setFiles] = useState<any[]>([])
  const [loading, setLoading] = useState(false)
  const [cron, setCron] = useState("0 2 * * *")
  const [retain, setRetain] = useState(7)
  const [compress, setCompress] = useState(true)

  useEffect(() => { Databases(name).then(setDbs).catch(()=>{}) }, [name])
  const load = useCallback(async () => {
    setLoading(true)
    try {
      const s: any = await GetBackupScheduleStatus(name);
      if (s) { setStatus(s); setCron(s.cron ?? s.Cron ?? "0 2 * * *"); setRetain(s.retain ?? s.Retain ?? 7) }
    } catch {}
    try { const f = await ListScheduledBackups(name); setFiles(f as any) } catch {}
    setLoading(false)
  }, [name])
  useEffect(() => { load() }, [load])

  const save = async (enabled: boolean) => {
    try { await SetBackupSchedule(name, { enabled, cron, databases: [], retain, compress, custom: false } as any); toast(enabled ? "Scheduler enabled" : "Scheduler disabled", "success"); load() } catch (e) { toast(String(e), "error") }
  }

  return (
    <div className="screen">
      <div className="card">
        <div className="card-header"><h2>Backups — {name}</h2><Button variant="outline" size="sm" onClick={load} disabled={loading} loading={loading}>Refresh</Button></div>
        <div className="p-4 grid gap-4">
          <div className="flex gap-2 items-end flex-wrap">
            <div className="field"><span className="field-label">Cron</span><input className="field-input mono" value={cron} onChange={e=>setCron(e.target.value)} placeholder="0 2 * * *" style={{width:140}} /></div>
            <div className="field"><span className="field-label">Retain</span><input type="number" className="field-input" value={retain} onChange={e=>setRetain(parseInt(e.target.value)||7)} style={{width:80}} /></div>
            <label className="checkbox-label"><input type="checkbox" checked={compress} onChange={e=>setCompress(e.target.checked)} /> Compress</label>
            <Button variant="primary" size="sm" onClick={()=>save(true)}>Enable</Button>
            <Button variant="outline" size="sm" onClick={()=>save(false)}>Disable</Button>
            <Button variant="secondary" size="sm" onClick={async()=>{ try{ await RunBackupNow(name); toast("Backup started","success")} catch(e){ toast(String(e),"error")}}}>Run now</Button>
          </div>
          {status && <div className="text-sm text-muted-foreground">Enabled: {String((status as any).enabled ?? (status as any).Enabled)} • Next: {((status as any).nextRun ?? (status as any).NextRun) ? new Date(((status as any).nextRun ?? (status as any).NextRun)).toLocaleString() : "—"} • Last: {((status as any).lastRun ?? (status as any).LastRun) ? new Date(((status as any).lastRun ?? (status as any).LastRun)).toLocaleString() : "—"}</div>}
          <div>
            <h3 className="font-semibold mb-2">Scheduled files ({files.length})</h3>
            {files.length===0 ? <div className="empty">No backups yet</div> : (
              <table><thead><tr><th>Name</th><th>Size</th><th>Modified</th></tr></thead><tbody>{files.map((f:any)=><tr key={f.Name||f.name}><td className="mono text-xs">{f.Name||f.name}</td><td>{f.Size ?? f.size ?? "—"}</td><td className="text-xs">{f.ModTime ? new Date(f.ModTime).toLocaleString() : f.modTime ? new Date(f.modTime).toLocaleString() : ""}</td></tr>)}</tbody></table>
            )}
          </div>
          <div className="text-xs text-muted-foreground">Databases: {dbs.map(d=>d.name).join(", ") || "—"} (scheduler backs up all by default)</div>
        </div>
      </div>
    </div>
  )
}
