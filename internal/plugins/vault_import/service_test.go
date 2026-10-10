package vault_import

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// fakePages is an in-memory PageStore. CreatePage records no visibility on
// purpose: the port has no way to ask for anything but GM only, which is the
// property the tests rely on.
type fakePages struct {
	mu       sync.Mutex
	next     int
	pages    map[string]*fakePage
	order    []string
	existing map[string]bool
	kindsErr error
	onCreate func(n int)
}

type fakePage struct {
	name, parent, label string
	kind                int
	body, json          string
}

func newFakePages() *fakePages {
	return &fakePages{pages: map[string]*fakePage{}, existing: map[string]bool{}}
}

func (f *fakePages) Kinds(context.Context, string) ([]PageKind, error) {
	if f.kindsErr != nil {
		return nil, f.kindsErr
	}
	return []PageKind{{ID: 1, Slug: "note", Enabled: true}, {ID: 2, Slug: "npc", Enabled: true}}, nil
}

func (f *fakePages) NameTaken(_ context.Context, _, name string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.existing[name], nil
}

func (f *fakePages) CreatePage(_ context.Context, _, _ string, p NewPage) (string, error) {
	f.mu.Lock()
	f.next++
	// uuid-shaped, as entity ids are: the link rewrite only trusts that shape.
	id := fmt.Sprintf("00000000-0000-4000-9000-%012d", f.next)
	f.pages[id] = &fakePage{name: p.Name, parent: p.ParentID, kind: p.KindID, label: p.Label}
	f.order = append(f.order, id)
	n := f.next
	cb := f.onCreate
	f.mu.Unlock()
	if cb != nil {
		cb(n)
	}
	return id, nil
}

func (f *fakePages) SetBody(_ context.Context, id, j, h string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.pages[id]
	if !ok {
		return errors.New("no such page")
	}
	p.json, p.body = j, h
	return nil
}

func (f *fakePages) Rename(_ context.Context, id, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pages[id].name = name
	return nil
}

func (f *fakePages) byName(name string) (string, *fakePage) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, id := range f.order {
		if f.pages[id].name == name {
			return id, f.pages[id]
		}
	}
	return "", nil
}

type fakePictures struct {
	mu     sync.Mutex
	stored []string
}

func (f *fakePictures) StorePicture(_ context.Context, _, _, name string, _ []byte) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stored = append(f.stored, name)
	// A uuid-shaped id, as the media service returns.
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", len(f.stored)), nil
}

type fakeFiles struct {
	mu       sync.Mutex
	attached []string // "pageID:name"
}

func (f *fakeFiles) CanAttach(name string) bool { return strings.HasSuffix(name, ".pdf") }
func (f *fakeFiles) AttachGMOnly(_ context.Context, _, pageID, _, name string, _ []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attached = append(f.attached, pageID+":"+name)
	return nil
}

type fakeAudit struct {
	mu      sync.Mutex
	actions []string
}

func (f *fakeAudit) LogCampaignEvent(_ context.Context, _, action string, _ map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.actions = append(f.actions, action)
}

type rig struct {
	svc      *Service
	pages    *fakePages
	pictures *fakePictures
	files    *fakeFiles
	audit    *fakeAudit
}

func newRig(t *testing.T, ctx context.Context) *rig {
	t.Helper()
	r := &rig{pages: newFakePages(), pictures: &fakePictures{}, files: &fakeFiles{}, audit: &fakeAudit{}}
	r.svc = NewService(Deps{
		Pages: r.pages, Pictures: r.pictures, Files: r.files, Audit: r.audit,
		ToJSON:    func(h string) (string, error) { return "J:" + h, nil },
		Lifecycle: ctx,
		TempDir:   t.TempDir(),
		Now:       func() time.Time { return time.Date(2026, 3, 4, 5, 6, 0, 0, time.UTC) },
	})
	return r
}

// importZip previews and runs an import to the end and returns the final view.
func (r *rig) importZip(t *testing.T, zipPath string) JobView {
	t.Helper()
	p, err := r.svc.Preview("camp", "owner", zipPath, "Vault.zip")
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	jv, err := r.svc.Start("camp", "owner", p.UploadID)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		v, ok := r.svc.Job("camp", jv.ID)
		if !ok {
			t.Fatal("job vanished")
		}
		if v.State != StateRunning {
			return v
		}
		if time.Now().After(deadline) {
			t.Fatal("import did not finish")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func fixtureZip(t *testing.T) string {
	return writeZip(t,
		zent{name: "Vault/Home.md", data: "---\nname: Welcome\ntype: npc\nsubcategory: Host\nvisibility: public\n---\nHi [[Bob]], see [[Bob|the baker]] and [[Bob#Past]].\n\n[[Ghost]] is not here. ![[map.png|300]]\n\n<script>alert(1)</script>\n\nRules: ![[rules.pdf]]\n\n```\n[[Bob]]\n```\n"},
		zent{name: "Vault/People/Bob.md", data: "Back to [Home](../Home.md). Same map: ![[map.png]] and ![x](../img/map.png)\n\n[[#Top]]"},
		zent{name: "Vault/People/Alice.md", data: "Alice knows [[Bob]]."},
		zent{name: "Vault/img/map.png", data: "not really a png"},
		zent{name: "Vault/docs/rules.pdf", data: "%PDF-1.4"},
		zent{name: "Vault/.obsidian/app.json", data: "{}"},
	)
}

func TestImport_EndToEnd(t *testing.T) {
	r := newRig(t, context.Background())
	v := r.importZip(t, fixtureZip(t))
	if v.State != StateDone {
		t.Fatalf("state = %s (%s), want done", v.State, v.Error)
	}

	rootID, root := r.pages.byName("Imported")
	if root == nil {
		t.Fatal("no Imported folder")
	}
	if root.parent != "" {
		t.Errorf("Imported folder has parent %q, want the top level", root.parent)
	}
	if !strings.Contains(root.body, "GM only") {
		t.Errorf("Imported folder summary lacks the GM-only line: %s", root.body)
	}

	// Tree: Welcome and People under Imported; Bob and Alice under People.
	_, welcome := r.pages.byName("Welcome")
	peopleID, people := r.pages.byName("People")
	bobID, bob := r.pages.byName("Bob")
	_, alice := r.pages.byName("Alice")
	if welcome == nil || people == nil || bob == nil || alice == nil {
		t.Fatalf("missing pages: %+v", r.pages.pages)
	}
	if welcome.parent != rootID || people.parent != rootID {
		t.Errorf("top pages should sit under Imported (%s): Welcome=%q People=%q", rootID, welcome.parent, people.parent)
	}
	if bob.parent != peopleID || alice.parent != peopleID {
		t.Errorf("Bob and Alice should sit under People (%s): %q %q", peopleID, bob.parent, alice.parent)
	}
	if welcome.kind != 2 || welcome.label != "Host" {
		t.Errorf("front matter type/subcategory not applied: kind=%d label=%q", welcome.kind, welcome.label)
	}

	// Links: every form points at Bob's page as the editor's mention markup.
	want := `data-mention-id="` + bobID + `"`
	if n := strings.Count(welcome.body, want); n != 3 {
		t.Errorf("Welcome has %d links to Bob, want 3:\n%s", n, welcome.body)
	}
	if !strings.Contains(welcome.body, `href="/campaigns/camp/entities/`+bobID+`"`) {
		t.Errorf("link does not use the page URL: %s", welcome.body)
	}
	if !strings.Contains(welcome.body, "Ghost") || strings.Contains(welcome.body, "[[Ghost") {
		t.Errorf("unresolved link should read as plain text Ghost: %s", welcome.body)
	}
	if !strings.Contains(welcome.body, "[[Bob]]") {
		t.Errorf("a [[link]] inside a code fence must stay as written: %s", welcome.body)
	}
	if strings.Contains(welcome.body, "<script") {
		t.Errorf("script survived the sanitiser: %s", welcome.body)
	}
	if !strings.Contains(bob.body, `data-mention-id="`) {
		t.Errorf("Bob's link back to Home (a Markdown link) was not turned into a page link: %s", bob.body)
	}

	// Pictures: one upload no matter how often the map is shown.
	if len(r.pictures.stored) != 1 || r.pictures.stored[0] != "map.png" {
		t.Errorf("stored pictures = %v, want map.png once", r.pictures.stored)
	}
	for _, p := range []*fakePage{welcome, bob} {
		if !strings.Contains(p.body, `src="/media/00000000-0000-4000-8000-000000000001"`) {
			t.Errorf("picture not placed in %q: %s", p.name, p.body)
		}
	}
	if !strings.Contains(welcome.body, "ce-img--w") {
		t.Errorf("the |300 size hint was lost: %s", welcome.body)
	}

	// Other files go to the page's Files section, once, for the page that named them.
	welcomeID, _ := r.pages.byName("Welcome")
	if len(r.files.attached) != 1 || r.files.attached[0] != welcomeID+":rules.pdf" {
		t.Errorf("attached = %v, want rules.pdf on Welcome", r.files.attached)
	}

	if v.Summary == nil || v.Summary.Pages != 4 || v.Summary.Pictures != 1 || v.Summary.FilesAttached != 1 {
		t.Errorf("summary = %+v, want 4 pages (Welcome, People, Bob, Alice; the Imported folder is not counted)", v.Summary)
	}
	if len(r.audit.actions) != 1 || r.audit.actions[0] != "campaign.vault_import.finished" {
		t.Errorf("audit = %v", r.audit.actions)
	}
}

func TestImport_SecondImportMakesASecondFolder(t *testing.T) {
	r := newRig(t, context.Background())
	r.pages.existing["Imported"] = true
	v := r.importZip(t, writeZip(t, zent{name: "a.md", data: "x"}))
	if v.State != StateDone {
		t.Fatalf("state = %s (%s)", v.State, v.Error)
	}
	if v.RootName != "Imported 2026-03-04 05:06" {
		t.Errorf("root = %q, want a dated second folder", v.RootName)
	}
}

func TestImport_FailuresSayWhatWasWritten(t *testing.T) {
	t.Run("before anything is written", func(t *testing.T) {
		r := newRig(t, context.Background())
		r.pages.kindsErr = apperror.NewInternal(errors.New("db down"))
		v := r.importZip(t, writeZip(t, zent{name: "a.md", data: "x"}))
		if v.State != StateFailed || v.Partial {
			t.Fatalf("state=%s partial=%v, want failed and not partial", v.State, v.Partial)
		}
		if !strings.Contains(v.Error, "Nothing was imported") {
			t.Errorf("message = %q", v.Error)
		}
		if len(r.pages.pages) != 0 {
			t.Errorf("pages were written: %v", r.pages.pages)
		}
	})

	t.Run("partway through", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := newRig(t, ctx)
		// The server stops while the third page is being created.
		r.pages.onCreate = func(n int) {
			if n == 3 {
				cancel()
			}
		}
		v := r.importZip(t, writeZip(t,
			zent{name: "a.md", data: "x"}, zent{name: "b.md", data: "x"},
			zent{name: "c.md", data: "x"}, zent{name: "d.md", data: "x"},
		))
		if v.State != StateFailed || !v.Partial {
			t.Fatalf("state=%s partial=%v, want failed and partial", v.State, v.Partial)
		}
		if !strings.Contains(v.Error, "Delete that folder") || !strings.Contains(v.Error, "(unfinished)") {
			t.Errorf("message = %q", v.Error)
		}
		_, root := r.pages.byName("Imported (unfinished)")
		if root == nil {
			t.Fatalf("the folder was not renamed: %+v", r.pages.pages)
		}
		if !strings.Contains(root.body, "stopped before it finished") {
			t.Errorf("folder note = %s", root.body)
		}
		if got := r.audit.actions; len(got) != 1 || got[0] != "campaign.vault_import.failed" {
			t.Errorf("audit = %v", got)
		}
	})
}

func TestStart_Guards(t *testing.T) {
	r := newRig(t, context.Background())
	p, err := r.svc.Preview("camp", "owner", writeZip(t, zent{name: "a.md", data: "x"}), "v.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Start("other-camp", "owner", p.UploadID); err == nil {
		t.Error("an upload was started in another campaign")
	}
	if _, err := r.svc.Start("camp", "someone-else", p.UploadID); err == nil {
		t.Error("an upload was started by another person")
	}
	if _, err := r.svc.Start("camp", "owner", "nope"); err == nil {
		t.Error("an unknown upload id was accepted")
	}
	if _, ok := r.svc.Job("other-camp", "x"); ok {
		t.Error("an unknown job was found")
	}
}

func TestPreview_RefusesWhatIsNotAVault(t *testing.T) {
	r := newRig(t, context.Background())
	if _, err := r.svc.Preview("camp", "owner", writeZip(t, zent{name: "pic.png", data: "x"}), "v.zip"); err == nil || !strings.Contains(err.Error(), "no Markdown notes") {
		t.Errorf("err = %v, want the no-notes message", err)
	}
	if _, err := r.svc.Preview("camp", "owner", writeZip(t, zent{name: "../x.md", data: "x"}), "v.zip"); err == nil {
		t.Error("a traversal zip was previewed")
	}
	if len(r.pages.pages) != 0 {
		t.Error("a refused preview wrote pages")
	}
}

func TestPreview_ReportsWhatWillHappen(t *testing.T) {
	r := newRig(t, context.Background())
	p, err := r.svc.Preview("camp", "owner", fixtureZip(t), "Vault.zip")
	if err != nil {
		t.Fatal(err)
	}
	s := p.Stats
	if s.Pages != 3 || s.FolderPages != 1 {
		t.Errorf("pages=%d folders=%d, want 3 notes and 1 folder", s.Pages, s.FolderPages)
	}
	if s.LinksPlain != 1 || len(s.UnresolvedNames) != 1 || s.UnresolvedNames[0] != "Ghost" {
		t.Errorf("unresolved = %d %v, want only Ghost", s.LinksPlain, s.UnresolvedNames)
	}
	if s.Pictures != 1 || s.Files != 1 {
		t.Errorf("pictures=%d files=%d, want 1 and 1", s.Pictures, s.Files)
	}
	if len(r.pages.pages) != 0 || len(r.pictures.stored) != 0 {
		t.Error("the preview wrote something")
	}
}
