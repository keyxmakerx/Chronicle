package entities

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// identity_edit.go is the one narrow door that lets a Player-role member edit
// a page the fields, metadata, entry and image routes otherwise reserve for
// Scribes: their own claimed character's identity, the picks the hero builder
// offers them, and the hero's description and picture. Everything outside
// this file still needs Scribe, so widening the door means editing this file,
// nowhere else.

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

// builderPicks is what the hero builder lets a player choose: for each step's
// field key, the names on that step's list, keyed by their lower-case form.
type builderPicks map[string]map[string]bool

// builderPicksFrom turns the campaign's creator plan, as a player sees it,
// into builderPicks. A nil plan gives none, so the door stays at identity.
func builderPicksFrom(plan *HeroPlan) builderPicks {
	out := builderPicks{}
	if plan == nil {
		return out
	}
	for _, st := range plan.Steps {
		names := make(map[string]bool, len(st.Choices))
		for _, ch := range st.Choices {
			names[strings.ToLower(strings.TrimSpace(ch.Name))] = true
		}
		out[st.FieldKey] = names
	}
	return out
}

// needsBuilderPicks reports whether a patch reaches past the identity keys,
// so the handler loads the creator plan only when it can matter.
func needsBuilderPicks(patch map[string]any) bool {
	for k := range patch {
		if !ownerIdentityFieldKeys[k] {
			return true
		}
	}
	return false
}

// authorizeFieldsWrite decides whether a fields PUT may proceed. Scribes pass
// untouched. A claimed owner may send only fields_patch; fields_data replaces
// the whole map, so it is never allowed to a Player, and one disallowed key
// rejects the whole request rather than being silently dropped. Each key must
// be in ownerIdentityFieldKeys, which take short free text, or be a hero
// builder step in picks, which takes only a name from that step's list, so a
// player can choose their class but never type a stat into it. A key the type
// marks GM-only is refused either way, since the owner may not see it, and
// every value must be a string or null.
func authorizeFieldsWrite(role campaigns.Role, entity *Entity, userID, campaignID string, hasFieldsData bool, patch map[string]any, typeFields []FieldDefinition, picks builderPicks) error {
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
		listed, isStep := picks[k]
		if gmOnly[k] || (!ownerIdentityFieldKeys[k] && !isStep) {
			return apperror.NewForbidden("you can only change your character's identity fields")
		}
		switch val := v.(type) {
		case nil:
		case string:
			if utf8.RuneCountInString(val) > maxOwnerIdentityValue {
				return apperror.NewBadRequest("that value is too long")
			}
			if !ownerIdentityFieldKeys[k] && !listed[strings.ToLower(strings.TrimSpace(val))] {
				return apperror.NewBadRequest(fmt.Sprintf("%q is not on the %s list", val, k))
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

// CanEditOwnPage reports whether the viewer may write the page's description
// and change its picture: any Scribe or above, or the player who claimed it.
// The player shapes their own hero; the Director keeps the numbers, the
// GM-only parts and who can see the page.
func CanEditOwnPage(role campaigns.Role, entity *Entity, userID string) bool {
	return CanEditIdentity(role, entity, userID)
}

// authorizeOwnPageWrite decides whether a description or picture write may
// proceed: Scribes pass; below that only the claimed owner, on a page of
// this campaign. An edit grant on someone else's page is not enough, so the
// route opening to players does not widen what those grants reach.
func authorizeOwnPageWrite(role campaigns.Role, entity *Entity, userID, campaignID string) error {
	if role >= campaigns.RoleScribe {
		return nil
	}
	if entity.CampaignID != campaignID || !IsClaimedOwnerPlayer(role, entity, userID) {
		return apperror.NewForbidden("insufficient permissions")
	}
	return nil
}

// ownerEntryWrite shapes a claimed owner's description save. The player's
// editor loads the text with the Director's parts taken out (secrets, GM-only
// pictures and diagrams, table rollers), so saving it over a text that has
// them would erase them: such a text stays the Director's to change. What the
// player sends is stripped of those parts too, since they could never read
// back text marked for the Director.
func ownerEntryWrite(stored *Entity, entryJSON, entryHTML string) (string, string, error) {
	if entryHasDirectorParts(derefStr(stored.Entry), derefStr(stored.EntryHTML)) {
		return "", "", apperror.NewForbidden("this description has parts only the game master can see, so only they can change it")
	}
	return sanitize.StripSecretsJSON(entryJSON), sanitize.StripSecretsHTML(entryHTML), nil
}

// entryHasDirectorParts reports whether a stored text holds anything a player
// is not shown. The HTML strip only removes, so any change means a part was
// there; the JSON strip re-encodes, so it is compared with a plain re-encode.
func entryHasDirectorParts(entryJSON, entryHTML string) bool {
	if entryHTML != "" && sanitize.StripSecretsHTML(entryHTML) != entryHTML {
		return true
	}
	if entryJSON == "" {
		return false
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(entryJSON), &doc); err != nil {
		return false
	}
	plain, err := json.Marshal(doc)
	if err != nil {
		return false
	}
	return sanitize.StripSecretsJSON(entryJSON) != string(plain)
}
