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
func (h *APIHandler) judgeActingPlayer(ctx context.Context, entity *entities.Entity, actingUserID string, patch map[string]any) (map[string]string, error) {
	if actingUserID == "" {
		return nil, nil
	}
	member, err := h.campaignSvc.GetMember(ctx, entity.CampaignID, actingUserID)
	if err != nil {
		var ae *apperror.AppError
		if errors.As(err, &ae) && ae.Code == http.StatusNotFound {
			return nil, apperror.NewForbidden("that player isn't a member of this campaign")
		}
		return nil, err
	}
	if member == nil {
		return nil, apperror.NewForbidden("that player isn't a member of this campaign")
	}
	if h.pageGate().visibilityRole(ctx, entity.CampaignID, actingUserID, int(member.Role)) >= int(campaigns.RoleScribe) {
		return nil, nil
	}
	if !entities.IsClaimedOwnerPlayer(member.Role, entity, actingUserID) {
		return nil, apperror.NewForbidden("you can only change a character you own")
	}
	access, err := h.entitySvc.CheckEntityAccess(ctx, entity.ID, int(campaigns.RolePlayer), actingUserID)
	if err != nil || access == nil || !access.CanView {
		return nil, apperror.NewNotFound("entity not found")
	}
	if entities.PatchChangesNothing(entity, patch) {
		return nil, nil
	}
	var plan *entities.HeroPlan
	if h.heroPlanner != nil {
		// A plan that can't be read must not turn into "no limits".
		if plan, err = h.heroPlanner.HeroPlan(ctx, entity.CampaignID, actingUserID, false); err != nil {
			return nil, err
		}
	}
	return entities.PlayerPicksHeld(entity, patch, plan), nil
}
