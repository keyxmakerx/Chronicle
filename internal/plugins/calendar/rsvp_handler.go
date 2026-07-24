package calendar

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// HTTP surface for calendar-event RSVPs (C-CAL-RSVP-P1). Thin per Chronicle
// rule 3: it resolves + visibility-gates the event, then delegates to
// RSVPService. Its own handler struct keeps the lane disjoint from the calendar
// Handler (and the entity-ties leak fix).

// RSVPHandler processes RSVP HTTP requests.
type RSVPHandler struct {
	svc    RSVPService
	events EventReader // resolve event + calendar for IDOR + visibility gating
}

// NewRSVPHandler constructs the RSVP handler.
func NewRSVPHandler(svc RSVPService, events EventReader) *RSVPHandler {
	return &RSVPHandler{svc: svc, events: events}
}

// requireViewableEvent resolves an event, enforces campaign scoping (IDOR), and
// enforces per-event visibility (canUserView) — returning 404 for both a
// cross-campaign event AND one the viewer may not see, so a hidden event's
// existence never leaks (same leak class the parallel entity-ties lane fixes).
func (h *RSVPHandler) requireViewableEvent(c echo.Context, eventID, campaignID string, role int, userID string) (*Event, *Calendar, error) {
	ctx := c.Request().Context()
	evt, err := h.events.GetEvent(ctx, eventID)
	if err != nil {
		return nil, nil, err
	}
	if evt == nil {
		return nil, nil, apperror.NewNotFound("event not found")
	}
	cal, err := h.events.GetCalendarByID(ctx, evt.CalendarID)
	if err != nil || cal == nil || cal.CampaignID != campaignID {
		return nil, nil, apperror.NewNotFound("event not found")
	}
	if !permissions.CanSeeDmOnly(role) && !canUserView(evt.Visibility, evt.VisibilityRules, role, userID) {
		return nil, nil, apperror.NewNotFound("event not found")
	}
	return evt, cal, nil
}

// UpsertRSVP records the caller's own RSVP and re-renders the in-app panel.
// POST /campaigns/:id/calendars/:calId/events/:eid/rsvp  (RolePlayer)
// Body: status (yes|maybe|no), optional note. Only for events the caller can
// VIEW and only while collection is open.
func (h *RSVPHandler) UpsertRSVP(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	userID := auth.GetUserID(c)
	ctx := c.Request().Context()
	eventID := c.Param("eid")

	evt, cal, err := h.requireViewableEvent(c, eventID, cc.Campaign.ID, cc.VisibilityRole(), userID)
	if err != nil {
		return err
	}
	enabled, err := h.svc.IsCollectionEnabled(ctx, eventID)
	if err != nil {
		return err
	}
	if !enabled {
		return apperror.NewBadRequest("RSVP collection is not open for this event")
	}

	// `action` carries one of the five verbs (yes|maybe|no|out_week|suggest);
	// `status` is accepted as a back-compat alias for the plain three.
	action := c.FormValue("action")
	if action == "" {
		action = c.FormValue("status")
	}
	var note *string
	if n := c.FormValue("note"); n != "" {
		note = &n
	}
	if err := h.svc.ApplyAction(ctx, eventID, userID, action, note); err != nil {
		return err
	}
	return h.renderPanel(c, cc, evt, cal, enabled)
}

// GetRSVPs returns the RSVP summary as JSON: counts + the caller's status for
// everyone; the per-person breakdown ONLY for Owner/co-DM (detail gating,
// mirroring the scheduler overlay).
// GET /campaigns/:id/calendars/:calId/events/:eid/rsvps  (RolePlayer)
func (h *RSVPHandler) GetRSVPs(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	userID := auth.GetUserID(c)
	eventID := c.Param("eid")

	if _, _, err := h.requireViewableEvent(c, eventID, cc.Campaign.ID, cc.VisibilityRole(), userID); err != nil {
		return err
	}
	summary, err := h.svc.GetSummary(c.Request().Context(), eventID, userID, cc.CanControlWorldState())
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, summary)
}

// RSVPPanel renders the in-app RSVP control fragment (buttons + counts +
// facepile; per-person list for Owner/co-DM). Embedded via hx-get on the event
// drawer, quick-view popover, and agenda rows.
// GET /campaigns/:id/calendars/:calId/events/:eid/rsvp-panel  (RolePlayer)
func (h *RSVPHandler) RSVPPanel(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	userID := auth.GetUserID(c)
	eventID := c.Param("eid")

	evt, cal, err := h.requireViewableEvent(c, eventID, cc.Campaign.ID, cc.VisibilityRole(), userID)
	if err != nil {
		return err
	}
	enabled, err := h.svc.IsCollectionEnabled(c.Request().Context(), eventID)
	if err != nil {
		return err
	}
	return h.renderPanel(c, cc, evt, cal, enabled)
}

// ToggleCollection enables or disables RSVP collection for an event. Enabling
// fans out invite emails + rings the bell for viewable members. Scribe+ only.
// POST /campaigns/:id/calendars/:calId/events/:eid/rsvp-collection  (RoleScribe)
// Body: enabled ("true"/"false").
func (h *RSVPHandler) ToggleCollection(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	userID := auth.GetUserID(c)
	ctx := c.Request().Context()
	eventID := c.Param("eid")

	evt, cal, err := h.requireViewableEvent(c, eventID, cc.Campaign.ID, cc.VisibilityRole(), userID)
	if err != nil {
		return err
	}
	enable := c.FormValue("enabled") == "true"
	if enable {
		if err := h.svc.EnableCollection(ctx, eventID); err != nil {
			return err
		}
	} else {
		if err := h.svc.DisableCollection(ctx, eventID); err != nil {
			return err
		}
	}
	return h.renderPanel(c, cc, evt, cal, enable)
}

// renderPanel builds the RSVP summary (detail-gated) and renders the panel
// fragment. Shared by every mutating handler so the in-place hx-swap always
// reflects fresh state.
func (h *RSVPHandler) renderPanel(c echo.Context, cc *campaigns.CampaignContext, evt *Event, cal *Calendar, enabled bool) error {
	userID := auth.GetUserID(c)
	canManage := cc.CanControlWorldState() // Owner or co-DM: per-person detail
	isScribe := cc.MemberRole >= campaigns.RoleScribe
	summary, err := h.svc.GetSummary(c.Request().Context(), evt.ID, userID, canManage)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK,
		RSVPPanel(cc.Campaign.ID, cal.ID, evt.ID, enabled, isScribe, canManage, summary))
}

// --- Public emailed-token flow (no auth; the token is the credential) ---

// RedeemRSVPToken renders the GET-confirm interstitial for an emailed RSVP link.
// GET /calendar-rsvp/:token — pure read (a mail scanner's prefetch must not
// record anything). For the "suggest another time" action it renders a free-text
// form; every other action renders a single confirm button. Both POST back with
// the CSRF double-submit token.
func (h *RSVPHandler) RedeemRSVPToken(c echo.Context) error {
	tokenStr := c.Param("token")
	if tokenStr == "" {
		return c.HTML(http.StatusBadRequest, rsvpTokenResultHTML("Invalid Link", "This RSVP link is invalid.", false))
	}
	token, err := h.svc.ValidateToken(c.Request().Context(), tokenStr)
	if err != nil {
		msg := apperror.UserMessage(err, "This RSVP link is invalid or has expired.")
		return c.HTML(http.StatusOK, rsvpTokenResultHTML("RSVP Failed", msg, false))
	}
	action := "/calendar-rsvp/" + tokenStr
	csrf := middleware.GetCSRFToken(c)
	if token.Action == RSVPActionSuggest {
		return c.HTML(http.StatusOK, rsvpSuggestFormHTML(action, csrf))
	}
	label := rsvpTokenActionLabel(token.Action)
	return c.HTML(http.StatusOK, rsvpTokenConfirmHTML("Confirm Your RSVP",
		"You're responding: "+label+". Tap below to confirm.", action, "Confirm — "+label, csrf))
}

// ApplyRSVPToken consumes the token and records the response.
// POST /calendar-rsvp/:token — the state-changing half (GET/POST split, 0b).
func (h *RSVPHandler) ApplyRSVPToken(c echo.Context) error {
	tokenStr := c.Param("token")
	if tokenStr == "" {
		return c.HTML(http.StatusBadRequest, rsvpTokenResultHTML("Invalid Link", "This RSVP link is invalid.", false))
	}
	var note *string
	if n := c.FormValue("note"); n != "" {
		note = &n
	}
	token, err := h.svc.ApplyToken(c.Request().Context(), tokenStr, note)
	if err != nil {
		msg := apperror.UserMessage(err, "This RSVP link is invalid or has expired.")
		return c.HTML(http.StatusOK, rsvpTokenResultHTML("RSVP Failed", msg, false))
	}
	return c.HTML(http.StatusOK, rsvpTokenResultHTML("Response Recorded",
		rsvpTokenAppliedMessage(token.Action), true))
}
