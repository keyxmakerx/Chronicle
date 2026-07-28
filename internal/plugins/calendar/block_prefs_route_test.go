// block_prefs_route_test.go — dispatch §12.1's security review, in executable
// form (C-CALV4-LAYERS-P9 [LYR-1 / LYR-4 SIGNED]).
//
// Each heading below is one claim from that review, and the assertion under it
// is the proof. This is the one new Echo route the whole calendar-v4 wave
// takes, so the review is short by CONSTRUCTION rather than by argument: the
// response has no body, so there is nothing to leak into; the request carries
// no host descriptor, so there is nothing to forge.
package calendar

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	calblock "github.com/keyxmakerx/chronicle/internal/widgets/calendar_block"
)

// servePrefs drives BlockPrefsAPI the way HTMX would: form-encoded, with the
// session user and the authorised campaign in context and nothing else.
func servePrefs(h *Handler, body string, userID string) (*httptest.ResponseRecorder, error) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/campaigns/camp-1/calendar/prefs",
		strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("camp-1")
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RolePlayer, IsMember: true,
	})
	if userID != "" {
		c.Set("auth_user_id", userID)
	}
	return rec, h.BlockPrefsAPI(c)
}

// §12.1 "What the route exposes: NOTHING." 204, no body, HX-Refresh: true. The
// host page rebuilds every Block through the handler that already owns its
// visibility decisions — that is what keeps the W5a split intact without this
// route knowing anything about it.
func TestBlockPrefs_AnswersNoContentAndRefreshes(t *testing.T) {
	h := NewHandler(NewCalendarService(&mockCalendarRepo{}))
	rec, err := servePrefs(h, "layers=moons,eras", "user-1")
	if err != nil {
		t.Fatalf("BlockPrefsAPI: %v", err)
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204 — a body is a render, and a render needs host "+
			"facts this route deliberately does not take", rec.Code)
	}
	if body := rec.Body.String(); body != "" {
		t.Errorf("response body = %q, want empty — there is no fragment to leak into", body)
	}
	if got := rec.Header().Get("HX-Refresh"); got != "true" {
		t.Errorf("HX-Refresh = %q, want \"true\" — without it the toggle persists and "+
			"nothing on screen changes", got)
	}
}

// §12.1 "Whose row it writes." user_id comes from the SESSION, never the body.
// Proven by SUPPLYING one: the request names somebody else three ways and the
// write still lands on the authenticated caller.
func TestBlockPrefs_BodySuppliedIdentityIsIgnored(t *testing.T) {
	for _, body := range []string{
		"layers=moons&user_id=victim",
		"layers=moons&userId=victim",
		"layers=moons&user=victim",
	} {
		var gotUser, gotCampaign string
		h := NewHandler(NewCalendarService(&mockCalendarRepo{
			setBlockLayersFn: func(_ context.Context, userID, campaignID string, _ []string) error {
				gotUser, gotCampaign = userID, campaignID
				return nil
			},
		}))
		if _, err := servePrefs(h, body, "user-1"); err != nil {
			t.Fatalf("%s: %v", body, err)
		}
		if gotUser != "user-1" {
			t.Errorf("%s wrote user %q; the primary key is (SESSION user, authorised campaign) "+
				"and there is no parameter that can move it", body, gotUser)
		}
		if gotCampaign != "camp-1" {
			t.Errorf("%s wrote campaign %q; campaign_id comes from :id, which "+
				"RequireCampaignAccess has already authorised", body, gotCampaign)
		}
	}
}

// §12.1's W5a demand 1: THE ROUTE TAKES NO HOST DESCRIPTOR. A calendar_id, an
// entity_id or a binding source in the body must change nothing — each of them
// would otherwise be a resolver call this handler does not make, and that is
// exactly where a leak hides.
func TestBlockPrefs_HostDescriptorsAreNotAccepted(t *testing.T) {
	var gotKeys []string
	h := NewHandler(NewCalendarService(&mockCalendarRepo{
		setBlockLayersFn: func(_ context.Context, _, _ string, keys []string) error {
			gotKeys = keys
			return nil
		},
	}))
	rec, err := servePrefs(h,
		"layers=moons&calendar_id=cal-hidden&entity_id=ent-9&source=own&campaign_id=camp-other",
		"user-1")
	if err != nil {
		t.Fatalf("BlockPrefsAPI: %v", err)
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if len(gotKeys) != 1 || gotKeys[0] != "moons" {
		t.Errorf("persisted %v; the route reads `layers` and nothing else", gotKeys)
	}
}

// §12.1 "What it accepts": an unknown key REJECTS the whole request. A silently
// dropped key half-applies a choice and the viewer cannot tell which half
// landed.
func TestBlockPrefs_UnknownKeyIsRejectedNotDropped(t *testing.T) {
	called := false
	h := NewHandler(NewCalendarService(&mockCalendarRepo{
		setBlockLayersFn: func(_ context.Context, _, _ string, _ []string) error {
			called = true
			return nil
		},
	}))
	_, err := servePrefs(h, "layers=moons,skybox", "user-1")
	if err == nil {
		t.Fatal("an unknown layer key must fail the request")
	}
	if called {
		t.Error("the repository was written despite the invalid key")
	}
}

// The empty set is a REAL CHOICE — a bare month — and it is spelled `layers=`.
// A missing `layers` field is a malformed request, and the two must not be
// indistinguishable.
func TestBlockPrefs_EmptySetIsAChoiceAndMissingIsAnError(t *testing.T) {
	var got []string
	var called bool
	h := NewHandler(NewCalendarService(&mockCalendarRepo{
		setBlockLayersFn: func(_ context.Context, _, _ string, keys []string) error {
			called, got = true, keys
			return nil
		},
	}))
	rec, err := servePrefs(h, "layers=", "user-1")
	if err != nil {
		t.Fatalf("`layers=` must be accepted: %v", err)
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if !called || got == nil || len(got) != 0 {
		t.Errorf("the bare month persisted as %#v; want an empty non-nil slice", got)
	}

	h2 := NewHandler(NewCalendarService(&mockCalendarRepo{}))
	if _, err := servePrefs(h2, "somethingelse=1", "user-1"); err == nil {
		t.Error("a request with no `layers` field must be a 400, not a silent bare month")
	}
}

// §12.1 "Who may call it." The group stack enforces membership and the addon;
// the handler's own floor is that an unauthenticated caller is 401, not 500.
func TestBlockPrefs_UnauthenticatedRejects(t *testing.T) {
	h := NewHandler(NewCalendarService(&mockCalendarRepo{}))
	if _, err := servePrefs(h, "layers=moons", ""); err == nil {
		t.Fatal("no session user must reject")
	}
}

// The full eight-key registry round-trips, so the route is not quietly narrower
// than the switchboard it serves. Duplicates collapse; order is preserved.
func TestBlockPrefs_WholeRegistryRoundTrips(t *testing.T) {
	var got []string
	h := NewHandler(NewCalendarService(&mockCalendarRepo{
		setBlockLayersFn: func(_ context.Context, _, _ string, keys []string) error {
			got = keys
			return nil
		},
	}))
	form := url.Values{"layers": {strings.Join(calblock.LayerKeys, ",")}}
	if _, err := servePrefs(h, form.Encode(), "user-1"); err != nil {
		t.Fatalf("BlockPrefsAPI: %v", err)
	}
	if len(got) != len(calblock.LayerKeys) {
		t.Errorf("persisted %d keys, want all %d", len(got), len(calblock.LayerKeys))
	}
}

// §12.1 "Egress." Layer preferences are display state, not campaign content.
// The column must not appear in any export/DTO surface — the availability
// precedent is that member-scoped rows stay out of egress BY CONSTRUCTION, and
// a preference column is the same class. Asserted as an absence over the
// package's own SQL, because the alternative is a promise.
func TestBlockPrefs_ColumnIsNotInAnyEgressQuery(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	for _, f := range files {
		if f == "repository.go" || strings.HasSuffix(f, "_test.go") {
			continue // the repository OWNS the SQL; that is the one legitimate site
		}
		body, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		// Comments are stripped first: a file may legitimately NAME the column
		// while explaining why it does not query it, and prose about a rule
		// must never be the thing that trips the rule.
		if strings.Contains(stripGoComments(string(body)), "block_layers") {
			t.Errorf("%s names the block_layers COLUMN — a per-viewer display preference is "+
				"not campaign content and must not reach an export/DTO/egress surface; the "+
				"column belongs to repository.go alone", f)
		}
	}
}

// stripGoComments blanks // and /* */ spans so the egress assertion above judges
// CODE and never prose.
func stripGoComments(src string) string {
	return goCommentRe.ReplaceAllString(src, " ")
}

var goCommentRe = regexp.MustCompile(`(?s)/\*.*?\*/|//[^\n]*`)
