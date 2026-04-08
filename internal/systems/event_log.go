// Package systems — event_log.go provides an in-memory ring buffer for
// system loading events. Captures manifest discoveries, successful
// instantiations, and failures so the admin diagnostics page can show
// what happened without requiring external log access.
package systems

import (
	"sync"
	"time"
)

// LoadEventKind categorizes a system loading event.
type LoadEventKind string

const (
	// EventDiscovered means the manifest was parsed and added to the registry.
	EventDiscovered LoadEventKind = "discovered"

	// EventInstantiated means the system was successfully instantiated.
	EventInstantiated LoadEventKind = "instantiated"

	// EventFailed means the system failed to load or instantiate.
	EventFailed LoadEventKind = "failed"
)

// LoadEvent records a single system loading event.
type LoadEvent struct {
	Timestamp time.Time
	SystemID  string
	Name      string
	Kind      LoadEventKind
	Source    string // "bundled" or "package"
	Error     string // non-empty on failure
	Dir       string // filesystem path
}

// EventLog is a thread-safe ring buffer of system loading events.
type EventLog struct {
	mu     sync.RWMutex
	events []LoadEvent
	cap    int
	idx    int  // next write index
	full   bool // whether the buffer has wrapped
}

// NewEventLog creates an event log with the given capacity.
func NewEventLog(capacity int) *EventLog {
	return &EventLog{
		events: make([]LoadEvent, capacity),
		cap:    capacity,
	}
}

// Record adds an event to the log. Thread-safe.
func (l *EventLog) Record(event LoadEvent) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	l.mu.Lock()
	l.events[l.idx] = event
	l.idx++
	if l.idx >= l.cap {
		l.idx = 0
		l.full = true
	}
	l.mu.Unlock()
}

// Events returns all recorded events in chronological order. Thread-safe.
func (l *EventLog) Events() []LoadEvent {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if !l.full {
		result := make([]LoadEvent, l.idx)
		copy(result, l.events[:l.idx])
		return result
	}

	// Ring has wrapped — read from idx..cap, then 0..idx.
	result := make([]LoadEvent, l.cap)
	copy(result, l.events[l.idx:])
	copy(result[l.cap-l.idx:], l.events[:l.idx])
	return result
}

// globalEventLog captures system loading events for admin diagnostics.
var globalEventLog *EventLog

// RecordEvent adds a system loading event to the global event log.
func RecordEvent(event LoadEvent) {
	if globalEventLog != nil {
		globalEventLog.Record(event)
	}
}

// DiagnosticEvents returns all recorded system loading events.
func DiagnosticEvents() []LoadEvent {
	if globalEventLog == nil {
		return nil
	}
	return globalEventLog.Events()
}
