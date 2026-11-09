package syncapi

// campaign_match_archive_test.go pins RequireCampaignMatch's archive gate: a
// Bearer-key write against an archived campaign is refused with the same 403
// the web app uses. Bearer requests never carry a session, so
// campaigns.RequireCampaignAccess (which enforces this for the web app) never
// runs on /api/v1/*; RequireCampaignMatch is the one place on that whole
// group that can.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// newCampaignMatchFixture wires RequireCampaignMatch alone behind a
// middleware that injects key directly into context, standing in for the
// auth middleware that normally resolves it first in production.
func newCampaignMatchFixture(campSvc campaigns.CampaignService, key *APIKey) *echo.Echo {
	e := echo.New()
	e.HTTPErrorHandler = func(c *echo.Context, err error) {
		type errBody struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if appErr, ok := err.(*apperror.AppError); ok {
			_ = c.JSON(appErr.Code, errBody{Error: appErr.Type, Message: appErr.Message})
			return
		}
		_ = c.JSON(http.StatusInternalServerError, errBody{Error: "internal_error"})
	}
	injectKey := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Set(apiKeyContextKey, key)
			return next(c)
		}
	}
	g := e.Group("/api/v1/campaigns/:id", injectKey, RequireCampaignMatch(campSvc))
	pong := func(c *echo.Context) error { return c.NoContent(http.StatusOK) }
	g.GET("", pong)
	g.POST("", pong)
	g.PUT("", pong)
	g.PATCH("", pong)
	g.DELETE("", pong)
	return e
}

// TestRequireCampaignMatch_BlocksWritesOnArchivedCampaign drives all four
// write verbs against an archived campaign.
func TestRequireCampaignMatch_BlocksWritesOnArchivedCampaign(t *testing.T) {
	at := time.Now()
	campSvc := &fakeCampaignService{
		getByIDFn: func(_ context.Context, id string) (*campaigns.Campaign, error) {
			return &campaigns.Campaign{ID: id, ArchivedAt: &at}, nil
		},
	}
	key := &APIKey{ID: 1, CampaignID: "camp-1"}
	e := newCampaignMatchFixture(campSvc, key)

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/v1/campaigns/camp-1", nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("%s on archived campaign: status = %d, want 403; body = %s", method, rec.Code, rec.Body.String())
			}
			if !containsIgnoreCase(rec.Body.String(), "archived") {
				t.Errorf("%s: body does not mention archived: %s", method, rec.Body.String())
			}
		})
	}
}

// TestRequireCampaignMatch_ReadsPassThroughOnArchivedCampaign confirms a GET
// is never blocked, and — since archive state is irrelevant to a read — the
// middleware doesn't even pay for a GetByID call to find out.
func TestRequireCampaignMatch_ReadsPassThroughOnArchivedCampaign(t *testing.T) {
	campSvc := &fakeCampaignService{
		getByIDFn: func(_ context.Context, id string) (*campaigns.Campaign, error) {
			t.Fatal("GetByID must not be called for a safe method")
			return nil, nil
		},
	}
	key := &APIKey{ID: 1, CampaignID: "camp-1"}
	e := newCampaignMatchFixture(campSvc, key)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/campaigns/camp-1", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET: status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
}

// TestRequireCampaignMatch_WritesPassThroughOnActiveCampaign is the
// necessary negative-space check: the gate must not block every write, only
// ones against an archived campaign.
func TestRequireCampaignMatch_WritesPassThroughOnActiveCampaign(t *testing.T) {
	campSvc := &fakeCampaignService{
		getByIDFn: func(_ context.Context, id string) (*campaigns.Campaign, error) {
			return &campaigns.Campaign{ID: id}, nil
		},
	}
	key := &APIKey{ID: 1, CampaignID: "camp-1"}
	e := newCampaignMatchFixture(campSvc, key)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/camp-1", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST on active campaign: status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
}

// TestRequireCampaignMatch_CampaignMismatchStillWinsOverArchiveCheck pins the
// existing ordering: a key scoped to a different campaign is refused before
// the new archive check ever runs (and never spends a GetByID call finding
// out), same as it always has been.
func TestRequireCampaignMatch_CampaignMismatchStillWinsOverArchiveCheck(t *testing.T) {
	campSvc := &fakeCampaignService{
		getByIDFn: func(_ context.Context, id string) (*campaigns.Campaign, error) {
			t.Fatal("GetByID must not be called once the campaign-match check has already failed")
			return nil, nil
		},
	}
	key := &APIKey{ID: 1, CampaignID: "camp-OTHER"}
	e := newCampaignMatchFixture(campSvc, key)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/camp-1", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("mismatched campaign: status = %d, want 403; body = %s", rec.Code, rec.Body.String())
	}
	if !containsIgnoreCase(rec.Body.String(), "not authorized for this campaign") {
		t.Errorf("body does not mention campaign mismatch: %s", rec.Body.String())
	}
}
