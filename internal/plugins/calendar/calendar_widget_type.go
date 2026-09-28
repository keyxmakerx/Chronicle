// Registers "calendar" and "worldstate" with the widget-binding framework
// (see internal/plugins/timeline/timeline_widget_type.go for the pattern
// this mirrors). Both bind a campaign's calendar as their instance — the
// entity-page "calendar" block and the "worldstate" sky/hourglass embed are
// two independently bindable slots that happen to point at the same kind of
// thing, so they share calendarInstanceBase and differ only in Slug and
// RenderBlock.
package calendar

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/widgetbindings"
	skywidget "github.com/keyxmakerx/chronicle/internal/widgets/sky/templates"
)

// WidgetTypeCalendar and WidgetTypeWorldstate (the persisted widget_type
// discriminators these two types answer to) are declared in calendar.go,
// kept there through the V5 rebuild so existing widget-binding rows never
// orphaned.

// entityCalendarBlockEventLimit is how many upcoming events the bound
// "calendar" block shows — the same default PreviewUpcomingEvents (the
// dashboard/category cards' own fragment) uses.
const entityCalendarBlockEventLimit = 5

// isCalendarNotFound reports whether err is (or wraps) a 404 AppError,
// mirroring timeline.isNotFound: apperror.SafeCode misses wrapped errors, so
// this uses errors.As instead.
func isCalendarNotFound(err error) bool {
	var ae *apperror.AppError
	return errors.As(err, &ae) && ae.Code == http.StatusNotFound
}

// calendarInstanceBase implements the widgetbindings.WidgetType methods that
// "calendar" and "worldstate" share: both bind a calendar as their instance,
// resolved, listed and existence-checked identically. Only Slug and
// RenderBlock differ per type — see calendarWidgetType/worldstateWidgetType
// below.
type calendarInstanceBase struct {
	svc CalendarService
}

// InstanceExists is the orphan guard + campaign-scope security check — see
// widgetbindings.WidgetType's own doc comment. Reads as a trusted,
// system-level Owner viewer (never a request-derived one: this is a binding
// validity check, not a content read, and must see a dm_only calendar's
// existence the same as any other) so only genuine absence or a cross-
// campaign mismatch answers false; the calendar's own visibility is
// re-checked against the REAL viewer at render time, in RenderBlock below.
func (w *calendarInstanceBase) InstanceExists(ctx context.Context, campaignID, instanceID string) (bool, error) {
	_, err := w.svc.GetCalendarForViewer(ctx, instanceID, campaignID, permissions.SystemViewer(permissions.RoleOwner))
	if err != nil {
		if isCalendarNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// DefaultInstance resolves to the campaign's default calendar — today's
// pre-binding behavior for every calendar-driven block (the "skybox"
// dashboard/template block already does the same lookup directly; see
// internal/app/routes.go's renderSkyboxBlock), so an unbound host renders
// identically to before the widget-binding framework existed.
func (w *calendarInstanceBase) DefaultInstance(ctx context.Context, host widgetbindings.HostRef) (string, bool, error) {
	if host.CampaignID == "" {
		return "", false, nil
	}
	cal, err := w.svc.GetDefaultCalendarForViewer(ctx, host.CampaignID, permissions.SystemViewer(permissions.RoleOwner))
	if err != nil {
		if isCalendarNotFound(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return cal.ID, true, nil
}

// ListInstances returns the campaign's calendars for the create-or-pick UI.
// Uses permissions.SystemViewer(role) — the same "trusted, route-gated
// surface" reasoning timelineWidgetType.ListInstances documents: the picker
// route is Scribe+-gated (widgetbindings.RegisterRoutes) and this interface
// carries no per-request user id to build a request viewer from.
func (w *calendarInstanceBase) ListInstances(ctx context.Context, campaignID string, role int) ([]widgetbindings.InstanceRef, error) {
	cals, err := w.svc.ListCalendars(ctx, campaignID, permissions.SystemViewer(role))
	if err != nil {
		return nil, err
	}
	out := make([]widgetbindings.InstanceRef, 0, len(cals))
	for _, c := range cals {
		out = append(out, widgetbindings.InstanceRef{ID: c.ID, Name: c.Name})
	}
	return out, nil
}

// CreateInstance: unlike a timeline (name + sensible defaults), a calendar
// needs a mode, a month/weekday structure and more — the same multi-step
// wizard CreateCalendarFromImport backs (calendar_wizard.templ). The
// generic picker's free-form CreateInput carries none of that, so "create
// new" isn't offered here; ErrNotImplemented tells the picker to fall back
// to "pick an existing calendar", per its own doc comment.
func (w *calendarInstanceBase) CreateInstance(_ context.Context, _ string, _ any) (string, error) {
	return "", widgetbindings.ErrNotImplemented
}

// --- calendar: the entity-page "calendar" block ---

type calendarWidgetType struct{ calendarInstanceBase }

// NewCalendarWidgetType builds the "calendar" WidgetType for registration
// into the widget-binding registry at app startup.
func NewCalendarWidgetType(svc CalendarService) widgetbindings.WidgetType {
	return &calendarWidgetType{calendarInstanceBase{svc: svc}}
}

func (w *calendarWidgetType) Slug() string { return WidgetTypeCalendar }

// RenderBlock re-renders the entity calendar block for an in-place HTMX
// swap. The full ambient calendar + linked-events engine the pre-V5 plugin
// had is real UI work of its own (unbuilt here, see the plugin's .ai.md) —
// this renders the smallest honest version instead: the bound calendar's
// own upcoming-events list, the same content/markup
// PreviewUpcomingEvents/UpcomingEventsEmbed already render for the
// dashboard cards, just scoped to whichever calendar this entity resolves
// to rather than always the campaign default.
func (w *calendarWidgetType) RenderBlock(ctx context.Context, rc widgetbindings.BlockRenderContext) templ.Component {
	return widgetbindings.BlockHost(WidgetTypeCalendar, rc.HostID, w.renderUpcoming(ctx, rc))
}

func (w *calendarWidgetType) renderUpcoming(ctx context.Context, rc widgetbindings.BlockRenderContext) templ.Component {
	if rc.CC == nil || rc.CC.Campaign == nil || !rc.Resolution.Resolved() {
		return UpcomingEventsEmbed(rc.CC, nil, nil)
	}
	campaignID := rc.CC.Campaign.ID
	v := permissions.RequestViewer(rc.Role, rc.UserID)
	cal, err := w.svc.GetCalendarForViewer(ctx, rc.Resolution.InstanceID, campaignID, v)
	if err != nil {
		if isCalendarNotFound(err) {
			return UpcomingEventsEmbed(rc.CC, nil, nil)
		}
		slog.Error("calendar block: get calendar for viewer",
			slog.String("campaign_id", campaignID), slog.Any("error", err))
		return templ.NopComponent
	}
	events, err := w.svc.ListUpcomingEvents(ctx, cal.ID, campaignID, entityCalendarBlockEventLimit, v)
	if err != nil {
		slog.Error("calendar block: list upcoming events",
			slog.String("campaign_id", campaignID), slog.Any("error", err))
		return templ.NopComponent
	}
	return UpcomingEventsEmbed(rc.CC, cal, events)
}

// --- worldstate: the entity-page/dashboard sky + hourglass embed ---

type worldstateWidgetType struct{ calendarInstanceBase }

// NewWorldstateWidgetType builds the "worldstate" WidgetType for
// registration into the widget-binding registry at app startup.
func NewWorldstateWidgetType(svc CalendarService) widgetbindings.WidgetType {
	return &worldstateWidgetType{calendarInstanceBase{svc: svc}}
}

func (w *worldstateWidgetType) Slug() string { return WidgetTypeWorldstate }

// RenderBlock mounts the same sky-pane widget the unbindable "skybox" block
// mounts (internal/app/routes.go's renderSkyboxBlock) — an ambient sky
// (moons, stars, weather, celestial events); the hourglass shelf ornament
// on top of it is real UI work of its own, not built here (see the
// plugin's .ai.md). What "worldstate" adds over skybox is bindability: a
// specific entity can point this block at a calendar other than the
// campaign default. The campaign dashboard hosts this block with no entity
// (widgetbindings.HostTypeDashboard is not yet wired into
// internal/app/routes.go's renderBoundBlock), so a dashboard render always
// arrives here unresolved and falls back to the campaign default, same as
// skybox today.
func (w *worldstateWidgetType) RenderBlock(ctx context.Context, rc widgetbindings.BlockRenderContext) templ.Component {
	return widgetbindings.BlockHost(WidgetTypeWorldstate, rc.HostID, w.renderSky(ctx, rc))
}

func (w *worldstateWidgetType) renderSky(ctx context.Context, rc widgetbindings.BlockRenderContext) templ.Component {
	if rc.CC == nil || rc.CC.Campaign == nil {
		return templ.NopComponent
	}
	campaignID := rc.CC.Campaign.ID
	v := permissions.RequestViewer(rc.Role, rc.UserID)

	var cal *Calendar
	var err error
	if rc.Resolution.Resolved() {
		cal, err = w.svc.GetCalendarForViewer(ctx, rc.Resolution.InstanceID, campaignID, v)
	} else {
		cal, err = w.svc.GetDefaultCalendarForViewer(ctx, campaignID, v)
	}
	if err != nil {
		if isCalendarNotFound(err) {
			return skywidget.Empty(campaignID)
		}
		slog.Error("worldstate block: get calendar for viewer",
			slog.String("campaign_id", campaignID), slog.Any("error", err))
		return templ.NopComponent
	}
	return skywidget.Mount(campaignID, cal.ID)
}
