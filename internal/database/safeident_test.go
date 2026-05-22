package database

import (
	"strings"
	"testing"
)

// TestSafeIdent_Valid asserts the helper accepts canonical Chronicle
// identifiers + returns them backtick-quoted.
func TestSafeIdent_Valid(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"campaigns", "`campaigns`"},
		{"_internal_table", "`_internal_table`"},
		{"my_table_2", "`my_table_2`"},
		{"camelCase", "`camelCase`"},
		{"a", "`a`"},
		{"foundry_module_versions", "`foundry_module_versions`"},
	}
	for _, c := range cases {
		got, err := SafeIdent(c.in)
		if err != nil {
			t.Errorf("SafeIdent(%q) returned error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("SafeIdent(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestSafeIdent_Invalid asserts the helper rejects everything else with a
// descriptive error. Each subcase represents an attack vector or an
// accidentally-malformed identifier.
func TestSafeIdent_Invalid(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"leading_digit", "1table"},
		{"backtick", "table`"},
		{"backslash", "table\\name"},
		{"space", "table name"},
		{"single_quote", "table'name"},
		{"double_quote", `table"name`},
		{"semicolon_injection", "users; DROP TABLE users--"},
		{"comment_injection", "users/*comment*/"},
		{"dash_injection", "users--"},
		{"newline", "users\n"},
		{"null_byte", "users\x00"},
		{"unicode_homoglyph", "uѕers"}, // Cyrillic 'es' (U+0455) instead of Latin 's'
		{"dot_qualified", "schema.table"},
		{"sql_keyword_with_chars", "DROP TABLE"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := SafeIdent(c.in)
			if err == nil {
				t.Errorf("SafeIdent(%q) returned %q, want error", c.in, got)
			}
			if got != "" {
				t.Errorf("SafeIdent(%q) returned non-empty %q on error path", c.in, got)
			}
		})
	}
}

// TestSafeIdent_ErrorMessageMentionsInput pins that the error includes the
// rejected input so operators debugging a failed migration can see what was
// rejected without enabling Debug logs.
func TestSafeIdent_ErrorMessageMentionsInput(t *testing.T) {
	_, err := SafeIdent("1bad")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "1bad") {
		t.Errorf("error message %q should contain the rejected input", err.Error())
	}
}
