package ai_workspace

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
)

// reviewProblem is a block the review can't offer at all: it sits in the
// problems box above the list and is never saved.
type reviewProblem struct {
	Block int
	Say   string
}

// reviewProblems lists the unusable blocks in paste order: pages whose
// markdown would not parse or whose sheet fields don't fit, and records the
// server refused.
func reviewProblems(d ImportReviewData) []reviewProblem {
	var out []reviewProblem
	for i, p := range d.Pages {
		switch {
		case p.Status == importer.StatusParseError:
			out = append(out, reviewProblem{Block: p.SourceIndex + 1, Say: p.ParseError})
		case d.Classes[i].Status == importer.StatusFieldProblem:
			out = append(out, reviewProblem{Block: p.SourceIndex + 1, Say: repeatSay(d, p.SourceIndex) + p.Name + ": " + d.Classes[i].Reason})
		}
	}
	for _, r := range d.Records {
		if r.Plan.Error != "" {
			say := r.Plan.Error
			if r.title() != "" {
				say = r.title() + ": " + say
			}
			out = append(out, reviewProblem{Block: r.Block, Say: repeatSay(d, r.Block-1) + say})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Block < out[j].Block })
	return out
}

// usablePages are the indexes of the pages offered in the list.
func usablePages(d ImportReviewData) []int {
	var out []int
	for i, p := range d.Pages {
		if p.Status != importer.StatusParseError && d.Classes[i].Status != importer.StatusFieldProblem {
			out = append(out, i)
		}
	}
	return out
}

// recordGroup is one heading of records in the list, by kind.
type recordGroup struct {
	Label string
	Rows  []RecordRow
}

// recordGroups groups the usable records by kind, in the order each kind
// first appears in the paste. Form fields keep each record's own index, so
// the order on screen never changes what is saved.
func recordGroups(d ImportReviewData) []recordGroup {
	var out []recordGroup
	at := map[string]int{}
	for _, r := range d.Records {
		if r.Plan.Error != "" {
			continue
		}
		i, ok := at[r.Label]
		if !ok {
			i = len(out)
			at[r.Label] = i
			out = append(out, recordGroup{Label: r.Label})
		}
		out[i].Rows = append(out[i].Rows, r)
	}
	return out
}

// pageStartsYes is a page row's first answer. Removals, and rows the
// classifier already knows can't go through as written, start on No.
func pageStartsYes(page importer.ParsedPage, cls importer.Classification) bool {
	if page.FrontMatter.Action == importer.ActionDelete {
		return false
	}
	return cls.Status != importer.StatusParentProblem && cls.Status != importer.StatusActionMismatch
}

// recordStartsYes is a record row's first answer: removals, and blocks
// applied before, start on No.
func recordStartsYes(r RecordRow) bool { return r.Action != records.ActionDelete && r.Repeat == "" }

// listedCount is how many rows the list offers.
func listedCount(d ImportReviewData) int {
	n := len(usablePages(d))
	for _, g := range recordGroups(d) {
		n += len(g.Rows)
	}
	return n
}

// startingYesCount is how many rows start on Yes, for the save bar's first
// render; the page script keeps it current as answers change.
func startingYesCount(d ImportReviewData) int {
	n := 0
	for _, i := range usablePages(d) {
		if pageStartsYes(d.Pages[i], d.Classes[i]) && !d.Classes[i].GMOnly.Any() && d.Repeats[d.Pages[i].SourceIndex] == "" {
			n++
		}
	}
	for _, g := range recordGroups(d) {
		for _, r := range g.Rows {
			if recordStartsYes(r) {
				n++
			}
		}
	}
	return n
}

// pageVerb is the row's pill word for a page.
func pageVerb(action string) string {
	switch action {
	case importer.ActionUpdate:
		return "Change"
	case importer.ActionDelete:
		return "Remove"
	}
	return "Add"
}

// verbClass is the pill's colour class for a verb.
func verbClass(verb string) string {
	switch verb {
	case "Change", "Generate":
		return "air-pill is-change"
	case "Remove":
		return "air-pill is-remove"
	}
	return "air-pill is-add"
}

// pageWhatLead is the words before the page's name on its row.
func pageWhatLead(page importer.ParsedPage) string {
	if page.FrontMatter.IsEra() {
		if page.FrontMatter.Action == importer.ActionDelete {
			return fmt.Sprintf("Remove the %s version of", page.FrontMatter.EraName())
		}
		return fmt.Sprintf("Write the %s version of", page.FrontMatter.EraName())
	}
	switch page.FrontMatter.Action {
	case importer.ActionUpdate:
		return "Change the page"
	case importer.ActionDelete:
		return "Remove the page"
	}
	return "Add the page"
}

// renamesOnConflict reports whether a row that names an existing page makes
// a second page beside it. Only an add does; a change or a removal works on
// the existing page, and an era row writes a version of it.
func renamesOnConflict(page importer.ParsedPage) bool {
	return !page.FrontMatter.IsEra() && page.FrontMatter.Action == importer.ActionCreate
}

// pageDetail is the plain line under a page row: where it goes and who sees it.
func pageDetail(page importer.ParsedPage, cls importer.Classification) string {
	if page.FrontMatter.IsEra() || page.FrontMatter.Action == importer.ActionDelete {
		return ""
	}
	where := ""
	switch {
	case cls.IsNewCategory:
		where = fmt.Sprintf("In a new category, %s", cls.ProposedTypeSlug)
	case cls.ExistingType != nil:
		where = "In " + cls.ExistingType.Name
	}
	if page.FrontMatter.Parent != "" && page.FrontMatter.Parent != importer.ParentNone {
		if where == "" {
			where = "Under " + page.FrontMatter.Parent
		} else {
			where += ", under " + page.FrontMatter.Parent
		}
	}
	vis := ""
	switch page.FrontMatter.Visibility {
	case "dm_only":
		vis = "only Directors see it"
	case "private":
		vis = "private"
	case "public":
		vis = "public"
	}
	switch {
	case where != "" && vis != "":
		return where + ", " + vis
	case vis != "":
		return capitalize(vis)
	}
	return where
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// repeatWarning is the heading for a paste applied before: all of it, or
// how many of its blocks. "" when nothing in it was.
func repeatWarning(d ImportReviewData) string {
	n := len(d.Repeats)
	// The latest date, compared as dates: "9 March" sorts after "10 May" as text.
	when, latest := "", time.Time{}
	for _, w := range d.Repeats {
		if t, err := time.Parse("2 January 2006", w); err == nil && (when == "" || t.After(latest)) {
			when, latest = w, t
		}
	}
	switch {
	case n == 0:
		return ""
	case n == len(d.Pages)+len(d.Records):
		return "You already applied this paste, on " + when + "."
	case n == 1:
		return "1 block in this paste was already applied."
	}
	return fmt.Sprintf("%d blocks in this paste were already applied.", n)
}

// repeatSay leads a problem with when its block was applied before, which is
// usually why it is refused now (the thing it makes already exists).
func repeatSay(d ImportReviewData, src int) string {
	if w := d.Repeats[src]; w != "" {
		return "Already applied on " + w + ". "
	}
	return ""
}
