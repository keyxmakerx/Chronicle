package entities

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func appendWorld(t *testing.T, et EntityType, named *PageLayouts) (EntityService, *string, *memPageLayouts) {
	t.Helper()
	var saved string
	repo := &mockEntityTypeRepo{
		findByIDFn: func(context.Context, int) (*EntityType, error) { c := et; return &c, nil },
		updateLayoutFn: func(_ context.Context, _ int, j string) error {
			saved = j
			return nil
		},
	}
	svc := newTestService(&mockEntityRepo{}, repo)
	reg := NewBlockRegistry()
	RegisterCoreBlocks(reg)
	for _, typ := range []string{"quest_board", "trackers"} {
		reg.Register(BlockMeta{Type: typ, Label: typ, Contexts: []string{"template"}, Singleton: true},
			func(BlockRenderContext) templ.Component { return templ.NopComponent })
	}
	svc.(*entityService).SetBlockRegistry(reg)
	mem := newMemPageLayouts()
	if named != nil {
		mem.types[et.ID] = *named
	}
	svc.(*entityService).SetPageLayoutStore(NewPageLayoutService(mem))
	return svc, &saved, mem
}

func TestAppendBlockToCategoryLayout(t *testing.T) {
	ctx := context.Background()
	base := EntityType{ID: 4, CampaignID: "c1", Name: "Quest", NamePlural: "Quests", Layout: layoutOf("title")}

	t.Run("goes in a section at the end", func(t *testing.T) {
		svc, saved, _ := appendWorld(t, base, nil)
		added, err := svc.AppendBlockToCategoryLayout(ctx, "c1", 4, "quest_board")
		if err != nil || !added {
			t.Fatalf("added=%v err=%v", added, err)
		}
		var got EntityTypeLayout
		if err := json.Unmarshal([]byte(*saved), &got); err != nil {
			t.Fatal(err)
		}
		if len(got.Rows) != 2 || got.Rows[0].Columns[0].Blocks[0].Type != "title" {
			t.Fatalf("the existing section must stay first: %+v", got.Rows)
		}
		last := got.Rows[1].Columns[0]
		if last.Width != 12 || len(last.Blocks) != 1 || last.Blocks[0].Type != "quest_board" {
			t.Errorf("last section = %+v", last)
		}
	})

	t.Run("a second time adds nothing", func(t *testing.T) {
		et := base
		et.Layout = layoutOf("quest_board")
		svc, saved, _ := appendWorld(t, et, nil)
		added, err := svc.AppendBlockToCategoryLayout(ctx, "c1", 4, "quest_board")
		if err != nil || added || *saved != "" {
			t.Fatalf("added=%v err=%v saved=%q", added, err, *saved)
		}
	})

	t.Run("a block inside a container counts as placed", func(t *testing.T) {
		et := base
		et.Layout = EntityTypeLayout{Rows: []TemplateRow{{ID: "r", Columns: []TemplateColumn{{ID: "c", Width: 12, Blocks: []TemplateBlock{
			{ID: "t", Type: "tabs", Config: map[string]any{"tabs": []any{map[string]any{"blocks": []any{map[string]any{"id": "x", "type": "trackers"}}}}}},
		}}}}}}
		svc, _, _ := appendWorld(t, et, nil)
		if added, err := svc.AppendBlockToCategoryLayout(ctx, "c1", 4, "trackers"); err != nil || added {
			t.Fatalf("added=%v err=%v", added, err)
		}
	})

	t.Run("each named layout gets it too", func(t *testing.T) {
		named := &PageLayouts{Extra: []PageLayout{{Key: "side", Name: "Side quest", Layout: layoutOf("title")}}}
		svc, _, mem := appendWorld(t, base, named)
		if added, err := svc.AppendBlockToCategoryLayout(ctx, "c1", 4, "quest_board"); err != nil || !added {
			t.Fatalf("added=%v err=%v", added, err)
		}
		rows := mem.types[4].Extra[0].Layout.Rows
		if len(rows) != 2 || rows[1].Columns[0].Blocks[0].Type != "quest_board" {
			t.Errorf("named layout rows = %+v", rows)
		}
	})

	t.Run("a category of another campaign is not found", func(t *testing.T) {
		svc, _, _ := appendWorld(t, base, nil)
		_, err := svc.AppendBlockToCategoryLayout(ctx, "other", 4, "quest_board")
		assertAppError(t, err, http.StatusNotFound)
	})

	t.Run("an unknown block is refused", func(t *testing.T) {
		svc, _, _ := appendWorld(t, base, nil)
		_, err := svc.AppendBlockToCategoryLayout(ctx, "c1", 4, "no_such_block")
		assertAppError(t, err, http.StatusBadRequest)
	})

	t.Run("a full layout is refused and nothing is saved", func(t *testing.T) {
		et := base
		for i := 0; i < maxLayoutRows; i++ {
			et.Layout.Rows = append(et.Layout.Rows, TemplateRow{ID: "r" + string(rune('a'+i)), Columns: []TemplateColumn{{ID: "c" + string(rune('a'+i)), Width: 12, Blocks: []TemplateBlock{{ID: "b" + string(rune('a'+i)), Type: "divider"}}}}})
		}
		svc, saved, _ := appendWorld(t, et, nil)
		_, err := svc.AppendBlockToCategoryLayout(ctx, "c1", 4, "quest_board")
		assertAppError(t, err, http.StatusBadRequest)
		if *saved != "" {
			t.Error("a refused append must not save")
		}
	})
}

func TestAppendLayoutBlockAPI(t *testing.T) {
	et := EntityType{ID: 4, CampaignID: "c1", Name: "Quest", NamePlural: "Quests", Layout: layoutOf("title")}
	run := func(t *testing.T, body string) (int, string) {
		t.Helper()
		svc, _, _ := appendWorld(t, et, nil)
		h := &Handler{service: svc}
		req := httptest.NewRequest(http.MethodPost, "/campaigns/c1/entity-types/4/layout/blocks", strings.NewReader(body))
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(req, rec)
		c.SetPathValues(echo.PathValues{{Name: "id", Value: "c1"}, {Name: "etid", Value: "4"}})
		c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RoleOwner})
		err := h.AppendLayoutBlock(c)
		if ae, ok := err.(*apperror.AppError); ok {
			return ae.Code, ""
		} else if err != nil {
			t.Fatal(err)
		}
		return rec.Code, rec.Body.String()
	}
	if code, out := run(t, `{"type":"quest_board"}`); code != 200 || !strings.Contains(out, `"added"`) || !strings.Contains(out, "Quests") {
		t.Errorf("added: %d %s", code, out)
	}
	if code, _ := run(t, `{}`); code != http.StatusBadRequest {
		t.Errorf("no type: %d", code)
	}
}

// The route is the owner's, and a co-DM arrives as the owner.
func TestAppendLayoutBlockRouteIsOwnerPost(t *testing.T) {
	raw, err := os.ReadFile("routes.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	want := `cg.POST("/entity-types/:etid/layout/blocks", h.AppendLayoutBlock, campaigns.RequireRole(campaigns.RoleOwner))`
	if !strings.Contains(src, want) {
		t.Errorf("routes.go must register %s", want)
	}
}

func TestBlockPlacementOptions(t *testing.T) {
	adv := PresetCategoryAdventures
	child := 1
	types := []EntityType{
		{ID: 1, CampaignID: "c1", Slug: "location", Name: "Location", NamePlural: "Locations", Enabled: true, Layout: layoutOf("title")},
		{ID: 2, CampaignID: "c1", Slug: "quest", Name: "Quest", NamePlural: "Quests", Enabled: true, Layout: layoutOf("quest_board")},
		{ID: 3, CampaignID: "c1", Slug: "adventures", Name: "Adventure", NamePlural: "Adventures", PresetCategory: &adv, Enabled: true, Layout: layoutOf("title")},
		{ID: 4, CampaignID: "c1", Slug: "off", Name: "Off", NamePlural: "Offs", Enabled: false, Layout: layoutOf("title")},
		{ID: 5, CampaignID: "c1", Slug: "kind", Name: "Kind", NamePlural: "Kinds", Enabled: true, ParentTypeID: &child, Layout: layoutOf("title")},
	}
	repo := &mockEntityTypeRepo{listByCampaignFn: func(context.Context, string) ([]EntityType, error) { return types, nil }}
	svc := newTestService(&mockEntityRepo{}, repo)
	reg := NewBlockRegistry()
	for _, b := range []string{"quest_board", "notice_boards", "trackers", "page_tests"} {
		reg.Register(BlockMeta{Type: b, Label: b, Contexts: []string{"template"}, Singleton: true},
			func(BlockRenderContext) templ.Component { return templ.NopComponent })
	}
	svc.(*entityService).SetBlockRegistry(reg)

	for _, tt := range []struct {
		block     string
		firstID   int
		wantOrder []int
	}{
		{"quest_board", 2, []int{2, 1, 3}},
		{"notice_boards", 1, []int{1, 2, 3}},
		{"trackers", 3, []int{3, 1, 2}},
		{"page_tests", 3, []int{3, 1, 2}},
	} {
		got, err := svc.BlockPlacementOptions(context.Background(), "c1", tt.block)
		if err != nil {
			t.Fatal(err)
		}
		var ids []int
		for _, o := range got {
			ids = append(ids, o.ID)
		}
		if len(got) != 3 || !got[0].Suggested || got[0].ID != tt.firstID || got[1].Suggested || got[2].Suggested {
			t.Errorf("%s: options = %+v", tt.block, got)
		}
		if !slices.Equal(ids, tt.wantOrder) {
			t.Errorf("%s: order = %v, want %v", tt.block, ids, tt.wantOrder)
		}
	}

	// With no matching category nothing is suggested, and a placed block shows.
	types = types[:1]
	got, _ := svc.BlockPlacementOptions(context.Background(), "c1", "quest_board")
	if len(got) != 1 || got[0].Suggested || got[0].Placed {
		t.Errorf("no match: %+v", got)
	}
	types = []EntityType{{ID: 2, CampaignID: "c1", Slug: "quest", Name: "Quest", NamePlural: "Quests", Enabled: true, Layout: layoutOf("quest_board")}}
	got, _ = svc.BlockPlacementOptions(context.Background(), "c1", "quest_board")
	if len(got) != 1 || !got[0].Placed {
		t.Errorf("placed: %+v", got)
	}
	if _, err := svc.BlockPlacementOptions(context.Background(), "c1", "title"); err == nil {
		// title is not registered in this test registry, so it must be refused.
		t.Error("an unregistered block must be refused")
	}
}

func TestBlockPlacementAPI(t *testing.T) {
	et := EntityType{ID: 4, CampaignID: "c1", Slug: "quest", Name: "Quest", NamePlural: "Quests", Enabled: true, Layout: layoutOf("title")}
	svc := newTestService(&mockEntityRepo{}, &mockEntityTypeRepo{
		listByCampaignFn: func(context.Context, string) ([]EntityType, error) { return []EntityType{et}, nil },
	})
	reg := NewBlockRegistry()
	reg.Register(BlockMeta{Type: "quest_board", Label: "Quest board", Contexts: []string{"template"}, Singleton: true},
		func(BlockRenderContext) templ.Component { return templ.NopComponent })
	svc.(*entityService).SetBlockRegistry(reg)
	h := &Handler{service: svc}
	call := func(block string) (int, string) {
		req := httptest.NewRequest(http.MethodGet, "/campaigns/c1/entity-types/placement?block="+block, nil)
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(req, rec)
		c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RoleOwner})
		if err := h.BlockPlacementAPI(c); err != nil {
			if ae, ok := err.(*apperror.AppError); ok {
				return ae.Code, ""
			}
			t.Fatal(err)
		}
		return rec.Code, rec.Body.String()
	}
	code, body := call("quest_board")
	if code != 200 || !strings.Contains(body, `"suggested":true`) || !strings.Contains(body, `"namePlural":"Quests"`) {
		t.Errorf("quest_board: %d %s", code, body)
	}
	if code, _ := call("nope"); code != http.StatusBadRequest {
		t.Errorf("unknown block: %d", code)
	}
}
