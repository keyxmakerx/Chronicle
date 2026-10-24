package calendar

import (
	"context"
	"fmt"
	"log/slog"
)

// DefaultCalendarStore is the persistence surface ReconcileDefaultCalendars
// needs. Satisfied by the calendar repository returned from
// NewCalendarRepository.
type DefaultCalendarStore interface {
	// ListCampaignsWithoutDefault returns the campaigns that have calendars
	// but none marked default.
	ListCampaignsWithoutDefault(ctx context.Context) ([]string, error)
	defaultMarker
}

// defaultMarker is what marking a campaign's first calendar default needs;
// the service's CalendarRepository satisfies it too.
type defaultMarker interface {
	ListByCampaignID(ctx context.Context, campaignID string) ([]Calendar, error)
	SetDefault(ctx context.Context, campaignID, calendarID string) error
}

// ReconcileDefaultCalendars marks the first calendar (by sort order) default
// in every campaign that has calendars but no default. Calendars made before
// a campaign's first calendar became its default never got the mark, and the
// readers that follow "the campaign's calendar" (the Calendar menu link, the
// header clock, Coming up) showed nothing for them. Idempotent: a campaign
// with a default is never touched. One campaign's failure is logged and
// skipped; the count is campaigns repaired.
func ReconcileDefaultCalendars(ctx context.Context, store DefaultCalendarStore) (int, error) {
	ids, err := store.ListCampaignsWithoutDefault(ctx)
	if err != nil {
		return 0, fmt.Errorf("calendar.ReconcileDefaultCalendars: list campaigns: %w", err)
	}
	repaired := 0
	for _, campaignID := range ids {
		if err := ensureDefaultCalendar(ctx, store, campaignID); err != nil {
			slog.Warn("calendar: default calendar reconcile skipped a campaign",
				slog.String("campaign_id", campaignID), slog.Any("error", err))
			continue
		}
		repaired++
	}
	return repaired, nil
}

// ensureDefaultCalendar marks campaignID's first calendar default. Callers
// have checked the campaign has none; no calendars at all is not an error.
func ensureDefaultCalendar(ctx context.Context, store defaultMarker, campaignID string) error {
	cals, err := store.ListByCampaignID(ctx, campaignID)
	if err != nil {
		return err
	}
	if len(cals) == 0 {
		return nil
	}
	return store.SetDefault(ctx, campaignID, cals[0].ID)
}

// ListCampaignsWithoutDefault implements DefaultCalendarStore.
func (r *calendarRepo) ListCampaignsWithoutDefault(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT campaign_id FROM calendars GROUP BY campaign_id HAVING MAX(is_default) = 0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
