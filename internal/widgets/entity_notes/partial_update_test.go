// partial_update_test.go pins the partial-update contract for entity note
// edits: the pin toggle and a rename each send one field and must leave the
// audience, body and share list alone.
package entity_notes

import (
	"context"
	"encoding/json"
	"testing"
)

func TestService_Update_PartialBody(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantTitle  string
		wantPinned bool
	}{
		{"empty body changes nothing", `{}`, "Original", false},
		{"pin keeps title and body", `{"pinned":true}`, "Original", true},
		{"rename keeps pin and body", `{"title":"Renamed"}`, "Renamed", false},
		{"explicit false is a value", `{"pinned":false}`, "Original", false},
		{"null title preserves", `{"title":null}`, "Original", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &stubRepo{findForAuthor: map[string]*Note{
				"n1": {
					ID: "n1", AuthorUserID: "u-player", CampaignID: "c1",
					Audience: AudienceCustom, SharedWith: []string{"u2"},
					Title: "Original", Body: json.RawMessage(`{"type":"doc"}`), BodyHTML: "<p>kept</p>",
				},
			}}
			var req UpdateNoteRequest
			if err := json.Unmarshal([]byte(tt.body), &req); err != nil {
				t.Fatalf("bad test body: %v", err)
			}
			if _, err := NewService(repo, nil).Update(context.Background(), "n1", playerViewer(), req); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got := repo.updated[0]
			if got.Title != tt.wantTitle || got.Pinned != tt.wantPinned {
				t.Errorf("wrote title=%q pinned=%v, want %q/%v", got.Title, got.Pinned, tt.wantTitle, tt.wantPinned)
			}
			if got.Audience != AudienceCustom || len(got.SharedWith) != 1 || got.SharedWith[0] != "u2" {
				t.Errorf("audience/share list changed: %v %v", got.Audience, got.SharedWith)
			}
			if string(got.Body) != `{"type":"doc"}` || got.BodyHTML != "<p>kept</p>" {
				t.Errorf("body changed: %s / %q", got.Body, got.BodyHTML)
			}
		})
	}
}
