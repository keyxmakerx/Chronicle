package connector

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// EndpointPath is where an AI app connects. It sits under /api/ so the
// cookie-based CSRF check skips it: the endpoint never reads a session
// cookie, only the connection key, so a browser can't be tricked into using it.
const EndpointPath = "/api/ai/campaigns/:id/mcp"

// EndpointURL is the address the owner gives their AI app.
func EndpointURL(baseURL, campaignID string) string {
	return strings.TrimRight(baseURL, "/") + "/api/ai/campaigns/" + url.PathEscape(campaignID) + "/mcp"
}

// maxBody caps one request; a tool call is a name and a few short arguments.
const maxBody = 64 * 1024

// Endpoint is the HTTP side of the connector.
type Endpoint struct {
	svc    *Service
	server *Server
	// origin is this site's own origin. A browser request from any other
	// origin is refused, as the protocol asks of every server.
	origin string
	// failures counts refused keys per address.
	failures *failures
}

// NewEndpoint creates the endpoint. baseURL is the site's public address.
func NewEndpoint(svc *Service, baseURL string) *Endpoint {
	origin := ""
	if u, err := url.Parse(baseURL); err == nil && u.Scheme != "" && u.Host != "" {
		origin = u.Scheme + "://" + u.Host
	}
	return &Endpoint{svc: svc, server: &Server{}, origin: origin, failures: newFailures()}
}

// SetLookups wires the lookups every tool answers with.
func (e *Endpoint) SetLookups(l *records.Lookups) { e.server.Lookups = l }

// RegisterRoutes mounts the endpoint. There is no per-address limit in front
// of it: hosted AI apps call from shared addresses, so one stranger's junk
// keys would lock every owner out. Failed keys are limited per address
// instead (Serve), and good keys per connection.
func RegisterRoutes(ech *echo.Echo, ep *Endpoint) {
	ech.POST(EndpointPath, ep.Serve)
	ech.GET(EndpointPath, ep.NoStream)
	ech.DELETE(EndpointPath, ep.NoStream)
}

// actorFor is who a connection's answers are read as: its maker, as a player
// in Safe mode (the copy-and-paste lookups' exportActor rule).
func actorFor(c *Connection) records.Actor {
	role := int(campaigns.RoleOwner)
	if c.Privacy == aiexport.PrivacyModeSafe {
		role = int(campaigns.RolePlayer)
	}
	return records.Actor{UserID: c.UserID, Role: role}
}

func refuse(c echo.Context, status int, msg string) error {
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(status, map[string]string{"error": msg})
}

// NoStream answers GET and DELETE: this server keeps no event streams or
// sessions, which the protocol allows with 405.
func (e *Endpoint) NoStream(c echo.Context) error {
	c.Response().Header().Set("Allow", http.MethodPost)
	return refuse(c, http.StatusMethodNotAllowed, "send requests with POST")
}

// keyFrom reads the key from Authorization: Bearer, or X-API-Key, the two
// headers AI apps offer for a fixed key.
func keyFrom(r *http.Request) string {
	if h := strings.TrimSpace(r.Header.Get("Authorization")); h != "" {
		if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
			return strings.TrimSpace(h[7:])
		}
		return ""
	}
	return strings.TrimSpace(r.Header.Get("X-API-Key"))
}

// Serve answers one MCP message.
func (e *Endpoint) Serve(c echo.Context) error {
	req := c.Request()
	if o := req.Header.Get("Origin"); o != "" && o != e.origin {
		return refuse(c, http.StatusForbidden, "requests from web pages are not accepted")
	}
	campaignID := c.Param("id")
	conn, name, err := e.svc.Authenticate(req.Context(), campaignID, keyFrom(req))
	if err != nil {
		ip := c.RealIP()
		n := e.failures.add(ip, time.Now())
		if n > failuresPerMinute {
			c.Response().Header().Set("Retry-After", "60")
			return refuse(c, http.StatusTooManyRequests, "Too many wrong keys from this address. Wait a minute.")
		}
		slog.Warn("ai connector: refused", slog.String("campaign_id", oneLine(campaignID, 40)),
			slog.String("ip", ip), slog.String("reason", err.Error()))
		switch {
		case errors.Is(err, ErrBadKey):
			c.Response().Header().Set("WWW-Authenticate", `Bearer realm="chronicle"`)
			return refuse(c, http.StatusUnauthorized, "This key doesn't open this campaign. Check the address and the key on the campaign's Your AI page.")
		case errors.Is(err, ErrKeyEnded):
			c.Response().Header().Set("WWW-Authenticate", `Bearer realm="chronicle"`)
			return refuse(c, http.StatusUnauthorized, "This key was disconnected or has run out. Make a new one on the campaign's Your AI page.")
		case errors.Is(err, ErrOwnerGone):
			return refuse(c, http.StatusForbidden, "Whoever made this key no longer owns the campaign. The owner can make a new one on the Your AI page.")
		}
		return refuse(c, http.StatusServiceUnavailable, "Chronicle couldn't check the key just now. Try again in a moment.")
	}
	if ok, wait := e.svc.Allow(conn.ID); !ok {
		secs := int(wait.Round(time.Second) / time.Second)
		if secs < 1 {
			secs = 1
		}
		c.Response().Header().Set("Retry-After", strconv.Itoa(secs))
		return refuse(c, http.StatusTooManyRequests, "Too many lookups at once. Wait a little and ask again.")
	}
	if ct := req.Header.Get("Content-Type"); !strings.HasPrefix(strings.ToLower(strings.TrimSpace(ct)), "application/json") {
		return refuse(c, http.StatusUnsupportedMediaType, "send JSON")
	}
	body, err := io.ReadAll(http.MaxBytesReader(c.Response(), req.Body, maxBody))
	if err != nil {
		return refuse(c, http.StatusRequestEntityTooLarge, "the request is too large")
	}

	out := e.server.Handle(req.Context(), Session{Conn: conn, CampaignName: name, Actor: actorFor(conn)}, body)
	switch {
	case out.Initialized:
		e.svc.Seen(req.Context(), conn, out.ClientName)
		if conn.LastUsedAt == nil {
			e.svc.Record(req.Context(), conn, "connect", "Connected", cleanClientName(out.ClientName), true)
		}
	case out.Called != nil:
		e.svc.Seen(req.Context(), conn, "")
		e.svc.Record(req.Context(), conn, out.Called.Tool, out.Called.Summary, out.Called.Note, out.Called.OK)
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	if out.Body == nil {
		return c.NoContent(out.Status)
	}
	return c.Blob(out.Status, "application/json", out.Body)
}

// failuresPerMinute is how many refused keys one address may send a minute
// before it is answered 429 without a log line each. A good key always gets
// through.
const failuresPerMinute = 20

// failures counts refused keys per address in one-minute windows, in memory.
type failures struct {
	mu   sync.Mutex
	byIP map[string]*failWindow
}

type failWindow struct {
	start time.Time
	n     int
}

func newFailures() *failures { return &failures{byIP: map[string]*failWindow{}} }

// add counts one refusal from ip and returns the count in its window.
func (f *failures) add(ip string, now time.Time) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.byIP) > 10000 {
		for k, w := range f.byIP {
			if now.Sub(w.start) >= time.Minute {
				delete(f.byIP, k)
			}
		}
		// Still full: many addresses at once. Forgetting them only loosens
		// the log throttle, never what a key opens.
		if len(f.byIP) > 10000 {
			f.byIP = map[string]*failWindow{}
		}
	}
	w := f.byIP[ip]
	if w == nil || now.Sub(w.start) >= time.Minute {
		w = &failWindow{start: now}
		f.byIP[ip] = w
	}
	w.n++
	return w.n
}
