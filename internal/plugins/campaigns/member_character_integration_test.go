package campaigns

import (
	"context"
	"testing"
)

// The People page's Character column on a real database: a hero given to a
// player by owning its page shows up, the older per-member link still counts
// while it is valid, and pages that are not a character never do. Skipped
// under -short; same database setup as TestNavPins_Integration.
func TestListMembersCharacter_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openNavPinsTestDB(t)
	ctx := context.Background()
	repo := NewCampaignRepository(db)

	owner, ginko, howie, sivin, ada, rue, kit := navPinsUUID(t), navPinsUUID(t), navPinsUUID(t), navPinsUUID(t), navPinsUUID(t), navPinsUUID(t), navPinsUUID(t)
	users := []string{owner, ginko, howie, sivin, ada, rue, kit}
	campaignID := navPinsUUID(t)
	for _, u := range users {
		navPinsExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
			u, "memchar-"+u+"@example.test", "Member "+u[:8], "x")
	}
	navPinsExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
		campaignID, "Member Character Int", "memchar-"+campaignID[:8], owner)
	navPinsExec(t, db, `INSERT INTO campaign_members (campaign_id, user_id, role) VALUES (?, ?, 'owner')`, campaignID, owner)
	for _, u := range users[1:] {
		navPinsExec(t, db, `INSERT INTO campaign_members (campaign_id, user_id, role) VALUES (?, ?, 'player')`, campaignID, u)
	}
	defer func() {
		navPinsExec(t, db, `DELETE FROM campaigns WHERE id = ?`, campaignID)
		for _, u := range users {
			navPinsExec(t, db, `DELETE FROM users WHERE id = ?`, u)
		}
	}()

	mkType := func(slug, preset string) int64 {
		var p any
		if preset != "" {
			p = preset
		}
		res, err := db.Exec(`INSERT INTO entity_types (campaign_id, slug, name, name_plural, preset_category) VALUES (?, ?, ?, ?, ?)`,
			campaignID, slug, slug, slug, p)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	hero, adventure := mkType("hero", ""), mkType("adventure", "adventures")

	// mk adds a page; age is how many days ago it was last edited.
	mk := func(typeID int64, name, ownerID string, playable bool, age int, extra string) string {
		id := navPinsUUID(t)
		var o any
		if ownerID != "" {
			o = ownerID
		}
		navPinsExec(t, db, `INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, created_by, owner_user_id, playable, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW() - INTERVAL ? DAY)`,
			id, campaignID, typeID, name, id[:8], owner, o, playable, age)
		if extra != "" {
			navPinsExec(t, db, `UPDATE entities SET `+extra+` WHERE id = ?`, id)
		}
		return id
	}

	// Ginko was given Tyne through the Characters page: owner set, no link.
	tyne := mk(hero, "Tyne", ginko, true, 3, "")
	// Howie owns two; the playable one wins over a more recently edited NPC
	// sidekick, and an owned adventure and a template never count.
	orrin := mk(hero, "Orrin", howie, true, 5, "")
	mk(hero, "Sidekick", howie, false, 0, "")
	mk(adventure, "Howie's quest", howie, true, 0, "")
	mk(hero, "Template hero", howie, true, 0, "is_template = 1")
	// Sivin has only the older per-member link, and the page is unowned.
	saatraaol := mk(hero, "Saatraaol", "", true, 1, "")
	navPinsExec(t, db, `UPDATE campaign_members SET character_entity_id = ? WHERE campaign_id = ? AND user_id = ?`, saatraaol, campaignID, sivin)
	// Ada's old link points at a page now owned by Rue: it is Rue's, and Ada
	// falls back to the page she owns. A trashed page of hers never counts.
	vex := mk(hero, "Vex", rue, true, 2, "")
	navPinsExec(t, db, `UPDATE campaign_members SET character_entity_id = ? WHERE campaign_id = ? AND user_id = ?`, vex, campaignID, ada)
	mk(hero, "Old Ada", ada, true, 0, "deleted_at = NOW()")
	brannoc := mk(hero, "Brannoc", ada, true, 4, "")
	// Kit's old link is to a hero since freed; the hero Kit owns now wins,
	// and it is hidden from the other players.
	freed := mk(hero, "Freed", "", true, 0, "")
	navPinsExec(t, db, `UPDATE campaign_members SET character_entity_id = ? WHERE campaign_id = ? AND user_id = ?`, freed, campaignID, kit)
	kestrel := mk(hero, "Kestrel", kit, true, 6, "is_private = 1")

	members, err := repo.ListMembers(ctx, campaignID)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][2]string{}
	for _, m := range members {
		var id, name string
		if m.CharacterEntityID != nil {
			id = *m.CharacterEntityID
		}
		if m.CharacterName != nil {
			name = *m.CharacterName
		}
		got[m.UserID] = [2]string{id, name}
	}

	cases := []struct {
		who      string
		user     string
		id, name string
	}{
		{"owner has no character", owner, "", ""},
		{"claimed hero shows", ginko, tyne, "Tyne"},
		{"playable owned hero beats sidekick, adventure and template", howie, orrin, "Orrin"},
		{"older member link still counts", sivin, saatraaol, "Saatraaol"},
		{"link to someone else's hero gives way to the owned one", ada, brannoc, "Brannoc"},
		{"owner of a linked page sees it", rue, vex, "Vex"},
		{"owned hero beats an old link to a freed one", kit, kestrel, "Kestrel"},
	}
	for _, c := range cases {
		t.Run(c.who, func(t *testing.T) {
			if g := got[c.user]; g != [2]string{c.id, c.name} {
				t.Errorf("character = %q %q, want %q %q", g[0], g[1], c.id, c.name)
			}
		})
	}

	// Another player reading the member list sees shared heroes and their
	// own, but not the name or id of a hidden one; a Director sees all.
	t.Run("hidden hero is kept from other players", func(t *testing.T) {
		asPlayer := append([]CampaignMember(nil), members...)
		RedactMemberCharacters(asPlayer, int(RolePlayer), ginko)
		for _, m := range asPlayer {
			switch m.UserID {
			case kit:
				if m.CharacterEntityID != nil || m.CharacterName != nil {
					t.Errorf("Kit's hidden hero reached Ginko: %v %v", m.CharacterEntityID, m.CharacterName)
				}
			case ginko, howie:
				if m.CharacterName == nil {
					t.Errorf("%s's shared or own hero was cleared", m.UserID)
				}
			}
		}
		asScribe := append([]CampaignMember(nil), members...)
		RedactMemberCharacters(asScribe, int(RoleScribe), ginko)
		// A co-DM's visibility role is Owner (CampaignContext.VisibilityRole).
		asCoDM := append([]CampaignMember(nil), members...)
		RedactMemberCharacters(asCoDM, int(RoleOwner), howie)
		asKit := append([]CampaignMember(nil), members...)
		RedactMemberCharacters(asKit, int(RolePlayer), kit)
		asOwner := append([]CampaignMember(nil), members...)
		RedactMemberCharacters(asOwner, int(RoleOwner), owner)
		for i := range members {
			if members[i].UserID != kit {
				continue
			}
			if asKit[i].CharacterName == nil || *asKit[i].CharacterName != "Kestrel" {
				t.Error("Kit lost sight of their own hero")
			}
			if asOwner[i].CharacterName == nil {
				t.Error("the Director lost sight of Kit's hero")
			}
			if asScribe[i].CharacterName != nil || asScribe[i].CharacterEntityID != nil {
				t.Error("Kit's hidden hero reached a Scribe")
			}
			if asCoDM[i].CharacterName == nil {
				t.Error("a co-DM lost sight of Kit's hero")
			}
		}
	})
}
