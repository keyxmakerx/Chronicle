package entities

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// folderScopeRepo answers FindByID from a fixed set of folders; the moves
// under test read nothing else before the check.
type folderScopeRepo struct {
	SidebarNodeRepository
	nodes map[string]*SidebarNode
}

func (r folderScopeRepo) FindByID(_ context.Context, id string) (*SidebarNode, error) {
	if n, ok := r.nodes[id]; ok {
		return n, nil
	}
	return nil, apperror.NewNotFound("sidebar node not found")
}

// A page or folder may only be moved into a folder of its own campaign.
func TestFolderInCampaign(t *testing.T) {
	h := &Handler{sidebarNodeRepo: folderScopeRepo{nodes: map[string]*SidebarNode{
		"mine":   {ID: "mine", CampaignID: "c1"},
		"theirs": {ID: "theirs", CampaignID: "c2"},
	}}}
	str := func(s string) *string { return &s }
	tests := []struct {
		name   string
		nodeID *string
		ok     bool
	}{
		{"no folder (root or a parent page)", nil, true},
		{"empty folder id", str(""), true},
		{"own campaign's folder", str("mine"), true},
		{"another campaign's folder", str("theirs"), false},
		{"unknown folder", str("nope"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := h.folderInCampaign(context.Background(), "c1", tt.nodeID)
			if (err == nil) != tt.ok {
				t.Fatalf("folderInCampaign(%v) error = %v, want ok=%v", tt.nodeID, err, tt.ok)
			}
		})
	}
	if err := (&Handler{}).folderInCampaign(context.Background(), "c1", str("mine")); err == nil {
		t.Error("with no folder store, a folder id must be refused")
	}
}
