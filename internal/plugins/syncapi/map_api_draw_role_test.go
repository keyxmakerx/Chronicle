package syncapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
)

// recordingDrawingSvc captures the caller role the sync API forwards on a
// drawing write. maps.DrawingService enforces the map's "who can draw" gate
// from that role, so the sync API must send the key owner's real campaign role
// (not 0, which is refused, and not a constant that would bypass the gate).
type recordingDrawingSvc struct {
	maps.DrawingService
	createRole, updateRole int
	createIsDM, updateIsDM bool
}

func (s *recordingDrawingSvc) CreateDrawing(_ context.Context, in maps.CreateDrawingInput) (*maps.Drawing, error) {
	s.createRole = in.CallerRole
	s.createIsDM = in.CallerIsDM
	return &maps.Drawing{}, nil
}

func (s *recordingDrawingSvc) UpdateDrawing(_ context.Context, _, _, _ string, role int, isDM bool, _ maps.UpdateDrawingInput) error {
	s.updateRole = role
	s.updateIsDM = isDM
	return nil
}

func TestMapAPIHandler_DrawingWrites_ForwardTheKeyOwnersRole(t *testing.T) {
	m := &maps.Map{ID: "map-1", CampaignID: "camp-1"}
	for _, role := range []campaigns.Role{campaigns.RoleScribe, campaigns.RoleOwner} {
		rec := &recordingDrawingSvc{}
		h := NewMapAPIHandler(nil, &stubMapSvcOwnerGate{m: m}, rec, &stubCampaignSvcOwnerGate{role: role})
		key := &APIKey{CampaignID: "camp-1", UserID: "u1"}

		c, _ := newMapAPIContext(http.MethodPost, "/", key)
		// The empty body is not a real drawing, but the stub accepts it; the role
		// the handler forwards is what this pins.
		_ = h.CreateDrawing(c)
		if rec.createRole != int(role) {
			t.Errorf("role %d: create forwarded role %d", role, rec.createRole)
		}
		// Scribes are not DM-equivalent, so they may not author shadows.
		wantDM := role == campaigns.RoleOwner
		if rec.createIsDM != wantDM {
			t.Errorf("role %d: create forwarded isDM=%v, want %v", role, rec.createIsDM, wantDM)
		}

		c, _ = newMapAPIContext(http.MethodPut, "/", key)
		c.SetParamNames("id", "mapID", "drawingID")
		c.SetParamValues("camp-1", "map-1", "d-1")
		_ = h.UpdateDrawing(c)
		if rec.updateRole != int(role) {
			t.Errorf("role %d: update forwarded role %d", role, rec.updateRole)
		}
		if rec.updateIsDM != wantDM {
			t.Errorf("role %d: update forwarded isDM=%v, want %v", role, rec.updateIsDM, wantDM)
		}
	}
}
