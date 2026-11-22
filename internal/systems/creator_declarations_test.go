package systems

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// drawFields is a small preset shaped like a system that declares creator
// blocks; the real package is not read, so these tests hold with any system.
func creatorTestManifest() *SystemManifest {
	return &SystemManifest{ID: "sys", Name: "Sys", EntityPresets: []EntityPresetDef{{
		Slug: "sys-character", Category: "character",
		CreatorOrder: []string{"ancestry", "class", "subclass", "kit", "nope"},
		Fields: []FieldDef{
			{Key: "class", Label: "Class", Type: "string", Creator: &CreatorDef{Needed: true}},
			{Key: "kit", Label: "Kit", Type: "string", Creator: &CreatorDef{CountFrom: []CreatorCount{
				{Step: "class", Property: "kits", Note: "kit_note"},
				{Step: "subclass", Property: "kits", Note: "kit_note"},
			}}},
			{Key: "subclass", Label: "Subclass", Type: "string", Creator: &CreatorDef{Needed: true, From: &CreatorFrom{Step: "class", Property: "subclasses"}}},
			{Key: "ancestry", Label: "Ancestry", Type: "string", Creator: &CreatorDef{Needed: true}},
			{Key: "complication", Label: "Complication", Type: "string"},
		},
	}}}
}

var creatorTestFiles = map[string]string{
	"sys/data/ancestries.json":    `[{"slug":"orc","name":"Orc"}]`,
	"sys/data/kits.json":          `[{"slug":"panther","name":"Panther"}]`,
	"sys/data/complications.json": `[{"slug":"lucky","name":"Lucky"}]`,
	"sys/data/classes.json": `[
		{"slug":"fury","name":"Fury","properties":{"kits":1,"subclasses":[{"name":"Berserker","summary":"Wild.","kits":0,"kit_note":"Special kits."},"Reaver"]}},
		{"slug":"conduit","name":"Conduit","properties":{"kits":0,"kit_note":"Conduits don't use a kit.","subclasses":["Light","Light"]}},
		{"slug":"beastheart","name":"Beastheart","properties":{"kits":1}}
	]`,
}

func TestCreatorPlanDeclarations(t *testing.T) {
	svc := newTestChoiceService(t, creatorTestFiles, []*SystemManifest{creatorTestManifest()}, fakeAddons{"sys": true})
	plan, err := svc.CreatorPlan(context.Background(), "c1")
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	by := map[string]CreatorStep{}
	for _, s := range plan.Steps {
		keys = append(keys, s.FieldKey)
		by[s.FieldKey] = s
	}
	// creator_order first (unknown names ignored), then the rest in preset order.
	if got, want := strings.Join(keys, ","), "ancestry,class,subclass,kit,complication"; got != want {
		t.Fatalf("order = %s, want %s", got, want)
	}
	if !by["ancestry"].Needed || !by["class"].Needed || !by["subclass"].Needed || by["kit"].Needed || by["complication"].Needed {
		t.Errorf("needed flags wrong: %+v", plan.Steps)
	}
	if by["subclass"].Parent != "class" || by["class"].Parent != "" {
		t.Errorf("parent = %q / %q", by["subclass"].Parent, by["class"].Parent)
	}
	if cf := by["kit"].CountFrom; len(cf) != 2 || cf[0].Step != "class" || cf[1].Step != "subclass" || cf[0].Note != "kit_note" {
		t.Errorf("countFrom = %+v", cf)
	}
}

// A system that declares nothing keeps the order and flags it always had.
func TestCreatorPlanUndeclaredUnchanged(t *testing.T) {
	m := creatorTestManifest()
	p := &m.EntityPresets[0]
	p.CreatorOrder = nil
	for i := range p.Fields {
		p.Fields[i].Creator = nil
	}
	svc := newTestChoiceService(t, creatorTestFiles, []*SystemManifest{m}, fakeAddons{"sys": true})
	plan, _ := svc.CreatorPlan(context.Background(), "c1")
	var keys []string
	for _, s := range plan.Steps {
		keys = append(keys, s.FieldKey)
		if s.Needed || s.Parent != "" || len(s.CountFrom) != 0 {
			t.Errorf("%s carries a declaration it never made: %+v", s.FieldKey, s)
		}
	}
	if got := strings.Join(keys, ","); got != "class,kit,ancestry,complication" {
		t.Errorf("order = %s", got)
	}
}

func TestDependentChoiceList(t *testing.T) {
	svc := newTestChoiceService(t, creatorTestFiles, []*SystemManifest{creatorTestManifest()}, fakeAddons{"sys": true})
	l, err := svc.CharacterChoiceList(context.Background(), "c1", "subclass")
	if err != nil {
		t.Fatal(err)
	}
	type row struct{ name, parent, key string }
	var got []row
	for _, c := range l.Choices {
		got = append(got, row{c.Name, c.Parent, c.Key})
	}
	// Parents come in the class list's order (by name); a repeated item under
	// one parent shows once; Beastheart lists none.
	want := []row{
		{"Light", "Conduit", "conduit--light"},
		{"Berserker", "Fury", "fury--berserker"},
		{"Reaver", "Fury", "fury--reaver"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	b := l.Choices[1]
	if b.Summary != "Wild." || b.Slug != b.Key || b.Source != ChoiceSourcePackage {
		t.Errorf("object item lost its words: %+v", b)
	}
	if v, ok := b.Properties["kits"].(float64); !ok || v != 0 || b.Properties["kit_note"] != "Special kits." {
		t.Errorf("object keys did not become properties: %+v", b.Properties)
	}
}

// Whatever the viewer may not pick at the parent yields no children either.
type dropByName string

func (d dropByName) Limit(_ context.Context, _ string, l *ChoiceList) error {
	if l.FieldKey != "class" {
		return nil
	}
	kept := l.Choices[:0:0]
	for _, c := range l.Choices {
		if c.Name != string(d) {
			kept = append(kept, c)
		}
	}
	l.Choices = kept
	return nil
}

func TestDependentChoiceListFollowsParentLimits(t *testing.T) {
	svc := newTestChoiceService(t, creatorTestFiles, []*SystemManifest{creatorTestManifest()}, fakeAddons{"sys": true})
	svc.SetChoiceLimiter(dropByName("Fury"))
	l, err := svc.CharacterChoiceList(context.Background(), "c1", "subclass")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range l.Choices {
		if c.Parent == "Fury" {
			t.Fatalf("a class the viewer may not pick still offers %q", c.Name)
		}
	}
	if len(l.Choices) != 1 {
		t.Errorf("choices = %+v", l.Choices)
	}
}

func TestSanitizeCreatorBlocks(t *testing.T) {
	str := func(k string) FieldDef { return FieldDef{Key: k, Label: k, Type: "string"} }
	cases := []struct {
		name  string
		build func() []FieldDef
		check func(t *testing.T, f []FieldDef)
	}{
		{"from naming a number field is dropped, field and needed stay", func() []FieldDef {
			a := str("sub")
			a.Creator = &CreatorDef{Needed: true, From: &CreatorFrom{Step: "level", Property: "subs"}}
			return []FieldDef{a, {Key: "level", Label: "L", Type: "number"}}
		}, func(t *testing.T, f []FieldDef) {
			if f[0].Creator.From != nil || !f[0].Creator.Needed {
				t.Errorf("%+v", f[0].Creator)
			}
		}},
		{"from naming itself or a missing step is dropped", func() []FieldDef {
			a := str("a")
			a.Creator = &CreatorDef{From: &CreatorFrom{Step: "a", Property: "x"}}
			b := str("b")
			b.Creator = &CreatorDef{From: &CreatorFrom{Step: "zzz", Property: "x"}}
			return []FieldDef{a, b}
		}, func(t *testing.T, f []FieldDef) {
			if f[0].Creator.From != nil || f[1].Creator.From != nil {
				t.Error("kept a from that names no other string field")
			}
		}},
		{"property that is not an identifier is dropped", func() []FieldDef {
			a := str("a")
			a.Creator = &CreatorDef{From: &CreatorFrom{Step: "b", Property: "../x"}}
			return []FieldDef{a, str("b")}
		}, func(t *testing.T, f []FieldDef) {
			if f[0].Creator.From != nil {
				t.Error("kept a path as a property")
			}
		}},
		{"count_from keeps the good entries and drops the bad", func() []FieldDef {
			k := str("kit")
			k.Creator = &CreatorDef{CountFrom: []CreatorCount{
				{Step: "class", Property: "kits", Note: "kit_note"},
				{Step: "ghost", Property: "kits"},
				{Step: "class", Property: "bad key"},
				{Step: "class", Property: "kits", Note: "a b"},
			}}
			return []FieldDef{k, str("class")}
		}, func(t *testing.T, f []FieldDef) {
			if cf := f[0].Creator.CountFrom; len(cf) != 1 || cf[0].Note != "kit_note" {
				t.Errorf("countFrom = %+v", cf)
			}
		}},
		{"a loop of from steps loses the closing link", func() []FieldDef {
			a := str("a")
			a.Creator = &CreatorDef{From: &CreatorFrom{Step: "b", Property: "x"}}
			b := str("b")
			b.Creator = &CreatorDef{From: &CreatorFrom{Step: "a", Property: "x"}}
			return []FieldDef{a, b}
		}, func(t *testing.T, f []FieldDef) {
			if (f[0].Creator.From == nil) == (f[1].Creator.From == nil) {
				t.Errorf("exactly one link should go: %+v %+v", f[0].Creator, f[1].Creator)
			}
		}},
		{"a good chain is untouched", func() []FieldDef {
			a := str("a")
			a.Creator = &CreatorDef{From: &CreatorFrom{Step: "b", Property: "x"}}
			b := str("b")
			b.Creator = &CreatorDef{From: &CreatorFrom{Step: "c", Property: "y"}}
			return []FieldDef{a, b, str("c")}
		}, func(t *testing.T, f []FieldDef) {
			if f[0].Creator.From == nil || f[1].Creator.From == nil {
				t.Error("dropped a valid chain")
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &SystemManifest{ID: "s", EntityPresets: []EntityPresetDef{{Slug: "s-character", Fields: tc.build()}}}
			m.sanitizeCreatorBlocks()
			tc.check(t, m.EntityPresets[0].Fields)
		})
	}
}

// The wire names a package author writes.
func TestCreatorManifestJSON(t *testing.T) {
	raw := `{"id":"s","entity_presets":[{"slug":"s-character","creator_order":["a","b"],"fields":[
		{"key":"b","label":"B","type":"string","creator":{"needed":true,"from":{"step":"a","property":"subs"},"count_from":[{"step":"a","property":"kits","note":"kit_note"}]}}]}]}`
	var m SystemManifest
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	p := m.EntityPresets[0]
	c := p.Fields[0].Creator
	if len(p.CreatorOrder) != 2 || c == nil || !c.Needed || c.From.Step != "a" || c.From.Property != "subs" || c.CountFrom[0].Note != "kit_note" {
		t.Fatalf("decoded %+v / %+v", p.CreatorOrder, c)
	}
}
