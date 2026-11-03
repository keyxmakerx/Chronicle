package transformations

import (
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

func errInvalid(msg string) error { return apperror.NewValidation(msg) }

// errNotFound is used for a missing layer or page and for one the viewer
// may not know exists, so a hidden one is not confirmed by a different error.
func errNotFound(what string) error { return apperror.NewNotFound(what) }

func errForbidden(msg string) error { return apperror.NewForbidden(msg) }

// apperrorInternal wraps a repository or adapter failure so no raw database
// error reaches a response.
func apperrorInternal(err error) error {
	return apperror.NewInternal(fmt.Errorf("transformations: %w", err))
}
