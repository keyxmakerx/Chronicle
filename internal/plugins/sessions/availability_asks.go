package sessions

import (
	"context"
	"fmt"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/timeutil"
)

// The Director's asks about players' times, and a player's away dates.
//
// NudgeUnansweredAvailability asks only the members who never answered. The
// two asks here go further, on an explicit press: one member, whatever their
// state, or everyone at once to confirm their times are still right. A
// confirmation is the member's answered_at stamp moving past the ask, so
// "who has confirmed" needs no store of its own: the ask's time is the newest
// confirm notification the campaign has sent.

// maxAwayDays bounds one away stretch, so a typo in a year cannot write a
// year of rows in one press (the per-user exception cap still applies).
const maxAwayDays = 92

// PingMemberAvailability asks one member to check their times. The target
// must be on the campaign's roster and not the asker.
func (s *sessionService) PingMemberAvailability(ctx context.Context, campaignID, askerID, askerName, targetID, link string, members []overlayMemberInput) (*NudgeResult, error) {
	if targetID == askerID {
		return nil, apperror.NewBadRequest("you can't remind yourself")
	}
	var target *overlayMemberInput
	for i := range members {
		if members[i].UserID == targetID {
			target = &members[i]
			break
		}
	}
	if target == nil {
		return nil, apperror.NewNotFound("that player isn't in this campaign")
	}
	msg := askerLabel(askerName) + " asked you to check your times"
	if err := s.NotifyUsers(ctx, []string{targetID}, campaignID, NotifAvailabilityNudge, msg, link); err != nil {
		return nil, err
	}
	return &NudgeResult{Notified: []string{target.Name}}, nil
}

// AskAllToConfirm asks every member but the asker to confirm their times,
// and returns who was asked so the handler can email the same people.
func (s *sessionService) AskAllToConfirm(ctx context.Context, campaignID, askerID, askerName, link string, members []overlayMemberInput) (*NudgeResult, []string, error) {
	res := &NudgeResult{Notified: []string{}}
	var ids []string
	for _, m := range members {
		if m.UserID == "" || m.UserID == askerID {
			continue
		}
		ids = append(ids, m.UserID)
		res.Notified = append(res.Notified, m.Name)
	}
	if len(ids) == 0 {
		return res, ids, nil
	}
	msg := askerLabel(askerName) + " asked everyone to confirm their times"
	if err := s.NotifyUsers(ctx, ids, campaignID, NotifAvailabilityConfirm, msg, link); err != nil {
		return nil, nil, err
	}
	return res, ids, nil
}

// LastConfirmAsk is when the campaign last asked everyone to confirm, or
// the zero time when it never has.
func (s *sessionService) LastConfirmAsk(ctx context.Context, campaignID string) (time.Time, error) {
	at, err := s.repo.LatestCampaignNotificationAt(ctx, campaignID, NotifAvailabilityConfirm)
	if err != nil {
		return time.Time{}, apperror.NewInternal(fmt.Errorf("loading the last confirm ask: %w", err))
	}
	return at, nil
}

// ConfirmMyAvailability records that the member's saved times are still
// right, without changing them. A member with nothing saved has nothing to
// confirm and is told to give their hours instead.
func (s *sessionService) ConfirmMyAvailability(ctx context.Context, campaignID, userID string) error {
	ok, err := s.repo.TouchAvailabilityAnswered(ctx, campaignID, userID, time.Now().UTC())
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("confirming availability: %w", err))
	}
	if !ok {
		return apperror.NewBadRequest("give your hours first, then there is something to confirm")
	}
	return nil
}

// MarkMeAway marks every date from..to (inclusive) as a day the member can't
// play, replacing whatever that day held. Lines, Best times and the planner
// read it like any other day off.
func (s *sessionService) MarkMeAway(ctx context.Context, campaignID, userID string, req AwayRequest) error {
	dates, err := awayDates(req.From, req.To)
	if err != nil {
		return err
	}
	if !timeutil.IsValidLocation(req.TZ) {
		return apperror.NewBadRequest("a valid IANA timezone is required")
	}
	existing, err := s.repo.ListUserExceptions(ctx, campaignID, userID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("loading exceptions: %w", err))
	}
	inRange := map[string]bool{}
	for _, d := range dates {
		inRange[d] = true
	}
	kept := 0
	for _, e := range existing {
		if !inRange[e.OnDate] {
			kept++
		}
	}
	if kept+len(dates) > maxExceptionsPerUser {
		return apperror.NewBadRequest("too many days marked; clear some before adding more")
	}
	for _, d := range dates {
		day := []AvailabilityException{{StartMinute: 0, EndMinute: 1440, State: AvailUnavailable, TZ: req.TZ}}
		if err := s.repo.ReplaceDayExceptions(ctx, campaignID, userID, d, day); err != nil {
			return apperror.NewInternal(fmt.Errorf("marking %s away: %w", d, err))
		}
	}
	return nil
}

// ClearMeAway takes the away mark off from..to, so those days follow the
// member's usual hours again. Only whole days off are removed: a day the
// member shaped by hand keeps its shape.
func (s *sessionService) ClearMeAway(ctx context.Context, campaignID, userID string, req AwayRequest) error {
	dates, err := awayDates(req.From, req.To)
	if err != nil {
		return err
	}
	inRange := map[string]bool{}
	for _, d := range dates {
		inRange[d] = true
	}
	existing, err := s.repo.ListUserExceptions(ctx, campaignID, userID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("loading exceptions: %w", err))
	}
	for _, e := range existing {
		if inRange[e.OnDate] && isWholeDayOff(e) {
			if err := s.repo.DeleteException(ctx, campaignID, userID, e.ID); err != nil {
				return apperror.NewInternal(fmt.Errorf("clearing %s: %w", e.OnDate, err))
			}
		}
	}
	return nil
}

// isWholeDayOff is the away mark's shape: one block, the whole day, off.
func isWholeDayOff(e AvailabilityException) bool {
	return e.StartMinute == 0 && e.EndMinute == 1440 && e.State == AvailUnavailable
}

// awayDates lists from..to inclusive, each inside the exception date window.
func awayDates(from, to string) ([]string, error) {
	start, err := validateExceptionDate(from)
	if err != nil {
		return nil, err
	}
	end, err := validateExceptionDate(to)
	if err != nil {
		return nil, err
	}
	if end.Before(start) {
		return nil, apperror.NewBadRequest("the last day can't be before the first")
	}
	days := int(end.Sub(start).Hours()/24) + 1
	if days > maxAwayDays {
		return nil, apperror.NewBadRequest(fmt.Sprintf("mark at most %d days at once", maxAwayDays))
	}
	out := make([]string, 0, days)
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		out = append(out, d.Format("2006-01-02"))
	}
	return out, nil
}

// askerLabel names the asker in a notification, falling back to the role.
func askerLabel(name string) string {
	if name == "" {
		return "Your DM"
	}
	return name
}
