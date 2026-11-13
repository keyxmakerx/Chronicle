package designs

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
)

// Kind is the owning feature's say over one kind of design. The feature is the
// authority: this package never looks inside Data.
type Kind interface {
	// Validate returns the canonical form of raw, or an error that names the
	// part it refuses. It runs on save, on import and on every take, so a
	// design made before a part was retired is refused rather than stored.
	Validate(raw json.RawMessage) (json.RawMessage, error)
	// Lacks says in plain sentences what the receiving campaign does not have
	// that this design uses (a material, a shape). It may return none.
	Lacks(ctx context.Context, campaignID string, raw json.RawMessage) ([]string, error)
}

// KindRegistry is what the service reads kinds from. App wiring registers a
// kind per feature, so features never import each other and a feature that
// starts later (crafting) can still add its kinds.
type KindRegistry interface {
	Kind(name string) (Kind, bool)
	Names() []string
}

// Kinds is the usual KindRegistry; Register may be called at any time.
type Kinds struct {
	mu sync.RWMutex
	m  map[string]Kind
}

// NewKinds returns an empty registry.
func NewKinds() *Kinds { return &Kinds{m: map[string]Kind{}} }

// Register adds or replaces a kind.
func (k *Kinds) Register(name string, kind Kind) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.m[name] = kind
}

// Kind returns the registered kind, if any.
func (k *Kinds) Kind(name string) (Kind, bool) {
	k.mu.RLock()
	defer k.mu.RUnlock()
	kind, ok := k.m[name]
	return kind, ok
}

// Names lists the registered kinds in order.
func (k *Kinds) Names() []string {
	k.mu.RLock()
	defer k.mu.RUnlock()
	out := make([]string, 0, len(k.m))
	for n := range k.m {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// KindRoomTheme is the shop counter room's theme; the armory validates it.
const KindRoomTheme = "room-theme"
