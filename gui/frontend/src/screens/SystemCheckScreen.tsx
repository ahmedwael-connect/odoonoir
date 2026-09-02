import { useEffect, useState, useCallback } from "react"
import { SystemCheck, CheckVersion } from "../../bindings/github.com/ahmed/odoonoir/gui/app"
import { useToast } from "../App"
import { Button } from "../components/atoms/Button"
import { SudoWizard } from "../components/system/SudoWizard"

export function SystemCheckScreen() {
  const { toast } = useToast()
  const [results, setResults] = useState<any[]>([])
  const [loading, setLoading] = useState(false)
  const [version, setVersion] = useState("18")
  const [sudoOpen, setSudoOpen] = useState(false)
  const [sudoPkgs, setSudoPkgs] = useState<string[]>([])

  const load = useCallback(async (v?: string) => {
    setLoading(true)
    try { const r = v ? await CheckVersion(v) : await SystemCheck(); setResults(r) } catch (e) { toast(String(e), "error") } finally { setLoading(false) }
  }, [toast])

  useEffect(()=>{ load(version) }, [load, version])

  const ok = results.filter(r=>r.severity==="ok"||r.Severity==="ok").length
  const warn = results.filter(r=>r.severity==="warning"||r.Severity==="warning").length
  const err = results.filter(r=>r.severity==="error"||r.Severity==="error"||r.Severity==="missing").length

  return (
    <div className="screen">
      <div className="card">
        <div className="card-header">
          <h2>System Check</h2>
          <div className="action-row">
            <select className="field-input" value={version} onChange={e=>setVersion(e.target.value)} style={{width:120}}>
              <option value="15">15.0</option><option value="16">16.0</option><option value="17">17.0</option><option value="18">18.0</option><option value="19">19.0</option>
            </select>
            <Button variant="outline" onClick={()=>load(version)} disabled={loading}>{loading?"Checking…":"Refresh"}</Button>
            {results.some((r:any)=> (r.check==="apt-packages"||r.Check==="apt-packages") && (r.severity==="missing"||r.Severity==="missing")) && (
              <Button variant="primary" size="sm" onClick={()=>{
                const r = results.find((x:any)=> x.check==="apt-packages"||x.Check==="apt-packages")
                const hint = r?.hint||r?.Hint||""
                const m = hint.match(/sudo apt install (.+)/)
                const pkgs = m ? m[1].split(" ").filter(Boolean) : ["build-essential","python3-dev","zlib1g-dev"]
                setSudoPkgs(pkgs); setSudoOpen(true)
              }}>Fix with sudo</Button>
            )}
          </div>
        </div>
        <div className="p-4 flex gap-2 text-sm"><span className="pill success">{ok} ok</span><span className="pill warning">{warn} warnings</span><span className="pill destructive">{err} errors</span></div>
        {loading ? <div className="p-4">Loading…</div> : (
          <div className="divide-y">
            {results.map((r:any,i:number)=>(
              <div key={i} className="p-3 flex gap-3">
                <span className={`pill ${r.severity==="ok" ? "success" : r.severity==="warning" ? "warning" : "destructive"}`}>{r.severity || r.Severity}</span>
                <div className="flex-1 min-w-0">
                  <div className="font-medium">{r.name || r.Name || r.check || r.Check}</div>
                  <div className="text-xs text-muted-foreground">{r.message || r.Message || r.hint || r.Hint || ""}</div>
                  {r.found || r.Found ? <div className="text-xs mono">found: {r.found || r.Found}</div> : null}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
      <SudoWizard open={sudoOpen} onOpenChange={setSudoOpen} pkgs={sudoPkgs} onDone={()=>load(version)} />
    </div>
  )
}