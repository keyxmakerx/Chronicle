package quests

// The campaign home's "Open quests" block: the three open quests that most
// need attention, each with one status word, and a quiet word to the rest.
//
// What a viewer may see is decided by QuestService.Digest, which applies the
// same page gate as a quest's own sheet and hides a notice the DM hid; this
// file only words the result. The view holds finished strings, so nothing
// that was filtered out can reach the template.

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// openQuestsShown is how many quests the block lists; the board has the rest.
const openQuestsShown = 3

// openQuestRow is one quest, already worded.
type openQuestRow struct {
	Title string
	// Word is "In progress", "New" or a due phrase; Late marks an overdue one.
	Word string
	Late bool
	Href string
}

// openQuestsView is what the template draws.
type openQuestsView struct {
	Rows []openQuestRow
	// AllWord is "All 7 quests"; empty (no link) when there is no board to
	// open.
	AllWord string
	AllHref string
}

// dueWord words a due date's distance in the campaign's own days.
func dueWord(days int) string {
	switch {
	case days < 0:
		return "Overdue"
	case days == 0:
		return "Due today"
	case days == 1:
		return "Due tomorrow"
	}
	return fmt.Sprintf("Due in %d days", days)
}

// statusWord words an open quest's status.
func statusWord(status string) string {
	if status == StatusActive {
		return "In progress"
	}
	return "New"
}

// allWord words the link to the board.
func allWord(total int) string {
	if total == 1 {
		return "All 1 quest"
	}
	return fmt.Sprintf("All %d quests", total)
}

// buildOpenQuests words the digest for campaignID.
func buildOpenQuests(campaignID string, d *QuestDigest) openQuestsView {
	base := "/campaigns/" + url.PathEscape(campaignID)
	var v openQuestsView
	for i, q := range d.Open {
		if i >= openQuestsShown {
			break
		}
		row := openQuestRow{Title: q.Title, Word: statusWord(q.Status), Href: base + "/entities/" + url.PathEscape(q.EntityID)}
		if q.DaysLeft != nil {
			row.Word, row.Late = dueWord(*q.DaysLeft), *q.DaysLeft < 0
		}
		v.Rows = append(v.Rows, row)
	}
	if d.Total > 0 && d.TypeSlug != "" {
		v.AllWord = allWord(d.Total)
		v.AllHref = base + "/" + url.PathEscape(d.TypeSlug)
	}
	return v
}

// OpenQuestsBlock draws the block. A guest of a public campaign gets it only
// where the owner placed it on the Guests home, and then only the quests an
// anonymous viewer may already open. An owner viewing as a player is read as
// a player.
// GET /campaigns/:id/open-quests
func (h *Handler) OpenQuestsBlock(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	campaignID, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	if !campaigns.BlockVisibleTo(c.Request().Context(), cc, h.placement, campaigns.BlockOpenQuests, c.QueryParam("on"), v.UserID) {
		return c.HTML(http.StatusOK, "")
	}
	if campaigns.ViewingAsPlayer(c, cc) {
		v.IsDM, v.MemberRole, v.VisibilityRole = false, int(campaigns.RolePlayer), int(campaigns.RolePlayer)
	}
	digest, err := h.quests.Digest(c.Request().Context(), campaignID, v)
	if err != nil {
		// A broken read removes the card rather than claiming no quests.
		return c.HTML(http.StatusOK, "")
	}
	// A guest with nothing to open gets no card at all.
	if !cc.IsMember && len(digest.Open) == 0 {
		return c.HTML(http.StatusOK, "")
	}
	return middleware.Render(c, http.StatusOK, openQuestsBlock(buildOpenQuests(campaignID, digest)))
}
