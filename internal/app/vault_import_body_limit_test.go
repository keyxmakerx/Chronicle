package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/config"
	"github.com/keyxmakerx/chronicle/internal/plugins/vault_import"
)

// zeroReader yields n zero bytes without holding them in memory.
type zeroReader struct{ left int64 }

func (z *zeroReader) Read(p []byte) (int, error) {
	if z.left <= 0 {
		return 0, io.EOF
	}
	n := int64(len(p))
	if n > z.left {
		n = z.left
	}
	for i := int64(0); i < n; i++ {
		p[i] = 0
	}
	z.left -= n
	return int(n), nil
}

// TestVaultImportUploadBodyLimit pins that the import upload is capped by
// middleware ahead of the CSRF check, which parses the multipart body before
// anyone is signed in. Without it an anonymous client could make the server
// spool an unbounded body to disk.
func TestVaultImportUploadBodyLimit(t *testing.T) {
	a := &App{Config: &config.Config{}, Echo: echo.New()}
	a.setupMiddleware()
	const path = "/campaigns/c1/import/markdown/preview"
	a.Echo.POST(path, func(c echo.Context) error { return c.NoContent(http.StatusOK) })

	limit := vault_import.RequestLimit(vault_import.DefaultLimits)
	const mb = 1 << 20

	t.Run("a declared size over the cap is refused before anything is read", func(t *testing.T) {
		cr := &countingReader{r: &zeroReader{left: limit + 50*mb}}
		req := httptest.NewRequest("POST", path, cr)
		req.ContentLength = limit + 50*mb
		req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
		rec := httptest.NewRecorder()
		a.Echo.ServeHTTP(rec, req)
		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("status = %d, want 413", rec.Code)
		}
		if cr.n > mb {
			t.Errorf("server read %d bytes of a body it should have refused up front", cr.n)
		}
	})

	t.Run("an unsized body is cut off at the cap", func(t *testing.T) {
		// A well-formed multipart body, so the CSRF check's form parse really
		// does stream it (and spool it) until something stops it.
		head := "--x\r\nContent-Disposition: form-data; name=\"file\"; filename=\"a.zip\"\r\nContent-Type: application/zip\r\n\r\n"
		cr := &countingReader{r: io.MultiReader(strings.NewReader(head), &zeroReader{left: limit + 50*mb})}
		req := httptest.NewRequest("POST", path, cr)
		req.ContentLength = -1
		req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
		a.Echo.ServeHTTP(httptest.NewRecorder(), req)
		if cr.n > limit+64*1024 {
			t.Errorf("server read %d bytes, want about %d at most", cr.n, limit)
		}
		if cr.n < limit/2 {
			t.Errorf("server read only %d bytes; the test is not exercising the cap", cr.n)
		}
	})

	t.Run("a zip-sized body under the cap is not refused for size", func(t *testing.T) {
		req := httptest.NewRequest("POST", path, &zeroReader{left: 20 * mb})
		req.ContentLength = 20 * mb
		req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
		rec := httptest.NewRecorder()
		a.Echo.ServeHTTP(rec, req)
		if rec.Code == http.StatusRequestEntityTooLarge {
			t.Errorf("a 20 MB body was refused as too large")
		}
	})
}
