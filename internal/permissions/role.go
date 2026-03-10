// Package permissions provides shared role constants and permission checks
// for use across services and repositories that cannot import the campaigns
// package due to circular dependency constraints.
//
// The campaigns package defines the canonical Role type and middleware.
// This package mirrors the role levels as plain int constants so that
// business logic outside the handler layer can reference named roles
// instead of magic numbers.
package permissions

// Role levels for campaign membership. These mirror the values in
// campaigns.Role but are plain ints to avoid circular imports.
const (
	RoleNone   = 0 // No membership (site admins viewing without joining)
	RolePlayer = 1 // Read access to permitted content
	RoleScribe = 2 // Create/edit access to notes, entities, events
	RoleOwner  = 3 // Full control, campaign ownership
)

// CanSeeDmOnly returns true if the role has permission to view dm_only content.
// Owners always can. Other roles can if they have been granted dm_only
// visibility via CampaignSettings.DmGrantIDs.
func CanSeeDmOnly(role int, dmGranted ...bool) bool {
	if role >= RoleOwner {
		return true
	}
	return len(dmGranted) > 0 && dmGranted[0]
}

// CanSetDmOnly returns true if the role has permission to create or toggle
// the dm_only flag on content (tags, relations, events, etc.).
// Only Owners can set dm_only; DM-granted users can view but not toggle.
func CanSetDmOnly(role int) bool {
	return role >= RoleOwner
}
