// icon.go — allowlisting for stored Font Awesome icon names.
package sanitize

import (
	"regexp"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// MaxIconLength caps a stored icon name. It matches the narrowest icon column
// (VARCHAR(50)) so a value that passes here always fits.
const MaxIconLength = apperror.MaxIconLength

// maxIconInputLength bounds the raw value NormalizeIcon will look at. It
// leaves room for a style token and stray whitespace around a name at the
// cap; anything longer is refused before it is split.
const maxIconInputLength = 4 * MaxIconLength

// iconNamePattern is a bare Font Awesome name: "fa-" then lowercase letters,
// digits and hyphens. Icons are stored without a style prefix; templates and
// scripts add "fa-solid" themselves.
var iconNamePattern = regexp.MustCompile(`^fa-[a-z0-9-]+$`)

// iconStyleTokens are the style prefixes a client may send ahead of the name.
// They all mean the solid or regular set that templates already render, so
// dropping them loses nothing. Brand styles are a different icon set and
// stay refused.
var iconStyleTokens = map[string]bool{
	"fa-solid":   true,
	"fas":        true,
	"fa-regular": true,
	"far":        true,
	"fa":         true,
}

// IsIconName reports whether s is an acceptable stored icon name. Icon names
// are placed into class attributes, so anything outside the allowlist is
// refused rather than escaped.
func IsIconName(s string) bool {
	return len(s) <= MaxIconLength && iconNamePattern.MatchString(s)
}

// NormalizeIcon turns raw into the stored form of an icon name: surrounding
// whitespace is dropped, and "<style> <name>" with a known style token keeps
// only the name. ok is false when raw is empty or is not one icon name after
// that, including a name with more than one style token or extra classes.
func NormalizeIcon(raw string) (icon string, ok bool) {
	if len(raw) > maxIconInputLength {
		return "", false
	}
	var name string
	switch fields := strings.Fields(raw); len(fields) {
	case 1:
		name = fields[0]
	case 2:
		if !iconStyleTokens[fields[0]] {
			return "", false
		}
		name = fields[1]
	default:
		return "", false
	}
	if !IsIconName(name) {
		return "", false
	}
	return name, true
}

// isBlankIcon reports whether raw holds no icon at all, checking the length
// first so an over-long value is never scanned.
func isBlankIcon(raw string) bool {
	return len(raw) <= maxIconInputLength && strings.TrimSpace(raw) == ""
}

// ValidateIcon normalises raw and checks it. An empty or all-whitespace value
// is returned as "" with no error so callers can apply their own default or
// keep the existing value; anything else that does not normalise to an icon
// name is a 400.
func ValidateIcon(raw string) (string, error) {
	if isBlankIcon(raw) {
		return "", nil
	}
	if icon, ok := NormalizeIcon(raw); ok {
		return icon, nil
	}
	return "", apperror.NewBadRequest("icon must be a Font Awesome icon name such as fa-book")
}

// IconOrDefault is for imports, where one bad row must not fail the whole
// run: it returns the normalised icon when valid, otherwise def. replaced is
// true when a non-empty value was discarded, so the caller can note it.
func IconOrDefault(raw, def string) (icon string, replaced bool) {
	if isBlankIcon(raw) {
		return def, false
	}
	if icon, ok := NormalizeIcon(raw); ok {
		return icon, false
	}
	return def, true
}
