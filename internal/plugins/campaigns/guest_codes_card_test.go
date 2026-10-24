package campaigns

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

// A fresh code offers a link with the code filled in, so the friend it is
// sent to only types a name; the join page reads ?code= to fill it.
func TestGuestCodesCard_OffersInviteLink(t *testing.T) {
	var buf bytes.Buffer
	fresh := &NewGuestCode{Code: "ABCD-2345"}
	err := GuestCodesCard("camp-1", nil, fresh, time.Now(), "https://chronicle.test/join", "csrf", "").Render(context.Background(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if !strings.Contains(html, "Copy invite link") {
		t.Errorf("no Copy invite link button:\n%s", html)
	}
	if !strings.Contains(html, "https://chronicle.test/join?code=ABCD-2345") {
		t.Errorf("the invite link does not carry the code:\n%s", html)
	}
}
