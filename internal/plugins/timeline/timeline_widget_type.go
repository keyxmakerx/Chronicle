// timeline_widget_type.go — registers "timeline" with the widget-binding
// framework (C-WIDGET-BINDING-P2-WORLDSTATE-TIMELINE). A timeline's bindable
// instance is a timeline record.
//
// NOTE on the default: the entity-page `BlockTimeline` today renders a
// campaign-level PREVIEW LIST (lazy /timelines/preview), not a single
// timeline. So there is NO single default instance — DefaultInstance returns
// (—, false), and the block keeps its list when unbound (today's behavior).
// A binding makes the block render that one specific timeline instead.
package timeline

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/widgetbindings"
)

// WidgetTypeTimeline is the persisted widget_type discriminator for timelines —
// an append-only namespace value; never rename.
const WidgetTypeTimeline = "timeline"

// isNotFound reports whether err is (or wraps) a 404 AppError. apperror.SafeCode
// misses wrapped errors (GetTimeline wraps not-found via %w), so we use
// errors.As: a genuine not-found is sweepable, anything else is transient and
// must NOT trigger a sweep.
func isNotFound(err error) bool {
	var ae *apperror.AppError
	return errors.As(err, &ae) && ae.Code == http.StatusNotFound
}

type timelineWidgetType struct {
	svc TimelineService
}

// NewTimelineWidgetType builds the timeline WidgetType for registration into
// the widget-binding registry at app startup.
func NewTimelineWidgetType(svc TimelineService) widgetbindings.WidgetType {
	return &timelineWidgetType{svc: svc}
}

func (w *timelineWidgetType) Slug() string { return WidgetTypeTimeline }

// InstanceExists is the orphan guard + campaign-scope security check: a
// timeline validates only if it exists AND belongs to the campaign. Genuine
// not-found → (false, nil) so the dead binding is sweepable; any other error →
// (false, err) so a transient DB blip does NOT sweep (mirror calendar).
func (w *timelineWidgetType) InstanceExists(ctx context.Context, campaignID, instanceID string) (bool, error) {
	t, err := w.svc.GetTimeline(ctx, instanceID)
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, err
	}
	if t == nil {
		return false, nil
	}
	// SECURITY: cross-campaign timelines must not validate.
	return t.CampaignID == campaignID, nil
}

// DefaultInstance: the entity timeline block's unbound behavior is a
// campaign-level preview LIST, not a single timeline — so there is no single
// default instance. Returning ok=false keeps the unbound block on its list
// (today's behavior, identical).
func (w *timelineWidgetType) DefaultInstance(ctx context.Context, host widgetbindings.HostRef) (string, bool, error) {
	return "", false, nil
}

// ListInstances returns the campaign's timelines for the create-or-pick UI
// (C-WIDGET-BINDING-P4b), role-filtered via the service. userID is "" — the
// picker is Scribe-gated at the route, and the role already authorizes the list.
func (w *timelineWidgetType) ListInstances(ctx context.Context, campaignID string, role int) ([]widgetbindings.InstanceRef, error) {
	tls, err := w.svc.ListTimelines(ctx, campaignID, role, "")
	if err != nil {
		return nil, err
	}
	out := make([]widgetbindings.InstanceRef, 0, len(tls))
	for _, t := range tls {
		out = append(out, widgetbindings.InstanceRef{ID: t.ID, Name: t.Name, Icon: t.Icon, Color: t.Color})
	}
	return out, nil
}

// CreateInstance makes a barebones (named) timeline and returns its id — the
// "create new" half of the picker. CreateTimeline applies all the sensible
// defaults (color/icon/visibility/zoom).
func (w *timelineWidgetType) CreateInstance(ctx context.Context, campaignID string, input any) (string, error) {
	name := ""
	if ci, ok := input.(widgetbindings.CreateInput); ok {
		name = strings.TrimSpace(ci.Name)
	}
	t, err := w.svc.CreateTimeline(ctx, campaignID, CreateTimelineInput{CampaignID: campaignID, Name: name})
	if err != nil {
		return "", err
	}
	return t.ID, nil
}

// RenderBlock re-renders the entity timeline block for an in-place HTMX swap
// (C-WIDGET-BINDING-P4b). The timeline-viz mount is a data-widget (boot.js
// re-mounts it on htmx:afterSettle), so the swapped block self-fetches and
// renders cleanly. Wrapped in BlockHost for the stable swap target.
func (w *timelineWidgetType) RenderBlock(ctx context.Context, rc widgetbindings.BlockRenderContext) templ.Component {
	isScribe := rc.Role >= int(campaigns.RoleScribe)
	inner := BlockTimeline(rc.CC, rc.Resolution.InstanceID, rc.HostID, rc.Resolution.Source, isScribe)
	return widgetbindings.BlockHost(WidgetTypeTimeline, rc.HostID, inner)
}
