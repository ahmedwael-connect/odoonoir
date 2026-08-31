import { useEffect, useState, useCallback } from "react"
import * as Dialog from "@radix-ui/react-dialog"
import { Button } from "../atoms/Button"
import { useToast } from "../../App"
import { ListAddonPaths, AddAddonPath, RemoveAddonPath, ToggleAddonPath, MoveAddonPath } from "../../../bindings/github.com/ahmed/odoonoir/gui/app"

type Entry = { path: string; enabled: boolean; exists: boolean; position: number }

export function AddonPathManagerWizard({ name, open, onOpenChange }: { name: string; open: boolean; onOpenChange: (o: boolean) => void }) {
  const { toast } = useToast()
  const [entries, setEntries] = useState<Entry[]>([])
  const [loading, setLoading] = useState(false)
  const [newPath, setNewPath] = useState("")
  const [newPos, setNewPos] = useState<number>(0)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const list: any = await ListAddonPaths(name)
      setEntries(list as any)
    } catch (e) { toast(String(e), "error") } finally { setLoading(false) }
  }, [name, toast])

  useEffect(() => { if (open) load() }, [open, load])

  const handleAdd = async () => {
    if (!newPath.trim()) { toast("Path required", "error"); return }
    try {
      const list: any = await AddAddonPath(name, newPath.trim(), newPos)
      setEntries(list as any); setNewPath(""); setNewPos(0)
      toast("Added", "success")
    } catch (e) { toast(String(e), "error") }
  }
  const handleRemove = async (p: string) => {
    try { const list: any = await RemoveAddonPath(name, p); setEntries(list as any); toast("Removed", "success") } catch (e) { toast(String(e), "error") }
  }
  const handleToggle = async (p: string, enabled: boolean) => {
    try { const list: any = await ToggleAddonPath(name, p, !enabled); setEntries(list as any); toast(!enabled ? "Enabled" : "Disabled", "success") } catch (e) { toast(String(e), "error") }
  }
  const handleMove = async (from: number, to: number) => {
    if (from === to) return
    try { const list: any = await MoveAddonPath(name, from, to); setEntries(list as any) } catch (e) { toast(String(e), "error") }
  }

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/60 backdrop-blur-sm z-50" />
        <Dialog.Content className="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 w-[760px] max-w-[95vw] bg-card border rounded-xl shadow-xl z-50 flex flex-col max-h-[90vh]">
          <div className="p-4 border-b">
            <Dialog.Title className="font-semibold">Addon Path Manager — {name}</Dialog.Title>
            <Dialog.Description className="text-sm text-muted-foreground">Manage addons_path in odoo.conf: add, remove, enable/disable (comment), reorder. Priority 1 = first match wins.</Dialog.Description>
          </div>

          {/* Add form top center */}
          <div className="p-4 border-b bg-muted/20">
            <div className="max-w-[600px] mx-auto flex gap-2 items-end">
              <div className="flex-1">
                <label className="text-xs font-medium">New path</label>
                <input className="field-input mono w-full" value={newPath} onChange={e=>setNewPath(e.target.value)} placeholder="/opt/odoo/custom_addons or /home/.../enterprise" />
              </div>
              <div className="w-[120px]">
                <label className="text-xs font-medium">Position</label>
                <input type="number" className="field-input w-full" value={newPos || ""} onChange={e=>setNewPos(parseInt(e.target.value)||0)} placeholder="auto (last)" min={0} />
              </div>
              <Button variant="primary" onClick={handleAdd}>Add</Button>
              <Button variant="outline" onClick={load} disabled={loading}>Refresh</Button>
            </div>
            <div className="text-xs text-muted-foreground text-center mt-2">0 or empty = append last; 1 = first priority. Paths are cleaned and de-duplicated.</div>
          </div>

          <div className="flex-1 overflow-y-auto p-4 space-y-2">
            {loading ? <div className="p-6 text-center">Loading…</div> : entries.length===0 ? <div className="empty">No addons_path entries — add one above</div> : (
              <div className="space-y-2">
                {entries.map(e => (
                  <div key={`${e.position}-${e.path}`} className={`flex items-center gap-3 p-3 rounded-lg border ${e.enabled ? "bg-card" : "bg-muted/40 opacity-60"} ${!e.exists ? "border-warning" : ""}`}>
                    <span className="text-xs font-mono bg-muted px-2 py-1 rounded">{e.position}</span>
                    <span className={`flex-1 mono text-sm truncate ${e.enabled?"":"line-through"}`} title={e.path}>{e.path} {!e.exists && " (missing)"}</span>
                    <span className={`text-xs px-2 py-0.5 rounded-full border ${e.enabled?"bg-success text-white border-success":"bg-muted border"}`}>{e.enabled?"enabled":"disabled"}</span>
                    <div className="flex gap-1">
                      <Button variant="outline" size="sm" disabled={e.position<=1} onClick={()=>handleMove(e.position, e.position-1)}>↑</Button>
                      <Button variant="outline" size="sm" disabled={e.position>=entries.length} onClick={()=>handleMove(e.position, e.position+1)}>↓</Button>
                      <Button variant={e.enabled?"ghost":"secondary"} size="sm" onClick={()=>handleToggle(e.path, e.enabled)}>{e.enabled?"Disable":"Enable"}</Button>
                      <Button variant="destructive" size="sm" onClick={()=>handleRemove(e.path)}>Remove</Button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>

          <div className="p-3 border-t flex justify-between">
            <span className="text-xs text-muted-foreground">{entries.length} paths • {entries.filter(e=>!e.enabled).length} disabled • {entries.filter(e=>!e.exists).length} missing</span>
            <Button variant="outline" onClick={()=>onOpenChange(false)}>Close</Button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
