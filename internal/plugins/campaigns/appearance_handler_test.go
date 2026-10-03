package campaigns

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// appearanceSvcStub embeds the interface so only SaveAppearance is real; any
// other call panics, which would mean the handler strayed from its job.
type appearanceSvcStub struct {
	CampaignService
	saved int
	last  AppearanceInput
}

func (s *appearanceSvcStub) SaveAppearance(_ context.Context, _ string, in AppearanceInput) error {
	s.saved++
	s.last = in
	return nil
}

type appearanceMediaStub struct {
	owns       bool
	ownsCalls  int
	uploads    int
	uploadName string
	gotMime    string

	// deleteResult is what DeletePicture reports: the adapter's own
	// ownership and usage-type check, which these handler tests stand in for.
	deleteResult bool
	deleteCalls  []string
}

func (m *appearanceMediaStub) UploadBackdrop(_ context.Context, _, _ string, _ []byte, _, mimeType string) (string, error) {
	m.uploads++
	m.gotMime = mimeType
	return m.uploadName, nil
}

func (m *appearanceMediaStub) DeletePicture(_ context.Context, campaignID, filename string) (bool, error) {
	m.deleteCalls = append(m.deleteCalls, campaignID+"|"+filename)
	return m.deleteResult, nil
}

func (m *appearanceMediaStub) OwnsFile(_ context.Context, _, _ string) (bool, error) {
	m.ownsCalls++
	return m.owns, nil
}

// pngBytes is a valid 1x1 PNG header plus padding; DetectContentType only
// needs the signature.
func pngBytes(extra int) []byte {
	sig := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00")
	return append(sig, make([]byte, extra)...)
}

func ownerContext(role Role, settings string, backdrop *string) *CampaignContext {
	return &CampaignContext{
		Campaign:   &Campaign{ID: "camp-1", Name: "Test", Settings: settings, BackdropPath: backdrop},
		MemberRole: role,
	}
}

func saveRequest(t *testing.T, h *Handler, cc *CampaignContext, body any) error {
	t.Helper()
	raw, _ := json.Marshal(body)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/campaigns/camp-1/appearance", bytes.NewReader(raw))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	c := e.NewContext(req, httptest.NewRecorder())
	c.Set(contextKeyCampaign, cc)
	return h.SaveAppearanceAPI(c)
}

func TestSaveAppearanceAPI(t *testing.T) {
	storedBackdrop := "stored-backdrop.png"
	cases := []struct {
		name      string
		role      Role
		settings  string
		backdrop  *string
		body      map[string]any
		owns      bool
		noMedia   bool
		wantCode  int // 0 = success
		wantSaved int
		wantOwns  int
	}{
		{"non-owner forbidden", RoleScribe, "", nil, map[string]any{}, true, false, http.StatusForbidden, 0, 0},
		{"player forbidden", RolePlayer, "", nil, map[string]any{}, true, false, http.StatusForbidden, 0, 0},
		{"owner without pictures", RoleOwner, "", nil, map[string]any{}, false, false, 0, 1, 0},
		{"unowned picture refused before service", RoleOwner, "", nil,
			map[string]any{"brand": map[string]any{"logo": "foreign.png"}}, false, false, http.StatusBadRequest, 0, 1},
		{"owned picture accepted", RoleOwner, "", nil,
			map[string]any{"brand": map[string]any{"logo": "mine.png"}}, true, false, 0, 1, 1},
		{"picture equal to stored logo skips ownership check", RoleOwner, `{"brand_logo":"logo.png"}`, nil,
			map[string]any{"brand": map[string]any{"logo": "logo.png"}}, false, false, 0, 1, 0},
		{"picture equal to stored header image skips ownership check", RoleOwner,
			`{"topbar_style":{"mode":"image","image_path":"hdr.png"}}`, nil,
			map[string]any{"header": map[string]any{"bg": "image", "image": "hdr.png"}}, false, false, 0, 1, 0},
		{"picture equal to stored backdrop skips ownership check", RoleOwner, "", &storedBackdrop,
			map[string]any{"brand": map[string]any{"backdrop": storedBackdrop}}, false, false, 0, 1, 0},
		{"new picture with no uploader refused", RoleOwner, "", nil,
			map[string]any{"brand": map[string]any{"logo": "new.png"}}, true, true, http.StatusBadRequest, 0, 0},
		{"one foreign picture among stored ones refused", RoleOwner, `{"brand_logo":"logo.png"}`, nil,
			map[string]any{"brand": map[string]any{"logo": "logo.png", "backdrop": "foreign.png"}}, false, false, http.StatusBadRequest, 0, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &appearanceSvcStub{}
			h := NewHandler(svc)
			media := &appearanceMediaStub{owns: tc.owns}
			if !tc.noMedia {
				h.SetMediaUploader(media)
			}
			err := saveRequest(t, h, ownerContext(tc.role, tc.settings, tc.backdrop), tc.body)
			if tc.wantCode != 0 {
				ae, ok := err.(*apperror.AppError)
				if !ok || ae.Code != tc.wantCode {
					t.Fatalf("err = %v, want code %d", err, tc.wantCode)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if svc.saved != tc.wantSaved {
				t.Errorf("SaveAppearance calls = %d, want %d", svc.saved, tc.wantSaved)
			}
			if media.ownsCalls != tc.wantOwns {
				t.Errorf("OwnsFile calls = %d, want %d", media.ownsCalls, tc.wantOwns)
			}
		})
	}
}

func TestSaveAppearanceAPI_MissingContext(t *testing.T) {
	h := NewHandler(&appearanceSvcStub{})
	e := echo.New()
	c := e.NewContext(httptest.NewRequest(http.MethodPut, "/", nil), httptest.NewRecorder())
	if err := h.SaveAppearanceAPI(c); err == nil {
		t.Fatal("expected an error without a campaign context")
	}
}

func uploadRequest(t *testing.T, h *Handler, cc *CampaignContext, kind, filename string, data []byte) (*httptest.ResponseRecorder, error) {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if kind != "" {
		_ = w.WriteField("kind", kind)
	}
	if data != nil {
		fw, _ := w.CreateFormFile("file", filename)
		_, _ = fw.Write(data)
	}
	_ = w.Close()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/campaigns/camp-1/appearance/picture", &body)
	req.Header.Set(echo.HeaderContentType, w.FormDataContentType())
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(contextKeyCampaign, cc)
	err := h.UploadAppearancePictureAPI(c)
	return rec, err
}

func TestUploadAppearancePictureAPI(t *testing.T) {
	cases := []struct {
		name     string
		role     Role
		kind     string
		data     []byte
		filename string
		wantCode int
	}{
		{"non-owner forbidden", RoleScribe, "logo", pngBytes(10), "a.png", http.StatusForbidden},
		{"unknown kind", RoleOwner, "avatar", pngBytes(10), "a.png", http.StatusBadRequest},
		{"missing kind", RoleOwner, "", pngBytes(10), "a.png", http.StatusBadRequest},
		{"no file", RoleOwner, "logo", nil, "", http.StatusBadRequest},
		{"logo over 1 MB", RoleOwner, "logo", pngBytes(1<<20 + 1), "a.png", http.StatusBadRequest},
		{"backdrop over 4 MB", RoleOwner, "backdrop", pngBytes(4<<20 + 1), "a.png", http.StatusBadRequest},
		{"header over 1.5 MB", RoleOwner, "header", pngBytes(3<<19 + 1), "a.png", http.StatusBadRequest},
		{"non-image bytes", RoleOwner, "logo", []byte("just some text, definitely not a picture"), "a.png", http.StatusBadRequest},
		{"html disguised as png", RoleOwner, "logo", []byte("<html><script>alert(1)</script></html>"), "a.png", http.StatusBadRequest},
		{"logo png ok", RoleOwner, "logo", pngBytes(100), "a.png", 0},
		{"backdrop larger than logo limit ok", RoleOwner, "backdrop", pngBytes(2 << 20), "a.png", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHandler(&appearanceSvcStub{})
			media := &appearanceMediaStub{uploadName: "2026/09/abc123.png"}
			h.SetMediaUploader(media)
			rec, err := uploadRequest(t, h, ownerContext(tc.role, "", nil), tc.kind, tc.filename, tc.data)
			if tc.wantCode != 0 {
				ae, ok := err.(*apperror.AppError)
				if !ok || ae.Code != tc.wantCode {
					t.Fatalf("err = %v, want code %d", err, tc.wantCode)
				}
				if media.uploads != 0 {
					t.Errorf("uploader called %d times for a refused request", media.uploads)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d", rec.Code)
			}
			var out map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatalf("bad JSON %q: %v", rec.Body.String(), err)
			}
			if out["name"] != "2026/09/abc123.png" {
				t.Errorf("name = %q", out["name"])
			}
			if !strings.Contains(out["url"], "abc123") {
				t.Errorf("url = %q", out["url"])
			}
			if media.uploads != 1 || media.gotMime != "image/png" {
				t.Errorf("uploads=%d mime=%q", media.uploads, media.gotMime)
			}
		})
	}

	t.Run("no uploader configured", func(t *testing.T) {
		h := NewHandler(&appearanceSvcStub{})
		_, err := uploadRequest(t, h, ownerContext(RoleOwner, "", nil), "logo", "a.png", pngBytes(10))
		ae, ok := err.(*apperror.AppError)
		if !ok || ae.Code != http.StatusInternalServerError {
			t.Fatalf("err = %v, want 500", err)
		}
	})
}

func deletePictureRequest(h *Handler, cc *CampaignContext, name string) (*httptest.ResponseRecorder, error) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/campaigns/camp-1/appearance/picture?name="+name, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(contextKeyCampaign, cc)
	return rec, h.DeleteAppearancePictureAPI(c)
}

func TestDeleteAppearancePictureAPI(t *testing.T) {
	const name = "2026/10/0f8fad5b-d9cb-469f-a165-70867728950e.png"
	backdrop := name
	other := "2026/10/other.png"
	cases := []struct {
		name         string
		role         Role
		query        string
		settings     string
		backdrop     *string
		mediaResult  bool // what the media layer says: owned by this campaign, uploaded as an appearance picture
		noMedia      bool
		wantCode     int // 0 = success
		wantDeleted  bool
		wantMediaHit int
	}{
		{"owned and unreferenced is deleted", RoleOwner, name, "", nil, true, false, 0, true, 1},
		{"unreferenced but not ours or not an appearance picture is kept", RoleOwner, name, "", nil, false, false, 0, false, 1},
		{"saved as brand logo is kept", RoleOwner, name, `{"brand_logo":"` + name + `"}`, nil, true, false, 0, false, 0},
		{"saved as header image is kept", RoleOwner, name,
			`{"topbar_style":{"mode":"image","image_path":"` + name + `"}}`, nil, true, false, 0, false, 0},
		{"saved as backdrop column is kept", RoleOwner, name, "", &backdrop, true, false, 0, false, 0},
		{"another saved picture does not protect this one", RoleOwner, name, `{"brand_logo":"` + other + `"}`, &other, true, false, 0, true, 1},
		{"no media service is a no-op", RoleOwner, name, "", nil, true, true, 0, false, 0},
		{"missing name rejected", RoleOwner, "", "", nil, true, false, http.StatusBadRequest, false, 0},
		{"traversal rejected", RoleOwner, "../secret.png", "", nil, true, false, http.StatusBadRequest, false, 0},
		{"dot segment rejected", RoleOwner, "2026/10/.hidden", "", nil, true, false, http.StatusBadRequest, false, 0},
		{"empty segment rejected", RoleOwner, "2026//x.png", "", nil, true, false, http.StatusBadRequest, false, 0},
		{"backslash rejected", RoleOwner, `a%5Cb.png`, "", nil, true, false, http.StatusBadRequest, false, 0},
		{"scribe forbidden", RoleScribe, name, "", nil, true, false, http.StatusForbidden, false, 0},
		{"player forbidden", RolePlayer, name, "", nil, true, false, http.StatusForbidden, false, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHandler(&appearanceSvcStub{})
			media := &appearanceMediaStub{deleteResult: tc.mediaResult}
			if !tc.noMedia {
				h.SetMediaUploader(media)
			}
			rec, err := deletePictureRequest(h, ownerContext(tc.role, tc.settings, tc.backdrop), tc.query)
			if tc.wantCode != 0 {
				ae, ok := err.(*apperror.AppError)
				if !ok || ae.Code != tc.wantCode {
					t.Fatalf("err = %v, want code %d", err, tc.wantCode)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				var resp struct {
					Status  string `json:"status"`
					Deleted bool   `json:"deleted"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
					t.Fatalf("response not JSON: %v", err)
				}
				if resp.Status != "ok" || resp.Deleted != tc.wantDeleted {
					t.Errorf("response = %+v, want ok/deleted=%v", resp, tc.wantDeleted)
				}
			}
			if len(media.deleteCalls) != tc.wantMediaHit {
				t.Errorf("DeletePicture calls = %v, want %d", media.deleteCalls, tc.wantMediaHit)
			}
			if tc.wantMediaHit == 1 && media.deleteCalls[0] != "camp-1|"+tc.query {
				t.Errorf("DeletePicture got %q, want this campaign and name", media.deleteCalls[0])
			}
		})
	}
}
