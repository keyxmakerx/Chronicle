// settings_tab_sanitize_test.go pins the reflected-XSS guard on the
// campaign Settings page (audit SEC-1; cordinator core-tenets §T-B1).
//
// The Settings handler reads `?tab=` from the query string and hands it
// to CampaignSettingsPage, which interpolates it into an Alpine.js
// `x-data` expression. Because the browser HTML-decodes an attribute
// before Alpine evaluates it as JavaScript, an unvalidated value like
// `');alert(1)//` breaks out of the JS string literal and executes in
// the owner's authenticated session. Two layers defend against it and
// both are pinned here:
//
//  1. sanitizeSettingsTab constrains the value to a known, visible tab
//     ID (source-side allowlist) — TestSanitizeSettingsTab_*.
//  2. jsEsc escapes the value at the templ sink (defense in depth) —
//     TestJsEsc_NeutralizesSettingsTabPayload.
package campaigns

import "testing"

// ownerVisibleTabs builds the real role-filtered tab set an owner sees,
// exactly as the Settings handler does, so the sanitizer is exercised
// against production tab IDs rather than a hand-rolled list.
func ownerVisibleTabs() []SettingsTab {
	h := &Handler{}
	return h.visibleSettingsTabs(ctxWithRole(RoleOwner), nil, nil, "csrf", nil, false)
}

// TestSanitizeSettingsTab_KnownTabsPassThrough confirms every legitimate,
// visible tab ID resolves to itself — the guard must not break normal
// navigation.
func TestSanitizeSettingsTab_KnownTabsPassThrough(t *testing.T) {
	tabs := ownerVisibleTabs()
	for _, tab := range tabs {
		if got := sanitizeSettingsTab(tab.ID, tabs); got != tab.ID {
			t.Errorf("sanitizeSettingsTab(%q) = %q, want %q (known tab must pass through)",
				tab.ID, got, tab.ID)
		}
	}
}

// TestSanitizeSettingsTab_HostileAndUnknownFallBackToGeneral is the core
// XSS-regression pin: any empty, unknown, or attacker-crafted value must
// resolve to the constant "general" — never echo the source text.
func TestSanitizeSettingsTab_HostileAndUnknownFallBackToGeneral(t *testing.T) {
	tabs := ownerVisibleTabs()

	cases := []struct {
		name string
		in   string
	}{
		// The confirmed exploit payload. As a URL it is
		// `?tab=%27%29%3Balert(1)%2F%2F`; Echo's QueryParam percent-decodes
		// it to the string below before it ever reaches the sanitizer.
		{"xss_breakout_payload", "');alert(1)//"},
		{"xss_script_tag", "<script>alert(1)</script>"},
		{"xss_quote_only", "'"},
		{"xss_backslash_quote", `\'`},
		{"empty", ""},
		{"unknown_slug", "does-not-exist"},
		{"general_with_trailing_junk", "general'"},
		{"case_variant", "General"},
		{"whitespace", "  general  "},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeSettingsTab(tc.in, tabs)
			if got != "general" {
				t.Fatalf("sanitizeSettingsTab(%q) = %q, want \"general\"", tc.in, got)
			}
			// Assert the sanitized value, not the source text: a hostile
			// input must never survive to the template verbatim.
			if got == tc.in && tc.in != "general" {
				t.Fatalf("sanitizeSettingsTab returned the raw input %q instead of the safe constant", tc.in)
			}
		})
	}
}

// TestSanitizeSettingsTab_RoleHiddenTabRejected proves the guard scopes
// to what the viewer can actually see: a tab that exists in the registry
// but is filtered out of this viewer's set (here, an owner-only plugin
// tab requested by a player) resolves to "general" — you cannot
// pre-select a tab your role hides.
func TestSanitizeSettingsTab_RoleHiddenTabRejected(t *testing.T) {
	h := &Handler{}
	h.RegisterSettingsTab(func(*CampaignContext) SettingsTab {
		return SettingsTab{ID: "secret-owner-tab", MinRole: RoleOwner, SortOrder: 55}
	})

	playerTabs := h.visibleSettingsTabs(ctxWithRole(RolePlayer), nil, nil, "csrf", nil, false)
	if got := sanitizeSettingsTab("secret-owner-tab", playerTabs); got != "general" {
		t.Errorf("player selecting an owner-only tab = %q, want \"general\" (role-hidden tab must be rejected)", got)
	}

	// Sanity: an owner CAN reach the same tab, so the rejection above is
	// role-based, not a blanket denial.
	ownerTabs := h.visibleSettingsTabs(ctxWithRole(RoleOwner), nil, nil, "csrf", nil, false)
	if got := sanitizeSettingsTab("secret-owner-tab", ownerTabs); got != "secret-owner-tab" {
		t.Errorf("owner selecting an owner-only tab = %q, want \"secret-owner-tab\"", got)
	}
}

// TestJsEsc_NeutralizesSettingsTabPayload pins the second defense layer:
// even if a non-constant value reached the sink, jsEsc must escape the
// single quotes that let the payload break out of the `'%s'` literal.
func TestJsEsc_NeutralizesSettingsTabPayload(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"breakout_payload", "');alert(1)//", `\');alert(1)//`},
		{"single_quote", "'", `\'`},
		{"backslash", `\`, `\\`},
		{"backslash_then_quote", `\'`, `\\\'`},
		{"newline", "a\nb", `a\nb`},
		{"carriage_return", "a\rb", `a\rb`},
		{"safe_slug_untouched", "general", "general"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := jsEsc(tc.in); got != tc.want {
				t.Errorf("jsEsc(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
