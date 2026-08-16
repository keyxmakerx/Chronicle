package sessions

import (
	"fmt"
	"time"

	"github.com/keyxmakerx/chronicle/internal/timeutil"
)

// This file holds the PURE overlay projection — the DST-correct heart of the
// availability heatmap. It takes each member's zone-local recurring pattern
// (plus per-date exceptions) and projects it onto a concrete week's real dates,
// converting every block into the viewer's (DM's) zone. Keeping it pure and
// campaigns-free makes the DST behavior directly unit-testable.

// effBlock is a member's effective availability for one real date, normalized
// from either a recurring block or an exception.
type effBlock struct {
	startMin int
	endMin   int
	state    string
	tz       string
}

// viewerSeg is a block after conversion into the viewer's zone, clipped to a
// single viewer-local calendar date.
type viewerSeg struct {
	date     string // YYYY-MM-DD (viewer zone)
	startMin int    // minute-of-local-midnight [0..1440)
	endMin   int    // (start, 1440]
}

// buildWeekOverlay projects every member's pattern onto the 7 real dates
// starting at weekStart, rendered in viewerLoc. Per-hour density counts are
// always populated; per-member identity (lanes, per-cell user IDs) only when
// includeDetail is true (owner / DM-granted — design §5).
//
// availByUser: userID -> recurring blocks (any zones). excByUser: userID ->
// exceptions (any dates). members carries render order + display data; a
// member with no availability still appears in the roster (empty lanes).
func buildWeekOverlay(
	members []overlayMemberInput,
	availByUser map[string][]AvailabilityBlock,
	excByUser map[string][]AvailabilityException,
	weekStart timeutil.CivilDate,
	viewerLoc *time.Location,
	viewerTZLabel string,
	includeDetail bool,
) WeekOverlay {
	// Column dates (viewer-zone calendar), and a date-string -> column lookup.
	colIndex := make(map[string]int, 7)
	days := make([]OverlayDay, 7)
	for i := 0; i < 7; i++ {
		d := weekStart.AddDays(i)
		days[i] = OverlayDay{
			Date:    d.String(),
			Weekday: int(d.Weekday()),
			Hours:   make([]OverlayHour, 24),
		}
		colIndex[d.String()] = i
	}

	overlay := WeekOverlay{
		WeekStart:     weekStart.String(),
		ViewerTZ:      viewerTZLabel,
		TotalMembers:  len(members),
		IncludeDetail: includeDetail,
		Days:          days,
		Members:       make([]OverlayMember, 0, len(members)),
	}

	for i, m := range members {
		// presence[col][hour] for THIS member: "" | available | preferred.
		// Deduped per member so two blocks touching the same cell count once.
		var presence [7][24]string
		var lanes []LaneSegment

		// Iterate an extended real-date range (-2..8) so blocks that spill
		// across a midnight/zone boundary INTO the window are still captured.
		// Two days of slack on each side (not one) is required: the maximum
		// real zone spread is 26h (UTC+14 vs UTC-12), so a block on a member's
		// real date can land up to ~26h — i.e. into the day-before-the-day-before
		// or the day-after-the-day-after — in the viewer's zone. The gate's
		// verified-failing case: a Pacific/Kiritimati (UTC+14) block Tue
		// 00:00–01:00 lands Sunday 23:00 for a Pacific/Pago_Pago (UTC-12) viewer,
		// sourced from real-date offset +8 — a column the old -1..7 loop never
		// visited (symmetric miss at -2). This range MUST match the exception
		// fetch window in availability_service.go (BuildOverlay).
		for offset := -2; offset <= 8; offset++ {
			realDate := weekStart.AddDays(offset)
			for _, eb := range effectiveBlocks(m.UserID, realDate, availByUser, excByUser) {
				if eb.state == AvailUnavailable {
					continue // punches a hole; nothing to render or count
				}
				memberLoc := timeutil.LoadLocation(eb.tz)
				startInstant := timeutil.WallClockInstant(memberLoc, realDate.Year, realDate.Month, realDate.Day, eb.startMin)
				endInstant := timeutil.WallClockInstant(memberLoc, realDate.Year, realDate.Month, realDate.Day, eb.endMin)

				for _, seg := range splitToViewerDays(startInstant, endInstant, viewerLoc) {
					col, ok := colIndex[seg.date]
					if !ok {
						continue // outside the visible 7-column window
					}
					lanes = append(lanes, LaneSegment{
						DayIndex:    col,
						StartMinute: seg.startMin,
						EndMinute:   seg.endMin,
						State:       eb.state,
					})
					// Top-of-hour sampling (matches the signed mockup): a member
					// counts for hour h when the segment covers [h*60].
					for h := 0; h < 24; h++ {
						top := h * 60
						if seg.startMin <= top && top < seg.endMin {
							if eb.state == AvailPreferred {
								presence[col][h] = AvailPreferred
							} else if presence[col][h] == "" {
								presence[col][h] = AvailAvailable
							}
						}
					}
				}
			}
		}

		// Fold this member's presence into the day aggregates.
		for col := 0; col < 7; col++ {
			for h := 0; h < 24; h++ {
				st := presence[col][h]
				if st == "" {
					continue
				}
				cell := &overlay.Days[col].Hours[h]
				cell.Free++
				if includeDetail {
					cell.FreeIDs = append(cell.FreeIDs, m.UserID)
				}
				if st == AvailPreferred {
					cell.Prefer++
					if includeDetail {
						cell.PreferIDs = append(cell.PreferIDs, m.UserID)
					}
				}
			}
		}

		// Roster (identity + lanes) is owner-only. Non-owners get just the
		// anonymous density in Days + the TotalMembers tally (design §5).
		if includeDetail {
			overlay.Members = append(overlay.Members, OverlayMember{
				UserID: m.UserID,
				Name:   m.Name,
				Color:  paletteColor(i),
				Avatar: m.Avatar,
				// COPIED, NEVER DERIVED. The old roleLabel(m.IsOwner) is gone:
				// it mislabelled a co-DM as "player" on a permission surface
				// (WG-4). This file stays pure and campaigns-free precisely
				// because the label arrives already resolved.
				Role:   m.RoleLabel,
				IsCoDM: m.IsCoDM,
				TZ:     m.TZ,
				Lanes:  lanes,
				// Empty Lanes is ambiguous on its own — "never free" and "never
				// asked" both render as nothing. This is what tells them apart.
				HasAnswered: m.HasAnswered,
			})
		}
	}

	return overlay
}

// effectiveBlocks returns a member's effective availability for one real date:
// the exception rows for that date if ANY exist (they fully replace the
// recurring pattern for that date), otherwise the recurring blocks for the
// date's weekday.
func effectiveBlocks(userID string, realDate timeutil.CivilDate,
	availByUser map[string][]AvailabilityBlock,
	excByUser map[string][]AvailabilityException) []effBlock {

	dateStr := realDate.String()
	var excForDate []effBlock
	for _, e := range excByUser[userID] {
		if e.OnDate == dateStr {
			excForDate = append(excForDate, effBlock{e.StartMinute, e.EndMinute, e.State, e.TZ})
		}
	}
	if len(excForDate) > 0 {
		return excForDate
	}

	wd := int(realDate.Weekday())
	var out []effBlock
	for _, b := range availByUser[userID] {
		// Cadence is checked against the REAL DATE, not the weekday: an
		// alternating block is in force on half the Mondays, and which half is
		// a fact about the date. cadenceApplies treats 0 (and anything
		// unrecognised) as every week, so pre-cadence rows are unaffected.
		if b.DayOfWeek == wd && cadenceApplies(b.WeekCadence, realDate) {
			out = append(out, effBlock{b.StartMinute, b.EndMinute, b.State, b.TZ})
		}
	}
	return out
}

// maxViewerSegs bounds splitToViewerDays' output. A single availability block
// spans at most one civil day (minute windows are validated into [0,1440]) and
// the widest real zone spread is 26h (UTC+14 vs UTC-12), so a block can touch
// at most three viewer-local dates. Eight is generous headroom.
//
// It is not an optimisation, it is a fuse. This loop used to run unbounded, and
// an unbounded loop that APPENDS is not a spin — it is an allocation storm that
// OOM-kills the whole self-hosted instance, taking the calendar, maps, entities
// and Foundry sync with it, from one authenticated GET. The zone arithmetic
// below is now correct; this exists so that if it is ever wrong again the blast
// radius is a short segment list, not the process.
const maxViewerSegs = 8

// splitToViewerDays converts a [start,end) instant range into the viewer zone
// and yields one segment per viewer-local calendar date it spans, so a block
// that crosses local midnight is placed on both days. End-of-day is reported as
// minute 1440, wall-clock-correct across DST transitions.
//
// THE BOUNDARY IS timeutil.StartOfCivilDay, NEVER time.Date(y,mo,d,0,0,0,0,loc).
// In a zone whose DST jump lands on midnight (America/Havana, America/Santiago,
// Atlantic/Azores) local 00:00 does not exist and Go normalises it backwards
// into the previous day, so the naive expression returned a "next midnight"
// that was EARLIER than the instant the loop was already holding — cur never
// advanced. See StartOfCivilDay's comment for the full mechanism.
func splitToViewerDays(start, end time.Time, loc *time.Location) []viewerSeg {
	var out []viewerSeg
	if !end.After(start) {
		return out
	}
	le := end.In(loc)
	cur := start.In(loc)
	for i := 0; i < maxViewerSegs && cur.Before(le); i++ {
		y, mo, d := cur.Date()
		// First real instant of the NEXT viewer-local civil day.
		nextDayStart := timeutil.StartOfCivilDay(loc, timeutil.CivilDate{Year: y, Month: mo, Day: d}.AddDays(1))
		segEnd := le
		endMin := le.Hour()*60 + le.Minute()
		if !nextDayStart.After(le) {
			segEnd = nextDayStart
			endMin = timeutil.MinutesPerDay // 1440 — end of this local day
		}
		startMin := cur.Hour()*60 + cur.Minute()
		if endMin > startMin { // guard against zero/negative spans
			out = append(out, viewerSeg{
				date:     fmt.Sprintf("%04d-%02d-%02d", y, mo, int(d)),
				startMin: startMin,
				endMin:   endMin,
			})
		}
		if !segEnd.After(cur) {
			// Belt-and-braces: a boundary that did not move past cur would spin
			// forever. Stopping loses at most the tail of one member's block on
			// one day; not stopping loses the server.
			break
		}
		cur = segEnd
	}
	return out
}

// roleLabel is RETIRED (C-CALV4-RSVP-P8 §4 / WG-4, ADR-048 §17). It mapped
// isOwner → "DM" | "player", which was a SECOND role vocabulary competing with
// campaigns.Role.DisplayName(), and it ignored IsDmGranted entirely — so a
// co-DM was labelled "player" on the surface whose whole subject is
// who-may-see-what, while receiving owner-tier detail. The label is now
// resolved once, at the handler, from the roster's own Role plus the campaign's
// DmGrantIDs, and copied through overlayMemberInput.RoleLabel / IsCoDM.
//
// The function is deleted rather than deprecated on purpose: leaving it
// compiling is how a second vocabulary comes back.
