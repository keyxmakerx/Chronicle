// Contract tests for the inline-IIFE onclick handlers.
//
// These tests pin the C-FMC-8 architectural rule: every onclick
// handler inside an HTMX-swapped fragment is a self-contained IIFE,
// NOT a templ `script` helper reference. Three regression checks
// per handler:
//
//  1. The Call body starts with `(function(`. Confirms it's an IIFE,
//     not a function reference.
//  2. The Call body contains NO double-quote character ("). Templ
//     writes Call directly into onclick="..." without HTML-escaping,
//     so any literal " would prematurely close the attribute.
//  3. The Call body contains NO `__templ_` substring. That prefix
//     would indicate a regression to the templ-script pattern that
//     caused the production "ReferenceError" bugs.
//
// If any of these fail, the onclick attribute breaks at runtime —
// either with an HTML-parse error (broken attribute) or a JS
// ReferenceError (re-introduced templ-script reference).
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
		{"pinSave", pinSaveOnClick("camp-1").Call},
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
// Function field is empty so no <script> tag gets emitted. Templ's
// RenderScriptItems is a no-op when Function is empty; if a future
// edit fills Function in, the script-tag race window we explicitly
// chose to avoid would come back.
func TestOnClick_NoEmptyScriptFunction(t *testing.T) {
	cases := []struct {
		name   string
		script struct{ Function string }
	}{
		{"notifyCampaign", struct{ Function string }{notifyCampaignOnClick("c", "v").Function}},
		{"forcePinCampaign", struct{ Function string }{forcePinCampaignOnClick("c", "v").Function}},
		{"notifyOlder", struct{ Function string }{notifyOlderOnClick("v").Function}},
		{"forcePinOlder", struct{ Function string }{forcePinOlderOnClick("v").Function}},
		{"pinSave", struct{ Function string }{pinSaveOnClick("c").Function}},
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

// TestOnClick_JSEscapesInterpolatedValues — defensive: if a campaign
// ID or version contains characters that could close the JS string
// (apostrophe, backslash), the handler must emit \uXXXX escapes
// rather than letting the literal break out. Specifically pins the
// jsStr helper's contract.
func TestOnClick_JSEscapesInterpolatedValues(t *testing.T) {
	// Campaign ID with embedded apostrophe — would close the
	// surrounding '' if not escaped.
	got := notifyCampaignOnClick("camp'evil", "v0.1.10").Call
	if strings.Contains(got, "camp'evil") {
		t.Error("interpolated campaign ID with apostrophe should be JS-escaped, " +
			"not embedded raw (would break out of the surrounding JS string literal)")
	}
	// SOME form of JS escape must be present. text/template's
	// JSEscapeString uses backslash-apostrophe (`\'`) which is
	// valid inside a JS single-quoted string. We don't pin the
	// exact form (the stdlib might change between releases); we
	// just confirm the apostrophe is NOT raw, by re-checking the
	// "doesn't terminate string early" property at the JS level —
	// look for the escaping backslash directly before the
	// apostrophe.
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
