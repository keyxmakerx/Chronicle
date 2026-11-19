package entities

import "context"

// startWritingParam marks an address that means "write this now". The editor
// drops it from the address once it has opened, so a reload or a copied
// link opens the page at rest like any other.
const startWritingParam = "write"

// newPageURL is where a page made a moment ago opens: the page itself, asked
// to open its text ready to type. Nobody makes a page to leave it empty, and
// at rest the way in is a small Edit word.
func newPageURL(campaignID, entityID string) string {
	return "/campaigns/" + campaignID + "/entities/" + entityID + "?" + startWritingParam + "=1"
}

type startWritingKey struct{}

// withStartWriting carries the request's "write this now" to the page's
// text block.
func withStartWriting(ctx context.Context) context.Context {
	return context.WithValue(ctx, startWritingKey{}, true)
}

// startsWriting reports whether the page's text opens straight into editing.
func startsWriting(ctx context.Context) bool {
	on, _ := ctx.Value(startWritingKey{}).(bool)
	return on
}
