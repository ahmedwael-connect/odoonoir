import { useState, useEffect } from "react"
import * as Dialog from "@radix-ui/react-dialog"
import { Button } from "../atoms/Button"
import { useToast } from "../../App"

export function PublishWizard({ open, onOpenChange }: { open: boolean; onOpenChange: (o: boolean) => void }) {
  const { toast } = useToast()
  const [step, setStep] = useState(1)
  const [modulePath, setModulePath] = useState("")
  const [owner, setOwner] = useState("myorg")
  const [repo, setRepo] = useState("my_module")
  const [branch, setBranch] = useState("18.0")
  const [branches, setBranches] = useState<string[]>([])
  const [isPrivate, setPrivate] = useState(false)
  const [license, setLicense] = useState("LGPL-3")
  const [desc, setDesc] = useState("My Odoo module")
  const [createTag, setCreateTag] = useState(true)
  const [busy, setBusy] = useState(false)
  const [log, setLog] = useState<string[]>([])

  useEffect(() => { if (!open) return; setStep(1); setLog([]) }, [open])

  const fetchBranches = async () => {
    if (!owner || !repo) return
    try {
      const { ListGitHubBranches } = await import("../../../bindings/github.com/ahmed/odoonoir/gui/app")
      const list: any = await ListGitHubBranches(owner, repo)
      setBranches(list as any)
      if ((list as any)?.length && !branches.includes(branch)) setBranch((list as any)[0])
    } catch (e) { toast(String(e), "error") }
  }

  const handlePublish = async () => {
    if (!modulePath) { toast("Module path required", "error"); return }
    setBusy(true); setLog([])
    try {
      const { PublishStandardModule } = await import("../../../bindings/github.com/ahmed/odoonoir/gui/app")
      const res: any = await PublishStandardModule({ ModulePath: modulePath, Owner: owner, Repo: repo, Branch: branch, Private: isPrivate, License: license, Description: desc, CreateTag: createTag, Topics: ["odoo", `odoo-${branch}`] } as any)
      setLog([`Repo: ${res.RepoURL || res.repoUrl}`, `Branch: ${res.Branch || res.branch}`, `Tag: ${res.Tag || res.tag || "—"}`, `Created: ${(res.Created||res.created||[]).join(", ")||"—"}`])
      toast(`Published ${owner}/${repo}#${branch}`, "success")
      setStep(4)
    } catch (e) { const msg=String(e); setLog([msg]); toast(msg, "error") } finally { setBusy(false) }
  }

  const handleIndex = async () => {
    try {
      const { IndexMarketplaceModule } = await import("../../../bindings/github.com/ahmed/odoonoir/gui/app")
      await IndexMarketplaceModule(owner, repo)
      toast(`Indexed ${owner}/${repo}`, "success")
    } catch (e) { toast(String(e), "error") }
  }

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/60 backdrop-blur-sm z-50" />
        <Dialog.Content className="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 w-[640px] max-w-[95vw] bg-card border rounded-xl shadow-xl z-50 p-4 space-y-3 max-h-[90vh] overflow-y-auto">
          <Dialog.Title className="font-semibold">Publish — Standard Odoo</Dialog.Title>
          <Dialog.Description className="text-sm text-muted-foreground">Validate manifest, ensure structure, push to GitHub, create release (tag v{`{version}`})</Dialog.Description>
          <div className="flex gap-1 text-xs">
            <span className={`pill ${step===1?"active border-primary bg-primary/10":""}`}>1 Module</span>
            <span className={`pill ${step===2?"active border-primary bg-primary/10":""}`}>2 Repo</span>
            <span className={`pill ${step===3?"active border-primary bg-primary/10":""}`}>3 Publish</span>
            <span className={`pill ${step===4?"active border-success bg-success/10":""}`}>4 Done</span>
          </div>

          {step===1 && (
            <div className="grid gap-3">
              <div className="field"><span className="field-label">Module path *</span><input className="field-input mono" value={modulePath} onChange={e=>setModulePath(e.target.value)} placeholder="/home/.../custom_addons/my_module or /tmp/my_module" /></div>
              <div className="text-xs text-muted-foreground">Must contain __manifest__.py — checklist: name/summary/author/license/version (18.0.1.0.0), static/description/index.html auto-created</div>
            </div>
          )}
          {step===2 && (
            <div className="grid gap-3">
              <div className="grid grid-cols-2 gap-3">
                <div className="field"><span className="field-label">Owner *</span><input className="field-input" value={owner} onChange={e=>setOwner(e.target.value)} placeholder="myorg" /></div>
                <div className="field"><span className="field-label">Repo *</span><input className="field-input mono" value={repo} onChange={e=>setRepo(e.target.value)} placeholder="my_module" /></div>
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div className="field"><span className="field-label">Branch</span>
                  <div className="flex gap-2">
                    <input className="field-input mono flex-1" value={branch} onChange={e=>setBranch(e.target.value)} placeholder="18.0" />
                    <Button variant="outline" size="sm" onClick={fetchBranches}>Fetch</Button>
                  </div>
                  {branches.length>0 && <select className="field-input mt-1" value={branch} onChange={e=>setBranch(e.target.value)}>{branches.map(b=><option key={b} value={b}>{b}</option>)}</select>}
                </div>
                <div className="field"><span className="field-label">License</span><select className="field-input" value={license} onChange={e=>setLicense(e.target.value)}><option>LGPL-3</option><option>MIT</option><option>AGPL-3</option><option>OPL-1</option></select></div>
              </div>
              <div className="field"><span className="field-label">Description</span><input className="field-input" value={desc} onChange={e=>setDesc(e.target.value)} placeholder="My Odoo module" /></div>
              <label className="checkbox-label"><input type="checkbox" checked={isPrivate} onChange={e=>setPrivate(e.target.checked)} /> Private</label>
              <label className="checkbox-label"><input type="checkbox" checked={createTag} onChange={e=>setCreateTag(e.target.checked)} /> Create tag v{'{version}'} + GitHub release</label>
            </div>
          )}
          {step===3 && (
            <div className="space-y-2 text-sm">
              <div>Publishing <b>{owner}/{repo}#{branch}</b> from <span className="mono">{modulePath||"—"}</span></div>
              <div className="text-muted-foreground">Will: validate __manifest__.py, ensure static/description/index.html + README, git init/remote/push, tag & release, topics [odoo, odoo-{branch}]</div>
              {log.length>0 && <pre className="bg-muted p-3 rounded-lg text-xs max-h-[160px] overflow-auto whitespace-pre-wrap">{log.join("\n")}</pre>}
            </div>
          )}
          {step===4 && (
            <div className="space-y-2">
              <div className="text-sm font-medium text-success">Published</div>
              <pre className="bg-muted p-3 rounded-lg text-xs whitespace-pre-wrap">{log.join("\n")}</pre>
              <Button variant="outline" size="sm" onClick={handleIndex}>Index in Marketplace</Button>
            </div>
          )}

          <div className="flex justify-between pt-2 border-t">
            <Button variant="ghost" onClick={()=>setStep(s=>Math.max(1,s-1))} disabled={step===1}>Back</Button>
            <div className="flex gap-2">
              <Button variant="outline" onClick={()=>onOpenChange(false)}>Close</Button>
              {step<3 && <Button variant="primary" onClick={()=>setStep(s=>s+1)} disabled={step===1 && !modulePath}>Next</Button>}
              {step===3 && <Button variant="primary" loading={busy} onClick={handlePublish}>Publish</Button>}
              {step===4 && <Button variant="primary" onClick={()=>onOpenChange(false)}>Done</Button>}
            </div>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
