package syncapi

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// --- minimal service stubs for CreateEntity handler tests ---
//
// CreateEntity exercises only a small slice of the EntityService /
// CampaignService surfaces. Rather than hand-fill all ~90 EntityService
// methods (cf. stubCalendarSvc), we embed the interface so unimplemented
// methods are present-but-panic, and override only what the code under
// test calls. The P1 zero-type path calls NOTHING on either service, so
// it can't accidentally rely on a stubbed default. (Matches the
// egress_sanitize_test.go rationale for not wiring the full interface.)

// stubEntityServiceForCreate embeds entities.EntityService; only Create
// and GetEntityTypes are reachable from CreateEntity. createFn captures
// the input so tests can assert the EntityTypeID that flowed through.
type stubEntityServiceForCreate struct {
	entities.EntityService
	createFn func(ctx context.Context, campaignID, userID string, input entities.CreateEntityInput) (*entities.Entity, error)
}

func (s *stubEntityServiceForCreate) Create(ctx context.Context, campaignID, userID string, input entities.CreateEntityInput) (*entities.Entity, error) {
	if s.createFn != nil {
		return s.createFn(ctx, campaignID, userID, input)
	}
	return &entities.Entity{ID: "ent-new", CampaignID: campaignID, EntityTypeID: input.EntityTypeID}, nil
}

// GetEntityTypes must NEVER be hit on the zero-type path post-fix
// (the handler 400s before any lookup). Fail loudly if it is — that
// would mean the silent-default coercion came back.
func (s *stubEntityServiceForCreate) GetEntityTypes(_ context.Context, _ string) ([]entities.EntityType, error) {
	return nil, errors.New("GetEntityTypes must not be called: zero entity_type_id must 400, not default to types[0]")
}

// stubCampaignServiceForCreate embeds campaigns.CampaignService. Only
// GetMember is reachable from CreateEntity (and only when owner_user_id
// is supplied, which these tests don't do).
type stubCampaignServiceForCreate struct {
	campaigns.CampaignService
}

// newCreateEntityContext builds an Echo context for POST
// /api/v1/campaigns/camp-1/entities with the given JSON body and a
// synthetic Owner-role API key on the context (so GetAPIKey / resolveRole
// succeed without a real auth chain).
func newCreateEntityContext(body []byte) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/camp-1/entities", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("camp-1")
	c.Set(apiKeyContextKey, &APIKey{
		ID:         synthKeySessionID,
		CampaignID: "camp-1",
		UserID:     "user-1",
		IsActive:   true,
	})
	return c, rec
}

// TestCreateEntity_RejectsZeroType is the P1 fix: a missing / malformed
// entity_type_id (binds to 0) must 400 "entity_type_id is required"
// rather than silently coercing to the campaign's first type. This is the
// server-side root of the calendar-as-Characters bug class.
func TestCreateEntity_RejectsZeroType(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"omitted field", `{"name":"Zaltar"}`},
		{"explicit zero", `{"name":"Zaltar","entity_type_id":0}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewAPIHandler(nil, &stubEntityServiceForCreate{}, &stubCampaignServiceForCreate{}, nil)
			c, _ := newCreateEntityContext([]byte(tc.body))

			err := h.CreateEntity(c)
			if err == nil {
				t.Fatalf("expected 400 error, got nil")
			}
			var appErr *apperror.AppError
			if !errors.As(err, &appErr) {
				t.Fatalf("expected *apperror.AppError, got %T: %v", err, err)
			}
			if appErr.Code != http.StatusBadRequest {
				t.Errorf("expected status 400, got %d", appErr.Code)
			}
			if appErr.Message != "entity_type_id is required" {
				t.Errorf("expected message %q, got %q", "entity_type_id is required", appErr.Message)
			}
		})
	}
}

// TestCreateEntity_AcceptsValidType pins the happy path: a real
// entity_type_id flows through to the service unchanged and yields 201.
// Guards against the fix over-rejecting (e.g. rejecting all creates).
func TestCreateEntity_AcceptsValidType(t *testing.T) {
	var gotType int
	svc := &stubEntityServiceForCreate{
		createFn: func(_ context.Context, campaignID, _ string, input entities.CreateEntityInput) (*entities.Entity, error) {
			gotType = input.EntityTypeID
			return &entities.Entity{ID: "ent-7", CampaignID: campaignID, EntityTypeID: input.EntityTypeID}, nil
		},
	}
	h := NewAPIHandler(nil, svc, &stubCampaignServiceForCreate{}, nil)
	c, rec := newCreateEntityContext([]byte(`{"name":"Zaltar","entity_type_id":42}`))

	if err := h.CreateEntity(c); err != nil {
		t.Fatalf("CreateEntity returned error: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201 Created, got %d", rec.Code)
	}
	if gotType != 42 {
		t.Errorf("expected EntityTypeID 42 to pass through, got %d", gotType)
	}
}

// TestSync_BatchCreatePassesType confirms the internal batch-sync create
// path is unaffected by the P1 fix: it builds CreateEntityInput directly
// from change.EntityTypeID (no zero-coercion was ever applied there), so a
// real type still reaches the service and the result is "ok". The fix
// lives in the handler's CreateEntity, not in the shared service Create,
// so the batch path was never coupled to the removed default.
func TestSync_BatchCreatePassesType(t *testing.T) {
	var gotType int
	called := false
	svc := &stubEntityServiceForCreate{
		createFn: func(_ context.Context, _, _ string, input entities.CreateEntityInput) (*entities.Entity, error) {
			called = true
			gotType = input.EntityTypeID
			return &entities.Entity{ID: "ent-batch", EntityTypeID: input.EntityTypeID}, nil
		},
	}
	h := NewAPIHandler(nil, svc, &stubCampaignServiceForCreate{}, nil)

	body := []byte(`{"changes":[{"action":"create","name":"Bridge","entity_type_id":9}]}`)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/camp-1/sync", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("camp-1")
	c.Set(apiKeyContextKey, &APIKey{ID: synthKeySessionID, CampaignID: "camp-1", UserID: "user-1", IsActive: true})

	if err := h.Sync(c); err != nil {
		t.Fatalf("Sync returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}
	if !called {
		t.Fatalf("batch create did not invoke entitySvc.Create")
	}
	if gotType != 9 {
		t.Errorf("expected batch EntityTypeID 9, got %d", gotType)
	}
}

// resolveRole on a CreateEntity/Sync call path: with a synthetic
// session key whose CampaignID matches, resolveRole still calls
// GetMember; the embedded CampaignService would panic. CreateEntity's
// zero-type branch returns BEFORE resolveRole runs, and the valid-type
// branch here doesn't supply owner_user_id, so GetMember is never hit on
// the create handler. Sync, however, calls resolveRole up front — so we
// give the stub a GetMember.
func (s *stubCampaignServiceForCreate) GetMember(_ context.Context, campaignID, userID string) (*campaigns.CampaignMember, error) {
	return &campaigns.CampaignMember{CampaignID: campaignID, UserID: userID, Role: campaigns.RoleOwner}, nil
}
