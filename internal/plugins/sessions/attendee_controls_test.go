package sessions

// The session detail page must offer RSVP controls to every member, not only to
// members who already hold a session_attendees row.
//
// THE DEFECT: InviteAll runs at session-creation time and nowhere else, and no
// route adds an attendee afterwards. AttendeeList rendered the buttons inside
// `for _, a := range attendees { if a.UserID == currentUserID }` and the whole
// block sat inside the `len(attendees) != 0` branch — so a player who joined the
// campaign after a session existed opened the page, read "3 going · 4 invited",
// and found no control anywhere. Posting the endpoint directly answered 404.
// The Director had no invite action either; the only escape was deleting and
// recreating the session, losing its notes, recap and entity links.

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// renderAttendeeList renders the component to a string.
func renderAttendeeList(t *testing.T, attendees []Attendee, userID string, isMember bool) string {
	t.Helper()
	cc := &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: "camp-1"},
		IsMember: isMember,
	}
	var sb strings.Builder
	if err := AttendeeList(cc, "s1", attendees, "csrf", userID).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	return sb.String()
}

// hasRSVPButtons reports whether the three RSVP buttons are present.
func hasRSVPButtons(html string) bool {
	return strings.Contains(html, `hx-vals='{"status":"accepted"}'`) &&
		strings.Contains(html, `hx-vals='{"status":"tentative"}'`) &&
		strings.Contains(html, `hx-vals='{"status":"declined"}'`)
}

func TestAttendeeList_LateJoinerGetsRSVPControls(t *testing.T) {
	// The session was created before "newcomer" joined the campaign, so only the
	// original members hold attendee rows.
	attendees := []Attendee{
		{UserID: "owner", DisplayName: "Ana", Status: RSVPAccepted},
		{UserID: "player-1", DisplayName: "Bo", Status: RSVPInvited},
	}
	html := renderAttendeeList(t, attendees, "newcomer", true)
	if !hasRSVPButtons(html) {
		t.Fatal("a campaign member with no attendee row was rendered NO RSVP control — " +
			"they cannot answer, and there is no other route that would let them")
	}
	// Nothing may be pre-highlighted: they have not answered.
	if strings.Contains(html, "ring-green-300") {
		t.Error("the newcomer's Going button is rendered as already-chosen")
	}
}

func TestAttendeeList_EmptyAttendeeListStillOffersControls(t *testing.T) {
	html := renderAttendeeList(t, nil, "player-1", true)
	if !hasRSVPButtons(html) {
		t.Fatal("with no attendees at all the page offers nobody any RSVP control, " +
			"so the first answer can never be recorded from the UI")
	}
}

func TestAttendeeList_ExistingAttendeeKeepsTheirHighlight(t *testing.T) {
	attendees := []Attendee{{UserID: "player-1", DisplayName: "Bo", Status: RSVPAccepted}}
	html := renderAttendeeList(t, attendees, "player-1", true)
	if !hasRSVPButtons(html) {
		t.Fatal("an invited member lost their RSVP controls")
	}
	if !strings.Contains(html, "ring-green-300") {
		t.Error("the member's stored 'accepted' answer is no longer highlighted")
	}
}

func TestAttendeeList_NonMemberGetsNoControls(t *testing.T) {
	attendees := []Attendee{{UserID: "owner", DisplayName: "Ana", Status: RSVPAccepted}}
	// A public / non-member visitor: aggregate counts only, and no controls.
	html := renderAttendeeList(t, attendees, "stranger", false)
	if hasRSVPButtons(html) {
		t.Fatal("a non-member was offered RSVP controls")
	}
	if !strings.Contains(html, "going ·") {
		t.Errorf("non-member view lost its aggregate line; html=%s", html)
	}
}

// TestAttendeeList_AnonymousGetsNoControls pins the userID == "" case: an
// unauthenticated visitor must never be handed buttons that post.
func TestAttendeeList_AnonymousGetsNoControls(t *testing.T) {
	attendees := []Attendee{{UserID: "owner", DisplayName: "Ana", Status: RSVPAccepted}}
	if hasRSVPButtons(renderAttendeeList(t, attendees, "", true)) {
		t.Fatal("an anonymous visitor was offered RSVP controls")
	}
}

// TestMyRSVPStatus keeps the helper honest: absence is "", never "invited".
func TestMyRSVPStatus(t *testing.T) {
	attendees := []Attendee{{UserID: "u1", Status: RSVPTentative}}
	if got := myRSVPStatus(attendees, "u1"); got != RSVPTentative {
		t.Errorf("stored status = %q, want %q", got, RSVPTentative)
	}
	if got := myRSVPStatus(attendees, "u2"); got != "" {
		t.Errorf("a member with no row = %q, want \"\" — they were never invited, "+
			"so reporting 'invited' would be a claim about a row that does not exist", got)
	}
	if got := myRSVPStatus(attendees, ""); got != "" {
		t.Errorf("anonymous = %q, want \"\"", got)
	}
}
