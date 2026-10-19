package quests

import (
	"context"
	"hash/fnv"
	"log/slog"
	"sort"

	"github.com/google/uuid"
)

// fillOrder is the order statuses are stored and shown in.
var fillOrder = []string{StatusNotStarted, StatusActive, StatusDone, StatusFailed}

// cleanFill validates a board's fill list and returns it in canonical order,
// once each; an empty list means the board is pinned by hand.
func cleanFill(in []string) ([]string, error) {
	want := map[string]bool{}
	for _, st := range in {
		if !validStatus(st) {
			return nil, errInvalid("fill must list quest statuses: not_started, active, done or failed")
		}
		want[st] = true
	}
	var out []string
	for _, st := range fillOrder {
		if want[st] {
			out = append(out, st)
		}
	}
	return out, nil
}

// questStatus reads a stored sheet's status; a sheet saved before status
// existed, or one that will not parse, counts as not started, as it does on
// the quest page.
func questStatus(raw []byte) string {
	var q struct {
		Status string `json:"status"`
	}
	if err := jsonUnmarshal(raw, &q); err != nil || q.Status == "" {
		return StatusNotStarted
	}
	return q.Status
}

// refillBoard brings every auto notice on one board in line with the
// campaign's quests, after the board's fill list changed.
func (s *boardService) refillBoard(ctx context.Context, campaignID string, b Board) error {
	docs, err := s.quests.ListAll(ctx, campaignID)
	if err != nil {
		return apperrorInternal(err)
	}
	statuses := make(map[string]string, len(docs))
	for id, raw := range docs {
		statuses[id] = questStatus(raw)
	}
	items, err := s.repo.ListItems(ctx, campaignID, []string{b.ID})
	if err != nil {
		return apperrorInternal(err)
	}
	_, err = s.fill(ctx, b, items, statuses, nil)
	return err
}

func (s *boardService) FillQuest(ctx context.Context, campaignID, questID string) ([]Home, error) {
	boards, err := s.repo.ListFillBoards(ctx, campaignID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if len(boards) == 0 {
		return nil, nil
	}
	docs, err := s.quests.GetMany(ctx, campaignID, []string{questID})
	if err != nil {
		return nil, apperrorInternal(err)
	}
	statuses := map[string]string{}
	if raw, ok := docs[questID]; ok {
		statuses[questID] = questStatus(raw)
	}
	ids := make([]string, len(boards))
	for i, b := range boards {
		ids[i] = b.ID
	}
	items, err := s.repo.ListItems(ctx, campaignID, ids)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	byBoard := map[string][]Item{}
	for _, it := range items {
		byBoard[it.BoardID] = append(byBoard[it.BoardID], it)
	}
	var changed []Home
	seen := map[Home]bool{}
	for _, b := range boards {
		ok, err := s.fill(ctx, b, byBoard[b.ID], statuses, &questID)
		if err != nil {
			return changed, err
		}
		if ok && !seen[b.Home] {
			seen[b.Home] = true
			changed = append(changed, b.Home)
		}
	}
	return changed, nil
}

// fill pins the quests whose status the board fills with and takes down the
// auto notices whose quest no longer matches. only limits it to one quest;
// nil means every quest in statuses. A quest the DM pinned by hand is never
// pinned twice, and hand-pinned notices are never touched. Who may see each
// notice is decided when the board is read, as for any notice, so a hidden
// or DM-only quest is pinned here and still reaches only the DM.
func (s *boardService) fill(ctx context.Context, b Board, items []Item, statuses map[string]string, only *string) (bool, error) {
	inScope := func(id string) bool { return only == nil || id == *only }
	want := map[string]bool{}
	for id, st := range statuses {
		if !inScope(id) {
			continue
		}
		for _, f := range b.Fill {
			if st == f {
				want[id] = true
			}
		}
	}
	auto, byHand := map[string]bool{}, map[string]bool{}
	gone := map[string]bool{}
	for _, it := range items {
		if it.Kind != KindNotice {
			continue
		}
		switch {
		case !it.Auto:
			byHand[it.RefID] = true
		case inScope(it.RefID) && !want[it.RefID]:
			gone[it.ID] = true
		default:
			auto[it.RefID] = true
		}
	}
	var del []string
	for _, it := range items {
		if gone[it.ID] || (it.Kind == KindString && (gone[it.FromID] || gone[it.ToID])) {
			del = append(del, it.ID)
		}
	}
	if err := s.repo.DeleteItems(ctx, b.CampaignID, b.ID, del); err != nil {
		return false, apperrorInternal(err)
	}

	var add []string
	for id := range want {
		if !auto[id] && !byHand[id] {
			add = append(add, id)
		}
	}
	sort.Strings(add)
	count := len(items) - len(del)
	notices := 0
	for _, it := range items {
		if it.Kind == KindNotice && !gone[it.ID] {
			notices++
		}
	}
	for _, id := range add {
		// The board cap still holds: a full board shows what fits.
		if count >= MaxItemsPerBoard {
			break
		}
		x, y, r := fillSpot(notices, id)
		it := Item{ID: uuid.NewString(), BoardID: b.ID, CampaignID: b.CampaignID, Kind: KindNotice,
			X: x, Y: y, W: defaultItemWidth, R: r, ByDM: true, RefID: id, Auto: true}
		if err := s.repo.InsertAutoItem(ctx, it); err != nil {
			return false, apperrorInternal(err)
		}
		count++
		notices++
	}
	return len(del) > 0 || len(add) > 0, nil
}

// fillSpot lays new notices out four to a row, three rows deep before the
// rows start again over the first, with a slight tilt that stays the same for
// a quest so the board does not reshuffle.
func fillSpot(i int, questID string) (x, y, r float64) {
	h := fnv.New32a()
	_, _ = h.Write([]byte(questID))
	r = float64(int(h.Sum32()%7)-3) * 0.8
	return 3 + float64(i%4)*24.5, 5 + float64((i/4)%3)*31, r
}

// WithBoardFill wraps the quest service so every saved quest is put on, or
// taken off, the boards that fill themselves. Pass the board service that
// announces changes, so open boards redraw. A failure here is logged and
// never undoes the save: the quest is right, and the board catches up on the
// quest's next save or when its fill list is set again.
func WithBoardFill(q QuestService, b BoardService) QuestService {
	return &fillingQuests{QuestService: q, boards: b}
}

type fillingQuests struct {
	QuestService
	boards BoardService
}

func (s *fillingQuests) Put(ctx context.Context, campaignID, entityID string, v Viewer, p QuestPatch) (*DMQuestView, error) {
	out, err := s.QuestService.Put(ctx, campaignID, entityID, v, p)
	if err == nil {
		if _, ferr := s.boards.FillQuest(ctx, campaignID, entityID); ferr != nil {
			slog.Warn("quests: filling boards after a quest save failed",
				slog.String("campaign_id", campaignID), slog.String("entity_id", entityID), slog.Any("error", ferr))
		}
	}
	return out, err
}
