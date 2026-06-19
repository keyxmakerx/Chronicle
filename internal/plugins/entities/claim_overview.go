package entities

import "github.com/keyxmakerx/chronicle/internal/plugins/campaigns"

// claim_overview.go holds the small, pure helpers that back the Player
// Character Claiming UI (PC-CLAIM-3): resolving owner display names for the
// show-page banner and the GM owner-overview roster. Kept out of the (large)
// handler file so the transformations are unit-testable in isolation.

// ownerDisplayNames builds an owner_user_id -> display name lookup from a
// campaign's members. Used to label claimed characters on the entity show page
// and in the GM owner-overview roster.
func ownerDisplayNames(members []campaigns.CampaignMember) map[string]string {
	names := make(map[string]string, len(members))
	for _, m := range members {
		names[m.UserID] = m.DisplayName
	}
	return names
}

// resolveOwnerName returns the display name for an entity's owner, or "" when
// the entity is unclaimed (ownerUserID == nil) or its owner is no longer a
// current campaign member (stale claim). Callers treat "" as "unknown owner".
func resolveOwnerName(ownerUserID *string, names map[string]string) string {
	if ownerUserID == nil {
		return ""
	}
	return names[*ownerUserID]
}
