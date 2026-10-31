package entities

import (
	"errors"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// A mention keeps its page's name only for a reader who can open that page,
// and every given text is checked in one batch.
func TestHideUnviewableMentions(t *testing.T) {
	link := func(id, name string) string {
		return `<a data-mention-id="` + id + `" href="/campaigns/c1/entities/` + id + `">` + name + `</a>`
	}
	const seen, hidden = "aaaaaaaa-0000-4000-8000-00000000000a", "aaaaaaaa-0000-4000-8000-00000000000b"
	tests := []struct {
		name      string
		filterErr error
		wantN     int
		wantNames []string
		wantGone  []string
	}{
		{"hidden page renamed, seen page kept", nil, 2, []string{"Bram"}, []string{"Vampire Duke", "Secret Lair"}},
		{"lookup failure hides every mention", errors.New("db down"), 3, nil, []string{"Bram", "Vampire Duke", "Secret Lair"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			repo := &mockEntityRepo{filterViewableFn: func(ids []string) (map[string]bool, error) {
				calls++
				if tt.filterErr != nil {
					return nil, tt.filterErr
				}
				out := map[string]bool{}
				for _, id := range ids {
					out[id] = id == seen
				}
				return out, nil
			}}
			svc := newTestService(repo, &mockEntityTypeRepo{})
			a := `<p>` + link(seen, "Bram") + ` met ` + link(hidden, "Vampire Duke") + `</p>`
			b := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Secret Lair","marks":[{"type":"link","attrs":{"data-mention-id":"` + hidden + `"}}]}]}]}`
			n, err := svc.HideUnviewableMentions(t.Context(), "c1", 1, "u1", &a, nil, &b)
			if (err != nil) != (tt.filterErr != nil) {
				t.Fatalf("err = %v", err)
			}
			if n != tt.wantN || calls != 1 {
				t.Errorf("replaced %d in %d lookups, want %d in 1", n, calls, tt.wantN)
			}
			all := a + b
			for _, name := range tt.wantNames {
				if !strings.Contains(all, name) {
					t.Errorf("%q should still be named: %s", name, all)
				}
			}
			for _, name := range tt.wantGone {
				if strings.Contains(all, name) {
					t.Errorf("%q leaked: %s", name, all)
				}
			}
			if !strings.Contains(all, sanitize.HiddenMentionText) {
				t.Errorf("no stand-in text: %s", all)
			}
		})
	}
}
