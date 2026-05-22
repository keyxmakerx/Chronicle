// safeident.go provides safe SQL identifier (table / column / index name)
// interpolation for DDL statements.
//
// Background per cordinator/reports/chronicle/2026-05-22-c-security-audit.md §2 M-2:
// MySQL/MariaDB does NOT support `?` placeholders for identifiers; DDL statements
// like `DROP TABLE` MUST interpolate the identifier into the SQL string. Chronicle's
// migration runner does this today by trusting that the identifier came from a
// safe source (e.g. SHOW TABLES). That convention is fragile — a future caller
// copying the pattern might interpolate an untrusted identifier without honoring
// the convention.
//
// SafeIdent validates the identifier against a conservative regex matching the
// shape of legitimate Chronicle table/column names + wraps the result in MySQL
// backtick-quotes. Callers that need to interpolate identifiers into DDL MUST
// pass through SafeIdent first; a non-nil error means the identifier should NOT
// be used.

package database

import (
	"fmt"
	"regexp"
)

// safeIdentRe matches identifiers that are safe to interpolate into MySQL DDL
// after backtick-quoting:
//   - Leading letter or underscore (no leading digit; MySQL accepts but
//     uncommon and a useful tripwire for accidentally-numeric input).
//   - Subsequent letters / digits / underscores only.
//
// This is conservative — MySQL technically allows additional characters in
// quoted identifiers, but accepting them here would defeat the helper's
// purpose. Chronicle's schema follows this convention throughout.
var safeIdentRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// SafeIdent validates the given string as a SQL identifier and returns it
// backtick-quoted for safe interpolation into DDL strings. Returns an error
// if the identifier doesn't match the conservative shape — callers MUST NOT
// fall back to raw interpolation on error.
//
// Use:
//
//	quoted, err := database.SafeIdent(tableName)
//	if err != nil { return err }
//	_, err = db.ExecContext(ctx, "DROP TABLE IF EXISTS "+quoted)
//
// The helper intentionally returns an error rather than sanitizing-and-
// continuing so that an invalid identifier surfaces loudly rather than
// silently mutating what the caller thought they were operating on.
func SafeIdent(s string) (string, error) {
	if s == "" {
		return "", fmt.Errorf("safe identifier: empty input")
	}
	if !safeIdentRe.MatchString(s) {
		return "", fmt.Errorf("safe identifier: %q does not match %s", s, safeIdentRe.String())
	}
	return "`" + s + "`", nil
}
