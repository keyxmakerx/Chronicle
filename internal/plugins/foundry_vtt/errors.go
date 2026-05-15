// Package foundry_vtt is the Foundry-VTT-specific sub-plugin that
// extends the generic packages plugin via the PostInstallHook
// extension point (added in C-FMC-5a). It owns every Foundry-specific
// behavior Chronicle exposes: per-campaign signed manifest URLs,
// per-campaign pinning, the chronicle-package.json descriptor reader,
// and the post-install module.json version rewrite.
//
// The packages plugin remains generic — it has zero Foundry-specific
// knowledge. foundry_vtt is the only place "manifest", "download",
// "module.json", and "chronicle-package.json" appear.
//
// See .ai.md in this directory for the full architecture, including
// the decision log explaining why a sub-plugin (not a packages
// extension) and why descriptor-driven (not hardcoded).
package foundry_vtt

import (
	"errors"
	"fmt"
	"net/http"
)

// ErrCategory classifies a foundry_vtt failure so the handler can pick
// the right HTTP status code and the public manifest endpoint can
// emit a Foundry-side-parseable JSON body. The Foundry module's
// update-check UI (FM-CSU-DIAG) consumes the category field to pick
// a visual cue (red/yellow/blue) without parsing the message.
//
// Categories are deliberately coarse — operators don't need to
// distinguish 20 failure modes, they need to know whether to fix
// config, fix auth, install a missing version, or escalate to the
// platform team.
type ErrCategory string

const (
	// ErrCategoryConfig — the operator needs to take a config action
	// (install a missing package version, configure the foundry-module
	// package, etc.). 503 Service Unavailable so Foundry's update
	// check distinguishes this from "endpoint broken" (404) or
	// "you're not allowed" (403).
	ErrCategoryConfig ErrCategory = "config"

	// ErrCategoryAuth — token verification failed (wrong HMAC,
	// rotated token, or malformed). 403 Forbidden. The owner needs
	// to rotate their install URL and reinstall in Foundry.
	ErrCategoryAuth ErrCategory = "auth"

	// ErrCategoryNotFound — a referenced entity doesn't exist
	// (campaign ID unknown, version not in catalog). 404. Distinct
	// from Auth because the fix is different — admin action, not
	// owner action.
	ErrCategoryNotFound ErrCategory = "not_found"

	// ErrCategoryValidation — request body / params are malformed
	// (invalid version string, missing field). 422 Unprocessable
	// Entity. Almost always a Foundry-side bug; surfaced for debug.
	ErrCategoryValidation ErrCategory = "validation"

	// ErrCategoryInternal — server-side fault (DB error, FS error,
	// JSON parse). 500. The operator's logs have the underlying
	// cause; the Foundry-side message is intentionally generic.
	ErrCategoryInternal ErrCategory = "internal"
)

// Error is the foundry_vtt-typed failure. Carries enough context for
// both the categorized HTTP response (status, JSON body) and the
// operator's logs (wrapped cause). The format of Message is the
// operator's explicit acceptance criteria:
//
//	<what failed>: <technical detail>. <likely cause>. <actionable next step>.
//
// See examples in the .ai.md "Error message contract" section.
type Error struct {
	Category ErrCategory
	// Code is a machine-readable identifier for the failure shape.
	// Stable across versions (Foundry's FM-CSU-DIAG keys off this).
	// Example: "invalid_token", "no_package_registered",
	// "pinned_version_not_installed".
	Code string
	// Message is the human-readable explanation. Must follow the
	// four-clause format above. Renders directly in Foundry's
	// update-check dialog — write it for the operator, not the
	// engineer.
	Message string
	// Cause is the underlying error (DB error, FS error, JSON
	// parse). Never returned to clients; included in server logs
	// for the operator's investigation.
	Cause error
}

// Error implements the error interface. Includes the cause if
// present so log lines have the full chain. The client-facing
// Message is the only thing surfaced over HTTP.
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s/%s] %s: %v", e.Category, e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s/%s] %s", e.Category, e.Code, e.Message)
}

// Unwrap returns the underlying cause for errors.Is / errors.As.
func (e *Error) Unwrap() error { return e.Cause }

// HTTPStatus maps the category to the HTTP status the handler emits.
// Kept here (not in the handler) so the mapping is the single source
// of truth and a category-to-status change is a one-line edit.
func (e *Error) HTTPStatus() int {
	switch e.Category {
	case ErrCategoryConfig:
		return http.StatusServiceUnavailable
	case ErrCategoryAuth:
		return http.StatusForbidden
	case ErrCategoryNotFound:
		return http.StatusNotFound
	case ErrCategoryValidation:
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

// AsError extracts the *Error from an error chain, returning nil if
// the error isn't a foundry_vtt typed error. Handlers use this to
// pick between the categorized JSON body shape and the generic
// apperror path (for non-foundry_vtt errors that bubble up).
func AsError(err error) *Error {
	var fe *Error
	if errors.As(err, &fe) {
		return fe
	}
	return nil
}

// --- pre-defined error constructors (the four-clause format is enforced here) ---

// ErrInvalidToken — public manifest hit with a token that fails HMAC
// or whose embedded token_version doesn't match the DB. 403.
func ErrInvalidToken(cause error) *Error {
	return &Error{
		Category: ErrCategoryAuth,
		Code:     "invalid_token",
		Message: "Foundry manifest URL has an invalid or rotated token: " +
			"the HMAC signature failed verification or the token has been " +
			"rotated since this URL was issued. " +
			"The campaign owner has rotated the install URL or the URL was " +
			"copied incorrectly. " +
			"Open the campaign settings → VTT Setup Guides in Chronicle, copy " +
			"the current install URL, and reinstall the module in Foundry.",
		Cause: cause,
	}
}

// ErrNoPackageRegistered — manifest endpoint hit but no foundry-module
// typed package exists in the packages catalog. 503.
func ErrNoPackageRegistered() *Error {
	return &Error{
		Category: ErrCategoryConfig,
		Code:     "no_package_registered",
		Message: "No Foundry VTT module package is registered on this " +
			"Chronicle instance: the manifest endpoint has nothing to serve. " +
			"An admin has not yet added the Chronicle Foundry module package. " +
			"An admin must add the keyxmakerx/Chronicle-Foundry-Module repo via " +
			"/admin/packages, approve it, and install a version before Foundry " +
			"can install modules from this Chronicle instance.",
	}
}

// ErrPinnedVersionNotInstalled — campaign is pinned to a specific
// version, but the packages plugin has no on-disk install dir for
// that version. 503. Common after an admin uninstalls an older
// version that some campaigns still pin to.
func ErrPinnedVersionNotInstalled(version string) *Error {
	return &Error{
		Category: ErrCategoryConfig,
		Code:     "pinned_version_not_installed",
		Message: fmt.Sprintf(
			"Campaign is pinned to Foundry module version %q but that version " +
				"is not installed on this Chronicle instance: the manifest endpoint " +
				"has no on-disk module.json to serve for the pinned version. " +
				"An admin has uninstalled %q or it was never installed. " +
				"An admin must install %q via /admin/packages → Chronicle-Foundry-Module → " +
				"Versions, or the campaign owner must change the pin to a different " +
				"version in campaign settings → VTT Setup Guides.",
			version, version, version),
	}
}

// ErrNoVersionAvailable — manifest endpoint hit but the foundry-module
// package has nothing installed (no version on disk, no fallback). 503.
func ErrNoVersionAvailable() *Error {
	return &Error{
		Category: ErrCategoryConfig,
		Code:     "no_version_available",
		Message: "No Foundry VTT module version is installed on this Chronicle " +
			"instance: the foundry-module package exists in the catalog but no " +
			"version has been installed yet. " +
			"An admin added the package but hasn't completed an install. " +
			"An admin must click Install on a version via /admin/packages → " +
			"Chronicle-Foundry-Module → Versions before Foundry can fetch the manifest.",
	}
}

// ErrTokenNotInitialized — campaign exists, manifest token has never
// been minted (no row in foundry_module_campaign_tokens). 503.
func ErrTokenNotInitialized() *Error {
	return &Error{
		Category: ErrCategoryConfig,
		Code:     "token_not_initialized",
		Message: "Foundry VTT install token has not been generated for this " +
			"campaign yet: the manifest endpoint has no token version to verify against. " +
			"The campaign owner has not visited the VTT Setup Guides section, which " +
			"is where the install URL is lazily minted on first load. " +
			"The campaign owner must visit campaign settings → VTT Setup Guides to " +
			"initialize the install URL, then copy it into Foundry.",
	}
}

// ErrDescriptorInvalid — chronicle-package.json was present in the
// installed zip but failed schema validation. Hook returns this to
// fail the install loudly (per the C-FMC-5a fail-loud contract).
func ErrDescriptorInvalid(cause error) *Error {
	return &Error{
		Category: ErrCategoryValidation,
		Code:     "descriptor_invalid",
		Message: fmt.Sprintf(
			"chronicle-package.json in the installed module is invalid: %v. " +
				"The module's descriptor file does not conform to schema v1 " +
				"(see chronicle.bnuuy.haus/schemas/foundry-package.v1.json). " +
				"Remove the descriptor file from the module zip to fall back to " +
				"hardcoded defaults, or fix the schema violation in the upstream " +
				"module repo and re-cut the release.",
			cause),
		Cause: cause,
	}
}

// ErrModuleJSONMissing — the install dir exists but module.json
// isn't at the descriptor-specified path. Hook returns this; rare
// (validation should have caught it earlier in the packages pipeline).
func ErrModuleJSONMissing(path string, cause error) *Error {
	return &Error{
		Category: ErrCategoryInternal,
		Code:     "module_json_missing",
		Message: fmt.Sprintf(
			"Foundry module.json was not found at %s after install: " +
				"the chronicle-package.json descriptor specifies this path but " +
				"the file does not exist on disk. " +
				"The upstream module's release zip is missing module.json at the " +
				"declared moduleJsonPath, or the zip extraction was interrupted. " +
				"An admin should retry the install via /admin/packages → " +
				"Chronicle-Foundry-Module → Versions. If the failure persists, " +
				"the upstream release zip is malformed and needs an upstream fix.",
			path),
		Cause: cause,
	}
}

// ErrCampaignNotFound — campaign ID in the URL doesn't exist. 404.
func ErrCampaignNotFound(campaignID string) *Error {
	return &Error{
		Category: ErrCategoryNotFound,
		Code:     "campaign_not_found",
		Message: fmt.Sprintf(
			"Campaign %q does not exist on this Chronicle instance: " +
				"the manifest endpoint has nothing to look up settings for. " +
				"The campaign was deleted or the install URL was copied from a " +
				"different Chronicle instance. " +
				"The user who created this install URL must replace it with one " +
				"minted by the current campaign owner from their campaign settings.",
			campaignID),
	}
}

// ErrInternal wraps a server-side fault as a foundry_vtt error so it
// flows through the same JSON-response path. The Foundry-side message
// is intentionally generic (we don't leak internals); the cause is
// logged.
func ErrInternal(code string, cause error) *Error {
	return &Error{
		Category: ErrCategoryInternal,
		Code:     code,
		Message: "An internal Chronicle error occurred while serving the Foundry " +
			"VTT manifest. The Chronicle operator's server logs contain the " +
			"underlying cause. Report this with the request timestamp to the " +
			"Chronicle operator; if you ARE the operator, the cause is in the " +
			"slog output around this request.",
		Cause: cause,
	}
}
