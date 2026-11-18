// guest_code_labels.go keeps the wording helpers for guest codes; the codes
// themselves are made and listed in the Invite someone box (invite_box.templ).

package campaigns

import (
	"fmt"
	"math"
	"net/url"
	"time"
)

// visitDaysLabel is "1 day" or "3 days".
func visitDaysLabel(days int) string {
	if days == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", days)
}

// visitRemaining is the owner's view of a running pass: days left, or hours
// on its last day, rounded up.
func visitRemaining(until, now time.Time) string {
	left := until.Sub(now)
	if left < 24*time.Hour {
		h := int(math.Ceil(left.Hours()))
		if h <= 1 {
			return "under an hour left"
		}
		return fmt.Sprintf("%d hours left", h)
	}
	return visitDaysLabel(int(math.Ceil(left.Hours()/24))) + " left"
}

// guestInviteLink is the join page with the code filled in, so the person
// it is sent to taps one link and types only a name.
func guestInviteLink(joinURL, code string) string {
	return joinURL + "?code=" + url.QueryEscape(code)
}

// guestCodeCopyData is the Alpine state for the fresh code's copy buttons.
// The values go through %q so they are JavaScript string literals.
func guestCodeCopyData(code, link string) string {
	return fmt.Sprintf(`{ code: %q, link: %q, copy(text, done) { navigator.clipboard.writeText(text).then(() => Chronicle.notify(done, 'success')); } }`, code, link)
}
