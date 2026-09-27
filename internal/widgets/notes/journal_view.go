package notes

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"
)

// CharacterLister names the characters a player has claimed, for the
// Journal's "Playing …" card. An adapter over the entities service satisfies
// it, so this widget never imports a plugin.
type CharacterLister interface {
	ClaimedCharacters(ctx context.Context, campaignID, userID string) ([]ClaimedCharacter, error)
}

// ClaimedCharacter is one character a player plays.
type ClaimedCharacter struct {
	ID       string
	Name     string
	TypeName string
}

// JournalView is what the Journal page needs from the server. The notes
// themselves load over the JSON routes, each behind the visibility gate, so
// nothing here reveals a note.
type JournalView struct {
	NoteID    string            // the note to open (deep link), when id-shaped
	IsGM      bool              // the campaign Owner or a co-DM
	Character *ClaimedCharacter // a player's character, for the claim card
}

// initials is the claim card's avatar text: the first letter of the first
// two words of a name, upper-cased.
func initials(name string) string {
	var out []rune
	for _, w := range strings.Fields(name) {
		r, _ := utf8.DecodeRuneInString(w)
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			continue
		}
		out = append(out, unicode.ToUpper(r))
		if len(out) == 2 {
			break
		}
	}
	if len(out) == 0 {
		return "?"
	}
	return string(out)
}
