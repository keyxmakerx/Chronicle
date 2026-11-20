package systems

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

func TestSystemEntryReading_Rewritable(t *testing.T) {
	svc, _, _ := newReadingSvc(t)
	ctx := context.Background()
	tests := []struct {
		field, slug string
		want        bool
	}{
		{"ancestry", "dwarf", true},
		{"ancestry", "elf", true},
		{"ancestry", "devil", false}, // one text detail too long
		{"class", "fury", false},
		{"class", "censor", false},  // classes never, by kind
		{"ancestry", "fury", false}, // another kind's entry
		{"ancestry", "nope", false},
		{"level", "dwarf", false},
		{"../x", "dwarf", false},
	}
	for _, tt := range tests {
		got, err := svc.Rewritable(ctx, "c1", tt.field, tt.slug)
		if err != nil || got != tt.want {
			t.Errorf("Rewritable(%s, %s) = %v %v, want %v", tt.field, tt.slug, got, err, tt.want)
		}
	}
}

func TestSystemEntryReading_Rewrite(t *testing.T) {
	tests := []struct {
		name    string
		actor   EntryActor
		in      RewriteInput
		wantErr int
	}{
		{"player refused", playerActor, RewriteInput{FieldKey: "ancestry", Slug: "elf"}, 403},
		{"stranger refused", stranger, RewriteInput{FieldKey: "ancestry", Slug: "elf"}, 403},
		{"not lossless", dirActor, RewriteInput{FieldKey: "ancestry", Slug: "devil"}, 422},
		{"class never", dirActor, RewriteInput{FieldKey: "class", Slug: "censor"}, 422},
		{"unknown standard", dirActor, RewriteInput{FieldKey: "ancestry", Slug: "gnome"}, 404},
		{"another kind's slug", dirActor, RewriteInput{FieldKey: "ancestry", Slug: "fury"}, 404},
		{"not a kind", dirActor, RewriteInput{FieldKey: "level", Slug: "elf"}, 422},
		{"name already taken", dirActor, RewriteInput{FieldKey: "ancestry", Slug: "dwarf"}, 409},
		{"fresh copy under another name", dirActor, RewriteInput{FieldKey: "ancestry", Slug: "dwarf", Name: "Hill Dwarf"}, 0},
		{"chosen name too long", dirActor, RewriteInput{FieldKey: "ancestry", Slug: "dwarf", Name: strings.Repeat("d", maxEntryName+1)}, 422},
		{"ok", dirActor, RewriteInput{FieldKey: "ancestry", Slug: "elf"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, data := newReadingSvc(t)
			e, err := svc.Rewrite(context.Background(), "c1", tt.actor, tt.in)
			wantCode(t, err, tt.wantErr)
			if tt.wantErr != 0 {
				return
			}
			std, _ := data.Get("ancestries", tt.in.Slug)
			if e.StandsFor != std.ID || e.Visibility != EntryVisibilityDirectors || e.Summary != cleanSummary(std.Summary) || e.CreatedBy != "dm" {
				t.Fatalf("copy = %+v", e)
			}
			var snap StandardSnapshot
			if err := json.Unmarshal([]byte(e.StandsForSeen), &snap); err != nil || snap.Name != std.Name {
				t.Fatalf("snapshot = %q", e.StandsForSeen)
			}
			// The copy is its own: changing it never touches the package.
			if _, err := svc.Update(context.Background(), "c1", dirActor, e.ID, UpdateSystemEntryInput{Properties: patch.Of(map[string]any{"speed": 9.0})}); err != nil {
				t.Fatal(err)
			}
			if again, _ := data.Get("ancestries", tt.in.Slug); again.Properties["speed"] == 9.0 {
				t.Fatal("editing the copy changed the package's data")
			}
		})
	}
}

// The tie rules: Directors only, a standard entry of the same system and
// kind, one stand-in per standard entry, never one that is left out.
func TestSystemEntryReading_Tie(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T, svc *systemEntryService, repo *fakeEntryRepo)
		actor   EntryActor
		entry   string
		slug    string
		wantErr int
	}{
		{"use the Dwarf I already wrote", nil, dirActor, "Dwarf", "dwarf", 0},
		{"a different name is fine", nil, dirActor, "Ashborn", "dwarf", 0},
		{"player refused", nil, playerActor, "Dwarf", "dwarf", 403},
		{"Wren's player refused", nil, tyneActor, "Fairy", "elf", 403},
		{"unknown standard", nil, dirActor, "Dwarf", "gnome", 422},
		{"another kind", nil, dirActor, "Dwarf", "fury", 422},
		{"second stand-in", func(t *testing.T, svc *systemEntryService, repo *fakeEntryRepo) {
			if _, err := svc.Tie(context.Background(), "c1", dirActor, entryByName(t, repo, "Ashborn").ID, "dwarf"); err != nil {
				t.Fatal(err)
			}
		}, dirActor, "Dwarf", "dwarf", 409},
		{"already tied elsewhere", func(t *testing.T, svc *systemEntryService, repo *fakeEntryRepo) {
			if _, err := svc.Tie(context.Background(), "c1", dirActor, entryByName(t, repo, "Dwarf").ID, "elf"); err != nil {
				t.Fatal(err)
			}
		}, dirActor, "Dwarf", "dwarf", 409},
		{"same tie again is a no-op", func(t *testing.T, svc *systemEntryService, repo *fakeEntryRepo) {
			if _, err := svc.Tie(context.Background(), "c1", dirActor, entryByName(t, repo, "Dwarf").ID, "dwarf"); err != nil {
				t.Fatal(err)
			}
		}, dirActor, "Dwarf", "dwarf", 0},
		{"left out", func(t *testing.T, svc *systemEntryService, _ *fakeEntryRepo) {
			if _, err := svc.LeaveOut(context.Background(), "c1", dirActor, LeftOutInput{FieldKey: "ancestry", Slug: "dwarf"}); err != nil {
				t.Fatal(err)
			}
		}, dirActor, "Dwarf", "dwarf", 409},
		{"a draft cannot", func(t *testing.T, _ *systemEntryService, repo *fakeEntryRepo) {
			for i := range repo.rows {
				if repo.rows[i].Name == "Dwarf" {
					repo.rows[i].Status = EntryStatusSent
				}
			}
		}, dirActor, "Dwarf", "dwarf", 422},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo, _ := newReadingSvc(t)
			if tt.setup != nil {
				tt.setup(t, svc, repo)
			}
			e, err := svc.Tie(context.Background(), "c1", tt.actor, entryByName(t, repo, tt.entry).ID, tt.slug)
			wantCode(t, err, tt.wantErr)
			if tt.wantErr == 0 && (e.StandsFor != tt.slug || e.StandsForSeen == "") {
				t.Fatalf("tie = %+v", e)
			}
		})
	}
}

func TestSystemEntryReading_UntieAndSeen(t *testing.T) {
	svc, repo, data := newReadingSvc(t)
	ctx := context.Background()
	d := entryByName(t, repo, "Dwarf")
	_, err := svc.Untie(ctx, "c1", playerActor, d.ID)
	wantCode(t, err, 403)
	_, err = svc.MarkSeen(ctx, "c1", dirActor, d.ID)
	wantCode(t, err, 422)
	if _, err := svc.Tie(ctx, "c1", dirActor, d.ID, "dwarf"); err != nil {
		t.Fatal(err)
	}
	if line, err := svc.StandardChange(ctx, "c1", dirActor, d.ID); err != nil || line != "" {
		t.Fatalf("nothing changed yet: %q %v", line, err)
	}
	// The package changes its Dwarf underneath.
	dw := &data["ancestries"][0]
	dw.Properties = copyProps(dw.Properties)
	dw.Properties["signature_traits"] = []any{map[string]any{"name": "Grounded", "description": "Very hard to move."}}
	line, err := svc.StandardChange(ctx, "c1", dirActor, d.ID)
	if err != nil || line != "Grounded reworded." {
		t.Fatalf("change line = %q %v", line, err)
	}
	_, err = svc.StandardChange(ctx, "c1", playerActor, d.ID)
	wantCode(t, err, 403)
	_, err = svc.MarkSeen(ctx, "c1", playerActor, d.ID)
	wantCode(t, err, 403)
	if _, err := svc.MarkSeen(ctx, "c1", dirActor, d.ID); err != nil {
		t.Fatal(err)
	}
	if line, _ := svc.StandardChange(ctx, "c1", dirActor, d.ID); line != "" {
		t.Fatalf("seen, yet still told: %q", line)
	}
	e, err := svc.Untie(ctx, "c1", dirActor, d.ID)
	if err != nil || e.StandsFor != "" || e.StandsForSeen != "" || e.Summary != "Sea dwarves." {
		t.Fatalf("untie keeps the text and drops the tie: %+v %v", e, err)
	}
	l, _ := svc.Catalog(ctx, "c1", "ancestries", playerActor)
	if !strings.Contains(rows(l), "Dwarf/campaign,Dwarf/standard*") {
		t.Fatalf("untied, both Dwarfs again: %s", rows(l))
	}
}

func TestSystemEntryReading_LeaveOut(t *testing.T) {
	tests := []struct {
		name    string
		actor   EntryActor
		in      LeftOutInput
		tieAsh  bool
		wantErr int
	}{
		{"ok with a sentence", dirActor, LeftOutInput{FieldKey: "ancestry", Slug: "polder", Sentence: "Gone."}, false, 0},
		{"ok without", dirActor, LeftOutInput{FieldKey: "ancestry", Slug: "polder"}, false, 0},
		{"player refused", playerActor, LeftOutInput{FieldKey: "ancestry", Slug: "polder"}, false, 403},
		{"stranger refused", stranger, LeftOutInput{FieldKey: "ancestry", Slug: "polder"}, false, 403},
		{"unknown", dirActor, LeftOutInput{FieldKey: "ancestry", Slug: "gnome"}, false, 404},
		{"not a kind", dirActor, LeftOutInput{FieldKey: "level", Slug: "polder"}, false, 422},
		{"sentence too long", dirActor, LeftOutInput{FieldKey: "ancestry", Slug: "polder", Sentence: strings.Repeat("s", maxLeftOutSentence+1)}, false, 422},
		{"has a stand-in", dirActor, LeftOutInput{FieldKey: "ancestry", Slug: "polder"}, true, 409},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo, _ := newReadingSvc(t)
			ctx := context.Background()
			if tt.tieAsh {
				if _, err := svc.Tie(ctx, "c1", dirActor, entryByName(t, repo, "Ashborn").ID, "polder"); err != nil {
					t.Fatal(err)
				}
			}
			l, err := svc.LeaveOut(ctx, "c1", tt.actor, tt.in)
			wantCode(t, err, tt.wantErr)
			if tt.wantErr != 0 {
				return
			}
			if l.Slug != "polder" || l.Sentence != tt.in.Sentence {
				t.Fatalf("left out = %+v", l)
			}
			// Changing the sentence keeps one row.
			if _, err := svc.LeaveOut(ctx, "c1", dirActor, LeftOutInput{FieldKey: "ancestry", Slug: "polder", Sentence: "Drowned."}); err != nil {
				t.Fatal(err)
			}
			all, _ := svc.LeftOutList(ctx, "c1", "ancestry")
			if len(all) != 1 || all[0].Sentence != "Drowned." {
				t.Fatalf("left-out list = %+v", all)
			}
			wantCode(t, svc.BringBack(ctx, "c1", playerActor, "ancestry", "polder"), 403)
			wantCode(t, svc.BringBack(ctx, "c1", dirActor, "ancestry", "polder"), 0)
			wantCode(t, svc.BringBack(ctx, "c1", dirActor, "ancestry", "polder"), 404)
		})
	}
}

// Old names are recorded on rename, refuse to be given to another entry
// with a message saying where they lead, and can be freed by a Director.
func TestSystemEntryReading_OldNames(t *testing.T) {
	svc, repo, _ := newReadingSvc(t)
	ctx := context.Background()
	ash := entryByName(t, repo, "Ashborn")
	if _, err := svc.Update(ctx, "c1", dirActor, ash.ID, UpdateSystemEntryInput{Name: patch.Of("Emberkin")}); err != nil {
		t.Fatal(err)
	}
	// Only a case change records nothing.
	if _, err := svc.Update(ctx, "c1", dirActor, ash.ID, UpdateSystemEntryInput{Name: patch.Of("EMBERKIN")}); err != nil {
		t.Fatal(err)
	}
	names, _ := svc.OldNames(ctx, "c1", "ancestry", dirActor)
	if len(names) != 1 || names[0].Name != "Ashborn" || names[0].EntryID != ash.ID {
		t.Fatalf("old names = %+v", names)
	}
	_, err := svc.Create(ctx, "c1", dirActor, CreateSystemEntryInput{FieldKey: "ancestry", Name: "ashborn"})
	wantCode(t, err, 409)
	if err == nil || !strings.Contains(err.Error(), "EMBERKIN") {
		t.Fatalf("the refusal names where the old name leads: %v", err)
	}
	tide := entryByName(t, repo, "Tideling")
	_, err = svc.Update(ctx, "c1", dirActor, tide.ID, UpdateSystemEntryInput{Name: patch.Of("Ashborn")})
	wantCode(t, err, 409)
	_, err = svc.Rewrite(ctx, "c1", dirActor, RewriteInput{FieldKey: "ancestry", Slug: "elf", Name: "Ashborn"})
	wantCode(t, err, 409)

	// Players see only the old names of entries they may see.
	if _, err := svc.Update(ctx, "c1", dirActor, tide.ID, UpdateSystemEntryInput{Name: patch.Of("Sea Folk")}); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		reader EntryActor
		want   int
	}{{dirActor, 2}, {playerActor, 1}, {stranger, 1}} {
		if n, _ := svc.OldNames(ctx, "c1", "ancestry", tt.reader); len(n) != tt.want {
			t.Fatalf("old names for %+v = %+v", tt.reader, n)
		}
	}

	// Renaming back to its own old name takes the old name back.
	if _, err := svc.Update(ctx, "c1", dirActor, ash.ID, UpdateSystemEntryInput{Name: patch.Of("Ashborn")}); err != nil {
		t.Fatal(err)
	}
	if o, _ := repo.FindOldName(ctx, "c1", "sys", "ancestry", "Ashborn"); o != nil {
		t.Fatalf("the current name is still an old name: %+v", o)
	}
	o, _ := repo.FindOldName(ctx, "c1", "sys", "ancestry", "Emberkin")
	if o == nil {
		t.Fatal("the name it gave up is now an old name")
	}
	wantCode(t, svc.FreeOldName(ctx, "c1", playerActor, o.ID), 403)
	wantCode(t, svc.FreeOldName(ctx, "c1", dirActor, o.ID), 0)
	wantCode(t, svc.FreeOldName(ctx, "c1", dirActor, o.ID), 404)
	if _, err := svc.Create(ctx, "c1", dirActor, CreateSystemEntryInput{FieldKey: "ancestry", Name: "Emberkin"}); err != nil {
		t.Fatalf("a freed name is free: %v", err)
	}
	// Deleting an entry frees its old names.
	if err := svc.Delete(ctx, "c1", dirActor, tide.ID); err != nil {
		t.Fatal(err)
	}
	if o, _ := repo.FindOldName(ctx, "c1", "sys", "ancestry", "Tideling"); o != nil {
		t.Fatal("old names outlived their entry")
	}
}

func TestSystemEntryReading_Notes(t *testing.T) {
	svc, repo, _ := newReadingSvc(t)
	ctx := context.Background()
	ash := entryByName(t, repo, "Ashborn")
	tide := entryByName(t, repo, "Tideling")
	tests := []struct {
		name    string
		actor   EntryActor
		id      int64
		line    string
		wantErr int
	}{
		{"player refused", playerActor, ash.ID, "x", 403},
		{"stranger refused", stranger, ash.ID, "x", 403},
		{"empty", dirActor, ash.ID, "   ", 422},
		{"too long", dirActor, ash.ID, strings.Repeat("l", maxNoteLine+1), 422},
		{"unknown entry", dirActor, 9999, "x", 404},
		{"first", dirActor, ash.ID, "  Ember Sight:   5 squares, was 10  ", 0},
		{"second", dirActor, ash.ID, "Cinder Step costs 2.", 0},
		{"on a Directors-only entry", dirActor, tide.ID, "Kept quiet.", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.AddNote(ctx, "c1", tt.actor, tt.id, tt.line)
			wantCode(t, err, tt.wantErr)
		})
	}
	notes, err := svc.Notes(ctx, "c1", playerActor, ash.ID)
	if err != nil || len(notes) != 2 || notes[0].Line != "Cinder Step costs 2." || notes[1].Line != "Ember Sight: 5 squares, was 10" {
		t.Fatalf("notes newest first, trimmed: %+v %v", notes, err)
	}
	_, err = svc.Notes(ctx, "c1", playerActor, tide.ID)
	wantCode(t, err, 404)
	if n, err := svc.Notes(ctx, "c1", dirActor, tide.ID); err != nil || len(n) != 1 {
		t.Fatalf("Director notes: %+v %v", n, err)
	}
}

func TestChangedTraits(t *testing.T) {
	entry := func(props map[string]any) *SystemEntry { return &SystemEntry{Properties: props} }
	base := map[string]any{
		"signature_traits": []any{map[string]any{"name": "Ember Sight", "description": "See 10 squares."}},
		"purchased_traits": []any{
			map[string]any{"name": "Cinder Step", "cost": 1.0, "description": "Embers."},
			map[string]any{"name": "Smoke Lungs", "cost": 1.0}},
		"speed": 5.0,
	}
	tests := []struct {
		name  string
		after map[string]any
		want  string
	}{
		{"nothing", base, ""},
		{"figure only", map[string]any{"signature_traits": base["signature_traits"], "purchased_traits": base["purchased_traits"], "speed": 6.0}, ""},
		{"trait added only", map[string]any{"signature_traits": base["signature_traits"], "purchased_traits": append(append([]any{}, base["purchased_traits"].([]any)...), map[string]any{"name": "New"})}, ""},
		{"reworded", map[string]any{"signature_traits": []any{map[string]any{"name": "Ember Sight", "description": "See 5 squares."}}, "purchased_traits": base["purchased_traits"]}, "Ember Sight text"},
		{"removed and recosted", map[string]any{"signature_traits": base["signature_traits"], "purchased_traits": []any{
			map[string]any{"name": "cinder step", "cost": 2.0, "description": "Embers."}}}, "Cinder Step cost 1->2;Smoke Lungs removed"},
		{"moved to another list", map[string]any{"purchased_traits": []any{
			map[string]any{"name": "Ember Sight", "description": "See 10 squares.", "cost": 1.0},
			map[string]any{"name": "Cinder Step", "cost": 1.0, "description": "Embers."},
			map[string]any{"name": "Smoke Lungs", "cost": 1.0}}}, "Ember Sight cost none->1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, c := range ChangedTraits(entry(base), entry(tt.after)) {
				switch {
				case c.Removed:
					got = append(got, c.Name+" removed")
				case c.CostChanged:
					got = append(got, c.Name+" cost "+costWord(c.CostBefore)+"->"+costWord(c.CostAfter))
					if c.TextChanged {
						got = append(got, "and text")
					}
				case c.TextChanged:
					got = append(got, c.Name+" text")
				}
			}
			if s := strings.ReplaceAll(strings.Join(got, ";"), "nothing", "none"); s != tt.want {
				t.Fatalf("got %q, want %q", s, tt.want)
			}
		})
	}
	if ChangedTraits(nil, entry(base)) != nil {
		t.Fatal("nil before")
	}
}

func TestDescribeStandardChange(t *testing.T) {
	trait := func(name, text string) map[string]any { return map[string]any{"name": name, "description": text} }
	was := StandardSnapshot{Name: "Dwarf", Summary: "Stone folk.", Description: "Dense.", Properties: map[string]any{
		"speed": 5.0, "size": "1M",
		"signature_traits":         []any{trait("Grounded", "Hard to move.")},
		"purchased_traits":         []any{map[string]any{"name": "Stone Skin", "cost": 1.0}, trait("Spark", "Zap.")},
		"signature_traits_display": "Grounded — Hard to move.",
	}}
	with := func(f func(p map[string]any)) StandardSnapshot {
		n := was
		n.Properties = copyProps(was.Properties)
		f(n.Properties)
		return n
	}
	tests := []struct {
		name string
		now  StandardSnapshot
		want string
	}{
		{"nothing", was, ""},
		{"display twin only", with(func(p map[string]any) { p["signature_traits_display"] = "other" }), ""},
		{"reworded", with(func(p map[string]any) { p["signature_traits"] = []any{trait("Grounded", "Very hard.")} }), "Grounded reworded."},
		{"added and dropped", with(func(p map[string]any) {
			p["purchased_traits"] = []any{map[string]any{"name": "Stone Skin", "cost": 1.0}, trait("Iron Gut", "Eat anything.")}
		}), "Iron Gut added; Spark dropped."},
		{"cost", with(func(p map[string]any) {
			p["purchased_traits"] = []any{map[string]any{"name": "Stone Skin", "cost": 2.0}, trait("Spark", "Zap.")}
		}), "Stone Skin now costs 2."},
		{"figures", with(func(p map[string]any) { p["speed"] = 6.0; delete(p, "size"); p["stability"] = 1.0 }),
			"Size dropped; speed 5 → 6; stability added."},
		{"5 and 5.0 are equal", with(func(p map[string]any) { p["speed"] = 5 }), ""},
		{"text", StandardSnapshot{Name: "Dwarf", Summary: "Stone people.", Description: was.Description, Properties: was.Properties}, "Text changed."},
		{"renamed", StandardSnapshot{Name: "Dwarves", Summary: was.Summary, Description: was.Description, Properties: was.Properties}, "Now called Dwarves."},
		{"many", with(func(p map[string]any) {
			p["speed"], p["size"], p["a"], p["b"], p["c"], p["d"] = 1.0, "2L", 1.0, 2.0, 3.0, 4.0
		}), "A added; b added; c added; d added; 2 more changes."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DescribeStandardChange(was, tt.now); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNotInLine(t *testing.T) {
	tests := []struct {
		name, field, sentence, want string
	}{
		{"Polder", "ancestry", "", "Polder is not a people of Therin."},
		{"Polder", "ancestry", " The halflings drowned. ", "Polder is not a people of Therin. The halflings drowned."},
		{"Arcane Archer", "kit", "", "Arcane Archer is not a kit of Therin."},
		{"Nomad", "upbringing", "", "Nomad is not an upbringing of Therin."},
	}
	for _, tt := range tests {
		if got := NotInLine(tt.name, tt.field, "Therin", tt.sentence); got != tt.want {
			t.Errorf("got %q, want %q", got, tt.want)
		}
	}
}
