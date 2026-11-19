package campaigns

// NeedsYouItem is one thing waiting on the owner, drawn like any other row of
// the page under "Needs you": its name, what is wrong in grey, and the whole
// row leading to it.
type NeedsYouItem struct {
	Lead string // the row's name
	Rest string // what is wrong, in a few words
	URL  string // where the row goes
	// State is the dot before the row: "warn" or "" for a plain dot. Amber
	// here means state only; the Director's amber edge is never used.
	State string
}
