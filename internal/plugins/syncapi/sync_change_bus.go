package syncapi

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"time"

	ws "github.com/keyxmakerx/chronicle/internal/websocket"
)

// changeRecordTimeout bounds the one INSERT a publisher waits on. Live
// delivery matters more than the feed, so a slow database loses a feed row
// (clients recover through resetRequired) rather than delaying the broadcast.
const changeRecordTimeout = 500 * time.Millisecond

// changeFeedTypes maps an allowlisted message-type prefix (the part before
// the final dot) to the resource type stored in the feed. Prefix matching on
// the whole segment keeps entity_note.* and calendar.date.* out: those have
// different audiences or no addressable resource.
var changeFeedTypes = map[string]string{
	"entity":         "entity",
	"entity_type":    "entity_type",
	"note":           "note",
	"marker":         "marker",
	"drawing":        "drawing",
	"token":          "token",
	"layer":          "layer",
	"fog":            "fog",
	"map":            "map",
	"relation":       "relation",
	"calendar.event": "calendar_event",
}

// recordedTypes is changeFeedTypes' resource types, sorted, as the feed
// response reports them.
var recordedTypes = func() []string {
	seen := map[string]bool{}
	out := []string{}
	for _, t := range changeFeedTypes {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return out
}()

// classifyChange returns the feed resource type and op for a message type,
// or ok=false when the type is not part of the feed.
func classifyChange(t ws.MessageType) (resourceType, op string, ok bool) {
	s := string(t)
	i := strings.LastIndex(s, ".")
	if i < 0 {
		return "", "", false
	}
	resourceType, ok = changeFeedTypes[s[:i]]
	if !ok {
		return "", "", false
	}
	switch s[i+1:] {
	case "created":
		op = "created"
	case "updated", "moved", "metadata_updated":
		// A token move changes the token and a relation's metadata write
		// changes the relation; the client refetches either way.
		op = "updated"
	case "deleted":
		op = "deleted"
	default:
		return "", "", false
	}
	return resourceType, op, true
}

// ChangeRecorder is the write side of the change feed.
type ChangeRecorder interface {
	Append(ctx context.Context, campaignID, resourceType, resourceID, op string) (int64, error)
}

// RecordingEventBus wraps the real bus so every allowlisted event lands in
// the change feed and carries its seq. Doing it at the bus means no
// publisher can forget to record.
type RecordingEventBus struct {
	next     ws.EventBus
	recorder ChangeRecorder
}

// NewRecordingEventBus wraps next; every message is still forwarded to it.
func NewRecordingEventBus(next ws.EventBus, recorder ChangeRecorder) *RecordingEventBus {
	return &RecordingEventBus{next: next, recorder: recorder}
}

// Publish records the change (best effort), stamps seq, then publishes. The
// caller's message is copied so stamping seq does not mutate it.
func (b *RecordingEventBus) Publish(msg *ws.Message) {
	if msg == nil {
		return
	}
	out := msg
	if resourceType, op, ok := classifyChange(msg.Type); ok && msg.CampaignID != "" {
		ctx, cancel := context.WithTimeout(context.Background(), changeRecordTimeout)
		seq, err := b.recorder.Append(ctx, msg.CampaignID, resourceType, msg.ResourceID, op)
		cancel()
		if err != nil {
			slog.Warn("sync change not recorded; publishing live anyway",
				slog.String("type", string(msg.Type)),
				slog.String("campaign_id", msg.CampaignID),
				slog.Any("error", err))
		} else {
			cp := *msg
			cp.Seq = seq
			out = &cp
		}
	}
	b.next.Publish(out)
}

// changeRetention is how long feed rows are kept. A client offline longer
// than this gets resetRequired and does a full resync.
const changeRetention = 30 * 24 * time.Hour

// StartChangePruner deletes expired feed rows at startup and then once a
// day until ctx ends; the startup pass keeps a server restarted more often
// than daily from never pruning.
func StartChangePruner(ctx context.Context, repo SyncChangeRepository) {
	pruneChanges(ctx, repo)
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pruneChanges(ctx, repo)
		}
	}
}

func pruneChanges(ctx context.Context, repo SyncChangeRepository) {
	n, err := repo.PruneOlderThan(ctx, time.Now().UTC().Add(-changeRetention))
	if err != nil {
		slog.Error("sync change pruning failed", slog.Any("error", err))
	} else if n > 0 {
		slog.Info("pruned sync changes", slog.Int64("rows", n))
	}
}
