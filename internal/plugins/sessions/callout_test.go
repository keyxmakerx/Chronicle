package sessions

import (
	"strings"
	"testing"
	"time"
)

// C-RSVP-P10 — the player call-to-action.
//
// The banner is the one thing in the app shell that speaks to a player without
// being asked, so every claim it can make is tested: that it stays silent when
// there is nothing to say, that it never speaks for a notification the player
// is not expected to answer, that the urgent thing outranks the standing one,
// and that nothing from the database reaches the page as markup.

func note(ntype string, read bool, link string) Notification {
	n := Notification{Type: ntype, CreatedAt: time.Now().UTC()}
	if read {
		t := time.Now().UTC()
		n.ReadAt = &t
	}
	if link != "" {
		l := link
		n.Link = &l
	}
	return n
}

// Silence is the default and by far the most common answer. A banner that
// appears when it has nothing to say is worse than no banner: it teaches every
// player to dismiss it before reading.
func TestCallout_SaysNothingWhenThereIsNothingToSay(t *testing.T) {
	got := BuildCallout(nil, true, time.Now())
	if got.Kind != CalloutNone {
		t.Fatalf("Kind = %q, want silence", got.Kind)
	}
	if got.Message != "" {
		t.Errorf("a silent callout still carried a message: %q", got.Message)
	}
	if renderCallout(got) != "" {
		t.Error("a silent callout rendered markup — the poll swaps innerHTML, so this " +
			"would leave a visible empty strip above every page")
	}
}

// An ANSWERED request must stop the banner. This is what makes it a state
// rather than an announcement, and it is the difference between a banner that
// people trust and one they learn to ignore.
func TestCallout_AReadNotificationNoLongerCounts(t *testing.T) {
	got := BuildCallout([]Notification{note(NotifCalendarRSVP, true, "/x")}, true, time.Now())
	if got.Kind != CalloutNone {
		t.Fatalf("a read RSVP still raised %q — the banner would never go away", got.Kind)
	}
}

// News is not a request. Promoting "the time is settled" or "somebody replied
// to your proposal" into a page-wide banner would train players to dismiss it
// unread, taking the one that DID need an answer with it.
func TestCallout_IgnoresNotificationsThatAreNewsRatherThanRequests(t *testing.T) {
	for _, ntype := range []string{
		NotifProposalConfirmed,
		NotifProposalResponse,
		NotifAvailabilityNudge,
	} {
		got := BuildCallout([]Notification{note(ntype, false, "/x")}, true, time.Now())
		if got.Kind != CalloutNone {
			t.Errorf("%s raised a banner (%q); only a request the player must ANSWER may",
				ntype, got.Kind)
		}
	}
}

func TestCallout_RaisesOnAnUnansweredRequest(t *testing.T) {
	for _, ntype := range []string{NotifProposalCreated, NotifCalendarRSVP} {
		got := BuildCallout([]Notification{note(ntype, false, "/campaigns/c1/proposals/p1")}, true, time.Now())
		if got.Kind != CalloutRSVP {
			t.Fatalf("%s: Kind = %q, want %q", ntype, got.Kind, CalloutRSVP)
		}
		if got.Count != 1 {
			t.Errorf("%s: Count = %d, want 1", ntype, got.Count)
		}
		if got.Link != "/campaigns/c1/proposals/p1" {
			t.Errorf("%s: Link = %q — the banner must point at the thing it is about", ntype, got.Link)
		}
	}
}

// Several outstanding requests report a count and link to the newest. The list
// arrives newest-first, so "first with a link wins" is that rule.
func TestCallout_CountsSeveralAndLinksToTheNewest(t *testing.T) {
	got := BuildCallout([]Notification{
		note(NotifCalendarRSVP, false, "/newest"),
		note(NotifProposalConfirmed, false, "/news-not-a-request"),
		note(NotifProposalCreated, false, "/older"),
		note(NotifCalendarRSVP, true, "/already-answered"),
	}, true, time.Now())

	if got.Count != 2 {
		t.Fatalf("Count = %d, want 2 — read rows and news must not be counted", got.Count)
	}
	if got.Link != "/newest" {
		t.Errorf("Link = %q, want /newest", got.Link)
	}
	if !strings.Contains(got.Message, "answers") {
		t.Errorf("Message = %q; with more than one outstanding it should not say \"answer\"", got.Message)
	}
}

// Precedence. An RSVP is time-limited — the session gets scheduled with or
// without this player — where a missing timezone is wrong every day but keeps
// until tomorrow. Showing both at once was rejected: two stacked banners is how
// a shell starts eating the screen.
func TestCallout_AnUnansweredRequestOutranksTheTimezoneAsk(t *testing.T) {
	got := BuildCallout([]Notification{note(NotifCalendarRSVP, false, "/x")}, false, time.Now())
	if got.Kind != CalloutRSVP {
		t.Fatalf("Kind = %q, want the time-limited one to win", got.Kind)
	}
}

func TestCallout_AsksForTheZoneWhenItIsUnsetAndNothingIsPending(t *testing.T) {
	got := BuildCallout(nil, false, time.Now())
	if got.Kind != CalloutTimezone {
		t.Fatalf("Kind = %q, want %q", got.Kind, CalloutTimezone)
	}
	// The sentence must state the CONSEQUENCE. "Set your timezone" is a chore;
	// "you are being shown UTC" is the reason anyone would act.
	if !strings.Contains(got.Message, "UTC") {
		t.Errorf("Message = %q — it must say what is happening now, not just ask for a setting", got.Message)
	}
}

func TestCallout_StaysSilentOnceTheZoneIsSet(t *testing.T) {
	if got := BuildCallout(nil, true, time.Now()); got.Kind != CalloutNone {
		t.Fatalf("Kind = %q, want silence for a player who has already answered", got.Kind)
	}
}

// --- rendering -------------------------------------------------------------

// The Link comes out of a database row and is interpolated into a fragment with
// Sprintf. That is exactly where a stored value becomes markup, so the escape
// is asserted rather than assumed.
func TestRenderCallout_EscapesTheStoredLink(t *testing.T) {
	got := renderCallout(Callout{
		Kind:    CalloutRSVP,
		Message: "Your table is waiting on your answer",
		Link:    `/x" onmouseover="alert(1)`,
		Count:   1,
	})
	if strings.Contains(got, `onmouseover="alert(1)`) {
		t.Fatalf("a stored link escaped into markup unescaped:\n%s", got)
	}
	if !strings.Contains(got, "&#34;") && !strings.Contains(got, "&quot;") {
		t.Errorf("the link was interpolated without escaping its quote:\n%s", got)
	}
}

func TestRenderCallout_OmitsTheActionWhenThereIsNowhereToGo(t *testing.T) {
	got := renderCallout(Callout{Kind: CalloutRSVP, Message: "m", Count: 1})
	if strings.Contains(got, "cta-go") {
		t.Error("rendered an 'Answer now' control with no link behind it")
	}
}

// The single-request case must not show a count chip: "1" beside a sentence
// that already says there is one thing is noise.
func TestRenderCallout_ShowsACountOnlyWhenThereIsMoreThanOne(t *testing.T) {
	one := renderCallout(Callout{Kind: CalloutRSVP, Message: "m", Count: 1, Link: "/x"})
	if strings.Contains(one, "cta-count") {
		t.Error("a single outstanding request rendered a count chip")
	}
	many := renderCallout(Callout{Kind: CalloutRSVP, Message: "m", Count: 3, Link: "/x"})
	if !strings.Contains(many, ">3<") {
		t.Errorf("three outstanding requests did not render the count:\n%s", many)
	}
}

// The accept button ships HIDDEN and is revealed by the widget once it has read
// a real zone from the browser. The server must never render it visible: it
// cannot know the zone, and a visible "Use my timezone" that submits nothing
// would clear the field it claims to set.
func TestRenderCallout_TheTimezoneAcceptShipsHidden(t *testing.T) {
	got := renderCallout(Callout{Kind: CalloutTimezone, Message: "m"})
	if !strings.Contains(got, "data-cta-tz-accept") {
		t.Fatal("the timezone banner has no accept control at all")
	}
	if !strings.Contains(got, "hidden") {
		t.Error("the accept button is rendered visible by the server, which does not know " +
			"the browser's zone — it must stay hidden until the widget fills one in")
	}
	if !strings.Contains(got, `href="/account"`) {
		t.Error("no fallback to /account — a browser that will not report a zone would " +
			"leave the player with a banner and no way to act on it")
	}
}

// Both banners must be dismissible. A shell-level strip with no way to close it
// is a shell-level strip people work around by ignoring the whole region.
func TestRenderCallout_BothKindsCanBeDismissed(t *testing.T) {
	for _, o := range []Callout{
		{Kind: CalloutRSVP, Message: "m", Count: 1, Link: "/x"},
		{Kind: CalloutTimezone, Message: "m"},
	} {
		if !strings.Contains(renderCallout(o), "data-cta-dismiss") {
			t.Errorf("%s has no dismiss control", o.Kind)
		}
	}
}
