package syncapi

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"golang.org/x/crypto/bcrypt"
)

// --- Mock Repository ---

// mockSyncAPIRepo implements SyncAPIRepository for testing.
type mockSyncAPIRepo struct {
	createKeyFn           func(ctx context.Context, key *APIKey) error
	findKeyByIDFn         func(ctx context.Context, id int) (*APIKey, error)
	findKeyByPrefixFn     func(ctx context.Context, prefix string) (*APIKey, error)
	listKeysByUserFn      func(ctx context.Context, userID string) ([]APIKey, error)
	listKeysByCampaignFn  func(ctx context.Context, campaignID string) ([]APIKey, error)
	listAllKeysFn         func(ctx context.Context, limit, offset int) ([]APIKey, int, error)
	updateKeyActiveFn     func(ctx context.Context, id int, active bool) error
	updateKeyLastUsedFn   func(ctx context.Context, id int, ip string) error
	deleteKeyFn           func(ctx context.Context, id int) error
	logRequestFn          func(ctx context.Context, log *APIRequestLog) error
	listRequestLogsFn     func(ctx context.Context, filter RequestLogFilter) ([]APIRequestLog, int, error)
	getReqTimeSeriesFn    func(ctx context.Context, since time.Time, interval string) ([]TimeSeriesPoint, error)
	getTopIPsFn           func(ctx context.Context, since time.Time, limit int) ([]TopEntry, error)
	getTopPathsFn         func(ctx context.Context, since time.Time, limit int) ([]TopEntry, error)
	getTopKeysFn          func(ctx context.Context, since time.Time, limit int) ([]TopEntry, error)
	logSecurityEventFn    func(ctx context.Context, event *SecurityEvent) error
	listSecurityEventsFn  func(ctx context.Context, filter SecurityEventFilter) ([]SecurityEvent, int, error)
	resolveSecurityEvtFn  func(ctx context.Context, id int64, adminID string) error
	getSecTimeSeriesFn    func(ctx context.Context, since time.Time) ([]TimeSeriesPoint, error)
	addIPBlockFn          func(ctx context.Context, block *IPBlock) error
	removeIPBlockFn       func(ctx context.Context, id int) error
	listIPBlocksFn        func(ctx context.Context) ([]IPBlock, error)
	isIPBlockedFn         func(ctx context.Context, ip string) (bool, error)
	getStatsFn            func(ctx context.Context, since time.Time) (*APIStats, error)
	getCampaignStatsFn    func(ctx context.Context, campaignID string, since time.Time) (*APIStats, error)
}

func (m *mockSyncAPIRepo) CreateKey(ctx context.Context, key *APIKey) error {
	if m.createKeyFn != nil {
		return m.createKeyFn(ctx, key)
	}
	key.ID = 1
	return nil
}

func (m *mockSyncAPIRepo) FindKeyByID(ctx context.Context, id int) (*APIKey, error) {
	if m.findKeyByIDFn != nil {
		return m.findKeyByIDFn(ctx, id)
	}
	return nil, apperror.NewNotFound("key not found")
}

func (m *mockSyncAPIRepo) FindKeyByPrefix(ctx context.Context, prefix string) (*APIKey, error) {
	if m.findKeyByPrefixFn != nil {
		return m.findKeyByPrefixFn(ctx, prefix)
	}
	return nil, apperror.NewNotFound("key not found")
}

func (m *mockSyncAPIRepo) ListKeysByUser(ctx context.Context, userID string) ([]APIKey, error) {
	if m.listKeysByUserFn != nil {
		return m.listKeysByUserFn(ctx, userID)
	}
	return nil, nil
}

func (m *mockSyncAPIRepo) ListKeysByCampaign(ctx context.Context, campaignID string) ([]APIKey, error) {
	if m.listKeysByCampaignFn != nil {
		return m.listKeysByCampaignFn(ctx, campaignID)
	}
	return nil, nil
}

func (m *mockSyncAPIRepo) ListAllKeys(ctx context.Context, limit, offset int) ([]APIKey, int, error) {
	if m.listAllKeysFn != nil {
		return m.listAllKeysFn(ctx, limit, offset)
	}
	return nil, 0, nil
}

func (m *mockSyncAPIRepo) UpdateKeyActive(ctx context.Context, id int, active bool) error {
	if m.updateKeyActiveFn != nil {
		return m.updateKeyActiveFn(ctx, id, active)
	}
	return nil
}

func (m *mockSyncAPIRepo) UpdateKeyLastUsed(ctx context.Context, id int, ip string) error {
	if m.updateKeyLastUsedFn != nil {
		return m.updateKeyLastUsedFn(ctx, id, ip)
	}
	return nil
}

func (m *mockSyncAPIRepo) DeleteKey(ctx context.Context, id int) error {
	if m.deleteKeyFn != nil {
		return m.deleteKeyFn(ctx, id)
	}
	return nil
}

func (m *mockSyncAPIRepo) LogRequest(ctx context.Context, log *APIRequestLog) error {
	if m.logRequestFn != nil {
		return m.logRequestFn(ctx, log)
	}
	return nil
}

func (m *mockSyncAPIRepo) ListRequestLogs(ctx context.Context, filter RequestLogFilter) ([]APIRequestLog, int, error) {
	if m.listRequestLogsFn != nil {
		return m.listRequestLogsFn(ctx, filter)
	}
	return nil, 0, nil
}

func (m *mockSyncAPIRepo) GetRequestTimeSeries(ctx context.Context, since time.Time, interval string) ([]TimeSeriesPoint, error) {
	if m.getReqTimeSeriesFn != nil {
		return m.getReqTimeSeriesFn(ctx, since, interval)
	}
	return nil, nil
}

func (m *mockSyncAPIRepo) GetTopIPs(ctx context.Context, since time.Time, limit int) ([]TopEntry, error) {
	if m.getTopIPsFn != nil {
		return m.getTopIPsFn(ctx, since, limit)
	}
	return nil, nil
}

func (m *mockSyncAPIRepo) GetTopPaths(ctx context.Context, since time.Time, limit int) ([]TopEntry, error) {
	if m.getTopPathsFn != nil {
		return m.getTopPathsFn(ctx, since, limit)
	}
	return nil, nil
}

func (m *mockSyncAPIRepo) GetTopKeys(ctx context.Context, since time.Time, limit int) ([]TopEntry, error) {
	if m.getTopKeysFn != nil {
		return m.getTopKeysFn(ctx, since, limit)
	}
	return nil, nil
}

func (m *mockSyncAPIRepo) LogSecurityEvent(ctx context.Context, event *SecurityEvent) error {
	if m.logSecurityEventFn != nil {
		return m.logSecurityEventFn(ctx, event)
	}
	return nil
}

func (m *mockSyncAPIRepo) ListSecurityEvents(ctx context.Context, filter SecurityEventFilter) ([]SecurityEvent, int, error) {
	if m.listSecurityEventsFn != nil {
		return m.listSecurityEventsFn(ctx, filter)
	}
	return nil, 0, nil
}

func (m *mockSyncAPIRepo) ResolveSecurityEvent(ctx context.Context, id int64, adminID string) error {
	if m.resolveSecurityEvtFn != nil {
		return m.resolveSecurityEvtFn(ctx, id, adminID)
	}
	return nil
}

func (m *mockSyncAPIRepo) GetSecurityTimeSeries(ctx context.Context, since time.Time) ([]TimeSeriesPoint, error) {
	if m.getSecTimeSeriesFn != nil {
		return m.getSecTimeSeriesFn(ctx, since)
	}
	return nil, nil
}

func (m *mockSyncAPIRepo) AddIPBlock(ctx context.Context, block *IPBlock) error {
	if m.addIPBlockFn != nil {
		return m.addIPBlockFn(ctx, block)
	}
	block.ID = 1
	return nil
}

func (m *mockSyncAPIRepo) RemoveIPBlock(ctx context.Context, id int) error {
	if m.removeIPBlockFn != nil {
		return m.removeIPBlockFn(ctx, id)
	}
	return nil
}

func (m *mockSyncAPIRepo) ListIPBlocks(ctx context.Context) ([]IPBlock, error) {
	if m.listIPBlocksFn != nil {
		return m.listIPBlocksFn(ctx)
	}
	return nil, nil
}

func (m *mockSyncAPIRepo) IsIPBlocked(ctx context.Context, ip string) (bool, error) {
	if m.isIPBlockedFn != nil {
		return m.isIPBlockedFn(ctx, ip)
	}
	return false, nil
}

func (m *mockSyncAPIRepo) GetStats(ctx context.Context, since time.Time) (*APIStats, error) {
	if m.getStatsFn != nil {
		return m.getStatsFn(ctx, since)
	}
	return &APIStats{}, nil
}

func (m *mockSyncAPIRepo) GetCampaignStats(ctx context.Context, campaignID string, since time.Time) (*APIStats, error) {
	if m.getCampaignStatsFn != nil {
		return m.getCampaignStatsFn(ctx, campaignID, since)
	}
	return &APIStats{}, nil
}

func (m *mockSyncAPIRepo) BindDevice(ctx context.Context, keyID int, fingerprint string, boundAt time.Time) error {
	return nil
}

func (m *mockSyncAPIRepo) UnbindDevice(ctx context.Context, keyID int) error {
	return nil
}

// --- Test Helpers ---

// assertAppError checks that err is an *apperror.AppError with the expected code.
func assertAppError(t *testing.T, err error, expectedCode int) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %d, got nil", expectedCode)
	}
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *apperror.AppError, got %T: %v", err, err)
	}
	if appErr.Code != expectedCode {
		t.Errorf("expected status %d, got %d (message: %s)", expectedCode, appErr.Code, appErr.Message)
	}
}

// --- CreateKey Tests ---

func TestCreateKey_Success(t *testing.T) {
	var storedKey *APIKey
	repo := &mockSyncAPIRepo{
		createKeyFn: func(ctx context.Context, key *APIKey) error {
			storedKey = key
			key.ID = 42
			return nil
		},
	}

	svc := NewSyncAPIService(repo)
	result, err := svc.CreateKey(context.Background(), "user-1", CreateAPIKeyInput{
		Name:        "Test Key",
		CampaignID:  "camp-1",
		Permissions: []APIKeyPermission{PermRead, PermWrite},
		RateLimit:   100,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Raw key should start with "chron_" prefix.
	if !strings.HasPrefix(result.RawKey, "chron_") {
		t.Errorf("expected raw key to start with chron_, got %s", result.RawKey[:10])
	}

	// Key should be stored with bcrypt hash.
	if storedKey.KeyHash == "" {
		t.Error("expected key hash to be set")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(storedKey.KeyHash), []byte(result.RawKey)); err != nil {
		t.Error("expected bcrypt hash to match raw key")
	}

	// Prefix should be first 8 chars of raw key.
	if storedKey.KeyPrefix != result.RawKey[:keyPrefixLen] {
		t.Errorf("expected prefix %s, got %s", result.RawKey[:keyPrefixLen], storedKey.KeyPrefix)
	}

	// Should be active by default.
	if !storedKey.IsActive {
		t.Error("expected key to be active")
	}

	if result.Key.ID != 42 {
		t.Errorf("expected ID 42, got %d", result.Key.ID)
	}
}

func TestCreateKey_EmptyName(t *testing.T) {
	svc := NewSyncAPIService(&mockSyncAPIRepo{})
	_, err := svc.CreateKey(context.Background(), "user-1", CreateAPIKeyInput{
		Name:        "",
		CampaignID:  "camp-1",
		Permissions: []APIKeyPermission{PermRead},
	})
	assertAppError(t, err, 400)
}

func TestCreateKey_EmptyCampaignID(t *testing.T) {
	svc := NewSyncAPIService(&mockSyncAPIRepo{})
	_, err := svc.CreateKey(context.Background(), "user-1", CreateAPIKeyInput{
		Name:        "Test",
		CampaignID:  "",
		Permissions: []APIKeyPermission{PermRead},
	})
	assertAppError(t, err, 400)
}

func TestCreateKey_NoPermissions(t *testing.T) {
	svc := NewSyncAPIService(&mockSyncAPIRepo{})
	_, err := svc.CreateKey(context.Background(), "user-1", CreateAPIKeyInput{
		Name:        "Test",
		CampaignID:  "camp-1",
		Permissions: []APIKeyPermission{},
	})
	assertAppError(t, err, 400)
}

func TestCreateKey_InvalidPermission(t *testing.T) {
	svc := NewSyncAPIService(&mockSyncAPIRepo{})
	_, err := svc.CreateKey(context.Background(), "user-1", CreateAPIKeyInput{
		Name:        "Test",
		CampaignID:  "camp-1",
		Permissions: []APIKeyPermission{"admin"},
	})
	assertAppError(t, err, 400)
}

func TestCreateKey_DefaultRateLimit(t *testing.T) {
	var capturedKey *APIKey
	repo := &mockSyncAPIRepo{
		createKeyFn: func(ctx context.Context, key *APIKey) error {
			capturedKey = key
			key.ID = 1
			return nil
		},
	}

	svc := NewSyncAPIService(repo)
	_, err := svc.CreateKey(context.Background(), "user-1", CreateAPIKeyInput{
		Name:        "Test",
		CampaignID:  "camp-1",
		Permissions: []APIKeyPermission{PermRead},
		RateLimit:   0, // Should default to 60.
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedKey.RateLimit != 60 {
		t.Errorf("expected default rate limit 60, got %d", capturedKey.RateLimit)
	}
}

func TestCreateKey_RateLimitTooHigh(t *testing.T) {
	svc := NewSyncAPIService(&mockSyncAPIRepo{})
	_, err := svc.CreateKey(context.Background(), "user-1", CreateAPIKeyInput{
		Name:        "Test",
		CampaignID:  "camp-1",
		Permissions: []APIKeyPermission{PermRead},
		RateLimit:   1001,
	})
	assertAppError(t, err, 400)
}

func TestCreateKey_RepoError(t *testing.T) {
	repo := &mockSyncAPIRepo{
		createKeyFn: func(ctx context.Context, key *APIKey) error {
			return errors.New("db error")
		},
	}

	svc := NewSyncAPIService(repo)
	_, err := svc.CreateKey(context.Background(), "user-1", CreateAPIKeyInput{
		Name:        "Test",
		CampaignID:  "camp-1",
		Permissions: []APIKeyPermission{PermRead},
	})
	assertAppError(t, err, 500)
}

func TestCreateKey_NameTrimming(t *testing.T) {
	var capturedName string
	repo := &mockSyncAPIRepo{
		createKeyFn: func(ctx context.Context, key *APIKey) error {
			capturedName = key.Name
			key.ID = 1
			return nil
		},
	}

	svc := NewSyncAPIService(repo)
	_, err := svc.CreateKey(context.Background(), "user-1", CreateAPIKeyInput{
		Name:        "  My Key  ",
		CampaignID:  "camp-1",
		Permissions: []APIKeyPermission{PermRead},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedName != "My Key" {
		t.Errorf("expected trimmed name, got %q", capturedName)
	}
}

// --- AuthenticateKey Tests ---

func TestAuthenticateKey_Success(t *testing.T) {
	// Generate a valid key and hash.
	rawKey := "chron_abcdef1234567890abcdef1234567890abcdef1234567890abcdef12345678"
	hash, err := bcrypt.GenerateFromPassword([]byte(rawKey), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt hash failed: %v", err)
	}

	repo := &mockSyncAPIRepo{
		findKeyByPrefixFn: func(ctx context.Context, prefix string) (*APIKey, error) {
			if prefix != "chron_ab" {
				t.Errorf("expected prefix chron_ab, got %s", prefix)
			}
			return &APIKey{
				ID:       1,
				KeyHash:  string(hash),
				IsActive: true,
			}, nil
		},
	}

	svc := NewSyncAPIService(repo)
	key, err := svc.AuthenticateKey(context.Background(), rawKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key.ID != 1 {
		t.Errorf("expected key ID 1, got %d", key.ID)
	}
}

func TestAuthenticateKey_ShortKey(t *testing.T) {
	svc := NewSyncAPIService(&mockSyncAPIRepo{})
	_, err := svc.AuthenticateKey(context.Background(), "short")
	assertAppError(t, err, 400)
}

func TestAuthenticateKey_PrefixNotFound(t *testing.T) {
	svc := NewSyncAPIService(&mockSyncAPIRepo{})
	_, err := svc.AuthenticateKey(context.Background(), "chron_nonexistent1234567890")
	assertAppError(t, err, 403)
}

func TestAuthenticateKey_WrongKey(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("chron_correct_key_here_0000000000000000000000000000000000000000"), bcrypt.DefaultCost)
	repo := &mockSyncAPIRepo{
		findKeyByPrefixFn: func(ctx context.Context, prefix string) (*APIKey, error) {
			return &APIKey{
				ID:       1,
				KeyHash:  string(hash),
				IsActive: true,
			}, nil
		},
	}

	svc := NewSyncAPIService(repo)
	_, err := svc.AuthenticateKey(context.Background(), "chron_wrong_key_here_00000000000000000000000000000000000000000")
	assertAppError(t, err, 403)
}

func TestAuthenticateKey_Deactivated(t *testing.T) {
	rawKey := "chron_test1234567890test1234567890test1234567890test1234567890test"
	hash, _ := bcrypt.GenerateFromPassword([]byte(rawKey), bcrypt.DefaultCost)
	repo := &mockSyncAPIRepo{
		findKeyByPrefixFn: func(ctx context.Context, prefix string) (*APIKey, error) {
			return &APIKey{
				ID:       1,
				KeyHash:  string(hash),
				IsActive: false,
			}, nil
		},
	}

	svc := NewSyncAPIService(repo)
	_, err := svc.AuthenticateKey(context.Background(), rawKey)
	assertAppError(t, err, 403)
}

func TestAuthenticateKey_Expired(t *testing.T) {
	rawKey := "chron_test1234567890test1234567890test1234567890test1234567890test"
	hash, _ := bcrypt.GenerateFromPassword([]byte(rawKey), bcrypt.DefaultCost)
	expired := time.Now().Add(-1 * time.Hour)
	repo := &mockSyncAPIRepo{
		findKeyByPrefixFn: func(ctx context.Context, prefix string) (*APIKey, error) {
			return &APIKey{
				ID:        1,
				KeyHash:   string(hash),
				IsActive:  true,
				ExpiresAt: &expired,
			}, nil
		},
	}

	svc := NewSyncAPIService(repo)
	_, err := svc.AuthenticateKey(context.Background(), rawKey)
	assertAppError(t, err, 403)
}

// --- ActivateKey / DeactivateKey / RevokeKey Tests ---

func TestActivateKey(t *testing.T) {
	var capturedActive bool
	repo := &mockSyncAPIRepo{
		updateKeyActiveFn: func(ctx context.Context, id int, active bool) error {
			capturedActive = active
			return nil
		},
	}

	svc := NewSyncAPIService(repo)
	err := svc.ActivateKey(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !capturedActive {
		t.Error("expected active=true")
	}
}

func TestDeactivateKey(t *testing.T) {
	var capturedActive bool
	capturedActive = true // Start as true.
	repo := &mockSyncAPIRepo{
		updateKeyActiveFn: func(ctx context.Context, id int, active bool) error {
			capturedActive = active
			return nil
		},
	}

	svc := NewSyncAPIService(repo)
	err := svc.DeactivateKey(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedActive {
		t.Error("expected active=false")
	}
}

func TestRevokeKey(t *testing.T) {
	var deletedID int
	repo := &mockSyncAPIRepo{
		deleteKeyFn: func(ctx context.Context, id int) error {
			deletedID = id
			return nil
		},
	}

	svc := NewSyncAPIService(repo)
	err := svc.RevokeKey(context.Background(), 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deletedID != 42 {
		t.Errorf("expected ID 42, got %d", deletedID)
	}
}

// --- BlockIP Tests ---

func TestBlockIP_Success(t *testing.T) {
	var capturedBlock *IPBlock
	repo := &mockSyncAPIRepo{
		addIPBlockFn: func(ctx context.Context, block *IPBlock) error {
			capturedBlock = block
			block.ID = 1
			return nil
		},
	}

	svc := NewSyncAPIService(repo)
	block, err := svc.BlockIP(context.Background(), "192.168.1.100", "Suspicious activity", "admin-1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if block.ID != 1 {
		t.Errorf("expected ID 1, got %d", block.ID)
	}
	if capturedBlock.IPAddress != "192.168.1.100" {
		t.Errorf("expected IP 192.168.1.100, got %s", capturedBlock.IPAddress)
	}
	if capturedBlock.BlockedBy != "admin-1" {
		t.Errorf("expected admin-1, got %s", capturedBlock.BlockedBy)
	}
	if capturedBlock.Reason == nil || *capturedBlock.Reason != "Suspicious activity" {
		t.Error("expected reason to be set")
	}
}

func TestBlockIP_EmptyIP(t *testing.T) {
	svc := NewSyncAPIService(&mockSyncAPIRepo{})
	_, err := svc.BlockIP(context.Background(), "", "reason", "admin-1", nil)
	assertAppError(t, err, 400)
}

func TestBlockIP_WhitespaceIP(t *testing.T) {
	svc := NewSyncAPIService(&mockSyncAPIRepo{})
	_, err := svc.BlockIP(context.Background(), "   ", "reason", "admin-1", nil)
	assertAppError(t, err, 400)
}

func TestBlockIP_EmptyReason(t *testing.T) {
	var capturedBlock *IPBlock
	repo := &mockSyncAPIRepo{
		addIPBlockFn: func(ctx context.Context, block *IPBlock) error {
			capturedBlock = block
			block.ID = 1
			return nil
		},
	}

	svc := NewSyncAPIService(repo)
	_, err := svc.BlockIP(context.Background(), "10.0.0.1", "", "admin-1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedBlock.Reason != nil {
		t.Errorf("expected nil reason for empty input, got %q", *capturedBlock.Reason)
	}
}

func TestBlockIP_RepoError(t *testing.T) {
	repo := &mockSyncAPIRepo{
		addIPBlockFn: func(ctx context.Context, block *IPBlock) error {
			return errors.New("db error")
		},
	}

	svc := NewSyncAPIService(repo)
	_, err := svc.BlockIP(context.Background(), "10.0.0.1", "reason", "admin-1", nil)
	assertAppError(t, err, 500)
}

// --- LogRequest Tests ---

func TestLogRequest_NonCritical(t *testing.T) {
	// LogRequest should never return an error, even if repo fails.
	repo := &mockSyncAPIRepo{
		logRequestFn: func(ctx context.Context, log *APIRequestLog) error {
			return errors.New("db error")
		},
	}

	svc := NewSyncAPIService(repo)
	err := svc.LogRequest(context.Background(), &APIRequestLog{
		Method: "GET", Path: "/api/v1/entities",
	})
	if err != nil {
		t.Errorf("expected nil error (non-critical), got: %v", err)
	}
}

// --- LogSecurityEvent Tests ---

func TestLogSecurityEvent_NonCritical(t *testing.T) {
	repo := &mockSyncAPIRepo{
		logSecurityEventFn: func(ctx context.Context, event *SecurityEvent) error {
			return errors.New("db error")
		},
	}

	svc := NewSyncAPIService(repo)
	err := svc.LogSecurityEvent(context.Background(), &SecurityEvent{
		EventType: EventRateLimit, IPAddress: "10.0.0.1",
	})
	if err != nil {
		t.Errorf("expected nil error (non-critical), got: %v", err)
	}
}

// --- Default Limit Tests ---

func TestListAllKeys_DefaultLimit(t *testing.T) {
	var capturedLimit int
	repo := &mockSyncAPIRepo{
		listAllKeysFn: func(ctx context.Context, limit, offset int) ([]APIKey, int, error) {
			capturedLimit = limit
			return nil, 0, nil
		},
	}

	svc := NewSyncAPIService(repo)
	_, _, _ = svc.ListAllKeys(context.Background(), 0, 0) // limit=0 should default.
	if capturedLimit != 50 {
		t.Errorf("expected default limit 50, got %d", capturedLimit)
	}
}

func TestListRequestLogs_DefaultLimit(t *testing.T) {
	var capturedLimit int
	repo := &mockSyncAPIRepo{
		listRequestLogsFn: func(ctx context.Context, filter RequestLogFilter) ([]APIRequestLog, int, error) {
			capturedLimit = filter.Limit
			return nil, 0, nil
		},
	}

	svc := NewSyncAPIService(repo)
	_, _, _ = svc.ListRequestLogs(context.Background(), RequestLogFilter{Limit: 0})
	if capturedLimit != 50 {
		t.Errorf("expected default limit 50, got %d", capturedLimit)
	}
}

func TestGetTopIPs_DefaultLimit(t *testing.T) {
	var capturedLimit int
	repo := &mockSyncAPIRepo{
		getTopIPsFn: func(ctx context.Context, since time.Time, limit int) ([]TopEntry, error) {
			capturedLimit = limit
			return nil, nil
		},
	}

	svc := NewSyncAPIService(repo)
	_, _ = svc.GetTopIPs(context.Background(), time.Now(), 0)
	if capturedLimit != 10 {
		t.Errorf("expected default limit 10, got %d", capturedLimit)
	}
}

// --- Model Tests ---

func TestAPIKey_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt *time.Time
		want      bool
	}{
		{"nil expiry (no expiration)", nil, false},
		{"future expiry", timePtr(time.Now().Add(1 * time.Hour)), false},
		{"past expiry", timePtr(time.Now().Add(-1 * time.Hour)), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := &APIKey{ExpiresAt: tt.expiresAt}
			if got := key.IsExpired(); got != tt.want {
				t.Errorf("IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAPIKey_HasPermission(t *testing.T) {
	key := &APIKey{Permissions: []APIKeyPermission{PermRead, PermSync}}

	if !key.HasPermission(PermRead) {
		t.Error("expected HasPermission(read) = true")
	}
	if !key.HasPermission(PermSync) {
		t.Error("expected HasPermission(sync) = true")
	}
	if key.HasPermission(PermWrite) {
		t.Error("expected HasPermission(write) = false")
	}
}

func TestAPIKey_HasPermission_Empty(t *testing.T) {
	key := &APIKey{Permissions: nil}
	if key.HasPermission(PermRead) {
		t.Error("expected no permissions on nil slice")
	}
}

// timePtr returns a pointer to a time value.
func timePtr(t time.Time) *time.Time {
	return &t
}
