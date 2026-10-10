package notes

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// NoteService defines the business logic contract for notes.
type NoteService interface {
	// Create persists a new note owned by creator.
	Create(ctx context.Context, campaignID string, creator permissions.Viewer, req CreateNoteRequest) (*Note, error)
	GetByID(ctx context.Context, id string) (*Note, error)

	// Update applies a partial update made by editor. The caller has already
	// checked editor may view the note and stripped the owner-only fields
	// (UpdateNoteRequest.StripOwnerOnly) when editor is not its owner.
	Update(ctx context.Context, id string, editor permissions.Viewer, req UpdateNoteRequest) (*Note, error)

	// Delete removes a note. Deleting a folder deletes the folder owner's
	// notes inside it and moves everyone else's out to the top level.
	Delete(ctx context.Context, id string) error
	ToggleCheck(ctx context.Context, id string, req ToggleCheckRequest) (*Note, error)

	// ListVisible returns the notes v can read, narrowed by scope.
	ListVisible(ctx context.Context, campaignID string, v permissions.Viewer, scope ListScope) ([]Note, error)

	// ListByUserAndCampaign lists what userID can read as a plain member:
	// their own notes and the ones shared with them, never the ones shared
	// only with the GM. Kept for callers that have a user but no role.
	ListByUserAndCampaign(ctx context.Context, userID, campaignID string) ([]Note, error)

	// The Journal's views (journal_service.go). Each reads only what v can see.
	JournalIndex(ctx context.Context, campaignID string, v permissions.Viewer) (*JournalIndex, error)
	Search(ctx context.Context, campaignID string, v permissions.Viewer, q string, withJots bool) ([]SearchHit, error)
	Backlinks(ctx context.Context, campaignID string, v permissions.Viewer, noteID string) ([]NoteRef, error)
	PageRefs(ctx context.Context, campaignID string, v permissions.Viewer, entityID string) ([]NoteRef, error)
	Labels(ctx context.Context, campaignID string, v permissions.Viewer, ids []string) (map[string]*NoteLabel, error)
	Bulk(ctx context.Context, campaignID string, v permissions.Viewer, req BulkRequest) (*BulkResult, error)

	// SendToJournal copies v's own jot into a new private Journal note linked
	// to the jot's page, and records it on the jot (jots.go).
	SendToJournal(ctx context.Context, campaignID string, v permissions.Viewer, jotID, pageName string) (*SendResult, error)

	// ListSharedByCampaign returns every shared note in the campaign across
	// all owners. Unlike the three list methods above it applies no per-user
	// visibility filter, so it is owner-gated data: campaign export is the
	// only caller. Notes are campaign content, and an export that omits them
	// is a backup that quietly lies.
	ListSharedByCampaign(ctx context.Context, campaignID string) ([]Note, error)

	// Locking
	AcquireLock(ctx context.Context, noteID, userID string) (*Note, error)
	ReleaseLock(ctx context.Context, noteID, userID string) error
	ForceReleaseLock(ctx context.Context, noteID string) error
	Heartbeat(ctx context.Context, noteID, userID string) error

	// Versions
	ListVersions(ctx context.Context, noteID string) ([]NoteVersion, error)
	GetVersion(ctx context.Context, versionID string) (*NoteVersion, error)
	RestoreVersion(ctx context.Context, noteID, versionID, userID string) (*Note, error)
}

// AttachmentService defines the business logic for note attachments.
type AttachmentService interface {
	ListAttachments(ctx context.Context, noteID string) ([]NoteAttachment, error)
	GetAttachment(ctx context.Context, id string) (*NoteAttachment, error)
	CreateAttachment(ctx context.Context, a *NoteAttachment) error
	DeleteAttachment(ctx context.Context, id string) (filePath string, err error)
	UpdateTranscript(ctx context.Context, id, transcript string) error
}

// NoteEvent is what the live-updates layer hears when a note changes: IDs and
// who may hear about it, never the note itself, so a private note's title or
// body cannot ride a broadcast to someone who could not read it (#715).
type NoteEvent struct {
	Type       string // "created", "updated" or "deleted"
	CampaignID string
	NoteID     string
	EntityID   *string // the page a jot sits on; nil for a Journal note
	Audience   Audience
}

// Audience is who may be told a note changed. Everyone overrides the rest.
type Audience struct {
	Everyone bool     // shared with the party
	GMs      bool     // shared with the campaign Owner and co-DMs
	Users    []string // the owner and anyone the note names
}

// audienceOf is the event audience for a note's current sharing: exactly the
// people Note.CanView admits.
func audienceOf(n *Note) Audience {
	if n.IsShared {
		return Audience{Everyone: true}
	}
	users := append([]string{n.UserID}, n.SharedWith...)
	return Audience{GMs: n.SharedWithGM, Users: users}
}

// union widens a to also reach b, so a change of audience tells both the
// people who could see the note before and the ones who can see it now.
func (a Audience) union(b Audience) Audience {
	if a.Everyone || b.Everyone {
		return Audience{Everyone: true}
	}
	seen := make(map[string]bool, len(a.Users)+len(b.Users))
	var users []string
	for _, u := range append(append([]string{}, a.Users...), b.Users...) {
		if u != "" && !seen[u] {
			seen[u] = true
			users = append(users, u)
		}
	}
	return Audience{GMs: a.GMs || b.GMs, Users: users}
}

// NoteEventPublisher emits domain events when notes change.
// Implemented by the WebSocket EventBus adapter in app/routes.go.
type NoteEventPublisher interface {
	PublishNoteEvent(ev NoteEvent)
}

// NoopNoteEventPublisher is a no-op implementation for tests.
type NoopNoteEventPublisher struct{}

// PublishNoteEvent discards the event.
func (NoopNoteEventPublisher) PublishNoteEvent(NoteEvent) {}

// noteService implements NoteService and AttachmentService.
type noteService struct {
	repo    NoteRepository
	attRepo AttachmentRepository
	events  NoteEventPublisher
}

// NewNoteService creates a new note service.
func NewNoteService(repo NoteRepository) NoteService {
	return &noteService{repo: repo, events: NoopNoteEventPublisher{}}
}

// NewNoteServiceWithAttachments creates a note service with attachment support.
func NewNoteServiceWithAttachments(repo NoteRepository, attRepo AttachmentRepository) *noteService {
	return &noteService{repo: repo, attRepo: attRepo, events: NoopNoteEventPublisher{}}
}

// SetEventPublisher sets the event publisher for real-time sync.
func (s *noteService) SetEventPublisher(pub NoteEventPublisher) {
	s.events = pub
}

// publish sends a change event for note n to aud.
func (s *noteService) publish(eventType string, n *Note, aud Audience) {
	s.events.PublishNoteEvent(NoteEvent{
		Type:       eventType,
		CampaignID: n.CampaignID,
		NoteID:     n.ID,
		EntityID:   n.EntityID,
		Audience:   aud,
	})
}

// Create validates and persists a new note.
func (s *noteService) Create(ctx context.Context, campaignID string, creator permissions.Viewer, req CreateNoteRequest) (*Note, error) {
	userID := creator.UserID()
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "Untitled"
	}
	if len(title) > 200 {
		return nil, apperror.NewBadRequest("title must be 200 characters or less")
	}

	color := strings.TrimSpace(req.Color)
	if color == "" {
		color = "#374151"
	}

	content := req.Content
	if content == nil {
		content = []Block{}
	}

	id := generateID()
	if req.ID != "" {
		id = req.ID
	}
	note := &Note{
		ID:           id,
		CampaignID:   campaignID,
		UserID:       userID,
		EntityID:     req.EntityID,
		IsFolder:     req.IsFolder,
		Title:        title,
		Content:      content,
		Color:        color,
		IsShared:     req.IsShared,
		SharedWith:   cleanSharedWith(req.SharedWith, userID),
		LastEditedBy: &userID,
	}
	if req.Visibility != "" {
		if !req.Visibility.Valid() {
			return nil, apperror.NewBadRequest("unknown visibility")
		}
		if err := checkCustomAudience(req.Visibility, note.SharedWith); err != nil {
			return nil, err
		}
		note.applyVisibility(req.Visibility, note.SharedWith)
	}
	if req.ParentID != nil && *req.ParentID != "" {
		if err := s.checkFolderTarget(ctx, note, *req.ParentID, creator); err != nil {
			return nil, err
		}
		note.ParentID = req.ParentID
	}

	if err := s.repo.Create(ctx, note); err != nil {
		return nil, err
	}

	created, err := s.repo.FindByID(ctx, note.ID)
	if err != nil {
		return nil, err
	}
	s.publish("created", created, audienceOf(created))
	return created, nil
}

// GetByID retrieves a note by ID.
func (s *noteService) GetByID(ctx context.Context, id string) (*Note, error) {
	return s.repo.FindByID(ctx, id)
}

// Update applies partial updates to a note and records a version snapshot.
func (s *noteService) Update(ctx context.Context, id string, editor permissions.Viewer, req UpdateNoteRequest) (*Note, error) {
	note, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	before := audienceOf(note)
	userID := editor.UserID()

	// Validate everything before the snapshot, so a refused edit leaves no
	// stray version behind.
	var title *string
	if req.Title != nil {
		t := strings.TrimSpace(*req.Title)
		if len(t) > 200 {
			return nil, apperror.NewBadRequest("title must be 200 characters or less")
		}
		if t == "" {
			t = "Untitled"
		}
		title = &t
	}
	if req.Visibility != nil {
		if !req.Visibility.Valid() {
			return nil, apperror.NewBadRequest("unknown visibility")
		}
		if err := checkCustomAudience(*req.Visibility, cleanSharedWith(req.SharedWith, note.UserID)); err != nil {
			return nil, err
		}
	}
	if req.ParentID != nil && *req.ParentID != "" {
		if err := s.checkFolderTarget(ctx, note, *req.ParentID, editor); err != nil {
			return nil, err
		}
	}

	// Snapshot the current state before a content edit. Pinning, sharing,
	// filing and archiving leave the text alone, so they add no version.
	if req.Title != nil || req.Content != nil || req.Entry != nil || req.EntryHTML != nil {
		s.createVersionSnapshot(ctx, note, userID)
	}

	if title != nil {
		note.Title = *title
	}
	if req.Content != nil {
		note.Content = *req.Content
	}
	if req.Entry != nil {
		note.Entry = req.Entry
	}
	if req.EntryHTML != nil {
		sanitized := sanitize.HTML(*req.EntryHTML)
		note.EntryHTML = &sanitized
	}
	if req.Color != nil {
		note.Color = *req.Color
	}
	if req.Pinned != nil {
		note.Pinned = *req.Pinned
	}
	if req.Visibility != nil {
		note.applyVisibility(*req.Visibility, cleanSharedWith(req.SharedWith, note.UserID))
	} else {
		applyLegacySharing(note, req.IsShared, req.SharedWith)
	}
	if req.ParentID != nil {
		// Empty string means move to top-level.
		if *req.ParentID == "" {
			note.ParentID = nil
		} else {
			note.ParentID = req.ParentID
		}
	}
	if req.Archived != nil {
		if *req.Archived {
			if note.ArchivedAt == nil {
				now := time.Now().UTC()
				note.ArchivedAt = &now
			}
			// An archived note leaves the everyday list, pinned group included.
			note.Pinned = false
		} else {
			note.ArchivedAt = nil
		}
	}

	note.LastEditedBy = &userID

	if err := s.repo.Update(ctx, note); err != nil {
		return nil, err
	}
	updated, err := s.repo.FindByID(ctx, note.ID)
	if err != nil {
		return nil, err
	}
	s.publish("updated", updated, before.union(audienceOf(updated)))
	return updated, nil
}

// applyLegacySharing applies isShared/sharedWith the way clients that predate
// the GM audience send them. Sharing with the party or with named people
// replaces a GM share; an explicit "neither" (both fields sent, both off) is
// the old picker's Private and clears it too. A partial update that sends one
// field alone leaves a GM share in place.
func applyLegacySharing(n *Note, isShared *bool, sharedWith []string) {
	if isShared == nil && sharedWith == nil {
		return
	}
	if isShared != nil {
		n.IsShared = *isShared
	}
	if sharedWith != nil {
		n.SharedWith = cleanSharedWith(sharedWith, n.UserID)
	}
	explicitPrivate := isShared != nil && !*isShared && sharedWith != nil && len(n.SharedWith) == 0
	if n.IsShared || len(n.SharedWith) > 0 || explicitPrivate {
		n.SharedWithGM = false
	}
	n.derive()
}

// cleanSharedWith trims and dedups a share list and drops the owner, who can
// always see their own note. Returns nil for an empty result so the column
// stores NULL rather than "[]".
func cleanSharedWith(ids []string, ownerID string) []string {
	var out []string
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || id == ownerID || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// checkCustomAudience refuses "shared with specific people" naming nobody:
// saving it would silently make the note private.
func checkCustomAudience(v Visibility, sharedWith []string) error {
	if v == VisibilityCustom && len(sharedWith) == 0 {
		return apperror.NewBadRequest("choose at least one person to share with")
	}
	return nil
}

// checkFolderTarget validates filing note into folderID on behalf of editor:
// the folder must exist in the same campaign, be a folder editor can see,
// and must not be the note itself or sit inside it.
func (s *noteService) checkFolderTarget(ctx context.Context, note *Note, folderID string, editor permissions.Viewer) error {
	if folderID == note.ID {
		return apperror.NewBadRequest("a folder cannot contain itself")
	}
	folder, err := s.repo.FindByID(ctx, folderID)
	if err != nil || folder == nil || folder.CampaignID != note.CampaignID || !folder.IsFolder || !folder.CanView(editor, note.CampaignID) {
		return apperror.NewBadRequest("folder not found")
	}
	if !note.IsFolder {
		return nil
	}
	tree, err := s.repo.ListTree(ctx, note.CampaignID)
	if err != nil {
		return err
	}
	parentOf := make(map[string]*string, len(tree))
	for _, t := range tree {
		parentOf[t.ID] = t.ParentID
	}
	// Walk up from the target; meeting the note means the move would cycle.
	// The step bound stops a corrupt, already-cyclic tree from looping.
	cur := folderID
	for steps := 0; steps <= len(tree); steps++ {
		if cur == note.ID {
			return apperror.NewBadRequest("a folder cannot move inside itself")
		}
		p := parentOf[cur]
		if p == nil {
			break
		}
		cur = *p
	}
	return nil
}

// Delete removes a note. For a folder, the database cascade would take every
// note filed inside it, including notes other people filed there; those are
// moved to the top level first, so a delete only ever removes the folder
// owner's own notes.
func (s *noteService) Delete(ctx context.Context, id string) error {
	note, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if note.IsFolder {
		if err := s.rescueForeignChildren(ctx, note); err != nil {
			return err
		}
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.publish("deleted", note, audienceOf(note))
	return nil
}

// rescueForeignChildren moves out of folder every note in its subtree that
// is not the folder owner's, cutting the cascade off at each one.
func (s *noteService) rescueForeignChildren(ctx context.Context, folder *Note) error {
	tree, err := s.repo.ListTree(ctx, folder.CampaignID)
	if err != nil {
		return err
	}
	children := make(map[string][]TreeRow, len(tree))
	for _, t := range tree {
		if t.ParentID != nil {
			children[*t.ParentID] = append(children[*t.ParentID], t)
		}
	}
	var rescue []string
	visited := map[string]bool{folder.ID: true}
	queue := []string{folder.ID}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, c := range children[cur] {
			if visited[c.ID] {
				continue
			}
			visited[c.ID] = true
			if c.UserID != folder.UserID {
				rescue = append(rescue, c.ID)
				continue
			}
			queue = append(queue, c.ID)
		}
	}
	return s.repo.ReparentToTop(ctx, rescue)
}

// ToggleCheck flips a checklist item's checked state within a note.
func (s *noteService) ToggleCheck(ctx context.Context, id string, req ToggleCheckRequest) (*Note, error) {
	note, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.BlockIndex < 0 || req.BlockIndex >= len(note.Content) {
		return nil, apperror.NewBadRequest("block index out of range")
	}

	block := &note.Content[req.BlockIndex]
	if block.Type != "checklist" {
		return nil, apperror.NewBadRequest("block is not a checklist")
	}

	if req.ItemIndex < 0 || req.ItemIndex >= len(block.Items) {
		return nil, apperror.NewBadRequest("item index out of range")
	}

	block.Items[req.ItemIndex].Checked = !block.Items[req.ItemIndex].Checked

	if err := s.repo.Update(ctx, note); err != nil {
		return nil, err
	}
	s.publish("updated", note, audienceOf(note))
	return note, nil
}

// ListVisible returns the notes v can read, narrowed by scope.
func (s *noteService) ListVisible(ctx context.Context, campaignID string, v permissions.Viewer, scope ListScope) ([]Note, error) {
	return s.repo.ListVisible(ctx, campaignID, v, scope)
}

// ListByUserAndCampaign lists what userID can read as a plain member.
func (s *noteService) ListByUserAndCampaign(ctx context.Context, userID, campaignID string) ([]Note, error) {
	return s.repo.ListVisible(ctx, campaignID, permissions.RequestViewer(permissions.RolePlayer, userID), ListScope{})
}

// ListSharedByCampaign returns every shared note in the campaign, across all
// owners, for campaign export. See the interface comment for why this one is
// unfiltered and who is allowed to call it.
func (s *noteService) ListSharedByCampaign(ctx context.Context, campaignID string) ([]Note, error) {
	return s.repo.ListSharedByCampaign(ctx, campaignID)
}

// AcquireLock attempts to take a pessimistic edit lock on the note. Returns
// the refreshed note on success or a conflict error if another user holds it.
func (s *noteService) AcquireLock(ctx context.Context, noteID, userID string) (*Note, error) {
	acquired, err := s.repo.AcquireLock(ctx, noteID, userID)
	if err != nil {
		return nil, err
	}
	if !acquired {
		note, _ := s.repo.FindByID(ctx, noteID)
		if note != nil && note.LockedBy != nil {
			return nil, apperror.NewConflict("note is currently being edited by another user")
		}
		return nil, apperror.NewConflict("could not acquire lock")
	}
	return s.repo.FindByID(ctx, noteID)
}

// ReleaseLock releases the edit lock (only if held by the requesting user).
func (s *noteService) ReleaseLock(ctx context.Context, noteID, userID string) error {
	return s.repo.ReleaseLock(ctx, noteID, userID)
}

// ForceReleaseLock releases the lock regardless of holder (owner override).
func (s *noteService) ForceReleaseLock(ctx context.Context, noteID string) error {
	return s.repo.ForceReleaseLock(ctx, noteID)
}

// Heartbeat keeps the edit lock alive by refreshing locked_at.
func (s *noteService) Heartbeat(ctx context.Context, noteID, userID string) error {
	return s.repo.RefreshLock(ctx, noteID, userID)
}

// ListVersions returns the version history for a note.
func (s *noteService) ListVersions(ctx context.Context, noteID string) ([]NoteVersion, error) {
	return s.repo.ListVersions(ctx, noteID, MaxVersionsPerNote)
}

// GetVersion retrieves a specific version.
func (s *noteService) GetVersion(ctx context.Context, versionID string) (*NoteVersion, error) {
	return s.repo.FindVersionByID(ctx, versionID)
}

// RestoreVersion reverts a note to a previous version's content. A new version
// is created to preserve the current state before the restore.
func (s *noteService) RestoreVersion(ctx context.Context, noteID, versionID, userID string) (*Note, error) {
	note, err := s.repo.FindByID(ctx, noteID)
	if err != nil {
		return nil, err
	}

	version, err := s.repo.FindVersionByID(ctx, versionID)
	if err != nil {
		return nil, err
	}
	if version.NoteID != noteID {
		return nil, apperror.NewBadRequest("version does not belong to this note")
	}

	// Snapshot current state before restoring.
	s.createVersionSnapshot(ctx, note, userID)

	// Apply the version's content. Sanitize restored HTML in case the version
	// was created before HTML sanitization was enforced.
	note.Title = version.Title
	note.Content = version.Content
	note.Entry = version.Entry
	if version.EntryHTML != nil {
		sanitized := sanitize.HTML(*version.EntryHTML)
		note.EntryHTML = &sanitized
	} else {
		note.EntryHTML = nil
	}
	note.LastEditedBy = &userID

	if err := s.repo.Update(ctx, note); err != nil {
		return nil, err
	}
	restored, err := s.repo.FindByID(ctx, note.ID)
	if err != nil {
		return nil, err
	}
	s.publish("updated", restored, audienceOf(restored))
	return restored, nil
}

// createVersionSnapshot saves the current note state as a version record.
// Errors are swallowed — version tracking is non-critical.
func (s *noteService) createVersionSnapshot(ctx context.Context, note *Note, userID string) {
	v := &NoteVersion{
		ID:        generateID(),
		NoteID:    note.ID,
		UserID:    userID,
		Title:     note.Title,
		Content:   note.Content,
		Entry:     note.Entry,
		EntryHTML: note.EntryHTML,
	}
	_ = s.repo.CreateVersion(ctx, v)
	_ = s.repo.PruneVersions(ctx, note.ID, MaxVersionsPerNote)
}

// --- Attachment Service Methods ---

// ListAttachments returns all attachments for a note.
func (s *noteService) ListAttachments(ctx context.Context, noteID string) ([]NoteAttachment, error) {
	if s.attRepo == nil {
		return nil, apperror.NewInternal(nil)
	}
	return s.attRepo.ListByNote(ctx, noteID)
}

// GetAttachment retrieves a single attachment by ID.
func (s *noteService) GetAttachment(ctx context.Context, id string) (*NoteAttachment, error) {
	if s.attRepo == nil {
		return nil, apperror.NewInternal(nil)
	}
	return s.attRepo.FindAttachmentByID(ctx, id)
}

// CreateAttachment validates and persists a new attachment record.
func (s *noteService) CreateAttachment(ctx context.Context, a *NoteAttachment) error {
	if s.attRepo == nil {
		return apperror.NewInternal(nil)
	}
	if a.ID == "" {
		a.ID = generateID()
	}
	return s.attRepo.CreateAttachment(ctx, a)
}

// DeleteAttachment removes an attachment record and returns the file path for cleanup.
func (s *noteService) DeleteAttachment(ctx context.Context, id string) (string, error) {
	if s.attRepo == nil {
		return "", apperror.NewInternal(nil)
	}
	att, err := s.attRepo.FindAttachmentByID(ctx, id)
	if err != nil {
		return "", err
	}
	if err := s.attRepo.DeleteAttachment(ctx, id); err != nil {
		return "", err
	}
	return att.FilePath, nil
}

// UpdateTranscript sets the transcript text for an attachment.
func (s *noteService) UpdateTranscript(ctx context.Context, id, transcript string) error {
	if s.attRepo == nil {
		return apperror.NewInternal(nil)
	}
	return s.attRepo.UpdateTranscript(ctx, id, transcript)
}

// generateID creates a random 36-char hex string formatted as a UUID-like ID.
func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	h := hex.EncodeToString(b)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}
