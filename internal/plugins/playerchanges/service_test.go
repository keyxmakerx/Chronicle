package playerchanges

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// fakeRepo keeps live rows and packs in memory.
type fakeRepo struct {
	rows   []Change
	packs  []Pack
	nextID int64
}

func (f *fakeRepo) Insert(_ context.Context, c Change) error {
	f.nextID++
	c.ID = f.nextID
	f.rows = append(f.rows, c)
	return nil
}

func (f *fakeRepo) LastFor(_ context.Context, c Change, since time.Time) (*Change, error) {
	for i := len(f.rows) - 1; i >= 0; i-- {
		r := f.rows[i]
		if r.CampaignID == c.CampaignID && r.EntityID == c.EntityID && r.UserID == c.UserID && r.Kind == c.Kind &&
			r.FieldKey == c.FieldKey && r.Source == c.Source && r.UndoneAt == nil && !r.CreatedAt.Before(since) {
			return &r, nil
		}
	}
	return nil, nil
}

func (f *fakeRepo) UpdateNew(_ context.Context, c Change) error {
	for i := range f.rows {
		if f.rows[i].ID == c.ID {
			f.rows[i] = c
		}
	}
	return nil
}

func (f *fakeRepo) Delete(_ context.Context, _ string, id int64) error {
	for i := range f.rows {
		if f.rows[i].ID == id {
			f.rows = append(f.rows[:i], f.rows[i+1:]...)
			return nil
		}
	}
	return nil
}

func (f *fakeRepo) Live(_ context.Context, campaignID string, q Query) ([]Change, error) {
	var out []Change
	for _, r := range f.rows {
		if r.CampaignID == campaignID && !r.CreatedAt.Before(q.Since) {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (f *fakeRepo) Get(_ context.Context, campaignID string, id int64) (*Change, error) {
	for _, r := range f.rows {
		if r.CampaignID == campaignID && r.ID == id {
			return &r, nil
		}
	}
	return nil, nil
}

func (f *fakeRepo) MarkUndone(_ context.Context, campaignID string, id int64, by string, at time.Time) (bool, error) {
	for i := range f.rows {
		if f.rows[i].CampaignID == campaignID && f.rows[i].ID == id && f.rows[i].UndoneAt == nil {
			f.rows[i].UndoneAt, f.rows[i].UndoneBy = &at, by
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeRepo) MarkPageRemovalsUndone(_ context.Context, campaignID, entityID, by string, at time.Time) error {
	for i := range f.rows {
		r := &f.rows[i]
		if r.CampaignID == campaignID && r.EntityID == entityID && r.Kind == KindRemoved && r.UndoneAt == nil {
			r.UndoneAt, r.UndoneBy = &at, by
		}
	}
	return nil
}

func (f *fakeRepo) Recent(context.Context, string, time.Time) ([]Suggestion, []Suggestion, error) {
	return []Suggestion{{Kind: "page", Value: "e1", Label: "Tyne"}}, nil, nil
}
func (f *fakeRepo) DueMonths(context.Context, time.Time) ([]PackKey, error) { return nil, nil }
func (f *fakeRepo) PackMonth(context.Context, PackKey, time.Time, time.Time) (int, error) {
	return 0, nil
}
func (f *fakeRepo) Packs(_ context.Context, campaignID string, since time.Time) ([]Pack, error) {
	var out []Pack
	for _, p := range f.packs {
		if p.CampaignID == campaignID && !p.Month.Before(since) {
			rows := append([]Change(nil), p.Rows...)
			for i := range rows {
				rows[i].Packed = true
			}
			p.Rows = rows
			out = append(out, p)
		}
	}
	return out, nil
}
func (f *fakeRepo) RewritePack(_ context.Context, k PackKey, rows []Change) error {
	for i := range f.packs {
		if f.packs[i].PackKey == k {
			f.packs[i].Rows = rows
		}
	}
	return nil
}
func (f *fakeRepo) PrunePacks(context.Context, time.Time) (int, error) { return 0, nil }
func (f *fakeRepo) KeptPages(context.Context, time.Time) ([]string, error) {
	var out []string
	for _, r := range f.rows {
		if r.Kind == KindRemoved && r.UndoneAt == nil {
			out = append(out, r.EntityID)
		}
	}
	return out, nil
}
func (f *fakeRepo) MonthlyBytes(context.Context) (int64, bool, error) { return 0, false, nil }
func (f *fakeRepo) ForgetPage(context.Context, string, string) error  { return nil }

// fakePages is one page with sheet fields, and a Trash.
type fakePages struct {
	fields   map[string]any
	gone     bool
	trashed  bool
	restored bool
	setBy    string
}

func (p *fakePages) FieldValue(_ context.Context, _, _ string, key string) (any, bool, error) {
	if p.gone {
		return nil, false, nil
	}
	return p.fields[key], true, nil
}
func (p *fakePages) SetField(_ context.Context, _, _ string, key string, value any, ownerID string) error {
	p.fields[key], p.setBy = value, ownerID
	return nil
}
func (p *fakePages) RestorePage(context.Context, string, string) error {
	if !p.trashed {
		return apperror.NewNotFound("not in the trash")
	}
	p.restored = true
	return nil
}
func (p *fakePages) TextDiff(older, newer string) (string, string) {
	return "-" + older, "+" + newer
}

func newTestService(repo *fakeRepo, pages Pages, now time.Time) *service {
	s := NewService(repo, pages, nil).(*service)
	s.now = func() time.Time { return now }
	return s
}

var t0 = time.Date(2026, 10, 20, 21, 0, 0, 0, time.UTC)

func fieldChange(at time.Time, oldV, newV string) Change {
	return Change{CampaignID: "c1", EntityID: "e1", PageName: "Tyne", UserID: "u1", UserName: "Bea",
		Kind: KindField, FieldKey: "hp", FieldLabel: "Stamina", OldText: oldV, NewText: newV,
		OldJSON: ValueJSON(oldV), NewJSON: ValueJSON(newV), Source: SourceWeb, CreatedAt: at}
}

func TestRecord_Folds(t *testing.T) {
	tests := []struct {
		name     string
		changes  []Change
		wantRows int
		wantOld  string
		wantNew  string
	}{
		{"one save", []Change{fieldChange(t0, "10", "8")}, 1, "10", "8"},
		{"quick saves fold into one", []Change{fieldChange(t0, "10", "8"), fieldChange(t0.Add(time.Minute), "8", "5")}, 1, "10", "5"},
		{"far apart saves stay apart", []Change{fieldChange(t0, "10", "8"), fieldChange(t0.Add(time.Hour), "8", "5")}, 2, "8", "5"},
		{"back where it started removes the line", []Change{fieldChange(t0, "10", "8"), fieldChange(t0.Add(time.Minute), "8", "10")}, 0, "", ""},
		{"another field never folds", []Change{fieldChange(t0, "10", "8"), func() Change {
			c := fieldChange(t0.Add(time.Minute), "a", "b")
			c.FieldKey = "class"
			return c
		}()}, 2, "a", "b"},
		{"the AI's change never folds into the player's", []Change{fieldChange(t0, "10", "8"), func() Change {
			c := fieldChange(t0.Add(time.Minute), "8", "5")
			c.Source = SourceAI
			return c
		}()}, 2, "8", "5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepo{}
			s := newTestService(repo, &fakePages{}, t0)
			for _, c := range tt.changes {
				if err := s.Record(context.Background(), c); err != nil {
					t.Fatal(err)
				}
			}
			if len(repo.rows) != tt.wantRows {
				t.Fatalf("rows = %d, want %d", len(repo.rows), tt.wantRows)
			}
			if tt.wantRows == 0 {
				return
			}
			last := repo.rows[len(repo.rows)-1]
			if last.OldText != tt.wantOld || last.NewText != tt.wantNew {
				t.Errorf("last = %q → %q, want %q → %q", last.OldText, last.NewText, tt.wantOld, tt.wantNew)
			}
		})
	}
}

func TestRecord_TextFoldRecomputesWords(t *testing.T) {
	repo := &fakeRepo{}
	s := newTestService(repo, &fakePages{}, t0)
	text := func(at time.Time, was, now string) Change {
		return Change{CampaignID: "c1", EntityID: "e1", UserID: "u1", Kind: KindText, OldJSON: was, NewJSON: now,
			OldText: "x", NewText: "y", Source: SourceWeb, CreatedAt: at}
	}
	_ = s.Record(context.Background(), text(t0, "A", "AB"))
	_ = s.Record(context.Background(), text(t0.Add(time.Minute), "AB", "ABC"))
	if len(repo.rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(repo.rows))
	}
	if r := repo.rows[0]; r.OldText != "-A" || r.NewText != "+ABC" || r.OldJSON != "A" {
		t.Errorf("folded = %+v; want the words worked out from A to ABC", r)
	}
}

func TestRecord_SkipsUnattributed(t *testing.T) {
	repo := &fakeRepo{}
	s := newTestService(repo, &fakePages{}, t0)
	c := fieldChange(t0, "1", "2")
	c.UserID = ""
	if err := s.Record(context.Background(), c); err != nil || len(repo.rows) != 0 {
		t.Fatalf("rows = %d, err = %v; a change nobody made is not recorded", len(repo.rows), err)
	}
}

func TestRecord_DropsOversizedValues(t *testing.T) {
	repo := &fakeRepo{}
	s := newTestService(repo, &fakePages{}, t0)
	c := fieldChange(t0, "1", "2")
	c.NewJSON = `"` + strings.Repeat("x", maxJSONBytes+1) + `"`
	_ = s.Record(context.Background(), c)
	if repo.rows[0].OldJSON != "" || repo.rows[0].NewJSON != "" {
		t.Error("an oversized value must be dropped, leaving the change readable but not undoable here")
	}
}

func code(err error) int {
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return 0
}

func TestUndo(t *testing.T) {
	tests := []struct {
		name     string
		change   Change
		pages    *fakePages
		wantCode int
		wantHP   any
	}{
		{"puts the old value back", fieldChange(t0, "10", "8"), &fakePages{fields: map[string]any{"hp": "8"}}, 0, "10"},
		{"refuses when it changed since", fieldChange(t0, "10", "8"), &fakePages{fields: map[string]any{"hp": "3"}}, http.StatusConflict, "3"},
		{"refuses when the page is gone", fieldChange(t0, "10", "8"), &fakePages{gone: true, fields: map[string]any{}}, http.StatusNotFound, nil},
		{"text goes through the page's history", Change{CampaignID: "c1", EntityID: "e1", UserID: "u1", Kind: KindText, CreatedAt: t0},
			&fakePages{fields: map[string]any{}}, http.StatusBadRequest, nil},
		{"too large to undo here", func() Change { c := fieldChange(t0, "1", "2"); c.OldJSON = ""; return c }(),
			&fakePages{fields: map[string]any{"hp": "2"}}, http.StatusBadRequest, "2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepo{}
			s := newTestService(repo, tt.pages, t0)
			_ = repo.Insert(context.Background(), tt.change)
			got, err := s.Undo(context.Background(), "c1", "owner", 1)
			if code(err) != tt.wantCode {
				t.Fatalf("err = %v, want code %d", err, tt.wantCode)
			}
			if tt.pages.fields["hp"] != tt.wantHP {
				t.Errorf("hp = %v, want %v", tt.pages.fields["hp"], tt.wantHP)
			}
			if tt.wantCode == 0 {
				if got.UndoneAt == nil || repo.rows[0].UndoneAt == nil || tt.pages.setBy != "owner" {
					t.Error("an undone change is marked undone, written as the owner")
				}
				if _, err := s.Undo(context.Background(), "c1", "owner", 1); code(err) != http.StatusConflict {
					t.Errorf("second undo err = %v, want a conflict", err)
				}
			}
		})
	}
}

func TestUndo_OtherCampaign(t *testing.T) {
	repo := &fakeRepo{}
	s := newTestService(repo, &fakePages{fields: map[string]any{"hp": "8"}}, t0)
	_ = repo.Insert(context.Background(), fieldChange(t0, "10", "8"))
	if _, err := s.Undo(context.Background(), "c2", "owner", 1); code(err) != http.StatusNotFound {
		t.Fatalf("err = %v; another campaign's change must not be found", err)
	}
}

func TestUndo_BringBack(t *testing.T) {
	removed := Change{CampaignID: "c1", EntityID: "e1", UserID: "u1", Kind: KindRemoved, CreatedAt: t0}
	for _, tt := range []struct {
		name     string
		trashed  bool
		wantCode int
	}{{"from the Trash", true, 0}, {"no longer in the Trash", false, http.StatusNotFound}} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepo{}
			pages := &fakePages{trashed: tt.trashed}
			s := newTestService(repo, pages, t0)
			_ = repo.Insert(context.Background(), removed)
			_, err := s.Undo(context.Background(), "c1", "owner", 1)
			if code(err) != tt.wantCode || pages.restored != tt.trashed {
				t.Fatalf("err = %v restored = %v", err, pages.restored)
			}
		})
	}
}

func TestUndo_Packed(t *testing.T) {
	month := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	c := fieldChange(month.AddDate(0, 0, 3), "10", "8")
	c.ID = 7
	repo := &fakeRepo{packs: []Pack{{PackKey: PackKey{CampaignID: "c1", Month: month}, Rows: []Change{c}}}}
	pages := &fakePages{fields: map[string]any{"hp": "8"}}
	s := newTestService(repo, pages, t0)
	got, err := s.Undo(context.Background(), "c1", "owner", 7)
	if err != nil {
		t.Fatal(err)
	}
	if pages.fields["hp"] != "10" || got.UndoneAt == nil || repo.packs[0].Rows[0].UndoneAt == nil {
		t.Error("a packed change is undone and its pack rewritten with the mark")
	}
}

func TestSearch(t *testing.T) {
	repo := &fakeRepo{}
	s := newTestService(repo, &fakePages{}, t0)
	ai := fieldChange(t0.Add(-time.Hour), "Gnome", "Elf")
	ai.FieldKey, ai.FieldLabel, ai.Source, ai.App = "ancestry", "Ancestry", SourceAI, "Claude"
	_ = repo.Insert(context.Background(), fieldChange(t0.Add(-2*time.Hour), "10", "8"))
	_ = repo.Insert(context.Background(), ai)
	_ = repo.Insert(context.Background(), Change{CampaignID: "c1", EntityID: "e2", PageName: "Old map", UserID: "u1", Kind: KindRemoved, CreatedAt: t0.Add(-3 * time.Hour)})
	month := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	old := fieldChange(month.AddDate(0, 0, 2), "12", "10")
	old.ID = 99
	repo.packs = []Pack{{PackKey: PackKey{CampaignID: "c1", Month: month}, Rows: []Change{old}}}

	since := t0.AddDate(0, 0, -LiveDays)
	tests := []struct {
		name       string
		q          Query
		withPacked bool
		want       int
	}{
		{"everything this month", Query{Since: since}, false, 3},
		{"by an AI", Query{Since: since, AI: true}, false, 1},
		{"removed things", Query{Since: since, Removed: true}, false, 1},
		{"a field", Query{Since: since, Fields: []string{"hp"}}, false, 1},
		{"words must all match", Query{Since: since, Words: []string{"elf", "claude"}}, false, 1},
		{"a word nothing holds", Query{Since: since, Words: []string{"dragon"}}, false, 0},
		{"six months unpacks older ones", Query{Since: t0.AddDate(0, -6, 0), Fields: []string{"hp"}}, true, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := s.Search(context.Background(), "c1", tt.q, tt.withPacked)
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Changes) != tt.want {
				t.Fatalf("got %d changes, want %d", len(res.Changes), tt.want)
			}
			for i := 1; i < len(res.Changes); i++ {
				if res.Changes[i].CreatedAt.After(res.Changes[i-1].CreatedAt) {
					t.Error("results must be newest first")
				}
			}
		})
	}
}

func TestSuggestions_EndWithKinds(t *testing.T) {
	s := newTestService(&fakeRepo{}, &fakePages{}, t0)
	got, _ := s.Suggestions(context.Background(), "c1")
	if len(got) != 3 || got[1].Label != "by an AI" || got[2].Label != "removed things" {
		t.Fatalf("suggestions = %+v", got)
	}
}

// A page the owner brings back through the ordinary Trash, not Undo, must stop
// being kept, or deleting it again would leave it out of the purge for good.
func TestPageRestored_StopsKeepingThePage(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{}
	s := newTestService(repo, &fakePages{}, t0)
	_ = repo.Insert(ctx, Change{CampaignID: "c1", EntityID: "e1", UserID: "u1", Kind: KindRemoved, CreatedAt: t0})
	_ = repo.Insert(ctx, Change{CampaignID: "c1", EntityID: "e2", UserID: "u1", Kind: KindRemoved, CreatedAt: t0})

	if kept, _ := s.KeptPages(ctx); len(kept) != 2 {
		t.Fatalf("kept before the restore = %v, want both pages", kept)
	}
	s.PageRestored(ctx, "c1", "e1")
	kept, _ := s.KeptPages(ctx)
	if len(kept) != 1 || kept[0] != "e2" {
		t.Errorf("kept after restoring e1 = %v, want only e2", kept)
	}
}

// Undo restores through the same path, so the restore's own PageRestored must
// credit the owner who pressed Undo and leave Undo nothing left to mark.
func TestUndo_BringBackCreditsOwnerOnce(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{}
	pages := &restoringPages{fakePages: &fakePages{trashed: true}}
	s := newTestService(repo, pages, t0)
	pages.onRestore = func(ctx context.Context) { s.PageRestored(ctx, "c1", "e1") }
	_ = repo.Insert(ctx, Change{CampaignID: "c1", EntityID: "e1", UserID: "u1", Kind: KindRemoved, CreatedAt: t0})

	c, err := s.Undo(ctx, "c1", "owner", 1)
	if err != nil {
		t.Fatal(err)
	}
	if c.UndoneBy != "owner" || repo.rows[0].UndoneBy != "owner" || repo.rows[0].UndoneAt == nil {
		t.Errorf("undone by %q (stored %q), want the owner", c.UndoneBy, repo.rows[0].UndoneBy)
	}
}

// restoringPages runs a hook inside the page restore, as the page safety
// service's restore calls the keeper.
type restoringPages struct {
	*fakePages
	onRestore func(ctx context.Context)
}

func (p *restoringPages) RestorePage(ctx context.Context, c, e string) error {
	if err := p.fakePages.RestorePage(ctx, c, e); err != nil {
		return err
	}
	p.onRestore(ctx)
	return nil
}
