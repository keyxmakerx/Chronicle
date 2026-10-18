// Package rumours is the Rumours page block: what the party has heard about a
// page, who said it, how sure they were, what the Director knows is true and
// what the party has decided to believe.
//
// The two judgements are kept apart on purpose. The Director's truth is theirs
// alone and never leaves the service for anyone else; the party's belief is
// set by a Scribe or the Director and shown to everyone once marked. A Scribe
// can add rumours and mark what the party thinks but can never change the
// truth. The service decides what each viewer is told and who may write; the
// handler, the block and the script add no rule of their own.
package rumours
