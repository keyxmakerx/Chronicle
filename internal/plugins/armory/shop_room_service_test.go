package armory

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

type fakeShopRoomRepo struct {
	layout   json.RawMessage
	upserts  int
	savedBy  string
	getErr   error
	lastCamp string
}

func (f *fakeShopRoomRepo) Get(_ context.Context, campaignID, _ string) (json.RawMessage, error) {
	f.lastCamp = campaignID
	return f.layout, f.getErr
}

func (f *fakeShopRoomRepo) Upsert(_ context.Context, _, _, userID string, l json.RawMessage) error {
	f.upserts++
	f.savedBy = userID
	f.layout = l
	return nil
}

// fakeShops knows one shop per campaign id.
type fakeShops struct {
	shops map[string]string // entityID -> campaignID
	err   error
}

func (f fakeShops) IsShopInCampaign(_ context.Context, campaignID, entityID string) (bool, error) {
	return f.shops[entityID] == campaignID && f.shops[entityID] != "", f.err
}

type fakeRoomVis struct {
	viewable map[string]bool
	err      error
	calls    int
}

func (f *fakeRoomVis) FilterViewableEntityIDs(_ context.Context, _ string, ids []string, _ int, _ string) (map[string]bool, error) {
	f.calls++
	out := map[string]bool{}
	for _, id := range ids {
		out[id] = f.viewable[id]
	}
	return out, f.err
}

func isStatus(err error, code int) bool {
	var ae *apperror.AppError
	return errors.As(err, &ae) && ae.Code == code
}

func TestShopRoomService_GetRoom(t *testing.T) {
	stored := json.RawMessage(`{"version":1}`)
	shops := fakeShops{shops: map[string]string{"shop-1": "camp-1", "shop-2": "camp-1"}}

	cases := []struct {
		name     string
		campaign string
		entity   string
		role     int
		vis      *fakeRoomVis
		stored   json.RawMessage
		wantNil  bool
		wantErr  int
	}{
		{"player sees visible shop", "camp-1", "shop-1", permissions.RolePlayer, &fakeRoomVis{viewable: map[string]bool{"shop-1": true}}, stored, false, 0},
		{"no row yet is nil", "camp-1", "shop-1", permissions.RolePlayer, &fakeRoomVis{viewable: map[string]bool{"shop-1": true}}, nil, true, 0},
		{"hidden shop is not found", "camp-1", "shop-2", permissions.RolePlayer, &fakeRoomVis{viewable: map[string]bool{}}, stored, true, http.StatusNotFound},
		{"owner skips the filter", "camp-1", "shop-2", permissions.RoleOwner, &fakeRoomVis{}, stored, false, 0},
		{"wrong campaign", "camp-9", "shop-1", permissions.RoleOwner, &fakeRoomVis{}, stored, true, http.StatusNotFound},
		{"not a shop or missing", "camp-1", "nope", permissions.RoleOwner, &fakeRoomVis{}, stored, true, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewShopRoomService(&fakeShopRoomRepo{layout: tc.stored}, shops, tc.vis)
			got, err := svc.GetRoom(context.Background(), tc.campaign, tc.entity, tc.role, "u1")
			if tc.wantErr != 0 {
				if !isStatus(err, tc.wantErr) {
					t.Fatalf("want %d, got %v", tc.wantErr, err)
				}
				if got != nil {
					t.Errorf("layout leaked on error: %s", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantNil != (got == nil) {
				t.Errorf("got %s", got)
			}
		})
	}

	t.Run("nil filter fails closed for non-owner", func(t *testing.T) {
		svc := NewShopRoomService(&fakeShopRoomRepo{layout: stored}, shops, nil)
		if _, err := svc.GetRoom(context.Background(), "camp-1", "shop-1", permissions.RolePlayer, "u1"); !isStatus(err, http.StatusNotFound) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("filter error is internal", func(t *testing.T) {
		svc := NewShopRoomService(&fakeShopRoomRepo{layout: stored}, shops, &fakeRoomVis{err: errors.New("boom")})
		if _, err := svc.GetRoom(context.Background(), "camp-1", "shop-1", permissions.RolePlayer, "u1"); !isStatus(err, http.StatusInternalServerError) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestShopRoomService_SaveRoom(t *testing.T) {
	shops := fakeShops{shops: map[string]string{"shop-1": "camp-1"}}
	good, _ := json.Marshal(goodRoom())

	cases := []struct {
		name     string
		campaign string
		entity   string
		body     []byte
		wantErr  int
		upserts  int
	}{
		{"owner save upserts", "camp-1", "shop-1", good, 0, 1},
		{"wrong campaign", "camp-2", "shop-1", good, http.StatusNotFound, 0},
		{"not a shop", "camp-1", "other", good, http.StatusNotFound, 0},
		{"invalid layout not stored", "camp-1", "shop-1", []byte(`{}`), http.StatusBadRequest, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeShopRoomRepo{}
			svc := NewShopRoomService(repo, shops, nil)
			got, err := svc.SaveRoom(context.Background(), tc.campaign, tc.entity, "owner-1", tc.body)
			if tc.wantErr != 0 {
				if !isStatus(err, tc.wantErr) {
					t.Fatalf("want %d, got %v", tc.wantErr, err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if repo.upserts != tc.upserts {
				t.Errorf("upserts = %d, want %d", repo.upserts, tc.upserts)
			}
			if tc.upserts == 1 {
				if repo.savedBy != "owner-1" || string(repo.layout) != string(got) {
					t.Errorf("stored %s by %q, returned %s", repo.layout, repo.savedBy, got)
				}
			}
		})
	}
}
