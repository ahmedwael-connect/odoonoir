import { useState } from "react"
import * as Dialog from "@radix-ui/react-dialog"
import { Button } from "../atoms/Button"
import { useToast } from "../../App"

export function SyncDialog({ module, open, onOpenChange }: { module: any; open: boolean; onOpenChange: (o: boolean) => void }) {
  const { toast } = useToast()
  const [dryRun, setDryRun] = useState(true)
  const [busy, setBusy] = useState(false)
  const [log, setLog] = useState<string[]>([])

  const [instance, setInstance] = useState("myapp")
  const handle = async () => {
    setBusy(true); setLog([])
    try {
      const { SyncGitHubModule, Instances } = await import("../../../bindings/github.com/ahmed/odoonoir/gui/app")
      // try to pick first instance if none selected
      let inst = instance
      if (!inst) {
        try { const list:any = await Instances(); inst = list?.[0]?.name || "myapp" } catch {}
      }
      const owner = module.Owner || module.owner
      const repo = module.Repo || module.repo
      const branch = module.DefaultBranch || module.defaultBranch || "main"
      if (dryRun) {
        setLog([`Old: ${module.Version||module.version||"—"} → New: check ${owner}/${repo}#${branch} (dry-run, no write)`, `Instance: ${inst}`])
        toast("Dry-run complete", "success")
      } else {
        const res:any = await SyncGitHubModule(owner, repo, branch, inst)
        setLog([`Old: ${res?.OldVersion || res?.oldVersion || "—"} → New: ${res?.NewVersion || res?.newVersion || "—"}`, `Updated: ${res?.Updated ?? res?.updated}`, res?.Changelog || res?.changelog || ""])
        toast("Sync complete", "success")
        onOpenChange(false)
      }
    } catch (e) { toast(String(e), "error"); setLog([String(e)]) } finally { setBusy(false) }
  }

  if (!module) return null
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/60 backdrop-blur-sm z-50" />
        <Dialog.Content className="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 w-[560px] max-w-[95vw] bg-card border rounded-xl shadow-xl z-50 p-4 space-y-3">
          <Dialog.Title className="font-semibold">Sync {module.DisplayName||module.Name}</Dialog.Title>
          <div className="text-sm">Current: <span className="mono">{module.Version||module.version||"—"}</span> → Latest from GitHub</div>
          <div className="field"><span className="field-label">Instance</span><input className="field-input" value={instance} onChange={e=>setInstance(e.target.value)} placeholder="myapp" /></div>
          <label className="checkbox-label"><input type="checkbox" checked={dryRun} onChange={e=>setDryRun(e.target.checked)} /> Dry-run (no write)</label>
          {log.length>0 && <pre className="bg-muted p-3 rounded-lg text-xs max-h-[200px] overflow-auto">{log.join("\n")}</pre>}
          <div className="flex justify-end gap-2">
            <Button variant="outline" onClick={()=>onOpenChange(false)}>Close</Button>
            <Button variant="primary" loading={busy} onClick={handle}>{dryRun?"Dry-run":"Sync Now"}</Button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
