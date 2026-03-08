package extensions

import (
	"fmt"
	"regexp"
	"strings"
)

// tableNamePattern matches SQL table names (alphanumeric + underscores).
var tableNamePattern = regexp.MustCompile(`(?i)(?:CREATE\s+TABLE(?:\s+IF\s+NOT\s+EXISTS)?|ALTER\s+TABLE|DROP\s+TABLE(?:\s+IF\s+EXISTS)?)\s+` + "`?" + `(\w+)` + "`?")

// indexOnPattern matches CREATE/DROP INDEX ... ON table_name.
var indexOnPattern = regexp.MustCompile(`(?i)(?:CREATE(?:\s+UNIQUE)?\s+INDEX|DROP\s+INDEX)\s+` + "`?" + `\w+` + "`?" + `\s+ON\s+` + "`?" + `(\w+)` + "`?")

// ValidateExtensionSQL checks that SQL statements in an extension migration
// only operate on tables prefixed with ext_<slug>_. This prevents extensions
// from modifying core Chronicle tables. Returns an error describing the first
// violation found.
func ValidateExtensionSQL(slug, sql string) error {
	if slug == "" {
		return fmt.Errorf("extension slug is required")
	}

	requiredPrefix := "ext_" + slug + "_"

	// Split into individual statements.
	statements := splitStatements(sql)

	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}

		// Check CREATE/ALTER/DROP TABLE statements.
		if matches := tableNamePattern.FindStringSubmatch(stmt); len(matches) > 1 {
			tableName := strings.Trim(matches[1], "`")
			if !strings.HasPrefix(strings.ToLower(tableName), requiredPrefix) {
				return fmt.Errorf("table %q is not allowed: extension tables must be prefixed with %q", tableName, requiredPrefix)
			}
		}

		// Check CREATE/DROP INDEX ... ON table_name.
		if matches := indexOnPattern.FindStringSubmatch(stmt); len(matches) > 1 {
			tableName := strings.Trim(matches[1], "`")
			if !strings.HasPrefix(strings.ToLower(tableName), requiredPrefix) {
				return fmt.Errorf("index on table %q is not allowed: extension tables must be prefixed with %q", tableName, requiredPrefix)
			}
		}

		// Block INSERT/UPDATE/DELETE on non-extension tables.
		if err := checkDMLStatement(stmt, requiredPrefix); err != nil {
			return err
		}
	}

	return nil
}

// splitStatements splits SQL text by semicolons, respecting quoted strings.
func splitStatements(sql string) []string {
	var statements []string
	var current strings.Builder
	inSingleQuote := false
	inDoubleQuote := false

	for i := 0; i < len(sql); i++ {
		ch := sql[i]
		switch {
		case ch == '\'' && !inDoubleQuote:
			inSingleQuote = !inSingleQuote
			current.WriteByte(ch)
		case ch == '"' && !inSingleQuote:
			inDoubleQuote = !inDoubleQuote
			current.WriteByte(ch)
		case ch == ';' && !inSingleQuote && !inDoubleQuote:
			s := strings.TrimSpace(current.String())
			if s != "" {
				statements = append(statements, s)
			}
			current.Reset()
		default:
			current.WriteByte(ch)
		}
	}

	// Handle last statement without trailing semicolon.
	if s := strings.TrimSpace(current.String()); s != "" {
		statements = append(statements, s)
	}

	return statements
}

// dmlPattern matches INSERT/UPDATE/DELETE statements and extracts the table name.
var dmlPattern = regexp.MustCompile(`(?i)(?:INSERT\s+INTO|UPDATE|DELETE\s+FROM)\s+` + "`?" + `(\w+)` + "`?")

// checkDMLStatement verifies that INSERT/UPDATE/DELETE only target extension tables.
func checkDMLStatement(stmt, requiredPrefix string) error {
	matches := dmlPattern.FindStringSubmatch(stmt)
	if len(matches) <= 1 {
		return nil
	}
	tableName := strings.Trim(matches[1], "`")
	if !strings.HasPrefix(strings.ToLower(tableName), requiredPrefix) {
		return fmt.Errorf("DML on table %q is not allowed: extension operations must target tables prefixed with %q", tableName, requiredPrefix)
	}
	return nil
}
