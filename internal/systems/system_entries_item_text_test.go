package systems

import (
	"strings"
	"testing"
)

func TestCleanItemText(t *testing.T) {
	tests := []struct {
		name, in string
		changed  bool
	}{
		{"plain prose", "It's Tom & Jerry.", false},
		{"a bare less-than", "Range < 5 squares", false},
		{"allowed markup", "<b>Bold</b> move", false},
		{"secret span", `<span data-secret="">only the Director</span> ok`, false},
		{"event handler", `<img src="x" onerror="alert(1)">hi`, true},
		{"script tag", `before<script>alert(1)</script>after`, true},
		{"removed markup beside kept markup and entities", `<script>x</script>it's & "q" <b>a</b>`, true},
		{"removed attribute around an entity", `<p onclick=x>Tom &amp; Jerry</p>`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := cleanItemText(tt.in)
			if changed != tt.changed {
				t.Fatalf("changed = %v, want %v (got %q)", changed, tt.changed, got)
			}
			if !changed && got != tt.in {
				t.Fatalf("unchanged text was rewritten: %q", got)
			}
			if lower := strings.ToLower(got); strings.Contains(lower, "onerror") || strings.Contains(lower, "<script") {
				t.Fatalf("markup survived: %q", got)
			}
			if again, did := cleanItemText(got); did || again != got {
				t.Fatalf("a second pass changed %q to %q", got, again)
			}
		})
	}
}

func TestNormalizeEntry_CleansItemTexts(t *testing.T) {
	e := &SystemEntry{
		Name:       "Ancestry",
		Visibility: EntryVisibilityEveryone,
		Properties: map[string]any{
			"signature_traits": []any{
				map[string]any{"name": "Keen", "description": `<img src="x" onerror="alert(1)">Sharp eyes`},
				map[string]any{"name": "Calm", "description": "It's fine & dandy"},
			},
		},
	}
	if err := normalizeEntry(e); err != nil {
		t.Fatalf("normalizeEntry: %v", err)
	}
	items := e.Properties["signature_traits"].([]any)
	if d := items[0].(map[string]any)["description"].(string); strings.Contains(d, "onerror") || !strings.Contains(d, "Sharp eyes") {
		t.Errorf("first item text = %q, want the handler gone and the words kept", d)
	}
	if d := items[1].(map[string]any)["description"].(string); d != "It's fine & dandy" {
		t.Errorf("plain item text = %q, want it as typed", d)
	}
}
