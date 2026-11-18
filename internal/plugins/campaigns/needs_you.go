package campaigns

// NeedsYouItem is one thing waiting on the owner, drawn as a row of the
// desk's "Needs you" sheet: a bold lead, the rest of the sentence, and the
// quiet word that goes to it.
type NeedsYouItem struct {
	Lead   string // the words that say what, in bold
	Rest   string // the rest of the sentence
	Action string // the quiet word's text
	URL    string // where it goes
	// State is the dot before the row: "warn" or "" for a plain dot. Amber
	// here means state only; the Director's amber edge is never used.
	State string
}
