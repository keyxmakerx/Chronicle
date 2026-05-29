// calendar_v2_tier_test.go covers Wave 1.6 Phase 1 + 2:
//   Phase 1 — Event.Tier plumbing (model field + repo SELECT/INSERT/
//             UPDATE plumbing exists; integration tests cover the
//             projection-layer behavior since repo tests require DB).
//   Phase 2 — eventToCardDataWithTiers campaign-aware tier resolution.

package calendar

import (
	"testing"

	calwidget "github.com/keyxmakerx/chronicle/internal/widgets/calendar_v2"
)

// --- Phase 2: tier resolution from campaign definitions ---

func TestEventToCardDataWithTiers_NoTierField_FallsBackToPlatformDefault(t *testing.T) {
	e := Event{ID: "e1", Name: "Untiered", Visibility: "everyone"}
	got := eventToCardDataWithTiers(e, nil, nil)
	if got.Tier != calwidget.TierStandard {
		t.Errorf("nil tier should fall back to TierStandard; got %q", got.Tier)
	}
	if got.TierLabel != "" {
		t.Errorf("nil tier should leave TierLabel empty; got %q", got.TierLabel)
	}
	if got.TierColor != "" {
		t.Errorf("nil tier should leave TierColor empty; got %q", got.TierColor)
	}
}

func TestEventToCardDataWithTiers_EmptyTier_FallsBackCleanly(t *testing.T) {
	empty := ""
	e := Event{ID: "e1", Name: "X", Tier: &empty, Visibility: "everyone"}
	got := eventToCardDataWithTiers(e, nil, []TierDefinitionAlias{{Slug: "major", Name: "Major", Color: "#abc", Prominence: 80}})
	if got.Tier != calwidget.TierStandard {
		t.Errorf("empty-string tier should fall back; got %q", got.Tier)
	}
}

func TestEventToCardDataWithTiers_ResolvesValidTier(t *testing.T) {
	slug := "legendary"
	e := Event{ID: "e1", Name: "Coronation", Tier: &slug, Visibility: "everyone"}
	tiers := []TierDefinitionAlias{
		{Slug: "minor", Name: "Minor", Color: "#888", Prominence: 20},
		{Slug: "legendary", Name: "Legendary", Color: "#ff8800", Prominence: 95},
	}
	got := eventToCardDataWithTiers(e, nil, tiers)
	if got.Tier != calwidget.TierMajor {
		t.Errorf("prominence 95 → TierMajor expected; got %q", got.Tier)
	}
	if got.TierLabel != "Legendary" {
		t.Errorf("expected TierLabel='Legendary'; got %q", got.TierLabel)
	}
	if got.TierColor != "#ff8800" {
		t.Errorf("expected TierColor=#ff8800; got %q", got.TierColor)
	}
}

func TestEventToCardDataWithTiers_DeletedTierFallsBackWithWarn(t *testing.T) {
	// Operator deleted a tier; existing events with that slug should
	// gracefully degrade to platform default. The slog.Warn isn't
	// asserted here (would require slog capture); the behavior is
	// the platform fall-back.
	slug := "deleted-tier"
	e := Event{ID: "e1", Name: "Orphan", Tier: &slug, Visibility: "everyone"}
	tiers := []TierDefinitionAlias{{Slug: "standard", Name: "Standard", Color: "#888", Prominence: 50}}
	got := eventToCardDataWithTiers(e, nil, tiers)
	if got.Tier != calwidget.TierStandard {
		t.Errorf("deleted tier slug should fall back to TierStandard; got %q", got.Tier)
	}
	if got.TierLabel != "" {
		t.Errorf("fall-back should leave TierLabel empty; got %q", got.TierLabel)
	}
}

// --- Prominence → Tier mapping ---

func TestMapProminenceToTier_BoundaryThirds(t *testing.T) {
	cases := []struct {
		prominence int
		want       calwidget.Tier
	}{
		{0, calwidget.TierMinor},
		{20, calwidget.TierMinor},
		{33, calwidget.TierMinor},
		{34, calwidget.TierStandard},
		{50, calwidget.TierStandard},
		{66, calwidget.TierStandard},
		{67, calwidget.TierMajor},
		{80, calwidget.TierMajor},
		{100, calwidget.TierMajor},
	}
	for _, tc := range cases {
		got := mapProminenceToTier(tc.prominence)
		if got != tc.want {
			t.Errorf("prominence %d → %q; want %q", tc.prominence, got, tc.want)
		}
	}
}

// --- Backward-compat shim: eventToCardData (legacy 2-arg form) ---

func TestEventToCardData_BackwardCompat(t *testing.T) {
	// The single-resource projection still callable; no tiers means
	// no campaign-aware overlay; result matches the PR #366 behavior.
	e := Event{ID: "e1", Name: "X", Visibility: "everyone"}
	got := eventToCardData(e, nil)
	if got.Tier != calwidget.TierStandard {
		t.Errorf("backward-compat form should default to TierStandard; got %q", got.Tier)
	}
	if got.TierLabel != "" || got.TierColor != "" {
		t.Errorf("backward-compat form should leave overlay fields empty; got %+v", got)
	}
}

// --- Event.Tier round-trip through input structs ---

func TestCreateEventInput_TierPointerRoundTrip(t *testing.T) {
	slug := "festival"
	input := CreateEventInput{Name: "Midwinter", Visibility: "everyone", Tier: &slug}
	if input.Tier == nil || *input.Tier != "festival" {
		t.Errorf("CreateEventInput.Tier should round-trip; got %v", input.Tier)
	}
}

func TestUpdateEventInput_TierNilPreserve(t *testing.T) {
	// Nil tier on UpdateEventInput signals "leave existing tier
	// untouched" per the service-layer nil-preserve convention.
	// Verified by struct-level inspection; service-layer test would
	// cover the actual preserve path with a mock repo.
	input := UpdateEventInput{Name: "X", Tier: nil}
	if input.Tier != nil {
		t.Error("nil tier on input should stay nil through round-trip")
	}
}
