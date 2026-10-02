// gallery_ui_test.go covers the gallery's tag filter source, the per-item
// collection membership listing, collection rename validation, the Scribe
// gate on collection management, and the inline handler escaping contract.
package armory

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func TestListTagOptions(t *testing.T) {
	allTags := map[string][]TagInfo{
		"a": {{ID: 1, Name: "Magic", Slug: "magic"}, {ID: 2, Name: "gm-only", Slug: "gm-only"}},
		"b": {{ID: 1, Name: "Magic", Slug: "magic"}, {ID: 3, Name: "Cursed", Slug: "cursed"}},
		"c": {{ID: 4, Name: "Hidden item tag", Slug: "hidden-item"}},
	}
	tests := []struct {
		name       string
		role       int
		viewable   map[string]bool
		noLister   bool
		wantSlugs  []string
		wantDmOnly bool
	}{
		{name: "owner sees every tag, deduped and name-sorted", role: permissions.RoleOwner, wantSlugs: []string{"cursed", "gm-only", "hidden-item", "magic"}, wantDmOnly: true},
		{name: "player: lister told to exclude dm_only", role: permissions.RolePlayer, viewable: map[string]bool{"a": true, "b": true}, wantSlugs: []string{"cursed", "magic"}, wantDmOnly: false},
		{name: "player: tags of invisible items are not offered", role: permissions.RolePlayer, viewable: map[string]bool{"b": true}, wantSlugs: []string{"cursed", "magic"}, wantDmOnly: false},
		{name: "scribe may see dm_only tags", role: permissions.RoleScribe, viewable: map[string]bool{"a": true, "b": true, "c": true}, wantSlugs: []string{"cursed", "gm-only", "hidden-item", "magic"}, wantDmOnly: true},
		{name: "no tag lister means no options", role: permissions.RoleOwner, noLister: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockArmoryRepo{listIDsFn: func(context.Context, string, []int, ItemListOptions) ([]string, error) {
				return []string{"a", "b", "c"}, nil
			}}
			tf := &mockTypeFinder{findIDsFn: func(context.Context, string) ([]int, error) { return []int{1}, nil }}
			vf := &mockVisibilityFilter{viewable: tt.viewable}
			svc := newTestArmoryService(repo, tf, vf)

			var gotDmOnly bool
			if !tt.noLister {
				svc.SetTagLister(&mockTagLister{listFn: func(_ context.Context, ids []string, includeDmOnly bool) (map[string][]TagInfo, error) {
					gotDmOnly = includeDmOnly
					out := map[string][]TagInfo{}
					for _, id := range ids {
						for _, ti := range allTags[id] {
							// Mimic the real lister: dm_only tag is named "gm-only".
							if ti.Slug == "gm-only" && !includeDmOnly {
								continue
							}
							out[id] = append(out[id], ti)
						}
					}
					return out, nil
				}})
			}

			got, err := svc.ListTagOptions(context.Background(), "camp-1", tt.role, "user-1")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var slugs []string
			for _, g := range got {
				slugs = append(slugs, g.Slug)
			}
			want := tt.wantSlugs
			if strings.Join(slugs, ",") != strings.Join(want, ",") {
				t.Errorf("slugs = %v, want %v", slugs, want)
			}
			if !tt.noLister && gotDmOnly != tt.wantDmOnly {
				t.Errorf("includeDmOnly = %v, want %v", gotDmOnly, tt.wantDmOnly)
			}
		})
	}
}

func TestListCollectionsForItem(t *testing.T) {
	two := []InventoryInstance{
		{ID: 1, Name: "Loot", Icon: "fa-box", Color: "#111111"},
		{ID: 2, Name: "Shop", Icon: "fa-store", Color: "#222222"},
	}
	tests := []struct {
		name     string
		entityID string
		links    map[int][]string
		wantHas  []bool
		wantErr  bool
	}{
		{name: "ticks only collections holding the item", entityID: "entity-1", links: map[int][]string{1: {"entity-1", "x"}, 2: {"x"}}, wantHas: []bool{true, false}},
		{name: "item in no collection", entityID: "entity-1", links: map[int][]string{1: {"x"}}, wantHas: []bool{false, false}},
		{name: "item in both", entityID: "entity-1", links: map[int][]string{1: {"entity-1"}, 2: {"entity-1"}}, wantHas: []bool{true, true}},
		{name: "entity from another campaign is not found", entityID: "foreign", links: nil, wantErr: true},
		{name: "empty entity id rejected", entityID: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockInstanceRepo{
				listByCampaignFn: func(context.Context, string) ([]InventoryInstance, error) {
					return append([]InventoryInstance(nil), two...), nil
				},
				entityIDsFn: func(context.Context, string) (map[int][]string, error) { return tt.links, nil },
			}
			svc := newTestInstanceService(repo)
			got, err := svc.ListCollectionsForItem(context.Background(), "camp-1", permissions.RoleOwner, "u", tt.entityID)
			if tt.wantErr {
				if err == nil || !isAppError(err) {
					t.Fatalf("expected app error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.wantHas) {
				t.Fatalf("got %d collections, want %d", len(got), len(tt.wantHas))
			}
			for i, c := range got {
				if c.HasItem != tt.wantHas[i] {
					t.Errorf("collection %d HasItem = %v, want %v", c.ID, c.HasItem, tt.wantHas[i])
				}
			}
		})
	}
}

func TestUpdateInstance_RenameValidation(t *testing.T) {
	tests := []struct {
		name    string
		input   CreateInstanceInput
		wantErr bool
	}{
		{name: "valid rename keeping other fields", input: CreateInstanceInput{Name: "New name", Description: "d", Icon: "fa-box", Color: "#6b7280"}},
		{name: "blank name", input: CreateInstanceInput{Name: "   "}, wantErr: true},
		{name: "name too long", input: CreateInstanceInput{Name: strings.Repeat("x", 101)}, wantErr: true},
		{name: "bad colour", input: CreateInstanceInput{Name: "ok", Color: "red"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotName string
			repo := &mockInstanceRepo{
				findByIDFn: func(context.Context, int) (*InventoryInstance, error) {
					return &InventoryInstance{ID: 1, CampaignID: "camp-1"}, nil
				},
				updateFn: func(_ context.Context, _ int, name, _, _, _, _ string) error {
					gotName = name
					return nil
				},
			}
			err := newTestInstanceService(repo).UpdateInstance(context.Background(), "camp-1", 1, tt.input)
			if tt.wantErr {
				if err == nil || !isAppError(err) {
					t.Fatalf("expected app error, got %v", err)
				}
				if gotName != "" {
					t.Error("repo was written despite validation failure")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotName != "New name" {
				t.Errorf("stored name = %q", gotName)
			}
		})
	}
}

// TestInstanceManagementRoleGate pins that collection management is open to
// Scribes (and Owners) but not Players, which is the gate routes.go applies
// to the manage/create/update/delete/item-membership routes.
func TestInstanceManagementRoleGate(t *testing.T) {
	tests := []struct {
		name    string
		role    campaigns.Role
		allowed bool
	}{
		{"owner", campaigns.RoleOwner, true},
		{"scribe", campaigns.RoleScribe, true},
		{"player", campaigns.RolePlayer, false},
		{"non-member", campaigns.RoleNone, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			c := e.NewContext(httptest.NewRequest(http.MethodPut, "/", nil), httptest.NewRecorder())
			c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: tt.role})
			called := false
			err := campaigns.RequireRole(campaigns.RoleScribe)(func(c echo.Context) error { called = true; return nil })(c)
			if tt.allowed && (err != nil || !called) {
				t.Errorf("role %v should pass the Scribe gate (err=%v)", tt.role, err)
			}
			if !tt.allowed && (err == nil || called) {
				t.Errorf("role %v should be rejected by the Scribe gate", tt.role)
			}
		})
	}
}

// TestInlineHandlers_AttributeSafe pins the swap-safety contract: handlers
// are IIFEs with no double quote (the attribute delimiter), no templ script
// helpers, and hostile names stay inside their JS string literal.
func TestInlineHandlers_AttributeSafe(t *testing.T) {
	hostile := `O'Brien "the" \ </script>`
	tests := []struct {
		name string
		call string
	}{
		{"collection menu", collectionMenuOnClick("camp-1", "ent'1", "pop-1").Call},
		{"rename", renameInstanceOnClick("camp-1", 7, hostile, hostile, "fa-box", "#6b7280").Call},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.HasPrefix(tt.call, "(function(") {
				t.Error("handler must be an IIFE")
			}
			if strings.Contains(tt.call, `"`) {
				t.Error("literal double quote would terminate the onclick attribute")
			}
			if strings.Contains(tt.call, "__templ_") {
				t.Error("must not depend on templ script helpers")
			}
			if strings.Contains(tt.call, "</script>") {
				t.Error("hostile value escaped its JS string literal")
			}
		})
	}
}
