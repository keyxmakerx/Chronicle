package rolltables

import (
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// The constructors keep one wording per failure so the service and tests agree.

func errInvalid(msg string) error { return apperror.NewValidation(msg) }

// apperrorInternal wraps a repository failure so no raw database error
// reaches a response.
func apperrorInternal(err error) error {
	return apperror.NewInternal(fmt.Errorf("roll tables: %w", err))
}
