package campaigns

// archive_readonly_test.go pins RequireCampaignAccess's default archive gate:
// an archived campaign refuses POST/PUT/PATCH/DELETE with the same 403 every
// route group built on this middleware inherits automatically, GET/HEAD/
// OPTIONS still pass, and the archive check runs only after membership
// resolves so a non-member's 403 is unchanged by archive state.
// RequireCampaignAccessEvenIfArchived is the escape hatch the handful of
// routes in routes.go's cgArchived group need.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// runCampaignAccess drives gate (RequireCampaignAccess or
// RequireCampaignAccessEvenIfArchived) for one request/method against a real
// Echo router. Returns the recorder (Header "X-Error-Message" carries an
// AppError's Message, for tests that need to distinguish which 403 fired)
// plus whether the terminal handler was reached.
func runCampaignAccess(svc CampaignService, session *auth.Session, method string, gate func(CampaignService) echo.MiddlewareFunc) (*httptest.ResponseRecorder, bool) {
	e := echo.New()
	e.HTTPErrorHandler = func(c *echo.Context, err error) {
		if ae, ok := err.(*apperror.AppError); ok {
			c.Response().Header().Set("X-Error-Message", ae.Message)
			_ = c.NoContent(ae.Code)
			return
		}
		_ = c.NoContent(http.StatusInternalServerError)
	}

	setSession := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if session != nil {
				auth.SetSession(c, session)
			}
			return next(c)
		}
	}

	reached := false
	g := e.Group("/campaigns/:id", setSession, gate(svc))
	g.Add(method, "", func(c *echo.Context) error {
		reached = true
		return c.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(method, "/campaigns/camp-1", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec, reached
}

func archivedCampaign() *Campaign {
	at := time.Now()
	return &Campaign{ID: "camp-1", ArchivedAt: &at}
}

func activeCampaign() *Campaign {
	return &Campaign{ID: "camp-1"}
}

// TestRequireCampaignAccess_ArchiveGate is the table-driven matrix of
// method x archive-state: only the four write verbs are blocked, and only
// when the campaign is archived.
func TestRequireCampaignAccess_ArchiveGate(t *testing.T) {
	owner := &auth.Session{UserID: "owner-1"}
	ownerMembership := &CampaignMember{UserID: "owner-1", Role: RoleOwner}

	cases := []struct {
		name        string
		method      string
		archived    bool
		wantStatus  int
		wantReached bool
	}{
		{"GET active", http.MethodGet, false, http.StatusOK, true},
		{"GET archived", http.MethodGet, true, http.StatusOK, true},
		{"HEAD archived", http.MethodHead, true, http.StatusOK, true},
		{"OPTIONS archived", http.MethodOptions, true, http.StatusOK, true},
		{"POST active", http.MethodPost, false, http.StatusOK, true},
		{"POST archived", http.MethodPost, true, http.StatusForbidden, false},
		{"PUT active", http.MethodPut, false, http.StatusOK, true},
		{"PUT archived", http.MethodPut, true, http.StatusForbidden, false},
		{"PATCH active", http.MethodPatch, false, http.StatusOK, true},
		{"PATCH archived", http.MethodPatch, true, http.StatusForbidden, false},
		{"DELETE active", http.MethodDelete, false, http.StatusOK, true},
		{"DELETE archived", http.MethodDelete, true, http.StatusForbidden, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			campaign := activeCampaign()
			if tc.archived {
				campaign = archivedCampaign()
			}
			svc := &stubPublicSvc{campaign: campaign, member: ownerMembership}
			rec, reached := runCampaignAccess(svc, owner, tc.method, RequireCampaignAccess)
			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d; body message = %q", rec.Code, tc.wantStatus, rec.Header().Get("X-Error-Message"))
			}
			if reached != tc.wantReached {
				t.Errorf("handler reached = %v, want %v", reached, tc.wantReached)
			}
		})
	}
}

// TestRequireCampaignAccess_NonMemberRejectionUnaffectedByArchiveState pins
// that the archive check runs strictly after membership resolves: a
// non-member gets the same "not a member" 403 whether the campaign is active
// or archived, never the archive message (which would leak archive state to
// someone who isn't even a member).
func TestRequireCampaignAccess_NonMemberRejectionUnaffectedByArchiveState(t *testing.T) {
	stranger := &auth.Session{UserID: "stranger-1"}
	notMember := apperror.NewNotFound("not a member")

	for _, tc := range []struct {
		name     string
		campaign *Campaign
	}{
		{"active campaign", activeCampaign()},
		{"archived campaign", archivedCampaign()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &stubPublicSvc{campaign: tc.campaign, memberErr: notMember}
			rec, reached := runCampaignAccess(svc, stranger, http.MethodPost, RequireCampaignAccess)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403", rec.Code)
			}
			if reached {
				t.Error("non-member must not reach the handler")
			}
			if got := rec.Header().Get("X-Error-Message"); got != "you are not a member of this campaign" {
				t.Errorf("message = %q, want the not-a-member message regardless of archive state", got)
			}
		})
	}
}

// TestRequireCampaignAccessEvenIfArchived_LetsWriteThrough is the escape
// hatch's positive case: a member's write reaches the handler even though
// the campaign is archived.
func TestRequireCampaignAccessEvenIfArchived_LetsWriteThrough(t *testing.T) {
	owner := &auth.Session{UserID: "owner-1"}
	svc := &stubPublicSvc{
		campaign: archivedCampaign(),
		member:   &CampaignMember{UserID: "owner-1", Role: RoleOwner},
	}
	rec, reached := runCampaignAccess(svc, owner, http.MethodPost, RequireCampaignAccessEvenIfArchived)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 -- EvenIfArchived must not block a write", rec.Code)
	}
	if !reached {
		t.Error("handler must be reached")
	}
}

// TestRequireCampaignAccessEvenIfArchived_StillRejectsNonMember confirms the
// escape hatch only removes the archive check, not membership resolution.
func TestRequireCampaignAccessEvenIfArchived_StillRejectsNonMember(t *testing.T) {
	stranger := &auth.Session{UserID: "stranger-1"}
	svc := &stubPublicSvc{campaign: archivedCampaign(), memberErr: apperror.NewNotFound("not a member")}
	rec, reached := runCampaignAccess(svc, stranger, http.MethodPost, RequireCampaignAccessEvenIfArchived)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if reached {
		t.Error("non-member must not reach the handler even on the archive-exempt gate")
	}
}

// TestCampaignRouteTable_ArchivedCampaign drives the real two-group route
// split routes.go uses: an ordinary owner write (mirroring PUT "") on the
// RequireCampaignAccess group, and unarchive/delete (mirroring POST
// "/unarchive" and DELETE "") on the RequireCampaignAccessEvenIfArchived
// group. This is the level RequireCampaignAccess's own unit test can't
// reach: a route accidentally registered on cg instead of cgArchived (or
// vice versa) would pass the middleware test above yet fail here, since here
// it's routes.go's actual group wiring under test, not the gate alone.
func TestCampaignRouteTable_ArchivedCampaign(t *testing.T) {
	owner := &auth.Session{UserID: "owner-1"}
	svc := &stubPublicSvc{
		campaign: archivedCampaign(),
		member:   &CampaignMember{UserID: "owner-1", Role: RoleOwner},
	}

	e := echo.New()
	e.HTTPErrorHandler = func(c *echo.Context, err error) {
		if ae, ok := err.(*apperror.AppError); ok {
			_ = c.NoContent(ae.Code)
			return
		}
		_ = c.NoContent(http.StatusInternalServerError)
	}
	setSession := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			auth.SetSession(c, owner)
			return next(c)
		}
	}
	ok := func(c *echo.Context) error { return c.NoContent(http.StatusOK) }

	// Mirrors routes.go: cg defaults to blocking writes on archive; cgArchived
	// is the short exception list (unarchive, delete).
	cg := e.Group("/campaigns/:id", setSession, RequireCampaignAccess(svc))
	cg.PUT("", ok, RequireRole(RoleOwner))

	cgArchived := e.Group("/campaigns/:id", setSession, RequireCampaignAccessEvenIfArchived(svc))
	cgArchived.POST("/unarchive", ok, RequireRole(RoleOwner))
	cgArchived.DELETE("", ok, RequireRole(RoleOwner))

	t.Run("ordinary write refused", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/campaigns/camp-1", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("PUT on archived campaign: status = %d, want 403", rec.Code)
		}
	})

	t.Run("unarchive still works", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/campaigns/camp-1/unarchive", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("POST /unarchive on archived campaign: status = %d, want 200", rec.Code)
		}
	})

	t.Run("delete still works", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/campaigns/camp-1", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("DELETE on archived campaign: status = %d, want 200", rec.Code)
		}
	})
}

// TestArchivedExceptionList pins which routes in routes.go stay writable on an
// archived campaign. Adding one is a deliberate choice, so it has to be made
// here as well as there.
func TestArchivedExceptionList(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "routes.go", nil, 0)
	if err != nil {
		t.Fatalf("parse routes.go: %v", err)
	}
	got := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if recv, ok := sel.X.(*ast.Ident); !ok || recv.Name != "cgArchived" {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok {
			t.Fatalf("cgArchived.%s registered with a non-literal path", sel.Sel.Name)
		}
		path, err := strconv.Unquote(lit.Value)
		if err != nil {
			t.Fatalf("unquote %s: %v", lit.Value, err)
		}
		got[sel.Sel.Name+" "+path] = true
		return true
	})
	want := map[string]bool{
		"DELETE ":                true,
		"POST /unarchive":        true,
		"POST /toggle-view-mode": true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("routes writable on an archived campaign = %v, want %v", got, want)
	}
}
