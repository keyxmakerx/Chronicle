package campaigns

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

func TestSetCategoryMenuVisibility(t *testing.T) {
	appItem := SidebarItem{Type: SidebarTypeApp, Slug: "maps", Visible: true}
	tests := []struct {
		name      string
		stored    string
		typeID    int
		visible   bool
		wantWrite bool
		wantItems []SidebarItem
		wantCode  int
	}{
		{
			name:      "hides an existing item and keeps the rest",
			stored:    `{"items":[{"type":"app","slug":"maps","visible":true},{"type":"category","type_id":5,"visible":true,"section":"categories"}],"hidden_entity_ids":["e1"]}`,
			typeID:    5,
			visible:   false,
			wantWrite: true,
			wantItems: []SidebarItem{appItem, {Type: SidebarTypeCategory, TypeID: 5, Visible: false, Section: "categories"}},
		},
		{
			name:      "shows a hidden item",
			stored:    `{"items":[{"type":"category","type_id":5,"visible":false,"section":"categories"}]}`,
			typeID:    5,
			visible:   true,
			wantWrite: true,
			wantItems: []SidebarItem{{Type: SidebarTypeCategory, TypeID: 5, Visible: true, Section: "categories"}},
		},
		{
			name:      "hiding a category with no item adds one in categories",
			stored:    `{"items":[{"type":"app","slug":"maps","visible":true}]}`,
			typeID:    9,
			visible:   false,
			wantWrite: true,
			wantItems: []SidebarItem{appItem, {Type: SidebarTypeCategory, TypeID: 9, Visible: false, Section: "categories"}},
		},
		{
			name:      "showing a category with no item writes nothing",
			stored:    `{"items":[{"type":"app","slug":"maps","visible":true}]}`,
			typeID:    9,
			visible:   true,
			wantWrite: false,
		},
		{
			name:      "already in the wanted state writes nothing",
			stored:    `{"items":[{"type":"category","type_id":5,"visible":true}]}`,
			typeID:    5,
			visible:   true,
			wantWrite: false,
		},
		{
			name:      "legacy model is converted before the write",
			stored:    `{"custom_links":[{"label":"Wiki","url":"https://example.com","section":""}]}`,
			typeID:    3,
			visible:   false,
			wantWrite: true,
		},
		{name: "bad id refused", stored: "", typeID: 0, visible: false, wantCode: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var saved string
			wrote := false
			svc := &campaignService{repo: &mockCampaignRepo{
				findByIDFn: func(_ context.Context, id string) (*Campaign, error) {
					return &Campaign{ID: id, SidebarConfig: tt.stored}, nil
				},
				updateSidebarConfigFn: func(_ context.Context, _, cfg string) error { saved, wrote = cfg, true; return nil },
			}}
			err := svc.SetCategoryMenuVisibility(context.Background(), "camp-1", tt.typeID, tt.visible)
			if tt.wantCode != 0 {
				if apperror.SafeCode(err) != tt.wantCode {
					t.Fatalf("err = %v, want code %d", err, tt.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if wrote != tt.wantWrite {
				t.Fatalf("wrote = %v, want %v", wrote, tt.wantWrite)
			}
			if !wrote {
				return
			}
			var got SidebarConfig
			if err := json.Unmarshal([]byte(saved), &got); err != nil {
				t.Fatal(err)
			}
			if tt.wantItems != nil && !reflect.DeepEqual(got.Items, tt.wantItems) {
				t.Errorf("items = %+v, want %+v", got.Items, tt.wantItems)
			}
			if tt.name == "hides an existing item and keeps the rest" && !reflect.DeepEqual(got.HiddenEntityIDs, []string{"e1"}) {
				t.Errorf("hidden entities = %v, want preserved", got.HiddenEntityIDs)
			}
			if tt.name == "legacy model is converted before the write" {
				var link, cat bool
				for _, it := range got.Items {
					link = link || it.Type == SidebarTypeLink
					cat = cat || (it.Type == SidebarTypeCategory && it.TypeID == 3 && !it.Visible)
				}
				if !link || !cat {
					t.Errorf("legacy link lost or category missing: %+v", got.Items)
				}
			}
		})
	}
}

func TestCategoryMenuVisible(t *testing.T) {
	h := &Handler{}
	cc := &CampaignContext{Campaign: &Campaign{SidebarConfig: `{"items":[
		{"type":"category","type_id":1,"visible":true},
		{"type":"category","type_id":2,"visible":false},
		{"type":"category","type_id":2,"visible":true},
		{"type":"app","slug":"maps","visible":false}]}`}}
	got := h.CategoryMenuVisible(cc)
	want := map[int]bool{1: true, 2: false}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	// A category with no item is absent, which nav treats as visible.
	if v, ok := got[3]; ok || v {
		t.Errorf("an unlisted category must be absent, got %v %v", v, ok)
	}
	if empty := h.CategoryMenuVisible(&CampaignContext{Campaign: &Campaign{}}); len(empty) != 0 {
		t.Errorf("empty config gave %v", empty)
	}
}
