package connector

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v5"

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

// maxBody caps one request: a request for changes (changesMax) and room for
// the message around it, its quoting included. Every other call is a name
// and a few short arguments.
const maxBody = changesMax*2 + 64*1024

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
	return &Endpoint{svc: svc, server: &Server{Asks: svc}, origin: origin, failures: newFailures()}
}

// SetLookups wires the lookups every tool answers with.
func (e *Endpoint) SetLookups(l *records.Lookups) { e.server.Lookups = l }

// SetInstructions wires how_to_write_changes' answer.
func (e *Endpoint) SetInstructions(f InstructionsFunc) { e.server.Instructions = f }

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
// in Safe mode (the copy-and-paste lookups' exportActor rule). A player's key
// always reads as that player, whatever its stored privacy says.
func actorFor(c *Connection) records.Actor {
	role := int(campaigns.RoleOwner)
	if c.ForPlayer || c.Privacy == aiexport.PrivacyModeSafe {
		role = int(campaigns.RolePlayer)
	}
	return records.Actor{UserID: c.UserID, Role: role, ReadsOwnNotes: c.ForPlayer}
}

func refuse(c *echo.Context, status int, msg string) error {
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(status, map[string]string{"error": msg})
}

// NoStream answers GET and DELETE: this server keeps no event streams or
// sessions, which the protocol allows with 405.
func (e *Endpoint) NoStream(c *echo.Context) error {
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
func (e *Endpoint) Serve(c *echo.Context) error {
	req := c.Request()
	if o := req.Header.Get("Origin"); o != "" && o != e.origin {
		return refuse(c, http.StatusForbidden, "requests from web pages are not accepted")
	}
	campaignID := c.Param("id")
	conn, name, err := e.svc.Authenticate(req.Context(), campaignID, keyFrom(req))
	switch {
	case errors.Is(err, ErrKeyPaused):
		// The key is right, so this is no failed guess to count.
		return refuse(c, http.StatusForbidden, "This key is paused. It can be resumed on the Your AI page in Chronicle.")
	case errors.Is(err, ErrPlayersOff), errors.Is(err, ErrPlayersBlocked):
		return refuse(c, http.StatusForbidden, "Players can't use AI in this campaign right now: "+err.Error()+".")
	case errors.Is(err, ErrCutOff):
		return refuse(c, http.StatusForbidden, "The campaign's owner has stopped AI in this campaign for now. They can turn it back on on the Your AI page.")
	case errors.Is(err, ErrAIOff):
		c.Response().Header().Set("Retry-After", "3600")
		return refuse(c, http.StatusServiceUnavailable, "AI is turned off on this server for now. Ask the server's admin.")
	}
	if err != nil {
		ip := c.RealIP()
		if e.svc.TurnedAway(ip) {
			// This address flooded wrong keys earlier: answer at once and
			// count nothing, so a flood costs a log line once, not each time.
			c.Response().Header().Set("Retry-After", "3600")
			return refuse(c, http.StatusTooManyRequests, "Too many wrong keys came from this address. Wrong keys from it are turned away for a day.")
		}
		n := e.failures.add(ip, time.Now())
		e.svc.NoteRefused(req.Context(), ip, oneLine(campaignID, 40), n > failuresPerMinute)
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
		case errors.Is(err, ErrPlayerGone):
			return refuse(c, http.StatusForbidden, "Whoever made this key is no longer in the campaign.")
		}
		return refuse(c, http.StatusServiceUnavailable, "Chronicle couldn't check the key just now. Try again in a moment.")
	}
	if ok, wait := e.svc.Allow(conn.ID); !ok {
		e.svc.NoteLimited(req.Context(), conn)
		secs := int(wait.Round(time.Second) / time.Second)
		if secs < 1 {
			secs = 1
		}
		c.Response().Header().Set("Retry-After", strconv.Itoa(secs))
		return refuse(c, http.StatusTooManyRequests, fmt.Sprintf("Too many requests at once. Wait %d seconds and ask again.", secs))
	}
	if ct := req.Header.Get("Content-Type"); !strings.HasPrefix(strings.ToLower(strings.TrimSpace(ct)), "application/json") {
		return refuse(c, http.StatusUnsupportedMediaType, "send JSON")
	}
	body, err := io.ReadAll(http.MaxBytesReader(c.Response(), req.Body, maxBody))
	if err != nil {
		return refuse(c, http.StatusRequestEntityTooLarge, "the request is too large")
	}

	sess := Session{Conn: conn, CampaignName: name, Actor: actorFor(conn),
		AllowCall: func() (bool, time.Duration) { return e.svc.AllowCall(conn.ID) }}
	out := e.server.Handle(req.Context(), sess, body)
	if out.Limited {
		e.svc.NoteLimited(req.Context(), conn)
	}
	switch {
	case out.Initialized:
		e.svc.Seen(req.Context(), conn, out.ClientName)
		e.svc.NoteConnect(req.Context(), conn, out.ClientName, c.RealIP())
		if conn.LastUsedAt == nil {
			e.svc.Record(req.Context(), conn, toolConnect, "Connected", cleanClientName(out.ClientName), true)
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
