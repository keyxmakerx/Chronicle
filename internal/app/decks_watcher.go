package app

// decks_watcher.go turns "something a card's tests read may have changed" into
// a recheck of that campaign's cards. Many writes arrive together (a sheet
// saved field by field, a tag set replaced), so a poke only marks the campaign
// and one recheck runs shortly after, whatever number of pokes came in.

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/decks"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/transformations"
	"github.com/keyxmakerx/chronicle/internal/widgets/tags"
)

const (
	// decksPokeDelay lets a burst of writes land before the cards are looked at.
	decksPokeDelay = 300 * time.Millisecond
	// decksRecheckBudget bounds one recheck: a poke carries no request context.
	decksRecheckBudget = 10 * time.Second
)

// decksRechecker is what the watcher asks to look again.
type decksRechecker interface {
	Recheck(ctx context.Context, campaignID string) error
}

// decksWatcher coalesces pokes per campaign. A poke that arrives while a
// recheck runs schedules one more, so a write that landed mid-read is never
// lost.
type decksWatcher struct {
	delay time.Duration

	mu      sync.Mutex
	pending map[string]bool
	svc     decksRechecker
}

func newDecksWatcher() *decksWatcher {
	return &decksWatcher{delay: decksPokeDelay, pending: map[string]bool{}}
}

// bind sets the service the watcher rechecks with. It is left unbound while the
// decks plugin is degraded, which makes every poke a no-op.
func (w *decksWatcher) bind(svc decksRechecker) {
	w.mu.Lock()
	w.svc = svc
	w.mu.Unlock()
}

// Poke never blocks and never fails: the write that caused it has finished and
// must not wait on, or fail from, a card.
func (w *decksWatcher) Poke(campaignID string) {
	if campaignID == "" {
		return
	}
	w.mu.Lock()
	if w.svc == nil || w.pending[campaignID] {
		w.mu.Unlock()
		return
	}
	w.pending[campaignID] = true
	w.mu.Unlock()
	time.AfterFunc(w.delay, func() { w.run(campaignID) })
}

func (w *decksWatcher) run(campaignID string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("decks: a recheck panicked", slog.Any("panic", r), slog.String("campaign_id", campaignID))
		}
	}()
	// Cleared before the read, so a write during it queues another recheck.
	w.mu.Lock()
	delete(w.pending, campaignID)
	svc := w.svc
	w.mu.Unlock()
	if svc == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), decksRecheckBudget)
	defer cancel()
	if err := svc.Recheck(ctx, campaignID); err != nil {
		slog.Warn("decks: a recheck failed", slog.String("campaign_id", campaignID), slog.Any("error", err))
	}
}

// decksEntityEvents pokes the watcher after a page is created, changed or
// removed, then lets the real publisher carry on. The poke comes second so the
// publisher is never held up by it.
type decksEntityEvents struct {
	next entities.EntityEventPublisher
	poke func(campaignID string)
}

func (p *decksEntityEvents) PublishEntityTypeEvent(eventType, campaignID string, et *entities.EntityType) {
	p.next.PublishEntityTypeEvent(eventType, campaignID, et)
	// A category's fields decide which keys are numbers.
	p.poke(campaignID)
}

func (p *decksEntityEvents) PublishEntityEvent(eventType, campaignID, entityID string, entity *entities.Entity) {
	p.next.PublishEntityEvent(eventType, campaignID, entityID, entity)
	switch eventType {
	case "updated", "created", "deleted":
		p.poke(campaignID)
	}
}

// decksTagHooks pokes the watcher when a tag goes on or comes off a page.
type decksTagHooks struct {
	next tags.TagHookDispatcher
	poke func(campaignID string)
}

func (h decksTagHooks) TagAdded(ctx context.Context, campaignID, entityID string, t tags.Tag) {
	h.next.TagAdded(ctx, campaignID, entityID, t)
	h.poke(campaignID)
}

func (h decksTagHooks) TagRemoved(ctx context.Context, campaignID, entityID string, t tags.Tag) {
	h.next.TagRemoved(ctx, campaignID, entityID, t)
	h.poke(campaignID)
}

// decksLayersNotifier pokes the watcher when a page's layers change.
type decksLayersNotifier struct {
	next transformations.Notifier
	poke func(campaignID string)
}

func (n decksLayersNotifier) LayersChanged(campaignID, entityID string) {
	n.next.LayersChanged(campaignID, entityID)
	n.poke(campaignID)
}

// decksTrackerObserver pokes the watcher when any tracker of a campaign changes.
type decksTrackerObserver struct{ poke func(campaignID string) }

func (o decksTrackerObserver) TrackersChanged(campaignID string) { o.poke(campaignID) }

var _ decks.MomentNotifier = (*decksLivePublisher)(nil)
