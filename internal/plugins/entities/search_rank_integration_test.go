package entities

import (
	"context"
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// Search lists the page a searcher most likely means first: its first row is
// the one Enter opens in quick search and the one a mention picks. Each case
// names its pages so that sorting by name alone would give a different order,
// and runs against a real schema because the ranking is one ORDER BY that
// leans on the name's collation and FULLTEXT index.
func TestSearchRanksNameMatchesFirst_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	userID, camp := testUUID(t), testUUID(t)
	mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, "rank-"+userID+"@example.test", "Rank Test", "x")
	mustExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`, camp, "Rank", "rank-"+camp[:8], userID)
	defer func() {
		mustExec(t, db, `DELETE FROM campaigns WHERE id = ?`, camp)
		mustExec(t, db, `DELETE FROM users WHERE id = ?`, userID)
	}()

	types := NewEntityTypeRepository(db)
	typ := &EntityType{CampaignID: camp, Slug: "thing", Name: "Thing", NamePlural: "Things", Icon: "fa-circle", Color: "#112233", Enabled: true}
	mustCreate(t, types, ctx, typ)

	// page makes one page; text is what its entry and fields say, which is
	// where a passing mention of another page's name lives.
	page := func(name, text string) string {
		id := testUUID(t)
		mustExec(t, db, `INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, search_text, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, NOW(), NOW())`, id, camp, typ.ID, name, "p-"+id[:8], name+" "+text, userID)
		return id
	}
	page("Thorne", "")
	page("Thorne's Rest", "an inn")
	page("Captain Aldous Thorne", "a salvage captain")
	oldSalt := page("Old Salt", "a deckhand")
	mustExec(t, db, `INSERT INTO entity_aliases (entity_id, alias) VALUES (?, ?)`, oldSalt, "Thorne's mate")
	page("Act two outline", "The party meets Captain Thorne at the quay.")
	page("Eel", "")
	page("Eel Market", "")
	page("The Gilded Eel", "a tavern")
	page("Steelyard", "")
	page("Wharf", "where the eel boats tie up")

	repo := NewEntityRepository(db)
	names := func(es []Entity) []string {
		out := make([]string, len(es))
		for i, e := range es {
			out[i] = e.Name
		}
		return out
	}

	cases := []struct {
		name, query string
		want        []string
	}{
		{
			name:  "exact name, name start, a word in the name, an alias, then a passing mention",
			query: "thorne",
			want:  []string{"Thorne", "Thorne's Rest", "Captain Aldous Thorne", "Old Salt", "Act two outline"},
		},
		{
			name:  "a short query ranks the same way, with a name that merely contains it after the word matches",
			query: "eel",
			want:  []string{"Eel", "Eel Market", "The Gilded Eel", "Steelyard", "Wharf"},
		},
		{
			name:  "several words match a name word by word, and those names still beat a passing mention",
			query: "captain thorne",
			want:  []string{"Captain Aldous Thorne", "Thorne", "Thorne's Rest", "Act two outline"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, total, err := repo.Search(ctx, camp, tc.query, nil, permissions.RoleOwner, userID, DefaultListOptions())
			if err != nil {
				t.Fatal(err)
			}
			if total != len(tc.want) {
				t.Errorf("total = %d, want %d", total, len(tc.want))
			}
			if !reflect.DeepEqual(names(got), tc.want) {
				t.Errorf("order for %q:\n got  %q\n want %q", tc.query, names(got), tc.want)
			}
		})
	}

	// A paged walk has to hand out the same order a row at a time, each row
	// once: the rank leads the sort and the id still ends it.
	t.Run("paging walks the ranked order without repeats", func(t *testing.T) {
		var walked []string
		for p := 1; p <= 3; p++ {
			got, _, err := repo.Search(ctx, camp, "thorne", nil, permissions.RoleOwner, userID, ListOptions{Page: p, PerPage: 2})
			if err != nil {
				t.Fatal(err)
			}
			walked = append(walked, names(got)...)
		}
		want := []string{"Thorne", "Thorne's Rest", "Captain Aldous Thorne", "Old Salt", "Act two outline"}
		if !reflect.DeepEqual(walked, want) {
			t.Errorf("paged walk:\n got  %q\n want %q", walked, want)
		}
	})
}

// Whatever a searcher types is searched for, never read as search syntax: a
// query of four or more characters goes to the FULLTEXT boolean parser, which
// rejects a stray operator outright. Runs against a real schema because only
// the real parser says what it rejects.
func TestSearchTakesAnyTypedCharacters_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	userID, camp := testUUID(t), testUUID(t)
	mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, "typed-"+userID+"@example.test", "Typed Test", "x")
	mustExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`, camp, "Typed", "typed-"+camp[:8], userID)
	defer func() {
		mustExec(t, db, `DELETE FROM campaigns WHERE id = ?`, camp)
		mustExec(t, db, `DELETE FROM users WHERE id = ?`, userID)
	}()

	types := NewEntityTypeRepository(db)
	typ := &EntityType{CampaignID: camp, Slug: "thing", Name: "Thing", NamePlural: "Things", Icon: "fa-circle", Color: "#112233", Enabled: true}
	mustCreate(t, types, ctx, typ)
	page := func(name, text string) {
		id := testUUID(t)
		mustExec(t, db, `INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, search_text, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, NOW(), NOW())`, id, camp, typ.ID, name, "p-"+id[:8], name+" "+text, userID)
	}
	page("Half-Elf Ranger", "")
	page("Notes for Kara", "write to kara@example.test")
	page("Dash ---- Row", "")
	page("Steelyard", "")

	repo := NewEntityRepository(db)
	cases := []struct {
		name, query string
		want        []string
	}{
		{"an @ between words", "Not @Notes", []string{"Notes for Kara"}},
		{"an @ on its own", "Notes @", []string{"Notes for Kara"}},
		{"an address", "kara@example.test", []string{"Notes for Kara"}},
		{"a hyphenated name is found by its words", "Half-Elf", []string{"Half-Elf Ranger"}},
		{"quotes, brackets and the rest", `"(Half)" ~<Elf>*`, []string{"Half-Elf Ranger"}},
		{"nothing but operators is matched as a run", "----", []string{"Dash ---- Row"}},
		{"nothing but operators, matching nothing", "@@@@", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, total, err := repo.Search(ctx, camp, tc.query, nil, permissions.RoleOwner, userID, DefaultListOptions())
			if err != nil {
				t.Fatalf("Search(%q): %v", tc.query, err)
			}
			var names []string
			for _, e := range got {
				names = append(names, e.Name)
			}
			if total != len(tc.want) || !reflect.DeepEqual(names, tc.want) {
				t.Errorf("Search(%q) = %q (total %d), want %q", tc.query, names, total, tc.want)
			}
		})
	}
}
