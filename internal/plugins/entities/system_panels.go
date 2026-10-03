package entities

import "context"

// system_panels.go lets a game-system package hang widget panels under a page's
// title without entities knowing which system, or which page types qualify:
// the app layer owns both decisions and hands back the finished list. The
// panel sits in the title block (and above a page renderer's output), so it is
// there whether the page uses the layout or a system renderer.

// SystemPanel is one widget mount resolved for a page.
type SystemPanel struct {
	// Widget is the slug of a widget the system's manifest declares.
	Widget string
	// SystemID is the owning system, passed to the widget so it addresses its
	// own state without guessing.
	SystemID string
}

// SystemPanelResolver returns the panels that apply to a page of this type in
// this campaign: the enabled system's declared panels, filtered by page type.
// It must answer quickly and return nil when nothing applies; it runs on every
// page view.
type SystemPanelResolver func(ctx context.Context, campaignID string, entityType *EntityType) []SystemPanel

// SetSystemPanelResolver registers the resolver the show page consults.
func (h *Handler) SetSystemPanelResolver(r SystemPanelResolver) { h.systemPanels = r }

type systemPanelsKey struct{}

// withSystemPanels carries the resolved panels to the show template.
func withSystemPanels(ctx context.Context, panels []SystemPanel) context.Context {
	return context.WithValue(ctx, systemPanelsKey{}, panels)
}

// systemPanelsFrom returns the panels resolved for this request, or nil.
func systemPanelsFrom(ctx context.Context) []SystemPanel {
	p, _ := ctx.Value(systemPanelsKey{}).([]SystemPanel)
	return p
}
