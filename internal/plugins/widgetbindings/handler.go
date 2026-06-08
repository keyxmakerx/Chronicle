// handler.go — the binding HTTP surface (C-WIDGET-BINDING-P4a). This is the
// ONLY Echo-aware code in the widgetbindings plugin; the Service stays
// Echo-free. Handlers are thin: collect inputs, call the Service, render the
// generic picker fragment (or signal a reload after a mutation).
//
// SECURITY: the host's campaign ALWAYS comes from the route's campaign context
// (never the request body); the Service re-checks the instance is in-campaign
// (no DB FK to lean on). host_type/widget_type are validated against the
// app-code namespace (registry + IsValidHostType), never a DB enum.
package widgetbindings

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Handler serves the create-or-pick binding UI + bind/create/unbind mutations.
type Handler struct {
	svc      Service
	registry *Registry
}

// NewHandler builds the binding handler over the service + widget-type registry.
func NewHandler(svc Service, registry *Registry) *Handler {
	return &Handler{svc: svc, registry: registry}
}

// pickerData is the generic picker fragment's view model.
type pickerData struct {
	CampaignID   string
	HostType     string
	HostID       string
	EntityTypeID string
	WidgetType   string
	WidgetLabel  string
	Instances    []InstanceRef
	CurrentID    string // the resolved instance (highlight)
	Source       string // own | entity_type | default | none
	CSRFToken    string
}

// hostFrom builds a campaign-scoped HostRef from the route context + the given
// host fields. CampaignID is authoritative from the route (never the body).
func hostFrom(cc *campaigns.CampaignContext, hostType, hostID, entityTypeID string) HostRef {
	return HostRef{CampaignID: cc.Campaign.ID, Type: hostType, ID: hostID, EntityTypeID: entityTypeID}
}

// PickerAPI renders the picker fragment (Scribe+):
// GET /campaigns/:id/bindings/picker?host_type=&host_id=&widget_type=&entity_type_id=
func (h *Handler) PickerAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil || cc.Campaign == nil {
		return apperror.NewMissingContext()
	}
	hostType := c.QueryParam("host_type")
	hostID := c.QueryParam("host_id")
	widgetType := c.QueryParam("widget_type")
	entityTypeID := c.QueryParam("entity_type_id")

	wt, ok := h.validate(hostType, hostID, widgetType)
	if !ok {
		return apperror.NewBadRequest("unknown host or widget type")
	}

	ctx := c.Request().Context()
	role := cc.VisibilityRole()
	instances, err := wt.ListInstances(ctx, cc.Campaign.ID, role)
	if err != nil {
		// A widget type without a picker yet (ErrNotImplemented) → empty list,
		// not a 500; the fragment still renders create/default controls.
		instances = nil
	}

	host := hostFrom(cc, hostType, hostID, entityTypeID)
	res, _ := h.svc.Resolve(ctx, host, widgetType)

	return middleware.Render(c, http.StatusOK, bindingPicker(pickerData{
		CampaignID:   cc.Campaign.ID,
		HostType:     hostType,
		HostID:       hostID,
		EntityTypeID: entityTypeID,
		WidgetType:   widgetType,
		WidgetLabel:  widgetLabel(widgetType),
		Instances:    instances,
		CurrentID:    res.InstanceID,
		Source:       res.Source,
		CSRFToken:    middleware.GetCSRFToken(c),
	}))
}

// BindAPI binds a host to an existing instance (Scribe+):
// POST /campaigns/:id/bindings {host_type, host_id, widget_type, instance_id}
func (h *Handler) BindAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil || cc.Campaign == nil {
		return apperror.NewMissingContext()
	}
	hostType := c.FormValue("host_type")
	hostID := c.FormValue("host_id")
	widgetType := c.FormValue("widget_type")
	instanceID := c.FormValue("instance_id")
	if _, ok := h.validate(hostType, hostID, widgetType); !ok {
		return apperror.NewBadRequest("unknown host or widget type")
	}
	host := hostFrom(cc, hostType, hostID, c.FormValue("entity_type_id"))
	if err := h.svc.Bind(c.Request().Context(), host, widgetType, instanceID); err != nil {
		return err
	}
	return reloadHost(c)
}

// CreateBindAPI creates a new instance then binds it (Scribe+):
// POST /campaigns/:id/bindings/create {host_type, host_id, widget_type, name, ...}
func (h *Handler) CreateBindAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil || cc.Campaign == nil {
		return apperror.NewMissingContext()
	}
	hostType := c.FormValue("host_type")
	hostID := c.FormValue("host_id")
	widgetType := c.FormValue("widget_type")
	wt, ok := h.validate(hostType, hostID, widgetType)
	if !ok {
		return apperror.NewBadRequest("unknown host or widget type")
	}
	ctx := c.Request().Context()
	instanceID, err := wt.CreateInstance(ctx, cc.Campaign.ID, CreateInput{Name: c.FormValue("name")})
	if err != nil {
		return err
	}
	host := hostFrom(cc, hostType, hostID, c.FormValue("entity_type_id"))
	if err := h.svc.Bind(ctx, host, widgetType, instanceID); err != nil {
		return err
	}
	return reloadHost(c)
}

// UnbindAPI removes a host's binding, reverting to the default (Scribe+):
// DELETE /campaigns/:id/bindings?host_type=&host_id=&widget_type=
func (h *Handler) UnbindAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil || cc.Campaign == nil {
		return apperror.NewMissingContext()
	}
	hostType := c.QueryParam("host_type")
	hostID := c.QueryParam("host_id")
	widgetType := c.QueryParam("widget_type")
	if _, ok := h.validate(hostType, hostID, widgetType); !ok {
		return apperror.NewBadRequest("unknown host or widget type")
	}
	host := hostFrom(cc, hostType, hostID, "")
	if err := h.svc.Unbind(c.Request().Context(), host, widgetType); err != nil {
		return err
	}
	return reloadHost(c)
}

// validate checks the host_type + widget_type against the app-code namespaces
// (no DB enum) and returns the resolved WidgetType.
func (h *Handler) validate(hostType, hostID, widgetType string) (WidgetType, bool) {
	if hostID == "" || !IsValidHostType(hostType) {
		return nil, false
	}
	wt, ok := h.registry.Get(widgetType)
	return wt, ok
}

// reloadHost signals the client to reload after a successful bind/create/unbind
// so the host's widget block re-renders against the new binding. This mirrors
// the established map-picker UX (AssignMap → full reload) and keeps this plugin
// free of any cross-plugin block-render coupling (it can't import calendar to
// re-render the entity_calendar block).
func reloadHost(c echo.Context) error {
	c.Response().Header().Set("HX-Refresh", "true")
	return c.NoContent(http.StatusOK)
}

// widgetLabel is a small display label for the picker header.
func widgetLabel(slug string) string {
	switch slug {
	case "calendar":
		return "calendar"
	case "worldstate":
		return "world-state calendar"
	case "timeline":
		return "timeline"
	case "map":
		return "map"
	default:
		return slug
	}
}
