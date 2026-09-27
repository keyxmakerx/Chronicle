package syncapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"golang.org/x/crypto/bcrypt"
)

// keyBytes is the number of random bytes in a generated API key.
const keyBytes = 32

// keyPrefixLen is the length of the prefix stored for key identification.
const keyPrefixLen = 8

// errAddonGateUnwired is the internal error behind a WebSocket refusal when
// SetAddonGate was never called. A distinct value so the boot/wiring fault is
// distinguishable in logs from a genuine "addon is off" refusal.
var errAddonGateUnwired = errors.New("syncapi: addon gate not wired (SetAddonGate was never called)")

// errMemberCheckerUnwired is the internal error behind a WebSocket refusal
// when SetMemberChecker was never called. A distinct value so the boot/wiring
// fault is distinguishable in logs from a genuine "creator lost access"
// refusal.
var errMemberCheckerUnwired = errors.New("syncapi: membership checker not wired (SetMemberChecker was never called)")

// SyncAPIService handles business logic for the sync API.
type SyncAPIService interface {
	// Key management.
	CreateKey(ctx context.Context, userID string, input CreateAPIKeyInput) (*CreateAPIKeyResult, error)
	GetKey(ctx context.Context, id int) (*APIKey, error)
	ListKeysByUser(ctx context.Context, userID string) ([]APIKey, error)
	ListKeysByCampaign(ctx context.Context, campaignID string) ([]APIKey, error)
	ListAllKeys(ctx context.Context, limit, offset int) ([]APIKey, int, error)
	ActivateKey(ctx context.Context, id int) error
	DeactivateKey(ctx context.Context, id int) error
	RevokeKey(ctx context.Context, id int) error

	// ListCampaignIDsWithKeys returns the distinct campaign IDs that own at
	// least one api_keys row, active or not. Backs ReconcileAddonEnablement.
	ListCampaignIDsWithKeys(ctx context.Context) ([]string, error)

	// SetAddonGate injects the campaign addon state reader/writer. MUST be
	// called during wiring: without it the WebSocket authenticator fails
	// closed (see AuthenticateKeyForWS) and CreateKey cannot record that a
	// campaign now uses the Sync API.
	SetAddonGate(gate SyncAPIAddonGate)

	// SetMemberChecker injects the campaign membership reader used to
	// confirm a stored key's creator is still an Owner before granting a
	// WebSocket connection Owner-level access. MUST be called during
	// wiring: without it AuthenticateKeyForWS fails closed.
	SetMemberChecker(mc MembershipChecker)

	// SetConnectionRevoker injects the hub's revocation surface. Late-bound
	// and nil-safe: the authoritative check is still AuthenticateKeyForWS
	// on reconnect, so a nil revoker only skips the live-drop nicety.
	SetConnectionRevoker(cr ConnectionRevoker)

	// Authentication.
	AuthenticateKey(ctx context.Context, rawKey string) (*APIKey, error)
	UpdateKeyLastUsed(ctx context.Context, id int, ip string) error
	BindDevice(ctx context.Context, keyID int, fingerprint string) error
	UnbindDevice(ctx context.Context, keyID int) error

	// Request logging.
	LogRequest(ctx context.Context, log *APIRequestLog) error
	ListRequestLogs(ctx context.Context, filter RequestLogFilter) ([]APIRequestLog, int, error)
	GetRequestTimeSeries(ctx context.Context, since time.Time, interval string) ([]TimeSeriesPoint, error)
	GetTopIPs(ctx context.Context, since time.Time, limit int) ([]TopEntry, error)
	GetTopPaths(ctx context.Context, since time.Time, limit int) ([]TopEntry, error)
	GetTopKeys(ctx context.Context, since time.Time, limit int) ([]TopEntry, error)

	// Security.
	LogSecurityEvent(ctx context.Context, event *SecurityEvent) error
	ListSecurityEvents(ctx context.Context, filter SecurityEventFilter) ([]SecurityEvent, int, error)
	ResolveSecurityEvent(ctx context.Context, id int64, adminID string) error
	GetSecurityTimeSeries(ctx context.Context, since time.Time) ([]TimeSeriesPoint, error)

	// IP blocklist.
	BlockIP(ctx context.Context, ip, reason, adminID string, expiresAt *time.Time) (*IPBlock, error)
	UnblockIP(ctx context.Context, id int) error
	ListIPBlocks(ctx context.Context) ([]IPBlock, error)
	IsIPBlocked(ctx context.Context, ip string) (bool, error)

	// Statistics.
	GetStats(ctx context.Context, since time.Time) (*APIStats, error)
	GetCampaignStats(ctx context.Context, campaignID string, since time.Time) (*APIStats, error)

	// WebSocket authentication. expiresAt is the key's expiry (nil if it
	// never expires) so the hub can cut the socket off when it passes.
	AuthenticateKeyForWS(ctx context.Context, rawKey string) (campaignID, userID string, role int, expiresAt *time.Time, err error)

	// Calendar date beacon.
	RecordCalendarDateBeacon(ctx context.Context, campaignID string, year, month, day int) error
	GetCalendarDateBeacon(ctx context.Context, campaignID string) (*CalendarDateBeacon, error)
	ConfirmCalendarDate(ctx context.Context, campaignID string, year, month, day int) error
}

// SyncAPIAddonGate is the narrow view of the addons service this plugin
// needs: read whether the campaign's "Sync API" addon is on, and turn it on.
// Declared here (structurally, not as an import of the addons package) for
// the same reason AddonChecker is — syncapi states what it needs and the
// addons service happens to satisfy it.
type SyncAPIAddonGate interface {
	IsEnabledForCampaign(ctx context.Context, campaignID string, addonSlug string) (bool, error)
	EnableForCampaignBySlug(ctx context.Context, campaignID string, addonSlug string, userID string) error
}

// MembershipChecker is the narrow campaigns.CampaignService surface
// AuthenticateKeyForWS needs to confirm a stored key's creator is still a
// campaign Owner — the WebSocket twin of RequireKeyOwnerStillOwner
// (middleware.go), which runs the same check on the REST path.
// campaigns.CampaignService satisfies this structurally.
type MembershipChecker interface {
	GetMember(ctx context.Context, campaignID, userID string) (*campaigns.CampaignMember, error)
}

// ConnectionRevoker is the narrow hub view this plugin needs:
// force-disconnect a campaign's Foundry sockets on a key revoke.
// Declared locally, not imported — same reason as SyncAPIAddonGate.
type ConnectionRevoker interface {
	RevokeAPIKeyClients(campaignID string)
}

// syncAPIService implements SyncAPIService.
type syncAPIService struct {
	repo SyncAPIRepository

	// addonGate reads and writes the campaign's Sync API toggle. Injected
	// after construction (SetAddonGate) rather than taken as a constructor
	// argument only because the addons service is built earlier in the
	// wiring than this one; a nil gate is treated as a wiring fault, not as
	// permission — see AuthenticateKeyForWS.
	addonGate SyncAPIAddonGate

	// memberChecker confirms a stored key's creator is still a campaign
	// Owner before AuthenticateKeyForWS grants Owner-level WS access. Same
	// injected-after-construction, fail-closed-when-nil treatment as
	// addonGate.
	memberChecker MembershipChecker

	// connRevoker drops already-open Foundry sockets on key revoke.
	// Injected after construction; nil means not wired yet, a no-op.
	connRevoker ConnectionRevoker
}

// NewSyncAPIService creates a new sync API service.
func NewSyncAPIService(repo SyncAPIRepository) SyncAPIService {
	return &syncAPIService{repo: repo}
}

// SetAddonGate injects the campaign addon state reader/writer.
func (s *syncAPIService) SetAddonGate(gate SyncAPIAddonGate) {
	s.addonGate = gate
}

// SetMemberChecker injects the campaign membership reader.
func (s *syncAPIService) SetMemberChecker(mc MembershipChecker) {
	s.memberChecker = mc
}

// SetConnectionRevoker injects the WebSocket hub's revocation surface.
func (s *syncAPIService) SetConnectionRevoker(cr ConnectionRevoker) {
	s.connRevoker = cr
}

// --- Key Management ---

// validPermissions enumerates allowed API key permissions.
var validPermissions = map[APIKeyPermission]bool{
	PermRead:  true,
	PermWrite: true,
	PermSync:  true,
}

// CreateKey generates a new API key with bcrypt-hashed storage.
func (s *syncAPIService) CreateKey(ctx context.Context, userID string, input CreateAPIKeyInput) (*CreateAPIKeyResult, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, apperror.NewBadRequest("key name is required")
	}
	if input.CampaignID == "" {
		return nil, apperror.NewBadRequest("campaign ID is required")
	}
	if len(input.Permissions) == 0 {
		return nil, apperror.NewBadRequest("at least one permission is required")
	}
	for _, p := range input.Permissions {
		if !validPermissions[p] {
			return nil, apperror.NewBadRequest(fmt.Sprintf("invalid permission: %s", p))
		}
	}
	if input.RateLimit <= 0 {
		input.RateLimit = 60 // Default.
	}
	if input.RateLimit > 1000 {
		return nil, apperror.NewBadRequest("rate limit cannot exceed 1000 requests per minute")
	}

	// Generate random key.
	raw := make([]byte, keyBytes)
	if _, err := rand.Read(raw); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("generating key: %w", err))
	}
	rawKey := "chron_" + hex.EncodeToString(raw)
	prefix := rawKey[:keyPrefixLen]

	// Hash for storage.
	hash, err := bcrypt.GenerateFromPassword([]byte(rawKey), bcrypt.DefaultCost)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("hashing key: %w", err))
	}

	var vttTag *string
	if tag := strings.TrimSpace(input.VTTTag); tag != "" {
		vttTag = &tag
	}

	key := &APIKey{
		KeyHash:     string(hash),
		KeyPrefix:   prefix,
		Name:        name,
		VTTTag:      vttTag,
		UserID:      userID,
		CampaignID:  input.CampaignID,
		Permissions: input.Permissions,
		IPAllowlist: input.IPAllowlist,
		RateLimit:   input.RateLimit,
		IsActive:    true,
		ExpiresAt:   input.ExpiresAt,
	}

	if err := s.repo.CreateKey(ctx, key); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("creating key: %w", err))
	}

	slog.Info("api key created",
		slog.String("prefix", prefix),
		slog.String("user_id", userID),
		slog.String("campaign_id", input.CampaignID),
	)

	// Minting a Bearer token for an outside client IS the affirmative act of
	// turning external access on, so the create path enables the Sync API
	// addon immediately rather than leaving the new key dead until
	// ReconcileAddonEnablement's next pass. This does re-enable a toggle the
	// owner may have switched off; that's deliberate (they're on the API
	// keys screen asking for a credential right now) and logged. Nothing
	// else re-enables it — switching it off after the fact stays off until
	// another key is created.
	//
	// Best-effort on failure: the key row is already committed and its
	// plaintext is shown exactly once, so returning an error here would
	// destroy a credential the caller cannot recover. Logged at ERROR with
	// the remedy instead.
	if s.addonGate != nil {
		if err := s.addonGate.EnableForCampaignBySlug(ctx, input.CampaignID, SyncAPIAddonSlug, userID); err != nil {
			slog.Error("api key created but the Sync API addon could not be enabled; "+
				"the key will be refused until an owner enables Sync API on the campaign's Extensions page (sidebar → Extensions)",
				slog.String("campaign_id", input.CampaignID),
				slog.String("prefix", prefix),
				slog.Any("error", err),
			)
		}
	} else {
		slog.Error("api key created with no addon gate wired; the Sync API addon state "+
			"was not updated and the key may be refused",
			slog.String("campaign_id", input.CampaignID),
			slog.String("prefix", prefix),
		)
	}

	return &CreateAPIKeyResult{Key: key, RawKey: rawKey}, nil
}

// GetKey retrieves an API key by ID.
func (s *syncAPIService) GetKey(ctx context.Context, id int) (*APIKey, error) {
	return s.repo.FindKeyByID(ctx, id)
}

// ListKeysByUser returns all keys owned by a user.
func (s *syncAPIService) ListKeysByUser(ctx context.Context, userID string) ([]APIKey, error) {
	return s.repo.ListKeysByUser(ctx, userID)
}

// ListKeysByCampaign returns all keys for a campaign.
func (s *syncAPIService) ListKeysByCampaign(ctx context.Context, campaignID string) ([]APIKey, error) {
	return s.repo.ListKeysByCampaign(ctx, campaignID)
}

// ListCampaignIDsWithKeys returns the distinct campaign IDs owning at least
// one API key. Deactivated and expired keys count: a deactivated key can be
// switched back on, so the campaign is still one that uses the Sync API.
func (s *syncAPIService) ListCampaignIDsWithKeys(ctx context.Context) ([]string, error) {
	return s.repo.ListCampaignIDsWithKeys(ctx)
}

// maxListLimit caps admin list pagination so a caller can't force a huge
// query/allocation via an oversized limit.
const maxListLimit = 200

// ListAllKeys returns all API keys with pagination (admin).
func (s *syncAPIService) ListAllKeys(ctx context.Context, limit, offset int) ([]APIKey, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}
	return s.repo.ListAllKeys(ctx, limit, offset)
}

// ActivateKey enables an API key.
func (s *syncAPIService) ActivateKey(ctx context.Context, id int) error {
	if err := s.repo.UpdateKeyActive(ctx, id, true); err != nil {
		return err
	}
	slog.Info("api key activated", slog.Int("id", id))
	return nil
}

// DeactivateKey disables a key without deleting it, and drops its
// already-open Foundry sockets — same treatment as RevokeKey, since
// AuthenticateKey only refuses a deactivated key on the next reconnect.
func (s *syncAPIService) DeactivateKey(ctx context.Context, id int) error {
	// Looked up before deactivate so we still know which campaign to drop
	// sockets for afterward — same best-effort treatment as RevokeKey: a
	// lookup failure only costs the live-drop nicety, not the deactivate.
	var campaignID string
	if key, err := s.repo.FindKeyByID(ctx, id); err == nil && key != nil {
		campaignID = key.CampaignID
	}

	if err := s.repo.UpdateKeyActive(ctx, id, false); err != nil {
		return err
	}

	if s.connRevoker != nil && campaignID != "" {
		s.connRevoker.RevokeAPIKeyClients(campaignID)
	}

	slog.Info("api key deactivated", slog.Int("id", id))
	return nil
}

// RevokeKey permanently deletes a key and drops its already-open Foundry
// sockets — the forced drop is what makes the revoke take effect
// immediately, rather than on the socket's next reconnect.
func (s *syncAPIService) RevokeKey(ctx context.Context, id int) error {
	// Looked up before delete so we still know which campaign to drop
	// sockets for afterward. A lookup failure only costs the live-drop
	// nicety, not the revoke itself — the key is deleted regardless.
	var campaignID string
	if key, err := s.repo.FindKeyByID(ctx, id); err == nil && key != nil {
		campaignID = key.CampaignID
	}

	if err := s.repo.DeleteKey(ctx, id); err != nil {
		return err
	}

	if s.connRevoker != nil && campaignID != "" {
		s.connRevoker.RevokeAPIKeyClients(campaignID)
	}

	slog.Info("api key revoked", slog.Int("id", id))
	return nil
}

// AuthenticateKey validates a raw API key and returns the associated key record.
// It extracts the prefix, looks up the key, and verifies with bcrypt.
//
// This is the single source of truth for API key validation. Both the REST
// middleware (RequireAPIKey) and the WebSocket authenticator call through
// here — the token-to-key translation is shared so they cannot diverge.
//
// The raw key is trimmed of surrounding whitespace before processing to
// defensively handle clients that append stray whitespace or line endings
// when copying tokens into config files.
//
// Failure paths log at debug level with the truncated prefix so admins can
// diagnose "REST accepts, WS rejects" style issues without exposing full
// tokens. The public error returned is always the same opaque
// "invalid api key" to avoid leaking which keys exist.
func (s *syncAPIService) AuthenticateKey(ctx context.Context, rawKey string) (*APIKey, error) {
	rawKey = strings.TrimSpace(rawKey)
	if len(rawKey) < keyPrefixLen {
		slog.Debug("api key auth failed",
			slog.String("reason", "token too short"),
			slog.Int("length", len(rawKey)),
		)
		return nil, apperror.NewBadRequest("invalid api key format")
	}

	prefix := rawKey[:keyPrefixLen]
	key, err := s.repo.FindKeyByPrefix(ctx, prefix)
	if err != nil {
		slog.Debug("api key auth failed",
			slog.String("reason", "prefix not found"),
			slog.String("prefix", prefix),
			slog.Any("error", err),
		)
		return nil, apperror.NewForbidden("invalid api key")
	}

	// Verify the full key against the stored hash.
	if err := bcrypt.CompareHashAndPassword([]byte(key.KeyHash), []byte(rawKey)); err != nil {
		slog.Debug("api key auth failed",
			slog.String("reason", "bcrypt mismatch"),
			slog.String("prefix", prefix),
			slog.Int("key_id", key.ID),
		)
		return nil, apperror.NewForbidden("invalid api key")
	}

	// Check if the key is active.
	if !key.IsActive {
		slog.Debug("api key auth failed",
			slog.String("reason", "deactivated"),
			slog.Int("key_id", key.ID),
		)
		return nil, apperror.NewForbidden("api key is deactivated")
	}

	// Check expiry.
	if key.IsExpired() {
		slog.Debug("api key auth failed",
			slog.String("reason", "expired"),
			slog.Int("key_id", key.ID),
		)
		return nil, apperror.NewForbidden("api key has expired")
	}

	return key, nil
}

// UpdateKeyLastUsed records the last-used timestamp and IP for an API key.
func (s *syncAPIService) UpdateKeyLastUsed(ctx context.Context, id int, ip string) error {
	return s.repo.UpdateKeyLastUsed(ctx, id, ip)
}

// BindDevice records a device fingerprint on an API key. Once bound, only
// requests from this device are accepted. The fingerprint is a client-provided
// opaque string (typically a hash of hardware/software identifiers).
func (s *syncAPIService) BindDevice(ctx context.Context, keyID int, fingerprint string) error {
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return apperror.NewBadRequest("device fingerprint is required")
	}
	now := time.Now().UTC()
	return s.repo.BindDevice(ctx, keyID, fingerprint, now)
}

// UnbindDevice removes device binding from an API key, allowing re-registration.
func (s *syncAPIService) UnbindDevice(ctx context.Context, keyID int) error {
	return s.repo.UnbindDevice(ctx, keyID)
}

// --- Request Logging ---

// LogRequest records an API request.
func (s *syncAPIService) LogRequest(ctx context.Context, log *APIRequestLog) error {
	if err := s.repo.LogRequest(ctx, log); err != nil {
		// Log errors are non-critical — don't fail the request.
		slog.Warn("failed to log api request", slog.Any("error", err))
	}
	return nil
}

// ListRequestLogs returns filtered request logs.
func (s *syncAPIService) ListRequestLogs(ctx context.Context, filter RequestLogFilter) ([]APIRequestLog, int, error) {
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	if filter.Limit > maxListLimit {
		filter.Limit = maxListLimit
	}
	return s.repo.ListRequestLogs(ctx, filter)
}

// GetRequestTimeSeries returns request counts bucketed by interval.
func (s *syncAPIService) GetRequestTimeSeries(ctx context.Context, since time.Time, interval string) ([]TimeSeriesPoint, error) {
	return s.repo.GetRequestTimeSeries(ctx, since, interval)
}

// GetTopIPs returns the most active IPs.
func (s *syncAPIService) GetTopIPs(ctx context.Context, since time.Time, limit int) ([]TopEntry, error) {
	if limit <= 0 {
		limit = 10
	}
	return s.repo.GetTopIPs(ctx, since, limit)
}

// GetTopPaths returns the most requested paths.
func (s *syncAPIService) GetTopPaths(ctx context.Context, since time.Time, limit int) ([]TopEntry, error) {
	if limit <= 0 {
		limit = 10
	}
	return s.repo.GetTopPaths(ctx, since, limit)
}

// GetTopKeys returns the most active keys.
func (s *syncAPIService) GetTopKeys(ctx context.Context, since time.Time, limit int) ([]TopEntry, error) {
	if limit <= 0 {
		limit = 10
	}
	return s.repo.GetTopKeys(ctx, since, limit)
}

// --- Security ---

// LogSecurityEvent records a security event.
func (s *syncAPIService) LogSecurityEvent(ctx context.Context, event *SecurityEvent) error {
	if err := s.repo.LogSecurityEvent(ctx, event); err != nil {
		slog.Warn("failed to log security event", slog.Any("error", err))
	}
	return nil
}

// ListSecurityEvents returns filtered security events.
func (s *syncAPIService) ListSecurityEvents(ctx context.Context, filter SecurityEventFilter) ([]SecurityEvent, int, error) {
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	return s.repo.ListSecurityEvents(ctx, filter)
}

// ResolveSecurityEvent marks an event as resolved.
func (s *syncAPIService) ResolveSecurityEvent(ctx context.Context, id int64, adminID string) error {
	if err := s.repo.ResolveSecurityEvent(ctx, id, adminID); err != nil {
		return err
	}
	slog.Info("security event resolved",
		slog.Int64("event_id", id),
		slog.String("admin_id", adminID),
	)
	return nil
}

// GetSecurityTimeSeries returns security event counts by hour.
func (s *syncAPIService) GetSecurityTimeSeries(ctx context.Context, since time.Time) ([]TimeSeriesPoint, error) {
	return s.repo.GetSecurityTimeSeries(ctx, since)
}

// --- IP Blocklist ---

// BlockIP adds an IP to the blocklist.
func (s *syncAPIService) BlockIP(ctx context.Context, ip, reason, adminID string, expiresAt *time.Time) (*IPBlock, error) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return nil, apperror.NewBadRequest("ip address is required")
	}

	block := &IPBlock{
		IPAddress: ip,
		BlockedBy: adminID,
		ExpiresAt: expiresAt,
	}
	if r := strings.TrimSpace(reason); r != "" {
		block.Reason = &r
	}

	if err := s.repo.AddIPBlock(ctx, block); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("blocking ip: %w", err))
	}

	slog.Info("ip blocked",
		slog.String("ip", ip),
		slog.String("admin_id", adminID),
	)
	return block, nil
}

// UnblockIP removes an IP from the blocklist.
func (s *syncAPIService) UnblockIP(ctx context.Context, id int) error {
	if err := s.repo.RemoveIPBlock(ctx, id); err != nil {
		return err
	}
	slog.Info("ip unblocked", slog.Int("id", id))
	return nil
}

// ListIPBlocks returns all blocked IPs.
func (s *syncAPIService) ListIPBlocks(ctx context.Context) ([]IPBlock, error) {
	return s.repo.ListIPBlocks(ctx)
}

// IsIPBlocked checks if an IP is currently blocked.
func (s *syncAPIService) IsIPBlocked(ctx context.Context, ip string) (bool, error) {
	return s.repo.IsIPBlocked(ctx, ip)
}

// --- Statistics ---

// GetStats returns aggregated API stats.
func (s *syncAPIService) GetStats(ctx context.Context, since time.Time) (*APIStats, error) {
	return s.repo.GetStats(ctx, since)
}

// GetCampaignStats returns API stats scoped to a campaign.
func (s *syncAPIService) GetCampaignStats(ctx context.Context, campaignID string, since time.Time) (*APIStats, error) {
	return s.repo.GetCampaignStats(ctx, campaignID, since)
}

// --- WebSocket Authentication ---

// AuthenticateKeyForWS validates a raw key and returns its campaign, owner
// user ID, Owner role (confirmed live, not just stored), and expiry — the
// WS twin of the REST gate RequireKeyOwnerStillOwner.
func (s *syncAPIService) AuthenticateKeyForWS(ctx context.Context, rawKey string) (campaignID, userID string, role int, expiresAt *time.Time, err error) {
	key, err := s.AuthenticateKey(ctx, rawKey)
	if err != nil {
		return "", "", 0, nil, err
	}

	// The campaign's "Sync API" toggle governs the push channel exactly as it
	// governs REST. The WS upgrade never passes through Echo's /api/v1 chain,
	// so the gate cannot live in middleware for this path; it lives here
	// rather than inside AuthenticateKey so that method keeps answering one
	// question ("is this token a live key?") and each transport can shape its
	// own refusal. Folding it into AuthenticateKey would surface as a 401
	// "invalid api key" on REST, which is a lie: the key is valid.
	//
	// A nil gate is a wiring fault, and a security control that silently
	// no-ops when unwired is the defect class this change exists to remove —
	// so it refuses, loudly, rather than assuming permission.
	if s.addonGate == nil {
		slog.Error("websocket api key auth refused: addon gate not wired",
			slog.String("campaign_id", key.CampaignID),
			slog.Int("key_id", key.ID),
		)
		return "", "", 0, nil, apperror.NewInternal(errAddonGateUnwired)
	}
	enabled, gateErr := s.addonGate.IsEnabledForCampaign(ctx, key.CampaignID, SyncAPIAddonSlug)
	if gateErr != nil {
		// Fail closed, same as the REST gate: an unreadable addon state is
		// not permission.
		slog.Error("websocket api key auth: sync-api addon check failed",
			slog.String("campaign_id", key.CampaignID),
			slog.Int("key_id", key.ID),
			slog.Any("error", gateErr),
		)
		return "", "", 0, nil, apperror.NewInternalMessage("temporarily unable to verify addon status", gateErr)
	}
	if !enabled {
		return "", "", 0, nil, syncAPIDisabledError()
	}

	// A sync key must never carry more power than its creator currently has:
	// confirm key.UserID is still an Owner of key.CampaignID before granting
	// Owner-level WS access — the WebSocket twin of the REST gate
	// (RequireKeyOwnerStillOwner). A nil checker is a wiring fault, not
	// permission — same fail-closed treatment as a nil addon gate above.
	if s.memberChecker == nil {
		slog.Error("websocket api key auth refused: membership checker not wired",
			slog.String("campaign_id", key.CampaignID),
			slog.Int("key_id", key.ID),
		)
		return "", "", 0, nil, apperror.NewInternal(errMemberCheckerUnwired)
	}
	member, memErr := s.memberChecker.GetMember(ctx, key.CampaignID, key.UserID)
	if memErr != nil || member.Role < campaigns.RoleOwner {
		return "", "", 0, nil, keyOwnerLostAccessError()
	}

	// key.UserID is confirmed Owner, so the WS session gets Owner role.
	return key.CampaignID, key.UserID, int(campaigns.RoleOwner), key.ExpiresAt, nil
}

// --- Calendar Date Beacon ---

// calendarDateBeaconThrottle is the minimum gap between beacon writes for
// an unchanged date. GetCurrentDate is polled repeatedly (every push, plus
// on sync); without throttling, a live Foundry session would write this row
// on nearly every poll.
const calendarDateBeaconThrottle = 60 * time.Second

// RecordCalendarDateBeacon records the date a Bearer-authed module read via
// GET /calendar/date. Write-throttled: skipped when the last beacon for
// this campaign is under calendarDateBeaconThrottle old AND the date is
// unchanged, so a live polling session doesn't hammer the table. A changed
// date always writes immediately regardless of throttle age — the chip
// needs the fresher date, not a stale throttled one.
//
// Callers MUST verify the caller is a real Bearer-authed module (API key
// ID != synthKeySessionID) before calling this — see calendar_api_handler.go
// GetCurrentDate. A member browsing Chronicle's own calendar over the
// session-auth path on the same endpoint must never beacon.
func (s *syncAPIService) RecordCalendarDateBeacon(ctx context.Context, campaignID string, year, month, day int) error {
	existing, err := s.repo.GetCalendarDateBeacon(ctx, campaignID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if existing != nil &&
		existing.Year == year && existing.Month == month && existing.Day == day &&
		now.Sub(existing.ServedAt) < calendarDateBeaconThrottle {
		return nil
	}
	return s.repo.UpsertCalendarDateBeacon(ctx, &CalendarDateBeacon{
		CampaignID: campaignID,
		Year:       year,
		Month:      month,
		Day:        day,
		ServedAt:   now,
	})
}

// GetCalendarDateBeacon returns the campaign's served-date beacon (nil, nil
// if none recorded yet). No auth restriction here beyond whatever the
// caller's route already enforces (member-read).
func (s *syncAPIService) GetCalendarDateBeacon(ctx context.Context, campaignID string) (*CalendarDateBeacon, error) {
	return s.repo.GetCalendarDateBeacon(ctx, campaignID)
}

// ConfirmCalendarDate records the date a Bearer-authed module actually
// APPLIED to its own calendar (POST /calendar/date/confirm), upgrading the
// beacon from "saw" (RecordCalendarDateBeacon's served-date write) to
// "applied". Unlike RecordCalendarDateBeacon this is never throttled: a
// confirm is a deliberate, one-shot module action, not a value read on
// every poll.
//
// Callers MUST verify the caller is a real Bearer-authed module (API key
// ID != synthKeySessionID) before calling this — see
// calendar_api_handler.go ConfirmDate, which mirrors GetCurrentDate's
// auth gate for the same reason: a member browsing Chronicle's own
// calendar over the session-auth path on this dual-auth route must never
// write a confirmation on the module's behalf.
func (s *syncAPIService) ConfirmCalendarDate(ctx context.Context, campaignID string, year, month, day int) error {
	return s.repo.ConfirmCalendarDateBeacon(ctx, campaignID, year, month, day, time.Now().UTC())
}
