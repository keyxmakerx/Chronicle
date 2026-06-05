// cogm_capability_test.go — C-CAL-COGM-CAPABILITY (Phase 3 / D6).
// The co-DM capability truth table + the RequireCapability gate: a DM-grantee
// now has Owner-equivalent live-play powers (world-state control + dm_only
// authoring), while Scribe/Player do not.
package campaigns

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestCapabilityTruthTable(t *testing.T) {
	cases := []struct {
		name      string
		role      Role
		dmGranted bool
		want      bool
	}{
		{"owner", RoleOwner, false, true},
		{"dm-granted scribe", RoleScribe, true, true},
		{"dm-granted player", RolePlayer, true, true},
		{"plain scribe", RoleScribe, false, false},
		{"plain player", RolePlayer, false, false},
		{"none", RoleNone, false, false},
	}
	for _, tc := range cases {
		cc := &CampaignContext{MemberRole: tc.role, IsDmGranted: tc.dmGranted}
		if got := cc.CanControlWorldState(); got != tc.want {
			t.Errorf("%s: CanControlWorldState = %v, want %v", tc.name, got, tc.want)
		}
		if got := cc.CanAuthorDmOnly(); got != tc.want {
			t.Errorf("%s: CanAuthorDmOnly = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestRequireCapability_GatesWorldStateControl: the middleware passes Owner +
// DM-grantee and 403s Scribe/Player — the world-state PUT gate semantics.
func TestRequireCapability_GatesWorldStateControl(t *testing.T) {
	mw := RequireCapability((*CampaignContext).CanControlWorldState, "nope")
	final := func(c echo.Context) error { return c.NoContent(http.StatusOK) }
	handler := mw(final)

	// passed reports whether the gate let the request through (err nil + 200).
	passed := func(cc *CampaignContext) bool {
		e := echo.New()
		req := httptest.NewRequest(http.MethodPut, "/x", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set(contextKeyCampaign, cc)
		err := handler(c)
		return err == nil && rec.Code == http.StatusOK
	}

	if !passed(&CampaignContext{MemberRole: RoleOwner}) {
		t.Errorf("owner should pass the world-state gate")
	}
	if !passed(&CampaignContext{MemberRole: RoleScribe, IsDmGranted: true}) {
		t.Errorf("co-DM (dm-granted) should pass the world-state gate")
	}
	if passed(&CampaignContext{MemberRole: RoleScribe}) {
		t.Errorf("plain scribe should be denied")
	}
	if passed(&CampaignContext{MemberRole: RolePlayer}) {
		t.Errorf("plain player should be denied")
	}
}
