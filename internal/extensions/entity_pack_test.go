package extensions

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// provRepo is an ExtensionRepository that keeps provenance in memory; any
// other call panics on the nil interface.
type provRepo struct {
	ExtensionRepository
	rows []Provenance
}

func (r *provRepo) CreateProvenance(_ context.Context, p *Provenance) error {
	r.rows = append(r.rows, *p)
	return nil
}

func (r *provRepo) ListProvenance(_ context.Context, campaignID, extensionID string) ([]Provenance, error) {
	var out []Provenance
	for _, p := range r.rows {
		if p.CampaignID == campaignID && p.ExtensionID == extensionID {
			out = append(out, p)
		}
	}
	return out, nil
}

// countingTypes creates entity types with increasing ids.
type countingTypes struct{ next int }

func (c *countingTypes) CreateEntityType(context.Context, string, EntityTypeCreateInput) (EntityTypeResult, error) {
	c.next++
	return EntityTypeResult{ID: 100 + c.next, Slug: "t" + strconv.Itoa(c.next)}, nil
}

// packPage is one page a fake creator was asked for.
type packPage struct {
	typeID  int
	name    string
	private bool
	userID  string
}

// fakePackPages records created pages; "npc" is the only existing category.
type fakePackPages struct {
	pages []packPage
}

func (f *fakePackPages) EntityTypeIDBySlug(_ context.Context, _ string, slug string) (int, error) {
	if slug == "npc" {
		return 7, nil
	}
	return 0, errors.New("no such category")
}

func (f *fakePackPages) CreatePackEntity(_ context.Context, _ string, userID string, typeID int, e PackEntry) (string, error) {
	f.pages = append(f.pages, packPage{typeID: typeID, name: e.Name, private: e.IsPrivate(), userID: userID})
	return "page-" + strconv.Itoa(len(f.pages)), nil
}

func writePack(t *testing.T, dir, file string, entries any) {
	t.Helper()
	b, _ := json.Marshal(entries)
	path := filepath.Join(dir, "my-pack", file)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestApplyEntityPacks(t *testing.T) {
	dir := t.TempDir()
	writePack(t, dir, "data/monsters.json", []map[string]any{
		{"name": "Goblin", "entry": "Small and **mean**."},
		{"name": "Owlbear", "private": false},
	})
	writePack(t, dir, "data/folk.json", []map[string]any{{"name": "Mayor"}})
	writePack(t, dir, "data/lost.json", []map[string]any{{"name": "Nobody"}})

	manifest := &ExtensionManifest{Contributes: &ManifestContributes{
		EntityTypeTemplates: []EntityTypeTemplate{{Slug: "monster", Name: "Monster", NamePlural: "Monsters"}},
		EntityPacks: []EntityPack{
			{Slug: "monsters", EntityTypeSlug: "monster", File: "data/monsters.json"},
			{Slug: "folk", EntityTypeSlug: "npc", File: "data/folk.json"},
			{Slug: "lost", EntityTypeSlug: "nowhere", File: "data/lost.json"},
			{Slug: "missing", EntityTypeSlug: "npc", File: "data/missing.json"},
		},
	}}
	ext := &Extension{ID: "ext-1", ExtID: "my-pack"}

	repo := &provRepo{}
	types := &countingTypes{}
	pages := &fakePackPages{}
	a := NewContentApplier(dir, repo, types, nil)
	a.SetPackEntityCreator(pages)

	if err := a.Apply(context.Background(), "camp-1", "owner-1", ext, manifest); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	want := []packPage{
		{typeID: 101, name: "Goblin", private: true, userID: "owner-1"},
		{typeID: 101, name: "Owlbear", private: false, userID: "owner-1"},
		{typeID: 7, name: "Mayor", private: true, userID: "owner-1"},
	}
	if len(pages.pages) != len(want) {
		t.Fatalf("pages = %+v, want %+v", pages.pages, want)
	}
	for i := range want {
		if pages.pages[i] != want[i] {
			t.Errorf("page %d = %+v, want %+v", i, pages.pages[i], want[i])
		}
	}

	var packRows int
	for _, p := range repo.rows {
		if p.TableName == "entities" {
			packRows++
		}
	}
	if packRows != 3 {
		t.Errorf("entity provenance rows = %d, want 3", packRows)
	}

	// Turning the extension off and on again creates nothing new.
	if err := a.Apply(context.Background(), "camp-1", "owner-1", ext, manifest); err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if types.next != 1 {
		t.Errorf("entity types created = %d after re-apply, want 1", types.next)
	}
	if len(pages.pages) != 3 {
		t.Errorf("pages = %d after re-apply, want 3", len(pages.pages))
	}

	// Another campaign gets its own copy.
	if err := a.Apply(context.Background(), "camp-2", "owner-2", ext, manifest); err != nil {
		t.Fatalf("Apply camp-2: %v", err)
	}
	if len(pages.pages) != 6 || pages.pages[3].typeID != 102 {
		t.Errorf("camp-2 pages = %+v, want three more on the new type 102", pages.pages[3:])
	}
}

func TestReadPackFileLimits(t *testing.T) {
	dir := t.TempDir()
	writePack(t, dir, "big.json", make([]map[string]any, DefaultMaxEntityPack+1))
	writePack(t, dir, "obj.json", map[string]any{"name": "not an array"})
	a := &contentApplier{extDir: dir}

	for _, file := range []string{"big.json", "obj.json", "absent.json"} {
		if _, err := a.readPackFile("my-pack", file); err == nil {
			t.Errorf("readPackFile(%s) = nil error, want refusal", file)
		}
	}
}
