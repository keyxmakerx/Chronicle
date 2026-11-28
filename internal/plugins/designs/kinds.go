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

// Stripper removes every Director-only part of a payload. It runs on every
// share path and again on every add, never trusting the sender: a payload that
// arrives already stripped is not evidence that the sender stripped it.
type Stripper interface {
	Strip(raw json.RawMessage) (json.RawMessage, error)
}

// Adder makes the added thing in the receiving campaign itself, instead of the
// default copy into the design library. It receives the stripped, validated
// payload.
type Adder interface {
	Add(ctx context.Context, campaignID, userID string, raw json.RawMessage) (AddResult, error)
}

// AddResult is what an Adder made: its id, and plain sentences on what the
// Owner still has to point at something (a shop, a bench) in this campaign.
type AddResult struct {
	ID          string   `json:"id"`
	LeftToPoint []string `json:"leftToPoint"`
}

// Describer lets a kind say how the Store shows it, so the Store learns its
// rail from the server instead of a list baked into the page. Kinds that share
// a Group share one rail entry (the first by Order, then kind name, names it).
type Describer interface {
	Describe() KindDescription
}

// KindDescription is one kind's place in the Store.
type KindDescription struct {
	// Group is the Store's id for the rail entry and the gallery's kind filter.
	Group string
	// Label names the group ("Shop rooms"); One names a single thing in it
	// ("Shop room"); Where says in a sentence where an added one is found.
	Label, One, Where string
	// Order sorts groups on the rail, lowest first; ties fall back to Group.
	Order int
}

// stripKind runs the kind's Stripper when it has one.
func stripKind(k Kind, raw json.RawMessage) (json.RawMessage, error) {
	if st, ok := k.(Stripper); ok {
		return st.Strip(raw)
	}
	return raw, nil
}

// described is a registered kind that said how the Store shows it.
type described struct {
	kind string
	KindDescription
}

// describedKinds lists the registered kinds that have a Describer, in kind
// name order so every caller sees the same sequence.
func describedKinds(reg KindRegistry) []described {
	var out []described
	for _, n := range reg.Names() {
		k, ok := reg.Kind(n)
		if !ok {
			continue
		}
		if d, ok := k.(Describer); ok {
			out = append(out, described{kind: n, KindDescription: d.Describe()})
		}
	}
	return out
}

// kindsInGroup is the registered kinds a Store group (the gallery's kind
// filter) stands for. This is the one place that mapping lives.
func kindsInGroup(reg KindRegistry, group string) []string {
	var out []string
	for _, d := range describedKinds(reg) {
		if d.Group == group {
			out = append(out, d.kind)
		}
	}
	return out
}

// storeGroups is one entry per group, in Order then Group.
func storeGroups(reg KindRegistry) []StoreKind {
	type ranked struct {
		StoreKind
		order int
	}
	seen := map[string]bool{}
	var groups []ranked
	for _, d := range describedKinds(reg) {
		if d.Group == "" || seen[d.Group] {
			continue
		}
		seen[d.Group] = true
		groups = append(groups, ranked{StoreKind{ID: d.Group, Label: d.Label, One: d.One, Where: d.Where}, d.Order})
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].order != groups[j].order {
			return groups[i].order < groups[j].order
		}
		return groups[i].ID < groups[j].ID
	})
	out := make([]StoreKind, len(groups))
	for i, g := range groups {
		out[i] = g.StoreKind
	}
	return out
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
