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

// The card offers the look-around choice, and a visitor's row carries the
// owner's two answers to it.
func TestGuestCodesCard_Visits(t *testing.T) {
	now := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	used := now.Add(-time.Hour)
	running := now.Add(49 * time.Hour)
	ended := now.Add(-24 * time.Hour)
	mira, sam := "mira", "sam"
	codes := []GuestCode{
		{ID: "gc-1", ExpiresAt: now.Add(time.Hour), UsedAt: &used, UsedBy: &mira, UsedByName: "Mira", VisitDays: 3, VisitUntil: &running, CreatedAt: used},
		{ID: "gc-2", ExpiresAt: now.Add(time.Hour), UsedAt: &used, UsedBy: &sam, UsedByName: "Sam", VisitDays: 1, VisitUntil: &ended, CreatedAt: used},
	}
	var buf bytes.Buffer
	if err := GuestCodesCard("camp-1", codes, nil, now, "https://chronicle.test/join", "csrf", "").Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	for _, want := range []string{
		`value="visit"`, `name="visit_days" value="1"`, `name="visit_days" value="7"`,
		"Looking around · 3 days left",
		`hx-post="/campaigns/camp-1/guest-codes/gc-1/player"`,
		`hx-post="/campaigns/camp-1/guest-codes/gc-1/end"`,
		`hx-post="/campaigns/camp-1/guest-codes/gc-2/player"`,
		"Looked around until Nov 11",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(html, "gc-2/end") {
		t.Error("an ended visit offers End now")
	}
}
