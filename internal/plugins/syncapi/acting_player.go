package syncapi

import (
	"context"
	"errors"
	"net/http"
	"sort"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// A player's edit in Foundry reaches Chronicle under the GM's key, which
// carries Owner rights, so the key says nothing about what that player may
// change. The module names the member the player is matched to
// (actingUserId) and Chronicle judges the write as that member with the same
// hero-choice check the web door applies, so a pick the web would refuse is
// refused here too.

// SetHeroPlanner gives the API the hero builder plan a player is shown, which
// the pick check reads. Without one a player's write is judged on identity
// texts only.
func (h *APIHandler) SetHeroPlanner(p entities.HeroPlanner) {
	h.heroPlanner = p
}

// SetHeroChanges gives the API the Director's times for changing a hero's
// picks, so a player's Foundry edit is locked and rings the bell exactly as
// a web edit does. Without it picks are never locked here.
func (h *APIHandler) SetHeroChanges(s *entities.HeroChangeService) {
	h.heroChanges = s
}

// heldKey is one key a player's write did not store, and why in plain words.
type heldKey struct {
	Key    string `json:"key"`
	Reason string `json:"reason"`
}

// heldKeysList is held in key order, so the answer is stable.
func heldKeysList(held map[string]string) []heldKey {
	out := make([]heldKey, 0, len(held))
	for k, why := range held {
		out = append(out, heldKey{Key: k, Reason: why})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// judgeActingPlayer returns the keys of patch that may not be stored for the
// acting member, with the reason for each. No acting member, or one with
// Scribe rights or above (or a Co-DM grant), is judged as the key's own user:
// nothing is held. A player who does not own the page, or cannot open it, is
// refused outright, since the GM's key must not be a way to write a page the
// player could not.
//
// The verdict's changes are the picks the write changes while the hero's
// picks are open, and raised the level the player gives their own hero, both
// for the Directors' bell once the write is stored. Staff and the plain sync
// get neither, so their writes ring nothing.
func (h *APIHandler) judgeActingPlayer(ctx context.Context, entity *entities.Entity, actingUserID string, patch map[string]any) (actingVerdict, error) {
	var v actingVerdict
	if actingUserID == "" {
		return v, nil
	}
	member, err := h.campaignSvc.GetMember(ctx, entity.CampaignID, actingUserID)
	if err != nil {
		var ae *apperror.AppError
		if errors.As(err, &ae) && ae.Code == http.StatusNotFound {
			return v, apperror.NewForbidden("that player isn't a member of this campaign")
		}
		return v, err
	}
	if member == nil {
		return v, apperror.NewForbidden("that player isn't a member of this campaign")
	}
	if h.pageGate().visibilityRole(ctx, entity.CampaignID, actingUserID, int(member.Role)) >= int(campaigns.RoleScribe) {
		return v, nil
	}
	if !entities.IsClaimedOwnerPlayer(member.Role, entity, actingUserID) {
		return v, apperror.NewForbidden("you can only change a character you own")
	}
	access, err := h.entitySvc.CheckEntityAccess(ctx, entity.ID, int(campaigns.RolePlayer), actingUserID)
	if err != nil || access == nil || !access.CanView {
		return v, apperror.NewNotFound("entity not found")
	}
	if entities.PatchChangesNothing(entity, patch) {
		return v, nil
	}
	var plan *entities.HeroPlan
	if h.heroPlanner != nil {
		// A plan that can't be read must not turn into "no limits".
		if plan, err = h.heroPlanner.HeroPlan(ctx, entity.CampaignID, actingUserID, false); err != nil {
			return v, err
		}
	}
	held := entities.PlayerPicksHeld(entity, patch, plan)
	if held == nil {
		held = map[string]string{}
	}
	rest := make(map[string]any, len(patch))
	for k, val := range patch {
		if _, refused := held[k]; !refused {
			rest[k] = val
		}
	}
	locked, changes, err := h.heroChanges.HeldPicks(ctx, entity, rest, plan)
	if err != nil {
		return v, err
	}
	for k, why := range locked {
		held[k] = why
	}
	v.held, v.changes = held, changes
	if _, refused := held[entities.HeroLevelKey]; !refused {
		v.raised = entities.HeroLevelRaised(entity, patch)
	}
	return v, nil
}

// actingVerdict is how a player's write through the sync was judged: the
// keys held back with why, and what the Directors' bell says once the rest
// is stored.
type actingVerdict struct {
	held    map[string]string
	changes []entities.HeroPickChange
	raised  int
}
