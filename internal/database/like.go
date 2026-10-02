// like.go builds LIKE patterns from user-typed search text.

package database

import "strings"

// LikeEscapeChar is the ESCAPE character that queries built with
// ContainsPattern must declare (`LIKE ? ESCAPE '!'`). A non-backslash
// character is used so the pattern means the same under every sql_mode,
// including NO_BACKSLASH_ESCAPES.
const LikeEscapeChar = "!"

// ContainsPattern returns a "contains" LIKE pattern for s with the LIKE
// wildcards (% and _) and the escape character itself neutralised, so a
// search for "50%" matches that literal text instead of everything.
// The result is only ever bound as a placeholder argument.
func ContainsPattern(s string) string {
	r := strings.NewReplacer(
		LikeEscapeChar, LikeEscapeChar+LikeEscapeChar,
		"%", LikeEscapeChar+"%",
		"_", LikeEscapeChar+"_",
	)
	return "%" + r.Replace(s) + "%"
}
