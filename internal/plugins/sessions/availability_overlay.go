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

		// Iterate an extended real-date range (-1..7) so blocks that spill
		// across a midnight/zone boundary INTO the window are still captured.
		for offset := -1; offset <= 7; offset++ {
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
				Role:   roleLabel(m.IsOwner),
				Lanes:  lanes,
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
		if b.DayOfWeek == wd {
			out = append(out, effBlock{b.StartMinute, b.EndMinute, b.State, b.TZ})
		}
	}
	return out
}

// splitToViewerDays converts a [start,end) instant range into the viewer zone
// and yields one segment per viewer-local calendar date it spans, so a block
// that crosses local midnight is placed on both days. End-of-day is reported as
// minute 1440, wall-clock-correct across DST transitions.
func splitToViewerDays(start, end time.Time, loc *time.Location) []viewerSeg {
	var out []viewerSeg
	if !end.After(start) {
		return out
	}
	le := end.In(loc)
	cur := start.In(loc)
	for cur.Before(le) {
		y, mo, d := cur.Date()
		nextMidnight := time.Date(y, mo, d, 0, 0, 0, 0, loc).AddDate(0, 0, 1)
		segEnd := le
		endMin := le.Hour()*60 + le.Minute()
		if !nextMidnight.After(le) {
			segEnd = nextMidnight
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
		cur = segEnd
	}
	return out
}

// roleLabel maps the owner flag to the overlay's coarse role label.
func roleLabel(isOwner bool) string {
	if isOwner {
		return "DM"
	}
	return "player"
}
