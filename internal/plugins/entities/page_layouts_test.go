package entities

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// memPageLayouts is PageLayoutRepository in memory.
type memPageLayouts struct {
	types map[int]PageLayouts
	pages map[string]string
}

func newMemPageLayouts() *memPageLayouts {
	return &memPageLayouts{types: map[int]PageLayouts{}, pages: map[string]string{}}
}

func (m *memPageLayouts) TypeLayouts(_ context.Context, typeID int) (PageLayouts, error) {
	return m.types[typeID], nil
}
func (m *memPageLayouts) CampaignTypeLayouts(_ context.Context, _ string) (map[int]PageLayouts, error) {
	return m.types, nil
}
func (m *memPageLayouts) SaveTypeLayouts(_ context.Context, _ string, typeID int, p PageLayouts) error {
	if p.Empty() {
		delete(m.types, typeID)
	} else {
		m.types[typeID] = p
	}
	return nil
}
func (m *memPageLayouts) PageKey(_ context.Context, entityID string) (string, error) {
	return m.pages[entityID], nil
}
func (m *memPageLayouts) CampaignPageKeys(_ context.Context, _ string) (map[string]string, error) {
	return m.pages, nil
}
func (m *memPageLayouts) SetPageKey(_ context.Context, _, entityID, key string) error {
	if key == "" {
		delete(m.pages, entityID)
	} else {
		m.pages[entityID] = key
	}
	return nil
}

// layoutOf is a one-block layout, so a test can tell layouts apart by block.
func layoutOf(block string) EntityTypeLayout {
	return EntityTypeLayout{Rows: []TemplateRow{{ID: "r1", Columns: []TemplateColumn{{ID: "c1", Width: 12, Blocks: []TemplateBlock{{ID: "b1", Type: block}}}}}}}
}

func heroAndNPC() PageLayouts {
	return PageLayouts{
		MainName: "NPC",
		Claimed:  "hero",
		Extra:    []PageLayout{{Key: "hero", Name: "Hero", Layout: layoutOf(BlockGameSystemSheet)}},
	}
}

func TestPageLayouts_Check(t *testing.T) {
	many := PageLayouts{}
	for i := 0; i <= MaxPageLayouts; i++ {
		many.Extra = append(many.Extra, PageLayout{Key: "l" + string(rune('a'+i)), Name: "L" + string(rune('a'+i))})
	}
	tests := []struct {
		name string
		p    PageLayouts
		ok   bool
	}{
		{"nothing", PageLayouts{}, true},
		{"a hero and an NPC layout", heroAndNPC(), true},
		{"too many", many, false},
		{"a key that is not short lowercase", PageLayouts{Extra: []PageLayout{{Key: "Hero!", Name: "Hero"}}}, false},
		{"the main key taken", PageLayouts{Extra: []PageLayout{{Key: "main", Name: "Other"}}}, false},
		{"two layouts share a key", PageLayouts{Extra: []PageLayout{{Key: "a", Name: "A"}, {Key: "a", Name: "B"}}}, false},
		{"a name shared with the main one, any case", PageLayouts{MainName: "Hero", Extra: []PageLayout{{Key: "a", Name: "hero"}}}, false},
		{"a layout with no name", PageLayouts{Extra: []PageLayout{{Key: "a", Name: " "}}}, false},
		{"a name on two lines", PageLayouts{Extra: []PageLayout{{Key: "a", Name: "A\nB"}}}, false},
		{"a name too long", PageLayouts{Extra: []PageLayout{{Key: "a", Name: strings.Repeat("x", 41)}}}, false},
		{"claimed pages start on a layout it does not have", PageLayouts{Claimed: "gone", Extra: []PageLayout{{Key: "a", Name: "A"}}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.p.Check()
			if tt.ok && err != nil {
				t.Fatalf("Check() = %v, want ok", err)
			}
			if !tt.ok {
				assertAppError(t, err, http.StatusBadRequest)
			}
		})
	}
}

func TestPageLayouts_WornKey(t *testing.T) {
	owner := "u1"
	npc := &Entity{ID: "npc"}
	claimed := &Entity{ID: "hero", OwnerUserID: &owner}
	playable := &Entity{ID: "free", Playable: true}
	tests := []struct {
		name   string
		p      PageLayouts
		e      *Entity
		chosen string
		want   string
	}{
		{"an NPC wears the main layout", heroAndNPC(), npc, "", MainLayoutKey},
		{"a claimed character starts on the hero layout", heroAndNPC(), claimed, "", "hero"},
		{"a playable character starts on it too", heroAndNPC(), playable, "", "hero"},
		{"the Director's pick wins over that", heroAndNPC(), claimed, MainLayoutKey, MainLayoutKey},
		{"an NPC the Director dressed as a hero", heroAndNPC(), npc, "hero", "hero"},
		{"a pick of a removed layout falls back", heroAndNPC(), npc, "gone", MainLayoutKey},
		{"no claimed layout keeps the main one", PageLayouts{Extra: heroAndNPC().Extra}, claimed, "", MainLayoutKey},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.WornKey(tt.e, tt.chosen); got != tt.want {
				t.Errorf("WornKey = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPageLayouts_KeyByName(t *testing.T) {
	et := &EntityType{Name: "Character"}
	tests := []struct {
		name, in, want string
		ok             bool
	}{
		{"the main layout by its name", "npc", MainLayoutKey, true},
		{"the main layout by main", "Main", MainLayoutKey, true},
		{"a named layout, any case", " HERO ", "hero", true},
		{"a named layout by its key", "hero", "hero", true},
		{"a layout it does not have", "Villain", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := heroAndNPC().KeyByName(et, tt.in)
			if got != tt.want || ok != tt.ok {
				t.Errorf("KeyByName(%q) = %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.ok)
			}
		})
	}
	if got := (PageLayouts{}).MainLabel(et); got != "Character" {
		t.Errorf("an unnamed main layout reads %q, want the category's name", got)
	}
}

func TestPageLayoutService_SetPageLayout(t *testing.T) {
	owner := "u1"
	et := &EntityType{ID: 7}
	tests := []struct {
		name      string
		layouts   PageLayouts
		e         *Entity
		key       string
		wantKey   string // what is stored; "" for nothing
		wantError int
	}{
		{"an NPC put on the hero layout", heroAndNPC(), &Entity{ID: "p", EntityTypeID: 7}, "hero", "hero", 0},
		{"an NPC put back on the main one stores nothing", heroAndNPC(), &Entity{ID: "p", EntityTypeID: 7}, MainLayoutKey, "", 0},
		{"a hero kept on the main one remembers that", heroAndNPC(), &Entity{ID: "p", EntityTypeID: 7, OwnerUserID: &owner}, MainLayoutKey, MainLayoutKey, 0},
		{"a layout the category does not have", heroAndNPC(), &Entity{ID: "p", EntityTypeID: 7}, "villain", "", http.StatusBadRequest},
		{"a page of another category", heroAndNPC(), &Entity{ID: "p", EntityTypeID: 8}, "hero", "", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMemPageLayouts()
			repo.types[7] = tt.layouts
			err := NewPageLayoutService(repo).SetPageLayout(context.Background(), tt.e, et, tt.key)
			if tt.wantError != 0 {
				assertAppError(t, err, tt.wantError)
			} else if err != nil {
				t.Fatal(err)
			}
			if got := repo.pages["p"]; got != tt.wantKey {
				t.Errorf("stored %q, want %q", got, tt.wantKey)
			}
		})
	}
}

// TestWornLayout_OneCategoryHoldsHeroesAndNPCs pins what the single layout
// per category lost: in one Characters category a player's hero and an NPC
// are drawn from different layouts, and only the hero's is a named one (the
// game system's renderer stands aside for it; the sheet is a block there).
func TestWornLayout_OneCategoryHoldsHeroesAndNPCs(t *testing.T) {
	owner := "u1"
	et := &EntityType{ID: 7, Name: "Character", Layout: layoutOf("title")}
	repo := newMemPageLayouts()
	repo.types[7] = heroAndNPC()
	repo.pages["kept-main"] = MainLayoutKey
	h := &Handler{pageLayouts: NewPageLayoutService(repo)}
	tests := []struct {
		name      string
		e         *Entity
		wantBlock string
		wantNamed bool
		wantName  string
	}{
		{"an NPC", &Entity{ID: "npc", EntityTypeID: 7}, "title", false, "NPC"},
		{"a claimed hero", &Entity{ID: "hero", EntityTypeID: 7, OwnerUserID: &owner}, BlockGameSystemSheet, true, "Hero"},
		{"a hero the Director kept on the main layout", &Entity{ID: "kept-main", EntityTypeID: 7, OwnerUserID: &owner}, "title", false, "NPC"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ctx := h.wornLayout(context.Background(), nil, tt.e, et)
			if b := got.Layout.Rows[0].Columns[0].Blocks[0].Type; b != tt.wantBlock {
				t.Errorf("drawn with %q, want %q", b, tt.wantBlock)
			}
			if wearsNamedLayout(ctx) != tt.wantNamed {
				t.Errorf("wearsNamedLayout = %v, want %v", !tt.wantNamed, tt.wantNamed)
			}
			v, ok := wornLayoutFrom(ctx)
			if !ok || v.Info.Name != tt.wantName || len(v.Info.Choices) != 2 {
				t.Errorf("chip view = %+v, %v", v, ok)
			}
			if tt.wantNamed && hasEntityShowRenderer(ctx, et) {
				t.Error("a page on a named layout must not be drawn by a game system's renderer")
			}
		})
	}
	if et.Layout.Rows[0].Columns[0].Blocks[0].Type != "title" {
		t.Error("wearing a named layout changed the category itself")
	}

	// A category that never added a layout is drawn as it always was.
	plain := &EntityType{ID: 9, Layout: layoutOf("title")}
	got, ctx := h.wornLayout(context.Background(), nil, &Entity{ID: "x", EntityTypeID: 9}, plain)
	if got != plain {
		t.Error("a category with one layout should come back as it is")
	}
	if _, ok := wornLayoutFrom(ctx); ok {
		t.Error("a category with one layout shows no layout chip")
	}
}

func TestApplyCustomizeCategories_PageLayouts(t *testing.T) {
	good := heroAndNPC()
	badBlock := PageLayouts{Extra: []PageLayout{{Key: "hero", Name: "Hero", Layout: layoutOf("no_such_block")}}}
	badName := PageLayouts{Extra: []PageLayout{{Key: "hero", Name: ""}}}
	tests := []struct {
		name     string
		items    map[string]CustomizeCategoryItem
		wantSave bool
		wantCode int
	}{
		{"named layouts are saved", map[string]CustomizeCategoryItem{"1": {PageLayouts: &good}}, true, 0},
		{"a layout with a block that does not exist stops the Save", map[string]CustomizeCategoryItem{"1": {PageLayouts: &badBlock}}, false, http.StatusBadRequest},
		{"a layout with no name stops the Save", map[string]CustomizeCategoryItem{"1": {PageLayouts: &badName}}, false, http.StatusBadRequest},
		{"a new category cannot carry them", map[string]CustomizeCategoryItem{"new-a": {New: true, Name: "A", NamePlural: "As", PageLayouts: &good}}, false, http.StatusBadRequest},
		{"a deleted category cannot carry them", map[string]CustomizeCategoryItem{"1": {Delete: true, PageLayouts: &good}}, false, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, svc := newCategoryWorld(typ(1, "c1", "Characters", nil))
			reg := NewBlockRegistry()
			RegisterCoreBlocks(reg)
			svc.(*entityService).SetBlockRegistry(reg)
			repo := newMemPageLayouts()
			svc.(*entityService).SetPageLayoutStore(NewPageLayoutService(repo))
			_, err := svc.ApplyCustomizeCategories(context.Background(), "c1", patchOf(tt.items))
			if tt.wantCode != 0 {
				assertAppError(t, err, tt.wantCode)
			} else if err != nil {
				t.Fatal(err)
			}
			if _, saved := repo.types[1]; saved != tt.wantSave {
				t.Errorf("saved = %v, want %v", saved, tt.wantSave)
			}
		})
	}

	t.Run("without the layout tables the Save says so", func(t *testing.T) {
		_, svc := newCategoryWorld(typ(1, "c1", "Characters", nil))
		_, err := svc.ApplyCustomizeCategories(context.Background(), "c1", patchOf(map[string]CustomizeCategoryItem{"1": {PageLayouts: &good}}))
		assertAppError(t, err, http.StatusBadRequest)
	})
}

func TestPageEditorState_PageLayouts(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner}
	et := &EntityType{ID: 7, Name: "Character", NamePlural: "Characters", Layout: layoutOf("title")}
	var off map[string]any
	if err := json.Unmarshal([]byte(pageEditorState(context.Background(), cc, et)), &off); err != nil {
		t.Fatal(err)
	}
	if _, ok := off["pageLayouts"]; ok {
		t.Error("named layouts must stay out of the editor while their tables are off")
	}
	var on struct {
		PageLayouts struct {
			MainName    string `json:"mainName"`
			MainDefault string `json:"mainDefault"`
			Claimed     string `json:"claimed"`
			Extra       []struct {
				Key, Name string
			} `json:"extra"`
		} `json:"pageLayouts"`
	}
	raw := pageEditorState(withTypeLayouts(context.Background(), heroAndNPC()), cc, et)
	if err := json.Unmarshal([]byte(raw), &on); err != nil {
		t.Fatal(err)
	}
	got := on.PageLayouts
	if got.MainName != "NPC" || got.Claimed != "hero" || !reflect.DeepEqual(len(got.Extra), 1) || got.Extra[0].Name != "Hero" {
		t.Errorf("pageLayouts = %+v", got)
	}
}

// Only the Director team picks a page's layout, never while viewing as a
// player, and only among the page's own category's layouts.
func TestSetPageLayoutAPI(t *testing.T) {
	tests := []struct {
		name       string
		role       campaigns.Role
		dmGranted  bool
		asPlayer   bool
		campaign   string
		body       string
		wantCode   int
		wantStored string
	}{
		{"the owner picks the hero layout by name", campaigns.RoleOwner, false, false, "camp-1", `{"layout":"Hero"}`, http.StatusOK, "hero"},
		{"a co-DM given DM access may too", campaigns.RoleScribe, true, false, "camp-1", `{"layout":"hero"}`, http.StatusOK, "hero"},
		{"a player may not", campaigns.RolePlayer, false, false, "camp-1", `{"layout":"hero"}`, http.StatusForbidden, ""},
		{"a scribe without DM access may not", campaigns.RoleScribe, false, false, "camp-1", `{"layout":"hero"}`, http.StatusForbidden, ""},
		{"not while viewing as a player", campaigns.RoleOwner, false, true, "camp-1", `{"layout":"hero"}`, http.StatusForbidden, ""},
		{"a page of another campaign is not found", campaigns.RoleOwner, false, false, "camp-2", `{"layout":"hero"}`, http.StatusNotFound, ""},
		{"a layout the category lacks", campaigns.RoleOwner, false, false, "camp-1", `{"layout":"Villain"}`, http.StatusBadRequest, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMemPageLayouts()
			repo.types[7] = heroAndNPC()
			h := &Handler{
				service: &stubSvcForFields{
					entity: &Entity{ID: "p1", CampaignID: tt.campaign, EntityTypeID: 7, Name: "Mara"},
					etype:  &EntityType{ID: 7, CampaignID: tt.campaign, Name: "Character"},
				},
				pageLayouts: NewPageLayoutService(repo),
			}
			req := httptest.NewRequest(http.MethodPut, "/campaigns/camp-1/entities/p1/layout", strings.NewReader(tt.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			if tt.asPlayer {
				req.AddCookie(&http.Cookie{Name: "chronicle_view_as_player", Value: "1"})
			}
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(req, rec)
			c.SetParamNames("id", "eid")
			c.SetParamValues("camp-1", "p1")
			c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: tt.role, IsDmGranted: tt.dmGranted})
			err := h.SetPageLayoutAPI(c)
			code := rec.Code
			var ae *apperror.AppError
			if errors.As(err, &ae) {
				code = ae.Code
			} else if err != nil {
				t.Fatalf("handler: %v", err)
			}
			if code != tt.wantCode {
				t.Fatalf("code = %d, want %d", code, tt.wantCode)
			}
			if got := repo.pages["p1"]; got != tt.wantStored {
				t.Errorf("stored %q, want %q", got, tt.wantStored)
			}
		})
	}
}
