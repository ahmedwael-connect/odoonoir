# Odoonoir GUI (Wails v3)

Desktop GUI for odoonoir, built on [Wails v3](https://v3.wails.io) and React.
It talks to the same engine the CLI uses — `internal/service` — so every
operation behaves exactly like the terminal commands.

## Prerequisites

- Go 1.24+
- Node.js 18+ and npm
- WebKitGTK4 dev libraries (Ubuntu) — required to compile the wails3 CLI
  and the app itself:

  ```sh
  sudo apt install libwebkitgtk-6.0-dev libgtk-4-dev build-essential pkg-config
  ```

- The [Wails CLI](https://v3.wails.io) (`wails3`), pinned to the Wails
  version this module uses (`github.com/wailsapp/wails/v3` at
  `v3.0.0-alpha2.121`):

  ```sh
  go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-alpha2.121
  ```

## Develop

```sh
cd gui
go mod tidy
cd frontend && npm install && cd ..
wails3 dev
```

`wails3 dev` rebuilds the Go backend on changes and hot-reloads the React
frontend (also reachable in a browser at `localhost:34115`).

## Build / package

```sh
cd gui
wails3 build        # -> bin/odoonoir-gui (debug)
wails3 package      # -> AppImage/deb/rpm/AUR packages
```

## How it fits together

- `main.go` — window + service registration (`application.New`).
- `app.go` — `App` struct: every exported method is exposed to the frontend
  as a Promise. It delegates to `internal/service` (the shared engine).
- `frontend/src/App.tsx` — instance list with live status, start/stop/restart
  actions, and an event subscription (`odoonoir-event`) for instant refresh.
- `frontend/bindings/` — auto-generated, do not edit.

## Engine API

The Go side currently exposes: `Instances`, `Status`, `Databases`, `Start`,
`Stop`, `Restart`, `Update`, `LogsSince`, `LogPath`. More screens (create
wizard, DB manager, conf editor, log viewer) are added by exposing the rest
of `internal/service` and adding React views in `frontend/src`.