package apperror

import "fmt"

// MaxNameLength is the maximum allowed length for name/title fields.
const MaxNameLength = 200

// MaxDescriptionLength is the maximum allowed length for description fields.
const MaxDescriptionLength = 5000

// MaxColorLength is the maximum allowed length for CSS color values.
const MaxColorLength = 20

// MaxIconLength is the maximum allowed length for icon identifier strings.
const MaxIconLength = 50

// ValidateStringLength checks that a string field does not exceed the given
// maximum length. Returns a user-facing BadRequest error if the value is too
// long. Designed for handler-level input validation before calling services.
func ValidateStringLength(field, value string, maxLen int) error {
	if len(value) > maxLen {
		return NewBadRequest(fmt.Sprintf("%s is too long (max %d characters)", field, maxLen))
	}
	return nil
}

// ValidateRequired checks that a string field is not empty.
func ValidateRequired(field, value string) error {
	if value == "" {
		return NewBadRequest(field + " is required")
	}
	return nil
}
