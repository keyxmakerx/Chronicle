package entities

import "context"

// page_panel.go lets another plugin hang a lazily loaded panel under a
// character's page without the entities plugin knowing which plugin it is.
// The panel sits after the layout (and after any game-system page renderer),
// so a system that takes over the sheet does not lose it. The fragment's route
// decides who actually sees anything; this hook only decides whether to ask.

// PagePanel describes one such panel.
type PagePanel struct {
	// Addon is the addon slug that must be enabled for the campaign.
	Addon string
	// URL builds the fragment route for an entity.
	URL func(campaignID, entityID string) string
}

// SetCharacterPagePanel registers the panel shown on character pages.
func (h *Handler) SetCharacterPagePanel(p PagePanel) { h.characterPanel = &p }

type pagePanelKey struct{}

// withPagePanelURL carries the resolved fragment URL to the show template.
func withPagePanelURL(ctx context.Context, url string) context.Context {
	return context.WithValue(ctx, pagePanelKey{}, url)
}

// pagePanelURL returns the fragment URL for this request, or "".
func pagePanelURL(ctx context.Context) string {
	u, _ := ctx.Value(pagePanelKey{}).(string)
	return u
}

// mayHoldCharacterPanel is a cheap pre-filter on the entity type so ordinary
// pages do not fire a request. It is deliberately generous (a sub-type may be
// part of the character family); the fragment route is the authority and
// answers empty for anything that is not a character.
func mayHoldCharacterPanel(et *EntityType) bool {
	if et == nil {
		return false
	}
	return isClaimableType(et) ||
		isPlayerCharacterType(derefStr(et.PresetCategory), et.Slug) ||
		et.ParentTypeID != nil
}
