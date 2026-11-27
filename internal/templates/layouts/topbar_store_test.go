package layouts

import (
	"context"
	"testing"
)

// The Store icon is for the campaign's owner in their own view of the
// campaign, and stays off where they put it away. Everyone else gets none:
// a co-DM, a player, a guest, an owner looking as a player or a guest, an
// owner in Site admin and a signed-out visitor.
func TestTopbarHasStore_PerViewer(t *testing.T) {
	owner := func() context.Context { return viewer(3, true, false, false, false) }
	siteAdmin := SetActivePath(SetIsAdmin(owner(), true), "/admin/users")
	tests := []struct {
		name string
		ctx  context.Context
		may  bool // the bar carries the icon at all, shown or put away
		has  bool // the icon is on the bar
	}{
		{"owner", owner(), true, true},
		{"owner who chose shown", SetViewPrefs(owner(), &ViewPrefsData{}), true, true},
		{"owner who put it away here", SetViewPrefs(owner(), &ViewPrefsData{StoreHidden: []string{"camp-2", "camp-1"}}), true, false},
		{"owner who put it away in another campaign only", SetViewPrefs(owner(), &ViewPrefsData{StoreHidden: []string{"camp-2"}}), true, true},
		{"owner who put the brush away keeps the Store", SetViewPrefs(owner(), &ViewPrefsData{BrushHidden: []string{"camp-1"}}), true, true},
		{"owner viewing as a player", viewer(1, true, false, true, false), false, false},
		{"owner previewing as a guest", SetPreviewAs(owner(), "guest"), false, false},
		{"owner in Site admin", siteAdmin, false, false},
		{"co-DM", viewer(1, false, true, false, false), false, false},
		{"player", viewer(1, false, false, false, false), false, false},
		{"guest", viewer(1, false, false, false, true), false, false},
		{"signed out", SetCampaignID(context.Background(), "camp-1"), false, false},
		{"signed in outside a campaign", SetIsOwner(SetIsAuthenticated(context.Background(), true), true), false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := topbarMayStore(tc.ctx); got != tc.may {
				t.Errorf("topbarMayStore = %v, want %v", got, tc.may)
			}
			if got := topbarHasStore(tc.ctx); got != tc.has {
				t.Errorf("topbarHasStore = %v, want %v", got, tc.has)
			}
		})
	}
}

// The Store counts toward the shelf like any other tool, so an owner who put
// it away has one tool fewer.
func TestTopbarToolCount_CountsTheStore(t *testing.T) {
	shown := topbarToolCount(viewer(3, true, false, false, false))
	hidden := topbarToolCount(SetViewPrefs(viewer(3, true, false, false, false), &ViewPrefsData{StoreHidden: []string{"camp-1"}}))
	if shown != hidden+1 {
		t.Errorf("tool count shown = %d, put away = %d; want one apart", shown, hidden)
	}
}
