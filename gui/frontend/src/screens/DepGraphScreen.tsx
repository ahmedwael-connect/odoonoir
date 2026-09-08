import { useEffect, useRef, useState, useCallback, useDeferredValue } from "react"
import * as d3 from "d3"
import { ModuleDepGraph, Databases, ValidateDBConfig } from "../../bindings/github.com/ahmed/odoonoir/gui/app"
import type { DatabaseView } from "../../bindings/github.com/ahmed/odoonoir/internal/service/models"
import { useToast } from "../App"
import { Button } from "../components/atoms/Button"

type Node = { id: string; name: string; state: string; onDisk: boolean; inDB: boolean; depends: string[]; x?: number; y?: number; fx?: number | null; fy?: number | null }
type Link = { source: string | Node; target: string | Node }

export function DepGraphScreen({ name }: { name: string }) {
  const { toast } = useToast()
  const [dbs, setDbs] = useState<DatabaseView[]>([])
  const [dbName, setDbName] = useState("")
  const [dbError, setDbError] = useState<string | null>(null)
  const [configWarns, setConfigWarns] = useState<string[]>([])
  const [graph, setGraph] = useState<{ nodes: Node[]; links: Link[] } | null>(null)
  const [loading, setLoading] = useState(false)
  const [filter, setFilter] = useState("")
  const deferredFilter = useDeferredValue(filter)
  const [onlyInstalled, setOnlyInstalled] = useState(false)
  const svgRef = useRef<SVGSVGElement>(null)
  const wrapRef = useRef<HTMLDivElement>(null)

  const loadDbs = useCallback(async () => {
    setDbError(null)
    try {
      const list:any = await Databases(name)
      setDbs(list)
      if (list?.length && !dbName) {
        const primary = (list as any[]).find((d:any)=> d.primary||d.Primary) || (list as any[])[0]
        setDbName(primary.name || primary.Name)
      }
      try { const warns:any = await ValidateDBConfig(name); if (warns?.length) setConfigWarns(warns) } catch {}
    } catch (e) { const msg=String(e); setDbError(msg); toast(msg,"error") }
  }, [name, dbName, toast])
  useEffect(() => { loadDbs() }, [loadDbs])

  const load = useCallback(async () => {
    if (!dbName) return
    setLoading(true)
    try {
      const g: any = await ModuleDepGraph(name, dbName, { OnlyInstalled: onlyInstalled } as any)
      const nodes: Node[] = (g.Nodes || g.nodes || []).map((n: any) => ({ id: n.Name || n.name, name: n.Name || n.name, state: n.State || n.state || "", onDisk: n.OnDisk ?? n.onDisk ?? true, inDB: n.InDB ?? n.inDB ?? true, depends: n.Depends || n.depends || [] }))
      const links: Link[] = (g.Links || g.links || []).map((l: any) => ({ source: l.Source || l.source, target: l.Target || l.target }))
      setGraph({ nodes, links })
    } catch (e) { toast(String(e), "error") } finally { setLoading(false) }
  }, [name, dbName, onlyInstalled, toast])

  useEffect(() => { load() }, [load])

  useEffect(() => {
    if (!graph || !svgRef.current || !wrapRef.current) return
    const svg = d3.select(svgRef.current)
    const w = wrapRef.current.clientWidth, h = 560
    svg.attr("viewBox", `0 0 ${w} ${h}`).selectAll("*").remove()
    const g = svg.append("g")
    const zoom = d3.zoom<SVGSVGElement, unknown>().scaleExtent([0.2, 3]).on("zoom", e => g.attr("transform", e.transform))
    svg.call(zoom as any)

    const filteredNodes = deferredFilter ? graph.nodes.filter(n => n.name.toLowerCase().includes(deferredFilter.toLowerCase()) || n.state.toLowerCase().includes(deferredFilter.toLowerCase())) : graph.nodes
    const idSet = new Set(filteredNodes.map(n => n.id))
    const filteredLinks = graph.links.filter(l => {
      const s = typeof l.source === "string" ? l.source : (l.source as Node).id
      const t = typeof l.target === "string" ? l.target : (l.target as Node).id
      return idSet.has(s) && idSet.has(t)
    })

    const sim = d3.forceSimulation<Node>(filteredNodes as any)
      .force("link", d3.forceLink<Node, Link>(filteredLinks as any).id((d: any) => d.id).distance(90).strength(0.6))
      .force("charge", d3.forceManyBody().strength(-250))
      .force("center", d3.forceCenter(w / 2, h / 2))
      .force("collide", d3.forceCollide(40))

    const link = g.append("g").selectAll("line").data(filteredLinks).enter().append("line").attr("stroke", "var(--border)").attr("stroke-width", 1.2).attr("marker-end", "url(#arrow)")
    const node = g.append("g").selectAll("g").data(filteredNodes).enter().append("g").call(d3.drag<SVGGElement, Node>().on("start", (e, d) => { if (!e.active) sim.alphaTarget(0.3).restart(); d.fx = d.x; d.fy = d.y }).on("drag", (e, d) => { d.fx = e.x; d.fy = e.y }).on("end", (e, d) => { if (!e.active) sim.alphaTarget(0); d.fx = null; d.fy = null }) as any)

    node.append("circle").attr("r", 22).attr("fill", d => {
      if (!d.inDB && d.onDisk) return "var(--warning)"
      if (d.inDB && !d.onDisk) return "var(--destructive)"
      if (d.state === "installed") return "var(--success)"
      if (d.state === "to upgrade") return "var(--warning)"
      return "var(--border)"
    }).attr("stroke", "var(--card)").attr("stroke-width", 2)

    node.append("text").attr("text-anchor", "middle").attr("dy", 4).attr("font-size", "9px").attr("fill", "var(--foreground)").text(d => d.name.length > 14 ? d.name.slice(0, 12) + "…" : d.name)

    const tooltip = d3.select(wrapRef.current).append("div").attr("class", "absolute bg-popover border rounded-lg shadow-lg p-3 text-xs pointer-events-none opacity-0").style("position", "absolute")
    node.on("mouseover", (e, d) => { tooltip.style("opacity", "1").html(`<b>${d.name}</b><br/>state: ${d.state}<br/>onDisk:${d.onDisk} inDB:${d.inDB}<br/>depends: ${d.depends.join(", ") || "—"}`).style("left", (e.pageX - (wrapRef.current?.getBoundingClientRect().left || 0) + 12) + "px").style("top", (e.pageY - (wrapRef.current?.getBoundingClientRect().top || 0) - 10) + "px") }).on("mouseout", () => tooltip.style("opacity", "0"))

    sim.on("tick", () => {
      link.attr("x1", d => (d.source as any).x).attr("y1", d => (d.source as any).y).attr("x2", d => (d.target as any).x).attr("y2", d => (d.target as any).y)
      node.attr("transform", d => `translate(${d.x},${d.y})`)
    })

    return () => { sim.stop(); tooltip.remove() }
  }, [graph, deferredFilter])

  return (
    <div className="screen">
      <div className="card">
        <div className="card-header">
          <h2>Graph — {name}</h2>
          <div className="action-row">
            <select className="field-input" value={dbName} onChange={e => setDbName(e.target.value)} style={{ minWidth: 140 }}>{dbs.map(d => <option key={d.name} value={d.name}>{d.name}</option>)}</select>
            <input className="search-input" placeholder="Filter module…" value={filter} onChange={e => setFilter(e.target.value)} />
            <label className="checkbox-label"><input type="checkbox" checked={onlyInstalled} onChange={e => setOnlyInstalled(e.target.checked)} /> Only installed</label>
            <Button variant="outline" onClick={load} disabled={loading}>{loading ? "Loading…" : "Refresh"}</Button>
          </div>
        </div>
        {configWarns.length > 0 && (
          <div className="warning-banner mx-4 mt-3 p-3 rounded-lg border border-warning bg-warning/10 text-sm"><div className="font-medium">Config drift</div>{configWarns.map((w,i)=><div key={i} className="text-muted-foreground">• {w}</div>)}</div>
        )}
        {dbError && (
          <div className="error mx-4 mt-3 p-3 rounded-lg border border-destructive bg-destructive/10 text-sm flex flex-col gap-2"><div className="font-medium">Database error</div><div className="mono text-xs whitespace-pre-wrap break-all">{dbError}</div><div className="flex gap-2"><Button variant="outline" size="sm" onClick={loadDbs}>Retry</Button><Button variant="ghost" size="sm" onClick={load}>Refresh graph</Button></div></div>
        )}
        <div ref={wrapRef} className="relative bg-card border-t" style={{ height: 560 }}>
          <svg ref={svgRef} width="100%" height="100%" className="block">
            <defs><marker id="arrow" viewBox="0 0 10 10" refX={9} refY={5} markerWidth={6} markerHeight={6} orient="auto"><path d="M 0 0 L 10 5 L 0 10 z" fill="var(--border)" /></marker></defs>
          </svg>
          {!graph && !loading && <div className="absolute inset-0 flex items-center justify-center text-sm text-muted-foreground">Load graph to see dependencies</div>}
          {loading && <div className="absolute inset-0 flex items-center justify-center"><span className="spinner" /> Loading graph…</div>}
        </div>
        <div className="p-3 flex gap-2 text-xs">
          <span className="pill success">installed</span><span className="pill warning">to upgrade</span><span className="pill stopped">not installed</span><span className="pill destructive">missing</span>
          <span className="ml-auto text-muted-foreground">{graph ? `${graph.nodes.length} nodes, ${graph.links.length} links` : ""}</span>
        </div>
      </div>
    </div>
  )
}