package homebrew

import (
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

func errInvalid(msg string) error { return apperror.NewValidation(msg) }

func errNotFound(what string) error { return apperror.NewNotFound(what) }

func errForbidden(msg string) error { return apperror.NewForbidden(msg) }

func errConflict() error {
	return apperror.NewConflict("this recipe was changed by someone else; reload and try again")
}

// errInternal wraps a repository failure so no raw database error reaches a
// response.
func errInternal(err error) error {
	return apperror.NewInternal(fmt.Errorf("homebrew: %w", err))
}
