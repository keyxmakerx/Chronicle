// foundry_matches_handler.go — the routes for creature-page matches: one
// page's match and the DM's changes to it (the creature page's "In Foundry"
// box), every page's match (the Foundry module and the Foundry page), and a
// search of Foundry's monsters for "Match another". All DM-only: matches name
// the GM's own actors.

package syncapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// CreaturePage is a page of the campaign's creature type.
type CreaturePage struct {
	ID, Name string
}

// CreaturePageLister finds the campaign's creature pages. Implemented in app
// over the entities service at the DM's visibility.
type CreaturePageLister interface {
	// CreaturePages lists them by name; truncated says more exist.
	CreaturePages(ctx context.Context, campaignID string) (pages []CreaturePage, truncated bool, err error)
	// CreaturePage is one of them; nil when the page is not a creature page
	// of this campaign.
	CreaturePage(ctx context.Context, campaignID, entityID string) (*CreaturePage, error)
}

// FoundryMatcher works out creature pages' matches and stores the DM's
// changes. It holds no Echo types, so the AI connection uses it too.
type FoundryMatcher struct {
	monsters  FoundryMonsterRepository
	matches   FoundryMatchRepository
	creatures CreaturePageLister
}

// NewFoundryMatcher wires the matcher.
func NewFoundryMatcher(monsters FoundryMonsterRepository, matches FoundryMatchRepository, creatures CreaturePageLister) *FoundryMatcher {
	return &FoundryMatcher{monsters: monsters, matches: matches, creatures: creatures}
}

// FoundryMatches is every creature page's match.
type FoundryMatches struct {
	Matches    []CreatureMatch `json:"data"`
	Total      int             `json:"total"`
	Truncated  bool            `json:"truncated"`
	ReportedAt *time.Time      `json:"reportedAt"`
}

func reportedAt(list *FoundryMonsterList) *time.Time {
	if list == nil {
		return nil
	}
	t := list.ReportedAt
	return &t
}

// Page is one page's match, with the report it came from.
func (m *FoundryMatcher) Page(ctx context.Context, campaignID, entityID string) (CreatureMatch, *FoundryMonsterList, error) {
	page, err := m.creatures.CreaturePage(ctx, campaignID, entityID)
	if err != nil {
		return CreatureMatch{}, nil, err
	}
	if page == nil {
		return CreatureMatch{}, nil, apperror.NewNotFound("creature page not found")
	}
	list, err := m.monsters.Get(ctx, campaignID)
	if err != nil {
		return CreatureMatch{}, nil, err
	}
	ch, err := m.matches.Get(ctx, campaignID, entityID)
	if err != nil {
		return CreatureMatch{}, nil, err
	}
	return matchCreature(page.ID, page.Name, list, ch), list, nil
}

// All is every creature page's match, by page name.
func (m *FoundryMatcher) All(ctx context.Context, campaignID string) (FoundryMatches, error) {
	pages, truncated, err := m.creatures.CreaturePages(ctx, campaignID)
	if err != nil {
		return FoundryMatches{}, err
	}
	list, err := m.monsters.Get(ctx, campaignID)
	if err != nil {
		return FoundryMatches{}, err
	}
	choices, err := m.matches.List(ctx, campaignID)
	if err != nil {
		return FoundryMatches{}, err
	}
	out := FoundryMatches{Matches: make([]CreatureMatch, 0, len(pages)), Truncated: truncated, ReportedAt: reportedAt(list)}
	for _, p := range pages {
		var ch *FoundryMatchChoice
		if c, ok := choices[p.ID]; ok {
			ch = &c
		}
		out.Matches = append(out.Matches, matchCreature(p.ID, p.Name, list, ch))
	}
	out.Total = len(out.Matches)
	return out, nil
}

// Change applies the DM's action to one page and returns its new match.
func (m *FoundryMatcher) Change(ctx context.Context, campaignID, entityID string, a FoundryMatchAction) (CreatureMatch, error) {
	cur, list, err := m.Page(ctx, campaignID, entityID)
	if err != nil {
		return CreatureMatch{}, err
	}
	stored, err := m.matches.Get(ctx, campaignID, entityID)
	if err != nil {
		return CreatureMatch{}, err
	}
	ch := FoundryMatchChoice{EntityID: entityID}
	if stored != nil {
		ch = *stored
	}
	next, err := applyMatchAction(ch, cur, list, a)
	if err != nil {
		return CreatureMatch{}, err
	}
	if err := m.matches.Put(ctx, campaignID, next); err != nil {
		return CreatureMatch{}, err
	}
	return matchCreature(entityID, cur.EntityName, list, &next), nil
}

// Search is Foundry's monsters whose name holds q, world actors first, at
// most limit. Actors creature sync made are left out: they are pages already.
func (m *FoundryMatcher) Search(ctx context.Context, campaignID, q string, limit int) ([]FoundryMonster, error) {
	list, err := m.monsters.Get(ctx, campaignID)
	if err != nil || list == nil {
		return []FoundryMonster{}, err
	}
	want := matchKey(q)
	out := []FoundryMonster{}
	for _, mon := range list.Monsters {
		if !mon.Linked && strings.Contains(matchKey(mon.Name), want) {
			out = append(out, mon)
			if len(out) == limit {
				break
			}
		}
	}
	return out, nil
}

// foundryMonsterSearchMax caps one "Match another" search.
const foundryMonsterSearchMax = 50

// SetFoundryMatcher wires the match routes. Without it they answer 404.
func (h *SyncHistoryHandler) SetFoundryMatcher(m *FoundryMatcher) { h.matcher = m }

// matcherFor gates a match route: wired, and an owner or DM-access caller.
func (h *SyncHistoryHandler) matcherFor(c echo.Context) (*FoundryMatcher, error) {
	if h.matcher == nil {
		return nil, apperror.NewNotFound("creature matches are not available")
	}
	if !h.isDMEquivalent(c) {
		return nil, apperror.NewForbidden("creature matches need owner or DM access")
	}
	return h.matcher, nil
}

// GetFoundryMatch is one creature page's match.
// GET /api/v1/campaigns/:id/entities/:entityID/foundry-match
func (h *SyncHistoryHandler) GetFoundryMatch(c echo.Context) error {
	m, err := h.matcherFor(c)
	if err != nil {
		return err
	}
	match, list, err := m.Page(c.Request().Context(), c.Param("id"), c.Param("entityID"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"match": match, "reportedAt": reportedAt(list)})
}

// PutFoundryMatch applies one change: {"action":"use","use":"foundry"|"page"},
// {"action":"match","name":…,"pack":…}, {"action":"not_same","name":…} or
// {"action":"reset"}.
// PUT /api/v1/campaigns/:id/entities/:entityID/foundry-match
func (h *SyncHistoryHandler) PutFoundryMatch(c echo.Context) error {
	m, err := h.matcherFor(c)
	if err != nil {
		return err
	}
	var a FoundryMatchAction
	if err := c.Bind(&a); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	match, err := m.Change(c.Request().Context(), c.Param("id"), c.Param("entityID"), a)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"match": match})
}

// ListFoundryMatches is every creature page's match.
// GET /api/v1/campaigns/:id/foundry-matches
func (h *SyncHistoryHandler) ListFoundryMatches(c echo.Context) error {
	m, err := h.matcherFor(c)
	if err != nil {
		return err
	}
	all, err := m.All(c.Request().Context(), c.Param("id"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, all)
}

// SearchFoundryMonsters finds Foundry's monsters by part of a name.
// GET /api/v1/campaigns/:id/foundry-monsters?q=
func (h *SyncHistoryHandler) SearchFoundryMonsters(c echo.Context) error {
	m, err := h.matcherFor(c)
	if err != nil {
		return err
	}
	found, err := m.Search(c.Request().Context(), c.Param("id"), c.QueryParam("q"), foundryMonsterSearchMax)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"data": found, "total": len(found)})
}
