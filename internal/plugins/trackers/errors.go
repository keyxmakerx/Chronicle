package trackers

import (
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// One wording per failure so the service and its tests agree.

func errInvalid(msg string) error { return apperror.NewValidation(msg) }

// errNotFound is used for a missing page and for one the viewer may not know
// exists, so a hidden page is not confirmed by a different error.
func errNotFound(what string) error { return apperror.NewNotFound(what) }

func errForbidden(msg string) error { return apperror.NewForbidden(msg) }

func errConflict() error {
	return apperror.NewConflict("this was changed by someone else; reload and try again")
}

// apperrorInternal wraps a repository or adapter failure so no raw database
// error reaches a response.
func apperrorInternal(err error) error {
	return apperror.NewInternal(fmt.Errorf("trackers: %w", err))
}

// errManaged refuses a hand edit of a kind another plugin keeps in step.
func errManaged() error {
	return apperror.NewValidation("this tracker kind comes from a homebrew recipe; change it in the Homebrew workshop")
}
