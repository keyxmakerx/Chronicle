package systems

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNormalizeTermKey(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Slowed", "slowed"},
		{"  Power Roll ", "power-roll"},
		{"save   ends", "save-ends"},
		{"Save\tEnds\n", "save-ends"},
		{"already-hyphenated", "already-hyphenated"},
		{"", ""},
	}
	for _, tc := range tests {
		if got := NormalizeTermKey(tc.in); got != tc.want {
			t.Errorf("NormalizeTermKey(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPlainTermText(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"plain stays", "A slowed creature's speed is 2.", "A slowed creature's speed is 2."},
		{"reference markup keeps shown words", "Ends at {@duration end of turn|the end of the turn}.", "Ends at the end of the turn."},
		{"reference markup without label", "They are {@condition slowed}.", "They are slowed."},
		{"book term markup", "See [[power roll|power rolls]] and [[edge]].", "See power rolls and edge."},
		{"html tags dropped", "<p>Takes a <b>bane</b>.</p><script>alert(1)</script>", "Takes a bane."},
		{"entities decoded", "Tom &amp; Jerry &lt;3", "Tom & Jerry <3"},
		{"emphasis markers dropped", "A **bold** and `coded` word", "A bold and coded word"},
		{"whitespace collapsed", "one\n\n  two\tthree", "one two three"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := PlainTermText(tc.in); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// termsSystem writes a system folder with the given book index and glossary.
func termsSystem(t *testing.T, bookYAML, glossary string) string {
	t.Helper()
	dir := t.TempDir()
	if bookYAML != "" {
		if err := os.MkdirAll(filepath.Join(dir, "book"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "book", "book.yaml"), []byte(bookYAML), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if glossary != "" {
		if err := os.MkdirAll(filepath.Join(dir, "data"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "data", "rules-glossary.json"), []byte(glossary), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLoadRulesTerms(t *testing.T) {
	const withGlossary = "title: Rules\nglossary: rules-glossary.json\nparts: []\n"
	const entries = `[
	  {"slug":"slowed","name":"Slowed","summary":"Speed is <b>2</b> &amp; no shift.","properties":{"category":"Condition"}},
	  {"slug":"save-ends","name":"Save Ends","description":"Roll at end of turn; see {@condition slowed|slowed}.","properties":{"category":"duration"}},
	  {"slug":"hex","name":"Hex","summary":"Director only.","properties":{"category":"secrets"}},
	  {"slug":"odd","name":"Odd Props","summary":"Properties are not a map.","properties":["x"]},
	  {"name":"","summary":"nameless"},
	  {"name":"Empty","summary":""}
	]`
	manifest := &SystemManifest{Categories: []CategoryDef{{Slug: "secrets", Director: true}}}

	tests := []struct {
		name        string
		book        string
		glossary    string
		wantPresent bool
		wantTerms   map[string]RulesTerm
		wantErr     bool
	}{
		{
			name: "glossary becomes plain terms keyed by normalised name", book: withGlossary, glossary: entries, wantPresent: true,
			wantTerms: map[string]RulesTerm{
				"slowed":    {Name: "Slowed", Category: "condition", Text: "Speed is 2 & no shift."},
				"save-ends": {Name: "Save Ends", Category: "duration", Text: "Roll at end of turn; see slowed."},
				"odd":       {Name: "Odd Props", Category: "term", Text: "Properties are not a map."},
				"odd-props": {Name: "Odd Props", Category: "term", Text: "Properties are not a map."},
			},
		},
		{name: "no book", book: "", wantPresent: false, wantTerms: map[string]RulesTerm{}},
		{name: "book without glossary", book: "title: Rules\nparts: []\n", wantPresent: false, wantTerms: map[string]RulesTerm{}},
		{name: "declared glossary file missing", book: withGlossary, glossary: "", wantErr: true},
		{name: "glossary not a list", book: withGlossary, glossary: `{"a":1}`, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := termsSystem(t, tc.book, tc.glossary)
			got, err := LoadRulesTerms(dir, manifest)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want an error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Present != tc.wantPresent {
				t.Errorf("Present = %v, want %v", got.Present, tc.wantPresent)
			}
			if !reflect.DeepEqual(got.Terms, tc.wantTerms) {
				t.Errorf("terms = %+v\nwant    %+v", got.Terms, tc.wantTerms)
			}
		})
	}
}

// A word in a Director-only category never reaches the builder, whoever asks.
func TestLoadRulesTermsLeavesOutDirectorOnly(t *testing.T) {
	dir := termsSystem(t, "title: Rules\nglossary: rules-glossary.json\nparts: []\n",
		`[{"slug":"hex","name":"Hex","summary":"Secret.","properties":{"category":"secrets"}}]`)
	got, err := LoadRulesTerms(dir, &SystemManifest{Categories: []CategoryDef{{Slug: "secrets", Director: true}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Terms) != 0 || !got.Present {
		t.Errorf("got %+v, want a present glossary with no words", got)
	}
}
