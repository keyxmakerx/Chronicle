// stash_events.go announces finished stash and downtime changes to live
// clients. Payloads carry ids and statuses only: a hidden character's or
// stash's name must never ride on a broadcast, so a listener that wants names
// fetches them through the API as the person it is acting for.
package armory

import (
	"fmt"
	"strconv"
)

// Event types, as sent to live clients. The websocket adapter in internal/app
// accepts exactly these.
const (
	EventStashMoved        = "stash.moved"
	EventStashRequested    = "stash.requested"
	EventStashSettled      = "stash.settled"
	EventStashMoneyChanged = "stash.money_changed"
	EventDowntimeChanged   = "downtime.changed"
)

// StashEventPublisher delivers one event to the campaign's live clients.
// Implemented over the websocket event bus in internal/app; optional, so
// tests and a bus-less setup simply publish nothing.
type StashEventPublisher interface {
	PublishStashEvent(eventType, campaignID, resourceID string, payload map[string]any)
}

type stashEvent struct {
	typ        string
	resourceID string
	payload    map[string]any
}

// eventBatch collects events during an operation so they are published after
// its campaign lock is released: the bus records each event in the change
// feed, and that must not run while other moves wait on the lock.
type eventBatch struct {
	list []stashEvent
}

func (b *eventBatch) add(typ, resourceID string, payload map[string]any) {
	b.list = append(b.list, stashEvent{typ: typ, resourceID: resourceID, payload: payload})
}

// flush publishes everything collected, in order. It is deferred before the
// lock is taken, so it runs after the lock is released.
func (b *eventBatch) flush(pub StashEventPublisher, campaignID string) {
	if pub == nil {
		return
	}
	for _, e := range b.list {
		// A campaign-wide event (the downtime switch) is addressed by the
		// campaign, so the change feed has an id to record.
		id := e.resourceID
		if id == "" {
			id = campaignID
		}
		pub.PublishStashEvent(e.typ, campaignID, id, e.payload)
	}
	b.list = nil
}

func moveResourceID(m *Move) string { return strconv.FormatInt(m.ID, 10) }

// touched returns the character and stash ids a move reads or writes.
func touched(m *Move) (characterIDs, stashIDs []string) {
	characterIDs, stashIDs = []string{}, []string{}
	seen := map[Endpoint]bool{}
	for _, e := range []Endpoint{m.From, m.To} {
		if seen[e] {
			continue
		}
		seen[e] = true
		if e.Kind == EndpointCharacter {
			characterIDs = append(characterIDs, e.ID)
		} else {
			stashIDs = append(stashIDs, e.ID)
		}
	}
	return characterIDs, stashIDs
}

// moved records that a move ran (applied or failed).
func (b *eventBatch) moved(m *Move) {
	chars, stashes := touched(m)
	b.add(EventStashMoved, moveResourceID(m), map[string]any{
		"moveId": m.ID, "status": m.Status, "characterIds": chars, "stashIds": stashes,
	})
}

// requested records a new request waiting for the GM. The summary names only
// the kind and size of the request, never what or where.
func (b *eventBatch) requested(m *Move) {
	summary := fmt.Sprintf("item x%d", m.Quantity)
	if m.Kind == MoveKindMoney {
		summary = "money " + m.Amount.String()
	}
	b.add(EventStashRequested, moveResourceID(m), map[string]any{
		"moveId": m.ID, "requestedBy": m.RequestedBy, "summary": summary,
	})
}

// settled records the answer to a request.
func (b *eventBatch) settled(m *Move) {
	b.add(EventStashSettled, moveResourceID(m), map[string]any{
		"moveId": m.ID, "status": m.Status, "decidedBy": m.DecidedBy,
	})
}

func (b *eventBatch) downtimeChanged(open bool) {
	b.add(EventDowntimeChanged, "", map[string]any{"open": open})
}
