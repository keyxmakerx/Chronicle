package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/crafting"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
)

// The item block goes right after the entry, wherever the entry sits, or at
// the end when a page has none; a page that has it is left alone.
func TestWithItemBlock(t *testing.T) {
	entry := entities.TemplateBlock{ID: "e", Type: "entry"}
	title := entities.TemplateBlock{ID: "t", Type: "title"}
	tests := []struct {
		name    string
		in      entities.EntityTypeLayout
		wantRow int
		wantCol int
		wantAt  int
		rows    int
	}{
		{name: "after the entry", in: entities.EntityTypeLayout{Rows: []entities.TemplateRow{{ID: "r", Columns: []entities.TemplateColumn{
			{ID: "a", Width: 4, Blocks: []entities.TemplateBlock{title}},
			{ID: "b", Width: 8, Blocks: []entities.TemplateBlock{title, entry, title}},
		}}}}, wantRow: 0, wantCol: 1, wantAt: 2, rows: 1},
		{name: "no entry adds a row", in: entities.EntityTypeLayout{Rows: []entities.TemplateRow{{ID: "r", Columns: []entities.TemplateColumn{
			{ID: "a", Width: 12, Blocks: []entities.TemplateBlock{title}},
		}}}}, wantRow: 1, wantCol: 0, wantAt: 0, rows: 2},
		{name: "an empty page", in: entities.EntityTypeLayout{}, wantRow: 0, wantCol: 0, wantAt: 0, rows: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if layoutHasBlock(tt.in, crafting.ItemBlockType) {
				t.Fatal("the layout has the block before it was placed")
			}
			got := withItemBlock(tt.in)
			if len(got.Rows) != tt.rows {
				t.Fatalf("rows = %d, want %d", len(got.Rows), tt.rows)
			}
			if b := got.Rows[tt.wantRow].Columns[tt.wantCol].Blocks[tt.wantAt]; b.Type != crafting.ItemBlockType {
				t.Fatalf("block there is %q", b.Type)
			}
			if !layoutHasBlock(got, crafting.ItemBlockType) {
				t.Fatal("layoutHasBlock missed the placed block")
			}
		})
	}
}

// knownOnly is a crafting service that holds recipes a and b.
type knownOnly struct{ crafting.Service }

func (knownOnly) KnownRecipes(_ context.Context, _ string, _ crafting.Viewer, ids []string) ([]string, error) {
	var out []string
	for _, id := range ids {
		if id == "a" || id == "b" {
			out = append(out, id)
		}
	}
	return out, nil
}

func TestCraftingPartCheck(t *testing.T) {
	check := craftingPartCheck(knownOnly{})
	tests := []struct {
		name string
		in   string
		want string // the cleaned data, or part of the error
		bad  bool
	}{
		{name: "defaults", in: `{}`, want: `{"who":"all","start":"draft"}`},
		{name: "gone recipes drop out", in: `{"who":"director","start":"ready","recipes":["a","gone","b"]}`, want: `{"who":"director","start":"ready","recipes":["a","b"]}`},
		{name: "an unknown who", in: `{"who":"players"}`, want: "who can make them", bad: true},
		{name: "an unknown start", in: `{"start":"approved"}`, want: "new rows start", bad: true},
		{name: "an unknown key", in: `{"html":"<b>"}`, bad: true},
		{name: "too many recipes", in: `{"recipes":[` + strings.TrimSuffix(strings.Repeat(`"a",`, 201), ",") + `]}`, want: "at most 200", bad: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := check(context.Background(), "c", json.RawMessage(tt.in))
			if tt.bad {
				if err == nil || !strings.Contains(err.Error(), tt.want) {
					t.Fatalf("got %v, want an error about %q", err, tt.want)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
		})
	}
	if _, err := checkCraftingTeach(context.Background(), "c", json.RawMessage(`{"recipe":"  "}`)); err == nil {
		t.Fatal("a teach with no recipe passed")
	}
}

// The part is drawn by crafting's own widget, which the catalog says.
func TestCraftingPartIsInTheCatalog(t *testing.T) {
	reg := homebrew.NewRegistry()
	if err := registerCraftingParts(reg, knownOnly{}); err != nil {
		t.Fatal(err)
	}
	cat := reg.Catalog()
	var found bool
	for _, p := range cat.Parts {
		if p.Name == craftingPartKind {
			found = true
			if p.Widget != craftingPartWidget || p.Label != "Crafting recipes" {
				t.Fatalf("part %+v", p)
			}
		}
	}
	if !found {
		t.Fatal("the Crafting recipes part is not in the catalog")
	}
	var teach bool
	for _, a := range cat.Actions {
		teach = teach || a.Name == "crafting.teach"
	}
	if !teach {
		t.Fatal("rules can't teach a crafting recipe")
	}
}
