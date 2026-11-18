package auth

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestInvitationLimiterMatchesJoin(t *testing.T) {
	l := newInvitationLimiter()
	now := time.Now()
	l.now = func() time.Time { return now }
	for i := 0; i < invitationLimit; i++ {
		if !l.allow("1.2.3.4") {
			t.Fatalf("lookup %d refused inside the budget", i+1)
		}
	}
	if l.allow("1.2.3.4") {
		t.Fatal("an eleventh lookup in a minute was allowed")
	}
	if !l.allow("5.6.7.8") {
		t.Fatal("another address shares the budget")
	}
	now = now.Add(invitationWindow + time.Second)
	if !l.allow("1.2.3.4") {
		t.Fatal("the budget did not reset after a minute")
	}
}

func TestInvitationCardShowsNamesOnly(t *testing.T) {
	inv := &Invitation{
		CampaignName: "The Ashen Reach",
		Members:      []InvitationMember{{Name: "Olive"}, {Name: "Pat"}, {Name: "Quinn"}},
		NextNight:    &InvitationNight{Date: "2026-12-01", Time: "", TZ: ""},
	}
	var b bytes.Buffer
	if err := InvitationCard(inv).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{"The Ashen Reach", "Olive, Pat and Quinn are at the table", "Next game night", "Tuesday, December 1"} {
		if !strings.Contains(out, want) {
			t.Errorf("card lacks %q", want)
		}
	}
	if strings.Contains(out, "@") {
		t.Error("the card carries an address")
	}
}

func TestInvitationNames(t *testing.T) {
	m := func(n ...string) []InvitationMember {
		var out []InvitationMember
		for _, s := range n {
			out = append(out, InvitationMember{Name: s})
		}
		return out
	}
	tests := []struct {
		in   []InvitationMember
		want string
	}{
		{m("Olive"), "Olive is at the table"},
		{m("Olive", "Pat"), "Olive and Pat are at the table"},
		{m("A", "B", "C", "D", "E", "F", "G", "H"), "A, B, C, D, E, F and 2 more are at the table"},
	}
	for _, tc := range tests {
		if got := invitationNames(tc.in); got != tc.want {
			t.Errorf("got %q, want %q", got, tc.want)
		}
	}
}

func TestJoinStepAsksForNameOnlyAfterTheInvitation(t *testing.T) {
	var b bytes.Buffer
	if err := JoinForm_("tok", "", "", "", "", nil).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), `name="name"`) || strings.Contains(b.String(), "invitation") {
		t.Error("the code step asked for a name or showed an invitation")
	}
	b.Reset()
	inv := &Invitation{CampaignName: "The Ashen Reach"}
	if err := JoinForm_("tok", "ABCD2345", "", "", "", inv).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `name="name"`) || !strings.Contains(b.String(), "The Ashen Reach") {
		t.Error("the name step lacks the name field or the invitation")
	}
}
