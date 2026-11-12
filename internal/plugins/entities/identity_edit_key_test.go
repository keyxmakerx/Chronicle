package entities

import (
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// A refusal names the offending key so a player's AI can drop it and resend.
func TestAuthorizeFieldsWrite_NamesTheRefusedKey(t *testing.T) {
	uid := "u1"
	e := &Entity{CampaignID: "c1", OwnerUserID: &uid}
	err := authorizeFieldsWrite(campaigns.RolePlayer, e, uid, "c1", false,
		map[string]any{"race": "Elf", "stamina": "40"}, nil, builderPicks{})
	if err == nil || !strings.Contains(err.Error(), `"stamina"`) {
		t.Fatalf("want a refusal naming stamina, got %v", err)
	}
}
