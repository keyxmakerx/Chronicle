// registration.go exposes the plugin's canonical slug for the App's
// lightweight plugin registry.
//
// Per cordinator/decisions/2026-05-23-plugin-registration.md (the
// architectural shape decision) + NW-2.2 Chunk A.
//
// Today this file holds only the PluginSlug const — the App's registry
// is metadata-only. There is intentionally NO Registration() function
// exported from this file because PluginRegistration lives in
// internal/app, and internal/app already imports this package; a
// reciprocal import would create a cycle. The decision doc records
// the "deferred Host-interface chunk" that resolves this in a future
// pass.

package foundry_vtt

// PluginSlug is the canonical identifier for the foundry_vtt plugin
// in the App's PluginRegistration registry. Matches the WS source
// identifier (foundry_vtt.ModuleSource) by value but is conceptually
// distinct — Slug is the plugin's name in the registry; ModuleSource
// is the WS protocol identifier. They happen to share the string
// because Chronicle's external wire-protocol uses the plugin slug.
const PluginSlug = "foundry-vtt"
