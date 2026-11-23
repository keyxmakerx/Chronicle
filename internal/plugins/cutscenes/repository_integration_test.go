package cutscenes

// Real-MariaDB tests for the repository: what the SQL does is the point.
// Names are unique per campaign ignoring case, every read stays in its
// campaign, a showing keeps its snapshot after the scene is deleted, a viewer
// sees only their own showings inside the window, watched and stopped are
// stamped once, and deleting a campaign takes everything along.
//
//	make test-db-up
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/plugins/cutscenes/ -run TestDB

import (
	"context"
	"database/sql"
	"encoding/json"
	"io/fs"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/database/dbtest"
)

func scratch(t *testing.T) *sql.DB {
	t.Helper()
	sub, err := fs.Sub(MigrationsFS, database.PluginMigrationsSubdir)
	if err != nil {
		t.Fatalf("sub-FS: %v", err)
	}
	return dbtest.Open(t, dbtest.Options{
		Prefix:           "chronicle_cut_",
		RequireDSN:       true,
		StrictMigrations: true,
		Plugins:          []dbtest.PluginMigrations{{Slug: PluginSlug, FS: sub}},
	})
}

func seedUser(t *testing.T, db *sql.DB, name string) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := db.Exec(`INSERT INTO users (id, email, display_name, password_hash) VALUES (?,?,?,?)`, id, id+"@example.test", name, "x"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func seedCampaign(t *testing.T, db *sql.DB, owner, name string) string {
	t.Helper()
	id := uuid.NewString()
	settings, _ := json.Marshal(map[string]string{})
	if _, err := db.Exec(`INSERT INTO campaigns (id, name, slug, created_by, settings) VALUES (?,?,?,?,?)`, id, name, id, owner, string(settings)); err != nil {
		t.Fatalf("seed campaign: %v", err)
	}
	return id
}

func TestDBScenesAndShowings(t *testing.T) {
	db := scratch(t)
	ctx := context.Background()
	repo := NewRepository(db)
	dir, ana, bo := seedUser(t, db, "Dir"), seedUser(t, db, "Ana"), seedUser(t, db, "Bo")
	c1, c2 := seedCampaign(t, db, dir, "One"), seedCampaign(t, db, dir, "Two")

	eye := NewScene()
	eye.Name = "The Eye Opens"
	sc, err := Normalize(eye)
	if err != nil {
		t.Fatal(err)
	}
	sc.ID = uuid.NewString()
	if err := repo.CreateScene(ctx, c1, dir, sc); err != nil {
		t.Fatal(err)
	}

	t.Run("a scene reads back whole, and only in its campaign", func(t *testing.T) {
		got, err := repo.GetScene(ctx, c1, sc.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != sc.Name || got.Background != sc.Background || len(got.Overlays) != 1 || got.Overlays[0].Params.Color != DefaultEyeColor ||
			len(got.Text) != 1 || got.Text[0].Style != StyleWhisper || got.Ending.Kind != EndingTap {
			t.Errorf("read back %+v", got)
		}
		if _, err := repo.GetScene(ctx, c2, sc.ID); code(err) != 404 {
			t.Errorf("another campaign read it: %v", err)
		}
		if l, _ := repo.ListScenes(ctx, c2); len(l) != 0 {
			t.Errorf("another campaign's list = %+v", l)
		}
	})

	t.Run("names are unique per campaign, ignoring case", func(t *testing.T) {
		dup := sc
		dup.ID, dup.Name = uuid.NewString(), "the eye opens"
		if err := repo.CreateScene(ctx, c1, dir, dup); code(err) != 409 {
			t.Errorf("duplicate name: %v", err)
		}
		if err := repo.CreateScene(ctx, c2, dir, dup); err != nil {
			t.Errorf("the same name in another campaign: %v", err)
		}
		other := sc
		other.ID, other.Name = uuid.NewString(), "Second"
		if err := repo.CreateScene(ctx, c1, dir, other); err != nil {
			t.Fatal(err)
		}
		other.Name = "THE EYE OPENS"
		if err := repo.UpdateScene(ctx, c1, other); code(err) != 409 {
			t.Errorf("renaming onto a taken name: %v", err)
		}
		other.ID = uuid.NewString()
		if err := repo.UpdateScene(ctx, c1, other); code(err) != 404 {
			t.Errorf("updating a missing scene: %v", err)
		}
	})

	now := time.Now().UTC().Truncate(time.Second)
	sh := Showing{ID: uuid.NewString(), CampaignID: c1, SceneID: sc.ID, Scene: sc, ShownBy: dir, ShownAt: now, RewatchUntil: now.Add(12 * time.Hour),
		Viewers: []ShowingViewer{{UserID: ana}, {UserID: bo}}}
	if err := repo.CreateShowing(ctx, sh); err != nil {
		t.Fatal(err)
	}

	t.Run("a viewer sees only their own showing, inside the window", func(t *testing.T) {
		mine, err := repo.ViewerShowings(ctx, c1, ana, now)
		if err != nil || len(mine) != 1 || len(mine[0].Viewers) != 1 || mine[0].Viewers[0].UserID != ana {
			t.Fatalf("ana's = %+v, %v", mine, err)
		}
		if l, _ := repo.ViewerShowings(ctx, c1, dir, now); len(l) != 0 {
			t.Errorf("a non-viewer's list = %+v", l)
		}
		if l, _ := repo.ViewerShowings(ctx, c2, ana, now); len(l) != 0 {
			t.Errorf("another campaign's list = %+v", l)
		}
		if l, _ := repo.ViewerShowings(ctx, c1, ana, now.Add(13*time.Hour)); len(l) != 0 {
			t.Errorf("after the window = %+v", l)
		}
	})

	t.Run("watched and stopped are stamped once", func(t *testing.T) {
		if err := repo.MarkWatched(ctx, c1, sh.ID, ana, now.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
		if err := repo.MarkWatched(ctx, c1, sh.ID, ana, now.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		if err := repo.MarkWatched(ctx, c2, sh.ID, bo, now); err != nil {
			t.Fatal(err)
		}
		got, err := repo.GetShowing(ctx, c1, sh.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range got.Viewers {
			switch v.UserID {
			case ana:
				if v.WatchedAt == nil || !v.WatchedAt.Equal(now.Add(time.Minute)) {
					t.Errorf("ana watched at %v", v.WatchedAt)
				}
			case bo:
				if v.WatchedAt != nil {
					t.Errorf("a mark through another campaign reached bo: %v", v.WatchedAt)
				}
			}
		}
		if err := repo.StopShowing(ctx, c1, sh.ID, now.Add(2*time.Minute)); err != nil {
			t.Fatal(err)
		}
		_ = repo.StopShowing(ctx, c1, sh.ID, now.Add(3*time.Minute))
		got, _ = repo.GetShowing(ctx, c1, sh.ID)
		if got.StoppedAt == nil || !got.StoppedAt.Equal(now.Add(2*time.Minute)) {
			t.Errorf("stopped at %v", got.StoppedAt)
		}
		if _, err := repo.GetShowing(ctx, c2, sh.ID); code(err) != 404 {
			t.Errorf("another campaign read the showing: %v", err)
		}
		if r, _ := repo.RecentShowings(ctx, c1, 10); len(r) != 1 || len(r[0].Viewers) != 2 {
			t.Errorf("recent = %+v", r)
		}
	})

	t.Run("a showing keeps its snapshot after the scene is deleted", func(t *testing.T) {
		if err := repo.DeleteScene(ctx, c1, sc.ID); err != nil {
			t.Fatal(err)
		}
		got, err := repo.GetShowing(ctx, c1, sh.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.SceneID != "" || got.Scene.Name != "The Eye Opens" || len(got.Scene.Overlays) != 1 {
			t.Errorf("after delete = %+v", got)
		}
	})

	t.Run("deleting a campaign takes everything along", func(t *testing.T) {
		if _, err := db.Exec(`DELETE FROM campaigns WHERE id = ?`, c1); err != nil {
			t.Fatal(err)
		}
		for _, q := range []string{
			`SELECT COUNT(*) FROM cutscenes WHERE campaign_id = ?`,
			`SELECT COUNT(*) FROM cutscene_showings WHERE campaign_id = ?`,
		} {
			var n int
			if err := db.QueryRow(q, c1).Scan(&n); err != nil || n != 0 {
				t.Errorf("%s = %d, %v", q, n, err)
			}
		}
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM cutscene_showing_viewers WHERE showing_id = ?`, sh.ID).Scan(&n); err != nil || n != 0 {
			t.Errorf("viewers left = %d, %v", n, err)
		}
	})
}
