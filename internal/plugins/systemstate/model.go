package systemstate

import (
	"encoding/json"
	"time"
)

// State is one stored document. Both halves are always valid JSON objects: a
// missing row reads as two empty objects, so callers never branch on absence.
type State struct {
	// GM is the DM-team half. The service returns it unfiltered; deciding who
	// may see it is the caller's job (handlers and the sync adapter).
	GM json.RawMessage
	// Public is the half any viewer of the page may read.
	Public json.RawMessage
	// UpdatedAt is nil when nothing has been stored yet.
	UpdatedAt *time.Time
}

// emptyObject is what an absent half reads as.
var emptyObject = json.RawMessage(`{}`)

// emptyState is the read result for a page that has no row.
func emptyState() State {
	return State{GM: emptyObject, Public: emptyObject}
}
