package entities

import "context"

// page_panel.go lets another plugin serve a lazily loaded panel for a
// character's page without the entities plugin knowing which plugin it is.
// It is the "Items & Money" block, and sits on its own after the layout only
// where page_extras.go says so. The fragment's route decides who actually
// sees anything; this hook only decides whether to ask.

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

// mayHoldCharacterPanel is the generous character-family pre-filter for pages
// a game-system renderer owns, which have no layout to place the panel in (a
// sub-type may be part of the character family); the fragment route is the
// authority and answers empty for anything that is not a character.
func mayHoldCharacterPanel(et *EntityType) bool {
	if et == nil {
		return false
	}
	return isClaimableType(et) ||
		isPlayerCharacterType(derefStr(et.PresetCategory), et.Slug) ||
		et.ParentTypeID != nil
}

// ShowedCharacterPanel reports whether this type's pages drew the panel on
// their own before it became a block, so it can be placed where it was.
func ShowedCharacterPanel(et *EntityType) bool { return mayHoldCharacterPanel(et) }
