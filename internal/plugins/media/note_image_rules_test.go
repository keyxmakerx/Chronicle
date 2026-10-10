// note_image_rules_test.go pins the parts of the note-picture design that sit
// around the access rule: where the usage type can come from, what upload and
// delete do with it, and which links get signed.
package media

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// A note picture is never merged with, nor looked up against, another file of
// the same bytes: whoever uploads it gets a file of their own, so its readers
// never depend on a stranger's identical upload.
func TestUpload_NotePictureIsNeverDeduplicated(t *testing.T) {
	lookedUp := false
	var created *MediaFile
	repo := &mockMediaRepo{
		findByContentHashFn: func(context.Context, string, string) (*MediaFile, error) {
			lookedUp = true
			return &MediaFile{ID: "someone-elses", MimeType: "image/png"}, nil
		},
		createFn: func(_ context.Context, f *MediaFile) error { created = f; return nil },
	}
	svc := newTestMediaService(repo)
	svc.mediaPath = t.TempDir()

	got, err := svc.Upload(context.Background(), UploadInput{
		CampaignID: "camp-1", UploadedBy: "ana", MimeType: "image/png",
		UsageType: UsageNoteImage, FileBytes: tinyPNG(t), FileSize: 10,
	})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if lookedUp {
		t.Error("a note picture upload consulted the content-hash dedup")
	}
	if got.ID == "someone-elses" || created == nil || created.UsageType != UsageNoteImage || created.UploadedBy != "ana" {
		t.Errorf("want a fresh note_image row owned by ana, got %+v", created)
	}
}

func TestUpload_NotePictureNeedsAnImageAndACampaign(t *testing.T) {
	tests := []struct {
		name  string
		input UploadInput
	}{
		{"audio", UploadInput{CampaignID: "camp-1", MimeType: "audio/mpeg", UsageType: UsageNoteImage, FileBytes: []byte("ID3"), FileSize: 3}},
		{"no campaign", UploadInput{MimeType: "image/png", UsageType: UsageNoteImage, FileBytes: []byte("x"), FileSize: 1}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.input.UploadedBy = "ana"
			_, err := newTestMediaService(&mockMediaRepo{}).Upload(context.Background(), tc.input)
			assertMediaAppError(t, err, http.StatusBadRequest)
		})
	}
}

// The campaign owner's media browser cannot remove a player's note picture.
func TestDeleteCampaignMedia_RefusesANotePicture(t *testing.T) {
	camp := "camp-1"
	deleted := false
	repo := &mockMediaRepo{
		findByIDFn: func(context.Context, string) (*MediaFile, error) {
			return &MediaFile{ID: "n1", CampaignID: &camp, UsageType: UsageNoteImage}, nil
		},
		deleteFn: func(context.Context, string) error { deleted = true; return nil },
	}
	err := newTestMediaService(repo).DeleteCampaignMedia(context.Background(), camp, "n1")
	assertMediaAppError(t, err, http.StatusNotFound)
	if deleted {
		t.Error("the note picture was deleted")
	}
}

// /media/upload will not take the note-picture usage type from a form field,
// even from a Scribe: that would mint files outside the page rule.
func TestHandlerUpload_RefusesNoteImageUsageType(t *testing.T) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, _ := mw.CreateFormFile("file", "a.png")
	_, _ = part.Write(tinyPNG(t))
	_ = mw.WriteField("campaign_id", "camp-1")
	_ = mw.WriteField("usage_type", UsageNoteImage)
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/media/upload", &body)
	req.Header.Set(echo.HeaderContentType, mw.FormDataContentType())
	c := echo.New().NewContext(req, httptest.NewRecorder())
	auth.SetSession(c, &auth.Session{UserID: "scribe"})

	svc := &fakeUploadService{}
	h := &Handler{service: svc, memberChecker: &stubMemberChecker{roles: map[string]map[string]int{"camp-1": {"scribe": 2}}}}
	err := h.Upload(c)
	if err == nil || svc.uploadCalled {
		t.Fatalf("want a refusal before the service is reached, err=%v called=%v", err, svc.uploadCalled)
	}
}

// Member links (the Foundry notebook) sign a note picture only for someone
// who may read it, and sign nothing for a stranger or for an id from elsewhere.
func TestSignedLinksForMember_NotePictures(t *testing.T) {
	const (
		readable = "77777777-7777-4777-8777-777777777777"
		hidden   = "88888888-8888-4888-8888-888888888888"
		mine     = "99999999-9999-4999-8999-999999999999"
	)
	camp := "camp-l"
	h := newLinkHandler()
	svc := h.service.(*linkFilesService)
	for _, id := range []string{readable, hidden, mine} {
		uploader := "someone" // a member, so their pictures still count
		if id == mine {
			uploader = "player"
		}
		svc.files[id] = &MediaFile{ID: id, CampaignID: &camp, CampaignIsPublic: boolPtr(false), UsageType: UsageNoteImage, UploadedBy: uploader}
	}
	h.memberChecker.(*stubMemberChecker).members["camp-l"]["someone"] = true
	h.noteMedia = noteMediaByFile{readable: {"player": true}}

	paths := []string{"/media/" + readable, "/media/" + hidden, "/media/" + mine + "/thumb/300"}
	got := h.SignedLinksForMember(context.Background(), "camp-l", "player", paths)
	if len(got) != 2 || got[paths[0]] == "" || got[paths[2]] == "" || got[paths[1]] != "" {
		t.Fatalf("want links for the readable and the own picture only, got %v", got)
	}
}

// noteMediaByFile answers per file, for tests that need several pictures.
type noteMediaByFile map[string]map[string]bool

func (n noteMediaByFile) CanReadNoteMedia(_ context.Context, _, mediaID string, _ int, userID string) (bool, error) {
	return n[mediaID][userID], nil
}

// A note picture is never cacheable by a shared cache, in a public campaign
// too: who may read it changes as its note's sharing does.
func TestSetSecurityHeaders_NotePictureIsNeverCached(t *testing.T) {
	camp := "camp-1"
	tests := []struct {
		name string
		file *MediaFile
		want string
	}{
		{"note picture, public campaign", &MediaFile{CampaignID: &camp, CampaignIsPublic: boolPtr(true), UsageType: UsageNoteImage}, "private, no-store, max-age=0"},
		{"note picture, private campaign", &MediaFile{CampaignID: &camp, CampaignIsPublic: boolPtr(false), UsageType: UsageNoteImage}, "private, no-store, max-age=0"},
		{"page picture, public campaign keeps its cache", &MediaFile{CampaignID: &camp, CampaignIsPublic: boolPtr(true), UsageType: UsageEntityImage}, "public, max-age=31536000, immutable"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := newAccessTestContext(nil, nil)
			(&Handler{}).setSecurityHeaders(c, tc.file)
			if got := c.Response().Header().Get("Cache-Control"); got != tc.want {
				t.Errorf("Cache-Control = %q, want %q", got, tc.want)
			}
		})
	}
}

// The orphan sweep also collects note pictures no note holds, but only ones
// older than a day.
func TestCleanupOrphans_CollectsUnboundNotePictures(t *testing.T) {
	var cutoff time.Time
	var deleted []string
	camp := "camp-1"
	repo := &mockMediaRepo{
		listUnboundFn: func(_ context.Context, olderThan time.Time) ([]string, error) {
			cutoff = olderThan
			return []string{"stale-1", "stale-2"}, nil
		},
		findByIDFn: func(_ context.Context, id string) (*MediaFile, error) {
			return &MediaFile{ID: id, CampaignID: &camp, Filename: id + ".png", UsageType: UsageNoteImage}, nil
		},
		deleteFn: func(_ context.Context, id string) error { deleted = append(deleted, id); return nil },
	}
	svc := newTestMediaService(repo)
	svc.mediaPath = t.TempDir()

	n, err := svc.CleanupOrphans(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 || len(deleted) != 2 {
		t.Errorf("removed %d, deleted %v; want both unbound pictures", n, deleted)
	}
	if age := time.Since(cutoff); age < 23*time.Hour || age > 25*time.Hour {
		t.Errorf("cutoff is %v old, want about 24h so a note still being written keeps its picture", age)
	}
}
