import React, { useEffect, useState, useCallback, useRef, useDeferredValue, createContext, useContext } from "react";
import { Events } from "@wailsio/runtime";
import { ErrorBoundary } from "./ErrorBoundary";
import { ConfirmModal } from "./ConfirmModal";
import { SkeletonLines, SkeletonTable } from "./Skeleton";
import { ModulesScreen, AdoptScreen, DoctorScreen } from "./Screens";
import {
  Instances,
  Status,
  Start,
  Stop,
  Restart,
  Update,
  Databases,
  Backup,
  Restore,
  DropDB,
  InitDB,
  LogsSince,
  Create,
  SwitchDB,
  ReadConf,
  SetConf,
  Remove,
} from "../bindings/github.com/ahmed/odoonoir/gui/app";
import type {
  InstanceView,
  StatusView,
  DatabaseView,
} from "../bindings/github.com/ahmed/odoonoir/internal/service/models";
import type {
  ConfEntry,
} from "../bindings/github.com/ahmed/odoonoir/gui/models";

type Screen = "instances" | "databases" | "logs" | "update" | "create" | "settings" | "adopt" | "modules" | "doctor";

/* ── Toast System ───────────────────────────────────────────────────── */

interface Toast {
  id: number;
  message: string;
  kind: "success" | "error" | "info";
}

const ToastCtx = createContext<{
  toast: (message: string, kind?: Toast["kind"]) => void;
}>({ toast: () => { throw new Error("useToast() must be used within ToastProvider"); } });

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const toastIdRef = useRef(0);

  const toast = useCallback((message: string, kind: Toast["kind"] = "info") => {
    const id = ++toastIdRef.current;
    setToasts((prev) => [...prev.slice(-6), { id, message, kind }]);
    setTimeout(() => {
      setToasts((prev) => prev.filter((t) => t.id !== id));
    }, 4000);
  }, []);

  return (
    <ToastCtx.Provider value={{ toast }}>
      {children}
      <div className="toast-container" role="status" aria-live="polite">
        {toasts.map((t) => (
          <div key={t.id} className={`toast toast-${t.kind}`}>
            <span>{t.message}</span>
            <button
              className="toast-close"
              onClick={() => setToasts((prev) => prev.filter((x) => x.id !== t.id))}
            >
              ×
            </button>
          </div>
        ))}
      </div>
    </ToastCtx.Provider>
  );
}

export function useToast() {
  return useContext(ToastCtx);
}

/* ── useAction hook ─────────────────────────────────────────────────── */

function useAction(refresh: () => Promise<void>, refreshOne?: (n: string) => Promise<void>) {
  const { toast } = useToast();
  const [busy, setBusy] = useState<string | null>(null);

  const run = useCallback(
    async (name: string, label: string, fn: () => Promise<void>, opts?: { noRefresh?: boolean }) => {
      setBusy(name);
      try {
        await fn();
        toast(`${name}: ${label} OK`, "success");
        if (!opts?.noRefresh) {
          if (refreshOne) await refreshOne(name);
          else await refresh();
        }
      } catch (e) {
        toast(String(e), "error");
      } finally {
        setBusy(null);
      }
    },
    [refresh, refreshOne, toast]
  );

  const act = useCallback(
    async (name: string, op: "start" | "stop" | "restart") => {
      await run(name, op, () => {
        if (op === "start") return Start(name);
        if (op === "stop") return Stop(name);
        return Restart(name);
      });
    },
    [run]
  );

  return { busy, run, act };
}

/* ── Helpers ─────────────────────────────────────────────────────────── */

function statusPill(running: boolean): string {
  return running ? "pill running" : "pill stopped";
}

function formatBytes(b: number): string {
  if (b === 0) return "—";
  const units = ["B", "KB", "MB", "GB"];
  let i = 0;
  let v = b;
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
  return v.toFixed(i === 0 ? 0 : 1) + " " + units[i];
}

/* ── Event kind constants ───────────────────────────────────────────── */

const EVT_STATUS = 0;
const EVT_STEP_START = 1;
const EVT_STEP_DONE = 2;
const EVT_STEP_FAIL = 3;

/* ── Sidebar Instance Item (memoized) ────────────────────────────────── */

const SidebarItem = React.memo(function SidebarItem({
  inst,
  selected,
  running,
  isBusy,
  onSelect,
  onAct,
}: {
  inst: InstanceView;
  selected: boolean;
  running: boolean;
  isBusy: boolean;
  onSelect: (name: string) => void;
  onAct: (name: string, op: "start" | "stop" | "restart") => void;
}) {
  return (
    <button
      className={`sidebar-item ${selected ? "active" : ""}`}
      onClick={() => onSelect(inst.name)}
      aria-label={`${inst.name}, ${running ? "running" : "stopped"}, v${inst.version}`}
      aria-current={selected ? "true" : undefined}
    >
      <span className={statusPill(running)}>
        <span className="pill-dot" />
      </span>
      <span className="sidebar-item-name">{inst.name}</span>
      <span className="sidebar-item-version">v{inst.version}</span>
      <div className="sidebar-quick-actions">
        {!running ? (
          <button
            className="qa-btn qa-start"
            disabled={isBusy}
            title="Start"
            aria-label={`Start ${inst.name}`}
            onClick={(ev) => { ev.stopPropagation(); onAct(inst.name, "start"); }}
          >
            ▶
          </button>
        ) : (
          <button
            className="qa-btn qa-stop"
            disabled={isBusy}
            title="Stop"
            aria-label={`Stop ${inst.name}`}
            onClick={(ev) => { ev.stopPropagation(); onAct(inst.name, "stop"); }}
          >
            ■
          </button>
        )}
        {running && (
          <button
            className="qa-btn qa-restart"
            disabled={isBusy}
            title="Restart"
            aria-label={`Restart ${inst.name}`}
            onClick={(ev) => { ev.stopPropagation(); onAct(inst.name, "restart"); }}
          >
            ↻
          </button>
        )}
      </div>
    </button>
  );
});

/* ═══════════════════════════════════════════════════════════════════════
   App
   ═══════════════════════════════════════════════════════════════════════ */

export default function App() {
  const { toast } = useToast();
  const [instances, setInstances] = useState<InstanceView[]>([]);
  const [statuses, setStatuses] = useState<Record<string, StatusView>>({});
  const [selected, setSelected] = useState<string | null>(null);
  const [screen, setScreen] = useState<Screen>("instances");
  const [eventLog, setEventLog] = useState<{ msg: string; ts: number; kind?: string }[]>([]);
  const eventLogRef = useRef<{ msg: string; ts: number; kind?: string }[]>([]);
  const eventLogTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [initialLoading, setInitialLoading] = useState(true);
  const [confirm, setConfirm] = useState<{ title: string; message: string; danger?: boolean; onConfirm: () => void } | null>(null);
  const [sidebarOpen, setSidebarOpen] = useState(true);
  const [eventLogOpen, setEventLogOpen] = useState(true);

  /* Refresh full instance list + all statuses in parallel */
  const refresh = useCallback(async () => {
    try {
      const list = await Instances();
      setInstances(list);
      const results = await Promise.allSettled(list.map((inst) => Status(inst.name)));
      const map: Record<string, StatusView> = {};
      list.forEach((inst, i) => {
        const r = results[i];
        if (r.status === "fulfilled") map[inst.name] = r.value;
      });
      setStatuses(map);
    } catch (e) {
      toast(String(e), "error");
    } finally {
      setInitialLoading(false);
    }
  }, [toast]);

  /* Refresh only one instance status */
  const refreshOne = useCallback(async (name: string) => {
    try {
      const st = await Status(name);
      setStatuses((prev) => ({ ...prev, [name]: st }));
    } catch {
      setStatuses((prev) => {
        const next = { ...prev };
        delete next[name];
        return next;
      });
    }
  }, []);

  const { busy, run, act } = useAction(refresh, refreshOne);

  /* Boot + event listeners */
  useEffect(() => {
    refresh();
    const t = setInterval(refresh, 30000);

    const off = Events.On("odoonoir-event", (raw: any) => {
      const e = raw?.data ?? raw;
      const ts = Date.now();

      if (e.message) {
        const kind = e.kind === EVT_STEP_FAIL ? "error" : e.kind === EVT_STEP_DONE ? "success" : undefined;
        eventLogRef.current = [...eventLogRef.current.slice(-500), { msg: e.message, ts, kind }];
        if (!eventLogTimer.current) {
          eventLogTimer.current = setTimeout(() => {
            eventLogTimer.current = null;
            setEventLog([...eventLogRef.current]);
          }, 250);
        }
      }

      if (e.kind === EVT_STATUS && e.instance) {
        refreshOne(e.instance);
      }
      if (e.kind === EVT_STEP_START || e.kind === EVT_STEP_DONE || e.kind === EVT_STEP_FAIL) {
        refreshOne(e.instance ?? "");
      }
    });

    return () => { clearInterval(t); off(); if (eventLogTimer.current) clearTimeout(eventLogTimer.current); };
  }, [refresh, refreshOne]);

  /* ── Action helpers using useAction ─────────────────────────────── */

  const handleBackup = useCallback(
    async (instName: string, dbName: string) => {
      await run(instName, `Backup ${dbName}`, () => Backup(instName, dbName, ""));
    },
    [run]
  );

  const handleRestore = useCallback(
    async (instName: string, dbName: string, dump: string, force: boolean) => {
      await run(instName, `Restore ${dbName}`, () => Restore(instName, dbName, dump, force));
    },
    [run]
  );

  const handleDropDB = useCallback(
    async (instName: string, dbName: string) => {
      await run(instName, `Drop ${dbName}`, () => DropDB(instName, dbName));
    },
    [run]
  );

  const handleInitDB = useCallback(
    async (instName: string, dbName: string) => {
      await run(instName, `Init ${dbName}`, () => InitDB(instName, dbName));
    },
    [run]
  );

  const handleUpdate = useCallback(
    async (instName: string, install: string[], update: string[]) => {
      await run(instName, "Update", () => Update(instName, install, update));
    },
    [run]
  );

  const handleRemove = useCallback(
    async (instName: string, keepData: boolean) => {
      await run(instName, "Remove", async () => {
        await Remove(instName, keepData);
        setSelected(null);
      });
    },
    [run]
  );

  /* ── Keyboard shortcuts ─────────────────────────────────────────── */

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const tag = (e.target as HTMLElement).tagName;
      if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") return;

      const key = e.key;

      if (key === "Escape") {
        setSelected(null);
        setScreen("instances");
      } else if (key === "r" && selected && !e.metaKey && !e.ctrlKey) {
        e.preventDefault();
        act(selected, "restart");
      } else if (key === "s" && selected && !e.metaKey && !e.ctrlKey) {
        e.preventDefault();
        const running = statuses[selected]?.Instance?.status === "running";
        act(selected, running ? "stop" : "start");
      } else if (key >= "1" && key <= "9" && !e.metaKey && !e.ctrlKey) {
        const screens: Screen[] = ["instances", "databases", "logs", "update", "settings", "modules", "create", "adopt", "doctor"];
        const idx = parseInt(key) - 1;
        if (idx < screens.length) {
          setScreen(screens[idx]);
          if (!selected && instances.length > 0) setSelected(instances[0].name);
        }
      } else if (key === "c" && !e.metaKey && !e.ctrlKey) {
        setScreen("create");
        setSelected(null);
      } else if (key === "?") {
        setScreen("instances");
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [selected, statuses, act, instances]);

  const selectedInst = selected ? statuses[selected] : null;

  return (
    <ErrorBoundary>
      <div className="shell">
        <header className="topbar">
          <h1>Odoonoir</h1>
          <span className="topbar-sub">
            {instances.length} instance{instances.length !== 1 ? "s" : ""}
          </span>
          <span className="topbar-hints">
            <kbd>1</kbd>-<kbd>9</kbd> screens <kbd>r</kbd> restart <kbd>s</kbd> start/stop <kbd>c</kbd> create <kbd>Esc</kbd> back
          </span>
        </header>

        <div className="layout">
          {/* Sidebar */}
          <aside className={`sidebar ${sidebarOpen ? "" : "collapsed"}`} role="navigation" aria-label="Instance navigation">
            <button
              className="sidebar-toggle"
              onClick={() => setSidebarOpen(!sidebarOpen)}
              aria-label={sidebarOpen ? "Collapse sidebar" : "Expand sidebar"}
            >
              {sidebarOpen ? "◀" : "▶"}
            </button>
            <div className="sidebar-section">
              <div className="sidebar-heading">Instances</div>
              {initialLoading ? (
                <SkeletonLines count={3} widths={["w80", "w60", "w80"]} />
              ) : instances.map((inst) => (
                <SidebarItem
                  key={inst.name}
                  inst={inst}
                  selected={selected === inst.name}
                  running={statuses[inst.name]?.Instance?.status === "running"}
                  isBusy={busy === inst.name}
                  onSelect={(n) => { setSelected(n); setScreen("instances"); }}
                  onAct={act}
                />
              ))}
              {!initialLoading && instances.length === 0 && (
                <div className="sidebar-empty">No instances yet</div>
              )}
            </div>

            {/* Screens — always visible */}
            <div className="sidebar-section">
              <div className="sidebar-heading">Screens</div>
              {instances.length === 0 ? (
                <>
                  <button
                    className={`sidebar-item ${screen === "create" ? "active" : ""}`}
                    onClick={() => setScreen("create")}
                  >
                    Create…
                  </button>
                  <button
                    className={`sidebar-item ${screen === "adopt" ? "active" : ""}`}
                    onClick={() => setScreen("adopt")}
                  >
                    Adopt…
                  </button>
                </>
              ) : (
                (["instances", "databases", "modules", "logs", "update", "settings", "create", "adopt", "doctor"] as Screen[]).map(
                  (s) => (
                    <button
                      key={s}
                      className={`sidebar-item ${screen === s ? "active" : ""}`}
                      onClick={() => setScreen(s)}
                      aria-current={screen === s ? "page" : undefined}
                    >
                      {s === "instances" && "Info"}
                      {s === "databases" && "Databases"}
                      {s === "modules" && "Modules"}
                      {s === "logs" && "Logs"}
                      {s === "update" && "Update"}
                      {s === "settings" && "Config"}
                      {s === "create" && "Create…"}
                      {s === "adopt" && "Adopt…"}
                      {s === "doctor" && "Doctor"}
                    </button>
                  )
                )
              )}
            </div>
          </aside>

          {/* Main content */}
          <main className="main" role="main" aria-label="Instance details">
            {!selected && screen !== "create" && screen !== "adopt" && (
              <div className="empty">
                <div className="empty-icon">⚡</div>
                <div className="empty-title">Odoonoir</div>
                <div className="empty-desc">
                  {instances.length === 0
                    ? "Create your first Odoo instance or adopt an existing installation."
                    : "Select an instance from the sidebar to get started."
                  }
                </div>
                <div className="action-row" style={{ justifyContent: "center" }}>
                  <button className="btn primary" onClick={() => setScreen("create")}>
                    Create Instance
                  </button>
                  <button className="btn" onClick={() => setScreen("adopt")}>
                    Adopt Existing
                  </button>
                </div>
              </div>
            )}

            {selected && screen === "instances" && selectedInst && (
              <InstanceDetail
                name={selected}
                status={selectedInst}
                busy={busy}
                onAct={act}
                onBackup={handleBackup}
                onNavigate={setScreen}
                onRemove={handleRemove}
                onConfirm={setConfirm}
              />
            )}

            {selected && screen === "databases" && (
              <DatabasesScreen
                name={selected}
                status={selectedInst}
                busy={busy}
                onBackup={handleBackup}
                onRestore={handleRestore}
                onDrop={handleDropDB}
                onInit={handleInitDB}
                onSwitch={async (n, db) => {
                  await run(n, `Switch to ${db}`, () => SwitchDB(n, db));
                }}
                onConfirm={setConfirm}
              />
            )}

            {selected && screen === "logs" && (
              <LogsScreen name={selected} />
            )}

            {selected && screen === "update" && (
              <UpdateScreen
                name={selected}
                busy={busy}
                onUpdate={handleUpdate}
              />
            )}

            {screen === "create" && (
              <CreateScreen
                onCreated={async () => {
                  await refresh();
                  setScreen("instances");
                }}
              />
            )}

            {screen === "adopt" && (
              <AdoptScreen
                onAdopted={async () => {
                  await refresh();
                  setScreen("instances");
                }}
              />
            )}

            {selected && screen === "settings" && (
              <SettingsScreen name={selected} />
            )}

            {selected && screen === "modules" && (
              <ModulesScreen name={selected} toast={toast} />
            )}

            {selected && screen === "doctor" && (
              <DoctorScreen name={selected} />
            )}
          </main>

          {/* Event log strip */}
          <div className={`event-log ${eventLogOpen ? "" : "collapsed"}`} role="log" aria-label="Event log">
            <div className="event-log-heading">
              Events
              <div className="action-row">
                <button
                  className="btn tiny"
                  onClick={() => setEventLogOpen(!eventLogOpen)}
                  aria-label={eventLogOpen ? "Collapse event log" : "Expand event log"}
                >
                  {eventLogOpen ? "◀" : "▶"}
                </button>
                <button
                  className="btn tiny"
                  onClick={() => setEventLog([])}
                  title="Clear"
                  aria-label="Clear events"
                >
                  Clear
                </button>
              </div>
            </div>
            <div className="event-log-lines">
              {eventLog.length === 0 && (
                <div className="event-log-empty">No events yet</div>
              )}
              {eventLog.slice(-80).map((entry, i) => (
                <div key={`${entry.ts}-${i}`} className={`event-log-line ${entry.kind === "error" ? "event-error" : entry.kind === "success" ? "event-success" : ""}`}>
                  <span className="event-log-time">
                    {new Date(entry.ts).toLocaleTimeString()}
                  </span>
                  {entry.msg}
                </div>
              ))}
            </div>
          </div>
        </div>

        {/* Global confirm modal */}
        <ConfirmModal
          open={!!confirm}
          title={confirm?.title ?? ""}
          message={confirm?.message ?? ""}
          danger={confirm?.danger}
          confirmLabel={confirm?.danger ? "Remove" : "Confirm"}
          onConfirm={() => { confirm?.onConfirm(); setConfirm(null); }}
          onCancel={() => setConfirm(null)}
        />
      </div>
    </ErrorBoundary>
  );
}

/* ═══════════════════════════════════════════════════════════════════════
   Instance Detail
   ═══════════════════════════════════════════════════════════════════════ */

function InstanceDetail({
  name,
  status,
  busy,
  onAct,
  onBackup,
  onNavigate,
  onRemove,
  onConfirm,
}: {
  name: string;
  status: StatusView;
  busy: string | null;
  onAct: (n: string, op: "start" | "stop" | "restart") => void;
  onBackup: (n: string, db: string) => void;
  onNavigate: (s: Screen) => void;
  onRemove: (n: string, keepData: boolean) => void;
  onConfirm: (c: { title: string; message: string; danger?: boolean; onConfirm: () => void }) => void;
}) {
  const inst = status.Instance;
  const running = status.Instance?.status === "running";
  const [dbList, setDbList] = useState<string[]>([]);

  useEffect(() => {
    Databases(name).then((list) => setDbList(list.map((d) => d.name))).catch(() => {});
  }, [name]);

  return (
    <div className="screen">
      <div className="card">
        <div className="card-header">
          <h2>{inst.name}</h2>
          <span className={statusPill(running)}>
            <span className="pill-dot" />
            {running ? "running" : "stopped"}
            {inst.pid > 0 ? ` (pid ${inst.pid})` : ""}
          </span>
        </div>
        <div className="card-grid">
          <div className="field">
            <span className="field-label">Version</span>
            <span className="field-value">{inst.version}</span>
          </div>
          <div className="field">
            <span className="field-label">Port</span>
            <span className="field-value">{inst.port}</span>
          </div>
          <div className="field">
            <span className="field-label">Database</span>
            <span className="field-value mono">{inst.dbName}</span>
          </div>
          <div className="field">
            <span className="field-label">Databases</span>
            <span className="field-value">{inst.dbs}</span>
          </div>
          <div className="field">
            <span className="field-label">Path</span>
            <span className="field-value mono">{inst.path}</span>
          </div>
          <div className="field">
            <span className="field-label">Serving</span>
            <span className="field-value mono">{status.Serving || "—"}</span>
          </div>
          {dbList.length > 1 && (
            <div className="field">
              <span className="field-label">Switch DB</span>
              <select
                className="field-input"
                value={status.Serving ?? ""}
                disabled={busy === name || !running}
                onChange={(e) => {
                  const db = e.target.value;
                  if (db && db !== status.Serving) {
                    onConfirm({
                      title: `Switch to "${db}"?`,
                      message: "This will restart the instance to serve this database.",
                      onConfirm: () => {
                        SwitchDB(name, db).catch(() => {});
                      },
                    });
                  }
                }}
              >
                <option value="">—</option>
                {dbList.map((d) => <option key={d} value={d}>{d}</option>)}
              </select>
            </div>
          )}
          {inst.adopted && (
            <div className="field">
              <span className="field-label">Adopted</span>
              <span className="field-value">yes</span>
            </div>
          )}
        </div>
      </div>

      <div className="card">
        <div className="card-header">
          <h3>Actions</h3>
        </div>
        <div className="action-grid">
          <div className="action-card">
            <div className="action-card-title">Process</div>
            <div className="action-row">
              {running ? (
                <>
                  <button
                    className="btn primary"
                    disabled={busy === name}
                    onClick={() => onAct(name, "restart")}
                  >
                    ↻ Restart
                  </button>
                  <button
                    className="btn danger"
                    disabled={busy === name}
                    onClick={() => onAct(name, "stop")}
                  >
                    ■ Stop
                  </button>
                </>
              ) : (
                <button
                  className="btn primary"
                  disabled={busy === name}
                  onClick={() => onAct(name, "start")}
                >
                  ▶ Start
                </button>
              )}
            </div>
          </div>

          <div className="action-card">
            <div className="action-card-title">Database</div>
            <div className="action-row">
              <button
                className="btn"
                disabled={busy === name}
                onClick={() => onBackup(name, inst.dbName)}
              >
                Backup
              </button>
              <button className="btn" onClick={() => onNavigate("databases")}>
                Manage →
              </button>
            </div>
          </div>

          <div className="action-card">
            <div className="action-card-title">Operations</div>
            <div className="action-row">
              <button className="btn" onClick={() => onNavigate("logs")}>View Logs</button>
              <button className="btn" onClick={() => onNavigate("settings")}>Edit Config</button>
              <button className="btn" onClick={() => onNavigate("update")}>Update</button>
              <button className="btn" onClick={() => onNavigate("modules")}>Modules</button>
              <button className="btn" onClick={() => onNavigate("doctor")}>Diagnose</button>
            </div>
          </div>

          <div className="action-card">
            <div className="action-card-title">Danger Zone</div>
            <div className="action-row">
              <button className="btn" onClick={() => onNavigate("adopt")}>Adopt Existing</button>
              <button
                className="btn danger"
                disabled={busy === name}
                onClick={() => onConfirm({
                  title: `Remove "${name}"?`,
                  message: "This deletes all files and the primary database. This cannot be undone.",
                  danger: true,
                  onConfirm: () => onRemove(name, false),
                })}
              >
                Remove Instance
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

/* ═══════════════════════════════════════════════════════════════════════
   Databases Screen
   ═══════════════════════════════════════════════════════════════════════ */

function DatabasesScreen({
  name,
  status,
  busy,
  onBackup,
  onRestore,
  onDrop,
  onInit,
  onSwitch,
  onConfirm,
}: {
  name: string;
  status: StatusView | null;
  busy: string | null;
  onBackup: (n: string, db: string) => void;
  onRestore: (n: string, db: string, dump: string, force: boolean) => void;
  onDrop: (n: string, db: string) => void;
  onInit: (n: string, db: string) => void;
  onSwitch: (n: string, db: string) => void;
  onConfirm: (c: { title: string; message: string; danger?: boolean; onConfirm: () => void }) => void;
}) {
  const [dbs, setDbs] = useState<DatabaseView[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [restoreDump, setRestoreDump] = useState("");
  const [restoreTarget, setRestoreTarget] = useState("");
  const [restoreForce, setRestoreForce] = useState(false);
  const [showRestore, setShowRestore] = useState(false);

  const loadDbs = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const list = await Databases(name);
      setDbs(list);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }, [name]);

  useEffect(() => { loadDbs(); }, [loadDbs]);

  return (
    <div className="screen">
      <div className="card">
        <div className="card-header">
          <h2>Databases</h2>
          <div className="action-row">
            <button className="btn" onClick={loadDbs} disabled={loading}>
              Refresh
            </button>
            <button className="btn" onClick={() => setShowRestore(!showRestore)}>
              Restore…
            </button>
          </div>
        </div>

        {error && (
          <div className="error">
            <span>{error}</span>
            <button className="error-close" onClick={() => setError(null)}>×</button>
          </div>
        )}

        {showRestore && (
          <div className="restore-form">
            <input
              className="field-input"
              placeholder="dump file path"
              value={restoreDump}
              onChange={(e) => setRestoreDump(e.target.value)}
            />
            <input
              className="field-input"
              placeholder="target db name"
              value={restoreTarget}
              onChange={(e) => setRestoreTarget(e.target.value)}
            />
            <label className="checkbox-label">
              <input
                type="checkbox"
                checked={restoreForce}
                onChange={(e) => setRestoreForce(e.target.checked)}
              />
              Force (drop existing)
            </label>
            <button
              className="btn primary"
              disabled={!restoreDump || !restoreTarget.trim() || busy === name}
              onClick={async () => {
                await onRestore(name, restoreTarget, restoreDump, restoreForce);
                setShowRestore(false);
                setRestoreDump("");
                setRestoreTarget("");
                setRestoreForce(false);
                loadDbs();
              }}
            >
              Restore
            </button>
          </div>
        )}

        {loading ? (
          <SkeletonTable rows={4} cols={5} />
        ) : (
          <table>
            <thead>
              <tr>
                <th>Name</th>
                <th>Size</th>
                <th>Owner</th>
                <th>Init</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {dbs.map((db) => (
                <tr key={db.name}>
                  <td>
                    <span className="mono">{db.name}</span>
                    {db.primary && <span className="pill primary">primary</span>}
                  </td>
                  <td>{formatBytes(db.sizeBytes)}</td>
                  <td>{db.owner || "—"}</td>
                  <td>{db.initialized ? "✓" : "—"}</td>
                  <td>
                    <div className="row-actions">
                      {!db.initialized && (
                        <button
                          className="btn small"
                          disabled={busy === name}
                          onClick={() => onInit(name, db.name)}
                        >
                          Init
                        </button>
                      )}
                      <button
                        className="btn small"
                        disabled={busy === name}
                        onClick={() => onBackup(name, db.name)}
                      >
                        Backup
                      </button>
                      {db.name !== (status?.Serving ?? "") && (
                        <button
                          className="btn small success"
                          disabled={busy === name}
                          onClick={() => onConfirm({
                            title: `Switch to "${db.name}"?`,
                            message: "This will restart the instance to serve this database.",
                            onConfirm: () => onSwitch(name, db.name),
                          })}
                        >
                          Serve
                        </button>
                      )}
                      {!db.primary && (
                        <button
                          className="btn small danger"
                          disabled={busy === name}
                          onClick={() => onConfirm({
                            title: `Drop "${db.name}"?`,
                            message: "This cannot be undone.",
                            danger: true,
                            onConfirm: () => { onDrop(name, db.name); loadDbs(); },
                          })}
                        >
                          Drop
                        </button>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
              {dbs.length === 0 && !loading && (
                <tr>
                  <td colSpan={5} className="empty">No databases</td>
                </tr>
              )}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}

/* ═══════════════════════════════════════════════════════════════════════
   Logs Screen
   ═══════════════════════════════════════════════════════════════════════ */

function LogsScreen({ name }: { name: string }) {
  const { toast } = useToast();
  const [lines, setLines] = useState<string[]>([]);
  const offsetRef = useRef(0);
  const [follow, setFollow] = useState(true);
  const [search, setSearch] = useState("");
  const deferredSearch = useDeferredValue(search);
  const endRef = useRef<HTMLDivElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);

  const filtered = React.useMemo(
    () => deferredSearch ? lines.filter((l) => l.toLowerCase().includes(deferredSearch.toLowerCase())) : lines,
    [lines, deferredSearch]
  );
  const matchCount = deferredSearch ? filtered.length : 0;

  useEffect(() => {
    offsetRef.current = 0;
    setLines([]);
  }, [name]);

  useEffect(() => {
    let active = true;
    const poll = async () => {
      if (!active) return;
      try {
        const [newLines, newOffset, rotated] = await LogsSince(name, 200, offsetRef.current);
        if (rotated) {
          toast("Log file was rotated", "info");
          setLines([]);
        }
        if (active && newLines.length > 0) {
          setLines((prev) => [...(rotated ? [] : prev), ...newLines].slice(-3000));
          offsetRef.current = newOffset;
        }
      } catch { /* skip */ }
    };
    poll();
    const t = setInterval(poll, 2000);
    return () => { active = false; clearInterval(t); };
  }, [name]);

  useEffect(() => {
    if (follow && endRef.current) {
      endRef.current.scrollIntoView({ behavior: "smooth" });
    }
  }, [lines, follow]);

  const handleScroll = useCallback(() => {
    if (!containerRef.current) return;
    const { scrollTop, scrollHeight, clientHeight } = containerRef.current;
    setFollow(scrollHeight - scrollTop - clientHeight < 50);
  }, []);

  return (
    <div className="screen">
      <div className="card">
        <div className="card-header">
          <h2>Logs</h2>
          <div className="action-row">
            <input
              className="search-input"
              placeholder="Filter logs…"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
            {search && <span className="search-count">{matchCount} matches</span>}
            <label className="checkbox-label">
              <input
                type="checkbox"
                checked={follow}
                onChange={(e) => setFollow(e.target.checked)}
              />
              Follow
            </label>
            <button className="btn" onClick={() => setLines([])}>Clear</button>
          </div>
        </div>
        <div
          ref={containerRef}
          className="log-container"
          onScroll={handleScroll}
        >
          {lines.length === 0 && (
            <div className="log-empty">Waiting for log lines…</div>
          )}
          {filtered.length === 0 && search && lines.length > 0 && (
            <div className="log-empty">No lines match &quot;{search}&quot;</div>
          )}
          {filtered.map((line, i) => (
            <div key={i} className={`log-line ${logLineClass(line)}`}>
              {deferredSearch ? highlightAll(line, deferredSearch) : line}
            </div>
          ))}
          <div ref={endRef} />
        </div>
      </div>
    </div>
  );
}

function logLineClass(line: string): string {
  const upper = line.toUpperCase();
  if (upper.includes("ERROR") || upper.includes("FATAL") || upper.includes("CRITICAL")) return "log-error";
  if (upper.includes("WARNING") || upper.includes("WARN")) return "log-warning";
  if (upper.includes("DEBUG")) return "log-debug";
  return "";
}

function highlightAll(line: string, query: string): React.ReactNode {
  if (!query) return line;
  const lower = line.toLowerCase();
  const qLower = query.toLowerCase();
  const parts: React.ReactNode[] = [];
  let lastIdx = 0;
  let idx = lower.indexOf(qLower, lastIdx);
  while (idx !== -1) {
    if (idx > lastIdx) parts.push(line.slice(lastIdx, idx));
    parts.push(
      <mark key={idx} className="search-hl">
        {line.slice(idx, idx + query.length)}
      </mark>
    );
    lastIdx = idx + query.length;
    idx = lower.indexOf(qLower, lastIdx);
  }
  if (lastIdx < line.length) parts.push(line.slice(lastIdx));
  return <>{parts}</>;
}

/* ═══════════════════════════════════════════════════════════════════════
   Update Screen
   ═══════════════════════════════════════════════════════════════════════ */

function UpdateScreen({
  name,
  busy,
  onUpdate,
}: {
  name: string;
  busy: string | null;
  onUpdate: (n: string, install: string[], update: string[]) => void;
}) {
  const [installMods, setInstallMods] = useState("");
  const [updateMods, setUpdateMods] = useState("");
  const [progress, setProgress] = useState<string[]>([]);

  useEffect(() => {
    const off = Events.On("odoonoir-event", (e: any) => {
      const data = e?.data ?? e;
      if (data.instance && data.instance !== name) return;
      if (data?.step || data?.message) {
        const msg = data.message || data.step;
        setProgress((prev) => [...prev.slice(-200), msg]);
      }
    });
    return () => off();
  }, [name]);

  return (
    <div className="screen">
      <div className="card">
        <div className="card-header">
          <h2>Update</h2>
        </div>
        <div className="update-form">
          <div className="field">
            <span className="field-label">Install modules (comma-separated)</span>
            <input
              className="field-input"
              placeholder="e.g. sale_stock,account"
              value={installMods}
              onChange={(e) => setInstallMods(e.target.value)}
            />
          </div>
          <div className="field">
            <span className="field-label">Update modules (comma-separated)</span>
            <input
              className="field-input"
              placeholder="e.g. sale,account"
              value={updateMods}
              onChange={(e) => setUpdateMods(e.target.value)}
            />
          </div>
          <button
            className="btn primary"
            disabled={busy === name}
            onClick={() => {
              setProgress([]);
              const install = installMods.split(",").map((s) => s.trim()).filter(Boolean);
              const update = updateMods.split(",").map((s) => s.trim()).filter(Boolean);
              onUpdate(name, install, update);
            }}
          >
            {busy === name ? (
              <><span className="spinner" /> Running…</>
            ) : (
              "Run Update"
            )}
          </button>
        </div>
      </div>

      {progress.length > 0 && (
        <div className="card">
          <div className="card-header">
            <h3>Progress</h3>
          </div>
          <div className="log-container">
            {progress.map((line, i) => (
              <div key={i} className="log-line">{line}</div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

/* ═══════════════════════════════════════════════════════════════════════
   Create Instance Screen
   ═══════════════════════════════════════════════════════════════════════ */

function CreateScreen({ onCreated }: { onCreated: () => void }) {
  const { toast } = useToast();
  const [name, setName] = useState("");
  const [version, setVersion] = useState("18");
  const [port, setPort] = useState("");
  const [dbUser, setDbUser] = useState("");
  const [dbPass, setDbPass] = useState("");
  const [dbName, setDbName] = useState("");
  const [creating, setCreating] = useState(false);
  const [steps, setSteps] = useState<{ name: string; status: "pending" | "running" | "done" | "fail" }[]>([]);
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;
    return () => { mountedRef.current = false; };
  }, []);

  const handleCreate = async () => {
    if (!name.trim() || !version.trim()) {
      toast("Name and version are required", "error");
      return;
    }
    setCreating(true);
    setSteps([]);

    /* Listen for step events — emit dynamically from backend events */
    const off = Events.On("odoonoir-event", (raw: any) => {
      const e = raw?.data ?? raw;
      if (!mountedRef.current) return;

      if (e.kind === EVT_STEP_START && e.step) {
        setSteps((prev) => {
          const exists = prev.some((s) => s.name === e.step);
          if (exists) {
            return prev.map((s) =>
              s.name === e.step ? { ...s, status: "running" as const } :
              s.status === "running" ? { ...s, status: "done" as const } : s
            );
          }
          return [...prev.map((s) =>
            s.status === "running" ? { ...s, status: "done" as const } : s
          ), { name: e.step, status: "running" as const }];
        });
      }
      if (e.kind === EVT_STEP_DONE && e.step) {
        setSteps((prev) =>
          prev.map((s) => s.name === e.step ? { ...s, status: "done" as const } : s)
        );
      }
      if (e.kind === EVT_STEP_FAIL) {
        setSteps((prev) =>
          prev.map((s) =>
            s.status === "running" || s.status === "pending" ? { ...s, status: "fail" as const } : s
          )
        );
      }
    });

    try {
      const res = await Create(name, version, port, dbUser, dbPass, dbName);
      toast(`Instance "${res.name}" created`, "success");
      setName(""); setPort(""); setDbUser(""); setDbPass(""); setDbName(""); setSteps([]);
      onCreated();
    } catch (e) {
      toast(String(e), "error");
      setSteps((prev) =>
        prev.map((s) =>
          s.status === "running" || s.status === "pending" ? { ...s, status: "fail" as const } : s
        )
      );
    } finally {
      off();
      setCreating(false);
    }
  };

  const stepIcon = (s: { status: string }) => {
    if (s.status === "done") return "✓";
    if (s.status === "running") return "●";
    if (s.status === "fail") return "✗";
    return "○";
  };

  return (
    <div className="screen">
      <div className="card">
        <div className="card-header">
          <h2>Create Instance</h2>
        </div>
        <div className="update-form">
          <div className="field">
            <span className="field-label">Name *</span>
            <input className="field-input" placeholder="e.g. myapp" value={name} onChange={(e) => setName(e.target.value)} disabled={creating} />
          </div>
          <div className="field">
            <span className="field-label">Version *</span>
            <select className="field-input" value={version} onChange={(e) => setVersion(e.target.value)} disabled={creating}>
              <option value="17">17.0</option>
              <option value="18">18.0</option>
              <option value="19">19.0</option>
            </select>
          </div>
          <div className="field">
            <span className="field-label">Port (auto if empty)</span>
            <input className="field-input" placeholder="e.g. 8069" value={port} onChange={(e) => setPort(e.target.value)} disabled={creating} />
          </div>
          <div className="field">
            <span className="field-label">DB User (default: odoo)</span>
            <input className="field-input" placeholder="odoo" value={dbUser} onChange={(e) => setDbUser(e.target.value)} disabled={creating} />
          </div>
          <div className="field">
            <span className="field-label">DB Password (default: same as user)</span>
            <input className="field-input" type="password" placeholder="odoo" value={dbPass} onChange={(e) => setDbPass(e.target.value)} disabled={creating} />
          </div>
          <div className="field">
            <span className="field-label">DB Name (default: same as instance)</span>
            <input className="field-input" placeholder="e.g. myapp_db" value={dbName} onChange={(e) => setDbName(e.target.value)} disabled={creating} />
          </div>
          <button
            className="btn primary"
            disabled={creating || !name.trim() || !version.trim()}
            onClick={handleCreate}
          >
            {creating ? <><span className="spinner" /> Creating…</> : "Create Instance"}
          </button>

          {steps.length > 0 && (
            <div className="create-steps">
              {steps.map((s, i) => (
                <div key={i} className={`create-step step-${s.status}`}>
                  <span className="step-icon">{stepIcon(s)}</span>
                  <span className="step-name">{s.name}</span>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

/* ═══════════════════════════════════════════════════════════════════════
   Settings / Config Screen
   ═══════════════════════════════════════════════════════════════════════ */

function SettingsScreen({ name }: { name: string }) {
  const { toast } = useToast();
  const [entries, setEntries] = useState<ConfEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [edits, setEdits] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState(false);

  const loadConf = useCallback(async () => {
    try {
      setLoading(true);
      const list = await ReadConf(name);
      setEntries(list);
    } catch (e) {
      toast(String(e), "error");
    } finally {
      setLoading(false);
    }
  }, [name, toast]);

  useEffect(() => { loadConf(); }, [loadConf]);

  const handleSave = async () => {
    setSaving(true);
    try {
      const keys = Object.entries(edits);
      const results = await Promise.allSettled(
        keys.map(([key, value]) => SetConf(name, key, value))
      );
      const succeeded = results.filter((r) => r.status === "fulfilled").length;
      const failed = results.filter((r) => r.status === "rejected").length;
      if (failed > 0) {
        toast(`Saved ${succeeded}/${keys.length} entries — ${failed} failed`, "error");
      } else {
        toast(`Saved ${succeeded} config entries`, "success");
      }
      setEdits({});
      await loadConf();
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="screen">
      <div className="card">
        <div className="card-header">
          <h2>Config — {name}</h2>
          <div className="action-row">
            {Object.keys(edits).length > 0 && (
              <span className="search-count">{Object.keys(edits).length} unsaved</span>
            )}
            <button className="btn" onClick={loadConf} disabled={loading}>Refresh</button>
            <button
              className="btn primary"
              disabled={saving || Object.keys(edits).length === 0}
              onClick={handleSave}
            >
              {saving ? <><span className="spinner" /> Saving…</> : "Save"}
            </button>
          </div>
        </div>

        {loading ? (
          <SkeletonLines count={6} widths={["w80", "w60", "w80", "w60", "w40", "w80"]} />
        ) : entries.length === 0 ? (
          <div className="empty">No config entries found.</div>
        ) : (
          <div className="conf-list">
            {entries.map((e) => (
              <div
                key={e.key}
                className={`conf-row ${edits[e.key] !== undefined ? "dirty" : ""}`}
              >
                <span className="conf-key mono">{e.key}</span>
                <input
                  className="conf-value mono"
                  value={edits[e.key] ?? e.value}
                  onChange={(ev) =>
                    setEdits((prev) => ({ ...prev, [e.key]: ev.target.value }))
                  }
                  title={e.active ? "Active" : "Commented out"}
                />
                {!e.active && <span className="pill commented">off</span>}
                {edits[e.key] !== undefined && (
                  <button
                    className="btn tiny"
                    title="Revert to original"
                    onClick={() =>
                      setEdits((prev) => {
                        const next = { ...prev };
                        delete next[e.key];
                        return next;
                      })
                    }
                  >
                    ↩
                  </button>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
