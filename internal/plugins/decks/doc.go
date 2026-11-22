// Package decks keeps a campaign's card decks: hero decks, party decks and
// event decks. A deck has cards, a card has copies (one row per physical
// card, never a weight), and each copy sits in one zone: the pile, shown to
// the Directors, a hero's hand, in play, used or gone.
//
// The server draws: a draw picks at random among the pile's copies with
// crypto randomness, and the pile has no stored order, so there is nothing to
// leak. Directors and players get different response types with no hidden
// field on the player ones; a deck, card or copy a player may not see answers
// exactly like a missing one.
package decks
