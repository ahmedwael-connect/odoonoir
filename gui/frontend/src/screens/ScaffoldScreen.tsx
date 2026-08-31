import { useState } from "react"
import { ScaffoldModule } from "../../bindings/github.com/ahmed/odoonoir/gui/app"
import { useToast } from "../App"
import { Button } from "../components/atoms/Button"

export function ScaffoldScreen({ name }: { name: string }) {
  const { toast } = useToast()
  const [modName, setModName] = useState("my_module")
  const [displayName, setDisplayName] = useState("My Module")
  const [summary, setSummary] = useState("My custom module")
  const [author, setAuthor] = useState("OdooNoir")
  const [license, setLicense] = useState("LGPL-3")
  const [version, setVersion] = useState("18.0.1.0.0")
  const [category, setCategory] = useState("Tools")
  const [addonsDir, setAddonsDir] = useState("")
  const [depends, setDepends] = useState("base")
  const [models, setModels] = useState("my.model")
  const [menus, setMenus] = useState(true)
  const [wizard, setWizard] = useState(false)
  const [tests, setTests] = useState(true)
  const [busy, setBusy] = useState(false)

  const handle = async () => {
    setBusy(true)
    try {
      const deps = depends.split(",").map(s => s.trim()).filter(Boolean)
      const mods = models.split(",").map(s => s.trim()).filter(Boolean).map(m => ({ Name: m, Label: m, Description: "", Fields: [] } as any))
      await ScaffoldModule(modName, displayName, summary, author, license, version, category, addonsDir, deps, mods, menus, wizard, tests)
      toast(`Module ${modName} scaffolded`, "success")
    } catch (e) { toast(String(e), "error") } finally { setBusy(false) }
  }

  return (
    <div className="screen">
      <div className="card">
        <div className="card-header"><h2>Scaffold — {name}</h2></div>
        <div className="p-4 grid grid-cols-1 md:grid-cols-2 gap-4">
          <div className="field"><span className="field-label">Technical Name</span><input className="field-input mono" value={modName} onChange={e => setModName(e.target.value)} /></div>
          <div className="field"><span className="field-label">Display Name</span><input className="field-input" value={displayName} onChange={e => setDisplayName(e.target.value)} /></div>
          <div className="field"><span className="field-label">Summary</span><input className="field-input" value={summary} onChange={e => setSummary(e.target.value)} /></div>
          <div className="field"><span className="field-label">Author</span><input className="field-input" value={author} onChange={e => setAuthor(e.target.value)} /></div>
          <div className="field"><span className="field-label">License</span><select className="field-input" value={license} onChange={e => setLicense(e.target.value)}><option>LGPL-3</option><option>AGPL-3</option><option>MIT</option></select></div>
          <div className="field"><span className="field-label">Version</span><input className="field-input mono" value={version} onChange={e => setVersion(e.target.value)} /></div>
          <div className="field"><span className="field-label">Category</span><input className="field-input" value={category} onChange={e => setCategory(e.target.value)} /></div>
          <div className="field"><span className="field-label">Addons Dir (empty=auto)</span><input className="field-input mono" value={addonsDir} onChange={e => setAddonsDir(e.target.value)} placeholder="/path/to/addons or empty" /></div>
          <div className="field"><span className="field-label">Depends (comma)</span><input className="field-input mono" value={depends} onChange={e => setDepends(e.target.value)} /></div>
          <div className="field"><span className="field-label">Models (comma)</span><input className="field-input mono" value={models} onChange={e => setModels(e.target.value)} /></div>
          <label className="checkbox-label"><input type="checkbox" checked={menus} onChange={e => setMenus(e.target.checked)} /> Menus</label>
          <label className="checkbox-label"><input type="checkbox" checked={wizard} onChange={e => setWizard(e.target.checked)} /> Wizard</label>
          <label className="checkbox-label"><input type="checkbox" checked={tests} onChange={e => setTests(e.target.checked)} /> Tests</label>
        </div>
        <div className="p-4 border-t">
          <Button variant="primary" onClick={handle} disabled={busy}>{busy ? "Scaffolding…" : "Scaffold Module"}</Button>
        </div>
      </div>
    </div>
  )
}