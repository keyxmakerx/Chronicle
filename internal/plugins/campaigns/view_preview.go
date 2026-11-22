package campaigns

// view_preview.go is the DM team's "look as someone else" switch: the player
// preview (a display cookie, see ViewingAsPlayer) and the two guest previews,
// a visitor with no code and a visitor on a guest code (a middleware
// downgrade, see AllowPublicCampaignAccess, ADR-074 and ADR-076). All are
// session cookies, only one is on at a time, and none can grant anything:
// the cookies are honoured only for the owner and co-DMs, and a preview can
// only lower what a request may see.

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
)

const (
	// viewAsPlayerCookie is the cookie name for the "view as player" display
	// preview.
	viewAsPlayerCookie = "chronicle_view_as_player"
	// viewAsGuestCookie is the cookie name for the guest preview. Server-only
	// (HttpOnly): no script needs it, the page reads the preview from the
	// server-rendered chrome.
	viewAsGuestCookie = "chronicle_view_as_guest"
	// viewAsGuestCodeCookie is the cookie for the guest-code preview, the
	// visitor who came in on a guest code. Server-only, like the guest one.
	viewAsGuestCodeCookie = "chronicle_view_as_guestcode"
)

// ViewingAsPlayer reports whether the owner or a co-DM has "view as player"
// on in this campaign. The cookie is site-wide but its value is the id of the
// campaign it was turned on for, so a preview started in one campaign is not
// honoured in another. Handlers read it themselves because the layout context
// that also carries it is only filled in when a page renders. Display only:
// never use it to grant.
func ViewingAsPlayer(c *echo.Context, cc *CampaignContext) bool {
	if cc == nil || !cc.CanAuthorDmOnly() || cc.Campaign == nil {
		return false
	}
	return previewCookieFor(c, viewAsPlayerCookie, cc.Campaign.ID)
}

// ViewingAsGuest reports whether the REAL owner or co-DM has the guest cookie
// on. It is the request, not the effect: on a private campaign guests cannot
// get in, so no downgrade happens there. A downgraded guest-preview context
// is not the DM team any more, so it answers false; read cc.Preview for that.
// Display only: never use it to grant.
func ViewingAsGuest(c *echo.Context, cc *CampaignContext) bool {
	return cc != nil && cc.CanAuthorDmOnly() && cc.Campaign != nil && previewCookieFor(c, viewAsGuestCookie, cc.Campaign.ID)
}

// viewingAsGuestCode reports whether the REAL owner or co-DM has the
// guest-code cookie on. A pass opens a private campaign too, so unlike the
// plain guest preview this one has an effect everywhere.
func viewingAsGuestCode(c *echo.Context, cc *CampaignContext) bool {
	return cc != nil && cc.CanAuthorDmOnly() && cc.Campaign != nil && previewCookieFor(c, viewAsGuestCodeCookie, cc.Campaign.ID)
}

// homeGuestsNoteParam and homeGuestsNoteValue are the query that asks a
// private campaign's Home for its "guests can't open it" note. A private
// campaign has no guest view to switch on, so asking for one leaves no cookie
// behind: one could only take effect later and unasked, on another campaign
// or once this one is made public.
const (
	homeGuestsNoteParam = "dashboard"
	homeGuestsNoteValue = "guests"
)

// GuestsNoteAsked reports whether this Home request is the DM team asking for
// the guests' dashboard of a private campaign. A player preview started from
// that address wins, so the page never claims two views at once. Display only.
func GuestsNoteAsked(c *echo.Context, cc *CampaignContext) bool {
	return cc != nil && cc.CanAuthorDmOnly() && !cc.Campaign.IsPublic &&
		c.QueryParam(homeGuestsNoteParam) == homeGuestsNoteValue &&
		!ViewingAsPlayer(c, cc)
}

// previewCookieFor reports whether the named preview cookie was turned on for
// this campaign.
func previewCookieFor(c *echo.Context, name, campaignID string) bool {
	cookie, err := c.Cookie(name)
	return err == nil && campaignID != "" && cookie.Value == campaignID
}

// isReadMethod reports whether the guest downgrade applies: only requests that
// read. A write always runs as the real person, so leaving the preview and
// anything the owner does is never lowered or refused.
func isReadMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead
}

// guestPreviewMode decides, from the REAL context of a request, whether it
// is lowered to a visitor's and which one: "" for no preview, PreviewGuest or
// PreviewGuestCode. It needs the DM team's own request and a read. The plain
// guest preview needs a public campaign too (a private one admits no guest
// without a code, so there is nothing to look at); a guest code opens any
// campaign. The cookies from anyone else have no effect at all.
func guestPreviewMode(c *echo.Context, real *CampaignContext) string {
	if !real.IsMember || !real.CanAuthorDmOnly() || !isReadMethod(c.Request().Method) {
		return ""
	}
	switch {
	case previewCookieFor(c, viewAsGuestCodeCookie, real.Campaign.ID):
		return PreviewGuestCode
	case real.Campaign.IsPublic && previewCookieFor(c, viewAsGuestCookie, real.Campaign.ID):
		return PreviewGuest
	}
	return ""
}

// guestPreviewActive reports whether either guest preview lowers this request.
func guestPreviewActive(c *echo.Context, real *CampaignContext) bool {
	return guestPreviewMode(c, real) != ""
}

// lowerForGuestPreview is the visitor's context for a request a guest
// preview lowers, or real unchanged when none applies.
func lowerForGuestPreview(c *echo.Context, real *CampaignContext) *CampaignContext {
	switch guestPreviewMode(c, real) {
	case PreviewGuestCode:
		return real.GuestCodePreviewContext(time.Now())
	case PreviewGuest:
		return real.GuestPreviewContext()
	}
	return real
}

// GuestPreviewAsked is the cheap first test for a door that does not run
// AllowPublicCampaignAccess: a read carrying a guest cookie. Whether it has
// any effect, and for which campaign, is GuestPreviewLowers' answer.
func GuestPreviewAsked(c *echo.Context) bool {
	if !isReadMethod(c.Request().Method) {
		return false
	}
	_, err := c.Cookie(viewAsGuestCookie)
	_, errCode := c.Cookie(viewAsGuestCodeCookie)
	return err == nil || errCode == nil
}

// GuestPreviewLowers reports whether the guest preview lowers this member's
// request to a visitor's, by the same rule as the web middleware. It is for
// the REST API's session door, which resolves the member itself: the preview
// must show a visitor's view on every door a browser session opens, and a
// visitor has no session door at all.
func GuestPreviewLowers(c *echo.Context, campaign *Campaign, member *CampaignMember) bool {
	if campaign == nil || member == nil {
		return false
	}
	return guestPreviewActive(c, &CampaignContext{
		Campaign:    campaign,
		MemberRole:  member.Role,
		IsMember:    true,
		IsDmGranted: hasDmGrant(campaign, member.UserID),
	})
}

// refuseDuringGuestPreview stops a read of a members-only route while the
// guest preview is on, the way a visitor who is not a member would be
// stopped. A page navigation gets a small page with a way back; a fragment or
// API read just gets the 403. handled is false when no preview applies and the
// request goes on; otherwise the response is already written (err nil) or err
// is the refusal.
func refuseDuringGuestPreview(c *echo.Context, real *CampaignContext) (handled bool, err error) {
	if !guestPreviewActive(c, real) {
		return false, nil
	}
	if middleware.IsHTMX(c) || !strings.Contains(c.Request().Header.Get("Accept"), "text/html") {
		return true, apperror.NewForbidden("a guest can't open this; you are viewing this campaign as a guest")
	}
	// The page is drawn as the visitor, with the preview's chrome, so the
	// way back is in the same place as everywhere else.
	c.Set(contextKeyCampaign, lowerForGuestPreview(c, real))
	return true, middleware.Render(c, http.StatusForbidden, GuestPreviewRefusalPage(real.Campaign.ID, middleware.GetCSRFToken(c)))
}

// writePreviewCookie turns one preview cookie on or off. A session cookie, so
// closing the browser ends the preview and an owner never comes back days
// later to a campaign that looks like it lost its edit buttons. The value is
// the campaign's id. Clearing is only sent when the request carries the
// cookie for this campaign, so turning it off here leaves another campaign's
// preview alone.
func writePreviewCookie(c *echo.Context, name, campaignID string, on, httpOnly bool) {
	req := c.Request()
	secure := req.TLS != nil || req.Header.Get("X-Forwarded-Proto") == "https"
	if on {
		c.SetCookie(&http.Cookie{
			Name:     name,
			Value:    campaignID,
			Path:     "/",
			HttpOnly: httpOnly,
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
		})
		return
	}
	if ck, err := c.Cookie(name); err != nil || ck.Value != campaignID {
		return
	}
	c.SetCookie(&http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		HttpOnly: httpOnly,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// setPreview puts the request's cookies in the state "as" names. One preview
// at a time: entering one clears the other.
func setPreview(c *echo.Context, campaignID, as string) {
	// The player cookie keeps its long-standing shape (readable by script).
	writePreviewCookie(c, viewAsPlayerCookie, campaignID, as == PreviewPlayer, false)
	writePreviewCookie(c, viewAsGuestCookie, campaignID, as == PreviewGuest, true)
	writePreviewCookie(c, viewAsGuestCodeCookie, campaignID, as == PreviewGuestCode, true)
}

// anyPreviewOn reports whether the eye would show as on for this DM-team
// request: the player preview, or the guest preview where it has an effect.
// On a private campaign a guest cookie left by another campaign changes
// nothing, so it does not count; the eye starts the player preview there.
func anyPreviewOn(c *echo.Context, cc *CampaignContext) bool {
	return ViewingAsPlayer(c, cc) || (cc.Campaign.IsPublic && ViewingAsGuest(c, cc)) || viewingAsGuestCode(c, cc)
}

// ToggleViewAsPlayer is the header eye (POST /campaigns/:id/toggle-view-mode)
// for the owner and co-DMs: any preview on goes off, otherwise the player
// preview starts. Display only: access control still uses the real role.
func (h *Handler) ToggleViewAsPlayer(c *echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if anyPreviewOn(c, cc) {
		setPreview(c, cc.Campaign.ID, "")
	} else {
		setPreview(c, cc.Campaign.ID, PreviewPlayer)
	}
	return previewRefresh(c, cc)
}

// viewAsRequest is the body of POST /campaigns/:id/view-as. Next and Home are
// enums, never addresses, so the endpoint cannot be used to redirect anywhere.
type viewAsRequest struct {
	As   string `json:"as" form:"as"`     // self, player, guest or guestcode.
	Next string `json:"next" form:"next"` // "", home or arrange.
	Home string `json:"home" form:"home"` // player or guest; only with next=arrange.
}

// ViewAs is the one preview switch (POST /campaigns/:id/view-as), for the
// owner and co-DMs. "self" ends any preview. With next=home it lands on Home;
// with next=arrange it first ends the preview, because the Customizer is a
// members' page a guest cannot open, then opens the owner's editor for that
// audience's dashboard.
func (h *Handler) ViewAs(c *echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	var req viewAsRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	switch req.As {
	case "self", PreviewPlayer, PreviewGuest, PreviewGuestCode:
	default:
		return apperror.NewBadRequest("as must be self, player, guest or guestcode")
	}
	base := "/campaigns/" + cc.Campaign.ID
	to := ""
	switch req.Next {
	case "":
		if req.Home != "" {
			return apperror.NewBadRequest("home goes only with next=arrange")
		}
	case "home":
		if req.Home != "" {
			return apperror.NewBadRequest("home goes only with next=arrange")
		}
		to = base
	case "arrange":
		if req.As != "self" || (req.Home != PreviewPlayer && req.Home != PreviewGuest) {
			return apperror.NewBadRequest("arrange needs as=self and home=player or guest")
		}
		// The Customizer is the owner's; a co-DM is told so rather than sent
		// to a 403 page.
		if cc.MemberRole < RoleOwner {
			return apperror.NewForbidden("only the campaign owner can arrange a dashboard")
		}
		to = base + "/customize/home/" + req.Home
	default:
		return apperror.NewBadRequest("next must be home or arrange")
	}

	switch {
	case req.As == "self":
		setPreview(c, cc.Campaign.ID, "")
	case req.As == PreviewGuest && !cc.Campaign.IsPublic:
		// Nothing to switch on: Home says why, once, and no cookie is left.
		setPreview(c, cc.Campaign.ID, "")
		return middleware.HTMXRedirect(c, base+"?"+homeGuestsNoteParam+"="+homeGuestsNoteValue)
	default:
		setPreview(c, cc.Campaign.ID, req.As)
	}
	if to != "" {
		return middleware.HTMXRedirect(c, to)
	}
	return previewRefresh(c, cc)
}

// previewRefresh ends a preview change with the page the person was on:
// HTMX does a full refresh so everything re-renders, anything else goes back
// to a page of this campaign, or Home.
func previewRefresh(c *echo.Context, cc *CampaignContext) error {
	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Refresh", "true")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, sameCampaignReferer(c.Request(), cc.Campaign.ID))
}

// sameCampaignReferer is the Referer's path and query when it is a page of
// this campaign on this host, and the campaign's Home otherwise, so a forged
// Referer cannot send anyone elsewhere.
func sameCampaignReferer(r *http.Request, campaignID string) string {
	home := "/campaigns/" + campaignID
	u, err := url.Parse(r.Header.Get("Referer"))
	if err != nil || u.Host != r.Host {
		return home
	}
	if u.Path != home && !strings.HasPrefix(u.Path, home+"/") {
		return home
	}
	return u.RequestURI()
}

// boolWord is the "true"/"false" an aria-pressed attribute needs.
func boolWord(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
