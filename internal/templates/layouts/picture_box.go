package layouts

import (
	"strings"
	"unicode"
)

// Initials returns up to two capital letters for an avatar placeholder:
// first and last word of the name. It works on runes so a name that starts
// with a multi-byte letter is not cut mid-character.
func Initials(name string) string {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return "?"
	}
	first := []rune(parts[0])[0]
	if len(parts) == 1 {
		return string(unicode.ToUpper(first))
	}
	last := []rune(parts[len(parts)-1])[0]
	return string(unicode.ToUpper(first)) + string(unicode.ToUpper(last))
}
