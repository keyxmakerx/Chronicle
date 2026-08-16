package calendar

// Closing "Collect RSVPs" must not turn every live link in every inbox into the
// failure page.
//
// THE DEFECT: rsvpDeadTokenPage required evt.CollectRSVPs before it would admit
// that an answer exists, the same condition resolveToken refuses on. So a
// Director who armed collection, took the head count and then closed collection
// — a natural thing to do; it is a toggle on the day card — left every player
// who re-opened their own link reading
//
//	RSVP Failed
//	this RSVP link is invalid or has expired
//
// including a player who answered YES, is still on the roster, and only wanted
// to check it went through. That is verbatim the sentence [GR-8] exists to stop
// players seeing, and its documented consequence — "a player who reads that
// tells their GM the RSVP system is broken, and the GM believes them" — applies
// unchanged.
//
// collect_rsvps is the operator's go/no-go for taking NEW answers. It is not a
// visibility rule, and the visibility rules (still on the roster, may still see
// the event) are unchanged and still enforced — the cases below prove both
// directions.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// answeredTokenFixture builds a handler whose token is spent and whose member
// answered "yes", with collection in the requested state.
func answeredTokenFixture(t *testing.T, collecting bool, roster []campaigns.CampaignMember, visibility string) *RSVPHandler {
	t.Helper()
	evt := testEvent(func(e *Event) {
		e.CollectRSVPs = collecting
		if visibility != "" {
			e.Visibility = visibility
		}
	})
	used := time.Now().UTC().Add(-time.Hour)
	future := time.Now().UTC().Add(24 * time.Hour)
	repo := &mockRSVPRepo{
		findTokenFn: func(_ context.Context, tok string) (*EventRSVPToken, error) {
			return &EventRSVPToken{Token: tok, EventID: evt.ID, UserID: "player-1",
				Action: RSVPActionYes, UsedAt: &used, ExpiresAt: future}, nil
		},
		getUserFn: func(_ context.Context, _, _ string) (*EventRSVP, error) {
			return &EventRSVP{ID: "r1", EventID: evt.ID, UserID: "player-1", Status: RSVPYes}, nil
		},
	}
	h := NewRSVPHandler(newTestRSVPService(repo, evt, testCalendar(nil)))
	h.SetMemberDirectory(&mockMemberDir{members: roster})
	return h
}

var answeredRoster = []campaigns.CampaignMember{{UserID: "player-1", Role: campaigns.RolePlayer}}

func TestDeadTokenPage_StillReassuresAfterCollectionIsClosed(t *testing.T) {
	h := answeredTokenFixture(t, false, answeredRoster, "")
	page := h.rsvpDeadTokenPage(context.Background(), "tok", errors.New("spent"))

	if strings.Contains(page, "RSVP Failed") {
		t.Fatalf("a member who answered YES, is still on the roster and re-opened their own "+
			"link was told the link failed, because collection had been closed; page=%s", page)
	}
	if !strings.Contains(page, "already answered") {
		t.Errorf("the reassurance page was not rendered; page=%s", page)
	}
}

func TestDeadTokenPage_StillReassuresWhileCollectionIsOpen(t *testing.T) {
	h := answeredTokenFixture(t, true, answeredRoster, "")
	page := h.rsvpDeadTokenPage(context.Background(), "tok", errors.New("spent"))
	if !strings.Contains(page, "already answered") {
		t.Errorf("the pre-existing collection-open case regressed; page=%s", page)
	}
}

// TestDeadTokenPage_StillHidesFromANonMember is the leak direction: dropping
// collect_rsvps from the gate must not drop the membership check with it.
func TestDeadTokenPage_StillHidesFromANonMember(t *testing.T) {
	for _, tc := range []struct {
		name   string
		roster []campaigns.CampaignMember
	}{
		{"removed from the campaign", []campaigns.CampaignMember{{UserID: "someone-else", Role: campaigns.RolePlayer}}},
		{"empty roster", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := answeredTokenFixture(t, false, tc.roster, "")
			page := h.rsvpDeadTokenPage(context.Background(), "tok", errors.New("spent"))
			if !strings.Contains(page, "RSVP Failed") {
				t.Fatalf("a non-member got the reassurance page; page=%s", page)
			}
			if strings.Contains(page, "Harvest Feast") {
				t.Errorf("the event's title leaked to a non-member; page=%s", page)
			}
		})
	}
}

// TestDeadTokenPage_StillHidesADMOnlyEventFromAPlayer is the other leak
// direction: an event flipped to dm_only after the invite went out.
func TestDeadTokenPage_StillHidesADMOnlyEventFromAPlayer(t *testing.T) {
	h := answeredTokenFixture(t, false, answeredRoster, storageVisibilityDMOnly)
	page := h.rsvpDeadTokenPage(context.Background(), "tok", errors.New("spent"))
	if !strings.Contains(page, "RSVP Failed") {
		t.Fatalf("a dm_only event's answer was disclosed to a Player; page=%s", page)
	}
	if strings.Contains(page, "Harvest Feast") {
		t.Errorf("a dm_only event's title leaked; page=%s", page)
	}
}
