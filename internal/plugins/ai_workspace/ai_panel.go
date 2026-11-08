package ai_workspace

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/prompt"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// ChipCounts is what the top bar's AI button shows: requests waiting for the
// owner, open warnings about their keys, and whether AI is stopped here.
type ChipCounts struct {
	Waiting  int  `json:"waiting"`
	Warnings int  `json:"warnings"`
	Stopped  bool `json:"stopped"`
}

// AIChip answers the AI button on every campaign page. It is JSON, not a
// fragment, so the button can tell when the count goes up and drop a notice.
func (h *Handler) AIChip(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	var out ChipCounts
	if h.connector != nil {
		ctx := c.Request().Context()
		reqs, err := h.connector.Waiting(ctx, cc.Campaign.ID, auth.GetUserID(c))
		if err != nil {
			return err
		}
		out.Waiting = len(reqs)
		if h.connector.Watching() {
			alerts, err := h.connector.Alerts(ctx, cc.Campaign.ID)
			if err != nil {
				return err
			}
			out.Warnings = len(alerts)
			sw, err := h.connector.CampaignSwitches(ctx, cc.Campaign.ID)
			if err != nil {
				return err
			}
			out.Stopped = sw.CutOffAt != nil
		}
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, out)
}

// AIBubble is the small bubble the AI button drops: copy and paste in one
// thin row, then each waiting request with Review.
func (h *Handler) AIBubble(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	v, err := h.connectView(c, cc)
	if err != nil {
		return err
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return middleware.Render(c, http.StatusOK, aiBubbleBody(cc, v))
}

// AIPanel is the side sheet a review opens in: the AI's waiting requests,
// answered in place, and the review of a pasted answer. With tool=1 it holds
// the whole copy-and-paste tool instead ("Write a request…").
func (h *Handler) AIPanel(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	v, err := h.connectView(c, cc)
	if err != nil {
		return err
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return middleware.Render(c, http.StatusOK, aiPanelBody(cc, v, c.QueryParam("tool") == "1"))
}

// copyPageAsk is what the copied text asks the AI to do with the page.
const copyPageAsk = "I'm looking at the page below. Ask me what I'd like to do with it, then answer in the format above."

// CopyForAI is the text the bubble's Copy button puts on the clipboard: the
// standing instructions and, when the owner is on a page, that page as a
// lookup reads it. privacy is the owner's remembered choice; a page it hides
// is left out, never named.
func (h *Handler) CopyForAI(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if h.promptBuilder == nil {
		return apperror.NewBadRequest("AI prompt builder is not configured on this server")
	}
	ctx := c.Request().Context()
	in := prompt.Input{
		IncludeEntityTypes:        true,
		IncludeCategoriesInUse:    true,
		IncludeFrontMatterExample: true,
		ContentMode:               "none",
		Privacy:                   parsePrivacy(c.QueryParam("privacy")),
	}
	var page string
	if id := strings.TrimSpace(c.QueryParam("page")); id != "" && h.lookups != nil {
		text, ok, err := h.lookups.PageByID(ctx, cc.Campaign.ID, exportActor(c, cc, in.Privacy), id)
		if err != nil {
			return err
		}
		if ok {
			page = text
			in.OperatorInstruction = copyPageAsk
		} else {
			// The bubble then says the page was left out, not "copied".
			c.Response().Header().Set("X-AI-Page", "left-out")
		}
	}
	h.fillPromptInput(c, cc, &in)
	out, err := h.promptBuilder.Build(ctx, cc.Campaign.Name, auth.GetUserID(c), cc.Campaign.ID, in)
	if err != nil {
		slog.Error("ai-workspace: copy for AI failed",
			slog.String("campaign_id", cc.Campaign.ID), slog.Any("error", err))
		return apperror.NewInternal(err)
	}
	if page != "" {
		out = strings.TrimRight(out, "\n") + "\n\n# The page I'm on\n\n" + strings.TrimSpace(page) + "\n"
	}
	if h.audit != nil {
		h.audit.LogCampaignEvent(ctx, cc.Campaign.ID, auth.GetUserID(c), "campaign.ai_prompt.copied",
			map[string]any{"privacy": c.QueryParam("privacy"), "with_page": page != "", "prompt_byte_count": len(out)})
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.String(http.StatusOK, out)
}
