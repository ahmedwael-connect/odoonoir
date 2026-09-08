import { useState, useEffect } from "react"
import { ScaffoldModule, ListAddonPaths, Status } from "../../bindings/github.com/ahmed/odoonoir/gui/app"
import { useToast } from "../App"
import { Button } from "../components/atoms/Button"

type FieldDef = { name: string; label: string; type: string; required: boolean; relation: string; help: string; readonly: boolean; index: boolean; selection: string; defaultValue: string }
type ModelDef = { name: string; label: string; description: string; recName: string; order: string; fields: FieldDef[] }

const fieldTypes = ["char","text","html","boolean","integer","float","monetary","date","datetime","selection","many2one","one2many","many2many"]
const odooCategories = ["Sales","Inventory","Accounting","Manufacturing","Project","Human Resources","Marketing","Website","Point of Sale","Services","Tools","Customization","Administration","Uncategorized"]

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
  const [addonsOptions, setAddonsOptions] = useState<string[]>([])
  const [depends, setDepends] = useState("base")
  const [menus, setMenus] = useState(true)
  const [wizard, setWizard] = useState(false)
  const [tests, setTests] = useState(true)
  const [controllers, setControllers] = useState(false)
  const [demo, setDemo] = useState(false)
  const [security, setSecurity] = useState(true)
  const [mailThread, setMailThread] = useState(false)
  const [activity, setActivity] = useState(false)
  const [models, setModels] = useState<ModelDef[]>([{ name: "my.model", label: "My Model", description: "", recName: "name", order: "name", fields: [{ name: "name", label: "Name", type: "char", required: true, relation: "", help: "", readonly: false, index: false, selection: "", defaultValue: "" }] }])
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    Status(name).then((st:any)=>{
      const ver = st?.Instance?.version || st?.instance?.version || "18.0"
      const major = String(ver).split(".")[0]
      const stamp = new Date().toISOString().slice(0,10).replace(/-/g,"")
      setVersion(`${major}.0.1.${stamp}`)
    }).catch(()=>{})
    ListAddonPaths(name).then((list:any)=>{
      const paths = (list||[]).map((e:any)=> e.path || e.Path).filter(Boolean)
      setAddonsOptions(paths)
      if (paths.length && !addonsDir) setAddonsDir(paths[paths.length-1])
    }).catch(()=>{})
  }, [name])

  const addModel = () => setModels([...models, { name: `my.model${models.length+1}`, label: `Model ${models.length+1}`, description: "", recName: "name", order: "name", fields: [] }])
  const updateModel = (idx: number, patch: Partial<ModelDef>) => setModels(models.map((m,i)=> i===idx ? {...m, ...patch} : m))
  const removeModel = (idx: number) => setModels(models.filter((_,i)=>i!==idx))
  const addField = (mIdx: number) => {
    const m = models[mIdx]
    updateModel(mIdx, { fields: [...m.fields, { name: `field_${m.fields.length+1}`, label: `Field ${m.fields.length+1}`, type: "char", required: false, relation: "", help: "", readonly: false, index: false, selection: "", defaultValue: "" }] })
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
      const mods = models.map(m => ({ Name: m.name, Label: m.label, Description: m.description, Fields: m.fields.map(f=> ({ Name: f.name, Label: f.label, Type: f.type, Relation: f.relation, Required: f.required, Help: f.help, Readonly: f.readonly, Index: f.index, Selection: f.selection, Default: f.defaultValue } as any)) } as any))
      // extend scaffold call with new flags: include mailThread/activity via security/demo handling
      const extraDeps = [...deps]
      if (mailThread && !extraDeps.includes("mail")) extraDeps.push("mail")
      await ScaffoldModule(modName, displayName, summary, author, license, version, category, addonsDir, extraDeps, mods, menus, wizard, tests, controllers, demo, security, mailThread, activity as any)
      toast(`Module ${modName} scaffolded with ${mods.length} models — ready to code`, "success")
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
          <div className="field"><span className="field-label">Version *</span><input className="field-input mono" value={version} onChange={e => setVersion(e.target.value)} placeholder="18.0.1.0.0" /><span className="text-xs text-muted-foreground">Auto from instance {name} version</span></div>
          <div className="field"><span className="field-label">Category *</span><select className="field-input" value={category} onChange={e=>setCategory(e.target.value)}>{odooCategories.map((c:string)=><option key={c} value={c}>{c}</option>)}<option value="Custom">Custom</option></select></div>
          <div className="field"><span className="field-label">Depends (comma)</span><input className="field-input mono" value={depends} onChange={e => setDepends(e.target.value)} placeholder="base, web, mail" /></div>
          <div className="field md:col-span-2"><span className="field-label">Addons Dir *</span>
            {addonsOptions.length ? (
              <select className="field-input mono" value={addonsDir} onChange={e=>setAddonsDir(e.target.value)}>
                <option value="">auto (custom_addons)</option>
                {addonsOptions.map(p=><option key={p} value={p}>{p}</option>)}
                <option value="custom">— custom path —</option>
              </select>
            ) : (
              <input className="field-input mono" value={addonsDir} onChange={e => setAddonsDir(e.target.value)} placeholder="/path/to/addons or empty" />
            )}
            {addonsDir==="custom" && <input className="field-input mono mt-2" placeholder="/custom/path" onChange={e=>setAddonsDir(e.target.value)} />}
          </div>
        </div>

        <div className="p-4 border-t">
          <div className="flex justify-between items-center mb-3">
            <h3 className="font-semibold">Models & Fields</h3>
            <Button variant="outline" size="sm" onClick={addModel}>+ Add Model</Button>
          </div>
          <div className="space-y-4">
            {models.map((m, mi) => (
              <div key={mi} className="border rounded-xl p-4 bg-muted/20 space-y-3">
                <div className="grid grid-cols-1 md:grid-cols-4 gap-3">
                  <input className="field-input mono" value={m.name} onChange={e=>updateModel(mi,{name:e.target.value})} placeholder="my.model" />
                  <input className="field-input" value={m.label} onChange={e=>updateModel(mi,{label:e.target.value})} placeholder="Label" />
                  <input className="field-input" value={m.recName} onChange={e=>updateModel(mi,{recName:e.target.value})} placeholder="rec_name (name)" />
                  <div className="flex gap-2">
                    <input className="field-input flex-1" value={m.order} onChange={e=>updateModel(mi,{order:e.target.value})} placeholder="order (name)" />
                    <Button variant="ghost" size="sm" onClick={()=>removeModel(mi)}>Remove</Button>
                  </div>
                  <input className="field-input md:col-span-4" value={m.description} onChange={e=>updateModel(mi,{description:e.target.value})} placeholder="Description for model" />
                </div>
                <div className="space-y-2">
                  <div className="flex justify-between items-center">
                    <span className="text-xs font-medium">Fields ({m.fields.length}) — detailed</span>
                    <Button variant="outline" size="sm" onClick={()=>addField(mi)}>+ Field</Button>
                  </div>
                  {m.fields.map((f, fi)=>(
                    <div key={fi} className="grid grid-cols-12 gap-2 items-center bg-card p-2 rounded-lg border">
                      <input className="field-input mono col-span-2" value={f.name} onChange={e=>updateField(mi,fi,{name:e.target.value})} placeholder="field_name" />
                      <input className="field-input col-span-2" value={f.label} onChange={e=>updateField(mi,fi,{label:e.target.value})} placeholder="Label" />
                      <select className="field-input col-span-2" value={f.type} onChange={e=>updateField(mi,fi,{type:e.target.value})}>
                        {fieldTypes.map(t=><option key={t} value={t}>{t}</option>)}
                      </select>
                      {(f.type==="many2one"||f.type==="one2many"||f.type==="many2many") ? <input className="field-input mono col-span-2" value={f.relation} onChange={e=>updateField(mi,fi,{relation:e.target.value})} placeholder="res.partner" /> : f.type==="selection" ? <input className="field-input mono col-span-2" value={f.selection} onChange={e=>updateField(mi,fi,{selection:e.target.value})} placeholder="a,b,c" /> : <input className="field-input mono col-span-2" value={f.help} onChange={e=>updateField(mi,fi,{help:e.target.value})} placeholder="help" />}
                      <div className="col-span-2 flex gap-1 flex-wrap">
                        <label className="flex items-center gap-1 text-xs"><input type="checkbox" checked={f.required} onChange={e=>updateField(mi,fi,{required:e.target.checked})} /> Req</label>
                        <label className="flex items-center gap-1 text-xs"><input type="checkbox" checked={f.readonly} onChange={e=>updateField(mi,fi,{readonly:e.target.checked})} /> RO</label>
                        <label className="flex items-center gap-1 text-xs"><input type="checkbox" checked={f.index} onChange={e=>updateField(mi,fi,{index:e.target.checked})} /> Idx</label>
                      </div>
                      <input className="field-input mono col-span-1" value={f.defaultValue} onChange={e=>updateField(mi,fi,{defaultValue:e.target.value})} placeholder="default" />
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
          <h3 className="font-semibold mb-3">Features — make it ready to design</h3>
          <div className="grid grid-cols-2 md:grid-cols-3 gap-3">
            <label className="flex items-center gap-2 p-3 border rounded-xl bg-card hover:bg-accent cursor-pointer"><input type="checkbox" checked={menus} onChange={e=>setMenus(e.target.checked)} /> Menus & Actions</label>
            <label className="flex items-center gap-2 p-3 border rounded-xl bg-card hover:bg-accent cursor-pointer"><input type="checkbox" checked={wizard} onChange={e=>setWizard(e.target.checked)} /> Wizard (TransientModel)</label>
            <label className="flex items-center gap-2 p-3 border rounded-xl bg-card hover:bg-accent cursor-pointer"><input type="checkbox" checked={tests} onChange={e=>setTests(e.target.checked)} /> Tests (TransactionCase)</label>
            <label className="flex items-center gap-2 p-3 border rounded-xl bg-card hover:bg-accent cursor-pointer"><input type="checkbox" checked={controllers} onChange={e=>setControllers(e.target.checked)} /> Controller (/hello)</label>
            <label className="flex items-center gap-2 p-3 border rounded-xl bg-card hover:bg-accent cursor-pointer"><input type="checkbox" checked={demo} onChange={e=>setDemo(e.target.checked)} /> Demo data</label>
            <label className="flex items-center gap-2 p-3 border rounded-xl bg-card hover:bg-accent cursor-pointer"><input type="checkbox" checked={security} onChange={e=>setSecurity(e.target.checked)} /> Security (groups + CSV)</label>
            <label className="flex items-center gap-2 p-3 border rounded-xl bg-card hover:bg-accent cursor-pointer"><input type="checkbox" checked={mailThread} onChange={e=>setMailThread(e.target.checked)} /> Mail Thread (mail.thread)</label>
            <label className="flex items-center gap-2 p-3 border rounded-xl bg-card hover:bg-accent cursor-pointer"><input type="checkbox" checked={activity} onChange={e=>setActivity(e.target.checked)} /> Activity Mix (mail.activity.mixin)</label>
          </div>
          <div className="text-xs text-muted-foreground mt-2">Mail Thread adds chatter to form; Activity adds kanban activities; all wired in manifest & views. Choose to make module design-ready.</div>
        </div>

        <div className="p-4 border-t flex justify-end">
          <Button variant="primary" onClick={handle} disabled={busy || !modName} loading={busy}>Scaffold Module</Button>
        </div>
      </div>
    </div>
  )
}
