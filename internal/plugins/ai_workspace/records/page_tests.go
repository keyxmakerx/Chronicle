package records

import (
	"context"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// TestWriteAPI is the Tests block's setup as AI Import writes it, adapted
// in app wiring so this package does not import the plugin. The service
// applies its own DM rule, page access and limits.
type TestWriteAPI interface {
	// OnPage is every test on a page, with its setup.
	OnPage(ctx context.Context, campaignID, pageID string, a Actor) ([]TestSetup, error)
	Create(ctx context.Context, campaignID, pageID string, a Actor, in TestSetup) error
	// Update replaces a test's setup; its tally and recorded result stay.
	Update(ctx context.Context, campaignID, id string, a Actor, in TestSetup) error
	Delete(ctx context.Context, campaignID, id string, a Actor) error
	// HasBlock says whether the page's category layout shows the Tests
	// block, so the review can say a test would not be seen on the page.
	HasBlock(ctx context.Context, campaignID, pageID string) (bool, error)
	// Pages lists the pages that have tests, for the export.
	Pages(ctx context.Context, campaignID string, a Actor) ([]entities.Entity, error)
}

// TestSetup is one test's setup. Successes and Failures are the totals that
// end it, 0 for none.
type TestSetup struct {
	ID, Name, Type      string
	Shown               bool
	Successes, Failures int
	Results             []TestSetupResult
	Version             int
}

// TestSetupResult is one way a test can end; On is "successes" or
// "failures" when that total filling offers it, "" when only picked by hand.
type TestSetupResult struct{ ID, Label, On string }

// The Tests block's own limits, repeated so a review refuses what the
// service would refuse at commit.
const (
	testMaxResults = 8
	testMaxTotal   = 30
)

// defaultTestResults is what a new test starts with when the block gives
// none, the same three the page's block offers.
var defaultTestResults = []TestSetupResult{{Label: "Success"}, {Label: "Partial success"}, {Label: "Failure"}}

// TestKind writes the tests (montages, chases, negotiations) on a page,
// matched by name within that page.
type TestKind struct {
	Svc      TestWriteAPI
	Entities EntityLookup
}

func (TestKind) Name() string  { return "test" }
func (TestKind) Label() string { return "Test" }
func (TestKind) Doc() string {
	return "A test on a page (a montage, a chase, a negotiation; owner or co-DM only), matched by `name` within its `page`. Keys: `type` (free words such as Montage), `successes` (the successes that end it, 0 to 30) and `failures` (the failures that end it), `players_see` (true shows players its name and the result once recorded; hidden otherwise), `rename_to`, and `results` (at most 8, in order): each a result's name, with `, on successes` or `, on failures` when that total filling offers it. A new test without `results` gets Success, Partial success and Failure. On update a key you leave out keeps what is there, and `results` replaces the list; a result keeps its place in encounter versions while its name stays the same. The tally and the recorded result are the table's, never written here. A test shows on its page only when the page's layout has a Tests block; an encounter version can name it either way.\n\n" +
		"```\n---\nkind: test\npage: The Sandstorm\nname: Cross the sandstorm\ntype: Montage\nsuccesses: 5\nfailures: 3\nresults:\n  - Total success, on successes\n  - Partial success\n  - Total failure, on failures\n---\n```"
}

// Title names the row: "Cross the sandstorm on The Sandstorm".
func (TestKind) Title(r Record) string {
	if r.Name == "" || r.Str("page") == "" {
		return ""
	}
	return r.Name + " on " + r.Str("page")
}

type testDraft struct {
	page     *entities.Entity
	existing *TestSetup
	setup    TestSetup
}

func (k TestKind) read(ctx context.Context, campaignID string, a Actor, r Record) (testDraft, error) {
	var d testDraft
	if !a.CanAuthorDmOnly() {
		return d, apperror.NewBadRequest("only the owner or a co-DM can set up tests")
	}
	if k.Svc == nil {
		return d, apperror.NewBadRequest("tests aren't available on this server")
	}
	if r.Name == "" {
		return d, apperror.NewBadRequest("a test needs a name")
	}
	pageName := r.Str("page")
	if pageName == "" {
		return d, apperror.NewBadRequest("a test needs `page`: the page it is on")
	}
	if k.Entities != nil {
		if e, err := k.Entities.GetBySlug(ctx, campaignID, entities.Slugify(pageName)); err == nil && e != nil && e.CampaignID == campaignID {
			d.page = e
		}
	}
	if d.page != nil {
		list, err := k.Svc.OnPage(ctx, campaignID, d.page.ID, a)
		if err != nil {
			return d, apperror.NewBadRequest("could not read the tests on " + d.page.Name)
		}
		for i := range list {
			if sameName(list[i].Name, r.Name) {
				d.existing = &list[i]
				break
			}
		}
	}
	if r.Action == ActionDelete {
		return d, nil
	}
	s := TestSetup{Name: r.Name, Results: defaultTestResults}
	if d.existing != nil {
		s = *d.existing
	}
	if v := r.Str("rename_to"); v != "" {
		s.Name = v
	}
	if r.Has("type") {
		s.Type = r.Str("type")
	}
	if r.Has("players_see") {
		s.Shown, _ = r.Bool("players_see")
	}
	for key, dst := range map[string]*int{"successes": &s.Successes, "failures": &s.Failures} {
		n, ok, err := r.Int(key)
		if err != nil {
			return d, err
		}
		if !ok {
			continue
		}
		if n < 0 || n > testMaxTotal {
			return d, badRequestf("%s is 0 to %d", key, testMaxTotal)
		}
		*dst = n
	}
	if r.Has("results") {
		res, err := readTestResults(r.List("results"), s.Results)
		if err != nil {
			return d, err
		}
		s.Results = res
	} else {
		s.Results = append([]TestSetupResult(nil), s.Results...)
	}
	for _, res := range s.Results {
		if res.On == "successes" && s.Successes == 0 {
			return d, badRequestf("%s is offered on successes, so give `successes`", quote(res.Label))
		}
		if res.On == "failures" && s.Failures == 0 {
			return d, badRequestf("%s is offered on failures, so give `failures`", quote(res.Label))
		}
	}
	d.setup = s
	return d, nil
}

// readTestResults reads `results:`, keeping the id of a result whose name is
// unchanged so encounter versions that name it still find it.
func readTestResults(items []any, before []TestSetupResult) ([]TestSetupResult, error) {
	if len(items) == 0 {
		return nil, apperror.NewBadRequest("a test needs at least one result")
	}
	if len(items) > testMaxResults {
		return nil, badRequestf("a test has at most %d results", testMaxResults)
	}
	oldID := map[string]string{}
	for _, b := range before {
		oldID[strings.ToLower(b.Label)] = b.ID
	}
	used := map[string]bool{}
	ons := map[string]bool{}
	out := make([]TestSetupResult, 0, len(items))
	for i, it := range items {
		var res TestSetupResult
		switch t := it.(type) {
		case map[string]any:
			m := lowerMap(t)
			res.Label = strings.TrimSpace(fmt.Sprint(m["name"]))
			if m["on"] != nil {
				res.On = strings.ToLower(strings.TrimSpace(fmt.Sprint(m["on"])))
			}
		case nil:
		default:
			parts := strings.Split(fmt.Sprint(t), ",")
			res.Label = strings.TrimSpace(parts[0])
			for _, p := range parts[1:] {
				p = strings.ToLower(strings.Join(strings.Fields(p), " "))
				switch p {
				case "on successes", "successes":
					res.On = "successes"
				case "on failures", "failures":
					res.On = "failures"
				case "":
				default:
					return nil, badRequestf("result %d: %q is not `on successes` or `on failures`", i+1, p)
				}
			}
		}
		if res.Label == "" || res.Label == "<nil>" {
			return nil, badRequestf("result %d has no name", i+1)
		}
		switch res.On {
		case "", "successes", "failures":
		case "none":
			res.On = ""
		default:
			return nil, badRequestf("result %d: `on` is successes or failures", i+1)
		}
		if res.On != "" {
			if ons[res.On] {
				return nil, badRequestf("only one result can be offered when the %s fill", res.On)
			}
			ons[res.On] = true
		}
		key := strings.ToLower(res.Label)
		if used["label:"+key] {
			return nil, badRequestf("two results are called %s", quote(res.Label))
		}
		used["label:"+key] = true
		res.ID = oldID[key]
		if res.ID == "" {
			res.ID = testResultID(res.Label, used)
		}
		used[res.ID] = true
		out = append(out, res)
	}
	return out, nil
}

// testResultID makes a short id from a result's name that the Tests block
// accepts (letters, digits, _ and -) and no other result on the test has.
func testResultID(label string, used map[string]bool) string {
	var b strings.Builder
	for _, r := range strings.ToLower(label) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "-"):
			b.WriteByte('-')
		}
		if b.Len() >= 40 {
			break
		}
	}
	base := strings.Trim(b.String(), "-")
	if base == "" {
		base = "result"
	}
	id := base
	for n := 2; used[id]; n++ {
		id = fmt.Sprintf("%s-%d", base, n)
	}
	return id
}

func testSummary(s TestSetup) string {
	labels := make([]string, len(s.Results))
	for i, r := range s.Results {
		labels[i] = r.Label
		if r.On != "" {
			labels[i] += " (on " + r.On + ")"
		}
	}
	var ends []string
	if s.Successes > 0 {
		ends = append(ends, fmt.Sprintf("%d successes", s.Successes))
	}
	if s.Failures > 0 {
		ends = append(ends, fmt.Sprintf("%d failures", s.Failures))
	}
	out := "results " + strings.Join(labels, ", ")
	if len(ends) > 0 {
		out = "ends at " + strings.Join(ends, " or ") + "; " + out
	}
	if s.Shown {
		out += "; players see it"
	} else {
		out += "; hidden from players"
	}
	return out
}

func (k TestKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	d, err := k.read(ctx, campaignID, a, r)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	if d.page == nil {
		if r.Action != ActionCreate || !r.Made[entities.Slugify(r.Str("page"))] {
			return Plan{Error: "no page called " + quote(r.Str("page")) + "; add it to this paste, or name a page that exists"}
		}
	}
	var p Plan
	switch r.Action {
	case ActionCreate:
		if d.existing != nil {
			return Plan{Error: "a test called " + quote(r.Name) + " is already on " + d.page.Name + "; use action: update"}
		}
		p.Summary = "new test: " + testSummary(d.setup)
	case ActionUpdate:
		if d.existing == nil {
			return Plan{Error: "no test called " + quote(r.Name) + " on " + quote(r.Str("page"))}
		}
		p.Summary = "changes the test: " + testSummary(d.setup)
	case ActionDelete:
		if d.existing == nil {
			return Plan{Error: "no test called " + quote(r.Name) + " on " + quote(r.Str("page"))}
		}
		p.Summary = "removes the test and its recorded result; encounter versions that named it are no longer picked by it"
		return p
	}
	if d.page != nil {
		if ok, err := k.Svc.HasBlock(ctx, campaignID, d.page.ID); err == nil && !ok {
			p.Warnings = append(p.Warnings, d.page.Name+"'s layout has no Tests block, so the test won't show on the page until one is added in the Customizer. Encounter versions can still use it.")
		}
	}
	return p
}

func (k TestKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	d, err := k.read(ctx, campaignID, a, r)
	if err != nil {
		return err
	}
	if d.page == nil {
		return badRequestf("no page called %s", quote(r.Str("page")))
	}
	switch r.Action {
	case ActionCreate:
		if d.existing != nil {
			return badRequestf("a test called %s is already on %s", quote(r.Name), d.page.Name)
		}
		return k.Svc.Create(ctx, campaignID, d.page.ID, a, d.setup)
	case ActionUpdate, ActionDelete:
		if d.existing == nil {
			return badRequestf("no test called %s on %s", quote(r.Name), d.page.Name)
		}
		if r.Action == ActionDelete {
			return k.Svc.Delete(ctx, campaignID, d.existing.ID, a)
		}
		return k.Svc.Update(ctx, campaignID, d.existing.ID, a, d.setup)
	}
	return nil
}

// Export writes every test as a block an AI can send back changed.
func (k TestKind) Export(ctx context.Context, campaignID string, a Actor) (string, error) {
	if k.Svc == nil || !a.CanAuthorDmOnly() {
		return "", nil
	}
	pages, err := k.Svc.Pages(ctx, campaignID, a)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, pg := range pages {
		list, err := k.Svc.OnPage(ctx, campaignID, pg.ID, a)
		if err != nil {
			return "", err
		}
		for _, t := range list {
			b.WriteString(testBlock(pg.Name, t))
		}
	}
	return b.String(), nil
}

// testBlock is one test in the block grammar.
func testBlock(page string, t TestSetup) string {
	var b strings.Builder
	b.WriteString("---\nkind: test\naction: update\npage: " + yamlStr(page) + "\nname: " + yamlStr(t.Name) + "\n")
	if t.Type != "" {
		b.WriteString("type: " + yamlStr(t.Type) + "\n")
	}
	if t.Successes > 0 {
		fmt.Fprintf(&b, "successes: %d\n", t.Successes)
	}
	if t.Failures > 0 {
		fmt.Fprintf(&b, "failures: %d\n", t.Failures)
	}
	if t.Shown {
		b.WriteString("players_see: true\n")
	}
	b.WriteString("results:\n")
	for _, r := range t.Results {
		line := r.Label
		if r.On != "" {
			line += ", on " + r.On
		}
		b.WriteString("  - " + yamlStr(line) + "\n")
	}
	b.WriteString("---\n\n")
	return b.String()
}
