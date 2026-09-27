package notes

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// backlinkStub answers Backlinks with one linking note.
type backlinkStub struct {
	*stubAccessSvc
}

func (s *backlinkStub) Backlinks(_ context.Context, _ string, _ permissions.Viewer, _ string) ([]NoteRef, error) {
	return []NoteRef{{ID: "n2", Title: "Session 12"}}, nil
}

// stubLinker records how it was asked and returns one page.
type stubLinker struct {
	calls       int
	seesSecrets []bool
}

func (s *stubLinker) PagesLinkingNote(_ context.Context, _ string, _ permissions.Viewer, seesSecrets bool, _ string) ([]PageRef, error) {
	s.calls++
	s.seesSecrets = append(s.seesSecrets, seesSecrets)
	return []PageRef{{ID: "p1", Name: "Ashkeep Ruins", TypeName: "Location"}}, nil
}

// TestBacklinks_NotesAndPages: the route answers {notes, pages}; only
// scribes and owners are asked about pages with inline secrets, and a note
// the caller cannot see never reaches the page lookup.
func TestBacklinks_NotesAndPages(t *testing.T) {
	linker := &stubLinker{}
	h := NewHandler(&backlinkStub{stubAccessSvc: newAccessSvc()})
	h.SetPageLinker(linker)

	c, _ := ctxFor(http.MethodGet, "/", "u-other", campaigns.RolePlayer, map[string]string{"noteId": "gm-private"}, nil)
	wantStatus(t, h.Backlinks(c), http.StatusNotFound)
	if linker.calls != 0 {
		t.Fatal("a hidden note must not reach the page lookup")
	}

	c, rec := ctxFor(http.MethodGet, "/", "u-other", campaigns.RolePlayer, map[string]string{"noteId": "party"}, nil)
	if err := h.Backlinks(c); err != nil {
		t.Fatal(err)
	}
	var got LinksIn
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Notes) != 1 || got.Notes[0].Title != "Session 12" || len(got.Pages) != 1 || got.Pages[0].Name != "Ashkeep Ruins" {
		t.Fatalf("want one note and one page, got %+v", got)
	}

	c, _ = ctxFor(http.MethodGet, "/", "u-scribe", campaigns.RoleScribe, map[string]string{"noteId": "party"}, nil)
	if err := h.Backlinks(c); err != nil {
		t.Fatal(err)
	}
	if len(linker.seesSecrets) != 2 || linker.seesSecrets[0] || !linker.seesSecrets[1] {
		t.Errorf("a player does not read secrets, a scribe does: %v", linker.seesSecrets)
	}
}

// TestBacklinks_NoPageLookup: without a page lookup wired, pages is an
// empty list, never null.
func TestBacklinks_NoPageLookup(t *testing.T) {
	h := NewHandler(&backlinkStub{stubAccessSvc: newAccessSvc()})
	c, rec := ctxFor(http.MethodGet, "/", "u-gm", campaigns.RoleOwner, map[string]string{"noteId": "party"}, nil)
	if err := h.Backlinks(c); err != nil {
		t.Fatal(err)
	}
	if body := rec.Body.String(); body != `{"notes":[{"id":"n2","title":"Session 12","archived":false}],"pages":[]}`+"\n" {
		t.Errorf("unexpected body %s", body)
	}
}
