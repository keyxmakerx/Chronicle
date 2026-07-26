package calendar

// calendar_v2_quickedit_test.go — the event quick-edit card (C-CAL-QUICKEDIT):
// the editing affordances must be SERVER-GATED to Scribes (markup-level, not
// CSS) — players receive a read-only card with no inputs or buttons.

import (
	"context"
	"strings"
	"testing"
)

func renderQuickEdit(t *testing.T, isScribe bool) string {
	t.Helper()
	var sb strings.Builder
	data := CalendarV2ViewData{IsScribe: isScribe}
	if err := eventQuickEditV2(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render quick-edit: %v", err)
	}
	return sb.String()
}

// TestEventQuickEditV2_ScribeGetsEditor: Scribes get the editable card —
// name input, description textarea, Save + Full-editor buttons.
func TestEventQuickEditV2_ScribeGetsEditor(t *testing.T) {
	html := renderQuickEdit(t, true)
	for _, want := range []string{
		`id="cal-v2-event-quickedit"`,
		"data-qe-name", "data-qe-desc", "data-qe-save", "data-qe-expand",
		"data-qe-close", "data-qe-meta", "data-qe-vis",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("scribe quick-edit card missing %q", want)
		}
	}
}

// TestEventQuickEditV2_PlayersReadOnly: the EVENT is read-only for players —
// no name/description fields, no Save, no Full-editor. The gate is the markup,
// not a CSS hide.
//
// This originally asserted the card contained no `<input`/`<textarea>` at all.
// C-CAL-RSVP-P2 narrowed it to the event-editing fields by name, because a
// player now legitimately types into this card — their own RSVP and their own
// offered availability. That is THEIR data, not the event's, so the blanket
// check was a proxy that had outlived its meaning. The invariant it actually
// protected (a player cannot edit the event from here) is asserted below, and
// pinned harder than before: by field, not by tag.
func TestEventQuickEditV2_PlayersReadOnly(t *testing.T) {
	html := renderQuickEdit(t, false)
	for _, want := range []string{"data-qe-name-ro", "data-qe-desc-ro", "data-qe-close"} {
		if !strings.Contains(html, want) {
			t.Errorf("player quick-edit card missing read-only %q", want)
		}
	}
	for _, gone := range []string{"data-qe-save", "data-qe-expand", "data-qe-name>", "data-qe-desc>"} {
		if strings.Contains(html, gone) {
			t.Errorf("player quick-edit card must not contain event-editing affordance %q", gone)
		}
	}
}

// TestEventQuickEditV2_RSVPSurfaceIsForEveryRole pins the C-CAL-RSVP-P2 gate
// split: answering an RSVP and offering times are PLAYER actions, so they must
// render for a non-Scribe. The full editor drawer is Scribe+ and players never
// receive its DOM, so if these lived only there, RSVP would ship to everyone
// except the people being asked.
func TestEventQuickEditV2_RSVPSurfaceIsForEveryRole(t *testing.T) {
	for _, isScribe := range []bool{false, true} {
		html := renderQuickEdit(t, isScribe)
		for _, want := range []string{
			"data-qe-rsvp", `data-qe-rsvp-btn="yes"`, `data-qe-rsvp-btn="maybe"`, `data-qe-rsvp-btn="no"`,
			"data-qe-rsvp-outweek", "data-qe-rsvp-suggest", "data-qe-rsvp-counts",
			// The temporary-availability offer panel.
			"data-qe-rsvp-offer", "data-qe-offer-date", "data-qe-offer-from",
			"data-qe-offer-to", "data-qe-offer-note", "data-qe-offer-send",
		} {
			if !strings.Contains(html, want) {
				t.Errorf("isScribe=%v: quick-edit card missing RSVP affordance %q", isScribe, want)
			}
		}
	}
}
