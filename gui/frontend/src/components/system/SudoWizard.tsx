import { useState } from "react"
import * as Dialog from "@radix-ui/react-dialog"
import { Button } from "../atoms/Button"
import { useToast } from "../../App"
import { SudoAptInstall } from "../../../bindings/github.com/ahmed/odoonoir/gui/app"

export function SudoWizard({ open, onOpenChange, pkgs, onDone }: { open: boolean; onOpenChange: (o: boolean) => void; pkgs: string[]; onDone?: () => void }) {
  const { toast } = useToast()
  const [password, setPassword] = useState("")
  const [busy, setBusy] = useState(false)
  const [log, setLog] = useState("")

  const handle = async () => {
    if (!password) { toast("Password required", "error"); return }
    setBusy(true); setLog("")
    try {
      const out = await SudoAptInstall(password, pkgs)
      setLog(out as any)
      toast(`Installed ${pkgs.join(", ")}`, "success")
      onDone?.()
      onOpenChange(false)
    } catch (e) { setLog(String(e)); toast(String(e), "error") } finally { setBusy(false); setPassword("") }
  }

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/60 backdrop-blur-sm z-50" />
        <Dialog.Content className="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 w-[520px] max-w-[95vw] bg-card border rounded-xl shadow-xl z-50 p-4 space-y-3">
          <Dialog.Title className="font-semibold">Sudo required — install system packages</Dialog.Title>
          <Dialog.Description className="text-sm text-muted-foreground">Enter your sudo password to run: <span className="mono">sudo apt-get install -y {pkgs.join(" ")}</span>. Password is not stored, used once via sudo -S.</Dialog.Description>
          <div className="field"><span className="field-label">Sudo password</span><input type="password" className="field-input" value={password} onChange={e=>setPassword(e.target.value)} placeholder="••••••••" autoFocus /></div>
          {log && <pre className="bg-muted p-3 rounded-lg text-xs max-h-[200px] overflow-auto whitespace-pre-wrap">{log}</pre>}
          <div className="flex justify-between">
            <span className="text-xs text-muted-foreground">{pkgs.length} packages</span>
            <div className="flex gap-2">
              <Button variant="outline" onClick={()=>onOpenChange(false)}>Cancel</Button>
              <Button variant="primary" loading={busy} onClick={handle}>Install with sudo</Button>
            </div>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
