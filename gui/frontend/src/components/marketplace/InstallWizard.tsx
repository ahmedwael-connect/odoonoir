import { useEffect, useState } from "react"
import * as Dialog from "@radix-ui/react-dialog"
import { Button } from "../atoms/Button"
import { useToast } from "../../App"
import { Databases, Instances, InstallGitHubModule, IncrementMarketplaceInstallCount } from "../../../bindings/github.com/ahmed/odoonoir/gui/app"

export function InstallWizard({ module, open, onOpenChange, onInstalled }: { module: any; open: boolean; onOpenChange: (o: boolean) => void; onInstalled?: () => void }) {
  const { toast } = useToast()
  const [instances, setInstances] = useState<any[]>([])
  const [dbs, setDbs] = useState<any[]>([])
  const [dbName, setDbName] = useState("")
  const [instance, setInstance] = useState("")
  const [branch, setBranch] = useState("18.0")
  const [autoDeps, setAutoDeps] = useState(true)
  const [busy, setBusy] = useState(false)
  const [step, setStep] = useState(1)

  useEffect(() => {
    if (!open) return
    Instances().then((list: any) => {
      setInstances(list as any)
      const first = (list as any)?.[0]?.name || "myapp"
      setInstance((prev) => prev || first)
      return Databases(first)
    }).then((list: any) => {
      setDbs(list as any)
      if ((list as any)?.length) setDbName((list as any)[0].name)
    }).catch(() => {})
  }, [open])

  useEffect(() => {
    if (!instance) return
    Databases(instance).then((list: any) => {
      setDbs(list as any)
      if ((list as any)?.length && !dbName) setDbName((list as any)[0].name)
    }).catch(() => {})
  }, [instance])

  const handleInstall = async () => {
    if (!module) return
    if (!instance) { toast("Select an instance", "error"); return }
    setBusy(true)
    try {
      const owner = module.Owner || module.owner
      const repo = module.Repo || module.repo
      const res: any = await InstallGitHubModule({ Owner: owner, Repo: repo, Branch: branch, Instance: instance, DBName: dbName, AutoDeps: autoDeps, RunTests: false } as any)
      const id = `${owner}/${repo}`
      try { await IncrementMarketplaceInstallCount(id) } catch {}
      toast(`Installed ${res?.ModuleName || res?.moduleName || module.Name || module.name} → ${instance} (deps: ${(res?.Dependencies || res?.dependencies || []).length})`, "success")
      onInstalled?.()
      onOpenChange(false)
    } catch (e) { toast(String(e), "error") } finally { setBusy(false) }
  }

  if (!module) return null
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/60 backdrop-blur-sm z-50" />
        <Dialog.Content className="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 w-[640px] max-w-[95vw] bg-card border rounded-xl shadow-xl z-50 max-h-[90vh] overflow-y-auto">
          <div className="p-4 border-b">
            <Dialog.Title className="font-semibold">Install {module.DisplayName || module.displayName || module.Name}</Dialog.Title>
            <Dialog.Description className="text-sm text-muted-foreground">Target instance, branch and dependency resolution</Dialog.Description>
            <div className="flex gap-1 mt-3">
              <span className={`pill ${step===1?"active":""}`}>1 Instance</span>
              <span className={`pill ${step===2?"active":""}`}>2 Deps</span>
              <span className={`pill ${step===3?"active":""}`}>3 Confirm</span>
            </div>
          </div>

          <div className="p-4 space-y-4">
            {step===1 && (
              <>
                <div className="field"><span className="field-label">Instance</span>
                  {instances.length ? (
                    <select className="field-input" value={instance} onChange={e=>setInstance(e.target.value)}>
                      {instances.map((i:any)=><option key={i.name} value={i.name}>{i.name} v{i.version}</option>)}
                    </select>
                  ) : (
                    <input className="field-input" value={instance} onChange={e=>setInstance(e.target.value)} placeholder="myapp" />
                  )}
                </div>
                <div className="field"><span className="field-label">Database</span>
                  {dbs.length ? (
                    <select className="field-input" value={dbName} onChange={e=>setDbName(e.target.value)}>
                      <option value="">default</option>
                      {dbs.map((d:any)=><option key={d.name} value={d.name}>{d.name}</option>)}
                    </select>
                  ) : (
                    <input className="field-input" value={dbName} onChange={e=>setDbName(e.target.value)} placeholder="auto" />
                  )}
                </div>
                <div className="field"><span className="field-label">Branch</span><input className="field-input mono" value={branch} onChange={e=>setBranch(e.target.value)} placeholder="18.0" /></div>
              </>
            )}
            {step===2 && (
              <>
                <div className="text-sm">Dependencies from manifest will be resolved:</div>
                <div className="bg-muted p-3 rounded-lg text-sm mono">{(module.Depends || module.depends || []).join(", ") || "none (base only)"}</div>
                <label className="checkbox-label"><input type="checkbox" checked={autoDeps} onChange={e=>setAutoDeps(e.target.checked)} /> Auto-resolve and install missing deps</label>
              </>
            )}
            {step===3 && (
              <div className="text-sm space-y-2">
                <div>Ready to install <b>{module.DisplayName || module.Name}</b> ({module.Owner}/{module.Repo}#{branch}) to <b>{instance}</b> / <b>{dbName||"default"}</b></div>
                <div className="text-muted-foreground">This will git clone, copy to addons, and run `Update` with the module.</div>
              </div>
            )}
          </div>

          <div className="p-3 border-t flex justify-between">
            <Button variant="ghost" onClick={()=>setStep(s=>Math.max(1,s-1))} disabled={step===1}>Back</Button>
            <div className="flex gap-2">
              <Button variant="outline" onClick={()=>onOpenChange(false)}>Cancel</Button>
              {step<3 ? <Button variant="primary" onClick={()=>setStep(s=>s+1)}>Next</Button> : <Button variant="primary" loading={busy} onClick={handleInstall}>Install</Button>}
            </div>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
