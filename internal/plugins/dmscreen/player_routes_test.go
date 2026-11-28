package dmscreen

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// routeAuth signs in whoever the session cookie names: "tok-<user>".
type routeAuth struct{ auth.AuthService }

func (routeAuth) ValidateSession(_ context.Context, token string) (*auth.Session, error) {
	if uid, ok := strings.CutPrefix(token, "tok-"); ok {
		return &auth.Session{UserID: uid}, nil
	}
	return nil, apperror.NewUnauthorized("no session")
}

// routeCampaigns is campaign c1 with an owner, a co-DM, a player and a scribe;
// anyone else is not a member.
type routeCampaigns struct{ campaigns.CampaignService }

func (routeCampaigns) GetByID(_ context.Context, id string) (*campaigns.Campaign, error) {
	if id != "c1" {
		return nil, apperror.NewNotFound("campaign not found")
	}
	return &campaigns.Campaign{ID: "c1", Settings: `{"dm_grant_ids":["codm"]}`}, nil
}

func (routeCampaigns) GetMember(_ context.Context, _, userID string) (*campaigns.CampaignMember, error) {
	roles := map[string]campaigns.Role{"own": campaigns.RoleOwner, "codm": campaigns.RolePlayer, "p1": campaigns.RolePlayer, "p2": campaigns.RolePlayer, "scr": campaigns.RoleScribe}
	if r, ok := roles[userID]; ok {
		return &campaigns.CampaignMember{UserID: userID, Role: r}, nil
	}
	return nil, apperror.NewNotFound("not a member")
}

func playerRoutes(notes *fakePlayerNotes) *echo.Echo {
	e := echo.New()
	e.HTTPErrorHandler = func(c *echo.Context, err error) {
		code := http.StatusInternalServerError
		if ae, ok := err.(*apperror.AppError); ok {
			code = ae.Code
		}
		_ = c.NoContent(code)
	}
	RegisterRoutes(e, NewHandler(NewService(Sources{PlayerNotes: notes})), routeCampaigns{}, routeAuth{})
	return e
}

func callAs(e *echo.Echo, method, path, user string, viewAsPlayer bool, body string) int {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Accept", "application/json")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if user != "" {
		req.AddCookie(&http.Cookie{Name: "chronicle_session", Value: "tok-" + user})
	}
	if viewAsPlayer {
		req.AddCookie(&http.Cookie{Name: "chronicle_view_as_player", Value: "c1"})
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec.Code
}

func TestPlayerScreenRoutes_Gate(t *testing.T) {
	notes := &fakePlayerNotes{byUser: map[string]*ScreenNote{
		"p2": {ID: "n-p2", Entry: `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"p2 only"}]}]}`},
	}}
	e := playerRoutes(notes)
	tests := []struct {
		name   string
		user   string
		asPC   bool
		method string
		path   string
		body   string
		want   int
	}{
		{"player opens the screen", "p1", false, http.MethodGet, "/campaigns/c1/player-screen", "", http.StatusOK},
		{"scribe opens the screen", "scr", false, http.MethodGet, "/campaigns/c1/player-screen", "", http.StatusOK},
		{"owner is refused (has the DM Screen)", "own", false, http.MethodGet, "/campaigns/c1/player-screen", "", http.StatusForbidden},
		{"co-DM is refused", "codm", false, http.MethodGet, "/campaigns/c1/player-screen", "", http.StatusForbidden},
		{"owner viewing as a player gets it", "own", true, http.MethodGet, "/campaigns/c1/player-screen", "", http.StatusOK},
		{"non-member is refused", "stranger", false, http.MethodGet, "/campaigns/c1/player-screen", "", http.StatusForbidden},
		{"unknown campaign is not found", "p1", false, http.MethodGet, "/campaigns/zz/player-screen", "", http.StatusNotFound},
		{"signed out is sent to sign in", "", false, http.MethodGet, "/campaigns/c1/player-screen", "", http.StatusSeeOther},
		{"signed out cannot read notes", "", false, http.MethodGet, "/campaigns/c1/player-screen/notes", "", http.StatusSeeOther},
		{"owner cannot read player notes", "own", false, http.MethodGet, "/campaigns/c1/player-screen/notes", "", http.StatusForbidden},
		{"co-DM cannot read player notes", "codm", false, http.MethodGet, "/campaigns/c1/player-screen/notes", "", http.StatusForbidden},
		{"owner cannot write player notes", "own", false, http.MethodPut, "/campaigns/c1/player-screen/notes", `{"text":"x"}`, http.StatusForbidden},
		{"non-member cannot read notes", "stranger", false, http.MethodGet, "/campaigns/c1/player-screen/notes", "", http.StatusForbidden},
		{"player reads their own note", "p1", false, http.MethodGet, "/campaigns/c1/player-screen/notes", "", http.StatusOK},
		{"player saves their own note", "p1", false, http.MethodPut, "/campaigns/c1/player-screen/notes", `{"text":"mine","version":""}`, http.StatusOK},
		{"a player is refused the DM Screen", "p1", false, http.MethodGet, "/campaigns/c1/dm-screen", "", http.StatusForbidden},
		{"a player is refused the DM note", "p1", false, http.MethodGet, "/campaigns/c1/dm-screen/notes", "", http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := callAs(e, tt.method, tt.path, tt.user, tt.asPC, tt.body); got != tt.want {
				t.Errorf("%s %s as %s = %d, want %d", tt.method, tt.path, tt.user, got, tt.want)
			}
		})
	}
	// Whatever was asked, the note source only ever looked up the caller's
	// own note: never p2's, and never on behalf of the DM team.
	for _, u := range notes.asked {
		if u != "p1" && u != "own" && u != "scr" {
			t.Errorf("the note source was asked for %q", u)
		}
	}
	if strings.Contains(notes.byUser["p1"].Entry, "p2 only") {
		t.Error("p1's note holds p2's text")
	}
}

// The owner viewing as a player reaches only their own note, as a player.
func TestPlayerScreenRoutes_ViewAsPlayerIsAPlayer(t *testing.T) {
	notes := &fakePlayerNotes{byUser: map[string]*ScreenNote{"p1": {ID: "n-p1"}}}
	e := playerRoutes(notes)
	if got := callAs(e, http.MethodGet, "/campaigns/c1/player-screen/notes", "own", true, ""); got != http.StatusOK {
		t.Fatalf("code = %d", got)
	}
	if len(notes.asked) != 1 || notes.asked[0] != "own" {
		t.Errorf("asked for %v, want only the owner's own note", notes.asked)
	}
}

func TestCanOpenPlayers(t *testing.T) {
	tests := []struct {
		name string
		cc   campaigns.CampaignContext
		want bool
	}{
		{"player", campaigns.CampaignContext{MemberRole: campaigns.RolePlayer, IsMember: true}, true},
		{"scribe", campaigns.CampaignContext{MemberRole: campaigns.RoleScribe, IsMember: true}, true},
		{"owner", campaigns.CampaignContext{MemberRole: campaigns.RoleOwner, IsMember: true}, false},
		{"player with DM access", campaigns.CampaignContext{MemberRole: campaigns.RolePlayer, IsMember: true, IsDmGranted: true}, false},
		{"site admin who isn't a member", campaigns.CampaignContext{MemberRole: campaigns.RoleNone, IsSiteAdmin: true}, false},
		{"guest preview", campaigns.CampaignContext{MemberRole: campaigns.RolePlayer, IsMember: true, Preview: campaigns.PreviewGuest}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.cc.Campaign = &campaigns.Campaign{ID: "c1"}
			c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
			if got := CanOpenPlayers(c, &tt.cc); got != tt.want {
				t.Errorf("CanOpenPlayers = %v, want %v", got, tt.want)
			}
		})
	}
}
