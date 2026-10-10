// partial_update_test.go pins the partial-update contract for post edits: the
// entity page sends one field at a time (a rename, a privacy toggle), and each
// must leave the rest of the post alone.
package posts

import (
	"context"
	"encoding/json"
	"testing"
)

func TestUpdate_PartialBody(t *testing.T) {
	html := "<p>kept</p>"
	tests := []struct {
		name        string
		body        string
		wantName    string
		wantPrivate bool
	}{
		{"empty body changes nothing", `{}`, "Original", false},
		{"rename keeps privacy and content", `{"name":"Renamed"}`, "Renamed", false},
		{"privacy toggle keeps name and content", `{"isPrivate":true}`, "Original", true},
		{"explicit false is a value", `{"isPrivate":false}`, "Original", false},
		{"null name preserves", `{"name":null}`, "Original", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var written *Post
			repo := &mockPostRepo{
				findByIDFn: func(_ context.Context, id string) (*Post, error) {
					return &Post{ID: id, Name: "Original", Entry: json.RawMessage(`{"type":"doc"}`), EntryHTML: &html}, nil
				},
				updateFn: func(_ context.Context, p *Post) error { written = p; return nil },
			}
			var req UpdatePostRequest
			if err := json.Unmarshal([]byte(tt.body), &req); err != nil {
				t.Fatalf("bad test body: %v", err)
			}
			if _, err := NewPostService(repo).Update(context.Background(), "p1", req); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if written.Name != tt.wantName || written.IsPrivate != tt.wantPrivate {
				t.Errorf("wrote name=%q private=%v, want %q/%v", written.Name, written.IsPrivate, tt.wantName, tt.wantPrivate)
			}
			if string(written.Entry) != `{"type":"doc"}` || written.EntryHTML == nil || *written.EntryHTML != html {
				t.Errorf("content must survive a body that does not name it, got %s / %v", written.Entry, written.EntryHTML)
			}
		})
	}
}
