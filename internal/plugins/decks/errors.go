package decks

import (
	"errors"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// One wording per failure so the service and its tests agree.

func errInvalid(msg string) error { return apperror.NewValidation(msg) }

// errNotFound answers for a missing deck, card or copy and for one the viewer
// may not know exists, so a hidden thing is never confirmed by another error.
func errNotFound(what string) error { return apperror.NewNotFound(what) }

func errForbidden(msg string) error { return apperror.NewForbidden(msg) }

// errStale is a deck saved from an older version of itself.
func errStale() error {
	return apperror.NewConflict("this deck changed; reload it and try again")
}

// errMoved is a copy that another move claimed first.
func errMoved() error {
	return apperror.NewConflict("that card just moved; look again and try once more")
}

// nothingLeftWords is what a draw from an empty pile says, and what the draw
// view says before one is tried.
const nothingLeftWords = "Nothing left in this deck."

func errNothingLeft() error { return apperror.NewConflict(nothingLeftWords) }

// apperrorInternal wraps a repository or adapter failure so no raw database
// error reaches a response.
func apperrorInternal(err error) error {
	return apperror.NewInternal(fmt.Errorf("decks: %w", err))
}

// Repository-level outcomes the service turns into the errors above.
var (
	errDuplicate = errors.New("duplicate name")
	// errCardDuplicate is two cards of one new deck sharing a name.
	errCardDuplicate = errors.New("duplicate card name")
	// errEmptyPile is a draw with no copy to take.
	errEmptyPile = errors.New("pile is empty")
	// errNoDraws is a player draw with nothing left to spend.
	errNoDraws = errors.New("no draws left")
	// errShownFull is a draw that would put more than MaxShown copies in the
	// shown spot.
	errShownFull = errors.New("the shown spot is full")
	// errTargetFull, errTargetCopies and errCardCopies are a keep that would
	// pass the target deck's card, copy or per-card copy limit.
	errTargetFull   = errors.New("target deck has too many cards")
	errTargetCopies = errors.New("target deck has too many copies")
	errCardCopies   = errors.New("target card has too many copies")
	// errNoneFit is a draw from a pile that holds copies, none of which the
	// plan allows.
	errNoneFit = errors.New("no card in the pile fits")
	// errMomentGone is an answer to a moment that is no longer open, or that
	// another answer closed first.
	errMomentGone = errors.New("the moment is no longer open")
	// errLostClaim is a move whose copy was no longer where it was read.
	errLostClaim = errors.New("copy moved first")
)

// heldError is a copies change that would take cards out of hands or play.
type heldError struct{ N int }

func (e *heldError) Error() string { return fmt.Sprintf("%d copies are held", e.N) }

// errTaken is a name another deck or card already has.
func errTaken(what string) error {
	return apperror.NewConflict("a " + what + " with that name already exists")
}

func errConflictMsg(msg string) error { return apperror.NewConflict(msg) }
