// eras_tint_preview_test.go — THE ERA EDITOR MUST PREVIEW THE COLOUR THE GRID
// ACTUALLY PAINTS, AND IT MUST KEEP DOING SO.
//
// THE DEFECT THESE TESTS CLOSE (C-CALV4-TILES §5). The eras editor has always
// bound an `<input type="color">` straight to `era.color` and shown that colour
// back. The day cell paints something else: `oklch(from <colour> L C h)` keeps
// the HUE and pins lightness and chroma (static/css/calendar-block.css §ERAS).
// So a saturated pick and a pale pick of the same hue paint identically —
// #1d4ed8 and #c7d2fe come out one channel apart, measured in Chromium 141 —
// and a grey, having no hue worth taking but not being hueless either, paints
// whatever trace survives the conversion: #808080 lands on h 23.6°, a faint
// pink at rgb(255,249,249). The pinning is correct and
// stays; it is what guarantees no era can make the grid shout. What was wrong
// is that the editor did not say so, and an author could only find out by
// saving and looking at the calendar.
//
// THE ONE THING THAT CAN DRIFT is that the formula now exists in two files: the
// stylesheet that paints and the template that previews. CSS cannot hand a
// constant to a template and the settings page does not load the Block's sheet
// at all, so the numbers are mirrored — and mirrored numbers go stale in
// silence, which is exactly the failure this preview was built to remove. The
// first test reads them back out of the stylesheet, so a change on either side
// reds CI rather than shipping an editor that advertises a tint the grid
// stopped painting.
//
// PIN DISCIPLINE (inherited from builder_css_contract_test.go): comments are
// stripped before any assertion, so prose ABOUT the old `color-mix` formula
// cannot satisfy a test looking for the live one, and no bare strings.Index
// result is used as a slice bound — that panics on a rename instead of failing
// with something a reader can act on.
package calendar

import (
	"bytes"
	"context"
	"html"
	"regexp"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

var eraTintCommentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)

// eraTintDeclRe matches a live `--eratint:` declaration and captures the `L C`
// pair the relative-colour expression pins. Whitespace is loose because the
// sheet is hand-written and the tile slice is rewriting the rule around it; the
// components are not, because their ORDER is the contract.
var eraTintDeclRe = regexp.MustCompile(`--eratint:\s*oklch\(\s*from\s+var\(--erahue\)\s+([0-9.]+\s+[0-9.]+)\s+h\s*\)`)

// eraTintCSS returns calendar-block.css with comment bodies removed.
func eraTintCSS(t *testing.T) string {
	t.Helper()
	return eraTintCommentRe.ReplaceAllString(readRepoFile(t, "static/css/calendar-block.css"), " ")
}

// eraTintDeclarations returns the sheet's two `--eratint:` pairs, keyed by the
// theme whose rule carries them. The theme is decided by the rule's own
// prelude — the text back to the previous `}` — rather than by source order or
// by which lightness is smaller, because both of those would silently swap if
// the tile slice reorders the sheet, and a swapped pin is worse than none.
func eraTintDeclarations(t *testing.T) map[string]string {
	t.Helper()
	css := eraTintCSS(t)
	out := map[string]string{}
	for _, m := range eraTintDeclRe.FindAllStringSubmatchIndex(css, -1) {
		prelude := css[:m[0]]
		if cut := strings.LastIndex(prelude, "}"); cut >= 0 {
			prelude = prelude[cut+1:]
		}
		theme := "light"
		if strings.Contains(prelude, ".dark") {
			theme = "dark"
		}
		pair := css[m[2]:m[3]]
		if prev, seen := out[theme]; seen && prev != pair {
			t.Fatalf("calendar-block.css declares two different %s --eratint pairs (%q and %q) — "+
				"the editor's preview can only mirror one", theme, prev, pair)
		}
		out[theme] = pair
	}
	return out
}

// eraTokenValue reads one custom property out of the token block introduced by
// selector, e.g. `--surface-card` from `.cal-block-host {`.
func eraTokenValue(t *testing.T, css, selector, token string) string {
	t.Helper()
	at := strings.Index(css, selector)
	if at < 0 {
		t.Fatalf("calendar-block.css no longer contains the %q token block — the era preview's "+
			"ground and untinted-day colours are copied from it", selector)
	}
	re := regexp.MustCompile(regexp.QuoteMeta(token) + `:\s*([^;]+);`)
	m := re.FindStringSubmatch(css[at:])
	if m == nil {
		t.Fatalf("%q declares no %s", selector, token)
	}
	return strings.TrimSpace(m[1])
}

// TestErasEditor_TintPreviewMatchesTheStylesheet is the drift guard. It is the
// reason the mirrored constants are allowed to exist.
func TestErasEditor_TintPreviewMatchesTheStylesheet(t *testing.T) {
	decls := eraTintDeclarations(t)
	if len(decls) != 2 {
		t.Fatalf("expected a light and a dark --eratint declaration in calendar-block.css, got %d: %v",
			len(decls), decls)
	}
	for _, tc := range []struct {
		theme, mirrored string
	}{
		{"light", eraTintLightLC},
		{"dark", eraTintDarkLC},
	} {
		if decls[tc.theme] != tc.mirrored {
			t.Errorf("the %s day cell is painted with oklch(from … %s h) but the eras editor "+
				"previews %s h — update the mirrored constant in calendar_settings.templ, or the "+
				"editor advertises a tint the calendar does not paint",
				tc.theme, decls[tc.theme], tc.mirrored)
		}
	}

	// The ground and the untinted day come from the same sheet and drift the
	// same way: a preview swatch sitting on the wrong ground misstates how much
	// the tint separates, which IS the question the author is asking it.
	css := eraTintCSS(t)
	for _, tc := range []struct {
		what, selector, token, mirrored string
	}{
		{"light ground", ".cal-block-host {", "--surface-inset", eraPreviewLightGround},
		{"light untinted day", ".cal-block-host {", "--surface-card", eraPreviewLightPlain},
		{"dark ground", ".dark .cal-block-host {", "--surface-card", eraPreviewDarkGround},
		{"dark untinted day", ".dark .cal-block-host {", "--surface-inset", eraPreviewDarkPlain},
	} {
		if got := eraTokenValue(t, css, tc.selector, tc.token); got != tc.mirrored {
			t.Errorf("%s: %s is %s in calendar-block.css but the eras editor previews on %s",
				tc.what, tc.token, got, tc.mirrored)
		}
	}
}

// erasTabHTML renders the settings page for a fantasy calendar (the eras tab
// does not exist in real-life mode) and returns it with entities decoded, so an
// assertion can name the JavaScript expression the author will actually run
// rather than its escaped spelling.
func erasTabHTML(t *testing.T) string {
	t.Helper()
	cc := &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Imix"}, MemberRole: campaigns.RoleOwner,
	}
	end := 1200
	cal := &Calendar{
		ID: "cal-1", CampaignID: "camp-1", Name: "Harptos", Mode: ModeFantasy,
		Eras: []Era{
			{Name: "First Age", StartYear: 1, EndYear: &end, Color: "#6366f1"},
			{Name: "Age of Fire", StartYear: 1201, Color: "#808080"},
		},
	}
	var buf bytes.Buffer
	if err := CalendarSettingsPage(cc, cal, "tok").Render(context.Background(), &buf); err != nil {
		t.Fatalf("render settings page: %v", err)
	}
	return html.UnescapeString(buf.String())
}

// TestErasEditor_PreviewsThePaintedTintInBothThemes pins the surface itself:
// the two swatches exist, they run the relative-colour formula rather than the
// raw pick, and they do it for both themes. A preview that quietly fell back to
// `background: e.color` would look right in review and teach the author the
// exact thing that is false.
func TestErasEditor_PreviewsThePaintedTintInBothThemes(t *testing.T) {
	page := erasTabHTML(t)

	for _, want := range []string{
		"eraTint(e.color, 'light')",
		"eraTint(e.color, 'dark')",
		"eraPlain('light')",
		"eraPlain('dark')",
		"eraGround('light')",
		"eraGround('dark')",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the eras tab does not bind %s — the author cannot see what the grid paints", want)
		}
	}

	// The formula, not a re-implementation of it. `oklch(from …)` is what makes
	// the preview and the product the same arithmetic; hand-rolled hue maths in
	// JS would be a second implementation to keep in step.
	if !strings.Contains(page, "'oklch(from ' + (c || '')") {
		t.Error("the preview no longer composes oklch(from <colour> L C h) — if the hue maths moved " +
			"into JavaScript, the preview and the stylesheet are now two implementations of one formula")
	}
	for _, lc := range []string{eraTintLightLC, eraTintDarkLC} {
		if !strings.Contains(page, lc) {
			t.Errorf("the rendered eras tab does not carry the pinned pair %q", lc)
		}
	}

	// One line of copy, and it must name both facts: the hue is all that is
	// taken, and a grey has no hue to give. Either half alone leaves the author
	// with a preview they cannot explain.
	lower := strings.ToLower(page)
	for _, want := range []string{"hue", "grey"} {
		if !strings.Contains(lower, want) {
			t.Errorf("the eras tab's copy never mentions %q — the preview shows the behaviour but "+
				"nothing states it", want)
		}
	}
}

// TestErasEditor_StoresTheRawPickNotThePaintedTint guards the column, not the
// look. calendar_eras.color is VARCHAR(20) (migrations/001_calendar_tables.up.sql)
// and `oklch(0.55 0.12 200)` is EXACTLY 20 characters, so the width has no
// headroom at all: a save that ever posted a computed colour string instead of
// the picker's seven-character hex would sit on the limit at best, and MariaDB
// truncates rather than refuses outside strict mode. The preview is a display
// concern and must stay one.
func TestErasEditor_StoresTheRawPickNotThePaintedTint(t *testing.T) {
	page := erasTabHTML(t)
	at := strings.Index(page, "const payload = eras.map(")
	if at < 0 {
		t.Fatal("the eras save handler no longer builds `payload` from eras.map — this test can no " +
			"longer see what is stored")
	}
	end := strings.Index(page[at:], "Chronicle.apiFetch")
	if end < 0 {
		t.Fatal("the eras save handler no longer calls Chronicle.apiFetch after building its payload")
	}
	payload := page[at : at+end]
	if !strings.Contains(payload, "color: e.color") {
		t.Error("the eras save no longer posts the picker's own value — anything computed is longer " +
			"than the 20-char color column and would be stored truncated")
	}
	if strings.Contains(payload, "oklch") {
		t.Error("the eras save posts an oklch string: `oklch(0.55 0.12 200)` is exactly the column's " +
			"20 characters and `oklch(from …)` is far past it")
	}
}
