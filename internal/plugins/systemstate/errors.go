package systemstate

import (
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// The constructors keep one wording per failure so the service and tests agree.

func errInvalid(msg string) error { return apperror.NewValidation(msg) }

func errEntityNotFound() error { return apperror.NewNotFound("entity not found") }

func errSystemNotEnabled() error {
	return apperror.NewValidation("this game system is not enabled for the campaign")
}

// apperrorInternal wraps a repository failure so no raw database error
// reaches a response.
func apperrorInternal(err error) error {
	return apperror.NewInternal(fmt.Errorf("system state: %w", err))
}
