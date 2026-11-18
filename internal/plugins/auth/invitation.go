package auth

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v5"
)

// InvitationMember is one person at the table: a name and a picture, nothing
// else, so the door never carries an email address or a role.
type InvitationMember struct {
	Name       string
	AvatarPath string
}

// Invitation is what a signed-out person sees about the campaign a link,
// emailed invite or guest code opens. It is built only for one that is valid
// right now.
type Invitation struct {
	CampaignName string
	// BackdropPath is the campaign's backdrop media id, "" when it has none.
	BackdropPath string
	Members      []InvitationMember
	// NextNight is the next planned game night, nil when none is planned.
	NextNight *InvitationNight
}

// InvitationNight is the next game night. Time is "" for an all-day night.
type InvitationNight struct {
	// Date is YYYY-MM-DD and Time "HH:MM", both on the wall clock of TZ.
	Date string
	Time string
	TZ   string
	// At is the start instant, so the page can show it in the reader's own zone.
	At time.Time
}

// InvitationSource is the campaign side of the door, implemented by an
// adapter in app so auth imports no other plugin. Each lookup returns nil
// for anything that is not valid right now: wrong, used, expired, revoked or
// archived all look the same.
type InvitationSource interface {
	ByLink(ctx context.Context, code string) *Invitation
	ByToken(ctx context.Context, token string) *Invitation
	ByGuestCode(ctx context.Context, code string) *Invitation
}

// ConfigureInvitations wires the door's lookups. Without it the pages stay plain.
func ConfigureInvitations(svc AuthService, src InvitationSource) {
	if s, ok := svc.(*authService); ok {
		s.invitations = src
	}
}

// invitationLimit and invitationWindow match POST /join: ten lookups a
// minute for one address.
const (
	invitationLimit  = 10
	invitationWindow = time.Minute
)

// invitationLimiter counts door lookups per address. A lookup is an
// unauthenticated read that confirms a code is real, so it is throttled like
// the join form itself.
type invitationLimiter struct {
	mu      sync.Mutex
	entries map[string]*invitationEntry
	now     func() time.Time
}

type invitationEntry struct {
	count int
	start time.Time
}

func newInvitationLimiter() *invitationLimiter {
	return &invitationLimiter{entries: map[string]*invitationEntry{}, now: time.Now}
}

// allow records one lookup from ip and reports whether it is within budget.
func (l *invitationLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	// Entries older than a window are dead weight; sweep as the map grows.
	if len(l.entries) > 4096 {
		for k, e := range l.entries {
			if now.Sub(e.start) > invitationWindow {
				delete(l.entries, k)
			}
		}
	}
	e, ok := l.entries[ip]
	if !ok || now.Sub(e.start) > invitationWindow {
		l.entries[ip] = &invitationEntry{count: 1, start: now}
		return true
	}
	e.count++
	return e.count <= invitationLimit
}

var doorLimiter = newInvitationLimiter()

// AllowInvitationLookup spends one lookup from the caller's address. When it
// reports false the page is drawn plain, the same as for a wrong code.
func AllowInvitationLookup(c *echo.Context) bool {
	return doorLimiter.allow(c.RealIP())
}

// invitationForRedirect reads the invitation a post-login destination points
// at: a share link (/join/<code>) or an emailed invite (/invites/accept?token=).
// Anything else, or a throttled address, gets nil.
func (h *Handler) invitationForRedirect(c *echo.Context, redirect string) *Invitation {
	src := h.invitationSource()
	if src == nil || redirect == "" {
		return nil
	}
	if code, ok := strings.CutPrefix(redirect, "/join/"); ok && code != "" && !strings.ContainsAny(code, "/?#") {
		if !AllowInvitationLookup(c) {
			return nil
		}
		return src.ByLink(c.Request().Context(), code)
	}
	if token := extractInviteToken(redirect); token != "" {
		if !AllowInvitationLookup(c) {
			return nil
		}
		return src.ByToken(c.Request().Context(), token)
	}
	return nil
}

// invitationForCode reads the invitation a guest code opens.
func (h *Handler) invitationForCode(c *echo.Context, code string) *Invitation {
	src := h.invitationSource()
	if src == nil || normalizeGuestCode(code) == "" {
		return nil
	}
	if !AllowInvitationLookup(c) {
		return nil
	}
	return src.ByGuestCode(c.Request().Context(), code)
}

func (h *Handler) invitationSource() InvitationSource {
	if s, ok := h.service.(*authService); ok {
		return s.invitations
	}
	return nil
}

