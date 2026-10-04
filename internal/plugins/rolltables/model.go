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
	Entries []Entry `json:"entries"`
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
