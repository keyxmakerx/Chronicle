package sessions

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/timeutil"
)

// maxAvailabilityBlocks caps a single member's recurring pattern to a sane
// upper bound (7 days × 48 half-hour slots) so a malformed client can't insert
// an unbounded number of rows.
const maxAvailabilityBlocks = 7 * 48

// Exception caps. maxExceptionsPerUser bounds the total override rows one
// member can hold in a campaign; maxExceptionBlocksPerDay bounds a single
// day's composed set (48 half-hour slots). exceptionDateWindowDays bounds how
// far from today an exception may be dated, so on_date can't be used to stuff
// far-future/far-past rows past the cap's practical reach.
const (
	maxExceptionsPerUser     = 500
	maxExceptionBlocksPerDay = 48
	exceptionDateWindowDays  = 366
)

// validateExceptionDate parses on_date and rejects dates outside today ±1
// year, mirroring the recurring-save validation style: a fixed bound rather
// than an open-ended date field.
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
			WeekCadence: b.WeekCadence,
		})
	}

	// Answered is read from the status store, NOT inferred from len(blocks):
	// saving an empty grid is a real answer ("never free"), and inferring would
	// report that member as silent forever.
	answered, err := s.repo.ListAnsweredUserIDs(ctx, campaignID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("loading availability answers: %w", err))
	}
	if at, ok := answered[userID]; ok {
		resp.Answered = true
		resp.AnsweredAt = at.UTC().Format(time.RFC3339)
	}

	// The two alternating tracks are labelled by the next real Sunday that
	// starts each one, so the picker offers dates rather than a convention.
	// "Today" is resolved in the member's own zone, not UTC: a UTC "today" can
	// land on the wrong side of midnight for the member and swap which track
	// the picker calls A.
	loc := timeutil.LoadLocation(resp.TZ) // falls back to UTC on empty/unknown
	now := time.Now().In(loc)
	today := timeutil.CivilDate{Year: now.Year(), Month: now.Month(), Day: now.Day()}
	resp.WeekALabel = CadenceLabel(CadenceWeekA, today).String()
	resp.WeekBLabel = CadenceLabel(CadenceWeekB, today).String()
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

	// Validate + dedupe by (day, start, end, cadence); the unique key forbids
	// exact duplicates and lets last-state-wins for an overlapping repaint.
	// Cadence is part of the key: the same Monday evening on week A and week B
	// are two different blocks and must not collapse into one.
	seen := make(map[[4]int]int, len(req.Blocks))
	blocks := make([]AvailabilityBlock, 0, len(req.Blocks))
	for _, b := range req.Blocks {
		if err := validateBlockRange(b.DayOfWeek, b.StartMinute, b.EndMinute); err != nil {
			return err
		}
		st, err := validateRecurringState(b.State)
		if err != nil {
			return err
		}
		if !ValidWeekCadence(b.WeekCadence) {
			return apperror.NewBadRequest("unknown week cadence")
		}
		key := [4]int{b.DayOfWeek, b.StartMinute, b.EndMinute, b.WeekCadence}
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
			WeekCadence: b.WeekCadence,
		})
	}

	// The zone rides separately so an EMPTY grid still records WHO answered and
	// in what zone — that is the save that makes "never free" a stated answer
	// instead of continued silence.
	if err := s.repo.ReplaceUserAvailability(ctx, campaignID, userID, req.TZ, blocks); err != nil {
		return apperror.NewInternal(fmt.Errorf("saving availability: %w", err))
	}
	return nil
}

// AvailabilityAnswerStatuses reports, for each member the caller supplies,
// whether they have answered the availability question and when. The roster
// is supplied by the handler (same shape as BuildOverlay's) so this stays
// free of the campaigns import; order is preserved.
func (s *sessionService) AvailabilityAnswerStatuses(ctx context.Context, campaignID string, members []overlayMemberInput) ([]AvailabilityAnswerStatus, error) {
	answered, err := s.repo.ListAnsweredUserIDs(ctx, campaignID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("loading availability answers: %w", err))
	}
	out := make([]AvailabilityAnswerStatus, 0, len(members))
	for _, m := range members {
		row := AvailabilityAnswerStatus{UserID: m.UserID, Name: m.Name}
		if at, ok := answered[m.UserID]; ok {
			row.Answered = true
			row.AnsweredAt = at.UTC().Format(time.RFC3339)
		}
		out = append(out, row)
	}
	return out, nil
}

// NudgeUnansweredAvailability writes a bell notification to every supplied
// member who has NOT answered, and reports who was asked. This is manually
// triggered by the Director, not scheduled — there is no job runner in this
// product. Members who have already answered are counted, not messaged: a
// nudge that pinged everyone would train the whole table to ignore the bell.
func (s *sessionService) NudgeUnansweredAvailability(ctx context.Context, campaignID, link string, members []overlayMemberInput) (*NudgeResult, error) {
	answered, err := s.repo.ListAnsweredUserIDs(ctx, campaignID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("loading availability answers: %w", err))
	}
	res := &NudgeResult{Notified: []string{}}
	var ids []string
	for _, m := range members {
		if m.UserID == "" {
			continue
		}
		if _, ok := answered[m.UserID]; ok {
			res.Skipped++
			continue
		}
		ids = append(ids, m.UserID)
		res.Notified = append(res.Notified, m.Name)
	}
	if len(ids) == 0 {
		return res, nil
	}
	const msg = "Your group needs your availability — set the times you can play"
	if err := s.NotifyUsers(ctx, ids, campaignID, NotifAvailabilityNudge, msg, link); err != nil {
		return nil, err
	}
	return res, nil
}

// ListMyExceptions returns the current user's per-date overrides.
func (s *sessionService) ListMyExceptions(ctx context.Context, campaignID, userID string) ([]AvailabilityException, error) {
	excs, err := s.repo.ListUserExceptions(ctx, campaignID, userID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("loading exceptions: %w", err))
	}
	return excs, nil
}

// AddMyException marks ONE window of a date with an explicit state, KEEPING
// the rest of that date as it already was.
//
// Exception rows fully replace the recurring pattern for their date
// (effectiveBlocks, availability_overlay.go), so a bare inserted row would
// become the member's entire day. This composes the day first — the same
// rule AddMyAvailableWindows and the client-side day editor use — including
// the zone rule: the day is written in the zone its own rows were authored
// in, and the incoming window is converted, never the other way round.
//
// The per-user cap and the date bound are re-checked by ReplaceMyDayExceptions,
// which owns the accounting for a whole-day write; the date bound is also
// checked up front so a malformed date is refused before any repo read.
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
	state, err := validateExceptionState(req.State)
	if err != nil {
		return err
	}

	recurring, err := s.repo.ListUserAvailability(ctx, campaignID, userID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("loading availability: %w", err))
	}
	existing, err := s.repo.ListUserExceptions(ctx, campaignID, userID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("loading exceptions: %w", err))
	}

	days, order, err := resolveOfferedDays(
		[]AvailabilityWindowDTO{{OnDate: req.OnDate, StartMinute: req.StartMinute, EndMinute: req.EndMinute}},
		req.TZ, recurring, existing)
	if err != nil {
		return err
	}

	for _, date := range order {
		d := days[date]
		// keepPreferred is FALSE here, unlike the RSVP offer: this is the member
		// stating what a window IS ("I'm busy 19:00–20:00"), so it outranks an
		// earlier preference. An offer is a generic "I could also do this" and
		// must not downgrade one.
		blocks, err := composeDayWithState(date, d.windows, state, false, recurring, existing)
		if err != nil {
			return err
		}
		if err := s.ReplaceMyDayExceptions(ctx, campaignID, userID, ReplaceDayExceptionsRequest{
			OnDate: date,
			TZ:     d.tz,
			Blocks: blocks,
		}); err != nil {
			return err
		}
	}
	return nil
}

// DeleteMyException removes one of the current user's own exceptions. The repo
// scopes the delete to (campaign, user) so a member can't delete another's.
func (s *sessionService) DeleteMyException(ctx context.Context, campaignID, userID, exceptionID string) error {
	return s.repo.DeleteException(ctx, campaignID, userID, exceptionID)
}

// ReplaceMyDayExceptions atomically replaces the current user's overrides for
// one date with a composed set. Validation mirrors the recurring-save path: a
// valid zone, a bounded date (today ±1 year), a per-day block cap, and a
// per-user total cap so the compose flow can't be used to exceed it. An empty
// Blocks clears the day.
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

	// Count rows on OTHER dates and ensure the new day's set keeps the member
	// under the per-user cap. This date is excluded because a day-replace
	// overwrites it — only the delta on other dates plus this day's new rows
	// counts toward the total.
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

	// Answered-or-not is stamped onto the roster HERE rather than by the
	// handler, because the handler has no business reaching the status store and
	// the pure builder has no business querying anything. The roster is a value
	// copy, so mutating it cannot leak back to the caller's slice contents.
	answered, err := s.repo.ListAnsweredUserIDs(ctx, campaignID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("loading availability answers: %w", err))
	}
	members = append([]overlayMemberInput(nil), members...)
	for i := range members {
		_, members[i].HasAnswered = answered[members[i].UserID]
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

// CampaignMemberZones returns the IANA zone each member set for THEMSELVES on
// the availability page (member_availability.tz), keyed by user id. This is
// distinct from users.timezone, which that page never writes — the two are
// different questions. Members who never painted a grid are absent from the
// map, since absence means "not set here", never a UTC guess.
//
// One read for the whole roster: the overlay renders per member, and asking
// per member would turn a roster render into an N+1.
func (s *sessionService) CampaignMemberZones(ctx context.Context, campaignID string) (map[string]string, error) {
	blocks, err := s.repo.ListCampaignAvailability(ctx, campaignID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("loading campaign availability: %w", err))
	}
	out := make(map[string]string, len(blocks))
	for _, b := range blocks {
		if _, seen := out[b.UserID]; seen {
			continue
		}
		if timeutil.IsValidLocation(b.TZ) {
			out[b.UserID] = b.TZ
		}
	}
	return out, nil
}

// --- Temporary offered availability ---

// maxOfferedWindows bounds one "here's when I could do it" submission. Small on
// purpose: this is an offer attached to a single event invite, not a pattern
// editor — that already exists at /campaigns/:id/availability.
const maxOfferedWindows = 8

// AddMyAvailableWindows records TEMPORARY availability for specific dates,
// WITHOUT touching the recurring weekly pattern.
//
// Exception rows fully replace the recurring pattern for a date (see
// effectiveBlocks in availability_overlay.go), so it must compose the day
// first — existing exceptions, else the recurring pattern for that weekday —
// paint the offered window on top, and write the merged set (this write
// arrives from an email link with no client-side editor to do that).
//
// The composed day is written in the zone its SOURCE ROWS were authored in,
// never the offer's zone: relabelling stored minutes into another zone would
// silently move a member's stated hours. Only the fresh offer gets converted.
//
// SELF-WRITE ONLY: userID comes from the caller's session or a redeemed
// token; every read/write here is scoped to (campaign, user).
func (s *sessionService) AddMyAvailableWindows(ctx context.Context, campaignID, userID, tz string, windows []AvailabilityWindowDTO) error {
	if len(windows) == 0 {
		return apperror.NewValidation("at least one time window is required")
	}
	if len(windows) > maxOfferedWindows {
		return apperror.NewBadRequest(fmt.Sprintf("at most %d time windows at once", maxOfferedWindows))
	}
	if !timeutil.IsValidLocation(tz) {
		return apperror.NewBadRequest("a valid IANA timezone is required")
	}
	for _, w := range windows {
		if _, err := validateExceptionDate(w.OnDate); err != nil {
			return err
		}
		if err := validateMinuteRange(w.StartMinute, w.EndMinute); err != nil {
			return err
		}
	}

	recurring, err := s.repo.ListUserAvailability(ctx, campaignID, userID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("loading availability: %w", err))
	}
	existing, err := s.repo.ListUserExceptions(ctx, campaignID, userID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("loading exceptions: %w", err))
	}

	days, order, err := resolveOfferedDays(windows, tz, recurring, existing)
	if err != nil {
		return err
	}

	for _, date := range order {
		d := days[date]
		blocks, err := composeOfferedDay(date, d.windows, recurring, existing)
		if err != nil {
			return err
		}
		if err := s.ReplaceMyDayExceptions(ctx, campaignID, userID, ReplaceDayExceptionsRequest{
			OnDate: date,
			TZ:     d.tz,
			Blocks: blocks,
		}); err != nil {
			return err
		}
	}
	return nil
}

// offeredDay is one date's worth of offered windows, already expressed in the
// zone that date's composed rows will be written in.
type offeredDay struct {
	tz      string
	windows []AvailabilityWindowDTO
}

// authoredZone reports the zone the member's OWN availability rows are stored
// in, or "" when they have none. The recurring pattern wins, since it is the
// only zone the member chose explicitly (via the "Your timezone" control);
// exception rows are the fallback because some are written by machinery
// (the RSVP "Out this week" action) carrying users.timezone, not a choice.
func authoredZone(recurring []AvailabilityBlock, existing []AvailabilityException) string {
	for _, b := range recurring {
		if timeutil.IsValidLocation(b.TZ) {
			return b.TZ
		}
	}
	for _, e := range existing {
		if timeutil.IsValidLocation(e.TZ) {
			return e.TZ
		}
	}
	return ""
}

// resolveOfferedDays converts offered windows out of the offer's zone and into
// the zone each affected day must be written in, returning the per-date windows
// plus a stable date order.
//
// The target zone for a date is that date's existing exception rows' zone if
// any (those rows already are the day and must stay in their own zone),
// otherwise the member's pattern zone, otherwise the offer's own zone. The
// zone a date resolves to can itself depend on the zone used to project it,
// so this runs as a small bounded fixed point (maxZoneRounds) rather than a
// single pass: project with the pattern zone, and where a resolved date turns
// out to be authored in another zone, re-project with that zone too.
func resolveOfferedDays(windows []AvailabilityWindowDTO, offerTZ string,
	recurring []AvailabilityBlock, existing []AvailabilityException) (map[string]offeredDay, []string, error) {

	const maxZoneRounds = 4

	patternTZ := authoredZone(recurring, existing)
	if patternTZ == "" {
		patternTZ = offerTZ
	}

	// dayTZ[date] = the zone that date's existing exception rows carry.
	dayTZ := make(map[string]string, len(existing))
	for _, e := range existing {
		if dayTZ[e.OnDate] == "" && timeutil.IsValidLocation(e.TZ) {
			dayTZ[e.OnDate] = e.TZ
		}
	}

	offerLoc := timeutil.LoadLocation(offerTZ)

	out := make(map[string]offeredDay, len(windows))
	var order []string
	pending := []string{patternTZ}
	tried := map[string]bool{}

	for round := 0; round < maxZoneRounds && len(pending) > 0; round++ {
		zone := pending[0]
		pending = pending[1:]
		if tried[zone] {
			continue
		}
		tried[zone] = true
		loc := timeutil.LoadLocation(zone)

		for _, w := range windows {
			d, err := timeutil.ParseCivilDate(w.OnDate)
			if err != nil {
				return nil, nil, apperror.NewBadRequest("onDate must be YYYY-MM-DD")
			}
			start := timeutil.WallClockInstant(offerLoc, d.Year, d.Month, d.Day, w.StartMinute)
			end := timeutil.WallClockInstant(offerLoc, d.Year, d.Month, d.Day, w.EndMinute)
			for _, seg := range splitToViewerDays(start, end, loc) {
				want := dayTZ[seg.date]
				if want == "" {
					want = patternTZ
				}
				if want != zone {
					// This date is authored in another zone; re-project the
					// whole offer there in a later round.
					if !tried[want] {
						pending = append(pending, want)
					}
					continue
				}
				if _, seen := out[seg.date]; !seen {
					out[seg.date] = offeredDay{tz: zone}
					order = append(order, seg.date)
				}
				od := out[seg.date]
				od.windows = append(od.windows, AvailabilityWindowDTO{
					OnDate: seg.date, StartMinute: seg.startMin, EndMinute: seg.endMin,
				})
				out[seg.date] = od
			}
		}
	}

	if len(order) == 0 {
		return nil, nil, apperror.NewValidation("at least one time window is required")
	}
	// Converting across zones can push a window onto a date outside the ±1y
	// exception window the direct writes are bounded to; re-check every
	// RESOLVED date so the bound cannot be sidestepped by choosing a zone.
	for _, date := range order {
		if _, err := validateExceptionDate(date); err != nil {
			return nil, nil, err
		}
	}
	sort.Strings(order)
	return out, order, nil
}

// composeOfferedDay builds the full replacement block set for one date: the
// member's current effective day with the offered windows painted on as
// `available`. Painting on a minute canvas handles overlapping, abutting, or
// straddling windows without interval arithmetic. An existing `preferred`
// minute is kept, since an explicit preference outranks a generic offer.
func composeOfferedDay(date string, offers []AvailabilityWindowDTO,
	recurring []AvailabilityBlock, existing []AvailabilityException) ([]ExceptionBlockDTO, error) {
	return composeDayWithState(date, offers, AvailAvailable, true, recurring, existing)
}

// composeDayWithState is composeOfferedDay generalised over the state being
// painted, so the "mark this window" endpoint (AddMyException) shares the exact
// same day-composition rule instead of writing a bare row that erases the day.
//
// keepPreferred is the one behavioural difference between the two callers: a
// generic offer must not downgrade an explicit preference, while a member
// stating "I'm busy 19:00–20:00" must be able to overwrite one.
func composeDayWithState(date string, windows []AvailabilityWindowDTO, state string, keepPreferred bool,
	recurring []AvailabilityBlock, existing []AvailabilityException) ([]ExceptionBlockDTO, error) {

	canvas := make([]string, timeutil.MinutesPerDay)

	var dayExc []AvailabilityException
	for _, e := range existing {
		if e.OnDate == date {
			dayExc = append(dayExc, e)
		}
	}
	if len(dayExc) > 0 {
		for _, e := range dayExc {
			paint(canvas, e.StartMinute, e.EndMinute, e.State, false)
		}
	} else {
		d, err := time.Parse("2006-01-02", date)
		if err != nil {
			return nil, apperror.NewBadRequest("onDate must be YYYY-MM-DD")
		}
		wd := int(d.Weekday())
		for _, b := range recurring {
			if b.DayOfWeek == wd {
				paint(canvas, b.StartMinute, b.EndMinute, b.State, false)
			}
		}
	}

	for _, w := range windows {
		paint(canvas, w.StartMinute, w.EndMinute, state, keepPreferred)
	}

	blocks := runsToBlocks(canvas)
	if len(blocks) > maxExceptionBlocksPerDay {
		return nil, apperror.NewBadRequest("that would split the day into too many blocks; tidy the date up first")
	}
	return blocks, nil
}

// paint fills [start,end) with state. When keepPreferred is set, minutes already
// marked `preferred` are left alone so a generic offer never downgrades an
// explicit preference.
func paint(canvas []string, start, end int, state string, keepPreferred bool) {
	if start < 0 {
		start = 0
	}
	if end > len(canvas) {
		end = len(canvas)
	}
	for i := start; i < end; i++ {
		if keepPreferred && canvas[i] == AvailPreferred {
			continue
		}
		canvas[i] = state
	}
}

// runsToBlocks collapses a minute canvas back into contiguous same-state blocks.
// Unpainted minutes are omitted entirely: on a date that carries any exception
// rows, the rows ARE the effective day, so a gap already means "not available"
// and an explicit `unavailable` row would be redundant.
func runsToBlocks(canvas []string) []ExceptionBlockDTO {
	out := make([]ExceptionBlockDTO, 0, 8)
	i := 0
	for i < len(canvas) {
		st := canvas[i]
		j := i
		for j < len(canvas) && canvas[j] == st {
			j++
		}
		if st != "" {
			out = append(out, ExceptionBlockDTO{StartMinute: i, EndMinute: j, State: st})
		}
		i = j
	}
	return out
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
	// time.Weekday: Sunday=0..Saturday=6, Monday=1.
	offset := (int(d.Weekday()) - int(time.Monday) + 7) % 7
	return d.AddDays(-offset)
}
