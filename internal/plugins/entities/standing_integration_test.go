package entities

import (
	"context"
	"database/sql"
	"io/fs"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/database/dbtest"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// openStandingDB is a scratch schema with the core migrations and this
// plugin's own standing migrations applied. Strict: a migration that does not
// apply must fail here, not skip.
func openStandingDB(t *testing.T) *sql.DB {
	t.Helper()
	sub, err := fs.Sub(MigrationsFS, database.PluginMigrationsSubdir)
	if err != nil {
		t.Fatalf("sub-FS: %v", err)
	}
	return dbtest.Open(t, dbtest.Options{
		Prefix:           "chronicle_stand_",
		StrictMigrations: true,
		Plugins:          []dbtest.PluginMigrations{{Slug: StandingPluginSlug, FS: sub}},
	})
}

// TestStandingRepository_Integration runs the repository and the service over
// real tables: the unique key that lets the party share a key space with
// characters, the cap, and above all the SQL filter that keeps a DM-only reason
// from a player's read.
func TestStandingRepository_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openStandingDB(t)
	ctx := context.Background()

	userID, camp := testUUID(t), testUUID(t)
	mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, "st-"+userID+"@example.test", "Standing Test", "x")
	mustExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`, camp, "Standing", "st-"+camp[:8], userID)

	types := NewEntityTypeRepository(db)
	typ := &EntityType{CampaignID: camp, Slug: "org", Name: "Org", NamePlural: "Orgs", Icon: "fa-user", Color: "#112233", Enabled: true}
	mustCreate(t, types, ctx, typ)
	page := func(name string, playable bool) string {
		id := testUUID(t)
		mustExec(t, db, `INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, search_text, created_by, playable, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW())`, id, camp, typ.ID, name, "p-"+id[:8], name, userID, playable)
		return id
	}
	guild, otherGuild := page("The Lantern Guild", false), page("The Tide Guild", false)
	mira, tobin := page("Mira", true), page("Tobin", true)

	repo := NewStandingRepository(db)
	svc := NewStandingService(repo, fakeGate{})
	dm := func(subject, dir, text string, dmOnly bool) {
		t.Helper()
		lvl := 1
		if _, err := svc.AddReason(ctx, AddStandingReasonInput{CampaignID: camp, EntityID: guild, Subject: subject, Direction: dir, Text: text, DMOnly: dmOnly, Level: &lvl,
			Role: permissions.RoleOwner, UserID: userID}); err != nil {
			t.Fatalf("AddReason(%s): %v", text, err)
		}
	}
	dm("party", "up", "Saved the granary", false)
	dm("party", "down", "Secretly paid off by the Duke", true)
	dm(mira, "up", "Vouched for an apprentice", false)
	// Two writes can land in the same millisecond; order them on purpose.
	mustExec(t, db, `UPDATE entity_standing_reasons SET created_at = created_at - INTERVAL 1 MINUTE WHERE reason = 'Saved the granary'`)

	t.Run("the migration can run again without error", func(t *testing.T) {
		up, err := fs.ReadFile(MigrationsFS, "migrations/001_entity_standing.up.sql")
		if err != nil {
			t.Fatal(err)
		}
		for _, stmt := range strings.Split(string(up), ";\n") {
			if strings.TrimSpace(stripSQLComments(stmt)) == "" {
				continue
			}
			if _, err := db.Exec(stmt); err != nil {
				t.Fatalf("re-running the up migration: %v", err)
			}
		}
	})

	t.Run("the party and a character are separate standings of one page", func(t *testing.T) {
		rows, err := repo.List(ctx, camp, guild)
		if err != nil || len(rows) != 2 {
			t.Fatalf("List = %d rows, %v; want 2", len(rows), err)
		}
		if p, _ := repo.Find(ctx, guild, StandingPartyKey); p == nil || p.Level != 1 {
			t.Errorf("party standing = %+v", p)
		}
		if p, _ := repo.Find(ctx, otherGuild, StandingPartyKey); p != nil {
			t.Error("another page got this page's standing")
		}
	})

	t.Run("a player's read never selects a DM-only reason", func(t *testing.T) {
		party, _ := repo.Find(ctx, guild, StandingPartyKey)
		for _, c := range []struct {
			includeDM bool
			wantN     int
		}{{false, 1}, {true, 2}} {
			rs, counts, err := repo.ReasonsFor(ctx, []string{party.ID}, c.includeDM, 10)
			if err != nil {
				t.Fatal(err)
			}
			if len(rs[party.ID]) != c.wantN || counts[party.ID] != c.wantN {
				t.Errorf("includeDMOnly=%v: %d reasons, count %d; want %d", c.includeDM, len(rs[party.ID]), counts[party.ID], c.wantN)
			}
			for _, r := range rs[party.ID] {
				if r.DMOnly && !c.includeDM {
					t.Error("a DM-only reason came back for a player")
				}
			}
		}
		pv, err := svc.View(ctx, camp, guild, permissions.RolePlayer, "p1")
		if err != nil || pv.Hidden {
			t.Fatalf("player View = %+v, %v", pv, err)
		}
		for _, s := range pv.Subjects {
			for _, r := range s.Reasons {
				if r.DMOnly || r.Text == "Secretly paid off by the Duke" {
					t.Errorf("player got %+v", r)
				}
			}
		}
	})

	t.Run("newest first, and the per-subject cap leaves the total whole", func(t *testing.T) {
		party, _ := repo.Find(ctx, guild, StandingPartyKey)
		rs, counts, _ := repo.ReasonsFor(ctx, []string{party.ID}, true, 1)
		if len(rs[party.ID]) != 1 || rs[party.ID][0].Text != "Secretly paid off by the Duke" || counts[party.ID] != 2 {
			t.Errorf("got %+v counts %v; want the newest alone with the whole count", rs[party.ID], counts)
		}
	})

	t.Run("a reason cannot be removed through another page or subject", func(t *testing.T) {
		party, _ := repo.Find(ctx, guild, StandingPartyKey)
		rs, _, _ := repo.ReasonsFor(ctx, []string{party.ID}, true, 10)
		id := rs[party.ID][0].ID
		if ok, _ := repo.DeleteReason(ctx, otherGuild, StandingPartyKey, id); ok {
			t.Error("deleted through another page")
		}
		if ok, _ := repo.DeleteReason(ctx, guild, mira, id); ok {
			t.Error("deleted through another subject")
		}
		if ok, _ := repo.DeleteReason(ctx, guild, StandingPartyKey, id); !ok {
			t.Error("could not delete through its own page and subject")
		}
	})

	t.Run("the cap is enforced in the transaction", func(t *testing.T) {
		for i := 0; i < MaxStandingReasons; i++ {
			err := repo.SaveReason(ctx, StandingReasonWrite{CampaignID: camp, EntityID: otherGuild, SubjectKey: StandingPartyKey, Level: 2,
				Reason: StandingReason{ID: uuid.NewString(), Direction: StandingNone, Text: "filler", When: ""}})
			if err != nil {
				t.Fatalf("reason %d: %v", i, err)
			}
		}
		err := repo.SaveReason(ctx, StandingReasonWrite{CampaignID: camp, EntityID: otherGuild, SubjectKey: StandingPartyKey, Level: 2,
			Reason: StandingReason{ID: uuid.NewString(), Direction: StandingNone, Text: "one too many"}})
		if err != errStandingFull {
			t.Errorf("err = %v, want errStandingFull", err)
		}
	})

	t.Run("names and characters skip a page in the Trash", func(t *testing.T) {
		names, _ := repo.Names(ctx, camp, []string{mira, tobin})
		if len(names) != 2 {
			t.Fatalf("names = %v", names)
		}
		mustExec(t, db, `UPDATE entities SET deleted_at = NOW() WHERE id = ?`, tobin)
		names, _ = repo.Names(ctx, camp, []string{mira, tobin})
		if _, ok := names[tobin]; ok || len(names) != 1 {
			t.Errorf("names = %v; the trashed page should be gone", names)
		}
		chars, _ := repo.Characters(ctx, camp, 10)
		if len(chars) != 1 || chars[0].Key != mira {
			t.Errorf("characters = %v; want only Mira", chars)
		}
	})

	t.Run("the switch defaults on, and deleting the page removes its standings", func(t *testing.T) {
		if see, _ := repo.PlayersSee(ctx, guild); !see {
			t.Error("a page with no setting should let players see")
		}
		if err := repo.SetPlayersSee(ctx, camp, guild, userID, false); err != nil {
			t.Fatal(err)
		}
		if see, _ := repo.PlayersSee(ctx, guild); see {
			t.Error("the switch did not turn off")
		}
		mustExec(t, db, `DELETE FROM entities WHERE id = ?`, guild)
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM entity_standings WHERE entity_id = ?`, guild).Scan(&n); err != nil || n != 0 {
			t.Errorf("standings left after the page was deleted: %d, %v", n, err)
		}
		if err := db.QueryRow(`SELECT COUNT(*) FROM entity_standing_reasons`).Scan(&n); err != nil {
			t.Fatal(err)
		}
	})
}

// stripSQLComments drops whole-line "--" comments so a chunk that is only a
// comment is recognised as empty.
func stripSQLComments(s string) string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(l), "--") {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}
