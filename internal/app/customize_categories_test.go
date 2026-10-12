package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

type fakeCategoriesApplier struct {
	res entities.CustomizeCategoriesResult
	err error
	got entities.CustomizeCategoriesPatch
}

func (f *fakeCategoriesApplier) ApplyCustomizeCategories(_ context.Context, _ string, in entities.CustomizeCategoriesPatch) (entities.CustomizeCategoriesResult, error) {
	f.got = in
	return f.res, f.err
}

type fakeCategoryMenu struct {
	calls []struct {
		id      int
		visible bool
	}
	failFor int
}

func (f *fakeCategoryMenu) SetCategoryMenuVisibility(_ context.Context, _ string, id int, visible bool) error {
	if id == f.failFor {
		return apperror.NewInternal(errors.New("db down"))
	}
	f.calls = append(f.calls, struct {
		id      int
		visible bool
	}{id, visible})
	return nil
}

func ownerCC() *campaigns.CampaignContext {
	return &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner}
}

func TestCategoriesSection_Apply(t *testing.T) {
	tests := []struct {
		name          string
		section       string
		res           entities.CustomizeCategoriesResult
		applyErr      error
		failMenuFor   int
		wantRemaining string // "" = nil
		wantErrCode   int
		wantMenu      []struct {
			id      int
			visible bool
		}
	}{
		{
			name:    "everything applies, menu follows by new id",
			section: `{"items":{"new-a":{"new":true,"name":"Ship","name_plural":"Ships","menu":false},"4":{"menu":true}},"_changes":["x"]}`,
			res:     entities.CustomizeCategoriesResult{Created: map[string]int{"new-a": 90}, Applied: []string{"new-a", "4"}},
			wantMenu: []struct {
				id      int
				visible bool
			}{{90, false}, {4, true}},
		},
		{
			name:          "failure keeps the unapplied item and rewrites parents to ids",
			section:       `{"items":{"new-a":{"new":true,"name":"A","name_plural":"As"},"new-b":{"new":true,"name":"B","name_plural":"Bs","parent":"new-a"},"7":{"name":"Seven"}},"_changes":["c"]}`,
			res:           entities.CustomizeCategoriesResult{Created: map[string]int{"new-a": 90}, Applied: []string{"new-a"}},
			applyErr:      apperror.NewBadRequest("color must be a valid hex value like #ff0000"),
			wantRemaining: `{"items":{"7":{"name":"Seven"},"new-b":{"new":true,"name":"B","name_plural":"Bs","parent":"90"}},"_changes":["c"]}`,
			wantErrCode:   http.StatusBadRequest,
		},
		{
			name:          "a created but unapplied category is addressed by id from then on",
			section:       `{"items":{"new-a":{"new":true,"name":"A","name_plural":"As","copy_from":2,"menu":false}}}`,
			res:           entities.CustomizeCategoriesResult{Created: map[string]int{"new-a": 90}, Applied: nil},
			applyErr:      apperror.NewInternal(nil),
			wantRemaining: `{"items":{"90":{"name":"A","name_plural":"As","menu":false}}}`,
			wantErrCode:   http.StatusInternalServerError,
		},
		{
			name:        "nothing applied keeps the raw section",
			section:     `{"items":{"7":{"name":"Seven"}}}`,
			res:         entities.CustomizeCategoriesResult{Created: map[string]int{}},
			applyErr:    apperror.NewNotFound("category not found"),
			wantErrCode: http.StatusNotFound,
		},
		{
			name:          "a menu failure keeps only the menu part",
			section:       `{"items":{"4":{"name":"Four","menu":false},"5":{"menu":true}}}`,
			res:           entities.CustomizeCategoriesResult{Created: map[string]int{}, Applied: []string{"4", "5"}},
			failMenuFor:   4,
			wantRemaining: `{"items":{"4":{"menu":false}}}`,
			wantErrCode:   http.StatusInternalServerError,
			wantMenu: []struct {
				id      int
				visible bool
			}{{5, true}},
		},
		{
			name:     "a deleted category gets no menu call",
			section:  `{"items":{"4":{"delete":true,"menu":false}}}`,
			res:      entities.CustomizeCategoriesResult{Created: map[string]int{}, Applied: []string{"4"}},
			wantMenu: nil,
		},
		{name: "empty items is nothing to do", section: `{"items":{}}`},
		{name: "no items key is nothing to do", section: `{"_changes":[]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ent := &fakeCategoriesApplier{res: tt.res, err: tt.applyErr}
			menu := &fakeCategoryMenu{failFor: tt.failMenuFor}
			sec := newCategoriesSection(ent, menu)
			rem, err := sec.Apply(context.Background(), ownerCC(), "u1", json.RawMessage(tt.section))

			if tt.wantErrCode != 0 {
				if apperror.SafeCode(err) != tt.wantErrCode {
					t.Fatalf("err = %v, want code %d", err, tt.wantErrCode)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantRemaining == "" {
				if rem != nil {
					t.Errorf("remaining = %s, want nil", rem)
				}
			} else if !jsonEqual(t, rem, tt.wantRemaining) {
				t.Errorf("remaining = %s\nwant       %s", rem, tt.wantRemaining)
			}
			if !reflect.DeepEqual(menu.calls, tt.wantMenu) {
				t.Errorf("menu calls = %v, want %v", menu.calls, tt.wantMenu)
			}
		})
	}
}

func TestCategoriesSection_PassesItemsAndFencesRole(t *testing.T) {
	ent := &fakeCategoriesApplier{res: entities.CustomizeCategoriesResult{Created: map[string]int{}}}
	sec := newCategoriesSection(ent, &fakeCategoryMenu{})
	if sec.Key != "categories" {
		t.Fatalf("key = %q", sec.Key)
	}
	scribe := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleScribe}
	if _, err := sec.Apply(context.Background(), scribe, "u", json.RawMessage(`{"items":{"1":{"delete":true}}}`)); apperror.SafeCode(err) != http.StatusForbidden {
		t.Errorf("scribe err = %v", err)
	}
	if ent.got.Items != nil {
		t.Error("a scribe's draft reached the entity service")
	}
	if _, err := sec.Apply(context.Background(), ownerCC(), "u", json.RawMessage(`{"items":{"3":{"name":"x","_junk":1}}}`)); err != nil {
		t.Fatal(err)
	}
	if ent.got.Items["3"].Name != "x" {
		t.Errorf("items = %+v", ent.got.Items)
	}
	if _, err := sec.Apply(context.Background(), ownerCC(), "u", json.RawMessage(`[`)); apperror.SafeCode(err) != http.StatusBadRequest {
		t.Errorf("malformed err = %v", err)
	}
}

func jsonEqual(t *testing.T, got json.RawMessage, want string) bool {
	t.Helper()
	var a, b any
	if err := json.Unmarshal(got, &a); err != nil {
		t.Fatalf("remaining is not JSON: %v (%s)", err, got)
	}
	if err := json.Unmarshal([]byte(want), &b); err != nil {
		t.Fatalf("bad expectation: %v", err)
	}
	return reflect.DeepEqual(a, b)
}
