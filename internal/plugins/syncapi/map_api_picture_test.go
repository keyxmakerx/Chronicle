package syncapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
	"github.com/keyxmakerx/chronicle/internal/plugins/media"
)

const pictureMediaID = "media-original-0001"

// pictureRepo is the one map the sync API reads, with a picture.
type pictureRepo struct{ maps.MapRepository }

func (pictureRepo) GetMap(_ context.Context, id string) (*maps.Map, error) {
	pic := pictureMediaID
	return &maps.Map{ID: id, CampaignID: "camp-1", ImageID: &pic, ImageWidth: 100, ImageHeight: 80}, nil
}
func (r pictureRepo) ListMaps(ctx context.Context, _ string) ([]maps.Map, error) {
	m, _ := r.GetMap(ctx, "map-1")
	return []maps.Map{*m}, nil
}
func (pictureRepo) ListMarkers(context.Context, string, int, string) ([]maps.Marker, error) {
	return nil, nil
}

type pictureShadows struct{}

func (pictureShadows) ShadowAreas(context.Context, string) ([]maps.ShadowArea, error) {
	return []maps.ShadowArea{{MinX: 10, MinY: 10, MaxX: 30, MaxY: 30, Strength: 0.5}}, nil
}

// A key whose user is below owner never receives the original picture id of a
// map with a shadow, on the single-map and the list endpoint alike.
func TestMapAPIHandler_MapPicture_ByRole(t *testing.T) {
	cases := []struct {
		role         campaigns.Role
		wantOriginal bool
	}{
		{campaigns.RolePlayer, false},
		{campaigns.RoleScribe, false},
		{campaigns.RoleOwner, true},
	}
	for _, tc := range cases {
		t.Run(tc.role.String(), func(t *testing.T) {
			svc := maps.NewMapService(pictureRepo{})
			svc.SetShadowLookup(pictureShadows{})
			h := NewMapAPIHandler(nil, svc, nil, &stubCampaignSvcOwnerGate{role: tc.role})
			key := &APIKey{CampaignID: "camp-1", UserID: "u1"}

			c, rec := newMapAPIContext(http.MethodGet, "/", key)
			if err := h.GetMap(c); err != nil || rec.Code != http.StatusOK {
				t.Fatalf("GetMap: code=%d err=%v", rec.Code, err)
			}
			single := rec.Body.String()
			c, rec = newMapAPIContext(http.MethodGet, "/", key)
			if err := h.ListMaps(c); err != nil || rec.Code != http.StatusOK {
				t.Fatalf("ListMaps: code=%d err=%v", rec.Code, err)
			}
			for name, body := range map[string]string{"GetMap": single, "ListMaps": rec.Body.String()} {
				if has := strings.Contains(body, pictureMediaID); has != tc.wantOriginal {
					t.Errorf("%s names the original picture = %v, want %v: %s", name, has, tc.wantOriginal, body)
				}
				if !tc.wantOriginal && !strings.Contains(body, "player-image") {
					t.Errorf("%s has no player copy address: %s", name, body)
				}
			}
		})
	}
}

type picturesMediaGuard struct {
	hidden  bool
	picture bool // a map picture that has no shadow yet
	err     error
}

func (g picturesMediaGuard) IsMapPicture(context.Context, string, string) (bool, error) {
	return g.hidden || g.picture, g.err
}

func (g picturesMediaGuard) IsShadowedMapImage(context.Context, string, string) (bool, error) {
	return g.hidden, g.err
}

// The API's media links are accepted without cookies, so for a caller below
// owner they are withheld for any map picture, shadowed or not (and when the check fails).
func TestMediaAPIHandler_Response_WithholdsShadowedPictureLinks(t *testing.T) {
	camp := "camp-1"
	file := &media.MediaFile{ID: pictureMediaID, CampaignID: &camp, ThumbnailPaths: map[string]string{"300": "t.jpg"}}
	cases := []struct {
		name      string
		role      campaigns.Role
		guard     media.MapImageGuard
		wantLinks bool
	}{
		{"scribe, shadowed picture", campaigns.RoleScribe, picturesMediaGuard{hidden: true}, false},
		{"scribe, check fails", campaigns.RoleScribe, picturesMediaGuard{err: errors.New("db")}, false},
		{"scribe, map picture with no shadow yet", campaigns.RoleScribe, picturesMediaGuard{picture: true}, false},
		{"player, map picture with no shadow yet", campaigns.RolePlayer, picturesMediaGuard{picture: true}, false},
		{"scribe, ordinary file", campaigns.RoleScribe, picturesMediaGuard{}, true},
		{"owner, map picture with no shadow yet", campaigns.RoleOwner, picturesMediaGuard{picture: true}, true},
		{"owner, shadowed picture", campaigns.RoleOwner, picturesMediaGuard{hidden: true}, true},
		{"unwired guard", campaigns.RoleScribe, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewMediaAPIHandler(nil, nil)
			h.SetURLSigner(media.NewURLSigner("secret"))
			h.SetCampaignService(&stubCampaignSvcOwnerGate{role: tc.role})
			if tc.guard != nil {
				h.SetMapImageGuard(tc.guard)
			}
			c, _ := newMapAPIContext(http.MethodGet, "/", &APIKey{CampaignID: camp, UserID: "u1"})
			resp := h.response(c, file)
			raw, _ := json.Marshal(resp)
			hasLinks := resp.URL != "" || resp.ThumbnailURL != "" || len(resp.Thumbnails) > 0
			if hasLinks != tc.wantLinks {
				t.Errorf("links present = %v, want %v: %s", hasLinks, tc.wantLinks, raw)
			}
		})
	}
}

// stubCampaignSvcGrant reports a member role plus a co-DM grant.
type stubCampaignSvcGrant struct {
	stubCampaignSvcOwnerGate
	granted    bool
	grantedErr error
}

func (s *stubCampaignSvcGrant) IsUserDmGranted(context.Context, string, string) (bool, error) {
	return s.granted, s.grantedErr
}

// A co-DM grantee's key is judged by the promoted role the web uses: it gets
// the original picture, while a plain scribe, and a scribe whose grant lookup
// fails, still get the player copy.
func TestMapAPIHandler_MapPicture_CoDMGrant(t *testing.T) {
	cases := []struct {
		name         string
		role         campaigns.Role
		granted      bool
		grantErr     error
		wantOriginal bool
	}{
		{"scribe with a co-DM grant", campaigns.RoleScribe, true, nil, true},
		{"player with a co-DM grant", campaigns.RolePlayer, true, nil, true},
		{"scribe without a grant", campaigns.RoleScribe, false, nil, false},
		{"grant lookup fails: stays hidden", campaigns.RoleScribe, true, errors.New("db"), false},
		{"owner", campaigns.RoleOwner, false, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := maps.NewMapService(pictureRepo{})
			svc.SetShadowLookup(pictureShadows{})
			camp := &stubCampaignSvcGrant{stubCampaignSvcOwnerGate: stubCampaignSvcOwnerGate{role: tc.role}, granted: tc.granted, grantedErr: tc.grantErr}
			h := NewMapAPIHandler(nil, svc, nil, camp)
			key := &APIKey{CampaignID: "camp-1", UserID: "u1"}
			c, rec := newMapAPIContext(http.MethodGet, "/", key)
			if err := h.GetMap(c); err != nil || rec.Code != http.StatusOK {
				t.Fatalf("GetMap: code=%d err=%v", rec.Code, err)
			}
			if has := strings.Contains(rec.Body.String(), pictureMediaID); has != tc.wantOriginal {
				t.Errorf("names the original picture = %v, want %v: %s", has, tc.wantOriginal, rec.Body.String())
			}
		})
	}
}
