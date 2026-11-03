// extension_enable.go — the addons-store check the campaigns plugin
// reads without importing the addons plugin.

package campaigns

import "context"

// ExtensionEnableChecker answers whether an addon is on for a campaign,
// for pages that change with it (the Customizer's character-claiming
// rows). Satisfied by addons.AddonService's IsEnabledForCampaign. A nil
// checker reads every addon as off.
type ExtensionEnableChecker interface {
	IsEnabledForCampaign(ctx context.Context, campaignID string, addonSlug string) (bool, error)
}

// SetExtensionEnableChecker wires the addons-store check into the
// campaigns Handler. Matches the SetAddonLister / SetSMTPChecker
// shape; called once at startup.
func (h *Handler) SetExtensionEnableChecker(c ExtensionEnableChecker) {
	h.extensionEnableChecker = c
}
