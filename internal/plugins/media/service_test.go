package media

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// --- Mock Repository ---

// mockMediaRepo implements MediaRepository for testing.
type mockMediaRepo struct {
	createFn           func(ctx context.Context, file *MediaFile) error
	findByIDFn         func(ctx context.Context, id string) (*MediaFile, error)
	deleteFn           func(ctx context.Context, id string) error
	listByCampaignFn   func(ctx context.Context, campaignID string, limit, offset int) ([]MediaFile, int, error)
	getStorageStatsFn  func(ctx context.Context) (*StorageStats, error)
	listAllFn          func(ctx context.Context, limit, offset int) ([]AdminMediaFile, int, error)
	getCampaignUsageFn func(ctx context.Context, campaignID string) (int64, int, error)
	findReferencesFn   func(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error)
	listAllFilenamesFn    func(ctx context.Context) (map[string]bool, error)
	listFilesByCampaignFn func(ctx context.Context, campaignID string) ([]MediaFile, error)
}

func (m *mockMediaRepo) Create(ctx context.Context, file *MediaFile) error {
	if m.createFn != nil {
		return m.createFn(ctx, file)
	}
	return nil
}

func (m *mockMediaRepo) FindByID(ctx context.Context, id string) (*MediaFile, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, id)
	}
	return nil, apperror.NewNotFound("media file not found")
}

func (m *mockMediaRepo) Delete(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

func (m *mockMediaRepo) ListByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]MediaFile, int, error) {
	if m.listByCampaignFn != nil {
		return m.listByCampaignFn(ctx, campaignID, limit, offset)
	}
	return nil, 0, nil
}

func (m *mockMediaRepo) GetStorageStats(ctx context.Context) (*StorageStats, error) {
	if m.getStorageStatsFn != nil {
		return m.getStorageStatsFn(ctx)
	}
	return &StorageStats{}, nil
}

func (m *mockMediaRepo) ListAll(ctx context.Context, limit, offset int) ([]AdminMediaFile, int, error) {
	if m.listAllFn != nil {
		return m.listAllFn(ctx, limit, offset)
	}
	return nil, 0, nil
}

func (m *mockMediaRepo) GetCampaignUsage(ctx context.Context, campaignID string) (int64, int, error) {
	if m.getCampaignUsageFn != nil {
		return m.getCampaignUsageFn(ctx, campaignID)
	}
	return 0, 0, nil
}

func (m *mockMediaRepo) FindReferences(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error) {
	if m.findReferencesFn != nil {
		return m.findReferencesFn(ctx, campaignID, mediaID)
	}
	return nil, nil
}

func (m *mockMediaRepo) ListAllFilenames(ctx context.Context) (map[string]bool, error) {
	if m.listAllFilenamesFn != nil {
		return m.listAllFilenamesFn(ctx)
	}
	return make(map[string]bool), nil
}

func (m *mockMediaRepo) ListFilesByCampaign(ctx context.Context, campaignID string) ([]MediaFile, error) {
	if m.listFilesByCampaignFn != nil {
		return m.listFilesByCampaignFn(ctx, campaignID)
	}
	return nil, nil
}

// --- Mock Storage Limiter ---

type mockStorageLimiter struct {
	getEffectiveLimitsFn func(ctx context.Context, userID, campaignID string) (int64, int64, int, error)
}

func (m *mockStorageLimiter) GetEffectiveLimits(ctx context.Context, userID, campaignID string) (int64, int64, int, error) {
	if m.getEffectiveLimitsFn != nil {
		return m.getEffectiveLimitsFn(ctx, userID, campaignID)
	}
	return 0, 0, 0, nil
}

// --- Test Helpers ---

func newTestMediaService(repo *mockMediaRepo) *mediaService {
	return &mediaService{
		repo:      repo,
		mediaPath: "/tmp/chronicle-test-media",
		maxSize:   10 * 1024 * 1024, // 10 MB
		sem:       &uploadSemaphore{slots: make(map[string]int)},
	}
}

func assertMediaAppError(t *testing.T, err error, expectedCode int) {
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

// --- GetByID Tests ---

func TestGetByID_Success(t *testing.T) {
	expected := &MediaFile{ID: "file-1", MimeType: "image/png", FileSize: 1024}
	repo := &mockMediaRepo{
		findByIDFn: func(ctx context.Context, id string) (*MediaFile, error) {
			return expected, nil
		},
	}
	svc := newTestMediaService(repo)

	result, err := svc.GetByID(context.Background(), "file-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.ID != "file-1" {
		t.Errorf("expected ID file-1, got %s", result.ID)
	}
}

func TestGetByID_NotFound(t *testing.T) {
	svc := newTestMediaService(&mockMediaRepo{})

	_, err := svc.GetByID(context.Background(), "nonexistent")
	assertMediaAppError(t, err, 404)
}

// --- Delete Tests ---

func TestDelete_Success(t *testing.T) {
	campaignID := "camp-1"
	file := &MediaFile{
		ID:         "file-1",
		CampaignID: &campaignID,
		Filename:   "2024/01/file-1.jpg",
		MimeType:   "image/jpeg",
	}

	var deletedID string
	repo := &mockMediaRepo{
		findByIDFn: func(ctx context.Context, id string) (*MediaFile, error) {
			return file, nil
		},
		deleteFn: func(ctx context.Context, id string) error {
			deletedID = id
			return nil
		},
	}
	svc := newTestMediaService(repo)

	err := svc.Delete(context.Background(), "file-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if deletedID != "file-1" {
		t.Errorf("expected deleted ID file-1, got %s", deletedID)
	}
}

func TestDelete_NotFound(t *testing.T) {
	svc := newTestMediaService(&mockMediaRepo{})

	err := svc.Delete(context.Background(), "nonexistent")
	assertMediaAppError(t, err, 404)
}

func TestDelete_RepoDeleteError(t *testing.T) {
	file := &MediaFile{ID: "file-1", Filename: "2024/01/file-1.jpg"}
	repo := &mockMediaRepo{
		findByIDFn: func(ctx context.Context, id string) (*MediaFile, error) {
			return file, nil
		},
		deleteFn: func(ctx context.Context, id string) error {
			return apperror.NewInternal(errors.New("db error"))
		},
	}
	svc := newTestMediaService(repo)

	err := svc.Delete(context.Background(), "file-1")
	assertMediaAppError(t, err, 500)
}

// --- DeleteCampaignMedia Tests ---

func TestDeleteCampaignMedia_Success(t *testing.T) {
	campaignID := "camp-1"
	file := &MediaFile{
		ID:         "file-1",
		CampaignID: &campaignID,
		Filename:   "2024/01/file-1.jpg",
	}

	repo := &mockMediaRepo{
		findByIDFn: func(ctx context.Context, id string) (*MediaFile, error) {
			return file, nil
		},
		deleteFn: func(ctx context.Context, id string) error {
			return nil
		},
	}
	svc := newTestMediaService(repo)

	err := svc.DeleteCampaignMedia(context.Background(), "camp-1", "file-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestDeleteCampaignMedia_WrongCampaign(t *testing.T) {
	otherCampaign := "camp-2"
	file := &MediaFile{
		ID:         "file-1",
		CampaignID: &otherCampaign,
		Filename:   "2024/01/file-1.jpg",
	}

	repo := &mockMediaRepo{
		findByIDFn: func(ctx context.Context, id string) (*MediaFile, error) {
			return file, nil
		},
	}
	svc := newTestMediaService(repo)

	err := svc.DeleteCampaignMedia(context.Background(), "camp-1", "file-1")
	assertMediaAppError(t, err, 404)
}

func TestDeleteCampaignMedia_NilCampaignID(t *testing.T) {
	file := &MediaFile{
		ID:         "file-1",
		CampaignID: nil, // avatar or backdrop — no campaign
		Filename:   "2024/01/file-1.jpg",
	}

	repo := &mockMediaRepo{
		findByIDFn: func(ctx context.Context, id string) (*MediaFile, error) {
			return file, nil
		},
	}
	svc := newTestMediaService(repo)

	err := svc.DeleteCampaignMedia(context.Background(), "camp-1", "file-1")
	assertMediaAppError(t, err, 404)
}

// --- ListCampaignMedia Tests ---

func TestListCampaignMedia_Success(t *testing.T) {
	files := []MediaFile{
		{ID: "file-1", MimeType: "image/png"},
		{ID: "file-2", MimeType: "image/jpeg"},
	}

	var capturedLimit, capturedOffset int
	repo := &mockMediaRepo{
		listByCampaignFn: func(ctx context.Context, campaignID string, limit, offset int) ([]MediaFile, int, error) {
			capturedLimit = limit
			capturedOffset = offset
			return files, 10, nil
		},
	}
	svc := newTestMediaService(repo)

	result, total, err := svc.ListCampaignMedia(context.Background(), "camp-1", 1, 20)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if total != 10 {
		t.Errorf("expected total 10, got %d", total)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 files, got %d", len(result))
	}
	if capturedLimit != 20 {
		t.Errorf("expected limit 20, got %d", capturedLimit)
	}
	if capturedOffset != 0 {
		t.Errorf("expected offset 0, got %d", capturedOffset)
	}
}

func TestListCampaignMedia_PageClamping(t *testing.T) {
	var capturedOffset int
	repo := &mockMediaRepo{
		listByCampaignFn: func(ctx context.Context, campaignID string, limit, offset int) ([]MediaFile, int, error) {
			capturedOffset = offset
			return nil, 0, nil
		},
	}
	svc := newTestMediaService(repo)

	tests := []struct {
		name           string
		page           int
		perPage        int
		expectedOffset int
	}{
		{"zero page clamped to 1", 0, 20, 0},
		{"negative page clamped to 1", -3, 20, 0},
		{"page 2 with 20 per page", 2, 20, 20},
		{"page 3 with 10 per page", 3, 10, 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := svc.ListCampaignMedia(context.Background(), "camp-1", tt.page, tt.perPage)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if capturedOffset != tt.expectedOffset {
				t.Errorf("expected offset %d, got %d", tt.expectedOffset, capturedOffset)
			}
		})
	}
}

// --- GetCampaignStats Tests ---

func TestGetCampaignStats_Success(t *testing.T) {
	repo := &mockMediaRepo{
		getCampaignUsageFn: func(ctx context.Context, campaignID string) (int64, int, error) {
			return 5242880, 15, nil // 5 MB, 15 files
		},
	}
	svc := newTestMediaService(repo)

	stats, err := svc.GetCampaignStats(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if stats.TotalBytes != 5242880 {
		t.Errorf("expected 5242880 bytes, got %d", stats.TotalBytes)
	}
	if stats.TotalFiles != 15 {
		t.Errorf("expected 15 files, got %d", stats.TotalFiles)
	}
}

func TestGetCampaignStats_RepoError(t *testing.T) {
	repo := &mockMediaRepo{
		getCampaignUsageFn: func(ctx context.Context, campaignID string) (int64, int, error) {
			return 0, 0, apperror.NewInternal(errors.New("db error"))
		},
	}
	svc := newTestMediaService(repo)

	_, err := svc.GetCampaignStats(context.Background(), "camp-1")
	assertMediaAppError(t, err, 500)
}

// --- FindReferences Tests ---

func TestFindReferences_Success(t *testing.T) {
	refs := []MediaRef{
		{EntityID: "ent-1", EntityName: "Gandalf", RefType: "image"},
		{EntityID: "ent-2", EntityName: "Bilbo", RefType: "content"},
	}

	repo := &mockMediaRepo{
		findReferencesFn: func(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error) {
			return refs, nil
		},
	}
	svc := newTestMediaService(repo)

	result, err := svc.FindReferences(context.Background(), "camp-1", "file-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 refs, got %d", len(result))
	}
}

// --- FilePath / ThumbnailPath Tests ---

func TestFilePath(t *testing.T) {
	svc := newTestMediaService(&mockMediaRepo{})
	file := &MediaFile{Filename: "2024/01/abc.jpg"}

	path := svc.FilePath(file)
	if path != "/tmp/chronicle-test-media/2024/01/abc.jpg" {
		t.Errorf("unexpected path: %s", path)
	}
}

func TestThumbnailPath_Exists(t *testing.T) {
	svc := newTestMediaService(&mockMediaRepo{})
	file := &MediaFile{
		Filename:       "2024/01/abc.jpg",
		ThumbnailPaths: map[string]string{"300": "2024/01/abc_300.jpg"},
	}

	path := svc.ThumbnailPath(file, "300")
	if path != "/tmp/chronicle-test-media/2024/01/abc_300.jpg" {
		t.Errorf("unexpected thumbnail path: %s", path)
	}
}

func TestThumbnailPath_FallbackToMain(t *testing.T) {
	svc := newTestMediaService(&mockMediaRepo{})
	file := &MediaFile{
		Filename:       "2024/01/abc.jpg",
		ThumbnailPaths: map[string]string{},
	}

	path := svc.ThumbnailPath(file, "300")
	if path != "/tmp/chronicle-test-media/2024/01/abc.jpg" {
		t.Errorf("expected fallback to main file path, got: %s", path)
	}
}

// --- Upload Validation Tests (partial — testing only pre-disk logic) ---

func TestUpload_UnsupportedMimeType(t *testing.T) {
	svc := newTestMediaService(&mockMediaRepo{})

	input := UploadInput{
		UploadedBy: "user-1",
		MimeType:   "application/pdf",
		FileSize:   1024,
	}

	_, err := svc.Upload(context.Background(), input)
	assertMediaAppError(t, err, 400)
}

func TestUpload_FileTooLarge(t *testing.T) {
	svc := newTestMediaService(&mockMediaRepo{})

	input := UploadInput{
		UploadedBy: "user-1",
		MimeType:   "image/png",
		FileSize:   20 * 1024 * 1024, // 20 MB, exceeds 10 MB limit
	}

	_, err := svc.Upload(context.Background(), input)
	assertMediaAppError(t, err, 400)
}

func TestUpload_ConcurrencyLimit(t *testing.T) {
	svc := newTestMediaService(&mockMediaRepo{})

	// Saturate the semaphore for user-1.
	for i := 0; i < maxConcurrentUploadsPerUser; i++ {
		svc.sem.acquire("user-1")
	}

	input := UploadInput{
		UploadedBy: "user-1",
		MimeType:   "image/png",
		FileSize:   1024,
	}

	_, err := svc.Upload(context.Background(), input)
	assertMediaAppError(t, err, 400)

	// Cleanup.
	for i := 0; i < maxConcurrentUploadsPerUser; i++ {
		svc.sem.release("user-1")
	}
}

// --- checkQuotas Tests ---

func TestCheckQuotas_PerFileSizeExceeded(t *testing.T) {
	svc := newTestMediaService(&mockMediaRepo{})
	svc.limiter = &mockStorageLimiter{
		getEffectiveLimitsFn: func(ctx context.Context, userID, campaignID string) (int64, int64, int, error) {
			return 1024, 0, 0, nil // 1 KB max upload
		},
	}

	input := UploadInput{
		UploadedBy: "user-1",
		CampaignID: "camp-1",
		FileSize:   2048,
	}

	err := svc.checkQuotas(context.Background(), input)
	assertMediaAppError(t, err, 400)
}

func TestCheckQuotas_CampaignStorageExceeded(t *testing.T) {
	repo := &mockMediaRepo{
		getCampaignUsageFn: func(ctx context.Context, campaignID string) (int64, int, error) {
			return 9000, 5, nil // 9000 bytes used
		},
	}
	svc := newTestMediaService(repo)
	svc.limiter = &mockStorageLimiter{
		getEffectiveLimitsFn: func(ctx context.Context, userID, campaignID string) (int64, int64, int, error) {
			return 0, 10000, 0, nil // 10000 byte total limit
		},
	}

	input := UploadInput{
		UploadedBy: "user-1",
		CampaignID: "camp-1",
		FileSize:   2000, // 9000 + 2000 > 10000
	}

	err := svc.checkQuotas(context.Background(), input)
	assertMediaAppError(t, err, 400)
}

func TestCheckQuotas_FileCountExceeded(t *testing.T) {
	repo := &mockMediaRepo{
		getCampaignUsageFn: func(ctx context.Context, campaignID string) (int64, int, error) {
			return 1000, 100, nil // 100 files already
		},
	}
	svc := newTestMediaService(repo)
	svc.limiter = &mockStorageLimiter{
		getEffectiveLimitsFn: func(ctx context.Context, userID, campaignID string) (int64, int64, int, error) {
			return 0, 0, 100, nil // max 100 files
		},
	}

	input := UploadInput{
		UploadedBy: "user-1",
		CampaignID: "camp-1",
		FileSize:   1024,
	}

	err := svc.checkQuotas(context.Background(), input)
	assertMediaAppError(t, err, 400)
}

func TestCheckQuotas_LimiterError_FailsOpen(t *testing.T) {
	svc := newTestMediaService(&mockMediaRepo{})
	svc.limiter = &mockStorageLimiter{
		getEffectiveLimitsFn: func(ctx context.Context, userID, campaignID string) (int64, int64, int, error) {
			return 0, 0, 0, errors.New("settings service unavailable")
		},
	}

	input := UploadInput{
		UploadedBy: "user-1",
		CampaignID: "camp-1",
		FileSize:   1024,
	}

	// Should NOT return an error — fail-open behavior.
	err := svc.checkQuotas(context.Background(), input)
	if err != nil {
		t.Fatalf("expected nil error (fail-open), got %v", err)
	}
}

func TestCheckQuotas_NoCampaign_SkipsCampaignChecks(t *testing.T) {
	svc := newTestMediaService(&mockMediaRepo{})
	svc.limiter = &mockStorageLimiter{
		getEffectiveLimitsFn: func(ctx context.Context, userID, campaignID string) (int64, int64, int, error) {
			return 0, 0, 0, nil // unlimited
		},
	}

	input := UploadInput{
		UploadedBy: "user-1",
		CampaignID: "", // No campaign — avatar/backdrop
		FileSize:   1024,
	}

	err := svc.checkQuotas(context.Background(), input)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

// --- SetStorageLimiter Tests ---

func TestSetStorageLimiter(t *testing.T) {
	svc := newTestMediaService(&mockMediaRepo{})

	if svc.limiter != nil {
		t.Fatal("expected nil limiter initially")
	}

	limiter := &mockStorageLimiter{}
	svc.SetStorageLimiter(limiter)

	if svc.limiter == nil {
		t.Fatal("expected limiter to be set")
	}
}
