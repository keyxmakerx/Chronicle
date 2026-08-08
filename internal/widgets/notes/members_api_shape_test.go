// members_api_shape_test.go pins the wire shape of
// GET /campaigns/:id/notes/members — the payload the notes widget's
// share-with-players picker consumes.
//
// The regression this guards: notes.js read `m.id` / `m.name` from a body
// that has only carried `user_id` / `username` / `role`, so every checkbox
// rendered blank with value="" and the note was persisted with a sharedWith
// array of empty user IDs. The JS side of the same contract is pinned by
// test/js/notes_share_picker.test.mjs, which reads the memberRef tags out of
// handler.go and asserts the widget reads exactly those keys — so a rename
// here goes red there too instead of silently blanking the picker again.
package notes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// stubMemberLister returns a fixed member roster, standing in for
// CampaignService without touching a database.
type stubMemberLister struct {
	members []campaigns.CampaignMember
}

func (s *stubMemberLister) ListMembers(_ context.Context, _ string) ([]campaigns.CampaignMember, error) {
	return s.members, nil
}

// TestMembersAPIShape drives the real handler and asserts the JSON keys the
// picker binds to are present (and that the phantom `id` / `name` the widget
// used to read are not — the shape must stay the one the JS expects).
func TestMembersAPIShape(t *testing.T) {
	h := NewHandler(nil)
	h.SetMemberLister(&stubMemberLister{members: []campaigns.CampaignMember{
		{UserID: "u-alice", DisplayName: "Alice", Role: campaigns.RoleOwner},
		{UserID: "u-bob", DisplayName: "Bob", Role: campaigns.RolePlayer},
	}})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/notes/members", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "camp-1"},
		MemberRole: campaigns.RolePlayer,
	})

	if err := h.MembersAPI(c); err != nil {
		t.Fatalf("MembersAPI returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	// A bare array, not an envelope — the widget consumes it as one.
	var rows []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatalf("body is not a bare JSON array: %v (body=%s)", err, rec.Body.String())
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2 (body=%s)", len(rows), rec.Body.String())
	}

	for i, row := range rows {
		for _, key := range []string{"user_id", "username", "role"} {
			if _, ok := row[key]; !ok {
				t.Errorf("row %d is missing %q — the share picker binds to it (row=%v)", i, key, row)
			}
		}
		for _, absent := range []string{"id", "name"} {
			if _, ok := row[absent]; ok {
				t.Errorf("row %d unexpectedly carries %q; the picker contract is user_id/username (row=%v)", i, absent, row)
			}
		}
	}

	if got := rows[0]["user_id"]; got != "u-alice" {
		t.Errorf("rows[0].user_id = %v, want u-alice", got)
	}
	if got := rows[0]["username"]; got != "Alice" {
		t.Errorf("rows[0].username = %v, want Alice (DisplayName)", got)
	}
}
