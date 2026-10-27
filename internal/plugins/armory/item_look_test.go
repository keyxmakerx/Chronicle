package armory

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

func TestNormalizeItemLook(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    string
		cleared bool
		wantErr bool
	}{
		{"look only", `{"look":"ember"}`, `{"look":"ember"}`, false, false},
		{"every field", `{"look":"frost","tint":"#ffd66b","sound":"bell","words":" 4 cold "}`, `{"look":"frost","tint":"#ffd66b","sound":"bell","words":"4 cold"}`, false, false},
		{"silent", `{"look":"rot","sound":"none"}`, `{"look":"rot","sound":"none"}`, false, false},
		{"empty look clears", `{"look":""}`, ``, true, false},
		{"unknown look", `{"look":"fireball"}`, ``, false, true},
		{"colour off the palette", `{"look":"ember","tint":"#123456"}`, ``, false, true},
		{"css in the colour", `{"look":"ember","tint":"red;background:url(x)"}`, ``, false, true},
		{"unknown sound", `{"look":"ember","sound":"scream"}`, ``, false, true},
		{"words too long", `{"look":"ember","words":"` + strings.Repeat("é", 25) + `"}`, ``, false, true},
		{"words at the limit", `{"look":"ember","words":"` + strings.Repeat("é", 24) + `"}`, `{"look":"ember","words":"` + strings.Repeat("é", 24) + `"}`, false, false},
		{"control characters", `{"look":"ember","words":"a\u0007b"}`, ``, false, true},
		{"unknown field", `{"look":"ember","script":"x"}`, ``, false, true},
		{"trailing data", `{"look":"ember"}{"look":"frost"}`, ``, false, true},
		{"not json", `look=ember`, ``, false, true},
		{"too large", `{"look":"ember","words":"` + strings.Repeat(" ", maxItemLookBytes) + `"}`, ``, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, cleared, err := NormalizeItemLook([]byte(tc.raw))
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, want error %v", err, tc.wantErr)
			}
			if tc.wantErr && !isStatus(err, http.StatusBadRequest) {
				t.Fatalf("err = %v, want 400", err)
			}
			if cleared != tc.cleared {
				t.Fatalf("cleared = %v, want %v", cleared, tc.cleared)
			}
			if string(got) != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

type fakeLookRepo struct {
	look    json.RawMessage
	upserts int
	deletes int
	savedBy string
}

func (f *fakeLookRepo) Get(context.Context, string, string) (json.RawMessage, error) { return f.look, nil }
func (f *fakeLookRepo) Upsert(_ context.Context, _, _, userID string, l json.RawMessage) error {
	f.upserts++
	f.savedBy = userID
	f.look = l
	return nil
}
func (f *fakeLookRepo) Delete(context.Context, string, string) error {
	f.deletes++
	f.look = nil
	return nil
}

// fakeItems knows which entity ids are items in which campaign.
type fakeItems map[string]string

func (f fakeItems) IsItemInCampaign(_ context.Context, campaignID, entityID string) (bool, error) {
	return f[entityID] != "" && f[entityID] == campaignID, nil
}

func TestItemLookService_GetLook(t *testing.T) {
	stored := json.RawMessage(`{"look":"ember"}`)
	items := fakeItems{"sword": "camp-1", "ring": "camp-1"}
	cases := []struct {
		name     string
		campaign string
		entity   string
		role     int
		vis      EntityVisibilityFilter
		wantNil  bool
		wantCode int
	}{
		{"player sees a visible item", "camp-1", "sword", permissions.RolePlayer, &fakeRoomVis{viewable: map[string]bool{"sword": true}}, false, 0},
		{"hidden item is not found", "camp-1", "ring", permissions.RolePlayer, &fakeRoomVis{viewable: map[string]bool{}}, true, http.StatusNotFound},
		{"owner sees a hidden item", "camp-1", "ring", permissions.RoleOwner, &fakeRoomVis{viewable: map[string]bool{}}, false, 0},
		{"not an item is not found", "camp-1", "shop", permissions.RoleOwner, nil, true, http.StatusNotFound},
		{"other campaign is not found", "camp-2", "sword", permissions.RoleOwner, nil, true, http.StatusNotFound},
		{"no visibility filter fails closed", "camp-1", "sword", permissions.RolePlayer, nil, true, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewItemLookService(&fakeLookRepo{look: stored}, items, tc.vis)
			got, err := svc.GetLook(context.Background(), tc.campaign, tc.entity, tc.role, "u1")
			if tc.wantCode != 0 {
				if !isStatus(err, tc.wantCode) {
					t.Fatalf("err = %v, want %d", err, tc.wantCode)
				}
			} else if err != nil {
				t.Fatalf("unexpected err %v", err)
			}
			if (got == nil) != tc.wantNil {
				t.Fatalf("got %s, want nil %v", got, tc.wantNil)
			}
		})
	}
}

func TestItemLookService_SaveLook(t *testing.T) {
	items := fakeItems{"sword": "camp-1"}

	repo := &fakeLookRepo{}
	svc := NewItemLookService(repo, items, nil)
	got, err := svc.SaveLook(context.Background(), "camp-1", "sword", "owner", []byte(`{"look":"storm","words":"7 lightning"}`))
	if err != nil || string(got) != `{"look":"storm","words":"7 lightning"}` || repo.upserts != 1 || repo.savedBy != "owner" {
		t.Fatalf("save: got %s err %v upserts %d by %q", got, err, repo.upserts, repo.savedBy)
	}

	got, err = svc.SaveLook(context.Background(), "camp-1", "sword", "owner", []byte(`{"look":""}`))
	if err != nil || got != nil || repo.deletes != 1 || repo.look != nil {
		t.Fatalf("clear: got %s err %v deletes %d", got, err, repo.deletes)
	}

	if _, err := svc.SaveLook(context.Background(), "camp-1", "sword", "owner", []byte(`{"look":"nope"}`)); !isStatus(err, http.StatusBadRequest) {
		t.Fatalf("bad look: err = %v, want 400", err)
	}
	if _, err := svc.SaveLook(context.Background(), "camp-2", "sword", "owner", []byte(`{"look":"ember"}`)); !isStatus(err, http.StatusNotFound) {
		t.Fatalf("other campaign: err = %v, want 404", err)
	}
	if repo.upserts != 1 {
		t.Fatalf("a refused save wrote: upserts = %d", repo.upserts)
	}
}
