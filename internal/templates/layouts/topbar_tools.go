package layouts

import "context"

// The top bar's tools and who has each. The bar draws a real button for every
// tool the viewer has, and on a phone the Tools shelf lists the same set by
// name, so both read these answers and can never disagree about who has what.

// topbarHasOwnerAI reports whether the owner's AI button is on the bar.
func topbarHasOwnerAI(ctx context.Context) bool {
	return InCampaign(ctx) && IsOwner(ctx) && !IsViewingAsPlayer(ctx) && !InSiteAdmin(ctx)
}

// topbarHasMemberAI reports whether a member's AI button is on the bar. It
// starts hidden and shows only once the plugin says members may use AI here.
func topbarHasMemberAI(ctx context.Context) bool {
	return InCampaign(ctx) && !IsOwner(ctx) && !IsGuest(ctx) && GetCampaignRole(ctx) >= 1 && !InSiteAdmin(ctx)
}

// topbarHasFind reports whether the bar's search button is drawn.
func topbarHasFind(ctx context.Context) bool { return InCampaign(ctx) }

// topbarHasViewAs reports whether the eye (view as player) is on the bar. It
// stays while an owner views as a player or as a guest, to take them back.
func topbarHasViewAs(ctx context.Context) bool {
	return InCampaign(ctx) && (IsOwner(ctx) || IsDmGranted(ctx) || IsViewingAsPlayer(ctx) || PreviewAs(ctx) == "guest")
}

// topbarHasTable reports whether the Table tab is on the bar.
func topbarHasTable(ctx context.Context) bool {
	return InCampaign(ctx) && IsAuthenticated(ctx) && !InSiteAdmin(ctx) && GetCampaignRole(ctx) >= 1
}

// topbarHasBell reports whether the notification bell is on the bar.
func topbarHasBell(ctx context.Context) bool { return IsAuthenticated(ctx) }

// topbarHasDMScreen reports whether the DM Screen button is on the bar.
func topbarHasDMScreen(ctx context.Context) bool {
	return InCampaign(ctx) && (GetCampaignRole(ctx) >= 3 || IsDmGranted(ctx))
}

// topbarMayCustomize reports whether the viewer may open Customize here,
// the check Customize's own routes make (the campaign's owner), and is not
// looking at the campaign as a player or a guest. It decides whether the
// header carries the brush at all, shown or put away.
func topbarMayCustomize(ctx context.Context) bool {
	return InCampaign(ctx) && IsAuthenticated(ctx) && IsOwner(ctx) && !IsGuest(ctx) &&
		!IsViewingAsPlayer(ctx) && PreviewAs(ctx) == "" && !InSiteAdmin(ctx)
}

// topbarHasBrush reports whether the Customize brush is on the bar: the
// viewer may customize and has not hidden it in this campaign (their own
// display choice, kept per campaign).
func topbarHasBrush(ctx context.Context) bool {
	if !topbarMayCustomize(ctx) {
		return false
	}
	p := GetViewPrefs(ctx)
	if p == nil {
		return true
	}
	id := GetCampaignID(ctx)
	for _, h := range p.BrushHidden {
		if h == id {
			return false
		}
	}
	return true
}

// topbarToolCount is how many tools the shelf lists besides Find. A member's
// AI is left out: it appears only once the plugin allows it, so the bar must
// not depend on it to earn a shelf.
func topbarToolCount(ctx context.Context) int {
	n := 0
	for _, has := range []bool{
		topbarHasOwnerAI(ctx), topbarHasBrush(ctx), topbarHasTable(ctx), topbarHasDMScreen(ctx),
		topbarHasViewAs(ctx), topbarHasBell(ctx),
	} {
		if has {
			n++
		}
	}
	return n
}

// topbarShowsToolsShelf reports whether a phone's bar folds its tools into
// the Tools tab. A viewer with fewer than two tools keeps the bar as it is.
func topbarShowsToolsShelf(ctx context.Context) bool {
	return topbarToolCount(ctx) >= 2
}

// topbarViewAsLabel is the shelf's name for the eye.
func topbarViewAsLabel(ctx context.Context) string {
	if IsViewingAsPlayer(ctx) || PreviewAs(ctx) == "guest" {
		return "Back to your view"
	}
	return "View as player"
}
