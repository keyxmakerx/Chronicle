package sessions

import (
	"fmt"
	"html"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// The player call-to-action endpoint (C-RSVP-P10). See callout.go for WHY this
// is one banner rather than two, and why it is state rather than an event.

// calloutNotificationScan bounds how many of the viewer's newest notifications
// are examined. It is a FUSE, not a page size: the banner only needs to know
// whether ANY unanswered request exists and roughly how many, and a player with
// hundreds of unread rows is a player for whom "several" is the honest answer.
// Reading their whole history on a 60-second poll to render one sentence would
// be the expensive way to say the same thing.
const calloutNotificationScan = 50

// CalloutAPI renders the player's one outstanding call-to-action, or nothing.
// GET /notifications/call-to-action
//
// EMPTY BODY MEANS EMPTY BANNER, which is the bell badge's contract
// (NotificationBadgeAPI) and is reused deliberately: the caller is an HTMX poll
// swapping innerHTML, so "nothing to say" has to be renderable, and an empty
// string is the only response that renders as genuinely nothing.
//
// It NEVER fails the poll. Every error path returns empty HTML with 200,
// because a banner that cannot be built is indistinguishable, to the player,
// from a banner with nothing to say — and a 500 here would put an error toast
// on every page of the product once a minute.
func (h *Handler) CalloutAPI(c echo.Context) error {
	ctx := c.Request().Context()
	userID := auth.GetUserID(c)
	if userID == "" {
		return c.HTML(http.StatusOK, "")
	}

	notes, err := h.svc.ListMyNotifications(ctx, userID, calloutNotificationScan)
	if err != nil {
		return c.HTML(http.StatusOK, "")
	}

	// The zone question is "has this player told us anywhere", not "is it on
	// the account row" — memberZone's own reason for existing (the Bench told
	// players who HAD set a zone that they had not). storedTZ is the account
	// half; the availability half is campaign-scoped and this banner is not, so
	// the account is the only zone that can be answered from here. A player who
	// set only the availability zone still sees the ask, and accepting it makes
	// the two agree — which is the outcome the split defect wants anyway.
	zoneSet := h.storedTZ(ctx, userID) != ""

	out := BuildCallout(notes, zoneSet, time.Now().UTC())
	if out.Kind == CalloutNone {
		return c.HTML(http.StatusOK, "")
	}
	return c.HTML(http.StatusOK, renderCallout(out))
}

// renderCallout builds the banner fragment.
//
// EVERY INTERPOLATION IS ESCAPED. The strings here are ours today, but Link
// arrives from a stored notification row, and a fragment assembled with
// Sprintf is exactly where a stored value becomes markup. html.EscapeString on
// the way in costs nothing and removes the question.
func renderCallout(o Callout) string {
	switch o.Kind {
	case CalloutRSVP:
		count := ""
		if o.Count > 1 {
			count = fmt.Sprintf(`<span class="cta-count">%d</span>`, o.Count)
		}
		action := ""
		if o.Link != "" {
			action = fmt.Sprintf(`<a class="cta-go" href="%s">Answer now</a>`, html.EscapeString(o.Link))
		}
		return fmt.Sprintf(
			`<div class="cta-bar" data-cta="rsvp" role="status">`+
				`<i class="fa-solid fa-hourglass-half cta-ico" aria-hidden="true"></i>`+
				`<span class="cta-msg">%s</span>%s%s`+
				`<button type="button" class="cta-x" data-cta-dismiss aria-label="Dismiss">`+
				`<i class="fa-solid fa-xmark" aria-hidden="true"></i></button></div>`,
			html.EscapeString(o.Message), count, action)

	case CalloutTimezone:
		// The zone the BROWSER reports is filled in by the widget, not here —
		// the server genuinely does not know it, and rendering a guess into the
		// sentence would be the product asserting something it cannot see. The
		// widget replaces [data-cta-zone] once it has read the real value, and
		// the fallback text below is what shows if it never does.
		return fmt.Sprintf(
			`<div class="cta-bar" data-cta="timezone" role="status">`+
				`<i class="fa-solid fa-clock cta-ico" aria-hidden="true"></i>`+
				`<span class="cta-msg">%s</span>`+
				`<button type="button" class="cta-go" data-cta-tz-accept hidden>`+
				`Use <span data-cta-zone>my timezone</span></button>`+
				`<a class="cta-alt" href="/account">Choose</a>`+
				`<button type="button" class="cta-x" data-cta-dismiss aria-label="Dismiss">`+
				`<i class="fa-solid fa-xmark" aria-hidden="true"></i></button></div>`,
			html.EscapeString(o.Message))
	}
	return ""
}
