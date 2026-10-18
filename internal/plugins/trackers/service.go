package trackers

import (
	"context"
	"errors"
	"sort"

	"github.com/google/uuid"
)

// Service owns the tracker rules. Only Directors define kinds, place, change
// or remove trackers, and see hidden ones; players see the trackers marked
// for everyone on pages they can open, without the log's notes.
type Service interface {
	// Page is every tracker on a page as v may see it, with the kinds a
	// Director can add.
	Page(ctx context.Context, campaignID, entityID string, v Viewer) (*PageView, error)
	Place(ctx context.Context, campaignID, entityID string, v Viewer, in PlaceInput) (*TrackerView, error)
	Patch(ctx context.Context, campaignID, trackerID string, v Viewer, in TrackerPatch) (*TrackerView, error)
	Remove(ctx context.Context, campaignID, trackerID string, v Viewer) error
	// Log writes one change on a tracker of the page and returns the
	// tracker as it now stands.
	Log(ctx context.Context, campaignID, entityID string, v Viewer, in LogInput) (*TrackerView, error)
	// Strike strikes a log line, or restores it when struck is false.
	Strike(ctx context.Context, campaignID string, entryID int64, struck bool, v Viewer) (*TrackerView, error)

	Kinds(ctx context.Context, campaignID string, v Viewer) ([]Kind, []TypeRef, error)
	CreateKind(ctx context.Context, campaignID string, v Viewer, in KindInput) (*Kind, error)
	UpdateKind(ctx context.Context, campaignID, kindID string, v Viewer, in KindInput) (*Kind, error)
	DeleteKind(ctx context.Context, campaignID, kindID string, v Viewer) error
}

type service struct {
	repo     Repository
	entities EntityDirectory
	clock    GameClock
	newID    func() string
}

// NewService builds the service. clock may be nil when the calendar plugin is
// unavailable; log lines are then undated.
func NewService(repo Repository, entities EntityDirectory, clock GameClock) Service {
	return &service{repo: repo, entities: entities, clock: clock, newID: func() string { return uuid.NewString() }}
}

func requireDirector(v Viewer) error {
	if !v.Director {
		return errForbidden("only the campaign's Directors can change trackers")
	}
	return nil
}

// page decides that a page exists in the campaign and v may see it. A hidden
// page is reported exactly like a missing one.
func (s *service) page(ctx context.Context, campaignID, entityID string, v Viewer) (PageAccess, error) {
	if !validToken(entityID) {
		return PageAccess{}, errNotFound("page")
	}
	a, err := s.entities.Access(ctx, campaignID, entityID, v)
	if err != nil {
		return PageAccess{}, apperrorInternal(err)
	}
	if !a.Exists || !a.CanView {
		return PageAccess{}, errNotFound("page")
	}
	return a, nil
}

// sees reports whether v may see a tracker with this visibility.
func sees(v Viewer, visibility string) bool { return v.Director || visibility == SeeEveryone }

// view builds what v sees of one tracker. entries are oldest first.
func view(k Kind, t Tracker, entries []Entry, v Viewer) TrackerView {
	value, after := fold(k, entries)
	tv := TrackerView{
		ID: t.ID, KindID: k.ID, Name: t.Name, Shape: k.Shape, Visibility: t.Visibility,
		Value: value, Min: k.Min, Max: k.Max, Steps: k.Steps, Bands: k.Bands,
		Now: nowText(k, value), LogCount: len(entries), Log: []EntryView{},
	}
	for i := len(entries) - 1; i >= 0 && len(tv.Log) < LogShown; i-- {
		e := entries[i]
		ev := EntryView{ID: e.ID, Op: e.Op, Amount: e.Amount, Source: e.Source, At: e.CreatedAt,
			Struck: e.StruckAt != nil, After: after[i]}
		// Notes are the DM's working words; players get the change and its
		// date only.
		if v.Director {
			ev.Note = e.Note
		}
		if e.Game != nil {
			ev.When = e.Game.Label
		}
		tv.Log = append(tv.Log, ev)
	}
	return tv
}

func (s *service) kindMap(ctx context.Context, campaignID string) (map[string]Kind, []Kind, error) {
	list, err := s.repo.ListKinds(ctx, campaignID)
	if err != nil {
		return nil, nil, apperrorInternal(err)
	}
	m := make(map[string]Kind, len(list))
	for _, k := range list {
		m[k.ID] = k
	}
	return m, list, nil
}

func (s *service) Page(ctx context.Context, campaignID, entityID string, v Viewer) (*PageView, error) {
	a, err := s.page(ctx, campaignID, entityID, v)
	if err != nil {
		return nil, err
	}
	kinds, list, err := s.kindMap(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	placed, err := s.repo.ListTrackers(ctx, campaignID, entityID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	ts := make([]Tracker, 0, len(placed))
	hasKind, hasName := map[string]bool{}, map[string]bool{}
	for _, t := range placed {
		hasKind[t.KindID], hasName[t.Name] = true, true
		if _, ok := kinds[t.KindID]; ok && sees(v, t.Visibility) {
			ts = append(ts, t)
		}
	}
	// Kinds that go on every page of this category show here before anyone
	// has changed them; the first change creates the row.
	for _, k := range list {
		if k.EveryTypeID == 0 || k.EveryTypeID != a.TypeID || hasKind[k.ID] || hasName[k.Name] {
			continue
		}
		if sees(v, k.Visibility) {
			ts = append(ts, Tracker{KindID: k.ID, Name: k.Name, Visibility: k.Visibility, SortOrder: 1 << 30})
		}
	}
	ids := make([]string, 0, len(ts))
	for _, t := range ts {
		if t.ID != "" {
			ids = append(ids, t.ID)
		}
	}
	entries, err := s.repo.ListEntries(ctx, campaignID, ids)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	out := &PageView{Trackers: []TrackerView{}, CanEdit: v.Director}
	for _, t := range ts {
		out.Trackers = append(out.Trackers, view(kinds[t.KindID], t, entries[t.ID], v))
	}
	if v.Director {
		out.Kinds = []KindChoice{}
		for _, k := range list {
			out.Kinds = append(out.Kinds, KindChoice{ID: k.ID, Name: k.Name, Shape: k.Shape, Visibility: k.Visibility})
		}
	}
	return out, nil
}

// one returns a single tracker's view after a change.
func (s *service) one(ctx context.Context, campaignID string, t Tracker, v Viewer) (*TrackerView, error) {
	k, ok, err := s.repo.GetKind(ctx, campaignID, t.KindID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !ok {
		return nil, errNotFound("tracker kind")
	}
	entries, err := s.repo.ListEntries(ctx, campaignID, []string{t.ID})
	if err != nil {
		return nil, apperrorInternal(err)
	}
	tv := view(k, t, entries[t.ID], v)
	return &tv, nil
}

func (s *service) Place(ctx context.Context, campaignID, entityID string, v Viewer, in PlaceInput) (*TrackerView, error) {
	if err := requireDirector(v); err != nil {
		return nil, err
	}
	if _, err := s.page(ctx, campaignID, entityID, v); err != nil {
		return nil, err
	}
	if !validToken(in.KindID) {
		return nil, errInvalid("pick a tracker kind")
	}
	k, ok, err := s.repo.GetKind(ctx, campaignID, in.KindID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !ok {
		return nil, errInvalid("pick a tracker kind")
	}
	if in.Name == "" {
		in.Name = k.Name
	}
	name, err := cleanName("name", in.Name)
	if err != nil {
		return nil, err
	}
	if in.Visibility == "" {
		in.Visibility = k.Visibility
	}
	if !validVisibility(in.Visibility) {
		return nil, errInvalid("visibility must be directors or everyone")
	}
	existing, err := s.repo.ListTrackers(ctx, campaignID, entityID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if len(existing) >= MaxTrackersPage {
		return nil, errInvalid("a page can hold at most 40 trackers")
	}
	t := Tracker{ID: s.newID(), CampaignID: campaignID, EntityID: entityID, KindID: k.ID, Name: name, Visibility: in.Visibility}
	if err := s.repo.CreateTracker(ctx, t); err != nil {
		if errors.Is(err, errDuplicate) {
			return nil, errInvalid("this page already has a tracker called " + name)
		}
		return nil, apperrorInternal(err)
	}
	return s.one(ctx, campaignID, t, v)
}

// tracker finds a placed tracker on a page v may see.
func (s *service) tracker(ctx context.Context, campaignID, trackerID string, v Viewer) (Tracker, error) {
	if !validToken(trackerID) {
		return Tracker{}, errNotFound("tracker")
	}
	t, ok, err := s.repo.GetTracker(ctx, campaignID, trackerID)
	if err != nil {
		return Tracker{}, apperrorInternal(err)
	}
	if !ok || !sees(v, t.Visibility) {
		return Tracker{}, errNotFound("tracker")
	}
	if _, err := s.page(ctx, campaignID, t.EntityID, v); err != nil {
		return Tracker{}, errNotFound("tracker")
	}
	return t, nil
}

func (s *service) Patch(ctx context.Context, campaignID, trackerID string, v Viewer, in TrackerPatch) (*TrackerView, error) {
	if err := requireDirector(v); err != nil {
		return nil, err
	}
	t, err := s.tracker(ctx, campaignID, trackerID, v)
	if err != nil {
		return nil, err
	}
	if in.Name != nil {
		name, err := cleanName("name", *in.Name)
		if err != nil {
			return nil, err
		}
		t.Name = name
	}
	if in.Visibility != nil {
		if !validVisibility(*in.Visibility) {
			return nil, errInvalid("visibility must be directors or everyone")
		}
		t.Visibility = *in.Visibility
	}
	if err := s.repo.UpdateTracker(ctx, t); err != nil {
		if errors.Is(err, errDuplicate) {
			return nil, errInvalid("this page already has a tracker called " + t.Name)
		}
		return nil, apperrorInternal(err)
	}
	return s.one(ctx, campaignID, t, v)
}

func (s *service) Remove(ctx context.Context, campaignID, trackerID string, v Viewer) error {
	if err := requireDirector(v); err != nil {
		return err
	}
	t, err := s.tracker(ctx, campaignID, trackerID, v)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteTracker(ctx, campaignID, t.ID); err != nil {
		return apperrorInternal(err)
	}
	return nil
}

// ensure returns the tracker a change is for: a placed one by id, or the row
// for a kind that goes on every page of this page's category, created on its
// first change.
func (s *service) ensure(ctx context.Context, campaignID, entityID string, a PageAccess, v Viewer, in LogInput) (Tracker, error) {
	if in.TrackerID != "" {
		t, err := s.tracker(ctx, campaignID, in.TrackerID, v)
		if err != nil {
			return Tracker{}, err
		}
		if t.EntityID != entityID {
			return Tracker{}, errNotFound("tracker")
		}
		return t, nil
	}
	if !validToken(in.KindID) {
		return Tracker{}, errNotFound("tracker")
	}
	k, ok, err := s.repo.GetKind(ctx, campaignID, in.KindID)
	if err != nil {
		return Tracker{}, apperrorInternal(err)
	}
	if !ok || k.EveryTypeID == 0 || k.EveryTypeID != a.TypeID {
		return Tracker{}, errNotFound("tracker")
	}
	if t, found, err := s.repo.TrackerForKind(ctx, campaignID, entityID, k.ID); err != nil {
		return Tracker{}, apperrorInternal(err)
	} else if found {
		return t, nil
	}
	t := Tracker{ID: s.newID(), CampaignID: campaignID, EntityID: entityID, KindID: k.ID, Name: k.Name, Visibility: k.Visibility}
	if err := s.repo.CreateTracker(ctx, t); err != nil {
		if !errors.Is(err, errDuplicate) {
			return Tracker{}, apperrorInternal(err)
		}
		// Someone else's first change made it a moment ago.
		got, found, err := s.repo.TrackerForKind(ctx, campaignID, entityID, k.ID)
		if err != nil {
			return Tracker{}, apperrorInternal(err)
		}
		if !found {
			return Tracker{}, errInvalid("this page already has a tracker called " + k.Name)
		}
		return got, nil
	}
	return t, nil
}

func (s *service) Log(ctx context.Context, campaignID, entityID string, v Viewer, in LogInput) (*TrackerView, error) {
	if err := requireDirector(v); err != nil {
		return nil, err
	}
	a, err := s.page(ctx, campaignID, entityID, v)
	if err != nil {
		return nil, err
	}
	in, err = validateLog(in)
	if err != nil {
		return nil, err
	}
	t, err := s.ensure(ctx, campaignID, entityID, a, v, in)
	if err != nil {
		return nil, err
	}
	n, err := s.repo.CountEntries(ctx, campaignID, t.ID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if n >= MaxEntries {
		return nil, errInvalid("this tracker's log is full")
	}
	e := Entry{TrackerID: t.ID, Op: in.Op, Amount: in.Amount, Note: in.Note, Source: SourceHand, CreatedBy: v.UserID}
	if s.clock != nil {
		// A calendar that can't be read leaves the line undated rather
		// than losing the change.
		if d, err := s.clock.Today(ctx, campaignID, v); err == nil {
			e.Game = d
		}
	}
	if err := s.repo.AddEntry(ctx, campaignID, e); err != nil {
		return nil, apperrorInternal(err)
	}
	return s.one(ctx, campaignID, t, v)
}

func (s *service) Strike(ctx context.Context, campaignID string, entryID int64, struck bool, v Viewer) (*TrackerView, error) {
	if err := requireDirector(v); err != nil {
		return nil, err
	}
	e, ok, err := s.repo.GetEntry(ctx, campaignID, entryID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !ok {
		return nil, errNotFound("log line")
	}
	t, err := s.tracker(ctx, campaignID, e.TrackerID, v)
	if err != nil {
		return nil, err
	}
	if err := s.repo.StrikeEntry(ctx, campaignID, entryID, struck, v.UserID); err != nil {
		return nil, apperrorInternal(err)
	}
	return s.one(ctx, campaignID, t, v)
}

func (s *service) Kinds(ctx context.Context, campaignID string, v Viewer) ([]Kind, []TypeRef, error) {
	if err := requireDirector(v); err != nil {
		return nil, nil, err
	}
	_, list, err := s.kindMap(ctx, campaignID)
	if err != nil {
		return nil, nil, err
	}
	types, err := s.entities.Types(ctx, campaignID)
	if err != nil {
		return nil, nil, apperrorInternal(err)
	}
	sort.SliceStable(types, func(i, j int) bool { return types[i].Name < types[j].Name })
	return list, types, nil
}

// checkType refuses a category that isn't the campaign's own.
func (s *service) checkType(ctx context.Context, campaignID string, id int) error {
	if id == 0 {
		return nil
	}
	types, err := s.entities.Types(ctx, campaignID)
	if err != nil {
		return apperrorInternal(err)
	}
	for _, t := range types {
		if t.ID == id {
			return nil
		}
	}
	return errInvalid("unknown category")
}

func dupName(err error, name string) error {
	if errors.Is(err, errDuplicate) {
		return errInvalid("there is already a tracker kind called " + name)
	}
	return apperrorInternal(err)
}

func (s *service) CreateKind(ctx context.Context, campaignID string, v Viewer, in KindInput) (*Kind, error) {
	if err := requireDirector(v); err != nil {
		return nil, err
	}
	k, err := validateKind(in)
	if err != nil {
		return nil, err
	}
	if err := s.checkType(ctx, campaignID, k.EveryTypeID); err != nil {
		return nil, err
	}
	n, err := s.repo.CountKinds(ctx, campaignID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if n >= MaxKindsPerCamp {
		return nil, errInvalid("a campaign can have at most 200 tracker kinds")
	}
	k.ID, k.CampaignID, k.Version = s.newID(), campaignID, 1
	if err := s.repo.CreateKind(ctx, k); err != nil {
		return nil, dupName(err, k.Name)
	}
	return &k, nil
}

func (s *service) UpdateKind(ctx context.Context, campaignID, kindID string, v Viewer, in KindInput) (*Kind, error) {
	if err := requireDirector(v); err != nil {
		return nil, err
	}
	if !validToken(kindID) {
		return nil, errNotFound("tracker kind")
	}
	old, ok, err := s.repo.GetKind(ctx, campaignID, kindID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !ok {
		return nil, errNotFound("tracker kind")
	}
	if in.Version != old.Version {
		return nil, errConflict()
	}
	k, err := validateKind(in)
	if err != nil {
		return nil, err
	}
	if err := s.checkType(ctx, campaignID, k.EveryTypeID); err != nil {
		return nil, err
	}
	// Changing the shape would turn every placed tracker's log into a
	// different kind of number.
	if k.Shape != old.Shape {
		list, err := s.repo.ListKinds(ctx, campaignID)
		if err != nil {
			return nil, apperrorInternal(err)
		}
		for _, x := range list {
			if x.ID == kindID && x.InUse > 0 {
				return nil, errInvalid("this kind is on pages, so its shape can't change; make a new kind instead")
			}
		}
	}
	k.ID, k.CampaignID = kindID, campaignID
	saved, err := s.repo.UpdateKind(ctx, k, old.Version)
	if err != nil {
		return nil, dupName(err, k.Name)
	}
	if !saved {
		return nil, errConflict()
	}
	k.Version = old.Version + 1
	return &k, nil
}

func (s *service) DeleteKind(ctx context.Context, campaignID, kindID string, v Viewer) error {
	if err := requireDirector(v); err != nil {
		return err
	}
	if !validToken(kindID) {
		return errNotFound("tracker kind")
	}
	if _, ok, err := s.repo.GetKind(ctx, campaignID, kindID); err != nil {
		return apperrorInternal(err)
	} else if !ok {
		return errNotFound("tracker kind")
	}
	inUse, err := s.repo.DeleteKind(ctx, campaignID, kindID)
	if err != nil {
		return apperrorInternal(err)
	}
	if inUse {
		return errInvalid("this kind is still on pages; remove those trackers first")
	}
	return nil
}
