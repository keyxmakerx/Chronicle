package notes

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

type recordingPictures struct {
	calls                    int
	campaignID, userID, mime string
	size                     int
	id                       string
	err                      error
}

func (r *recordingPictures) UploadPicture(_ context.Context, campaignID, userID string, b []byte, _ string, mime string) (string, error) {
	r.calls++
	r.campaignID, r.userID, r.mime, r.size = campaignID, userID, mime, len(b)
	return r.id, r.err
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 3, 3))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func pictureRequest(t *testing.T, role campaigns.Role, file []byte, declared string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if file != nil {
		part, err := mw.CreatePart(map[string][]string{
			"Content-Disposition": {`form-data; name="file"; filename="p.png"`},
			"Content-Type":        {declared},
		})
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write(file)
	}
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/campaigns/c1/notes/pictures", &body)
	req.Header.Set(echo.HeaderContentType, mw.FormDataContentType())
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: role, IsMember: true})
	auth.SetSession(c, &auth.Session{UserID: "u-player"})
	return c, rec
}

func TestUploadPicture(t *testing.T) {
	big := append(pngBytes(t), bytes.Repeat([]byte{0}, maxNotePictureBytes)...)
	tests := []struct {
		name     string
		file     []byte
		declared string
		wired    bool
		status   int // 0 = success
		stored   bool
	}{
		{name: "a player's PNG", file: pngBytes(t), declared: "image/png", wired: true, stored: true},
		{name: "a PNG the browser calls octet-stream", file: pngBytes(t), declared: "application/octet-stream", wired: true, stored: true},
		{name: "HTML that claims to be a PNG", file: []byte("<html><script>alert(1)</script>"), declared: "image/png", wired: true, status: 400},
		{name: "audio is for attachments, not pictures", file: []byte("ID3\x03\x00\x00\x00\x00\x00\x00"), declared: "audio/mpeg", wired: true, status: 400},
		{name: "over the note picture limit", file: big, declared: "image/png", wired: true, status: 400},
		{name: "no file", file: nil, wired: true, status: 400},
		{name: "not configured", file: pngBytes(t), declared: "image/png", wired: false, status: 400},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			up := &recordingPictures{id: "0b6a3f0e-8c1d-4f6a-9a51-2f3c4d5e6f70"}
			h := NewHandler(newAccessSvc())
			if tc.wired {
				h.SetPictureUploader(up)
			}
			// A plain Player: this route must not ask for Scribe.
			c, rec := pictureRequest(t, campaigns.RolePlayer, tc.file, tc.declared)
			err := h.UploadPicture(c)
			if tc.status != 0 {
				wantStatus(t, err, tc.status)
				if up.calls != 0 {
					t.Errorf("a refused picture still reached storage")
				}
				return
			}
			if err != nil {
				t.Fatalf("upload: %v", err)
			}
			if rec.Code != http.StatusCreated {
				t.Errorf("status %d, want 201", rec.Code)
			}
			var got map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got["id"] != up.id || got["url"] != "/media/"+up.id {
				t.Errorf("response %v, want the plain /media/<id> path (never a signed link)", got)
			}
			if up.campaignID != "c1" || up.userID != "u-player" || up.mime != "image/png" {
				t.Errorf("stored as campaign %q user %q type %q", up.campaignID, up.userID, up.mime)
			}
		})
	}
}

// A note body keeps its pictures through the same sanitizer pages use, and a
// picture cannot smuggle script in.
func TestUpdate_KeepsAPictureInTheBody(t *testing.T) {
	const id = "0b6a3f0e-8c1d-4f6a-9a51-2f3c4d5e6f70"
	existing := sampleNote()
	svc := NewNoteService(&mockNoteRepo{findByIDFn: func(context.Context, string) (*Note, error) { return existing, nil }})
	html := `<p>Look</p><figure class="ce-img ce-img--w40 ce-img--right"><img src="/media/` + id + `" alt="Mira" onerror="x()"><figcaption>Mira</figcaption></figure>`
	note, err := svc.Update(context.Background(), "note-123", player("user-1"), UpdateNoteRequest{EntryHTML: &html})
	if err != nil {
		t.Fatal(err)
	}
	got := *note.EntryHTML
	for _, want := range []string{`<figure class="ce-img ce-img--w40 ce-img--right">`, `src="/media/` + id + `"`, `<figcaption>Mira</figcaption>`} {
		if !strings.Contains(got, want) {
			t.Errorf("body lost %q: %s", want, got)
		}
	}
	if strings.Contains(got, "onerror") {
		t.Errorf("body kept a script handler: %s", got)
	}
}

// An edit may bind new pictures; a restore may not, because the text it brings
// back was written by someone else at another time.
func TestSaves_SayWhetherTheyMayBindPictures(t *testing.T) {
	html := "<p>old</p>"
	tests := []struct {
		name     string
		run      func(svc NoteService) error
		wantBind bool
	}{
		{"edit", func(svc NoteService) error {
			body := "<p>new</p>"
			_, err := svc.Update(context.Background(), "note-123", player("user-1"), UpdateNoteRequest{EntryHTML: &body})
			return err
		}, true},
		{"restore", func(svc NoteService) error {
			_, err := svc.RestoreVersion(context.Background(), "note-123", "v1", "user-1")
			return err
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockNoteRepo{
				findByIDFn: func(context.Context, string) (*Note, error) { return sampleNote(), nil },
				findVersionByIDFn: func(context.Context, string) (*NoteVersion, error) {
					return &NoteVersion{ID: "v1", NoteID: "note-123", EntryHTML: &html}, nil
				},
			}
			if err := tt.run(NewNoteService(repo)); err != nil {
				t.Fatal(err)
			}
			if repo.lastBindNew == nil || *repo.lastBindNew != tt.wantBind {
				t.Errorf("bindNew = %v, want %v", repo.lastBindNew, tt.wantBind)
			}
		})
	}
}
