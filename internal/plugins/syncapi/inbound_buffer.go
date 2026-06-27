package syncapi

import (
	"sync"
	"time"
)

// inbound_buffer.go is a bounded in-memory ring of recent INBOUND sync payloads —
// what external clients (the Foundry module) actually sent. It backs the
// operator's `sync.inbound` / `sync.recent` diagnostics so the AI can compare
// "what Foundry sent" against "what Chronicle stored" (entity.fields) and pinpoint
// where data dies. No DB, no migration — mirrors the systems loader's in-memory
// DiagnosticEvents pattern; the buffer simply rolls over on restart.

// InboundRecord is one captured inbound field payload.
type InboundRecord struct {
	EntityID string
	At       time.Time
	Source   string // e.g. "fields" (UpdateEntityFields)
	Fields   map[string]any
}

// inboundBufferCap bounds memory: only the most recent N payloads are kept.
const inboundBufferCap = 200

var (
	inboundMu  sync.Mutex
	inboundBuf []InboundRecord // oldest first; trimmed from the front past the cap
)

// recordInbound appends a captured payload, trimming to the cap. A shallow copy of
// fields is stored so a later mutation of the request map can't alter the record.
func recordInbound(entityID, source string, fields map[string]any, now time.Time) {
	cp := make(map[string]any, len(fields))
	for k, v := range fields {
		cp[k] = v
	}
	inboundMu.Lock()
	inboundBuf = append(inboundBuf, InboundRecord{EntityID: entityID, At: now, Source: source, Fields: cp})
	if len(inboundBuf) > inboundBufferCap {
		inboundBuf = inboundBuf[len(inboundBuf)-inboundBufferCap:]
	}
	inboundMu.Unlock()
}

// RecentInbound returns up to limit records, newest first. entityID=="" spans all
// entities. Exported for the app layer to wire into systems.SetSyncInboundProvider.
func RecentInbound(entityID string, limit int) []InboundRecord {
	if limit <= 0 {
		limit = 10
	}
	inboundMu.Lock()
	defer inboundMu.Unlock()
	out := make([]InboundRecord, 0, limit)
	for i := len(inboundBuf) - 1; i >= 0 && len(out) < limit; i-- {
		if entityID == "" || inboundBuf[i].EntityID == entityID {
			out = append(out, inboundBuf[i])
		}
	}
	return out
}
