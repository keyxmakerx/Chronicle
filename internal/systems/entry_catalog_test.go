package systems

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// readingManifest is a system with two kinds (ancestry, class), a rules
// category that is not a kind, and a Directors-only category.
func readingManifest() *SystemManifest {
	return &SystemManifest{ID: "sys", Name: "Draw Steel",
		Categories: []CategoryDef{{Slug: "ancestries"}, {Slug: "classes"}, {Slug: "abilities"}, {Slug: "monsters", Director: true}},
		EntityPresets: []EntityPresetDef{{Slug: "hero", Category: "character", Fields: []FieldDef{
			{Key: "ancestry", Type: "string"}, {Key: "class", Type: "string"}, {Key: "level", Type: "number"}}}},
	}
}

func readingData() fakeData {
	trait := func(name, text string, cost float64) map[string]any {
		t := map[string]any{"name": name, "description": text}
		if cost > 0 {
			t["cost"] = cost
		}
		return t
	}
	return fakeData{
		"ancestries": {
			// The Dwarf carries what only a package can hold: an object, an
			// extra key on a trait, a generated twin and a twin with no base.
			{ID: "dwarf", Name: "Dwarf", Summary: "Stone folk.", Description: "Dense and sturdy.", Properties: map[string]any{
				"speed": 5.0, "ancestry_points": 3.0,
				"quick_build":              []any{"Grounded", "Stone Skin"},
				"breath":                   map[string]any{"damage": "2d6", "keywords": []any{"Area"}},
				"signature_traits":         []any{trait("Grounded", "Hard to move.", 0)},
				"signature_traits_display": "Grounded — Hard to move.",
				"provenance_display":       "Printed in the core rules.",
				"purchased_traits": []any{map[string]any{"name": "Stone Skin", "description": "Tough.", "cost": 1.0,
					"ability": map[string]any{"type": "triggered", "distance": "self"}}}}},
			{ID: "elf", Name: "Elf", Summary: strings.Repeat("Old folk. ", 30), Properties: map[string]any{"speed": 6.0}},
			{ID: "polder", Name: "Polder", Summary: "Small folk.", Properties: map[string]any{
				"purchased_traits": []any{trait("Corruption Immunity", "Unbothered.", 1)}}},
			// One text detail longer than a campaign entry can hold: the
			// devil cannot be rewritten without loss.
			{ID: "devil", Name: "Devil", Properties: map[string]any{"quick_build": []any{"Barbed Tail"}, "lore": strings.Repeat("x", maxEntryPropText+1)}},
		},
		// The censor would copy cleanly; classes are refused by kind.
		"classes": {{ID: "fury", Name: "Fury", Properties: map[string]any{"subclasses": []any{"Berserker", "Reaver"}}},
			{ID: "censor", Name: "Censor", Properties: map[string]any{"heroic_resource": "Wrath"}}},
		"abilities": {{ID: "strike", Name: "Strike"}},
		"monsters":  {{ID: "goblin", Name: "Goblin"}},
	}
}

// newReadingSvc is Therin: Ashborn (its own), a second Dwarf beside the
// package's, Tideling (Directors only) and Fairy (kept to Wren, whom tyne
// plays), over a package of four ancestries.
func newReadingSvc(t *testing.T) (*systemEntryService, *fakeEntryRepo, fakeData) {
	t.Helper()
	m := readingManifest()
	data := readingData()
	repo := &fakeEntryRepo{}
	svc := NewSystemEntryService(repo, func(context.Context, string) *SystemManifest { return m },
		WithEntryCharacters(testCharacters),
		WithStandardSystem(func(context.Context, string) System { return fakeSystem{m: m, data: data} }),
	).(*systemEntryService)
	ctx := context.Background()
	for _, in := range []CreateSystemEntryInput{
		{FieldKey: "ancestry", Name: "Ashborn", Description: `<p>Fire folk.</p><span data-secret="true">Cursed.</span>`},
		{FieldKey: "ancestry", Name: "Dwarf", Summary: "Sea dwarves."},
		{FieldKey: "ancestry", Name: "Tideling", Visibility: EntryVisibilityDirectors},
		{FieldKey: "ancestry", Name: "Fairy", Visibility: EntryVisibilityCharacter, ForEntityID: "wren"},
	} {
		if _, err := svc.Create(ctx, "c1", dirActor, in); err != nil {
			t.Fatal(err)
		}
	}
	return svc, repo, data
}

var (
	tyneActor = EntryActor{UserID: "tyne"} // plays Wren
	stranger  = EntryActor{}
)

// rows renders a catalog as "Name/origin" words, with "*" on a printed one.
func rows(l *CatalogList) string {
	var out []string
	for _, e := range l.Entries {
		w := e.Name + "/" + e.Origin
		if e.Printed {
			w += "*"
		}
		out = append(out, w)
	}
	return strings.Join(out, ",")
}

func entryByName(t *testing.T, repo *fakeEntryRepo, name string) SystemEntry {
	t.Helper()
	for _, r := range repo.rows {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("no entry %s", name)
	return SystemEntry{}
}

// Untied data: both Dwarfs are listed, the package's at ?printed=1 with its
// credit, and each reader sees exactly what visibility allows.
func TestCatalog_PerReaderUntied(t *testing.T) {
	svc, _, _ := newReadingSvc(t)
	ctx := context.Background()
	common := "Ashborn/campaign,Devil/standard,Dwarf/campaign,Dwarf/standard*,Elf/standard"
	tests := []struct {
		name   string
		reader EntryActor
		want   string
	}{
		{"director", dirActor, common + ",Fairy/campaign,Polder/standard,Tideling/campaign"},
		{"a player", playerActor, common + ",Polder/standard"},
		{"the player of Wren", tyneActor, common + ",Fairy/campaign,Polder/standard"},
		{"a stranger", stranger, common + ",Polder/standard"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, err := svc.Catalog(ctx, "c1", "ancestries", tt.reader)
			if err != nil {
				t.Fatal(err)
			}
			if got := rows(l); got != tt.want {
				t.Fatalf("catalog = %s\nwant      %s", got, tt.want)
			}
			for _, e := range l.Entries {
				if e.Origin == OriginStandard && e.Credit != "Draw Steel" {
					t.Fatalf("standard entry without credit: %+v", e)
				}
				if e.Name == "Ashborn" && strings.Contains(e.Description, "Cursed") != tt.reader.IsDirector {
					t.Fatalf("secret span for %s: %q", tt.name, e.Description)
				}
				if e.Name == "Tideling" && !e.DirectorsOnly {
					t.Fatal("Tideling is Directors only")
				}
			}
		})
	}
}

// A tie replaces the standard entry for exactly the readers who may see the
// stand-in: a draft is the Director's alone, "everyone" is everyone's, and
// an entry kept to Wren replaces it for Wren's player only.
func TestCatalog_RewriteDecidedPerReader(t *testing.T) {
	svc, _, _ := newReadingSvc(t)
	ctx := context.Background()
	elf, err := svc.Rewrite(ctx, "c1", dirActor, RewriteInput{FieldKey: "ancestry", Slug: "elf"})
	if err != nil {
		t.Fatal(err)
	}
	if elf.Visibility != EntryVisibilityDirectors || elf.StandsFor != "elf" || elf.StandsForSeen == "" {
		t.Fatalf("a rewrite starts as the Director's tied draft: %+v", elf)
	}
	elfOrigin := func(r EntryActor) string {
		l, err := svc.Catalog(ctx, "c1", "ancestries", r)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, e := range l.Entries {
			if e.Name == "Elf" {
				got = append(got, e.Origin)
			}
		}
		return strings.Join(got, ",")
	}
	steps := []struct {
		name  string
		patch UpdateSystemEntryInput
		want  map[string]string // reader -> origins of the Elf rows
	}{
		{"draft", UpdateSystemEntryInput{}, map[string]string{"dm": "rewritten", "pl": "standard", "tyne": "standard", "": "standard"}},
		{"everyone", UpdateSystemEntryInput{Visibility: patch.Of(EntryVisibilityEveryone)},
			map[string]string{"dm": "rewritten", "pl": "rewritten", "tyne": "rewritten", "": "rewritten"}},
		{"kept to Wren", UpdateSystemEntryInput{Visibility: patch.Of(EntryVisibilityCharacter), ForEntityID: patch.Of("wren")},
			map[string]string{"dm": "rewritten", "pl": "standard", "tyne": "rewritten", "": "standard"}},
	}
	readers := map[string]EntryActor{"dm": dirActor, "pl": playerActor, "tyne": tyneActor, "": stranger}
	for _, st := range steps {
		if st.name != "draft" {
			if _, err := svc.Update(ctx, "c1", dirActor, elf.ID, st.patch); err != nil {
				t.Fatal(err)
			}
		}
		for who, want := range st.want {
			if got := elfOrigin(readers[who]); got != want {
				t.Errorf("%s, reader %q: Elf rows = %s, want %s", st.name, who, got, want)
			}
		}
	}
}

// The two Dwarfs: tying Therin's Dwarf to the package's leaves one Dwarf for
// every reader, the plain address is Therin's, and only the Director can
// read the printed one.
func TestCatalog_TwoDwarfsTied(t *testing.T) {
	svc, repo, _ := newReadingSvc(t)
	ctx := context.Background()
	d := entryByName(t, repo, "Dwarf")
	if _, err := svc.Tie(ctx, "c1", dirActor, d.ID, "dwarf"); err != nil {
		t.Fatal(err)
	}
	for _, r := range []EntryActor{dirActor, playerActor, tyneActor, stranger} {
		l, _ := svc.Catalog(ctx, "c1", "ancestries", r)
		if n := strings.Count(rows(l), "Dwarf/"); n != 1 || !strings.Contains(rows(l), "Dwarf/rewritten") {
			t.Fatalf("reader %+v: %s", r, rows(l))
		}
		pick, _ := svc.PickList(WithChoiceViewerUser(WithChoiceViewer(ctx, r.IsDirector), r.UserID), "c1", "sys", "ancestry", packageChoices())
		dwarfs := 0
		for _, c := range pick {
			if c.Name == "Dwarf" {
				dwarfs++
				if c.Origin != OriginRewritten || c.StandsFor != "dwarf" || c.Source != ChoiceSourceCampaign {
					t.Fatalf("pick row: %+v", c)
				}
			}
		}
		if dwarfs != 1 {
			t.Fatalf("reader %+v has %d Dwarfs in the pick list", r, dwarfs)
		}
	}
	tests := []struct {
		name     string
		reader   EntryActor
		printed  bool
		wantKind string
		wantFrom string
	}{
		{"director plain", dirActor, false, ResolvedEntry, OriginRewritten},
		{"director printed", dirActor, true, ResolvedPrinted, OriginStandard},
		{"player plain", playerActor, false, ResolvedEntry, OriginRewritten},
		{"player printed answers as if not asked", playerActor, true, ResolvedEntry, OriginRewritten},
		{"stranger printed", stranger, true, ResolvedEntry, OriginRewritten},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := svc.ResolveAddress(ctx, "c1", "ancestries", "dwarf", tt.printed, tt.reader)
			if err != nil {
				t.Fatal(err)
			}
			if r.Kind != tt.wantKind || r.Entry == nil || r.Entry.Origin != tt.wantFrom {
				t.Fatalf("got %+v", r)
			}
		})
	}
}

// packageChoices is the package's ancestry pick list as the choice file
// gives it.
func packageChoices() []Choice {
	var out []Choice
	for _, it := range readingData()["ancestries"] {
		out = append(out, Choice{Slug: it.ID, Name: it.Name, Summary: it.Summary, Source: ChoiceSourcePackage})
	}
	return out
}

// With nothing tied or left out, the pick list is what it was before ties
// existed: the package's rows in order, then the campaign's, both Dwarfs.
func TestPickList_UntiedUnchanged(t *testing.T) {
	svc, _, _ := newReadingSvc(t)
	ctx := context.Background()
	tests := []struct {
		name   string
		reader EntryActor
		want   string
	}{
		{"director", dirActor, "Dwarf,Elf,Polder,Devil,Ashborn,Dwarf,Tideling,Fairy"},
		{"player", playerActor, "Dwarf,Elf,Polder,Devil,Ashborn,Dwarf"},
		{"Wren's player", tyneActor, "Dwarf,Elf,Polder,Devil,Ashborn,Dwarf,Fairy"},
		{"unmarked context", stranger, "Dwarf,Elf,Polder,Devil,Ashborn,Dwarf"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := ctx
			if tt.reader != stranger {
				c = WithChoiceViewerUser(WithChoiceViewer(ctx, tt.reader.IsDirector), tt.reader.UserID)
			}
			pick, err := svc.PickList(c, "c1", "sys", "ancestry", packageChoices())
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, ch := range pick {
				names = append(names, ch.Name)
				if ch.Name == "Ashborn" && (ch.Source != ChoiceSourceCampaign || ch.Origin != OriginCampaign) {
					t.Fatalf("campaign row: %+v", ch)
				}
				if ch.Source == ChoiceSourcePackage && ch.Origin != OriginStandard {
					t.Fatalf("package row: %+v", ch)
				}
			}
			if got := strings.Join(names, ","); got != tt.want {
				t.Fatalf("pick = %s, want %s", got, tt.want)
			}
		})
	}
	// A list drawn from another system's file keeps every row.
	_, _ = svc.LeaveOut(ctx, "c1", dirActor, LeftOutInput{FieldKey: "ancestry", Slug: "polder"})
	other, _ := svc.PickList(WithChoiceViewer(ctx, true), "c1", "other", "ancestry", packageChoices())
	if len(other) != 8 {
		t.Fatalf("another system's list lost rows: %d", len(other))
	}
}

// Left out: gone from the offer for everyone, still answering by name and
// address with its content; lists end with only the ones with a sentence,
// and only Directors get the full left-out list.
func TestCatalog_LeftOut(t *testing.T) {
	svc, _, _ := newReadingSvc(t)
	ctx := context.Background()
	if _, err := svc.LeaveOut(ctx, "c1", dirActor, LeftOutInput{FieldKey: "ancestry", Slug: "polder", Sentence: "  The halflings drowned.  "}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.LeaveOut(ctx, "c1", dirActor, LeftOutInput{FieldKey: "ancestry", Slug: "devil"}); err != nil {
		t.Fatal(err)
	}
	for _, r := range []EntryActor{dirActor, playerActor, tyneActor, stranger} {
		l, _ := svc.Catalog(ctx, "c1", "ancestries", r)
		if strings.Contains(rows(l), "Polder") || strings.Contains(rows(l), "Devil") {
			t.Fatalf("left-out entry offered: %s", rows(l))
		}
		if len(l.LeftOutNoted) != 1 || l.LeftOutNoted[0].Name != "Polder" || l.LeftOutNoted[0].Sentence != "The halflings drowned." {
			t.Fatalf("noted left-out = %+v", l.LeftOutNoted)
		}
		if wantAll := 0; r.IsDirector {
			if len(l.LeftOut) != 2 {
				t.Fatalf("Director's left-out list = %+v", l.LeftOut)
			}
		} else if len(l.LeftOut) != wantAll {
			t.Fatalf("a player got the full left-out list: %+v", l.LeftOut)
		}
		pick, _ := svc.PickList(WithChoiceViewerUser(WithChoiceViewer(ctx, r.IsDirector), r.UserID), "c1", "sys", "ancestry", packageChoices())
		for _, c := range pick {
			if c.Name == "Polder" || c.Name == "Devil" {
				t.Fatalf("pick list offers a left-out entry: %+v", c)
			}
		}
	}
	for _, tt := range []struct {
		name, sentence string
		byName         bool
	}{{"Polder", "The halflings drowned.", true}, {"polder", "The halflings drowned.", false}, {"Devil", "", true}} {
		var r *Resolution
		var err error
		if tt.byName {
			r, err = svc.ResolveName(ctx, "c1", "ancestries", tt.name, playerActor)
		} else {
			r, err = svc.ResolveAddress(ctx, "c1", "ancestries", tt.name, false, playerActor)
		}
		if err != nil || r.Kind != ResolvedLeftOut || r.Entry == nil || r.Entry.Sentence != tt.sentence {
			t.Fatalf("%s: %+v %v", tt.name, r, err)
		}
		if r.Entry.Name == "Polder" && r.Entry.Properties["purchased_traits"] == nil {
			t.Fatal("a hero who is a Polder can no longer read its traits")
		}
	}
}

// Old names: a renamed entry is still found by the name a hero stored, and
// by the standard name it stands for, but never for a reader who may not
// see it.
func TestCatalog_ResolveNames(t *testing.T) {
	svc, repo, _ := newReadingSvc(t)
	ctx := context.Background()
	d := entryByName(t, repo, "Dwarf")
	if _, err := svc.Tie(ctx, "c1", dirActor, d.ID, "dwarf"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(ctx, "c1", dirActor, d.ID, UpdateSystemEntryInput{Name: patch.Of("Deepkin")}); err != nil {
		t.Fatal(err)
	}
	ash := entryByName(t, repo, "Ashborn")
	if _, err := svc.Update(ctx, "c1", dirActor, ash.ID, UpdateSystemEntryInput{Name: patch.Of("Emberkin")}); err != nil {
		t.Fatal(err)
	}
	tide := entryByName(t, repo, "Tideling")
	if _, err := svc.Update(ctx, "c1", dirActor, tide.ID, UpdateSystemEntryInput{Name: patch.Of("Sea Folk")}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		reader   EntryActor
		query    string
		wantKind string
		wantSlug string
		wantVia  string
	}{
		{"standard name to the renamed stand-in", playerActor, "dwarf", ResolvedRedirect, "dwarf", ViaRewritten},
		{"current name", playerActor, "Deepkin", ResolvedEntry, "", ""},
		{"old name", playerActor, "ASHBORN", ResolvedRedirect, "ashborn", ViaOldName},
		{"old name of a hidden entry for a player", playerActor, "Tideling", ResolvedNotFound, "", ""},
		{"old name of a hidden entry for a stranger", stranger, "Tideling", ResolvedNotFound, "", ""},
		{"old name of a hidden entry for the Director", dirActor, "Tideling", ResolvedRedirect, "tideling", ViaOldName},
		{"untouched standard", playerActor, "Elf", ResolvedEntry, "", ""},
		{"unknown", playerActor, "Gnome", ResolvedNotFound, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := svc.ResolveName(ctx, "c1", "ancestries", tt.query, tt.reader)
			if err != nil {
				t.Fatal(err)
			}
			if r.Kind != tt.wantKind || r.Slug != tt.wantSlug || r.Via != tt.wantVia {
				t.Fatalf("got %+v", r)
			}
			if r.Kind == ResolvedRedirect && (r.Entry == nil || r.Entry.Slug != r.Slug) {
				t.Fatalf("a redirect carries its target: %+v", r)
			}
		})
	}
}

// An old slug that no entry holds any more leads to the renamed entry, for a
// reader who may see it.
func TestCatalog_OldSlug(t *testing.T) {
	svc, repo, _ := newReadingSvc(t)
	ctx := context.Background()
	repo.nextID++
	repo.rows = append(repo.rows, SystemEntry{ID: repo.nextID, CampaignID: "c1", SystemID: "sys", FieldKey: "ancestry",
		Slug: "x", Name: "New Name", Visibility: EntryVisibilityDirectors, Status: EntryStatusAccepted, Properties: map[string]any{}})
	repo.oldNames = append(repo.oldNames, EntryOldName{ID: 999, CampaignID: "c1", SystemID: "sys", FieldKey: "ancestry", Name: "Old Name", EntryID: repo.nextID})
	r, _ := svc.ResolveAddress(ctx, "c1", "ancestries", "old-name", false, dirActor)
	if r.Kind != ResolvedRedirect || r.Slug != "x" || r.Via != ViaOldSlug {
		t.Fatalf("director: %+v", r)
	}
	r, _ = svc.ResolveAddress(ctx, "c1", "ancestries", "old-name", false, playerActor)
	if r.Kind != ResolvedNotFound {
		t.Fatalf("player reached a Directors-only entry by its old slug: %+v", r)
	}
}

// A player can never get the printed original of an entry they read
// rewritten, nor a Directors-only stand-in, through any path.
func TestCatalog_PlayerNeverGetsPrintedOrDraft(t *testing.T) {
	svc, _, _ := newReadingSvc(t)
	ctx := context.Background()
	// Elf is rewritten for everyone; Dwarf's stand-in is the Director's draft.
	elf, err := svc.Rewrite(ctx, "c1", dirActor, RewriteInput{FieldKey: "ancestry", Slug: "elf", Name: "Elf"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(ctx, "c1", dirActor, elf.ID, UpdateSystemEntryInput{
		Visibility: patch.Of(EntryVisibilityEveryone), Description: patch.Of("Therin's elves.")}); err != nil {
		t.Fatal(err)
	}
	draft, err := svc.Rewrite(ctx, "c1", dirActor, RewriteInput{FieldKey: "ancestry", Slug: "dwarf", Name: "Mountain Dwarf"})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range []EntryActor{playerActor, tyneActor, stranger} {
		var got []*CatalogEntry
		var kinds []string
		for _, printed := range []bool{false, true} {
			for _, slug := range []string{"elf", "dwarf", draft.Slug} {
				res, err := svc.ResolveAddress(ctx, "c1", "ancestries", slug, printed, r)
				if err != nil {
					t.Fatal(err)
				}
				kinds = append(kinds, res.Kind)
				got = append(got, res.Entry)
			}
		}
		for _, name := range []string{"Elf", "Dwarf", "Mountain Dwarf"} {
			res, _ := svc.ResolveName(ctx, "c1", "ancestries", name, r)
			kinds = append(kinds, res.Kind)
			got = append(got, res.Entry)
		}
		l, _ := svc.Catalog(ctx, "c1", "ancestries", r)
		for i := range l.Entries {
			got = append(got, &l.Entries[i])
		}
		for _, k := range kinds {
			if k == ResolvedPrinted {
				t.Fatalf("reader %+v got a printed original", r)
			}
		}
		for _, e := range got {
			if e == nil {
				continue
			}
			if e.DirectorsOnly || e.Name == "Mountain Dwarf" {
				t.Fatalf("reader %+v got the Director's draft: %+v", r, e)
			}
			if e.Slug == "elf" && e.Origin == OriginStandard {
				t.Fatalf("reader %+v got the printed Elf: %+v", r, e)
			}
		}
		pick, _ := svc.PickList(WithChoiceViewerUser(WithChoiceViewer(ctx, false), r.UserID), "c1", "sys", "ancestry", packageChoices())
		for _, c := range pick {
			if c.Name == "Mountain Dwarf" || (c.Slug == "elf" && c.Source == ChoiceSourcePackage) {
				t.Fatalf("pick list for %+v: %+v", r, c)
			}
		}
	}
	// The Director reads the printed Elf and the plain Dwarf's draft.
	res, _ := svc.ResolveAddress(ctx, "c1", "ancestries", "elf", true, dirActor)
	if res.Kind != ResolvedPrinted || res.Entry.Origin != OriginStandard {
		t.Fatalf("director printed: %+v", res)
	}
	res, _ = svc.ResolveAddress(ctx, "c1", "ancestries", "dwarf", false, dirActor)
	if res.Kind != ResolvedEntry || res.Entry.Origin != OriginCampaign || res.Entry.Name != "Dwarf" {
		// Therin's untied Dwarf holds the plain address; the draft that
		// stands for the package's Dwarf has its own slug.
		t.Fatalf("director plain dwarf: %+v", res)
	}
	res, _ = svc.ResolveName(ctx, "c1", "ancestries", "Elf", dirActor)
	if res.Kind != ResolvedEntry || res.Entry.Origin != OriginRewritten {
		t.Fatalf("director Elf by name: %+v", res)
	}
}

// Categories that are not kinds have no campaign overlay; Directors-only
// categories are empty and unresolvable for anyone else.
func TestCatalog_OtherCategories(t *testing.T) {
	svc, _, _ := newReadingSvc(t)
	ctx := context.Background()
	l, err := svc.Catalog(ctx, "c1", "abilities", playerActor)
	if err != nil || rows(l) != "Strike/standard" || l.FieldKey != "" {
		t.Fatalf("abilities = %s %v", rows(l), err)
	}
	r, _ := svc.ResolveName(ctx, "c1", "abilities", "strike", stranger)
	if r.Kind != ResolvedEntry {
		t.Fatalf("ability by name: %+v", r)
	}
	tests := []struct {
		reader EntryActor
		want   string
		kind   string
	}{
		{dirActor, "Goblin/standard", ResolvedEntry},
		{playerActor, "", ResolvedNotFound},
		{stranger, "", ResolvedNotFound},
	}
	for _, tt := range tests {
		l, _ := svc.Catalog(ctx, "c1", "monsters", tt.reader)
		if rows(l) != tt.want {
			t.Fatalf("monsters for %+v = %s", tt.reader, rows(l))
		}
		r, _ := svc.ResolveAddress(ctx, "c1", "monsters", "goblin", false, tt.reader)
		r2, _ := svc.ResolveName(ctx, "c1", "monsters", "Goblin", tt.reader)
		if r.Kind != tt.kind || r2.Kind != tt.kind {
			t.Fatalf("goblin for %+v = %s / %s", tt.reader, r.Kind, r2.Kind)
		}
	}
}

func TestPlanReading(t *testing.T) {
	camp := []SystemEntry{{Slug: "dwarf"}, {Slug: "deepkin", StandsFor: "elf"}, {Slug: "x", StandsFor: "gone"}}
	p := planReading([]string{"dwarf", "elf", "polder", "orc"}, camp, map[string]LeftOut{"polder": {}})
	var offered []int
	offered = append(offered, p.offered...)
	sort.Ints(offered)
	if len(offered) != 2 || offered[0] != 0 || offered[1] != 3 || !p.printed[0] || p.printed[3] {
		t.Fatalf("plan = %+v", p)
	}
	if p.replacedBy[1] != 1 || p.standsFor[1] != 1 || len(p.leftOut) != 1 || p.leftOut[0] != 2 {
		t.Fatalf("plan = %+v", p)
	}
	if _, ok := p.standsFor[2]; ok {
		t.Fatal("a tie to a slug the package no longer has replaces nothing")
	}
}

// TestFieldCatalog pins that a field's catalog is the category its pick list
// draws from, and that a key that is no pick list of the system has none.
func TestFieldCatalog(t *testing.T) {
	svc, _, _ := newReadingSvc(t)
	ctx := context.Background()
	got, err := svc.FieldCatalog(ctx, "c1", "ancestry", dirActor)
	if err != nil || got == nil || got.FieldKey != "ancestry" || got.Category != "ancestries" {
		t.Fatalf("ancestry: %+v, %v", got, err)
	}
	for _, key := range []string{"nope", "../ancestry", ""} {
		if got, err := svc.FieldCatalog(ctx, "c1", key, dirActor); err != nil || got != nil {
			t.Errorf("%q: %+v, %v", key, got, err)
		}
	}
}
