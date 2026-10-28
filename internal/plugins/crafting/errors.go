package crafting

import (
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// One wording per failure so the service and its tests agree.

func errInvalid(msg string) error { return apperror.NewValidation(msg) }

// errNotFound is used for a missing recipe and for one the viewer may not
// know exists (a draft, a recipe they haven't learned), so a hidden recipe
// is not confirmed by a different error.
func errNotFound(what string) error { return apperror.NewNotFound(what) }

func errDirectorsOnly() error {
	return apperror.NewForbidden("only the campaign's Directors can change its crafting")
}

func errConflict() error {
	return apperror.NewConflict("this was changed by someone else; reload and try again")
}

func errNameTaken(what string) error {
	return apperror.NewConflict(fmt.Sprintf("there is already a %s with that name", what))
}

// errInternal wraps a repository or adapter failure so no raw database
// error reaches a response.
func errInternal(err error) error {
	return apperror.NewInternal(fmt.Errorf("crafting: %w", err))
}
