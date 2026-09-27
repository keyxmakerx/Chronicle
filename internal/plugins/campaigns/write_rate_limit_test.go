package campaigns

// write_rate_limit_test.go pins the per-user write budget on both campaign
// gates; reads are never counted. The limiter is shared process-wide, so each
// test uses its own user ID.

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// TestRequireCampaignAccess_WriteRateLimit drives campaignWriteRateLimitBudget+1
// writes from the same member through a fresh RequireCampaignAccess instance
// and asserts the one over budget is refused with 429, while a read from the
// same user in between is never counted against the write budget.
func TestRequireCampaignAccess_WriteRateLimit(t *testing.T) {
	owner := &auth.Session{UserID: "write-limit-owner"}
	svc := &stubPublicSvc{
		campaign: activeCampaign(),
		member:   &CampaignMember{UserID: "write-limit-owner", Role: RoleOwner},
	}

	e := echo.New()
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if ae, ok := err.(*apperror.AppError); ok {
			_ = c.NoContent(ae.Code)
			return
		}
		_ = c.NoContent(http.StatusInternalServerError)
	}
	setSession := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			auth.SetSession(c, owner)
			return next(c)
		}
	}
	ok := func(c echo.Context) error { return c.NoContent(http.StatusOK) }

	g := e.Group("/campaigns/:id", setSession, RequireCampaignAccess(svc))
	g.PUT("", ok)
	g.GET("", ok)

	// Exhaust the write budget.
	var lastWriteCode int
	for i := 0; i < campaignWriteRateLimitBudget+1; i++ {
		req := httptest.NewRequest(http.MethodPut, "/campaigns/camp-1", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		lastWriteCode = rec.Code
	}
	if lastWriteCode != http.StatusTooManyRequests {
		t.Fatalf("write %d: status = %d, want %d", campaignWriteRateLimitBudget+1, lastWriteCode, http.StatusTooManyRequests)
	}

	// A read from the same user must still succeed — reads aren't rate
	// limited by this gate.
	req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("GET after write budget exhausted: status = %d, want 200", rec.Code)
	}
}

// TestRequireCampaignAccess_WriteRateLimit_SharedAcrossInstances pins that
// the budget is per user across the app: two gate instances, built the way
// two plugins would build them, share one counter.
func TestRequireCampaignAccess_WriteRateLimit_SharedAcrossInstances(t *testing.T) {
	owner := &auth.Session{UserID: "write-limit-shared"}
	svc := &stubPublicSvc{
		campaign: activeCampaign(),
		member:   &CampaignMember{UserID: "write-limit-shared", Role: RoleOwner},
	}

	e := echo.New()
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if ae, ok := err.(*apperror.AppError); ok {
			_ = c.NoContent(ae.Code)
			return
		}
		_ = c.NoContent(http.StatusInternalServerError)
	}
	setSession := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			auth.SetSession(c, owner)
			return next(c)
		}
	}
	ok := func(c echo.Context) error { return c.NoContent(http.StatusOK) }

	// Two independent gate instances, as two different plugins' routes.go
	// would each produce by calling RequireCampaignAccess(svc) themselves.
	gEntities := e.Group("/entities/:id", setSession, RequireCampaignAccess(svc))
	gEntities.PUT("", ok)
	gMaps := e.Group("/maps/:id", setSession, RequireCampaignAccess(svc))
	gMaps.PUT("", ok)

	// Exhaust the budget entirely through the first instance.
	var lastCode int
	for i := 0; i < campaignWriteRateLimitBudget; i++ {
		req := httptest.NewRequest(http.MethodPut, "/entities/camp-1", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		lastCode = rec.Code
	}
	if lastCode != http.StatusOK {
		t.Fatalf("write %d on first instance: status = %d, want 200 (budget not yet exhausted)", campaignWriteRateLimitBudget, lastCode)
	}

	// A single write through the SECOND instance, for the same user, must
	// already be over budget if the limiter is truly shared.
	req := httptest.NewRequest(http.MethodPut, "/maps/camp-1", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("write on second RequireCampaignAccess instance after first instance's budget was exhausted: status = %d, want %d (limiter is not shared across instances)", rec.Code, http.StatusTooManyRequests)
	}
}

// TestRequireCampaignAccessEvenIfArchived_WriteRateLimit pins the same budget
// on the archive-exempt gate, since cgArchived groups carry real writes too
// (unarchive, delete).
func TestRequireCampaignAccessEvenIfArchived_WriteRateLimit(t *testing.T) {
	owner := &auth.Session{UserID: "write-limit-archived"}
	svc := &stubPublicSvc{
		campaign: archivedCampaign(),
		member:   &CampaignMember{UserID: "write-limit-archived", Role: RoleOwner},
	}

	e := echo.New()
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if ae, ok := err.(*apperror.AppError); ok {
			_ = c.NoContent(ae.Code)
			return
		}
		_ = c.NoContent(http.StatusInternalServerError)
	}
	setSession := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			auth.SetSession(c, owner)
			return next(c)
		}
	}
	ok := func(c echo.Context) error { return c.NoContent(http.StatusOK) }

	g := e.Group("/campaigns/:id", setSession, RequireCampaignAccessEvenIfArchived(svc))
	g.POST("/unarchive", ok)

	var lastCode int
	for i := 0; i < campaignWriteRateLimitBudget+1; i++ {
		req := httptest.NewRequest(http.MethodPost, "/campaigns/camp-1/unarchive", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		lastCode = rec.Code
	}
	if lastCode != http.StatusTooManyRequests {
		t.Fatalf("write %d: status = %d, want %d", campaignWriteRateLimitBudget+1, lastCode, http.StatusTooManyRequests)
	}
}
