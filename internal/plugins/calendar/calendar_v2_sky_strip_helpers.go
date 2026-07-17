// calendar_v2_sky_strip_helpers.go — view helpers for the collapsible SKY
// strip (C-CAL-SKY-STRIP, wave 7). The strip sits between the command bar
// and the view grid (calendarV2Header / calendarV2View in calendar_v2.templ)
// and renders a compact glyph summary of the SAME data.WorldState the
// ambient band + skybox widget already consume — no new fetch, per the
// dispatch. The expanded pane mounts the #543 skybox widget itself
// (data-widget="skybox") for the full visual render; this file only builds
// the collapsed row's lightweight glyphs.
package calendar

import "fmt"

// skyStripGlyph is one compact glyph in the strip's collapsed row: a moon
// phase or a celestial event, rendered as an icon + label + optional sub-line.
type skyStripGlyph struct {
	Glyph string
	Label string
	Sub   string
}

// skyStripActive reports whether the strip has anything to show. The strip
// itself still renders (chrome + sync chip) without an active calendar's
// worldstate — only the glyph row degrades — but callers use this to decide
// whether to render glyphs vs. the "no sky data" fallback.
func skyStripActive(data CalendarV2ViewData) bool {
	return wsActive(data)
}

// skyStripMoonPhaseGlyph maps a WorldStateMoon.Phase index (0..7, the same
// new→waxing→full→waning cycle as moonShortPhaseNames) to its Unicode moon
// emoji. The skybox engine's client-side EMOJI_PHASE_CODES table
// (cal-almanac.js) renders the same 8-phase cycle for the canvas moon body;
// this is the plain-Unicode equivalent for the strip's SSR text glyph, no JS
// dependency.
func skyStripMoonPhaseGlyph(phase int) string {
	glyphs := []string{"🌑", "🌒", "🌓", "🌔", "🌕", "🌖", "🌗", "🌘"}
	if phase < 0 || phase >= len(glyphs) {
		return "🌙"
	}
	return glyphs[phase]
}

// skyStripEventGlyph maps a celestial event type to a compact glyph for the
// strip row. Covers the common types; anything else (future catalog
// additions) falls back to a generic star. The full type→glyph taxonomy for
// the skybox widget's canvas rendering lives client-side in cal-almanac.js's
// SKY_FX_META — deliberately NOT duplicated here in full (60+ entries): this
// is a smaller, SSR-only cosmetic subset for the strip's plain-text label.
func skyStripEventGlyph(eventType string) string {
	switch eventType {
	case "meteor-shower", "meteor-storm", "shooting-star", "star-fall", "comet":
		return "☄"
	case "eclipse-solar", "eclipse-lunar":
		return "◑"
	case "blood-moon":
		return "●"
	case "supermoon", "harvest-moon", "blue-moon":
		return "○"
	case "aurora", "arcane-aurora":
		return "✦"
	default:
		return "✦"
	}
}

// skyStripMoonGlyphs projects the seed's moons into strip glyphs. Empty (nil
// WorldState, or a calendar with no moons) renders no moon glyphs — the
// no-worldstate degrade case.
func skyStripMoonGlyphs(data CalendarV2ViewData) []skyStripGlyph {
	if data.WorldState == nil {
		return nil
	}
	out := make([]skyStripGlyph, 0, len(data.WorldState.Moons))
	for _, m := range data.WorldState.Moons {
		label := m.NamedPhase
		if label == "" {
			label = "Moon"
		}
		sub := m.Name
		if m.Name != "" {
			sub = fmt.Sprintf("%s · %d%%", m.Name, int(m.CyclePct*100))
		}
		out = append(out, skyStripGlyph{Glyph: skyStripMoonPhaseGlyph(m.Phase), Label: label, Sub: sub})
	}
	return out
}

// skyStripEventGlyphs projects the seed's active celestial events (meteor
// showers, eclipses, ...) into strip glyphs.
func skyStripEventGlyphs(data CalendarV2ViewData) []skyStripGlyph {
	if data.WorldState == nil {
		return nil
	}
	out := make([]skyStripGlyph, 0, len(data.WorldState.Events))
	for _, e := range data.WorldState.Events {
		if e.Name == "" {
			continue
		}
		out = append(out, skyStripGlyph{Glyph: skyStripEventGlyph(e.Type), Label: e.Name})
	}
	return out
}

// skyStripAllGlyphs combines the moon + event glyphs in display order (moons
// first, then active events) for the collapsed row and the expanded pane's
// detail grid alike — one source, so the two surfaces can't drift apart.
func skyStripAllGlyphs(data CalendarV2ViewData) []skyStripGlyph {
	moons := skyStripMoonGlyphs(data)
	events := skyStripEventGlyphs(data)
	out := make([]skyStripGlyph, 0, len(moons)+len(events))
	out = append(out, moons...)
	out = append(out, events...)
	return out
}

// skyStripCurrentDateString formats data.ActiveCalendar's CURRENT date
// (not the navigated view date — data.Year/Month/Day change as the user
// pages the grid) as "YYYY-MM-DD", matching the %04d-%02d-%02d convention
// used elsewhere for date identifiers (e.g. handler.go's dateStr). SSR'd
// into the strip's data-cal-current-date attribute so calendar_v2_shell.js
// can diff it against the served-date beacon (C-SYNC-DATE-BEACON) without
// a second fetch — ActiveCalendar already has the real-time seam applied
// by the service layer (same source GetCurrentDate's cal.CurrentYear/
// Month/Day reads), so both sides of the drift comparison agree.
func skyStripCurrentDateString(data CalendarV2ViewData) string {
	if data.ActiveCalendar == nil {
		return ""
	}
	cal := data.ActiveCalendar
	return fmt.Sprintf("%04d-%02d-%02d", cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay)
}
