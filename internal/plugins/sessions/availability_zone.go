package sessions

import (
	"context"
	"fmt"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/timeutil"
)

// ConfirmMyAvailabilityZone records the member's answer to "which zone are
// your hours in?". Hours saved under the wrong zone show the Director the
// wrong times, and only the member knows which reading they meant.
func (s *sessionService) ConfirmMyAvailabilityZone(ctx context.Context, campaignID, userID string, req ConfirmZoneRequest) error {
	if req.Mode == "keep" {
		return s.repo.ConfirmAvailabilityZone(ctx, campaignID, userID)
	}
	if req.Mode != "relabel" && req.Mode != "convert" {
		return apperror.NewBadRequest("unknown answer")
	}
	if !timeutil.IsValidLocation(req.TZ) {
		return apperror.NewBadRequest("a valid IANA timezone is required")
	}
	blocks, err := s.repo.ListUserAvailability(ctx, campaignID, userID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("loading availability: %w", err))
	}
	from := ""
	for _, b := range blocks {
		from = b.TZ
	}
	if req.Mode == "relabel" {
		return s.repo.RewriteAvailabilityZone(ctx, campaignID, userID, req.TZ, blocks, from)
	}
	now := time.Now().In(timeutil.LoadLocation(req.TZ))
	ref := timeutil.CivilDate{Year: now.Year(), Month: now.Month(), Day: now.Day()}
	// Converted one-day changes keep their own zone, which is already right.
	return s.repo.RewriteAvailabilityZone(ctx, campaignID, userID, req.TZ, convertBlocks(blocks, req.TZ, ref), "")
}

// convertBlocks re-tells weekly blocks in zone to, keeping their real times.
// Each block is placed on its next date on or after ref (in its own week
// track) and split where it crosses midnight in the new zone, so the
// offsets are this week's: a block can move by an hour across a DST change
// in only one of the two zones, which a weekly pattern can't express anyway.
func convertBlocks(blocks []AvailabilityBlock, to string, ref timeutil.CivilDate) []AvailabilityBlock {
	toLoc := timeutil.LoadLocation(to)
	seen := map[[4]int]int{}
	var out []AvailabilityBlock
	for _, b := range blocks {
		fromLoc := timeutil.LoadLocation(b.TZ)
		d := ref
		for i := 0; i < 14; i++ {
			d = ref.AddDays(i)
			if int(d.Weekday()) == b.DayOfWeek && cadenceApplies(b.WeekCadence, d) {
				break
			}
		}
		start := timeutil.WallClockInstant(fromLoc, d.Year, d.Month, d.Day, b.StartMinute)
		end := timeutil.WallClockInstant(fromLoc, d.Year, d.Month, d.Day, b.EndMinute)
		for _, seg := range splitToViewerDays(start, end, toLoc) {
			sd, err := timeutil.ParseCivilDate(seg.date)
			if err != nil {
				continue
			}
			cadence := b.WeekCadence
			if cadence != CadenceEveryWeek {
				cadence = WeekCadenceFor(sd)
			}
			nb := AvailabilityBlock{DayOfWeek: int(sd.Weekday()), StartMinute: seg.startMin, EndMinute: seg.endMin, State: b.State, TZ: to, WeekCadence: cadence}
			key := [4]int{nb.DayOfWeek, nb.StartMinute, nb.EndMinute, nb.WeekCadence}
			if i, ok := seen[key]; ok {
				out[i].State = nb.State
				continue
			}
			seen[key] = len(out)
			out = append(out, nb)
		}
	}
	return out
}
