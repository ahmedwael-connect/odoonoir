# OdooNoir

Odoo instance manager for Ubuntu — install, run, and develop Odoo like a pro.
Docker-style workflow, but purpose-built for Odoo (17/18/19).

## Install

### CLI

```bash
make build          # builds bin/odoonoir
make install        # builds and copies to ~/.local/bin (adds it to PATH if missing)
```

### GUI (Desktop App)

Download the `.deb` package from [Releases](https://github.com/ahmedwael-connect/odoonoir/releases) and install:

```bash
sudo apt install ./odoonoir_0.5.0_amd64.deb
```

Or build from source:

```bash
# Install build prerequisites (Ubuntu)
sudo apt install libwebkitgtk-6.0-dev libgtk-4-dev build-essential pkg-config

# Build frontend + backend
cd gui/frontend && npm install && npm run build && cd ..
go build -tags "bindings,production" -trimpath -ldflags="-w -s" -o bin/odoonoir .

# Create .deb package
go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest
nfpm pkg -f linux/nfpm/nfpm.yaml -p deb -t bin/odoonoir_0.5.0_amd64.deb
sudo dpkg -i bin/odoonoir_0.5.0_amd64.deb
```

Launch from **Activities** → search **OdooNoir**, or run `odoonoir-launcher` in terminal.

## Quick start

```bash
odooNoir check -v 18              # audit the system for Odoo 18
odooNoir create myapp -v 18       # full install: check, clone, venv, conf, db, start
odooNoir list                     # all instances
odooNoir start myapp              # http://localhost:8069
odooNoir list myapp               # the databases served by myapp
odooNoir start myapp sales        # serve a specific database of myapp
```

## Commands

| Command | Description |
|---|---|
| `check [-v 17/18/19]` | System audit: python, node, git, PostgreSQL, apt libs, version↔python compat |
| `create <name> -v 17\|18\|19` | Full setup: system check → clone source → venv → pip requirements → role+db → `odoo.conf` → `custom_addons` → DB init (`-i base`) → start |
| `adopt [name]` | Import an **existing** Odoo installation (read-only): auto-detects source/conf/venv/port/db/addons, discovers every database, `--check` audits it, `--scan` finds installs on the system, `--scan-dir <path>` searches inside a specific directory instead |
| `list [instance] [--running] [--json]` | All instances, or the databases served by one instance (size, owner, init state, * = primary); `--running` filters, `--json` prints machine-readable output |
| `start/stop/restart/status <name> [db] [--all]` | Process control (SIGTERM → SIGKILL); `start <name> <db>` serves a specific database (`-d db`, remembered by `restart`, shown by `status`); running state is verified against the process table, so externally-managed instances are detected too |
| `init <name> <db>` | Create + initialize a database on an instance (`-i base`) |
| `backup/restore/drop <name> [db]` | Per-database lifecycle (pg_dump/pg_restore); refuse while the instance is running. `restore` validates the dump **before** touching anything and can restore into a new database name (tracked automatically); dumps stream instead of loading into RAM |
| `config list\|get\|set\|unset\|path <name>` | Edit `odoo.conf` attributes (persisted with `[options]` section, restart to apply) |
| `edit <name> [--conf --log --pidfile --venv --python --source --port --db --version --description]` | Fix instance metadata after adoption |
| `config comment/uncomment <name> <key>` | Disable an option in place or re-enable it |
| `config addons <name> [add\|remove\|move\|up\|down]` | `addons_path` as a list: numbered by priority, reorder/add/delete; `--edit` opens an interactive editor |
| `logs <name> [-f] [--errors]` | Tail logs; `--errors` filters detected failures with fix hints |
| `doctor <name>` | Diagnose the log: port conflicts, DB auth, missing modules, missing deps |
| `module new/list/uninstall <name> [db]` | Scaffold complete modules; list what a database has installed; uninstall a module safely through the odoo shell |
| `test <name> [db] -m <modules>` | Run the test suite of modules |
| `update <name> [db] [-i mod] [-u mod]` | git pull + addons_path refresh + pip reinstall + module install/upgrade |
| `shell <name> [db] [-c CODE \| --script FILE \| -- args…]` | Interactive `odoo shell` in the instance venv |
| `clone <name> <newname> [--dbs]` | Copy source, modules, data and database(s) |
| `rename <name> <newname>` | Move an instance |
| `remove <name> --force` | Managed: stop, drop database, delete files. Adopted: unregister only |
| `ps` | Process overview of every instance |
| `watch <name> [--tail N] [--interval D]` | Live status + colored log tail |
| `dash` | Interactive TUI: logs, detail panel, conf viewer, doctor, help overlay |
| `completion [bash\|zsh\|fish\|powershell]` | Shell completion scripts |

## GUI Features

The desktop GUI (Wails v3 + React) provides:

- **Instance management** — start/stop/restart from sidebar, real-time status polling
- **Database manager** — browse databases, backup/restore/drop, switch serving database
- **Module manager** — list modules per database, uninstall with confirmation
- **Config editor** — edit `odoo.conf` with per-entry revert, batch save
- **Live logs** — streaming with search, filter, syntax highlighting, rotation detection
- **Update runner** — install/update modules with scoped progress events
- **Create wizard** — step-by-step instance creation with real-time progress
- **Adopt existing** — import current Odoo installations
- **Doctor** — scan logs for known issues with severity and fix hints
- **Keyboard shortcuts** — `1`-`9` screens, `r` restart, `s` start/stop, `c` create, `Esc` back
- **Event log strip** — color-coded event history with severity coloring
- **Responsive layout** — sidebar/eventlog collapse on smaller screens

## Instances and databases

- One instance = one process on one port, serving its **primary database**
  plus any number of **additional databases**.
- Every database command takes an instance: `init myapp otherdb`,
  `backup myapp otherdb`, `restore myapp dump.dump otherdb`,
  `drop --force myapp otherdb`.
- `start <name> <db>` serves a specific database (`-d db`); the choice is
  remembered for `restart` and shown by `status` (`serving:` line).
- Adopted instances work exactly like created ones.

## Instance layout

```
~/.odoonoir/instances/myapp/
├── src/odoo/                    # git-cloned Odoo source (version branch)
├── custom_addons/               # your modules
├── venv/                        # isolated python environment
├── etc/odoo.conf                # generated + editable via `odoonoir config`
├── logs/odoo.log
└── data/                        # filestore
```

Configuration lives in `~/.odoonoir/config.json` (instances root, postgres
role). PostgreSQL access is auto-detected (peer auth preferred; roles and
databases are created on demand).

## Safety

- `restore` validates the dump file **before** dropping or creating anything
- `create` is all-or-nothing: on failure it rolls back folder, database, and registry entry
- Ports are checked against the registry — two stopped instances can never silently claim the same port
- SQL injection prevention: all database identifiers validated, parameterized where possible
- Config mutation is mutex-protected for concurrent safety

## Development

```bash
# CLI
make build   # go build
make test    # unit tests
make vet     # static analysis

# GUI (dev mode)
cd gui
export WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1
wails3 dev
```

## Roadmap

- [ ] `upgrade`: switch an instance between Odoo minors (git checkout + pip + DB upgrade)
- [ ] pip dependency freeze/lock per instance
- [ ] `bootstrap`: one-command setup of system deps + postgres role
- [ ] Windows/macOS support
- [ ] Multi-language UI

## License

MIT
