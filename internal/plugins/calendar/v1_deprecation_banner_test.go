// v1_deprecation_banner_test.go covers the V1 deprecation banner.
//
// REINSTATED 2026-07-17 (dispatch C-CAL-V1-SUNSET). The banner was pulled
// dormant on 2026-05-29 (C-CAL-V1-BANNER-PULL) while V2 was unreachable from
// chrome; the V1→V2 cutover has since landed, so the banner is mounted again on
// the two still-live V1 pages (setup chooser + per-calendar timeline). These
// tests assert (a) the banner IS mounted on exactly those two pages and nowhere
// else in calendar.templ, (b) it is NOT on the V2 shell or the embed grid, and
// (c) the component + its Switch-to-V2 href helper resolve correctly.

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

// --- Reinstatement assertions -----------------------------------------

// liveTemplBlocks reads a .templ file, strips `//`-comment lines (so pointer
// comments that mention the component name don't false-positive a substring
// scan), and returns a map of templ-component name → its live body text.
func liveTemplBlocks(t *testing.T, file string) map[string]string {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	blocks := map[string]string{}
	var curName string
	var cur strings.Builder
	flush := func() {
		if curName != "" {
			blocks[curName] = cur.String()
		}
		cur.Reset()
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue // skip comment lines (incl. any pointer comments)
		}
		if strings.HasPrefix(line, "templ ") {
			flush()
			name := strings.TrimPrefix(line, "templ ")
			if i := strings.IndexByte(name, '('); i >= 0 {
				name = name[:i]
			}
			curName = strings.TrimSpace(name)
		}
		cur.WriteString(line)
		cur.WriteByte('\n')
	}
	flush()
	return blocks
}

// TestV1DeprecationBanner_MountedOnLiveV1Pages inverts the former "banner
// pulled" guard (C-CAL-V1-BANNER-PULL). After C-CAL-V1-SUNSET the banner was
// reinstated on exactly the two still-live V1 pages — the setup chooser and the
// per-calendar timeline — and must appear nowhere else in calendar.templ.
//
// PIN REFRESHED, NOT DELETED, by C-CALV4-WIZARD-P13 [WZ-13] SIGNED. THE LIST IS
// NOW ONE PAGE LONG: the three-card setup chooser was retired and its
// CalendarSetupPage deleted, so W-H did not only ship a wizard — it retired a
// V1 surface, which is why this list shrank rather than this test being
// deleted. The old assertion t.Fatalf'd the moment CalendarSetupPage
// disappeared, exactly as its dispatch predicted; that Fatal was the pin doing
// its job and this is the refresh it demanded.
//
// If the timeline is ever retired too, this test does not become vacuous: the
// second half below still forbids the banner appearing anywhere else in the
// file, so a banner mounted on a NEW page would still fail.
func TestV1DeprecationBanner_MountedOnLiveV1Pages(t *testing.T) {
	blocks := liveTemplBlocks(t, "calendar.templ")
	const mount = "@V1DeprecationBanner("

	if _, gone := blocks["CalendarSetupPage"]; gone {
		t.Error("CalendarSetupPage is back in calendar.templ — [WZ-13] retired the " +
			"three-card chooser, and a second create door is the incoherence L6 removes")
	}

	for _, page := range []string{"TimelinePage"} {
		body, ok := blocks[page]
		if !ok {
			t.Fatalf("calendar.templ no longer defines templ %s", page)
		}
		if !strings.Contains(body, mount) {
			t.Errorf("templ %s must mount %s after the V1→V2 cutover (C-CAL-V1-SUNSET)", page, mount)
		}
	}

	// No OTHER component in calendar.templ may mount it (the dead pages are gone).
	for name, body := range blocks {
		if name == "TimelinePage" {
			continue
		}
		if strings.Contains(body, mount) {
			t.Errorf("templ %s unexpectedly mounts %s — only the setup + timeline pages should", name, mount)
		}
	}
}

// TestV1DeprecationBanner_NotOnV2OrEmbed pins that the banner stays off the V2
// shell and the dashboard/entity embed grid — those surfaces self-promote (V2)
// or are chrome-free previews (embed), so the V1 cutover notice must not leak in.
func TestV1DeprecationBanner_NotOnV2OrEmbed(t *testing.T) {
	for _, file := range []string{"calendar_v2.templ", "blocks.templ"} {
		for name, body := range liveTemplBlocks(t, file) {
			if strings.Contains(body, "@V1DeprecationBanner(") {
				t.Errorf("%s templ %s must NOT mount the V1 deprecation banner", file, name)
			}
		}
	}
}

// TestV1DeprecationBanner_ComponentDefined pins that the banner component
// itself is defined. It stays live for as long as any V1 page it mounts on
// (setup chooser, timeline) survives; the two remaining pages sunset in a later
// slice, at which point the component and this test retire together.
func TestV1DeprecationBanner_ComponentDefined(t *testing.T) {
	b, err := os.ReadFile("v1_deprecation_banner.templ")
	if err != nil {
		t.Fatalf("read banner source: %v", err)
	}
	if !strings.Contains(string(b), "templ V1DeprecationBanner(") {
		t.Error("v1_deprecation_banner.templ must keep the V1DeprecationBanner " +
			"component while the setup + timeline pages still mount it")
	}
}
