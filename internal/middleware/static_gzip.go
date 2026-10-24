package middleware

import (
	"path"
	"strings"

	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
)

// staticGzipExts are the text assets worth compressing. Images, fonts and
// archives are already compressed; gzipping them again only costs CPU.
var staticGzipExts = map[string]bool{
	".js": true, ".mjs": true, ".css": true, ".svg": true,
	".json": true, ".map": true, ".txt": true,
}

// StaticGzip compresses text assets under urlPrefix. A page's scripts and
// styles come to a few megabytes, and a phone on a slow link spends most of
// its first visit downloading them. Pages themselves are never compressed
// here: they carry CSRF tokens, which compression exposes to BREACH-style
// guessing, while static files hold nothing secret. A reverse proxy that
// compresses too sees Content-Encoding already set and passes it through.
func StaticGzip(urlPrefix string) echo.MiddlewareFunc {
	return echomw.GzipWithConfig(echomw.GzipConfig{
		Level:     5,
		MinLength: 1024,
		Skipper: func(c echo.Context) bool {
			return !staticGzipWanted(urlPrefix, c.Request().URL.Path)
		},
	})
}

// staticGzipWanted reports whether a request path is a compressible static asset.
func staticGzipWanted(urlPrefix, p string) bool {
	return strings.HasPrefix(p, urlPrefix) && staticGzipExts[strings.ToLower(path.Ext(p))]
}
