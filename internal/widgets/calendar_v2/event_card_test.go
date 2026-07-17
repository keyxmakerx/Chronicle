// event_card_test.go — C-CAL-UX-PAIR §Fix 2. The Standard (week view) and
// Detailed (day view) densities must carry a [data-rt-hint] marker next to
// TimeLabel so calendar_v2_shell.js's wireEventTimeHints can fill in the
// viewer-zone hint client-side. Compact (used for all-day chips only, no
// clock time to convert) intentionally carries no marker.

package calendar_v2

import (
	"context"
	"strings"
	"testing"
)

func renderCard(t *testing.T, data EventCardData, density Density) string {
	t.Helper()
	var sb strings.Builder
	if err := EventCard(data, density).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render EventCard(density=%v): %v", density, err)
	}
	return sb.String()
}

func TestEventCard_Standard_RtHintMarkerPresent_WhenTimed(t *testing.T) {
	// StartLabel gates the whole metadata row (pre-existing behavior); a
	// real timed event always carries one, so the fixture needs it too.
	html := renderCard(t, EventCardData{ID: "ev-1", Name: "Siege", StartLabel: "Mirtul 15", TimeLabel: "19:00"}, DensityStandard)
	if !strings.Contains(html, "data-rt-hint") {
		t.Error("standard card must carry the data-rt-hint marker next to a timed TimeLabel")
	}
}

func TestEventCard_Standard_NoRtHintMarker_WhenUntimed(t *testing.T) {
	html := renderCard(t, EventCardData{ID: "ev-1", Name: "Siege", StartLabel: "Mirtul 15", TimeLabel: ""}, DensityStandard)
	if strings.Contains(html, "data-rt-hint") {
		t.Error("standard card must not emit a hint marker for an all-day / untimed event (nothing to convert)")
	}
}

func TestEventCard_Detailed_RtHintMarkerPresent_WhenTimed(t *testing.T) {
	html := renderCard(t, EventCardData{ID: "ev-1", Name: "Siege", TimeLabel: "19:00 — 20:00"}, DensityDetailed)
	if !strings.Contains(html, "data-rt-hint") {
		t.Error("detailed card must carry the data-rt-hint marker next to a timed TimeLabel")
	}
}

func TestEventCard_Compact_NeverEmitsRtHintMarker(t *testing.T) {
	// Compact is the all-day chip density — it has no TimeLabel slot at all.
	html := renderCard(t, EventCardData{ID: "ev-1", Name: "Siege"}, DensityCompact)
	if strings.Contains(html, "data-rt-hint") {
		t.Error("compact density has no time text to hint at; it must never emit the marker")
	}
}
