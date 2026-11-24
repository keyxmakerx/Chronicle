package entities

import "strings"

// IsHeroPage reports whether a character page is a hero rather than an NPC:
// a player has it, the owner marked it playable (a hero nobody has claimed
// yet), it is of the player character sub-type, or it names one of the
// hero's picks (pickKeys: the game system's class and hero choices, such as
// ancestry or kit). The Foundry module applies the same test when it decides
// between a hero and an NPC actor, so both sides agree on who is a hero.
func IsHeroPage(e *Entity, pcType bool, pickKeys []string) bool {
	if e == nil {
		return false
	}
	if isPlayersCharacter(e) || pcType {
		return true
	}
	for _, k := range pickKeys {
		if s, ok := e.FieldsData[k].(string); ok && strings.TrimSpace(s) != "" {
			return true
		}
	}
	return false
}

// IsPlayerCharacterType reports whether a page type is the player character
// sub-type.
func IsPlayerCharacterType(t EntityType) bool { return isPCType(t) }
