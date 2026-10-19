package trackers

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

const (
	cmp      = "camp-1"
	pageA    = "page-a"
	pageHid  = "page-hidden"
	pageGone = "page-missing"
	typeHero = 7
)

var (
	dm     = Viewer{UserID: "dm-1", MemberRole: 3, VisibilityRole: 3, Director: true}
	player = Viewer{UserID: "jo-1", MemberRole: 1, VisibilityRole: 1}
)

// fakeRepo is an in-memory Repository. Its semantics mirror the SQL one:
// unique names per page and per campaign, optimistic versions, and kinds
// that refuse deletion while a tracker uses them.
type fakeRepo struct {
	kinds    map[string]Kind
	trackers map[string]Tracker
	entries  []Entry
	seq      int64
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{kinds: map[string]Kind{}, trackers: map[string]Tracker{}}
}

func (f *fakeRepo) inUse(kindID string) int {
	n := 0
	for _, t := range f.trackers {
		if t.KindID == kindID {
			n++
		}
	}
	return n
}

func (f *fakeRepo) ListKinds(_ context.Context, c string) ([]Kind, error) {
	var out []Kind
	for _, k := range f.kinds {
		if k.CampaignID == c {
			k.InUse = f.inUse(k.ID)
			out = append(out, k)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (f *fakeRepo) GetKind(_ context.Context, c, id string) (Kind, bool, error) {
	k, ok := f.kinds[id]
	if !ok || k.CampaignID != c {
		return Kind{}, false, nil
	}
	k.InUse = f.inUse(id)
	return k, true, nil
}

func (f *fakeRepo) CountKinds(_ context.Context, c string) (int, error) {
	n := 0
	for _, k := range f.kinds {
		if k.CampaignID == c {
			n++
		}
	}
	return n, nil
}

func (f *fakeRepo) CreateKind(_ context.Context, k Kind) error {
	for _, x := range f.kinds {
		if x.CampaignID == k.CampaignID && x.Name == k.Name {
			return errDuplicate
		}
	}
	f.kinds[k.ID] = k
	return nil
}

func (f *fakeRepo) UpdateKind(_ context.Context, k Kind, expected int) (bool, error) {
	cur, ok := f.kinds[k.ID]
	if !ok || cur.Version != expected {
		return false, nil
	}
	k.Version = expected + 1
	f.kinds[k.ID] = k
	return true, nil
}

func (f *fakeRepo) DeleteKind(_ context.Context, c, id string) (bool, error) {
	if f.inUse(id) > 0 {
		return true, nil
	}
	delete(f.kinds, id)
	return false, nil
}

func (f *fakeRepo) ListTrackers(_ context.Context, c, entityID string) ([]Tracker, error) {
	var out []Tracker
	for _, t := range f.trackers {
		if t.CampaignID == c && t.EntityID == entityID {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (f *fakeRepo) GetTracker(_ context.Context, c, id string) (Tracker, bool, error) {
	t, ok := f.trackers[id]
	if !ok || t.CampaignID != c {
		return Tracker{}, false, nil
	}
	return t, true, nil
}

func (f *fakeRepo) ListAllTrackers(_ context.Context, c string) ([]Tracker, error) {
	out := []Tracker{}
	for _, t := range f.trackers {
		if t.CampaignID == c {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].EntityID != out[j].EntityID {
			return out[i].EntityID < out[j].EntityID
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (f *fakeRepo) TrackerForKind(_ context.Context, c, entityID, kindID string) (Tracker, bool, error) {
	for _, t := range f.trackers {
		if t.CampaignID == c && t.EntityID == entityID && t.KindID == kindID {
			return t, true, nil
		}
	}
	return Tracker{}, false, nil
}

func (f *fakeRepo) CreateTracker(_ context.Context, t Tracker) error {
	for _, x := range f.trackers {
		if x.EntityID == t.EntityID && x.Name == t.Name {
			return errDuplicate
		}
	}
	f.trackers[t.ID] = t
	return nil
}

func (f *fakeRepo) UpdateTracker(_ context.Context, t Tracker) error {
	for _, x := range f.trackers {
		if x.ID != t.ID && x.EntityID == t.EntityID && x.Name == t.Name {
			return errDuplicate
		}
	}
	f.trackers[t.ID] = t
	return nil
}

func (f *fakeRepo) DeleteTracker(_ context.Context, _, id string) error {
	delete(f.trackers, id)
	return nil
}

func (f *fakeRepo) ListEntries(_ context.Context, _ string, ids []string) (map[string][]Entry, error) {
	out := map[string][]Entry{}
	for _, id := range ids {
		for _, e := range f.entries {
			if e.TrackerID == id {
				out[id] = append(out[id], e)
			}
		}
	}
	return out, nil
}

func (f *fakeRepo) CountEntries(_ context.Context, _, id string) (int, error) {
	n := 0
	for _, e := range f.entries {
		if e.TrackerID == id {
			n++
		}
	}
	return n, nil
}

func (f *fakeRepo) AddEntry(_ context.Context, _ string, e Entry) error {
	f.seq++
	e.ID = f.seq
	e.CreatedAt = time.Date(2026, 10, 18, 12, 0, int(f.seq), 0, time.UTC)
	f.entries = append(f.entries, e)
	return nil
}

func (f *fakeRepo) GetEntry(_ context.Context, _ string, id int64) (Entry, bool, error) {
	for _, e := range f.entries {
		if e.ID == id {
			return e, true, nil
		}
	}
	return Entry{}, false, nil
}

func (f *fakeRepo) StrikeEntry(_ context.Context, _ string, id int64, struck bool, _ string) error {
	for i := range f.entries {
		if f.entries[i].ID == id {
			if struck {
				now := time.Now()
				f.entries[i].StruckAt = &now
			} else {
				f.entries[i].StruckAt = nil
			}
		}
	}
	return nil
}

// fakeDir answers page access from a table; pages absent from it do not exist.
type fakeDir struct {
	pages map[string]PageAccess
	types []TypeRef
}

func (d *fakeDir) Access(_ context.Context, _, entityID string, _ Viewer) (PageAccess, error) {
	return d.pages[entityID], nil
}

func (d *fakeDir) Types(context.Context, string) ([]TypeRef, error) { return d.types, nil }

// Names names every page the directory knows, as "Page <id>".
func (d *fakeDir) Names(_ context.Context, _ string, ids []string) (map[string]string, error) {
	out := map[string]string{}
	for _, id := range ids {
		if _, ok := d.pages[id]; ok {
			out[id] = "Page " + id
		}
	}
	return out, nil
}

type fakeClock struct {
	date *GameDate
	err  error
}

func (c *fakeClock) Today(context.Context, string, Viewer) (*GameDate, error) { return c.date, c.err }

type rig struct {
	svc   Service
	repo  *fakeRepo
	clock *fakeClock
}

// newRig builds a service over a campaign with a visible hero page, a page
// the viewer cannot see, and a hero category (typeHero) with one type listed.
func newRig(t *testing.T) *rig {
	t.Helper()
	repo := newFakeRepo()
	dir := &fakeDir{
		pages: map[string]PageAccess{
			pageA:   {Exists: true, CanView: true, TypeID: typeHero},
			pageHid: {Exists: true, CanView: false, TypeID: typeHero},
		},
		types: []TypeRef{{ID: typeHero, Name: "Hero"}, {ID: 2, Name: "Place"}},
	}
	clock := &fakeClock{date: &GameDate{Year: 1, Month: 2, Day: 3, Label: "3 Frostfall 1"}}
	svc := NewService(repo, dir, clock).(*service)
	n := 0
	svc.newID = func() string { n++; return "id-" + string(rune('a'+n)) }
	return &rig{svc: svc, repo: repo, clock: clock}
}

func (r *rig) addKind(t *testing.T, k Kind) Kind {
	t.Helper()
	k.CampaignID = cmp
	if k.Version == 0 {
		k.Version = 1
	}
	if k.Visibility == "" {
		k.Visibility = SeeEveryone
	}
	r.repo.kinds[k.ID] = k
	return k
}

func (r *rig) addTracker(t *testing.T, tr Tracker) Tracker {
	t.Helper()
	tr.CampaignID = cmp
	if tr.EntityID == "" {
		tr.EntityID = pageA
	}
	if tr.Visibility == "" {
		tr.Visibility = SeeEveryone
	}
	r.repo.trackers[tr.ID] = tr
	return tr
}

func code(err error) int {
	if isKind(err, http.StatusNotFound) {
		return http.StatusNotFound
	}
	for _, c := range []int{http.StatusForbidden, http.StatusConflict, http.StatusUnprocessableEntity, http.StatusInternalServerError} {
		if isKind(err, c) {
			return c
		}
	}
	return 0
}

func TestNonDirectorWritesRefused(t *testing.T) {
	r := newRig(t)
	k := r.addKind(t, Kind{ID: "k1", Name: "Gold", Shape: ShapeCount})
	tr := r.addTracker(t, Tracker{ID: "t1", KindID: k.ID, Name: "Gold"})
	r.repo.entries = append(r.repo.entries, Entry{ID: 1, TrackerID: tr.ID, Op: OpAdd, Amount: 1})
	ctx := context.Background()
	name := "x"

	tests := []struct {
		name string
		call func() error
	}{
		{"Place", func() error { _, err := r.svc.Place(ctx, cmp, pageA, player, PlaceInput{KindID: k.ID}); return err }},
		{"Log", func() error {
			_, err := r.svc.Log(ctx, cmp, pageA, player, LogInput{TrackerID: tr.ID, Op: OpAdd, Amount: 1})
			return err
		}},
		{"Patch", func() error { _, err := r.svc.Patch(ctx, cmp, tr.ID, player, TrackerPatch{Name: &name}); return err }},
		{"Remove", func() error { return r.svc.Remove(ctx, cmp, tr.ID, player) }},
		{"Strike", func() error { _, err := r.svc.Strike(ctx, cmp, 1, true, player); return err }},
		{"CreateKind", func() error {
			_, err := r.svc.CreateKind(ctx, cmp, player, KindInput{Name: "N", Shape: ShapeCount})
			return err
		}},
		{"UpdateKind", func() error {
			_, err := r.svc.UpdateKind(ctx, cmp, k.ID, player, KindInput{Name: "N", Shape: ShapeCount, Version: 1})
			return err
		}},
		{"DeleteKind", func() error { return r.svc.DeleteKind(ctx, cmp, k.ID, player) }},
		{"Kinds", func() error { _, _, err := r.svc.Kinds(ctx, cmp, player); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := code(tt.call()); got != http.StatusForbidden {
				t.Fatalf("code = %d want 403", got)
			}
		})
	}
	if len(r.repo.entries) != 1 || len(r.repo.trackers) != 1 || len(r.repo.kinds) != 1 {
		t.Fatal("a refused write changed the repository")
	}
}

func TestHiddenPageLooksMissing(t *testing.T) {
	r := newRig(t)
	k := r.addKind(t, Kind{ID: "k1", Name: "Gold", Shape: ShapeCount})
	ctx := context.Background()

	for _, tt := range []struct {
		name string
		call func(page string) error
	}{
		{"Page as player", func(p string) error { _, err := r.svc.Page(ctx, cmp, p, player); return err }},
		{"Page as director", func(p string) error { _, err := r.svc.Page(ctx, cmp, p, dm); return err }},
		{"Place", func(p string) error { _, err := r.svc.Place(ctx, cmp, p, dm, PlaceInput{KindID: k.ID}); return err }},
		{"Log", func(p string) error {
			_, err := r.svc.Log(ctx, cmp, p, dm, LogInput{KindID: k.ID, Op: OpAdd, Amount: 1})
			return err
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hidden, missing, bad := tt.call(pageHid), tt.call(pageGone), tt.call("bad id!")
			if code(hidden) != http.StatusNotFound {
				t.Fatalf("hidden page: %v", hidden)
			}
			// Same code and same message, so the answer confirms nothing.
			if hidden.Error() != missing.Error() || hidden.Error() != bad.Error() {
				t.Fatalf("hidden %q, missing %q, malformed %q differ", hidden, missing, bad)
			}
		})
	}
}

func TestPagePlayerView(t *testing.T) {
	r := newRig(t)
	pub := r.addKind(t, Kind{ID: "kpub", Name: "Heat", Shape: ShapeCount, Visibility: SeeEveryone})
	sec := r.addKind(t, Kind{ID: "ksec", Name: "Doom", Shape: ShapeClock, Max: ip(4), Visibility: SeeDirectors})
	r.addTracker(t, Tracker{ID: "tpub", KindID: pub.ID, Name: "Heat", Visibility: SeeEveryone})
	r.addTracker(t, Tracker{ID: "tsec", KindID: sec.ID, Name: "Doom", Visibility: SeeDirectors})
	r.repo.entries = []Entry{
		{ID: 1, TrackerID: "tpub", Op: OpAdd, Amount: 2, Note: "they were seen", Game: &GameDate{Label: "1 Thaw 1"}, CreatedAt: time.Now()},
		{ID: 2, TrackerID: "tsec", Op: OpAdd, Amount: 1, Note: "cult stirs", CreatedAt: time.Now()},
	}
	ctx := context.Background()

	p, err := r.svc.Page(ctx, cmp, pageA, player)
	if err != nil {
		t.Fatal(err)
	}
	if p.CanEdit {
		t.Error("player can edit")
	}
	if len(p.Kinds) != 0 {
		t.Errorf("player got kinds: %+v", p.Kinds)
	}
	if len(p.Trackers) != 1 || p.Trackers[0].ID != "tpub" {
		t.Fatalf("player trackers = %+v", p.Trackers)
	}
	tv := p.Trackers[0]
	if tv.Value != 2 || tv.LogCount != 1 || len(tv.Log) != 1 {
		t.Fatalf("view = %+v", tv)
	}
	if tv.Log[0].Note != "" {
		t.Errorf("player saw the note %q", tv.Log[0].Note)
	}
	if tv.Log[0].When != "1 Thaw 1" || tv.Log[0].After != 2 {
		t.Errorf("player should still see the date and balance: %+v", tv.Log[0])
	}

	d, err := r.svc.Page(ctx, cmp, pageA, dm)
	if err != nil {
		t.Fatal(err)
	}
	if !d.CanEdit || len(d.Trackers) != 2 || len(d.Kinds) != 2 {
		t.Fatalf("director view = %+v", d)
	}
	for _, x := range d.Trackers {
		if x.ID == "tpub" && x.Log[0].Note != "they were seen" {
			t.Errorf("director lost the note: %+v", x.Log[0])
		}
	}
}

func TestPageHidesTrackerOfDeletedKind(t *testing.T) {
	r := newRig(t)
	r.addTracker(t, Tracker{ID: "orphan", KindID: "nokind", Name: "Ghost"})
	p, err := r.svc.Page(context.Background(), cmp, pageA, dm)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Trackers) != 0 {
		t.Fatalf("got %+v", p.Trackers)
	}
}

func TestVirtualTrackerFromEveryTypeKind(t *testing.T) {
	r := newRig(t)
	r.addKind(t, Kind{ID: "khero", Name: "Hit points", Shape: ShapeCount, Min: ip(0), Max: ip(30),
		EveryTypeID: typeHero, Visibility: SeeEveryone, Bands: []Band{{At: 0, Text: "down"}, {At: 10, Text: "hurt"}}})
	r.addKind(t, Kind{ID: "kplace", Name: "Unrest", Shape: ShapeCount, EveryTypeID: 2})
	ctx := context.Background()

	p, err := r.svc.Page(ctx, cmp, pageA, player)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Trackers) != 1 {
		t.Fatalf("want only the hero kind, got %+v", p.Trackers)
	}
	v := p.Trackers[0]
	if v.ID != "" || v.KindID != "khero" || v.Name != "Hit points" || v.Value != 0 || v.Now != "down" {
		t.Fatalf("virtual tracker = %+v", v)
	}
	if len(r.repo.trackers) != 0 {
		t.Fatal("viewing created a row")
	}

	t.Run("a Directors-only kind is not shown to players", func(t *testing.T) {
		r2 := newRig(t)
		r2.addKind(t, Kind{ID: "k", Name: "Secret", Shape: ShapeCount, EveryTypeID: typeHero, Visibility: SeeDirectors})
		pl, _ := r2.svc.Page(ctx, cmp, pageA, player)
		dv, _ := r2.svc.Page(ctx, cmp, pageA, dm)
		if len(pl.Trackers) != 0 || len(dv.Trackers) != 1 {
			t.Fatalf("player %d director %d", len(pl.Trackers), len(dv.Trackers))
		}
	})

	t.Run("first log creates the row", func(t *testing.T) {
		got, err := r.svc.Log(ctx, cmp, pageA, dm, LogInput{KindID: "khero", Op: OpSet, Amount: 12, Note: "ambush"})
		if err != nil {
			t.Fatal(err)
		}
		if got.ID == "" || got.Value != 12 || got.Now != "hurt" {
			t.Fatalf("got %+v", got)
		}
		if len(r.repo.trackers) != 1 {
			t.Fatalf("trackers = %d", len(r.repo.trackers))
		}
		// The page now shows the real row, not a second virtual one.
		p, _ := r.svc.Page(ctx, cmp, pageA, dm)
		if len(p.Trackers) != 1 || p.Trackers[0].ID != got.ID {
			t.Fatalf("page = %+v", p.Trackers)
		}
		// A second log reuses the row.
		again, err := r.svc.Log(ctx, cmp, pageA, dm, LogInput{KindID: "khero", Op: OpAdd, Amount: -2})
		if err != nil || again.ID != got.ID || again.Value != 10 || len(r.repo.trackers) != 1 {
			t.Fatalf("again = %+v err %v rows %d", again, err, len(r.repo.trackers))
		}
	})

	t.Run("log by kind is refused when the page type does not match", func(t *testing.T) {
		_, err := r.svc.Log(ctx, cmp, pageA, dm, LogInput{KindID: "kplace", Op: OpAdd, Amount: 1})
		if code(err) != http.StatusNotFound {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestLogDatesAndValidation(t *testing.T) {
	ctx := context.Background()

	t.Run("stamps the clock's date", func(t *testing.T) {
		r := newRig(t)
		k := r.addKind(t, Kind{ID: "k1", Name: "Gold", Shape: ShapeCount})
		r.addTracker(t, Tracker{ID: "t1", KindID: k.ID, Name: "Gold"})
		v, err := r.svc.Log(ctx, cmp, pageA, dm, LogInput{TrackerID: "t1", Op: OpAdd, Amount: 5})
		if err != nil {
			t.Fatal(err)
		}
		if v.Value != 5 || len(v.Log) != 1 || v.Log[0].When != "3 Frostfall 1" {
			t.Fatalf("got %+v", v)
		}
		e := r.repo.entries[0]
		if e.Game == nil || e.Source != SourceHand || e.CreatedBy != dm.UserID {
			t.Fatalf("entry = %+v", e)
		}
	})

	t.Run("a clock error still logs, undated", func(t *testing.T) {
		r := newRig(t)
		r.clock.err = errors.New("calendar down")
		r.clock.date = &GameDate{Label: "must not be used"}
		k := r.addKind(t, Kind{ID: "k1", Name: "Gold", Shape: ShapeCount})
		r.addTracker(t, Tracker{ID: "t1", KindID: k.ID, Name: "Gold"})
		v, err := r.svc.Log(ctx, cmp, pageA, dm, LogInput{TrackerID: "t1", Op: OpAdd, Amount: 5})
		if err != nil {
			t.Fatal(err)
		}
		if v.Value != 5 || v.Log[0].When != "" || r.repo.entries[0].Game != nil {
			t.Fatalf("got %+v / %+v", v.Log[0], r.repo.entries[0])
		}
	})

	t.Run("no calendar logs undated", func(t *testing.T) {
		r := newRig(t)
		r.clock.date = nil
		k := r.addKind(t, Kind{ID: "k1", Name: "Gold", Shape: ShapeCount})
		r.addTracker(t, Tracker{ID: "t1", KindID: k.ID, Name: "Gold"})
		if _, err := r.svc.Log(ctx, cmp, pageA, dm, LogInput{TrackerID: "t1", Op: OpAdd, Amount: 1}); err != nil {
			t.Fatal(err)
		}
		if r.repo.entries[0].Game != nil {
			t.Fatal("dated without a calendar")
		}
	})

	t.Run("nil clock service", func(t *testing.T) {
		r := newRig(t)
		svc := NewService(r.repo, &fakeDir{pages: map[string]PageAccess{pageA: {Exists: true, CanView: true}}}, nil)
		k := r.addKind(t, Kind{ID: "k1", Name: "Gold", Shape: ShapeCount})
		r.addTracker(t, Tracker{ID: "t1", KindID: k.ID, Name: "Gold"})
		if _, err := svc.Log(ctx, cmp, pageA, dm, LogInput{TrackerID: "t1", Op: OpAdd, Amount: 1}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("invalid input is refused before anything is written", func(t *testing.T) {
		r := newRig(t)
		k := r.addKind(t, Kind{ID: "k1", Name: "Gold", Shape: ShapeCount})
		r.addTracker(t, Tracker{ID: "t1", KindID: k.ID, Name: "Gold"})
		_, err := r.svc.Log(ctx, cmp, pageA, dm, LogInput{TrackerID: "t1", Op: OpAdd, Amount: 0})
		if code(err) != http.StatusUnprocessableEntity || len(r.repo.entries) != 0 {
			t.Fatalf("err = %v entries %d", err, len(r.repo.entries))
		}
	})

	t.Run("tracker on another page is not found", func(t *testing.T) {
		r := newRig(t)
		k := r.addKind(t, Kind{ID: "k1", Name: "Gold", Shape: ShapeCount})
		r.addTracker(t, Tracker{ID: "t1", EntityID: pageHid, KindID: k.ID, Name: "Gold"})
		_, err := r.svc.Log(ctx, cmp, pageA, dm, LogInput{TrackerID: "t1", Op: OpAdd, Amount: 1})
		if code(err) != http.StatusNotFound {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("full log is refused", func(t *testing.T) {
		r := newRig(t)
		k := r.addKind(t, Kind{ID: "k1", Name: "Gold", Shape: ShapeCount})
		r.addTracker(t, Tracker{ID: "t1", KindID: k.ID, Name: "Gold"})
		r.repo.entries = make([]Entry, MaxEntries)
		for i := range r.repo.entries {
			r.repo.entries[i] = Entry{ID: int64(i + 1), TrackerID: "t1", Op: OpAdd, Amount: 1}
		}
		_, err := r.svc.Log(ctx, cmp, pageA, dm, LogInput{TrackerID: "t1", Op: OpAdd, Amount: 1})
		if code(err) != http.StatusUnprocessableEntity {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestStrikeChangesValue(t *testing.T) {
	r := newRig(t)
	k := r.addKind(t, Kind{ID: "k1", Name: "Gold", Shape: ShapeCount})
	r.addTracker(t, Tracker{ID: "t1", KindID: k.ID, Name: "Gold"})
	ctx := context.Background()
	for _, n := range []int{5, 3} {
		if _, err := r.svc.Log(ctx, cmp, pageA, dm, LogInput{TrackerID: "t1", Op: OpAdd, Amount: n}); err != nil {
			t.Fatal(err)
		}
	}
	v, err := r.svc.Strike(ctx, cmp, 2, true, dm)
	if err != nil {
		t.Fatal(err)
	}
	if v.Value != 5 || !v.Log[0].Struck || v.Log[0].ID != 2 {
		t.Fatalf("after strike = %+v", v)
	}
	v, err = r.svc.Strike(ctx, cmp, 2, false, dm)
	if err != nil || v.Value != 8 || v.Log[0].Struck {
		t.Fatalf("after restore = %+v err %v", v, err)
	}
	if _, err := r.svc.Strike(ctx, cmp, 99, true, dm); code(err) != http.StatusNotFound {
		t.Fatalf("missing line: %v", err)
	}
}

func TestPlace(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	k := r.addKind(t, Kind{ID: "k1", Name: "Gold", Shape: ShapeCount, Visibility: SeeDirectors})

	v, err := r.svc.Place(ctx, cmp, pageA, dm, PlaceInput{KindID: k.ID})
	if err != nil {
		t.Fatal(err)
	}
	if v.Name != "Gold" || v.Visibility != SeeDirectors || v.ID == "" {
		t.Fatalf("defaults not taken from the kind: %+v", v)
	}
	if _, err := r.svc.Place(ctx, cmp, pageA, dm, PlaceInput{KindID: k.ID}); code(err) != http.StatusUnprocessableEntity {
		t.Fatalf("duplicate name: %v", err)
	}
	if _, err := r.svc.Place(ctx, cmp, pageA, dm, PlaceInput{KindID: "nope"}); code(err) != http.StatusUnprocessableEntity {
		t.Fatalf("unknown kind: %v", err)
	}
	if _, err := r.svc.Place(ctx, cmp, pageA, dm, PlaceInput{KindID: k.ID, Name: "Second", Visibility: "all"}); code(err) != http.StatusUnprocessableEntity {
		t.Fatalf("bad visibility: %v", err)
	}
}

func TestKindLifecycle(t *testing.T) {
	ctx := context.Background()

	t.Run("create validates, versions at 1 and checks category", func(t *testing.T) {
		r := newRig(t)
		k, err := r.svc.CreateKind(ctx, cmp, dm, KindInput{Name: "Gold", Shape: ShapeCount, EveryTypeID: typeHero})
		if err != nil || k.Version != 1 || k.ID == "" || k.CampaignID != cmp {
			t.Fatalf("k = %+v err %v", k, err)
		}
		if _, err := r.svc.CreateKind(ctx, cmp, dm, KindInput{Name: "Gold", Shape: ShapeCount}); code(err) != http.StatusUnprocessableEntity {
			t.Fatalf("duplicate: %v", err)
		}
		if _, err := r.svc.CreateKind(ctx, cmp, dm, KindInput{Name: "Odd", Shape: ShapeCount, EveryTypeID: 999}); code(err) != http.StatusUnprocessableEntity {
			t.Fatalf("unknown category: %v", err)
		}
		if _, err := r.svc.CreateKind(ctx, cmp, dm, KindInput{Name: "Bad", Shape: "dial"}); code(err) != http.StatusUnprocessableEntity {
			t.Fatalf("bad shape: %v", err)
		}
	})

	t.Run("update bumps the version and refuses a stale one", func(t *testing.T) {
		r := newRig(t)
		k := r.addKind(t, Kind{ID: "k1", Name: "Gold", Shape: ShapeCount, Version: 3})
		in := KindInput{Name: "Coin", Shape: ShapeCount, Version: 3}
		got, err := r.svc.UpdateKind(ctx, cmp, k.ID, dm, in)
		if err != nil || got.Version != 4 || got.Name != "Coin" {
			t.Fatalf("got %+v err %v", got, err)
		}
		if _, err := r.svc.UpdateKind(ctx, cmp, k.ID, dm, in); code(err) != http.StatusConflict {
			t.Fatalf("stale version: %v", err)
		}
		if r.repo.kinds["k1"].Name != "Coin" || r.repo.kinds["k1"].Version != 4 {
			t.Fatalf("stored = %+v", r.repo.kinds["k1"])
		}
	})

	t.Run("update that loses the race is a conflict", func(t *testing.T) {
		r := newRig(t)
		r.addKind(t, Kind{ID: "k1", Name: "Gold", Shape: ShapeCount})
		race := &racyRepo{fakeRepo: r.repo}
		svc := NewService(race, &fakeDir{pages: map[string]PageAccess{}}, nil)
		_, err := svc.UpdateKind(ctx, cmp, "k1", dm, KindInput{Name: "Coin", Shape: ShapeCount, Version: 1})
		if code(err) != http.StatusConflict {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("update of an unknown kind", func(t *testing.T) {
		r := newRig(t)
		_, err := r.svc.UpdateKind(ctx, cmp, "nope", dm, KindInput{Name: "x", Shape: ShapeCount, Version: 1})
		if code(err) != http.StatusNotFound {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("shape change refused while in use, allowed when unused", func(t *testing.T) {
		r := newRig(t)
		k := r.addKind(t, Kind{ID: "k1", Name: "Gold", Shape: ShapeCount})
		in := KindInput{Name: "Gold", Shape: ShapeClock, Max: ip(6), Version: 1}

		r.addTracker(t, Tracker{ID: "t1", KindID: k.ID, Name: "Gold"})
		if _, err := r.svc.UpdateKind(ctx, cmp, k.ID, dm, in); code(err) != http.StatusUnprocessableEntity {
			t.Fatalf("in use: %v", err)
		}
		if r.repo.kinds["k1"].Shape != ShapeCount {
			t.Fatal("shape changed despite the refusal")
		}
		// Same shape is still editable while in use.
		if _, err := r.svc.UpdateKind(ctx, cmp, k.ID, dm, KindInput{Name: "Gold", Shape: ShapeCount, Max: ip(9), Version: 1}); err != nil {
			t.Fatalf("same-shape edit: %v", err)
		}

		delete(r.repo.trackers, "t1")
		in.Version = 2
		got, err := r.svc.UpdateKind(ctx, cmp, k.ID, dm, in)
		if err != nil || got.Shape != ShapeClock {
			t.Fatalf("unused: %+v err %v", got, err)
		}
	})

	t.Run("delete refused while in use, then allowed", func(t *testing.T) {
		r := newRig(t)
		k := r.addKind(t, Kind{ID: "k1", Name: "Gold", Shape: ShapeCount})
		r.addTracker(t, Tracker{ID: "t1", KindID: k.ID, Name: "Gold"})
		if err := r.svc.DeleteKind(ctx, cmp, k.ID, dm); code(err) != http.StatusUnprocessableEntity {
			t.Fatalf("in use: %v", err)
		}
		if _, ok := r.repo.kinds["k1"]; !ok {
			t.Fatal("kind removed while in use")
		}
		if err := r.svc.Remove(ctx, cmp, "t1", dm); err != nil {
			t.Fatal(err)
		}
		if err := r.svc.DeleteKind(ctx, cmp, k.ID, dm); err != nil {
			t.Fatalf("unused: %v", err)
		}
		if err := r.svc.DeleteKind(ctx, cmp, k.ID, dm); code(err) != http.StatusNotFound {
			t.Fatalf("already gone: %v", err)
		}
	})

	t.Run("kinds list sorts categories by name", func(t *testing.T) {
		r := newRig(t)
		r.addKind(t, Kind{ID: "k1", Name: "Gold", Shape: ShapeCount})
		_, types, err := r.svc.Kinds(ctx, cmp, dm)
		if err != nil || len(types) != 2 || types[0].Name != "Hero" || types[1].Name != "Place" {
			t.Fatalf("types = %+v err %v", types, err)
		}
	})
}

// racyRepo reports a lost update race: the version check fails at write time.
type racyRepo struct{ *fakeRepo }

func (racyRepo) UpdateKind(context.Context, Kind, int) (bool, error) { return false, nil }

func TestRepoFailureNeverLeaksRawError(t *testing.T) {
	r := newRig(t)
	svc := NewService(brokenRepo{r.repo}, &fakeDir{pages: map[string]PageAccess{pageA: {Exists: true, CanView: true}}}, nil)
	_, err := svc.Page(context.Background(), cmp, pageA, dm)
	if code(err) != http.StatusInternalServerError {
		t.Fatalf("err = %v", err)
	}
	if msg := apperror.SafeMessage(err); strings.Contains(msg, "sql") || strings.Contains(msg, "refused") {
		t.Fatalf("client message leaks the raw error: %q", msg)
	}
}

type brokenRepo struct{ *fakeRepo }

func (brokenRepo) ListKinds(context.Context, string) ([]Kind, error) {
	return nil, errors.New("sql: connection refused")
}
