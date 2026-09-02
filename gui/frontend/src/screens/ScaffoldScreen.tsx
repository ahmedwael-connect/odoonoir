import { useState } from "react"
import { ScaffoldModule } from "../../bindings/github.com/ahmed/odoonoir/gui/app"
import { useToast } from "../App"
import { Button } from "../components/atoms/Button"

type FieldDef = { name: string; label: string; type: string; required: boolean; relation: string }
type ModelDef = { name: string; label: string; description: string; fields: FieldDef[] }

const fieldTypes = ["char","text","html","boolean","integer","float","monetary","date","datetime","selection","many2one","one2many","many2many"]

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
  const [menus, setMenus] = useState(true)
  const [wizard, setWizard] = useState(false)
  const [tests, setTests] = useState(true)
  const [controllers, setControllers] = useState(false)
  const [demo, setDemo] = useState(false)
  const [security, setSecurity] = useState(true)
  const [models, setModels] = useState<ModelDef[]>([{ name: "my.model", label: "My Model", description: "", fields: [{ name: "name", label: "Name", type: "char", required: true, relation: "" }] }])
  const [busy, setBusy] = useState(false)

  const addModel = () => setModels([...models, { name: `my.model${models.length+1}`, label: `Model ${models.length+1}`, description: "", fields: [] }])
  const updateModel = (idx: number, patch: Partial<ModelDef>) => setModels(models.map((m,i)=> i===idx ? {...m, ...patch} : m))
  const removeModel = (idx: number) => setModels(models.filter((_,i)=>i!==idx))
  const addField = (mIdx: number) => {
    const m = models[mIdx]
    updateModel(mIdx, { fields: [...m.fields, { name: `field_${m.fields.length+1}`, label: `Field ${m.fields.length+1}`, type: "char", required: false, relation: "" }] })
  }
  const updateField = (mIdx: number, fIdx: number, patch: Partial<FieldDef>) => {
    const m = models[mIdx]
    const fields = m.fields.map((f,i)=> i===fIdx ? {...f,...patch} : f)
    updateModel(mIdx, { fields })
  }
  const removeField = (mIdx: number, fIdx: number) => {
    const m = models[mIdx]
    updateModel(mIdx, { fields: m.fields.filter((_,i)=>i!==fIdx) })
  }

  const handle = async () => {
    setBusy(true)
    try {
      const deps = depends.split(",").map(s => s.trim()).filter(Boolean)
      const mods = models.map(m => ({ Name: m.name, Label: m.label, Description: m.description, Fields: m.fields.map(f=> ({ Name: f.name, Label: f.label, Type: f.type, Relation: f.relation, Required: f.required } as any)) } as any))
      await ScaffoldModule(modName, displayName, summary, author, license, version, category, addonsDir, deps, mods, menus, wizard, tests, controllers, demo, security as any)
      toast(`Module ${modName} scaffolded with ${mods.length} models`, "success")
    } catch (e) { toast(String(e), "error") } finally { setBusy(false) }
  }

  return (
    <div className="screen space-y-4">
      <div className="card">
        <div className="card-header"><h2>Scaffold — {name}</h2><span className="text-xs text-muted-foreground">Ready-to-code module with fields, views, security, demo</span></div>

        <div className="p-4 grid grid-cols-1 md:grid-cols-2 gap-4">
          <div className="field"><span className="field-label">Technical Name *</span><input className="field-input mono" value={modName} onChange={e => setModName(e.target.value)} placeholder="my_module" /></div>
          <div className="field"><span className="field-label">Display Name *</span><input className="field-input" value={displayName} onChange={e => setDisplayName(e.target.value)} placeholder="My Module" /></div>
          <div className="field"><span className="field-label">Summary</span><input className="field-input" value={summary} onChange={e => setSummary(e.target.value)} /></div>
          <div className="field"><span className="field-label">Author</span><input className="field-input" value={author} onChange={e => setAuthor(e.target.value)} /></div>
          <div className="field"><span className="field-label">License</span><select className="field-input" value={license} onChange={e => setLicense(e.target.value)}><option>LGPL-3</option><option>AGPL-3</option><option>MIT</option><option>OPL-1</option></select></div>
          <div className="field"><span className="field-label">Version</span><input className="field-input mono" value={version} onChange={e => setVersion(e.target.value)} placeholder="18.0.1.0.0" /></div>
          <div className="field"><span className="field-label">Category</span><input className="field-input" value={category} onChange={e => setCategory(e.target.value)} placeholder="Tools" /></div>
          <div className="field"><span className="field-label">Depends (comma)</span><input className="field-input mono" value={depends} onChange={e => setDepends(e.target.value)} placeholder="base, web" /></div>
          <div className="field md:col-span-2"><span className="field-label">Addons Dir (empty=auto instance custom_addons)</span><input className="field-input mono" value={addonsDir} onChange={e => setAddonsDir(e.target.value)} placeholder="/path/to/addons or empty" /></div>
        </div>

        <div className="p-4 border-t">
          <div className="flex justify-between items-center mb-3">
            <h3 className="font-semibold">Models & Fields</h3>
            <Button variant="outline" size="sm" onClick={addModel}>+ Add Model</Button>
          </div>
          <div className="space-y-4">
            {models.map((m, mi) => (
              <div key={mi} className="border rounded-xl p-4 bg-muted/20 space-y-3">
                <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                  <input className="field-input mono" value={m.name} onChange={e=>updateModel(mi,{name:e.target.value})} placeholder="my.model" />
                  <input className="field-input" value={m.label} onChange={e=>updateModel(mi,{label:e.target.value})} placeholder="Label" />
                  <div className="flex gap-2">
                    <input className="field-input flex-1" value={m.description} onChange={e=>updateModel(mi,{description:e.target.value})} placeholder="Description" />
                    <Button variant="ghost" size="sm" onClick={()=>removeModel(mi)}>Remove</Button>
                  </div>
                </div>
                <div className="space-y-2">
                  <div className="flex justify-between items-center">
                    <span className="text-xs font-medium">Fields ({m.fields.length})</span>
                    <Button variant="outline" size="sm" onClick={()=>addField(mi)}>+ Field</Button>
                  </div>
                  {m.fields.map((f, fi)=>(
                    <div key={fi} className="grid grid-cols-12 gap-2 items-center bg-card p-2 rounded-lg border">
                      <input className="field-input mono col-span-3" value={f.name} onChange={e=>updateField(mi,fi,{name:e.target.value})} placeholder="field_name" />
                      <input className="field-input col-span-3" value={f.label} onChange={e=>updateField(mi,fi,{label:e.target.value})} placeholder="Label" />
                      <select className="field-input col-span-2" value={f.type} onChange={e=>updateField(mi,fi,{type:e.target.value})}>
                        {fieldTypes.map(t=><option key={t} value={t}>{t}</option>)}
                      </select>
                      {(f.type==="many2one"||f.type==="one2many"||f.type==="many2many") && <input className="field-input mono col-span-2" value={f.relation} onChange={e=>updateField(mi,fi,{relation:e.target.value})} placeholder="res.partner" />}
                      <label className="flex items-center gap-1 col-span-1 text-xs"><input type="checkbox" checked={f.required} onChange={e=>updateField(mi,fi,{required:e.target.checked})} /> Req</label>
                      <Button variant="ghost" size="sm" onClick={()=>removeField(mi,fi)}>✕</Button>
                    </div>
                  ))}
                  {m.fields.length===0 && <div className="text-xs text-muted-foreground">No fields — will default to `name` Char required.</div>}
                </div>
              </div>
            ))}
          </div>
        </div>

        <div className="p-4 border-t">
          <h3 className="font-semibold mb-3">Features — make it ready</h3>
          <div className="grid grid-cols-2 md:grid-cols-3 gap-3">
            <label className="flex items-center gap-2 p-3 border rounded-xl bg-card hover:bg-accent cursor-pointer"><input type="checkbox" checked={menus} onChange={e=>setMenus(e.target.checked)} /> Menus & Actions</label>
            <label className="flex items-center gap-2 p-3 border rounded-xl bg-card hover:bg-accent cursor-pointer"><input type="checkbox" checked={wizard} onChange={e=>setWizard(e.target.checked)} /> Wizard (TransientModel)</label>
            <label className="flex items-center gap-2 p-3 border rounded-xl bg-card hover:bg-accent cursor-pointer"><input type="checkbox" checked={tests} onChange={e=>setTests(e.target.checked)} /> Tests (TransactionCase)</label>
            <label className="flex items-center gap-2 p-3 border rounded-xl bg-card hover:bg-accent cursor-pointer"><input type="checkbox" checked={controllers} onChange={e=>setControllers(e.target.checked)} /> Controller (/hello)</label>
            <label className="flex items-center gap-2 p-3 border rounded-xl bg-card hover:bg-accent cursor-pointer"><input type="checkbox" checked={demo} onChange={e=>setDemo(e.target.checked)} /> Demo data (demo.xml)</label>
            <label className="flex items-center gap-2 p-3 border rounded-xl bg-card hover:bg-accent cursor-pointer"><input type="checkbox" checked={security} onChange={e=>setSecurity(e.target.checked)} /> Security groups</label>
          </div>
          <div className="text-xs text-muted-foreground mt-2">Security creates base.group_user access + manager group; Controllers adds http.Controller; Demo adds demo.xml; all wired in manifest.</div>
        </div>

        <div className="p-4 border-t flex justify-end">
          <Button variant="primary" onClick={handle} disabled={busy || !modName} loading={busy}>Scaffold Module</Button>
        </div>
      </div>
    </div>
  )
}
