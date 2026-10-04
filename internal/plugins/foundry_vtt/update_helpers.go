package foundry_vtt

import (
	"encoding/json"
	"fmt"
)

// Where the owner's update screens post and swap. The same actions serve the
// dashboard line and the Foundry page's Module version card; the form says which one to
// answer with so each swaps its own markup.
const (
	updateViewBanner = "banner"
	updateViewRow    = "row"

	// updateBannerID and updateRowID are the swap targets of the two screens.
	updateBannerID = "fvtt-update-line"
	updateRowID    = "fvtt-update-row"
)

// updateActionURL is an owner update action under the campaign.
func updateActionURL(campaignID, action string) string {
	return fmt.Sprintf("/campaigns/%s/foundry-vtt/update%s", campaignID, action)
}

// updateTargetSelector is the hx-target for a view.
func updateTargetSelector(view string) string {
	if view == updateViewRow {
		return "#" + updateRowID
	}
	return "#" + updateBannerID
}

// updateDialogID names a dialog so the two screens never share an id.
func updateDialogID(kind, view string) string {
	return "fvtt-" + kind + "-dlg-" + view
}

// hxCSRFHeaders is the hx-headers value that carries the CSRF token.
func hxCSRFHeaders(token string) string {
	b, err := json.Marshal(map[string]string{"X-CSRF-Token": token})
	if err != nil {
		return "{}"
	}
	return string(b)
}

// foundryPageURL is the owner's Foundry page, where the module version, the
// install link and the setup steps all live.
func foundryPageURL(campaignID string) string {
	return fmt.Sprintf("/campaigns/%s/foundry", campaignID)
}
