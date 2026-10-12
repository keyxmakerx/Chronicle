package sessions

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// DefaultCalendarResolver names a campaign's default calendar, or "" when it
// has none. Implemented in internal/app over the calendar service, so this
// plugin never imports the calendar plugin.
type DefaultCalendarResolver interface {
	DefaultCalendarID(ctx context.Context, campaignID string) (string, error)
}

// SetDefaultCalendarResolver wires the lookup that stamps a session's
// in-world date with its calendar. Without it sessions are saved with no
// calendar id (as before) and the boot reconciler stamps them later.
func (s *sessionService) SetDefaultCalendarResolver(r DefaultCalendarResolver) {
	s.calendarResolver = r
}

// stampWorldDateCalendar keeps Session.CalendarID consistent with the
// in-world date after a create or merge: a cleared or partial date has no
// calendar, and a complete date with none recorded gets the campaign's
// default calendar (today's only way a date is chosen: there is no calendar
// picker on the session form, so the default is the one it was set against).
// A calendar already recorded is kept when the date is edited. A failed
// lookup leaves the id empty rather than failing the save; the reconciler
// stamps it on the next boot.
func (s *sessionService) stampWorldDateCalendar(ctx context.Context, sess *Session) {
	if !sess.HasCalendarDate() {
		sess.CalendarID = nil
		return
	}
	if sess.CalendarID != nil || s.calendarResolver == nil {
		return
	}
	id, err := s.calendarResolver.DefaultCalendarID(ctx, sess.CampaignID)
	if err != nil {
		slog.Warn("sessions: resolving default calendar for a world date failed; leaving it unstamped",
			slog.String("campaign_id", sess.CampaignID), slog.Any("error", err))
		return
	}
	if id != "" {
		sess.CalendarID = &id
	}
}

// ListCampaignIDsWithUnstampedWorldDates implements SessionService.
func (s *sessionService) ListCampaignIDsWithUnstampedWorldDates(ctx context.Context) ([]string, error) {
	ids, err := s.repo.ListCampaignIDsWithUnstampedWorldDates(ctx)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("listing campaigns with unstamped world dates: %w", err))
	}
	return ids, nil
}

// StampWorldDateCalendar implements SessionService.
func (s *sessionService) StampWorldDateCalendar(ctx context.Context, campaignID, calendarID string) (int64, error) {
	if campaignID == "" || calendarID == "" {
		return 0, apperror.NewBadRequest("campaign and calendar are required")
	}
	n, err := s.repo.StampWorldDateCalendar(ctx, campaignID, calendarID)
	if err != nil {
		return 0, apperror.NewInternal(fmt.Errorf("stamping world dates for campaign %s: %w", campaignID, err))
	}
	return n, nil
}

// ReconcileWorldDateCalendars gives every session that has an in-world date
// but no calendar the campaign's default calendar, which is the calendar the
// date was set against before sessions recorded one. Campaigns with no
// calendar are skipped and retried on the next boot. Idempotent: stamped
// rows are never rewritten. A failure on one campaign is logged and does not
// stop the rest; the first error is returned. Returns sessions stamped.
func ReconcileWorldDateCalendars(ctx context.Context, svc SessionService, resolver DefaultCalendarResolver) (int, error) {
	if svc == nil || resolver == nil {
		return 0, fmt.Errorf("sessions.ReconcileWorldDateCalendars: nil dependency")
	}
	ids, err := svc.ListCampaignIDsWithUnstampedWorldDates(ctx)
	if err != nil {
		return 0, fmt.Errorf("sessions.ReconcileWorldDateCalendars: listing campaigns: %w", err)
	}
	stamped := 0
	var firstErr error
	for _, id := range ids {
		n, err := reconcileCampaignWorldDates(ctx, svc, resolver, id)
		stamped += n
		if err != nil {
			slog.Error("sessions: world date calendar backfill failed for one campaign; continuing",
				slog.String("campaign_id", id), slog.Any("error", err))
			if firstErr == nil {
				firstErr = fmt.Errorf("sessions.ReconcileWorldDateCalendars: campaign %s: %w", id, err)
			}
		}
	}
	return stamped, firstErr
}

func reconcileCampaignWorldDates(ctx context.Context, svc SessionService, resolver DefaultCalendarResolver, campaignID string) (int, error) {
	calID, err := resolver.DefaultCalendarID(ctx, campaignID)
	if err != nil {
		return 0, err
	}
	if calID == "" {
		return 0, nil
	}
	n, err := svc.StampWorldDateCalendar(ctx, campaignID, calID)
	return int(n), err
}
