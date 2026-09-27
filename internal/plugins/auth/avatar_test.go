// avatar_test.go covers the avatar upload/clear slice: the service
// delegates entirely to the configured AvatarUploader (media pipeline)
// rather than doing any disk I/O itself, and the handler binds only from
// the caller's own session.

package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"

	"github.com/labstack/echo/v4"
)

// fakeAvatarUploader is a controllable AvatarUploader for service/handler tests.
type fakeAvatarUploader struct {
	calls       []avatarUploadCall
	mediaID     string
	url         string
	err         error
	deleteCalls []avatarDeleteCall
	deleteErr   error
}

type avatarUploadCall struct {
	userID       string
	fileBytes    []byte
	originalName string
	mimeType     string
}

type avatarDeleteCall struct {
	userID  string
	mediaID string
}

func (f *fakeAvatarUploader) UploadAvatar(ctx context.Context, userID string, fileBytes []byte, originalName, mimeType string) (string, string, error) {
	f.calls = append(f.calls, avatarUploadCall{userID, fileBytes, originalName, mimeType})
	if f.err != nil {
		return "", "", f.err
	}
	return f.mediaID, f.url, nil
}

func (f *fakeAvatarUploader) DeleteAvatarMedia(ctx context.Context, userID, mediaID string) error {
	f.deleteCalls = append(f.deleteCalls, avatarDeleteCall{userID, mediaID})
	return f.deleteErr
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

// TestAuthService_UploadAvatar_DeletesReplacedMedia pins that replacing an
// avatar deletes the one it replaced -- after the new one is safely
// persisted -- so the old file doesn't keep being served forever.
func TestAuthService_UploadAvatar_DeletesReplacedMedia(t *testing.T) {
	oldPath := "old-media-id"
	repo := &mockUserRepo{
		findByIDFn: func(ctx context.Context, id string) (*User, error) {
			return &User{ID: id, AvatarPath: &oldPath}, nil
		},
	}
	svc := NewAuthService(repo, nil, 0)
	uploader := &fakeAvatarUploader{mediaID: "new-media-id", url: "/media/new-media-id"}
	ConfigureAvatarUploader(svc, uploader)

	if _, _, err := svc.UploadAvatar(context.Background(), "user-1", []byte("bytes"), "photo.jpg", "image/jpeg"); err != nil {
		t.Fatalf("UploadAvatar: %v", err)
	}

	if len(uploader.deleteCalls) != 1 {
		t.Fatalf("expected exactly one delete call for the replaced avatar, got %d", len(uploader.deleteCalls))
	}
	call := uploader.deleteCalls[0]
	if call.userID != "user-1" || call.mediaID != "old-media-id" {
		t.Errorf("delete called with %+v, want {user-1 old-media-id}", call)
	}
}

// TestAuthService_UploadAvatar_NoPreviousAvatarSkipsDelete pins that a
// user's first avatar upload (no previous media to clean up) never calls
// DeleteAvatarMedia.
func TestAuthService_UploadAvatar_NoPreviousAvatarSkipsDelete(t *testing.T) {
	repo := &mockUserRepo{
		findByIDFn: func(ctx context.Context, id string) (*User, error) {
			return &User{ID: id}, nil // AvatarPath is nil.
		},
	}
	svc := NewAuthService(repo, nil, 0)
	uploader := &fakeAvatarUploader{mediaID: "new-media-id", url: "/media/new-media-id"}
	ConfigureAvatarUploader(svc, uploader)

	if _, _, err := svc.UploadAvatar(context.Background(), "user-1", []byte("bytes"), "photo.jpg", "image/jpeg"); err != nil {
		t.Fatalf("UploadAvatar: %v", err)
	}
	if len(uploader.deleteCalls) != 0 {
		t.Errorf("expected no delete call when there was no previous avatar, got %d", len(uploader.deleteCalls))
	}
}

// TestAuthService_ClearAvatar_DeletesMedia pins that clearing an avatar
// deletes its media file, not just the column pointing to it.
func TestAuthService_ClearAvatar_DeletesMedia(t *testing.T) {
	oldPath := "old-media-id"
	repo := &mockUserRepo{
		findByIDFn: func(ctx context.Context, id string) (*User, error) {
			return &User{ID: id, AvatarPath: &oldPath}, nil
		},
	}
	svc := NewAuthService(repo, nil, 0)
	uploader := &fakeAvatarUploader{}
	ConfigureAvatarUploader(svc, uploader)

	if err := svc.ClearAvatar(context.Background(), "user-1"); err != nil {
		t.Fatalf("ClearAvatar: %v", err)
	}
	if len(uploader.deleteCalls) != 1 {
		t.Fatalf("expected exactly one delete call, got %d", len(uploader.deleteCalls))
	}
	if call := uploader.deleteCalls[0]; call.userID != "user-1" || call.mediaID != "old-media-id" {
		t.Errorf("delete called with %+v, want {user-1 old-media-id}", call)
	}
}

// TestAuthService_ClearAvatar_NoPreviousAvatarSkipsDelete pins that clearing
// an already-cleared avatar never calls DeleteAvatarMedia.
func TestAuthService_ClearAvatar_NoPreviousAvatarSkipsDelete(t *testing.T) {
	repo := &mockUserRepo{
		findByIDFn: func(ctx context.Context, id string) (*User, error) {
			return &User{ID: id}, nil // AvatarPath is nil.
		},
	}
	svc := NewAuthService(repo, nil, 0)
	uploader := &fakeAvatarUploader{}
	ConfigureAvatarUploader(svc, uploader)

	if err := svc.ClearAvatar(context.Background(), "user-1"); err != nil {
		t.Fatalf("ClearAvatar: %v", err)
	}
	if len(uploader.deleteCalls) != 0 {
		t.Errorf("expected no delete call when there was no previous avatar, got %d", len(uploader.deleteCalls))
	}
}

// TestAuthService_UploadAvatar_RefreshesActiveSessions pins that a fresh
// upload is visible in the top bar immediately (via the cached Session, see
// layouts.GetUserAvatarPath), not only after the periodic revalidation.
func TestAuthService_UploadAvatar_RefreshesActiveSessions(t *testing.T) {
	repo := &mockUserRepo{}
	svc, mr := newTestAuthServiceWithRedis(t, repo)
	uploader := &fakeAvatarUploader{mediaID: "media-1", url: "/media/media-1"}
	ConfigureAvatarUploader(svc, uploader)

	token := seedSession(t, svc, mr, Session{UserID: "user-1", Name: "Alice"})
	if err := svc.redis.SAdd(context.Background(), userSessionsKeyPrefix+"user-1", token).Err(); err != nil {
		t.Fatalf("seeding user session set: %v", err)
	}

	if _, _, err := svc.UploadAvatar(context.Background(), "user-1", []byte("bytes"), "photo.jpg", "image/jpeg"); err != nil {
		t.Fatalf("UploadAvatar: %v", err)
	}

	session := readSession(t, svc, token)
	if session.AvatarPath != "media-1" {
		t.Errorf("session.AvatarPath = %q, want media-1", session.AvatarPath)
	}
	if session.Name != "Alice" {
		t.Errorf("refreshing the avatar must not disturb other session fields; Name = %q", session.Name)
	}
}

// TestAuthService_ClearAvatar_RefreshesActiveSessions is UploadAvatar's
// counterpart for removal: the cached session must stop pointing at the
// avatar the moment it's cleared.
func TestAuthService_ClearAvatar_RefreshesActiveSessions(t *testing.T) {
	repo := &mockUserRepo{}
	svc, mr := newTestAuthServiceWithRedis(t, repo)

	token := seedSession(t, svc, mr, Session{UserID: "user-1", AvatarPath: "old-media-id"})
	if err := svc.redis.SAdd(context.Background(), userSessionsKeyPrefix+"user-1", token).Err(); err != nil {
		t.Fatalf("seeding user session set: %v", err)
	}

	if err := svc.ClearAvatar(context.Background(), "user-1"); err != nil {
		t.Fatalf("ClearAvatar: %v", err)
	}

	session := readSession(t, svc, token)
	if session.AvatarPath != "" {
		t.Errorf("session.AvatarPath = %q, want cleared to empty", session.AvatarPath)
	}
}

// TestAuthService_RefreshSessionAvatar_KeyDeletedBeforeWrite_StaysGone mirrors
// the revalidateSession race fix: refreshSessionAvatar must not recreate a
// session key that was deleted (e.g. a concurrent logout) between listing
// the user's sessions and writing this one back.
func TestAuthService_RefreshSessionAvatar_KeyDeletedBeforeWrite_StaysGone(t *testing.T) {
	repo := &mockUserRepo{}
	svc, mr := newTestAuthServiceWithRedis(t, repo)

	token := seedSession(t, svc, mr, Session{UserID: "user-1", AvatarPath: "old-media-id"})
	if err := svc.redis.SAdd(context.Background(), userSessionsKeyPrefix+"user-1", token).Err(); err != nil {
		t.Fatalf("seeding user session set: %v", err)
	}
	key := sessionKeyPrefix + token
	mr.Del(key)

	newPath := "new-media-id"
	svc.refreshSessionAvatar(context.Background(), "user-1", &newPath)

	if mr.Exists(key) {
		t.Error("refreshSessionAvatar must not recreate a session key that was deleted concurrently")
	}
}

// readSession fetches and unmarshals a session directly from the service's
// Redis client, for asserting on fields a refresh touched.
func readSession(t *testing.T, svc *authService, token string) Session {
	t.Helper()
	data, err := svc.redis.Get(context.Background(), sessionKeyPrefix+token).Bytes()
	if err != nil {
		t.Fatalf("reading session: %v", err)
	}
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		t.Fatalf("unmarshaling session: %v", err)
	}
	return session
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
		// A real PNG signature named .jpg: the handler must sniff the
		// actual bytes rather than trust the declared Content-Type or the
		// filename's extension.
		fakePNG := pngMagicBytes + "rest-of-fake-image-bytes"
		req := newMultipartAvatarRequest(t, "photo.jpg", fakePNG)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set(contextKeyUserID, "user-1")

		var gotUserID, gotName, gotMimeType string
		var gotBytes []byte
		h := &Handler{service: &stubAvatarAuthService{
			uploadAvatarFn: func(ctx context.Context, userID string, fileBytes []byte, originalName, mimeType string) (string, string, error) {
				gotUserID = userID
				gotBytes = fileBytes
				gotName = originalName
				gotMimeType = mimeType
				return "media-1", "/media/media-1", nil
			},
		}}

		if err := h.UploadAvatarAPI(c); err != nil {
			t.Fatalf("UploadAvatarAPI: %v", err)
		}
		if gotUserID != "user-1" {
			t.Errorf("service.UploadAvatar called with userID=%q, want user-1", gotUserID)
		}
		if string(gotBytes) != fakePNG {
			t.Errorf("service.UploadAvatar called with bytes=%q, want %q", gotBytes, fakePNG)
		}
		if gotName != "photo.jpg" {
			t.Errorf("service.UploadAvatar called with originalName=%q, want photo.jpg", gotName)
		}
		if gotMimeType != "image/png" {
			t.Errorf("service.UploadAvatar called with mimeType=%q, want image/png (sniffed from bytes, not the .jpg extension)", gotMimeType)
		}
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}
		if body := rec.Body.String(); !bytes.Contains([]byte(body), []byte("/media/media-1")) {
			t.Errorf("response body = %s, want it to carry the returned url", body)
		}
	})

	t.Run("non-image content is refused even with a spoofed image Content-Type", func(t *testing.T) {
		e := echo.New()
		req := newMultipartAvatarRequestWithContentType(t, "photo.jpg", "image/jpeg", "not actually an image, just text")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set(contextKeyUserID, "user-1")

		h := &Handler{service: &stubAvatarAuthService{
			uploadAvatarFn: func(ctx context.Context, userID string, fileBytes []byte, originalName, mimeType string) (string, string, error) {
				t.Fatal("service.UploadAvatar must not be called for non-image content")
				return "", "", nil
			},
		}}

		if err := h.UploadAvatarAPI(c); err == nil {
			t.Fatal("expected a bad-request error for non-image content, even with a declared image Content-Type")
		}
	})
}

// pngMagicBytes is the 8-byte PNG file signature http.DetectContentType
// recognizes, used to build a fake-but-sniffable "image/png" payload.
const pngMagicBytes = "\x89PNG\r\n\x1a\n"

// newMultipartAvatarRequestWithContentType is newMultipartAvatarRequest but
// with an explicit, caller-chosen part Content-Type header -- used to prove
// the handler sniffs the actual bytes rather than trusting this header.
func newMultipartAvatarRequestWithContentType(t *testing.T, filename, contentType, content string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="avatar"; filename="%s"`, filename))
	header.Set("Content-Type", contentType)
	part, err := w.CreatePart(header)
	if err != nil {
		t.Fatalf("CreatePart: %v", err)
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
