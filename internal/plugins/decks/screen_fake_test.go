package decks

import (
	"context"
	"sort"
	"time"
)

// memScreen is an in-memory ScreenRepository over a memRepo, applying the same
// filters as the SQL; the integration tests prove the SQL keeps them.
type memScreen struct {
	repo  *memRepo
	picks map[string][]string
	seen  map[string]bool
}

func newMemScreen(repo *memRepo) *memScreen {
	return &memScreen{repo: repo, picks: map[string][]string{}, seen: map[string]bool{}}
}

func (m *memScreen) Picks(_ context.Context, campaignID, userID string) ([]string, error) {
	var out []string
	for _, id := range m.picks[campaignID+"|"+userID] {
		// A pick goes with its deck, as the foreign key does.
		if _, ok := m.repo.decks[id]; ok {
			out = append(out, id)
		}
	}
	return out, nil
}

func (m *memScreen) SetPicks(_ context.Context, campaignID, userID string, deckIDs []string) error {
	m.picks[campaignID+"|"+userID] = append([]string(nil), deckIDs...)
	return nil
}

func (m *memScreen) LastMoves(_ context.Context, _ string) (map[string]int64, error) {
	out := map[string]int64{}
	for _, l := range m.repo.logs {
		if l.ID > out[l.DeckID] {
			out[l.DeckID] = l.ID
		}
	}
	return out, nil
}

func (m *memScreen) Fresh(_ context.Context, _, userID string, heroIDs []string, window time.Duration, limit int) ([]FreshRow, error) {
	hero := map[string]bool{}
	for _, id := range heroIDs {
		hero[id] = true
	}
	var rows []Copy
	for _, c := range m.repo.copies {
		if c.Zone != ZoneHand || c.FaceUp || !hero[c.HolderEntityID] || c.DealtAt == nil || c.DealtBy == "" || c.DealtBy == userID {
			continue
		}
		if c.DealtAt.Before(time.Now().Add(-window)) || m.seen[c.ID+"|"+userID] {
			continue
		}
		rows = append(rows, c)
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].DealtAt.After(*rows[j].DealtAt) })
	var out []FreshRow
	for _, c := range rows {
		if len(out) == limit {
			break
		}
		out = append(out, FreshRow{CopyID: c.ID, DeckID: c.DeckID})
	}
	return out, nil
}

func (m *memScreen) MarkSeen(_ context.Context, _, userID, copyID string) error {
	m.seen[copyID+"|"+userID] = true
	return nil
}
