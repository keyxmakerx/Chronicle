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

func errNothingLeft() error { return apperror.NewConflict("Nothing left in this deck.") }

// apperrorInternal wraps a repository or adapter failure so no raw database
// error reaches a response.
func apperrorInternal(err error) error {
	return apperror.NewInternal(fmt.Errorf("decks: %w", err))
}

// Repository-level outcomes the service turns into the errors above.
var (
	errDuplicate = errors.New("duplicate name")
	// errEmptyPile is a draw with no copy to take.
	errEmptyPile = errors.New("pile is empty")
	// errNoDraws is a player draw with nothing left to spend.
	errNoDraws = errors.New("no draws left")
	// errShownTaken is a draw to shown while a copy already sits there.
	errShownTaken = errors.New("a card is already shown")
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
