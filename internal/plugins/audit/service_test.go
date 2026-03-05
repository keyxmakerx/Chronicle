package audit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// --- Mock Repository ---

// mockAuditRepo implements AuditRepository for testing.
type mockAuditRepo struct {
	logFn              func(ctx context.Context, entry *AuditEntry) error
	listByCampaignFn   func(ctx context.Context, campaignID string, limit, offset int) ([]AuditEntry, int, error)
	listByEntityFn     func(ctx context.Context, entityID string, limit int) ([]AuditEntry, error)
	countByCampaignFn  func(ctx context.Context, campaignID string) (int, error)
	getCampaignStatsFn func(ctx context.Context, campaignID string) (*CampaignStats, error)
}

func (m *mockAuditRepo) Log(ctx context.Context, entry *AuditEntry) error {
	if m.logFn != nil {
		return m.logFn(ctx, entry)
	}
	return nil
}

func (m *mockAuditRepo) ListByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]AuditEntry, int, error) {
	if m.listByCampaignFn != nil {
		return m.listByCampaignFn(ctx, campaignID, limit, offset)
	}
	return nil, 0, nil
}

func (m *mockAuditRepo) ListByEntity(ctx context.Context, entityID string, limit int) ([]AuditEntry, error) {
	if m.listByEntityFn != nil {
		return m.listByEntityFn(ctx, entityID, limit)
	}
	return nil, nil
}

func (m *mockAuditRepo) CountByCampaign(ctx context.Context, campaignID string) (int, error) {
	if m.countByCampaignFn != nil {
		return m.countByCampaignFn(ctx, campaignID)
	}
	return 0, nil
}

func (m *mockAuditRepo) GetCampaignStats(ctx context.Context, campaignID string) (*CampaignStats, error) {
	if m.getCampaignStatsFn != nil {
		return m.getCampaignStatsFn(ctx, campaignID)
	}
	return &CampaignStats{}, nil
}

// --- Test Helpers ---

func newTestAuditService(repo *mockAuditRepo) *auditService {
	return &auditService{repo: repo}
}

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

// --- Log Tests ---

func TestLog_Success(t *testing.T) {
	var captured *AuditEntry
	repo := &mockAuditRepo{
		logFn: func(ctx context.Context, entry *AuditEntry) error {
			captured = entry
			return nil
		},
	}
	svc := newTestAuditService(repo)

	entry := &AuditEntry{
		CampaignID: "camp-1",
		UserID:     "user-1",
		Action:     ActionEntityCreated,
		EntityType: "character",
		EntityID:   "ent-1",
		EntityName: "Gandalf",
	}

	err := svc.Log(context.Background(), entry)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if captured == nil {
		t.Fatal("expected repo.Log to be called")
	}
	if captured.Action != ActionEntityCreated {
		t.Errorf("expected action %s, got %s", ActionEntityCreated, captured.Action)
	}
}

func TestLog_ValidationErrors(t *testing.T) {
	svc := newTestAuditService(&mockAuditRepo{})

	tests := []struct {
		name  string
		entry *AuditEntry
	}{
		{
			name:  "missing campaign ID",
			entry: &AuditEntry{UserID: "user-1", Action: ActionEntityCreated},
		},
		{
			name:  "missing user ID",
			entry: &AuditEntry{CampaignID: "camp-1", Action: ActionEntityCreated},
		},
		{
			name:  "missing action",
			entry: &AuditEntry{CampaignID: "camp-1", UserID: "user-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.Log(context.Background(), tt.entry)
			assertAppError(t, err, 400)
		})
	}
}

func TestLog_RepoError(t *testing.T) {
	repo := &mockAuditRepo{
		logFn: func(ctx context.Context, entry *AuditEntry) error {
			return errors.New("db connection lost")
		},
	}
	svc := newTestAuditService(repo)

	entry := &AuditEntry{
		CampaignID: "camp-1",
		UserID:     "user-1",
		Action:     ActionEntityCreated,
	}

	err := svc.Log(context.Background(), entry)
	assertAppError(t, err, 500)
}

// --- GetCampaignActivity Tests ---

func TestGetCampaignActivity_Success(t *testing.T) {
	entries := []AuditEntry{
		{ID: 1, CampaignID: "camp-1", Action: ActionEntityCreated},
		{ID: 2, CampaignID: "camp-1", Action: ActionEntityUpdated},
	}

	var capturedLimit, capturedOffset int
	repo := &mockAuditRepo{
		listByCampaignFn: func(ctx context.Context, campaignID string, limit, offset int) ([]AuditEntry, int, error) {
			capturedLimit = limit
			capturedOffset = offset
			return entries, 25, nil
		},
	}
	svc := newTestAuditService(repo)

	result, total, err := svc.GetCampaignActivity(context.Background(), "camp-1", 1)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if total != 25 {
		t.Errorf("expected total 25, got %d", total)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 entries, got %d", len(result))
	}
	if capturedLimit != 50 {
		t.Errorf("expected limit 50, got %d", capturedLimit)
	}
	if capturedOffset != 0 {
		t.Errorf("expected offset 0, got %d", capturedOffset)
	}
}

func TestGetCampaignActivity_PageClamping(t *testing.T) {
	var capturedOffset int
	repo := &mockAuditRepo{
		listByCampaignFn: func(ctx context.Context, campaignID string, limit, offset int) ([]AuditEntry, int, error) {
			capturedOffset = offset
			return nil, 0, nil
		},
	}
	svc := newTestAuditService(repo)

	tests := []struct {
		name           string
		page           int
		expectedOffset int
	}{
		{"zero page clamped to 1", 0, 0},
		{"negative page clamped to 1", -5, 0},
		{"page 2", 2, 50},
		{"page 3", 3, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := svc.GetCampaignActivity(context.Background(), "camp-1", tt.page)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if capturedOffset != tt.expectedOffset {
				t.Errorf("expected offset %d, got %d", tt.expectedOffset, capturedOffset)
			}
		})
	}
}

func TestGetCampaignActivity_RepoError(t *testing.T) {
	repo := &mockAuditRepo{
		listByCampaignFn: func(ctx context.Context, campaignID string, limit, offset int) ([]AuditEntry, int, error) {
			return nil, 0, errors.New("db error")
		},
	}
	svc := newTestAuditService(repo)

	_, _, err := svc.GetCampaignActivity(context.Background(), "camp-1", 1)
	assertAppError(t, err, 500)
}

// --- GetEntityHistory Tests ---

func TestGetEntityHistory_Success(t *testing.T) {
	entries := []AuditEntry{
		{ID: 1, EntityID: "ent-1", Action: ActionEntityUpdated},
	}

	var capturedLimit int
	repo := &mockAuditRepo{
		listByEntityFn: func(ctx context.Context, entityID string, limit int) ([]AuditEntry, error) {
			capturedLimit = limit
			return entries, nil
		},
	}
	svc := newTestAuditService(repo)

	result, err := svc.GetEntityHistory(context.Background(), "ent-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 entry, got %d", len(result))
	}
	if capturedLimit != 100 {
		t.Errorf("expected limit 100, got %d", capturedLimit)
	}
}

func TestGetEntityHistory_EmptyID(t *testing.T) {
	svc := newTestAuditService(&mockAuditRepo{})

	_, err := svc.GetEntityHistory(context.Background(), "")
	assertAppError(t, err, 400)
}

func TestGetEntityHistory_RepoError(t *testing.T) {
	repo := &mockAuditRepo{
		listByEntityFn: func(ctx context.Context, entityID string, limit int) ([]AuditEntry, error) {
			return nil, errors.New("db error")
		},
	}
	svc := newTestAuditService(repo)

	_, err := svc.GetEntityHistory(context.Background(), "ent-1")
	assertAppError(t, err, 500)
}

// --- GetCampaignStats Tests ---

func TestGetCampaignStats_Success(t *testing.T) {
	now := time.Now()
	expected := &CampaignStats{
		TotalEntities: 42,
		TotalWords:    10000,
		LastEditedAt:  &now,
		ActiveEditors: 3,
	}

	repo := &mockAuditRepo{
		getCampaignStatsFn: func(ctx context.Context, campaignID string) (*CampaignStats, error) {
			return expected, nil
		},
	}
	svc := newTestAuditService(repo)

	result, err := svc.GetCampaignStats(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.TotalEntities != 42 {
		t.Errorf("expected 42 entities, got %d", result.TotalEntities)
	}
	if result.ActiveEditors != 3 {
		t.Errorf("expected 3 active editors, got %d", result.ActiveEditors)
	}
}

func TestGetCampaignStats_EmptyID(t *testing.T) {
	svc := newTestAuditService(&mockAuditRepo{})

	_, err := svc.GetCampaignStats(context.Background(), "")
	assertAppError(t, err, 400)
}

func TestGetCampaignStats_RepoError(t *testing.T) {
	repo := &mockAuditRepo{
		getCampaignStatsFn: func(ctx context.Context, campaignID string) (*CampaignStats, error) {
			return nil, errors.New("db error")
		},
	}
	svc := newTestAuditService(repo)

	_, err := svc.GetCampaignStats(context.Background(), "camp-1")
	assertAppError(t, err, 500)
}
