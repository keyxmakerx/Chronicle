// hub_revoke_test.go pins the hub's connection-revocation surface, closing
// only matching clients. Unlike hub_visibility_test.go's registerTestClient,
// it drives real *gorilla websocket.Conn pairs, not a fake conn.
package websocket

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	gorillaWs "github.com/gorilla/websocket"

	"github.com/keyxmakerx/chronicle/internal/plugins/foundry_vtt"
)

// newRevokeTestServer upgrades every request to a WebSocket and registers
// it with h. Campaign/user/source/expires come from query params — a
// stand-in for the real Authenticator, which this test doesn't need.
func newRevokeTestServer(t *testing.T, h *Hub) *httptest.Server {
	t.Helper()
	upgrader := gorillaWs.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		q := r.URL.Query()
		var expiresAt *time.Time
		if raw := q.Get("expires"); raw != "" {
			if ns, err := strconv.ParseInt(raw, 10, 64); err == nil {
				exp := time.Unix(0, ns)
				expiresAt = &exp
			}
		}
		h.RegisterClient(conn, q.Get("campaign"), q.Get("user"), q.Get("source"), 1, false, expiresAt)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// dialTestClient opens a real WebSocket connection to srv for the given
// campaign/user/source triple and registers it for cleanup.
func dialTestClient(t *testing.T, srv *httptest.Server, campaignID, userID, source string) *gorillaWs.Conn {
	t.Helper()
	return dialTestClientWithExpiry(t, srv, campaignID, userID, source, nil)
}

// dialTestClientWithExpiry is dialTestClient plus an optional key-expiry
// carried in the "expires" query param newRevokeTestServer reads.
func dialTestClientWithExpiry(t *testing.T, srv *httptest.Server, campaignID, userID, source string, expiresAt *time.Time) *gorillaWs.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") +
		"/ws?campaign=" + campaignID + "&user=" + userID + "&source=" + source
	if expiresAt != nil {
		url += "&expires=" + strconv.FormatInt(expiresAt.UnixNano(), 10)
	}
	conn, _, err := gorillaWs.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// waitForClientCount polls until the campaign has at least n registered
// clients, so a revoke call under test can't race the async RegisterClient.
func waitForClientCount(t *testing.T, h *Hub, campaignID string, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if h.CampaignClientCount(campaignID) >= n {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("campaign %s never reached %d registered clients", campaignID, n)
}

// wasClosed reports whether conn was actually closed within timeout — a
// read-deadline timeout does not count, only a real close does. The exact
// complement of staysOpen, so a no-op revoke can't misreport as a close.
func wasClosed(conn *gorillaWs.Conn, timeout time.Duration) bool {
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	_, _, err := conn.ReadMessage()
	if err == nil {
		return false
	}
	ne, ok := err.(net.Error)
	return !ok || !ne.Timeout()
}

// staysOpen reports whether conn is still open after the timeout — a read
// timeout (no close frame arrived) means still open; any other error means
// the server dropped it.
func staysOpen(conn *gorillaWs.Conn, timeout time.Duration) bool {
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	_, _, err := conn.ReadMessage()
	if err == nil {
		return true
	}
	ne, ok := err.(net.Error)
	return ok && ne.Timeout()
}

func TestRevokeAPIKeyClients_ClosesOnlyFoundrySourcedSockets(t *testing.T) {
	h := NewHub()
	go h.Run()
	srv := newRevokeTestServer(t, h)

	foundryConn := dialTestClient(t, srv, "camp-1", "u-foundry", foundry_vtt.ModuleSource)
	legacyConn := dialTestClient(t, srv, "camp-1", "u-legacy", "foundry")
	browserConn := dialTestClient(t, srv, "camp-1", "u-browser", "browser")
	otherCampaignConn := dialTestClient(t, srv, "camp-2", "u-other-campaign", foundry_vtt.ModuleSource)

	waitForClientCount(t, h, "camp-1", 3)
	waitForClientCount(t, h, "camp-2", 1)

	h.RevokeAPIKeyClients("camp-1")

	if !wasClosed(foundryConn, 2*time.Second) {
		t.Error("foundry-module client in the revoked campaign stayed connected")
	}
	if !wasClosed(legacyConn, 2*time.Second) {
		t.Error("legacy 'foundry' source client in the revoked campaign stayed connected")
	}
	if !staysOpen(browserConn, 300*time.Millisecond) {
		t.Error("browser client was disconnected by a campaign-scoped API-key revoke")
	}
	if !staysOpen(otherCampaignConn, 300*time.Millisecond) {
		t.Error("a foundry-module client in a DIFFERENT campaign was disconnected")
	}
}

func TestRevokeUser_ClosesOnlyThatUsersSockets(t *testing.T) {
	h := NewHub()
	go h.Run()
	srv := newRevokeTestServer(t, h)

	targetConn := dialTestClient(t, srv, "camp-1", "u-target", "browser")
	otherUserConn := dialTestClient(t, srv, "camp-1", "u-other", "browser")
	sameUserOtherCampaignConn := dialTestClient(t, srv, "camp-2", "u-target", "browser")

	waitForClientCount(t, h, "camp-1", 2)
	waitForClientCount(t, h, "camp-2", 1)

	h.RevokeUser("camp-1", "u-target")

	if !wasClosed(targetConn, 2*time.Second) {
		t.Error("target user's client stayed connected after RevokeUser")
	}
	if !staysOpen(otherUserConn, 300*time.Millisecond) {
		t.Error("a different user's client was disconnected by RevokeUser")
	}
	if !staysOpen(sameUserOtherCampaignConn, 300*time.Millisecond) {
		t.Error("the target user's client in a DIFFERENT campaign was disconnected")
	}
}

func TestRevoke_NoMatchingClientsIsNoOp(t *testing.T) {
	h := NewHub()
	go h.Run()
	// Nothing registered anywhere; both calls must return without blocking
	// or panicking on an absent campaign entry.
	h.RevokeAPIKeyClients("camp-empty")
	h.RevokeUser("camp-empty", "nobody")
}

func TestRevokeUserEverywhere_ClosesThatUsersSocketsInEveryCampaign(t *testing.T) {
	h := NewHub()
	go h.Run()
	srv := newRevokeTestServer(t, h)

	targetCamp1Conn := dialTestClient(t, srv, "camp-1", "u-target", "browser")
	targetCamp2Conn := dialTestClient(t, srv, "camp-2", "u-target", "browser")
	otherUserConn := dialTestClient(t, srv, "camp-1", "u-other", "browser")

	waitForClientCount(t, h, "camp-1", 2)
	waitForClientCount(t, h, "camp-2", 1)

	h.RevokeUserEverywhere("u-target")

	if !wasClosed(targetCamp1Conn, 2*time.Second) {
		t.Error("target user's socket in camp-1 stayed connected after RevokeUserEverywhere")
	}
	if !wasClosed(targetCamp2Conn, 2*time.Second) {
		t.Error("target user's socket in camp-2 stayed connected after RevokeUserEverywhere")
	}
	if !staysOpen(otherUserConn, 300*time.Millisecond) {
		t.Error("a different user's socket was disconnected by RevokeUserEverywhere")
	}
}

func TestRevokeUserEverywhere_NoMatchingClientsIsNoOp(t *testing.T) {
	h := NewHub()
	go h.Run()
	// Nothing registered anywhere; must return without blocking or
	// panicking on an empty h.clients map.
	h.RevokeUserEverywhere("nobody")
}

func TestRevokeCampaign_ClosesEveryClientRegardlessOfUserOrSource(t *testing.T) {
	h := NewHub()
	go h.Run()
	srv := newRevokeTestServer(t, h)

	browserConn := dialTestClient(t, srv, "camp-1", "u-a", "browser")
	foundryConn := dialTestClient(t, srv, "camp-1", "u-b", foundry_vtt.ModuleSource)
	otherCampaignConn := dialTestClient(t, srv, "camp-2", "u-a", "browser")

	waitForClientCount(t, h, "camp-1", 2)
	waitForClientCount(t, h, "camp-2", 1)

	h.RevokeCampaign("camp-1")

	if !wasClosed(browserConn, 2*time.Second) {
		t.Error("browser client in the revoked campaign stayed connected")
	}
	if !wasClosed(foundryConn, 2*time.Second) {
		t.Error("foundry client in the revoked campaign stayed connected")
	}
	if !staysOpen(otherCampaignConn, 300*time.Millisecond) {
		t.Error("a client in a DIFFERENT campaign was disconnected")
	}
}

func TestRevokeCampaign_NoMatchingClientsIsNoOp(t *testing.T) {
	h := NewHub()
	go h.Run()
	h.RevokeCampaign("camp-empty")
}
