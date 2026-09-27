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

// iconNamePattern is a bare Font Awesome name: "fa-" then lowercase letters,
// digits and hyphens. Icons are stored without a style prefix; templates and
// scripts add "fa-solid" themselves.
var iconNamePattern = regexp.MustCompile(`^fa-[a-z0-9-]+$`)

// IsIconName reports whether s is an acceptable stored icon name. Icon names
// are placed into class attributes, so anything outside the allowlist is
// refused rather than escaped.
func IsIconName(s string) bool {
	return len(s) <= MaxIconLength && iconNamePattern.MatchString(s)
}

// ValidateIcon trims raw and checks it. An empty result is returned as ""
// with no error so callers can apply their own default or keep the existing
// value; anything else that fails IsIconName is a 400.
func ValidateIcon(raw string) (string, error) {
	icon := strings.TrimSpace(raw)
	if icon == "" || IsIconName(icon) {
		return icon, nil
	}
	return "", apperror.NewBadRequest("icon must be a Font Awesome icon name such as fa-book")
}

// IconOrDefault is for imports, where one bad row must not fail the whole
// run: it returns the trimmed icon when valid, otherwise def. replaced is true
// when a non-empty value was discarded, so the caller can note it.
func IconOrDefault(raw, def string) (icon string, replaced bool) {
	icon = strings.TrimSpace(raw)
	if icon == "" {
		return def, false
	}
	if IsIconName(icon) {
		return icon, false
	}
	return def, true
}
