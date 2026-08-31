import { useEffect, useRef, useState, useCallback } from "react"
import { ShellURL, Databases, ValidateDBConfig } from "../../bindings/github.com/ahmed/odoonoir/gui/app"
import type { DatabaseView } from "../../bindings/github.com/ahmed/odoonoir/internal/service/models"
import { useToast } from "../App"
import { Button } from "../components/atoms/Button"

export function TerminalScreen({ name }: { name: string }) {
  const { toast } = useToast()
  const termRef = useRef<HTMLDivElement>(null)
  const xtermRef = useRef<any>(null)
  const wsRef = useRef<WebSocket | null>(null)
  const [databases, setDatabases] = useState<DatabaseView[]>([])
  const [dbName, setDbName] = useState("")
  const [connected, setConnected] = useState(false)
  const [shellUrl, setShellUrl] = useState("")
  const [dbError, setDbError] = useState<string | null>(null)
  const [configWarns, setConfigWarns] = useState<string[]>([])

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
      const url = await ShellURL(name)
      const wsUrl = url.replace("http://", "ws://").replace("https://", "wss://") + `?db=${dbName}`
      setShellUrl(wsUrl)
    } catch (e) { toast(String(e), "error") }
  }, [name, dbName, toast])

  useEffect(() => {
    if (!shellUrl || !termRef.current) return
    let term: any, fitAddon: any, ws: WebSocket
    let cancelled = false
    ;(async () => {
      const { Terminal } = await import("xterm")
      const { FitAddon } = await import("xterm-addon-fit")
      if (cancelled || !termRef.current) return
      term = new Terminal({ cursorBlink: true, fontFamily: "var(--font-mono)", fontSize: 13, theme: { background: "#0a0c10", foreground: "#e0e0e0" } })
      fitAddon = new FitAddon()
      term.loadAddon(fitAddon)
      term.open(termRef.current!)
      fitAddon.fit()
      xtermRef.current = term
      ws = new WebSocket(shellUrl)
      wsRef.current = ws
      ws.binaryType = "arraybuffer"
      ws.onopen = () => { setConnected(true); term.writeln("\r\n[Connected to Odoo Shell]\r\n") }
      ws.onmessage = (ev) => {
        if (ev.data instanceof ArrayBuffer) term.write(new TextDecoder().decode(ev.data))
        else term.write(ev.data)
      }
      ws.onclose = () => { setConnected(false); term.writeln("\r\n[Disconnected]\r\n") }
      ws.onerror = () => { setConnected(false); term.writeln("\r\n[Connection error]\r\n") }
      term.onData((data: string) => { if (ws.readyState === WebSocket.OPEN) ws.send(data) })
      const onResize = () => { fitAddon.fit(); if (ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify({ type: "resize", rows: term.rows, cols: term.cols })) }
      window.addEventListener("resize", onResize)
      term.attachCustomKeyEventHandler((ev: KeyboardEvent) => { if (ev.key === "c" && ev.ctrlKey) { ws.send("\x03"); return false } return true })
      return () => window.removeEventListener("resize", onResize)
    })()
    return () => {
      cancelled = true
      try { wsRef.current?.close() } catch {}
      try { xtermRef.current?.dispose() } catch {}
      setConnected(false)
    }
  }, [shellUrl])

  return (
    <div className="screen">
      <div className="card">
        <div className="card-header">
          <h2>Terminal — {name}</h2>
          <div className="action-row">
            <select className="field-input" style={{ minWidth: 160 }} value={dbName} onChange={e => setDbName(e.target.value)}>
              <option value="">Select DB</option>
              {databases.map(d => <option key={d.name} value={d.name}>{d.name}{d.primary?" (primary)":""}</option>)}
            </select>
            <Button variant="primary" onClick={connect} disabled={!dbName || connected}>Connect</Button>
            <Button variant="outline" onClick={() => { wsRef.current?.close(); setShellUrl("") }} disabled={!connected}>Disconnect</Button>
            <span className={`pill ${connected ? "running" : "stopped"}`}>{connected ? "connected" : "disconnected"}</span>
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
            </div>
          </div>
        )}
        {!dbName && !dbError && <div className="empty">Select a database to start Odoo shell. Works when instance is stopped or running (shell uses `odoo shell -d`).</div>}
        <div ref={termRef} className="log-container" style={{ height: 480, background: "#0a0c10", padding: 0, overflow: "hidden" }} />
        <div className="p-3 text-xs text-muted-foreground border-t">Tip: <kbd>Ctrl+C</kbd> interrupt, `env['res.partner'].search([])` to test.</div>
      </div>
    </div>
  )
}