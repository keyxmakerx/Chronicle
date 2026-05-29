// v1_deprecation_banner_test.go covers the Wave 1.5 follow-up V1
// deprecation banner. Verifies the Switch-to-V2 link target +
// safe-handling of nil campaign context.

package calendar

import (
	"os"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

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

// TestV1DeprecationBanner_RendersSunsetDateInSource — sanity check
// that the banner source literally contains the documented sunset
// date string. Catches accidental date drift if either dispatch text
// or banner copy change without the other updating.
func TestV1DeprecationBanner_RendersSunsetDateInSource(t *testing.T) {
	b, err := os.ReadFile("v1_deprecation_banner.templ")
	if err != nil {
		t.Fatalf("read banner source: %v", err)
	}
	src := string(b)
	if !strings.Contains(src, "August") || !strings.Contains(src, "2026") {
		t.Errorf("banner source should include 'August' and '2026' for the 2026-08-01 sunset; got %d bytes", len(src))
	}
}
