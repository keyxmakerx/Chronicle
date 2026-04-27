package entity_notes

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// Service is the business-logic interface for entity notes.
// Handlers are thin: bind, call service, render. All audience and
// authorship rules live here; tests in service_test.go pin them.
type Service interface {
	// Create persists a new note authored by viewer.UserID. The
	// `author_user_id` is taken from the viewer, never from the request.
	Create(ctx context.Context, entityID string, viewer ViewerContext, req CreateNoteRequest) (*Note, error)

	// List returns the notes on an entity that viewer can read.
	List(ctx context.Context, entityID string, viewer ViewerContext) ([]Note, error)

	// Get returns one note if viewer can read it. Returns NotFound
	// (rather than Forbidden) when the viewer can't see it, to avoid
	// leaking existence to unprivileged users.
	Get(ctx context.Context, id string, viewer ViewerContext) (*Note, error)

	// Update patches a note. Only the author may update; others get
	// NotFound (same existence-leak prevention as Get).
	Update(ctx context.Context, id string, viewer ViewerContext, req UpdateNoteRequest) (*Note, error)

	// Delete removes a note. Only the author may delete.
	Delete(ctx context.Context, id string, viewer ViewerContext) error
}

// Notifier is the optional callback the service invokes after each
// mutation so a separate live-updates layer can broadcast over the
// WebSocket hub. Optional because tests don't need it and core
// correctness shouldn't depend on it.
type Notifier func(event string, note *Note, audience Audience)

// ErrAudienceForbidden is returned when the viewer's role doesn't
// permit creating/updating a note with the requested audience
// (e.g., a player attempting to create dm_only or dm_scribe).
var ErrAudienceForbidden = errors.New("you do not have permission to use this audience")

type service struct {
	repo     Repository
	notifier Notifier
}

// NewService constructs a Service. Pass a non-nil Notifier to enable
// live-update broadcasting; pass nil for tests / non-realtime flows.
func NewService(repo Repository, notifier Notifier) Service {
	return &service{repo: repo, notifier: notifier}
}

func (s *service) Create(ctx context.Context, entityID string, viewer ViewerContext, req CreateNoteRequest) (*Note, error) {
	if entityID == "" {
		return nil, apperror.NewBadRequest("entity ID is required")
	}
	audience := req.Audience
	if audience == "" {
		audience = AudiencePrivate
	}
	if !audience.Valid() {
		return nil, apperror.NewBadRequest("invalid audience")
	}
	if err := checkAudienceWrite(audience, viewer); err != nil {
		return nil, err
	}
	if err := checkSharedWith(audience, req.SharedWith); err != nil {
		return nil, err
	}
	if err := checkBodySize(req.Body); err != nil {
		return nil, err
	}
	if err := checkBodyHTMLSize(req.BodyHTML); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	note := &Note{
		ID:           uuid.New().String(),
		EntityID:     entityID,
		AuthorUserID: viewer.UserID,
		Audience:     audience,
		SharedWith:   normalizeSharedWith(audience, req.SharedWith),
		Title:        strings.TrimSpace(req.Title),
		Body:         req.Body,
		BodyHTML:     sanitizeHTML(req.BodyHTML),
		Pinned:       req.Pinned,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	// CampaignID isn't on the request — bind it from the parent entity.
	// Handler resolves entity → campaign before calling us. We require
	// it on the note for FK + indexing reasons, so the service contract
	// is: handler MUST set CampaignID. Failsafe: reject if empty.
	if viewer.CampaignID == "" {
		return nil, apperror.NewBadRequest("missing campaign context")
	}
	note.CampaignID = viewer.CampaignID

	if len(note.Title) > 200 {
		return nil, apperror.NewBadRequest("title must be 200 characters or less")
	}

	if err := s.repo.Create(ctx, note); err != nil {
		return nil, apperror.NewInternal(err)
	}
	s.broadcast("entity_notes.created", note)
	return note, nil
}

func (s *service) List(ctx context.Context, entityID string, viewer ViewerContext) ([]Note, error) {
	if entityID == "" {
		return nil, apperror.NewBadRequest("entity ID is required")
	}
	notes, err := s.repo.ListByEntity(ctx, entityID, viewer)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return notes, nil
}

func (s *service) Get(ctx context.Context, id string, viewer ViewerContext) (*Note, error) {
	n, err := s.repo.FindByID(ctx, id, viewer)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if n == nil {
		return nil, apperror.NewNotFound("note not found")
	}
	return n, nil
}

func (s *service) Update(ctx context.Context, id string, viewer ViewerContext, req UpdateNoteRequest) (*Note, error) {
	existing, err := s.repo.FindByIDForAuthor(ctx, id, viewer.UserID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	// Existence-leak prevention: even if the note exists but the viewer
	// isn't the author, return NotFound.
	if existing == nil {
		return nil, apperror.NewNotFound("note not found")
	}

	if req.Audience != nil {
		newAud := *req.Audience
		if !newAud.Valid() {
			return nil, apperror.NewBadRequest("invalid audience")
		}
		if err := checkAudienceWrite(newAud, viewer); err != nil {
			return nil, err
		}
		existing.Audience = newAud
	}
	if req.SharedWith != nil {
		if err := checkSharedWith(existing.Audience, req.SharedWith); err != nil {
			return nil, err
		}
		existing.SharedWith = normalizeSharedWith(existing.Audience, req.SharedWith)
	} else if existing.Audience != AudienceCustom {
		// Audience changed to non-custom: drop any stale sharing list.
		existing.SharedWith = nil
	} else if req.Audience != nil && existing.Audience == AudienceCustom && len(existing.SharedWith) == 0 {
		// Caller flipped audience to `custom` but didn't send a fresh
		// shared_with and the existing list is empty. Reject rather
		// than silently saving a custom-audience note no one but the
		// author can see — that's the "I shared this and nobody can
		// read it" UX hole. Force callers to be explicit.
		return nil, apperror.NewBadRequest("changing audience to custom requires shared_with")
	}
	if req.Title != nil {
		t := strings.TrimSpace(*req.Title)
		if len(t) > 200 {
			return nil, apperror.NewBadRequest("title must be 200 characters or less")
		}
		existing.Title = t
	}
	if req.Body != nil {
		if err := checkBodySize(req.Body); err != nil {
			return nil, err
		}
		existing.Body = req.Body
	}
	if req.BodyHTML != nil {
		if err := checkBodyHTMLSize(*req.BodyHTML); err != nil {
			return nil, err
		}
		existing.BodyHTML = sanitizeHTML(*req.BodyHTML)
	}
	if req.Pinned != nil {
		existing.Pinned = *req.Pinned
	}
	existing.UpdatedAt = time.Now().UTC()

	if err := s.repo.Update(ctx, existing); err != nil {
		return nil, apperror.NewInternal(err)
	}
	s.broadcast("entity_notes.updated", existing)
	return existing, nil
}

func (s *service) Delete(ctx context.Context, id string, viewer ViewerContext) error {
	existing, err := s.repo.FindByIDForAuthor(ctx, id, viewer.UserID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if existing == nil {
		return apperror.NewNotFound("note not found")
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return apperror.NewInternal(err)
	}
	s.broadcast("entity_notes.deleted", existing)
	return nil
}

// --- ACL helpers (write-side) ---

// checkAudienceWrite enforces the role gate on which audiences a viewer
// is allowed to *author* a note with. The read-side filter lives in the
// repo (noteACLFilter); these two are the full ACL surface and must be
// kept in sync if either ever grows new tiers.
func checkAudienceWrite(audience Audience, viewer ViewerContext) error {
	switch audience {
	case AudiencePrivate, AudienceEveryone, AudienceCustom:
		return nil
	case AudienceDMScribe:
		if viewer.IsOwner || viewer.IsScribe || viewer.IsDMGranted {
			return nil
		}
	case AudienceDMOnly:
		// IsDmGranted users can READ dm_only (per the column docstring at
		// internal/plugins/campaigns/model.go:246) but only Owners can
		// AUTHOR them. Without this, a dm-granted player could quietly
		// post notes that pretended to be GM-authored.
		if viewer.IsOwner {
			return nil
		}
	}
	return apperror.NewForbidden(ErrAudienceForbidden.Error())
}

// checkSharedWith validates the SharedWith list against the audience.
// Non-custom audiences must not have entries (they'd be silently
// ignored, which is confusing); custom audiences must have at least
// one valid UUID.
//
// UUID validation here is the cheap defensive layer — it doesn't
// guarantee the user exists or is a campaign member (those checks
// require a service round-trip we don't currently do). What it does
// catch: a buggy client sending arbitrary strings, which would
// otherwise sit in the JSON column and produce confusing UX when the
// "shared user" never matches anything.
func checkSharedWith(audience Audience, ids []string) error {
	if audience == AudienceCustom {
		if len(ids) == 0 {
			return apperror.NewBadRequest("custom audience requires at least one shared user")
		}
		seen := make(map[string]struct{}, len(ids))
		for _, id := range ids {
			if id == "" {
				return apperror.NewBadRequest("shared_with contains empty user id")
			}
			if _, err := uuid.Parse(id); err != nil {
				return apperror.NewBadRequest("shared_with contains malformed user id")
			}
			if _, dup := seen[id]; dup {
				return apperror.NewBadRequest("shared_with contains duplicate user ids")
			}
			seen[id] = struct{}{}
		}
		return nil
	}
	if len(ids) > 0 {
		return apperror.NewBadRequest("shared_with is only valid with audience='custom'")
	}
	return nil
}

// maxBodyBytes is the cap on the raw TipTap JSON body we accept. 1 MB
// holds a several-thousand-word note with structured content; beyond
// that we're either dealing with a runaway client or someone trying
// to fill the column. The DB column is JSON (max 4 GB) so the schema
// won't reject; this is the application-level brake.
const maxBodyBytes = 1 << 20 // 1 MiB

// maxBodyHTMLBytes is the cap on the rendered HTML body, applied
// before sanitize.HTML runs. Set to 2× the JSON cap because HTML
// inflates relative to ProseMirror JSON for the same content.
const maxBodyHTMLBytes = 2 << 20 // 2 MiB

func checkBodySize(body []byte) error {
	if len(body) > maxBodyBytes {
		return apperror.NewBadRequest("note body too large (1 MB max)")
	}
	return nil
}

func checkBodyHTMLSize(html string) error {
	if len(html) > maxBodyHTMLBytes {
		return apperror.NewBadRequest("note body HTML too large (2 MB max)")
	}
	return nil
}

// normalizeSharedWith trims/dedups the list when audience is custom,
// or returns nil for any other audience so the column lands as NULL.
func normalizeSharedWith(audience Audience, ids []string) []string {
	if audience != AudienceCustom {
		return nil
	}
	out := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// sanitizeHTML routes through the shared sanitizer, matching what
// notes / posts / entities do for their rich-text bodies.
func sanitizeHTML(raw string) string {
	if raw == "" {
		return ""
	}
	return sanitize.HTML(raw)
}

func (s *service) broadcast(event string, note *Note) {
	if s.notifier == nil || note == nil {
		return
	}
	s.notifier(event, note, note.Audience)
}
