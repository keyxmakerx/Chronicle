// foundry_creatures.go — the Foundry page's Creatures in Foundry card: its
// fragment, its row buttons, and the words it shows.

package syncapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// creaturesByNeed puts look-alikes waiting on the owner first, then pages
// whose monster went missing, then the rest in their name order.
func creaturesByNeed(in []CreatureMatch) []CreatureMatch {
	rank := map[string]int{MatchSuggested: 0, MatchMissing: 1}
	out := append([]CreatureMatch(nil), in...)
	sort.SliceStable(out, func(i, j int) bool {
		ri, ok := rank[out[i].State]
		if !ok {
			ri = 2
		}
		rj, ok := rank[out[j].State]
		if !ok {
			rj = 2
		}
		return ri < rj
	})
	return out
}

// useWords says whose numbers play.
func useWords(use string) string {
	if use == MatchUsePage {
		return "this page's numbers"
	}
	return "Foundry's numbers"
}

// monsterWhere says where Foundry keeps a monster.
func monsterWhere(m *FoundryMonster) string {
	if m == nil {
		return ""
	}
	if m.Pack != "" {
		return "Foundry's " + m.Name + ", in the " + m.Pack + " compendium"
	}
	return "Foundry's " + m.Name + ", an actor in the world"
}

// matchVals is a row button's form values.
func matchVals(action string, m *FoundryMonster) string {
	v := map[string]string{"action": action}
	if m != nil {
		v["name"], v["pack"] = m.Name, m.Pack
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// CreaturesFragment is the Creatures in Foundry card body.
// GET /campaigns/:id/foundry/creatures
func (h *SyncHistoryHandler) CreaturesFragment(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if h.matcher == nil {
		return apperror.NewNotFound("creature matches are not available")
	}
	all, err := h.matcher.All(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, FoundryCreaturesFragment(FoundryCreaturesView{CampaignID: cc.Campaign.ID, Matches: all}))
}

// ChangeCreatureMatch applies a row's button and redraws the card. A refused
// change is said on the card, not as an error page.
// POST /campaigns/:id/foundry/creatures/:eid
func (h *SyncHistoryHandler) ChangeCreatureMatch(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if h.matcher == nil {
		return apperror.NewNotFound("creature matches are not available")
	}
	ctx := c.Request().Context()
	a := FoundryMatchAction{Action: c.FormValue("action"), Use: c.FormValue("use"), Name: c.FormValue("name"), Pack: c.FormValue("pack")}
	v := FoundryCreaturesView{CampaignID: cc.Campaign.ID}
	if _, err := h.matcher.Change(ctx, cc.Campaign.ID, c.Param("eid"), a); err != nil {
		var ae *apperror.AppError
		if !errors.As(err, &ae) || ae.Code >= 500 {
			return err
		}
		v.Error = ae.Message
	}
	all, err := h.matcher.All(ctx, cc.Campaign.ID)
	if err != nil {
		return err
	}
	v.Matches = all
	return middleware.Render(c, http.StatusOK, FoundryCreaturesFragment(v))
}
