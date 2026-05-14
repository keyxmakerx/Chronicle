// Package foundry_modules manages the catalog of Foundry VTT module
// versions an admin has uploaded to Chronicle. It owns:
//
//   - Admin upload / status / notify / force-pin endpoints
//   - Per-campaign pinning (the version field lives on CampaignSettings;
//     the foundry_modules service is the one that validates pin choices)
//   - The public manifest endpoint Foundry hits (signed with a per-
//     campaign token; replaces GitHub-served updates)
//
// The plugin is server-global — versions are owned by the site admin,
// not by individual campaigns. Per-campaign concerns (pin, token version)
// live in dedicated tables / settings fields.
package foundry_modules

import "embed"

// MigrationsFS contains the embedded SQL migration files for the
// foundry_modules plugin. The cmd/server/main.go registeredPlugins()
// loop reads from this and runs migrations at boot.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS
