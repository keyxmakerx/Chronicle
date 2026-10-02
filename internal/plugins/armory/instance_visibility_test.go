package armory

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// AddItem must refuse an entity outside the campaign, and fail closed when no
// checker is wired, before anything reaches the repository.
func TestAddItem_EntityCampaignCheck(t *testing.T) {
	inst := func(_ context.Context, _ int) (*InventoryInstance, error) {
		return &InventoryInstance{ID: 1, CampaignID: "camp-1"}, nil
	}
	tests := []struct {
		name      string
		checker   EntityCampaignChecker
		wantCode  int // 0 = success, -1 = any non-app error
		wantAdded bool
	}{
		{"in campaign", fakeEntityCampaign{inCampaign: map[string]bool{"e1": true}}, 0, true},
		{"other campaign", fakeEntityCampaign{inCampaign: map[string]bool{}}, http.StatusNotFound, false},
		{"entity missing", fakeEntityCampaign{errs: map[string]error{"e1": apperror.NewNotFound("entity")}}, http.StatusNotFound, false},
		{"lookup failure", fakeEntityCampaign{errs: map[string]error{"e1": errors.New("db down")}}, -1, false},
		{"no checker wired", nil, http.StatusInternalServerError, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			added := false
			repo := &mockInstanceRepo{
				findByIDFn: inst,
				addItemFn: func(context.Context, int, string, int) error {
					added = true
					return nil
				},
			}
			svc := &instanceService{repo: repo, entityCampaign: tc.checker}
			err := svc.AddItem(context.Background(), "camp-1", 1, "e1")
			switch {
			case tc.wantCode == 0 && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.wantCode == -1 && err == nil:
				t.Fatal("expected lookup failure to surface")
			case tc.wantCode > 0:
				var ae *apperror.AppError
				if !errors.As(err, &ae) || ae.Code != tc.wantCode {
					t.Fatalf("err = %v, want app error %d", err, tc.wantCode)
				}
			}
			if added != tc.wantAdded {
				t.Errorf("repo.AddItem called = %v, want %v", added, tc.wantAdded)
			}
		})
	}
}

// ListInstances must count only what the viewer may see; an Owner sees the
// full linked count.
func TestListInstances_VisibleCounts(t *testing.T) {
	repo := &mockInstanceRepo{
		listByCampaignFn: func(context.Context, string) ([]InventoryInstance, error) {
			return []InventoryInstance{{ID: 1, ItemCount: 99}, {ID: 2, ItemCount: 99}}, nil
		},
		entityIDsFn: func(context.Context, string) (map[int][]string, error) {
			return map[int][]string{1: {"a", "b", "c"}, 2: {"c"}}, nil
		},
	}
	tests := []struct {
		name string
		role int
		vis  EntityVisibilityFilter
		want []int
	}{
		{"owner unrestricted", permissions.RoleOwner, nil, []int{3, 1}},
		{"player sees subset", permissions.RolePlayer, &mockVisibilityFilter{viewable: map[string]bool{"a": true, "c": true}}, []int{2, 1}},
		{"player sees none", permissions.RolePlayer, &mockVisibilityFilter{viewable: map[string]bool{}}, []int{0, 0}},
		{"no filter fails closed", permissions.RolePlayer, nil, []int{0, 0}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &instanceService{repo: repo, visibility: tc.vis}
			got, err := svc.ListInstances(context.Background(), "camp-1", tc.role, "u1")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for i, w := range tc.want {
				if got[i].ItemCount != w {
					t.Errorf("instance %d count = %d, want %d", got[i].ID, got[i].ItemCount, w)
				}
			}
		})
	}
}
