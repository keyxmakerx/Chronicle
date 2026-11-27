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

// fakePictures knows which media ids are pictures of which campaign.
type fakePictures struct {
	of  map[string]string // mediaID -> campaignID
	err error
}

func (f fakePictures) IsCampaignPicture(_ context.Context, campaignID, mediaID string) (bool, error) {
	return f.of[mediaID] == campaignID && f.of[mediaID] != "", f.err
}

func TestShopRoomService_SaveRoom_Picture(t *testing.T) {
	shops := fakeShops{shops: map[string]string{"shop-1": "camp-1"}}
	other := "1b9c2f4e-1111-4222-8333-444455556666"
	pictures := fakePictures{of: map[string]string{testPictureID: "camp-1", other: "camp-2"}}
	withPic := func(id string) []byte {
		return withPicture(map[string]any{"id": id, "on": "wall", "fit": "fill"})
	}
	cases := []struct {
		name    string
		checker ShopRoomPictureChecker
		body    []byte
		wantErr int
	}{
		{"this campaign's picture is kept", pictures, withPic(testPictureID), 0},
		{"another campaign's picture is refused", pictures, withPic(other), http.StatusBadRequest},
		{"an unknown picture is refused", pictures, withPic("2b9c2f4e-1111-4222-8333-444455556666"), http.StatusBadRequest},
		{"without a checker a picture is refused", nil, withPic(testPictureID), http.StatusBadRequest},
		{"without a checker a room with no picture saves", nil, withCounter(func(map[string]any) {}), 0},
		{"a failed check refuses", fakePictures{err: errors.New("db down")}, withPic(testPictureID), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeShopRoomRepo{}
			svc := NewShopRoomService(repo, shops, nil)
			if tc.checker != nil {
				SetShopRoomPictures(svc, tc.checker)
			}
			_, err := svc.SaveRoom(context.Background(), "camp-1", "shop-1", "owner-1", tc.body)
			if tc.wantErr == 0 {
				if err != nil {
					t.Fatal(err)
				}
				if repo.upserts != 1 {
					t.Errorf("upserts = %d, want 1", repo.upserts)
				}
				return
			}
			if repo.upserts != 0 {
				t.Errorf("a refused room was stored")
			}
			if tc.wantErr == http.StatusInternalServerError {
				if err == nil || isStatus(err, http.StatusBadRequest) {
					t.Fatalf("want a server error, got %v", err)
				}
				return
			}
			if !isStatus(err, tc.wantErr) {
				t.Fatalf("want %d, got %v", tc.wantErr, err)
			}
		})
	}
}
