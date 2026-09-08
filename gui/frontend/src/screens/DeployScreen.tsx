import { useState, useCallback } from "react"
import { useToast } from "../App"
import { Button } from "../components/atoms/Button"
import { ReadConf } from "../../bindings/github.com/ahmed/odoonoir/gui/app"

interface ChecklistItem {
  id: string
  label: string
  description: string
  checked: boolean
  critical: boolean
}

export function DeployScreen({ name }: { name: string }) {
  const { toast } = useToast()
  const [loading, setLoading] = useState(false)
  const [checklist, setChecklist] = useState<ChecklistItem[]>([
    { id: "db", label: "PostgreSQL configured", description: "db_host, db_port, db_user set in odoo.conf", checked: false, critical: true },
    { id: "workers", label: "Workers configured", description: "workers > 0 for production (default: CPU cores)", checked: false, critical: true },
    { id: "proxy", label: "Reverse proxy planned", description: "Nginx/Caddy in front of Odoo for SSL + static", checked: false, critical: true },
    { id: "db_password", label: "Strong DB password", description: "Not using default 'odoo' password", checked: false, critical: true },
    { id: "longpolling", label: "Longpolling port exposed", description: "Port (http_port + 3) for real-time", checked: false, critical: false },
    { id: "log", label: "Log file configured", description: "logfile set, not stdout", checked: false, critical: false },
    { id: "data_dir", label: "Data dir on persistent volume", description: "data_dir not in container layer", checked: false, critical: true },
    { id: "backup", label: "Backup strategy", description: "Automated DB backups + filestore", checked: false, critical: false },
    { id: "workers_limit", label: "Worker limits set", description: "limit_memory_worker, limit_time_cpu, limit_time_real", checked: false, critical: false },
    { id: "db_maxconn", label: "DB connection pool", description: "db_maxconn appropriate for worker count", checked: false, critical: false },
  ])
  const [composeContent, setComposeContent] = useState("")
  const [showCompose, setShowCompose] = useState(false)

  const toggleItem = (id: string) => {
    setChecklist((prev) => prev.map((item) => item.id === id ? { ...item, checked: !item.checked } : item))
  }

  const criticalCount = checklist.filter((i) => i.critical).length
  const criticalDone = checklist.filter((i) => i.critical && i.checked).length
  const allDone = checklist.every((i) => i.checked)
  const progress = Math.round((checklist.filter((i) => i.checked).length / checklist.length) * 100)

  const generateCompose = useCallback(async () => {
    setLoading(true)
    try {
      const conf = await ReadConf(name)
      const confMap: Record<string, string> = {}
      for (const entry of conf) { confMap[entry.key] = entry.value }

      const httpPort = confMap["http_port"] || "8069"
      const dbHost = confMap["db_host"] || "db"
      const dbName = confMap["db_name"] || "odoo"
      const dbUser = confMap["db_user"] || "odoo"
      const workers = confMap["workers"] || "4"
      const dataDir = confMap["data_dir"] || "/var/lib/odoo"

      const compose = `version: "3.8"

services:
  odoo:
    image: odoo:${name.includes("15") ? "15.0" : name.includes("16") ? "16.0" : name.includes("17") ? "17.0" : name.includes("18") ? "18.0" : "18.0"}
    depends_on:
      - db
    ports:
      - "${httpPort}:8069"
      - "${String(Number(httpPort) + 3)}:8072"
    volumes:
      - odoo-data:${dataDir}
      - ./addons:/mnt/extra-addons
      - ./odoo.conf:/etc/odoo/odoo.conf
    environment:
      - HOST=${dbHost}
      - PORT=5432
      - USER=${dbUser}
      - PASSWORD=\${DB_PASSWORD}
    restart: unless-stopped
    deploy:
      resources:
        limits:
          memory: ${Number(workers) * 512}M

  db:
    image: postgres:15
    environment:
      - POSTGRES_DB=postgres
      - POSTGRES_USER=${dbUser}
      - POSTGRES_PASSWORD=\${DB_PASSWORD}
    volumes:
      - odoo-db:/var/lib/postgresql/data
    restart: unless-stopped
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${dbUser}"]
      interval: 10s
      timeout: 5s
      retries: 5

volumes:
  odoo-data:
  odoo-db:
`
      setComposeContent(compose)
      setShowCompose(true)
      toast("Docker Compose generated", "success")
    } catch (e) {
      toast(`Error: ${e}`, "error")
    } finally {
      setLoading(false)
    }
  }, [name, toast])

  const downloadCompose = () => {
    const blob = new Blob([composeContent], { type: "text/yaml" })
    const url = URL.createObjectURL(blob)
    const a = document.createElement("a")
    a.href = url
    a.download = "docker-compose.yml"
    a.click()
    URL.revokeObjectURL(url)
  }

  return (
    <div className="screen">
      <div className="card">
        <div className="card-header">
          <h2>Deploy — {name}</h2>
          <div className="action-row">
            <div className="text-sm text-muted-foreground">
              {criticalDone}/{criticalCount} critical
            </div>
            <div className="w-24 h-2 bg-muted rounded-full overflow-hidden">
              <div className="h-full bg-primary rounded-full transition-all" style={{ width: `${progress}%` }} />
            </div>
            <span className="text-xs text-muted-foreground">{progress}%</span>
          </div>
        </div>

        <div className="p-4 space-y-3">
          <div className="text-sm text-muted-foreground mb-4">
            Production deployment checklist. Complete all critical items before deploying.
          </div>

          {checklist.map((item) => (
            <label
              key={item.id}
              className={`flex items-start gap-3 p-3 rounded-lg border cursor-pointer transition-colors ${
                item.checked ? "bg-primary/5 border-primary/20" : "bg-card hover:bg-muted/50"
              } ${item.critical && !item.checked ? "border-destructive/30" : ""}`}
            >
              <input
                type="checkbox"
                checked={item.checked}
                onChange={() => toggleItem(item.id)}
                className="mt-0.5"
              />
              <div className="flex-1">
                <div className="text-sm font-medium flex items-center gap-2">
                  {item.label}
                  {item.critical && <span className="text-xs text-destructive font-normal">critical</span>}
                </div>
                <div className="text-xs text-muted-foreground mt-0.5">{item.description}</div>
              </div>
            </label>
          ))}
        </div>

        <div className="p-4 border-t space-y-3">
          <div className="flex gap-2">
            <Button
              variant="primary"
              onClick={generateCompose}
              loading={loading}
              disabled={loading}
            >
              Generate docker-compose.yml
            </Button>
            {showCompose && (
              <Button variant="outline" onClick={downloadCompose}>
                Download
              </Button>
            )}
          </div>

          {showCompose && (
            <div className="mt-4">
              <div className="text-sm font-medium mb-2">docker-compose.yml</div>
              <pre className="bg-muted p-4 rounded-lg text-xs overflow-x-auto max-h-96 overflow-y-auto">
                {composeContent}
              </pre>
              <div className="mt-3 text-xs text-muted-foreground space-y-1">
                <div>1. Save the file as <code className="bg-muted px-1 rounded">docker-compose.yml</code> in your project root</div>
                <div>2. Create a <code className="bg-muted px-1 rounded">.env</code> file with <code className="bg-muted px-1 rounded">DB_PASSWORD=your_secret</code></div>
                <div>3. Run <code className="bg-muted px-1 rounded">docker compose up -d</code></div>
                <div>4. Access Odoo at <code className="bg-muted px-1 rounded">http://localhost:{checklist[0] ? "8069" : "8069"}</code></div>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
