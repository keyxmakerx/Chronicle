package foundry_vtt

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/packages"
)

// AutoPinHook implements packages.PostInstallHook for the C-FMC-6
// auto-pin-on-install behavior. Registered as a SECOND hook
// alongside the existing version-rewrite hook from C-FMC-5b —
// separation of concerns:
//
//   - PostInstallHook (C-FMC-5b): on-disk state. Reads
//     chronicle-package.json, rewrites module.json's version field.
//   - AutoPinHook (C-FMC-6): DB state. Pins auto-tracking campaigns
//     to the previous version so admin sees the version spread
//     instead of silently bumping every campaign on install.
//
// The two hooks are independent. Either one failing fails the
// install (per the C-FMC-5a fail-loud contract). Running order is
// registration order — see routes.go.
type AutoPinHook struct {
	svc Service
}

// NewAutoPinHook constructs the hook. Takes the service so it can
// call AutoPinOnInstall (which holds the repo + settings + events
// dependencies).
func NewAutoPinHook(svc Service) *AutoPinHook {
	return &AutoPinHook{svc: svc}
}

// PackageType identifies which package type this hook fires on.
func (h *AutoPinHook) PackageType() packages.PackageType {
	return packages.PackageTypeFoundryModule
}

// AfterInstall is the C-FMC-6 auto-pin step. Pins every auto-tracking
// campaign to the previousVersion so they stay on the version they
// were effectively running before the admin's install changed
// InstalledVersion. The admin sees the version spread via the
// C-FMC-5c "Campaigns Using v0.X.Y" admin UI and can per-campaign
// bump from there.
//
// Errors fail the install loudly (matches C-FMC-5a contract). A
// post-install failure leaves the operator's catalog state pristine
// (destDir cleaned by packages.InstallVersion) and surfaces the
// problem instead of producing a half-applied state.
//
// actor* are recorded in the security_events rows so the audit trail
// shows which admin triggered the install. Currently always empty
// strings — the install is triggered from an admin handler that
// doesn't pass actor info through to the hook context yet. A future
// refinement could thread these through via the package install
// path (out of scope for C-FMC-6).
func (h *AutoPinHook) AfterInstall(ctx context.Context, pkg *packages.Package, version, previousVersion, destDir string) error {
	_ = pkg     // not needed; service infers the foundry-module package itself
	_ = destDir // on-disk path is the PostInstallHook's domain
	_, err := h.svc.AutoPinOnInstall(ctx, previousVersion, version, "", "", "")
	return err
}
