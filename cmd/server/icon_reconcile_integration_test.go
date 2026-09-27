package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/app"
	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/foundry_vtt"
	"github.com/keyxmakerx/chronicle/internal/plugins/syncapi"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// newIconSchema bootstraps the full schema the way main.go does.
func newIconSchema(t *testing.T) *sql.DB {
	t.Helper()
	db, dsn := newScratchSchema(t)
	if err := database.RunMigrations(db, dsn, coreMigrationsDir(t)); err != nil {
		t.Fatalf("core migrations: %v", err)
	}
	if err := foundry_vtt.ReconcileConsolidationState(context.Background(), db); err != nil {
		t.Fatalf("foundry_vtt reconcile: %v", err)
	}
	for _, r := range database.RunPluginMigrations(db, registeredPlugins()) {
		if !r.Healthy {
			t.Fatalf("plugin %q degraded: %v", r.Slug, r.Error)
		}
	}
	return db
}

// TestIconReconcile_RealSchema bootstraps the full schema the way main.go does
// and runs the boot icon reconciler over app.IconColumns(): every declared
// column must exist and fit a maximum-length icon, bad stored values must be
// reset to the column's default, style-prefixed ones repaired to the bare
// name, good ones left alone, and a second run must change nothing.
func TestIconReconcile_RealSchema(t *testing.T) {
	if testing.Short() {
		t.Skip("requires a database; skipped under -short")
	}
	db := newIconSchema(t)
	ctx := context.Background()

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
	exec(`INSERT INTO entity_types (campaign_id, slug, name, name_plural, icon) VALUES (?, 'styled', 'Styled', 'Styleds', 'fa-solid fa-anchor')`, campaignID)
	exec(`INSERT INTO entity_types (campaign_id, slug, name, name_plural, icon) VALUES (?, 'styled-junk', 'Junk', 'Junks', ?)`, campaignID, `fa-solid fa-anchor" onmouseover`)
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
	if n != 6 {
		t.Errorf("first run changed %d rows, want 6", n)
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
		{"style-prefixed entity type repaired", icon(`SELECT icon FROM entity_types WHERE slug = 'styled'`), "fa-anchor"},
		{"style-prefixed junk reset", icon(`SELECT icon FROM entity_types WHERE slug = 'styled-junk'`), defaults["entity_types"]},
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

// TestIconReconcile_SyncAPICreateEntityType sends the category body the
// Foundry module's import wizard sends, through the sync API handler and the
// real entities service and repositories, and checks the stored icon is the
// bare name.
func TestIconReconcile_SyncAPICreateEntityType(t *testing.T) {
	if testing.Short() {
		t.Skip("requires a database; skipped under -short")
	}
	db := newIconSchema(t)

	const (
		userID     = "00000000-0000-0000-0000-00000000001a"
		campaignID = "00000000-0000-0000-0000-00000000001b"
	)
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO users (id, email, display_name, password_hash) VALUES (?, 'sync-icons@example.test', 'Sync', 'x')`, []any{userID}},
		{`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, 'Sync Icons', 'sync-icons', ?)`, []any{campaignID, userID}},
	} {
		if _, err := db.Exec(q.sql, q.args...); err != nil {
			t.Fatalf("exec %q: %v", q.sql, err)
		}
	}

	svc := entities.NewEntityService(entities.NewEntityRepository(db), entities.NewEntityTypeRepository(db), nil)
	h := syncapi.NewAPIHandler(nil, svc, nil, nil)

	tests := []struct {
		name, body string
		wantStatus int
		wantIcon   string
	}{
		{"style-prefixed icon stored bare", `{"name":"Ships","icon":"fa-solid fa-circle"}`, http.StatusCreated, "fa-circle"},
		{"bare icon kept", `{"name":"Ports","icon":"fa-anchor"}`, http.StatusCreated, "fa-anchor"},
		{"brand icon refused", `{"name":"Guilds","icon":"fa-brands fa-github"}`, http.StatusBadRequest, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/"+campaignID+"/entity-types", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id")
			c.SetParamValues(campaignID)

			err := h.CreateEntityType(c)
			if tt.wantStatus != http.StatusCreated {
				var ae *apperror.AppError
				if !errors.As(err, &ae) || ae.Code != tt.wantStatus {
					t.Fatalf("CreateEntityType error = %v, want status %d", err, tt.wantStatus)
				}
				return
			}
			if err != nil {
				t.Fatalf("CreateEntityType: %v", err)
			}
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			var created struct {
				ID   int    `json:"id"`
				Icon string `json:"icon"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
				t.Fatalf("decoding response: %v", err)
			}
			if created.Icon != tt.wantIcon {
				t.Errorf("response icon = %q, want %q", created.Icon, tt.wantIcon)
			}
			var stored string
			if err := db.QueryRow(`SELECT icon FROM entity_types WHERE id = ?`, created.ID).Scan(&stored); err != nil {
				t.Fatalf("reading stored icon: %v", err)
			}
			if stored != tt.wantIcon {
				t.Errorf("stored icon = %q, want %q", stored, tt.wantIcon)
			}
		})
	}
}
