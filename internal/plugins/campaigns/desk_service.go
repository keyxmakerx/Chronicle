// desk_service.go assembles the owner's desk. It owns the provider registry
// (the seam other plugins register through) and the rule that a provider can
// never break the page: each runs under a short deadline, a failure or a
// panic is logged, and the line it would have filled falls back to a plain
// link sentence.

package campaigns

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"
)

// deskProviderTimeout bounds each provider so one slow plugin cannot hold the
// whole desk. The desk is the owner's front door; it must always open.
const deskProviderTimeout = 2 * time.Second

// DeskService builds the desk from campaigns' own data and whatever the
// registered providers can tell it.
type DeskService interface {
	// Register adds a provider for its slot; a second one for the same slot
	// replaces the first.
	Register(p DeskProvider)
	// Build reads every provider and words every line. It never fails: a
	// line whose numbers are missing is a plain sentence.
	Build(ctx context.Context, req DeskRequest, in DeskInputs) *Desk
	// Line words one line again, from fresh numbers, so a change made inside
	// its fold can update its sentence without redrawing the desk. False when
	// the line does not exist for this campaign.
	Line(ctx context.Context, req DeskRequest, in DeskInputs, id string) (DeskLine, bool)
	// Fold reads what a line shows when it unfolds. It never fails: a fold
	// that cannot be read is a plain sentence and a link. False when the
	// line does not exist or does not unfold.
	Fold(ctx context.Context, req DeskRequest, in DeskInputs, id string) (*DeskFold, bool)
}

type deskService struct {
	mu        sync.RWMutex
	providers map[DeskSlot]DeskProvider
}

// NewDeskService returns an empty desk service; plugins register on it.
func NewDeskService() DeskService {
	return &deskService{providers: map[DeskSlot]DeskProvider{}}
}

func (s *deskService) Register(p DeskProvider) {
	if p == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.providers[p.Slot()] = p
}

// gather runs every provider at once and merges what they return. Each result
// is merged under its own slot only, so a provider that fills a pointer that
// is not its own cannot change another line.
func (s *deskService) gather(ctx context.Context, req DeskRequest, only ...DeskSlot) DeskFacts {
	s.mu.RLock()
	ps := make([]DeskProvider, 0, len(s.providers))
	for slot, p := range s.providers {
		if len(only) > 0 && !slices.Contains(only, slot) {
			continue
		}
		ps = append(ps, p)
	}
	s.mu.RUnlock()

	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		merged DeskFacts
	)
	for _, p := range ps {
		wg.Add(1)
		go func(p DeskProvider) {
			defer wg.Done()
			slot := p.Slot()
			f, err := callProvider(ctx, p, req)
			if err != nil {
				slog.Warn("desk: provider failed, line shows a plain sentence",
					slog.String("slot", string(slot)), slog.String("campaign_id", req.CampaignID), slog.Any("error", err))
				return
			}
			mu.Lock()
			defer mu.Unlock()
			switch slot {
			case DeskChanges:
				merged.Changes = f.Changes
			case DeskMedia:
				merged.Media = f.Media
			case DeskFoundry:
				merged.Foundry = f.Foundry
			case DeskAI:
				merged.AI = f.AI
			case DeskTrash:
				merged.Trash = f.Trash
			}
		}(p)
	}
	wg.Wait()
	return merged
}

// callProvider calls one provider under the deadline and turns a panic into
// an error, so the goroutine cannot take the server down with it. The call
// runs on its own goroutine and is abandoned at the deadline: a provider that
// ignores its context must still not hold the desk shut.
func callProvider(ctx context.Context, p DeskProvider, req DeskRequest) (DeskFacts, error) {
	ctx, cancel := context.WithTimeout(ctx, deskProviderTimeout)
	defer cancel()
	type result struct {
		f   DeskFacts
		err error
	}
	done := make(chan result, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- result{err: fmt.Errorf("provider panicked: %v", r)}
			}
		}()
		f, err := p.Facts(ctx, req)
		done <- result{f, err}
	}()
	select {
	case r := <-done:
		return r.f, r.err
	case <-ctx.Done():
		return DeskFacts{}, ctx.Err()
	}
}

func (s *deskService) Build(ctx context.Context, req DeskRequest, in DeskInputs) *Desk {
	c := in.Campaign
	facts := s.gather(ctx, req)
	line := func(id string) DeskLine { l, _ := s.line(id, c, in, facts, req.Now); return l }

	table := []DeskLine{line("people"), line("changes")}
	campaign := []DeskLine{nameLine(c), gameLine(c, in), pagesLine(c), lookLine(c)}
	if in.Addons["media-gallery"] {
		campaign = append(campaign, line("media"))
	}
	campaign = append(campaign, settingsLine(c))

	var conns []DeskLine
	if in.Addons["sync-api"] {
		conns = append(conns, line("foundry"))
	}
	conns = append(conns, line("ai"), keysLine(c))

	d := &Desk{
		CampaignName: c.Name,
		Needs:        needsYou(c, withoutDisabled(facts, in.Addons), req.Now),
		Groups: []DeskGroup{
			{Name: "The table", Lines: table},
			{Name: "The campaign", Lines: campaign},
			{Name: "Connections", Lines: conns},
			{Name: "Safety net", Lines: []DeskLine{line("trash"), endLine(c)}},
		},
	}
	return d
}

// line words one provider-backed or people line. Foundry and Media exist only
// while their add-on is on, the same rule Build applies.
func (s *deskService) line(id string, c *Campaign, in DeskInputs, f DeskFacts, now time.Time) (DeskLine, bool) {
	var l DeskLine
	switch id {
	case "people":
		l = peopleLine(c, in.Members, in.Invited)
	case "changes":
		l = changesLine(c, f.Changes)
	case "media":
		if !in.Addons["media-gallery"] {
			return DeskLine{}, false
		}
		l = mediaLine(c, f.Media)
	case "foundry":
		if !in.Addons["sync-api"] {
			return DeskLine{}, false
		}
		l = foundryLine(c, f.Foundry, now)
	case "ai":
		l = aiLine(c, f.AI)
	case "trash":
		l = trashLine(c, f.Trash)
	default:
		return DeskLine{}, false
	}
	l.Fold = s.foldable(id)
	return l, true
}

func (s *deskService) Line(ctx context.Context, req DeskRequest, in DeskInputs, id string) (DeskLine, bool) {
	var facts DeskFacts
	if slot, ok := deskSlotOf[id]; ok {
		facts = s.gather(ctx, req, slot)
	}
	return s.line(id, in.Campaign, in, facts, req.Now)
}

// deskSlotOf is the provider slot whose numbers each line draws on; People
// has none.
var deskSlotOf = map[string]DeskSlot{
	"changes": DeskChanges, "media": DeskMedia, "foundry": DeskFoundry, "ai": DeskAI, "trash": DeskTrash,
}

func (s *deskService) Fold(ctx context.Context, req DeskRequest, in DeskInputs, id string) (*DeskFold, bool) {
	c := in.Campaign
	plain, ok := s.line(id, c, in, DeskFacts{}, req.Now)
	if !ok || !s.foldable(id) {
		return nil, false
	}
	if id == "people" {
		return peopleFold(c, in), true
	}
	s.mu.RLock()
	p, _ := s.providers[deskFoldSlots[id]].(DeskFolder)
	s.mu.RUnlock()
	f, err := callFold(ctx, p, req)
	if err != nil || f == nil {
		slog.Warn("desk: fold not read, showing a plain sentence",
			slog.String("line", id), slog.String("campaign_id", req.CampaignID), slog.Any("error", err))
		return fallbackFold(plain), true
	}
	if len(f.Rows) > deskFoldRows {
		f.More += len(f.Rows) - deskFoldRows
		f.Rows = f.Rows[:deskFoldRows]
	}
	if f.MoreHref == "" && len(plain.Acts) > 0 {
		f.MoreHref = plain.Acts[0].Href
	}
	return f, true
}

// callFold is callProvider for a fold: under the same deadline, with a panic
// turned into an error.
func callFold(ctx context.Context, p DeskFolder, req DeskRequest) (*DeskFold, error) {
	ctx, cancel := context.WithTimeout(ctx, deskProviderTimeout)
	defer cancel()
	type result struct {
		f   *DeskFold
		err error
	}
	done := make(chan result, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- result{err: fmt.Errorf("provider panicked: %v", r)}
			}
		}()
		f, err := p.Fold(ctx, req)
		done <- result{f, err}
	}()
	select {
	case r := <-done:
		return r.f, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// withoutDisabled drops the facts of a feature that is off, so the desk never
// asks the owner to look at Foundry while Foundry sync is switched off.
func withoutDisabled(f DeskFacts, addons map[string]bool) DeskFacts {
	if !addons["sync-api"] {
		f.Foundry = nil
	}
	return f
}
