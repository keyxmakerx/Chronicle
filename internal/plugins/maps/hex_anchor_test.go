package maps

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// SetAnchor mirrors the real repository: it creates the layer on first use and
// bumps the version on every call.
func (r *fakeHexRepo) SetAnchor(_ context.Context, mapID string, anchor *string) (uint64, error) {
	if r.layer == nil {
		l := DefaultHexLayer(mapID)
		r.layer = &l
	}
	r.layer.AnchorDrawingID = anchor
	r.layer.Version++
	r.anchorSets++
	return r.layer.Version, nil
}

// fakeHexPictures serves drawings by id and records rotation resets.
type fakeHexPictures struct {
	drawings map[string]*Drawing
	cleared  []string
}

func (p *fakeHexPictures) GetPicture(_ context.Context, id string) (*Drawing, error) {
	return p.drawings[id], nil
}

func (p *fakeHexPictures) ClearRotation(_ context.Context, _, id string) error {
	p.cleared = append(p.cleared, id)
	p.drawings[id].Rotation = 0
	return nil
}

func picture(id, mapID string) *Drawing {
	return &Drawing{ID: id, MapID: mapID, DrawingType: DrawingTypeImage, Visibility: "everyone"}
}

// anchorFixture builds a service whose map "map-1" has: a usable picture
// "pic-1", a picture "pic-other" on another map, a rectangle "rect-1", and a
// picture "pic-hidden" hidden from players.
func anchorFixture() (HexService, *fakeHexRepo, *fakeHexPictures) {
	repo := newFakeHexRepo()
	pics := &fakeHexPictures{drawings: map[string]*Drawing{
		"pic-1":     picture("pic-1", "map-1"),
		"pic-other": picture("pic-other", "map-2"),
		"rect-1":    {ID: "rect-1", MapID: "map-1", DrawingType: "rectangle", Visibility: "everyone"},
		"pic-hidden": func() *Drawing {
			d := picture("pic-hidden", "map-1")
			d.Visibility = "dm_only"
			return d
		}(),
	}}
	svc := hexSvc(repo, DrawWhoScribes)
	svc.SetPictures(pics)
	return svc, repo, pics
}

func anchorTo(id string) UpdateHexLayerInput {
	return UpdateHexLayerInput{AnchorDrawingID: patchOf(id)}
}

func TestUpdateLayer_AnchorMustBeAnImageOnThisMap(t *testing.T) {
	tests := []struct {
		name string
		id   string
		ok   bool
	}{
		{"a picture on this map", "pic-1", true},
		{"a picture on another map", "pic-other", false},
		{"a drawing that is not a picture", "rect-1", false},
		{"an id that does not exist", "nope", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, _ := anchorFixture()
			res, err := svc.UpdateLayer(context.Background(), "camp-1", "map-1", actorOwner, anchorTo(tc.id))
			if tc.ok {
				if err != nil || res.AnchorDrawingID == nil || *res.AnchorDrawingID != tc.id {
					t.Fatalf("got %+v, %v", res, err)
				}
				return
			}
			if !isBadRequest(err) {
				t.Fatalf("expected 400, got %v", err)
			}
			if repo.anchorSets != 0 {
				t.Error("a refused anchor reached the repository")
			}
		})
	}
}

func TestUpdateLayer_RoleGates(t *testing.T) {
	tests := []struct {
		name  string
		actor HexActor
		allow bool
	}{
		{"owner", actorOwner, true},
		{"DM grant", actorDM, true},
		{"scribe", actorScribe, false},
		{"player", actorPlayer, false},
		{"member-less visitor", actorNone, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, _ := anchorFixture()
			_, err := svc.UpdateLayer(context.Background(), "camp-1", "map-1", tc.actor, anchorTo("pic-1"))
			if tc.allow {
				if err != nil || repo.anchorSets != 1 {
					t.Fatalf("expected success, got %v (writes %d)", err, repo.anchorSets)
				}
				return
			}
			if !isForbidden(err) || repo.anchorSets != 0 {
				t.Fatalf("expected 403 and no write, got %v (writes %d)", err, repo.anchorSets)
			}
		})
	}
}

func TestUpdateLayer_CrossCampaignMapIsNotFound(t *testing.T) {
	svc, repo, _ := anchorFixture()
	_, err := svc.UpdateLayer(context.Background(), "camp-2", "map-1", actorOwner, anchorTo("pic-1"))
	if !isHexNotFound(err) || repo.anchorSets != 0 {
		t.Fatalf("expected 404 and no write, got %v", err)
	}
}

func TestUpdateLayer_FailsClosedWithoutPictures(t *testing.T) {
	repo := newFakeHexRepo()
	svc := hexSvc(repo, DrawWhoScribes)
	if _, err := svc.UpdateLayer(context.Background(), "camp-1", "map-1", actorOwner, anchorTo("pic-1")); err == nil || repo.anchorSets != 0 {
		t.Errorf("a write without a picture lookup must fail, got %v", err)
	}
	repo.layer = &HexLayer{MapID: "map-1", AnchorDrawingID: strPtr("pic-1")}
	if _, err := svc.GetLayer(context.Background(), "camp-1", "map-1", permissions.RolePlayer); err == nil {
		t.Error("a read of an anchored layer without a picture lookup must fail, not guess")
	}
}

func TestUpdateLayer_VersionBumpsOnEveryWrite(t *testing.T) {
	svc, repo, _ := anchorFixture()
	repo.layer = &HexLayer{MapID: "map-1", Version: 7, MilesPerHex: 6, MilesPerDay: 24}
	for i, in := range []UpdateHexLayerInput{
		anchorTo("pic-1"),
		{AnchorDrawingID: patchNull()}, // back to the whole map
		anchorTo("pic-1"),              // same again still counts: it is a write
	} {
		res, err := svc.UpdateLayer(context.Background(), "camp-1", "map-1", actorOwner, in)
		if err != nil {
			t.Fatal(err)
		}
		if want := uint64(8 + i); res.Version != want {
			t.Errorf("write %d: version = %d, want %d", i, res.Version, want)
		}
	}
	// The first write on a map with no layer row creates it at version 1.
	svc2, _, _ := anchorFixture()
	res, err := svc2.UpdateLayer(context.Background(), "camp-1", "map-1", actorOwner, anchorTo("pic-1"))
	if err != nil || res.Version != 1 {
		t.Errorf("first write = %+v, %v; want version 1", res, err)
	}
}

func TestUpdateLayer_PartialContract(t *testing.T) {
	t.Run("absent keeps and is refused as a no-op", func(t *testing.T) {
		svc, repo, _ := anchorFixture()
		_, err := svc.UpdateLayer(context.Background(), "camp-1", "map-1", actorOwner, UpdateHexLayerInput{})
		if !isBadRequest(err) || repo.anchorSets != 0 {
			t.Fatalf("expected 400 and no write, got %v", err)
		}
	})
	t.Run("null puts the hexes back on the whole map", func(t *testing.T) {
		svc, repo, _ := anchorFixture()
		repo.layer = &HexLayer{MapID: "map-1", AnchorDrawingID: strPtr("pic-1")}
		res, err := svc.UpdateLayer(context.Background(), "camp-1", "map-1", actorOwner, UpdateHexLayerInput{AnchorDrawingID: patchNull()})
		if err != nil || res.AnchorDrawingID != nil || repo.layer.AnchorDrawingID != nil {
			t.Fatalf("got %+v, %v, stored %v", res, err, repo.layer.AnchorDrawingID)
		}
	})
}

func TestUpdateLayer_AnchoringStraightensThePicture(t *testing.T) {
	tests := []struct {
		name     string
		rotation float64
		cleared  int
	}{
		{"a turned picture is reset to 0", 12, 1},
		{"a turned the other way picture is reset to 0", -30, 1},
		{"an upright picture is left alone", 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, pics := anchorFixture()
			pics.drawings["pic-1"].Rotation = tc.rotation
			if _, err := svc.UpdateLayer(context.Background(), "camp-1", "map-1", actorOwner, anchorTo("pic-1")); err != nil {
				t.Fatal(err)
			}
			if len(pics.cleared) != tc.cleared || pics.drawings["pic-1"].Rotation != 0 {
				t.Errorf("cleared %v, rotation now %v", pics.cleared, pics.drawings["pic-1"].Rotation)
			}
		})
	}
	t.Run("a refused anchor leaves the picture turned", func(t *testing.T) {
		svc, _, pics := anchorFixture()
		pics.drawings["pic-other"].Rotation = 20
		_, _ = svc.UpdateLayer(context.Background(), "camp-1", "map-1", actorOwner, anchorTo("pic-other"))
		if len(pics.cleared) != 0 || pics.drawings["pic-other"].Rotation != 20 {
			t.Error("a picture of another map was straightened")
		}
	})
	t.Run("clearing the anchor does not touch a picture", func(t *testing.T) {
		svc, _, pics := anchorFixture()
		pics.drawings["pic-1"].Rotation = 9
		if _, err := svc.UpdateLayer(context.Background(), "camp-1", "map-1", actorOwner, UpdateHexLayerInput{AnchorDrawingID: patchNull()}); err != nil {
			t.Fatal(err)
		}
		if len(pics.cleared) != 0 {
			t.Error("a null anchor reset a rotation")
		}
	})
}

func TestGetLayer_AnchorFallsBackToTheWholeMap(t *testing.T) {
	tests := []struct {
		name   string
		anchor string
		keeps  bool
	}{
		{"a live picture is kept", "pic-1", true},
		{"a deleted picture falls back", "deleted", false},
		{"a picture moved to another map falls back", "pic-other", false},
		{"a drawing that stopped being a picture falls back", "rect-1", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, _ := anchorFixture()
			repo.layer = &HexLayer{MapID: "map-1", AnchorDrawingID: strPtr(tc.anchor), Version: 4}
			repo.cells[HexKey{1, 1}] = HexCell{Col: 1, Row: 1}
			v, err := svc.GetLayer(context.Background(), "camp-1", "map-1", permissions.RolePlayer)
			if err != nil {
				t.Fatal(err)
			}
			if got := v.Layer.AnchorDrawingID != nil; got != tc.keeps {
				t.Errorf("anchor kept = %v, want %v", got, tc.keeps)
			}
			// A fallback is not a hiding: the painted hexes still arrive.
			if v.Hidden || len(v.Cells) != 1 {
				t.Errorf("hidden = %v, cells = %d; want the layer visible with its cell", v.Hidden, len(v.Cells))
			}
			// Reading never rewrites the stored anchor.
			if repo.layer.AnchorDrawingID == nil || *repo.layer.AnchorDrawingID != tc.anchor {
				t.Error("a read changed the stored anchor")
			}
		})
	}
}

func TestGetLayer_HiddenAnchorHidesCells(t *testing.T) {
	tests := []struct {
		name   string
		role   int
		hidden bool
	}{
		{"public visitor", permissions.RoleNone, true},
		{"player", permissions.RolePlayer, true},
		{"scribe", permissions.RoleScribe, true},
		{"owner", permissions.RoleOwner, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, _ := anchorFixture()
			repo.layer = &HexLayer{MapID: "map-1", AnchorDrawingID: strPtr("pic-hidden"), Version: 5}
			repo.cells[HexKey{2, 3}] = HexCell{Col: 2, Row: 3, Name: "secret keep"}
			v, err := svc.GetLayer(context.Background(), "camp-1", "map-1", tc.role)
			if err != nil {
				t.Fatal(err)
			}
			if v.Hidden != tc.hidden {
				t.Fatalf("hidden = %v, want %v", v.Hidden, tc.hidden)
			}
			body, _ := json.Marshal(v)
			if tc.hidden {
				if len(v.Cells) != 0 || v.Layer.AnchorDrawingID != nil || strings.Contains(string(body), "secret") || strings.Contains(string(body), "pic-hidden") {
					t.Errorf("a hidden layer leaked: %s", body)
				}
				return
			}
			if len(v.Cells) != 1 || v.Layer.AnchorDrawingID == nil || *v.Layer.AnchorDrawingID != "pic-hidden" {
				t.Errorf("the owner's view = %s", body)
			}
		})
	}
}

// A DM grant arrives at the service as the visibility role the handler derives,
// so this runs the whole path to prove a DM-granted player is not hidden from.
func TestHexHandler_HiddenAnchorByRole(t *testing.T) {
	tests := []struct {
		name      string
		role      campaigns.Role
		dmGranted bool
		hidden    bool
	}{
		{"owner", campaigns.RoleOwner, false, false},
		{"DM-granted player", campaigns.RolePlayer, true, false},
		{"scribe", campaigns.RoleScribe, false, true},
		{"player", campaigns.RolePlayer, false, true},
		{"public visitor", campaigns.RoleNone, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, _ := anchorFixture()
			repo.layer = &HexLayer{MapID: "map-1", AnchorDrawingID: strPtr("pic-hidden"), Version: 2}
			repo.cells[HexKey{0, 0}] = HexCell{Col: 0, Row: 0, Name: "secret keep"}
			h := NewHexHandler(svc)
			rec := hexRequest(t, h, http.MethodGet, "", tc.role, tc.dmGranted, h.GetHexes)
			if rec.status != http.StatusOK {
				t.Fatalf("status = %d", rec.status)
			}
			if got := strings.Contains(rec.Body.String(), "secret keep"); got == tc.hidden {
				t.Errorf("secret in response = %v, want %v", got, !tc.hidden)
			}
		})
	}
}

func TestHexHandler_PutLayer(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		role      campaigns.Role
		dmGranted bool
		want      int
	}{
		{"owner anchors", `{"anchor_drawing_id":"pic-1"}`, campaigns.RoleOwner, false, http.StatusOK},
		{"DM grant anchors", `{"anchor_drawing_id":"pic-1"}`, campaigns.RolePlayer, true, http.StatusOK},
		{"owner clears with null", `{"anchor_drawing_id":null}`, campaigns.RoleOwner, false, http.StatusOK},
		{"scribe refused", `{"anchor_drawing_id":"pic-1"}`, campaigns.RoleScribe, false, http.StatusForbidden},
		{"player refused", `{"anchor_drawing_id":"pic-1"}`, campaigns.RolePlayer, false, http.StatusForbidden},
		{"empty body changes nothing", `{}`, campaigns.RoleOwner, false, http.StatusBadRequest},
		{"wrong type", `{"anchor_drawing_id":5}`, campaigns.RoleOwner, false, http.StatusBadRequest},
		{"not json", `nope`, campaigns.RoleOwner, false, http.StatusBadRequest},
		{"another map's picture", `{"anchor_drawing_id":"pic-other"}`, campaigns.RoleOwner, false, http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, _ := anchorFixture()
			h := NewHexHandler(svc)
			rec := hexRequest(t, h, http.MethodPut, tc.body, tc.role, tc.dmGranted, h.PutHexLayer)
			if rec.status != tc.want {
				t.Fatalf("status = %d, want %d (%s)", rec.status, tc.want, rec.Body.String())
			}
			if tc.want == http.StatusOK {
				var res HexLayerWriteResult
				if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || res.Version != 1 {
					t.Errorf("response = %s", rec.Body.String())
				}
			}
		})
	}
}
