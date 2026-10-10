package entities

import (
	"encoding/json"
	"sort"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// identity_edit.go is the one narrow door that lets a Player-role member edit
// a page the fields and metadata routes otherwise reserve for Scribes: their
// own claimed character's identity. Everything outside this file still needs
// Scribe, so widening the door means editing the constants below, nowhere else.

// maxOwnerIdentityValue bounds an identity value a player writes: a name such
// as "Dragon Knight", never a document.
const maxOwnerIdentityValue = 200

// ownerIdentityFieldKeys are the only field keys a claimed owner may change.
// They describe who the character is (ancestry, upbringing, calling), never
// rules-bearing numbers or GM-facing data, so a player cannot rewrite their
// own stats. Game systems name the same idea differently, hence the synonyms
// (race/species/heritage, career/kit).
var ownerIdentityFieldKeys = map[string]bool{
	"ancestry":   true,
	"culture":    true,
	"career":     true,
	"kit":        true,
	"race":       true,
	"species":    true,
	"heritage":   true,
	"background": true,
}

// OwnerIdentityFieldKeys returns the allowlist, sorted, for UI and docs.
func OwnerIdentityFieldKeys() []string {
	keys := make([]string, 0, len(ownerIdentityFieldKeys))
	for k := range ownerIdentityFieldKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// IsClaimedOwnerPlayer reports whether the viewer is a campaign member below
// Scribe who owns this entity. The claim addon's on/off state is irrelevant:
// an existing claim keeps its meaning if the addon is later switched off.
func IsClaimedOwnerPlayer(role campaigns.Role, entity *Entity, userID string) bool {
	return role >= campaigns.RolePlayer && role < campaigns.RoleScribe && entity.IsOwnedBy(userID)
}

// CanEditIdentity reports whether the viewer may edit the identity fields and
// rename the page: any Scribe or above, or the claimed owner.
func CanEditIdentity(role campaigns.Role, entity *Entity, userID string) bool {
	return role >= campaigns.RoleScribe || IsClaimedOwnerPlayer(role, entity, userID)
}

// authorizeFieldsWrite decides whether a fields PUT may proceed. Scribes pass
// untouched. A claimed owner may send only fields_patch whose every key is in
// ownerIdentityFieldKeys; fields_data replaces the whole map, so it is never
// allowed to a Player, and one disallowed key rejects the whole request rather
// than being silently dropped. A key the type marks GM-only is refused even if
// allowlisted, since the owner may not see it, and each value must be a short
// string or null so the narrow path cannot store anything else.
func authorizeFieldsWrite(role campaigns.Role, entity *Entity, userID, campaignID string, hasFieldsData bool, patch map[string]any, typeFields []FieldDefinition) error {
	if role >= campaigns.RoleScribe {
		return nil
	}
	if entity.CampaignID != campaignID || !IsClaimedOwnerPlayer(role, entity, userID) {
		return apperror.NewForbidden("insufficient permissions")
	}
	if hasFieldsData || len(patch) == 0 {
		return apperror.NewForbidden("you can only change your character's identity fields")
	}
	gmOnly := map[string]bool{}
	for i := range typeFields {
		if typeFields[i].GMOnly {
			gmOnly[typeFields[i].Key] = true
		}
	}
	for k, v := range patch {
		if !ownerIdentityFieldKeys[k] || gmOnly[k] {
			return apperror.NewForbidden("you can only change your character's identity fields")
		}
		switch val := v.(type) {
		case nil:
		case string:
			if utf8.RuneCountInString(val) > maxOwnerIdentityValue {
				return apperror.NewBadRequest("that value is too long")
			}
		default:
			return apperror.NewBadRequest("identity fields take text")
		}
	}
	return nil
}

// authorizeMetadataWrite decides whether a metadata PUT may proceed. A claimed
// owner may send the name and nothing else; descriptor, parent and anything
// unknown are refused so the narrow path cannot become a general edit.
func authorizeMetadataWrite(role campaigns.Role, entity *Entity, userID, campaignID string, rawBody []byte) error {
	if role >= campaigns.RoleScribe {
		return nil
	}
	if entity.CampaignID != campaignID || !IsClaimedOwnerPlayer(role, entity, userID) {
		return apperror.NewForbidden("insufficient permissions")
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(rawBody, &keys); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}
	name, ok := keys["name"]
	if !ok || string(name) == "null" || len(keys) != 1 {
		return apperror.NewForbidden("you can only rename your character")
	}
	return nil
}
