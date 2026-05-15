package foundry_vtt

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/packages"
)

// CampaignSettingsAdapter is the narrow contract foundry_vtt needs
// from the campaigns plugin: read/write FoundryModulePin on
// CampaignSettings, and check whether a campaign exists. Implemented
// by an adapter in routes.go that wraps the campaigns service.
//
// Defined here (not in campaigns) because the dependency points
// FROM foundry_vtt TO campaigns — the campaigns plugin doesn't know
// foundry_vtt exists. Adapter pattern keeps the import direction
// one-way.
type CampaignSettingsAdapter interface {
	// GetFoundryModulePin returns the campaign's current pin
	// string. Empty string = latest-tracking (auto-resolve to the
	// active install).
	GetFoundryModulePin(ctx context.Context, campaignID string) (string, error)

	// SetFoundryModulePin writes the campaign's pin. Empty string
	// clears the pin (back to latest-tracking).
	SetFoundryModulePin(ctx context.Context, campaignID, version string) error

	// CampaignExists is the existence check the rotate-token and
	// install-URL builders use to reject unknown campaign IDs.
	CampaignExists(ctx context.Context, campaignID string) (bool, error)
}

// PackageReader is the narrow contract foundry_vtt needs from the
// packages plugin. Same one-way-import-direction reasoning as
// CampaignSettingsAdapter — but routes.go wires the packages
// service directly here since packages doesn't import foundry_vtt.
type PackageReader interface {
	// ListPackages enumerates the catalog. foundry_vtt filters by
	// PackageTypeFoundryModule to find the one foundry-module
	// package (typically Chronicle-Foundry-Module). Multiple rows
	// would be unusual but supported — foundry_vtt picks the first
	// installed one.
	ListPackages(ctx context.Context) ([]packages.Package, error)

	// InstallDirForVersion returns the on-disk path for a specific
	// version of a package. Doesn't check existence; foundry_vtt
	// stats the returned path to confirm the version is actually
	// installed on disk.
	InstallDirForVersion(pkgType packages.PackageType, slug, version string) string
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

	// BuildManifestForCampaign reads the on-disk module.json for
	// the campaign's resolved version, rewrites the descriptor-
	// declared fields with per-campaign signed Chronicle URLs, and
	// returns the JSON bytes. The downloadPath is the absolute
	// path to the version's extracted directory (the download
	// handler will stream the same module.json's content from
	// there — Foundry's download endpoint fetches the same dir).
	//
	// Returns *Error on failure so the handler can pick the right
	// HTTP status + JSON shape.
	BuildManifestForCampaign(ctx context.Context, campaignID string) (manifestJSON json.RawMessage, downloadDir string, err error)

	// --- owner pin management ---

	// SetPinnedVersion writes the campaign's pin. Empty version
	// clears the pin (latest-tracking).
	SetPinnedVersion(ctx context.Context, campaignID, version string) error

	// GetCampaignPin returns the campaign's saved pin string
	// ("" if unpinned). Thin pass-through to the adapter.
	GetCampaignPin(ctx context.Context, campaignID string) (string, error)

	// --- owner tab fragment data ---

	// OwnerTabData assembles the OwnerTabData struct for the
	// owner settings tab fragment. Handles the empty-state cases
	// (no foundry-module package registered, no version installed)
	// by setting flags on the returned struct rather than
	// returning errors.
	OwnerTabData(ctx context.Context, campaignID string) (OwnerTabData, error)
}

// service is the default Service implementation.
type service struct {
	repo     Repository
	tokens   *TokenSigner
	settings CampaignSettingsAdapter
	pkgs     PackageReader
	baseURL  string // public Chronicle origin, trimmed of trailing slash
}

// NewService constructs the foundry_vtt service.
func NewService(
	repo Repository,
	tokens *TokenSigner,
	settings CampaignSettingsAdapter,
	pkgs PackageReader,
	baseURL string,
) Service {
	return &service{
		repo:     repo,
		tokens:   tokens,
		settings: settings,
		pkgs:     pkgs,
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

// buildURL assembles the public install URL using the locked
// foundry-vtt namespace. NOT driven by the per-package descriptor
// (yet) — the descriptor's manifestEndpoint affects what gets
// written INTO the served module.json, while THIS URL is the
// outside-the-server one Foundry hits. They're the same shape
// today but separated so a future per-module descriptor could
// override.
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
//
// No caching — module.json is small (< 5 KB), reading fresh per
// request keeps the post-install hook's version rewrite visible
// immediately. If profiling shows the file read is a hotspot, add
// a single-flight cache keyed on (packageID, version) at a future
// PR.
func (s *service) BuildManifestForCampaign(ctx context.Context, campaignID string) (json.RawMessage, string, error) {
	// 1. Find the foundry-module package row.
	pkg, err := s.findFoundryPackage(ctx)
	if err != nil {
		return nil, "", err
	}
	if pkg == nil {
		return nil, "", ErrNoPackageRegistered()
	}
	if pkg.InstalledVersion == "" {
		return nil, "", ErrNoVersionAvailable()
	}

	// 2. Resolve the campaign's pin to a version string.
	pin, err := s.settings.GetFoundryModulePin(ctx, campaignID)
	if err != nil {
		return nil, "", ErrInternal("get_campaign_pin", err)
	}
	version := pin
	if version == "" {
		// Latest-tracking: use whatever's currently installed.
		version = pkg.InstalledVersion
	}

	// 3. Resolve version → install dir; confirm it exists on disk.
	installDir := s.pkgs.InstallDirForVersion(packages.PackageTypeFoundryModule, pkg.Slug, version)
	if installDir == "" {
		return nil, "", ErrPinnedVersionNotInstalled(version)
	}
	if _, statErr := os.Stat(installDir); statErr != nil {
		if os.IsNotExist(statErr) {
			return nil, "", ErrPinnedVersionNotInstalled(version)
		}
		return nil, "", ErrInternal("stat_install_dir", statErr)
	}

	// 4. Load this version's descriptor (or fall back to defaults).
	//    Errors from a present-but-invalid descriptor surface to
	//    the operator via the categorized error; a missing descriptor
	//    is the normal path for modules without FM-PKG-DESCRIPTOR
	//    yet shipped.
	desc, descErr := loadDescriptor(installDir)
	if descErr != nil && descErr != errDescriptorNotFound {
		// Real validation error — surface it.
		return nil, "", descErr
	}

	// 5. Read module.json at the descriptor-declared path.
	manifestPath := filepath.Join(installDir, desc.Package.ModuleJSONPath)
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, "", ErrModuleJSONMissing(manifestPath, err)
	}

	// 6. Mint a fresh signed URL for this campaign's current token.
	tok, err := s.repo.GetCampaignToken(ctx, campaignID)
	if err != nil {
		return nil, "", ErrInternal("get_campaign_token", err)
	}
	if tok == nil {
		// VerifyManifestToken already passed, so a nil row here is
		// a real anomaly — the row was deleted between Verify and
		// BuildManifest. Not the operator's normal failure mode.
		return nil, "", ErrInternal("token_row_disappeared",
			fmt.Errorf("token row for verified campaign %q vanished", campaignID))
	}
	signed := s.tokens.Sign(campaignID, tok.TokenVersion)

	// 7. Parse, rewrite the descriptor-declared fields, marshal back.
	//    Using map[string]any preserves unknown fields verbatim —
	//    every key the upstream module ships flows through untouched
	//    except for the ones explicitly listed in rewriteFields.
	var m map[string]any
	if err := json.Unmarshal(manifestBytes, &m); err != nil {
		return nil, "", ErrInternal("parse_module_json",
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
		return nil, "", ErrInternal("marshal_rewritten_manifest", err)
	}
	return rewritten, installDir, nil
}

// substituteURLTemplate replaces {campaign_id} and {token} in a URL
// template with the campaign-specific values. Defined as a method-
// less helper so it's trivially testable. Substitution is literal
// string replace, not template parsing — the template is tightly
// constrained by schema v1.
func substituteURLTemplate(tmpl, campaignID, token string) string {
	out := strings.ReplaceAll(tmpl, "{campaign_id}", campaignID)
	out = strings.ReplaceAll(out, "{token}", token)
	return out
}

// findFoundryPackage looks up the foundry-module typed package row.
// Returns (nil, nil) if none exists — caller treats this as the
// "no package registered" empty state. Multiple foundry-module
// packages would be unusual; the first installed one wins.
func (s *service) findFoundryPackage(ctx context.Context) (*packages.Package, error) {
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

// --- owner pin management ---

// SetPinnedVersion validates the version is installed on disk
// (rejecting pins to versions that don't exist), then writes via
// the campaigns adapter.
func (s *service) SetPinnedVersion(ctx context.Context, campaignID, version string) error {
	if version != "" {
		pkg, err := s.findFoundryPackage(ctx)
		if err != nil {
			return err
		}
		if pkg == nil {
			return ErrNoPackageRegistered()
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

	pkg, err := s.findFoundryPackage(ctx)
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

// listInstalledVersionsOnDisk enumerates the version directories
// inside the foundry-module install root. Best-effort — a read
// failure returns an empty slice so the owner tab still renders.
//
// Sorted descending (newest semver-ish first) using simple lex
// sort on the version strings. Foundry module versions are
// SemVer-compatible (v0.1.5, etc.), and lex sort matches numeric
// sort for the canonical zero-padded shape. If a deployment ships
// non-canonical versions, this still produces a stable order; the
// dropdown is for owners to pick, not for chronological accuracy.
func (s *service) listInstalledVersionsOnDisk(slug string) []string {
	// The packages plugin owns the on-disk layout convention;
	// we re-derive the root from InstallDirForVersion by
	// stripping a sentinel version. This avoids re-encoding the
	// "media/packages/foundry-module/" path here.
	sentinelDir := s.pkgs.InstallDirForVersion(packages.PackageTypeFoundryModule, slug, "__list__")
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
