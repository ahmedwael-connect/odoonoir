import { useEffect, useRef, useState, useCallback } from "react"
import { ShellURL, GetShellCommand, ShellCheck, Databases, ValidateDBConfig } from "../../bindings/github.com/ahmed/odoonoir/gui/app"
import type { DatabaseView } from "../../bindings/github.com/ahmed/odoonoir/internal/service/models"
import { useToast } from "../App"
import { Button } from "../components/atoms/Button"

const MAX_RECONNECT_ATTEMPTS = 5
const BASE_RECONNECT_DELAY = 1000

export function TerminalScreen({ name }: { name: string }) {
  const { toast } = useToast()
  const termRef = useRef<HTMLDivElement>(null)
  const xtermRef = useRef<any>(null)
  const wsRef = useRef<WebSocket | null>(null)
  const reconnectAttempts = useRef(0)
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const [databases, setDatabases] = useState<DatabaseView[]>([])
  const [dbName, setDbName] = useState("")
  const [connected, setConnected] = useState(false)
  const [shellUrl, setShellUrl] = useState("")
  const [dbError, setDbError] = useState<string | null>(null)
  const [configWarns, setConfigWarns] = useState<string[]>([])
  const [systemCmd, setSystemCmd] = useState<string>("")
  const [showSystem, setShowSystem] = useState(false)
  const [reconnecting, setReconnecting] = useState(false)

  const loadDbs = useCallback(async () => {
    setDbError(null)
    try {
      const list = await Databases(name) as any
      setDatabases(list)
      if (list?.length && !dbName) {
        // prefer primary, else first
        const primary = (list as any[]).find((d: any) => d.primary || d.Primary) || (list as any[])[0]
        setDbName(primary.name || primary.Name)
      }
      // fetch config drift warnings (non-fatal)
      try {
        const warns = await ValidateDBConfig(name) as any
        if (warns?.length) setConfigWarns(warns)
      } catch {}
    } catch (e) {
      const msg = String(e)
      setDbError(msg)
      toast(msg, "error")
    }
  }, [name, dbName, toast])

  useEffect(() => { loadDbs() }, [loadDbs])

  const connect = useCallback(async () => {
    if (!dbName) { toast("Select a database first", "error"); return }
    try {
      // pre-check to give immediate detailed error (python/odoo-bin/conf)
      try { await ShellCheck(name, dbName) } catch (e) { const msg=String(e); setDbError(msg); toast(msg, "error"); return }
      const url = await ShellURL(name)
      const wsUrl = url.replace("http://", "ws://").replace("https://", "wss://") + `?db=${dbName}`
      setShellUrl(wsUrl)
    } catch (e) { const msg=String(e); setDbError(msg); toast(msg, "error") }
  }, [name, dbName, toast])

  const showSystemTerminal = useCallback(async () => {
    try {
      const cmd = await GetShellCommand(name, dbName) as any
      setSystemCmd(String(cmd))
      setShowSystem(true)
    } catch (e) { toast(String(e), "error") }
  }, [name, dbName, toast])

  // history + snippets
  const snippets = [
    "env['res.partner'].search([], limit=5)",
    "env['sale.order'].search([('state','=','draft')], limit=5)",
    "env.user.name",
    "self.env.context",
    "env['ir.model'].search([('model','=','res.partner')], limit=1)",
  ]

  useEffect(() => {
    if (!shellUrl || !termRef.current) return
    let term: any, fitAddon: any, webLinks: any, ws: WebSocket
    let cancelled = false
    ;(async () => {
      const { Terminal } = await import("xterm")
      const { FitAddon } = await import("xterm-addon-fit")
      const { WebLinksAddon } = await import("xterm-addon-web-links")
      if (cancelled || !termRef.current) return
      term = new Terminal({ cursorBlink: true, fontFamily: "var(--font-mono)", fontSize: 13, theme: { background: "#0a0c10", foreground: "#e0e0e0" }, allowProposedApi: true })
      fitAddon = new FitAddon()
      webLinks = new WebLinksAddon()
      term.loadAddon(fitAddon)
      term.loadAddon(webLinks)
      term.open(termRef.current!)
      fitAddon.fit()
      xtermRef.current = term
      // history
      const histKey = `odoonoir:shell:${name}:${dbName}`
      let hist: string[] = JSON.parse(localStorage.getItem(histKey) || "[]")
      let histIdx = hist.length
      ws = new WebSocket(shellUrl)
      wsRef.current = ws
      ws.binaryType = "arraybuffer"
      ws.onclose = () => {
        setConnected(false)
        // Auto-reconnect with exponential backoff
        if (!cancelled && reconnectAttempts.current < MAX_RECONNECT_ATTEMPTS) {
          const delay = BASE_RECONNECT_DELAY * Math.pow(2, reconnectAttempts.current)
          reconnectAttempts.current++
          setReconnecting(true)
          term.writeln(`\r\n[Disconnected — reconnecting in ${delay / 1000}s (attempt ${reconnectAttempts.current}/${MAX_RECONNECT_ATTEMPTS})…]\r\n`)
          reconnectTimer.current = setTimeout(() => {
            if (!cancelled && shellUrl) {
              // Re-trigger by updating shellUrl ref (same URL, fresh WS)
              setShellUrl(u => u)
            }
          }, delay)
        } else {
          term.writeln("\r\n[Disconnected — Reconnect to continue]\r\n")
          setReconnecting(false)
        }
      }
      ws.onerror = () => {
        setConnected(false)
        if (reconnectAttempts.current >= MAX_RECONNECT_ATTEMPTS) {
          const msg = `[Connection error — ${shellUrl} — check DB, python, odoo-bin. Try System terminal below]`
          term.writeln("\r\n" + msg + "\r\n")
          setDbError(msg)
          setReconnecting(false)
        }
      }
      ws.onopen = () => {
        setConnected(true)
        reconnectAttempts.current = 0
        setReconnecting(false)
        term.writeln("\r\n[Connected to Odoo Shell — history ↑↓, Ctrl+L clear, snippets below]\r\n")
      }
      ws.onmessage = (ev) => {
        if (ev.data instanceof ArrayBuffer) term.write(new TextDecoder().decode(ev.data))
        else term.write(ev.data)
      }
      let buf = ""
      term.onData((data: string) => {
        // handle history and clear
        if (data === "\r") {
          if (buf.trim()) { hist.push(buf); if (hist.length>100) hist.shift(); localStorage.setItem(histKey, JSON.stringify(hist)); histIdx = hist.length }
          buf = ""
        } else if (data === "\x7f") {
          buf = buf.slice(0,-1)
        } else if (data.length===1 && data >= " ") {
          buf += data
        }
        if (ws.readyState === WebSocket.OPEN) ws.send(data)
      })
      term.attachCustomKeyEventHandler((ev: KeyboardEvent) => {
        if (ev.key === "c" && ev.ctrlKey) { if (ws.readyState===WebSocket.OPEN) ws.send("\x03"); return false }
        if (ev.key === "l" && ev.ctrlKey) { term.clear(); return false }
        if (ev.key === "ArrowUp") {
          if (histIdx>0) { histIdx--; const cmd = hist[histIdx]||""; term.write("\r\x1b[K" + cmd); buf = cmd }
          return false
        }
        if (ev.key === "ArrowDown") {
          if (histIdx < hist.length-1) { histIdx++; const cmd = hist[histIdx]||""; term.write("\r\x1b[K"+cmd); buf=cmd } else { histIdx=hist.length; term.write("\r\x1b[K"); buf="" }
          return false
        }
        return true
      })
      const onResize = () => { try{ fitAddon.fit(); if (ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify({ type: "resize", rows: term.rows, cols: term.cols })) }catch{} }
      window.addEventListener("resize", onResize)
      // drag resize observer
      const ro = new ResizeObserver(onResize)
      if (termRef.current) ro.observe(termRef.current)
      return () => { window.removeEventListener("resize", onResize); ro.disconnect() }
    })()
    return () => {
      cancelled = true
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current)
      try { wsRef.current?.close() } catch {}
      try { xtermRef.current?.dispose() } catch {}
      setConnected(false)
      setReconnecting(false)
      reconnectAttempts.current = 0
    }
  }, [shellUrl, name, dbName])

  return (
    <div className="screen">
      <div className="card flex flex-col max-h-[75vh] rounded-[20px] shadow-[var(--bento-shadow)]">
        <div className="card-header shrink-0">
          <h2>Terminal — {name}</h2>
          <div className="action-row">
            <select className="field-input" style={{ minWidth: 160 }} value={dbName} onChange={e => setDbName(e.target.value)}>
              <option value="">Select DB</option>
              {databases.map(d => <option key={d.name} value={d.name}>{d.name}{d.primary?" (primary)":""}</option>)}
            </select>
            <Button variant="primary" onClick={connect} disabled={!dbName || connected}>Connect</Button>
            <Button variant="outline" onClick={() => { wsRef.current?.close(); setShellUrl(""); reconnectAttempts.current = MAX_RECONNECT_ATTEMPTS }} disabled={!connected}>Disconnect</Button>
            <Button variant="ghost" size="sm" onClick={showSystemTerminal} disabled={!dbName}>System terminal</Button>
            <span className={`pill ${connected ? "running" : reconnecting ? "updating" : "stopped"}`}>{connected ? "connected" : reconnecting ? "reconnecting…" : "disconnected"}</span>
          </div>
        </div>
        {configWarns.length > 0 && (
          <div className="warning-banner mx-4 mt-3 p-3 rounded-lg border border-warning bg-warning/10 text-sm">
            <div className="font-medium">Config drift detected</div>
            {configWarns.map((w,i)=><div key={i} className="text-muted-foreground">• {w}</div>)}
          </div>
        )}
        {dbError && (
          <div className="error mx-4 mt-3 p-3 rounded-lg border border-destructive bg-destructive/10 text-sm flex flex-col gap-2">
            <div className="font-medium">Database connection failed</div>
            <div className="mono text-xs whitespace-pre-wrap break-all">{dbError}</div>
            <div className="flex gap-2">
              <Button variant="outline" size="sm" onClick={loadDbs}>Retry</Button>
              <Button variant="ghost" size="sm" onClick={()=>toast("Check odoo.conf db_host/db_user and systemctl status postgresql", "info")}>Hints</Button>
              <Button variant="ghost" size="sm" onClick={showSystemTerminal}>System terminal</Button>
            </div>
          </div>
        )}
        {showSystem && (
          <div className="mx-4 mt-3 p-3 rounded-lg border bg-card space-y-2">
            <div className="font-medium text-sm">Connect on system terminal</div>
            <div className="text-xs text-muted-foreground">Copy, paste into your system terminal, hit Enter — opens same odoo shell:</div>
            <div className="flex gap-2">
              <input className="field-input mono flex-1" readOnly value={systemCmd} onFocus={e=>e.currentTarget.select()} />
              <Button variant="outline" size="sm" onClick={()=>{ navigator.clipboard.writeText(systemCmd); toast("Copied", "success") }}>Copy</Button>
              <Button variant="ghost" size="sm" onClick={()=>setShowSystem(false)}>Close</Button>
            </div>
            <div className="text-xs mono bg-muted p-2 rounded">{systemCmd || "—"}</div>
            <div className="text-xs text-muted-foreground">Tip: runs <span className="mono">{systemCmd.split(" ")[0]}</span> with your instance's venv & conf — works even if WS fails.</div>
          </div>
        )}
        {!dbName && !dbError && !showSystem && <div className="empty">Select a database to start Odoo shell. Works when instance is stopped or running (shell uses `odoo shell -d`). Or use System terminal.</div>}
        <div className="px-3 py-2 flex gap-2 items-center border-t bg-muted/20">
          <select className="field-input text-xs" style={{ minWidth: 220 }} defaultValue="" onChange={e=>{ const v=e.target.value; if(v && wsRef.current?.readyState===WebSocket.OPEN) wsRef.current.send(v+"\n"); e.target.value="" }}>
            <option value="">Snippets…</option>
            {snippets.map(s=><option key={s} value={s}>{s.slice(0,60)}</option>)}
          </select>
          <label className="text-xs flex items-center gap-1 cursor-pointer border rounded px-2 py-1 bg-card hover:bg-accent">
            <input type="file" accept=".py" className="hidden" onChange={e=>{
              const f=e.target.files?.[0]; if(!f) return; const r=new FileReader(); r.onload=()=>{ const txt=r.result as string; if(wsRef.current?.readyState===WebSocket.OPEN) wsRef.current.send(txt+"\n") }; r.readAsText(f)
            }} /> Upload .py
          </label>
          <Button variant="ghost" size="sm" onClick={()=>xtermRef.current?.clear()}>Clear</Button>
          <Button variant="ghost" size="sm" onClick={()=>{ if(xtermRef.current) { const sel=xtermRef.current.getSelection(); if(sel) navigator.clipboard.writeText(sel) } }}>Copy sel</Button>
          <span className="ml-auto text-xs text-muted-foreground hidden md:inline">↑↓ history • Ctrl+C/K • autodetect `lxml.html.clean` already fixed</span>
        </div>
        <div ref={termRef} className="log-container flex-1 min-h-[320px] h-[420px] max-h-[55vh] resize-y overflow-hidden rounded-[12px] m-3" style={{ background: "#0a0c10", padding: 4 }} />
        <div className="p-3 text-xs text-muted-foreground border-t shrink-0 flex gap-2 flex-wrap">Tip: <kbd>Ctrl+C</kbd> interrupt, `env['res.partner'].search([])` to test. <span className="ml-auto">Drag bottom edge to resize • <kbd>Ctrl+L</kbd> clear • Web links clickable</span></div>
      </div>
    </div>
  )
}