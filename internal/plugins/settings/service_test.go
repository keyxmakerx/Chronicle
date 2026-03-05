package settings

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// --- Mock Repository ---

// mockSettingsRepo implements SettingsRepository for testing.
type mockSettingsRepo struct {
	getFn                 func(ctx context.Context, key string) (string, error)
	setFn                 func(ctx context.Context, key, value string) error
	getAllFn              func(ctx context.Context) (map[string]string, error)
	getUserLimitFn        func(ctx context.Context, userID string) (*UserStorageLimit, error)
	setUserLimitFn        func(ctx context.Context, limit *UserStorageLimit) error
	deleteUserLimitFn     func(ctx context.Context, userID string) error
	getCampaignLimitFn    func(ctx context.Context, campaignID string) (*CampaignStorageLimit, error)
	setCampaignLimitFn    func(ctx context.Context, limit *CampaignStorageLimit) error
	deleteCampaignLimitFn func(ctx context.Context, campaignID string) error
	listUserLimitsFn      func(ctx context.Context) ([]UserStorageLimitWithName, error)
	listCampaignLimitsFn  func(ctx context.Context) ([]CampaignStorageLimitWithName, error)
	setUserBypassFn       func(ctx context.Context, userID string, maxUpload *int64, expiresAt time.Time, reason, grantedBy string) error
	clearUserBypassFn     func(ctx context.Context, userID string) error
	setCampaignBypassFn   func(ctx context.Context, campaignID string, maxStorage *int64, maxFiles *int, expiresAt time.Time, reason, grantedBy string) error
	clearCampaignBypassFn func(ctx context.Context, campaignID string) error
}

func (m *mockSettingsRepo) Get(ctx context.Context, key string) (string, error) {
	if m.getFn != nil {
		return m.getFn(ctx, key)
	}
	return "", apperror.NewNotFound("setting not found")
}

func (m *mockSettingsRepo) Set(ctx context.Context, key, value string) error {
	if m.setFn != nil {
		return m.setFn(ctx, key, value)
	}
	return nil
}

func (m *mockSettingsRepo) GetAll(ctx context.Context) (map[string]string, error) {
	if m.getAllFn != nil {
		return m.getAllFn(ctx)
	}
	return make(map[string]string), nil
}

func (m *mockSettingsRepo) GetUserLimit(ctx context.Context, userID string) (*UserStorageLimit, error) {
	if m.getUserLimitFn != nil {
		return m.getUserLimitFn(ctx, userID)
	}
	return nil, nil
}

func (m *mockSettingsRepo) SetUserLimit(ctx context.Context, limit *UserStorageLimit) error {
	if m.setUserLimitFn != nil {
		return m.setUserLimitFn(ctx, limit)
	}
	return nil
}

func (m *mockSettingsRepo) DeleteUserLimit(ctx context.Context, userID string) error {
	if m.deleteUserLimitFn != nil {
		return m.deleteUserLimitFn(ctx, userID)
	}
	return nil
}

func (m *mockSettingsRepo) GetCampaignLimit(ctx context.Context, campaignID string) (*CampaignStorageLimit, error) {
	if m.getCampaignLimitFn != nil {
		return m.getCampaignLimitFn(ctx, campaignID)
	}
	return nil, nil
}

func (m *mockSettingsRepo) SetCampaignLimit(ctx context.Context, limit *CampaignStorageLimit) error {
	if m.setCampaignLimitFn != nil {
		return m.setCampaignLimitFn(ctx, limit)
	}
	return nil
}

func (m *mockSettingsRepo) DeleteCampaignLimit(ctx context.Context, campaignID string) error {
	if m.deleteCampaignLimitFn != nil {
		return m.deleteCampaignLimitFn(ctx, campaignID)
	}
	return nil
}

func (m *mockSettingsRepo) ListUserLimits(ctx context.Context) ([]UserStorageLimitWithName, error) {
	if m.listUserLimitsFn != nil {
		return m.listUserLimitsFn(ctx)
	}
	return nil, nil
}

func (m *mockSettingsRepo) ListCampaignLimits(ctx context.Context) ([]CampaignStorageLimitWithName, error) {
	if m.listCampaignLimitsFn != nil {
		return m.listCampaignLimitsFn(ctx)
	}
	return nil, nil
}

func (m *mockSettingsRepo) SetUserBypass(ctx context.Context, userID string, maxUpload *int64, expiresAt time.Time, reason, grantedBy string) error {
	if m.setUserBypassFn != nil {
		return m.setUserBypassFn(ctx, userID, maxUpload, expiresAt, reason, grantedBy)
	}
	return nil
}

func (m *mockSettingsRepo) ClearUserBypass(ctx context.Context, userID string) error {
	if m.clearUserBypassFn != nil {
		return m.clearUserBypassFn(ctx, userID)
	}
	return nil
}

func (m *mockSettingsRepo) SetCampaignBypass(ctx context.Context, campaignID string, maxStorage *int64, maxFiles *int, expiresAt time.Time, reason, grantedBy string) error {
	if m.setCampaignBypassFn != nil {
		return m.setCampaignBypassFn(ctx, campaignID, maxStorage, maxFiles, expiresAt, reason, grantedBy)
	}
	return nil
}

func (m *mockSettingsRepo) ClearCampaignBypass(ctx context.Context, campaignID string) error {
	if m.clearCampaignBypassFn != nil {
		return m.clearCampaignBypassFn(ctx, campaignID)
	}
	return nil
}

// --- Test Helpers ---

func newTestSettingsService(repo *mockSettingsRepo) *settingsService {
	return &settingsService{repo: repo}
}

func assertSettingsAppError(t *testing.T, err error, expectedCode int) {
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

func int64Ptr(v int64) *int64 { return &v }
func intPtr(v int) *int       { return &v }
func strPtr(v string) *string { return &v }

// --- GetStorageLimits Tests ---

func TestGetStorageLimits_Defaults(t *testing.T) {
	repo := &mockSettingsRepo{
		getAllFn: func(ctx context.Context) (map[string]string, error) {
			return map[string]string{}, nil // No settings stored yet.
		},
	}
	svc := newTestSettingsService(repo)

	limits, err := svc.GetStorageLimits(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if limits.MaxUploadSize != 10485760 {
		t.Errorf("expected default max upload 10485760, got %d", limits.MaxUploadSize)
	}
	if limits.MaxStoragePerUser != 0 {
		t.Errorf("expected default max storage per user 0 (unlimited), got %d", limits.MaxStoragePerUser)
	}
	if limits.RateLimitUploadsPerMin != 30 {
		t.Errorf("expected default rate limit 30, got %d", limits.RateLimitUploadsPerMin)
	}
}

func TestGetStorageLimits_ParsedValues(t *testing.T) {
	repo := &mockSettingsRepo{
		getAllFn: func(ctx context.Context) (map[string]string, error) {
			return map[string]string{
				KeyMaxUploadSize:          "52428800",
				KeyMaxStoragePerUser:      "1073741824",
				KeyMaxStoragePerCampaign:  "536870912",
				KeyMaxFilesPerCampaign:    "500",
				KeyRateLimitUploadsPerMin: "60",
			}, nil
		},
	}
	svc := newTestSettingsService(repo)

	limits, err := svc.GetStorageLimits(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if limits.MaxUploadSize != 52428800 {
		t.Errorf("expected 52428800, got %d", limits.MaxUploadSize)
	}
	if limits.MaxFilesPerCampaign != 500 {
		t.Errorf("expected 500, got %d", limits.MaxFilesPerCampaign)
	}
}

func TestGetStorageLimits_RepoError(t *testing.T) {
	repo := &mockSettingsRepo{
		getAllFn: func(ctx context.Context) (map[string]string, error) {
			return nil, errors.New("db error")
		},
	}
	svc := newTestSettingsService(repo)

	_, err := svc.GetStorageLimits(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- UpdateStorageLimits Tests ---

func TestUpdateStorageLimits_Success(t *testing.T) {
	var setCalls int
	repo := &mockSettingsRepo{
		setFn: func(ctx context.Context, key, value string) error {
			setCalls++
			return nil
		},
	}
	svc := newTestSettingsService(repo)

	limits := &GlobalStorageLimits{
		MaxUploadSize:         52428800,
		MaxStoragePerUser:     1073741824,
		MaxStoragePerCampaign: 536870912,
		MaxFilesPerCampaign:   500,
		RateLimitUploadsPerMin: 60,
	}

	err := svc.UpdateStorageLimits(context.Background(), limits)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if setCalls != 5 {
		t.Errorf("expected 5 repo.Set calls, got %d", setCalls)
	}
}

func TestUpdateStorageLimits_NegativeValues(t *testing.T) {
	svc := newTestSettingsService(&mockSettingsRepo{})

	tests := []struct {
		name   string
		limits *GlobalStorageLimits
	}{
		{
			name:   "negative upload size",
			limits: &GlobalStorageLimits{MaxUploadSize: -1},
		},
		{
			name:   "negative storage per user",
			limits: &GlobalStorageLimits{MaxStoragePerUser: -1},
		},
		{
			name:   "negative storage per campaign",
			limits: &GlobalStorageLimits{MaxStoragePerCampaign: -1},
		},
		{
			name:   "negative files per campaign",
			limits: &GlobalStorageLimits{MaxFilesPerCampaign: -1},
		},
		{
			name:   "negative rate limit",
			limits: &GlobalStorageLimits{RateLimitUploadsPerMin: -1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.UpdateStorageLimits(context.Background(), tt.limits)
			assertSettingsAppError(t, err, 400)
		})
	}
}

// --- GetEffectiveLimits Tests ---

func TestGetEffectiveLimits_GlobalOnly(t *testing.T) {
	repo := &mockSettingsRepo{
		getAllFn: func(ctx context.Context) (map[string]string, error) {
			return map[string]string{
				KeyMaxUploadSize:       "10485760",
				KeyMaxStoragePerUser:   "1073741824",
				KeyMaxFilesPerCampaign: "500",
			}, nil
		},
	}
	svc := newTestSettingsService(repo)

	// No user or campaign — should get global defaults.
	eff, err := svc.GetEffectiveLimits(context.Background(), "", "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if eff.MaxUploadSize != 10485760 {
		t.Errorf("expected 10485760, got %d", eff.MaxUploadSize)
	}
	if eff.MaxTotalStorage != 1073741824 {
		t.Errorf("expected 1073741824, got %d", eff.MaxTotalStorage)
	}
	if eff.MaxFiles != 500 {
		t.Errorf("expected 500, got %d", eff.MaxFiles)
	}
}

func TestGetEffectiveLimits_UserOverride(t *testing.T) {
	repo := &mockSettingsRepo{
		getAllFn: func(ctx context.Context) (map[string]string, error) {
			return map[string]string{
				KeyMaxUploadSize:     "10485760",
				KeyMaxStoragePerUser: "1073741824",
			}, nil
		},
		getUserLimitFn: func(ctx context.Context, userID string) (*UserStorageLimit, error) {
			return &UserStorageLimit{
				UserID:          "user-1",
				MaxUploadSize:   int64Ptr(52428800),  // Override to 50 MB.
				MaxTotalStorage: nil,                  // Inherit global.
			}, nil
		},
	}
	svc := newTestSettingsService(repo)

	eff, err := svc.GetEffectiveLimits(context.Background(), "user-1", "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if eff.MaxUploadSize != 52428800 {
		t.Errorf("expected user override 52428800, got %d", eff.MaxUploadSize)
	}
	if eff.MaxTotalStorage != 1073741824 {
		t.Errorf("expected inherited global 1073741824, got %d", eff.MaxTotalStorage)
	}
}

func TestGetEffectiveLimits_CampaignOverride(t *testing.T) {
	repo := &mockSettingsRepo{
		getAllFn: func(ctx context.Context) (map[string]string, error) {
			return map[string]string{
				KeyMaxStoragePerCampaign: "536870912",
				KeyMaxFilesPerCampaign:   "500",
			}, nil
		},
		getUserLimitFn: func(ctx context.Context, userID string) (*UserStorageLimit, error) {
			return nil, nil // No user override.
		},
		getCampaignLimitFn: func(ctx context.Context, campaignID string) (*CampaignStorageLimit, error) {
			return &CampaignStorageLimit{
				CampaignID:      "camp-1",
				MaxTotalStorage: int64Ptr(1073741824), // Campaign gets 1 GB.
				MaxFiles:        nil,                   // Inherit global.
			}, nil
		},
	}
	svc := newTestSettingsService(repo)

	eff, err := svc.GetEffectiveLimits(context.Background(), "user-1", "camp-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if eff.MaxTotalStorage != 1073741824 {
		t.Errorf("expected campaign override 1073741824, got %d", eff.MaxTotalStorage)
	}
	if eff.MaxFiles != 500 {
		t.Errorf("expected inherited global 500, got %d", eff.MaxFiles)
	}
}

func TestGetEffectiveLimits_ActiveBypass(t *testing.T) {
	futureTime := time.Now().Add(24 * time.Hour)
	repo := &mockSettingsRepo{
		getAllFn: func(ctx context.Context) (map[string]string, error) {
			return map[string]string{
				KeyMaxUploadSize: "10485760",
			}, nil
		},
		getUserLimitFn: func(ctx context.Context, userID string) (*UserStorageLimit, error) {
			return &UserStorageLimit{
				UserID:          "user-1",
				MaxUploadSize:   int64Ptr(10485760),
				BypassMaxUpload: int64Ptr(104857600), // 100 MB bypass.
				BypassExpiresAt: &futureTime,
				BypassReason:    strPtr("large map upload"),
				BypassGrantedBy: strPtr("admin-1"),
			}, nil
		},
		getCampaignLimitFn: func(ctx context.Context, campaignID string) (*CampaignStorageLimit, error) {
			return nil, nil
		},
	}
	svc := newTestSettingsService(repo)

	eff, err := svc.GetEffectiveLimits(context.Background(), "user-1", "camp-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	// Bypass should override the user limit.
	if eff.MaxUploadSize != 104857600 {
		t.Errorf("expected bypass 104857600, got %d", eff.MaxUploadSize)
	}
}

func TestGetEffectiveLimits_ExpiredBypass(t *testing.T) {
	pastTime := time.Now().Add(-24 * time.Hour)
	repo := &mockSettingsRepo{
		getAllFn: func(ctx context.Context) (map[string]string, error) {
			return map[string]string{
				KeyMaxUploadSize: "10485760",
			}, nil
		},
		getUserLimitFn: func(ctx context.Context, userID string) (*UserStorageLimit, error) {
			return &UserStorageLimit{
				UserID:          "user-1",
				MaxUploadSize:   int64Ptr(20971520),  // 20 MB user limit.
				BypassMaxUpload: int64Ptr(104857600), // 100 MB bypass — expired.
				BypassExpiresAt: &pastTime,
			}, nil
		},
	}
	svc := newTestSettingsService(repo)

	eff, err := svc.GetEffectiveLimits(context.Background(), "user-1", "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	// Expired bypass should be ignored; user limit should apply.
	if eff.MaxUploadSize != 20971520 {
		t.Errorf("expected user limit 20971520 (bypass expired), got %d", eff.MaxUploadSize)
	}
}

// --- Per-User Limit Validation Tests ---

func TestGetUserLimit_EmptyID(t *testing.T) {
	svc := newTestSettingsService(&mockSettingsRepo{})

	_, err := svc.GetUserLimit(context.Background(), "")
	assertSettingsAppError(t, err, 400)
}

func TestSetUserLimit_EmptyID(t *testing.T) {
	svc := newTestSettingsService(&mockSettingsRepo{})

	err := svc.SetUserLimit(context.Background(), &UserStorageLimit{})
	assertSettingsAppError(t, err, 400)
}

func TestSetUserLimit_NegativeUploadSize(t *testing.T) {
	svc := newTestSettingsService(&mockSettingsRepo{})

	err := svc.SetUserLimit(context.Background(), &UserStorageLimit{
		UserID:        "user-1",
		MaxUploadSize: int64Ptr(-1),
	})
	assertSettingsAppError(t, err, 400)
}

func TestSetUserLimit_NegativeTotalStorage(t *testing.T) {
	svc := newTestSettingsService(&mockSettingsRepo{})

	err := svc.SetUserLimit(context.Background(), &UserStorageLimit{
		UserID:          "user-1",
		MaxTotalStorage: int64Ptr(-1),
	})
	assertSettingsAppError(t, err, 400)
}

func TestSetUserLimit_Success(t *testing.T) {
	var captured *UserStorageLimit
	repo := &mockSettingsRepo{
		setUserLimitFn: func(ctx context.Context, limit *UserStorageLimit) error {
			captured = limit
			return nil
		},
	}
	svc := newTestSettingsService(repo)

	err := svc.SetUserLimit(context.Background(), &UserStorageLimit{
		UserID:        "user-1",
		MaxUploadSize: int64Ptr(52428800),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if captured == nil {
		t.Fatal("expected repo.SetUserLimit to be called")
	}
}

func TestDeleteUserLimit_EmptyID(t *testing.T) {
	svc := newTestSettingsService(&mockSettingsRepo{})

	err := svc.DeleteUserLimit(context.Background(), "")
	assertSettingsAppError(t, err, 400)
}

// --- Per-Campaign Limit Validation Tests ---

func TestGetCampaignLimit_EmptyID(t *testing.T) {
	svc := newTestSettingsService(&mockSettingsRepo{})

	_, err := svc.GetCampaignLimit(context.Background(), "")
	assertSettingsAppError(t, err, 400)
}

func TestSetCampaignLimit_EmptyID(t *testing.T) {
	svc := newTestSettingsService(&mockSettingsRepo{})

	err := svc.SetCampaignLimit(context.Background(), &CampaignStorageLimit{})
	assertSettingsAppError(t, err, 400)
}

func TestSetCampaignLimit_NegativeStorage(t *testing.T) {
	svc := newTestSettingsService(&mockSettingsRepo{})

	err := svc.SetCampaignLimit(context.Background(), &CampaignStorageLimit{
		CampaignID:      "camp-1",
		MaxTotalStorage: int64Ptr(-1),
	})
	assertSettingsAppError(t, err, 400)
}

func TestSetCampaignLimit_NegativeFiles(t *testing.T) {
	svc := newTestSettingsService(&mockSettingsRepo{})

	err := svc.SetCampaignLimit(context.Background(), &CampaignStorageLimit{
		CampaignID: "camp-1",
		MaxFiles:   intPtr(-1),
	})
	assertSettingsAppError(t, err, 400)
}

func TestDeleteCampaignLimit_EmptyID(t *testing.T) {
	svc := newTestSettingsService(&mockSettingsRepo{})

	err := svc.DeleteCampaignLimit(context.Background(), "")
	assertSettingsAppError(t, err, 400)
}

// --- Bypass Validation Tests ---

func TestSetUserBypass_EmptyUserID(t *testing.T) {
	svc := newTestSettingsService(&mockSettingsRepo{})

	err := svc.SetUserBypass(context.Background(), "", nil, time.Now().Add(time.Hour), "test", "admin-1")
	assertSettingsAppError(t, err, 400)
}

func TestSetUserBypass_EmptyGrantedBy(t *testing.T) {
	svc := newTestSettingsService(&mockSettingsRepo{})

	err := svc.SetUserBypass(context.Background(), "user-1", nil, time.Now().Add(time.Hour), "test", "")
	assertSettingsAppError(t, err, 400)
}

func TestSetUserBypass_PastExpiration(t *testing.T) {
	svc := newTestSettingsService(&mockSettingsRepo{})

	err := svc.SetUserBypass(context.Background(), "user-1", nil, time.Now().Add(-time.Hour), "test", "admin-1")
	assertSettingsAppError(t, err, 400)
}

func TestSetUserBypass_NegativeValue(t *testing.T) {
	svc := newTestSettingsService(&mockSettingsRepo{})

	err := svc.SetUserBypass(context.Background(), "user-1", int64Ptr(-1), time.Now().Add(time.Hour), "test", "admin-1")
	assertSettingsAppError(t, err, 400)
}

func TestSetUserBypass_Success(t *testing.T) {
	repo := &mockSettingsRepo{}
	svc := newTestSettingsService(repo)

	err := svc.SetUserBypass(context.Background(), "user-1", int64Ptr(104857600), time.Now().Add(time.Hour), "large upload", "admin-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestClearUserBypass_EmptyID(t *testing.T) {
	svc := newTestSettingsService(&mockSettingsRepo{})

	err := svc.ClearUserBypass(context.Background(), "")
	assertSettingsAppError(t, err, 400)
}

func TestSetCampaignBypass_EmptyID(t *testing.T) {
	svc := newTestSettingsService(&mockSettingsRepo{})

	err := svc.SetCampaignBypass(context.Background(), "", nil, nil, time.Now().Add(time.Hour), "test", "admin-1")
	assertSettingsAppError(t, err, 400)
}

func TestSetCampaignBypass_PastExpiration(t *testing.T) {
	svc := newTestSettingsService(&mockSettingsRepo{})

	err := svc.SetCampaignBypass(context.Background(), "camp-1", nil, nil, time.Now().Add(-time.Hour), "test", "admin-1")
	assertSettingsAppError(t, err, 400)
}

func TestSetCampaignBypass_NegativeStorage(t *testing.T) {
	svc := newTestSettingsService(&mockSettingsRepo{})

	err := svc.SetCampaignBypass(context.Background(), "camp-1", int64Ptr(-1), nil, time.Now().Add(time.Hour), "test", "admin-1")
	assertSettingsAppError(t, err, 400)
}

func TestSetCampaignBypass_NegativeFiles(t *testing.T) {
	svc := newTestSettingsService(&mockSettingsRepo{})

	err := svc.SetCampaignBypass(context.Background(), "camp-1", nil, intPtr(-1), time.Now().Add(time.Hour), "test", "admin-1")
	assertSettingsAppError(t, err, 400)
}

func TestClearCampaignBypass_EmptyID(t *testing.T) {
	svc := newTestSettingsService(&mockSettingsRepo{})

	err := svc.ClearCampaignBypass(context.Background(), "")
	assertSettingsAppError(t, err, 400)
}

// --- List Pass-Through Tests ---

func TestListUserLimits_Success(t *testing.T) {
	expected := []UserStorageLimitWithName{
		{UserStorageLimit: UserStorageLimit{UserID: "user-1"}, DisplayName: "Alice"},
	}
	repo := &mockSettingsRepo{
		listUserLimitsFn: func(ctx context.Context) ([]UserStorageLimitWithName, error) {
			return expected, nil
		},
	}
	svc := newTestSettingsService(repo)

	result, err := svc.ListUserLimits(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 result, got %d", len(result))
	}
}

func TestListCampaignLimits_Success(t *testing.T) {
	expected := []CampaignStorageLimitWithName{
		{CampaignStorageLimit: CampaignStorageLimit{CampaignID: "camp-1"}, CampaignName: "Middle-earth"},
	}
	repo := &mockSettingsRepo{
		listCampaignLimitsFn: func(ctx context.Context) ([]CampaignStorageLimitWithName, error) {
			return expected, nil
		},
	}
	svc := newTestSettingsService(repo)

	result, err := svc.ListCampaignLimits(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 result, got %d", len(result))
	}
}

// --- HasActiveBypass Model Tests ---

func TestUserStorageLimit_HasActiveBypass(t *testing.T) {
	tests := []struct {
		name     string
		limit    *UserStorageLimit
		expected bool
	}{
		{"nil receiver", nil, false},
		{"no bypass", &UserStorageLimit{UserID: "u1"}, false},
		{
			"active bypass",
			&UserStorageLimit{
				UserID:          "u1",
				BypassExpiresAt: timePtr(time.Now().Add(time.Hour)),
			},
			true,
		},
		{
			"expired bypass",
			&UserStorageLimit{
				UserID:          "u1",
				BypassExpiresAt: timePtr(time.Now().Add(-time.Hour)),
			},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.limit.HasActiveBypass()
			if got != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestCampaignStorageLimit_HasActiveBypass(t *testing.T) {
	tests := []struct {
		name     string
		limit    *CampaignStorageLimit
		expected bool
	}{
		{"nil receiver", nil, false},
		{"no bypass", &CampaignStorageLimit{CampaignID: "c1"}, false},
		{
			"active bypass",
			&CampaignStorageLimit{
				CampaignID:      "c1",
				BypassExpiresAt: timePtr(time.Now().Add(time.Hour)),
			},
			true,
		},
		{
			"expired bypass",
			&CampaignStorageLimit{
				CampaignID:      "c1",
				BypassExpiresAt: timePtr(time.Now().Add(-time.Hour)),
			},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.limit.HasActiveBypass()
			if got != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}

func timePtr(t time.Time) *time.Time { return &t }
