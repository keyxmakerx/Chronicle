package app

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
	"github.com/keyxmakerx/chronicle/internal/tilegrid"
)

// listedRecipes answers List from a fixed list; nothing else is called.
type listedRecipes struct {
	homebrew.Service
	list []homebrew.RecipeSummary
	err  error
	seen homebrew.Viewer
}

func (l *listedRecipes) List(_ context.Context, _ string, v homebrew.Viewer) ([]homebrew.RecipeSummary, error) {
	l.seen = v
	return l.list, l.err
}

func TestRecipeBlockMetas(t *testing.T) {
	svc := &listedRecipes{list: []homebrew.RecipeSummary{
		{ID: "r-need", Name: "Need", PartKinds: []string{"tracker"}},
		{ID: "r-words", Name: "Words", PartKinds: []string{"fieldrule"}},
		{ID: "r-doom", Name: "Doom", PartKinds: []string{"clock", "fieldrule"}},
	}}
	got := recipeBlockMetas(context.Background(), svc, "camp")
	if !svc.seen.Director {
		t.Error("recipes should be read as a Director")
	}
	var names []string
	for _, m := range got {
		names = append(names, m.Label)
		if m.Type != recipeBlockType || m.Group != "Homebrew" || m.PresetOnly || m.Preset["recipe_id"] == "" || len(m.TileSizes) == 0 {
			t.Errorf("entry %+v", m)
		}
	}
	if !reflect.DeepEqual(names, []string{"Need", "Doom"}) {
		t.Errorf("entries = %v, want the recipes with a tracker or clock", names)
	}

	svc.err = errors.New("db down")
	if got := recipeBlockMetas(context.Background(), svc, "camp"); got != nil {
		t.Errorf("a failed list should offer nothing, got %v", got)
	}
}

func recipeBlock(id string) entities.TemplateBlock {
	return entities.TemplateBlock{ID: "b", Type: recipeBlockType, Config: map[string]any{"recipe_id": id}}
}

func TestLayoutPlacesRecipe(t *testing.T) {
	col := func(b ...entities.TemplateBlock) entities.EntityTypeLayout {
		return entities.EntityTypeLayout{Rows: []entities.TemplateRow{{ID: "r", Columns: []entities.TemplateColumn{{ID: "c", Width: 12, Blocks: b}}}}}
	}
	tests := []struct {
		name   string
		layout entities.EntityTypeLayout
		want   bool
	}{
		{"in a column", col(entities.TemplateBlock{ID: "t", Type: "title"}, recipeBlock("r1")), true},
		{"another recipe", col(recipeBlock("r10")), false},
		{"another type with the id", col(entities.TemplateBlock{ID: "x", Type: "trackers", Config: map[string]any{"recipe_id": "r1"}}), false},
		{"in a tile", entities.EntityTypeLayout{Rows: []entities.TemplateRow{{ID: "r", Kind: "tiles",
			Tiles: []tilegrid.Tile{{ID: "t", Type: recipeBlockType, Size: "2x1", Config: map[string]any{"recipe_id": "r1"}}}}}}, true},
		{"inside an older container", col(entities.TemplateBlock{ID: "s", Type: "tabs", Config: map[string]any{
			"tabs": []any{map[string]any{"blocks": []any{map[string]any{"type": recipeBlockType, "config": map[string]any{"recipe_id": "r1"}}}}}}}), true},
		{"empty", entities.EntityTypeLayout{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := layoutPlacesRecipe(tt.layout, "r1"); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// typedEntities answers GetEntityTypes from a fixed list.
type typedEntities struct {
	entities.EntityService
	types []entities.EntityType
	err   error
}

func (e *typedEntities) GetEntityTypes(context.Context, string) ([]entities.EntityType, error) {
	return e.types, e.err
}

func TestRecipePlacements(t *testing.T) {
	placed := entities.EntityTypeLayout{Rows: []entities.TemplateRow{{ID: "r", Columns: []entities.TemplateColumn{{ID: "c", Width: 12,
		Blocks: []entities.TemplateBlock{recipeBlock("r1")}}}}}}
	ents := &typedEntities{types: []entities.EntityType{
		{ID: 1, Name: "Villain", NamePlural: "Villains", Layout: placed},
		{ID: 2, Name: "Place", NamePlural: "Places"},
		{ID: 3, Name: "Hero", Layout: placed},
	}}
	got, err := (&recipePlacements{entities: ents}).Placements(context.Background(), "camp", "r1")
	if err != nil {
		t.Fatal(err)
	}
	want := []homebrew.Placement{{Where: "category", Name: "Hero"}, {Where: "category", Name: "Villains"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	ents.err = errors.New("db down")
	if _, err := (&recipePlacements{entities: ents}).Placements(context.Background(), "camp", "r1"); err == nil {
		t.Error("a failed read should be an error")
	}
}

func TestRecipeBlockRenders(t *testing.T) {
	reg := entities.NewBlockRegistry()
	registerRecipeBlock(reg)
	var meta entities.BlockMeta
	for _, m := range reg.Types() {
		if m.Type == recipeBlockType {
			meta = m
		}
	}
	if !meta.PresetOnly || meta.Preset == nil || meta.Group != "Homebrew" {
		t.Fatalf("bare type = %+v", meta)
	}
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp"}}
	render := func(id string) string {
		var buf bytes.Buffer
		c := reg.Render(context.Background(), entities.BlockRenderContext{Block: recipeBlock(id), CC: cc, Entity: &entities.Entity{ID: "page"}, CSRFToken: "tok"})
		if err := c.Render(context.Background(), &buf); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	out := render("r1")
	if !strings.Contains(out, `data-owner="homebrew:r1:"`) || !strings.Contains(out, `data-endpoint="/campaigns/camp/trackers/page/page"`) {
		t.Errorf("mount = %s", out)
	}
	for _, bad := range []string{"", "r1\" onfocus=\"x", strings.Repeat("a", 65)} {
		if out := render(bad); strings.Contains(out, "data-widget") {
			t.Errorf("recipe id %q rendered %s", bad, out)
		}
	}
}
