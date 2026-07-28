package calendar_block

// identity_triple_test.go — pin §3.2: a calendar's identity is hue + pattern +
// letter, and all three must reach the DOM.
//
// Wave 1 shipped with none of them working. calToken whitelisted colour VALUES
// while the producer emits TOKEN NAMES, so every real calendar fell through to
// the neutral structural rule and rendered grey; Pattern and Letter were set by
// the producer and rendered nowhere at all. On a surface whose stated thesis is
// that colour is never the only identity channel, the identity was a grey dot.
//
// The fixtures hid it: they set CalHue to "var(--cal-harptos)" — a colour value,
// which the OLD whitelist accepted — so the widget's own suite rendered a
// correctly-coloured dot while the composed product rendered grey. That is why
// these assertions are written against TOKEN NAMES, exactly what the producer
// emits.

import (
	"strings"
	"testing"
)

func TestIdentityTriple_ReachesTheDOM(t *testing.T) {
	body := render(t, fxHarptos(true))

	if !strings.Contains(body, "--cal:var(--cal-harptos)") {
		t.Error(`the --cal channel is not the calendar's own hue; a token name that falls ` +
			`through the allowlist greys out every calendar's identity`)
	}
	if strings.Contains(body, "--cal:var(--rule-structural-strong)") {
		t.Error("the identity hue resolved to the neutral structural fallback")
	}
	if !strings.Contains(body, `class="dot p`) {
		t.Error("the identity dot carries no pattern class — colour is the only channel")
	}
	if !strings.Contains(body, `class="callet"`) {
		t.Error("the calendar letter does not render — the third identity channel is absent")
	}
}

// TestCalToken_RejectsRawValues keeps the whitelist discipline that the old
// implementation existed for: a bare colour must never reach a style attribute.
func TestCalToken_RejectsRawValues(t *testing.T) {
	for _, bad := range []string{
		"var(--cal-harptos)", // a VALUE, not a token — what the fixtures used to send
		"#ff0000",
		"oklch(0.5 0.2 200)",
		"red; background:url(x)",
		"",
		"unknown-calendar",
	} {
		if got := calToken(bad); got != "var(--rule-structural-strong)" {
			t.Errorf("calToken(%q) = %q, want the structural fallback", bad, got)
		}
	}
}

// TestCalToken_MatchesStylesheet catches half the mirror drift: a token the
// renderer accepts that the stylesheet never defines would resolve to nothing.
//
// The other half — the producer adding a token this allowlist lacks — cannot be
// seen from inside this package, and is the cross-layer seam test's job.
func TestCalToken_MatchesStylesheet(t *testing.T) {
	css := blockCSS(t)
	for token := range calHueTokens {
		if !strings.Contains(css, "--cal-"+token+":") {
			t.Errorf("calHueTokens accepts %q but the stylesheet defines no --cal-%s; "+
				"the identity would resolve to nothing", token, token)
		}
	}
}
