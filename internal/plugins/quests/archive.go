package quests

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// Archive moves quest sheets in and out of a campaign export. It is separate
// from QuestService because an export is the owner's whole copy (DM-only
// ledger included) and an import writes straight to storage: it must not
// create due-date calendar events, which the imported calendar already holds.
type Archive interface {
	// Export returns every stored sheet, ordered by page id.
	Export(ctx context.Context, campaignID string) ([]ArchivedQuest, error)
	// Import validates a sheet and stores it on the page entityID, with every
	// reference already pointed at the new campaign by remap. It returns what
	// it had to leave out, in plain words.
	Import(ctx context.Context, campaignID, entityID, userID string, data []byte, remap Remap) ([]string, error)
}

// ArchivedQuest is one stored sheet and the page it belongs to.
type ArchivedQuest struct {
	EntityID string
	// Name is the page's name, for naming a sheet in an import report.
	Name string
	Data []byte
}

// Remap turns ids from an export into ids in the new campaign. Each returns
// false when the original did not come back.
type Remap struct {
	Entity func(string) (string, bool)
	Map    func(string) (string, bool)
	Event  func(string) (string, bool)
}

type archive struct{ svc *questService }

// NewArchive builds the archive over the same stores the quest service uses.
func NewArchive(repo QuestRepository, entities EntityDirectory, maps MapDirectory, cal CalendarDirectory) Archive {
	return &archive{svc: &questService{repo: repo, gate: gate{entities: entities}, maps: maps, cal: cal}}
}

func (a *archive) Export(ctx context.Context, campaignID string) ([]ArchivedQuest, error) {
	docs, err := a.svc.repo.ListAll(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("export quests: %w", err)
	}
	ids := make([]string, 0, len(docs))
	for id := range docs {
		ids = append(ids, id)
	}
	infos := map[string]EntityInfo{}
	if len(ids) > 0 {
		if infos, err = a.svc.entities.Entities(ctx, campaignID, ids); err != nil {
			return nil, fmt.Errorf("export quests: %w", err)
		}
	}
	out := make([]ArchivedQuest, 0, len(docs))
	for id, d := range docs {
		out = append(out, ArchivedQuest{EntityID: id, Name: infos[id].Name, Data: d})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EntityID < out[j].EntityID })
	return out, nil
}

func (a *archive) Import(ctx context.Context, campaignID, entityID, userID string, data []byte, remap Remap) ([]string, error) {
	ents, err := a.svc.entities.Entities(ctx, campaignID, []string{entityID})
	if err != nil {
		return nil, apperrorInternal(err)
	}
	ent, ok := ents[entityID]
	if !ok {
		return nil, errNotFound("page")
	}
	var in Quest
	if err := json.Unmarshal(data, &in); err != nil {
		return nil, errInvalid("the quest sheet is not readable")
	}
	var left []string
	dropped := func(what string) { left = append(left, what) }

	// A reference whose target did not come back is dropped rather than
	// failing the sheet; the caller reports it.
	rewards := in.Rewards[:0:0]
	for _, r := range in.Rewards {
		if r.EntityID != "" {
			id, ok := remap.Entity(r.EntityID)
			if !ok {
				dropped("a reward's linked page")
				r.EntityID = ""
			} else {
				r.EntityID = id
			}
		}
		rewards = append(rewards, r)
	}
	foes := in.Foes[:0:0]
	for _, f := range in.Foes {
		if f.EntityID != "" {
			id, ok := remap.Entity(f.EntityID)
			if !ok {
				dropped("a foe's linked page")
				f.EntityID = ""
			} else {
				f.EntityID = id
			}
		}
		foes = append(foes, f)
	}
	links := in.Links[:0:0]
	for _, l := range in.Links {
		resolve := remap.Entity
		if l.Kind == KindMap {
			resolve = remap.Map
		}
		id, ok := resolve(l.RefID)
		if !ok {
			dropped("a link to a page or map")
			continue
		}
		l.RefID = id
		links = append(links, l)
	}
	mapID := in.MapID
	if mapID != "" {
		if id, ok := remap.Map(mapID); ok {
			mapID = id
		} else {
			dropped("the map on the board")
			mapID = ""
		}
	}

	q := defaultQuest(ent.Name)
	p := QuestPatch{
		Notice:    patch.Of(in.Notice),
		Status:    patch.Of(in.Status),
		HandedOut: patch.Of(in.HandedOut),
		Steps:     patch.Of(in.Steps),
		Rewards:   patch.Of(rewards),
		Foes:      patch.Of(foes),
		Links:     patch.Of(links),
		Layout: patch.Of(LayoutPatch{
			Notice: &in.Layout.Notice, Map: &in.Layout.Map, Tag: &in.Layout.Tag,
		}),
		Looks: patch.Of(LooksPatch{Board: patch.Of(in.Looks.Board), Ledger: patch.Of(in.Looks.Ledger)}),
	}
	if mapID != "" {
		p.MapID = patch.Of(mapID)
	}
	if err := a.svc.merge(ctx, campaignID, &q, p); err != nil {
		return nil, err
	}

	// The due day needs the new campaign's calendar to agree it is a real day;
	// its event link carries over only when that event was restored.
	if in.DueDate != nil {
		cal := a.svc.viewCalendar(ctx, campaignID)
		if cal != nil && checkDue(cal, *in.DueDate) == nil {
			d := *in.DueDate
			q.DueDate = &d
			if in.DueEventID != "" {
				if id, ok := remap.Event(in.DueEventID); ok {
					q.DueEventID = id
				}
			}
		} else {
			dropped("the due date")
		}
	}

	out, err := json.Marshal(q)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	saved, err := a.svc.repo.Save(ctx, campaignID, entityID, out, 0, userID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !saved {
		return nil, errInvalid("the page already has a quest sheet")
	}
	return left, nil
}
