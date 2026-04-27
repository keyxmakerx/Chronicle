// Tests for the entity_notes service. The ACL matrix is the hard
// security surface, so the bulk of this file pins the read-filter
// behavior (via a stub repo that captures the ViewerContext) and the
// write-side audience checks. There are no real DB tests here —
// SQL-level coverage is implicit in the repo contract and verified
// manually against MariaDB during integration testing.
package entity_notes

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// isStatus returns true when err is (or wraps) an apperror.AppError
// with the given HTTP status code. Used in lieu of dedicated Is*
// helpers that the apperror package doesn't expose.
func isStatus(err error, code int) bool {
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) {
		return false
	}
	return appErr.Code == code
}

// stubRepo records every call. Tests inspect captured ViewerContexts
// to confirm the service forwards role/grant info accurately, and
// stubs return values to drive the service through its branches.
type stubRepo struct {
	created []Note

	listNotes   []Note
	listErr     error
	listViewer  ViewerContext

	findByIDNote   *Note
	findByIDViewer ViewerContext
	findByIDErr    error

	findForAuthor map[string]*Note
	updated       []Note
	deletedIDs    []string
}

func (s *stubRepo) Create(_ context.Context, n *Note) error {
	s.created = append(s.created, *n)
	return nil
}

func (s *stubRepo) FindByID(_ context.Context, _ string, viewer ViewerContext) (*Note, error) {
	s.findByIDViewer = viewer
	return s.findByIDNote, s.findByIDErr
}

func (s *stubRepo) FindByIDForAuthor(_ context.Context, id, _ string) (*Note, error) {
	if s.findForAuthor == nil {
		return nil, nil
	}
	return s.findForAuthor[id], nil
}

func (s *stubRepo) ListByEntity(_ context.Context, _ string, viewer ViewerContext) ([]Note, error) {
	s.listViewer = viewer
	return s.listNotes, s.listErr
}

func (s *stubRepo) Update(_ context.Context, n *Note) error {
	s.updated = append(s.updated, *n)
	return nil
}

func (s *stubRepo) Delete(_ context.Context, id string) error {
	s.deletedIDs = append(s.deletedIDs, id)
	return nil
}

// helpers

func ownerViewer() ViewerContext {
	return ViewerContext{UserID: "u-owner", CampaignID: "c1", IsOwner: true}
}
func scribeViewer() ViewerContext {
	return ViewerContext{UserID: "u-scribe", CampaignID: "c1", IsScribe: true}
}
func playerViewer() ViewerContext {
	return ViewerContext{UserID: "u-player", CampaignID: "c1"}
}
func dmGrantedPlayerViewer() ViewerContext {
	return ViewerContext{UserID: "u-grantee", CampaignID: "c1", IsDMGranted: true}
}

// --- ViewerContext role logic ---

func TestViewerContext_CanSee(t *testing.T) {
	cases := []struct {
		name    string
		viewer  ViewerContext
		dmScribe, dmOnly bool
	}{
		{"owner sees both", ownerViewer(), true, true},
		{"scribe sees dm_scribe only", scribeViewer(), true, false},
		{"plain player sees neither", playerViewer(), false, false},
		{"dm-granted player sees both", dmGrantedPlayerViewer(), true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.viewer.CanSeeDMScribe(); got != c.dmScribe {
				t.Errorf("CanSeeDMScribe: got %v, want %v", got, c.dmScribe)
			}
			if got := c.viewer.CanSeeDMOnly(); got != c.dmOnly {
				t.Errorf("CanSeeDMOnly: got %v, want %v", got, c.dmOnly)
			}
		})
	}
}

// --- Write-side audience checks ---

func TestCheckAudienceWrite(t *testing.T) {
	cases := []struct {
		name      string
		audience  Audience
		viewer    ViewerContext
		wantAllow bool
	}{
		// Anyone can author these.
		{"player private", AudiencePrivate, playerViewer(), true},
		{"player everyone", AudienceEveryone, playerViewer(), true},
		{"player custom", AudienceCustom, playerViewer(), true},

		// dm_scribe — Owner + Scribe + DM-granted ok; player not.
		{"player dm_scribe forbidden", AudienceDMScribe, playerViewer(), false},
		{"scribe dm_scribe ok", AudienceDMScribe, scribeViewer(), true},
		{"owner dm_scribe ok", AudienceDMScribe, ownerViewer(), true},
		{"dm_granted dm_scribe ok", AudienceDMScribe, dmGrantedPlayerViewer(), true},

		// dm_only — Owner only for write. dm_granted users can READ
		// but cannot AUTHOR; scribe is not enough either.
		{"player dm_only forbidden", AudienceDMOnly, playerViewer(), false},
		{"scribe dm_only forbidden", AudienceDMOnly, scribeViewer(), false},
		{"dm_granted dm_only forbidden for write", AudienceDMOnly, dmGrantedPlayerViewer(), false},
		{"owner dm_only ok", AudienceDMOnly, ownerViewer(), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := checkAudienceWrite(c.audience, c.viewer)
			if c.wantAllow && err != nil {
				t.Errorf("expected allowed, got %v", err)
			}
			if !c.wantAllow && err == nil {
				t.Errorf("expected forbidden, got allowed")
			}
		})
	}
}

// --- Service.Create — full path through audience + sharing checks ---

func TestService_Create_PrivateAsPlayer(t *testing.T) {
	s := &stubRepo{}
	svc := NewService(s, nil)
	got, err := svc.Create(context.Background(), "e1", playerViewer(), CreateNoteRequest{
		Audience: AudiencePrivate,
		Title:    "spoiler check",
		Body:     json.RawMessage(`{"type":"doc","content":[]}`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AuthorUserID != "u-player" {
		t.Errorf("AuthorUserID = %q, want u-player", got.AuthorUserID)
	}
	if got.CampaignID != "c1" {
		t.Errorf("CampaignID = %q, want c1", got.CampaignID)
	}
	if got.Audience != AudiencePrivate {
		t.Errorf("Audience = %q, want private", got.Audience)
	}
	if len(s.created) != 1 {
		t.Errorf("expected 1 note created, got %d", len(s.created))
	}
}

func TestService_Create_DefaultsToPrivateWhenAudienceEmpty(t *testing.T) {
	s := &stubRepo{}
	svc := NewService(s, nil)
	got, err := svc.Create(context.Background(), "e1", playerViewer(), CreateNoteRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Audience != AudiencePrivate {
		t.Errorf("Audience = %q, want private (default)", got.Audience)
	}
}

func TestService_Create_PlayerCannotAuthorDMOnly(t *testing.T) {
	svc := NewService(&stubRepo{}, nil)
	_, err := svc.Create(context.Background(), "e1", playerViewer(), CreateNoteRequest{
		Audience: AudienceDMOnly,
	})
	if err == nil {
		t.Fatal("expected forbidden, got nil")
	}
	if !isStatus(err, http.StatusForbidden) {
		t.Errorf("expected Forbidden error, got %T: %v", err, err)
	}
}

func TestService_Create_DMGrantedCannotAuthorDMOnly(t *testing.T) {
	// Pinning the asymmetry: dm_granted users CAN read dm_only but cannot
	// author dm_only. Without this, a player could be silently elevated
	// to DM-author by getting the dm_granted flag.
	svc := NewService(&stubRepo{}, nil)
	_, err := svc.Create(context.Background(), "e1", dmGrantedPlayerViewer(), CreateNoteRequest{
		Audience: AudienceDMOnly,
	})
	if err == nil {
		t.Fatal("expected forbidden, got nil")
	}
}

func TestService_Create_OwnerCanAuthorDMOnly(t *testing.T) {
	s := &stubRepo{}
	svc := NewService(s, nil)
	_, err := svc.Create(context.Background(), "e1", ownerViewer(), CreateNoteRequest{
		Audience: AudienceDMOnly,
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if len(s.created) != 1 || s.created[0].Audience != AudienceDMOnly {
		t.Errorf("expected dm_only note created, got %#v", s.created)
	}
}

func TestService_Create_ScribeCanAuthorDMScribe(t *testing.T) {
	s := &stubRepo{}
	svc := NewService(s, nil)
	_, err := svc.Create(context.Background(), "e1", scribeViewer(), CreateNoteRequest{
		Audience: AudienceDMScribe,
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestService_Create_CustomRequiresSharedWith(t *testing.T) {
	svc := NewService(&stubRepo{}, nil)
	_, err := svc.Create(context.Background(), "e1", playerViewer(), CreateNoteRequest{
		Audience: AudienceCustom,
		// SharedWith intentionally empty
	})
	if err == nil {
		t.Fatal("expected bad request, got nil")
	}
	if !isStatus(err, http.StatusBadRequest) {
		t.Errorf("expected BadRequest, got %T: %v", err, err)
	}
}

func TestService_Create_SharedWithRejectedForNonCustomAudience(t *testing.T) {
	svc := NewService(&stubRepo{}, nil)
	_, err := svc.Create(context.Background(), "e1", playerViewer(), CreateNoteRequest{
		Audience:   AudienceEveryone,
		SharedWith: []string{"u-other"},
	})
	if err == nil {
		t.Fatal("expected bad request when shared_with set on non-custom audience")
	}
}

func TestService_Create_SharedWithDuplicatesRejected(t *testing.T) {
	svc := NewService(&stubRepo{}, nil)
	_, err := svc.Create(context.Background(), "e1", playerViewer(), CreateNoteRequest{
		Audience:   AudienceCustom,
		SharedWith: []string{"u-a", "u-a"},
	})
	if err == nil {
		t.Fatal("expected bad request on duplicates")
	}
}

func TestService_Create_TitleTooLong(t *testing.T) {
	svc := NewService(&stubRepo{}, nil)
	long := make([]byte, 201)
	for i := range long {
		long[i] = 'x'
	}
	_, err := svc.Create(context.Background(), "e1", playerViewer(), CreateNoteRequest{
		Title: string(long),
	})
	if err == nil {
		t.Fatal("expected bad request on title >200 chars")
	}
}

// --- Service.Update — author-only contract ---

func TestService_Update_NonAuthorGetsNotFound(t *testing.T) {
	// Note exists, viewer is not author. Service must return NotFound,
	// NOT Forbidden — that's the existence-leak prevention.
	svc := NewService(&stubRepo{
		findForAuthor: map[string]*Note{}, // empty: no match for this user
	}, nil)
	_, err := svc.Update(context.Background(), "n1", playerViewer(), UpdateNoteRequest{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !isStatus(err, http.StatusNotFound) {
		t.Errorf("expected NotFound, got %T: %v", err, err)
	}
}

func TestService_Update_AuthorCanUpdateOwnPrivateNote(t *testing.T) {
	noteID := "n1"
	repo := &stubRepo{
		findForAuthor: map[string]*Note{
			noteID: {
				ID: noteID, AuthorUserID: "u-player", CampaignID: "c1",
				Audience: AudiencePrivate,
			},
		},
	}
	svc := NewService(repo, nil)
	newTitle := "renamed"
	_, err := svc.Update(context.Background(), noteID, playerViewer(), UpdateNoteRequest{
		Title: &newTitle,
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if len(repo.updated) != 1 || repo.updated[0].Title != "renamed" {
		t.Errorf("expected renamed note, got %#v", repo.updated)
	}
}

func TestService_Update_PlayerCannotEscalateToDMOnly(t *testing.T) {
	noteID := "n1"
	repo := &stubRepo{
		findForAuthor: map[string]*Note{
			noteID: {ID: noteID, AuthorUserID: "u-player", CampaignID: "c1"},
		},
	}
	svc := NewService(repo, nil)
	bad := AudienceDMOnly
	_, err := svc.Update(context.Background(), noteID, playerViewer(), UpdateNoteRequest{
		Audience: &bad,
	})
	if err == nil || !isStatus(err, http.StatusForbidden) {
		t.Errorf("expected Forbidden, got %v", err)
	}
}

// --- Service.Delete — author-only ---

func TestService_Delete_NonAuthorGetsNotFound(t *testing.T) {
	svc := NewService(&stubRepo{findForAuthor: map[string]*Note{}}, nil)
	err := svc.Delete(context.Background(), "n1", playerViewer())
	if err == nil || !isStatus(err, http.StatusNotFound) {
		t.Errorf("expected NotFound, got %v", err)
	}
}

// --- Service.List — viewer context plumbed to repo unchanged ---

func TestService_List_ForwardsViewerToRepo(t *testing.T) {
	repo := &stubRepo{}
	svc := NewService(repo, nil)
	v := dmGrantedPlayerViewer()
	if _, err := svc.List(context.Background(), "e1", v); err != nil {
		t.Fatal(err)
	}
	if !repo.listViewer.IsDMGranted {
		t.Errorf("repo did not receive IsDMGranted=true; got %#v", repo.listViewer)
	}
	if repo.listViewer.UserID != v.UserID {
		t.Errorf("UserID not forwarded: got %q want %q", repo.listViewer.UserID, v.UserID)
	}
}

// --- Notifier callback ---

func TestService_BroadcastsOnMutation(t *testing.T) {
	var captured []string
	notifier := func(event string, _ *Note, _ Audience) {
		captured = append(captured, event)
	}
	repo := &stubRepo{
		findForAuthor: map[string]*Note{
			"n1": {ID: "n1", AuthorUserID: "u-player", CampaignID: "c1"},
		},
	}
	svc := NewService(repo, notifier)

	if _, err := svc.Create(context.Background(), "e1", playerViewer(), CreateNoteRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(context.Background(), "n1", playerViewer(), UpdateNoteRequest{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(context.Background(), "n1", playerViewer()); err != nil {
		t.Fatal(err)
	}

	want := []string{"entity_notes.created", "entity_notes.updated", "entity_notes.deleted"}
	if len(captured) != len(want) {
		t.Fatalf("event count = %d, want %d (%v)", len(captured), len(want), captured)
	}
	for i, w := range want {
		if captured[i] != w {
			t.Errorf("event %d = %q, want %q", i, captured[i], w)
		}
	}
}

// --- normalizeSharedWith ---

func TestNormalizeSharedWith(t *testing.T) {
	got := normalizeSharedWith(AudienceCustom, []string{"a", "b", "a", "  ", "c"})
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("idx %d: got %q, want %q", i, got[i], w)
		}
	}

	// Non-custom audiences always get nil.
	if normalizeSharedWith(AudiencePrivate, []string{"a"}) != nil {
		t.Errorf("private audience should drop shared_with")
	}
}

// --- ErrAudienceForbidden surface ---

func TestErrAudienceForbidden_IsExposed(t *testing.T) {
	// External callers (handler, future integration tests) may want
	// errors.Is(err, ErrAudienceForbidden) — pin that the sentinel
	// is exported and stable.
	err := errors.New("wrap: " + ErrAudienceForbidden.Error())
	if !contains(err.Error(), "you do not have permission to use this audience") {
		t.Errorf("ErrAudienceForbidden text changed: %q", ErrAudienceForbidden)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || (len(sub) == 0) || indexOf(s, sub) >= 0)
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
