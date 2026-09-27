package app

// timeline_entity_visibility_leak_test.go pins that the timeline plugin's
// event listing and entity-group listing route through the entities
// plugin's canonical FilterViewableEntityIDs (the same
// entityVisibilityFilterAdapter maps, sessions, npcs and armory use), not an
// unfiltered join — so a private entity linked to an 'everyone' timeline
// event or entity group is never named to a viewer who can't otherwise see
// that entity. Runs against a real database, in internal/app (timeline may
// not import the entities repository directly, per plugin isolation),
// because a mock of the predicate would not prove the wiring is real.
//
// Skipped under -short. Run against tools/start-test-db.sh's local MariaDB
// with `CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test
// ./internal/app/ -run 'Timeline.*VisibilityLeak' -v`.

import (
	"context"
	"database/sql"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/timeline"
)

// openTimelineTestDB is openGalleryTestDB plus the timeline plugin's tables,
// which the core migrations don't create. Timeline's tables reference the
// calendar plugin's, so those go first, as at startup.
func openTimelineTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db := openGalleryTestDB(t)
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	sub, err := fs.Sub(timeline.MigrationsFS, database.PluginMigrationsSubdir)
	if err != nil {
		t.Fatalf("sub-FS: %v", err)
	}
	calDir := os.DirFS(filepath.Join(root, "internal", "plugins", "calendar", "migrations"))
	for _, res := range database.RunPluginMigrations(db, []database.PluginSchema{
		{Slug: "calendar", MigrationsFS: calDir},
		{Slug: "timeline", MigrationsFS: sub},
	}) {
		// Fatal, not skip: the server answered, so a failure here is real and
		// must not pass quietly as a skipped test.
		if !res.Healthy {
			t.Fatalf("%s plugin migrations did not apply: %v", res.Slug, res.Error)
		}
	}
	return db
}

// TestTimelineEvents_PrivateEntityVisibilityLeak is the timeline-event half
// of ADR-055 rule 3 for this plugin: a standalone event's own visibility is
// 'everyone', but it links to a private entity a Player was never granted.
// The event itself must still show; only the entity's id/name/icon must be
// hidden.
func TestTimelineEvents_PrivateEntityVisibilityLeak(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTimelineTestDB(t)
	defer db.Close()
	ctx := context.Background()

	fx := newGalleryFixture(t, db)
	defer fx.cleanup()

	entityTypeID := fx.entityType("timeline-vis-npc", "NPC", "", "")
	privateEntityID := galleryTestUUID(t)
	mustGalleryExec(t, db,
		`INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, is_private, visibility, created_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, true, 'default', ?, NOW(), NOW())`,
		privateEntityID, fx.campaignID, entityTypeID, "Secret Spy", "secret-spy-timeline", fx.ownerID)

	timelineID := galleryTestUUID(t)
	mustGalleryExec(t, db, `INSERT INTO timelines (id, campaign_id, name) VALUES (?, ?, ?)`,
		timelineID, fx.campaignID, "Vis Leak Test Timeline")

	timelineRepo := timeline.NewTimelineRepository(db)
	eventID := galleryTestUUID(t)
	if err := timelineRepo.CreateEvent(ctx, &timeline.TimelineEvent{
		ID: eventID, TimelineID: timelineID, EntityID: &privateEntityID,
		Name: "Meeting With The Spy", Year: 1, Month: 1, Day: 1, Visibility: "everyone",
	}); err != nil {
		t.Fatalf("create standalone event: %v", err)
	}

	timelineSvc := timeline.NewTimelineService(timelineRepo, nil, nil, nil)
	entityService := fx.entityService()
	g, ok := timelineSvc.(interface {
		SetEntityVisibilityGate(timeline.EntityVisibilityGate)
	})
	if !ok {
		t.Fatal("timelineService does not expose SetEntityVisibilityGate")
	}
	g.SetEntityVisibilityGate(&entityVisibilityFilterAdapter{svc: entityService})

	cases := []struct {
		name       string
		role       int
		userID     string
		wantEntity bool
	}{
		{"anonymous viewer on a public campaign", permissions.RoleNone, "", false},
		{"player", permissions.RolePlayer, "player-1", false},
		{"owner", permissions.RoleOwner, fx.ownerID, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			events, err := timelineSvc.ListTimelineEvents(ctx, timelineID, fx.campaignID, permissions.RequestViewer(tc.role, tc.userID))
			if err != nil {
				t.Fatalf("ListTimelineEvents: %v", err)
			}
			if len(events) != 1 {
				t.Fatalf("expected exactly 1 event, got %d", len(events))
			}
			evt := events[0]
			if evt.EventName != "Meeting With The Spy" {
				t.Errorf("the event itself (visibility=everyone) must still show; got name %q", evt.EventName)
			}
			gotEntity := evt.EventEntityID != nil && *evt.EventEntityID == privateEntityID
			if gotEntity != tc.wantEntity {
				t.Errorf("entity link presence = %v, want %v for %s (role=%d)", gotEntity, tc.wantEntity, tc.name, tc.role)
			}
			if !tc.wantEntity && (evt.EventEntityName != "" || evt.EventEntityIcon != "") {
				t.Errorf("LEAK: %s saw the private entity's name/icon: name=%q icon=%q", tc.name, evt.EventEntityName, evt.EventEntityIcon)
			}
			if tc.wantEntity && evt.EventEntityName != "Secret Spy" {
				t.Errorf("owner should see the entity name unfiltered, got %q", evt.EventEntityName)
			}
		})
	}
}

// TestTimelineEntityGroups_PrivateEntityVisibilityLeak is the entity-group
// half of the same fix: a swim-lane group member pointing at a private
// entity must not name that entity to a viewer who can't otherwise see it.
func TestTimelineEntityGroups_PrivateEntityVisibilityLeak(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTimelineTestDB(t)
	defer db.Close()
	ctx := context.Background()

	fx := newGalleryFixture(t, db)
	defer fx.cleanup()

	entityTypeID := fx.entityType("timeline-vis-group-npc", "NPC", "", "")
	privateEntityID := galleryTestUUID(t)
	mustGalleryExec(t, db,
		`INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, is_private, visibility, created_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, true, 'default', ?, NOW(), NOW())`,
		privateEntityID, fx.campaignID, entityTypeID, "Hidden Cultist", "hidden-cultist-timeline", fx.ownerID)

	timelineID := galleryTestUUID(t)
	mustGalleryExec(t, db, `INSERT INTO timelines (id, campaign_id, name) VALUES (?, ?, ?)`,
		timelineID, fx.campaignID, "Vis Leak Group Test Timeline")

	timelineRepo := timeline.NewTimelineRepository(db)
	group := &timeline.EntityGroup{TimelineID: timelineID, Name: "Cult", Color: "#663399"}
	if err := timelineRepo.CreateEntityGroup(ctx, group); err != nil {
		t.Fatalf("create entity group: %v", err)
	}
	if err := timelineRepo.AddGroupMember(ctx, group.ID, timelineID, privateEntityID); err != nil {
		t.Fatalf("add group member: %v", err)
	}

	timelineSvc := timeline.NewTimelineService(timelineRepo, nil, nil, nil)
	entityService := fx.entityService()
	g, ok := timelineSvc.(interface {
		SetEntityVisibilityGate(timeline.EntityVisibilityGate)
	})
	if !ok {
		t.Fatal("timelineService does not expose SetEntityVisibilityGate")
	}
	g.SetEntityVisibilityGate(&entityVisibilityFilterAdapter{svc: entityService})

	cases := []struct {
		name       string
		role       int
		userID     string
		wantEntity bool
	}{
		{"player", permissions.RolePlayer, "player-1", false},
		{"owner", permissions.RoleOwner, fx.ownerID, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			groups, err := timelineSvc.ListEntityGroups(ctx, timelineID, fx.campaignID, permissions.RequestViewer(tc.role, tc.userID))
			if err != nil {
				t.Fatalf("ListEntityGroups: %v", err)
			}
			if len(groups) != 1 || len(groups[0].Members) != 1 {
				t.Fatalf("expected exactly 1 group with 1 member, got %+v", groups)
			}
			member := groups[0].Members[0]
			gotEntity := member.EntityID == privateEntityID
			if gotEntity != tc.wantEntity {
				t.Errorf("entity link presence = %v, want %v for %s (role=%d)", gotEntity, tc.wantEntity, tc.name, tc.role)
			}
			if !tc.wantEntity && (member.EntityName != "" || member.EntityIcon != "") {
				t.Errorf("LEAK: %s saw the private entity's name/icon: name=%q icon=%q", tc.name, member.EntityName, member.EntityIcon)
			}
			if tc.wantEntity && member.EntityName != "Hidden Cultist" {
				t.Errorf("owner should see the entity name unfiltered, got %q", member.EntityName)
			}
		})
	}
}
