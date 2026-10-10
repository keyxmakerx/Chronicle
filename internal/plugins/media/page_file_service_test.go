package media

import (
	"context"
	"net/http"
	"os"
	"sort"
	"sync"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// --- doubles ---

// memPageFiles is an in-memory PageFileRepository.
type memPageFiles struct {
	mu    sync.Mutex
	files map[string]PageFile
}

func newMemPageFiles() *memPageFiles { return &memPageFiles{files: map[string]PageFile{}} }

func (m *memPageFiles) Bind(_ context.Context, f PageFile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[f.ID] = f
	return nil
}
func (m *memPageFiles) Find(_ context.Context, id string) (*PageFile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.files[id]
	if !ok {
		return nil, apperror.NewNotFound("file not found")
	}
	return &f, nil
}
func (m *memPageFiles) ListByEntity(_ context.Context, campaignID, entityID string, includeGMOnly bool) ([]PageFile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []PageFile
	for _, f := range m.files {
		if f.CampaignID == campaignID && f.EntityID == entityID && (includeGMOnly || !f.GMOnly) {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func (m *memPageFiles) CountByEntity(_ context.Context, entityID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, f := range m.files {
		if f.EntityID == entityID {
			n++
		}
	}
	return n, nil
}
func (m *memPageFiles) SetGMOnly(_ context.Context, id string, gm bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	f := m.files[id]
	f.GMOnly = gm
	m.files[id] = f
	return nil
}

// fakeAccess answers PageAccess from a table keyed by campaign, page and user.
type fakeAccess struct {
	table map[string]PageAccess
	roles []int
}

func accessKey(campaign, entity, user string) string { return campaign + "|" + entity + "|" + user }

func (f *fakeAccess) PageAccess(_ context.Context, campaignID, entityID string, role int, userID string) (PageAccess, error) {
	f.roles = append(f.roles, role)
	return f.table[accessKey(campaignID, entityID, userID)], nil
}

// pageFileFixture is a real media service (real type checks, sanitising and
// disk writes) over in-memory stores, behind the real page file service.
type pageFileFixture struct {
	svc    PageFileService
	media  MediaService
	binds  *memPageFiles
	stored map[string]*MediaFile
	access *fakeAccess
	dir    string
}

const (
	campA = "camp-a"
	campB = "camp-b"
	pgOpen = "page-open"
	pgPriv = "page-private"
	pgGone = "page-trashed"
)

// The people. Roles are the promoted campaign role (player 1, scribe 2,
// owner 3). editor is a player the owner gave edit on the page; viewer is a
// player who can only read it.
var (
	owner  = PageFileViewer{UserID: "owner", Role: 3}
	scribe = PageFileViewer{UserID: "scribe", Role: 2}
	editor = PageFileViewer{UserID: "editor", Role: 1}
	viewer = PageFileViewer{UserID: "viewer", Role: 1}
)

func newPageFileFixture(t *testing.T) *pageFileFixture {
	t.Helper()
	dir := t.TempDir()
	stored := map[string]*MediaFile{}
	repo := &mockMediaRepo{
		createFn: func(_ context.Context, f *MediaFile) error { c := *f; stored[f.ID] = &c; return nil },
		findByIDFn: func(_ context.Context, id string) (*MediaFile, error) {
			if f, ok := stored[id]; ok {
				c := *f
				return &c, nil
			}
			return nil, apperror.NewNotFound("media file not found")
		},
		deleteFn: func(_ context.Context, id string) error {
			if _, ok := stored[id]; !ok {
				return apperror.NewNotFound("media file not found")
			}
			delete(stored, id)
			return nil
		},
	}
	mediaSvc := NewMediaService(repo, dir, 10<<20)
	// Players may edit a visible page only where the table says so.
	table := map[string]PageAccess{}
	for _, p := range []string{pgOpen} {
		table[accessKey(campA, p, "owner")] = PageAccess{CanView: true, CanEdit: true}
		table[accessKey(campA, p, "scribe")] = PageAccess{CanView: true, CanEdit: true}
		table[accessKey(campA, p, "editor")] = PageAccess{CanView: true, CanEdit: true}
		table[accessKey(campA, p, "viewer")] = PageAccess{CanView: true}
	}
	// A private page: only the GM tier sees it.
	table[accessKey(campA, pgPriv, "owner")] = PageAccess{CanView: true, CanEdit: true}
	table[accessKey(campA, pgPriv, "scribe")] = PageAccess{CanView: true, CanEdit: true}
	// A trashed page is seen by no one, the owner included (pgGone has no rows).
	access := &fakeAccess{table: table}
	binds := newMemPageFiles()
	return &pageFileFixture{
		svc: NewPageFileService(binds, mediaSvc, access), media: mediaSvc,
		binds: binds, stored: stored, access: access, dir: dir,
	}
}

// attach puts a file on a page as who, failing the test if it is refused.
func (f *pageFileFixture) attach(t *testing.T, who PageFileViewer, campaign, page, name string, data []byte, gm bool) *PageFile {
	t.Helper()
	pf, err := f.svc.Attach(context.Background(), PageFileUpload{CampaignID: campaign, EntityID: page, Viewer: who, Name: name, Bytes: data, GMOnly: gm})
	if err != nil {
		t.Fatalf("attach %s as %s: %v", name, who.UserID, err)
	}
	return pf
}

func code(err error) int {
	if err == nil {
		return 0
	}
	return apperror.SafeCode(err)
}

// --- tests ---

func TestPageFileService_Attach(t *testing.T) {
	pdf := []byte("%PDF-1.7\nbody")
	tests := []struct {
		name     string
		who      PageFileViewer
		campaign string
		page     string
		file     string
		data     []byte
		gm       bool
		want     int // 0 = ok, otherwise the HTTP status
	}{
		{"owner attaches", owner, campA, pgOpen, "map.pdf", pdf, false, 0},
		{"scribe attaches", scribe, campA, pgOpen, "map.pdf", pdf, false, 0},
		{"editor-player attaches", editor, campA, pgOpen, "map.pdf", pdf, false, 0},
		{"view-only player cannot", viewer, campA, pgOpen, "map.pdf", pdf, false, http.StatusForbidden},
		{"player who cannot see a private page gets not-found", editor, campA, pgPriv, "map.pdf", pdf, false, http.StatusNotFound},
		{"nobody attaches to a trashed page", owner, campA, pgGone, "map.pdf", pdf, false, http.StatusNotFound},
		{"another campaign's page", owner, campB, pgOpen, "map.pdf", pdf, false, http.StatusNotFound},
		{"owner marks GM only", owner, campA, pgOpen, "secret.pdf", pdf, true, 0},
		{"scribe marks GM only", scribe, campA, pgOpen, "secret.pdf", pdf, true, 0},
		{"editor-player cannot mark GM only", editor, campA, pgOpen, "secret.pdf", pdf, true, http.StatusForbidden},
		{"html is refused", owner, campA, pgOpen, "page.html", []byte("<html><script>alert(1)</script>"), false, http.StatusBadRequest},
		{"svg is refused", owner, campA, pgOpen, "logo.svg", []byte(`<svg onload="alert(1)"/>`), false, http.StatusBadRequest},
		{"exe is refused", owner, campA, pgOpen, "setup.exe", []byte("MZ"), false, http.StatusBadRequest},
		{"html renamed .png is refused by its content", owner, campA, pgOpen, "x.png", []byte("<html>hi</html>"), false, http.StatusBadRequest},
		{"program renamed .txt is refused", owner, campA, pgOpen, "notes.txt", []byte("#!/bin/sh\nid\n"), false, http.StatusBadRequest},
		{"empty file is refused", owner, campA, pgOpen, "empty.txt", nil, false, http.StatusBadRequest},
		{"markdown", editor, campA, pgOpen, "notes.md", []byte("# Notes"), false, 0},
		{"picture", editor, campA, pgOpen, "map.png", nil, false, 0}, // data filled in below
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newPageFileFixture(t)
			data := tt.data
			if tt.file == "map.png" {
				data = tinyPNG(t)
			}
			pf, err := f.svc.Attach(context.Background(), PageFileUpload{
				CampaignID: tt.campaign, EntityID: tt.page, Viewer: tt.who, Name: tt.file, Bytes: data, GMOnly: tt.gm,
			})
			if code(err) != tt.want {
				t.Fatalf("status = %d (%v), want %d", code(err), err, tt.want)
			}
			if tt.want != 0 {
				if n, _ := f.binds.CountByEntity(context.Background(), tt.page); n != 0 {
					t.Errorf("a refused attach left %d bindings", n)
				}
				if len(f.stored) != 0 {
					t.Errorf("a refused attach left %d stored files", len(f.stored))
				}
				return
			}
			stored := f.stored[pf.ID]
			if stored == nil || stored.UsageType != UsagePageFile {
				t.Fatalf("stored record = %+v, want usage_type %s", stored, UsagePageFile)
			}
			if pf.GMOnly != tt.gm {
				t.Errorf("GMOnly = %v, want %v", pf.GMOnly, tt.gm)
			}
			if _, err := os.Stat(f.media.FilePath(stored)); err != nil {
				t.Errorf("file not on disk: %v", err)
			}
		})
	}
}

func TestPageFileService_AttachAfterCapIsRefused(t *testing.T) {
	f := newPageFileFixture(t)
	for i := 0; i < maxPageFiles; i++ {
		_ = f.binds.Bind(context.Background(), PageFile{ID: string(rune('a'+i%26)) + string(rune('A'+i/26)), EntityID: pgOpen, CampaignID: campA})
	}
	_, err := f.svc.Attach(context.Background(), PageFileUpload{CampaignID: campA, EntityID: pgOpen, Viewer: owner, Name: "one-more.txt", Bytes: []byte("hi")})
	if code(err) != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", code(err))
	}
}

func TestPageFileService_List(t *testing.T) {
	f := newPageFileFixture(t)
	open := f.attach(t, editor, campA, pgOpen, "handout.txt", []byte("hi"), false)
	secret := f.attach(t, owner, campA, pgOpen, "plot.txt", []byte("twist"), true)
	other := f.attach(t, owner, campA, pgPriv, "hidden-page-file.txt", []byte("x"), false)

	tests := []struct {
		name       string
		who        PageFileViewer
		page       string
		wantIDs    []string
		wantAttach bool
		wantMark   bool
		wantStatus int
	}{
		{"owner sees all and manages", owner, pgOpen, []string{open.ID, secret.ID}, true, true, 0},
		{"scribe sees all and manages", scribe, pgOpen, []string{open.ID, secret.ID}, true, true, 0},
		{"editor-player sees only visible files but may attach", editor, pgOpen, []string{open.ID}, true, false, 0},
		{"view-only player sees only visible files", viewer, pgOpen, []string{open.ID}, false, false, 0},
		{"private page: a player gets not-found", editor, pgPriv, nil, false, false, http.StatusNotFound},
		{"private page: scribe sees its file", scribe, pgPriv, []string{other.ID}, true, true, 0},
		{"trashed page: no one, owner included", owner, pgGone, nil, false, false, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := f.svc.List(context.Background(), campA, tt.page, tt.who)
			if code(err) != tt.wantStatus {
				t.Fatalf("status = %d (%v), want %d", code(err), err, tt.wantStatus)
			}
			if err != nil {
				return
			}
			var ids []string
			for _, pf := range got.Files {
				ids = append(ids, pf.ID)
			}
			sort.Strings(ids)
			want := append([]string(nil), tt.wantIDs...)
			sort.Strings(want)
			if len(ids) != len(want) {
				t.Fatalf("files = %v, want %v", ids, want)
			}
			for i := range ids {
				if ids[i] != want[i] {
					t.Fatalf("files = %v, want %v", ids, want)
				}
			}
			if got.CanAttach != tt.wantAttach || got.CanMarkGMOnly != tt.wantMark {
				t.Errorf("CanAttach/CanMarkGMOnly = %v/%v, want %v/%v", got.CanAttach, got.CanMarkGMOnly, tt.wantAttach, tt.wantMark)
			}
		})
	}
}

func TestPageFileService_Open(t *testing.T) {
	f := newPageFileFixture(t)
	open := f.attach(t, editor, campA, pgOpen, "handout.txt", []byte("hi"), false)
	secret := f.attach(t, owner, campA, pgOpen, "plot.txt", []byte("twist"), true)
	onPrivate := f.attach(t, owner, campA, pgPriv, "dm-notes.txt", []byte("x"), false)

	tests := []struct {
		name     string
		who      PageFileViewer
		campaign string
		page     string
		file     *PageFile
		want     int
	}{
		{"owner opens a visible file", owner, campA, pgOpen, open, 0},
		{"scribe opens a visible file", scribe, campA, pgOpen, open, 0},
		{"editor-player opens a visible file", editor, campA, pgOpen, open, 0},
		{"view-only player opens a visible file", viewer, campA, pgOpen, open, 0},
		{"owner opens a GM-only file", owner, campA, pgOpen, secret, 0},
		{"scribe opens a GM-only file", scribe, campA, pgOpen, secret, 0},
		{"editor-player gets not-found for a GM-only file", editor, campA, pgOpen, secret, http.StatusNotFound},
		{"view-only player gets not-found for a GM-only file", viewer, campA, pgOpen, secret, http.StatusNotFound},
		{"player gets not-found for a file on a private page", editor, campA, pgPriv, onPrivate, http.StatusNotFound},
		{"scribe opens a file on a private page", scribe, campA, pgPriv, onPrivate, 0},
		{"a file asked for through a different page is not found", owner, campA, pgPriv, open, http.StatusNotFound},
		{"a file asked for through another campaign is not found", owner, campB, pgOpen, open, http.StatusNotFound},
		{"an unknown file is not found", owner, campA, pgOpen, &PageFile{ID: "nope"}, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pf, stored, err := f.svc.Open(context.Background(), tt.campaign, tt.page, tt.file.ID, tt.who)
			if code(err) != tt.want {
				t.Fatalf("status = %d (%v), want %d", code(err), err, tt.want)
			}
			if tt.want == 0 && (pf == nil || stored == nil || !stored.IsPageFile()) {
				t.Errorf("open returned %v / %v", pf, stored)
			}
		})
	}

	t.Run("a GM-only refusal is byte-for-byte the missing-file refusal", func(t *testing.T) {
		_, _, hidden := f.svc.Open(context.Background(), campA, pgOpen, secret.ID, editor)
		_, _, missing := f.svc.Open(context.Background(), campA, pgOpen, "nope", editor)
		a, b := hidden.(*apperror.AppError), missing.(*apperror.AppError)
		if a.Code != b.Code || a.Message != b.Message || a.Type != b.Type {
			t.Errorf("hidden = %+v, missing = %+v: a player could tell them apart", a, b)
		}
	})

	t.Run("the promoted role reaches the page rule", func(t *testing.T) {
		f.access.roles = nil
		_, _, _ = f.svc.Open(context.Background(), campA, pgOpen, open.ID, owner)
		if len(f.access.roles) == 0 || f.access.roles[0] != 3 {
			t.Errorf("page rule saw roles %v, want [3]", f.access.roles)
		}
	})
}

func TestPageFileService_Remove(t *testing.T) {
	tests := []struct {
		name string
		who  PageFileViewer
		gm   bool // the file is GM only
		want int
	}{
		{"owner removes", owner, false, 0},
		{"scribe removes", scribe, false, 0},
		{"editor-player removes", editor, false, 0},
		{"view-only player cannot", viewer, false, http.StatusForbidden},
		{"owner removes a GM-only file", owner, true, 0},
		{"editor-player cannot even find a GM-only file", editor, true, http.StatusNotFound},
		{"view-only player cannot find a GM-only file", viewer, true, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newPageFileFixture(t)
			pf := f.attach(t, owner, campA, pgOpen, "a.txt", []byte("hi"), tt.gm)
			path := f.media.FilePath(f.stored[pf.ID])
			_, err := f.svc.Remove(context.Background(), campA, pgOpen, pf.ID, tt.who)
			if code(err) != tt.want {
				t.Fatalf("status = %d (%v), want %d", code(err), err, tt.want)
			}
			_, onDisk := os.Stat(path)
			if tt.want == 0 {
				if f.stored[pf.ID] != nil || onDisk == nil {
					t.Errorf("file survived its removal (record %v, disk err %v)", f.stored[pf.ID], onDisk)
				}
			} else if f.stored[pf.ID] == nil || onDisk != nil {
				t.Errorf("a refused removal still removed the file")
			}
		})
	}

	t.Run("another campaign cannot remove it", func(t *testing.T) {
		f := newPageFileFixture(t)
		pf := f.attach(t, owner, campA, pgOpen, "a.txt", []byte("hi"), false)
		if _, err := f.svc.Remove(context.Background(), campB, pgOpen, pf.ID, owner); code(err) != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", code(err))
		}
	})
}

func TestPageFileService_SetGMOnly(t *testing.T) {
	tests := []struct {
		name string
		who  PageFileViewer
		want int
	}{
		{"owner marks", owner, 0},
		{"scribe marks", scribe, 0},
		{"editor-player cannot", editor, http.StatusForbidden},
		{"view-only player cannot", viewer, http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newPageFileFixture(t)
			pf := f.attach(t, owner, campA, pgOpen, "a.txt", []byte("hi"), false)
			_, err := f.svc.SetGMOnly(context.Background(), campA, pgOpen, pf.ID, tt.who, true)
			if code(err) != tt.want {
				t.Fatalf("status = %d (%v), want %d", code(err), err, tt.want)
			}
			got, _ := f.binds.Find(context.Background(), pf.ID)
			if got.GMOnly != (tt.want == 0) {
				t.Errorf("GMOnly = %v after the call", got.GMOnly)
			}
			if tt.want == 0 {
				// Once marked, the editor-player can no longer see it, and
				// unmarking brings it back.
				if _, _, err := f.svc.Open(context.Background(), campA, pgOpen, pf.ID, editor); code(err) != http.StatusNotFound {
					t.Errorf("editor-player still opens a file marked GM only: %v", err)
				}
				if _, err := f.svc.SetGMOnly(context.Background(), campA, pgOpen, pf.ID, tt.who, false); err != nil {
					t.Fatal(err)
				}
				if _, _, err := f.svc.Open(context.Background(), campA, pgOpen, pf.ID, editor); err != nil {
					t.Errorf("editor-player cannot open a file after it was unmarked: %v", err)
				}
			}
		})
	}
}

func TestPageFileService_FailedBindRemovesTheStoredFile(t *testing.T) {
	f := newPageFileFixture(t)
	svc := NewPageFileService(failingBinder{f.binds}, f.media, f.access)
	_, err := svc.Attach(context.Background(), PageFileUpload{CampaignID: campA, EntityID: pgOpen, Viewer: owner, Name: "a.txt", Bytes: []byte("hi")})
	if err == nil {
		t.Fatal("want an error when the binding fails")
	}
	if len(f.stored) != 0 {
		t.Errorf("an unbound file was left in storage: %d records", len(f.stored))
	}
}

type failingBinder struct{ *memPageFiles }

func (failingBinder) Bind(context.Context, PageFile) error { return context.DeadlineExceeded }

func TestPageFileService_UnwiredAccessRefusesEverything(t *testing.T) {
	f := newPageFileFixture(t)
	svc := NewPageFileService(f.binds, f.media, nil)
	if _, err := svc.List(context.Background(), campA, pgOpen, owner); code(err) != http.StatusNotFound {
		t.Errorf("List with no page rule: %d, want 404", code(err))
	}
	if _, err := svc.Attach(context.Background(), PageFileUpload{CampaignID: campA, EntityID: pgOpen, Viewer: owner, Name: "a.txt", Bytes: []byte("x")}); code(err) != http.StatusNotFound {
		t.Errorf("Attach with no page rule: %d, want 404", code(err))
	}
}

func TestUploadService_PageFileTypesOnlyForPageFiles(t *testing.T) {
	f := newPageFileFixture(t)
	// The general upload path must keep refusing documents and archives, and a
	// page file upload must keep refusing pictures' lookalikes.
	for _, tt := range []struct {
		name  string
		input UploadInput
	}{
		{"pdf as an attachment", UploadInput{CampaignID: campA, UploadedBy: "u", OriginalName: "a.pdf", MimeType: mimePDF, UsageType: UsageAttachment, FileBytes: []byte("%PDF-1.7 hi")}},
		{"zip as an entity image", UploadInput{CampaignID: campA, UploadedBy: "u", OriginalName: "a.zip", MimeType: mimeZip, UsageType: UsageEntityImage, FileBytes: []byte("PK")}},
		{"svg as a page file", UploadInput{CampaignID: campA, UploadedBy: "u", OriginalName: "a.svg", MimeType: "image/svg+xml", UsageType: UsagePageFile, FileBytes: []byte("<svg/>")}},
		{"html as a page file", UploadInput{CampaignID: campA, UploadedBy: "u", OriginalName: "a.html", MimeType: "text/html", UsageType: UsagePageFile, FileBytes: []byte("<html/>")}},
		{"page file with no campaign", UploadInput{UploadedBy: "u", OriginalName: "a.txt", MimeType: mimeText, UsageType: UsagePageFile, FileBytes: []byte("hi")}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt.input.FileSize = int64(len(tt.input.FileBytes))
			if _, err := f.media.Upload(context.Background(), tt.input); code(err) != http.StatusBadRequest {
				t.Fatalf("status = %d (%v), want 400", code(err), err)
			}
		})
	}
}

func TestPageFileUploadIsNeverDeduplicated(t *testing.T) {
	f := newPageFileFixture(t)
	a := f.attach(t, owner, campA, pgOpen, "a.txt", []byte("same bytes"), true)
	b := f.attach(t, editor, campA, pgOpen, "a.txt", []byte("same bytes"), false)
	if a.ID == b.ID {
		t.Fatal("two uploads of the same bytes shared one file, so one's GM-only mark would decide the other's")
	}
}
