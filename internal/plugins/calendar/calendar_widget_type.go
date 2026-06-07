// calendar_widget_type.go — registers "calendar" with the widget-binding
// framework (C-WIDGET-BINDING-P1-SPINE; worldstate added in P2). The calendar
// plugin declares its behavior to the registry; the binding service drives
// resolution through it without importing calendar internals.
//
// The DEFAULT instance is the campaign's default calendar (svc.GetCalendar) —
// exactly what `entity_calendar` resolved before the framework — so an unbound
// host renders identically (zero churn for #411–#420).
package calendar

import (
	"context"
	"errors"
	"net/http"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/widgetbindings"
)

// WidgetTypeCalendar is the persisted widget_type discriminator for calendars.
const WidgetTypeCalendar = "calendar"

// isNotFound reports whether err is (or wraps) a 404 AppError. apperror.SafeCode
// uses a direct type assertion that misses wrapped errors (the calendar service
// wraps not-found via %w), so the binding guards use errors.As to be robust: a
// genuine not-found is sweepable, anything else is a transient error we must
// NOT sweep on (precedent refinement #1: don't sweep on a blip).
func isNotFound(err error) bool {
	var ae *apperror.AppError
	return errors.As(err, &ae) && ae.Code == http.StatusNotFound
}

// calendarInstanceBacking provides the shared "a calendar id is the instance"
// behavior reused by BOTH the calendar widget type and the worldstate widget
// type (P2): worldstate is a view over a calendar's clock, so its bindable
// instance is also a calendar id. Each widget type embeds this and supplies its
// own Slug() — the only thing that differs.
type calendarInstanceBacking struct {
	svc CalendarService
}

// InstanceExists is both the orphan guard and the campaign-scope security
// check: a calendar instance validates only if it exists AND belongs to the
// campaign. A genuine not-found returns (false, nil) so the service sweeps the
// dead binding; any other error returns (false, err) so a transient DB blip
// does NOT delete the binding.
func (b calendarInstanceBacking) InstanceExists(ctx context.Context, campaignID, instanceID string) (bool, error) {
	cal, err := b.svc.GetCalendarByID(ctx, instanceID)
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, err
	}
	if cal == nil {
		return false, nil
	}
	// SECURITY: cross-campaign instances must not validate.
	return cal.CampaignID == campaignID, nil
}

// DefaultInstance returns the campaign's default calendar id — today's
// pre-framework behavior for the entity calendar + worldstate blocks.
func (b calendarInstanceBacking) DefaultInstance(ctx context.Context, host widgetbindings.HostRef) (string, bool, error) {
	cal, err := b.svc.GetCalendar(ctx, host.CampaignID)
	if err != nil {
		if isNotFound(err) {
			return "", false, nil
		}
		return "", false, err
	}
	if cal == nil {
		return "", false, nil
	}
	return cal.ID, true, nil
}

// ListInstances / CreateInstance power the P4 create-or-pick UI; not wired yet.
func (b calendarInstanceBacking) ListInstances(ctx context.Context, campaignID string, role int) ([]widgetbindings.InstanceRef, error) {
	return nil, widgetbindings.ErrNotImplemented
}

func (b calendarInstanceBacking) CreateInstance(ctx context.Context, campaignID string, input any) (string, error) {
	return "", widgetbindings.ErrNotImplemented
}

// calendarWidgetType is the "calendar" widget type (the entity-page calendar
// embed's instance).
type calendarWidgetType struct {
	calendarInstanceBacking
}

// NewCalendarWidgetType builds the calendar WidgetType for registration into
// the widget-binding registry at app startup.
func NewCalendarWidgetType(svc CalendarService) widgetbindings.WidgetType {
	return &calendarWidgetType{calendarInstanceBacking{svc: svc}}
}

func (w *calendarWidgetType) Slug() string { return WidgetTypeCalendar }
