package syncapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
)

// --- relation campaign-scoping regression (syncapi IDOR) ---
//
// PUT/DELETE /api/v1/campaigns/:id/relations/:relationId address the relation
// by its enumerable integer primary key. The middleware chain only proves the
// caller may write to the campaign in the URL — it says nothing about which
// campaign the relation belongs to. Without an ownership check the handlers
// hand a campaign-A key write access to every relation row in the database.
// These tests pin the check: a relation owned by another campaign must 404 and
// must not be touched, while the same-campaign path keeps working.

// stubRelationSvcForScope embeds relations.RelationService so unimplemented
// methods are present-but-panic (same rationale as stubEntityServiceForCreate),
// and overrides only the three methods the two handlers under test can reach.
// The map is keyed by ID, mirroring the real `WHERE id = ?` SQL — there is no
// campaign filter in the repository, which is exactly why the handler needs one.
type stubRelationSvcForScope struct {
	relations.RelationService
	rels    map[int]*relations.Relation
	deleted []int
	updated map[int]json.RawMessage
}

func (s *stubRelationSvcForScope) GetByID(_ context.Context, id int) (*relations.Relation, error) {
	rel, ok := s.rels[id]
	if !ok {
		return nil, apperror.NewNotFound("relation not found")
	}
	return rel, nil
}

func (s *stubRelationSvcForScope) Delete(_ context.Context, id int) error {
	if _, ok := s.rels[id]; !ok {
		return apperror.NewNotFound("relation not found")
	}
	delete(s.rels, id)
	s.deleted = append(s.deleted, id)
	return nil
}

func (s *stubRelationSvcForScope) UpdateMetadata(_ context.Context, id int, metadata json.RawMessage) error {
	rel, ok := s.rels[id]
	if !ok {
		return apperror.NewNotFound("relation not found")
	}
	rel.Metadata = metadata
	if s.updated == nil {
		s.updated = map[int]json.RawMessage{}
	}
	s.updated[id] = metadata
	return nil
}

// newRelationScopeSvc seeds one relation owned by campaign-B (the victim) and
// one owned by campaign-A (the negative control).
func newRelationScopeSvc() *stubRelationSvcForScope {
	return &stubRelationSvcForScope{
		rels: map[int]*relations.Relation{
			12345: {ID: 12345, CampaignID: "campaign-B", Metadata: json.RawMessage(`{"price":100}`)},
			777:   {ID: 777, CampaignID: "campaign-A", Metadata: json.RawMessage(`{"price":5}`)},
		},
	}
}

// newRelationScopeContext builds an Echo context for the given method against
// /api/v1/campaigns/<campaignID>/relations/<relationID>.
func newRelationScopeContext(method, campaignID, relationID string, body []byte) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, "/api/v1/campaigns/"+campaignID+"/relations/"+relationID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "relationId")
	c.SetParamValues(campaignID, relationID)
	return c, rec
}

// TestDeleteRelation_RejectsForeignCampaign is the IDOR fix: a campaign-A
// caller must not be able to delete campaign-B's relation by guessing its ID.
func TestDeleteRelation_RejectsForeignCampaign(t *testing.T) {
	svc := newRelationScopeSvc()
	h := NewAPIHandler(nil, nil, nil, svc)

	c, _ := newRelationScopeContext(http.MethodDelete, "campaign-A", "12345", nil)

	err := h.DeleteRelation(c)
	if err == nil {
		t.Fatalf("cross-campaign DELETE must be rejected, got nil error")
	}
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != http.StatusNotFound {
		t.Fatalf("want 404 not-found AppError, got %#v", err)
	}
	if _, still := svc.rels[12345]; !still {
		t.Fatalf("campaign-B relation 12345 was deleted across campaigns (IDOR)")
	}
	if len(svc.deleted) != 0 {
		t.Fatalf("Delete must not reach the service for a foreign relation, deleted=%v", svc.deleted)
	}
}

// TestUpdateRelation_RejectsForeignCampaign is the same fix on the write path:
// a campaign-A caller must not overwrite campaign-B relation metadata.
func TestUpdateRelation_RejectsForeignCampaign(t *testing.T) {
	svc := newRelationScopeSvc()
	h := NewAPIHandler(nil, nil, nil, svc)

	c, _ := newRelationScopeContext(http.MethodPut, "campaign-A", "12345", []byte(`{"metadata":{"price":1}}`))

	err := h.UpdateRelation(c)
	if err == nil {
		t.Fatalf("cross-campaign PUT must be rejected, got nil error")
	}
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != http.StatusNotFound {
		t.Fatalf("want 404 not-found AppError, got %#v", err)
	}
	if got := string(svc.rels[12345].Metadata); got != `{"price":100}` {
		t.Fatalf("campaign-B relation metadata was overwritten across campaigns (IDOR): %s", got)
	}
	if len(svc.updated) != 0 {
		t.Fatalf("UpdateMetadata must not reach the service for a foreign relation, updated=%v", svc.updated)
	}
}

// TestRelationScope_SameCampaignStillWorks is the negative control: the
// ownership check must not break the legitimate path.
func TestRelationScope_SameCampaignStillWorks(t *testing.T) {
	t.Run("update", func(t *testing.T) {
		svc := newRelationScopeSvc()
		h := NewAPIHandler(nil, nil, nil, svc)

		c, rec := newRelationScopeContext(http.MethodPut, "campaign-A", "777", []byte(`{"metadata":{"price":9}}`))
		if err := h.UpdateRelation(c); err != nil {
			t.Fatalf("same-campaign PUT must succeed, got %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("want 200, got %d", rec.Code)
		}
		if got := string(svc.rels[777].Metadata); got != `{"price":9}` {
			t.Fatalf("same-campaign metadata not written: %s", got)
		}
	})

	t.Run("delete", func(t *testing.T) {
		svc := newRelationScopeSvc()
		h := NewAPIHandler(nil, nil, nil, svc)

		c, rec := newRelationScopeContext(http.MethodDelete, "campaign-A", "777", nil)
		if err := h.DeleteRelation(c); err != nil {
			t.Fatalf("same-campaign DELETE must succeed, got %v", err)
		}
		if rec.Code != http.StatusNoContent {
			t.Fatalf("want 204, got %d", rec.Code)
		}
		if _, still := svc.rels[777]; still {
			t.Fatalf("same-campaign relation was not deleted")
		}
	})
}

// TestRelationScope_UnknownRelationIs404 pins that a relation that does not
// exist at all still 404s rather than surfacing a raw lookup error.
func TestRelationScope_UnknownRelationIs404(t *testing.T) {
	svc := newRelationScopeSvc()
	h := NewAPIHandler(nil, nil, nil, svc)

	c, _ := newRelationScopeContext(http.MethodDelete, "campaign-A", "99999", nil)
	err := h.DeleteRelation(c)
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != http.StatusNotFound {
		t.Fatalf("want 404 not-found AppError, got %#v", err)
	}
}
