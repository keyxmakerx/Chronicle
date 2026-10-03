package backup

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

const testUser = "user-1"

// newTestHandler returns a Handler whose current user is always testUser.
func newTestHandler(svc Service) *Handler {
	h := NewHandler(svc)
	h.SetDownloadAuth("test-secret", func(echo.Context) string { return testUser })
	return h
}

func TestDownloadSigner_Verify(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	s := &downloadSigner{key: []byte("k"), now: func() time.Time { return base }}
	tok := s.Issue(testUser, "a.gz")
	otherKey := []byte("other")

	tests := []struct {
		name  string
		key   []byte
		token string
		user  string
		file  string
		now   time.Time
		want  bool
	}{
		{"valid", s.key, tok, testUser, "a.gz", base.Add(downloadTokenTTL), true},
		{"expired", s.key, tok, testUser, "a.gz", base.Add(downloadTokenTTL + 2*time.Second), false},
		{"wrong user", s.key, tok, "user-2", "a.gz", base, false},
		{"wrong name", s.key, tok, testUser, "b.gz", base, false},
		{"tampered mac", s.key, tok[:len(tok)-2] + "AA", testUser, "a.gz", base, false},
		{"tampered expiry", s.key, "9999999999" + tok[10:], testUser, "a.gz", base, false},
		{"different key", otherKey, tok, testUser, "a.gz", base, false},
		{"empty token", s.key, "", testUser, "a.gz", base, false},
		{"garbage", s.key, "nodot", testUser, "a.gz", base, false},
		{"empty user", s.key, tok, "", "a.gz", base, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &downloadSigner{key: tt.key, now: func() time.Time { return tt.now }}
			if got := v.Verify(tt.token, tt.user, tt.file); got != tt.want {
				t.Fatalf("Verify = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDownload_RequiresToken(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.gz"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := newTestHandler(&stubService{dir: dir})
	good := h.signer.Issue(testUser, "a.gz")
	wrongName := h.signer.Issue(testUser, "b.gz")

	tests := []struct {
		name  string
		query string
		want  int
	}{
		{"no token", "", http.StatusForbidden},
		{"empty token", "?t=", http.StatusForbidden},
		{"garbage token", "?t=abc", http.StatusForbidden},
		{"token for other file", "?t=" + url.QueryEscape(wrongName), http.StatusForbidden},
		{"valid token", "?t=" + url.QueryEscape(good), http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			rec := httptest.NewRecorder()
			c := e.NewContext(httptest.NewRequest(http.MethodGet, "/x"+tt.query, nil), rec)
			c.SetParamNames("name")
			c.SetParamValues("a.gz")
			err := h.Download(c)
			code := rec.Code
			if he, ok := err.(*echo.HTTPError); ok {
				code = he.Code
			}
			if code != tt.want {
				t.Fatalf("status = %d, want %d", code, tt.want)
			}
			if tt.want == http.StatusForbidden && rec.Body.Len() != 0 {
				t.Fatal("file bytes written on a refused download")
			}
		})
	}
}

func TestDownload_UnwiredHandlerFailsClosed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.gz"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(&stubService{dir: dir})
	tok := h.signer.Issue("", "a.gz")
	e := echo.New()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/x?t="+url.QueryEscape(tok), nil), httptest.NewRecorder())
	c.SetParamNames("name")
	c.SetParamValues("a.gz")
	if he, ok := h.Download(c).(*echo.HTTPError); !ok || he.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %v", he)
	}
}

func TestDownloadLink(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.gz"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := newTestHandler(&stubService{dir: dir})

	t.Run("valid name redirects with token that verifies", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, "/x", nil)
		req.Header.Set("HX-Request", "true")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("name")
		c.SetParamValues("a.gz")
		if err := h.DownloadLink(c); err != nil {
			t.Fatal(err)
		}
		u, err := url.Parse(rec.Header().Get("HX-Redirect"))
		if err != nil || u.Path != "/admin/backup/files/a.gz" {
			t.Fatalf("bad redirect %q (%v)", rec.Header().Get("HX-Redirect"), err)
		}
		if !h.signer.Verify(u.Query().Get("t"), testUser, "a.gz") {
			t.Fatal("issued token does not verify")
		}
	})

	t.Run("traversal rejected", func(t *testing.T) {
		e := echo.New()
		c := e.NewContext(httptest.NewRequest(http.MethodPost, "/x", nil), httptest.NewRecorder())
		c.SetParamNames("name")
		c.SetParamValues("../etc/passwd")
		if he, ok := h.DownloadLink(c).(*echo.HTTPError); !ok || he.Code != http.StatusBadRequest {
			t.Fatalf("want 400, got %v", he)
		}
	})
}

// TestRoutesGuardDownloadLink pins that the link route sits behind reauth.
func TestRoutesGuardDownloadLink(t *testing.T) {
	e := echo.New()
	reauth := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error { return c.NoContent(http.StatusForbidden) }
	}
	// A nil handler is safe: the stub refuses before any handler runs.
	RegisterRoutes(e.Group("/admin"), nil, reauth)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/backup/files/a.gz/link", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403 from reauth", rec.Code)
	}
}
