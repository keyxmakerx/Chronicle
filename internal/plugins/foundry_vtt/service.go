package foundry_vtt

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/packages"
)

// CampaignSettingsAdapter is the narrow contract foundry_vtt needs from
// the campaigns plugin: read/write FoundryModulePin on CampaignSettings,
// and check whether a campaign exists. Implemented by an adapter in
// routes.go that wraps the campaigns service — defined here, not in
// campaigns, because the campaigns plugin must not import foundry_vtt.
type CampaignSettingsAdapter interface {
	// GetFoundryModulePin returns the campaign's current pin
	// string. Empty string = latest-tracking (auto-resolve to the
	// active install).
	GetFoundryModulePin(ctx context.Context, campaignID string) (string, error)

	// SetFoundryModulePin writes the campaign's pin. Empty string
	// clears the pin (back to latest-tracking).
	SetFoundryModulePin(ctx context.Context, campaignID, version string) error

	// GetFoundryModulePinMode returns the campaign's pin_mode setting
	// — one of foundry_vtt.PinMode* constants, or empty string for
	// "not yet set."
	GetFoundryModulePinMode(ctx context.Context, campaignID string) (string, error)

	// SetFoundryModulePinMode writes the campaign's pin_mode.
	// Caller is responsible for validation via
	// foundry_vtt.IsValidPinMode.
	SetFoundryModulePinMode(ctx context.Context, campaignID, mode string) error

	// CampaignExists is the existence check the rotate-token and
	// install-URL builders use to reject unknown campaign IDs.
	CampaignExists(ctx context.Context, campaignID string) (bool, error)
}

// PackageReader is the narrow contract foundry_vtt needs from the
// packages plugin, wired directly (packages doesn't import foundry_vtt).
type PackageReader interface {
	// ListPackages enumerates the catalog. foundry_vtt filters by
	// PackageTypeFoundryModule and picks the first installed match.
	ListPackages(ctx context.Context) ([]packages.Package, error)

	// GetPackage returns a single package by ID, or an error if not
	// found. Used by the admin per-row fragment handler.
	GetPackage(ctx context.Context, id string) (*packages.Package, error)

	// InstallDirForVersion returns the on-disk path for a specific
	// version of a package. Doesn't check existence; foundry_vtt
	// stats the returned path to confirm the version is actually
	// installed on disk.
	InstallDirForVersion(pkgType packages.PackageType, slug, version string) string

	// ListVersions enumerates every version known to the packages
	// plugin for a given packageID (installed + uninstalled). The
	// admin's "campaigns using version X" UI iterates this list to
	// know which version cards to render.
	ListVersions(ctx context.Context, packageID string) ([]packages.PackageVersion, error)
}

// SecurityEventLogger writes one row per admin action to the audit log
// table. Same shape as admin's SecurityService.LogEvent; passed as an
// interface so the plugin doesn't import admin directly.
type SecurityEventLogger interface {
	LogEvent(ctx context.Context, eventType, userID, actorID, ip, userAgent string, details map[string]any) error
}

// MailNotifier is the SMTP boundary used by the notify-older-campaigns
// admin action. Same shape as smtp.MailService. The service treats
// IsConfigured=false as a soft fallback: the in-app banner (via
// security_events) is the primary surface, email is a courtesy.
type MailNotifier interface {
	IsConfigured(ctx context.Context) bool
	SendMail(ctx context.Context, to []string, subject, body string) error
}

// CampaignOwnerLookup resolves a campaign's owner email so the
// notify action knows where to send. Implemented by an adapter in
// routes.go that wraps campaigns.CampaignService + auth.AuthService.
// Best-effort — a non-nil error skips the email and logs only the
// security event.
type CampaignOwnerLookup interface {
	GetCampaignOwnerEmail(ctx context.Context, campaignID string) (email, displayName string, err error)
}

// Service is the foundry_vtt plugin's business-logic surface.
// Kept narrow — this plugin has fewer responsibilities than
// foundry_modules because catalog/upload/poller all live in the
// generic packages plugin now.
type Service interface {
	// --- per-campaign URL minting ---

	// BuildInstallURL returns the current per-campaign manifest
	// URL, lazily minting a token (version=1) if none exists yet.
	BuildInstallURL(ctx context.Context, campaignID string) (string, error)

	// RotateCampaignToken bumps the token version (invalidating
	// every previously-issued URL for this campaign) and returns
	// the new install URL.
	RotateCampaignToken(ctx context.Context, campaignID string) (newURL string, err error)

	// VerifyManifestToken validates that the token on a manifest
	// request matches the campaign's current token version.
	VerifyManifestToken(ctx context.Context, campaignID, token string) error

	// --- public manifest endpoint ---

	// BuildManifestForCampaign reads the on-disk module.json for the
	// campaign's resolved version, rewrites the descriptor-declared
	// fields with per-campaign signed Chronicle URLs, and returns the
	// JSON bytes plus the version's install directory. Returns *Error
	// on failure so the handler can pick the right HTTP status.
	BuildManifestForCampaign(ctx context.Context, campaignID string) (manifestJSON json.RawMessage, downloadDir string, err error)

	// BuildDownloadParams returns the params needed to stream a
	// per-campaign rewritten zip from the download endpoint. See
	// resolveCampaignManifest for the shared resolution logic.
	BuildDownloadParams(ctx context.Context, campaignID string) (DownloadParams, error)

	// --- owner pin management ---

	// SetPinnedVersion writes the campaign's pin. Empty version
	// clears the pin (latest-tracking).
	SetPinnedVersion(ctx context.Context, campaignID, version string) error

	// GetCampaignPin returns the campaign's saved pin string
	// ("" if unpinned). Thin pass-through to the adapter.
	GetCampaignPin(ctx context.Context, campaignID string) (string, error)

	// --- owner tab fragment data ---

	// OwnerTabData assembles the OwnerTabData struct for the owner
	// settings tab fragment. Empty-state cases (no package registered,
	// no version installed) are flags on the struct, not errors.
	OwnerTabData(ctx context.Context, campaignID string) (OwnerTabData, error)

	// GetBannerStatus reports whether the campaign should see the
	// "newer version available" dashboard banner. Best-effort UX: any
	// lookup failure yields HasUpdate=false rather than an error.
	GetBannerStatus(ctx context.Context, campaignID string) (BannerStatus, error)

	// --- admin operations ---

	// FindFoundryPackage looks up the foundry-module typed package
	// row. Returns (nil, nil) if no foundry-module package is
	// registered.
	FindFoundryPackage(ctx context.Context) (*packages.Package, error)

	// GetPackageByID looks up a single package by ID. Thin wrapper
	// around packages.GetPackage kept on the Service surface so
	// handlers don't need a separate PackageReader handle.
	GetPackageByID(ctx context.Context, id string) (*packages.Package, error)

	// CampaignsUsingVersion returns the campaigns currently pinned to a
	// specific Foundry module version.
	CampaignsUsingVersion(ctx context.Context, version string) ([]CampaignUsage, error)

	// AutoTrackingCampaigns returns campaigns with no explicit
	// foundry_module_pin — they auto-follow the newest module version.
	AutoTrackingCampaigns(ctx context.Context) ([]CampaignUsage, error)

	// ForcePinCampaign mutates a campaign's FoundryModulePin directly,
	// bypassing the owner-side flow. Validates the target version exists
	// on disk and logs a security_events row for the audit trail.
	ForcePinCampaign(ctx context.Context, campaignID, version, actorID, actorIP, actorUA string) error

	// NotifyCampaignOfUpdate logs a security_events row and sends an
	// SMTP courtesy email (best-effort). Does NOT change the pin — this
	// is the "tell the owner" action, complementing force-pin.
	NotifyCampaignOfUpdate(ctx context.Context, campaignID, newVersion, actorID, actorIP, actorUA string) error

	// NotifyOlderCampaigns is the mass variant of NotifyCampaignOfUpdate:
	// every campaign whose pin is strictly older than version. Returns
	// the count notified.
	NotifyOlderCampaigns(ctx context.Context, version, actorID, actorIP, actorUA string) (notified int, err error)

	// ForcePinAllToVersion is the mass variant of ForcePinCampaign.
	// Partial failures don't abort — each campaign is independent.
	ForcePinAllToVersion(ctx context.Context, version, actorID, actorIP, actorUA string) (pinned int, err error)

	// AutoPinOnInstall runs after a foundry-module package installs:
	// each auto-tracking campaign in "preserve" pin_mode is explicit-
	// pinned to previousVersion so it stays on the version it was
	// running. Empty previousVersion (first-ever install) is a no-op.
	// Per-campaign events use EventModuleAutoPinOnInstall; a single
	// EventModuleAutoPinInstallSummary covers the whole install.
	AutoPinOnInstall(ctx context.Context, previousVersion, newVersion, actorID, actorIP, actorUA string) (affected int, err error)

	// MigrateAutoPinToVersion is the one-time bootstrap migration's
	// inner step: pin every empty-pin campaign to the given version,
	// logging EventModuleAutoPinMigration per campaign. Unlike
	// AutoPinOnInstall it never short-circuits on previous==new.
	MigrateAutoPinToVersion(ctx context.Context, version string) (affected int, err error)

	// --- admin auto-pin banner ---

	// GetUnreadAutoPinSummary returns the latest auto-pin install
	// summary if the admin hasn't dismissed it yet, or nil if there's
	// nothing to surface. Drives the banner on /admin/packages.
	GetUnreadAutoPinSummary(ctx context.Context) (*AutoPinSummary, error)

	// DismissAutoPinBanner marks the banner read by stamping the
	// current time into the dismissal settings key. Subsequent calls
	// to GetUnreadAutoPinSummary return nil until a new install
	// produces a fresh summary.
	DismissAutoPinBanner(ctx context.Context) error
}

// service is the default Service implementation.
type service struct {
	repo     Repository
	tokens   *TokenSigner
	settings CampaignSettingsAdapter
	pkgs     PackageReader
	// registry centralizes the "one foundry-module per Chronicle"
	// assumption; new call sites should use s.registry.FoundryPackage
	// directly rather than the legacy FindFoundryPackage wrapper.
	registry *PackageRegistry
	baseURL  string // public Chronicle origin, trimmed of trailing slash

	// kv is the site-settings KV store, used to persist and read the
	// auto-pin summary. Optional — nil disables the banner path softly.
	kv SettingsKVStore

	// Admin-action dependencies. Nil-safe — the per-campaign owner
	// flow doesn't need them, only the admin flow. If a deployment
	// doesn't wire SMTP/security events (e.g. tests), the admin
	// methods soft-fail on those steps and continue.
	events SecurityEventLogger
	mail   MailNotifier
	owners CampaignOwnerLookup
}

// Security event types — distinct rows in security_events per admin
// action so the audit trail can distinguish "notified" from "force-
// pinned" without parsing the details JSON.
const (
	// EventModuleForcePin — admin directly mutated a campaign's pin.
	EventModuleForcePin = "foundry_vtt.module_force_pin"

	// EventModuleUpdateNotify — admin notified the campaign owner
	// of a newer version. No pin change.
	EventModuleUpdateNotify = "foundry_vtt.module_update_notify"

	// EventModuleAutoPinOnInstall — one per campaign auto-pinned by
	// the install hook. Records from-version + new version + campaign.
	EventModuleAutoPinOnInstall = "foundry_vtt.module_autopin_on_install"

	// EventModuleAutoPinInstallSummary — single event per install
	// summarising how many campaigns got auto-pinned. The admin
	// banner on /admin/packages reads the latest event of this type
	// to display "N campaigns were auto-pinned to v0.X; review via..."
	EventModuleAutoPinInstallSummary = "foundry_vtt.module_autopin_install_summary"

	// EventModuleAutoPinMigration — one per campaign touched by the
	// one-time bootstrap migration (auto-tracking → explicit pin),
	// kept distinct from the install-hook variant for auditability.
	EventModuleAutoPinMigration = "foundry_vtt.module_autopin_migration"
)

// NewService constructs the foundry_vtt service. events/mail/owners
// are admin-action dependencies; passing nil for any of them disables
// the corresponding behavior softly (no panic, no per-request error).
func NewService(
	repo Repository,
	tokens *TokenSigner,
	settings CampaignSettingsAdapter,
	pkgs PackageReader,
	events SecurityEventLogger,
	mail MailNotifier,
	owners CampaignOwnerLookup,
	kv SettingsKVStore,
	baseURL string,
) Service {
	return &service{
		repo:     repo,
		tokens:   tokens,
		settings: settings,
		pkgs:     pkgs,
		registry: NewPackageRegistry(pkgs),
		events:   events,
		mail:     mail,
		owners:   owners,
		kv:       kv,
		baseURL:  strings.TrimRight(baseURL, "/"),
	}
}

// --- per-campaign URL minting ---

// BuildInstallURL returns the current per-campaign manifest URL,
// lazily minting a token row (version=1) on first call.
func (s *service) BuildInstallURL(ctx context.Context, campaignID string) (string, error) {
	exists, err := s.settings.CampaignExists(ctx, campaignID)
	if err != nil {
		return "", ErrInternal("campaign_exists_check", err)
	}
	if !exists {
		return "", ErrCampaignNotFound(campaignID)
	}
	tok, err := s.repo.GetCampaignToken(ctx, campaignID)
	if err != nil {
		return "", ErrInternal("get_campaign_token", err)
	}
	if tok == nil {
		// Lazy mint: first caller initializes token_version=1 so
		// every subsequent rotation has a row to bump.
		err = s.repo.UpsertCampaignToken(ctx, &CampaignToken{CampaignID: campaignID, TokenVersion: 1})
		if err != nil {
			return "", ErrInternal("upsert_initial_token", err)
		}
		tok = &CampaignToken{CampaignID: campaignID, TokenVersion: 1}
	}
	return s.buildURL(campaignID, tok.TokenVersion), nil
}

// RotateCampaignToken bumps the campaign's token version. Every
// previously-minted install URL stops verifying after this returns.
func (s *service) RotateCampaignToken(ctx context.Context, campaignID string) (string, error) {
	exists, err := s.settings.CampaignExists(ctx, campaignID)
	if err != nil {
		return "", ErrInternal("campaign_exists_check", err)
	}
	if !exists {
		return "", ErrCampaignNotFound(campaignID)
	}
	newVer, err := s.repo.BumpCampaignTokenVersion(ctx, campaignID)
	if err != nil {
		return "", ErrInternal("bump_token_version", err)
	}
	return s.buildURL(campaignID, newVer), nil
}

// buildURL assembles the public install URL using the fixed
// foundry-vtt namespace. Not driven by the per-package descriptor:
// the descriptor's manifestEndpoint affects what's written INTO the
// served module.json, while this is the outside-the-server URL
// Foundry hits — same shape today, kept separate so a future
// per-module descriptor could override it.
func (s *service) buildURL(campaignID string, tokenVersion int) string {
	signed := s.tokens.Sign(campaignID, tokenVersion)
	return fmt.Sprintf("%s/api/v1/campaigns/%s/foundry-vtt/module.json?token=%s",
		s.baseURL, campaignID, signed)
}

// VerifyManifestToken validates a token on an inbound manifest
// request. Two-step check: HMAC validates the token was minted by
// this server; DB token_version validates it hasn't been rotated.
func (s *service) VerifyManifestToken(ctx context.Context, campaignID, token string) error {
	cid, ver, err := s.tokens.Verify(token)
	if err != nil {
		return ErrInvalidToken(err)
	}
	if cid != campaignID {
		return ErrInvalidToken(fmt.Errorf("token campaign mismatch: token=%q url=%q", cid, campaignID))
	}
	current, err := s.repo.GetCampaignToken(ctx, campaignID)
	if err != nil {
		return ErrInternal("get_campaign_token", err)
	}
	if current == nil {
		return ErrTokenNotInitialized()
	}
	if current.TokenVersion != ver {
		return ErrInvalidToken(fmt.Errorf("token version stale: presented=%d current=%d",
			ver, current.TokenVersion))
	}
	return nil
}

// --- public manifest endpoint ---

// BuildManifestForCampaign is the serve-time core. Reads the on-disk
// module.json for the resolved version, rewrites descriptor-declared
// fields with per-campaign signed Chronicle URLs, returns the JSON.
// No caching — module.json is small and reading fresh keeps a version
// rewrite visible immediately.
func (s *service) BuildManifestForCampaign(ctx context.Context, campaignID string) (json.RawMessage, string, error) {
	params, err := s.resolveCampaignManifest(ctx, campaignID)
	if err != nil {
		return nil, "", err
	}
	return params.RewrittenManifest, params.InstallDir, nil
}

// BuildDownloadParams returns everything the download handler needs to
// stream a per-campaign rewritten zip: the install dir to walk, the
// path of module.json within it, and the rewritten manifest bytes to
// substitute there. Shares resolveCampaignManifest with
// BuildManifestForCampaign so both serve-time paths stay consistent —
// the zip's embedded module.json must carry Chronicle URLs, not the
// upstream ones, or Foundry's later update checks revert to GitHub.
func (s *service) BuildDownloadParams(ctx context.Context, campaignID string) (DownloadParams, error) {
	return s.resolveCampaignManifest(ctx, campaignID)
}

// DownloadParams is the bundle of values the download handler needs
// to stream a rewritten zip. RewrittenManifest is what gets written
// in place of the file at ModuleJSONPath (relative to InstallDir).
// Every other file in InstallDir is copied byte-for-byte.
type DownloadParams struct {
	// InstallDir is the absolute path to the version's extracted
	// directory on disk. Walk this to assemble the zip.
	InstallDir string
	// ModuleJSONPath is the path to the manifest INSIDE InstallDir
	// (e.g. "module.json" or "dist/module.json"). The descriptor
	// declares this; defaults apply when no chronicle-package.json
	// exists in the install dir.
	ModuleJSONPath string
	// RewrittenManifest is the bytes to write for the ModuleJSONPath
	// zip entry. Contains the per-campaign Chronicle URLs (whatever
	// fields the descriptor's rewriteFields lists). Every other key
	// from the upstream module.json is preserved verbatim.
	RewrittenManifest []byte
}

// resolveCampaignManifest is the shared resolution path for the
// manifest endpoint AND the download endpoint. Single source of
// truth for descriptor loading + rewrite — both serve-time paths
// produce identical bytes for the rewritten module.json.
//
// Returns *Error on failure (categorized for the JSON response).
func (s *service) resolveCampaignManifest(ctx context.Context, campaignID string) (DownloadParams, error) {
	// 1. Find the foundry-module package row.
	pkg, err := s.FindFoundryPackage(ctx)
	if err != nil {
		return DownloadParams{}, err
	}
	if pkg == nil {
		return DownloadParams{}, ErrNoPackageRegistered()
	}
	if pkg.InstalledVersion == "" {
		return DownloadParams{}, ErrNoVersionAvailable()
	}

	// 2. Resolve the campaign's pin to a version string.
	pin, err := s.settings.GetFoundryModulePin(ctx, campaignID)
	if err != nil {
		return DownloadParams{}, ErrInternal("get_campaign_pin", err)
	}
	version := pin
	if pin != "" && !packages.ValidVersionString(pin) {
		// A stored pin that fails validation is never turned into a path;
		// it is handled like a pin whose folder is missing.
		slog.Warn("ignoring invalid stored foundry module pin",
			slog.String("campaign_id", campaignID))
		return DownloadParams{}, ErrPinnedVersionNotInstalled(version)
	}
	if version == "" {
		// Latest-tracking: use whatever's currently installed.
		version = pkg.InstalledVersion
	}

	// 3. Resolve version → install dir; confirm it exists on disk.
	installDir := s.pkgs.InstallDirForVersion(packages.PackageTypeFoundryModule, pkg.Slug, version)
	if installDir == "" {
		return DownloadParams{}, ErrPinnedVersionNotInstalled(version)
	}
	if _, statErr := os.Stat(installDir); statErr != nil {
		if os.IsNotExist(statErr) {
			return DownloadParams{}, ErrPinnedVersionNotInstalled(version)
		}
		return DownloadParams{}, ErrInternal("stat_install_dir", statErr)
	}

	// 4. Load this version's descriptor, or fall back to defaults if
	//    the module ships none. A present-but-invalid descriptor is a
	//    real error and surfaces to the operator.
	desc, descErr := loadDescriptor(installDir)
	if descErr != nil && descErr != errDescriptorNotFound {
		// Real validation error — surface it.
		return DownloadParams{}, descErr
	}

	// 5. Read module.json at the descriptor-declared path.
	manifestPath := filepath.Join(installDir, desc.Package.ModuleJSONPath)
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return DownloadParams{}, ErrModuleJSONMissing(manifestPath, err)
	}

	// 6. Mint a fresh signed URL for this campaign's current token.
	tok, err := s.repo.GetCampaignToken(ctx, campaignID)
	if err != nil {
		return DownloadParams{}, ErrInternal("get_campaign_token", err)
	}
	if tok == nil {
		// VerifyManifestToken already passed, so a nil row here is
		// a real anomaly — the row was deleted between Verify and
		// BuildManifest. Not the operator's normal failure mode.
		return DownloadParams{}, ErrInternal("token_row_disappeared",
			fmt.Errorf("token row for verified campaign %q vanished", campaignID))
	}
	signed := s.tokens.Sign(campaignID, tok.TokenVersion)

	// 7. Parse, rewrite the descriptor-declared fields, marshal back.
	//    Using map[string]any preserves unknown fields verbatim —
	//    every key the upstream module ships flows through untouched
	//    except for the ones explicitly listed in rewriteFields.
	var m map[string]any
	if err := json.Unmarshal(manifestBytes, &m); err != nil {
		return DownloadParams{}, ErrInternal("parse_module_json",
			fmt.Errorf("parse module.json at %s: %w", manifestPath, err))
	}
	for _, field := range desc.Serving.RewriteFields {
		switch field {
		case "manifest":
			m["manifest"] = substituteURLTemplate(s.baseURL+desc.Serving.ManifestEndpoint, campaignID, signed)
		case "download":
			m["download"] = substituteURLTemplate(s.baseURL+desc.Serving.DownloadEndpoint, campaignID, signed)
		default:
			// Unknown rewrite field — silently skip. v1 only
			// recognizes manifest+download; future schemas can
			// add new ones. A future Chronicle build that learns
			// a new field name will handle it; older builds skip.
			continue
		}
	}
	rewritten, err := json.Marshal(m)
	if err != nil {
		return DownloadParams{}, ErrInternal("marshal_rewritten_manifest", err)
	}
	return DownloadParams{
		InstallDir:        installDir,
		ModuleJSONPath:    desc.Package.ModuleJSONPath,
		RewrittenManifest: rewritten,
	}, nil
}

// substituteURLTemplate replaces {campaign_id} and {token} in a URL
// template with literal string replacement (no template parsing —
// the template shape is constrained by descriptor schema v1).
func substituteURLTemplate(tmpl, campaignID, token string) string {
	out := strings.ReplaceAll(tmpl, "{campaign_id}", campaignID)
	out = strings.ReplaceAll(out, "{token}", token)
	return out
}

// FindFoundryPackage looks up the foundry-module typed package row.
// Returns (nil, nil) if none exists — caller treats this as the
// "no package registered" empty state. Delegates to
// s.registry.FoundryPackage so the "one foundry-module per Chronicle"
// assumption lives in one place (package_registry.go); prefer the
// registry directly at new call sites.
func (s *service) FindFoundryPackage(ctx context.Context) (*packages.Package, error) {
	if s.registry == nil {
		// Defensive: callers/tests that construct a service struct
		// without registry init fall through to an inline lookup.
		all, err := s.pkgs.ListPackages(ctx)
		if err != nil {
			return nil, ErrInternal("list_packages", err)
		}
		for i := range all {
			if all[i].Type == packages.PackageTypeFoundryModule {
				return &all[i], nil
			}
		}
		return nil, nil
	}
	return s.registry.FoundryPackage(ctx)
}

// GetPackageByID is a thin wrapper around the packages service's
// GetPackage, kept on Service so handlers don't need a separate
// PackageReader handle.
func (s *service) GetPackageByID(ctx context.Context, id string) (*packages.Package, error) {
	return s.pkgs.GetPackage(ctx, id)
}

// --- owner pin management ---

// SetPinnedVersion validates the version is installed on disk
// (rejecting pins to versions that don't exist), then writes via
// the campaigns adapter.
func (s *service) SetPinnedVersion(ctx context.Context, campaignID, version string) error {
	if version != "" {
		if !packages.ValidVersionString(version) {
			return apperror.NewValidation("invalid module version")
		}
		pkg, err := s.FindFoundryPackage(ctx)
		if err != nil {
			return err
		}
		if pkg == nil {
			return ErrNoPackageRegistered()
		}
		// Only versions the packages plugin knows about can be pinned, in
		// addition to the folder existing on disk.
		known, err := s.pkgs.ListVersions(ctx, pkg.ID)
		if err != nil {
			return ErrInternal("list_package_versions", err)
		}
		found := false
		for _, v := range known {
			if v.Version == version {
				found = true
				break
			}
		}
		if !found {
			return ErrPinnedVersionNotInstalled(version)
		}
		installDir := s.pkgs.InstallDirForVersion(packages.PackageTypeFoundryModule, pkg.Slug, version)
		if installDir == "" {
			return ErrPinnedVersionNotInstalled(version)
		}
		if _, err := os.Stat(installDir); err != nil {
			if os.IsNotExist(err) {
				return ErrPinnedVersionNotInstalled(version)
			}
			return ErrInternal("stat_install_dir", err)
		}
	}
	if err := s.settings.SetFoundryModulePin(ctx, campaignID, version); err != nil {
		return ErrInternal("set_foundry_module_pin", err)
	}
	return nil
}

// GetCampaignPin returns the campaign's saved pin (or "").
func (s *service) GetCampaignPin(ctx context.Context, campaignID string) (string, error) {
	pin, err := s.settings.GetFoundryModulePin(ctx, campaignID)
	if err != nil {
		return "", ErrInternal("get_foundry_module_pin", err)
	}
	return pin, nil
}

// --- owner tab data ---

// OwnerTabData assembles the renderable state for the owner tab.
// Pins all "missing" cases as flags on the returned struct (no
// errors) so the templ can render an empty-state UI instead of a
// 500 page.
func (s *service) OwnerTabData(ctx context.Context, campaignID string) (OwnerTabData, error) {
	out := OwnerTabData{CampaignID: campaignID}

	pkg, err := s.FindFoundryPackage(ctx)
	if err != nil {
		return out, err
	}
	if pkg == nil {
		// No foundry-module package registered — templ renders
		// the "ask an admin to register the module" empty state.
		return out, nil
	}
	out.PackageRegistered = true

	// Enumerate every version on disk for the foundry-module
	// package's install root. Returns an empty list if no
	// versions are installed (admin added the package but hasn't
	// installed any version yet).
	out.AvailableVersions = s.listInstalledVersionsOnDisk(pkg.Slug)

	pin, err := s.settings.GetFoundryModulePin(ctx, campaignID)
	if err != nil {
		return out, ErrInternal("get_foundry_module_pin", err)
	}
	out.CurrentPin = pin
	if pin == "" {
		out.CurrentVersion = pkg.InstalledVersion
	} else {
		out.CurrentVersion = pin
	}

	// Soft-fail: treat a read error as empty mode so a campaigns-side
	// issue doesn't break the whole owner tab.
	mode, modeErr := s.settings.GetFoundryModulePinMode(ctx, campaignID)
	if modeErr == nil {
		out.CurrentPinMode = mode
	}

	// Detect descriptor presence on the currently-served version.
	// Soft check — used only for the "descriptor: present|defaults"
	// transparency cue in the templ. Errors silently fall through
	// to DescriptorPresent=false.
	if out.CurrentVersion != "" {
		installDir := s.pkgs.InstallDirForVersion(packages.PackageTypeFoundryModule, pkg.Slug, out.CurrentVersion)
		if installDir != "" {
			if _, err := os.Stat(filepath.Join(installDir, descriptorFilename)); err == nil {
				out.DescriptorPresent = true
			}
		}
	}

	installURL, err := s.BuildInstallURL(ctx, campaignID)
	if err != nil {
		return out, err
	}
	out.InstallURL = installURL

	return out, nil
}

// listInstalledVersionsOnDisk enumerates the version directories inside
// the foundry-module install root, sorted descending by lex order (matches
// numeric order for canonical zero-padded semver like v0.1.5). Best-effort
// — a read failure returns an empty slice so the owner tab still renders.
func (s *service) listInstalledVersionsOnDisk(slug string) []string {
	// The packages plugin owns the on-disk layout convention;
	// we re-derive the root from InstallDirForVersion by
	// stripping a sentinel version. This avoids re-encoding the
	// "media/packages/foundry-module/" path here.
	sentinelDir := s.pkgs.InstallDirForVersion(packages.PackageTypeFoundryModule, slug, "0")
	if sentinelDir == "" {
		return nil
	}
	root := filepath.Dir(sentinelDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var versions []string
	for _, e := range entries {
		if e.IsDir() {
			versions = append(versions, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(versions)))
	return versions
}

// --- admin operations ---

// CampaignsUsingVersion lists campaigns currently pinned to the given
// version. Used by the admin's expandable "Campaigns Using v0.1.5"
// card on /admin/packages.
func (s *service) CampaignsUsingVersion(ctx context.Context, version string) ([]CampaignUsage, error) {
	usage, err := s.repo.CampaignsUsingVersion(ctx, version)
	if err != nil {
		return nil, ErrInternal("campaigns_using_version", err)
	}
	return usage, nil
}

// AutoTrackingCampaigns lists campaigns with an empty foundry_module_pin —
// the ones that auto-follow the newest installed module version. Surfaced
// on the admin's per-version card so an empty exact-pin list doesn't read
// as "nobody uses this module".
func (s *service) AutoTrackingCampaigns(ctx context.Context) ([]CampaignUsage, error) {
	usage, err := s.repo.CampaignsWithEmptyPin(ctx)
	if err != nil {
		return nil, ErrInternal("auto_tracking_campaigns", err)
	}
	return usage, nil
}

// ForcePinCampaign is the admin "Force-update this campaign to v0.X"
// action. Validates the version is installed on disk before touching
// CampaignSettings, so Foundry update checks don't immediately 404, and
// logs a security_events row (EventModuleForcePin) for the audit trail.
func (s *service) ForcePinCampaign(ctx context.Context, campaignID, version, actorID, actorIP, actorUA string) error {
	if version == "" {
		// Empty version means "clear pin / latest-tracking", which
		// doesn't apply to a force-update; reject rather than silently
		// no-op.
		return &Error{
			Category: ErrCategoryValidation,
			Code:     "force_pin_empty_version",
			Message: "Cannot force-pin a campaign to an empty version: " +
				"force-pin is for redirecting campaigns TO a specific newer version. " +
				"The admin endpoint requires a non-empty version. " +
				"If the goal is to clear a campaign's pin, the owner can do that " +
				"from campaign settings → VTT Setup Guides; admins don't have a " +
				"force-clear endpoint by design (clearing is a non-destructive choice).",
		}
	}

	// Reuses SetPinnedVersion's validation (installed-on-disk check).
	// Wrap the error so the audit log can tell a force-pin failure from
	// other SetPinnedVersion callers while keeping the typed error (%w).
	if err := s.SetPinnedVersion(ctx, campaignID, version); err != nil {
		return fmt.Errorf("force-pin campaign %q to %q: %w", campaignID, version, err)
	}

	// Audit log. Soft-fail if events is unwired (e.g. tests).
	if s.events != nil {
		_ = s.events.LogEvent(ctx, EventModuleForcePin,
			"", actorID, actorIP, actorUA,
			map[string]any{
				"campaign_id": campaignID,
				"version":     version,
			})
	}
	return nil
}

// NotifyCampaignOfUpdate logs the audit event and sends a courtesy SMTP
// email (when configured). Does NOT change the pin. SMTP failures are
// swallowed: the in-app banner (via security_events) is the primary
// surface, email is supplementary.
func (s *service) NotifyCampaignOfUpdate(ctx context.Context, campaignID, newVersion, actorID, actorIP, actorUA string) error {
	if s.events == nil {
		// Without an events sink, the notify is a no-op — the
		// banner depends on security_events row presence. Surface
		// this as an internal error rather than silently dropping.
		return ErrInternal("notify_events_unwired",
			fmt.Errorf("SecurityEventLogger is nil; can't record notify event for campaign %q", campaignID))
	}
	if err := s.events.LogEvent(ctx, EventModuleUpdateNotify,
		"", actorID, actorIP, actorUA,
		map[string]any{
			"campaign_id": campaignID,
			"new_version": newVersion,
		}); err != nil {
		return ErrInternal("log_notify_event", err)
	}

	// Best-effort email. Don't fail the call if SMTP isn't
	// configured — banner still fires.
	if s.mail != nil && s.owners != nil && s.mail.IsConfigured(ctx) {
		email, name, err := s.owners.GetCampaignOwnerEmail(ctx, campaignID)
		if err == nil && email != "" {
			subject := "Foundry module update available"
			body := fmt.Sprintf(
				"Hi %s,\n\nA newer version of the Chronicle Foundry module (%s) is available "+
					"for your campaign. Open campaign settings → VTT Setup Guides to switch.\n",
				name, newVersion)
			_ = s.mail.SendMail(ctx, []string{email}, subject, body)
		}
	}
	return nil
}

// NotifyOlderCampaigns is the mass-notify variant. Pulls every campaign
// with a pin strictly older than `version` and fires the per-campaign
// notify path on each. Partial failures don't abort — the count
// reflects how many succeeded.
func (s *service) NotifyOlderCampaigns(ctx context.Context, version, actorID, actorIP, actorUA string) (int, error) {
	older, err := s.repo.CampaignsOlderThan(ctx, version, semverLess)
	if err != nil {
		return 0, ErrInternal("campaigns_older_than", err)
	}
	count := 0
	for _, c := range older {
		if err := s.NotifyCampaignOfUpdate(ctx, c.CampaignID, version, actorID, actorIP, actorUA); err != nil {
			// Skip and continue — one campaign's failure shouldn't
			// abort the fan-out.
			continue
		}
		count++
	}
	return count, nil
}

// ForcePinAllToVersion is the mass force-update variant. Iterates
// every campaign whose pin is strictly older than the target and
// repins them. Returns the count of campaigns successfully repinned.
// Partial failures don't abort — see NotifyOlderCampaigns rationale.
func (s *service) ForcePinAllToVersion(ctx context.Context, version, actorID, actorIP, actorUA string) (int, error) {
	older, err := s.repo.CampaignsOlderThan(ctx, version, semverLess)
	if err != nil {
		return 0, ErrInternal("campaigns_older_than", err)
	}
	count := 0
	for _, c := range older {
		if err := s.ForcePinCampaign(ctx, c.CampaignID, version, actorID, actorIP, actorUA); err != nil {
			continue
		}
		count++
	}
	return count, nil
}

// GetBannerStatus reports whether the dashboard banner should fire: only
// when the campaign's pin is strictly older than the latest installed
// version. Errors swallow to zero-value — the banner is soft UX, not
// worth failing the dashboard over.
func (s *service) GetBannerStatus(ctx context.Context, campaignID string) (BannerStatus, error) {
	pkg, err := s.FindFoundryPackage(ctx)
	if err != nil || pkg == nil || pkg.InstalledVersion == "" {
		return BannerStatus{}, nil
	}
	pin, err := s.settings.GetFoundryModulePin(ctx, campaignID)
	if err != nil {
		return BannerStatus{}, nil
	}
	if pin == "" {
		// Latest-tracking — by definition at-or-after latest.
		return BannerStatus{}, nil
	}
	if !semverLess(pin, pkg.InstalledVersion) {
		// Pin is at-or-after the latest installed version. Could
		// be a deliberate stay-on-older or just current; no banner.
		return BannerStatus{}, nil
	}
	return BannerStatus{
		HasUpdate:      true,
		CurrentVersion: pin,
		LatestVersion:  pkg.InstalledVersion,
	}, nil
}

// AutoPinOnInstall is the install-time auto-pin path. previousVersion is
// the foundry-module version installed before the current install.
//
// Per-campaign behavior is driven by each auto-tracking campaign's
// foundry_module_pin_mode: "preserve" explicit-pins to previousVersion so
// the campaign stays on the version it was running (the admin bumps it
// manually via the "Campaigns Using v0.X" UI); "promote" and the
// empty/unset default leave the pin empty so the campaign keeps
// auto-tracking the newest installed version. A mode read error is
// treated as promote, never as a silent freeze.
//
// Empty previousVersion (first-ever install) is a no-op. Logs one
// EventModuleAutoPinOnInstall per preserved campaign plus a single
// EventModuleAutoPinInstallSummary; partial failures don't abort the
// fan-out.
func (s *service) AutoPinOnInstall(ctx context.Context, previousVersion, newVersion, actorID, actorIP, actorUA string) (int, error) {
	if previousVersion == "" {
		// First-ever install of the foundry-module package. No prior
		// state to preserve; nothing to do.
		return 0, nil
	}
	if previousVersion == newVersion {
		// Re-install of the same version. No version transition; no
		// auto-pin needed. Defensive — packages.InstallVersion shouldn't
		// call the hook in this case but the no-op is cheap.
		return 0, nil
	}

	if !packages.ValidVersionString(previousVersion) {
		// Never persist a pin that could not later be resolved safely.
		slog.Warn("skipping auto-pin: previous version is not a valid version string")
		return 0, nil
	}

	campaigns, err := s.repo.CampaignsWithEmptyPin(ctx)
	if err != nil {
		return 0, ErrInternal("campaigns_with_empty_pin", err)
	}

	affected := 0
	for _, c := range campaigns {
		// Only "preserve" mode freezes the campaign; a mode read error
		// defaults to promote (do nothing) rather than freezing.
		mode, modeErr := s.settings.GetFoundryModulePinMode(ctx, c.CampaignID)
		if modeErr != nil || mode != PinModePreserve {
			continue
		}
		if err := s.settings.SetFoundryModulePin(ctx, c.CampaignID, previousVersion); err != nil {
			// Skip and continue — one campaign's failure shouldn't
			// abort the rest. The summary event records the
			// successful count for admin visibility.
			continue
		}
		if s.events != nil {
			_ = s.events.LogEvent(ctx, EventModuleAutoPinOnInstall,
				"", actorID, actorIP, actorUA,
				map[string]any{
					"campaign_id":   c.CampaignID,
					"campaign_name": c.CampaignName,
					"from":          "auto-latest",
					"to_pin":        previousVersion,
					"new_installed": newVersion,
					"reason":        "preserve mode: froze at prior version on admin install",
				})
		}
		affected++
	}

	// Summary event: one row per install (regardless of affected
	// count). The admin audit log shows every summary; the banner
	// (below) shows only the latest unread one.
	if s.events != nil {
		_ = s.events.LogEvent(ctx, EventModuleAutoPinInstallSummary,
			"", actorID, actorIP, actorUA,
			map[string]any{
				"previous_version": previousVersion,
				"new_version":      newVersion,
				"affected":         affected,
			})
	}

	// Persist the summary to the settings KV so the admin /admin/packages
	// banner can read it. Soft-fail: the summary is supplementary.
	_ = s.storeAutoPinSummary(ctx, AutoPinSummary{
		PreviousVersion: previousVersion,
		NewVersion:      newVersion,
		Affected:        affected,
		Timestamp:       time.Now().Unix(),
	})

	return affected, nil
}

// MigrateAutoPinToVersion pins every empty-pin campaign to the given
// version, logging EventModuleAutoPinMigration per campaign. Unlike
// AutoPinOnInstall it never short-circuits on previous==new, since the
// point is to make the effective state explicit. Partial failures are
// skipped; the count reflects only successful pins.
func (s *service) MigrateAutoPinToVersion(ctx context.Context, version string) (int, error) {
	if version == "" {
		return 0, fmt.Errorf("MigrateAutoPinToVersion: empty version")
	}
	if !packages.ValidVersionString(version) {
		return 0, fmt.Errorf("MigrateAutoPinToVersion: invalid version")
	}
	campaigns, err := s.repo.CampaignsWithEmptyPin(ctx)
	if err != nil {
		return 0, fmt.Errorf("CampaignsWithEmptyPin: %w", err)
	}
	count := 0
	for _, c := range campaigns {
		if err := s.settings.SetFoundryModulePin(ctx, c.CampaignID, version); err != nil {
			continue
		}
		if s.events != nil {
			_ = s.events.LogEvent(ctx, EventModuleAutoPinMigration,
				"", "", "", "",
				map[string]any{
					"campaign_id":   c.CampaignID,
					"campaign_name": c.CampaignName,
					"from":          "auto-latest",
					"to":            version,
					"reason":        "C-FMC-6 one-time migration: auto-tracking campaigns get explicit pin to current installed version",
				})
		}
		count++
	}
	return count, nil
}
