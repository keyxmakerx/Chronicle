package app

// health_config.go centralizes the startup health-check configuration so the
// two surfaces that run it — boot (RunStartupHealthChecks) and the admin
// Database > Health tab (RunHealthChecks) — can never disagree about what
// "healthy" means.

import (
	"github.com/keyxmakerx/chronicle/internal/config"
	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// StartupHealthCheckConfig builds the HealthCheckConfig used at boot and on
// demand by the admin Health tab. The critical-column pins document which
// migration each column came from — a deploy missing that migration fails fast
// with a precise error instead of a downstream 500.
func StartupHealthCheckConfig(cfg *config.Config) database.HealthCheckConfig {
	return database.HealthCheckConfig{
		ExpectedMigrationVersion: database.ExpectedCoreMigrationVersion,
		CriticalColumns: map[string][]string{
			"campaigns":        {"id", "name", "slug", "archived_at", "join_code", "settings", "sidebar_config"},
			"entities":         {"id", "campaign_id", "name", "slug", "entry", "entry_html", "fields_data", "visibility", "owner_user_id", "map_id"},
			"users":            {"id", "email", "display_name", "password_hash"},
			"campaign_members": {"campaign_id", "user_id", "role"},
			"entity_notes":     {"id", "entity_id", "campaign_id", "author_user_id", "audience", "shared_with", "pinned"},
			"media_files":      {"id", "campaign_id", "uploaded_by", "filename", "mime_type", "file_size", "content_hash"},
			"tag_permissions":  {"id", "tag_id", "subject_type", "subject_id", "created_by", "created_at"},
			"entity_types":     {"id", "campaign_id", "name", "slug", "claimable"},
		},
		Env:        cfg.Env,
		BaseURL:    cfg.BaseURL,
		DBTLSMode:  cfg.Database.TLSMode,
		DBPassword: cfg.Database.Password,
		DBHost:     cfg.Database.Host,
		DBUser:     cfg.Database.User,
		DBName:     cfg.Database.Name,
		SmokeTests: []database.SmokeTest{
			campaigns.ScanSmokeTest(),
		},
	}
}
