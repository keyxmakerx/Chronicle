package systems

import (
	"context"
	"strings"
)

// EntryPerson is a character page whose field names an entry of a kind:
// what it stores there (a name, as heroes keep it) and the traits it bought
// from that entry's list.
type EntryPerson struct {
	ID   string
	Name string
	// Hero is true for a character a player has claimed.
	Hero bool
	// Mine is true when the reader plays it.
	Mine bool
	// Value is what the character's field stores (the entry's name, or an
	// old or printed name).
	Value string
	// Traits are the names it bought from the entry's list.
	Traits []string
}

// PeopleReader is who is looking, in the terms the character pages use:
// the account and its visibility role, so a page they may not see is never
// returned.
type PeopleReader struct {
	UserID string
	Role   int
}

// EntryPeople finds the campaign's characters by what their kind fields
// hold. Implemented at the composition root over the entities plugin, which
// this package cannot import, and always with the reader's own permissions.
type EntryPeople interface {
	// People lists up to limit characters the reader may see whose field
	// fieldKey is filled.
	People(ctx context.Context, campaignID, fieldKey string, reader PeopleReader, limit int) ([]EntryPerson, error)
	// Person returns one character the reader may see with what its field
	// fieldKey holds, or nil when there is none they may see.
	Person(ctx context.Context, campaignID, entityID, fieldKey string, reader PeopleReader) (*EntryPerson, error)
}

const (
	// maxPeopleScanned bounds how many characters one page looks through.
	maxPeopleScanned = 500
	// maxPeopleShown bounds each run of names under "In <campaign>".
	maxPeopleShown = 12
)

// personLink is one name under "In <campaign>".
type personLink struct {
	Name string
	URL  string
	// Preview is the page's preview address, for the hover card and peek.
	Preview string
}

// peopleView is the "In <campaign>" section: heroes first, then the other
// characters, each run drawn only when it has someone.
type peopleView struct {
	Heroes     []personLink
	MoreHeroes int
	Others     []personLink
	MoreOthers int
}

func (v *peopleView) empty() bool { return v == nil || (len(v.Heroes) == 0 && len(v.Others) == 0) }

// heroView is the hero a peek opens for: "<Short> has: <trait>, ...".
type heroView struct {
	ID     string
	Name   string
	Short  string
	Traits []tableItem
}

// shortName is how a peek calls a hero: the first word, unless that word is
// an article, then the whole name.
func shortName(name string) string {
	words := strings.Fields(name)
	if len(words) == 0 {
		return name
	}
	switch strings.ToLower(words[0]) {
	case "the", "a", "an":
		return name
	}
	return words[0]
}
