// view_prefs_test.go pins the <html> attributes that carry one person's own
// choices: which appear, that bad values never become attributes, that they
// sit beside (never inside) the campaign's data-cz-* set, and that a signed-out
// visitor gets none.

package layouts

import (
	"context"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func TestViewPrefAttrs(t *testing.T) {
	tests := []struct {
		name string
		p    *ViewPrefsData
		want map[string]any
	}{
		{"visitor gets nothing", nil, map[string]any{}},
		{"signed in, all defaults: only the theme marker", &ViewPrefsData{Theme: "device", Motion: "owner", TextSize: "standard", Contrast: "standard"}, map[string]any{"data-view-theme": "device"}},
		{"zero value reads as defaults", &ViewPrefsData{}, map[string]any{"data-view-theme": "device"}},
		{"everything chosen", &ViewPrefsData{Theme: "dark", Motion: "calm", TextSize: "largest", Contrast: "high"},
			map[string]any{"data-view-theme": "dark", "data-view-motion": "calm", "data-view-text": "largest", "data-view-contrast": "high"}},
		{"light and larger", &ViewPrefsData{Theme: "light", TextSize: "larger"}, map[string]any{"data-view-theme": "light", "data-view-text": "larger"}},
		{"bad values fall back, never reach an attribute", &ViewPrefsData{Theme: `"><script>`, Motion: "x", TextSize: "huge", Contrast: "max"}, map[string]any{"data-view-theme": "device"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.p != nil {
				ctx = SetViewPrefs(ctx, tc.p)
			}
			got := map[string]any(ViewPrefAttrs(ctx))
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAppearanceAttrs_CarriesViewPrefsBesideCampaignLook(t *testing.T) {
	ctx := SetViewPrefs(context.Background(), &ViewPrefsData{Theme: "dark", Motion: "calm"})
	ctx = SetAppearance(ctx, &AppearanceData{TypeScale: "roomy", ReduceMotion: false})
	got := AppearanceAttrs(ctx)
	if got["data-cz-scale"] != "roomy" || got["data-view-theme"] != "dark" || got["data-view-motion"] != "calm" {
		t.Fatalf("expected campaign and personal attributes side by side, got %v", got)
	}
	if _, set := got["data-cz-reduce"]; set {
		t.Error("a personal Calmer choice must not write the campaign's data-cz-reduce")
	}

	// And the personal attributes work on a page with no campaign at all.
	ctx = SetViewPrefs(context.Background(), &ViewPrefsData{Theme: "light", Contrast: "high"})
	got = AppearanceAttrs(ctx)
	if got["data-view-contrast"] != "high" || got["data-view-theme"] != "light" {
		t.Fatalf("non-campaign page lost the personal attributes: %v", got)
	}
}

// renderBase renders the real base layout with an empty body.
func renderBase(t *testing.T, ctx context.Context) string {
	t.Helper()
	var sb strings.Builder
	body := templ.ComponentFunc(func(context.Context, io.Writer) error { return nil })
	if err := Base("Test").Render(templ.WithChildren(ctx, body), &sb); err != nil {
		t.Fatalf("render Base: %v", err)
	}
	return sb.String()
}

// htmlTag returns the opening <html ...> tag of a rendered page.
func htmlTag(t *testing.T, page string) string {
	t.Helper()
	i := strings.Index(page, "<html")
	if i < 0 {
		t.Fatal("no <html> in rendered page")
	}
	return page[i : i+strings.Index(page[i:], ">")+1]
}

func TestBase_RendersViewPrefsOnHTML(t *testing.T) {
	tests := []struct {
		name    string
		ctx     context.Context
		want    []string
		notWant []string
	}{
		{"visitor: no personal attributes", context.Background(), nil, []string{"data-view-"}},
		{"signed in with all four chosen",
			SetViewPrefs(context.Background(), &ViewPrefsData{Theme: "dark", Motion: "calm", TextSize: "larger", Contrast: "high"}),
			[]string{`data-view-theme="dark"`, `data-view-motion="calm"`, `data-view-text="larger"`, `data-view-contrast="high"`}, nil},
		{"campaign page: personal and campaign attributes together",
			SetViewPrefs(SetAppearance(context.Background(), &AppearanceData{ReduceMotion: true}), &ViewPrefsData{Theme: "light"}),
			[]string{`data-view-theme="light"`, `data-cz-reduce="1"`}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tag := htmlTag(t, renderBase(t, tc.ctx))
			for _, w := range tc.want {
				if !strings.Contains(tag, w) {
					t.Errorf("<html> tag %q lacks %s", tag, w)
				}
			}
			for _, n := range tc.notWant {
				if strings.Contains(tag, n) {
					t.Errorf("<html> tag %q should not contain %s", tag, n)
				}
			}
		})
	}
}

func TestBase_FirstPaintScriptHonoursAccountTheme(t *testing.T) {
	page := renderBase(t, context.Background())
	// The account's choice must win over this browser's saved theme, "device"
	// must ignore it, and either reduce switch must set nav-rm.
	for _, frag := range []string{"data-view-theme", "v==='device'?null", "data-view-motion", "nav-rm"} {
		if !strings.Contains(page, frag) {
			t.Errorf("first-paint script lost %q", frag)
		}
	}
}
