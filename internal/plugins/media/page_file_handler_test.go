package media

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// pageFileCtx builds a request context for the handler as userID, a member of
// campA with the given role.
func pageFileCtx(method, target string, body *bytes.Buffer, contentType, userID string, role campaigns.Role, params map[string]string) (echo.Context, *httptest.ResponseRecorder) {
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, target, body)
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	if contentType != "" {
		req.Header.Set(echo.HeaderContentType, contentType)
	}
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	auth.SetSession(c, &auth.Session{UserID: userID})
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: campA}, MemberRole: role, IsMember: true,
	})
	names, values := []string{}, []string{}
	for k, v := range params {
		names, values = append(names, k), append(values, v)
	}
	c.SetParamNames(names...)
	c.SetParamValues(values...)
	return c, rec
}

func multipartFile(t *testing.T, name string, data []byte, fields map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(data)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	_ = mw.Close()
	return &body, mw.FormDataContentType()
}

// openingService returns one fixed file from Open, standing in for the
// service so the handler's response is the only thing under test.
type openingService struct {
	PageFileService
	file   PageFile
	stored *MediaFile
}

func (o openingService) Open(context.Context, string, string, string, PageFileViewer) (*PageFile, *MediaFile, error) {
	return &o.file, o.stored, nil
}

type pathMedia struct {
	*fakeUploadService
	path string
}

func (p pathMedia) FilePath(*MediaFile) string { return p.path }

// A page file is always a download. Even a row that somehow holds a hostile
// name or type (a legacy row, a tampered database) is sent as an attachment
// with sniffing off, and its type is replaced by octet-stream rather than ever
// echoing text/html or SVG.
func TestPageFileHandler_DownloadIsAlwaysAnAttachment(t *testing.T) {
	tests := []struct {
		name      string
		fileName  string
		mime      string
		body      string
		wantCType string
	}{
		{"html upload", "page.html", "text/html", "<html><script>alert(1)</script></html>", "application/octet-stream"},
		{"svg upload", "logo.svg", "image/svg+xml", `<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`, "application/octet-stream"},
		{"javascript", "x.js", "application/javascript", "alert(1)", "application/octet-stream"},
		{"xhtml", "x.xhtml", "application/xhtml+xml", "<html/>", "application/octet-stream"},
		{"pdf", "map.pdf", mimePDF, "%PDF-1.7", mimePDF},
		{"picture", "map.png", "image/png", "\x89PNG", "image/png"},
		{"text", "notes.txt", mimeText, "hello", mimeText},
		{"name that tries to break the header", "a\"; filename=\"b.html\r\nX: y.txt", mimeText, "hello", mimeText},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "stored")
			if err := os.WriteFile(path, []byte(tt.body), 0o600); err != nil {
				t.Fatal(err)
			}
			h := NewPageFileHandler(openingService{
				file:   PageFile{ID: "f1", Name: CleanPageFileName(tt.fileName), MimeType: tt.mime},
				stored: &MediaFile{ID: "f1", UsageType: UsagePageFile},
			}, pathMedia{fakeUploadService: &fakeUploadService{}, path: path})
			c, rec := pageFileCtx(http.MethodGet, "/x", nil, "", "u1", campaigns.RolePlayer,
				map[string]string{"eid": pgOpen, "fid": "f1"})

			if err := h.Download(c); err != nil {
				t.Fatalf("download: %v", err)
			}
			hdr := rec.Header()
			if got := hdr.Get("Content-Disposition"); !strings.HasPrefix(got, "attachment") {
				t.Errorf("Content-Disposition = %q, want an attachment", got)
			}
			if strings.ContainsAny(hdr.Get("Content-Disposition"), "\r\n") {
				t.Errorf("a file name reached the header unescaped: %q", hdr.Get("Content-Disposition"))
			}
			if got := hdr.Get("X-Content-Type-Options"); got != "nosniff" {
				t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
			}
			if got := hdr.Get("Content-Type"); got != tt.wantCType {
				t.Errorf("Content-Type = %q, want %q", got, tt.wantCType)
			}
			if got := hdr.Get("Cache-Control"); !strings.Contains(got, "no-store") {
				t.Errorf("Cache-Control = %q, want no-store", got)
			}
			if !strings.Contains(hdr.Get("Content-Security-Policy"), "sandbox") {
				t.Errorf("CSP = %q, want a sandbox", hdr.Get("Content-Security-Policy"))
			}
			if rec.Body.String() != tt.body {
				t.Errorf("body = %q, want the stored bytes", rec.Body.String())
			}
		})
	}
}

// Whatever a person uploads, the stored type is never HTML or SVG, so a real
// round trip through upload and download cannot produce an inline page.
func TestPageFileHandler_UploadRefusesHTMLAndSVG(t *testing.T) {
	for _, tt := range []struct {
		name string
		file string
		data string
	}{
		{"html", "page.html", "<html><script>alert(1)</script></html>"},
		{"htm", "page.htm", "<html/>"},
		{"svg", "logo.svg", `<svg onload="alert(1)"/>`},
		{"svgz", "logo.svgz", "x"},
		{"xhtml", "page.xhtml", "<html/>"},
		{"js", "x.js", "alert(1)"},
		{"html bytes named .png", "x.png", "<html><script>alert(1)</script></html>"},
		{"svg bytes named .pdf", "x.pdf", `<svg onload="alert(1)"/>`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newPageFileFixture(t)
			h := NewPageFileHandler(f.svc, f.media)
			body, ctype := multipartFile(t, tt.file, []byte(tt.data), nil)
			c, _ := pageFileCtx(http.MethodPost, "/x", body, ctype, "owner", campaigns.RoleOwner,
				map[string]string{"eid": pgOpen})
			// The page rule reads the viewer's role; the fixture knows owner.
			err := h.Upload(c)
			if apperror.SafeCode(err) != http.StatusBadRequest {
				t.Fatalf("upload: status %d (%v), want 400", apperror.SafeCode(err), err)
			}
			if len(f.stored) != 0 {
				t.Errorf("a refused upload stored %d files", len(f.stored))
			}
		})
	}
}

// An uploaded text file that contains HTML is stored as text and comes back as
// an attachment of type text/plain, never as a page.
func TestPageFileHandler_HTMLInsideATextFileIsStillADownload(t *testing.T) {
	f := newPageFileFixture(t)
	h := NewPageFileHandler(f.svc, f.media)
	payload := "<html><script>alert(1)</script></html>"

	body, ctype := multipartFile(t, "notes.txt", []byte(payload), nil)
	c, rec := pageFileCtx(http.MethodPost, "/x", body, ctype, "owner", campaigns.RoleOwner, map[string]string{"eid": pgOpen})
	if err := h.Upload(c); err != nil {
		t.Fatalf("upload: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d", rec.Code)
	}
	var id string
	for k := range f.stored {
		id = k
	}

	c, rec = pageFileCtx(http.MethodGet, "/x", nil, "", "viewer", campaigns.RolePlayer, map[string]string{"eid": pgOpen, "fid": id})
	if err := h.Download(c); err != nil {
		t.Fatalf("download: %v", err)
	}
	if got := rec.Header().Get("Content-Type"); got != mimeText {
		t.Errorf("Content-Type = %q, want %s", got, mimeText)
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(got, "attachment") {
		t.Errorf("Content-Disposition = %q", got)
	}
}

// The general media routes never take or serve a page file.
func TestMediaRoutes_RefusePageFiles(t *testing.T) {
	camp := campA
	stored := &MediaFile{ID: "pf1", CampaignID: &camp, UploadedBy: "owner", UsageType: UsagePageFile, MimeType: mimeText, Filename: "x", CampaignIsPublic: boolPtr(true), OriginalName: "x.txt"}
	h := &Handler{
		service:       pageFileGetter{fakeUploadService: &fakeUploadService{}, file: stored},
		memberChecker: &stubMemberChecker{members: map[string]map[string]bool{campA: {"owner": true}}},
	}

	t.Run("serve", func(t *testing.T) {
		c, _ := pageFileCtx(http.MethodGet, "/media/pf1", nil, "", "owner", campaigns.RoleOwner, map[string]string{"id": "pf1"})
		if apperror.SafeCode(h.Serve(c)) != http.StatusNotFound {
			t.Error("Serve opened a page file")
		}
	})
	t.Run("thumbnail", func(t *testing.T) {
		c, _ := pageFileCtx(http.MethodGet, "/media/pf1/thumb/300", nil, "", "owner", campaigns.RoleOwner, map[string]string{"id": "pf1", "size": "300"})
		if apperror.SafeCode(h.ServeThumbnail(c)) != http.StatusNotFound {
			t.Error("ServeThumbnail opened a page file")
		}
	})
	t.Run("info", func(t *testing.T) {
		c, _ := pageFileCtx(http.MethodGet, "/media/pf1/info", nil, "", "owner", campaigns.RoleOwner, map[string]string{"fileID": "pf1"})
		if apperror.SafeCode(h.Info(c)) != http.StatusNotFound {
			t.Error("Info described a page file")
		}
	})
	t.Run("delete by its uploader", func(t *testing.T) {
		c, _ := pageFileCtx(http.MethodDelete, "/media/pf1", nil, "", "owner", campaigns.RoleOwner, map[string]string{"fileID": "pf1"})
		if apperror.SafeCode(h.Delete(c)) != http.StatusNotFound {
			t.Error("Delete removed a page file outside its page's rule")
		}
	})
	t.Run("access check backstop", func(t *testing.T) {
		c, _ := pageFileCtx(http.MethodGet, "/media/pf1", nil, "", "owner", campaigns.RoleOwner, nil)
		if apperror.SafeCode(h.checkMediaAccess(c, stored, false, "")) != http.StatusNotFound {
			t.Error("checkMediaAccess allowed a page file")
		}
	})
	t.Run("upload route will not mint one", func(t *testing.T) {
		svc := &fakeUploadService{}
		up := &Handler{service: svc, memberChecker: &stubMemberChecker{roles: map[string]map[string]int{campA: {"owner": 3}}, members: map[string]map[string]bool{campA: {"owner": true}}}}
		body, ctype := multipartFile(t, "a.png", tinyPNG(t), map[string]string{"campaign_id": campA, "usage_type": UsagePageFile})
		c, _ := pageFileCtx(http.MethodPost, "/media/upload", body, ctype, "owner", campaigns.RoleOwner, nil)
		if apperror.SafeCode(up.Upload(c)) != http.StatusBadRequest || svc.uploadCalled {
			t.Error("/media/upload accepted usage_type=page_file")
		}
	})
}

type pageFileGetter struct {
	*fakeUploadService
	file *MediaFile
}

func (p pageFileGetter) GetByID(context.Context, string) (*MediaFile, error) { return p.file, nil }

func TestPageFileHandler_SectionHidesWhenThereIsNothingToShow(t *testing.T) {
	tests := []struct {
		name     string
		listing  *PageFileListing
		err      error
		wantBody bool
		wantCode int
	}{
		{"no files, cannot attach", &PageFileListing{}, nil, false, http.StatusNoContent},
		{"no files, can attach", &PageFileListing{CanAttach: true}, nil, true, http.StatusOK},
		{"files, read only", &PageFileListing{Files: []PageFile{{ID: "f1", Name: "a.pdf", MimeType: mimePDF, Size: 10}}}, nil, true, http.StatusOK},
		{"a failure never reaches the page as an error", nil, apperror.NewInternal(os.ErrClosed), false, http.StatusNoContent},
		{"a missing page is quiet", nil, apperror.NewNotFound("page not found"), false, http.StatusNoContent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewPageFileHandler(listingService{listing: tt.listing, err: tt.err}, nil)
			c, rec := pageFileCtx(http.MethodGet, "/x", nil, "", "u1", campaigns.RolePlayer, map[string]string{"eid": pgOpen})
			if err := h.Section(c); err != nil {
				t.Fatalf("Section returned %v; a lazy fragment must not raise", err)
			}
			if rec.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if (rec.Body.Len() > 0) != tt.wantBody {
				t.Errorf("body present = %v, want %v", rec.Body.Len() > 0, tt.wantBody)
			}
		})
	}
}

type listingService struct {
	PageFileService
	listing *PageFileListing
	err     error
}

func (l listingService) List(context.Context, string, string, PageFileViewer) (*PageFileListing, error) {
	return l.listing, l.err
}

// What the section draws follows the listing: a GM-only file shows a lock and
// the words "GM only"; the controls appear only for people who can use them.
func TestPageFilesSection_Render(t *testing.T) {
	files := []PageFile{
		{ID: "11111111-1111-4111-8111-111111111111", Name: "handout.pdf", MimeType: mimePDF, Size: 2048},
		{ID: "22222222-2222-4222-8222-222222222222", Name: "plot.txt", MimeType: mimeText, Size: 12, GMOnly: true},
	}
	tests := []struct {
		name    string
		listing *PageFileListing
		want    []string
		not     []string
	}{
		{"viewer sees names, sizes and Download, no controls",
			&PageFileListing{Files: files[:1]},
			[]string{"handout.pdf", "2.0 KB", "Download", "/files/11111111-1111-4111-8111-111111111111/download"},
			[]string{"Attach a file", "data-pf-remove", "GM only"}},
		{"editor sees Attach and Remove but no GM controls",
			&PageFileListing{Files: files[:1], CanAttach: true},
			[]string{"Attach a file", "data-pf-remove", "Download"},
			[]string{`type="checkbox"`, "Make GM only"}},
		{"GM sees the lock, the label and the toggle",
			&PageFileListing{Files: files, CanAttach: true, CanMarkGMOnly: true},
			[]string{"GM only", "fa-lock", `type="checkbox"`, "Show to everyone who can see this page", "Make GM only"},
			nil},
		{"an editor with no files is told there are none",
			&PageFileListing{CanAttach: true},
			[]string{"No files yet", "Attach a file"},
			[]string{"Download"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := PageFilesSection(campA, pgOpen, tt.listing, false).Render(context.Background(), &buf); err != nil {
				t.Fatal(err)
			}
			html := buf.String()
			for _, w := range tt.want {
				if !strings.Contains(html, w) {
					t.Errorf("section is missing %q:\n%s", w, html)
				}
			}
			for _, n := range tt.not {
				if strings.Contains(html, n) {
					t.Errorf("section should not contain %q", n)
				}
			}
			// Swap safety: every control is an inline handler, never a script
			// tag a swap could drop.
			if strings.Contains(html, "<script") {
				t.Errorf("section emits a script tag, which an HTMX swap may not run")
			}
		})
	}
}
