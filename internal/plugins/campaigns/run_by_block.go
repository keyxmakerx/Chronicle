package campaigns

// The campaign home's "Run by" block: who runs the table, by name. It lists
// the owner and the members the owner made co-Directors, and nothing else
// about them (no email, no role beyond the title).

import (
	"net/http"
	"sort"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// runByPerson is one line of the block.
type runByPerson struct {
	Name  string
	Title string // "Owner" or "Co-" plus the campaign's word for its game master.
}

// runByPeople picks the owner and the co-Directors from the member list, the
// owner first, then the rest in the order they joined.
func runByPeople(campaign *Campaign, members []CampaignMember, gm string) []runByPerson {
	co := map[string]bool{}
	for _, id := range campaign.ParseSettings().DmGrantIDs {
		co[id] = true
	}
	sorted := append([]CampaignMember(nil), members...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if (sorted[i].Role >= RoleOwner) != (sorted[j].Role >= RoleOwner) {
			return sorted[i].Role >= RoleOwner
		}
		return sorted[i].JoinedAt.Before(sorted[j].JoinedAt)
	})
	var out []runByPerson
	for _, m := range sorted {
		name := m.DisplayName
		if name == "" {
			continue
		}
		switch {
		case m.Role >= RoleOwner:
			out = append(out, runByPerson{Name: name, Title: "Owner"})
		case co[m.UserID]:
			out = append(out, runByPerson{Name: name, Title: "Co-" + gm})
		}
	}
	return out
}

// RunByBlock draws the block, or an empty answer that removes the placeholder
// for a visitor or when no one can be named.
// GET /campaigns/:id/run-by
func (h *Handler) RunByBlock(c *echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if !cc.IsMember {
		return c.HTML(http.StatusOK, "")
	}
	ctx := c.Request().Context()
	members, err := h.service.ListMembers(ctx, cc.Campaign.ID)
	if err != nil {
		return err
	}
	people := runByPeople(cc.Campaign, members, layouts.GMTerm(ctx))
	if len(people) == 0 {
		return c.HTML(http.StatusOK, "")
	}
	return middleware.Render(c, http.StatusOK, runByBlock(people))
}
