package notes

import (
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// TestCanView pins the one read predicate every note route shares. The row
// that matters most: a player's private note is private from the GM too.
func TestCanView(t *testing.T) {
	owner := "u-owner"
	tests := []struct {
		name   string
		note   Note
		viewer permissions.Viewer
		camp   string
		want   bool
	}{
		{"owner sees their private note", Note{UserID: owner}, player(owner), "c1", true},
		{"a player cannot see someone's private note", Note{UserID: owner}, player("u-other"), "c1", false},
		{"the GM cannot see a player's private note", Note{UserID: owner}, gm("u-gm"), "c1", false},
		{"a co-DM (Owner tier via VisibilityRole) cannot either", Note{UserID: owner}, permissions.RequestViewer(permissions.RoleOwner, "u-codm"), "c1", false},
		{"party note: any member", Note{UserID: owner, IsShared: true}, player("u-other"), "c1", true},
		{"custom note: the named person", Note{UserID: owner, SharedWith: []string{"u-named"}}, player("u-named"), "c1", true},
		{"custom note: not someone else", Note{UserID: owner, SharedWith: []string{"u-named"}}, player("u-other"), "c1", false},
		{"custom note: not the GM unless named", Note{UserID: owner, SharedWith: []string{"u-named"}}, gm("u-gm"), "c1", false},
		{"GM note: the GM", Note{UserID: owner, SharedWithGM: true}, gm("u-gm"), "c1", true},
		{"GM note: a Scribe is not a GM", Note{UserID: owner, SharedWithGM: true}, permissions.RequestViewer(permissions.RoleScribe, "u-scribe"), "c1", false},
		{"GM note: another player", Note{UserID: owner, SharedWithGM: true}, player("u-other"), "c1", false},
		{"another campaign: never", Note{UserID: owner, IsShared: true}, player(owner), "c2", false},
		{"anonymous: never, even a party note", Note{UserID: owner, IsShared: true}, permissions.RequestViewer(permissions.RolePlayer, ""), "c1", false},
		{"system viewer (no user): never", Note{UserID: owner, IsShared: true}, permissions.SystemViewer(permissions.RoleOwner), "c1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := tt.note
			n.CampaignID = "c1"
			if got := n.CanView(tt.viewer, tt.camp); got != tt.want {
				t.Errorf("CanView = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestDerive pins how the three stored columns read as one audience, so rows
// written before the GM audience existed keep the audience they had.
func TestDerive(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name string
		note Note
		want Visibility
	}{
		{"nothing set is private", Note{}, VisibilityPrivate},
		{"is_shared is the party", Note{IsShared: true}, VisibilityParty},
		{"is_shared wins over a stale list", Note{IsShared: true, SharedWith: []string{"u"}}, VisibilityParty},
		{"a share list is custom", Note{SharedWith: []string{"u"}}, VisibilityCustom},
		{"the GM flag is gm", Note{SharedWithGM: true}, VisibilityGM},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := tt.note
			n.derive()
			if n.Visibility != tt.want {
				t.Errorf("Visibility = %q, want %q", n.Visibility, tt.want)
			}
		})
	}

	archived := Note{ArchivedAt: &now}
	archived.derive()
	if !archived.Archived {
		t.Error("a note with archived_at set must read as archived")
	}
}

// TestApplyVisibility checks each audience writes the columns so that derive
// and CanView agree with what was asked for.
func TestApplyVisibility(t *testing.T) {
	for _, v := range []Visibility{VisibilityPrivate, VisibilityGM, VisibilityParty, VisibilityCustom} {
		t.Run(string(v), func(t *testing.T) {
			n := Note{CampaignID: "c1", UserID: "u-owner", IsShared: true, SharedWith: []string{"stale"}, SharedWithGM: true}
			n.applyVisibility(v, []string{"u-named"})
			if n.Visibility != v {
				t.Fatalf("Visibility = %q after applying %q", n.Visibility, v)
			}
			if got := n.CanView(player("u-named"), "c1"); got != (v == VisibilityParty || v == VisibilityCustom) {
				t.Errorf("named player CanView = %v for %q", got, v)
			}
			if got := n.CanView(gm("u-gm"), "c1"); got != (v == VisibilityParty || v == VisibilityGM) {
				t.Errorf("GM CanView = %v for %q", got, v)
			}
			if v != VisibilityCustom && n.SharedWith != nil {
				t.Errorf("a %q note must not keep a share list, got %v", v, n.SharedWith)
			}
		})
	}
}

// TestStripOwnerOnly: a person the note is shared with keeps title and body
// but loses every field that changes who sees the note or where it lives.
func TestStripOwnerOnly(t *testing.T) {
	yes, title, parent, vis := true, "t", "f", VisibilityParty
	req := UpdateNoteRequest{
		Title: &title, IsShared: &yes, SharedWith: []string{"x"}, Visibility: &vis,
		Pinned: &yes, ParentID: &parent, Archived: &yes,
	}
	req.StripOwnerOnly()
	if req.Title == nil {
		t.Error("the title is not owner-only")
	}
	if req.IsShared != nil || req.SharedWith != nil || req.Visibility != nil || req.Pinned != nil || req.ParentID != nil || req.Archived != nil {
		t.Errorf("owner-only fields survived: %+v", req)
	}
}
