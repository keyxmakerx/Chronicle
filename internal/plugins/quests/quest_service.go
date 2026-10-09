package quests

import (
	"context"
	"encoding/json"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// QuestService owns the quest sheet rules.
type QuestService interface {
	// Get returns the DM view (*DMQuestView) for the DM team and the filtered
	// *PlayerQuestView for everyone else. A page that is missing or hidden
	// from the viewer is NotFound.
	Get(ctx context.Context, campaignID, entityID string, v Viewer) (any, error)
	// Put applies a partial update and returns the DM view. DM team only.
	Put(ctx context.Context, campaignID, entityID string, v Viewer, p QuestPatch) (*DMQuestView, error)
}

type questService struct {
	repo QuestRepository
	gate
	maps MapDirectory
}

// NewQuestService builds the service.
func NewQuestService(repo QuestRepository, entities EntityDirectory, maps MapDirectory) QuestService {
	return &questService{repo: repo, gate: gate{entities: entities}, maps: maps}
}

// load reads the stored sheet or the defaults for the page.
func (s *questService) load(ctx context.Context, campaignID string, ent EntityInfo) (Quest, int, error) {
	data, version, found, err := s.repo.Get(ctx, campaignID, ent.ID)
	if err != nil {
		return Quest{}, 0, apperrorInternal(err)
	}
	if !found {
		return defaultQuest(ent.Name), 0, nil
	}
	q := defaultQuest(ent.Name)
	if err := json.Unmarshal(data, &q); err != nil {
		return Quest{}, 0, apperrorInternal(err)
	}
	fillDefaults(&q, ent.Name)
	return q, version, nil
}

// fillDefaults keeps a sheet written before a field existed readable, and
// never lets a nil slice reach the wire as null.
func fillDefaults(q *Quest, name string) {
	if q.Notice.Title == "" {
		q.Notice.Title = name
	}
	if q.Notice.Kicker == "" {
		q.Notice.Kicker = defaultKicker
	}
	if q.Notice.Body == nil {
		q.Notice.Body = []string{}
	}
	if q.Status == "" {
		q.Status = StatusNotStarted
	}
	if q.Steps == nil {
		q.Steps = []Step{}
	}
	if q.Rewards == nil {
		q.Rewards = []Reward{}
	}
	if q.Foes == nil {
		q.Foes = []Foe{}
	}
	if q.Links == nil {
		q.Links = []Link{}
	}
	if q.Layout.Notice.W == 0 && q.Layout.Map.W == 0 && q.Layout.Tag.W == 0 {
		q.Layout = defaultLayout()
	}
	if q.Looks.Board == "" {
		q.Looks.Board = LookLit
	}
	if q.Looks.Ledger == "" {
		q.Looks.Ledger = LookLit
	}
}

func (s *questService) Get(ctx context.Context, campaignID, entityID string, v Viewer) (any, error) {
	ent, err := s.requireViewable(ctx, campaignID, entityID, v)
	if err != nil {
		return nil, err
	}
	q, version, err := s.load(ctx, campaignID, ent)
	if err != nil {
		return nil, err
	}
	if v.IsDM {
		return s.dmView(ctx, campaignID, q, version)
	}
	return s.playerView(ctx, campaignID, q)
}

func (s *questService) playerView(ctx context.Context, campaignID string, q Quest) (*PlayerQuestView, error) {
	out := &PlayerQuestView{
		CanEdit:   false,
		Status:    q.Status,
		HandedOut: q.HandedOut,
		Steps:     []PlayerStep{},
		Layout:    q.Layout,
		Looks:     q.Looks,
	}
	if !q.Layout.Notice.Hidden {
		n := q.Notice
		// A hidden reward tag hides the reward line on the notice too.
		if q.Layout.Tag.Hidden {
			n.Reward = ""
		}
		out.Notice = &n
	}
	out.ShowTag = q.Notice.Reward != "" && !q.Layout.Tag.Hidden
	for _, st := range q.Steps {
		if st.Shown {
			out.Steps = append(out.Steps, PlayerStep{Text: st.Text, Done: st.Done})
		} else {
			out.HiddenSteps = true
		}
	}
	if q.MapID != "" && !q.Layout.Map.Hidden {
		maps, err := s.maps.Maps(ctx, campaignID, []string{q.MapID})
		if err != nil {
			return nil, apperrorInternal(err)
		}
		if m, ok := maps[q.MapID]; ok {
			out.Map = &MapRef{ID: m.ID, Name: m.Name}
		}
	}
	return out, nil
}

func (s *questService) dmView(ctx context.Context, campaignID string, q Quest, version int) (*DMQuestView, error) {
	var entIDs, mapIDs []string
	for _, r := range q.Rewards {
		if r.EntityID != "" {
			entIDs = append(entIDs, r.EntityID)
		}
	}
	for _, f := range q.Foes {
		if f.EntityID != "" {
			entIDs = append(entIDs, f.EntityID)
		}
	}
	for _, l := range q.Links {
		if l.Kind == "map" {
			mapIDs = append(mapIDs, l.RefID)
		} else {
			entIDs = append(entIDs, l.RefID)
		}
	}
	if q.MapID != "" {
		mapIDs = append(mapIDs, q.MapID)
	}
	ents, err := s.entities.Entities(ctx, campaignID, entIDs)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	maps, err := s.maps.Maps(ctx, campaignID, mapIDs)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	mapsOn, err := s.maps.Enabled(ctx, campaignID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	out := &DMQuestView{
		MapsOn:  mapsOn,
		CanEdit: true, Version: version, Notice: q.Notice, Status: q.Status,
		HandedOut: q.HandedOut, Steps: q.Steps, MapID: q.MapID,
		MapName: maps[q.MapID].Name, Layout: q.Layout, Looks: q.Looks,
		Rewards: make([]RewardView, 0, len(q.Rewards)),
		Foes:    make([]FoeView, 0, len(q.Foes)),
		Links:   make([]LinkView, 0, len(q.Links)),
	}
	for _, r := range q.Rewards {
		out.Rewards = append(out.Rewards, RewardView{Reward: r, Name: ents[r.EntityID].Name})
	}
	for _, f := range q.Foes {
		out.Foes = append(out.Foes, FoeView{Foe: f, Name: ents[f.EntityID].Name})
	}
	for _, l := range q.Links {
		name := ents[l.RefID].Name
		if l.Kind == "map" {
			name = maps[l.RefID].Name
		}
		out.Links = append(out.Links, LinkView{Link: l, Name: name})
	}
	return out, nil
}

func (s *questService) Put(ctx context.Context, campaignID, entityID string, v Viewer, p QuestPatch) (*DMQuestView, error) {
	if !v.IsDM {
		return nil, errForbidden("only the campaign owner or a member with DM access may do this")
	}
	ent, err := s.requireViewable(ctx, campaignID, entityID, v)
	if err != nil {
		return nil, err
	}
	want, ok := p.Version.Get()
	if !ok || want < 0 {
		return nil, errInvalid("version is required")
	}
	q, version, err := s.load(ctx, campaignID, ent)
	if err != nil {
		return nil, err
	}
	if want != version {
		return nil, apperror.NewConflict("this quest was changed by someone else; reload and try again")
	}
	if err := s.merge(ctx, campaignID, &q, p); err != nil {
		return nil, err
	}
	data, err := json.Marshal(q)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	saved, err := s.repo.Save(ctx, campaignID, entityID, data, version, v.UserID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !saved {
		return nil, apperror.NewConflict("this quest was changed by someone else; reload and try again")
	}
	return s.dmView(ctx, campaignID, q, version+1)
}

// merge applies the patch over the stored sheet and validates what changed.
// Only the references the patch carries are checked against the campaign, so
// a stale link elsewhere on the sheet cannot block an unrelated save.
func (s *questService) merge(ctx context.Context, campaignID string, q *Quest, p QuestPatch) error {
	hadEnt, hadMap := storedRefs(*q)
	if p.Notice.Present() {
		if n, ok := p.Notice.Get(); ok {
			nn, err := normalizeNotice(n)
			if err != nil {
				return err
			}
			if nn.Kicker == "" {
				nn.Kicker = defaultKicker
			}
			q.Notice = nn
		}
	}
	if st, ok := p.Status.Get(); ok {
		if !validStatus(st) {
			return errInvalid("status must be not_started, active, done or failed")
		}
		q.Status = st
	}
	q.HandedOut = p.HandedOut.Val(q.HandedOut)
	var err error
	if v, ok := p.Steps.Get(); ok {
		if q.Steps, err = normalizeSteps(v); err != nil {
			return err
		}
	}
	var entIDs, mapIDs []string
	if v, ok := p.Rewards.Get(); ok {
		if q.Rewards, err = normalizeRewards(v); err != nil {
			return err
		}
		for _, r := range q.Rewards {
			if r.EntityID != "" {
				entIDs = append(entIDs, r.EntityID)
			}
		}
	}
	if v, ok := p.Foes.Get(); ok {
		if q.Foes, err = normalizeFoes(v); err != nil {
			return err
		}
		for _, f := range q.Foes {
			if f.EntityID != "" {
				entIDs = append(entIDs, f.EntityID)
			}
		}
	}
	if v, ok := p.Links.Get(); ok {
		if q.Links, err = normalizeLinks(v); err != nil {
			return err
		}
		for _, l := range q.Links {
			if l.Kind == "map" {
				mapIDs = append(mapIDs, l.RefID)
			} else {
				entIDs = append(entIDs, l.RefID)
			}
		}
	}
	if p.MapID.Present() {
		id, _ := p.MapID.Get() // explicit null leaves "" = no map
		if id != "" && !idPattern.MatchString(id) {
			return errInvalid("map id is not valid")
		}
		q.MapID = id
		if id != "" {
			mapIDs = append(mapIDs, id)
		}
	}
	if lp, ok := p.Layout.Get(); ok {
		if lp.Notice != nil {
			q.Layout.Notice = clampRect(*lp.Notice)
		}
		if lp.Map != nil {
			q.Layout.Map = clampRect(*lp.Map)
		}
		if lp.Tag != nil {
			q.Layout.Tag = clampRect(*lp.Tag)
		}
	}
	if lk, ok := p.Looks.Get(); ok {
		if b, ok := lk.Board.Get(); ok {
			if !validLook(b) {
				return errInvalid("board look is not valid")
			}
			q.Looks.Board = b
		}
		if l, ok := lk.Ledger.Get(); ok {
			if !validLook(l) {
				return errInvalid("ledger look is not valid")
			}
			q.Looks.Ledger = l
		}
	}
	// Only ids new to the sheet are checked: a stored one was checked when it
	// was added, and a page or map deleted since must not block every later
	// edit of the list that still names it.
	return s.checkRefs(ctx, campaignID, newIDs(entIDs, hadEnt), newIDs(mapIDs, hadMap))
}

// storedRefs collects the page and map ids a sheet already points at.
func storedRefs(q Quest) (ents, maps map[string]bool) {
	ents, maps = map[string]bool{}, map[string]bool{}
	for _, r := range q.Rewards {
		ents[r.EntityID] = true
	}
	for _, f := range q.Foes {
		ents[f.EntityID] = true
	}
	for _, l := range q.Links {
		if l.Kind == "map" {
			maps[l.RefID] = true
		} else {
			ents[l.RefID] = true
		}
	}
	maps[q.MapID] = true
	return ents, maps
}

func newIDs(ids []string, had map[string]bool) []string {
	var out []string
	for _, id := range ids {
		if !had[id] {
			out = append(out, id)
		}
	}
	return out
}

// checkRefs refuses ids that are not pages / maps of this campaign, so a
// sheet can never point across campaigns.
func (s *questService) checkRefs(ctx context.Context, campaignID string, entIDs, mapIDs []string) error {
	if len(entIDs) > 0 {
		found, err := s.entities.Entities(ctx, campaignID, entIDs)
		if err != nil {
			return apperrorInternal(err)
		}
		for _, id := range entIDs {
			if _, ok := found[id]; !ok {
				return errInvalid("a linked page is not in this campaign")
			}
		}
	}
	if len(mapIDs) > 0 {
		found, err := s.maps.Maps(ctx, campaignID, mapIDs)
		if err != nil {
			return apperrorInternal(err)
		}
		for _, id := range mapIDs {
			if _, ok := found[id]; !ok {
				return errInvalid("a linked map is not in this campaign")
			}
		}
	}
	return nil
}
