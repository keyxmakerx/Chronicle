package maps

import (
	"context"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// deleteCase enumerates who may delete an item with a given creator. The
// same matrix applies to markers and drawings, so both services share it.
type deleteCase struct {
	name      string
	role      int
	actor     string
	createdBy *string
	visibil   string
	wantCode  int // 0 = allowed
}

func strPtr(s string) *string { return &s }

func deleteCases() []deleteCase {
	return []deleteCase{
		{"owner deletes anyone's", permissions.RoleOwner, "owner-1", strPtr("scribe-1"), "everyone", 0},
		{"owner deletes nil-creator", permissions.RoleOwner, "owner-1", nil, "everyone", 0},
		{"scribe deletes own", permissions.RoleScribe, "scribe-1", strPtr("scribe-1"), "everyone", 0},
		{"scribe cannot delete other's", permissions.RoleScribe, "scribe-1", strPtr("scribe-2"), "everyone", http.StatusForbidden},
		{"scribe cannot delete nil-creator", permissions.RoleScribe, "scribe-1", nil, "everyone", http.StatusForbidden},
		{"scribe with empty actor cannot delete nil-creator", permissions.RoleScribe, "", nil, "everyone", http.StatusForbidden},
		{"player cannot delete own", permissions.RolePlayer, "player-1", strPtr("player-1"), "everyone", http.StatusForbidden},
	}
}

func TestDeleteMarker_Ownership(t *testing.T) {
	for _, tc := range deleteCases() {
		t.Run(tc.name, func(t *testing.T) {
			deleted := false
			repo := &mockMapRepo{
				getMarkerFn: func(context.Context, string) (*Marker, error) {
					return &Marker{ID: "mk-1", MapID: "map-1", CreatedBy: tc.createdBy, Visibility: tc.visibil}, nil
				},
				deleteMarkerFn: func(context.Context, string) error { deleted = true; return nil },
			}
			err := newTestMapService(repo).DeleteMarker(context.Background(), "mk-1", nil, tc.role >= permissions.RoleOwner, tc.actor, tc.role)
			if tc.wantCode == 0 {
				if err != nil || !deleted {
					t.Fatalf("want delete, got err=%v deleted=%v", err, deleted)
				}
				return
			}
			assertAppError(t, err, tc.wantCode)
			if deleted {
				t.Error("repo delete must not run when forbidden")
			}
		})
	}
}

// A scribe with no dm_only authority gets the same NotFound a missing marker
// gives, even when they would otherwise be forbidden, so existence is not
// leaked.
func TestDeleteMarker_DmOnlyInvisibleToScribe(t *testing.T) {
	repo := &mockMapRepo{getMarkerFn: func(context.Context, string) (*Marker, error) {
		return &Marker{ID: "mk-1", MapID: "map-1", CreatedBy: strPtr("owner-1"), Visibility: "dm_only"}, nil
	}}
	err := newTestMapService(repo).DeleteMarker(context.Background(), "mk-1", nil, false, "scribe-1", permissions.RoleScribe)
	assertAppError(t, err, http.StatusNotFound)
}

// drawingOwnershipRepo serves one drawing and records deletes; embedding the
// interface leaves every unused method nil.
type drawingOwnershipRepo struct {
	DrawingRepository
	d       *Drawing
	deleted bool
}

func (r *drawingOwnershipRepo) GetDrawing(context.Context, string) (*Drawing, error) { return r.d, nil }
func (r *drawingOwnershipRepo) ListShadows(context.Context, string) ([]Drawing, error) {
	return nil, nil
}
func (r *drawingOwnershipRepo) DeleteDrawing(context.Context, string) error {
	r.deleted = true
	return nil
}

func TestDeleteDrawing_Ownership(t *testing.T) {
	for _, tc := range deleteCases() {
		t.Run(tc.name, func(t *testing.T) {
			repo := &drawingOwnershipRepo{d: &Drawing{ID: "d-1", MapID: "map-1", CreatedBy: tc.createdBy, Visibility: tc.visibil}}
			err := NewDrawingService(repo).DeleteDrawing(context.Background(), "d-1", "map-1", nil, tc.actor, tc.role, false)
			if tc.wantCode == 0 {
				if err != nil || !repo.deleted {
					t.Fatalf("want delete, got err=%v deleted=%v", err, repo.deleted)
				}
				return
			}
			assertAppError(t, err, tc.wantCode)
			if repo.deleted {
				t.Error("repo delete must not run when forbidden")
			}
		})
	}
}

func TestDeleteDrawing_DmOnlyOfOthersIsNotFoundForScribe(t *testing.T) {
	repo := &drawingOwnershipRepo{d: &Drawing{ID: "d-1", MapID: "map-1", CreatedBy: strPtr("owner-1"), Visibility: "dm_only"}}
	err := NewDrawingService(repo).DeleteDrawing(context.Background(), "d-1", "map-1", nil, "scribe-1", permissions.RoleScribe, false)
	assertAppError(t, err, http.StatusNotFound)
	if repo.deleted {
		t.Error("must not delete")
	}
}

type fakeSession struct{ id string }

func (f fakeSession) GetUserID() string { return f.id }

// TestDeleteMarkerAPI_ScribeOwnVsOther runs the real handler: a scribe's
// session user id and campaign role must reach the service rule.
func TestDeleteMarkerAPI_ScribeOwnVsOther(t *testing.T) {
	cases := []struct {
		name     string
		userID   string
		wantCode int
	}{
		{"scribe deletes own marker", "scribe-1", 0},
		{"scribe refused on another's marker", "scribe-2", http.StatusForbidden},
		{"scribe without session refused", "", http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deleted := false
			repo := everyoneMapRepo()
			repo.getMarkerFn = func(context.Context, string) (*Marker, error) {
				m := storedEveryoneMarker()
				m.CreatedBy = strPtr("scribe-1")
				return m, nil
			}
			repo.deleteMarkerFn = func(context.Context, string) error { deleted = true; return nil }
			h := NewHandler(NewMapService(repo))
			c, _ := newDMWriteRequest(http.MethodDelete, "/campaigns/camp-1/maps/map-1/markers/mk-public", "")
			c.SetParamNames("id", "mid", "mkid")
			c.SetParamValues("camp-1", "map-1", "mk-public")
			c.Set("campaign_context", dmWriteCampaignCtx(campaigns.RoleScribe, false))
			if tc.userID != "" {
				c.Set("session", fakeSession{tc.userID})
			}
			err := h.DeleteMarkerAPI(c)
			if tc.wantCode == 0 {
				if err != nil || !deleted {
					t.Fatalf("want delete, err=%v deleted=%v", err, deleted)
				}
				return
			}
			assertAppError(t, err, tc.wantCode)
			if deleted {
				t.Error("must not delete")
			}
		})
	}
}
