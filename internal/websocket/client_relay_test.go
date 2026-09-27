// client_relay_test.go pins that a client-sent message is never forwarded
// to other sockets — only a server-emitted broadcast should ever arrive.
// Reuses hub_revoke_test.go's real-server/real-conn helpers (same package).
package websocket

import (
	"encoding/json"
	"net"
	"testing"
	"time"

	gorillaWs "github.com/gorilla/websocket"
)

// readOneMessage waits up to timeout for the next message on conn.
// ok=false on a read-deadline timeout (nothing arrived), distinct from a
// real connection error — mirrors wasClosed/staysOpen's timeout split.
func readOneMessage(conn *gorillaWs.Conn, timeout time.Duration) (data []byte, ok bool, err error) {
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	_, data, err = conn.ReadMessage()
	if err == nil {
		return data, true, nil
	}
	if ne, isNetErr := err.(net.Error); isNetErr && ne.Timeout() {
		return nil, false, nil
	}
	return nil, false, err
}

func TestReadPump_DoesNotRelayClientMessage(t *testing.T) {
	h := NewHub()
	go h.Run()
	srv := newRevokeTestServer(t, h)

	senderConn := dialTestClient(t, srv, "camp-1", "u-sender", "browser")
	// Two victims: gorilla's read errors are permanent, including a
	// timeout, so the "nothing arrived" conn can't be read again after.
	victimA := dialTestClient(t, srv, "camp-1", "u-victim-a", "browser")
	victimB := dialTestClient(t, srv, "camp-1", "u-victim-b", "browser")
	waitForClientCount(t, h, "camp-1", 3)

	forged := &Message{
		Type:       MsgMarkerCreated,
		CampaignID: "camp-1",
		ResourceID: "forged-marker",
	}
	data, err := forged.Encode()
	if err != nil {
		t.Fatalf("encode forged message: %v", err)
	}
	if err := senderConn.WriteMessage(gorillaWs.TextMessage, data); err != nil {
		t.Fatalf("write forged message: %v", err)
	}

	if got, arrived, readErr := readOneMessage(victimA, 300*time.Millisecond); arrived || readErr != nil {
		t.Errorf("victim received a relayed client message: arrived=%v err=%v data=%s", arrived, readErr, got)
	}

	// A real, server-emitted broadcast must still get through — this is
	// the over-correction guard: the fix must not blackhole legitimate
	// events along with the forged one.
	h.Broadcast(NewMessage(MsgMarkerCreated, "camp-1", "real-marker", nil))

	got, arrived, err := readOneMessage(victimB, 2*time.Second)
	if err != nil {
		t.Fatalf("reading server broadcast: %v", err)
	}
	if !arrived {
		t.Fatal("a server-emitted broadcast never arrived at the victim")
	}
	var decoded Message
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("decode server broadcast: %v", err)
	}
	if decoded.ResourceID != "real-marker" {
		t.Errorf("resourceId = %q, want real-marker — got some other message (the forged one?)", decoded.ResourceID)
	}
}
