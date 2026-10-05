package records

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// RollTablesAPI is the rolltables service, as its JSON document (adapted in
// app wiring so this package does not import the plugin). The campaign's
// tables are one document, so every change is read, edit, write the whole
// set, and the service validates the set it is given.
type RollTablesAPI interface {
	Get(ctx context.Context, campaignID string) (TableDoc, error)
	Put(ctx context.Context, campaignID string, doc TableDoc, userID string) error
}

// TableDoc mirrors the rolltables document's JSON.
type TableDoc struct {
	Tables []Table `json:"tables"`
}

// Table is one rolling table.
type Table struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Entries []Entry `json:"entries"`
}

// Entry is one row of a table. Weight is always sent: the service keeps an
// explicit 0 and the roller never picks a 0-weight entry, so new entries
// get 1 unless the record gives a weight.
type Entry struct {
	Name   string  `json:"name"`
	Brief  string  `json:"brief"`
	Weight float64 `json:"weight"`
}

// TableKind adds, replaces and removes rolling tables, matched by name.
type TableKind struct{ Svc RollTablesAPI }

func (TableKind) Name() string  { return "table" }
func (TableKind) Label() string { return "Rolling table" }
func (TableKind) Doc() string {
	return "A rolling table, matched by `name`. Its entries are a markdown list in the body (one entry per `-` line; `Name: short note` adds a note), or `entries:` in the front matter. An update replaces all entries; `rename_to` renames it.\n\n```\n---\nkind: table\nname: Tavern names\n---\n- The Rusty Anchor\n- The Drowned Rat: a smugglers' haunt\n```"
}

var bulletRe = regexp.MustCompile(`^\s*(?:[-*+]|\d+[.)])\s+(.+)$`)

// entries reads the record's entries from front matter or body bullets.
func tableEntries(r Record) []Entry {
	var out []Entry
	for _, it := range r.List("entries") {
		switch t := it.(type) {
		case map[string]any:
			e := Entry{Name: strings.TrimSpace(fmt.Sprint(t["name"])), Weight: 1}
			if b, ok := t["brief"]; ok && b != nil {
				e.Brief = strings.TrimSpace(fmt.Sprint(b))
			}
			if w, ok := t["weight"].(int); ok {
				e.Weight = float64(w)
			} else if w, ok := t["weight"].(float64); ok {
				e.Weight = w
			}
			out = append(out, e)
		case nil:
		default:
			out = append(out, Entry{Name: strings.TrimSpace(fmt.Sprint(t)), Weight: 1})
		}
	}
	if len(out) > 0 {
		return out
	}
	for _, line := range strings.Split(r.Body, "\n") {
		m := bulletRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name, brief, _ := strings.Cut(m[1], ": ")
		out = append(out, Entry{Name: strings.TrimSpace(name), Brief: strings.TrimSpace(brief), Weight: 1})
	}
	return out
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// tableID makes the stable id rolltables requires from a name.
func tableID(name string, taken map[string]bool) string {
	id := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if id == "" || id[0] < 'a' || id[0] > 'z' {
		id = "t-" + id
	}
	if len(id) > 56 {
		id = strings.TrimRight(id[:56], "-")
	}
	base := id
	for i := 2; taken[id]; i++ {
		id = fmt.Sprintf("%s-%d", base, i)
	}
	return id
}

func findTable(doc TableDoc, name string) int {
	for i, t := range doc.Tables {
		if sameName(t.Name, name) {
			return i
		}
	}
	return -1
}

func (k TableKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	if !a.CanAuthorDmOnly() {
		return Plan{Error: "only the owner or a co-DM can change rolling tables"}
	}
	if r.Name == "" {
		return Plan{Error: "a table needs a name"}
	}
	doc, err := k.Svc.Get(ctx, campaignID)
	if err != nil {
		return Plan{Error: "could not read the rolling tables"}
	}
	i := findTable(doc, r.Name)
	n := len(tableEntries(r))
	switch r.Action {
	case ActionCreate:
		if i >= 0 {
			return Plan{Error: "a table called " + quote(r.Name) + " already exists; use action: update"}
		}
		if n == 0 {
			return Plan{Error: "the table has no entries (one `-` line per entry)"}
		}
		return Plan{Summary: fmt.Sprintf("%d entries", n)}
	case ActionUpdate:
		if i < 0 {
			return Plan{Error: "no table called " + quote(r.Name) + " to change"}
		}
		if n == 0 && r.Str("rename_to") == "" {
			return Plan{Error: "nothing to change: no entries and no rename_to"}
		}
		if n == 0 {
			return Plan{Summary: "renames it to " + quote(r.Str("rename_to"))}
		}
		return Plan{Summary: fmt.Sprintf("%d entries (was %d)", n, len(doc.Tables[i].Entries))}
	default:
		if i < 0 {
			return Plan{Error: "no table called " + quote(r.Name) + " to remove"}
		}
		return Plan{Summary: fmt.Sprintf("removes the table and its %d entries", len(doc.Tables[i].Entries))}
	}
}

func (k TableKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	if p := k.Plan(ctx, campaignID, a, r); p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	doc, err := k.Svc.Get(ctx, campaignID)
	if err != nil {
		return err
	}
	i := findTable(doc, r.Name)
	if r.Action != ActionCreate && i < 0 {
		return apperror.NewBadRequest("it changed while you were reviewing; check it again")
	}
	switch r.Action {
	case ActionCreate:
		taken := map[string]bool{}
		for _, t := range doc.Tables {
			taken[t.ID] = true
		}
		doc.Tables = append(doc.Tables, Table{ID: tableID(r.Name, taken), Name: r.Name, Entries: tableEntries(r)})
	case ActionUpdate:
		if es := tableEntries(r); len(es) > 0 {
			doc.Tables[i].Entries = es
		}
		if s := r.Str("rename_to"); s != "" {
			doc.Tables[i].Name = s
		}
	default:
		doc.Tables = append(doc.Tables[:i], doc.Tables[i+1:]...)
	}
	return k.put(ctx, campaignID, a, doc)
}

// put writes the set back; the service's own validation is the gate.
func (k TableKind) put(ctx context.Context, campaignID string, a Actor, doc TableDoc) error {
	return k.Svc.Put(ctx, campaignID, doc, a.UserID)
}

// appendEntries adds entries to a table, creating it when missing. Used by
// the names generator.
func (k TableKind) appendEntries(ctx context.Context, campaignID string, a Actor, name string, es []Entry) error {
	doc, err := k.Svc.Get(ctx, campaignID)
	if err != nil {
		return err
	}
	if i := findTable(doc, name); i >= 0 {
		doc.Tables[i].Entries = append(doc.Tables[i].Entries, es...)
	} else {
		taken := map[string]bool{}
		for _, t := range doc.Tables {
			taken[t.ID] = true
		}
		doc.Tables = append(doc.Tables, Table{ID: tableID(name, taken), Name: name, Entries: es})
	}
	return k.put(ctx, campaignID, a, doc)
}

func (k TableKind) Export(ctx context.Context, campaignID string, a Actor) (string, error) {
	if !a.CanAuthorDmOnly() {
		return "", nil
	}
	doc, err := k.Svc.Get(ctx, campaignID)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, t := range doc.Tables {
		b.WriteString("### " + t.Name + "\n\n")
		for _, e := range t.Entries {
			b.WriteString("- " + e.Name)
			if e.Brief != "" {
				b.WriteString(": " + e.Brief)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}
