package notes

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// stubAccessSvc answers GetByID/GetVersion from maps, like the real
// `WHERE id = ?` lookups with no user filter, and records what reaches the
// mutating methods — so a missing handler gate shows up as a call.
type stubAccessSvc struct {
	NoteService
	notes    map[string]*Note
	versions map[string]*NoteVersion

	forceUnlocked []string
	updates       []UpdateNoteRequest
}

func (s *stubAccessSvc) GetByID(_ context.Context, id string) (*Note, error) {
	n, ok := s.notes[id]
	if !ok {
		return nil, apperror.NewNotFound("note not found")
	}
	return n, nil
}

func (s *stubAccessSvc) GetVersion(_ context.Context, id string) (*NoteVersion, error) {
	v, ok := s.versions[id]
	if !ok {
		return nil, apperror.NewNotFound("note version not found")
	}
	return v, nil
}

func (s *stubAccessSvc) ForceReleaseLock(_ context.Context, id string) error {
	s.forceUnlocked = append(s.forceUnlocked, id)
	return nil
}

func (s *stubAccessSvc) Update(_ context.Context, id string, _ permissions.Viewer, req UpdateNoteRequest) (*Note, error) {
	s.updates = append(s.updates, req)
	return s.notes[id], nil
}

// stubAttachments records deletes and transcript writes.
type stubAttachments struct {
	AttachmentService
	atts    map[string]*NoteAttachment
	deleted []string
}

func (s *stubAttachments) GetAttachment(_ context.Context, id string) (*NoteAttachment, error) {
	a, ok := s.atts[id]
	if !ok {
		return nil, apperror.NewNotFound("attachment not found")
	}
	return a, nil
}

func (s *stubAttachments) DeleteAttachment(_ context.Context, id string) (string, error) {
	s.deleted = append(s.deleted, id)
	return "", nil
}

func (s *stubAttachments) ListAttachments(_ context.Context, _ string) ([]NoteAttachment, error) {
	return nil, nil
}

func newAccessSvc() *stubAccessSvc {
	return &stubAccessSvc{
		notes: map[string]*Note{
			"gm-private":     {ID: "gm-private", CampaignID: "c1", UserID: "u-gm"},
			"player-private": {ID: "player-private", CampaignID: "c1", UserID: "u-player"},
			"party":          {ID: "party", CampaignID: "c1", UserID: "u-gm", IsShared: true},
			"for-gm":         {ID: "for-gm", CampaignID: "c1", UserID: "u-player", SharedWithGM: true},
			"other-campaign": {ID: "other-campaign", CampaignID: "c2", UserID: "u-gm", IsShared: true},
		},
		versions: map[string]*NoteVersion{
			"v-party":          {ID: "v-party", NoteID: "party"},
			"v-player-private": {ID: "v-player-private", NoteID: "player-private", Title: "My secret"},
		},
	}
}

// ctxFor builds a request context for userID at role in campaign c1.
func ctxFor(method, path string, userID string, role campaigns.Role, params map[string]string, body []byte) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	names, values := make([]string, 0, len(params)), make([]string, 0, len(params))
	for k, v := range params {
		names = append(names, k)
		values = append(values, v)
	}
	c.SetParamNames(names...)
	c.SetParamValues(values...)
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "c1"},
		MemberRole: role,
		IsMember:   true,
	})
	auth.SetSession(c, &auth.Session{UserID: userID})
	return c, rec
}

func wantStatus(t *testing.T, err error, code int) {
	t.Helper()
	var appErr *apperror.AppError
	if err == nil || !errors.As(err, &appErr) || appErr.Code != code {
		t.Fatalf("want a %d AppError, got %v", code, err)
	}
}

// TestGetVersion_OnlyOfTheNoteInTheURL: viewing one note is not access to a
// version of a different, private note addressed by its version id.
func TestGetVersion_OnlyOfTheNoteInTheURL(t *testing.T) {
	h := NewHandler(newAccessSvc())

	c, _ := ctxFor(http.MethodGet, "/", "u-other", campaigns.RolePlayer, map[string]string{"noteId": "party", "vid": "v-player-private"}, nil)
	wantStatus(t, h.GetVersion(c), http.StatusNotFound)

	c, rec := ctxFor(http.MethodGet, "/", "u-other", campaigns.RolePlayer, map[string]string{"noteId": "party", "vid": "v-party"}, nil)
	if err := h.GetVersion(c); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("a version of the note in the URL must load: err=%v code=%d", err, rec.Code)
	}
}

// TestForceUnlock_StaysInsideTheCampaign: an Owner of one campaign cannot
// clear locks on another campaign's notes, or on notes they cannot see.
func TestForceUnlock_StaysInsideTheCampaign(t *testing.T) {
	svc := newAccessSvc()
	h := NewHandler(svc)

	c, _ := ctxFor(http.MethodPost, "/", "u-gm", campaigns.RoleOwner, map[string]string{"noteId": "other-campaign"}, nil)
	wantStatus(t, h.ForceUnlock(c), http.StatusNotFound)

	c, _ = ctxFor(http.MethodPost, "/", "u-gm", campaigns.RoleOwner, map[string]string{"noteId": "player-private"}, nil)
	wantStatus(t, h.ForceUnlock(c), http.StatusNotFound)

	c, _ = ctxFor(http.MethodPost, "/", "u-gm", campaigns.RoleOwner, map[string]string{"noteId": "party"}, nil)
	if err := h.ForceUnlock(c); err != nil {
		t.Fatalf("the Owner must force-unlock a note of their campaign: %v", err)
	}
	if len(svc.forceUnlocked) != 1 || svc.forceUnlocked[0] != "party" {
		t.Errorf("force-unlocked %v, want only [party]", svc.forceUnlocked)
	}
}

// TestAttachments_BoundToTheCampaignAndTheViewer covers the three attachment
// writes: none may reach a note outside the campaign or one the caller
// cannot see, whatever their role.
func TestAttachments_BoundToTheCampaignAndTheViewer(t *testing.T) {
	svc := newAccessSvc()
	atts := &stubAttachments{atts: map[string]*NoteAttachment{
		"a-other": {ID: "a-other", NoteID: "other-campaign"},
		"a-priv":  {ID: "a-priv", NoteID: "player-private"},
	}}
	h := NewHandler(svc)
	h.SetAttachmentService(atts)
	h.SetMediaUploader(nil)

	c, _ := ctxFor(http.MethodDelete, "/", "u-gm", campaigns.RoleOwner, map[string]string{"nid": "other-campaign", "aid": "a-other"}, nil)
	wantStatus(t, h.DeleteAttachment(c), http.StatusNotFound)

	c, _ = ctxFor(http.MethodDelete, "/", "u-gm", campaigns.RoleOwner, map[string]string{"nid": "player-private", "aid": "a-priv"}, nil)
	wantStatus(t, h.DeleteAttachment(c), http.StatusNotFound)

	c, _ = ctxFor(http.MethodPut, "/", "u-gm", campaigns.RoleOwner, map[string]string{"nid": "other-campaign", "aid": "a-other"}, []byte(`{"transcript":"x"}`))
	wantStatus(t, h.UpdateTranscript(c), http.StatusNotFound)

	if len(atts.deleted) != 0 {
		t.Errorf("no attachment may be deleted, got %v", atts.deleted)
	}

	c, _ = ctxFor(http.MethodGet, "/", "u-other", campaigns.RolePlayer, map[string]string{"nid": "gm-private"}, nil)
	wantStatus(t, h.ListAttachments(c), http.StatusNotFound)
}

// TestUploadAttachment_RefusesANoteTheCallerCannotSee: the upload gate runs
// before the file is read, for a private note of the same campaign too.
func TestUploadAttachment_RefusesANoteTheCallerCannotSee(t *testing.T) {
	h := NewHandler(newAccessSvc())
	h.SetAttachmentService(&stubAttachments{})
	h.SetMediaUploader(uploaderThatMustNotRun{t})

	c, _ := ctxFor(http.MethodPost, "/", "u-other", campaigns.RolePlayer, map[string]string{"nid": "gm-private"}, nil)
	wantStatus(t, h.UploadAttachment(c), http.StatusNotFound)
}

type uploaderThatMustNotRun struct{ t *testing.T }

func (u uploaderThatMustNotRun) UploadRaw(context.Context, string, string, []byte, string, string) (string, error) {
	u.t.Error("the upload must be refused before anything is stored")
	return "", nil
}

// TestUpdate_GMShareIsVisibleToTheGMOnly: the GM can open and edit a note a
// player shared with the GM; another player cannot.
func TestUpdate_GMShareIsVisibleToTheGMOnly(t *testing.T) {
	svc := newAccessSvc()
	h := NewHandler(svc)

	c, _ := ctxFor(http.MethodPut, "/", "u-other", campaigns.RolePlayer, map[string]string{"noteId": "for-gm"}, []byte(`{"title":"x"}`))
	wantStatus(t, h.Update(c), http.StatusNotFound)

	c, _ = ctxFor(http.MethodPut, "/", "u-gm", campaigns.RoleOwner, map[string]string{"noteId": "for-gm"}, []byte(`{"title":"x","visibility":"party","archived":true}`))
	if err := h.Update(c); err != nil {
		t.Fatalf("the GM must be able to edit a note shared with the GM: %v", err)
	}
	got := svc.updates[len(svc.updates)-1]
	if got.Title == nil || *got.Title != "x" {
		t.Error("the title edit must reach the service")
	}
	if got.Visibility != nil || got.Archived != nil {
		t.Error("the GM is not the owner: sharing and archiving must be stripped")
	}
}

// TestJournalRoutes_GateOnTheNote: a single note and its backlinks load only
// for someone who can see it; the stub panics if the service is reached.
func TestJournalRoutes_GateOnTheNote(t *testing.T) {
	h := NewHandler(newAccessSvc())

	c, _ := ctxFor(http.MethodGet, "/", "u-other", campaigns.RolePlayer, map[string]string{"noteId": "gm-private"}, nil)
	wantStatus(t, h.Get(c), http.StatusNotFound)

	c, _ = ctxFor(http.MethodGet, "/", "u-other", campaigns.RolePlayer, map[string]string{"noteId": "gm-private"}, nil)
	wantStatus(t, h.Backlinks(c), http.StatusNotFound)

	c, rec := ctxFor(http.MethodGet, "/", "u-other", campaigns.RolePlayer, map[string]string{"noteId": "party"}, nil)
	if err := h.Get(c); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("a party note loads: %v %d", err, rec.Code)
	}
}
