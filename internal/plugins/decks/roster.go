package decks

import "context"

// Roster is the campaign's hero pages and people, as a Director picks them on
// a deck: a hero for a hero deck, and the players a deck may be drawn by.
type Roster struct {
	Heroes  []Hero
	Members []Member
}

// Roster reads the hero pages and members for a Director. Anyone else gets the
// same answer as a deck that does not exist.
func (s *service) Roster(ctx context.Context, campaignID string, v Viewer) (Roster, error) {
	if !v.Director {
		return Roster{}, errNotFound("deck")
	}
	heroes, err := s.heroes.Party(ctx, campaignID, v)
	if err != nil {
		return Roster{}, apperrorInternal(err)
	}
	members, err := s.people.Members(ctx, campaignID)
	if err != nil {
		return Roster{}, apperrorInternal(err)
	}
	return Roster{Heroes: heroes, Members: members}, nil
}
