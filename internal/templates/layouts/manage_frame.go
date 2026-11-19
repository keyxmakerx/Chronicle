package layouts

import (
	"context"
	"sync/atomic"

	"github.com/a-h/templ"
)

// ManageSentenceFunc words a Manage page's own desk sentence ("4 people: you,
// 1 scribe and 2 players.") for the frame to draw under the page's title. The
// campaigns plugin registers it at wiring time: layouts cannot import it, and
// the sentence must come from the same provider data the desk reads, so the
// page opened by address and the page reached from the desk start the same
// way. It returns nil when there is nothing true to say (a viewer who is not
// the owner, a provider that failed), which draws no sentence at all.
type ManageSentenceFunc func(ctx context.Context, line string) templ.Component

var manageSentence atomic.Pointer[ManageSentenceFunc]

// SetManageSentence registers the resolver the frame draws sentences with.
func SetManageSentence(f ManageSentenceFunc) {
	if f == nil {
		manageSentence.Store(nil)
		return
	}
	manageSentence.Store(&f)
}

// manageSentenceFor returns the sentence for a page's desk line, or nil.
func manageSentenceFor(ctx context.Context, line string) templ.Component {
	if line == "" {
		return nil
	}
	f := manageSentence.Load()
	if f == nil {
		return nil
	}
	return (*f)(ctx, line)
}
