// Package changesource records, on the request context, which door an entity
// write came through (the web UI, the Foundry module, a stash move, a shop, an
// extension). Observers of a write read it to describe the change to people;
// it never decides who may write, so nothing here is a permission check.
package changesource

import "context"

// Kinds of write origin. The strings are stable: observers branch on them.
const (
	KindWeb       = "web"
	KindFoundry   = "foundry"
	KindStash     = "stash"
	KindShop      = "shop"
	KindExtension = "extension"
)

// Source says where a write came from.
type Source struct {
	// Kind is one of the Kind constants.
	Kind string
	// Label is an optional human-readable origin, such as a shop's name.
	Label string
	// UserID is the account the write ran as; empty when none applies.
	UserID string
}

type ctxKey struct{}

// With returns ctx carrying s. A nested call replaces the outer source, so the
// innermost door wins (a stash move inside a web request is a stash write).
func With(ctx context.Context, s Source) context.Context {
	return context.WithValue(ctx, ctxKey{}, s)
}

// From returns the source on ctx, or false when no door set one. Callers must
// treat "absent" as "unknown", never as a default kind.
func From(ctx context.Context) (Source, bool) {
	s, ok := ctx.Value(ctxKey{}).(Source)
	return s, ok
}
