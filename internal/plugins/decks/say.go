package decks

import "context"

// Say sets or clears "the Director says so" on one card. The tests that name
// it hold while it is set, and it clears itself when a copy of the card leaves
// the pile.
func (s *service) Say(ctx context.Context, campaignID, deckID, cardID string, v Viewer, said bool) (DeckView, error) {
	r := s.newRequest(ctx, campaignID, v)
	d, err := r.directorDeck(deckID)
	if err != nil {
		return nil, err
	}
	if !validToken(cardID) || s.moments == nil {
		return nil, errNotFound("card")
	}
	ok, err := s.moments.SetSaid(ctx, campaignID, d.ID, cardID, said)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !ok {
		return nil, errNotFound("card")
	}
	s.poked(campaignID)
	return r.view(d)
}

// WhenOptions is what the card editor may offer for a test, with the words a
// test is said in. It is for Directors: a tag a test may name can be one no
// player can read.
func (s *service) WhenOptions(ctx context.Context, campaignID string, v Viewer) (WhenOptionsView, error) {
	if err := directorOnly(v); err != nil {
		return WhenOptionsView{}, err
	}
	out := WhenOptionsView{
		WhenOptions: WhenOptions{Heroes: []OptionHero{}, Fields: []NumberField{}, Trackers: []TrackerKindOption{}, Tags: []TagOption{}},
		Ops:         opPhrases, Shares: sharePhrases, TestKinds: testKindWords, MaxTests: MaxTests,
	}
	if s.facts == nil {
		return out, nil
	}
	o, err := s.facts.Options(ctx, campaignID)
	if err != nil {
		return WhenOptionsView{}, apperrorInternal(err)
	}
	out.Available = true
	if o.Heroes != nil {
		out.Heroes = o.Heroes
	}
	if o.Fields != nil {
		out.Fields = o.Fields
	}
	if o.Trackers != nil {
		out.Trackers = o.Trackers
	}
	if o.Tags != nil {
		out.Tags = o.Tags
	}
	return out, nil
}
