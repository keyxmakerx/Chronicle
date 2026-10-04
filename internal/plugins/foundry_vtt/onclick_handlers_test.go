// Contract tests for the inline-IIFE onclick handlers: every onclick
// handler inside an HTMX-swapped fragment must be a self-contained
// IIFE, not a templ `script` helper reference. Checks per handler:
// the Call body starts with `(function(` (it's an IIFE); contains no
// literal `"` (templ writes Call into onclick="..." unescaped, so a
// quote would break the attribute); and contains no `__templ_`
// (a regression to the templ-script pattern that caused runtime
// ReferenceErrors).
package foundry_vtt

import (
	"strings"
	"testing"
)

// TestOnClick_HandlersAreInlineIIFE runs the three-check contract
// across every handler. Table-driven so adding new handlers is one
// line per case.
func TestOnClick_HandlersAreInlineIIFE(t *testing.T) {
	cases := []struct {
		name string
		call string
	}{
		{"notifyCampaign", notifyCampaignOnClick("camp-1", "v0.1.10").Call},
		{"forcePinCampaign", forcePinCampaignOnClick("camp-1", "v0.1.10").Call},
		{"notifyOlder", notifyOlderOnClick("v0.1.10").Call},
		{"forcePinOlder", forcePinOlderOnClick("v0.1.10").Call},
		{"openDialog", openDialogOnClick("fvtt-update-dlg-row").Call},
		{"closeDialog", closeDialogOnClick().Call},
		{"rotateToken", rotateTokenOnClick("camp-1").Call},
		{"dismissAutoPinBanner", dismissAutoPinBannerOnClick().Call},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.HasPrefix(tc.call, "(function(") {
				t.Errorf("Call body should start with `(function(` (IIFE pattern), got prefix: %q", first(tc.call, 30))
			}
			if strings.Contains(tc.call, `"`) {
				t.Errorf("Call body must not contain literal \" character "+
					"(would close the onclick=\"...\" attribute prematurely "+
					"since templ writes Call without HTML-escaping). Found:\n%s",
					tc.call)
			}
			if strings.Contains(tc.call, "__templ_") {
				t.Errorf("Call body contains `__templ_` — regression to the "+
					"templ-script pattern that caused the production "+
					"ReferenceError bugs. Body:\n%s", tc.call)
			}
		})
	}
}

// TestOnClick_NoEmptyScriptFunction confirms the ComponentScript's
// Function field is empty so no <script> tag gets emitted (templ's
// RenderScriptItems is a no-op when Function is empty). A non-empty
// value would reintroduce the script-tag race with hx-swap.
func TestOnClick_NoEmptyScriptFunction(t *testing.T) {
	cases := []struct {
		name   string
		script struct{ Function string }
	}{
		{"notifyCampaign", struct{ Function string }{notifyCampaignOnClick("c", "v").Function}},
		{"forcePinCampaign", struct{ Function string }{forcePinCampaignOnClick("c", "v").Function}},
		{"notifyOlder", struct{ Function string }{notifyOlderOnClick("v").Function}},
		{"forcePinOlder", struct{ Function string }{forcePinOlderOnClick("v").Function}},
		{"openDialog", struct{ Function string }{openDialogOnClick("d").Function}},
		{"closeDialog", struct{ Function string }{closeDialogOnClick().Function}},
		{"rotateToken", struct{ Function string }{rotateTokenOnClick("c").Function}},
		{"dismissAutoPinBanner", struct{ Function string }{dismissAutoPinBannerOnClick().Function}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.script.Function != "" {
				t.Errorf("Function field must be empty (no <script> tag emission); "+
					"any non-empty value re-introduces the templ-script race window. Got: %q",
					first(tc.script.Function, 50))
			}
		})
	}
}

// TestOnClick_JSEscapesInterpolatedValues pins the jsStr helper's
// contract: a campaign ID or version containing characters that could
// close the JS string (apostrophe, backslash) must be escaped, not
// embedded raw.
func TestOnClick_JSEscapesInterpolatedValues(t *testing.T) {
	// Campaign ID with embedded apostrophe — would close the
	// surrounding '' if not escaped.
	got := notifyCampaignOnClick("camp'evil", "v0.1.10").Call
	if strings.Contains(got, "camp'evil") {
		t.Error("interpolated campaign ID with apostrophe should be JS-escaped, " +
			"not embedded raw (would break out of the surrounding JS string literal)")
	}
	// Don't pin the exact escape form (text/template's JSEscapeString
	// could change between releases); just confirm the apostrophe is
	// preceded by an escaping backslash rather than left raw.
	if !strings.Contains(got, `\'evil`) {
		t.Errorf("apostrophe should appear with a preceding escape; got:\n%s", got)
	}
}

// first returns the first n characters of s (or all of s if shorter).
// Used to keep test error output readable when Call bodies are long.
func first(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
