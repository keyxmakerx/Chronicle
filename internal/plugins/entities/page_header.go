package entities

// page_header.go decides what the system-page header shows about a claim. The
// header itself is page_header.templ; the behaviour is
// static/js/widgets/page_header.js.

// claimPillState says which claim marker the header shows.
type claimPillState int

const (
	claimNone      claimPillState = iota // nothing to show
	claimMine                            // "Yours"
	claimOther                           // "Claimed by <name>"
	claimNotYet                          // "Not claimed yet" (GM view)
	claimAvailable                       // the Claim button (player view)
)

// headerClaimState decides the claim marker. An existing claim always shows,
// whatever the addon says; the unclaimed states need the addon and a
// claimable type, matching what the server accepts on POST .../claim.
func headerClaimState(entity *Entity, userID string, isScribe, claimingEnabled, claimable bool) claimPillState {
	switch {
	case entity.OwnerUserID != nil && entity.IsOwnedBy(userID):
		return claimMine
	case entity.OwnerUserID != nil:
		return claimOther
	case !claimingEnabled || !claimable:
		return claimNone
	case isScribe:
		return claimNotYet
	default:
		return claimAvailable
	}
}
