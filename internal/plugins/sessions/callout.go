package sessions

import (
	"strings"
	"time"
)

// The player call-to-action (C-RSVP-P10).
//
// TWO ASKS, ONE MOMENT. The operator asked for two things — that the product
// ASK a player for their timezone rather than silently guessing, and that
// players see something on the page when an RSVP opens. Both are the same
// question from the player's side ("is there something I owe the table?"), both
// want the same answer surface, and building them as two mechanisms would mean
// two polls, two banners that can stack, and two places to get the dismissal
// wrong. So this is one decision, made once, server-side.
//
// WHY SERVER-SIDE AND POLLED, rather than pushed at the moment RSVP is armed:
// a player is almost never looking at the page in the second the GM arms it. A
// fire-once toast would be seen by nobody and would then be gone. What a player
// needs is the STATE — "there is an unanswered request" — which survives a
// reload, and which stops being shown the moment they answer. This mirrors the
// bell badge exactly (notifications_handler.go), including returning nothing at
// all when there is nothing to say.

// CalloutKind names what the banner is asking for. The zero value is "nothing",
// so a caller that ignores the error still shows nothing rather than something
// wrong.
type CalloutKind string

const (
	// CalloutNone is "say nothing". It is the zero value on purpose.
	CalloutNone CalloutKind = ""
	// CalloutRSVP is an unanswered scheduling or RSVP request.
	CalloutRSVP CalloutKind = "rsvp"
	// CalloutTimezone is "we do not know your timezone and are about to guess".
	CalloutTimezone CalloutKind = "timezone"
)

// Callout is what the banner should say, already decided.
type Callout struct {
	Kind CalloutKind
	// Message is the human sentence. Empty when Kind is CalloutNone.
	Message string
	// Link is where the primary action goes, for CalloutRSVP. Empty otherwise —
	// the timezone callout acts in place rather than navigating.
	Link string
	// Count is how many unanswered requests there are, for CalloutRSVP.
	Count int
}

// calloutNotifTypes are the notification types that represent something a
// player is expected to ANSWER, as opposed to something they are merely being
// told.
//
// The distinction is the whole point of the banner. NotifProposalConfirmed
// ("the time is settled") and NotifProposalResponse ("somebody replied to your
// proposal") are news; raising a persistent banner for them would train players
// to dismiss the banner unread, and the one that actually needs an answer would
// go with it. NotifAvailabilityNudge is deliberately NOT here either: the
// Director already chose to send that one to the bell, and promoting their
// nudge into a page-wide banner would be this code overriding their judgement
// about how loud to be.
var calloutNotifTypes = map[string]bool{
	NotifProposalCreated: true,
	NotifCalendarRSVP:    true,
}

// BuildCallout decides what one player should be shown, from their unread
// notifications and their stored account timezone.
//
// PRECEDENCE IS DELIBERATE AND IS NOT "most recent wins". An unanswered RSVP
// outranks the timezone ask because it is time-limited — a session gets
// scheduled with or without that player, and the window closes. A missing
// timezone is wrong every day but wrong in a way that keeps until tomorrow.
// Showing both at once was considered and rejected: two stacked banners above
// the page content is how a shell starts eating the screen, and the second one
// gets dismissed by reflex along with the first.
//
// zoneSet reports whether the account carries an IANA zone. It is passed rather
// than read here so this stays pure and so the caller owns the auth seam.
func BuildCallout(unread []Notification, zoneSet bool, now time.Time) Callout {
	pending := 0
	link := ""
	for _, n := range unread {
		// ReadAt is the truth, not a derived flag: the store records WHEN a row
		// was read and leaves it nil otherwise, so nil is unread.
		if n.ReadAt != nil || !calloutNotifTypes[n.Type] {
			continue
		}
		pending++
		// The FIRST unanswered one wins the link, and the list arrives newest
		// first, so the link points at the most recent request. With several
		// outstanding, the banner says how many and sends them to the newest —
		// answering it surfaces the next on the following poll.
		if link == "" && n.Link != nil && strings.TrimSpace(*n.Link) != "" {
			link = *n.Link
		}
	}

	if pending > 0 {
		msg := "Your table is waiting on your answer"
		if pending > 1 {
			msg = "Your table is waiting on your answers"
		}
		return Callout{Kind: CalloutRSVP, Message: msg, Link: link, Count: pending}
	}

	if !zoneSet {
		return Callout{
			Kind: CalloutTimezone,
			// The sentence states the CONSEQUENCE, not the chore. "Set your
			// timezone" is a task; "your times are being shown in UTC" is the
			// reason anyone would do it.
			Message: "Chronicle does not know your timezone, so it is showing you times in UTC",
		}
	}

	return Callout{Kind: CalloutNone}
}
