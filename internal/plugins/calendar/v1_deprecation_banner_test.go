// v1_deprecation_banner_test.go covers the Wave 1.5 follow-up V1
// deprecation banner.
//
// As of 2026-05-29 (dispatch C-CAL-V1-BANNER-PULL) the banner is DORMANT:
// its five mount sites in calendar.templ were removed because V2 calendar
// is not yet reachable from campaign chrome. These tests therefore assert
// (a) the banner is no longer mounted on any V1 page, and (b) the
// component + its Switch-to-V2 href helper are kept intact so reinstating
// it under C-CAL-V1-SUNSET is a one-line re-add.

package calendar

import (
	"os"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// --- Dormant-component guards (flip-back safety) ----------------------
//
// The Switch-to-V2 href helper stays exercised so that when the banner is
// reinstated the link target is still correct. Pure function; no mount.

func TestV1BannerSwitchHref_RoutesToV2Shell(t *testing.T) {
	cc := &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: "camp-1"},
	}
	got := string(v1BannerSwitchHref(cc))
	if got != "/campaigns/camp-1/calendar/v2" {
		t.Errorf("switch href = %q; want '/campaigns/camp-1/calendar/v2'", got)
	}
}

func TestV1BannerSwitchHref_NilCCFallsBackSafely(t *testing.T) {
	got := string(v1BannerSwitchHref(nil))
	if got != "/" {
		t.Errorf("nil cc should fall back to /; got %q", got)
	}
}

func TestV1BannerSwitchHref_NilCampaignInsideCC(t *testing.T) {
	cc := &campaigns.CampaignContext{} // Campaign nil
	got := string(v1BannerSwitchHref(cc))
	if got != "/" {
		t.Errorf("nil campaign inside cc should fall back to /; got %q", got)
	}
}

// --- Pull assertions --------------------------------------------------

// TestV1DeprecationBanner_NotMountedOnV1Pages is the inverse of the
// original "banner renders" guard: it asserts the banner is no longer
// mounted on ANY V1 calendar page. We read calendar.templ source and
// confirm no live `@V1DeprecationBanner(...)` mount call survives.
//
// `//`-comment lines are stripped before the check because the pull left
// pointer comments at each former mount site that themselves reference
// the component name (e.g. "re-add `@V1DeprecationBanner(cc)` here"); a
// naive substring scan would false-positive on those.
func TestV1DeprecationBanner_NotMountedOnV1Pages(t *testing.T) {
	b, err := os.ReadFile("calendar.templ")
	if err != nil {
		t.Fatalf("read calendar.templ: %v", err)
	}
	var live strings.Builder
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue // skip comment lines (incl. the pull pointer comments)
		}
		live.WriteString(line)
		live.WriteByte('\n')
	}
	if strings.Contains(live.String(), "@V1DeprecationBanner(") {
		t.Error("calendar.templ still mounts @V1DeprecationBanner; the banner " +
			"was pulled per C-CAL-V1-BANNER-PULL and must not render on V1 pages")
	}
}

// TestV1DeprecationBanner_ComponentKeptDormant pins the Option A choice:
// the component file is preserved intact (not deleted) so reinstating the
// banner under C-CAL-V1-SUNSET is a single-line mount re-add rather than a
// re-implementation. If a future change deletes the component, this test
// fails loudly so the reinstating path is reconsidered deliberately.
func TestV1DeprecationBanner_ComponentKeptDormant(t *testing.T) {
	b, err := os.ReadFile("v1_deprecation_banner.templ")
	if err != nil {
		t.Fatalf("read banner source: %v", err)
	}
	if !strings.Contains(string(b), "templ V1DeprecationBanner(") {
		t.Error("v1_deprecation_banner.templ should keep the dormant " +
			"V1DeprecationBanner component intact for one-line reinstating")
	}
}
