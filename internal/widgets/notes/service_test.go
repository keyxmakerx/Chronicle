package notes

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// --- Mock Repository ---

// mockNoteRepo implements NoteRepository for testing.
type mockNoteRepo struct {
	createFn               func(ctx context.Context, note *Note) error
	findByIDFn             func(ctx context.Context, id string) (*Note, error)
	updateFn               func(ctx context.Context, note *Note) error
	deleteFn               func(ctx context.Context, id string) error
	listVisibleFn          func(ctx context.Context, campaignID string, v permissions.Viewer, scope ListScope) ([]Note, error)
	listTreeFn             func(ctx context.Context, campaignID string) ([]TreeRow, error)
	reparentToTopFn        func(ctx context.Context, ids []string) error
	findByIDsFn            func(ctx context.Context, ids []string) ([]Note, error)
	listVisibleLinkingFn   func(ctx context.Context, campaignID string, v permissions.Viewer, kind, targetID string) ([]Note, error)
	listSharedByCampaignFn func(ctx context.Context, campaignID string) ([]Note, error)
	acquireLockFn          func(ctx context.Context, noteID, userID string) (bool, error)
	releaseLockFn          func(ctx context.Context, noteID, userID string) error
	forceReleaseLockFn     func(ctx context.Context, noteID string) error
	refreshLockFn          func(ctx context.Context, noteID, userID string) error
	createVersionFn        func(ctx context.Context, v *NoteVersion) error
	listVersionsFn         func(ctx context.Context, noteID string, limit int) ([]NoteVersion, error)
	findVersionByIDFn      func(ctx context.Context, id string) (*NoteVersion, error)
	pruneVersionsFn        func(ctx context.Context, noteID string, keep int) error
}

func (m *mockNoteRepo) Create(ctx context.Context, note *Note) error {
	if m.createFn != nil {
		return m.createFn(ctx, note)
	}
	return nil
}

func (m *mockNoteRepo) FindByID(ctx context.Context, id string) (*Note, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, id)
	}
	return nil, apperror.NewNotFound("note not found")
}

func (m *mockNoteRepo) Update(ctx context.Context, note *Note) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, note)
	}
	return nil
}

func (m *mockNoteRepo) Delete(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

func (m *mockNoteRepo) ListVisible(ctx context.Context, campaignID string, v permissions.Viewer, scope ListScope) ([]Note, error) {
	if m.listVisibleFn != nil {
		return m.listVisibleFn(ctx, campaignID, v, scope)
	}
	return nil, nil
}

func (m *mockNoteRepo) ListTree(ctx context.Context, campaignID string) ([]TreeRow, error) {
	if m.listTreeFn != nil {
		return m.listTreeFn(ctx, campaignID)
	}
	return nil, nil
}

func (m *mockNoteRepo) FindByIDs(ctx context.Context, ids []string) ([]Note, error) {
	if m.findByIDsFn != nil {
		return m.findByIDsFn(ctx, ids)
	}
	return nil, nil
}

func (m *mockNoteRepo) ListVisibleLinking(ctx context.Context, campaignID string, v permissions.Viewer, kind, targetID string) ([]Note, error) {
	if m.listVisibleLinkingFn != nil {
		return m.listVisibleLinkingFn(ctx, campaignID, v, kind, targetID)
	}
	return nil, nil
}

func (m *mockNoteRepo) ReparentToTop(ctx context.Context, ids []string) error {
	if m.reparentToTopFn != nil {
		return m.reparentToTopFn(ctx, ids)
	}
	return nil
}

func (m *mockNoteRepo) ListSharedByCampaign(ctx context.Context, campaignID string) ([]Note, error) {
	if m.listSharedByCampaignFn != nil {
		return m.listSharedByCampaignFn(ctx, campaignID)
	}
	return nil, nil
}

func (m *mockNoteRepo) AcquireLock(ctx context.Context, noteID, userID string) (bool, error) {
	if m.acquireLockFn != nil {
		return m.acquireLockFn(ctx, noteID, userID)
	}
	return true, nil
}

func (m *mockNoteRepo) ReleaseLock(ctx context.Context, noteID, userID string) error {
	if m.releaseLockFn != nil {
		return m.releaseLockFn(ctx, noteID, userID)
	}
	return nil
}

func (m *mockNoteRepo) ForceReleaseLock(ctx context.Context, noteID string) error {
	if m.forceReleaseLockFn != nil {
		return m.forceReleaseLockFn(ctx, noteID)
	}
	return nil
}

func (m *mockNoteRepo) RefreshLock(ctx context.Context, noteID, userID string) error {
	if m.refreshLockFn != nil {
		return m.refreshLockFn(ctx, noteID, userID)
	}
	return nil
}

func (m *mockNoteRepo) CreateVersion(ctx context.Context, v *NoteVersion) error {
	if m.createVersionFn != nil {
		return m.createVersionFn(ctx, v)
	}
	return nil
}

func (m *mockNoteRepo) ListVersions(ctx context.Context, noteID string, limit int) ([]NoteVersion, error) {
	if m.listVersionsFn != nil {
		return m.listVersionsFn(ctx, noteID, limit)
	}
	return nil, nil
}

func (m *mockNoteRepo) FindVersionByID(ctx context.Context, id string) (*NoteVersion, error) {
	if m.findVersionByIDFn != nil {
		return m.findVersionByIDFn(ctx, id)
	}
	return nil, apperror.NewNotFound("version not found")
}

func (m *mockNoteRepo) PruneVersions(ctx context.Context, noteID string, keep int) error {
	if m.pruneVersionsFn != nil {
		return m.pruneVersionsFn(ctx, noteID, keep)
	}
	return nil
}

// --- Test Helpers ---

// player is a plain member viewer: no GM visibility.
func player(userID string) permissions.Viewer {
	return permissions.RequestViewer(permissions.RolePlayer, userID)
}

// gm is a campaign Owner viewer.
func gm(userID string) permissions.Viewer {
	return permissions.RequestViewer(permissions.RoleOwner, userID)
}

// assertAppError checks that err is an *apperror.AppError with the expected code.
func assertAppError(t *testing.T, err error, expectedCode int) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %d, got nil", expectedCode)
	}
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *apperror.AppError, got %T: %v", err, err)
	}
	if appErr.Code != expectedCode {
		t.Errorf("expected status %d, got %d (message: %s)", expectedCode, appErr.Code, appErr.Message)
	}
}

// sampleNote creates a note for testing.
func sampleNote() *Note {
	return &Note{
		ID:         "note-123",
		CampaignID: "camp-1",
		UserID:     "user-1",
		Title:      "Test Note",
		Content: []Block{
			{Type: "text", Value: "Hello world"},
			{Type: "checklist", Items: []ChecklistItem{
				{Text: "Item 1", Checked: false},
				{Text: "Item 2", Checked: true},
			}},
		},
		Color:     "#374151",
		Pinned:    false,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// --- Create Tests ---

func TestCreate_Success(t *testing.T) {
	var createdNote *Note
	repo := &mockNoteRepo{
		createFn: func(ctx context.Context, note *Note) error {
			createdNote = note
			return nil
		},
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return createdNote, nil
		},
	}

	svc := NewNoteService(repo)
	note, err := svc.Create(context.Background(), "camp-1", player("user-1"), CreateNoteRequest{
		Title:   "My Note",
		Content: []Block{{Type: "text", Value: "Hello"}},
		Color:   "#ff0000",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if note.Title != "My Note" {
		t.Errorf("expected title My Note, got %s", note.Title)
	}
	if note.Color != "#ff0000" {
		t.Errorf("expected color #ff0000, got %s", note.Color)
	}
	if note.CampaignID != "camp-1" {
		t.Errorf("expected campaign camp-1, got %s", note.CampaignID)
	}
	if note.UserID != "user-1" {
		t.Errorf("expected user user-1, got %s", note.UserID)
	}
	if note.ID == "" {
		t.Error("expected ID to be generated")
	}
}

func TestCreate_EmptyTitleDefaultsToUntitled(t *testing.T) {
	var capturedTitle string
	repo := &mockNoteRepo{
		createFn: func(ctx context.Context, note *Note) error {
			capturedTitle = note.Title
			return nil
		},
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return &Note{ID: id, Title: capturedTitle}, nil
		},
	}

	svc := NewNoteService(repo)
	note, err := svc.Create(context.Background(), "camp-1", player("user-1"), CreateNoteRequest{
		Title: "",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if note.Title != "Untitled" {
		t.Errorf("expected Untitled, got %s", note.Title)
	}
}

func TestCreate_WhitespaceOnlyTitleDefaultsToUntitled(t *testing.T) {
	var capturedTitle string
	repo := &mockNoteRepo{
		createFn: func(ctx context.Context, note *Note) error {
			capturedTitle = note.Title
			return nil
		},
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return &Note{ID: id, Title: capturedTitle}, nil
		},
	}

	svc := NewNoteService(repo)
	note, err := svc.Create(context.Background(), "camp-1", player("user-1"), CreateNoteRequest{
		Title: "   ",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if note.Title != "Untitled" {
		t.Errorf("expected Untitled, got %s", note.Title)
	}
}

func TestCreate_TitleTooLong(t *testing.T) {
	svc := NewNoteService(&mockNoteRepo{})
	longTitle := string(make([]byte, 201))
	for i := range longTitle {
		longTitle = longTitle[:i] + "a" + longTitle[i+1:]
	}
	_, err := svc.Create(context.Background(), "camp-1", player("user-1"), CreateNoteRequest{
		Title: longTitle,
	})
	assertAppError(t, err, 400)
}

func TestCreate_DefaultColor(t *testing.T) {
	var capturedColor string
	repo := &mockNoteRepo{
		createFn: func(ctx context.Context, note *Note) error {
			capturedColor = note.Color
			return nil
		},
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return &Note{ID: id, Color: capturedColor}, nil
		},
	}

	svc := NewNoteService(repo)
	note, err := svc.Create(context.Background(), "camp-1", player("user-1"), CreateNoteRequest{
		Title: "Test",
		Color: "", // Empty should default.
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if note.Color != "#374151" {
		t.Errorf("expected default color #374151, got %s", note.Color)
	}
}

func TestCreate_NilContentDefaultsToEmpty(t *testing.T) {
	var capturedContent []Block
	repo := &mockNoteRepo{
		createFn: func(ctx context.Context, note *Note) error {
			capturedContent = note.Content
			return nil
		},
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return &Note{ID: id, Content: capturedContent}, nil
		},
	}

	svc := NewNoteService(repo)
	note, err := svc.Create(context.Background(), "camp-1", player("user-1"), CreateNoteRequest{
		Title:   "Test",
		Content: nil,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if note.Content == nil {
		t.Error("expected non-nil content slice")
	}
	if len(note.Content) != 0 {
		t.Errorf("expected empty content, got %d blocks", len(note.Content))
	}
}

func TestCreate_WithEntityScope(t *testing.T) {
	var capturedEntityID *string
	entityID := "entity-42"
	repo := &mockNoteRepo{
		createFn: func(ctx context.Context, note *Note) error {
			capturedEntityID = note.EntityID
			return nil
		},
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return &Note{ID: id, EntityID: capturedEntityID}, nil
		},
	}

	svc := NewNoteService(repo)
	note, err := svc.Create(context.Background(), "camp-1", player("user-1"), CreateNoteRequest{
		Title:    "Test",
		EntityID: &entityID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if note.EntityID == nil || *note.EntityID != "entity-42" {
		t.Errorf("expected entity ID entity-42, got %v", note.EntityID)
	}
}

func TestCreate_RepoError(t *testing.T) {
	repo := &mockNoteRepo{
		createFn: func(ctx context.Context, note *Note) error {
			return errors.New("db error")
		},
	}

	svc := NewNoteService(repo)
	_, err := svc.Create(context.Background(), "camp-1", player("user-1"), CreateNoteRequest{
		Title: "Test",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- Update Tests ---

func TestUpdate_Success(t *testing.T) {
	existing := sampleNote()
	repo := &mockNoteRepo{
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return existing, nil
		},
	}

	svc := NewNoteService(repo)
	newTitle := "Updated Title"
	pinned := true
	note, err := svc.Update(context.Background(), "note-123", player("user-1"), UpdateNoteRequest{
		Title:  &newTitle,
		Pinned: &pinned,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if note.Title != "Updated Title" {
		t.Errorf("expected Updated Title, got %s", note.Title)
	}
	if !note.Pinned {
		t.Error("expected pinned to be true")
	}
}

func TestUpdate_NotFound(t *testing.T) {
	svc := NewNoteService(&mockNoteRepo{})
	newTitle := "Test"
	_, err := svc.Update(context.Background(), "nonexistent", player("user-1"), UpdateNoteRequest{
		Title: &newTitle,
	})
	assertAppError(t, err, 404)
}

func TestUpdate_TitleTooLong(t *testing.T) {
	existing := sampleNote()
	repo := &mockNoteRepo{
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return existing, nil
		},
	}

	svc := NewNoteService(repo)
	longTitle := string(make([]byte, 201))
	for i := range longTitle {
		longTitle = longTitle[:i] + "x" + longTitle[i+1:]
	}
	_, err := svc.Update(context.Background(), "note-123", player("user-1"), UpdateNoteRequest{
		Title: &longTitle,
	})
	assertAppError(t, err, 400)
}

func TestUpdate_EmptyTitleBecomesUntitled(t *testing.T) {
	existing := sampleNote()
	repo := &mockNoteRepo{
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return existing, nil
		},
	}

	svc := NewNoteService(repo)
	emptyTitle := ""
	note, err := svc.Update(context.Background(), "note-123", player("user-1"), UpdateNoteRequest{
		Title: &emptyTitle,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if note.Title != "Untitled" {
		t.Errorf("expected Untitled, got %s", note.Title)
	}
}

func TestUpdate_ContentReplacement(t *testing.T) {
	existing := sampleNote()
	repo := &mockNoteRepo{
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return existing, nil
		},
	}

	svc := NewNoteService(repo)
	newContent := []Block{{Type: "text", Value: "Replaced"}}
	note, err := svc.Update(context.Background(), "note-123", player("user-1"), UpdateNoteRequest{
		Content: &newContent,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(note.Content) != 1 || note.Content[0].Value != "Replaced" {
		t.Errorf("expected replaced content, got %v", note.Content)
	}
}

func TestUpdate_ColorChange(t *testing.T) {
	existing := sampleNote()
	repo := &mockNoteRepo{
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return existing, nil
		},
	}

	svc := NewNoteService(repo)
	newColor := "#ff5733"
	note, err := svc.Update(context.Background(), "note-123", player("user-1"), UpdateNoteRequest{
		Color: &newColor,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if note.Color != "#ff5733" {
		t.Errorf("expected #ff5733, got %s", note.Color)
	}
}

// --- Delete Tests ---

func TestDelete_Success(t *testing.T) {
	var deletedID string
	repo := &mockNoteRepo{
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return &Note{ID: id, CampaignID: "camp-1"}, nil
		},
		deleteFn: func(ctx context.Context, id string) error {
			deletedID = id
			return nil
		},
	}

	svc := NewNoteService(repo)
	err := svc.Delete(context.Background(), "note-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deletedID != "note-123" {
		t.Errorf("expected note-123, got %s", deletedID)
	}
}

func TestDelete_NotFound(t *testing.T) {
	repo := &mockNoteRepo{
		deleteFn: func(ctx context.Context, id string) error {
			return apperror.NewNotFound("not found")
		},
	}

	svc := NewNoteService(repo)
	err := svc.Delete(context.Background(), "nonexistent")
	assertAppError(t, err, 404)
}

// --- ToggleCheck Tests ---

func TestToggleCheck_Success(t *testing.T) {
	existing := sampleNote()
	// Item 0 starts as unchecked.
	repo := &mockNoteRepo{
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return existing, nil
		},
	}

	svc := NewNoteService(repo)
	note, err := svc.ToggleCheck(context.Background(), "note-123", ToggleCheckRequest{
		BlockIndex: 1, // The checklist block.
		ItemIndex:  0, // First item (unchecked -> checked).
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !note.Content[1].Items[0].Checked {
		t.Error("expected item 0 to be toggled to checked")
	}
}

func TestToggleCheck_ToggleCheckedToUnchecked(t *testing.T) {
	existing := sampleNote()
	// Item 1 starts as checked.
	repo := &mockNoteRepo{
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return existing, nil
		},
	}

	svc := NewNoteService(repo)
	note, err := svc.ToggleCheck(context.Background(), "note-123", ToggleCheckRequest{
		BlockIndex: 1,
		ItemIndex:  1, // Second item (checked -> unchecked).
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if note.Content[1].Items[1].Checked {
		t.Error("expected item 1 to be toggled to unchecked")
	}
}

func TestToggleCheck_BlockIndexOutOfRange(t *testing.T) {
	existing := sampleNote()
	repo := &mockNoteRepo{
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return existing, nil
		},
	}

	svc := NewNoteService(repo)
	_, err := svc.ToggleCheck(context.Background(), "note-123", ToggleCheckRequest{
		BlockIndex: 99,
		ItemIndex:  0,
	})
	assertAppError(t, err, 400)
}

func TestToggleCheck_NegativeBlockIndex(t *testing.T) {
	existing := sampleNote()
	repo := &mockNoteRepo{
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return existing, nil
		},
	}

	svc := NewNoteService(repo)
	_, err := svc.ToggleCheck(context.Background(), "note-123", ToggleCheckRequest{
		BlockIndex: -1,
		ItemIndex:  0,
	})
	assertAppError(t, err, 400)
}

func TestToggleCheck_NotChecklist(t *testing.T) {
	existing := sampleNote()
	repo := &mockNoteRepo{
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return existing, nil
		},
	}

	svc := NewNoteService(repo)
	_, err := svc.ToggleCheck(context.Background(), "note-123", ToggleCheckRequest{
		BlockIndex: 0, // Text block, not checklist.
		ItemIndex:  0,
	})
	assertAppError(t, err, 400)
}

func TestToggleCheck_ItemIndexOutOfRange(t *testing.T) {
	existing := sampleNote()
	repo := &mockNoteRepo{
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return existing, nil
		},
	}

	svc := NewNoteService(repo)
	_, err := svc.ToggleCheck(context.Background(), "note-123", ToggleCheckRequest{
		BlockIndex: 1,
		ItemIndex:  99,
	})
	assertAppError(t, err, 400)
}

func TestToggleCheck_NoteNotFound(t *testing.T) {
	svc := NewNoteService(&mockNoteRepo{})
	_, err := svc.ToggleCheck(context.Background(), "nonexistent", ToggleCheckRequest{
		BlockIndex: 0,
		ItemIndex:  0,
	})
	assertAppError(t, err, 404)
}

// --- List Tests ---

func TestListByUserAndCampaign_ListsAsAPlainMember(t *testing.T) {
	var got permissions.Viewer
	var gotScope ListScope
	repo := &mockNoteRepo{
		listVisibleFn: func(ctx context.Context, campaignID string, v permissions.Viewer, scope ListScope) ([]Note, error) {
			got, gotScope = v, scope
			return []Note{{ID: "1"}, {ID: "2"}}, nil
		},
	}

	svc := NewNoteService(repo)
	notes, err := svc.ListByUserAndCampaign(context.Background(), "user-1", "camp-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notes) != 2 {
		t.Errorf("expected 2 notes, got %d", len(notes))
	}
	if got.UserID() != "user-1" || permissions.CanSeeDmOnly(got.Role()) {
		t.Errorf("must list as a plain member (no GM visibility), got role %d user %q", got.Role(), got.UserID())
	}
	if gotScope.Kind != "" {
		t.Errorf("expected the unscoped list, got %q", gotScope.Kind)
	}
}

func TestListVisible_PassesScopeThrough(t *testing.T) {
	var gotScope ListScope
	repo := &mockNoteRepo{
		listVisibleFn: func(ctx context.Context, campaignID string, v permissions.Viewer, scope ListScope) ([]Note, error) {
			gotScope = scope
			return []Note{{ID: "1"}}, nil
		},
	}

	svc := NewNoteService(repo)
	if _, err := svc.ListVisible(context.Background(), "camp-1", player("user-1"), ListScope{Kind: "entity", EntityID: "entity-42"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotScope.Kind != "entity" || gotScope.EntityID != "entity-42" {
		t.Errorf("scope not passed through: %+v", gotScope)
	}
}

// --- generateID Tests ---

func TestGenerateID_Format(t *testing.T) {
	id := generateID()
	// Should be UUID-like format: 8-4-4-4-12 = 36 chars.
	if len(id) != 36 {
		t.Errorf("expected 36-char ID, got %d chars: %s", len(id), id)
	}
	if id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' {
		t.Errorf("expected UUID-like format, got %s", id)
	}
}

func TestGenerateID_Unique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := generateID()
		if seen[id] {
			t.Fatalf("ID collision after %d iterations", i)
		}
		seen[id] = true
	}
}

// --- Folder Tests ---

func TestCreate_Folder(t *testing.T) {
	var createdNote *Note
	repo := &mockNoteRepo{
		createFn: func(ctx context.Context, note *Note) error {
			createdNote = note
			return nil
		},
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return createdNote, nil
		},
	}

	svc := NewNoteService(repo)
	note, err := svc.Create(context.Background(), "camp-1", player("user-1"), CreateNoteRequest{
		Title:    "My Folder",
		IsFolder: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !note.IsFolder {
		t.Error("expected IsFolder to be true")
	}
	if note.Title != "My Folder" {
		t.Errorf("expected title 'My Folder', got %q", note.Title)
	}
}

func TestCreate_NoteWithParentID(t *testing.T) {
	parentID := "folder-1"
	folder := &Note{ID: "folder-1", CampaignID: "camp-1", UserID: "user-1", IsFolder: true}
	var createdNote *Note
	repo := &mockNoteRepo{
		createFn: func(ctx context.Context, note *Note) error {
			createdNote = note
			return nil
		},
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			if id == "folder-1" {
				return folder, nil
			}
			return createdNote, nil
		},
	}

	svc := NewNoteService(repo)
	note, err := svc.Create(context.Background(), "camp-1", player("user-1"), CreateNoteRequest{
		Title:    "Child Note",
		ParentID: &parentID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if note.ParentID == nil || *note.ParentID != "folder-1" {
		t.Errorf("expected parentId 'folder-1', got %v", note.ParentID)
	}
}

func TestUpdate_MoveNoteToFolder(t *testing.T) {
	existing := sampleNote()
	folder := &Note{ID: "folder-1", CampaignID: "camp-1", UserID: "user-1", IsFolder: true}
	repo := &mockNoteRepo{
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			if id == "folder-1" {
				return folder, nil
			}
			return existing, nil
		},
	}

	svc := NewNoteService(repo)
	parentID := "folder-1"
	note, err := svc.Update(context.Background(), "note-123", player("user-1"), UpdateNoteRequest{
		ParentID: &parentID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if note.ParentID == nil || *note.ParentID != "folder-1" {
		t.Errorf("expected parentId 'folder-1', got %v", note.ParentID)
	}
}

func TestUpdate_MoveNoteToTopLevel(t *testing.T) {
	parentID := "folder-1"
	existing := sampleNote()
	existing.ParentID = &parentID

	repo := &mockNoteRepo{
		findByIDFn: func(ctx context.Context, id string) (*Note, error) {
			return existing, nil
		},
	}

	svc := NewNoteService(repo)
	emptyParent := ""
	note, err := svc.Update(context.Background(), "note-123", player("user-1"), UpdateNoteRequest{
		ParentID: &emptyParent,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if note.ParentID != nil {
		t.Errorf("expected nil parentId, got %v", note.ParentID)
	}
}
