package systems

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// The package's ancestries file as it ships, with a key only the file has.
const rawAncestries = `[
 {"slug":"dwarf","name":"Dwarf","source":"Heroes","properties":{"purchased_traits":[{"name":"Stone Skin","cost":1}]}},
 {"slug":"elf","name":"Elf","source":"Heroes","properties":{"purchased_traits":[{"name":"Old Eyes","cost":2}]}},
 {"slug":"polder","name":"Polder","source":"Heroes","properties":{"purchased_traits":[{"name":"Corruption Immunity","cost":1}]}},
 {"slug":"devil","name":"Devil","source":"Heroes","properties":{}}
]`

// dataWords renders a kind file as "name/origin" words with its marks, by name.
func dataWords(t *testing.T, b []byte) string {
	t.Helper()
	var items []map[string]any
	if err := json.Unmarshal(b, &items); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, it := range items {
		w, _ := it["name"].(string)
		if o, _ := it["origin"].(string); o != "" {
			w += "/" + o
		} else if it["source"] != "Heroes" {
			t.Fatalf("a package object lost its own keys: %v", it)
		}
		for _, m := range []string{"printed", "leftOut", "directorsOnly"} {
			if it[m] == true {
				w += "+" + m
			}
		}
		out = append(out, w)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func TestKindData_PerReader(t *testing.T) {
	p, svc, _, sys := newRulesPages(t)
	ctx := context.Background()
	elf, err := svc.Rewrite(ctx, "c1", dirActor, RewriteInput{FieldKey: "ancestry", Slug: "elf", Name: "Sea Elf"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.LeaveOut(ctx, "c1", dirActor, LeftOutInput{FieldKey: "ancestry", Slug: "polder"}); err != nil {
		t.Fatal(err)
	}
	// Both Dwarfs: the campaign's holds the plain slug, so the package's is
	// marked printed; the left-out Polder stays for heroes who are one.
	const common = "Ashborn/campaign,Devil,Dwarf+printed,Dwarf/campaign,Polder+leftOut"
	tests := []struct {
		name    string
		reader  EntryActor
		visible bool // the rewritten Elf is for everyone, not a draft
		want    string
	}{
		{"a player while the rewrite is a draft", playerActor, false, common + ",Elf"},
		{"the Director reads the draft in its place", dirActor, false, common + ",Fairy/campaign,Sea Elf/rewritten+directorsOnly,Tideling/campaign+directorsOnly"},
		{"a player once it is for everyone", playerActor, true, common + ",Sea Elf/rewritten"},
		{"Wren's player also has the Fairy", tyneActor, true, common + ",Fairy/campaign,Sea Elf/rewritten"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.visible {
				if _, err := svc.Update(ctx, "c1", dirActor, elf.ID, UpdateSystemEntryInput{Visibility: patch.Of(EntryVisibilityEveryone)}); err != nil {
					t.Fatal(err)
				}
			}
			out, handled, err := p.KindData(ctx, sys, "ancestries", []byte(rawAncestries), reader(tt.reader))
			if err != nil || !handled {
				t.Fatalf("handled %v err %v", handled, err)
			}
			want := strings.Split(tt.want, ",")
			sort.Strings(want)
			if got := dataWords(t, out); got != strings.Join(want, ",") {
				t.Fatalf("file = %s\nwant   %s", got, tt.want)
			}
			if strings.Contains(string(out), "Cursed") && !tt.reader.IsDirector {
				t.Fatal("a secret span reached a player")
			}
		})
	}
	// Not a kind, or another system: served as it is.
	if _, handled, _ := p.KindData(ctx, sys, "abilities", []byte(`[]`), reader(playerActor)); handled {
		t.Fatal("abilities are not a kind")
	}
	other := fakeSystem{m: &SystemManifest{ID: "other", Categories: sys.Info().Categories, EntityPresets: sys.Info().EntityPresets}}
	if _, handled, _ := p.KindData(ctx, other, "ancestries", []byte(rawAncestries), reader(playerActor)); handled {
		t.Fatal("another system has no campaign overlay")
	}
}

// Two saves racing can leave a standard entry both rewritten and left out,
// which the service otherwise refuses. A reader who has the rewrite then gets
// it alone: never the printed object beside it, a left-out line for it, or
// its printed words at the printed address.
func TestLeftOutAndReplacedAtOnce(t *testing.T) {
	p, svc, repo, sys := newRulesPages(t)
	ctx := context.Background()
	elf, err := svc.Rewrite(ctx, "c1", dirActor, RewriteInput{FieldKey: "ancestry", Slug: "elf", Name: "Sea Elf"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(ctx, "c1", dirActor, elf.ID, UpdateSystemEntryInput{Visibility: patch.Of(EntryVisibilityEveryone)}); err != nil {
		t.Fatal(err)
	}
	// Written past the service, as the race would leave it.
	if err := repo.PutLeftOut(ctx, &LeftOut{CampaignID: "c1", SystemID: sys.Info().ID, FieldKey: "ancestry", Slug: "elf", Sentence: "Gone."}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name        string
		reader      EntryActor
		printedKind string
	}{
		{"a player", playerActor, ResolvedRedirect},
		{"the Director", dirActor, ResolvedPrinted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := reader(tt.reader)
			out, _, err := p.KindData(ctx, sys, "ancestries", []byte(rawAncestries), r)
			if err != nil {
				t.Fatal(err)
			}
			if w := dataWords(t, out); strings.Contains(","+w+",", ",Elf") || !strings.Contains(w, "Sea Elf/rewritten") {
				t.Fatalf("file = %s", w)
			}
			if strings.Contains(string(out), "Old Eyes") {
				t.Fatal("the printed Elf's words came with the rewrite")
			}
			l, err := svc.Catalog(ctx, "c1", "ancestries", tt.reader)
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range append(append([]CatalogEntry{}, l.LeftOutNoted...), l.LeftOut...) {
				if e.Slug == "elf" {
					t.Fatalf("the Elf is listed as left out: %+v", e)
				}
			}
			hits, err := p.Search(ctx, sys, "elf", r)
			if err != nil {
				t.Fatal(err)
			}
			for _, h := range hits {
				if h.Kind == "left_out" {
					t.Fatalf("search gave a left-out line: %+v", h)
				}
			}
			res, err := svc.ResolveAddress(ctx, "c1", "ancestries", "elf", true, tt.reader)
			if err != nil || res.Kind != tt.printedKind {
				t.Fatalf("printed address = %+v %v, want %s", res, err, tt.printedKind)
			}
		})
	}
}

// A campaign entry stores its lists alone, with no generated twin, while the
// manifest's fields point at the twins. The Handbook's index and the data
// file read the twin's text from the list, as the entry page does, so a
// campaign entry's column is never blank.
func TestCampaignEntryTwins(t *testing.T) {
	p, svc, _, sys := newRulesPages(t)
	ctx := context.Background()
	in := CreateSystemEntryInput{FieldKey: "ancestry", Name: "Gillfolk", Properties: map[string]any{
		"speed":            5.0,
		"purchased_traits": []any{map[string]any{"name": "Gills", "cost": 1.0}, map[string]any{"name": "Webbed Feet", "cost": 2.0}},
	}}
	if _, err := svc.Create(ctx, "c1", dirActor, in); err != nil {
		t.Fatal(err)
	}
	l, err := svc.Catalog(ctx, "c1", "ancestries", dirActor)
	if err != nil {
		t.Fatal(err)
	}
	def := findCategory(sys.Info(), "ancestries")
	ie := catalogIndex(l, def, "Therin")
	var fields []string
	for _, it := range ie.items {
		if it.Name != "Gillfolk" {
			continue
		}
		for _, f := range ie.indexEntry(it, true, "").Fields {
			fields = append(fields, f.Label+"="+f.Value)
		}
	}
	if got := strings.Join(fields, ";"); got != "Speed=5;Purchased Traits=Gills, Webbed Feet" {
		t.Fatalf("index fields = %s", got)
	}
	out, _, err := p.KindData(ctx, sys, "ancestries", []byte(rawAncestries), reader(dirActor))
	if err != nil {
		t.Fatal(err)
	}
	var items []map[string]any
	if err := json.Unmarshal(out, &items); err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it["name"] == "Gillfolk" {
			props, _ := it["properties"].(map[string]any)
			if props["purchased_traits_display"] != "Gills, Webbed Feet" || props["purchased_traits"] == nil {
				t.Fatalf("data file properties = %v", props)
			}
			return
		}
	}
	t.Fatal("Gillfolk is not in the data file")
}

// Each campaign object in the data file names what else it answers to for
// the reader: the printed name a rewrite replaces and the entry's old names,
// so a sheet finds it from a hero who still stores the printed or old name.
func TestKindData_NamesAnEntryAnswersTo(t *testing.T) {
	p, svc, repo, sys := newRulesPages(t)
	ctx := context.Background()
	elf, err := svc.Rewrite(ctx, "c1", dirActor, RewriteInput{FieldKey: "ancestry", Slug: "elf", Name: "Sea Elf"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(ctx, "c1", dirActor, elf.ID, UpdateSystemEntryInput{Visibility: patch.Of(EntryVisibilityEveryone)}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(ctx, "c1", dirActor, entryByName(t, repo, "Ashborn").ID, UpdateSystemEntryInput{Name: patch.Of("Emberkin")}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(ctx, "c1", dirActor, entryByName(t, repo, "Tideling").ID, UpdateSystemEntryInput{Name: patch.Of("Deepling")}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		reader EntryActor
		want   string
	}{
		{"a player", playerActor, "Emberkin<Ashborn,Sea Elf=Elf"},
		{"the Director also has the hidden entry's old name", dirActor, "Deepling<Tideling,Emberkin<Ashborn,Sea Elf=Elf"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, _, err := p.KindData(ctx, sys, "ancestries", []byte(rawAncestries), reader(tt.reader))
			if err != nil {
				t.Fatal(err)
			}
			var items []map[string]any
			if err := json.Unmarshal(out, &items); err != nil {
				t.Fatal(err)
			}
			var words []string
			for _, it := range items {
				w, _ := it["name"].(string)
				if s, _ := it["standsForName"].(string); s != "" {
					w += "=" + s
				}
				if ns, _ := it["oldNames"].([]any); len(ns) > 0 {
					for _, n := range ns {
						w += "<" + n.(string)
					}
				}
				if w != it["name"] {
					words = append(words, w)
				}
			}
			sort.Strings(words)
			if got := strings.Join(words, ","); got != tt.want {
				t.Fatalf("names = %s, want %s", got, tt.want)
			}
		})
	}
}
