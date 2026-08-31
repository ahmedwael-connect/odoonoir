import { useState } from "react"
import { Clone } from "../../bindings/github.com/ahmed/odoonoir/gui/app"
import { useToast } from "../App"
import { Button } from "../components/atoms/Button"

export function CloneScreen({ name, onCloned }: { name: string; onCloned?: () => void }) {
  const { toast } = useToast()
  const [newName, setNewName] = useState(`${name}_clone`)
  const [port, setPort] = useState("")
  const [busy, setBusy] = useState(false)

  const handle = async () => {
    if (!newName.trim()) { toast("New name required", "error"); return }
    setBusy(true)
    try {
      const res = await Clone(name, newName, port)
      toast(`Cloned ${name} → ${res.name} on :${res.port}`, "success")
      onCloned?.()
    } catch (e) { toast(String(e), "error") } finally { setBusy(false) }
  }

  return (
    <div className="screen">
      <div className="card">
        <div className="card-header"><h2>Clone — {name}</h2></div>
        <div className="p-4 grid gap-4 max-w-lg">
          <div className="field"><span className="field-label">New Name</span><input className="field-input mono" value={newName} onChange={e => setNewName(e.target.value)} /></div>
          <div className="field"><span className="field-label">Port (auto if empty)</span><input className="field-input mono" value={port} onChange={e => setPort(e.target.value)} placeholder="auto" /></div>
          <Button variant="primary" onClick={handle} disabled={busy}>{busy ? "Cloning…" : "Clone Instance"}</Button>
          <div className="text-xs text-muted-foreground">Clones DB + filestore + addons, assigns new port.</div>
        </div>
      </div>
    </div>
  )
}