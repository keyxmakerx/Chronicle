// Contract tests for C-UPDATER-CLUSTER fixes:
//
//   - Bug #20: showAffectedCampaignsOnClick now does a two-stage
//     expand cascade (find foundry-module Versions trigger via
//     data-fvtt-versions-trigger → wait for htmx:afterSwap → click
//     the per-version Campaigns trigger). The old direct-DOM-lookup
//     surfaced a confusing error because the per-version trigger is
//     lazy-loaded by HTMX and doesn't exist on initial page render.
//
//   - Bug #22: regression test pinning the auto-pin banner's
//     version-display path. Operator reported the display is now
//     fixed; this test catches future regressions.
//
// SO #3 contract: every onclick IIFE must (a) start with
// `(function(`, (b) contain no literal `"` character, (c) not
// reference `__templ_`. Validated by onclick_handlers_test.go
// against every helper; the new IIFE is included in that sweep
// because it's still returned by showAffectedCampaignsOnClick.

package foundry_vtt

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestShowAffectedCampaignsOnClick_TwoStageExpand pins the new
// two-stage behavior: the IIFE must locate the foundry-module
// Versions trigger via the data-attribute selector and wire an
// htmx:afterSwap listener before clicking through. Without these,
// the click surfaces the same broken state bug #20 introduced.
func TestShowAffectedCampaignsOnClick_TwoStageExpand(t *testing.T) {
	got := showAffectedCampaignsOnClick("v0.1.10").Call

	// Fast-path: per-version trigger lookup by ID still present so
	// already-expanded callers skip the cascade.
	if !strings.Contains(got, "fvtt-campaigns-trigger-v0-1-10") {
		t.Errorf("IIFE missing per-version campaigns-trigger ID: %s", got)
	}
	// Stage 1: selector for the foundry-module Versions trigger.
	if !strings.Contains(got, "data-fvtt-versions-trigger") {
		t.Errorf("IIFE missing data-fvtt-versions-trigger selector — stage 1 lookup absent: %s", got)
	}
	// Stage 2: htmx:afterSwap listener wiring.
	if !strings.Contains(got, "htmx:afterSwap") {
		t.Errorf("IIFE missing htmx:afterSwap listener — stage 2 cascade absent: %s", got)
	}
	// Fallback timer for swap-target-missing.
	if !strings.Contains(got, "setTimeout") {
		t.Errorf("IIFE missing setTimeout fallback for stage 2: %s", got)
	}
	// scrollIntoView + click are still required (operator UX).
	if !strings.Contains(got, "scrollIntoView") {
		t.Errorf("IIFE missing scrollIntoView: %s", got)
	}
	if !strings.Contains(got, ".click()") {
		t.Errorf("IIFE missing programmatic click: %s", got)
	}
	// Pre-fix error string MUST be gone — the new flow can't reach
	// that state, and leaving the legacy text in the bundle is the
	// kind of stale UX that erodes operator trust.
	if strings.Contains(got, "expand the foundry-module package manually") {
		t.Errorf("IIFE still emits the pre-fix manual-fallback error message; remove it: %s", got)
	}
}

// TestShowAffectedCampaignsOnClick_NoDoubleQuotes — SO #3 contract
// reiteration scoped to the rewritten helper. The full SO #3 sweep
// runs in onclick_handlers_test.go; this is a fast-fail check for
// the specific helper this PR touches so a regression bisects to
// this test immediately.
func TestShowAffectedCampaignsOnClick_NoDoubleQuotes(t *testing.T) {
	got := showAffectedCampaignsOnClick("v0.1.10").Call
	if strings.Contains(got, `"`) {
		t.Errorf("IIFE contains literal `\"` character — would break onclick=\"...\" attribute. SO #3 contract regressed: %s", got)
	}
	if !strings.HasPrefix(got, "(function(") {
		t.Errorf("IIFE does not start with `(function(` — SO #3 contract regressed: %s", got)
	}
	if strings.Contains(got, "__templ_") {
		t.Errorf("IIFE references __templ_ — SO #3 contract regressed: %s", got)
	}
}

// TestAutoPinBanner_VersionDisplay_BugFFFRegression pins bug #22's
// fix in place: the banner's headline + body must display the
// previous + new versions verbatim. Pre-#22 the banner had a
// version-display issue (operator reported it's now fixed). This
// regression test catches any future updater work that silently
// drops the version strings.
func TestAutoPinBanner_VersionDisplay_BugFFFRegression(t *testing.T) {
	summary := AutoPinSummary{
		PreviousVersion: "v0.1.14",
		NewVersion:      "v0.1.15",
		Affected:        3,
		Timestamp:       1700000000,
	}
	component := AutoPinBanner(summary)

	var buf bytes.Buffer
	if err := component.Render(context.Background(), &buf); err != nil {
		t.Fatalf("AutoPinBanner render failed: %v", err)
	}
	html := buf.String()

	// Both version strings must appear in the rendered banner —
	// the new version in the "you installed" headline, the
	// previous version in the "affected pinned to" body.
	if !strings.Contains(html, "v0.1.15") {
		t.Errorf("banner missing new-version display (v0.1.15); operator reported #22 fix has regressed.\nHTML:\n%s", html)
	}
	if !strings.Contains(html, "v0.1.14") {
		t.Errorf("banner missing previous-version display (v0.1.14); operator reported #22 fix has regressed.\nHTML:\n%s", html)
	}
	// Affected count must also render — it's the "act on it" cue.
	if !strings.Contains(html, "3 campaigns") {
		t.Errorf("banner missing affected-count display (3 campaigns).\nHTML:\n%s", html)
	}
}

// TestAutoPinBanner_SingleCampaignPluralization mirrors the
// pluralization branch in autoPinBannerHeadline — Affected=1
// renders "1 campaign was" (singular). Regression-pins the
// branch so future updater UX work doesn't silently drop the
// singular form.
func TestAutoPinBanner_SingleCampaignPluralization(t *testing.T) {
	summary := AutoPinSummary{
		PreviousVersion: "v0.1.14",
		NewVersion:      "v0.1.15",
		Affected:        1,
	}
	component := AutoPinBanner(summary)

	var buf bytes.Buffer
	if err := component.Render(context.Background(), &buf); err != nil {
		t.Fatalf("AutoPinBanner render failed: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, "1 campaign was") {
		t.Errorf("banner missing singular pluralization for Affected=1.\nHTML:\n%s", html)
	}
}
