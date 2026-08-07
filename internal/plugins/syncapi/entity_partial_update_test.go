// entity_partial_update_test.go — sweep R4, the syncapi half of the
// absent-means-preserve contract.
//
// Two defects, one shape, both reproduced against the shipped code before
// this file existed:
//
//   - apiUpdateEntityRequest had no parent_id member AT ALL, so every
//     update on the sync wire detached the entity from the Chronicle
//     hierarchy.
//   - is_private was a value-typed bool, so the Foundry actor-sync's
//     {name}-only rename push bound false and PUBLISHED a hidden character
//     entity to every player in the campaign. That is the privacy break.
//
// The batch door (POST .../sync, syncChange) had both of the same holes;
// fixing one and not the other would have left the break reachable.
package syncapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// stubCampaignSvcOwner answers the one call resolveRole makes for a
// session-authed key: the caller is an Owner of the campaign.
type stubCampaignSvcOwner struct {
	campaigns.CampaignService
}

func (stubCampaignSvcOwner) GetMember(_ context.Context, campaignID, userID string) (*campaigns.CampaignMember, error) {
	return &campaigns.CampaignMember{CampaignID: campaignID, UserID: userID, Role: campaigns.RoleOwner}, nil
}

// stubEntityServiceForPartialUpdate captures the UpdateEntityInput the
// handler builds. GetByID answers with a stored entity that is PRIVATE and
// PARENTED — the two things the bugs destroyed.
type stubEntityServiceForPartialUpdate struct {
	entities.EntityService
	updates []entities.UpdateEntityInput
}

func (s *stubEntityServiceForPartialUpdate) GetByID(_ context.Context, id string) (*entities.Entity, error) {
	parent := "parent-1"
	return &entities.Entity{
		ID:         id,
		CampaignID: "camp-1",
		Name:       "Shadow Contact",
		ParentID:   &parent,
		IsPrivate:  true,
	}, nil
}

func (s *stubEntityServiceForPartialUpdate) Update(_ context.Context, id string, input entities.UpdateEntityInput) (*entities.Entity, error) {
	s.updates = append(s.updates, input)
	return &entities.Entity{ID: id, CampaignID: "camp-1"}, nil
}

func newPartialUpdateContext(method, path, body string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "entityID")
	c.SetParamValues("camp-1", "ent-1")
	c.Set(apiKeyContextKey, &APIKey{
		ID:         synthKeySessionID,
		CampaignID: "camp-1",
		UserID:     "user-1",
		IsActive:   true,
	})
	return c, rec
}

// THE privacy regression: the exact body Foundry's actor-sync sends on a
// rename must not touch is_private, and must not touch the parent either.
func TestUpdateEntity_NameOnlyPushLeavesPrivacyAndParentAlone(t *testing.T) {
	svc := &stubEntityServiceForPartialUpdate{}
	h := NewAPIHandler(nil, svc, stubCampaignSvcOwner{}, nil)

	c, _ := newPartialUpdateContext(http.MethodPut, "/api/v1/campaigns/camp-1/entities/ent-1", `{"name":"Shadow Contact (renamed)"}`)
	if err := h.UpdateEntity(c); err != nil {
		t.Fatalf("UpdateEntity: %v", err)
	}
	if len(svc.updates) != 1 {
		t.Fatalf("expected 1 Update call, got %d", len(svc.updates))
	}
	in := svc.updates[0]

	if in.IsPrivate != nil {
		t.Errorf("IsPrivate = %v, want nil (preserve). A non-nil pointer here PUBLISHES a hidden character entity to every player.", *in.IsPrivate)
	}
	if in.ParentID.Present() {
		t.Error("ParentID was sent as present on a {name}-only push; absent must mean preserve, or every sync flattens the hierarchy")
	}
	if in.TypeLabel.Present() {
		t.Error("TypeLabel was sent as present on a {name}-only push; absent must mean preserve")
	}
	if in.Entry.Present() {
		t.Error("Entry was sent as present on a {name}-only push; absent must mean preserve")
	}
	if name, ok := in.Name.Get(); !ok || name != "Shadow Contact (renamed)" {
		t.Errorf("Name = %q (present=%v), want the pushed value", name, ok)
	}
}

// The other two directions on the wire: a present value writes, and an
// explicit null clears the nullable ones.
func TestUpdateEntity_PresentReplacesAndNullClears(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		check func(t *testing.T, in entities.UpdateEntityInput)
	}{
		{
			name: "present is_private writes",
			body: `{"is_private":false}`,
			check: func(t *testing.T, in entities.UpdateEntityInput) {
				if in.IsPrivate == nil || *in.IsPrivate {
					t.Errorf("IsPrivate = %v, want an explicit false", in.IsPrivate)
				}
			},
		},
		{
			name: "present parent_id re-parents",
			body: `{"parent_id":"parent-2"}`,
			check: func(t *testing.T, in entities.UpdateEntityInput) {
				if v, ok := in.ParentID.Get(); !ok || v != "parent-2" {
					t.Errorf("ParentID = %q (present=%v), want parent-2", v, ok)
				}
			},
		},
		{
			name: "explicit null parent_id clears the parent",
			body: `{"parent_id":null}`,
			check: func(t *testing.T, in entities.UpdateEntityInput) {
				if !in.ParentID.IsNull() {
					t.Error("an explicit null parent_id must be present-and-null, i.e. CLEAR")
				}
			},
		},
		{
			name: "explicit null type_label clears the descriptor",
			body: `{"type_label":null}`,
			check: func(t *testing.T, in entities.UpdateEntityInput) {
				if !in.TypeLabel.IsNull() {
					t.Error("an explicit null type_label must be present-and-null, i.e. CLEAR")
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &stubEntityServiceForPartialUpdate{}
			h := NewAPIHandler(nil, svc, stubCampaignSvcOwner{}, nil)
			c, _ := newPartialUpdateContext(http.MethodPut, "/api/v1/campaigns/camp-1/entities/ent-1", tc.body)
			if err := h.UpdateEntity(c); err != nil {
				t.Fatalf("UpdateEntity: %v", err)
			}
			if len(svc.updates) != 1 {
				t.Fatalf("expected 1 Update call, got %d", len(svc.updates))
			}
			tc.check(t, svc.updates[0])
		})
	}
}

// The batch door must behave identically — leaving it asymmetric would have
// been worse than leaving both, because the hole would still be reachable
// and would now be invisible.
func TestSyncBatch_UpdateChangeIsAlsoPartial(t *testing.T) {
	svc := &stubEntityServiceForPartialUpdate{}
	h := NewAPIHandler(nil, svc, stubCampaignSvcOwner{}, nil)

	body := `{"changes":[{"action":"update","entity_id":"ent-1","name":"Renamed via batch"}]}`
	c, _ := newPartialUpdateContext(http.MethodPost, "/api/v1/campaigns/camp-1/sync", body)
	if err := h.Sync(c); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(svc.updates) != 1 {
		t.Fatalf("expected 1 Update call, got %d", len(svc.updates))
	}
	in := svc.updates[0]
	if in.IsPrivate != nil {
		t.Errorf("batch IsPrivate = %v, want nil (preserve) — the same privacy break through the other door", *in.IsPrivate)
	}
	if in.ParentID.Present() {
		t.Error("batch ParentID present on a name-only change; absent must preserve")
	}
	if in.TypeLabel.Present() {
		t.Error("batch TypeLabel present on a name-only change; absent must preserve")
	}
}

// The wire struct itself: decoding a name-only body must leave every other
// field ABSENT. This is the mechanism the two handler tests above rely on,
// pinned directly so a regression names itself.
func TestAPIUpdateEntityRequest_AbsentKeysStayAbsent(t *testing.T) {
	var req apiUpdateEntityRequest
	if err := json.Unmarshal([]byte(`{"name":"only the name"}`), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, f := range []struct {
		name    string
		present bool
	}{
		{"type_label", req.TypeLabel.Present()},
		{"parent_id", req.ParentID.Present()},
		{"is_private", req.IsPrivate.Present()},
		{"entry", req.Entry.Present()},
	} {
		if f.present {
			t.Errorf("%s reported present after a {name}-only body", f.name)
		}
	}
	if req.IsPrivate.Ptr(nil) != nil {
		t.Error("an absent is_private must produce a nil *bool, which the service reads as preserve")
	}
}
