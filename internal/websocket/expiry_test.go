// expiry_test.go pins that a Client whose carried API-key ExpiresAt has
// passed stops receiving broadcasts immediately and gets its socket closed
// on the next ping tick — expiry was previously only checked at connect, so
// a key that expired mid-connection kept a Foundry socket alive forever.
package websocket

import (
	"testing"
	"time"
)

// TestBroadcast_SkipsExpiredClient pins the broadcast-loop half: an expired
// client is skipped even before writePump's ping ticker gets a chance to
// close it, and an unrelated live client still receives normally.
func TestBroadcast_SkipsExpiredClient(t *testing.T) {
	h := NewHub()
	go h.Run()
	srv := newRevokeTestServer(t, h)

	past := time.Now().Add(-time.Hour)
	expiredConn := dialTestClientWithExpiry(t, srv, "camp-1", "u-expired", "browser", &past)
	liveConn := dialTestClient(t, srv, "camp-1", "u-live", "browser")
	waitForClientCount(t, h, "camp-1", 2)

	h.Broadcast(NewMessage(MsgMarkerCreated, "camp-1", "r-1", nil))

	if _, arrived, err := readOneMessage(expiredConn, 300*time.Millisecond); arrived || err != nil {
		t.Errorf("expired client received a broadcast: arrived=%v err=%v", arrived, err)
	}
	if _, arrived, err := readOneMessage(liveConn, 2*time.Second); !arrived || err != nil {
		t.Errorf("live client never received the broadcast: arrived=%v err=%v", arrived, err)
	}
}

// TestWritePump_ClosesExpiredClient pins the ticker half: once a client's
// expiry passes, the next ping tick closes its socket instead of pinging
// it. The hub's ping interval is shortened so the test needn't wait 30s.
func TestWritePump_ClosesExpiredClient(t *testing.T) {
	h := NewHub()
	h.pingEvery = 20 * time.Millisecond
	go h.Run()
	srv := newRevokeTestServer(t, h)

	soon := time.Now().Add(10 * time.Millisecond)
	conn := dialTestClientWithExpiry(t, srv, "camp-1", "u-a", "browser", &soon)
	waitForClientCount(t, h, "camp-1", 1)

	if !wasClosed(conn, 2*time.Second) {
		t.Error("expired client's socket stayed open past its ping-tick check")
	}
}

// TestWritePump_DoesNotCloseUnexpiredClient is the over-correction guard: a
// client whose expiry is still in the future must survive ping ticks.
func TestWritePump_DoesNotCloseUnexpiredClient(t *testing.T) {
	h := NewHub()
	h.pingEvery = 20 * time.Millisecond
	go h.Run()
	srv := newRevokeTestServer(t, h)

	future := time.Now().Add(time.Hour)
	conn := dialTestClientWithExpiry(t, srv, "camp-1", "u-a", "browser", &future)
	waitForClientCount(t, h, "camp-1", 1)

	if !staysOpen(conn, 200*time.Millisecond) {
		t.Error("a client with a future expiry was closed by the ping ticker")
	}
}
