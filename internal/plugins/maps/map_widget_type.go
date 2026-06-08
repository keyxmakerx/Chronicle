// map_widget_type.go — registers "map" with the widget-binding framework
// (C-WIDGET-BINDING-P3a-MAPS). Maps is the original precedent: `entity.map_id`
// + AssignMap is the hardcoded per-entity→instance binding the whole framework
// generalizes. This makes maps a first-class WidgetType so a widget_bindings
// row (widget_type="map") takes precedence over the legacy column — while an
// unbound entity still resolves via entity.map_id (the block closure's
// fallback), exactly as today.
package maps

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/widgetbindings"
)

// WidgetTypeMap is the persisted widget_type discriminator for maps — an
// append-only namespace value; never rename.
const WidgetTypeMap = "map"

// isNotFound reports whether err is (or wraps) a 404 AppError. apperror.SafeCode
// misses wrapped errors (GetMap wraps not-found via %w), so we use errors.As: a
// genuine not-found is sweepable, anything else is transient and must NOT
// trigger a sweep (mirror calendar/timeline).
func isNotFound(err error) bool {
	var ae *apperror.AppError
	return errors.As(err, &ae) && ae.Code == http.StatusNotFound
}

type mapWidgetType struct {
	svc MapService
}

// NewMapWidgetType builds the map WidgetType for registration into the
// widget-binding registry at app startup.
func NewMapWidgetType(svc MapService) widgetbindings.WidgetType {
	return &mapWidgetType{svc: svc}
}

func (w *mapWidgetType) Slug() string { return WidgetTypeMap }

// InstanceExists is the orphan guard + campaign-scope security check: a map
// validates only if it exists AND belongs to the campaign. Genuine not-found →
// (false, nil) so the dead binding is sweepable; any other error → (false, err)
// so a transient DB blip does NOT sweep.
func (w *mapWidgetType) InstanceExists(ctx context.Context, campaignID, instanceID string) (bool, error) {
	m, err := w.svc.GetMap(ctx, instanceID)
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, err
	}
	if m == nil {
		return false, nil
	}
	// SECURITY: cross-campaign maps must not validate.
	return m.CampaignID == campaignID, nil
}

// DefaultInstance: maps has NO campaign-level default. The per-entity legacy
// fallback (entity.map_id) is handled in the map_editor block closure (which
// has the entities-plugin context), keeping this WidgetType free of any
// entities dependency. Mirrors P2 timeline's "no single default".
func (w *mapWidgetType) DefaultInstance(ctx context.Context, host widgetbindings.HostRef) (string, bool, error) {
	return "", false, nil
}

// ListInstances returns the campaign's maps for the create-or-pick UI
// (C-WIDGET-BINDING-P4b), replacing the bespoke BlockEntityMapPicker grid.
func (w *mapWidgetType) ListInstances(ctx context.Context, campaignID string, role int) ([]widgetbindings.InstanceRef, error) {
	ms, err := w.svc.ListMaps(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	out := make([]widgetbindings.InstanceRef, 0, len(ms))
	for _, m := range ms {
		out = append(out, widgetbindings.InstanceRef{ID: m.ID, Name: m.Name, Icon: "fa-map-location-dot"})
	}
	return out, nil
}

// CreateInstance makes a barebones (named) map and returns its id — the "create
// new" half of the picker; the user adds the image + markers later in the editor.
func (w *mapWidgetType) CreateInstance(ctx context.Context, campaignID string, input any) (string, error) {
	name := ""
	if ci, ok := input.(widgetbindings.CreateInput); ok {
		name = strings.TrimSpace(ci.Name)
	}
	m, err := w.svc.CreateMap(ctx, CreateMapInput{CampaignID: campaignID, Name: name})
	if err != nil {
		return "", err
	}
	return m.ID, nil
}

// RenderBlock re-renders the map_editor block for an in-place HTMX swap
// (C-WIDGET-BINDING-P4b). This is where the map_editor block's three render
// branches (embed / choose / empty) now live — moved out of the routes.go
// closure so the binding handler can re-render after a bind/unbind. The
// resolved instance is rc.Resolution.InstanceID (the binding, or the legacy
// entity.map_id fallback the closure folds in on first render). Wrapped in
// BlockHost for the stable swap target.
//
// JS NOTE: the embed's Leaflet init is an inline IIFE in MapEditorBody; the
// entity-map data-widget re-mounts on htmx:afterSettle but the inline IIFE does
// not re-run under htmx.config.allowScriptTags=false. So a bind that swaps INTO
// the embed shows the editor chrome immediately and the Leaflet canvas paints on
// the next full load — consistent with hx-boosted navigation today. (Flagged.)
func (w *mapWidgetType) RenderBlock(ctx context.Context, rc widgetbindings.BlockRenderContext) templ.Component {
	return widgetbindings.BlockHost(WidgetTypeMap, rc.HostID, w.renderInner(ctx, rc))
}

func (w *mapWidgetType) renderInner(ctx context.Context, rc widgetbindings.BlockRenderContext) templ.Component {
	if rc.CC == nil || rc.CC.Campaign == nil || rc.HostID == "" {
		return templ.NopComponent
	}
	isScribe := rc.Role >= int(campaigns.RoleScribe)
	mapID := rc.Resolution.InstanceID
	if mapID != "" {
		m, err := w.svc.GetMap(ctx, mapID)
		// SECURITY: only render an in-campaign map (cross-campaign / dangling →
		// fall through to the choose/empty branch rather than leak or 500).
		if err == nil && m != nil && m.CampaignID == rc.CC.Campaign.ID {
			markers, mErr := w.svc.ListMarkers(ctx, m.ID, rc.Role, rc.UserID)
			if mErr != nil {
				slog.Warn("map widget RenderBlock: failed to list markers",
					slog.String("map_id", m.ID), slog.Any("error", mErr))
				markers = nil
			}
			viewData := MapViewData{CampaignID: rc.CC.Campaign.ID, Map: m, Markers: markers, IsScribe: isScribe}
			return BlockEntityMapEmbed(rc.CC, rc.HostID, viewData, isScribe, rc.Resolution.Source)
		}
		if err != nil {
			slog.Warn("map widget RenderBlock: assigned map not found, falling back",
				slog.String("entity_id", rc.HostID), slog.String("map_id", mapID), slog.Any("error", err))
		}
	}
	// No (valid) map: players get the friendly empty state; Scribe+ get the
	// generic create-or-pick affordance (replaces the bespoke picker grid).
	if !isScribe {
		return BlockEntityMapEmpty()
	}
	return BlockEntityMapChoose(rc.CC, rc.HostID, rc.Resolution.Source)
}
