package sessions

import (
	"context"
	"fmt"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/timeutil"
)

// maxAvailabilityBlocks caps a single member's recurring pattern to a sane
// upper bound (7 days × 48 half-hour slots) so a malformed client can't insert
// an unbounded number of rows.
const maxAvailabilityBlocks = 7 * 48

// Exception caps (C-SCHED-P2 0d). maxExceptionsPerUser bounds the total override
// rows one member can hold in a campaign; maxExceptionBlocksPerDay bounds a
// single day's composed set (48 half-hour slots). exceptionDateWindowDays bounds
// how far from today an exception may be dated, so on_date can't be used to
// stuff far-future/far-past rows past the cap's practical reach.
const (
	maxExceptionsPerUser     = 500
	maxExceptionBlocksPerDay = 48
	exceptionDateWindowDays  = 366
)

// validateExceptionDate parses on_date and rejects dates outside today ±1 year
// (C-SCHED-P2 0d). Mirrors the recurring-save validation style: a fixed, sane
// bound rather than an open-ended date field.
func validateExceptionDate(onDate string) (time.Time, error) {
	d, err := time.Parse("2006-01-02", onDate)
	if err != nil {
		return time.Time{}, apperror.NewBadRequest("onDate must be YYYY-MM-DD")
	}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	lo := today.AddDate(0, 0, -exceptionDateWindowDays)
	hi := today.AddDate(0, 0, exceptionDateWindowDays)
	if d.Before(lo) || d.After(hi) {
		return time.Time{}, apperror.NewBadRequest("onDate must be within one year of today")
	}
	return d, nil
}

// GetMyAvailability returns the current user's own recurring pattern for the
// campaign, ready to seed the paint grid.
func (s *sessionService) GetMyAvailability(ctx context.Context, campaignID, userID string) (*MyAvailabilityResponse, error) {
	blocks, err := s.repo.ListUserAvailability(ctx, campaignID, userID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("loading availability: %w", err))
	}
	resp := &MyAvailabilityResponse{Blocks: make([]AvailabilityBlockDTO, 0, len(blocks))}
	for _, b := range blocks {
		resp.TZ = b.TZ // rows share the member's zone; last wins (all equal)
		resp.Blocks = append(resp.Blocks, AvailabilityBlockDTO{
			DayOfWeek:   b.DayOfWeek,
			StartMinute: b.StartMinute,
			EndMinute:   b.EndMinute,
			State:       b.State,
		})
	}
	return resp, nil
}

// SaveMyAvailability validates and atomically replaces the current user's
// recurring pattern. The whole grid is sent every save (replace-all).
func (s *sessionService) SaveMyAvailability(ctx context.Context, campaignID, userID string, req SaveAvailabilityRequest) error {
	if !timeutil.IsValidLocation(req.TZ) {
		return apperror.NewBadRequest("a valid IANA timezone is required")
	}
	if len(req.Blocks) > maxAvailabilityBlocks {
		return apperror.NewBadRequest("too many availability blocks")
	}

	// Validate + dedupe by (day, start, end); the unique key forbids exact
	// duplicates, and deduping lets last-state-wins for an overlapping repaint.
	seen := make(map[[3]int]int, len(req.Blocks))
	blocks := make([]AvailabilityBlock, 0, len(req.Blocks))
	for _, b := range req.Blocks {
		if err := validateBlockRange(b.DayOfWeek, b.StartMinute, b.EndMinute); err != nil {
			return err
		}
		st, err := validateRecurringState(b.State)
		if err != nil {
			return err
		}
		key := [3]int{b.DayOfWeek, b.StartMinute, b.EndMinute}
		if idx, ok := seen[key]; ok {
			blocks[idx].State = st // last write wins on an exact overlap
			continue
		}
		seen[key] = len(blocks)
		blocks = append(blocks, AvailabilityBlock{
			DayOfWeek:   b.DayOfWeek,
			StartMinute: b.StartMinute,
			EndMinute:   b.EndMinute,
			State:       st,
			TZ:          req.TZ,
		})
	}

	if err := s.repo.ReplaceUserAvailability(ctx, campaignID, userID, blocks); err != nil {
		return apperror.NewInternal(fmt.Errorf("saving availability: %w", err))
	}
	return nil
}

// ListMyExceptions returns the current user's per-date overrides.
func (s *sessionService) ListMyExceptions(ctx context.Context, campaignID, userID string) ([]AvailabilityException, error) {
	excs, err := s.repo.ListUserExceptions(ctx, campaignID, userID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("loading exceptions: %w", err))
	}
	return excs, nil
}

// AddMyException validates and stores a per-date override for the current user.
func (s *sessionService) AddMyException(ctx context.Context, campaignID, userID string, req AddExceptionRequest) error {
	if _, err := validateExceptionDate(req.OnDate); err != nil {
		return err
	}
	if !timeutil.IsValidLocation(req.TZ) {
		return apperror.NewBadRequest("a valid IANA timezone is required")
	}
	if err := validateMinuteRange(req.StartMinute, req.EndMinute); err != nil {
		return err
	}
	if _, err := validateExceptionState(req.State); err != nil {
		return err
	}
	// Per-user cap (0d): reject once the member is already at the ceiling. The
	// underlying add upserts on the unique block key, so a repaint of an
	// existing block doesn't grow the count — only genuinely new rows do.
	count, err := s.repo.CountUserExceptions(ctx, campaignID, userID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("counting exceptions: %w", err))
	}
	if count >= maxExceptionsPerUser {
		return apperror.NewBadRequest("too many availability exceptions; delete some before adding more")
	}
	exc := &AvailabilityException{
		ID:          generateUUID(),
		CampaignID:  campaignID,
		UserID:      userID,
		OnDate:      req.OnDate,
		StartMinute: req.StartMinute,
		EndMinute:   req.EndMinute,
		State:       req.State,
		TZ:          req.TZ,
	}
	if err := s.repo.AddException(ctx, exc); err != nil {
		return apperror.NewInternal(fmt.Errorf("adding exception: %w", err))
	}
	return nil
}

// DeleteMyException removes one of the current user's own exceptions. The repo
// scopes the delete to (campaign, user) so a member can't delete another's.
func (s *sessionService) DeleteMyException(ctx context.Context, campaignID, userID, exceptionID string) error {
	return s.repo.DeleteException(ctx, campaignID, userID, exceptionID)
}

// ReplaceMyDayExceptions atomically replaces the current user's overrides for
// one date with a composed set (C-SCHED-P2 0c). Validation mirrors the
// recurring-save path: a valid zone, a bounded date (today ±1 year, 0d), a
// per-day block cap, and a per-user total cap so the compose flow can't be used
// to blow past 0d's ceiling. An empty Blocks clears the day.
func (s *sessionService) ReplaceMyDayExceptions(ctx context.Context, campaignID, userID string, req ReplaceDayExceptionsRequest) error {
	if _, err := validateExceptionDate(req.OnDate); err != nil {
		return err
	}
	if !timeutil.IsValidLocation(req.TZ) {
		return apperror.NewBadRequest("a valid IANA timezone is required")
	}
	if len(req.Blocks) > maxExceptionBlocksPerDay {
		return apperror.NewBadRequest("too many blocks for one day")
	}

	excs := make([]AvailabilityException, 0, len(req.Blocks))
	for _, b := range req.Blocks {
		if err := validateMinuteRange(b.StartMinute, b.EndMinute); err != nil {
			return err
		}
		st, err := validateExceptionState(b.State)
		if err != nil {
			return err
		}
		excs = append(excs, AvailabilityException{
			StartMinute: b.StartMinute,
			EndMinute:   b.EndMinute,
			State:       st,
			TZ:          req.TZ,
		})
	}

	// Per-user cap (0d): count rows on OTHER dates and ensure the new day's set
	// keeps the member under the ceiling. Counting excludes this date because a
	// day-replace overwrites it — only the delta on other dates plus this day's
	// new rows counts toward the total.
	existing, err := s.repo.CountUserExceptions(ctx, campaignID, userID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("counting exceptions: %w", err))
	}
	dayExisting, err := s.repo.ListUserExceptions(ctx, campaignID, userID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("loading exceptions: %w", err))
	}
	onThisDate := 0
	for _, e := range dayExisting {
		if e.OnDate == req.OnDate {
			onThisDate++
		}
	}
	if existing-onThisDate+len(excs) > maxExceptionsPerUser {
		return apperror.NewBadRequest("too many availability exceptions; delete some before adding more")
	}

	if err := s.repo.ReplaceDayExceptions(ctx, campaignID, userID, req.OnDate, excs); err != nil {
		return apperror.NewInternal(fmt.Errorf("replacing day exceptions: %w", err))
	}
	return nil
}

// BuildOverlay loads the whole campaign's availability and projects it onto the
// week starting at weekStart (snapped to Monday), rendered in viewerTZ. The
// members roster (render order + display) is supplied by the handler; per-member
// detail is included only when includeDetail is true (owner / DM-granted).
func (s *sessionService) BuildOverlay(ctx context.Context, campaignID string, members []overlayMemberInput, weekStart, viewerTZ string, includeDetail bool) (*WeekOverlay, error) {
	start, err := timeutil.ParseCivilDate(weekStart)
	if err != nil {
		return nil, apperror.NewBadRequest("week must be YYYY-MM-DD")
	}
	start = mondayOf(start)

	blocks, err := s.repo.ListCampaignAvailability(ctx, campaignID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("loading campaign availability: %w", err))
	}
	// Exceptions can spill into the window from up to two days before/after
	// (a 26h zone crossing, UTC+14 vs UTC-12), so fetch the extended
	// [start-2, start+8] range the projection iterates over. This MUST stay in
	// lockstep with the offset loop in buildWeekOverlay (availability_overlay.go)
	// or an exception whose block projects into the visible week from the far
	// edge would be dropped.
	excs, err := s.repo.ListCampaignExceptionsInRange(ctx, campaignID,
		start.AddDays(-2).String(), start.AddDays(8).String())
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("loading campaign exceptions: %w", err))
	}

	availByUser := make(map[string][]AvailabilityBlock)
	for _, b := range blocks {
		availByUser[b.UserID] = append(availByUser[b.UserID], b)
	}
	excByUser := make(map[string][]AvailabilityException)
	for _, e := range excs {
		excByUser[e.UserID] = append(excByUser[e.UserID], e)
	}

	viewerLoc := timeutil.LoadLocation(viewerTZ)
	overlay := buildWeekOverlay(members, availByUser, excByUser, start, viewerLoc, viewerTZ, includeDetail)
	return &overlay, nil
}

// --- validation helpers ---

// validateMinuteRange checks a [start,end) minute window is within one civil day.
func validateMinuteRange(startMin, endMin int) error {
	if startMin < 0 || endMin > timeutil.MinutesPerDay || startMin >= endMin {
		return apperror.NewBadRequest("invalid time range")
	}
	return nil
}

// validateBlockRange checks a recurring block's weekday and minute window.
func validateBlockRange(dayOfWeek, startMin, endMin int) error {
	if dayOfWeek < 0 || dayOfWeek > 6 {
		return apperror.NewBadRequest("day_of_week must be 0..6")
	}
	return validateMinuteRange(startMin, endMin)
}

// validateRecurringState allows only available/preferred for the recurring
// pattern (absence of a row means unavailable).
func validateRecurringState(state string) (string, error) {
	switch state {
	case AvailAvailable, AvailPreferred:
		return state, nil
	default:
		return "", apperror.NewBadRequest("state must be available or preferred")
	}
}

// validateExceptionState additionally allows an explicit unavailable override.
func validateExceptionState(state string) (string, error) {
	switch state {
	case AvailAvailable, AvailPreferred, AvailUnavailable:
		return state, nil
	default:
		return "", apperror.NewBadRequest("state must be available, preferred, or unavailable")
	}
}

// mondayOf snaps a civil date back to the Monday of its week, so overlay
// columns are always Mon..Sun regardless of the date the client requested.
func mondayOf(d timeutil.CivilDate) timeutil.CivilDate {
	// time.Weekday: Sunday=0..Saturday=6; Monday=1.
	offset := (int(d.Weekday()) - int(time.Monday) + 7) % 7
	return d.AddDays(-offset)
}
