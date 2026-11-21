package campaigns

// The owner's "Getting your table ready" line on the campaign home. Each step
// is worked out from what exists when the home is drawn (a game system, an
// invitation or join link, a saved look, a page), so nothing is ticked by
// hand and a step that stops being true shows as open again. Only two steps
// can be passed on instead of done, because "no game system" and "keep this
// look" leave nothing behind to find; those are the keys stored in
// CampaignSettings.SetupKept.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// Setup step keys, also the path segment of the keep route.
const (
	setupGame   = "game"
	setupInvite = "invite"
	setupLook   = "look"
	setupPage   = "page"
)

// setupFacts is what the steps are worked out from.
type setupFacts struct {
	SystemName  string // The chosen game system's name; empty when none or unnamed.
	HasSystem   bool
	Invites     int  // Invitations ever sent, accepted or not.
	HasJoinLink bool // A join link has been made.
	LookSaved   bool
	HasPage     bool
	Kept        []string
}

// setupStep is one row of the unfolded list.
type setupStep struct {
	Key   string
	Title string
	Value string // What is true now ("Draw Steel", "1 invite sent"); empty while open.
	Done  bool
	// Href opens where the step is done; Action is its words.
	Href, Action string
	// KeepWord is the quiet word that passes the step on, empty when the step
	// cannot be passed on or is done.
	KeepWord string
}

// setupView is the line and its rows.
type setupView struct {
	Steps      []setupStep
	DoneCount  int
	Next       *setupStep
	CampaignID string
	CSRF       string
	Open       bool   // The list is unfolded (it stays so after passing a step on).
	JustDone   string // Step key that ticked in this response, for its tick motion.
}

// lookChosen reports whether the saved look holds anything beyond what every
// new campaign starts with (the Annals page look and its timeline popup).
func lookChosen(a *Appearance) bool {
	if a == nil {
		return false
	}
	c := *a
	if c.PageLook == "annals" {
		c.PageLook = ""
	}
	if c.TimelinePopup == "corner" {
		c.TimelinePopup = ""
	}
	b, err := json.Marshal(c)
	return err == nil && string(b) != "{}"
}

func setupKept(kept []string, key string) bool {
	for _, k := range kept {
		if k == key {
			return true
		}
	}
	return false
}

// buildSetupView words the four steps from the facts.
func buildSetupView(campaignID string, f setupFacts) setupView {
	base := "/campaigns/" + campaignID
	game := setupStep{Key: setupGame, Title: "Choose your game", Href: base + "/extensions", Action: "Choose a game"}
	switch {
	case f.HasSystem:
		game.Done = true
		game.Value = f.SystemName
	case setupKept(f.Kept, setupGame):
		game.Done, game.Value = true, "No game system"
	default:
		game.KeepWord = "No game system"
	}
	invite := setupStep{Key: setupInvite, Title: "Invite your players", Href: base + "/members", Action: "Invite"}
	switch {
	case f.Invites == 1:
		invite.Done, invite.Value = true, "1 invite sent"
	case f.Invites > 1:
		invite.Done, invite.Value = true, fmt.Sprintf("%d invites sent", f.Invites)
	case f.HasJoinLink:
		invite.Done, invite.Value = true, "Invite link made"
	}
	look := setupStep{Key: setupLook, Title: "Pick a look", Href: base + "/customize/look", Action: "Open the Customizer"}
	switch {
	case f.LookSaved:
		look.Done, look.Value = true, "Look saved"
	case setupKept(f.Kept, setupLook):
		look.Done, look.Value = true, "Kept as it is"
	default:
		look.KeepWord = "Keep this one"
	}
	page := setupStep{Key: setupPage, Title: "Write your first page", Href: base + "/entities/new", Action: "New page"}
	if f.HasPage {
		page.Done, page.Value = true, "Page written"
	}
	v := setupView{Steps: []setupStep{game, invite, look, page}, CampaignID: campaignID}
	for i := range v.Steps {
		if v.Steps[i].Done {
			v.DoneCount++
		} else if v.Next == nil {
			v.Next = &v.Steps[i]
		}
	}
	return v
}

// setupFactsFor reads what the steps depend on. A fact that cannot be read
// counts as not done: the line then offers a step the owner may already have
// done, which costs a press, rather than hiding one that is still open.
func (h *Handler) setupFactsFor(ctx context.Context, cc *CampaignContext, userID string) setupFacts {
	settings := cc.Campaign.ParseSettings()
	f := setupFacts{
		LookSaved:   lookChosen(settings.Appearance),
		HasJoinLink: cc.Campaign.JoinCode != nil && *cc.Campaign.JoinCode != "",
		Kept:        settings.SetupKept,
	}
	in := h.deskInputsFor(ctx, cc.Campaign, deskSystem)
	f.HasSystem = in.SystemID != "" || settings.SystemID != ""
	f.SystemName = in.SystemName
	if f.HasSystem && f.SystemName == "" {
		f.SystemName = "Game chosen"
	}
	if h.invites != nil {
		if inv, err := h.invites.ListInvites(ctx, cc.Campaign.ID); err != nil {
			slog.Warn("setup steps: invitations not read", slog.String("campaign_id", cc.Campaign.ID), slog.Any("error", err))
		} else {
			f.Invites = len(inv)
		}
	}
	if h.recentLister != nil {
		if pages, err := h.recentLister.ListRecentForDashboard(ctx, cc.Campaign.ID, int(RoleOwner), userID, 1); err != nil {
			slog.Warn("setup steps: pages not read", slog.String("campaign_id", cc.Campaign.ID), slog.Any("error", err))
		} else {
			f.HasPage = len(pages) > 0
		}
	}
	return f
}

// SetupStepsBlock draws the line, or an empty answer that removes the
// placeholder once all four are done.
// GET /campaigns/:id/setup-steps
func (h *Handler) SetupStepsBlock(c *echo.Context) error {
	return h.renderSetupSteps(c, false, "")
}

// KeepSetupStepPost passes a step on and answers with the line, unfolded, so
// the owner sees the step tick where they pressed.
// POST /campaigns/:id/setup-steps/:step/keep
func (h *Handler) KeepSetupStepPost(c *echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	step := c.Param("step")
	if err := h.service.KeepSetupStep(c.Request().Context(), cc.Campaign.ID, step); err != nil {
		return err
	}
	if !middleware.IsHTMX(c) {
		return c.Redirect(http.StatusSeeOther, "/campaigns/"+cc.Campaign.ID)
	}
	// The campaign in the context predates the write, so read it again.
	fresh, err := h.service.GetByID(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}
	cc.Campaign = fresh
	return h.renderSetupSteps(c, true, step)
}

func (h *Handler) renderSetupSteps(c *echo.Context, open bool, justDone string) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	// The note is the owner's; a co-Director has a note of their own.
	if cc.MemberRole < RoleOwner {
		return c.HTML(http.StatusOK, "")
	}
	v := buildSetupView(cc.Campaign.ID, h.setupFactsFor(c.Request().Context(), cc, auth.GetUserID(c)))
	if v.Next == nil {
		return c.HTML(http.StatusOK, "")
	}
	v.CSRF = middleware.GetCSRFToken(c)
	v.Open = open
	v.JustDone = justDone
	return middleware.Render(c, http.StatusOK, setupStepsBlock(v))
}
