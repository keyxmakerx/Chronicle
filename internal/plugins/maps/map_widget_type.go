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
	"net/http"

	"github.com/keyxmakerx/chronicle/internal/apperror"
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

// ListInstances / CreateInstance power the P4 create-or-pick UI. Maps already
// has a working picker via AssignMap; P4 generalizes it onto the registry.
func (w *mapWidgetType) ListInstances(ctx context.Context, campaignID string, role int) ([]widgetbindings.InstanceRef, error) {
	return nil, widgetbindings.ErrNotImplemented
}

func (w *mapWidgetType) CreateInstance(ctx context.Context, campaignID string, input any) (string, error) {
	return "", widgetbindings.ErrNotImplemented
}
