package quests

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// One wording per failure so the service and its tests agree.

func errInvalid(msg string) error { return apperror.NewValidation(msg) }

// errNotFound is used for a missing id and for one the viewer may not know
// exists, so a hidden page is not confirmed by a different error.
func errNotFound(what string) error { return apperror.NewNotFound(what) }

func errForbidden(msg string) error { return apperror.NewForbidden(msg) }

// apperrorInternal wraps a repository or adapter failure so no raw database
// error reaches a response.
func apperrorInternal(err error) error {
	return apperror.NewInternal(fmt.Errorf("quests: %w", err))
}

// passThrough keeps an apperror (NotFound from a repository lookup) as is and
// wraps anything else, so a raw database error never leaves the service.
func passThrough(err error) error {
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		return err
	}
	return apperrorInternal(err)
}

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }
