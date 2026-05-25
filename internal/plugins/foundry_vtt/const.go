// Package foundry_vtt provides the Foundry VTT integration plugin.
//
// const.go centralizes the plugin's identifier constants — strings that
// were previously interpolated as raw literals from other packages
// (per `reports/chronicle/2026-05-23-c-plugin-isolation-audit.md §1.1`).
// Per T-B2 (plugin isolation), all references to these identifiers from
// outside `internal/plugins/foundry_vtt/` should import these constants
// rather than carry the string literal.

package foundry_vtt

// ModuleSource is the WebSocket Source identifier the Foundry module
// self-reports on its WS upgrade URL (`?client=foundry-module`) and the
// Hub stores on Client.Source to drive Foundry-presence tracking.
//
// Lives here (the owning plugin) rather than in `internal/websocket/`
// per T-B2: only foundry_vtt knows what identifies a Foundry-module
// connection; the websocket package is generic transport.
//
// Note this is conceptually distinct from
// `packages.PackageTypeFoundryModule` ("foundry-module" as a value of
// the `PackageType` enum, owned by the packages plugin). They share a
// string value by coincidence — same name, different roles. Don't
// collapse them.
const ModuleSource = "foundry-module"
