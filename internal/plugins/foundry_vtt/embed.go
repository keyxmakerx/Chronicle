package foundry_vtt

import "embed"

// MigrationsFS embeds the foundry_vtt plugin's migrations directory.
// Registered with the database.RunPluginMigrations runner from
// cmd/server/main.go. The migrations rename foundry_modules' token
// table into this plugin's namespace and drop foundry_modules'
// versions table (added in C-FMC-5c when foundry_modules is deleted).
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS
