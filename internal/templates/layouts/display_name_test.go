package layouts

// display_name_test.go — C-CUSTOMIZE-RESCUE B1 pin. The owner-set custom brand
// name must override the campaign name in the TOPBAR, not only the sidebar.
// Before this fix the sidebar honored the brand name (via #464) but the two
// topbar sites read GetCampaignName directly, so a custom brand appeared in the
// sidebar and silently reverted to the campaign name in the header — the
// operator-reported "header customization is broken" (audit §8.1). These tests
// pin GetDisplayName + the two topbar render sites so the two chrome surfaces
// cannot drift apart again.

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestGetDisplayName(t *testing.T) {
	base := SetCampaignName(context.Background(), "Rime of the Frostmaiden")

	if got := GetDisplayName(base); got != "Rime of the Frostmaiden" {
		t.Fatalf("no brand set: GetDisplayName = %q, want the campaign name", got)
	}
	if got := GetDisplayName(SetBrandName(base, "The Frozen North")); got != "The Frozen North" {
		t.Fatalf("brand set: GetDisplayName = %q, want the brand name", got)
	}
	if got := GetDisplayName(SetBrandName(base, "")); got != "Rime of the Frostmaiden" {
		t.Fatalf("empty brand: GetDisplayName = %q, want the campaign name fallback", got)
	}
}

// ctxTopbarName builds a context for the in-campaign topbar name render sites.
func ctxTopbarName(authed bool, campaignName, brandName string) context.Context {
	ctx := context.Background()
	ctx = SetIsAuthenticated(ctx, authed)
	ctx = SetCampaignID(ctx, "camp-1") // makes InCampaign(ctx) true
	ctx = SetCampaignName(ctx, campaignName)
	if brandName != "" {
		ctx = SetBrandName(ctx, brandName)
	}
	return ctx
}

func TestTopbar_BrandNameOverridesCampaignName(t *testing.T) {
	const campaign = "Rime of the Frostmaiden"
	const brand = "The Frozen North"

	// Both the authenticated (name-picker trigger) and anonymous/public
	// (plain span) in-campaign topbar paths must show the brand when set.
	for _, authed := range []bool{true, false} {
		t.Run(map[bool]string{true: "authenticated", false: "anonymous"}[authed], func(t *testing.T) {
			var buf bytes.Buffer
			if err := Topbar().Render(ctxTopbarName(authed, campaign, brand), &buf); err != nil {
				t.Fatalf("render Topbar: %v", err)
			}
			html := buf.String()
			if !strings.Contains(html, brand) {
				t.Errorf("topbar must show the custom brand name %q; got:\n%s", brand, html)
			}
			if strings.Contains(html, campaign) {
				t.Errorf("topbar must NOT show the campaign name %q when a brand name is set (that is the §8.1 bug)", campaign)
			}
		})
	}
}

func TestTopbar_FallsBackToCampaignNameWithoutBrand(t *testing.T) {
	const campaign = "Rime of the Frostmaiden"
	for _, authed := range []bool{true, false} {
		var buf bytes.Buffer
		if err := Topbar().Render(ctxTopbarName(authed, campaign, ""), &buf); err != nil {
			t.Fatalf("render Topbar: %v", err)
		}
		if !strings.Contains(buf.String(), campaign) {
			t.Errorf("authed=%v: topbar must fall back to the campaign name %q when no brand is set", authed, campaign)
		}
	}
}
