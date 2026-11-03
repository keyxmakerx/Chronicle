package records

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// TrackersAPI is the Trackers block as AI Import writes and reads it,
// adapted in app wiring with the operator as the viewer, so this package
// does not import the plugin. The service applies its own Director rule,
// page access and limits.
type TrackersAPI interface {
	// Check says whether the service would accept a kind's settings,
	// without saving, so the review refuses a bad one before commit.
	Check(in TrackerSetup) error
	Kinds(ctx context.Context, campaignID string, a Actor) ([]TrackerSetup, []TrackerCategory, error)
	CreateKind(ctx context.Context, campaignID string, a Actor, in TrackerSetup) (*TrackerSetup, error)
	UpdateKind(ctx context.Context, campaignID, kindID string, a Actor, in TrackerSetup) (*TrackerSetup, error)
	DeleteKind(ctx context.Context, campaignID, kindID string, a Actor) error
	// Page is every tracker on a page, the hidden ones included.
	Page(ctx context.Context, campaignID, pageID string, a Actor) (*TrackerPage, error)
	Place(ctx context.Context, campaignID, pageID string, a Actor, in TrackerPlace) error
	Log(ctx context.Context, campaignID, pageID string, a Actor, in TrackerChange) error
	// Pages lists the pages that have trackers.
	Pages(ctx context.Context, campaignID string, a Actor) ([]TrackerPageRef, error)
	// HasBlock says whether a page's category layout shows the Trackers block.
	HasBlock(ctx context.Context, campaignID, pageID string) (bool, error)
}

// Trackers' shapes, who sees them and the two ways a value changes, in the
// plugin's own words.
const (
	trackerCount  = "count"
	trackerSteps  = "steps"
	trackerClock  = "clock"
	trackerHidden = "directors"
	trackerShown  = "everyone"
	trackerAdd    = "add"
	trackerSet    = "set"
)

// TrackerSetup is a tracker kind: its shape and rules, saved once per
// campaign. Min and Max bound a count (nil is open); a clock's Max is its
// segments. EveryTypeID puts it on every page of one category, 0 none.
type TrackerSetup struct {
	ID, Name, Shape string
	Min, Max        *int
	Steps           []TrackerStep
	Bands           []TrackerBand
	Visibility      string
	EveryTypeID     int
	Version         int
	// Managed is a kind a Homebrew recipe keeps; InUse counts its trackers
	// on pages. Read only.
	Managed bool
	InUse   int
}

// TrackerStep is one named step; TrackerBand what a value means from At up.
type (
	TrackerStep struct{ Name, Text string }
	TrackerBand struct {
		At   int
		Text string
	}
)

// TrackerCategory is a page category a kind can be on every page of.
type TrackerCategory struct {
	ID   int
	Name string
}

// TrackerPage is the trackers on one page.
type TrackerPage struct{ Trackers []TrackerOnPage }

// TrackerOnPage is one tracker on a page, with its value. ID is "" for a
// kind a category puts on every page that nobody has changed there yet.
type TrackerOnPage struct {
	ID, KindID, Name, Shape, Visibility string
	Value                               int
	Max                                 *int
	Steps                               []TrackerStep
	// Now is what the value means: the step's or band's text.
	Now string
	// Log is the latest changes first.
	Log []TrackerLogLine
}

// TrackerLogLine is one change in a tracker's log; When is the in-game date.
type TrackerLogLine struct {
	Op         string
	Amount     int
	Note, When string
	Struck     bool
}

// TrackerPlace puts a kind on a page.
type TrackerPlace struct{ KindID, Visibility string }

// TrackerChange is one change of value: on a placed tracker by TrackerID, or
// on a category's every-page tracker that has no row yet by KindID.
type TrackerChange struct {
	TrackerID, KindID string
	Op                string
	Amount            int
	Note              string
}

// TrackerPageRef is a page that has trackers.
type TrackerPageRef struct{ ID, Name string }

// TrackerKind writes the campaign's trackers: a kind (a counter, named steps
// or a clock) put on pages or on every page of a category, and a change to
// its value on one page, logged with a note like a change made by hand.
type TrackerKind struct {
	Svc      TrackersAPI
	Entities EntityLookup
}

func (TrackerKind) Name() string  { return "tracker" }
func (TrackerKind) Label() string { return "Tracker" }
func (TrackerKind) Doc() string {
	return "A tracker (owner or co-DM only): a number, named steps or a clock kept on pages, with a dated log of every change. Matched by `name`. Keys: `shape` (`count`, the default; `steps`; or `clock`), `min` and `max` (a count's ends, either optional; a clock's `max` is its segments, 2 to 24), `steps` (2 to 20, in order, each `Name: what it means`), `bands` (a count's or clock's meanings from a value up, each `3: what it means`), `players_see` (false, the default, keeps it hidden from players: a secret counter; true shows it on the page), `every` (a category's name: the tracker is on every page of it), and `pages` (page names to put it on). " +
		"To change its value on one page, add `page` with `add` (a step up or down, such as 1 or -1) or `set` (the new value), and a `note` saying why; a page that doesn't have the tracker yet gets it. On update a key you leave out keeps what is there and `pages` adds pages. A tracker kept by a Homebrew recipe is changed through its recipe, not here. Removing one is refused while it is still on pages. A tracker shows only on pages whose layout has a Trackers block. Read today's values with `what: trackers`.\n\n" +
		"```\n---\nkind: tracker\nname: Counsel\nmin: 0\nmax: 5\nbands:\n  - \"0: Orrin has no counsel to give\"\n  - \"3: Orrin's advice comes unbidden\"\npages: [Orrin]\n---\n\n---\nkind: tracker\naction: update\nname: Counsel\npage: Orrin\nadd: 1\nnote: Counselled Saatraaol at the Lastwell gate\n---\n```"
}

// Title is the row's heading: "Counsel", or "Counsel on Orrin" for a change.
func (TrackerKind) Title(r Record) string {
	if r.Name == "" {
		return ""
	}
	if p := r.Str("page"); p != "" && (r.Has("add") || r.Has("set")) {
		return r.Name + " on " + p
	}
	return r.Name
}

// trackerKeys are the keys that change the kind itself, as opposed to one
// page's value.
var trackerKeys = []string{"shape", "min", "max", "steps", "bands", "players_see", "every", "rename_to"}

type trackerDraft struct {
	existing *TrackerSetup
	in       TrackerSetup
	kindSet  bool // the block changes the kind's settings
	places   []*entities.Entity
	missing  []string // pages this paste makes
	logPage  *entities.Entity
	logName  string
	log      *TrackerChange
	every    string
}

func (k TrackerKind) page(ctx context.Context, campaignID, name string) *entities.Entity {
	if k.Entities == nil || name == "" {
		return nil
	}
	e, err := k.Entities.GetBySlug(ctx, campaignID, entities.Slugify(name))
	if err != nil || e == nil || e.CampaignID != campaignID {
		return nil
	}
	return e
}

func intPtr(r Record, key string) (*int, error) {
	if !r.Has(key) || r.Fields[key] == nil {
		return nil, nil
	}
	n, _, err := r.Int(key)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

// pairs reads a list of "key: text" items, written either as strings or as
// the one-key maps YAML makes of them, or as maps with the named keys.
func pairs(items []any, keyName, textName string) ([][2]string, error) {
	var out [][2]string
	for i, it := range items {
		switch t := it.(type) {
		case map[string]any:
			m := lowerMap(t)
			if m[keyName] != nil || m[textName] != nil {
				out = append(out, [2]string{strings.TrimSpace(fmt.Sprint(m[keyName])), strings.TrimSpace(fmt.Sprint(nilText(m[textName])))})
				continue
			}
			if len(t) != 1 {
				return nil, badRequestf("item %d must be one `%s: text`", i+1, keyName)
			}
			for kk, v := range t {
				out = append(out, [2]string{strings.TrimSpace(kk), strings.TrimSpace(fmt.Sprint(nilText(v)))})
			}
		case nil:
			return nil, badRequestf("item %d is empty", i+1)
		default:
			s := fmt.Sprint(t)
			key, text, _ := strings.Cut(s, ":")
			out = append(out, [2]string{strings.TrimSpace(key), strings.TrimSpace(text)})
		}
	}
	return out, nil
}

func nilText(v any) any {
	if v == nil {
		return ""
	}
	return v
}

func (k TrackerKind) read(ctx context.Context, campaignID string, a Actor, r Record) (trackerDraft, error) {
	var d trackerDraft
	if !a.CanAuthorDmOnly() {
		return d, apperror.NewBadRequest("only the owner or a co-DM can change trackers")
	}
	if k.Svc == nil {
		return d, apperror.NewBadRequest("trackers aren't available on this server")
	}
	if r.Name == "" {
		return d, apperror.NewBadRequest("a tracker needs a name")
	}
	kinds, types, err := k.Svc.Kinds(ctx, campaignID, a)
	if err != nil {
		return d, apperror.NewBadRequest("could not read the campaign's trackers")
	}
	for i := range kinds {
		if sameName(kinds[i].Name, r.Name) {
			d.existing = &kinds[i]
			break
		}
	}
	if r.Action == ActionDelete {
		return d, nil
	}

	in := TrackerSetup{Name: r.Name, Shape: trackerCount, Visibility: trackerHidden}
	if d.existing != nil {
		in = *d.existing
	}
	for _, key := range trackerKeys {
		if r.Has(key) {
			d.kindSet = true
		}
	}
	if v := r.Str("rename_to"); v != "" {
		in.Name = v
	}
	if r.Has("shape") {
		in.Shape = strings.ToLower(r.Str("shape"))
	}
	if r.Has("min") {
		if in.Min, err = intPtr(r, "min"); err != nil {
			return d, err
		}
	}
	if r.Has("max") {
		if in.Max, err = intPtr(r, "max"); err != nil {
			return d, err
		}
	}
	if r.Has("steps") {
		ps, err := pairs(r.List("steps"), "name", "text")
		if err != nil {
			return d, badRequestf("steps: %s", planError(err))
		}
		in.Steps = nil
		for _, p := range ps {
			in.Steps = append(in.Steps, TrackerStep{Name: p[0], Text: p[1]})
		}
	}
	if r.Has("bands") {
		ps, err := pairs(r.List("bands"), "at", "text")
		if err != nil {
			return d, badRequestf("bands: %s", planError(err))
		}
		in.Bands = nil
		for _, p := range ps {
			at, err := strconv.Atoi(p[0])
			if err != nil {
				return d, badRequestf("bands: %q doesn't start with a whole number", p[0]+": "+p[1])
			}
			in.Bands = append(in.Bands, TrackerBand{At: at, Text: p[1]})
		}
	}
	if r.Has("players_see") {
		see, _ := r.Bool("players_see")
		in.Visibility = map[bool]string{true: trackerShown, false: trackerHidden}[see]
	}
	if r.Has("every") {
		d.every = r.Str("every")
		in.EveryTypeID = 0
		if d.every != "" && !strings.EqualFold(d.every, "none") {
			for _, t := range types {
				if sameName(t.Name, d.every) {
					in.EveryTypeID = t.ID
				}
			}
			if in.EveryTypeID == 0 {
				return d, badRequestf("no category called %s", quote(d.every))
			}
		}
	}
	if err := k.Svc.Check(in); err != nil {
		return d, err
	}
	d.in = in

	for _, it := range r.List("pages") {
		name := strings.TrimSpace(fmt.Sprint(it))
		if p := k.page(ctx, campaignID, name); p != nil {
			d.places = append(d.places, p)
		} else {
			d.missing = append(d.missing, name)
		}
	}

	_, hasAdd := r.Fields["add"]
	_, hasSet := r.Fields["set"]
	if hasAdd || hasSet {
		if hasAdd && hasSet {
			return d, apperror.NewBadRequest("a change is `add` or `set`, not both")
		}
		d.logName = r.Str("page")
		if d.logName == "" {
			return d, apperror.NewBadRequest("say which page's value changes with `page`")
		}
		d.logPage = k.page(ctx, campaignID, d.logName)
		op, key := trackerAdd, "add"
		if hasSet {
			op, key = trackerSet, "set"
		}
		n, _, err := r.Int(key)
		if err != nil {
			return d, err
		}
		if op == trackerAdd && n == 0 {
			return d, apperror.NewBadRequest("`add` must move the value: 1, -1 or more")
		}
		d.log = &TrackerChange{Op: op, Amount: n, Note: r.Str("note")}
	} else if r.Has("page") {
		return d, apperror.NewBadRequest("to change a value on `page`, add `add` or `set`; to put the tracker on pages, use `pages`")
	}
	return d, nil
}

func trackerShapeLine(in TrackerSetup) string {
	switch in.Shape {
	case trackerSteps:
		names := make([]string, len(in.Steps))
		for i, s := range in.Steps {
			names[i] = s.Name
		}
		return "steps " + strings.Join(names, ", ")
	case trackerClock:
		return fmt.Sprintf("a clock of %d segments", *in.Max)
	}
	switch {
	case in.Min != nil && in.Max != nil:
		return fmt.Sprintf("a count from %d to %d", *in.Min, *in.Max)
	case in.Min != nil:
		return fmt.Sprintf("a count from %d", *in.Min)
	case in.Max != nil:
		return fmt.Sprintf("a count up to %d", *in.Max)
	}
	return "a count"
}

func (k TrackerKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	d, err := k.read(ctx, campaignID, a, r)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	needed := append([]string(nil), d.missing...)
	if d.log != nil && d.logPage == nil {
		needed = append(needed, d.logName)
	}
	for _, m := range needed {
		if !r.Made[entities.Slugify(m)] {
			return Plan{Error: "no page called " + quote(m) + "; add it to this paste, or name a page that exists"}
		}
	}
	var parts []string
	switch r.Action {
	case ActionCreate:
		if d.existing != nil {
			return Plan{Error: "a tracker called " + quote(r.Name) + " already exists; use action: update"}
		}
		parts = append(parts, "new tracker: "+trackerShapeLine(d.in))
	case ActionUpdate:
		if d.existing == nil {
			return Plan{Error: "no tracker called " + quote(r.Name) + "; use action: create"}
		}
		if d.kindSet {
			parts = append(parts, "changes it to "+trackerShapeLine(d.in))
		}
	case ActionDelete:
		if d.existing == nil {
			return Plan{Error: "no tracker called " + quote(r.Name)}
		}
		if d.existing.InUse > 0 {
			return Plan{Error: fmt.Sprintf("%s is still on %s; take it off them in Chronicle first", r.Name, plural(d.existing.InUse, "page", "pages"))}
		}
		return Plan{Summary: "removes the tracker"}
	}
	if d.kindSet || r.Action == ActionCreate {
		if d.in.Visibility == trackerShown {
			parts = append(parts, "players see it")
		} else {
			parts = append(parts, "hidden from players")
		}
		if d.in.EveryTypeID != 0 {
			parts = append(parts, "on every "+d.every+" page")
		}
	}
	if n := len(d.places) + len(d.missing); n > 0 {
		parts = append(parts, "put on "+plural(n, "page", "pages"))
	}
	if d.log != nil {
		line := fmt.Sprintf("%s %+d", d.logName, d.log.Amount)
		if d.log.Op == trackerSet {
			line = fmt.Sprintf("%s set to %d", d.logName, d.log.Amount)
		}
		if d.log.Note != "" {
			line += " (" + d.log.Note + ")"
		}
		parts = append(parts, line)
	}
	if len(parts) == 0 {
		return Plan{Error: "say what to change: the tracker's settings, `pages`, or `page` with `add` or `set`"}
	}
	p := Plan{Summary: strings.Join(parts, "; ")}
	for _, pg := range append(append([]*entities.Entity(nil), d.places...), d.logPage) {
		if pg == nil {
			continue
		}
		if ok, err := k.Svc.HasBlock(ctx, campaignID, pg.ID); err == nil && !ok {
			p.Warnings = append(p.Warnings, pg.Name+"'s layout has no Trackers block, so the tracker won't show there until one is added in the Customizer.")
		}
	}
	return p
}

func (k TrackerKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	d, err := k.read(ctx, campaignID, a, r)
	if err != nil {
		return err
	}
	if r.Action == ActionDelete {
		if d.existing == nil {
			return badRequestf("no tracker called %s", quote(r.Name))
		}
		return k.Svc.DeleteKind(ctx, campaignID, d.existing.ID, a)
	}
	// Pages this paste made exist by now.
	for _, m := range d.missing {
		p := k.page(ctx, campaignID, m)
		if p == nil {
			return badRequestf("no page called %s", quote(m))
		}
		d.places = append(d.places, p)
	}
	if d.log != nil && d.logPage == nil {
		if d.logPage = k.page(ctx, campaignID, d.logName); d.logPage == nil {
			return badRequestf("no page called %s", quote(d.logName))
		}
	}
	var kind *TrackerSetup
	switch {
	case r.Action == ActionCreate:
		if d.existing != nil {
			return badRequestf("a tracker called %s already exists", quote(r.Name))
		}
		if kind, err = k.Svc.CreateKind(ctx, campaignID, a, d.in); err != nil {
			return err
		}
	case d.existing == nil:
		return badRequestf("no tracker called %s", quote(r.Name))
	case d.kindSet:
		if kind, err = k.Svc.UpdateKind(ctx, campaignID, d.existing.ID, a, d.in); err != nil {
			return err
		}
	default:
		kind = d.existing
	}
	for _, pg := range d.places {
		if _, err := k.placed(ctx, campaignID, pg.ID, a, kind); err != nil {
			return err
		}
	}
	if d.log == nil {
		return nil
	}
	in := *d.log
	tv, err := k.placed(ctx, campaignID, d.logPage.ID, a, kind)
	if err != nil {
		return err
	}
	if tv.ID != "" {
		in.TrackerID = tv.ID
	} else {
		in.KindID = kind.ID
	}
	return k.Svc.Log(ctx, campaignID, d.logPage.ID, a, in)
}

// placed is the page's tracker of kind, put on the page first when it has
// none. A kind on every page of the page's category answers its virtual
// tracker (no id), whose first change creates it.
func (k TrackerKind) placed(ctx context.Context, campaignID, pageID string, a Actor, kind *TrackerSetup) (TrackerOnPage, error) {
	find := func() (TrackerOnPage, bool, error) {
		pv, err := k.Svc.Page(ctx, campaignID, pageID, a)
		if err != nil {
			return TrackerOnPage{}, false, err
		}
		for _, t := range pv.Trackers {
			if t.KindID == kind.ID {
				return t, true, nil
			}
		}
		return TrackerOnPage{}, false, nil
	}
	if t, ok, err := find(); err != nil || ok {
		return t, err
	}
	if err := k.Svc.Place(ctx, campaignID, pageID, a, TrackerPlace{KindID: kind.ID, Visibility: kind.Visibility}); err != nil {
		return TrackerOnPage{}, err
	}
	t, ok, err := find()
	if err == nil && !ok {
		err = apperror.NewBadRequest("the tracker could not be put on the page")
	}
	return t, err
}

// Export writes each tracker as a block, with the pages it is on.
func (k TrackerKind) Export(ctx context.Context, campaignID string, a Actor) (string, error) {
	if k.Svc == nil || !a.CanAuthorDmOnly() {
		return "", nil
	}
	kinds, types, err := k.Svc.Kinds(ctx, campaignID, a)
	if err != nil {
		return "", err
	}
	onPages, err := trackerPages(ctx, k.Svc, campaignID, a)
	if err != nil {
		return "", err
	}
	typeName := map[int]string{}
	for _, t := range types {
		typeName[t.ID] = t.Name
	}
	var b strings.Builder
	for _, kd := range kinds {
		if kd.Managed {
			continue
		}
		b.WriteString(trackerBlock(kd, typeName[kd.EveryTypeID], onPages[kd.ID]))
	}
	return b.String(), nil
}

// trackerPages maps each kind to the names of the pages it was placed on.
func trackerPages(ctx context.Context, svc TrackersAPI, campaignID string, a Actor) (map[string][]string, error) {
	pages, err := svc.Pages(ctx, campaignID, a)
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, pg := range pages {
		pv, err := svc.Page(ctx, campaignID, pg.ID, a)
		if err != nil {
			return nil, err
		}
		for _, t := range pv.Trackers {
			if t.ID != "" {
				out[t.KindID] = append(out[t.KindID], pg.Name)
			}
		}
	}
	for _, names := range out {
		sort.Strings(names)
	}
	return out, nil
}

func trackerBlock(kd TrackerSetup, every string, pages []string) string {
	var b strings.Builder
	b.WriteString("---\nkind: tracker\naction: update\nname: " + yamlStr(kd.Name) + "\nshape: " + kd.Shape + "\n")
	if kd.Min != nil && kd.Shape == trackerCount {
		fmt.Fprintf(&b, "min: %d\n", *kd.Min)
	}
	if kd.Max != nil {
		fmt.Fprintf(&b, "max: %d\n", *kd.Max)
	}
	if len(kd.Steps) > 0 {
		b.WriteString("steps:\n")
		for _, s := range kd.Steps {
			b.WriteString("  - " + quote(s.Name+": "+s.Text) + "\n")
		}
	}
	if len(kd.Bands) > 0 {
		b.WriteString("bands:\n")
		for _, bd := range kd.Bands {
			b.WriteString("  - " + quote(fmt.Sprintf("%d: %s", bd.At, bd.Text)) + "\n")
		}
	}
	fmt.Fprintf(&b, "players_see: %t\n", kd.Visibility == trackerShown)
	if every != "" {
		b.WriteString("every: " + yamlStr(every) + "\n")
	}
	if len(pages) > 0 {
		quoted := make([]string, len(pages))
		for i, p := range pages {
			quoted[i] = yamlStr(p)
		}
		b.WriteString("pages: [" + strings.Join(quoted, ", ") + "]\n")
	}
	b.WriteString("---\n\n")
	return b.String()
}

// trackerLogShown is how many of a tracker's latest log lines the lookup
// quotes: enough to see where a count came from, not its whole history.
const trackerLogShown = 3

// trackersLookup lists each page's trackers with their values, the hidden
// ones marked, for the Director's AI only.
func (run *lookupRun) trackersLookup(page string) (LookupAnswer, error) {
	ans := LookupAnswer{Chip: "Trackers"}
	if run.l.Trackers == nil {
		return ans, badRequestf("trackers can't be looked up on this server")
	}
	if !run.director() {
		return ans, badRequestf("trackers are left out in Safe privacy; choose Permitted to include them")
	}
	pages, err := run.l.Trackers.Pages(run.ctx, run.campaignID, run.a)
	if err != nil {
		return ans, err
	}
	var lines []string
	count := 0
	for _, pg := range pages {
		if page != "" && !sameName(pg.Name, page) {
			continue
		}
		pv, err := run.l.Trackers.Page(run.ctx, run.campaignID, pg.ID, run.a)
		if err != nil {
			return ans, err
		}
		for _, t := range pv.Trackers {
			if t.ID == "" {
				continue
			}
			count++
			line := fmt.Sprintf("- %s on %s: %s", t.Name, pg.Name, trackerValue(t))
			if t.Now != "" {
				line += " (" + t.Now + ")"
			}
			if t.Visibility != trackerShown {
				line += "; hidden from players"
			}
			lines = append(lines, line)
			for i, e := range t.Log {
				if i >= trackerLogShown {
					break
				}
				l := fmt.Sprintf("  - %s %d", e.Op, e.Amount)
				if e.When != "" {
					l += " on " + e.When
				}
				if e.Note != "" {
					l += ": " + e.Note
				}
				if e.Struck {
					l += " (struck out)"
				}
				lines = append(lines, l)
			}
		}
	}
	ans.Chip = fmt.Sprintf("Trackers · %d", count)
	var b strings.Builder
	fmt.Fprintf(&b, "## Trackers (%d)\n\nChange one with a `kind: tracker` block: `page`, then `add` or `set`, and a `note`. A tracker a category puts on every page shows here once it has been changed.\n\n", count)
	if len(lines) == 0 {
		b.WriteString("None yet.\n")
	}
	writeLines(&b, lines)
	ans.Text = b.String()
	return ans, nil
}

// trackerValue is a tracker's value in its own shape's words.
func trackerValue(t TrackerOnPage) string {
	switch t.Shape {
	case trackerSteps:
		if t.Value >= 0 && t.Value < len(t.Steps) {
			return "at " + t.Steps[t.Value].Name
		}
	case trackerClock:
		if t.Max != nil {
			return fmt.Sprintf("%d of %d segments", t.Value, *t.Max)
		}
	}
	return strconv.Itoa(t.Value)
}
