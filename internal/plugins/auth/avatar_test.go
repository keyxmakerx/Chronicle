// avatar_test.go covers the M0 avatar-upload slice (#610, #733): the
// service delegates entirely to the configured AvatarUploader (media
// pipeline) rather than doing any disk I/O itself, and the handler binds
// only from the caller's own session.

package auth

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

// fakeAvatarUploader is a controllable AvatarUploader for service/handler tests.
type fakeAvatarUploader struct {
	calls   []avatarUploadCall
	mediaID string
	url     string
	err     error
}

type avatarUploadCall struct {
	userID       string
	fileBytes    []byte
	originalName string
	mimeType     string
}

func (f *fakeAvatarUploader) UploadAvatar(ctx context.Context, userID string, fileBytes []byte, originalName, mimeType string) (string, string, error) {
	f.calls = append(f.calls, avatarUploadCall{userID, fileBytes, originalName, mimeType})
	if f.err != nil {
		return "", "", f.err
	}
	return f.mediaID, f.url, nil
}

func TestAuthService_UploadAvatar(t *testing.T) {
	tests := []struct {
		name          string
		configureFn   bool // whether ConfigureAvatarUploader is called
		uploaderErr   error
		wantErr       bool
		wantMediaID   string
		wantRepoWrite bool
	}{
		{
			name:          "delegates to the media uploader and persists the returned id",
			configureFn:   true,
			wantMediaID:   "media-123",
			wantRepoWrite: true,
		},
		{
			name:        "no uploader configured fails closed",
			configureFn: false,
			wantErr:     true,
		},
		{
			name:        "uploader error is propagated without touching the repo",
			configureFn: true,
			uploaderErr: errors.New("quota exceeded"),
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var wroteUserID string
			var wrotePath *string
			repo := &mockUserRepo{
				updateAvatarPathFn: func(ctx context.Context, userID string, avatarPath *string) error {
					wroteUserID = userID
					wrotePath = avatarPath
					return nil
				},
			}
			svc := NewAuthService(repo, nil, 0)
			uploader := &fakeAvatarUploader{mediaID: "media-123", url: "/media/media-123", err: tt.uploaderErr}
			if tt.configureFn {
				ConfigureAvatarUploader(svc, uploader)
			}

			mediaID, url, err := svc.UploadAvatar(context.Background(), "user-1", []byte("bytes"), "photo.jpg", "image/jpeg")

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got mediaID=%q url=%q", mediaID, url)
				}
				if wroteUserID != "" {
					t.Errorf("repo.UpdateAvatarPath must not be called when the upload fails; got userID=%q", wroteUserID)
				}
				return
			}
			if err != nil {
				t.Fatalf("UploadAvatar: %v", err)
			}
			if mediaID != tt.wantMediaID {
				t.Errorf("mediaID = %q, want %q", mediaID, tt.wantMediaID)
			}
			if len(uploader.calls) != 1 {
				t.Fatalf("expected exactly one delegated upload call, got %d", len(uploader.calls))
			}
			call := uploader.calls[0]
			if call.userID != "user-1" || string(call.fileBytes) != "bytes" || call.originalName != "photo.jpg" || call.mimeType != "image/jpeg" {
				t.Errorf("uploader called with unexpected params: %+v", call)
			}
			if tt.wantRepoWrite {
				if wroteUserID != "user-1" {
					t.Errorf("repo.UpdateAvatarPath called with userID=%q, want user-1", wroteUserID)
				}
				if wrotePath == nil || *wrotePath != tt.wantMediaID {
					t.Errorf("repo.UpdateAvatarPath called with avatarPath=%v, want &%q", wrotePath, tt.wantMediaID)
				}
			}
		})
	}
}

func TestAuthService_ClearAvatar(t *testing.T) {
	var clearedUserID string
	var clearedPath *string
	repo := &mockUserRepo{
		updateAvatarPathFn: func(ctx context.Context, userID string, avatarPath *string) error {
			clearedUserID = userID
			clearedPath = avatarPath
			return nil
		},
	}
	svc := NewAuthService(repo, nil, 0)

	if err := svc.ClearAvatar(context.Background(), "user-1"); err != nil {
		t.Fatalf("ClearAvatar: %v", err)
	}
	if clearedUserID != "user-1" {
		t.Errorf("cleared userID = %q, want user-1", clearedUserID)
	}
	if clearedPath != nil {
		t.Errorf("ClearAvatar must write a nil avatar_path, got %v", clearedPath)
	}
}

// stubAvatarAuthService embeds a nil AuthService so only the two avatar
// methods under test need implementing; calling any other method panics,
// which is exactly what should happen if the handler starts reaching for
// something outside the avatar flow.
type stubAvatarAuthService struct {
	AuthService
	uploadAvatarFn func(ctx context.Context, userID string, fileBytes []byte, originalName, mimeType string) (string, string, error)
	clearAvatarFn  func(ctx context.Context, userID string) error
}

func (s *stubAvatarAuthService) UploadAvatar(ctx context.Context, userID string, fileBytes []byte, originalName, mimeType string) (string, string, error) {
	return s.uploadAvatarFn(ctx, userID, fileBytes, originalName, mimeType)
}

func (s *stubAvatarAuthService) ClearAvatar(ctx context.Context, userID string) error {
	return s.clearAvatarFn(ctx, userID)
}

// newMultipartAvatarRequest builds a POST with a single "avatar" file part.
func newMultipartAvatarRequest(t *testing.T, filename, content string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("avatar", filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatalf("writing part: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("closing writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/account/avatar", &buf)
	req.Header.Set(echo.HeaderContentType, w.FormDataContentType())
	return req
}

func TestHandler_UploadAvatarAPI(t *testing.T) {
	t.Run("unauthenticated request is rejected", func(t *testing.T) {
		e := echo.New()
		req := newMultipartAvatarRequest(t, "photo.jpg", "bytes")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		h := &Handler{service: &stubAvatarAuthService{
			uploadAvatarFn: func(ctx context.Context, userID string, fileBytes []byte, originalName, mimeType string) (string, string, error) {
				t.Fatal("service.UploadAvatar must not be called for an unauthenticated request")
				return "", "", nil
			},
		}}
		if err := h.UploadAvatarAPI(c); err == nil {
			t.Fatal("expected an unauthorized error")
		}
	})

	t.Run("goes through the media-backed service, not direct disk I/O", func(t *testing.T) {
		e := echo.New()
		req := newMultipartAvatarRequest(t, "photo.jpg", "fake-image-bytes")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set(contextKeyUserID, "user-1")

		var gotUserID, gotName string
		var gotBytes []byte
		h := &Handler{service: &stubAvatarAuthService{
			uploadAvatarFn: func(ctx context.Context, userID string, fileBytes []byte, originalName, mimeType string) (string, string, error) {
				gotUserID = userID
				gotBytes = fileBytes
				gotName = originalName
				return "media-1", "/media/media-1", nil
			},
		}}

		if err := h.UploadAvatarAPI(c); err != nil {
			t.Fatalf("UploadAvatarAPI: %v", err)
		}
		if gotUserID != "user-1" {
			t.Errorf("service.UploadAvatar called with userID=%q, want user-1", gotUserID)
		}
		if string(gotBytes) != "fake-image-bytes" {
			t.Errorf("service.UploadAvatar called with bytes=%q, want fake-image-bytes", gotBytes)
		}
		if gotName != "photo.jpg" {
			t.Errorf("service.UploadAvatar called with originalName=%q, want photo.jpg", gotName)
		}
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}
		if body := rec.Body.String(); !bytes.Contains([]byte(body), []byte("/media/media-1")) {
			t.Errorf("response body = %s, want it to carry the returned url", body)
		}
	})
}

func TestHandler_ClearAvatarAPI(t *testing.T) {
	t.Run("unauthenticated request is rejected", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodDelete, "/account/avatar", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		h := &Handler{service: &stubAvatarAuthService{
			clearAvatarFn: func(ctx context.Context, userID string) error {
				t.Fatal("service.ClearAvatar must not be called for an unauthenticated request")
				return nil
			},
		}}
		if err := h.ClearAvatarAPI(c); err == nil {
			t.Fatal("expected an unauthorized error")
		}
	})

	t.Run("clears only the calling user's own avatar", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodDelete, "/account/avatar", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set(contextKeyUserID, "user-1")

		var gotUserID string
		h := &Handler{service: &stubAvatarAuthService{
			clearAvatarFn: func(ctx context.Context, userID string) error {
				gotUserID = userID
				return nil
			},
		}}
		if err := h.ClearAvatarAPI(c); err != nil {
			t.Fatalf("ClearAvatarAPI: %v", err)
		}
		if gotUserID != "user-1" {
			t.Errorf("service.ClearAvatar called with userID=%q, want user-1 (the caller's own session, never a request param)", gotUserID)
		}
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}
	})
}
