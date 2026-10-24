package entities

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// Every line under a heading in a built-in template is an empty line holding
// its prompt, so nothing a player could read is left on a new page.
func TestDefaultTemplates_PromptsAreHintsNotText(t *testing.T) {
	var walk func(t *testing.T, node map[string]any, prompts *int)
	walk = func(t *testing.T, node map[string]any, prompts *int) {
		if node["type"] == "paragraph" {
			attrs, _ := node["attrs"].(map[string]any)
			if p, _ := attrs["prompt"].(string); p == "" {
				t.Errorf("paragraph without a prompt: %v", node)
			}
			if c, ok := node["content"].([]any); ok && len(c) > 0 {
				t.Errorf("prompt line holds text: %v", node)
			}
			*prompts++
			return
		}
		kids, _ := node["content"].([]any)
		for _, k := range kids {
			if m, ok := k.(map[string]any); ok {
				walk(t, m, prompts)
			}
		}
	}
	for _, tmpl := range defaultContentTemplates("c1") {
		t.Run(tmpl.Name, func(t *testing.T) {
			var doc map[string]any
			if err := json.Unmarshal([]byte(tmpl.ContentJSON), &doc); err != nil {
				t.Fatalf("content JSON: %v", err)
			}
			prompts := 0
			walk(t, doc, &prompts)
			if prompts == 0 {
				t.Error("template has no prompts")
			}
			if html := sanitize.HTML(tmpl.ContentHTML); strings.Contains(html, "data-prompt") {
				t.Errorf("saved HTML keeps prompts: %s", html)
			}
		})
	}
}

type fakePromptStore struct{ rows map[int]string }

func (f *fakePromptStore) ReplaceTemplateContent(_ context.Context, oldJSON, newJSON, _ string) (int64, error) {
	var n int64
	for id, j := range f.rows {
		if j == oldJSON {
			f.rows[id] = newJSON
			n++
		}
	}
	return n, nil
}

// Only templates still exactly as first seeded change, and a second run
// changes nothing.
func TestReconcileTemplatePrompts(t *testing.T) {
	store := &fakePromptStore{rows: map[int]string{
		1: legacyLocationJSON,
		2: legacyQuestLogJSON,
		3: strings.Replace(legacyNpcProfileJSON, "Appearance", "Looks", 1), // edited by its owner
		4: locationJSON,
	}}
	n, err := ReconcileTemplatePrompts(context.Background(), store)
	if err != nil || n != 2 {
		t.Fatalf("first run changed %d (err %v), want 2", n, err)
	}
	if store.rows[1] != locationJSON || store.rows[2] != questLogJSON {
		t.Error("untouched seeds were not replaced")
	}
	if !strings.Contains(store.rows[3], "Looks") {
		t.Error("an edited template was overwritten")
	}
	if n, _ := ReconcileTemplatePrompts(context.Background(), store); n != 0 {
		t.Errorf("second run changed %d, want 0", n)
	}
}
