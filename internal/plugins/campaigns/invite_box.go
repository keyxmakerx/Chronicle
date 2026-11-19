package campaigns

import (
	"context"
	"github.com/a-h/templ"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// InviteBoxData is everything the People page's "Invite someone" box draws:
// the three ways in, and the one list of whatever is still waiting.
type InviteBoxData struct {
	CampaignID string
	CSRF       string
	Now        time.Time
	// LinkURL is the campaign's live invite link, "" when there is none.
	LinkURL string
	// Invites are the emailed invites not yet accepted.
	Invites []Invite
	// Codes are the guest codes still open, plus the ones in use as a
	// look-around pass, which keep Make player and End now.
	Codes []GuestCode
	// Fresh is a code just made: the only time it is shown.
	Fresh *NewGuestCode
	// JoinURL is the join page a code is typed at.
	JoinURL string
	// Mode is the choice left open: email, link or code.
	Mode string
	// Done is a plain confirmation; Err a refusal.
	Done, Err string
	// Sentence redraws the People page's summary line in place; the box's
	// own changes move its invited count.
	Sentence templ.Component
	// DoneLink is an invite's accept link, shown once when the site can't
	// email it, so the owner can send it themselves.
	DoneLink string
	// EmailReady is false when the site can't send email.
	EmailReady bool
}

// inviteBoxDeps are the Invite box's collaborators, set once at start-up.
type inviteBoxDeps struct {
	invites InviteService
	codes   *GuestCodeService
	baseURL string
}

// SetInviteBox wires the People page's Invite someone box.
func (h *Handler) SetInviteBox(invites InviteService, codes *GuestCodeService, baseURL string) {
	h.box = inviteBoxDeps{invites: invites, codes: codes, baseURL: strings.TrimRight(baseURL, "/")}
}

// registerInviteBoxRoutes adds the box's routes to the owner-only group.
func (h *Handler) registerInviteBoxRoutes(cg *echo.Group) {
	owner := RequireRole(RoleOwner)
	limit := middleware.RateLimit(30, time.Minute)
	cg.GET("/invite-box", h.InviteBox, owner)
	cg.POST("/invite-box/email", h.InviteBoxEmail, owner, limit)
	cg.POST("/invite-box/link", h.InviteBoxMakeLink, owner, limit)
	cg.DELETE("/invite-box/link", h.InviteBoxRevokeLink, owner)
	cg.POST("/invite-box/code", h.InviteBoxMakeCode, owner, limit)
	cg.DELETE("/invite-box/codes/:gcid", h.InviteBoxRemoveCode, owner)
	cg.POST("/invite-box/codes/:gcid/player", h.InviteBoxMakePlayer, owner)
	cg.POST("/invite-box/codes/:gcid/end", h.InviteBoxEndVisit, owner)
	cg.DELETE("/invite-box/invites/:iid", h.InviteBoxRevokeInvite, owner)
}

// boxData gathers the box's state for the campaign.
func (h *Handler) boxData(c *echo.Context, cc *CampaignContext) (InviteBoxData, error) {
	ctx := c.Request().Context()
	d := InviteBoxData{
		CampaignID: cc.Campaign.ID,
		CSRF:       middleware.GetCSRFToken(c),
		Now:        h.box.codes.Now(),
		JoinURL:    h.box.baseURL + "/join",
		Mode:       "email",
		EmailReady: h.smtpChecker != nil && h.smtpChecker.IsConfigured(ctx),
	}
	// The link is read fresh: cc was loaded before any change this request made.
	camp, err := h.service.GetByID(ctx, cc.Campaign.ID)
	if err != nil {
		return d, err
	}
	if camp.JoinCode != nil && *camp.JoinCode != "" {
		d.LinkURL = h.box.baseURL + "/join/" + *camp.JoinCode
	}
	invites, err := h.box.invites.ListInvites(ctx, cc.Campaign.ID)
	if err != nil {
		return d, apperror.NewInternal(err)
	}
	for _, inv := range invites {
		if inv.IsPending() {
			d.Invites = append(d.Invites, inv)
		}
	}
	codes, err := h.box.codes.List(ctx, cc.Campaign.ID)
	if err != nil {
		return d, err
	}
	for _, gc := range codes {
		switch gc.Status(d.Now) {
		case "open", "visiting", "visit_ended":
			d.Codes = append(d.Codes, gc)
		}
	}
	return d, nil
}

// boxResponse draws the box, applying fill to its data first.
func (h *Handler) boxResponse(c *echo.Context, fill func(*InviteBoxData)) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	d, err := h.boxData(c, cc)
	if err != nil {
		return err
	}
	if fill != nil {
		fill(&d)
	}
	if c.Request().Method != http.MethodGet {
		d.Sentence = h.manageSentenceSwap("people")
	}
	return middleware.Render(c, http.StatusOK, InviteBox(d))
}

func boxError(mode string, err error, fallback string) func(*InviteBoxData) {
	return func(d *InviteBoxData) { d.Mode, d.Err = mode, apperror.UserMessage(err, fallback) }
}

// InviteBox draws the box (GET /campaigns/:id/invite-box).
func (h *Handler) InviteBox(c *echo.Context) error { return h.boxResponse(c, nil) }

// InviteBoxEmail invites by email (POST /campaigns/:id/invite-box/email).
// Nobody joins without accepting, and the answer is the same whether or not
// the address has an account, so the owner learns nothing about who has one.
func (h *Handler) InviteBoxEmail(c *echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	ctx := c.Request().Context()
	var req AddMemberRequest
	if err := c.Bind(&req); err != nil {
		return h.boxResponse(c, boxError("email", apperror.NewBadRequest("invalid request"), ""))
	}
	email := strings.TrimSpace(req.Email)
	role := strings.ToLower(strings.TrimSpace(req.Role))
	if role == "" {
		role = "player"
	}
	inv, err := h.box.invites.CreateInvite(ctx, cc.Campaign.ID, auth.GetUserID(c), CreateInviteInput(req))
	if err != nil {
		return h.boxResponse(c, boxError("email", err, "the invite couldn't be sent"))
	}
	h.logAudit(c, cc.Campaign.ID, "member.invited", map[string]any{"invite_id": inv.ID, "role": role})
	return h.boxResponse(c, func(d *InviteBoxData) {
		if d.EmailReady {
			d.Done = "Invite sent to " + email + "."
		} else {
			d.Done = "Invite made for " + email + ", but this site can't send email. Send them this link yourself; they join when they accept."
			d.DoneLink = h.box.baseURL + "/invites/accept?token=" + inv.Token
		}
	})
}

// InviteBoxMakeLink makes the campaign's invite link, replacing any earlier
// one (POST /campaigns/:id/invite-box/link).
func (h *Handler) InviteBoxMakeLink(c *echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if _, err := h.service.GenerateJoinCode(c.Request().Context(), cc.Campaign.ID); err != nil {
		return h.boxResponse(c, boxError("link", err, "the link couldn't be made"))
	}
	h.logAudit(c, cc.Campaign.ID, "campaign.join_code.generated", nil)
	return h.boxResponse(c, func(d *InviteBoxData) { d.Mode = "link" })
}

// InviteBoxRevokeLink ends the invite link (DELETE /campaigns/:id/invite-box/link).
func (h *Handler) InviteBoxRevokeLink(c *echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if err := h.service.RevokeJoinCode(c.Request().Context(), cc.Campaign.ID); err != nil {
		return h.boxResponse(c, boxError("link", err, "the link couldn't be ended"))
	}
	h.logAudit(c, cc.Campaign.ID, "campaign.join_code.revoked", nil)
	return h.boxResponse(c, func(d *InviteBoxData) { d.Mode = "link" })
}

// InviteBoxMakeCode makes a guest code and shows it once
// (POST /campaigns/:id/invite-box/code).
func (h *Handler) InviteBoxMakeCode(c *echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	days := 0
	if c.FormValue("kind") == "visit" {
		n, err := strconv.Atoi(c.FormValue("visit_days"))
		if err != nil {
			return h.boxResponse(c, boxError("code", apperror.NewBadRequest("pick 1, 3 or 7 days to look around"), ""))
		}
		days = n
	}
	fresh, err := h.box.codes.Create(c.Request().Context(), cc.Campaign.ID, auth.GetUserID(c), c.FormValue("note"), days)
	if err != nil {
		return h.boxResponse(c, boxError("code", err, "the code couldn't be made"))
	}
	return h.boxResponse(c, func(d *InviteBoxData) { d.Mode, d.Fresh = "code", fresh })
}

// InviteBoxRemoveCode removes a code nobody used
// (DELETE /campaigns/:id/invite-box/codes/:gcid).
func (h *Handler) InviteBoxRemoveCode(c *echo.Context) error {
	return h.codeAction(c, "the code couldn't be removed", h.box.codes.Remove)
}

// InviteBoxMakePlayer turns a visitor into a player
// (POST /campaigns/:id/invite-box/codes/:gcid/player).
func (h *Handler) InviteBoxMakePlayer(c *echo.Context) error {
	return h.codeAction(c, "they couldn't be made a player", h.box.codes.MakeVisitorPlayer)
}

// InviteBoxEndVisit ends a look-around now
// (POST /campaigns/:id/invite-box/codes/:gcid/end).
func (h *Handler) InviteBoxEndVisit(c *echo.Context) error {
	return h.codeAction(c, "the visit couldn't be ended", h.box.codes.EndVisit)
}

func (h *Handler) codeAction(c *echo.Context, fallback string, act func(ctx context.Context, campaignID, id string) error) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if err := act(c.Request().Context(), cc.Campaign.ID, c.Param("gcid")); err != nil {
		return h.boxResponse(c, boxError("code", err, fallback))
	}
	return h.boxResponse(c, func(d *InviteBoxData) { d.Mode = "code" })
}

// InviteBoxRevokeInvite withdraws an emailed invite
// (DELETE /campaigns/:id/invite-box/invites/:iid).
func (h *Handler) InviteBoxRevokeInvite(c *echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if err := h.box.invites.RevokeInvite(c.Request().Context(), cc.Campaign.ID, c.Param("iid")); err != nil {
		return h.boxResponse(c, boxError("email", err, "the invite couldn't be withdrawn"))
	}
	return h.boxResponse(c, nil)
}
