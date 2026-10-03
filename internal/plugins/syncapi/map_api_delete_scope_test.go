package syncapi

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
)

// recordingDeleteMapSvc captures the actor and role the handler forwards, so
// the test proves the key owner's identity (not a request value) feeds the
// service's ownership rule.
type recordingDeleteMapSvc struct {
	stubMapSvcOwnerGate
	actor string
	role  int
	hit   bool
}

func (s *recordingDeleteMapSvc) DeleteMarker(_ context.Context, _ string, _ *time.Time, _ bool, actor string, role int) error {
	s.hit, s.actor, s.role = true, actor, role
	return nil
}

type recordingDeleteDrawingSvc struct {
	stubDrawingSvcOwnerGate
	actor string
	role  int
	hit   bool
}

func (s *recordingDeleteDrawingSvc) DeleteDrawing(_ context.Context, _, _ string, _ *time.Time, actor string, role int, _ bool) error {
	s.hit, s.actor, s.role = true, actor, role
	return nil
}

// dmGrantCampaignSvc adds the co-DM lookup canAuthorDmOnly makes for
// non-owners.
type dmGrantCampaignSvc struct{ stubCampaignSvcOwnerGate }

func (s *dmGrantCampaignSvc) IsUserDmGranted(context.Context, string, string) (bool, error) {
	return false, nil
}

func TestMapAPIHandler_Delete_ScribeReachesServiceWithKeyIdentity(t *testing.T) {
	m := &maps.Map{ID: "map-1", CampaignID: "camp-1"}
	cases := []struct {
		name     string
		role     campaigns.Role
		wantHit  bool
		wantCode int
	}{
		{"owner key", campaigns.RoleOwner, true, 0},
		{"scribe key", campaigns.RoleScribe, true, 0},
		{"player key", campaigns.RolePlayer, false, http.StatusForbidden},
	}
	for _, tc := range cases {
		for _, kind := range []string{"marker", "drawing"} {
			t.Run(tc.name+"/"+kind, func(t *testing.T) {
				ms := &recordingDeleteMapSvc{stubMapSvcOwnerGate: stubMapSvcOwnerGate{m: m}}
				ds := &recordingDeleteDrawingSvc{}
				h := NewMapAPIHandler(nil, ms, ds, &dmGrantCampaignSvc{stubCampaignSvcOwnerGate{role: tc.role}})
				key := &APIKey{ID: 1, CampaignID: "camp-1", UserID: "user-9", IsActive: true, Permissions: []APIKeyPermission{PermRead, PermWrite}}
				c, _ := newMapAPIContext(http.MethodDelete, "/x", key)
				c.SetParamNames("id", "mapID", "markerID", "drawingID")
				c.SetParamValues("camp-1", "map-1", "mk-1", "d-1")

				var err error
				var hit bool
				var actor string
				var role int
				if kind == "marker" {
					err = h.DeleteMarker(c)
					hit, actor, role = ms.hit, ms.actor, ms.role
				} else {
					err = h.DeleteDrawing(c)
					hit, actor, role = ds.hit, ds.actor, ds.role
				}
				if tc.wantCode != 0 {
					var ae *apperror.AppError
					if !errors.As(err, &ae) || ae.Code != tc.wantCode {
						t.Fatalf("want %d, got %v", tc.wantCode, err)
					}
				} else if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if hit != tc.wantHit {
					t.Fatalf("service reached = %v, want %v", hit, tc.wantHit)
				}
				if hit && (actor != "user-9" || role != int(tc.role)) {
					t.Errorf("forwarded actor=%q role=%d, want user-9/%d", actor, role, tc.role)
				}
			})
		}
	}
}
