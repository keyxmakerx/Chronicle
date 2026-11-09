package maintenance

import (
	"html/template"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
)

// RetryAfterSeconds is what a refused client is told to wait.
const RetryAfterSeconds = "60"

// OwnerHandler serves the restoring admin's own pages while the gate is
// closed. It returns handled=false for any path it does not own, which then
// gets the ordinary refusal. It runs without the router, the auth middleware
// or the database, because the database is the thing being replaced.
type OwnerHandler func(c *echo.Context) (handled bool, err error)

// SetOwnerHandler installs the handler for the owning admin's session.
func (s *State) SetOwnerHandler(h OwnerHandler) {
	s.mu.Lock()
	s.owner = h
	s.mu.Unlock()
}

// Gate is the global middleware. While the state is open it only counts
// requests in flight. While closed it refuses everything except:
//   - health probes, which answer "maintenance" themselves;
//   - static files, which touch no data and let the pages below look right;
//   - the owning admin's session on the paths its OwnerHandler claims.
func (s *State) Gate() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if !s.Active() {
				if isWebSocket(c.Request()) {
					return next(c)
				}
				done := s.Track()
				defer done()
				return next(c)
			}

			req := c.Request()
			path := req.URL.Path
			if path == "/healthz" || path == "/health" {
				return next(c)
			}
			if (req.Method == http.MethodGet || req.Method == http.MethodHead) &&
				strings.HasPrefix(path, "/static/") {
				return next(c)
			}
			if s.IsOwner(req) {
				s.mu.Lock()
				h := s.owner
				s.mu.Unlock()
				if h != nil {
					handled, err := h(c)
					if handled {
						return err
					}
				}
			}
			return refuse(c)
		}
	}
}

// refuse answers 503 in the shape the caller can use: JSON with Retry-After
// for API, Foundry and websocket callers, a plain page for browsers.
func refuse(c *echo.Context) error {
	h := c.Response().Header()
	h.Set("Retry-After", RetryAfterSeconds)
	h.Set("Cache-Control", "no-store")
	h.Set("X-Chronicle-Maintenance", "1")
	if wantsJSON(c.Request()) {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{
			"error":       "maintenance",
			"message":     "Chronicle is being restored from a backup and will be back in a few minutes.",
			"retry_after": 60,
		})
	}
	if c.Request().Method == http.MethodHead {
		return c.NoContent(http.StatusServiceUnavailable)
	}
	return c.HTMLBlob(http.StatusServiceUnavailable, publicPage)
}

func isWebSocket(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") || r.URL.Path == "/ws"
}

func wantsJSON(r *http.Request) bool {
	p := r.URL.Path
	if strings.HasPrefix(p, "/api/") || isWebSocket(r) {
		return true
	}
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "text/html") {
		return false
	}
	return strings.Contains(accept, "application/json") ||
		strings.Contains(r.Header.Get("Content-Type"), "application/json")
}

// publicPage is self-contained on purpose: it must render with no database,
// no session and no build assets.
var publicPage = func() []byte {
	var b strings.Builder
	_ = template.Must(template.New("p").Parse(publicPageSrc)).Execute(&b, nil)
	return []byte(b.String())
}()

const publicPageSrc = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta http-equiv="refresh" content="30">
<title>Back in a few minutes</title>
<style>
:root{color-scheme:light dark;--bg:#f9fafb;--fg:#111827;--muted:#6b7280;--edge:#e5e7eb}
@media (prefers-color-scheme:dark){:root{--bg:#111827;--fg:#f3f4f6;--muted:#9ca3af;--edge:#374151}}
body{margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;background:var(--bg);color:var(--fg);font:16px/1.5 system-ui,sans-serif}
main{max-width:28rem;padding:1rem}
h1{font-size:1.5rem;font-weight:600;margin:0 0 .75rem}
p{margin:0 0 .5rem}
.muted{color:var(--muted);font-size:.875rem;padding-top:.75rem;margin-top:1rem;border-top:1px solid var(--edge)}
</style>
</head>
<body>
<main id="main-content">
<h1>Back in a few minutes</h1>
<p>This site is being put back to how it was when a backup was made.</p>
<p class="muted">This page checks again by itself every 30 seconds. Nothing you need to do.</p>
</main>
</body>
</html>`
