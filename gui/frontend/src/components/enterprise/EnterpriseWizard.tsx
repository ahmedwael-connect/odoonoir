import { useState, useEffect } from "react"
import * as Dialog from "@radix-ui/react-dialog"
import { Button } from "../atoms/Button"
import { useToast } from "../../App"
import { DetectEnterprise, LoadEnterprise, UnloadEnterprise, ListGitHubBranches } from "../../../bindings/github.com/ahmed/odoonoir/gui/app"

export function EnterpriseWizard({ name, open, onOpenChange, onDone }: { name: string; open: boolean; onOpenChange: (o: boolean) => void; onDone?: () => void }) {
  const { toast } = useToast()
  const [repo, setRepo] = useState("odoo/enterprise")
  const [branch, setBranch] = useState("16.0")
  const [branches, setBranches] = useState<string[]>([])
  const [position, setPosition] = useState<"default" | "last">("default")
  const [status, setStatus] = useState<any>(null)
  const [busy, setBusy] = useState(false)
  const [log, setLog] = useState<string[]>([])

  const loadStatus = async () => {
    try {
      const s: any = await DetectEnterprise(name)
      setStatus(s)
      if (s?.branch) setBranch(s.branch)
      if (s?.repo) setRepo(s.repo)
    } catch {}
  }

  useEffect(() => { if (open) loadStatus() }, [open, name])

  const fetchBranches = async () => {
    if (!repo.includes("/")) { toast("repo must be owner/repo", "error"); return }
    const [owner, r] = repo.split("/")
    try {
      const list: any = await ListGitHubBranches(owner, r)
      setBranches(list as any)
      toast(`Found ${ (list as any)?.length } branches`, "success")
    } catch (e) { toast(String(e), "error") }
  }

  const handleLoad = async () => {
    setBusy(true); setLog([])
    try {
      const res: any = await LoadEnterprise(name, repo, branch, position)
      setLog([`Enterprise: ${res.isEnterprise ? "yes" : "no"}`, `Path: ${res.path || "—"}`, `Repo: ${res.repo} Branch: ${res.branch}`, `Addons: ${(res.addonsPath||[]).join(", ")}`])
      toast(`Enterprise loaded ${repo}#${branch} at ${position}`, "success")
      loadStatus()
      onDone?.()
    } catch (e) { const msg = String(e); setLog([msg]); toast(msg, "error") } finally { setBusy(false) }
  }

  const handleUnload = async () => {
    setBusy(true)
    try {
      await UnloadEnterprise(name)
      toast("Enterprise removed from addons_path", "success")
      loadStatus()
    } catch (e) { toast(String(e), "error") } finally { setBusy(false) }
  }

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/60 backdrop-blur-sm z-50" />
        <Dialog.Content className="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 w-[640px] max-w-[95vw] bg-card border rounded-xl shadow-xl z-50 p-4 space-y-3 max-h-[90vh] overflow-y-auto">
          <Dialog.Title className="font-semibold">Load Enterprise {status?.isEnterprise ? "• Enterprise" : ""}</Dialog.Title>
          <Dialog.Description className="text-sm text-muted-foreground">
            Smart detect: {status?.isEnterprise ? `enterprise at ${status.path || "addons_path"}` : "community (no enterprise path found)"} — provide GitHub repo path, branch auto-matches instance version, choose position.
          </Dialog.Description>

          {status?.isEnterprise && (
            <div className="p-3 rounded-lg bg-success/10 border border-success/20 text-sm flex justify-between items-center">
              <span>Enterprise active: <span className="mono">{status.path}</span> ({status.repo}#{status.branch})</span>
              <Button variant="outline" size="sm" onClick={handleUnload} disabled={busy}>Unload</Button>
            </div>
          )}
          {status?.warning && <div className="p-2 rounded bg-warning/10 border border-warning text-xs">{status.warning}</div>}

          <div className="grid gap-3">
            <div className="field"><span className="field-label">GitHub repo *</span><input className="field-input mono" value={repo} onChange={e=>setRepo(e.target.value)} placeholder="odoo/enterprise or myorg/enterprise" /></div>
            <div className="field"><span className="field-label">Branch (stable)</span>
              <div className="flex gap-2">
                <input className="field-input mono flex-1" value={branch} onChange={e=>setBranch(e.target.value)} placeholder="16.0" />
                <Button variant="outline" size="sm" onClick={fetchBranches}>Fetch branches</Button>
              </div>
              {branches.length>0 && <select className="field-input mt-2" value={branch} onChange={e=>setBranch(e.target.value)}>{branches.map(b=><option key={b} value={b}>{b}</option>)}</select>}
              <div className="text-xs text-muted-foreground mt-1">Auto: instance version {branch} stable — loads that branch</div>
            </div>
            <div className="field"><span className="field-label">Position in addons_path</span>
              <div className="flex gap-2">
                <label className={`flex-1 border rounded-lg p-3 cursor-pointer ${position==="default"?"bg-primary/10 border-primary":""}`}><input type="radio" checked={position==="default"} onChange={()=>setPosition("default")} className="mr-2" />Default (first) — enterprise overrides community</label>
                <label className={`flex-1 border rounded-lg p-3 cursor-pointer ${position==="last"?"bg-primary/10 border-primary":""}`}><input type="radio" checked={position==="last"} onChange={()=>setPosition("last")} className="mr-2" />Last — after custom_addons (low priority)</label>
              </div>
            </div>
          </div>

          {log.length>0 && <pre className="bg-muted p-3 rounded-lg text-xs max-h-[160px] overflow-auto whitespace-pre-wrap">{log.join("\n")}</pre>}

          <div className="flex justify-between pt-2 border-t">
            <Button variant="ghost" onClick={()=>onOpenChange(false)}>Close</Button>
            <div className="flex gap-2">
              <Button variant="outline" onClick={loadStatus} disabled={busy}>Refresh status</Button>
              <Button variant="primary" loading={busy} onClick={handleLoad}>Load Enterprise</Button>
            </div>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
