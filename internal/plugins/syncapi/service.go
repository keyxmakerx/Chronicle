package syncapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"golang.org/x/crypto/bcrypt"
)

// keyBytes is the number of random bytes in a generated API key.
const keyBytes = 32

// keyPrefixLen is the length of the prefix stored for key identification.
const keyPrefixLen = 8

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

	// Authentication.
	AuthenticateKey(ctx context.Context, rawKey string) (*APIKey, error)

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
}

// syncAPIService implements SyncAPIService.
type syncAPIService struct {
	repo SyncAPIRepository
}

// NewSyncAPIService creates a new sync API service.
func NewSyncAPIService(repo SyncAPIRepository) SyncAPIService {
	return &syncAPIService{repo: repo}
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

	key := &APIKey{
		KeyHash:     string(hash),
		KeyPrefix:   prefix,
		Name:        name,
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

// ListAllKeys returns all API keys with pagination (admin).
func (s *syncAPIService) ListAllKeys(ctx context.Context, limit, offset int) ([]APIKey, int, error) {
	if limit <= 0 {
		limit = 50
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

// DeactivateKey disables an API key without deleting it.
func (s *syncAPIService) DeactivateKey(ctx context.Context, id int) error {
	if err := s.repo.UpdateKeyActive(ctx, id, false); err != nil {
		return err
	}
	slog.Info("api key deactivated", slog.Int("id", id))
	return nil
}

// RevokeKey permanently deletes an API key.
func (s *syncAPIService) RevokeKey(ctx context.Context, id int) error {
	if err := s.repo.DeleteKey(ctx, id); err != nil {
		return err
	}
	slog.Info("api key revoked", slog.Int("id", id))
	return nil
}

// AuthenticateKey validates a raw API key and returns the associated key record.
// It extracts the prefix, looks up the key, and verifies with bcrypt.
func (s *syncAPIService) AuthenticateKey(ctx context.Context, rawKey string) (*APIKey, error) {
	if len(rawKey) < keyPrefixLen {
		return nil, apperror.NewBadRequest("invalid api key format")
	}

	prefix := rawKey[:keyPrefixLen]
	key, err := s.repo.FindKeyByPrefix(ctx, prefix)
	if err != nil {
		return nil, apperror.NewForbidden("invalid api key")
	}

	// Verify the full key against the stored hash.
	if err := bcrypt.CompareHashAndPassword([]byte(key.KeyHash), []byte(rawKey)); err != nil {
		return nil, apperror.NewForbidden("invalid api key")
	}

	// Check if the key is active.
	if !key.IsActive {
		return nil, apperror.NewForbidden("api key is deactivated")
	}

	// Check expiry.
	if key.IsExpired() {
		return nil, apperror.NewForbidden("api key has expired")
	}

	return key, nil
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
