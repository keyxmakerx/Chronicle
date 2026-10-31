package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// MySession is one browser signed in as the user, as their own account page
// lists it. ID is the token's hash, so the page never holds a usable token.
type MySession struct {
	ID        string
	Device    string
	Mobile    bool
	CreatedAt time.Time
	Current   bool
}

// ConnectedApp is an app outside Chronicle the user let act for them, such
// as a Foundry world using their notes. Owned by whichever plugin issued it;
// the account page only lists and disconnects.
type ConnectedApp struct {
	ID       string
	Name     string
	Place    string // Where it is used: a campaign name.
	Origin   string
	LastUsed time.Time
}

// ConnectedApps lists and disconnects the apps a user allowed. Wired in
// app/routes.go so auth never imports the plugin that issues them.
type ConnectedApps interface {
	ListForUser(ctx context.Context, userID string) ([]ConnectedApp, error)
	// DisconnectForUser ends one of the user's own apps; anyone else's is
	// NotFound.
	DisconnectForUser(ctx context.Context, userID, appID string) error
}

// ListMySessions returns the user's live sessions, newest first, marking the
// one making the request. A token left in the user's set after its session
// expired is dropped from the set.
func (s *authService) ListMySessions(ctx context.Context, userID, currentToken string) ([]MySession, error) {
	if s.redis == nil {
		return nil, nil
	}
	tokens, err := s.redis.SMembers(ctx, userSessionsKeyPrefix+userID).Result()
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("listing user sessions: %w", err))
	}
	out := make([]MySession, 0, len(tokens))
	for _, token := range tokens {
		data, err := s.redis.Get(ctx, sessionKeyPrefix+token).Bytes()
		if err != nil {
			s.redis.SRem(ctx, userSessionsKeyPrefix+userID, token)
			continue
		}
		var sess Session
		if json.Unmarshal(data, &sess) != nil || sess.UserID != userID {
			continue
		}
		device, mobile := deviceLabel(sess.UserAgent)
		out = append(out, MySession{
			ID:        hashToken(token),
			Device:    device,
			Mobile:    mobile,
			CreatedAt: sess.CreatedAt,
			Current:   token == currentToken,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Current != out[j].Current {
			return out[i].Current
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

// SignOutMySession ends one of the user's own sessions by its ID. It looks
// only in the user's own set, so another person's session is NotFound. The
// session making the request is refused: Sign out does that one.
func (s *authService) SignOutMySession(ctx context.Context, userID, sessionID, currentToken string) error {
	if s.redis == nil {
		return apperror.NewNotFound("session not found")
	}
	tokens, err := s.redis.SMembers(ctx, userSessionsKeyPrefix+userID).Result()
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("listing user sessions: %w", err))
	}
	for _, token := range tokens {
		if hashToken(token) != sessionID {
			continue
		}
		if token == currentToken {
			return apperror.NewBadRequest("use Sign out to end this browser's session")
		}
		return s.DestroySession(ctx, token)
	}
	return apperror.NewNotFound("session not found")
}

// deviceLabel names a browser from its User-Agent in a few words ("Firefox
// on Linux"), and says whether it is a phone or tablet. Unknown parts are
// left out rather than guessed.
func deviceLabel(ua string) (string, bool) {
	browser := ""
	switch {
	case strings.Contains(ua, "Edg/"):
		browser = "Edge"
	case strings.Contains(ua, "OPR/"):
		browser = "Opera"
	case strings.Contains(ua, "Firefox/") || strings.Contains(ua, "FxiOS/"):
		browser = "Firefox"
	case strings.Contains(ua, "Chrome/") || strings.Contains(ua, "CriOS/"):
		browser = "Chrome"
	case strings.Contains(ua, "Safari/"):
		browser = "Safari"
	}
	system := ""
	switch {
	case strings.Contains(ua, "iPad"):
		system = "iPad"
	case strings.Contains(ua, "iPhone"):
		system = "iPhone"
	case strings.Contains(ua, "Android"):
		system = "Android"
	case strings.Contains(ua, "CrOS"):
		system = "ChromeOS"
	case strings.Contains(ua, "Windows"):
		system = "Windows"
	case strings.Contains(ua, "Mac OS X") || strings.Contains(ua, "Macintosh"):
		system = "Mac"
	case strings.Contains(ua, "Linux"):
		system = "Linux"
	}
	mobile := system == "iPad" || system == "iPhone" || system == "Android"
	switch {
	case browser != "" && system != "":
		return browser + " on " + system, mobile
	case browser != "":
		return browser, mobile
	case system != "":
		return "A browser on " + system, mobile
	}
	return "Unknown browser", mobile
}
