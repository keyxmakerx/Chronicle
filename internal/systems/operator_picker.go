// operator_picker.go shapes the diagnostic catalog for the AI helper page:
// six everyday questions that each pick their checks, the whole catalog in
// plain-word groups for picking by hand, and ComposeBatch, which turns either
// pick into the same batch request an outside AI would paste. The page never
// runs a check from here; it hands the composed request to ParseBatch and the
// usual review step, so a picked request is validated exactly like a pasted one.
package systems

import (
	"encoding/json"
	"strings"
)

// ArgKind says which blank a check needs filled before it can run.
type ArgKind string

const (
	ArgNone         ArgKind = ""              // no argument
	ArgCampaign     ArgKind = "campaign"      // a campaign id
	ArgCampaignPage ArgKind = "campaign-page" // campaign id + a page's name, slug or id
	ArgCampaignText ArgKind = "campaign-text" // campaign id + free text (a name to find, a category)
	ArgText         ArgKind = "text"          // free text in the check's own arg shape
)

// PickerCheck is one check as the page lists it: a plain title for people,
// the catalog name, and the blank it needs.
type PickerCheck struct {
	Name     string
	Title    string
	Hint     string // the catalog's arg hint, shown as the placeholder of a text blank
	Kind     ArgKind
	Optional bool // the text blank may stay empty
	FullDump bool // large; picking it is the explicit opt-in
}

// PickerGroup is one fold of the hand-picking list.
type PickerGroup struct {
	ID     string
	Title  string
	Desc   string
	Checks []PickerCheck
}

// DiagnosticQuestion is one everyday problem and the checks that answer it.
// Sentence names what Chronicle will look at; {campaign} and {page} mark the
// blanks the operator fills in place.
type DiagnosticQuestion struct {
	ID       string
	Icon     string // Font Awesome name
	Title    string
	Desc     string
	Sentence string
	Calls    []string
}

// NeedsCampaign reports whether the question's sentence has a campaign blank.
func (q DiagnosticQuestion) NeedsCampaign() bool { return strings.Contains(q.Sentence, "{campaign}") }

// NeedsPage reports whether the question's sentence has a page blank.
func (q DiagnosticQuestion) NeedsPage() bool { return strings.Contains(q.Sentence, "{page}") }

// pickerTitles are the plain-word names people see; the catalog's own titles
// are written for the AI that reads the functions list.
var pickerTitles = map[string]string{
	"host.deploy-check":            "Did my deploy land?",
	"host.build":                   "Which build this server is running",
	"host.runtime":                 "Memory, uptime and load",
	"host.plugins":                 "Parts registered in this build",
	"host.widgets":                 "Widget files and their fingerprints",
	"host.assets":                  "Style and script files on disk",
	"host.asset-contains":          "Does a style or script file contain…",
	"host.embedded":                "Files built into the program",
	"host.embedded-contains":       "Does a built-in file contain…",
	"host.errors-summary":          "Errors grouped by page",
	"host.errors":                  "Recent errors, newest first",
	"packages.installed-vs-loaded": "Installed version against served version",
	"system.versions":              "Served game system versions",
	"systems.load-events":          "What loaded, skipped or failed",
	"packages.on-disk-versions":    "Every version folder on disk",
	"packages.prune-preview":       "Old folders cleanup would remove",
	"system.files":                 "File fingerprints for one game system",
	"system.file-contains":         "Does a game system file contain…",
	"system.health":                "Everything about every game system",
	"campaigns.list":               "List campaigns",
	"campaign.config":              "Add-ons on, and widgets placed",
	"campaign.surfaces":            "Which calendar page a link opens",
	"calendar.stats":               "Calendar counts",
	"entity.find":                  "Find a page by name",
	"entity.fields":                "Stored fields for one page",
	"entity.types":                 "Categories in a campaign",
	"entity.field-coverage":        "Empty fields across a category",
	"entity.sync-mappings":         "Is this page linked to Foundry?",
	"sync.recent":                  "Latest updates from Foundry",
	"sync.inbound":                 "What Foundry sent for one page",
	"probes":                       "Commands to run yourself",
}

// pickerKinds says which blank each check needs. Checks not listed take none.
var pickerKinds = map[string]ArgKind{
	"host.deploy-check":      ArgText,
	"host.errors":            ArgText,
	"host.assets":            ArgText,
	"host.asset-contains":    ArgText,
	"host.embedded":          ArgText,
	"host.embedded-contains": ArgText,
	"host.widgets":           ArgText,
	"system.files":           ArgText,
	"system.file-contains":   ArgText,
	"campaign.surfaces":      ArgCampaign,
	"campaign.config":        ArgCampaign,
	"calendar.stats":         ArgCampaign,
	"entity.types":           ArgCampaign,
	"entity.find":            ArgCampaignText,
	"entity.field-coverage":  ArgCampaignText,
	"entity.fields":          ArgCampaignPage,
	"entity.sync-mappings":   ArgCampaignPage,
	"sync.inbound":           ArgCampaignPage,
}

// pickerGroupOrder lays the catalog out in folds; every catalog check sits in
// exactly one (pinned by the tests).
var pickerGroupOrder = []struct {
	id, title, desc string
	names           []string
}{
	{"build", "This server and its build", "Which build is running and what it serves", []string{
		"host.deploy-check", "host.build", "host.runtime", "host.plugins", "host.widgets",
		"host.assets", "host.asset-contains", "host.embedded", "host.embedded-contains"}},
	{"errors", "Errors", "What broke since the last restart", []string{"host.errors-summary", "host.errors"}},
	{"systems", "Game systems and packages", "Installed versions against what is served", []string{
		"packages.installed-vs-loaded", "system.versions", "systems.load-events", "packages.on-disk-versions",
		"packages.prune-preview", "system.files", "system.file-contains", "system.health"}},
	{"campaigns", "Campaigns", "Add-ons, placed widgets and calendar", []string{
		"campaigns.list", "campaign.config", "campaign.surfaces", "calendar.stats"}},
	{"pages", "Pages and sheets", "Stored fields for a page or a category", []string{
		"entity.find", "entity.fields", "entity.types", "entity.field-coverage", "entity.sync-mappings"}},
	{"sync", "Foundry sync", "What Foundry sent, and when", []string{"sync.recent", "sync.inbound"}},
	{"probes", "Commands to run yourself", "For things the server cannot see", []string{"probes"}},
}

// diagnosticQuestions are the signed six. Each call must be in the catalog.
var diagnosticQuestions = []DiagnosticQuestion{
	{ID: "deploy", Icon: "fa-server", Title: "Did my deploy land?",
		Desc:     "After an update: is the new build running and serving its files",
		Sentence: "Check the build this server runs, the files it serves and the packages it loaded.",
		Calls:    []string{"host.deploy-check", "host.build", "packages.installed-vs-loaded"}},
	{ID: "errors", Icon: "fa-bolt", Title: "Something broke",
		Desc:     "Errors since the last restart, grouped by page",
		Sentence: "Show the errors this server recorded, grouped by page, then the latest few in full.",
		Calls:    []string{"host.errors-summary", "host.errors"}},
	{ID: "old-version", Icon: "fa-cube", Title: "A game system shows an old version",
		Desc:     "Packages says one version, pages show another",
		Sentence: "Compare what is installed with what is served, and look for old folders in the way.",
		Calls:    []string{"packages.installed-vs-loaded", "packages.on-disk-versions", "systems.load-events", "system.versions"}},
	{ID: "blank-page", Icon: "fa-user", Title: "A sheet or page shows blank",
		Desc:     "Is the data stored, and did Foundry send it",
		Sentence: "In {campaign}, look up {page}: its stored fields, its Foundry link and what Foundry last sent.",
		Calls:    []string{"entity.fields", "entity.sync-mappings", "sync.inbound"}},
	{ID: "sync", Icon: "fa-arrows-rotate", Title: "Foundry is not syncing",
		Desc:     "Is anything arriving from Foundry at all",
		Sentence: "Show the latest updates from Foundry across all pages.",
		Calls:    []string{"sync.recent"}},
	{ID: "campaign-page", Icon: "fa-calendar", Title: "A campaign page shows the wrong thing",
		Desc:     "Add-ons, placed widgets, and which calendar page opens",
		Sentence: "For {campaign}, list the add-ons that are on, the widgets placed, and which calendar page each link opens.",
		Calls:    []string{"campaign.config", "campaign.surfaces", "calendar.stats"}},
}

// pickerCheck builds the page's view of one catalog entry.
func pickerCheck(d Diagnostic) PickerCheck {
	title := pickerTitles[d.Name]
	if title == "" {
		title = d.Title
	}
	return PickerCheck{
		Name:     d.Name,
		Title:    title,
		Hint:     d.ArgHint,
		Kind:     pickerKinds[d.Name],
		Optional: strings.HasPrefix(d.ArgHint, "["),
		FullDump: d.FullDump,
	}
}

// DiagnosticGroups returns the catalog in its plain-word folds.
func DiagnosticGroups() []PickerGroup {
	byName := map[string]Diagnostic{}
	for _, d := range diagnosticCatalog() {
		byName[d.Name] = d
	}
	out := make([]PickerGroup, 0, len(pickerGroupOrder))
	for _, g := range pickerGroupOrder {
		pg := PickerGroup{ID: g.id, Title: g.title, Desc: g.desc}
		for _, n := range g.names {
			if d, ok := byName[n]; ok {
				pg.Checks = append(pg.Checks, pickerCheck(d))
			}
		}
		out = append(out, pg)
	}
	return out
}

// DiagnosticCheckTitle returns the plain title of a catalog check, or "".
func DiagnosticCheckTitle(name string) string { return pickerTitles[name] }

// DiagnosticQuestions returns the everyday questions in page order.
func DiagnosticQuestions() []DiagnosticQuestion { return diagnosticQuestions }

// ComposeInput is what the page sends: the ticked checks and the blanks.
type ComposeInput struct {
	Note     string            // the question asked, echoed into the result
	Calls    []string          // catalog names, in the order ticked
	Campaign string            // campaign id for every campaign blank
	Page     string            // page name, slug or id for every page blank
	Args     map[string]string // free-text blanks, by check name
}

// ComposeBatch turns a pick into the batch request text ParseBatch reads. A
// blank left empty becomes the placeholder the review step already asks about,
// so nothing is guessed. Ticking a large check is the full-dump opt-in.
func ComposeBatch(in ComposeInput) string {
	kinds := map[string]ArgKind{}
	full := map[string]bool{}
	for _, d := range diagnosticCatalog() {
		kinds[d.Name] = pickerKinds[d.Name]
		full[d.Name] = d.FullDump
	}
	camp := strings.TrimSpace(in.Campaign)
	if camp == "" {
		camp = "<campaignId>"
	}
	page := strings.TrimSpace(in.Page)
	if page == "" {
		page = "<entity>"
	}
	req := BatchRequest{V: batchSpecVersion, Note: strings.TrimSpace(in.Note), Calls: []BatchCall{}}
	for _, name := range in.Calls {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		text := strings.TrimSpace(in.Args[name])
		var arg string
		switch kinds[name] {
		case ArgCampaign:
			arg = camp
		case ArgCampaignPage:
			arg = camp + ":" + page
		case ArgCampaignText:
			arg = camp + ":" + text
		case ArgText:
			arg = text
		}
		if full[name] {
			req.FullDump = true
		}
		req.Calls = append(req.Calls, BatchCall{Name: name, Arg: arg})
	}
	b, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
}
