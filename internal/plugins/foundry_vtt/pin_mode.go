// pin_mode.go — pin-mode domain constants and normalization helpers.
//
// Three modes distinguish what AutoPinOnInstall does to each campaign
// when an admin installs a new module version. The choice lives on the
// per-campaign settings JSON alongside the `foundry_module_pin` key —
// see CampaignSettings.FoundryModulePinMode in the campaigns plugin's
// model.go.

package foundry_vtt

import "github.com/keyxmakerx/chronicle/internal/plugins/packages"

// PinMode constants name the three valid values for the
// `foundry_module_pin_mode` settings key. Empty string means "not yet
// set"; AutoPinOnInstall treats it the same as PinModePromote.
const (
	// PinModePreserve: when admin installs a new module version,
	// AutoPinOnInstall sets the campaign's pin to the *previous*
	// version, so the campaign keeps serving what it was already
	// serving. Operator must consciously bump from there.
	PinModePreserve = "preserve"

	// PinModePromote: when admin installs a new module version,
	// AutoPinOnInstall sets the campaign's pin to the *new* version.
	// The default for new campaigns.
	PinModePromote = "promote"

	// PinModePinned is set implicitly when the campaign has a
	// non-empty `foundry_module_pin` (the version string). The mode
	// key may be stored as `"pinned"` for clarity in admin UIs, but
	// the source of truth is still the pin field: if pin != "",
	// the campaign is pinned regardless of pin_mode.
	PinModePinned = "pinned"
)

// PinModeApproveFirst is the "Ask me first" update mode: a newly installed
// version is held for the campaign until its owner (or an admin) approves
// it, and the campaign keeps its pin meanwhile. See CampaignBinding.
const PinModeApproveFirst = "approve_first"

// UpdateModeFromLegacy maps a campaign's stored pin and pin_mode onto the
// shared update modes without changing what the campaign is served:
//
//   - approve_first stays approve_first (the stored choice wins);
//   - any non-empty pin is pinned, whatever pin_mode says;
//   - "preserve" with no pin is pinned (it freezes on the next install);
//   - everything else, including empty, "promote" and a bare "pinned" with
//     no pin (which today still follows the installed version), is automatic.
//
// The returned version is the pin, "" when the campaign has none.
func UpdateModeFromLegacy(pin, pinMode string) (packages.UpdateMode, string) {
	switch {
	case pinMode == PinModeApproveFirst:
		return packages.UpdateModeApproveFirst, pin
	case pin != "":
		return packages.UpdateModePinned, pin
	case pinMode == PinModePreserve:
		return packages.UpdateModePinned, ""
	default:
		return packages.UpdateModeAutomatic, ""
	}
}

// IsValidPinMode reports whether the given string is one of the
// canonical pin modes. Empty string ("not yet set") is NOT
// valid here; callers that handle it should check separately.
func IsValidPinMode(mode string) bool {
	switch mode {
	case PinModePreserve, PinModePromote, PinModePinned, PinModeApproveFirst:
		return true
	default:
		return false
	}
}
