// helpers.go — pure-Go helpers consumed by the V2 widget templ files.
// Kept separate so unit tests can pin per-helper behavior independent
// of templ generation.

package calendar_v2

import (
	"encoding/json"
	"strings"

	"github.com/a-h/templ"
)

// compactTierClasses returns the Tailwind classes for a compact-density
// event card's tier treatment. Compact lives inside a Month-view day
// cell so the treatment is restrained (no border weight changes; only
// opacity for tier-minor + accent text for tier-major).
func compactTierClasses(t Tier) string {
	switch t {
	case TierMinor:
		return "opacity-60"
	case TierMajor:
		return "font-medium"
	}
	return ""
}

// tierBadgeColor returns the badge color class for the tier badge.
// Major uses accent; standard uses subtle muted.
func tierBadgeColor(t Tier) string {
	switch t {
	case TierMajor:
		return "text-accent"
	case TierStandard:
		return "text-fg-secondary"
	}
	return ""
}

// leftBarColor returns the event card's left-bar color hex. Falls
// back to the theme's neutral border color when no category color
// is set (the inline style is "border-left-color:<c>" so a fallback
// keeps the visual rhythm consistent).
func leftBarColor(categoryColor string) string {
	if categoryColor == "" {
		return "var(--color-edge, #444)"
	}
	return categoryColor
}

// ribbonClasses returns the Tailwind classes for a multi-day ribbon's
// tier treatment. Major gets a thin top border + deeper saturation;
// minor goes to 40% opacity per dispatch §A.2.
func ribbonClasses(t Tier) string {
	base := "card border-y border-edge"
	switch t {
	case TierMajor:
		return base + " border-t-accent ring-1 ring-accent/30"
	case TierMinor:
		return base + " opacity-40"
	}
	return base
}

// ribbonStyle builds the CSS Grid + accent-tint inline style for a
// ribbon. The Span is clamped to >=1 so a 0-span doesn't render an
// invisible row.
func ribbonStyle(data MultiDayRibbonData) string {
	span := data.Span
	if span < 1 {
		span = 1
	}
	tint := "var(--color-surface, #fff)"
	if data.CategoryColor != "" {
		// Subtle tint: 18%-ish alpha via CSS color-mix when the
		// browser supports it; falls back to the raw color.
		tint = "color-mix(in srgb, " + data.CategoryColor + " 18%, transparent)"
	}
	var b strings.Builder
	b.WriteString("grid-column: ")
	b.WriteString(itoa(data.StartCol))
	b.WriteString(" / span ")
	b.WriteString(itoa(span))
	b.WriteString("; background-color: ")
	b.WriteString(tint)
	b.WriteString(";")
	return b.String()
}

// specificPanelStyle returns the inline style for the
// "Specific people" rule panel. When the public radio is selected
// the panel hides with display:none rather than removing the DOM —
// preserves any in-progress rule chips if the operator toggles back.
func specificPanelStyle(isPublic bool) string {
	if isPublic {
		return "display: none;"
	}
	return ""
}

// chipClasses returns the Tailwind classes for a rule chip's
// allow/deny color treatment. Green for allow; amber for deny.
func chipClasses(mode string) string {
	base := "border-edge bg-surface-1"
	switch mode {
	case "allow":
		return "border-green-500/40 bg-green-500/10"
	case "deny":
		return "border-amber-500/40 bg-amber-500/10"
	}
	return base
}

// chipIconClasses returns the icon color class for a rule chip.
func chipIconClasses(mode string) string {
	switch mode {
	case "allow":
		return "text-green-500"
	case "deny":
		return "text-amber-500"
	}
	return "text-fg-secondary"
}

// chipLabel returns the display label for a rule chip. Users get
// "@username"; roles get the role label.
func chipLabel(rule VisibilityRule) string {
	if rule.Label != "" {
		return rule.Label
	}
	if rule.Kind == "user" {
		return "@" + rule.Target
	}
	return rule.Target
}

// rulesToJSON serializes the rule list for the hidden round-trip
// input. JSON encoding failure → empty array string (safe default).
func rulesToJSON(rules []VisibilityRule) string {
	if len(rules) == 0 {
		return "[]"
	}
	b, err := json.Marshal(rules)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func usersToJSON(users []UserOption) string {
	if len(users) == 0 {
		return "[]"
	}
	b, err := json.Marshal(users)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func rolesToJSON(roles []RoleOption) string {
	if len(roles) == 0 {
		return "[]"
	}
	b, err := json.Marshal(roles)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// effectiveAudienceSummary computes the human-readable "N people can
// see this" line below the rule chip row. Logic mirrors the
// server-side visibility evaluator:
//   - public mode: "Everyone"
//   - specific + no rules: "Nobody (no rules)"
//   - specific + allow-only: count of distinct allowed users (+ "and N more" cap)
//   - specific + deny-only: "Everyone except: <names>"
//   - specific + mixed: "Allow rules: N · Deny rules: N" (defer to server)
//
// Client-side computation gives operators an instant signal during
// editing. The server is the final authority on render-time access
// checks; this is purely a UX aid.
func effectiveAudienceSummary(data VisibilityEditorData) string {
	if data.IsPublic {
		return "Everyone with campaign access can see this."
	}
	if len(data.Rules) == 0 {
		return "Nobody yet — add an allow rule to grant access."
	}
	var allows, denies int
	for _, r := range data.Rules {
		if r.Mode == "deny" {
			denies++
		} else {
			allows++
		}
	}
	switch {
	case denies == 0:
		return summaryFromAllows(data.Rules)
	case allows == 0:
		return "Everyone except: " + joinDenyTargets(data.Rules)
	default:
		return itoa(allows) + " allow rule(s), " + itoa(denies) + " deny rule(s). Server resolves precedence."
	}
}

// summaryFromAllows builds the "alice, bob, carol can see this"
// summary when only allow rules are present.
func summaryFromAllows(rules []VisibilityRule) string {
	const cap = 3
	var labels []string
	for _, r := range rules {
		if r.Mode != "allow" {
			continue
		}
		labels = append(labels, chipLabel(r))
		if len(labels) == cap {
			break
		}
	}
	if len(labels) == 0 {
		return "Nobody yet."
	}
	joined := strings.Join(labels, ", ")
	total := 0
	for _, r := range rules {
		if r.Mode == "allow" {
			total++
		}
	}
	if total > cap {
		joined += " and " + itoa(total-cap) + " more"
	}
	return joined + " can see this."
}

// joinDenyTargets builds the comma-separated list of deny-rule labels
// for the "Everyone except: …" summary.
func joinDenyTargets(rules []VisibilityRule) string {
	var labels []string
	for _, r := range rules {
		if r.Mode != "deny" {
			continue
		}
		labels = append(labels, chipLabel(r))
	}
	return strings.Join(labels, ", ")
}

// safeHTML wraps a pre-sanitized HTML string for templ.Raw use in the
// detailed-density event card. Caller MUST sanitize via
// internal/sanitize before passing the string in — this helper
// trusts the input. Exposed as a function returning templ.Component
// so the .templ file can call it like a regular component.
func safeHTML(s string) templ.Component {
	return templ.Raw(s)
}

// itoa renders a non-negative int as decimal without importing strconv
// at this layer — keeps the widget package's import block tidy.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
