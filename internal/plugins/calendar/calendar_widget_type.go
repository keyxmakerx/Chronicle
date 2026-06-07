// calendar_widget_type.go — registers "calendar" with the widget-binding
// framework (C-WIDGET-BINDING-P1-SPINE, retrofit step). The calendar plugin
// declares its behavior to the registry; the binding service drives resolution
// through this without importing calendar internals.
//
// The DEFAULT instance is the campaign's default calendar (svc.GetCalendar) —
// exactly what `entity_calendar` resolved before the framework — so an unbound
// host renders identically (zero churn for #411–#420).
package calendar

import (
	"context"
	"net/http"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/widgetbindings"
)

// WidgetTypeCalendar is the persisted widget_type discriminator for calendars.
const WidgetTypeCalendar = "calendar"

type calendarWidgetType struct {
	svc CalendarService
}

// NewCalendarWidgetType builds the calendar WidgetType for registration into
// the widget-binding registry at app startup.
func NewCalendarWidgetType(svc CalendarService) widgetbindings.WidgetType {
	return &calendarWidgetType{svc: svc}
}

func (w *calendarWidgetType) Slug() string { return WidgetTypeCalendar }

// InstanceExists is both the orphan guard and the campaign-scope security
// check: a calendar instance validates only if it exists AND belongs to the
// campaign. A genuine not-found returns (false, nil) so the service sweeps the
// dead binding; any other error returns (false, err) so a transient DB blip
// does NOT delete the binding (precedent refinement #1: don't sweep on a blip).
func (w *calendarWidgetType) InstanceExists(ctx context.Context, campaignID, instanceID string) (bool, error) {
	cal, err := w.svc.GetCalendarByID(ctx, instanceID)
	if err != nil {
		if apperror.SafeCode(err) == http.StatusNotFound {
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
// pre-framework behavior for the entity calendar block.
func (w *calendarWidgetType) DefaultInstance(ctx context.Context, host widgetbindings.HostRef) (string, bool, error) {
	cal, err := w.svc.GetCalendar(ctx, host.CampaignID)
	if err != nil {
		if apperror.SafeCode(err) == http.StatusNotFound {
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
func (w *calendarWidgetType) ListInstances(ctx context.Context, campaignID string, role int) ([]widgetbindings.InstanceRef, error) {
	return nil, widgetbindings.ErrNotImplemented
}

func (w *calendarWidgetType) CreateInstance(ctx context.Context, campaignID string, input any) (string, error) {
	return "", widgetbindings.ErrNotImplemented
}
