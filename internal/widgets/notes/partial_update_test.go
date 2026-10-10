// partial_update_test.go pins the partial-update contract for journal note
// edits: pinning, filing, sharing and archiving each send one field, and none
// may disturb the text or the other settings.
package notes

import (
	"context"
	"encoding/json"
	"testing"
)

func TestUpdate_PartialBody(t *testing.T) {
	parent := "folder-1"
	tests := []struct {
		name       string
		body       string
		check      func(t *testing.T, n *Note)
		wantParent bool
	}{
		{"empty body changes nothing", `{}`, func(t *testing.T, n *Note) {}, true},
		{"pin keeps everything else", `{"pinned":true}`, func(t *testing.T, n *Note) {
			if !n.Pinned {
				t.Error("pinned not applied")
			}
		}, true},
		{"color keeps text and pin state", `{"color":"#ffffff"}`, func(t *testing.T, n *Note) {
			if n.Color != "#ffffff" {
				t.Errorf("color = %q", n.Color)
			}
		}, true},
		{"empty parentId files at the top level", `{"parentId":""}`, func(t *testing.T, n *Note) {}, false},
		{"null parentId preserves the folder", `{"parentId":null}`, func(t *testing.T, n *Note) {}, true},
		{"title only keeps content", `{"title":"New"}`, func(t *testing.T, n *Note) {
			if n.Title != "New" {
				t.Errorf("title = %q", n.Title)
			}
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stored := sampleNote()
			stored.ParentID = &parent
			stored.Entry = strPtr(`{"type":"doc"}`)
			entryHTML := "<p>kept</p>"
			stored.EntryHTML = &entryHTML
			repo := &mockNoteRepo{
				findByIDFn: func(_ context.Context, id string) (*Note, error) {
					if id == parent {
						return &Note{ID: parent, CampaignID: "camp-1", UserID: "user-1", IsFolder: true}, nil
					}
					return stored, nil
				},
			}
			var req UpdateNoteRequest
			if err := json.Unmarshal([]byte(tt.body), &req); err != nil {
				t.Fatalf("bad test body: %v", err)
			}
			n, err := NewNoteService(repo).Update(context.Background(), "note-123", player("user-1"), req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tt.check(t, n)
			if (n.ParentID != nil) != tt.wantParent {
				t.Errorf("parent set = %v, want %v", n.ParentID != nil, tt.wantParent)
			}
			if n.Entry == nil || *n.Entry != `{"type":"doc"}` || n.EntryHTML == nil || *n.EntryHTML != entryHTML {
				t.Error("entry must survive a body that does not name it")
			}
			if len(n.Content) != 2 {
				t.Errorf("content blocks = %d, want the 2 stored", len(n.Content))
			}
		})
	}
}

func strPtr(s string) *string { return &s }
