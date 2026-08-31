import { useEffect, useState } from "react"
import * as Dialog from "@radix-ui/react-dialog"
import { X, Star, GitFork, Eye, BookOpen, Package, AlertCircle } from "lucide-react"
import { Button } from "../atoms/Button"
import { Badge } from "../atoms/Badge"

export function ModuleDetailDrawer({ module, open, onOpenChange, onInstall }: { module: any; open: boolean; onOpenChange: (o: boolean) => void; onInstall: (m: any) => void }) {
  const [tab, setTab] = useState<"overview"|"readme"|"submodules"|"deps">("overview")
  if (!module) return null
  const m = module
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/60 backdrop-blur-sm z-50" />
        <Dialog.Content className="fixed right-0 top-0 bottom-0 w-[720px] max-w-[100vw] bg-card border-l shadow-xl z-50 flex flex-col overflow-hidden">
          <div className="p-4 border-b flex items-start justify-between gap-3">
            <div className="min-w-0">
              <Dialog.Title className="font-semibold truncate">{m.DisplayName || m.displayName || m.Name || m.name}</Dialog.Title>
              <Dialog.Description className="text-sm text-muted-foreground truncate">{m.Owner || m.owner}/{m.Repo || m.repo} • {m.Category || m.category || "—"} • v{m.Version || m.version || "—"}</Dialog.Description>
              <div className="flex gap-1 mt-2 flex-wrap">
                <Badge variant="outline"><Star className="h-3 w-3" /> {m.Stars ?? m.stars ?? 0}</Badge>
                <Badge variant="outline"><GitFork className="h-3 w-3" /> {m.Forks ?? m.forks ?? 0}</Badge>
                <Badge variant="outline"><Eye className="h-3 w-3" /> {m.Topics?.length || 0} topics</Badge>
                {m.Verified || m.verified ? <Badge variant="success">verified</Badge> : null}
                {m.License ? <Badge variant="outline">{m.License}</Badge> : null}
              </div>
            </div>
            <Dialog.Close asChild><Button variant="ghost" size="icon"><X className="h-4 w-4" /></Button></Dialog.Close>
          </div>

          <div className="flex gap-1 p-2 border-b bg-muted/30">
            <button className={`tab ${tab==="overview"?"active":""}`} onClick={()=>setTab("overview")}>Overview</button>
            <button className={`tab ${tab==="readme"?"active":""}`} onClick={()=>setTab("readme")}>Readme</button>
            <button className={`tab ${tab==="submodules"?"active":""}`} onClick={()=>setTab("submodules")}>Modules</button>
            <button className={`tab ${tab==="deps"?"active":""}`} onClick={()=>setTab("deps")}>Deps</button>
          </div>

          <div className="flex-1 overflow-y-auto p-4 space-y-4">
            {tab==="overview" && (
              <>
                <div className="text-sm">{m.Summary || m.summary || m.Description || m.description || "No description"}</div>
                <div className="grid grid-cols-2 gap-3 text-sm">
                  <div><span className="text-muted-foreground">Author:</span> {m.Author || m.author || "—"}</div>
                  <div><span className="text-muted-foreground">Odoo:</span> {(m.OdooVersions || m.odooVersions || []).join(", ") || "—"}</div>
                  <div><span className="text-muted-foreground">Updated:</span> {m.UpdatedAt || m.updatedAt || "—"}</div>
                  <div><span className="text-muted-foreground">Branch:</span> {m.DefaultBranch || m.defaultBranch || "—"}</div>
                </div>
                {m.Topics?.length ? <div className="flex flex-wrap gap-1">{m.Topics.map((t:string)=><Badge key={t} variant="outline">{t}</Badge>)}</div> : null}
              </>
            )}
            {tab==="readme" && <pre className="text-xs whitespace-pre-wrap bg-muted p-3 rounded-lg max-h-[60vh] overflow-auto">{m.Readme || "No readme"}</pre>}
            {tab==="submodules" && <div className="space-y-2">{(m.SubModules || m.subModules || []).length ? (m.SubModules || m.subModules).map((s:any)=><div key={s.Name||s.name} className="border rounded-lg p-3"><div className="font-medium flex gap-2"><Package className="h-4 w-4" /> {s.Name||s.name}</div><div className="text-sm text-muted-foreground">{s.Description||s.description||""}</div><div className="text-xs mono">{s.Path||s.path}</div></div>) : <div className="empty">No submodules</div>}</div>}
            {tab==="deps" && <div className="space-y-2">{(m.Dependencies || m.dependencies || []).length ? (m.Dependencies || m.dependencies).map((d:any)=><div key={d.Name||d.name} className="border rounded-lg p-3 flex justify-between"><span>{d.Name||d.name} <span className="text-muted-foreground">({d.Repo||d.repo||""})</span></span><Badge variant={d.Satisfied||d.satisfied?"success":"destructive"}>{d.Satisfied||d.satisfied?"satisfied":"missing"}</Badge></div>) : <div className="empty flex gap-2"><AlertCircle className="h-4 w-4" /> No dependencies parsed</div>}</div>}
          </div>

          <div className="p-3 border-t flex justify-end gap-2">
            <Button variant="outline" onClick={()=>onOpenChange(false)}>Close</Button>
            <Button variant="primary" onClick={()=>onInstall(m)}><BookOpen className="h-4 w-4" /> Install</Button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
