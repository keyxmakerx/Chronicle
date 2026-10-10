// upload_authz_test.go pins that POST /media/upload checks the caller
// belongs to campaign_id (taken from the form body) and holds a role
// allowed to write to it, before Upload is reached.
//
// These tests drive the REAL Handler.Upload — never a reimplementation of
// the check — against a fake MediaService that only records whether Upload
// was reached.
package media

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// fakeUploadService is a minimal MediaService double for handler-level
// authorization tests. Only Upload is meaningfully exercised — the
// authorization gate under test must reject before any other method is
// ever reached, so every other method is an unused stub.
type fakeUploadService struct {
	uploadFn     func(ctx context.Context, input UploadInput) (*MediaFile, error)
	uploadCalled bool
}

func (f *fakeUploadService) Upload(ctx context.Context, input UploadInput) (*MediaFile, error) {
	f.uploadCalled = true
	if f.uploadFn != nil {
		return f.uploadFn(ctx, input)
	}
	return &MediaFile{ID: "new-file", ThumbnailPaths: map[string]string{}}, nil
}
func (f *fakeUploadService) GetByID(ctx context.Context, id string) (*MediaFile, error) {
	return nil, apperror.NewNotFound("media file not found")
}
func (f *fakeUploadService) Delete(ctx context.Context, id string) error { return nil }
func (f *fakeUploadService) FilePath(file *MediaFile) string             { return "" }
func (f *fakeUploadService) ThumbnailPath(file *MediaFile, size string) string {
	return ""
}
func (f *fakeUploadService) SetStorageLimiter(limiter StorageLimiter) {}
func (f *fakeUploadService) SetMemberChecker(checker MemberChecker)   {}
func (f *fakeUploadService) SetEntityVisibilityFilter(v EntityVisibilityFilter) {}
func (f *fakeUploadService) ListCampaignMedia(ctx context.Context, campaignID string, page, perPage int) ([]MediaFile, int, error) {
	return nil, 0, nil
}
func (f *fakeUploadService) GetCampaignStats(ctx context.Context, campaignID string) (*CampaignMediaStats, error) {
	return nil, nil
}
func (f *fakeUploadService) FindReferences(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error) {
	return nil, nil
}
func (f *fakeUploadService) DeleteCampaignMedia(ctx context.Context, campaignID, mediaID string) error {
	return nil
}
func (f *fakeUploadService) DeleteCampaignFiles(ctx context.Context, campaignID string) (int, error) {
	return 0, nil
}
func (f *fakeUploadService) CleanupOrphans(ctx context.Context) (int, error) { return 0, nil }
func (f *fakeUploadService) TrashFiles(ctx context.Context, batchID string, ids []string) (int, int64, error) {
	return 0, 0, nil
}
func (f *fakeUploadService) RestoreTrashedFiles(ctx context.Context, batchID string) (int, error) { return 0, nil }
func (f *fakeUploadService) ListTrashedFileIDs(ctx context.Context, batchID string) ([]string, error) {
	return nil, nil
}
func (f *fakeUploadService) PurgeTrashedFile(ctx context.Context, batchID, fileID string) error { return nil }
func (f *fakeUploadService) BackfillContentHashes(ctx context.Context, batchSize int) (int, error) {
	return 0, nil
}
func (f *fakeUploadService) ValidateMediaPath() error { return nil }

// newUploadTestContext builds a real multipart POST /media/upload request
// (file + optional campaign_id field) and an Echo context carrying an
// authenticated session for userID (or none, if userID == "").
func newUploadTestContext(t *testing.T, userID, campaignID string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "test.png")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte("fake-file-bytes")); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if campaignID != "" {
		if err := mw.WriteField("campaign_id", campaignID); err != nil {
			t.Fatalf("write campaign_id field: %v", err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/media/upload", &body)
	req.Header.Set(echo.HeaderContentType, mw.FormDataContentType())
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	if userID != "" {
		auth.SetSession(c, &auth.Session{UserID: userID})
	}
	return c, rec
}

// assertForbidden fails the test unless err is a *apperror.AppError with
// Code == http.StatusForbidden.
func assertForbidden(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a Forbidden error, got nil")
	}
	appErr, ok := err.(*apperror.AppError)
	if !ok {
		t.Fatalf("expected *apperror.AppError, got %T: %v", err, err)
	}
	if appErr.Code != http.StatusForbidden {
		t.Errorf("expected status %d (Forbidden), got %d (message: %s)", http.StatusForbidden, appErr.Code, appErr.Message)
	}
}

// TestUpload_NonMemberForbidden pins that an authenticated user who is not
// a member of the target campaign cannot write media into it, no matter
// what campaign_id they name in the form body.
func TestUpload_NonMemberForbidden(t *testing.T) {
	svc := &fakeUploadService{}
	h := &Handler{
		service:       svc,
		memberChecker: &stubMemberChecker{}, // no memberships/roles stubbed — everyone resolves to RoleNone
	}
	c, _ := newUploadTestContext(t, "attacker", "camp-victim")

	err := h.Upload(c)

	assertForbidden(t, err)
	if svc.uploadCalled {
		t.Error("service.Upload must not be called for a non-member's upload — the write must never reach the service layer")
	}
}

// TestUpload_PlayerRoleForbidden pins the threshold: mere membership isn't
// enough. Every other campaign-scoped media write route requires Scribe+
// (the media-picker list and entity image endpoints are Scribe,
// CampaignDeleteMedia is Owner) — Upload must match, not merely require
// "is a member".
func TestUpload_PlayerRoleForbidden(t *testing.T) {
	svc := &fakeUploadService{}
	h := &Handler{
		service: svc,
		memberChecker: &stubMemberChecker{
			roles: map[string]map[string]int{"camp-1": {"player-user": 1}}, // RolePlayer
		},
	}
	c, _ := newUploadTestContext(t, "player-user", "camp-1")

	err := h.Upload(c)

	assertForbidden(t, err)
	if svc.uploadCalled {
		t.Error("service.Upload must not be called for a Player-role member — Upload requires Scribe+")
	}
}

// TestUpload_ScribeMemberAllowed is the positive control: a genuine
// Scribe+ member of the named campaign must still be able to upload,
// proving the authorization gate doesn't overcorrect into blocking
// legitimate uploads.
func TestUpload_ScribeMemberAllowed(t *testing.T) {
	svc := &fakeUploadService{}
	h := &Handler{
		service: svc,
		memberChecker: &stubMemberChecker{
			roles: map[string]map[string]int{"camp-1": {"scribe-user": 2}}, // RoleScribe
		},
	}
	c, _ := newUploadTestContext(t, "scribe-user", "camp-1")

	err := h.Upload(c)

	if err != nil {
		t.Fatalf("expected Scribe member's upload to succeed, got error: %v", err)
	}
	if !svc.uploadCalled {
		t.Error("service.Upload should have been called for an authorized Scribe member")
	}
}

// TestUpload_DedupHitNeverReachedByNonMember pins that the dedup
// short-circuit in mediaService.Upload (FindByContentHash), which hands
// back an EXISTING file's id and would let the handler mint that file a
// fresh signed URL, is never reached by a non-member. uploadFn here plays
// the part of that short-circuit, returning a file the caller did not just
// create — the authorization gate must stop a non-member from ever
// reaching Upload at all.
func TestUpload_DedupHitNeverReachedByNonMember(t *testing.T) {
	const victimFileID = "victim-existing-file"
	svc := &fakeUploadService{
		uploadFn: func(ctx context.Context, input UploadInput) (*MediaFile, error) {
			// Simulates a per-campaign dedup hit: an existing record in the
			// target campaign, not something this call created.
			return &MediaFile{ID: victimFileID, CampaignID: &input.CampaignID}, nil
		},
	}
	h := &Handler{
		service:       svc,
		memberChecker: &stubMemberChecker{},
	}
	c, rec := newUploadTestContext(t, "attacker", "camp-victim")

	err := h.Upload(c)

	assertForbidden(t, err)
	if svc.uploadCalled {
		t.Errorf("the dedup-capable Upload path must never be reached for a non-member — this is finding 1(b)'s credential oracle. Response body: %s", rec.Body.String())
	}
}
