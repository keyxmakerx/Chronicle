package sessions

import (
	"strings"
	"time"
)

// The player call-to-action banner.
//
// It unifies two asks — an unanswered RSVP, and an unset timezone — into one
// decision made once, server-side, rather than two banners that could stack.
// It is server-side and polled rather than pushed at the moment an RSVP
// opens, because a player is rarely looking at the page in that instant; what
// they need is STATE ("there is an unanswered request") that survives a
// reload and stops the moment they answer, mirroring the bell badge
// (notifications_handler.go), including returning nothing when there is
// nothing to say.

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
// player is expected to ANSWER, as opposed to news (NotifProposalConfirmed,
// NotifProposalResponse) or a Director-directed nudge
// (NotifAvailabilityNudge) that stays in the bell. Raising a persistent
// banner for every notification type would train players to dismiss it
// unread, burying the one that actually needs an answer.
var calloutNotifTypes = map[string]bool{
	NotifProposalCreated: true,
}

// BuildCallout decides what one player should be shown, from their unread
// notifications and their stored account timezone.
//
// An unanswered RSVP always outranks the timezone ask, never "most recent
// wins": the RSVP is time-limited (a session gets scheduled with or without
// that player), while a missing timezone is wrong every day but keeps until
// tomorrow. The two are never shown together, since stacked banners crowd
// the page and train a reflex dismissal that loses both.
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
