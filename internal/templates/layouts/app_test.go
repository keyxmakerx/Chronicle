package layouts

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// ctxWithTopbarStyle builds a context carrying the given topbar style, mirroring
// what the LayoutInjector does at request time.
func ctxWithTopbarStyle(s *TopbarStyleData) context.Context {
	return SetTopbarStyle(context.Background(), s)
}

// ctxForNotes builds a context for the notesWidgetVisible / notes-button render
// tests: an authenticated user in campaign campaignID, at request path
// activePath, with the "notes" addon enabled or not.
func ctxForNotes(authed bool, campaignID string, notesEnabled bool, activePath string) context.Context {
	ctx := context.Background()
	ctx = SetIsAuthenticated(ctx, authed)
	ctx = SetCampaignID(ctx, campaignID)
	ctx = SetEnabledAddons(ctx, map[string]bool{"notes": notesEnabled})
	ctx = SetActivePath(ctx, activePath)
	return ctx
}

// TestTopbarInlineStyle pins the contract that the helper emits the correct
// background CSS for each mode — in particular that gradient mode produces a
// linear-gradient and image mode produces a background-image url(), the two
// modes that previously rendered nothing on the topbar.
func TestTopbarInlineStyle(t *testing.T) {
	tests := []struct {
		name      string
		style     *TopbarStyleData
		want      string // required substring; ignored when wantEmpty is true
		wantEmpty bool
	}{
		{name: "nil style falls back to default", style: nil, wantEmpty: true},
		{name: "empty mode falls back to default", style: &TopbarStyleData{}, wantEmpty: true},
		{
			name:  "solid emits background-color",
			style: &TopbarStyleData{Mode: "solid", Color: "#6366f1"},
			want:  "background-color: #6366f1;",
		},
		{
			name:  "gradient emits linear-gradient with mapped direction",
			style: &TopbarStyleData{Mode: "gradient", GradientFrom: "#6366f1", GradientTo: "#ec4899", GradientDir: "to-br"},
			want:  "background: linear-gradient(to bottom right, #6366f1, #ec4899);",
		},
		{
			name:  "gradient defaults direction to right",
			style: &TopbarStyleData{Mode: "gradient", GradientFrom: "#111111", GradientTo: "#222222"},
			want:  "linear-gradient(to right, #111111, #222222)",
		},
		{
			name:  "image emits background-image url",
			style: &TopbarStyleData{Mode: "image", ImagePath: "bg.png"},
			want:  "background-image: url('/media/bg.png');",
		},
		{
			name:      "gradient missing a color falls back to default",
			style:     &TopbarStyleData{Mode: "gradient", GradientFrom: "#111111"},
			wantEmpty: true,
		},
		{
			name:      "image missing path falls back to default",
			style:     &TopbarStyleData{Mode: "image"},
			wantEmpty: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := topbarInlineStyle(ctxWithTopbarStyle(tt.style))
			if tt.wantEmpty {
				if got != "" {
					t.Fatalf("expected empty style, got %q", got)
				}
				return
			}
			if !strings.Contains(got, tt.want) {
				t.Fatalf("style %q does not contain %q", got, tt.want)
			}
		})
	}
}

// TestTopbarStyleIsImage verifies the scrim-gating predicate.
func TestTopbarStyleIsImage(t *testing.T) {
	if !topbarStyleIsImage(ctxWithTopbarStyle(&TopbarStyleData{Mode: "image", ImagePath: "x.png"})) {
		t.Fatal("image mode with a path should be reported as image")
	}
	if topbarStyleIsImage(ctxWithTopbarStyle(&TopbarStyleData{Mode: "image"})) {
		t.Fatal("image mode without a path should not be reported as image")
	}
	if topbarStyleIsImage(ctxWithTopbarStyle(&TopbarStyleData{Mode: "gradient", GradientFrom: "#111", GradientTo: "#222"})) {
		t.Fatal("gradient mode should not be reported as image")
	}
	if topbarStyleIsImage(ctxWithTopbarStyle(nil)) {
		t.Fatal("nil style should not be reported as image")
	}
}

// TestTopbarHasCustomStyle verifies the layer-gating predicate tracks
// topbarInlineStyle exactly.
func TestTopbarHasCustomStyle(t *testing.T) {
	if topbarHasCustomStyle(ctxWithTopbarStyle(nil)) {
		t.Fatal("nil style should report no custom style")
	}
	if topbarHasCustomStyle(ctxWithTopbarStyle(&TopbarStyleData{})) {
		t.Fatal("empty mode should report no custom style")
	}
	if !topbarHasCustomStyle(ctxWithTopbarStyle(&TopbarStyleData{Mode: "solid", Color: "#ffffff"})) {
		t.Fatal("solid with a color should report a custom style")
	}
}

// TestTopbarHeaderIsolate is the stacking-context pinning test for cordinator#29.
// The <header> must carry "isolate" (CSS isolation:isolate) so it forms its own
// stacking context. Without it, z-index:-1 brand layers escape to the nearest
// ancestor stacking context and paint before the header's own surface background,
// making any custom topbar color or image invisible to the user.
func TestTopbarHeaderIsolate(t *testing.T) {
	ctx := ctxWithTopbarStyle(&TopbarStyleData{Mode: "solid", Color: "#6366f1"})
	var buf bytes.Buffer
	if err := Topbar().Render(ctx, &buf); err != nil {
		t.Fatalf("render Topbar: %v", err)
	}
	html := buf.String()
	headerIdx := strings.Index(html, "<header ")
	if headerIdx == -1 {
		t.Fatal("<header> element not found in rendered output")
	}
	openTag := html[headerIdx:]
	closeIdx := strings.Index(openTag, ">")
	if closeIdx == -1 {
		t.Fatal("opening <header> tag has no closing '>'")
	}
	openTag = openTag[:closeIdx+1]
	classStart := strings.Index(openTag, ` class="`)
	if classStart == -1 {
		t.Fatal("no class attribute on <header>")
	}
	classVal := openTag[classStart+8:]
	classVal = classVal[:strings.Index(classVal, `"`)]
	for _, c := range strings.Fields(classVal) {
		if c == "isolate" {
			return
		}
	}
	t.Fatalf("<header> classes %q must include \"isolate\" — without it z-index:-1 brand layers escape the stacking context and paint behind the header surface", classVal)
}

// TestNotesWidgetVisible pins the notesWidgetVisible predicate that gates both
// the floating notes panel's mount point and its topbar trigger button (single
// source of truth — see C-NAV-ACTIVE-FIX). Before this fix the topbar button
// used its own, looser condition that omitted the journal exclusion, so it
// rendered a dead button on the journal page (Chronicle.toggleNotes is only
// ever defined once the floating widget's init() runs, which the journal
// exclusion prevents).
func TestNotesWidgetVisible(t *testing.T) {
	tests := []struct {
		name         string
		authed       bool
		campaignID   string
		notesEnabled bool
		activePath   string
		want         bool
	}{
		{
			name: "authed, in campaign, addon on, dashboard page", authed: true,
			campaignID: "camp1", notesEnabled: true, activePath: "/campaigns/camp1/dashboard",
			want: true,
		},
		{
			name: "authed, in campaign, addon on, journal page itself", authed: true,
			campaignID: "camp1", notesEnabled: true, activePath: "/campaigns/camp1/journal",
			want: false,
		},
		{
			name: "authed, in campaign, addon on, journal sub-path", authed: true,
			campaignID: "camp1", notesEnabled: true, activePath: "/campaigns/camp1/journal/entry/5",
			want: false,
		},
		{
			name: "addon disabled", authed: true,
			campaignID: "camp1", notesEnabled: false, activePath: "/campaigns/camp1/dashboard",
			want: false,
		},
		{
			name: "not authenticated", authed: false,
			campaignID: "camp1", notesEnabled: true, activePath: "/campaigns/camp1/dashboard",
			want: false,
		},
		{
			name: "not in a campaign", authed: true,
			campaignID: "", notesEnabled: true, activePath: "/campaigns",
			want: false,
		},
		{
			// A different campaign's journal path must not falsely exclude this one.
			name: "in campaign, active path is ANOTHER campaign's journal", authed: true,
			campaignID: "camp1", notesEnabled: true, activePath: "/campaigns/camp2/journal",
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := notesWidgetVisible(ctxForNotes(tt.authed, tt.campaignID, tt.notesEnabled, tt.activePath))
			if got != tt.want {
				t.Fatalf("notesWidgetVisible() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestNotesButtonRenderGate is the render-level regression pin for the same
// bug: the topbar's #topbar-notes-trigger button must be present exactly when
// notesWidgetVisible is true, and absent on the journal page even when the
// notes addon is enabled and the user is authenticated. A render-level test
// (not just a predicate-function test) guards against the template's two call
// sites drifting apart again in a future edit.
func TestNotesButtonRenderGate(t *testing.T) {
	tests := []struct {
		name       string
		activePath string
		wantButton bool
	}{
		{name: "present on a non-journal page", activePath: "/campaigns/camp1/dashboard", wantButton: true},
		{name: "absent on the journal page", activePath: "/campaigns/camp1/journal", wantButton: false},
		{name: "absent on a journal sub-path", activePath: "/campaigns/camp1/journal/entry/5", wantButton: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := ctxForNotes(true, "camp1", true, tt.activePath)
			var buf bytes.Buffer
			if err := Topbar().Render(ctx, &buf); err != nil {
				t.Fatalf("render Topbar: %v", err)
			}
			html := buf.String()
			hasButton := strings.Contains(html, `id="topbar-notes-trigger"`)
			if hasButton != tt.wantButton {
				t.Fatalf("topbar-notes-trigger present=%v, want %v (path %q)", hasButton, tt.wantButton, tt.activePath)
			}
		})
	}
}

// TestSidebarEmitsNavClassVocabulary pins that #sidebar exposes
// data-nav-active-classes / data-nav-inactive-classes carrying the SAME class
// vocabulary the server renders on every nav link. boot.js's boosted-nav
// highlighter reads these as its single source of truth; nothing else pinned
// them, so dropping the attribute pair would silently fall production back to
// boot.js's hardcoded FALLBACK_* literals (the exact drift C-NAV-ACTIVE-FIX
// removed). r2-2.
func TestSidebarEmitsNavClassVocabulary(t *testing.T) {
	// The attributes render at the top of Sidebar(), before any context getter;
	// a background context is enough (the getters return safe zero values).
	var buf bytes.Buffer
	if err := Sidebar().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render Sidebar: %v", err)
	}
	html := buf.String()

	for _, want := range []string{
		`data-nav-active-classes="` + sidebarNavActive + `"`,
		`data-nav-inactive-classes="` + sidebarNavInactive + `"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("#sidebar must emit %q — boot.js reads it as the nav vocabulary source of truth;"+
				" without it the boosted-nav highlighter falls back to stale hardcoded literals", want)
		}
	}
}

// TestAllPagesLinkHighlightsOnEntitySubpath pins r2-3: the Entities/"All Pages"
// link uses longest-prefix (isPathPrefix), so an entity detail page highlights
// it on a hard server load, matching boot.js. It must stay inactive on an
// unrelated page — the prefix must not over-highlight.
func TestAllPagesLinkHighlightsOnEntitySubpath(t *testing.T) {
	render := func(activePath string) string {
		ctx := SetCampaignID(context.Background(), "camp1")
		ctx = SetActivePath(ctx, activePath)
		var buf bytes.Buffer
		if err := sidebarAllPagesLink(ctx).Render(ctx, &buf); err != nil {
			t.Fatalf("render All Pages link: %v", err)
		}
		return buf.String()
	}
	if got := render("/campaigns/camp1/entities/42"); !strings.Contains(got, sidebarNavActive) {
		t.Errorf("All Pages link must be active on an entity detail sub-path (r2-3); got %q", got)
	}
	if got := render("/campaigns/camp1/members"); strings.Contains(got, sidebarNavActive) {
		t.Errorf("All Pages link must be inactive off the entities tree; got %q", got)
	}
}
