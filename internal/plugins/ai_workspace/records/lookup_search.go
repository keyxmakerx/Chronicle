package records

import (
	"context"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// PageSearchAPI is the page search the Search screen runs, with the quoted
// line under each hit (entities.EntityService). It is its own interface so a
// server or a test that can list pages but not search them still builds; the
// search lookup then says it can't be used.
//
// Both calls take the viewer's role and user id and apply the same page
// visibility as the screen, so a search never finds a page the Actor could not
// open, nor quotes text they could not read there.
type PageSearchAPI interface {
	Search(ctx context.Context, campaignID, query string, typeID int, role int, userID string, opts entities.ListOptions) ([]entities.Entity, int, error)
	SearchSnippets(ctx context.Context, campaignID, query string, ids []string, role int, userID string) map[string][]entities.SnippetPart
}

// searchPerPiece is how many hits one piece of a search carries: enough to
// find a page, few enough that every hit keeps its quoted line.
const searchPerPiece = 25

// search answers `what: search`: the visible pages whose name, other names,
// kind or text hold the words, each with a short quote. `piece` steps to the
// next hits.
func (run *lookupRun) search(query, typeName string) (LookupAnswer, error) {
	ans := LookupAnswer{Chip: "Search"}
	s, ok := run.l.Pages.(PageSearchAPI)
	if !ok || run.l.Pages == nil {
		return ans, badRequestf("search can't be used on this server")
	}
	query = strings.TrimSpace(query)
	if len([]rune(query)) < 2 {
		return ans, badRequestf("it needs `query`: at least two characters to look for")
	}
	ans.Chip = "Search · " + oneLine(query, 40)
	typeID, err := run.searchType(typeName)
	if err != nil {
		return ans, err
	}
	piece := max(run.piece, 1)
	hits, total, err := s.Search(run.ctx, run.campaignID, query, typeID, run.a.Role, run.a.UserID, entities.ListOptions{Page: piece, PerPage: searchPerPiece})
	if err != nil {
		return ans, err
	}
	run.linesPaged = true
	pieces := (total + searchPerPiece - 1) / searchPerPiece
	if len(hits) == 0 {
		if piece > 1 && total > 0 {
			return ans, badRequestf("there is no piece %d: this search has %d (%s)", piece, pieces, plural(pieces, "piece", "pieces"))
		}
		ans.Text = fmt.Sprintf("## Search: %s\n\nNo page you can see matches.\n", quote(query))
		return ans, nil
	}
	// The service filters by the Actor's role and user id; the campaign check
	// is the one thing it leaves to the caller.
	ids := make([]string, 0, len(hits))
	for _, h := range hits {
		if h.CampaignID == run.campaignID {
			ids = append(ids, h.ID)
		}
	}
	quotes := s.SearchSnippets(run.ctx, run.campaignID, query, ids, run.a.Role, run.a.UserID)
	// A Director's hits carry the page index's marks, so a GM-only page found
	// by its text is not repeated to players; nobody else reads a marked page.
	director := run.director()
	var lines []string
	marked := false
	for _, h := range hits {
		if h.CampaignID != run.campaignID {
			continue
		}
		line := "- " + h.Name
		if kind := firstNonEmpty(h.TypeName, h.TypeSlug); kind != "" {
			line += " (" + kind + ")"
		}
		if director {
			if m := searchSightMark(&h); m != "" {
				line, marked = line+m, true
			}
		}
		if q := snippetLine(quotes[h.ID]); q != "" {
			line += ": " + q
		}
		lines = append(lines, line)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Search: %s (%d)\n\n", quote(query), total)
	if marked {
		b.WriteString(sightLegend + "\n")
	}
	if pieces > 1 {
		fmt.Fprintf(&b, "Hits %d to %d of %d (piece %d of %d).\n\n", (piece-1)*searchPerPiece+1, (piece-1)*searchPerPiece+len(hits), total, piece, pieces)
	}
	for _, l := range lines {
		b.WriteString(l + "\n")
	}
	if piece < pieces {
		fmt.Fprintf(&b, "\n…%d more; ask again with `piece: %d` for the next, or narrow it with `type`.\n", total-piece*searchPerPiece, piece+1)
	}
	b.WriteString("\nRead any hit in full with read_page.\n")
	ans.Text = b.String()
	return ans, nil
}

// searchSightMark is a search hit's mark. Hits are not grouped by kind, so a
// page of a DM-only category carries the mark on its own line.
func searchSightMark(e *entities.Entity) string {
	if m := kindSightMark(e); m != "" {
		return m
	}
	return pageSightMark(e)
}

// searchType is the id of the page kind named, or 0 for every kind.
func (run *lookupRun) searchType(name string) (int, error) {
	if strings.TrimSpace(name) == "" {
		return 0, nil
	}
	types, err := run.l.Pages.GetEntityTypes(run.ctx, run.campaignID)
	if err != nil {
		return 0, err
	}
	for _, t := range types {
		if sameName(t.Name, name) || sameName(t.NamePlural, name) || sameName(t.Slug, name) {
			return t.ID, nil
		}
	}
	return 0, badRequestf("no kind of page called %s", quote(name))
}

// snippetLine is a search quote on one line, the matched words in bold. The
// quote is the page's own writing, so it is flattened and shortened like any
// other text the AI is shown.
func snippetLine(parts []entities.SnippetPart) string {
	var b strings.Builder
	for _, p := range parts {
		if p.Mark {
			if t := strings.TrimSpace(p.Text); t != "" {
				b.WriteString("**" + t + "**")
			}
			continue
		}
		b.WriteString(p.Text)
	}
	return oneLine(b.String(), 300)
}
