package layouts

import (
	"context"
	"sync/atomic"

	"github.com/a-h/templ"
)

// ManageFigureFunc builds a Manage page's own figures ("5 members · 1
// invited") for the frame to draw beside the page's title. The campaigns
// plugin registers it at wiring time: layouts cannot import it, and the
// figures must come from the same provider data the desk reads, so the page
// opened by address and the page reached from the desk show the same ones. It
// returns nil when there is nothing true to show (a viewer who is not the
// owner, a provider that failed), which draws nothing at all.
type ManageFigureFunc func(ctx context.Context, line string) templ.Component

var manageFigure atomic.Pointer[ManageFigureFunc]

// SetManageFigure registers the resolver the frame draws figures with.
func SetManageFigure(f ManageFigureFunc) {
	if f == nil {
		manageFigure.Store(nil)
		return
	}
	manageFigure.Store(&f)
}

// manageFigureFor returns the figures for a page's desk line, or nil.
func manageFigureFor(ctx context.Context, line string) templ.Component {
	if line == "" {
		return nil
	}
	f := manageFigure.Load()
	if f == nil {
		return nil
	}
	return (*f)(ctx, line)
}
