// helpers_test.go covers the V2 widget package helpers — class
// composition for tier treatments + visibility editor logic per
// Q-V2-7 (chip-row builder + effective-audience summary).

package calendar_v2

import (
	"encoding/json"
	"strings"
	"testing"
)

// --- Tier classes / badge ---

func TestTierClasses_MajorGetsAccentRingAndElev(t *testing.T) {
	got := tierClasses(TierMajor)
	if !strings.Contains(got, "ring-accent") || !strings.Contains(got, "card-elev") {
		t.Errorf("major tier should get accent ring + elev; got %q", got)
	}
}

func TestTierClasses_MinorGetsLowOpacity(t *testing.T) {
	got := tierClasses(TierMinor)
	if !strings.Contains(got, "opacity-60") {
		t.Errorf("minor tier should fade; got %q", got)
	}
}

func TestTierBadge_MajorAndStandardDiffer(t *testing.T) {
	if tierBadge(TierMajor) == tierBadge(TierStandard) {
		t.Error("major and standard tier badges should differ")
	}
	if tierBadge(TierMinor) != "" {
		t.Errorf("minor tier should not show a badge; got %q", tierBadge(TierMinor))
	}
}

func TestCompactTierClasses_OnlyMinorAndMajor(t *testing.T) {
	if compactTierClasses(TierStandard) != "" {
		t.Errorf("compact standard should be empty; got %q", compactTierClasses(TierStandard))
	}
	if !strings.Contains(compactTierClasses(TierMinor), "opacity-60") {
		t.Errorf("compact minor should fade; got %q", compactTierClasses(TierMinor))
	}
}

// --- Ribbon style ---

func TestRibbonStyle_ComputesGridColumnAndTint(t *testing.T) {
	data := MultiDayRibbonData{StartCol: 2, Span: 3, CategoryColor: "#ff0000"}
	got := ribbonStyle(data)
	if !strings.Contains(got, "grid-column: 2 / span 3") {
		t.Errorf("expected grid-column with start/span; got %q", got)
	}
	if !strings.Contains(got, "#ff0000") {
		t.Errorf("expected category color in tint; got %q", got)
	}
}

func TestRibbonStyle_ClampsZeroSpanToOne(t *testing.T) {
	data := MultiDayRibbonData{StartCol: 1, Span: 0}
	got := ribbonStyle(data)
	if !strings.Contains(got, "span 1") {
		t.Errorf("expected zero span clamped to 1; got %q", got)
	}
}

// --- Chip color treatments ---

func TestChipClasses_AllowGreenDenyAmber(t *testing.T) {
	if !strings.Contains(chipClasses("allow"), "green") {
		t.Error("allow chip should be green")
	}
	if !strings.Contains(chipClasses("deny"), "amber") {
		t.Error("deny chip should be amber")
	}
}

func TestChipLabel_UserPrefixesAtAndRoleUsesTarget(t *testing.T) {
	u := VisibilityRule{Kind: "user", Target: "alice"}
	if chipLabel(u) != "@alice" {
		t.Errorf("user chip label = %q; want '@alice'", chipLabel(u))
	}
	r := VisibilityRule{Kind: "role", Target: "scribe"}
	if chipLabel(r) != "scribe" {
		t.Errorf("role chip label = %q; want 'scribe'", chipLabel(r))
	}
	// Explicit label wins.
	c := VisibilityRule{Kind: "user", Target: "alice", Label: "Alice the Bold"}
	if chipLabel(c) != "Alice the Bold" {
		t.Errorf("explicit label should win; got %q", chipLabel(c))
	}
}

// --- Effective audience summary (Q-V2-7 chip-row computation) ---

func TestEffectiveAudienceSummary_Public(t *testing.T) {
	got := effectiveAudienceSummary(VisibilityEditorData{IsPublic: true})
	if !strings.Contains(got, "Everyone") {
		t.Errorf("public should mention Everyone; got %q", got)
	}
}

func TestEffectiveAudienceSummary_SpecificEmpty(t *testing.T) {
	got := effectiveAudienceSummary(VisibilityEditorData{IsPublic: false})
	if !strings.Contains(got, "Nobody") {
		t.Errorf("empty rule set should say Nobody; got %q", got)
	}
}

func TestEffectiveAudienceSummary_AllowOnly(t *testing.T) {
	data := VisibilityEditorData{
		IsPublic: false,
		Rules: []VisibilityRule{
			{Mode: "allow", Kind: "user", Target: "alice"},
			{Mode: "allow", Kind: "user", Target: "bob"},
		},
	}
	got := effectiveAudienceSummary(data)
	if !strings.Contains(got, "@alice") || !strings.Contains(got, "@bob") {
		t.Errorf("allow-only should list allowed users; got %q", got)
	}
	if !strings.Contains(got, "can see this") {
		t.Errorf("allow-only should mention 'can see this'; got %q", got)
	}
}

func TestEffectiveAudienceSummary_AllowOnlyOverCapShowsExtra(t *testing.T) {
	rules := []VisibilityRule{}
	for _, n := range []string{"alice", "bob", "carol", "dave", "eve"} {
		rules = append(rules, VisibilityRule{Mode: "allow", Kind: "user", Target: n})
	}
	got := effectiveAudienceSummary(VisibilityEditorData{IsPublic: false, Rules: rules})
	if !strings.Contains(got, "and 2 more") {
		t.Errorf("over-cap allow should include 'and 2 more'; got %q", got)
	}
}

func TestEffectiveAudienceSummary_DenyOnly(t *testing.T) {
	data := VisibilityEditorData{
		IsPublic: false,
		Rules: []VisibilityRule{
			{Mode: "deny", Kind: "user", Target: "mallory"},
		},
	}
	got := effectiveAudienceSummary(data)
	if !strings.Contains(got, "Everyone except") {
		t.Errorf("deny-only should say 'Everyone except'; got %q", got)
	}
	if !strings.Contains(got, "@mallory") {
		t.Errorf("deny-only should list denied users; got %q", got)
	}
}

func TestEffectiveAudienceSummary_MixedDefersToServer(t *testing.T) {
	data := VisibilityEditorData{
		IsPublic: false,
		Rules: []VisibilityRule{
			{Mode: "allow", Kind: "role", Target: "scribe"},
			{Mode: "deny", Kind: "user", Target: "alice"},
		},
	}
	got := effectiveAudienceSummary(data)
	if !strings.Contains(got, "Server resolves") {
		t.Errorf("mixed should defer to server; got %q", got)
	}
}

// --- Rule JSON round-trip ---

func TestRulesToJSON_RoundTripCleanly(t *testing.T) {
	in := []VisibilityRule{
		{Mode: "allow", Kind: "user", Target: "alice", Label: "Alice"},
		{Mode: "deny", Kind: "role", Target: "player", Label: "Players"},
	}
	got := rulesToJSON(in)
	var out []VisibilityRule
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("rules JSON not parseable: %v (got %q)", err, got)
	}
	if len(out) != 2 || out[0].Mode != "allow" || out[1].Mode != "deny" {
		t.Errorf("round-trip mismatch: %+v", out)
	}
}

func TestRulesToJSON_EmptyReturnsArrayLiteral(t *testing.T) {
	if got := rulesToJSON(nil); got != "[]" {
		t.Errorf("empty rules should marshal to '[]'; got %q", got)
	}
}

// --- Specific panel visibility ---

func TestSpecificPanelStyle_HidesWhenPublic(t *testing.T) {
	if !strings.Contains(specificPanelStyle(true), "display: none") {
		t.Error("public mode should hide specific-rules panel")
	}
	if specificPanelStyle(false) != "" {
		t.Errorf("specific mode should show panel (empty style); got %q", specificPanelStyle(false))
	}
}

// --- Left bar color fallback ---

func TestLeftBarColor_FallsBackToVariable(t *testing.T) {
	if got := leftBarColor(""); !strings.Contains(got, "var(") {
		t.Errorf("empty color should use CSS variable fallback; got %q", got)
	}
	if got := leftBarColor("#abc"); got != "#abc" {
		t.Errorf("explicit color should pass through; got %q", got)
	}
}
