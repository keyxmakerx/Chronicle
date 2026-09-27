package main

import (
	"context"
	"database/sql"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/app"
	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/plugins/foundry_vtt"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// TestIconReconcile_RealSchema bootstraps the full schema the way main.go does
// and runs the boot icon reconciler over app.IconColumns(): every declared
// column must exist and fit a maximum-length icon, bad stored values must be
// reset to the column's default, good ones left alone, and a second run must
// change nothing.
func TestIconReconcile_RealSchema(t *testing.T) {
	if testing.Short() {
		t.Skip("requires a database; skipped under -short")
	}
	db, dsn := newScratchSchema(t)
	ctx := context.Background()

	if err := database.RunMigrations(db, dsn, coreMigrationsDir(t)); err != nil {
		t.Fatalf("core migrations: %v", err)
	}
	if err := foundry_vtt.ReconcileConsolidationState(ctx, db); err != nil {
		t.Fatalf("foundry_vtt reconcile: %v", err)
	}
	for _, r := range database.RunPluginMigrations(db, registeredPlugins()) {
		if !r.Healthy {
			t.Fatalf("plugin %q degraded: %v", r.Slug, r.Error)
		}
	}

	cols := app.IconColumns()
	if len(cols) == 0 {
		t.Fatal("app.IconColumns() is empty")
	}
	defaults := map[string]string{}
	for _, c := range cols {
		var width sql.NullInt64
		err := db.QueryRow(`SELECT CHARACTER_MAXIMUM_LENGTH FROM information_schema.COLUMNS
			WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?`, c.Table, c.Column).Scan(&width)
		if err != nil {
			t.Errorf("%s.%s is declared as an icon column but does not exist: %v", c.Table, c.Column, err)
			continue
		}
		if width.Int64 < sanitize.MaxIconLength {
			t.Errorf("%s.%s is VARCHAR(%d), narrower than the icon cap %d", c.Table, c.Column, width.Int64, sanitize.MaxIconLength)
		}
		defaults[c.Table] = c.Default
	}

	const (
		userID     = "00000000-0000-0000-0000-00000000000a"
		campaignID = "00000000-0000-0000-0000-00000000000b"
		mapID      = "00000000-0000-0000-0000-00000000000c"
		bad        = `fa-x" data-y="z`
	)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	exec(`INSERT INTO users (id, email, display_name, password_hash) VALUES (?, 'icons@example.test', 'Icons', 'x')`, userID)
	exec(`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, 'Icons', 'icons', ?)`, campaignID, userID)
	exec(`INSERT INTO entity_types (campaign_id, slug, name, name_plural, icon) VALUES (?, 'bad', 'Bad', 'Bads', ?)`, campaignID, bad)
	exec(`INSERT INTO entity_types (campaign_id, slug, name, name_plural, icon) VALUES (?, 'good', 'Good', 'Goods', 'fa-dragon')`, campaignID)
	exec(`INSERT INTO maps (id, campaign_id, name) VALUES (?, ?, 'Map')`, mapID, campaignID)
	exec(`INSERT INTO map_markers (id, map_id, name, icon) VALUES ('mk-bad', ?, 'Bad', 'fa-<b>')`, mapID)
	exec(`INSERT INTO map_markers (id, map_id, name, icon) VALUES ('mk-good', ?, 'Good', 'fa-castle')`, mapID)
	exec(`INSERT INTO timelines (id, campaign_id, name, icon) VALUES ('tl-bad', ?, 'Bad', 'FA-TIMELINE')`, campaignID)
	exec(`INSERT INTO layout_presets (campaign_id, name, layout_json, icon) VALUES (?, 'Bad', '{}', 'fa-')`, campaignID)
	exec(`INSERT INTO inventory_instances (campaign_id, name, slug, icon) VALUES (?, 'Loot', 'loot', NULL)`, campaignID)

	n, err := database.ReconcileIconColumns(ctx, db, cols)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if n != 4 {
		t.Errorf("first run changed %d rows, want 4", n)
	}

	icon := func(q string, args ...any) string {
		t.Helper()
		var s sql.NullString
		if err := db.QueryRow(q, args...).Scan(&s); err != nil {
			t.Fatalf("query %q: %v", q, err)
		}
		if !s.Valid {
			return "<NULL>"
		}
		return s.String
	}
	checks := []struct{ name, got, want string }{
		{"bad entity type", icon(`SELECT icon FROM entity_types WHERE slug = 'bad'`), defaults["entity_types"]},
		{"good entity type", icon(`SELECT icon FROM entity_types WHERE slug = 'good'`), "fa-dragon"},
		{"bad marker", icon(`SELECT icon FROM map_markers WHERE id = 'mk-bad'`), defaults["map_markers"]},
		{"good marker", icon(`SELECT icon FROM map_markers WHERE id = 'mk-good'`), "fa-castle"},
		{"bad timeline", icon(`SELECT icon FROM timelines WHERE id = 'tl-bad'`), defaults["timelines"]},
		{"bad layout preset", icon(`SELECT icon FROM layout_presets WHERE name = 'Bad'`), defaults["layout_presets"]},
		{"null inventory icon", icon(`SELECT icon FROM inventory_instances WHERE slug = 'loot'`), "<NULL>"},
	}
	for _, c := range checks {
		if c.want == "" {
			t.Errorf("%s: no default declared for its table", c.name)
		}
		if c.got != c.want {
			t.Errorf("%s: icon = %q, want %q", c.name, c.got, c.want)
		}
	}

	n, err = database.ReconcileIconColumns(ctx, db, cols)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if n != 0 {
		t.Errorf("second run changed %d rows, want 0", n)
	}
}
