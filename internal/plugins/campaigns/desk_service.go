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
	// NeedsCount is how many things wait on the owner, for the sidebar's
	// Manage row. It asks only the providers that feed Needs-you, so a count
	// on every page does not cost the whole desk.
	NeedsCount(ctx context.Context, req DeskRequest, in DeskInputs) int
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

	look := []DeskLine{nameLine(c), descriptionLine(c), lookLine(c), gameLine(c, in)}
	if in.Addons["media-gallery"] {
		look = append(look, line("media"))
	}

	var conns []DeskLine
	if in.Addons["sync-api"] {
		conns = append(conns, line("foundry"))
	}
	conns = append(conns, line("ai"), keysLine(c))

	return &Desk{
		CampaignName: c.Name,
		Needs:        needsYou(c, withoutDisabled(facts, in.Addons), req.Now),
		Groups: []DeskGroup{
			{Name: "People", Lines: []DeskLine{line("people"), line("changes")}},
			{Name: "What players and guests see", Lines: []DeskLine{playersHomeLine(), guestsHomeLine(c), pagesLine(c), publicLine(c)}},
			{Name: "Look and game", Lines: look},
			{Name: "Connections", Lines: conns},
			{Name: "Safety net", Lines: []DeskLine{exportLine(c), importLine(c), activityLine(c), line("trash")}},
			{Name: "Archive or delete", Lines: []DeskLine{endLine(c), deleteLine(c)}},
		},
	}
}

// line words one line by its id. Foundry and Media exist only
// while their add-on is on, the same rule Build applies. Only the lines a
// tool page opens with are worded here; the rest are built by Build alone.
func (s *deskService) line(id string, c *Campaign, in DeskInputs, f DeskFacts, now time.Time) (DeskLine, bool) {
	switch id {
	case "people":
		return peopleLine(c, in.Members, in.Invited), true
	case "changes":
		return changesLine(c, f.Changes), true
	case "media":
		if !in.Addons["media-gallery"] {
			return DeskLine{}, false
		}
		return mediaLine(c, f.Media), true
	case "foundry":
		if !in.Addons["sync-api"] {
			return DeskLine{}, false
		}
		return foundryLine(c, f.Foundry, now), true
	case "game":
		return gameLine(c, in), true
	case "settings":
		return settingsLine(c), true
	case "ai":
		return aiLine(c, f.AI), true
	case "trash":
		return trashLine(c, f.Trash), true
	}
	return DeskLine{}, false
}

func (s *deskService) NeedsCount(ctx context.Context, req DeskRequest, in DeskInputs) int {
	facts := s.gather(ctx, req, DeskFoundry, DeskAI)
	return len(needsYou(in.Campaign, withoutDisabled(facts, in.Addons), req.Now))
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

// withoutDisabled drops the facts of a feature that is off, so the desk never
// asks the owner to look at Foundry while Foundry sync is switched off.
func withoutDisabled(f DeskFacts, addons map[string]bool) DeskFacts {
	if !addons["sync-api"] {
		f.Foundry = nil
	}
	return f
}
