// pin_mode.go — pin-mode domain constants + normalization helpers
// added in C-FMC-ADMIN-UX-AUDIT Chunk 1.
//
// Three modes distinguish what AutoPinOnInstall does to each campaign
// when an admin installs a new module version. The choice lives on the
// per-campaign settings JSON alongside the existing
// `foundry_module_pin` key — see CampaignSettings.FoundryModulePinMode
// in the campaigns plugin's model.go.
//
// Chunk 1 (this file) introduces only the storage shape and constants.
// The hook behavior that actually consults pin_mode lands in Chunk 2;
// the UI surfaces in Chunks 3 + 4; the default-flip backfill is
// Chunk 6. Until Chunk 2 ships, AutoPinOnInstall continues to ignore
// pin_mode and behaves per the pre-existing preserve-state design.
//
// Reference: `cordinator/reports/chronicle/2026-05-20-c-fmc-admin-ux-audit.md`
// §0.5 (D5 = (S.2) two JSON keys) + §4 Chunk 1.

package foundry_vtt

// PinMode constants name the three valid values for the
// `foundry_module_pin_mode` settings key. Empty string is reserved
// for "not yet set" — campaigns predating Chunk 6's backfill will
// read empty until the migration runs.
const (
	// PinModePreserve = today's C-FMC-6 behavior on empty-pin
	// campaigns. When admin installs a new module version,
	// AutoPinOnInstall sets the campaign's pin to the *previous*
	// version so the campaign keeps serving what it was already
	// serving. Operator must consciously bump from there.
	PinModePreserve = "preserve"

	// PinModePromote = the always-promote behavior bug #21 reported
	// as the expected default. When admin installs a new module
	// version, AutoPinOnInstall sets the campaign's pin to the
	// *new* version. Audit-resolved 2026-05-20 (D1 = (b)) as the
	// default for new campaigns; Phase 4 / Chunk 6 backfills
	// existing empty-pin campaigns to this mode.
	PinModePromote = "promote"

	// PinModePinned is set implicitly when the campaign has a
	// non-empty `foundry_module_pin` (the version string). The mode
	// key may be stored as `"pinned"` for clarity in admin UIs, but
	// the source of truth is still the pin field: if pin != "",
	// the campaign is pinned regardless of pin_mode.
	PinModePinned = "pinned"
)

// IsValidPinMode reports whether the given string is one of the
// three canonical pin modes. Empty string is NOT valid here — that's
// the "not yet set" pre-backfill state; callers that handle it
// should check separately.
func IsValidPinMode(mode string) bool {
	switch mode {
	case PinModePreserve, PinModePromote, PinModePinned:
		return true
	default:
		return false
	}
}
