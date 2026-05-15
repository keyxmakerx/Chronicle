package foundry_modules

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// CampaignSettingsWriter is the narrow interface the foundry_modules
// plugin uses to read campaign settings and mutate the FoundryModulePin
// field. Implemented by an adapter in routes.go over the campaigns
// service so this plugin doesn't import campaigns directly (which
// would risk a cycle once the campaigns plugin grows a foundry-module
// banner query).
type CampaignSettingsWriter interface {
	// SetFoundryModulePin updates CampaignSettings.FoundryModulePin
	// for the given campaign. Empty version clears the pin (latest-
	// tracking mode).
	SetFoundryModulePin(ctx context.Context, campaignID, version string) error

	// GetFoundryModulePin returns the current pin for the campaign,
	// or "" if unpinned.
	GetFoundryModulePin(ctx context.Context, campaignID string) (string, error)

	// CampaignExists confirms a campaign ID is real before the
	// service mints a token URL for it. Returns (false, nil) for
	// unknown campaigns; non-nil error for DB issues.
	CampaignExists(ctx context.Context, campaignID string) (bool, error)
}

// SecurityEventLogger writes to the security_events table. Same
// interface shape as admin's existing logger; passed in so the plugin
// doesn't import the admin package.
type SecurityEventLogger interface {
	LogEvent(ctx context.Context, eventType, userID, actorID, ip, userAgent string, details map[string]any) error
}

// MailNotifier is the SMTP boundary. Implemented by smtp.MailService.
// Notify*** methods on the service are no-ops when IsConfigured returns
// false, so the notify admin action still completes (banner + audit log)
// without email when SMTP isn't set up.
type MailNotifier interface {
	IsConfigured(ctx context.Context) bool
	SendMail(ctx context.Context, to []string, subject, body string) error
}

// CampaignOwnerLookup resolves a campaign's owner email so the notify
// action knows where to send. Implemented by the campaigns service.
type CampaignOwnerLookup interface {
	GetCampaignOwnerEmail(ctx context.Context, campaignID string) (email, displayName string, err error)
}

// Service is the foundry_modules plugin's business-logic surface.
// Mirrors the pattern of packages.PackageService — wide interface,
// single struct implementing it.
type Service interface {
	// --- admin / catalog ---
	UploadVersion(ctx context.Context, in UploadVersionInput) (*ModuleVersion, error)
	ListVersions(ctx context.Context, includeWithdrawn bool) ([]*ModuleVersion, error)
	GetVersion(ctx context.Context, version string) (*ModuleVersion, error)
	SetVersionStatus(ctx context.Context, version string, status ModuleStatus) error
	LatestAvailable(ctx context.Context) (*ModuleVersion, error)
	CampaignsUsingVersion(ctx context.Context, version string) ([]CampaignUsage, error)
	NotifyOlderCampaigns(ctx context.Context, version, actorID, actorIP, actorUA string) (notified int, err error)
	ForcePinCampaign(ctx context.Context, campaignID, version, actorID, actorIP, actorUA string) error

	// --- per-campaign ---
	SetPinnedVersion(ctx context.Context, campaignID, version string) error
	RotateCampaignToken(ctx context.Context, campaignID, actorID, actorIP, actorUA string) (newURL string, err error)
	BuildInstallURL(ctx context.Context, campaignID string) (string, error)

	// --- public manifest endpoint ---
	BuildManifestForCampaign(ctx context.Context, campaignID string) (manifestJSON json.RawMessage, downloadPath string, err error)
	VerifyManifestToken(ctx context.Context, campaignID, token string) error
	GetVersionForCampaign(ctx context.Context, campaignID string) (*ModuleVersion, error)

	// --- pinning validation (for the owner-side PUT settings handler) ---
	ValidatePinnable(ctx context.Context, version string) error

	// GetCampaignPin returns the raw pin string for a campaign (or
	// "" if unpinned). Wraps the campaigns-side getter so the owner
	// settings tab can render the dropdown with the saved value
	// preselected.
	GetCampaignPin(ctx context.Context, campaignID string) (string, error)

	// GetBannerStatus reports whether the campaign should see the
	// "newer version available" dashboard banner. Returns a zero-
	// value status (HasUpdate=false) when the catalog is empty, the
	// campaign already resolves to the latest, or any lookup fails
	// — the banner is best-effort UX, not security state.
	GetBannerStatus(ctx context.Context, campaignID string) (BannerStatus, error)
}

// BannerStatus is the renderable banner state for the owner dashboard.
// HasUpdate=false suppresses the banner entirely.
type BannerStatus struct {
	HasUpdate      bool
	CurrentVersion string
	LatestVersion  string
}

// UploadVersionInput is what the handler hands to the service after
// it's pulled the multipart upload out of the HTTP request. Keeping
// the service free of *multipart.FileHeader means tests can hand it
// any io.Reader.
type UploadVersionInput struct {
	File         io.Reader
	FileSize     int64
	UploaderID   string
	ReleaseNotes string // optional admin-provided override; falls back to manifest description
}

// service is the default Service implementation.
type service struct {
	repo         Repository
	tokens       *TokenSigner
	settings     CampaignSettingsWriter
	events       SecurityEventLogger
	mail         MailNotifier
	owners       CampaignOwnerLookup
	storageDir   string // directory under which uploaded zips are stored
	baseURL      string // e.g. "https://chronicle.example.com" — used to build install URLs
}

// NewService constructs the foundry_modules service. storageDir is the
// directory uploaded module zips are written to; the caller is
// responsible for ensuring it exists and is writable. baseURL is the
// public Chronicle origin (used in the install-URL builder).
func NewService(
	repo Repository,
	tokens *TokenSigner,
	settings CampaignSettingsWriter,
	events SecurityEventLogger,
	mail MailNotifier,
	owners CampaignOwnerLookup,
	storageDir, baseURL string,
) Service {
	return &service{
		repo:       repo,
		tokens:     tokens,
		settings:   settings,
		events:     events,
		mail:       mail,
		owners:     owners,
		storageDir: storageDir,
		baseURL:    strings.TrimRight(baseURL, "/"),
	}
}

// --- security event types ---

const (
	// EventFoundryModuleUpdateNotify — admin clicked "notify owners of
	// older versions" on a newer release. One event per notified campaign.
	EventFoundryModuleUpdateNotify = "foundry.module_update_notify"

	// EventFoundryModuleForcePin — admin directly mutated a campaign's
	// pin. Distinct from _notify so the audit trail shows the difference
	// between "told the owner to update" and "updated for them."
	EventFoundryModuleForcePin = "foundry.module_force_pin"

	// EventFoundryModuleTokenRotated — owner (or admin) rotated the
	// per-campaign signing version. Every previously-issued install URL
	// is now invalid; the UI surfaces this so all players know to
	// reinstall.
	EventFoundryModuleTokenRotated = "foundry.module_token_rotated"

	// EventFoundryModuleGitHubPoll — one row per poll cycle (whether
	// triggered by the background goroutine or the admin "Fetch from
	// GitHub now" button), summarising how many new versions were
	// ingested and any errors. Lets operators see at a glance whether
	// the poller is alive and healthy from the security-events
	// dashboard.
	EventFoundryModuleGitHubPoll = "foundry.module_github_poll_completed"
)

// --- admin: upload ---

// UploadVersion writes the zip to disk, extracts module.json, parses
// the manifest, and inserts a row. Rejects duplicates with
// apperror.NewConflict.
//
// The zip is written under storageDir as "{version}.zip" (filename
// derived from the parsed manifest, not the uploaded filename, so an
// attacker can't path-traverse via the upload form).
func (s *service) UploadVersion(ctx context.Context, in UploadVersionInput) (*ModuleVersion, error) {
	if s.storageDir == "" {
		return nil, apperror.NewInternal(errors.New("foundry_modules storageDir not configured"))
	}
	if in.UploaderID == "" {
		return nil, apperror.NewBadRequest("uploader_id is required")
	}

	// Read the whole upload into memory so we can both (a) compute
	// SHA-256, (b) parse the embedded module.json, and (c) re-write
	// to disk. Module zips are small (< ~5 MB) so the buffer is fine.
	buf, err := io.ReadAll(in.File)
	if err != nil {
		return nil, apperror.NewBadRequest("could not read upload")
	}
	if int64(len(buf)) != in.FileSize && in.FileSize > 0 {
		// The handler's Content-Length and the read bytes disagree —
		// either the upload was truncated or someone is fuzzing us.
		// Reject rather than guess which one to trust.
		return nil, apperror.NewBadRequest("upload size mismatch")
	}

	parsed, manifestBytes, err := extractManifest(buf)
	if err != nil {
		return nil, err
	}

	// Reject duplicates before we touch the filesystem.
	existing, err := s.repo.GetVersion(ctx, parsed.Version)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("checking version existence: %w", err))
	}
	if existing != nil {
		return nil, apperror.NewConflict("version " + parsed.Version + " already exists")
	}

	// Write the zip to disk. Filename = sanitized version string to
	// avoid path traversal via crafted module.json values.
	sanitizedVersion := sanitizeFilename(parsed.Version)
	if sanitizedVersion == "" {
		return nil, apperror.NewBadRequest("manifest version is empty or unsafe")
	}
	path := filepath.Join(s.storageDir, sanitizedVersion+".zip")
	if err := os.MkdirAll(s.storageDir, 0o755); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("creating storage dir: %w", err))
	}
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("writing module zip: %w", err))
	}

	sum := sha256.Sum256(buf)

	uploader := in.UploaderID
	v := &ModuleVersion{
		ID:               generateID(),
		Version:          parsed.Version,
		FilePath:         path,
		FileSize:         int64(len(buf)),
		ContentSHA256:    hex.EncodeToString(sum[:]),
		ManifestJSON:     manifestBytes,
		Status:           StatusAvailable,
		ReleaseNotes:     firstNonEmpty(in.ReleaseNotes, parsed.Description),
		UploadedByUserID: &uploader,
		UploadedAt:       time.Now().UTC(),
		// Manual uploads are explicitly tagged so the catalog UI can
		// render "Manual upload by …" rather than "GitHub release …"
		// next to the version. The DB default covers older rows.
		Source: SourceManualUpload,
	}
	if parsed.Compatibility != nil {
		v.CompatibilityMinimum = parsed.Compatibility.Minimum
		v.CompatibilityVerified = parsed.Compatibility.Verified
		v.CompatibilityMaximum = parsed.Compatibility.Maximum
	}

	if err := s.repo.InsertVersion(ctx, v); err != nil {
		// Clean up the file we just wrote so failed inserts don't
		// leave orphaned bytes on disk.
		_ = os.Remove(path)
		if errors.Is(err, ErrVersionExists) {
			return nil, apperror.NewConflict("version " + parsed.Version + " already exists")
		}
		return nil, apperror.NewInternal(fmt.Errorf("inserting version row: %w", err))
	}

	return v, nil
}

// --- admin: list/status/usage ---

func (s *service) ListVersions(ctx context.Context, includeWithdrawn bool) ([]*ModuleVersion, error) {
	return s.repo.ListVersions(ctx, includeWithdrawn)
}

func (s *service) GetVersion(ctx context.Context, version string) (*ModuleVersion, error) {
	v, err := s.repo.GetVersion(ctx, version)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if v == nil {
		return nil, apperror.NewNotFound("module version not found")
	}
	return v, nil
}

func (s *service) SetVersionStatus(ctx context.Context, version string, status ModuleStatus) error {
	if !IsValidStatus(status) {
		return apperror.NewBadRequest("invalid status")
	}
	if err := s.repo.SetVersionStatus(ctx, version, status); err != nil {
		if errors.Is(err, ErrVersionExists) {
			// shouldn't happen on update, but defensive
			return apperror.NewConflict("version conflict")
		}
		return apperror.NewInternal(err)
	}
	return nil
}

// ErrNoVersionAvailable is the sentinel returned by LatestAvailable
// (and by GetVersionForCampaign indirectly) when the catalog is
// completely empty — no available, no deprecated. Distinct from a
// generic 404 so the public manifest endpoint can render the
// informative 503 response the Foundry-side "Check Chronicle for
// updates" button knows how to surface.
var ErrNoVersionAvailable = errors.New("no foundry module version available")

// LatestAvailable returns the newest non-deprecated version. When no
// version is marked available (admin deprecated everything), falls
// back to the newest deprecated as a degraded mode — better than
// returning 404 to Foundry, which would freeze every campaign mid-
// session. When the catalog is truly empty, returns
// ErrNoVersionAvailable so the manifest endpoint can return a 503
// with operator-actionable copy instead of an opaque 404.
func (s *service) LatestAvailable(ctx context.Context) (*ModuleVersion, error) {
	v, err := s.repo.LatestAvailable(ctx)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if v != nil {
		return v, nil
	}
	// Fallback: newest non-withdrawn (i.e., newest deprecated).
	all, err := s.repo.ListVersions(ctx, false)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if len(all) == 0 {
		return nil, ErrNoVersionAvailable
	}
	return all[0], nil
}

func (s *service) CampaignsUsingVersion(ctx context.Context, version string) ([]CampaignUsage, error) {
	return s.repo.CampaignsUsingVersion(ctx, version)
}

// NotifyOlderCampaigns logs a security event per campaign with an older
// pin AND optionally emails the owner. Returns the notified count.
//
// Email is best-effort: a failure to send doesn't roll back the audit
// event because the in-app banner (which reads from security_events)
// is the primary surface; email is a courtesy.
func (s *service) NotifyOlderCampaigns(ctx context.Context, version, actorID, actorIP, actorUA string) (int, error) {
	if _, err := s.GetVersion(ctx, version); err != nil {
		return 0, err
	}
	older, err := s.repo.CampaignsOlderThan(ctx, version, semverLess)
	if err != nil {
		return 0, apperror.NewInternal(err)
	}
	count := 0
	smtpReady := s.mail != nil && s.mail.IsConfigured(ctx)
	for _, c := range older {
		details := map[string]any{
			"campaign_id":      c.CampaignID,
			"campaign_name":    c.CampaignName,
			"pinned_to":        c.PinnedTo,
			"new_version":      version,
		}
		if err := s.events.LogEvent(ctx,
			EventFoundryModuleUpdateNotify,
			c.OwnerUserID, actorID, actorIP, actorUA, details,
		); err != nil {
			continue // skip and keep notifying others
		}
		count++

		if smtpReady && s.owners != nil {
			email, name, err := s.owners.GetCampaignOwnerEmail(ctx, c.CampaignID)
			if err == nil && email != "" {
				subject := "Foundry module update available for " + c.CampaignName
				body := fmt.Sprintf(
					"Hi %s,\n\nA newer version of the Chronicle Foundry module (%s) is available."+
						" Your campaign \"%s\" is pinned to %s.\n\n"+
						"Open the campaign settings → Foundry Module tab to switch.\n",
					name, version, c.CampaignName, c.PinnedTo,
				)
				_ = s.mail.SendMail(ctx, []string{email}, subject, body)
			}
		}
	}
	return count, nil
}

// ForcePinCampaign directly mutates a campaign's FoundryModulePin,
// bypassing the owner-side flow. Used by site admins for emergency
// rollbacks. Logs a distinct security_events row so the audit trail
// distinguishes admin force-pins from owner-initiated pin changes.
func (s *service) ForcePinCampaign(ctx context.Context, campaignID, version, actorID, actorIP, actorUA string) error {
	if err := s.ValidatePinnable(ctx, version); err != nil {
		return err
	}
	if err := s.settings.SetFoundryModulePin(ctx, campaignID, version); err != nil {
		return apperror.NewInternal(fmt.Errorf("setting pin: %w", err))
	}
	_ = s.events.LogEvent(ctx, EventFoundryModuleForcePin,
		"", actorID, actorIP, actorUA,
		map[string]any{
			"campaign_id": campaignID,
			"version":     version,
		})
	return nil
}

// --- owner: pin / token / install-url ---

// SetPinnedVersion is the owner-side pin entry point. Validates the
// version is currently pinnable (exists and not withdrawn) before
// touching campaign settings.
func (s *service) SetPinnedVersion(ctx context.Context, campaignID, version string) error {
	if version != "" {
		if err := s.ValidatePinnable(ctx, version); err != nil {
			return err
		}
	}
	if err := s.settings.SetFoundryModulePin(ctx, campaignID, version); err != nil {
		return apperror.NewInternal(fmt.Errorf("setting pin: %w", err))
	}
	return nil
}

// GetCampaignPin returns the campaign's currently-saved pin string
// ("" when unpinned). Thin pass-through to the settings adapter.
func (s *service) GetCampaignPin(ctx context.Context, campaignID string) (string, error) {
	return s.settings.GetFoundryModulePin(ctx, campaignID)
}

// GetBannerStatus reports whether a "newer module version available"
// banner should render for this campaign's owner. Comparison is by
// semver — a campaign pinned to v0.1.5 with latest=v0.2.0 sees the
// banner; a latest-tracking campaign never sees the banner because
// its current always equals latest by definition.
//
// All errors are swallowed to zero-value (no banner). The banner is
// soft UX; a flaky lookup shouldn't break the dashboard.
func (s *service) GetBannerStatus(ctx context.Context, campaignID string) (BannerStatus, error) {
	latest, err := s.repo.LatestAvailable(ctx)
	if err != nil || latest == nil {
		return BannerStatus{}, nil
	}
	pin, err := s.settings.GetFoundryModulePin(ctx, campaignID)
	if err != nil {
		return BannerStatus{}, nil
	}
	if pin == "" {
		// Latest-tracking campaigns auto-resolve to latest; no banner needed.
		return BannerStatus{}, nil
	}
	if !semverLess(pin, latest.Version) {
		// Pin is at-or-after latest. Could be a deliberate stay-on-
		// older choice or simply current; either way no banner.
		return BannerStatus{}, nil
	}
	return BannerStatus{
		HasUpdate:      true,
		CurrentVersion: pin,
		LatestVersion:  latest.Version,
	}, nil
}

// ValidatePinnable returns nil iff the version exists and is not
// withdrawn. Deprecated versions ARE pinnable (an owner may want to
// freeze on a specific deprecated build); the owner UI surfaces the
// deprecation warning.
func (s *service) ValidatePinnable(ctx context.Context, version string) error {
	v, err := s.repo.GetVersion(ctx, version)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if v == nil {
		return apperror.NewNotFound("version not found")
	}
	if v.Status == StatusWithdrawn {
		return apperror.NewBadRequest("version is withdrawn and cannot be pinned")
	}
	return nil
}

// RotateCampaignToken bumps the token version, invalidating every
// previously-issued install URL for the campaign. Returns the new
// install URL so the UI can show it immediately.
func (s *service) RotateCampaignToken(ctx context.Context, campaignID, actorID, actorIP, actorUA string) (string, error) {
	exists, err := s.settings.CampaignExists(ctx, campaignID)
	if err != nil {
		return "", apperror.NewInternal(err)
	}
	if !exists {
		return "", apperror.NewNotFound("campaign not found")
	}
	newVer, err := s.repo.BumpCampaignTokenVersion(ctx, campaignID)
	if err != nil {
		return "", apperror.NewInternal(fmt.Errorf("rotating token: %w", err))
	}
	_ = s.events.LogEvent(ctx, EventFoundryModuleTokenRotated,
		"", actorID, actorIP, actorUA,
		map[string]any{"campaign_id": campaignID, "new_token_version": newVer})
	return s.buildURL(campaignID, newVer), nil
}

// BuildInstallURL returns the current manifest URL for the campaign,
// minting a token row at version=1 if none exists yet (so the first
// caller doesn't get a stale "no token" error).
func (s *service) BuildInstallURL(ctx context.Context, campaignID string) (string, error) {
	exists, err := s.settings.CampaignExists(ctx, campaignID)
	if err != nil {
		return "", apperror.NewInternal(err)
	}
	if !exists {
		return "", apperror.NewNotFound("campaign not found")
	}
	tok, err := s.repo.GetCampaignToken(ctx, campaignID)
	if err != nil {
		return "", apperror.NewInternal(err)
	}
	if tok == nil {
		// Lazy mint: token_version=1 on first request.
		err = s.repo.UpsertCampaignToken(ctx, &CampaignToken{CampaignID: campaignID, TokenVersion: 1})
		if err != nil {
			return "", apperror.NewInternal(err)
		}
		tok = &CampaignToken{CampaignID: campaignID, TokenVersion: 1}
	}
	return s.buildURL(campaignID, tok.TokenVersion), nil
}

func (s *service) buildURL(campaignID string, tokenVersion int) string {
	signed := s.tokens.Sign(campaignID, tokenVersion)
	return fmt.Sprintf("%s/api/v1/campaigns/%s/foundry/module.json?token=%s",
		s.baseURL, campaignID, signed)
}

// --- public manifest endpoint ---

// VerifyManifestToken decodes the signed token, confirms the HMAC, AND
// confirms the embedded token-version still matches the campaign's
// current value. The two-step check is the revocation primitive:
// rotation bumps the DB value while leaving the HMAC valid for the
// old version, so old URLs HMAC-verify but DB-mismatch and get
// rejected.
func (s *service) VerifyManifestToken(ctx context.Context, campaignID, token string) error {
	cid, ver, err := s.tokens.Verify(token)
	if err != nil {
		return apperror.NewNotFound("invalid token")
	}
	if cid != campaignID {
		return apperror.NewNotFound("invalid token")
	}
	current, err := s.repo.GetCampaignToken(ctx, campaignID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if current == nil {
		// No token row at all — the URL must be forged or someone
		// rotated everything to scratch. 404 either way.
		return apperror.NewNotFound("invalid token")
	}
	if current.TokenVersion != ver {
		return apperror.NewNotFound("token revoked; reinstall via campaign settings")
	}
	return nil
}

// GetVersionForCampaign returns the version a Foundry update check
// for this campaign should resolve to. Honors the pin if set,
// otherwise falls back to LatestAvailable.
func (s *service) GetVersionForCampaign(ctx context.Context, campaignID string) (*ModuleVersion, error) {
	pin, err := s.settings.GetFoundryModulePin(ctx, campaignID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if pin == "" {
		return s.LatestAvailable(ctx)
	}
	v, err := s.repo.GetVersion(ctx, pin)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if v == nil || v.Status == StatusWithdrawn {
		// Pinned to a withdrawn version: 404 so Foundry stops trying.
		return nil, apperror.NewNotFound("pinned version unavailable")
	}
	return v, nil
}

// BuildManifestForCampaign rewrites the stored manifest's "manifest"
// and "download" URLs to point back at Chronicle's signed endpoints,
// then returns the rewritten JSON. The "download" path returned
// alongside is a server-relative URL (Chronicle's handler 302s to a
// short-lived signed media URL — same shape Foundry's installer
// expects).
func (s *service) BuildManifestForCampaign(ctx context.Context, campaignID string) (json.RawMessage, string, error) {
	v, err := s.GetVersionForCampaign(ctx, campaignID)
	if err != nil {
		return nil, "", err
	}
	tok, err := s.repo.GetCampaignToken(ctx, campaignID)
	if err != nil || tok == nil {
		// VerifyManifestToken already passed at this point, so a
		// nil token row here is a real DB anomaly.
		return nil, "", apperror.NewInternal(fmt.Errorf("token row missing for verified campaign"))
	}

	var m map[string]any
	if err := json.Unmarshal(v.ManifestJSON, &m); err != nil {
		return nil, "", apperror.NewInternal(fmt.Errorf("parsing stored manifest: %w", err))
	}
	signed := s.tokens.Sign(campaignID, tok.TokenVersion)
	manifestURL := fmt.Sprintf("%s/api/v1/campaigns/%s/foundry/module.json?token=%s",
		s.baseURL, campaignID, signed)
	downloadURL := fmt.Sprintf("%s/api/v1/campaigns/%s/foundry/module.zip?token=%s",
		s.baseURL, campaignID, signed)
	m["manifest"] = manifestURL
	m["download"] = downloadURL
	rewritten, err := json.Marshal(m)
	if err != nil {
		return nil, "", apperror.NewInternal(err)
	}
	return rewritten, v.FilePath, nil
}

// --- helpers ---

// extractManifest reads a Foundry module zip and returns the parsed
// module.json plus its raw bytes. Module.json must live at the zip
// root or in the top-level directory (Foundry release zips use both
// shapes in the wild).
func extractManifest(zipBytes []byte) (*ParsedManifest, json.RawMessage, error) {
	r, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, nil, apperror.NewBadRequest("uploaded file is not a valid zip")
	}
	for _, f := range r.File {
		// Match "module.json" at root or one level deep.
		base := filepath.Base(f.Name)
		if base != "module.json" {
			continue
		}
		// Reject paths with traversal so a malicious zip can't trick a
		// future extractor into writing outside storageDir. We don't
		// extract anything but still defensive.
		if strings.Contains(f.Name, "..") {
			return nil, nil, apperror.NewBadRequest("manifest path contains traversal")
		}
		rc, err := f.Open()
		if err != nil {
			return nil, nil, apperror.NewBadRequest("could not open manifest in zip")
		}
		raw, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return nil, nil, apperror.NewBadRequest("could not read manifest in zip")
		}
		var pm ParsedManifest
		if err := json.Unmarshal(raw, &pm); err != nil {
			return nil, nil, apperror.NewBadRequest("module.json is not valid JSON")
		}
		if pm.ID == "" || pm.Version == "" {
			return nil, nil, apperror.NewBadRequest("module.json must include id and version")
		}
		return &pm, raw, nil
	}
	return nil, nil, apperror.NewBadRequest("zip does not contain module.json")
}

// sanitizeFilename strips path separators and other shell-unfriendly
// characters from a version string before it's used as a filename.
// We use the parsed manifest version (not the upload's original
// filename) so the only attack surface is the version string itself.
func sanitizeFilename(s string) string {
	var out strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '.', r == '-', r == '_':
			out.WriteRune(r)
		default:
			// Drop anything else — slashes, control chars, unicode.
		}
	}
	return out.String()
}

func firstNonEmpty(strs ...string) string {
	for _, s := range strs {
		if s != "" {
			return s
		}
	}
	return ""
}

func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
