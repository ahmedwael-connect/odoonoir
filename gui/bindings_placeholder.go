//go:build bindings

package main

// This file is only used by `wails generate bindings` so the frontend can
// import typed bindings from the module path:
//   import { ... } from "../bindings/github.com/ahmed/odoonoir/gui/App"
//
// The actual binding generation is automatic during `wails3 build`/`dev`.
