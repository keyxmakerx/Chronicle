package sessions

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Slot-proposal HTTP surface (C-SCHED-P2). Handlers stay thin: bind, call the
// service, render. Notification + email fan-out is coordination (enumerate
// members, call the service's notify methods, send email) — it lives here like
// the RSVP email path, not in the service.

// ListProposals renders the proposals list for a campaign.
// GET /campaigns/:id/proposals
func (h *Handler) ListProposals(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	userID := auth.GetUserID(c)
	canManage := cc.MemberRole >= campaigns.RoleScribe
	summaries, err := h.svc.ListProposalSummaries(c.Request().Context(), cc.Campaign.ID, userID)
	if err != nil {
		return c.JSON(apperror.SafeCode(err), map[string]string{"error": apperror.SafeMessage(err)})
	}
	viewerTZ := h.resolveViewerTZ(c, userID)
	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, ProposalListFragment(cc, summaries, viewerTZ, canManage))
	}
	return middleware.Render(c, http.StatusOK, ProposalListPage(cc, summaries, viewerTZ, canManage))
}

// ShowProposal renders one proposal's detail: options in the viewer's zone, the
// viewer's own responses, tallies, and (owner only) the per-responder breakdown.
// GET /campaigns/:id/proposals/:pid
func (h *Handler) ShowProposal(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	userID := auth.GetUserID(c)
	viewerTZ := h.resolveViewerTZ(c, userID)
	includeDetail := cc.MemberRole >= campaigns.RoleOwner || cc.IsDmGranted

	view, err := h.svc.GetProposalView(c.Request().Context(), cc.Campaign.ID, c.Param("pid"), userID, viewerTZ, includeDetail)
	if err != nil {
		return c.JSON(apperror.SafeCode(err), map[string]string{"error": apperror.SafeMessage(err)})
	}
	if includeDetail {
		h.fillResponderNames(c.Request().Context(), cc.Campaign.ID, view)
	}
	csrf := middleware.GetCSRFToken(c)
	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, ProposalDetailFragment(cc, view, csrf, userID))
	}
	return middleware.Render(c, http.StatusOK, ProposalDetailPage(cc, view, csrf, userID))
}

// CreateProposalAPI creates a proposal from the DM slot builder, then fans out
// notifications + email invites to members.
// POST /campaigns/:id/proposals
func (h *Handler) CreateProposalAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	userID := auth.GetUserID(c)
	var req CreateProposalRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}
	proposal, err := h.svc.CreateProposal(c.Request().Context(), cc.Campaign.ID, userID, req)
	if err != nil {
		return c.JSON(apperror.SafeCode(err), map[string]string{"error": apperror.SafeMessage(err)})
	}

	// Fan out to members (notifications + email) off the request path.
	if h.memberLister != nil {
		if members, mErr := h.memberLister.ListMembers(c.Request().Context(), cc.Campaign.ID); mErr == nil {
			go h.fanoutProposalCreated(context.Background(), cc.Campaign.ID, cc.Campaign.Name, proposal, userID, members)
		}
	}

	return c.JSON(http.StatusOK, map[string]string{
		"status": "ok",
		"id":     proposal.ID,
		"link":   fmt.Sprintf("/campaigns/%s/proposals/%s", cc.Campaign.ID, proposal.ID),
	})
}

// RespondOptionAPI records a member's response to one option, then notifies the
// proposer. Returns the refreshed detail fragment so the card re-renders in place.
// POST /campaigns/:id/proposals/:pid/options/:oid/respond
func (h *Handler) RespondOptionAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	userID := auth.GetUserID(c)
	proposalID := c.Param("pid")
	optionID := c.Param("oid")
	var req RespondRequest
	// Bind from JSON (proposals API) or form (HTMX hx-vals on the response card).
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}
	if err := h.svc.RespondToOption(c.Request().Context(), cc.Campaign.ID, proposalID, optionID, userID, req.Response); err != nil {
		return c.JSON(apperror.SafeCode(err), map[string]string{"error": apperror.SafeMessage(err)})
	}

	// Notify the proposer (best-effort, off the request path).
	responderName := h.resolveDisplayName(c.Request().Context(), userID)
	go func() {
		if err := h.svc.NotifyProposalResponse(context.Background(), cc.Campaign.ID, proposalID, responderName, req.Response); err != nil {
			slog.Warn("failed to write response notification", slog.Any("error", err))
		}
	}()

	// HTMX response cards swap the refreshed proposal in place; JSON callers get
	// a status.
	if middleware.IsHTMX(c) {
		viewerTZ := h.resolveViewerTZ(c, userID)
		includeDetail := cc.MemberRole >= campaigns.RoleOwner || cc.IsDmGranted
		view, err := h.svc.GetProposalView(c.Request().Context(), cc.Campaign.ID, proposalID, userID, viewerTZ, includeDetail)
		if err != nil {
			return c.JSON(apperror.SafeCode(err), map[string]string{"error": apperror.SafeMessage(err)})
		}
		if includeDetail {
			h.fillResponderNames(c.Request().Context(), cc.Campaign.ID, view)
		}
		return middleware.Render(c, http.StatusOK, ProposalDetailFragment(cc, view, middleware.GetCSRFToken(c), userID))
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// RedeemProposalToken renders the confirm interstitial for a one-click response
// token. GET /proposals/respond/:token — no auth, the token is the credential.
// GET is a pure READ (0b): it validates the token, rechecks the proposal is still
// open (0a, in the service) + the user is still a member (0a, below), then shows a
// POST form. ApplyProposalToken records the response, so a mail prefetcher's GET
// never writes.
func (h *Handler) RedeemProposalToken(c echo.Context) error {
	tokenStr := c.Param("token")
	if tokenStr == "" {
		return c.HTML(http.StatusBadRequest, rsvpResultHTML("Invalid Link", "This response link is invalid.", false))
	}
	tc, err := h.svc.ValidateProposalToken(c.Request().Context(), tokenStr)
	if err != nil {
		msg := apperror.UserMessage(err, "This response link is invalid or has expired.")
		return c.HTML(http.StatusOK, rsvpResultHTML("Response Failed", msg, false))
	}
	// 0a: the token's user must still belong to the proposal's campaign. Derived
	// from the proposal (the token, not a URL, is the credential); fail-closed.
	if !h.isCampaignMember(c.Request().Context(), tc.Proposal.CampaignID, tc.Token.UserID) {
		return c.HTML(http.StatusOK, rsvpResultHTML("Response Failed",
			"You're no longer a member of this campaign, so this link can't be used.", false))
	}
	respLabel := proposalResponseLabel(tc.Token.Response)
	local := renderLocalSlotForTZ(*tc.Option, h.tokenUserTZ(c.Request().Context(), tc.Token.UserID))
	msg := fmt.Sprintf("Mark %q for %s (%s) on %q. Tap below to confirm.",
		respLabel, local.DateLabel, local.TimeLabel, tc.Proposal.Title)
	return c.HTML(http.StatusOK, tokenConfirmHTML("Confirm Your Response", msg,
		fmt.Sprintf("/proposals/respond/%s", tokenStr), "Confirm — "+respLabel, middleware.GetCSRFToken(c)))
}

// ApplyProposalToken records a one-click response and consumes the token.
// POST /proposals/respond/:token — the state-changing half (0b). Re-runs the
// open-proposal (service) + current-membership (0a) checks before applying.
func (h *Handler) ApplyProposalToken(c echo.Context) error {
	tokenStr := c.Param("token")
	if tokenStr == "" {
		return c.HTML(http.StatusBadRequest, rsvpResultHTML("Invalid Link", "This response link is invalid.", false))
	}
	tc, err := h.svc.ValidateProposalToken(c.Request().Context(), tokenStr)
	if err != nil {
		msg := apperror.UserMessage(err, "This response link is invalid or has expired.")
		return c.HTML(http.StatusOK, rsvpResultHTML("Response Failed", msg, false))
	}
	if !h.isCampaignMember(c.Request().Context(), tc.Proposal.CampaignID, tc.Token.UserID) {
		return c.HTML(http.StatusOK, rsvpResultHTML("Response Failed",
			"You're no longer a member of this campaign, so this link can't be used.", false))
	}
	if _, err := h.svc.ApplyProposalToken(c.Request().Context(), tokenStr); err != nil {
		msg := apperror.UserMessage(err, "This response link is invalid or has expired.")
		return c.HTML(http.StatusOK, rsvpResultHTML("Response Failed", msg, false))
	}
	return c.HTML(http.StatusOK, rsvpResultHTML("Response Recorded",
		"Thanks — your availability has been recorded. You can close this page.", true))
}

// ConfirmProposalAPI is the DM/Scribe confirm-winner action: pick a winning
// option, which closes the proposal and creates a planned session from that
// slot's UTC instant (C-SCHED-P3). Members are auto-invited to the new session
// and every responder is notified — both off the request path, mirroring create.
// POST /campaigns/:id/proposals/:pid/confirm
func (h *Handler) ConfirmProposalAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	userID := auth.GetUserID(c)
	proposalID := c.Param("pid")
	var req struct {
		OptionID string `json:"optionId" form:"optionId"`
	}
	// Bind from JSON (API) or form (the HTMX confirm button's hx-vals).
	if err := c.Bind(&req); err != nil || req.OptionID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "an option to confirm is required"})
	}
	confirmerTZ := h.resolveViewerTZ(c, userID)
	session, err := h.svc.ConfirmProposalWinner(c.Request().Context(), cc.Campaign.ID, proposalID, req.OptionID, userID, confirmerTZ)
	if err != nil {
		return c.JSON(apperror.SafeCode(err), map[string]string{"error": apperror.SafeMessage(err)})
	}

	// Auto-invite members + send RSVP email (parity with manual CreateSession) and
	// notify everyone who responded — all best-effort, off the request path.
	if h.memberLister != nil {
		if members, mErr := h.memberLister.ListMembers(c.Request().Context(), cc.Campaign.ID); mErr == nil {
			userIDs := make([]string, 0, len(members))
			for _, m := range members {
				userIDs = append(userIDs, m.UserID)
			}
			_ = h.svc.InviteAll(c.Request().Context(), session.ID, userIDs)
			if h.mailer != nil && h.mailer.IsConfigured(c.Request().Context()) {
				go h.sendRSVPEmails(context.Background(), session, cc.Campaign.Name, members)
			}
		}
	}
	go func() {
		if err := h.svc.NotifyProposalConfirmed(context.Background(), cc.Campaign.ID, proposalID, session.ID); err != nil {
			slog.Warn("failed to write confirm notifications", slog.Any("error", err))
		}
	}()

	link := fmt.Sprintf("/campaigns/%s/sessions/%s", cc.Campaign.ID, session.ID)
	// The HTMX confirm button just needs to land on the new session; a client-side
	// redirect keeps the handler thin (no fragment to render).
	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", link)
		return c.NoContent(http.StatusNoContent)
	}
	return c.JSON(http.StatusOK, map[string]string{
		"status":    "ok",
		"sessionId": session.ID,
		"link":      link,
	})
}

// proposalResponseLabel renders a response value for the confirm interstitial.
func proposalResponseLabel(response string) string {
	switch response {
	case ResponseNo:
		return "No"
	case ResponseMaybe:
		return "Maybe"
	default:
		return "Yes"
	}
}

// tokenUserTZ resolves a token user's stored zone (default UTC) so the confirm
// interstitial labels the slot in the same zone the email used.
func (h *Handler) tokenUserTZ(ctx context.Context, userID string) string {
	if h.userDir == nil {
		return "UTC"
	}
	if u, err := h.userDir.GetUser(ctx, userID); err == nil && u != nil && u.Timezone != nil && *u.Timezone != "" {
		return *u.Timezone
	}
	return "UTC"
}

// --- helpers ---

// fillResponderNames resolves the display name for each per-responder row in an
// owner's proposal view. Name resolution is a member-directory concern, kept out
// of the (campaigns-free) service.
func (h *Handler) fillResponderNames(ctx context.Context, campaignID string, view *ProposalView) {
	if h.memberLister == nil {
		return
	}
	members, err := h.memberLister.ListMembers(ctx, campaignID)
	if err != nil {
		return
	}
	nameByUser := make(map[string]string, len(members))
	for _, m := range members {
		nameByUser[m.UserID] = m.DisplayName
	}
	for i := range view.Options {
		for j := range view.Options[i].Responders {
			if n := nameByUser[view.Options[i].Responders[j].UserID]; n != "" {
				view.Options[i].Responders[j].Name = n
			} else {
				view.Options[i].Responders[j].Name = "Member"
			}
		}
	}
}

// resolveDisplayName resolves a user's display name for a notification message.
func (h *Handler) resolveDisplayName(ctx context.Context, userID string) string {
	if h.userDir == nil {
		return ""
	}
	if u, err := h.userDir.GetUser(ctx, userID); err == nil && u != nil {
		return u.DisplayName
	}
	return ""
}

// fanoutProposalCreated writes in-app notifications to every member (except the
// creator) and sends one-click email invites where the mailer is configured.
func (h *Handler) fanoutProposalCreated(ctx context.Context, campaignID, campaignName string, proposal *SlotProposal, creatorID string, members []campaigns.CampaignMember) {
	recipients := make([]string, 0, len(members))
	for _, m := range members {
		if m.UserID == creatorID {
			continue
		}
		recipients = append(recipients, m.UserID)
	}
	if err := h.svc.NotifyProposalCreated(ctx, campaignID, proposal.ID, proposal.Title, recipients); err != nil {
		slog.Warn("failed to write proposal notifications", slog.Any("error", err))
	}

	if h.mailer == nil || !h.mailer.IsConfigured(ctx) {
		return
	}
	// Options are needed to build per-option one-click tokens for the email.
	_, options, err := h.getProposalForEmail(ctx, campaignID, proposal.ID)
	if err != nil {
		slog.Warn("failed to load proposal for email", slog.Any("error", err))
		return
	}
	for _, m := range members {
		if m.UserID == creatorID || m.Email == "" {
			continue
		}
		h.sendProposalEmail(ctx, campaignID, campaignName, proposal, options, m)
	}
}

// getProposalForEmail re-loads a proposal + options via the service view (owner
// zone irrelevant here — only the option ids/UTC instants are used).
func (h *Handler) getProposalForEmail(ctx context.Context, campaignID, proposalID string) (*ProposalView, []ProposalOptionView, error) {
	view, err := h.svc.GetProposalView(ctx, campaignID, proposalID, "", "UTC", false)
	if err != nil {
		return nil, nil, err
	}
	return view, view.Options, nil
}

// sendProposalEmail sends one member a scheduling-proposal invite: each option
// gets three one-click buttons (Yes / Maybe / No), each backed by a token keyed
// (option, user, response) so the response records with no login. Slots are
// rendered in the member's own stored zone.
func (h *Handler) sendProposalEmail(ctx context.Context, campaignID, campaignName string, proposal *SlotProposal, options []ProposalOptionView, m campaigns.CampaignMember) {
	memberTZ := "UTC"
	if u, err := h.userDir.GetUser(ctx, m.UserID); err == nil && u != nil && u.Timezone != nil {
		memberTZ = *u.Timezone
	}

	var optionsHTML, optionsText string
	for _, ov := range options {
		tokens, err := h.svc.CreateProposalTokens(ctx, ov.Option.ID, m.UserID)
		if err != nil {
			slog.Warn("failed to create proposal tokens", slog.Any("error", err), slog.String("user_id", m.UserID))
			continue
		}
		local := renderLocalSlotForTZ(ov.Option, memberTZ)
		yesURL := fmt.Sprintf("%s/proposals/respond/%s", h.baseURL, tokens[ResponseYes])
		maybeURL := fmt.Sprintf("%s/proposals/respond/%s", h.baseURL, tokens[ResponseMaybe])
		noURL := fmt.Sprintf("%s/proposals/respond/%s", h.baseURL, tokens[ResponseNo])

		optionsHTML += fmt.Sprintf(`<div style="background:#f8f9fa;border-radius:8px;padding:14px 16px;margin-bottom:12px">
  <div style="font-weight:600;font-size:14px;margin-bottom:8px">%s<br><span style="color:#666;font-weight:400">%s</span></div>
  <a href="%s" style="display:inline-block;padding:8px 16px;background:#22c55e;color:#fff;text-decoration:none;border-radius:6px;font-weight:600;font-size:13px;margin-right:6px">✓ Yes</a>
  <a href="%s" style="display:inline-block;padding:8px 16px;background:#eab308;color:#fff;text-decoration:none;border-radius:6px;font-weight:600;font-size:13px;margin-right:6px">~ Maybe</a>
  <a href="%s" style="display:inline-block;padding:8px 16px;background:#ef4444;color:#fff;text-decoration:none;border-radius:6px;font-weight:600;font-size:13px">✗ No</a>
</div>`, local.DateLabel, local.TimeLabel, yesURL, maybeURL, noURL)

		optionsText += fmt.Sprintf("%s, %s\n  Yes: %s\n  Maybe: %s\n  No: %s\n\n",
			local.DateLabel, local.TimeLabel, yesURL, maybeURL, noURL)
	}

	subject := fmt.Sprintf("When can you play? — %s", campaignName)
	// Plain-text body: no markup, so the raw values are safe as-is.
	plainBody := fmt.Sprintf("You've been asked to weigh in on session times for %s.\n\nProposal: %s\nTimes shown in %s.\n\n%sThese links expire in 7 days.\n",
		campaignName, proposal.Title, memberTZ, optionsText)
	// HTML body: escape every interpolated data value (campaign name + operator-
	// authored proposal title + zone) so a title like `<img onerror=…>` can't
	// inject markup into the email (C-SCHED-P3 0c). optionsHTML is our own
	// server-built markup (date/time labels + token URLs), so it is NOT escaped.
	htmlBody := fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="utf-8"></head><body style="font-family:system-ui,-apple-system,sans-serif;max-width:520px;margin:0 auto;padding:20px;color:#333">
<div style="text-align:center;margin-bottom:20px"><div style="font-size:32px;margin-bottom:8px">📅</div>
<h1 style="font-size:20px;margin:0">When can you play?</h1>
<p style="color:#666;font-size:14px;margin:6px 0 0">%s · times in %s</p></div>
<h2 style="font-size:16px;margin:0 0 12px">%s</h2>
%s
<p style="text-align:center;color:#999;font-size:12px;margin-top:20px">These links expire in 7 days.</p>
</body></html>`, html.EscapeString(campaignName), html.EscapeString(memberTZ), html.EscapeString(proposal.Title), optionsHTML)

	if err := h.mailer.SendHTMLMail(ctx, []string{m.Email}, subject, plainBody, htmlBody); err != nil {
		slog.Warn("failed to send proposal email", slog.Any("error", err), slog.String("to", m.Email))
	}
}
