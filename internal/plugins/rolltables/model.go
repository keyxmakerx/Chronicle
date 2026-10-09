package rolltables

// Document is a campaign's whole set of rolling tables. It is both the wire
// shape and the stored shape, so what a client reads is what the service
// normalized on the way in.
type Document struct {
	// Tables is never nil: an empty set marshals as [] so clients can iterate
	// without a null check.
	Tables []Table `json:"tables"`
}

// Table is one rolling table.
type Table struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Credit  *Credit `json:"credit,omitempty"`
	Entries []Entry `json:"entries"`
}

// Credit says where a table copied from an openly licensed source came from.
// A copy of a library table carries it, so the licence's attribution travels
// with the campaign's own version.
type Credit struct {
	Source     string `json:"source"`
	Author     string `json:"author,omitempty"`
	Page       string `json:"page,omitempty"`
	URL        string `json:"url,omitempty"`
	Licence    string `json:"licence,omitempty"`
	LicenceURL string `json:"licenceUrl,omitempty"`
	Notice     string `json:"notice,omitempty"`
}

// Entry is one row of a table; Weight is its relative chance of being rolled.
type Entry struct {
	Name   string  `json:"name"`
	Brief  string  `json:"brief"`
	Weight float64 `json:"weight"`
}

// emptyDocument is the read result for a campaign with no row.
func emptyDocument() Document {
	return Document{Tables: []Table{}}
}
