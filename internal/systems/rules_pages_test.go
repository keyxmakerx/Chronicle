package systems

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
)

// fakePeople is the campaign's characters; DirectorsOnly ones are what a
// player's permissions would leave out, as the real adapter does.
type fakePeople struct {
	people []fakePerson
}

type fakePerson struct {
	EntryPerson
	field         string
	owner         string
	directorsOnly bool
}

func (f fakePeople) visible(p fakePerson, r PeopleReader) bool {
	return !p.directorsOnly || r.Role >= 3
}

func (f fakePeople) People(_ context.Context, _, fieldKey string, r PeopleReader, limit int) ([]EntryPerson, error) {
	var out []EntryPerson
	for _, p := range f.people {
		if p.field != fieldKey || !f.visible(p, r) {
			continue
		}
		e := p.EntryPerson
		e.Mine = p.owner != "" && p.owner == r.UserID
		out = append(out, e)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (f fakePeople) Person(_ context.Context, _, id, fieldKey string, r PeopleReader) (*EntryPerson, error) {
	for _, p := range f.people {
		if p.ID == id && p.field == fieldKey && f.visible(p, r) {
			e := p.EntryPerson
			return &e, nil
		}
	}
	return nil, nil
}

func therinPeople() fakePeople {
	return fakePeople{people: []fakePerson{
		{EntryPerson: EntryPerson{ID: "wren", Name: "Wren", Hero: true, Value: "Fairy"}, field: "ancestry", owner: "tyne"},
		{EntryPerson: EntryPerson{ID: "sable", Name: "Sable Varr", Hero: true, Value: "Ashborn"}, field: "ancestry", owner: "pl"},
		{EntryPerson: EntryPerson{ID: "marrow", Name: "Old Marrow", Value: "ashborn"}, field: "ancestry"},
		{EntryPerson: EntryPerson{ID: "spy", Name: "The Spy", Value: "Ashborn"}, field: "ancestry", directorsOnly: true},
		{EntryPerson: EntryPerson{ID: "pip", Name: "Pip", Hero: true, Value: "Polder", Traits: []string{"Corruption Immunity"}}, field: "ancestry", owner: "tyne"},
		{EntryPerson: EntryPerson{ID: "gimli", Name: "Gimli", Hero: true, Value: "Dwarf", Traits: []string{"Stone Skin"}}, field: "ancestry", owner: "pl"},
	}}
}

// newRulesPages is Therin's reading over the catalog test's package.
func newRulesPages(t *testing.T) (*RulesPages, *systemEntryService, *fakeEntryRepo, System) {
	t.Helper()
	svc, repo, data := newReadingSvc(t)
	m := readingManifest()
	m.Categories[0].Name = "Ancestries"
	m.Categories[0].Fields = []FieldDef{{Key: "speed", Label: "Speed"}, {Key: "ancestry_points", Label: "Ancestry Points"},
		{Key: "quick_build_display", Label: "Quick Build"}, {Key: "signature_traits_display", Label: "Signature Traits"},
		{Key: "purchased_traits_display", Label: "Purchased Traits"}, {Key: "provenance_display", Label: "Provenance"}}
	m.Categories[2].Name = "Abilities"
	m.Categories[3].Name = "Monsters"
	sys := fakeSystem{m: m, data: data}
	p := NewRulesPages(svc, func(context.Context, string) *SystemManifest { return m }, therinPeople())
	return p, svc, repo, sys
}

func reader(a EntryActor) RulesReader {
	role := 1
	if a.IsDirector {
		role = 3
	}
	return RulesReader{CampaignID: "c1", CampaignName: "Therin", Actor: a, Role: role}
}

func rowWords(v *RulesListView) string {
	var out []string
	for _, r := range v.Rows {
		w := r.Name
		if r.End != "" {
			w += "[" + r.End + "]"
		}
		if r.DirectorsOnly {
			w += "!"
		}
		out = append(out, w)
	}
	return strings.Join(out, ",")
}

func TestRulesList_PerReader(t *testing.T) {
	p, svc, _, sys := newRulesPages(t)
	ctx := context.Background()
	if _, err := svc.LeaveOut(ctx, "c1", dirActor, LeftOutInput{FieldKey: "ancestry", Slug: "polder", Sentence: "The halflings drowned."}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.LeaveOut(ctx, "c1", dirActor, LeftOutInput{FieldKey: "ancestry", Slug: "devil"}); err != nil {
		t.Fatal(err)
	}
	credit := "[As Draw Steel prints it]"
	tests := []struct {
		name    string
		reader  EntryActor
		rows    string
		leftOut []string
	}{
		{"director", dirActor, "Ashborn,Dwarf,Dwarf" + credit + ",Elf" + credit + ",Fairy,Tideling!",
			[]string{"Devil is not a people of Therin.", "Polder is not a people of Therin. The halflings drowned."}},
		{"a player", playerActor, "Ashborn,Dwarf,Dwarf" + credit + ",Elf" + credit,
			[]string{"Polder is not a people of Therin. The halflings drowned."}},
		{"the player of Wren", tyneActor, "Ashborn,Dwarf,Dwarf" + credit + ",Elf" + credit + ",Fairy",
			[]string{"Polder is not a people of Therin. The halflings drowned."}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := p.List(ctx, sys, "ancestries", "", reader(tt.reader))
			if err != nil {
				t.Fatal(err)
			}
			if got := rowWords(v); got != tt.rows {
				t.Fatalf("rows = %s\nwant   %s", got, tt.rows)
			}
			if v.Count != len(v.Rows) {
				t.Fatalf("count %d is not the list's %d", v.Count, len(v.Rows))
			}
			var lines []string
			for _, r := range v.LeftOut {
				lines = append(lines, r.Line)
				if r.DirectorsOnly != (r.Name == "Devil") {
					t.Fatalf("left-out row mark: %+v", r)
				}
			}
			if strings.Join(lines, "|") != strings.Join(tt.leftOut, "|") {
				t.Fatalf("left out = %q, want %q", lines, tt.leftOut)
			}
			for _, r := range v.Rows {
				if r.Name == "Dwarf" && r.End != "" && !strings.HasSuffix(r.URL, "?printed=1") {
					t.Fatalf("the package's Dwarf must be at ?printed=1: %s", r.URL)
				}
			}
		})
	}
	// A category with no campaign entries of its own carries no credit.
	v, err := p.List(ctx, sys, "abilities", "", reader(playerActor))
	if err != nil || rowWords(v) != "Strike" {
		t.Fatalf("abilities = %q %v", rowWords(v), err)
	}
	// The filter narrows the rows but the count stays the list's.
	v, _ = p.List(ctx, sys, "ancestries", "ash", reader(playerActor))
	if rowWords(v) != "Ashborn" || v.Count != 4 || !v.Filterable {
		t.Fatalf("filtered = %s, count %d", rowWords(v), v.Count)
	}
	if _, err := p.List(ctx, sys, "monsters", "", reader(playerActor)); !isNotFound(err) {
		t.Fatalf("a Directors-only category for a player: %v", err)
	}
}

func isNotFound(err error) bool {
	var ae *apperror.AppError
	return errors.As(err, &ae) && ae.Code == 404
}

func TestRulesPage_Addresses(t *testing.T) {
	p, svc, _, sys := newRulesPages(t)
	ctx := context.Background()
	// Therin's own Elf under another name, tied to the package's, for everyone.
	elf, err := svc.Rewrite(ctx, "c1", dirActor, RewriteInput{FieldKey: "ancestry", Slug: "elf", Name: "Sea Elf"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(ctx, "c1", dirActor, elf.ID, UpdateSystemEntryInput{Visibility: patch.Of(EntryVisibilityEveryone)}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.LeaveOut(ctx, "c1", dirActor, LeftOutInput{FieldKey: "ancestry", Slug: "polder", Sentence: "The halflings drowned."}); err != nil {
		t.Fatal(err)
	}
	const base = "/campaigns/c1/systems/sys/ancestries/"
	tests := []struct {
		name     string
		reader   EntryActor
		slug     string
		printed  bool
		redirect string
		notFound bool
		check    func(t *testing.T, v *EntryView)
	}{
		{name: "standard address of a rewritten entry", reader: playerActor, slug: "elf", redirect: base + "sea-elf"},
		{name: "a player asking for the printed one", reader: playerActor, slug: "elf", printed: true, redirect: base + "sea-elf"},
		{name: "the rewritten page", reader: playerActor, slug: "sea-elf", check: func(t *testing.T, v *EntryView) {
			if v.Whose == nil || v.Whose.Text != "Rewritten for Therin. The printed Elf does not apply here." || len(v.Marks) != 0 {
				t.Fatalf("whose = %+v marks = %+v", v.Whose, v.Marks)
			}
		}},
		{name: "the Director's rewritten page", reader: dirActor, slug: "sea-elf", check: func(t *testing.T, v *EntryView) {
			if len(v.Marks) != 1 || v.Marks[0].LinkText != "Read the printed Elf" || v.Marks[0].Href != base+"elf?printed=1" {
				t.Fatalf("marks = %+v", v.Marks)
			}
		}},
		{name: "the Director's printed original", reader: dirActor, slug: "elf", printed: true, check: func(t *testing.T, v *EntryView) {
			if !v.Printed || len(v.Marks) != 1 || v.Marks[0].Href != base+"sea-elf" || v.InCampaign != nil {
				t.Fatalf("printed view = %+v", v)
			}
		}},
		{name: "the package's Dwarf beside Therin's", reader: playerActor, slug: "dwarf", printed: true, check: func(t *testing.T, v *EntryView) {
			if v.Entry.Origin != OriginStandard || v.Whose == nil || v.Whose.Text != "As Draw Steel prints it" || v.URL != base+"dwarf?printed=1" {
				t.Fatalf("printed Dwarf = %+v", v)
			}
		}},
		{name: "Therin's Dwarf holds the plain address", reader: playerActor, slug: "dwarf", check: func(t *testing.T, v *EntryView) {
			if v.Entry.Origin != OriginCampaign || v.Whose != nil {
				t.Fatalf("Therin's Dwarf = %+v", v)
			}
		}},
		{name: "left out, no hero of it", reader: playerActor, slug: "polder", check: func(t *testing.T, v *EntryView) {
			if !v.LeftOut || v.LeftOutLine != "Polder is not a people of Therin. The halflings drowned." || len(v.Table) != 0 {
				t.Fatalf("left out = %+v", v)
			}
		}},
		{name: "left out, Pip's player", reader: tyneActor, slug: "polder", check: func(t *testing.T, v *EntryView) {
			if !v.LeftOut || len(v.Table) == 0 || !v.Table[0].Items[0].NoLongerOffered {
				t.Fatalf("Pip's player should read the traits, no longer offered: %+v", v.Table)
			}
		}},
		{name: "Directors only, for a player", reader: playerActor, slug: "tideling", notFound: true},
		{name: "Directors only, for the Director", reader: dirActor, slug: "tideling", check: func(t *testing.T, v *EntryView) {
			if len(v.Marks) != 1 || v.Marks[0].Text != "Only Directors see this ancestry." {
				t.Fatalf("marks = %+v", v.Marks)
			}
		}},
		{name: "nothing there", reader: playerActor, slug: "nobody", notFound: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := p.Page(ctx, sys, "ancestries", tt.slug, tt.printed, reader(tt.reader))
			if tt.notFound {
				if !isNotFound(err) {
					t.Fatalf("want not found, got %+v %v", v, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if v.Redirect != tt.redirect {
				t.Fatalf("redirect = %q, want %q", v.Redirect, tt.redirect)
			}
			if tt.check != nil {
				tt.check(t, v)
			}
		})
	}
}

// "In Therin" lists exactly the characters the reader may see whose stored
// name leads, for them, to this entry: a lower-case or old name counts.
func TestRulesPage_InCampaign(t *testing.T) {
	p, svc, repo, sys := newRulesPages(t)
	ctx := context.Background()
	names := func(ls []personLink) string {
		var out []string
		for _, l := range ls {
			out = append(out, l.Name)
		}
		return strings.Join(out, ",")
	}
	tests := []struct {
		name           string
		reader         EntryActor
		slug           string
		heroes, others string
	}{
		{"a player on Ashborn", playerActor, "ashborn", "Sable Varr", "Old Marrow"},
		{"the Director on Ashborn", dirActor, "ashborn", "Sable Varr", "Old Marrow,The Spy"},
		{"Fairy for Wren's player", tyneActor, "fairy", "Wren", ""},
		{"Therin's Dwarf takes the stored name", playerActor, "dwarf", "Gimli", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := p.Page(ctx, sys, "ancestries", tt.slug, false, reader(tt.reader))
			if err != nil {
				t.Fatal(err)
			}
			if v.InCampaign == nil {
				t.Fatalf("no In Therin")
			}
			if got := names(v.InCampaign.Heroes); got != tt.heroes {
				t.Fatalf("heroes = %s, want %s", got, tt.heroes)
			}
			if got := names(v.InCampaign.Others); got != tt.others {
				t.Fatalf("others = %s, want %s", got, tt.others)
			}
		})
	}
	// The package's Dwarf is a different entry: nobody is drawn there.
	v, err := p.Page(ctx, sys, "ancestries", "dwarf", true, reader(playerActor))
	if err != nil || v.InCampaign != nil || len(v.Contents) != 0 {
		t.Fatalf("printed Dwarf In Therin = %+v %v", v.InCampaign, err)
	}
	// After a rename, a hero who stored the old name still appears.
	ash := entryByName(t, repo, "Ashborn")
	if _, err := svc.Update(ctx, "c1", dirActor, ash.ID, UpdateSystemEntryInput{Name: patch.Of("Emberkin")}); err != nil {
		t.Fatal(err)
	}
	v, err = p.Page(ctx, sys, "ancestries", "ashborn", false, reader(playerActor))
	if err != nil || v.InCampaign == nil || names(v.InCampaign.Heroes) != "Sable Varr" {
		t.Fatalf("renamed entry lost its hero: %+v %v", v, err)
	}
}

func TestRulesPeek_Hero(t *testing.T) {
	p, svc, repo, sys := newRulesPages(t)
	ctx := context.Background()
	d := entryByName(t, repo, "Dwarf")
	if _, err := svc.Tie(ctx, "c1", dirActor, d.ID, "dwarf"); err != nil {
		t.Fatal(err)
	}
	// Gimli bought Stone Skin from the package's Dwarf; tied, the name leads
	// to Therin's, whose list has no Stone Skin: the line names nothing.
	v, err := p.Peek(ctx, sys, "ancestries", "dwarf", false, "gimli", reader(playerActor))
	if err != nil || v.Hero == nil || len(v.Hero.Traits) != 0 {
		t.Fatalf("tied Dwarf peek = %+v %v", v.Hero, err)
	}
	if _, err := svc.Untie(ctx, "c1", dirActor, d.ID); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		slug    string
		printed bool
		hero    string
		reader  EntryActor
		want    string // "<Short>: traits" or "" for no hero line
	}{
		{"Pip on the left-out Polder", "polder", false, "pip", tyneActor, "Pip: Corruption Immunity"},
		{"a hero of another people", "elf", false, "pip", tyneActor, ""},
		{"a hero the reader may not see", "ashborn", false, "spy", playerActor, ""},
		{"no hero asked", "ashborn", false, "", playerActor, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := p.Peek(ctx, sys, "ancestries", tt.slug, tt.printed, tt.hero, reader(tt.reader))
			if err != nil {
				t.Fatal(err)
			}
			got := ""
			if v.Hero != nil {
				var ts []string
				for _, it := range v.Hero.Traits {
					ts = append(ts, it.Name)
				}
				got = v.Hero.Short + ": " + strings.Join(ts, ",")
			}
			if got != tt.want {
				t.Fatalf("hero line = %q, want %q", got, tt.want)
			}
		})
	}
}

// The structured value wins over its generated twin, a list of words is a
// comma line, a list of named items with costs is the buy list, and a
// campaign entry draws its remaining details the same way.
func TestBuildTable_DisplayRule(t *testing.T) {
	_, _, _, sys := newRulesPages(t)
	cat := findCategory(sys.Info(), "ancestries")
	dwarf, _ := sys.DataProvider().Get("ancestries", "dwarf")
	tests := []struct {
		name     string
		props    map[string]any
		campaign bool
		want     string
	}{
		{"package entry", dwarf.Properties, false,
			"figures:Speed=5,Points to spend=3|words:Quick build=Grounded, Stone Skin|items:Signature trait=Grounded|items:Traits to buy(3 points to spend)=Stone Skin/1 point[Type:triggered,Distance:self]"},
		{"twin with no structured base is drawn as text", map[string]any{"signature_traits_display": "Grounded — Hard to move."}, false,
			"figures:Signature traits=Grounded — Hard to move."},
		{"campaign entry draws the rest", map[string]any{"speed": 6.0, "breath": map[string]any{"damage": "2d6"}, "lore_display": "x", "lore": "Old songs."}, true,
			"figures:Speed=6,Lore=Old songs.|parts:Breath=Damage:2d6"},
		{"a campaign entry's card look and provenance parts are not figures", map[string]any{"speed": 5.0,
			"card_border": "thorn-glass", "card_colour": "#c4243a", "custom_fields": []any{"description"},
			"derived_from": "dwarf", "omissions": []any{"x"}}, true,
			"figures:Speed=5"},
		{"an ability inside a trait is its own block, its repeated name dropped", map[string]any{
			"signature_traits": []any{map[string]any{"name": "Shadowmeld", "description": "You have the following ability.",
				"ability": map[string]any{"name": "Shadowmeld", "flavor": "You become an actual {@condition hidden|shadow}.",
					"type": "maneuver", "keywords": []any{"Magic"}, "distance": "Self"}}}}, true,
			"items:Signature trait=Shadowmeld[lead=You become an actual shadow.;Type:maneuver,Keywords:Magic,Distance:Self]"},
		{"a nested object with its own name keeps it", map[string]any{
			"signature_traits": []any{map[string]any{"name": "Ember Heart",
				"ability": map[string]any{"name": "Flare", "effect": "Dazed."}}}}, true,
			"items:Signature trait=Ember Heart[Flare;Effect:Dazed.]"},
		{"a kit's ability leads with its flavor", map[string]any{
			"signature_ability": map[string]any{"name": "Exploding Arrow", "flavor": "Your ammunition explodes.", "distance": "Ranged 15"}}, true,
			"items:Signature ability=Exploding Arrow{Your ammunition explodes.}(Distance:Ranged 15)"},
		{"an object without a name leads with its flavor too", map[string]any{
			"breath": map[string]any{"flavor": "Smoke first.", "damage": "2d6"}}, true,
			"parts:Breath{Smoke first.}=Damage:2d6"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, d := range buildTable(cat, tt.props, tt.campaign, "trait-") {
				switch d.Kind {
				case detailFigures:
					var fs []string
					for _, f := range d.Figures {
						fs = append(fs, f.Label+"="+f.Value)
					}
					got = append(got, "figures:"+strings.Join(fs, ","))
				case detailWords:
					got = append(got, "words:"+d.Label+"="+d.Words)
				case detailItems:
					var is []string
					for _, it := range d.Items {
						w := it.Name
						if it.Cost != "" {
							w += "/" + it.Cost
						}
						if len(it.Lead) > 0 {
							w += "{" + it.Lead[0][0].Text + "}"
						}
						if len(it.Parts) > 0 {
							w += "(" + partWords(it.Parts) + ")"
						}
						for _, n := range it.Nested {
							var bits []string
							if n.Name != "" {
								bits = append(bits, n.Name)
							}
							if len(n.Lead) > 0 {
								bits = append(bits, "lead="+n.Lead[0][0].Text)
							}
							bits = append(bits, partWords(n.Parts))
							w += "[" + strings.Join(bits, ";") + "]"
						}
						is = append(is, w)
					}
					l := d.Label
					if d.Aside != "" {
						l += "(" + d.Aside + ")"
					}
					got = append(got, "items:"+l+"="+strings.Join(is, ","))
				case detailParts:
					l := d.Label
					if len(d.Lead) > 0 {
						l += "{" + d.Lead[0][0].Text + "}"
					}
					got = append(got, "parts:"+l+"="+partWords(d.Parts))
				case detailText:
					got = append(got, "text:"+d.Label)
				}
			}
			if strings.Join(got, "|") != tt.want {
				t.Fatalf("table = %s\nwant    %s", strings.Join(got, "|"), tt.want)
			}
		})
	}
}

// partWords writes parts as "Label:text,..." from each part's first run.
func partWords(parts []part) string {
	var ps []string
	for _, pt := range parts {
		ps = append(ps, pt.Label+":"+pt.Text[0][0].Text)
	}
	return strings.Join(ps, ",")
}

func TestRichParas(t *testing.T) {
	got := richParas("A {@condition taunted|taunts} foe. **Light:** glows.\n\nSecond.")
	if len(got) != 2 || got[0][0].Text != "A taunts foe. " || !got[0][1].Bold || got[0][1].Text != "Light:" || got[1][0].Text != "Second." {
		t.Fatalf("paras = %+v", got)
	}
}

func TestRulesSearchAndTooltip(t *testing.T) {
	p, svc, _, sys := newRulesPages(t)
	ctx := context.Background()
	if _, err := svc.LeaveOut(ctx, "c1", dirActor, LeftOutInput{FieldKey: "ancestry", Slug: "polder", Sentence: "The halflings drowned."}); err != nil {
		t.Fatal(err)
	}
	elf, _ := svc.Rewrite(ctx, "c1", dirActor, RewriteInput{FieldKey: "ancestry", Slug: "elf", Name: "Sea Elf"})
	_, _ = svc.Update(ctx, "c1", dirActor, elf.ID, UpdateSystemEntryInput{Visibility: patch.Of(EntryVisibilityEveryone)})
	hits := func(q string, a EntryActor) string {
		hs, err := p.Search(ctx, sys, q, reader(a))
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, h := range hs {
			out = append(out, h.Kind+":"+h.Name+"="+h.Summary)
		}
		return strings.Join(out, "|")
	}
	tests := []struct {
		q      string
		reader EntryActor
		want   string
	}{
		{"polder", playerActor, "left_out:Polder=Polder is not a people of Therin. The halflings drowned."},
		{"tide", playerActor, ""},
		{"tide", dirActor, "entry:Tideling="},
		{"stone", playerActor, "trait:Stone Skin=Stone folk."},
		// The rewritten Elf answers by its name; the printed one never does.
		{"elf", playerActor, "entry:Sea Elf=" + cleanSummary(strings.Repeat("Old folk. ", 30))},
	}
	for _, tt := range tests {
		if got := hits(tt.q, tt.reader); got != tt.want {
			t.Errorf("search %q as %+v = %s\nwant %s", tt.q, tt.reader, got, tt.want)
		}
	}
	tip, err := p.Tooltip(ctx, sys, "ancestries", "polder", false, reader(playerActor))
	if err != nil || !tip.LeftOut || tip.Line != "Polder is not a people of Therin. The halflings drowned." {
		t.Fatalf("left-out tip = %+v %v", tip, err)
	}
	tip, err = p.Tooltip(ctx, sys, "ancestries", "elf", true, reader(playerActor))
	if err != nil || tip.Entry.Name != "Sea Elf" || tip.Whose != "Rewritten for Therin. The printed Elf does not apply here." {
		t.Fatalf("rewritten tip = %+v %v", tip, err)
	}
	if _, err := p.Tooltip(ctx, sys, "ancestries", "tideling", false, reader(playerActor)); !isNotFound(err) {
		t.Fatalf("Directors-only tip reached a player: %v", err)
	}
}
