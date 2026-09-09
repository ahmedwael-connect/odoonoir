import { useEffect, useState, useCallback } from "react";
import {
  ModuleList,
  ModuleUninstall,
  Adopt,
  Doctor,
  Databases,
} from "../bindings/github.com/ahmed/odoonoir/gui/app";
import type {
  ModuleView,
  DoctorIssue,
  AdoptResult,
} from "../bindings/github.com/ahmed/odoonoir/gui/models";
import type {
  DatabaseView,
} from "../bindings/github.com/ahmed/odoonoir/internal/service/models";
import { useToast } from "./App";
import { ConfirmModal } from "./ConfirmModal";
import { Button } from "./components/atoms/Button";

/* ── Modules Screen ──────────────────────────────────────────────── */

export function ModulesScreen({
  name,
  toast,
}: {
  name: string;
  toast: (msg: string, kind?: "success" | "error" | "info", hint?: string) => void;
}) {
  const [modules, setModules] = useState<ModuleView[]>([]);
  const [databases, setDatabases] = useState<DatabaseView[]>([]);
  const [dbName, setDbName] = useState("");
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");
  const [customOnly, setCustomOnly] = useState(false);
  const [uninstalling, setUninstalling] = useState<string | null>(null);
  const [confirmTarget, setConfirmTarget] = useState<ModuleView | null>(null);

  const load = useCallback(async () => {
    try {
      setLoading(true);
      const list = await ModuleList(name, dbName);
      setModules(list);
    } catch (e) {
      toast(String(e), "error");
    } finally {
      setLoading(false);
    }
  }, [name, dbName, toast]);

  useEffect(() => {
    Databases(name).then(setDatabases).catch(() => {});
  }, [name]);

  useEffect(() => {
    load();
  }, [load]);

  let filtered = search
    ? modules.filter(
        (m) =>
          m.name.toLowerCase().includes(search.toLowerCase()) ||
          m.shortdesc.toLowerCase().includes(search.toLowerCase()) ||
          m.author.toLowerCase().includes(search.toLowerCase())
      )
    : modules;
  if (customOnly) filtered = filtered.filter((m) => (m as any).custom);

  const installed = modules.filter((m) => m.installed).length;

  const handleUninstall = async (m: ModuleView) => {
    setUninstalling(m.name);
    try {
      await ModuleUninstall(name, m.name, "");
      toast(`Module "${m.name}" uninstalled`, "success");
      await load();
    } catch (e) {
      toast(String(e), "error");
    } finally {
      setUninstalling(null);
      setConfirmTarget(null);
    }
  };

  return (
    <div className="screen">
      <div className="card">
        <div className="card-header">
          <h2>
            Modules
            <span className="screen-header-sub">
              {installed} installed / {modules.length} available
            </span>
          </h2>
          <div className="action-row">
            {databases.length > 1 && (
              <select
                className="field-input"
                style={{ minWidth: 120 }}
                value={dbName}
                onChange={(e) => setDbName(e.target.value)}
              >
                <option value="">Default DB</option>
                {databases.map((db) => (
                  <option key={db.name} value={db.name}>{db.name}</option>
                ))}
              </select>
            )}
            <input
              className="search-input"
              placeholder="Filter modules…"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
            <label className="flex items-center gap-1 text-xs cursor-pointer"><input type="checkbox" checked={customOnly} onChange={e=>setCustomOnly(e.target.checked)} /> Custom only</label>
            {search && (
              <span className="search-count">
                {filtered.length} match{filtered.length !== 1 ? "es" : ""}
              </span>
            )}
            <Button variant="outline" size="sm" onClick={load} disabled={loading} loading={loading}>
              Refresh
            </Button>
          </div>
        </div>

        {loading ? (
          <div className="empty">Loading modules…</div>
        ) : filtered.length === 0 ? (
          <div className="empty">
            {search ? `No modules match "${search}"` : "No modules found"}
          </div>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Module</th>
                <th>State</th>
                <th>Description</th>
                <th>Author</th>
                <th>Version</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {filtered.map((m) => (
                <tr key={m.name}>
                  <td>
                    <span className="mono">{m.name}</span>
                  </td>
                  <td>
                    <span
                      className={`pill ${m.installed ? "running" : "stopped"}`}
                    >
                      {m.state || (m.installed ? "installed" : "uninstalled")}
                    </span>
                  </td>
                  <td>{m.shortdesc || "—"}</td>
                  <td>{m.author || "—"}</td>
                  <td>{m.version || "—"}</td>
                  <td>
                    {m.installed && (
                      <Button
                        variant="destructive"
                        size="sm"
                        loading={uninstalling === m.name}
                        onClick={() => setConfirmTarget(m)}
                      >
                        Uninstall
                      </Button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <ConfirmModal
        open={!!confirmTarget}
        title={`Uninstall "${confirmTarget?.name}"?`}
        message={`This will uninstall the module "${confirmTarget?.name}" from the database. The module files will remain on disk.`}
        confirmLabel="Uninstall"
        danger
        onConfirm={() => confirmTarget && handleUninstall(confirmTarget)}
        onCancel={() => setConfirmTarget(null)}
      />
    </div>
  );
}

/* ── Adopt Screen ────────────────────────────────────────────────── */

export function AdoptScreen({ onAdopted }: { onAdopted: () => void }) {
  const { toast } = useToast();

  const [name, setName] = useState("");
  const [source, setSource] = useState("");
  const [conf, setConf] = useState("");
  const [port, setPort] = useState("");
  const [dbName, setDbName] = useState("");
  const [dbUser, setDbUser] = useState("");
  const [version, setVersion] = useState("18");

  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<AdoptResult | null>(null);

  const handleAdopt = async () => {
    if (!name.trim()) {
      toast("Name is required", "error");
      return;
    }
    setBusy(true);
    setResult(null);
    try {
      const res = await Adopt(name, source, conf, port, dbName, dbUser, version);
      setResult(res);
      toast(`Instance "${res.name}" adopted`, "success");
      onAdopted();
    } catch (e) {
      toast(String(e), "error");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="screen">
      <div className="card">
        <div className="card-header">
          <h2>Adopt Existing Instance</h2>
        </div>
        <div className="update-form">
          <div className="field">
            <span className="field-label">Name *</span>
            <input
              className="field-input"
              placeholder="e.g. myapp"
              value={name}
              onChange={(e) => setName(e.target.value)}
              disabled={busy}
            />
          </div>
          <div className="field">
            <span className="field-label">Source directory</span>
            <input
              className="field-input"
              placeholder="/path/to/odoo-source"
              value={source}
              onChange={(e) => setSource(e.target.value)}
              disabled={busy}
            />
          </div>
          <div className="field">
            <span className="field-label">Config path</span>
            <input
              className="field-input"
              placeholder="/path/to/odoo.conf"
              value={conf}
              onChange={(e) => setConf(e.target.value)}
              disabled={busy}
            />
          </div>
          <div className="field">
            <span className="field-label">Port</span>
            <input
              className="field-input"
              placeholder="e.g. 8069"
              value={port}
              onChange={(e) => setPort(e.target.value)}
              disabled={busy}
            />
          </div>
          <div className="field">
            <span className="field-label">Database</span>
            <input
              className="field-input"
              placeholder="e.g. myapp_db"
              value={dbName}
              onChange={(e) => setDbName(e.target.value)}
              disabled={busy}
            />
          </div>
          <div className="field">
            <span className="field-label">DB User</span>
            <input
              className="field-input"
              placeholder="odoo"
              value={dbUser}
              onChange={(e) => setDbUser(e.target.value)}
              disabled={busy}
            />
          </div>
          <div className="field">
            <span className="field-label">Version</span>
            <select
              className="field-input"
              value={version}
              onChange={(e) => setVersion(e.target.value)}
              disabled={busy}
            >
              <option value="17">17.0</option>
              <option value="18">18.0</option>
              <option value="19">19.0</option>
            </select>
          </div>
          <Button
            variant="primary"
            loading={busy}
            disabled={busy || !name.trim()}
            onClick={handleAdopt}
          >
            Adopt Instance
          </Button>

          {result && (
            <div className="success">
              Adopted "{result.name}" — port {result.port}, database "{result.dbName}"
            </div>
          )}

          {result && result.warnings.length > 0 && (
            <div className="warning-banner-flex">
              <strong>Warnings:</strong>
              {result.warnings.map((w, i) => (
                <div key={i}>⚠ {w}</div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

/* ── Doctor Screen ───────────────────────────────────────────────── */

export function DoctorScreen({ name }: { name: string }) {
  const [issues, setIssues] = useState<DoctorIssue[]>([]);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    try {
      setLoading(true);
      const list = await Doctor(name);
      setIssues(list);
    } catch {
      /* skip */
    } finally {
      setLoading(false);
    }
  }, [name]);

  useEffect(() => {
    load();
  }, [load]);

  const errors = issues.filter((i) => i.severity === "error").length;
  const warnings = issues.filter((i) => i.severity === "warning").length;

  return (
    <div className="screen">
      <div className="card">
        <div className="card-header">
          <h2>
            Doctor
            {!loading && issues.length > 0 && (
              <span className="screen-header-sub">
                {errors > 0 && `${errors} error${errors !== 1 ? "s" : ""}`}
                {errors > 0 && warnings > 0 && ", "}
                {warnings > 0 && `${warnings} warning${warnings !== 1 ? "s" : ""}`}
              </span>
            )}
          </h2>
          <Button variant="outline" size="sm" onClick={load} disabled={loading} loading={loading}>
            Refresh
          </Button>
        </div>

        {loading ? (
          <div className="empty">Running diagnostics…</div>
        ) : issues.length === 0 ? (
          <div className="empty">No issues found — the log looks clean</div>
        ) : (
          <div className="doctor-list-flex">
            {issues.map((issue, i) => (
              <div key={i} className={`doctor-issue ${issue.severity === "error" ? "issue-error" : "issue-warning"}`}>
                <div className="doctor-header">
                  <span className="doctor-icon" style={{ color: issue.severity === "error" ? "#fca5a5" : "#fbbf24" }}>
                    {issue.severity === "error" ? "✗" : "⚠"}
                  </span>
                  <span className="doctor-message">{issue.message}</span>
                </div>
                {issue.hint && (
                  <div className="doctor-hint">{issue.hint}</div>
                )}
                {issue.line && (
                  <div className="doctor-line">line {issue.lineNo}</div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
