package decks

import (
	"context"
	"sort"
	"time"
)

// memRepo is an in-memory Repository that applies the same claim, draw and
// resize rules as the SQL, so the service tests exercise the rules and the
// integration tests prove the SQL keeps them.
type memRepo struct {
	decks   map[string]Deck
	cards   map[string]Card
	copies  []Copy
	logs    []LogLine
	nextLog int64
	seq     int
	clock   int
	// failMoves makes the next Move lose its claim, as a race would.
	failMoves bool
}

func newMemRepo() *memRepo {
	return &memRepo{decks: map[string]Deck{}, cards: map[string]Card{}}
}

func (m *memRepo) id(prefix string) string {
	m.seq++
	return prefix + "-" + string(rune('a'+m.seq%26)) + itoa(m.seq)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

func (m *memRepo) CountDecks(_ context.Context, campaignID string) (int, error) {
	n := 0
	for _, d := range m.decks {
		if d.CampaignID == campaignID {
			n++
		}
	}
	return n, nil
}

func (m *memRepo) ListDecks(_ context.Context, campaignID string) ([]Deck, error) {
	var out []Deck
	for _, d := range m.decks {
		if d.CampaignID == campaignID {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SortOrder < out[j].SortOrder })
	return out, nil
}

func (m *memRepo) GetDeck(_ context.Context, campaignID, id string) (Deck, bool, error) {
	d, ok := m.decks[id]
	if !ok || d.CampaignID != campaignID {
		return Deck{}, false, nil
	}
	return d, true, nil
}

func (m *memRepo) nameTaken(name, except, campaignID string) bool {
	for _, d := range m.decks {
		if d.CampaignID == campaignID && d.Name == name && d.ID != except {
			return true
		}
	}
	return false
}

func (m *memRepo) CreateDeck(_ context.Context, d Deck) error {
	if m.nameTaken(d.Name, "", d.CampaignID) {
		return errDuplicate
	}
	d.Version = 1
	d.Players = []Player{}
	m.decks[d.ID] = d
	return nil
}

func (m *memRepo) UpdateDeck(_ context.Context, d Deck, expected int, picks PickChange) (bool, error) {
	cur, ok := m.decks[d.ID]
	if !ok || cur.Version != expected {
		return false, nil
	}
	if m.nameTaken(d.Name, d.ID, d.CampaignID) {
		return false, errDuplicate
	}
	d.Version = expected + 1
	d.Players = cur.Players
	// Like the SQL, the draw nudges change only when the update names them.
	d.NextCardID, d.LastCardID = cur.NextCardID, cur.LastCardID
	if picks.SetNext {
		d.NextCardID = m.pickIfCard(d.ID, picks.Next)
	}
	if picks.SetLast {
		d.LastCardID = m.pickIfCard(d.ID, picks.Last)
	}
	m.decks[d.ID] = d
	return true, nil
}

// pickIfCard mirrors the SQL subselect: the id stands only while the card is
// still a card of the deck.
func (m *memRepo) pickIfCard(deckID, id string) string {
	if c, ok := m.cards[id]; ok && c.DeckID == deckID {
		return id
	}
	return ""
}

func (m *memRepo) DeleteDeck(_ context.Context, _, id string) error {
	delete(m.decks, id)
	for cid, c := range m.cards {
		if c.DeckID == id {
			delete(m.cards, cid)
		}
	}
	kept := m.copies[:0]
	for _, c := range m.copies {
		if c.DeckID != id {
			kept = append(kept, c)
		}
	}
	m.copies = kept
	return nil
}

func (m *memRepo) SetPlayers(_ context.Context, _, deckID string, players []Player, expected int) (bool, error) {
	d, ok := m.decks[deckID]
	if !ok || d.Version != expected {
		return false, nil
	}
	d.Version++
	d.Players = players
	m.decks[deckID] = d
	return true, nil
}

func (m *memRepo) ListCards(_ context.Context, _, deckID string) ([]Card, error) {
	var out []Card
	for _, c := range m.cards {
		if c.DeckID == deckID {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SortOrder < out[j].SortOrder })
	return out, nil
}

func (m *memRepo) GetCard(_ context.Context, _, deckID, id string) (Card, bool, error) {
	c, ok := m.cards[id]
	if !ok || c.DeckID != deckID {
		return Card{}, false, nil
	}
	return c, true, nil
}

func (m *memRepo) cardNameTaken(deckID, name, except string) bool {
	for _, c := range m.cards {
		if c.DeckID == deckID && c.Name == name && c.ID != except {
			return true
		}
	}
	return false
}

func (m *memRepo) addCopies(c Card, n int) {
	for i := 0; i < n; i++ {
		m.copies = append(m.copies, Copy{ID: m.id("cp"), DeckID: c.DeckID, CampaignID: c.CampaignID, CardID: c.ID, Zone: ZonePile, MovedAt: m.tick()})
	}
}

func (m *memRepo) CreateCard(_ context.Context, c Card) error {
	if m.cardNameTaken(c.DeckID, c.Name, "") {
		return errDuplicate
	}
	cards, copies := 0, 0
	for _, o := range m.cards {
		if o.DeckID == c.DeckID {
			cards++
			copies += o.Copies
		}
	}
	if cards >= MaxCardsPerDeck {
		return errTargetFull
	}
	if copies+c.Copies > MaxCopiesPerDeck {
		return errTargetCopies
	}
	c.SortOrder = cards
	m.cards[c.ID] = c
	m.addCopies(c, c.Copies)
	return nil
}

func (m *memRepo) UpdateCard(_ context.Context, c Card, resize bool) (ResizeResult, error) {
	if m.cardNameTaken(c.DeckID, c.Name, c.ID) {
		return ResizeResult{}, errDuplicate
	}
	if !resize {
		c.Copies = m.cards[c.ID].Copies
		m.cards[c.ID] = c
		return ResizeResult{}, nil
	}
	others := 0
	for _, o := range m.cards {
		if o.DeckID == c.DeckID && o.ID != c.ID {
			others += o.Copies
		}
	}
	if others+c.Copies > MaxCopiesPerDeck {
		return ResizeResult{}, errTargetCopies
	}
	var mine []int
	for i, cp := range m.copies {
		if cp.CardID == c.ID {
			mine = append(mine, i)
		}
	}
	have := len(mine)
	var res ResizeResult
	switch {
	case c.Copies > have:
		m.addCopies(c, c.Copies-have)
		res.Added = c.Copies - have
	case c.Copies < have:
		remove := have - c.Copies
		var doomed []int
		for _, zone := range []string{ZonePile, ZoneUsed, ZoneGone} {
			for _, i := range mine {
				if m.copies[i].Zone == zone && len(doomed) < remove {
					doomed = append(doomed, i)
				}
			}
		}
		if len(doomed) < remove {
			return ResizeResult{}, &heldError{N: have - len(doomed)}
		}
		gone := map[int]bool{}
		for _, i := range doomed {
			gone[i] = true
		}
		var kept []Copy
		for i, cp := range m.copies {
			if !gone[i] {
				kept = append(kept, cp)
			}
		}
		m.copies = kept
		res.Removed = remove
	}
	m.cards[c.ID] = c
	return res, nil
}

func (m *memRepo) DeleteCard(_ context.Context, _, deckID, id string) error {
	delete(m.cards, id)
	if d, ok := m.decks[deckID]; ok {
		if d.NextCardID == id {
			d.NextCardID = ""
		}
		if d.LastCardID == id {
			d.LastCardID = ""
		}
		m.decks[deckID] = d
	}
	var kept []Copy
	for _, cp := range m.copies {
		if cp.CardID != id {
			kept = append(kept, cp)
		}
	}
	m.copies = kept
	return nil
}

func (m *memRepo) ListCopies(_ context.Context, _, deckID string) ([]Copy, error) {
	var out []Copy
	for _, c := range m.copies {
		if c.DeckID == deckID {
			out = append(out, c)
		}
	}
	return out, nil
}

func (m *memRepo) GetCopy(_ context.Context, _, deckID, id string) (Copy, bool, error) {
	for _, c := range m.copies {
		if c.ID == id && c.DeckID == deckID {
			return c, true, nil
		}
	}
	return Copy{}, false, nil
}

func (m *memRepo) ListHeldBy(_ context.Context, _ string, holders []string) ([]Copy, error) {
	want := map[string]bool{}
	for _, h := range holders {
		want[h] = true
	}
	var out []Copy
	for _, c := range m.copies {
		if (c.Zone == ZoneHand || c.Zone == ZonePlay) && want[c.HolderEntityID] {
			out = append(out, c)
		}
	}
	return out, nil
}

func (m *memRepo) ZoneCounts(_ context.Context, _ string) (map[string]ZoneCounts, error) {
	out := map[string]ZoneCounts{}
	for _, c := range m.copies {
		z := out[c.DeckID]
		z.add(c.Zone, 1)
		out[c.DeckID] = z
	}
	return out, nil
}

func (m *memRepo) ListTable(_ context.Context, _ string) ([]TableRow, error) {
	var out []TableRow
	for _, c := range m.copies {
		if c.Zone == ZonePlay && c.HolderEntityID == "" && c.FaceUp {
			d, card := m.decks[c.DeckID], m.cards[c.CardID]
			out = append(out, TableRow{CopyID: c.ID, DeckID: d.ID, DeckName: d.Name, DeckKind: d.Kind, Look: d.Look, Face: faceOf(card)})
		}
	}
	return out, nil
}

func (m *memRepo) index(id string) int {
	for i, c := range m.copies {
		if c.ID == id {
			return i
		}
	}
	return -1
}

func (m *memRepo) writeLog(l LogLine) {
	m.nextLog++
	l.ID = m.nextLog
	l.CreatedAt = time.Now()
	m.logs = append(m.logs, l)
	var mine []LogLine
	var rest []LogLine
	for _, x := range m.logs {
		if x.DeckID == l.DeckID {
			mine = append(mine, x)
		} else {
			rest = append(rest, x)
		}
	}
	if len(mine) > LogKept {
		mine = mine[len(mine)-LogKept:]
	}
	m.logs = append(rest, mine...)
}

// reset sends a copy back to the pile with a fresh id, as the SQL does.
func (m *memRepo) reset(i int) {
	c := &m.copies[i]
	c.ID = m.id("cp")
	c.Zone, c.HolderEntityID, c.FaceUp, c.Open, c.AskedAt, c.DealtBy, c.DealtAt, c.TurnedAt = ZonePile, "", false, false, nil, "", nil, nil
	c.MovedAt = m.tick()
}

// tick is a clock that moves one second per call, so arrival order is exact.
func (m *memRepo) tick() time.Time {
	m.clock++
	return longAgo.Add(time.Duration(m.clock) * time.Second)
}

func (m *memRepo) Move(_ context.Context, mv Move) error {
	i := m.index(mv.CopyID)
	if m.failMoves {
		m.failMoves = false
		return errLostClaim
	}
	if i < 0 {
		return errLostClaim
	}
	c := m.copies[i]
	if c.DeckID != mv.DeckID || c.Zone != mv.From.Zone || c.HolderEntityID != mv.From.HolderEntityID ||
		c.FaceUp != mv.From.FaceUp || c.Open != mv.From.Open || (mv.ExpectAsked && c.AskedAt == nil) {
		return errLostClaim
	}
	now := time.Now()
	oldID := c.ID
	c.Zone, c.HolderEntityID, c.FaceUp = mv.To.Zone, mv.To.HolderEntityID, mv.To.FaceUp
	c.Open = mv.To.Open && mv.To.FaceUp && mv.To.Zone == ZoneHand
	c.MovedAt = m.tick()
	if mv.To.Zone == ZonePile {
		c.ID = m.id("cp")
	}
	switch mv.Asked {
	case AskedMark:
		c.AskedAt = &now
	case AskedClear:
		c.AskedAt = nil
	}
	switch {
	case mv.DealtBy != "":
		c.DealtBy, c.DealtAt = mv.DealtBy, &now
	case mv.To.Zone == ZonePile || mv.To.Zone == ZoneShown:
		c.DealtBy, c.DealtAt, c.TurnedAt = "", nil, nil
	}
	if mv.Turned {
		c.TurnedAt = &now
	}
	m.copies[i] = c
	mv.Log.DeckID, mv.Log.CardID, mv.Log.CardName, mv.Log.CopyID = mv.DeckID, c.CardID, m.cards[c.CardID].Name, oldID
	m.writeLog(mv.Log)
	return nil
}

func (m *memRepo) pile(p DrawPlan, exclude string) []int {
	var out []int
	for i, c := range m.copies {
		if c.DeckID == p.DeckID && c.Zone == ZonePile && (p.CardID == "" || c.CardID == p.CardID) {
			out = append(out, i)
		}
	}
	if exclude != "" {
		var rest []int
		for _, i := range out {
			if m.copies[i].ID != exclude {
				rest = append(rest, i)
			}
		}
		if len(rest) > 0 {
			return rest
		}
	}
	return out
}

func (m *memRepo) Draw(_ context.Context, p DrawPlan) ([]Copy, error) {
	// Work on a snapshot so an error leaves nothing spent or moved, as the
	// SQL transaction rolls back.
	saved := append([]Copy(nil), m.copies...)
	savedDeck := m.decks[p.DeckID]
	savedPlayers := append([]Player(nil), savedDeck.Players...)
	savedLogs, savedNext := append([]LogLine(nil), m.logs...), m.nextLog
	undo := func() {
		m.copies = saved
		savedDeck.Players = savedPlayers
		m.decks[p.DeckID] = savedDeck
		m.logs, m.nextLog = savedLogs, savedNext
	}
	count := p.Count
	if count < 1 {
		count = 1
	}
	if p.SpendUserID != "" {
		d := m.decks[p.DeckID]
		found := false
		for i, pl := range d.Players {
			if pl.UserID != p.SpendUserID {
				continue
			}
			found = true
			if pl.DrawsLeft != nil {
				if *pl.DrawsLeft <= 0 {
					return nil, errNoDraws
				}
				n := *pl.DrawsLeft - 1
				pls := append([]Player(nil), d.Players...)
				pls[i].DrawsLeft = &n
				d.Players = pls
				m.decks[p.DeckID] = d
			}
		}
		if !found {
			return nil, errNoDraws
		}
	}
	exclude := ""
	if p.PutBackCopyID != "" {
		i := m.index(p.PutBackCopyID)
		if i < 0 || m.copies[i].DeckID != p.DeckID || m.copies[i].Zone != ZoneShown {
			undo()
			return nil, errLostClaim
		}
		old := m.copies[i]
		m.reset(i)
		exclude = m.copies[i].ID
		m.writeLog(LogLine{DeckID: p.DeckID, CardID: old.CardID, CardName: m.cards[old.CardID].Name, CopyID: old.ID,
			Action: ActPutBack, ActorUserID: p.Log.ActorUserID})
	}
	shown := 0
	if p.To.Zone == ZoneShown {
		for _, c := range m.copies {
			if c.DeckID == p.DeckID && c.Zone == ZoneShown {
				shown++
			}
		}
	}
	deck := m.decks[p.DeckID]
	next, last := deck.NextCardID, deck.LastCardID
	if p.CardID != "" {
		next, last = "", ""
	}
	nextUsed, lastUsed := false, false
	var out []Copy
	for n := 0; n < count; n++ {
		if n == 0 && p.To.Zone == ZoneShown && shown+1 > MaxShown {
			undo()
			return nil, errShownFull
		}
		cands := m.pile(p, exclude)
		if p.Reshuffle && p.CardID == "" && (len(cands) == 0 || (exclude != "" && len(cands) == 1 && m.copies[cands[0]].ID == exclude)) {
			moved := false
			for i, c := range m.copies {
				if c.DeckID == p.DeckID && c.Zone == ZoneUsed {
					m.reset(i)
					moved = true
				}
			}
			if moved {
				cands = m.pile(p, exclude)
			}
		}
		if len(cands) == 0 {
			if n > 0 {
				break
			}
			undo()
			return nil, errEmptyPile
		}
		if n > 0 && p.To.Zone == ZoneShown && shown+len(out)+1 > MaxShown {
			undo()
			return nil, errShownFull
		}
		pool, tookNext := cands, false
		if n == 0 && next != "" {
			var sub []int
			for _, i := range cands {
				if m.copies[i].CardID == next {
					sub = append(sub, i)
				}
			}
			if len(sub) > 0 {
				pool, tookNext = sub, true
			}
		}
		if !tookNext && last != "" {
			var rest []int
			for _, i := range cands {
				if m.copies[i].CardID != last {
					rest = append(rest, i)
				}
			}
			if len(rest) > 0 {
				pool = rest
			}
		}
		i := pool[p.Pick(len(pool))]
		c := &m.copies[i]
		if tookNext {
			nextUsed, next = true, ""
		}
		if last != "" && c.CardID == last {
			lastUsed, last = true, ""
		}
		now := time.Now()
		c.Zone, c.HolderEntityID, c.FaceUp, c.Open, c.AskedAt, c.TurnedAt = p.To.Zone, p.To.HolderEntityID, p.To.FaceUp, false, nil, nil
		c.MovedAt = m.tick()
		if p.DealtBy != "" {
			c.DealtBy, c.DealtAt = p.DealtBy, &now
		}
		line := p.Log
		line.DeckID, line.CardID, line.CardName, line.CopyID = p.DeckID, c.CardID, m.cards[c.CardID].Name, c.ID
		m.writeLog(line)
		out = append(out, *c)
	}
	if nextUsed || lastUsed {
		d := m.decks[p.DeckID]
		if nextUsed {
			d.NextCardID = ""
		}
		if lastUsed {
			d.LastCardID = ""
		}
		m.decks[p.DeckID] = d
	}
	return out, nil
}

// Keep mirrors the SQL: both decks change together or neither does.
func (m *memRepo) Keep(_ context.Context, k KeepPlan) error {
	i := m.index(k.CopyID)
	if i < 0 {
		return errLostClaim
	}
	c := m.copies[i]
	if c.DeckID != k.DeckID || c.Zone != k.From.Zone || c.HolderEntityID != k.From.HolderEntityID ||
		c.FaceUp != k.From.FaceUp || c.Open != k.From.Open {
		return errLostClaim
	}
	src := m.cards[c.CardID]
	var targetCards []Card
	copies := 0
	for _, x := range m.cards {
		if x.DeckID == k.TargetDeckID {
			targetCards = append(targetCards, x)
			copies += x.Copies
		}
	}
	if copies+1 > MaxCopiesPerDeck {
		return errTargetCopies
	}
	var existing *Card
	for j := range targetCards {
		if targetCards[j].Name == src.Name {
			existing = &targetCards[j]
		}
	}
	var target Card
	if existing != nil {
		if existing.Copies+1 > MaxCopiesPerCard {
			return errCardCopies
		}
		target = *existing
		target.Copies++
	} else {
		if len(targetCards) >= MaxCardsPerDeck {
			return errTargetFull
		}
		kind := ""
		if k.TargetKinds[src.KindKey] {
			kind = src.KindKey
		}
		target = Card{ID: m.id("card"), DeckID: k.TargetDeckID, CampaignID: k.CampaignID, Name: src.Name, KindKey: kind,
			Words: src.Words, DirectorNote: src.DirectorNote, Picture: src.Picture, Corner: src.Corner, Rare: src.Rare,
			Copies: 1, Fate: src.Fate, AskFirst: src.AskFirst, SortOrder: len(targetCards)}
	}
	m.cards[target.ID] = target
	m.copies[i].Zone, m.copies[i].HolderEntityID, m.copies[i].FaceUp, m.copies[i].Open, m.copies[i].AskedAt = ZoneGone, "", true, false, nil
	m.copies[i].MovedAt = m.tick()
	m.copies = append(m.copies, Copy{ID: m.id("cp"), DeckID: target.DeckID, CampaignID: target.CampaignID, CardID: target.ID, Zone: ZonePile, MovedAt: m.tick()})
	k.Log.DeckID, k.Log.CardID, k.Log.CardName, k.Log.CopyID = k.DeckID, src.ID, src.Name, k.CopyID
	m.writeLog(k.Log)
	return nil
}

func (m *memRepo) bulk(deckID string, only func(Copy) bool, log LogLine) int {
	n := 0
	for i, c := range m.copies {
		if c.DeckID == deckID && only(c) {
			m.reset(i)
			n++
		}
	}
	log.DeckID = deckID
	m.writeLog(log)
	return n
}

func (m *memRepo) Shuffle(_ context.Context, _, deckID string, log LogLine) (int, error) {
	return m.bulk(deckID, func(c Copy) bool { return c.Zone == ZoneUsed }, log), nil
}

func (m *memRepo) Gather(_ context.Context, _, deckID string, log LogLine) (int, error) {
	return m.bulk(deckID, func(c Copy) bool { return c.Zone != ZonePile }, log), nil
}

func (m *memRepo) ListLog(_ context.Context, _, deckID string, limit int) ([]LogLine, error) {
	var out []LogLine
	for i := len(m.logs) - 1; i >= 0 && len(out) < limit; i-- {
		if m.logs[i].DeckID == deckID {
			out = append(out, m.logs[i])
		}
	}
	return out, nil
}

// fakeHeroes knows pages by id.
type fakeHeroes struct {
	pages map[string]Hero
	// hidden pages cannot be opened by a player.
	hidden map[string]bool
}

func (f fakeHeroes) Owned(_ context.Context, _, userID string) ([]Hero, error) {
	var out []Hero
	for _, h := range f.pages {
		if userID != "" && h.OwnerUserID == userID {
			out = append(out, h)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (f fakeHeroes) InCampaign(_ context.Context, _, id string) (Hero, bool, error) {
	h, ok := f.pages[id]
	return h, ok, nil
}

func (f fakeHeroes) Viewable(_ context.Context, _, id string, v Viewer) (bool, error) {
	if _, ok := f.pages[id]; !ok {
		return false, nil
	}
	return v.Director || !f.hidden[id], nil
}

type fakeMembers struct{ list []Member }

func (f fakeMembers) Members(context.Context, string) ([]Member, error) { return f.list, nil }

// capture records moves the notifier was told about.
type capture struct{ got []string }

func (c *capture) Moved(_, deckID, copyID, action, holder, actor string) {
	c.got = append(c.got, deckID+"|"+copyID+"|"+action+"|"+holder+"|"+actor)
}

// ZoneCountsFor counts one deck's copies by zone.
func (m *memRepo) ZoneCountsFor(deckID string) ZoneCounts {
	var z ZoneCounts
	for _, c := range m.copies {
		if c.DeckID == deckID {
			z.add(c.Zone, 1)
		}
	}
	return z
}
