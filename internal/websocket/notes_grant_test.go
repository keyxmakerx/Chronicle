// notes_grant_test.go pins the notes grant socket: a Foundry notebook frame
// signs in with the player's notes grant (offered as a subprotocol), gets
// note events only, and is closed when the grant or Sync API goes.
package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	gorillaWs "github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/foundry_vtt"
)

type fakeNotesGrants map[string][2]string // token -> campaign, user

func (f fakeNotesGrants) AuthenticateNotesGrantForWS(_ context.Context, token string) (string, string, error) {
	g, ok := f[token]
	if !ok {
		return "", "", errors.New("notes access not allowed")
	}
	return g[0], g[1], nil
}

type fakeRoles map[string]int // campaign:user -> role

func (f fakeRoles) GetUserCampaignRole(_ context.Context, campaignID, userID string) (int, error) {
	return f[campaignID+":"+userID], nil
}

func (f fakeRoles) IsUserDmGranted(context.Context, string, string) (bool, error) { return false, nil }

// failingSession fails the test if anything falls through to cookie auth.
type failingSession struct{ t *testing.T }

func (s failingSession) AuthenticateSessionForWS(*http.Request) (string, error) {
	s.t.Error("a notes grant offer fell through to session auth")
	return "", errors.New("no session")
}

func newNotesAuth(t *testing.T) *MultiAuthenticator {
	a := NewMultiAuthenticator(nil, failingSession{t}, fakeRoles{"camp:player": 1})
	a.SetNotesGrantAuth(fakeNotesGrants{"cnt_good": {"camp", "player"}, "cnt_left": {"camp", "gone"}})
	return a
}

func TestAuthenticateWS_NotesGrant(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		protocols string
		wantErr   bool
	}{
		{"a live grant signs in as its player", "/ws?campaign=camp", "chronicle.notes, chronicle.grant.cnt_good", false},
		{"the campaign may be left out", "/ws", "chronicle.grant.cnt_good, chronicle.notes", false},
		{"a grant for another campaign is refused", "/ws?campaign=other", "chronicle.notes, chronicle.grant.cnt_good", true},
		{"an unknown grant is refused", "/ws?campaign=camp", "chronicle.notes, chronicle.grant.cnt_bad", true},
		{"a player who left the campaign is refused", "/ws?campaign=camp", "chronicle.notes, chronicle.grant.cnt_left", true},
		{"the notes protocol without a grant never falls back to a cookie", "/ws?campaign=camp", "chronicle.notes", true},
		{"a grant without the notes protocol is refused", "/ws?campaign=camp", "chronicle.grant.cnt_good", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tt.url, nil)
			r.Header.Set("Sec-WebSocket-Protocol", tt.protocols)
			campaignID, userID, source, role, _, expiresAt, err := newNotesAuth(t).AuthenticateWS(r)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("signed in as %s/%s, want refused", campaignID, userID)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if campaignID != "camp" || userID != "player" || source != NotesAppSource || role != 1 || expiresAt != nil {
				t.Fatalf("got %s %s %s role %d expires %v", campaignID, userID, source, role, expiresAt)
			}
		})
	}
}

func TestAuthenticateWS_NotesGrantNotWiredIsRefused(t *testing.T) {
	a := NewMultiAuthenticator(nil, failingSession{t}, fakeRoles{"camp:player": 1})
	r := httptest.NewRequest(http.MethodGet, "/ws?campaign=camp", nil)
	r.Header.Set("Sec-WebSocket-Protocol", "chronicle.notes, chronicle.grant.cnt_good")
	if _, _, _, _, _, _, err := a.AuthenticateWS(r); err == nil {
		t.Fatal("a grant was accepted with no grant authenticator wired")
	}
}

// TestHandleUpgrade_NotesGrantFromTheFrame drives the real upgrade path the
// way the frame does: same-origin, grant offered as a subprotocol. The
// server must answer with the notes protocol only, never echo the token.
func TestHandleUpgrade_NotesGrantFromTheFrame(t *testing.T) {
	h := NewHub()
	go h.Run()
	e := echo.New()
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	e.GET("/ws", HandleUpgrade(h, newNotesAuth(t), []string{srv.URL}, nil))

	dialer := gorillaWs.Dialer{Subprotocols: []string{NotesSubprotocol, NotesGrantProtocolPrefix + "cnt_good"}}
	hdr := http.Header{"Origin": {srv.URL}}
	conn, resp, err := dialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ws?campaign=camp", hdr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if got := resp.Header.Get("Sec-WebSocket-Protocol"); got != NotesSubprotocol {
		t.Fatalf("server answered subprotocol %q, want %q", got, NotesSubprotocol)
	}
	waitForClientCount(t, h, "camp", 1)

	// Another site can't use a grant it got hold of from a browser page.
	_, resp, err = dialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ws?campaign=camp", http.Header{"Origin": {"https://evil.example"}})
	if err == nil {
		t.Fatal("a cross-site page opened a notes socket")
	}
	if resp != nil && resp.StatusCode == http.StatusSwitchingProtocols {
		t.Fatal("cross-site upgrade succeeded")
	}
}

func TestHub_NotesGrantSocketGetsNoteEventsOnly(t *testing.T) {
	h := newVisibilityTestHub(t)
	frame := registerTestClientAs(t, h, NotesAppSource, "camp", "player", 1, false)
	browser := registerTestClient(t, h, "camp", "other", 1, false)

	h.Broadcast(NewMessage(MsgEntityUpdated, "camp", "e1", map[string]string{"name": "Harbor"}))
	settleBroadcast()
	if got := drainOrNil(frame); got != nil {
		t.Fatalf("notes socket received a non-note event: %s", got)
	}
	if drainOrNil(browser) == nil {
		t.Fatal("browser socket missed the entity event (test setup broken)")
	}

	h.Broadcast(NewMessage(MsgNoteUpdated, "camp", "n1", map[string]string{"noteId": "n1"}))
	settleBroadcast()
	got := drainOrNil(frame)
	if got == nil {
		t.Fatal("notes socket missed a note event")
	}
	var m Message
	if err := json.Unmarshal(got, &m); err != nil || m.Type != MsgNoteUpdated {
		t.Fatalf("got %s (%v)", got, err)
	}

	// A note private to someone else still never reaches the frame.
	private := NewMessage(MsgNoteUpdated, "camp", "n2", map[string]string{"noteId": "n2"})
	private.AllowedUsers, private.StrictAudience = []string{"other"}, true
	h.Broadcast(private)
	settleBroadcast()
	if got := drainOrNil(frame); got != nil {
		t.Fatalf("notes socket received someone else's private note: %s", got)
	}
}

func TestRevokeNotesAppClients_ClosesOnlyThatPlayersFrames(t *testing.T) {
	h := NewHub()
	go h.Run()
	srv := newRevokeTestServer(t, h)

	target := dialTestClient(t, srv, "camp-1", "u-target", NotesAppSource)
	targetBrowser := dialTestClient(t, srv, "camp-1", "u-target", "browser")
	otherPlayer := dialTestClient(t, srv, "camp-1", "u-other", NotesAppSource)
	otherCampaign := dialTestClient(t, srv, "camp-2", "u-target", NotesAppSource)
	waitForClientCount(t, h, "camp-1", 3)
	waitForClientCount(t, h, "camp-2", 1)

	h.RevokeNotesAppClients("camp-1", "u-target")

	if !wasClosed(target, 2*time.Second) {
		t.Error("the revoked player's notes frame stayed connected")
	}
	if !staysOpen(targetBrowser, 300*time.Millisecond) {
		t.Error("the player's signed-in browser tab was disconnected")
	}
	if !staysOpen(otherPlayer, 300*time.Millisecond) {
		t.Error("another player's notes frame was disconnected")
	}
	if !staysOpen(otherCampaign, 300*time.Millisecond) {
		t.Error("the player's notes frame in another campaign was disconnected")
	}
}

func TestRevokeAPIKeyClients_ClosesNotesFramesToo(t *testing.T) {
	h := NewHub()
	go h.Run()
	srv := newRevokeTestServer(t, h)
	frame := dialTestClient(t, srv, "camp-1", "u-player", NotesAppSource)
	waitForClientCount(t, h, "camp-1", 1)

	h.RevokeAPIKeyClients("camp-1")

	if !wasClosed(frame, 2*time.Second) {
		t.Error("a notes frame outlived Sync API being switched off")
	}
}

func TestRevokeNotesAppClientsEverywhere_LeavesOtherSockets(t *testing.T) {
	h := NewHub()
	go h.Run()
	srv := newRevokeTestServer(t, h)
	frame1 := dialTestClient(t, srv, "camp-1", "u-target", NotesAppSource)
	frame2 := dialTestClient(t, srv, "camp-2", "u-target", NotesAppSource)
	sync := dialTestClient(t, srv, "camp-1", "u-target", foundry_vtt.ModuleSource)
	waitForClientCount(t, h, "camp-1", 2)
	waitForClientCount(t, h, "camp-2", 1)

	h.RevokeNotesAppClientsEverywhere("u-target")

	if !wasClosed(frame1, 2*time.Second) || !wasClosed(frame2, 2*time.Second) {
		t.Error("a notes frame outlived the user's grants")
	}
	if !staysOpen(sync, 300*time.Millisecond) {
		t.Error("the user's Foundry sync socket was disconnected")
	}
}

// TestNotesFramesUseTheNotesSubprotocols pins the strings the Journal and
// jot panel offer in a frame to the ones AuthenticateWS reads; a drift would
// leave the frame reconnecting forever with no live updates.
func TestNotesFramesUseTheNotesSubprotocols(t *testing.T) {
	want := "['" + NotesSubprotocol + "', '" + NotesGrantProtocolPrefix + "' + embed.token]"
	for _, f := range []string{"journal.js", "notes.js"} {
		src, err := os.ReadFile("../../static/js/widgets/" + f)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(src), want) {
			t.Errorf("%s does not offer %s", f, want)
		}
	}
}
