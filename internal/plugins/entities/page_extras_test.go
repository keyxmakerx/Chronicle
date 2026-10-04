package entities

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/labstack/echo/v4"
)

func TestPlacePageExtras(t *testing.T) {
	tests := []struct {
		name    string
		layout  EntityTypeLayout
		plan    PageExtrasPlan
		want    []string
		changed bool
	}{
		{
			name:    "plain type gets the pieces in their old order after the layout",
			layout:  DefaultLayout(),
			want:    []string{"title", "entry", "image", "attributes", "details", "children", "posts", "writing_prompts", "backlinks"},
			changed: true,
		},
		{
			name:    "NPC type gets system panels right after the title, then items",
			layout:  DefaultLayout(),
			plan:    PageExtrasPlan{SystemPanels: true, CharacterItems: true},
			want:    []string{"title", "system_panels", "entry", "image", "attributes", "details", "character_items", "children", "posts", "writing_prompts", "backlinks"},
			changed: true,
		},
		{
			name:    "a placed Posts block is not added twice",
			layout:  EntityTypeLayout{Rows: []TemplateRow{fullWidthRow("r", TemplateBlock{ID: "t", Type: "title"}, TemplateBlock{ID: "p", Type: "posts"})}},
			want:    []string{"title", "posts", "children", "writing_prompts", "backlinks"},
			changed: true,
		},
		{
			name:    "a placed Inventory block keeps items and money off",
			layout:  EntityTypeLayout{Rows: []TemplateRow{fullWidthRow("r", TemplateBlock{ID: "i", Type: "inventory"})}},
			plan:    PageExtrasPlan{CharacterItems: true},
			want:    []string{"inventory", "children", "posts", "writing_prompts", "backlinks"},
			changed: true,
		},
		{
			name:    "no title puts system panels in a row at the top",
			layout:  EntityTypeLayout{Rows: []TemplateRow{fullWidthRow("r", TemplateBlock{ID: "e", Type: "entry"})}},
			plan:    PageExtrasPlan{SystemPanels: true},
			want:    []string{"system_panels", "entry", "children", "posts", "writing_prompts", "backlinks"},
			changed: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := placePageExtras(tt.layout, tt.plan)
			if changed != tt.changed {
				t.Errorf("changed = %v, want %v", changed, tt.changed)
			}
			if order := blockTypeOrder(got); !reflect.DeepEqual(order, tt.want) {
				t.Errorf("order = %v, want %v", order, tt.want)
			}
			// Running it again must change nothing.
			again, changed2 := placePageExtras(got, tt.plan)
			if changed2 || !reflect.DeepEqual(blockTypeOrder(again), tt.want) {
				t.Errorf("second run changed the layout: %v", blockTypeOrder(again))
			}
		})
	}
}

func TestAutoCharacterPanel(t *testing.T) {
	pc := PresetCategoryPlayerCharacter
	tests := []struct {
		name string
		et   *EntityType
		want bool
	}{
		{"player character without the block", &EntityType{Slug: "player-character", PresetCategory: &pc, Layout: CharacterLayout()}, true},
		{"player character with Items & Money placed", &EntityType{Slug: "player-character", PresetCategory: &pc,
			Layout: EntityTypeLayout{Rows: []TemplateRow{fullWidthRow("r", TemplateBlock{ID: "c", Type: BlockCharacterItems})}}}, false},
		{"player character with Inventory placed", &EntityType{Slug: "player-character", PresetCategory: &pc,
			Layout: EntityTypeLayout{Rows: []TemplateRow{fullWidthRow("r", TemplateBlock{ID: "i", Type: "inventory"})}}}, false},
		{"NPC sub-type is not the exception", &EntityType{Slug: "npc", ParentTypeID: new(int), Layout: DefaultLayout()}, false},
		{"plain location", &EntityType{Slug: "location", Layout: DefaultLayout()}, false},
		{"nil type", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := autoCharacterPanel(tt.et); got != tt.want {
				t.Errorf("autoCharacterPanel = %v, want %v", got, tt.want)
			}
		})
	}
}

// A page drawn from a layout shows only what the layout places: Posts once,
// and no sub-pages, prompts or backlinks it did not place.
func TestShow_LayoutPageDrawsOnlyPlacedPieces(t *testing.T) {
	reg := NewBlockRegistry()
	RegisterCoreBlocks(reg)
	SetGlobalBlockRegistry(reg)
	t.Cleanup(func() { SetGlobalBlockRegistry(nil) })

	ent, et := dmOnlyFixture()
	et.Layout = EntityTypeLayout{Rows: []TemplateRow{fullWidthRow("r",
		TemplateBlock{ID: "t", Type: "title"}, TemplateBlock{ID: "p", Type: BlockPosts})}}
	child := Entity{ID: "child-1", CampaignID: "c1", EntityTypeID: 7, Name: "The Hidden Vault", Visibility: VisibilityDefault}
	h := &Handler{service: &dmGrantEntitySvc{entity: ent, etype: et, children: []Entity{child}}}

	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/campaigns/c1/entities/e1", nil), rec)
	c.SetParamNames("id", "eid")
	c.SetParamValues("c1", "e1")
	c.Set("campaign_context", coDmContext())
	auth.SetSession(c, &auth.Session{UserID: "codm-1"})
	if err := h.Show(c); err != nil {
		t.Fatalf("Show: %v", err)
	}
	body := rec.Body.String()
	if n := strings.Count(body, `data-widget="entity-posts"`); n != 1 {
		t.Errorf("Posts drawn %d times, want once", n)
	}
	for _, absent := range []string{"The Hidden Vault", "/backlinks", "Writing Prompts"} {
		if strings.Contains(body, absent) {
			t.Errorf("page draws %q although its layout does not place it", absent)
		}
	}
}
