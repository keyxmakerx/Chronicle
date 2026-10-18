package importer

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// creatureFields is a stat-block-shaped sheet: numbers, a pick list, text
// that holds JSON lists, a checkbox and a link.
func creatureFields() []entities.FieldDefinition {
	return []entities.FieldDefinition{
		{Key: "level", Label: "Level", Type: "number"},
		{Key: "organization", Label: "Organization", Type: "select", Options: []string{"Minion", "Horde", "Platoon", "Elite", "Leader", "Solo"}},
		{Key: "stamina", Label: "Stamina", Type: "number"},
		{Key: "keywords", Label: "Keywords", Type: "text"},
		{Key: "abilities_json", Label: "Abilities", Type: "textarea"},
		{Key: "with_captain", Label: "With captain", Type: "checkbox"},
		{Key: "art", Label: "Art", Type: "url"},
	}
}

func creatureTypes() []entities.EntityType {
	return []entities.EntityType{{ID: 7, Name: "Creature", Slug: "creature", Enabled: true, Fields: creatureFields()}}
}

func TestCheckFields(t *testing.T) {
	tests := []struct {
		name    string
		in      map[string]any
		want    map[string]any
		problem string
	}{
		{"numbers, a pick and text", map[string]any{"level": 3, "organization": "horde", "stamina": "30", "keywords": "Humanoid, Orc"},
			map[string]any{"level": 3, "organization": "Horde", "stamina": 30, "keywords": "Humanoid, Orc"}, ""},
		{"a list goes into a text field as JSON", map[string]any{"abilities_json": []any{map[string]any{"name": "Spear"}}},
			map[string]any{"abilities_json": `[{"name":"Spear"}]`}, ""},
		{"yes and no", map[string]any{"with_captain": "yes"}, map[string]any{"with_captain": true}, ""},
		{"null clears", map[string]any{"stamina": nil}, map[string]any{"stamina": nil}, ""},
		{"unknown key", map[string]any{"hp": 30}, nil, `"hp" is not one of this category's fields.`},
		{"not a number", map[string]any{"level": "high"}, nil, `Level: needs a number, not "high".`},
		{"not an option", map[string]any{"organization": "Swarm"}, nil, `"Swarm" is not one of: Minion, Horde`},
		{"not a web address", map[string]any{"art": "javascript:alert(1)"}, nil, "is not an http or https address"},
		{"a list is not a number", map[string]any{"level": []any{1}}, nil, "needs a number, not a list."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, problems := CheckFields(creatureFields(), tc.in)
			if tc.problem != "" {
				if len(problems) == 0 || !strings.Contains(strings.Join(problems, " "), tc.problem) {
					t.Fatalf("problems = %q, want one containing %q", problems, tc.problem)
				}
				if got != nil {
					t.Errorf("nothing should be returned with a problem; got %v", got)
				}
				return
			}
			if len(problems) > 0 {
				t.Fatalf("unexpected problems %q", problems)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("%s = %#v, want %#v", k, got[k], v)
				}
			}
		})
	}
}

func creaturePage(name string, fields map[string]any) ParsedPage {
	p := page(name, "creature", "# "+name+"\n\nA brute.")
	p.FrontMatter.Fields = fields
	return p
}

// A new page gets its checked fields; an update merges only the named ones
// and never sends a whole field set.
func TestCommit_SheetFields(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		f := &fakeCreator{types: creatureTypes()}
		res, err := NewCommitter(f).Commit(context.Background(), "camp-1", CommitInput{
			OwnerID:   "u-1",
			Pages:     []ParsedPage{creaturePage("Orc Brute", map[string]any{"level": 3, "stamina": 30, "stamina_x": nil})},
			Decisions: []RowDecision{decision(true, "Orc Brute", "creature", "", "rename")},
		})
		if err != nil {
			t.Fatal(err)
		}
		if res.Rows[0].Status != StatusFailed || len(f.createCalls) != 0 {
			t.Fatalf("an unknown field must stop the create: %+v, creates %d", res.Rows[0], len(f.createCalls))
		}

		f = &fakeCreator{types: creatureTypes()}
		if _, err := NewCommitter(f).Commit(context.Background(), "camp-1", CommitInput{
			OwnerID:   "u-1",
			Pages:     []ParsedPage{creaturePage("Orc Brute", map[string]any{"level": 3, "stamina": 30, "keywords": nil})},
			Decisions: []RowDecision{decision(true, "Orc Brute", "creature", "", "rename")},
		}); err != nil {
			t.Fatal(err)
		}
		got := f.createCalls[0].FieldsData
		if got["level"] != 3 || got["stamina"] != 30 || len(got) != 2 {
			t.Errorf("create fields = %#v, want level and stamina only", got)
		}
	})
	t.Run("update", func(t *testing.T) {
		f := &fakeCreator{types: creatureTypes(), existing: map[string]*entities.Entity{
			"orc-brute": {ID: "ent-9", Name: "Orc Brute", Slug: "orc-brute", CampaignID: "camp-1", EntityTypeID: 7},
		}}
		p := creaturePage("Orc Brute", map[string]any{"stamina": 40, "keywords": nil})
		p.FrontMatter.Action = ActionUpdate
		d := decision(true, "Orc Brute", "creature", "", "")
		d.Action = ActionUpdate
		if _, err := NewCommitter(f).Commit(context.Background(), "camp-1", CommitInput{OwnerID: "u-1", Pages: []ParsedPage{p}, Decisions: []RowDecision{d}}); err != nil {
			t.Fatal(err)
		}
		if len(f.updateCalls) != 1 || f.updateCalls[0].FieldsData != nil {
			t.Fatalf("an update must never send a whole field set: %+v", f.updateCalls)
		}
		merged := f.mergeCalls["ent-9"]
		if merged["stamina"] != 40 || len(merged) != 2 {
			t.Errorf("merged = %#v, want stamina and the keywords clear", merged)
		}
		if v, ok := merged["keywords"]; !ok || v != nil {
			t.Errorf("a null must reach the merge as a clear: %#v", merged)
		}
	})
}

// The review flags fields that don't fit before anything is applied.
func TestClassify_SheetFields(t *testing.T) {
	lookup := &stubLookup{types: creatureTypes(), bySlug: map[string]*entities.Entity{}}
	pages := []ParsedPage{
		creaturePage("Orc Brute", map[string]any{"level": 3, "organization": "Horde"}),
		creaturePage("Orc Shaman", map[string]any{"hp": 9}),
	}
	cls, err := NewClassifier(lookup, "camp-1").ClassifyAll(context.Background(), pages)
	if err != nil {
		t.Fatal(err)
	}
	if cls[0].Status != StatusNew || cls[0].FieldsSummary != "Sheet: Level 3, Organization Horde" {
		t.Errorf("good row = %s %q", cls[0].Status, cls[0].FieldsSummary)
	}
	if cls[1].Status != StatusFieldProblem || !strings.Contains(cls[1].Reason, `"hp" is not one of this category's fields`) {
		t.Errorf("bad row = %s %q", cls[1].Status, cls[1].Reason)
	}
}

// Link fields take a page's name at check time; the id comes later, once the
// pages of the paste exist.
func TestCheckFields_LinkField(t *testing.T) {
	defs := []entities.FieldDefinition{{Key: "home", Label: "Home", Type: entities.FieldTypeLink, LinkTo: 3}}
	tests := []struct {
		name    string
		in      any
		want    string
		problem string
	}{
		{"a name is kept as text", "  Ironhold ", "Ironhold", ""},
		{"a list is refused", []any{"Ironhold"}, "", "needs the name of a page."},
		{"an empty name is refused", "", "", "needs the name of a page."},
		{"a blank name is refused", "   ", "", "needs the name of a page."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, problems := CheckFields(defs, map[string]any{"home": tc.in})
			if tc.problem != "" {
				if len(problems) != 1 || !strings.Contains(problems[0], tc.problem) {
					t.Fatalf("problems = %q, want one containing %q", problems, tc.problem)
				}
				return
			}
			if len(problems) > 0 || got["home"] != tc.want {
				t.Fatalf("got %#v, problems %q; want home = %q", got, problems, tc.want)
			}
		})
	}
	t.Run("null clears", func(t *testing.T) {
		got, problems := CheckFields(defs, map[string]any{"home": nil})
		if len(problems) > 0 || len(got) != 1 || got["home"] != nil {
			t.Fatalf("got %#v, problems %q", got, problems)
		}
	})
}

// linkFieldAccess is a campaign with pages by slug, all viewable.
type linkFieldAccess struct{ bySlug map[string]*entities.Entity }

func (a linkFieldAccess) GetBySlug(_ context.Context, _, slug string) (*entities.Entity, error) {
	return a.bySlug[slug], nil
}

func (linkFieldAccess) CheckEntityAccess(context.Context, string, int, string) (*entities.EffectivePermission, error) {
	return &entities.EffectivePermission{CanView: true}, nil
}

func TestLinkFields(t *testing.T) {
	defs := []entities.FieldDefinition{
		{Key: "home", Label: "Home", Type: entities.FieldTypeLink},
		{Key: "notes", Label: "Notes", Type: "text"},
		{Key: "level", Label: "Level", Type: "number"},
	}
	access := linkFieldAccess{bySlug: map[string]*entities.Entity{
		"ironhold": {ID: "ent-1", Name: "Ironhold", Slug: "ironhold", CampaignID: "camp"},
	}}
	newLinks := func() *PageLinks {
		return NewPageLinks(context.Background(), access, "camp", Viewer{Role: 1, UserID: "u"})
	}
	tests := []struct {
		name     string
		links    func() *PageLinks
		checked  map[string]any
		wantHome any
		problem  string
	}{
		{"campaign page resolves to id", newLinks, map[string]any{"home": "Ironhold"}, "ent-1", ""},
		{"page made in this paste resolves first", func() *PageLinks {
			l := newLinks()
			l.Add("new-7", "Frostmere")
			return l
		}, map[string]any{"home": "Frostmere"}, "new-7", ""},
		{"unknown name is a problem and stays a name", newLinks, map[string]any{"home": "Nowhere"}, "Nowhere", `there is no page called "Nowhere"`},
		{"nil links is a problem", func() *PageLinks { return nil }, map[string]any{"home": "Ironhold"}, "Ironhold", "pages can't be looked up here"},
		{"cleared link is skipped", newLinks, map[string]any{"home": nil}, nil, ""},
		{"absent link is skipped", newLinks, map[string]any{"level": 2}, nil, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			problems := LinkFields(defs, tc.checked, tc.links())
			if tc.problem != "" {
				if len(problems) != 1 || !strings.Contains(problems[0], tc.problem) {
					t.Fatalf("problems = %q, want one containing %q", problems, tc.problem)
				}
			} else if len(problems) > 0 {
				t.Fatalf("unexpected problems %q", problems)
			}
			if got := tc.checked["home"]; got != tc.wantHome {
				t.Errorf("home = %#v, want %#v", got, tc.wantHome)
			}
		})
	}
	t.Run("non-link fields are untouched", func(t *testing.T) {
		checked := map[string]any{"home": "Ironhold", "notes": "Ironhold", "level": 3}
		if p := LinkFields(defs, checked, newLinks()); len(p) > 0 {
			t.Fatalf("problems %q", p)
		}
		if checked["notes"] != "Ironhold" || checked["level"] != 3 {
			t.Errorf("non-link values changed: %#v", checked)
		}
	})
}
