package sessions

import (
	"fmt"
	"html"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// The player call-to-action endpoint. See callout.go for why this is one
// banner rather than two, and why it is state rather than an event.

// calloutNotificationScan bounds how many of the viewer's newest notifications
// are examined. It is a fuse, not a page size: the banner only needs to know
// whether any unanswered request exists and roughly how many, so reading a
// player's whole notification history to render one sentence would be wasteful.
const calloutNotificationScan = 50

// CalloutAPI renders the player's one outstanding call-to-action, or nothing.
// GET /notifications/call-to-action
//
// An empty body means an empty banner, matching NotificationBadgeAPI's
// contract: the caller is an HTMX poll swapping innerHTML.
//
// Every error path returns empty HTML with 200 rather than failing the poll:
// a banner that cannot be built should read the same as one with nothing to
// say, not as a page-wide error toast.
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

	// storedTZ is the account-level zone. The availability zone is
	// campaign-scoped and this banner is not, so a player who only set that
	// one still sees the ask here; accepting it sets the account zone too.
	zoneSet := h.storedTZ(ctx, userID) != ""

	out := BuildCallout(notes, zoneSet, time.Now().UTC())
	if out.Kind == CalloutNone {
		return c.HTML(http.StatusOK, "")
	}
	if sess := auth.GetSession(c); sess != nil && sess.GuestCampaignID != "" {
		out.NoAccountPage = true
	}
	return c.HTML(http.StatusOK, renderCallout(out))
}

// renderCallout builds the banner fragment.
//
// Every interpolation must be escaped: Link and Message come from a stored
// notification row, and this fragment is assembled with Sprintf.
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
		// The browser-reported zone is filled in by the widget, which replaces
		// [data-cta-zone] once it has read it; the server does not know it.
		choose := `<a class="cta-alt" href="/account">Choose</a>`
		if o.NoAccountPage {
			choose = ""
		}
		return fmt.Sprintf(
			`<div class="cta-bar" data-cta="timezone" role="status">`+
				`<i class="fa-solid fa-clock cta-ico" aria-hidden="true"></i>`+
				`<span class="cta-msg">%s</span>`+
				`<button type="button" class="cta-go" data-cta-tz-accept hidden>`+
				`Use <span data-cta-zone>my timezone</span></button>%s`+
				`<button type="button" class="cta-x" data-cta-dismiss aria-label="Dismiss">`+
				`<i class="fa-solid fa-xmark" aria-hidden="true"></i></button></div>`,
			html.EscapeString(o.Message), choose)
	}
	return ""
}
