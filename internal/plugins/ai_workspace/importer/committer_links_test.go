package importer

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// Two new pages that link each other both end with working links; a name that
// matches nothing is plain text and never a broken anchor.
func TestCommit_PageLinks(t *testing.T) {
	f := &fakeCreator{
		types: []entities.EntityType{{ID: 1, Name: "Character", Slug: "character", Enabled: true}},
		existing: map[string]*entities.Entity{
			"old-keep": {ID: "ent-old", CampaignID: "camp-1", Name: "Old Keep", Slug: "old-keep"},
		},
	}
	pages := []ParsedPage{
		page("Maro", "character", "Friend of @[Lyra], lives at @[Old Keep]."),
		page("Lyra", "character", "Knows @[Maro|the old sailor] and @[Ghost]."),
	}
	res, err := NewCommitter(f).Commit(context.Background(), "camp-1", CommitInput{
		OwnerID: "u",
		Pages:   pages,
		Decisions: []RowDecision{
			decision(true, "Maro", "character", "private", "rename"),
			decision(true, "Lyra", "character", "private", "rename"),
		},
	})
	if err != nil || res.Created != 2 {
		t.Fatalf("Created=%d err=%v rows=%+v", res.Created, err, res.Rows)
	}
	last := map[string]string{}
	for _, u := range f.updateEntries {
		last[u.EntityID] = u.EntryHTML
	}
	maro, lyra := last["ent-maro"], last["ent-lyra"]
	if !strings.Contains(maro, `data-mention-id="ent-lyra"`) || !strings.Contains(maro, `data-mention-id="ent-old"`) {
		t.Errorf("Maro's body lacks its links: %s", maro)
	}
	if !strings.Contains(lyra, `data-mention-id="ent-maro"`) || !strings.Contains(lyra, ">the old sailor</a>") {
		t.Errorf("Lyra's body lacks its link: %s", lyra)
	}
	if strings.Contains(lyra, "@[") || !strings.Contains(lyra, "Ghost.") || strings.Contains(lyra, "ent-ghost") {
		t.Errorf("unknown name should be plain text: %s", lyra)
	}
	if !strings.Contains(f.updateEntries[len(f.updateEntries)-1].EntryJSON, "data-entity-preview") {
		t.Errorf("editor JSON should keep the preview address")
	}
}

// A page the viewer cannot open is never linked.
func TestCommit_PageLinks_SkipsHiddenPage(t *testing.T) {
	f := &fakeCreator{
		types: []entities.EntityType{{ID: 1, Name: "Character", Slug: "character", Enabled: true}},
		existing: map[string]*entities.Entity{
			"vault": {ID: "ent-vault", CampaignID: "camp-1", Name: "Vault", Slug: "vault"},
		},
		hidden: map[string]bool{"ent-vault": true},
	}
	_, err := NewCommitter(f).Commit(context.Background(), "camp-1", CommitInput{
		OwnerID:   "u",
		Pages:     []ParsedPage{page("Maro", "character", "Guards @[Vault].")},
		Decisions: []RowDecision{decision(true, "Maro", "character", "private", "rename")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if h := f.updateEntries[0].EntryHTML; strings.Contains(h, "ent-vault") || !strings.Contains(h, "Guards Vault.") {
		t.Errorf("hidden page leaked into body: %s", h)
	}
}
