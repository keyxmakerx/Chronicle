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
func (s *deskService) gather(ctx context.Context, req DeskRequest) DeskFacts {
	s.mu.RLock()
	ps := make([]DeskProvider, 0, len(s.providers))
	for _, p := range s.providers {
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

	table := []DeskLine{peopleLine(c, in.Members), changesLine(c, facts.Changes)}
	campaign := []DeskLine{nameLine(c), gameLine(c, in), pagesLine(c), lookLine(c)}
	if in.Addons["media-gallery"] {
		campaign = append(campaign, mediaLine(c, facts.Media))
	}
	campaign = append(campaign, settingsLine(c))

	var conns []DeskLine
	if in.Addons["sync-api"] {
		conns = append(conns, foundryLine(c, facts.Foundry, req.Now))
	}
	conns = append(conns, aiLine(c, facts.AI), keysLine(c))

	d := &Desk{
		CampaignName: c.Name,
		Needs:        needsYou(c, withoutDisabled(facts, in.Addons), req.Now),
		Groups: []DeskGroup{
			{Name: "The table", Lines: table},
			{Name: "The campaign", Lines: campaign},
			{Name: "Connections", Lines: conns},
			{Name: "Safety net", Lines: []DeskLine{trashLine(c, facts.Trash), endLine(c)}},
		},
	}
	return d
}

// withoutDisabled drops the facts of a feature that is off, so the desk never
// asks the owner to look at Foundry while Foundry sync is switched off.
func withoutDisabled(f DeskFacts, addons map[string]bool) DeskFacts {
	if !addons["sync-api"] {
		f.Foundry = nil
	}
	return f
}
