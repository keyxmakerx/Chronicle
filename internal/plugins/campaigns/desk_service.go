// desk_service.go assembles the owner's desk. It owns the provider registry
// (the seam other plugins register through) and the rule that a provider can
// never break the page: each runs under a short deadline, a failure or a
// panic is logged, and the row it would have filled falls back to the quiet
// word "Open".

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
	// Build reads every provider and builds every row. It never fails: a
	// figure whose numbers are missing is the word "Open".
	Build(ctx context.Context, req DeskRequest, in DeskInputs) *Desk
	// Figure builds the figures a Manage tool page draws beside its title,
	// from fresh numbers, so a change made on the page can redraw them
	// without redrawing the page. False when the page's line does not exist
	// for this campaign; an empty figure draws nothing.
	Figure(ctx context.Context, req DeskRequest, in DeskInputs, id string) (DeskFigure, bool)
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
				slog.Warn("desk: provider failed, row shows Open",
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
			case DeskKeys:
				merged.Keys = f.Keys
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

	data := []DeskLine{copyLine(c), activityLine(c)}
	if in.Addons["media-gallery"] {
		data = append(data, mediaLine(c, facts.Media))
	}
	data = append(data, trashLine(c, facts.Trash))
	var conns []DeskLine
	if in.Addons["sync-api"] {
		conns = append(conns, foundryLine(c, facts.Foundry, req.Now))
	}
	conns = append(conns, aiLine(c, facts.AI), keysLine(c, facts.Keys))

	return &Desk{
		CampaignName: c.Name,
		Needs:        needsYou(c, withoutDisabled(facts, in.Addons), req.Now),
		Left: []DeskGroup{
			{Name: "General", Lines: []DeskLine{nameLine(c), descriptionLine(c)}},
			{Name: "Access", Lines: []DeskLine{publicLine(c), pagesLine(c), previewLine(c)}},
			{Name: "People", Lines: []DeskLine{peopleLine(c, in.Members), invitedLine(c, in), changesLine(c, facts.Changes)}},
			{Name: "Game", Lines: []DeskLine{systemLine(c, in), featuresLine(c, in)}},
		},
		Right: []DeskGroup{
			{Name: "Customize", Lines: customizeLines(c, in)},
			{Name: "Connections", Lines: conns},
			{Name: "Data", Lines: data},
			{Name: "Danger zone", Danger: true, Lines: []DeskLine{endLine(c)}},
		},
	}
}

// figure builds one tool page's figures by the id of the line that page names.
// Foundry and Media exist only while their add-on is on, the same rule Build
// applies. Details and data is a page with a line but no figure.
func (s *deskService) figure(id string, c *Campaign, in DeskInputs, f DeskFacts, now time.Time) (DeskFigure, bool) {
	switch id {
	case "people":
		return peopleFigure(in.Members, in), true
	case "changes":
		return changesFigure(f.Changes), true
	case "media":
		if !in.Addons["media-gallery"] {
			return DeskFigure{}, false
		}
		return mediaFigure(f.Media), true
	case "foundry":
		if !in.Addons["sync-api"] {
			return DeskFigure{}, false
		}
		return foundryFigure(c, f.Foundry, now), true
	case "game":
		return gameFigure(in), true
	case "settings":
		return DeskFigure{}, true
	case "ai":
		return aiFigure(c, f.AI), true
	case "trash":
		return trashFigure(c, f.Trash), true
	}
	return DeskFigure{}, false
}

func (s *deskService) NeedsCount(ctx context.Context, req DeskRequest, in DeskInputs) int {
	facts := s.gather(ctx, req, DeskFoundry, DeskAI)
	return len(needsYou(in.Campaign, withoutDisabled(facts, in.Addons), req.Now))
}

func (s *deskService) Figure(ctx context.Context, req DeskRequest, in DeskInputs, id string) (DeskFigure, bool) {
	var facts DeskFacts
	if slot, ok := deskSlotOf[id]; ok {
		facts = s.gather(ctx, req, slot)
	}
	return s.figure(id, in.Campaign, in, facts, req.Now)
}

// deskSlotOf is the provider slot whose numbers each tool page's figures draw
// on; People and Game have none.
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
