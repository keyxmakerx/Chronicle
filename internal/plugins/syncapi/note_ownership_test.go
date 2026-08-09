package syncapi

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/widgets/notes"
)

// --- note ownership regression (syncapi IDOR) ---
//
// GET/PUT/DELETE /api/v1/campaigns/:id/notes/:noteID address a note by its ID.
// The middleware chain (RequireAuthOrAPIKey -> RequireCampaignMatch ->
// RequirePermission) only proves the caller is a member of the campaign in the
// URL holding the role-derived read/write permission; it knows nothing about
// who owns the note. The notes service and repository address the single
// resource by bare `WHERE id = ?` with no user predicate at any layer, so the
// handler is the only ownership gate. Before the fix these three handlers
// checked campaign membership of the note and stopped there, so any member read
// any other member's private journal and any Scribe overwrote or deleted it —
// while the web routes over the identical caller population 404 both callers.
//
// These tests pin the gate: a non-owner must 404 on an unshared note and must
// not reach the service, the owner and share recipients keep working, and a
// share recipient must not be able to re-share or delete a note they don't own.

// stubNoteSvcForOwnership embeds notes.NoteService so unimplemented methods are
// present-but-panic (same rationale as stubRelationSvcForScope), and overrides
// only the three methods the handlers under test can reach. The map is keyed by
// ID alone, mirroring the real `WHERE id = ?` SQL — there is no user filter in
// the repository, which is exactly why the handler needs one.
type stubNoteSvcForOwnership struct {
	notes.NoteService
	notes   map[string]*notes.Note
	deleted []string
	updated map[string]notes.UpdateNoteRequest
}

func (s *stubNoteSvcForOwnership) GetByID(_ context.Context, id string) (*notes.Note, error) {
	n, ok := s.notes[id]
	if !ok {
		return nil, apperror.NewNotFound("note not found")
	}
	return n, nil
}

// Update replicates noteService.Update's patch-with-no-owner-check verbatim for
// the fields these handlers can set, so a missing handler gate really does
// mutate the stored note (rather than being masked by a stricter stub).
func (s *stubNoteSvcForOwnership) Update(_ context.Context, id, userID string, req notes.UpdateNoteRequest) (*notes.Note, error) {
	n, ok := s.notes[id]
	if !ok {
		return nil, apperror.NewNotFound("note not found")
	}
	if req.Title != nil {
		n.Title = *req.Title
	}
	if req.EntryHTML != nil {
		n.EntryHTML = req.EntryHTML
	}
	if req.IsShared != nil {
		n.IsShared = *req.IsShared
	}
	if req.SharedWith != nil {
		n.SharedWith = req.SharedWith
	}
	if req.Pinned != nil {
		n.Pinned = *req.Pinned
	}
	n.LastEditedBy = &userID
	if s.updated == nil {
		s.updated = map[string]notes.UpdateNoteRequest{}
	}
	s.updated[id] = req
	return n, nil
}

func (s *stubNoteSvcForOwnership) Delete(_ context.Context, id string) error {
	if _, ok := s.notes[id]; !ok {
		return apperror.NewNotFound("note not found")
	}
	delete(s.notes, id)
	s.deleted = append(s.deleted, id)
	return nil
}

// newNoteOwnershipSvc seeds three notes in campaign-C: bob's private journal
// (the victim), a note bob shared with carol only, and a campaign-wide shared
// note (the negative controls).
func newNoteOwnershipSvc() *stubNoteSvcForOwnership {
	private := "<p>I suspect the Duke is a doppelganger.</p>"
	return &stubNoteSvcForOwnership{
		notes: map[string]*notes.Note{
			"note-bob-private": {
				ID:         "note-bob-private",
				CampaignID: "campaign-C",
				UserID:     "bob",
				Title:      "Bob's private journal",
				EntryHTML:  &private,
				IsShared:   false,
			},
			"note-bob-shared-with-carol": {
				ID:         "note-bob-shared-with-carol",
				CampaignID: "campaign-C",
				UserID:     "bob",
				Title:      "Party plans",
				IsShared:   false,
				SharedWith: []string{"carol"},
			},
			"note-bob-public": {
				ID:         "note-bob-public",
				CampaignID: "campaign-C",
				UserID:     "bob",
				Title:      "Session recap",
				IsShared:   true,
			},
		},
	}
}

// newNoteOwnershipContext builds an Echo context for the given method against
// /api/v1/campaigns/<campaignID>/notes/<noteID>, carrying the APIKey that
// RequireAuthOrAPIKey puts on the context (a session-authed member gets a
// synthetic key whose UserID is the session user and whose CampaignID is the
// URL :id, so RequireCampaignMatch always passes for a member).
func newNoteOwnershipContext(method, campaignID, noteID, userID string, body []byte) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, "/api/v1/campaigns/"+campaignID+"/notes/"+noteID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "noteID")
	c.SetParamValues(campaignID, noteID)
	c.Set(apiKeyContextKey, &APIKey{
		UserID:      userID,
		CampaignID:  campaignID,
		Permissions: []APIKeyPermission{PermRead, PermWrite},
		IsActive:    true,
	})
	return c, rec
}

// assertNotFound fails unless err is a 404 AppError.
func assertNotFound(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("want 404 not-found AppError, got nil error")
	}
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != http.StatusNotFound {
		t.Fatalf("want 404 not-found AppError, got %#v", err)
	}
}

// TestGetNote_RejectsNonOwnerOfPrivateNote is the read half of the IDOR fix: a
// campaign member who neither owns the note nor was shared it must not read it.
func TestGetNote_RejectsNonOwnerOfPrivateNote(t *testing.T) {
	svc := newNoteOwnershipSvc()
	h := NewNoteAPIHandler(nil, svc)

	c, rec := newNoteOwnershipContext(http.MethodGet, "campaign-C", "note-bob-private", "alice", nil)

	assertNotFound(t, h.GetNote(c))
	if body := rec.Body.String(); bytes.Contains([]byte(body), []byte("doppelganger")) {
		t.Fatalf("bob's private note body leaked to a non-owner (IDOR): %s", body)
	}
}

// TestUpdateNote_RejectsNonOwnerOfPrivateNote is the write half: a Scribe's
// campaign-wide "write" permission is not authority over another member's note.
func TestUpdateNote_RejectsNonOwnerOfPrivateNote(t *testing.T) {
	svc := newNoteOwnershipSvc()
	h := NewNoteAPIHandler(nil, svc)

	c, _ := newNoteOwnershipContext(http.MethodPut, "campaign-C", "note-bob-private", "carol",
		[]byte(`{"title":"pwned","entry_html":"<p></p>"}`))

	assertNotFound(t, h.UpdateNote(c))
	if got := svc.notes["note-bob-private"].Title; got != "Bob's private journal" {
		t.Fatalf("bob's private note was overwritten by a non-owner (IDOR): title = %q", got)
	}
	if len(svc.updated) != 0 {
		t.Fatalf("Update must not reach the service for a note the caller cannot access, updated=%v", svc.updated)
	}
}

// TestDeleteNote_RejectsNonOwnerOfPrivateNote is the destructive half.
func TestDeleteNote_RejectsNonOwnerOfPrivateNote(t *testing.T) {
	svc := newNoteOwnershipSvc()
	h := NewNoteAPIHandler(nil, svc)

	c, _ := newNoteOwnershipContext(http.MethodDelete, "campaign-C", "note-bob-private", "carol", nil)

	assertNotFound(t, h.DeleteNote(c))
	if _, still := svc.notes["note-bob-private"]; !still {
		t.Fatalf("bob's private note was deleted by a non-owner (IDOR)")
	}
	if len(svc.deleted) != 0 {
		t.Fatalf("Delete must not reach the service for a note the caller does not own, deleted=%v", svc.deleted)
	}
}

// TestDeleteNote_RejectsShareRecipient pins that deletion is strictly
// owner-only: being able to READ a shared note is not authority to destroy it,
// matching the web route (which requires existing.UserID == userID).
func TestDeleteNote_RejectsShareRecipient(t *testing.T) {
	svc := newNoteOwnershipSvc()
	h := NewNoteAPIHandler(nil, svc)

	c, _ := newNoteOwnershipContext(http.MethodDelete, "campaign-C", "note-bob-shared-with-carol", "carol", nil)

	assertNotFound(t, h.DeleteNote(c))
	if _, still := svc.notes["note-bob-shared-with-carol"]; !still {
		t.Fatalf("a share recipient deleted a note they do not own")
	}
}

// TestUpdateNote_ShareRecipientCannotReshare pins the owner-only sharing
// controls: carol may edit the body of a note bob shared with her, but must not
// be able to broadcast it to the whole campaign or re-target the share list.
func TestUpdateNote_ShareRecipientCannotReshare(t *testing.T) {
	svc := newNoteOwnershipSvc()
	h := NewNoteAPIHandler(nil, svc)

	c, rec := newNoteOwnershipContext(http.MethodPut, "campaign-C", "note-bob-shared-with-carol", "carol",
		[]byte(`{"title":"Party plans v2","is_shared":true,"shared_with":["dave"],"pinned":true}`))

	if err := h.UpdateNote(c); err != nil {
		t.Fatalf("a share recipient must still be able to edit the body, got %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	n := svc.notes["note-bob-shared-with-carol"]
	if n.Title != "Party plans v2" {
		t.Fatalf("share recipient's body edit was not applied: title = %q", n.Title)
	}
	if n.IsShared {
		t.Fatalf("share recipient broadcast someone else's note campaign-wide")
	}
	if len(n.SharedWith) != 1 || n.SharedWith[0] != "carol" {
		t.Fatalf("share recipient re-targeted someone else's share list: %v", n.SharedWith)
	}
	if n.Pinned {
		t.Fatalf("share recipient changed pinned state on someone else's note")
	}
}

// TestNoteOwnership_OwnerAndSharesStillWork is the negative control: the gate
// must not break any legitimate path.
func TestNoteOwnership_OwnerAndSharesStillWork(t *testing.T) {
	t.Run("owner reads own private note", func(t *testing.T) {
		svc := newNoteOwnershipSvc()
		h := NewNoteAPIHandler(nil, svc)
		c, rec := newNoteOwnershipContext(http.MethodGet, "campaign-C", "note-bob-private", "bob", nil)
		if err := h.GetNote(c); err != nil {
			t.Fatalf("owner must be able to read their own note, got %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("want 200, got %d", rec.Code)
		}
	})

	t.Run("share recipient reads note shared with them", func(t *testing.T) {
		svc := newNoteOwnershipSvc()
		h := NewNoteAPIHandler(nil, svc)
		c, rec := newNoteOwnershipContext(http.MethodGet, "campaign-C", "note-bob-shared-with-carol", "carol", nil)
		if err := h.GetNote(c); err != nil {
			t.Fatalf("share recipient must be able to read, got %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("want 200, got %d", rec.Code)
		}
	})

	t.Run("any member reads a campaign-wide shared note", func(t *testing.T) {
		svc := newNoteOwnershipSvc()
		h := NewNoteAPIHandler(nil, svc)
		c, rec := newNoteOwnershipContext(http.MethodGet, "campaign-C", "note-bob-public", "alice", nil)
		if err := h.GetNote(c); err != nil {
			t.Fatalf("is_shared note must be readable by any member, got %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("want 200, got %d", rec.Code)
		}
	})

	t.Run("owner updates and deletes own note", func(t *testing.T) {
		svc := newNoteOwnershipSvc()
		h := NewNoteAPIHandler(nil, svc)

		cu, _ := newNoteOwnershipContext(http.MethodPut, "campaign-C", "note-bob-private", "bob",
			[]byte(`{"title":"Revised","is_shared":true}`))
		if err := h.UpdateNote(cu); err != nil {
			t.Fatalf("owner must be able to update, got %v", err)
		}
		if got := svc.notes["note-bob-private"]; got.Title != "Revised" || !got.IsShared {
			t.Fatalf("owner's update was not applied: %+v", got)
		}

		cd, rec := newNoteOwnershipContext(http.MethodDelete, "campaign-C", "note-bob-private", "bob", nil)
		if err := h.DeleteNote(cd); err != nil {
			t.Fatalf("owner must be able to delete, got %v", err)
		}
		if rec.Code != http.StatusNoContent {
			t.Fatalf("want 204, got %d", rec.Code)
		}
		if _, still := svc.notes["note-bob-private"]; still {
			t.Fatalf("owner's delete did not happen")
		}
	})
}

// TestNoteOwnership_ForeignCampaignStill404s pins that the pre-existing
// cross-campaign check survives the rewrite — Note.CanAccess/IsOwnedBy subsume
// it, so a note owned by the caller in another campaign must still 404 when
// addressed through the wrong campaign's URL.
func TestNoteOwnership_ForeignCampaignStill404s(t *testing.T) {
	svc := newNoteOwnershipSvc()
	h := NewNoteAPIHandler(nil, svc)

	for _, tc := range []struct {
		name string
		call func(echo.Context) error
		verb string
	}{
		{"get", h.GetNote, http.MethodGet},
		{"update", h.UpdateNote, http.MethodPut},
		{"delete", h.DeleteNote, http.MethodDelete},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newNoteOwnershipContext(tc.verb, "campaign-OTHER", "note-bob-private", "bob", []byte(`{"title":"x"}`))
			assertNotFound(t, tc.call(c))
			if _, still := svc.notes["note-bob-private"]; !still {
				t.Fatalf("note touched across campaigns")
			}
		})
	}
}

// TestNoteOwnership_MissingAPIKeyIs401 pins that the three single-note handlers
// refuse to proceed without a resolved identity rather than defaulting to an
// empty user ID (which would match a note whose UserID is "").
func TestNoteOwnership_MissingAPIKeyIs401(t *testing.T) {
	svc := newNoteOwnershipSvc()
	h := NewNoteAPIHandler(nil, svc)

	for _, tc := range []struct {
		name string
		call func(echo.Context) error
		verb string
	}{
		{"get", h.GetNote, http.MethodGet},
		{"update", h.UpdateNote, http.MethodPut},
		{"delete", h.DeleteNote, http.MethodDelete},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newNoteOwnershipContext(tc.verb, "campaign-C", "note-bob-private", "bob", []byte(`{"title":"x"}`))
			c.Set(apiKeyContextKey, nil)

			err := tc.call(c)
			var appErr *apperror.AppError
			if !errors.As(err, &appErr) || appErr.Code != http.StatusUnauthorized {
				t.Fatalf("want 401 unauthorized AppError, got %#v", err)
			}
		})
	}
}
