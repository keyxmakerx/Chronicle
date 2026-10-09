package quests

import (
	"context"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// In-memory fakes for the service tests. They enforce campaign scoping the
// way the SQL does, so IDOR tests exercise the same contract.

type fakeEntities struct {
	byID   map[string]EntityInfo
	camp   map[string]string // entity id -> campaign id
	hidden map[string]bool   // ids invisible below owner visibility
}

func newFakeEntities() *fakeEntities {
	return &fakeEntities{byID: map[string]EntityInfo{}, camp: map[string]string{}, hidden: map[string]bool{}}
}

func (f *fakeEntities) add(campaign, id, name string, hidden bool) {
	f.byID[id] = EntityInfo{ID: id, Name: name, TypeName: "Place", ImagePath: "img/" + id}
	f.camp[id] = campaign
	f.hidden[id] = hidden
}

func (f *fakeEntities) Entities(_ context.Context, campaignID string, ids []string) (map[string]EntityInfo, error) {
	out := map[string]EntityInfo{}
	for _, id := range ids {
		if e, ok := f.byID[id]; ok && f.camp[id] == campaignID {
			out[id] = e
		}
	}
	return out, nil
}

func (f *fakeEntities) FilterViewable(_ context.Context, campaignID string, ids []string, _ int, _ string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, id := range ids {
		if _, ok := f.byID[id]; ok && f.camp[id] == campaignID && !f.hidden[id] {
			out[id] = true
		}
	}
	return out, nil
}

func (f *fakeEntities) Search(_ context.Context, campaignID, q string, _ int, _ string, limit int) ([]EntityInfo, error) {
	var out []EntityInfo
	for id, e := range f.byID {
		if f.camp[id] == campaignID && strings.Contains(strings.ToLower(e.Name), strings.ToLower(q)) {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

type fakeMaps struct {
	byID map[string]MapInfo
	camp map[string]string
}

func newFakeMaps() *fakeMaps { return &fakeMaps{byID: map[string]MapInfo{}, camp: map[string]string{}} }

func (f *fakeMaps) add(campaign, id, name string) {
	f.byID[id] = MapInfo{ID: id, Name: name}
	f.camp[id] = campaign
}

func (f *fakeMaps) Maps(_ context.Context, campaignID string, ids []string) (map[string]MapInfo, error) {
	out := map[string]MapInfo{}
	for _, id := range ids {
		if m, ok := f.byID[id]; ok && f.camp[id] == campaignID {
			out[id] = m
		}
	}
	return out, nil
}

func (f *fakeMaps) ListMaps(_ context.Context, campaignID string) ([]MapInfo, error) {
	var out []MapInfo
	for id, m := range f.byID {
		if f.camp[id] == campaignID {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

type fakeNames map[string]string

func (f fakeNames) DisplayNames(context.Context, string, []string) (map[string]string, error) {
	return f, nil
}

type fakeQuestRepo struct {
	docs     map[string][]byte
	versions map[string]int
	camp     map[string]string
}

func newFakeQuestRepo() *fakeQuestRepo {
	return &fakeQuestRepo{docs: map[string][]byte{}, versions: map[string]int{}, camp: map[string]string{}}
}

func (f *fakeQuestRepo) Get(_ context.Context, campaignID, id string) ([]byte, int, bool, error) {
	if d, ok := f.docs[id]; ok && f.camp[id] == campaignID {
		return d, f.versions[id], true, nil
	}
	return nil, 0, false, nil
}

func (f *fakeQuestRepo) GetMany(_ context.Context, campaignID string, ids []string) (map[string][]byte, error) {
	out := map[string][]byte{}
	for _, id := range ids {
		if d, ok := f.docs[id]; ok && f.camp[id] == campaignID {
			out[id] = d
		}
	}
	return out, nil
}

func (f *fakeQuestRepo) Save(_ context.Context, campaignID, id string, data []byte, expected int, _ string) (bool, error) {
	if f.versions[id] != expected {
		return false, nil
	}
	f.docs[id], f.camp[id] = data, campaignID
	f.versions[id] = expected + 1
	return true, nil
}

type fakeBoardRepo struct {
	looks  map[Home]Looks
	boards []Board
	items  []Item
}

func newFakeBoardRepo() *fakeBoardRepo { return &fakeBoardRepo{looks: map[Home]Looks{}} }

func (f *fakeBoardRepo) GetLooks(_ context.Context, _ string, eid Home) (Looks, error) {
	if l, ok := f.looks[eid]; ok {
		return l, nil
	}
	return Looks{Board: LookLit, Ledger: LookLit}, nil
}
func (f *fakeBoardRepo) SetLooks(_ context.Context, _ string, eid Home, l Looks) error {
	f.looks[eid] = l
	return nil
}
func (f *fakeBoardRepo) ListBoards(_ context.Context, cid string, eid Home) ([]Board, error) {
	var out []Board
	for _, b := range f.boards {
		if b.CampaignID == cid && b.Home == eid {
			out = append(out, b)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].SortOrder < out[j].SortOrder })
	return out, nil
}
func (f *fakeBoardRepo) GetBoard(_ context.Context, cid string, eid Home, bid string) (*Board, error) {
	for _, b := range f.boards {
		if b.ID == bid && b.CampaignID == cid && b.Home == eid {
			c := b
			return &c, nil
		}
	}
	return nil, apperror.NewNotFound("board")
}
func (f *fakeBoardRepo) InsertBoard(_ context.Context, b Board) error {
	f.boards = append(f.boards, b)
	return nil
}
func (f *fakeBoardRepo) UpdateBoard(_ context.Context, b Board) error {
	for i := range f.boards {
		if f.boards[i].ID == b.ID {
			f.boards[i] = b
		}
	}
	return nil
}
func (f *fakeBoardRepo) DeleteBoard(_ context.Context, _ string, _ Home, bid string) error {
	var keep []Board
	for _, b := range f.boards {
		if b.ID != bid {
			keep = append(keep, b)
		}
	}
	f.boards = keep
	return nil
}
func (f *fakeBoardRepo) SetOrder(_ context.Context, _ string, _ Home, ids []string) error {
	for i, id := range ids {
		for j := range f.boards {
			if f.boards[j].ID == id {
				f.boards[j].SortOrder = i
			}
		}
	}
	return nil
}
func (f *fakeBoardRepo) ListItems(_ context.Context, cid string, bids []string) ([]Item, error) {
	var out []Item
	for _, it := range f.items {
		for _, b := range bids {
			if it.BoardID == b && it.CampaignID == cid {
				out = append(out, it)
			}
		}
	}
	return out, nil
}
func (f *fakeBoardRepo) GetItem(_ context.Context, cid, bid, iid string) (*Item, error) {
	for _, it := range f.items {
		if it.ID == iid && it.BoardID == bid && it.CampaignID == cid {
			c := it
			return &c, nil
		}
	}
	return nil, apperror.NewNotFound("item")
}
func (f *fakeBoardRepo) CountItems(_ context.Context, _, bid string) (int, error) {
	n := 0
	for _, it := range f.items {
		if it.BoardID == bid {
			n++
		}
	}
	return n, nil
}
func (f *fakeBoardRepo) InsertItem(_ context.Context, it Item) error {
	f.items = append(f.items, it)
	return nil
}
func (f *fakeBoardRepo) UpdateItem(_ context.Context, it Item) error {
	for i := range f.items {
		if f.items[i].ID == it.ID {
			f.items[i] = it
		}
	}
	return nil
}
func (f *fakeBoardRepo) DeleteItems(_ context.Context, _, bid string, ids []string) error {
	gone := map[string]bool{}
	for _, id := range ids {
		gone[id] = true
	}
	var keep []Item
	for _, it := range f.items {
		if it.BoardID != bid || !gone[it.ID] {
			keep = append(keep, it)
		}
	}
	f.items = keep
	return nil
}

func parseItemPatch(t interface{ Fatal(...any) }, body string) ItemPatch {
	var p ItemPatch
	if err := jsonUnmarshal([]byte(body), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func parseLooksPatch(t interface{ Fatal(...any) }, body string) LooksPatch {
	var p LooksPatch
	if err := jsonUnmarshal([]byte(body), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

// fakeTypes maps a campaign to the category ids it owns.
type fakeTypes map[string]map[int]bool

func (f fakeTypes) TypeInCampaign(_ context.Context, campaignID string, typeID int) (bool, error) {
	return f[campaignID][typeID], nil
}
