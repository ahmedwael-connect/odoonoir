import { useEffect, useState, useCallback } from "react"
import { SearchMarketplaceModules, GetMarketplaceStats, SetGitHubToken, GetGitHubToken, ValidateGitHubToken, ClearGitHubToken } from "../../bindings/github.com/ahmed/odoonoir/gui/app"
import { useToast } from "../App"
import { ModuleDetailDrawer } from "../components/marketplace/ModuleDetailDrawer"
import { InstallWizard } from "../components/marketplace/InstallWizard"
import { SyncDialog } from "../components/marketplace/SyncDialog"
import { PublishWizard } from "../components/marketplace/PublishWizard"
import { Button } from "../components/atoms/Button"

export function MarketplaceScreen() {
  const { toast } = useToast()
  const [query, setQuery] = useState("")
  const [modules, setModules] = useState<any[]>([])
  const [stats, setStats] = useState<any>(null)
  const [loading, setLoading] = useState(false)
  const [filter, setFilter] = useState<"all"|"featured"|"verified"|"top">("all")
  const [detail, setDetail] = useState<any>(null)
  const [install, setInstall] = useState<any>(null)
  const [sync, setSync] = useState<any>(null)
  const [showPublish, setShowPublish] = useState(false)
  const [token, setToken] = useState("")
  const [tokenStatus, setTokenStatus] = useState<string>("")

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await SearchMarketplaceModules(query, { Page: 0, PerPage: 20, SortBy: "rating", SortOrder: "desc" } as any)
      setModules(res as any)
    } catch (e) { toast(String(e), "error") } finally { setLoading(false) }
  }, [query, toast])

  const loadStats = useCallback(async () => {
    try { const s = await GetMarketplaceStats(); setStats(s) } catch {}
  }, [])

  const loadToken = useCallback(async () => {
    try { const t = await GetGitHubToken(); setToken(t || ""); setTokenStatus(t ? "saved" : "missing") } catch {}
  }, [])

  useEffect(() => { load(); loadStats(); loadToken() }, [load, loadStats, loadToken])

  return (
    <div className="screen">
      <div className="card">
        <div className="card-header">
          <h2>Marketplace</h2>
          <div className="action-row">
            <input className="search-input" placeholder="Search modules…" value={query} onChange={e => setQuery(e.target.value)} onKeyDown={e => e.key==="Enter" && load()} />
            <Button variant="primary" onClick={load} disabled={loading}>{loading?"Searching…":"Search"}</Button>
            <Button variant="outline" onClick={()=>setShowPublish(true)}>Publish</Button>
          </div>
        </div>

        <div className="p-3 flex gap-2 items-center border-b bg-muted/30 flex-wrap">
          <input className="field-input mono" placeholder="GitHub token (ghp_…)" value={token} onChange={e=>setToken(e.target.value)} style={{ flex: 1, minWidth: 220 }} type="password" />
          <Button variant="ghost" size="sm" onClick={async()=>{ try{ await SetGitHubToken(token); setTokenStatus("saved"); window.dispatchEvent(new CustomEvent("odoonoir-token-changed")); toast("Token saved","success")}catch(e){ toast(String(e),"error")}}}>Save</Button>
          <Button variant="ghost" size="sm" onClick={async()=>{ try{ const u:any = await ValidateGitHubToken(); toast(`Valid: ${u.login||u.Login||"ok"}`,"success"); setTokenStatus("valid"); window.dispatchEvent(new CustomEvent("odoonoir-token-changed"))}catch(e){ toast(String(e),"error"); setTokenStatus("invalid")}}}>Validate</Button>
          <Button variant="ghost" size="sm" onClick={async()=>{ try{ await ClearGitHubToken(); setToken(""); setTokenStatus("missing"); window.dispatchEvent(new CustomEvent("odoonoir-token-changed")); toast("Token cleared","success")}catch(e){ toast(String(e),"error")}}}>Clear</Button>
          <span className={`text-xs px-2 py-0.5 rounded-full border ${tokenStatus==="valid"?"bg-success text-success-foreground border-success":tokenStatus==="saved"?"bg-primary/10 border-primary/20":tokenStatus==="invalid"?"bg-destructive text-destructive-foreground": "bg-muted"}`}>status: {tokenStatus||"—"}</span>
        </div>

        {stats && (
          <div className="p-3 flex gap-2 flex-wrap border-b">
            <span className="pill">{stats.TotalModules ?? stats.totalModules ?? 0} modules</span>
            <span className="pill success">{stats.VerifiedModules ?? stats.verifiedModules ?? 0} verified</span>
            <span className="pill">{stats.TotalReviews ?? stats.totalReviews ?? 0} reviews</span>
          </div>
        )}

        <div className="tabs p-2 border-b">
          <button className={`tab ${filter==="all"?"active":""}`} onClick={()=>setFilter("all")}>All</button>
          <button className={`tab ${filter==="featured"?"active":""}`} onClick={()=>setFilter("featured")}>Featured</button>
          <button className={`tab ${filter==="verified"?"active":""}`} onClick={()=>setFilter("verified")}>Verified</button>
          <button className={`tab ${filter==="top"?"active":""}`} onClick={()=>setFilter("top")}>Top</button>
        </div>

        {loading ? <div className="p-6">Loading marketplace…</div> : modules.length===0 ? <div className="empty">No modules found — try indexing a GitHub repo via <code>IndexMarketplaceModule</code></div> : (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3 p-3">
            {modules.map((m: any) => (
              <div key={m.ID || m.id || `${m.Owner}/${m.Repo}`} className="card p-3 hover:shadow-md transition-shadow">
                <div className="flex gap-2 items-start">
                  <div className="flex-1 min-w-0">
                    <div className="font-semibold truncate">{m.DisplayName || m.displayName || m.Name || m.name}</div>
                    <div className="text-xs text-muted-foreground truncate">{m.Owner}/{m.Repo} • {m.Category || m.category || "—"}</div>
                    <div className="text-sm mt-1 line-clamp-2">{m.Summary || m.summary || m.Description || ""}</div>
                    <div className="flex gap-1 mt-2 flex-wrap">
                      <span className="pill">★ {m.Stars ?? m.stars ?? 0}</span>
                      <span className="pill">{m.License || m.license || "—"}</span>
                      {m.Verified || m.verified ? <span className="pill success">verified</span> : null}
                      {m.Featured || m.featured ? <span className="pill warning">featured</span> : null}
                    </div>
                  </div>
                  <div className="text-right">
                    <div className="text-sm font-medium">{(m.Rating ?? m.rating ?? 0).toFixed(1)} ★</div>
                    <div className="text-xs text-muted-foreground">{m.ReviewCount ?? m.reviewCount ?? 0} reviews</div>
                  </div>
                </div>
                <div className="mt-3 flex gap-2">
                  <Button variant="primary" size="sm" onClick={()=>setInstall(m)}>Install</Button>
                  <Button variant="ghost" size="sm" onClick={()=>setDetail(m)}>View</Button>
                  <Button variant="ghost" size="sm" onClick={()=>setSync(m)}>Sync</Button>
                </div>
              </div>
            ))}
          </div>
        )}
        <ModuleDetailDrawer module={detail} open={!!detail} onOpenChange={(o)=>!o && setDetail(null)} onInstall={(m)=>{ setDetail(null); setInstall(m)}} />
        <InstallWizard module={install} open={!!install} onOpenChange={(o)=>!o && setInstall(null)} onInstalled={()=>load()} />
        <SyncDialog module={sync} open={!!sync} onOpenChange={(o)=>!o && setSync(null)} />
        <PublishWizard open={showPublish} onOpenChange={setShowPublish} />
      </div>
    </div>
  )
}