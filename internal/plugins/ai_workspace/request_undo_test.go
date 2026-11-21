package ai_workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/connector"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// memUndo is the connector's Undo store in memory, scoped like the real one
// to the one owner the request fake knows.
type memUndo struct {
	mu      sync.Mutex
	repo    *reqRepo
	befores []connector.Before
	undone  map[int64]*time.Time
	nextID  int64
}

func (m *memUndo) SaveBefores(_ context.Context, _, _ string, id int64, _ string, bs []connector.Before) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, b := range bs {
		m.nextID++
		b.ID, b.RequestID = m.nextID, id
		m.befores = append(m.befores, b)
	}
	return nil
}

func (m *memUndo) Summaries(_ context.Context, _, _ string, ids []int64, _ time.Time) (map[int64]connector.UndoSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[int64]connector.UndoSummary{}
	for _, id := range ids {
		s := connector.UndoSummary{UndoneAt: m.undone[id]}
		for _, b := range m.befores {
			if b.RequestID == id && b.UndoneAt == nil {
				b.BeforeJSON = nil
				s.Left = append(s.Left, b)
			}
		}
		out[id] = s
	}
	return out, nil
}

func (m *memUndo) Claim(_ context.Context, _, _ string, id int64, at time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.undone == nil {
		m.undone = map[int64]*time.Time{}
	}
	if m.undone[id] != nil {
		return false, nil
	}
	m.undone[id] = &at
	return true, nil
}

func (m *memUndo) Release(_ context.Context, _, _ string, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.undone, id)
	return nil
}

func (m *memUndo) Live(_ context.Context, _, _ string, id int64, _ time.Time) ([]connector.Before, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []connector.Before
	for i := len(m.befores) - 1; i >= 0; i-- {
		if b := m.befores[i]; b.RequestID == id && b.UndoneAt == nil {
			out = append(out, b)
		}
	}
	return out, nil
}

func (m *memUndo) MarkUndone(_ context.Context, beforeID int64, _ string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.befores {
		if m.befores[i].ID == beforeID {
			m.befores[i].UndoneAt = &at
		}
	}
	return nil
}

func (m *memUndo) Purge(context.Context, time.Time) (int64, error) { return 0, nil }

// thingKind is an undoable kind over one string, the home page's shape: Before
// reads it, Apply writes it, Fingerprint reads it, Restore writes it back.
type thingKind struct {
	mu      sync.Mutex
	value   string
	restore int
	failOn  string
}

func (k *thingKind) Name() string  { return "thing" }
func (k *thingKind) Label() string { return "Thing" }
func (k *thingKind) Doc() string   { return "thing" }
func (k *thingKind) Plan(_ context.Context, _ string, _ aiplug.Actor, r aiplug.Record) aiplug.Plan {
	return aiplug.Plan{Summary: "sets the thing to " + r.Str("value")}
}
func (k *thingKind) Apply(_ context.Context, _ string, _ aiplug.Actor, r aiplug.Record) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.value = r.Str("value")
	return nil
}
func (k *thingKind) Export(context.Context, string, aiplug.Actor) (string, error) { return "", nil }
func (k *thingKind) Before(context.Context, string, aiplug.Actor, aiplug.Record) (*aiplug.Snapshot, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	return &aiplug.Snapshot{Target: "t1", Name: "The thing", Data: k.value}, nil
}
func (k *thingKind) Fingerprint(context.Context, string, aiplug.Actor, aiplug.Snapshot) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	return aiplug.Hash(k.value), nil
}
func (k *thingKind) Restore(_ context.Context, _ string, _ aiplug.Actor, s aiplug.Snapshot) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.failOn != "" && s.Data == k.failOn {
		return errors.New("db down")
	}
	k.value = s.Data
	k.restore++
	return nil
}

// fakePages is PageUndo over a few pages and a Trash.
type fakePages struct {
	live      map[string]*entities.Entity
	trash     map[string]bool
	restored  map[string]string
	trashed   []string
	untrashed []string
}

func (f *fakePages) Read(_ context.Context, campaignID, id string) (*entities.Entity, error) {
	if e := f.live[id]; e != nil && e.CampaignID == campaignID {
		return e, nil
	}
	return nil, nil
}
func (f *fakePages) RestoreText(_ context.Context, id, name string, _, html *string) error {
	f.restored[id] = name + "|" + *html
	e := f.live[id]
	e.Name, e.EntryHTML = name, html
	return nil
}
func (f *fakePages) Trash(_ context.Context, id string) error {
	f.trashed = append(f.trashed, id)
	delete(f.live, id)
	f.trash[id] = true
	return nil
}
func (f *fakePages) RestoreFromTrash(_ context.Context, _, id string) error {
	if !f.trash[id] {
		return errors.New("not in Trash")
	}
	f.untrashed = append(f.untrashed, id)
	return nil
}

func sp(s string) *string { return &s }

// undoRig is an applied request (id 1, the owner's) with an undoable record
// kept, and the pages Undo may touch.
type undoRig struct {
	h     *Handler
	repo  *reqRepo
	store *memUndo
	thing *thingKind
	pages *fakePages
	id    int64
}

func newUndoRig(t *testing.T) *undoRig {
	t.Helper()
	m := &memTables{}
	h := recordsHandler(m)
	thing := &thingKind{value: "old"}
	h.SetRecords(records.NewRegistry(records.TableKind{Svc: m}, thing))
	repo := &reqRepo{}
	svc := connector.NewService(repo, anyOwner{})
	store := &memUndo{repo: repo}
	svc.SetUndo(store)
	h.SetConnector(svc, "https://c.example")
	pages := &fakePages{
		live: map[string]*entities.Entity{
			"p1": {ID: "p1", CampaignID: "camp-1", Name: "Lyra", EntryHTML: sp("<p>AI text</p>")},
			"p2": {ID: "p2", CampaignID: "camp-1", Name: "Sable Quay", EntryHTML: sp("<p>New</p>")},
		},
		trash: map[string]bool{"p3": true}, restored: map[string]string{},
	}
	h.SetPageUndo(pages)
	conn := &connector.Connection{ID: 1, CampaignID: "camp-1", CanAsk: true, ClientName: "Claude"}
	q, err := svc.Ask(context.Background(), conn, "Changes", "---\nkind: thing\nname: x\nvalue: new\n---\n")
	if err != nil {
		t.Fatal(err)
	}
	return &undoRig{h: h, repo: repo, store: store, thing: thing, pages: pages, id: q.ID}
}

// applyThing applies the rig's request, so its record keeps a before.
func (r *undoRig) applyThing(t *testing.T) {
	t.Helper()
	rec := post(t, r.h.CommitImport, "/x", url.Values{
		"markdown_source": {"---\nkind: thing\nname: x\nvalue: new\n---\n"}, "rec_0_include": {"on"}, "ai_request": {"1"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("apply: %d %s", rec.Code, rec.Body)
	}
}

func (r *undoRig) keepPages(t *testing.T) {
	t.Helper()
	gm := `{"name":"Lyra","entry":"{}","entryHtml":"<p>Old <span data-secret=\"true\">GM secret</span></p>"}`
	err := r.h.connector.KeepBefores(context.Background(), "camp-1", "user-1", r.id, []connector.Before{
		{Kind: kindPage, TargetID: "p1", TargetName: "Lyra", Action: importer.ActionUpdate, BeforeJSON: &gm,
			AfterHash: importer.PageFingerprint(r.pages.live["p1"])},
		{Kind: kindPage, TargetID: "p2", TargetName: "Sable Quay", Action: importer.ActionCreate,
			AfterHash: importer.PageFingerprint(r.pages.live["p2"])},
		{Kind: kindPage, TargetID: "p3", TargetName: "Old Mill", Action: importer.ActionDelete},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (r *undoRig) answer(t *testing.T) {
	t.Helper()
	if err := r.repo.setStatus(r.id, connector.RequestAnswered); err != nil {
		t.Fatal(err)
	}
}

func (r *reqRepo) setStatus(id int64, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, q := range r.reqs {
		if q.ID == id {
			q.Status = status
			return nil
		}
	}
	return errors.New("no such request")
}

func undo(t *testing.T, r *undoRig) (int, string) {
	t.Helper()
	rec := post(t, r.h.UndoRequest, "/campaigns/camp-1/ai/requests/1/undo", url.Values{}, "rid", "1")
	return rec.Code, rec.Body.String()
}

// Applying a connected AI's request keeps what a record was; Undo puts it back
// and the row then says when.
func TestUndo_RecordIsPutBack(t *testing.T) {
	r := newUndoRig(t)
	r.applyThing(t)
	if r.thing.value != "new" {
		t.Fatalf("apply left %q", r.thing.value)
	}
	if len(r.store.befores) != 1 || r.store.befores[0].Kind != "thing" || r.store.befores[0].AfterHash == "" {
		t.Fatalf("befores = %+v", r.store.befores)
	}
	code, body := undo(t, r)
	if code != http.StatusOK || !strings.Contains(body, "Undone today") {
		t.Fatalf("undo: %d %s", code, body)
	}
	if r.thing.value != "old" {
		t.Errorf("the thing is %q, want it put back to old", r.thing.value)
	}
	// Pressed again: refused, nothing runs twice.
	if code, _ := undo(t, r); code == http.StatusOK || r.thing.restore != 1 {
		t.Errorf("second press: %d, restores = %d", code, r.thing.restore)
	}
}

// A paste (no connected AI) keeps nothing, and a request applied before
// anything was kept has no Undo.
func TestUndo_NothingKeptNoUndo(t *testing.T) {
	r := newUndoRig(t)
	rec := post(t, r.h.CommitImport, "/x", url.Values{
		"markdown_source": {"---\nkind: thing\nname: x\nvalue: new\n---\n"}, "rec_0_include": {"on"}})
	if rec.Code != http.StatusOK || len(r.store.befores) != 0 {
		t.Fatalf("paste: %d, befores %d", rec.Code, len(r.store.befores))
	}
	r.answer(t)
	if code, body := undo(t, r); code == http.StatusOK || !strings.Contains(body, "nothing left") {
		t.Errorf("old request: %d %s", code, body)
	}
}

// Undo refuses, and changes nothing, when the live state is no longer what the
// request left; the owner can press again once they have dealt with it.
func TestUndo_RefusesWhenChangedSince(t *testing.T) {
	r := newUndoRig(t)
	r.applyThing(t)
	r.thing.value = "someone edited this"
	code, body := undo(t, r)
	if code != http.StatusOK || !strings.Contains(body, "Nothing was put back") || !strings.Contains(body, "“The thing” has been changed") {
		t.Fatalf("refusal: %d %s", code, body)
	}
	if r.thing.value != "someone edited this" || r.thing.restore != 0 {
		t.Errorf("later work was overwritten: %q", r.thing.value)
	}
	if strings.Contains(body, "Undone") {
		t.Error("a refused undo said it was undone")
	}
	// Still offered, since the claim was given back.
	if !strings.Contains(body, "Undo it") {
		t.Errorf("the refused row lost its Undo: %s", body)
	}
}

// Pages: an update goes back to its kept title and text, a created page goes to
// the Trash, a removed one comes back; and the answer never carries the kept
// text, which held a GM-only passage.
func TestUndo_Pages(t *testing.T) {
	r := newUndoRig(t)
	r.keepPages(t)
	r.answer(t)
	code, body := undo(t, r)
	if code != http.StatusOK || !strings.Contains(body, "Undone today") {
		t.Fatalf("undo: %d %s", code, body)
	}
	if got := r.pages.restored["p1"]; !strings.HasPrefix(got, "Lyra|<p>Old ") {
		t.Errorf("update restored %q", got)
	}
	if len(r.pages.trashed) != 1 || r.pages.trashed[0] != "p2" {
		t.Errorf("created page trashed = %v", r.pages.trashed)
	}
	if len(r.pages.untrashed) != 1 || r.pages.untrashed[0] != "p3" {
		t.Errorf("removed page brought back = %v", r.pages.untrashed)
	}
	if strings.Contains(body, "GM secret") {
		t.Error("the response carries the kept GM-only text")
	}
}

func TestUndo_PageEditedSinceIsRefused(t *testing.T) {
	r := newUndoRig(t)
	r.keepPages(t)
	r.answer(t)
	r.pages.live["p1"].EntryHTML = sp("<p>A scribe fixed a typo</p>")
	code, body := undo(t, r)
	if code != http.StatusOK || !strings.Contains(body, "“Lyra” has been changed since the AI applied it") || !strings.Contains(body, "History") {
		t.Fatalf("refusal: %d %s", code, body)
	}
	if len(r.pages.restored) != 0 || len(r.pages.trashed) != 0 || len(r.pages.untrashed) != 0 {
		t.Errorf("a refused undo still changed pages: %+v", r.pages)
	}
	if strings.Contains(body, "GM secret") {
		t.Error("the refusal carries the kept text")
	}
}

// A created page that is already gone has nothing left to undo, and the rest
// still goes back.
func TestUndo_CreatedPageAlreadyGone(t *testing.T) {
	r := newUndoRig(t)
	r.keepPages(t)
	r.answer(t)
	delete(r.pages.live, "p2")
	if code, body := undo(t, r); code != http.StatusOK || !strings.Contains(body, "Undone today") {
		t.Fatalf("undo: %d %s", code, body)
	}
	if len(r.pages.trashed) != 0 {
		t.Errorf("trashed %v", r.pages.trashed)
	}
}

// A step that fails part way gives the claim back, says how far it got, and a
// second press does the rest.
func TestUndo_PartFailureCanBeRetried(t *testing.T) {
	r := newUndoRig(t)
	r.applyThing(t)
	r.keepPages(t)
	r.thing.failOn = "old"
	code, body := undo(t, r)
	if code != http.StatusOK || !strings.Contains(body, "put back “The thing”") || strings.Contains(body, "Undone") {
		t.Fatalf("partial: %d %s", code, body)
	}
	r.thing.failOn = ""
	if code, body := undo(t, r); code != http.StatusOK || !strings.Contains(body, "Undone today") {
		t.Fatalf("retry: %d %s", code, body)
	}
	if r.thing.value != "old" {
		t.Errorf("thing = %q", r.thing.value)
	}
}

// Undo only reaches the signed-in owner's own requests.
func TestUndo_OnlyTheOwnersRequest(t *testing.T) {
	r := newUndoRig(t)
	r.applyThing(t)
	r.repo.conn = &connector.Connection{ID: 9, CampaignID: "camp-1", UserID: "someone-else"}
	if code, _ := undo(t, r); code == http.StatusOK {
		t.Error("undid a request that is not the signed-in owner's")
	}
	if r.thing.value != "new" {
		t.Errorf("the thing changed to %q", r.thing.value)
	}
}

func TestUndoConfirm(t *testing.T) {
	b := func(kind, action, name string) connector.Before {
		return connector.Before{Kind: kind, Action: action, TargetName: name}
	}
	tests := []struct {
		name   string
		left   []connector.Before
		blocks int
		want   string
	}{
		{"pages put back and created", []connector.Before{
			b("page", "update", "A"), b("page", "update", "B"), b("page", "create", "C")}, 3,
			"Puts back 2 pages as they were before. 1 page the AI created goes to the Trash."},
		{"one page", []connector.Before{b("page", "update", "A")}, 1, "Puts back 1 page as it was before."},
		{"removed and a home", []connector.Before{b("page", "delete", "A"), b("home", "update", "players home")}, 2,
			"Brings back 1 page from the Trash. Puts back players home as it was before."},
		{"part of the request", []connector.Before{b("page", "update", "A")}, 4,
			"Puts back 1 page as it was before. The request's other changes stay as they are."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := undoConfirm(tt.left, tt.blocks); got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

// keptBefores keeps a page update's text and a record's snapshot, and leaves
// everything else out.
func TestKeptBefores(t *testing.T) {
	res := importer.CommitResult{Rows: []importer.RowOutcome{
		{Before: &importer.PageBefore{Action: importer.ActionUpdate, EntityID: "p1", Name: "Lyra", EntryHTML: sp("<p>x</p>"), AfterHash: "h"}},
		{Before: &importer.PageBefore{Action: importer.ActionCreate, EntityID: "p2", Name: "Sable", AfterHash: "h2"}},
		{Kept: &importer.KeptRecord{Kind: "home", Action: "update", Snapshot: aiplug.Snapshot{Target: "player", Name: "players home", Data: "{}"}, AfterHash: "h3"}},
		{},
	}}
	got := keptBefores(res)
	if len(got) != 3 {
		t.Fatalf("kept %d, want 3", len(got))
	}
	var kept pageBeforeJSON
	if got[0].BeforeJSON == nil || json.Unmarshal([]byte(*got[0].BeforeJSON), &kept) != nil || *kept.EntryHTML != "<p>x</p>" || kept.Name != "Lyra" {
		t.Errorf("update kept = %+v", got[0])
	}
	if got[1].BeforeJSON != nil || *got[2].BeforeJSON != "{}" {
		t.Errorf("kept = %+v", got)
	}
}

// The row renders its states: the Undo word and its confirm, Undone with
// when, and the refusal inline.
func TestUndoCellStates(t *testing.T) {
	now := time.Date(2026, 11, 21, 14, 5, 0, 0, time.UTC)
	u := undoCell{CampaignID: "camp-1", RequestID: 7, Confirm: "Puts back 2 pages as they were before.", Now: now, Loc: time.UTC}
	html := renderTempl(t, aiUndoCell(u))
	for _, want := range []string{`hx-post="/campaigns/camp-1/ai/requests/7/undo"`, "Puts back 2 pages as they were before.", "Undo</button>", "Keep it"} {
		if !strings.Contains(html, want) {
			t.Errorf("ready cell missing %q:\n%s", want, html)
		}
	}
	at := now.Add(-time.Hour)
	u.UndoneAt = &at
	html = renderTempl(t, aiUndoCell(u))
	if !strings.Contains(html, "Undone today 1:05 pm") || strings.Contains(html, "hx-post") {
		t.Errorf("done cell:\n%s", html)
	}
	u.UndoneAt, u.Refusal = nil, "Nothing was put back. “Lyra” has been changed since."
	html = renderTempl(t, aiUndoCell(u))
	if !strings.Contains(html, `role="alert"`) || !strings.Contains(html, "“Lyra” has been changed since.") || !strings.Contains(html, "hx-post") {
		t.Errorf("refused cell:\n%s", html)
	}
}

func renderTempl(t *testing.T, c interface {
	Render(context.Context, io.Writer) error
}) string {
	t.Helper()
	var buf bytes.Buffer
	if err := c.Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}
