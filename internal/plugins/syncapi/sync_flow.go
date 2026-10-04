package syncapi

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The call flow draws a stretch of the sync history as a sequence diagram:
// three lanes (Chronicle in a browser, the Chronicle server, Foundry), one
// arrow per step, steps about the same thing grouped and chained. It is
// built here from history rows so the drawing rules are tested in Go; the
// templ only lays it out.

// Lanes, left to right.
const (
	laneBrowser = 0
	laneServer  = 1
	laneFoundry = 2
)

// Line colours: which way a step went, or that it failed.
const (
	lineToChronicle = "toch"
	lineToFoundry   = "tofo"
	lineWeb         = "web"
	lineBad         = "bad"
)

const (
	// flowWindow is how far either side of the chosen row the flow reads.
	flowWindow = 5 * time.Minute
	// flowMaxEvents bounds the rows one flow draws; the closest to the
	// chosen row are kept.
	flowMaxEvents = 40
	// flowChainGap is the longest pause between two steps about the same
	// thing that still links them as cause and result.
	flowChainGap = 2 * time.Minute
	// flowRepeatWindow is how far back "same problem lately" looks.
	flowRepeatWindow = 24 * time.Hour
	// flowRepeatTimes is how many of those times are listed.
	flowRepeatTimes = 8
	// flowGroupColours is how many group colours the stylesheet defines.
	flowGroupColours = 6
)

// FlowKey describes the API key a call came in on.
type FlowKey struct {
	Name  string
	Owner string
	Role  string
}

// FlowProblem is the full story of a failed step, opened under it.
type FlowProblem struct {
	Headline string
	What     string
	Why      string
	// Asked and Was are what the call asked for and what it would have
	// replaced, when the row knows them.
	Asked, Was string
	Next       []string
	Fix        []string
	// FixedBy is the later step that set things right, when there is one.
	FixedBy     string
	FixedByWho  string
	FixedByAt   time.Time
	Repeats     []time.Time
	RepeatCount int
	// RepeatKey names the key when every repeat came in on the same one.
	RepeatKey string
	// OnlyThese filters the campaign's history to the repeats.
	OnlyThese string
	// OpenHistory is the admin's way to the campaign's own Sync history.
	OpenHistory string
}

// FlowStep is one arrow.
type FlowStep struct {
	ID       string
	EventID  int64
	At       time.Time
	From, To int
	Label    string
	Who      string
	Code     string
	Result   string
	Ms       int
	ShowMs   bool
	Line     string
	Reply    bool
	Fail     bool
	Group    int
	What     string
	Recorded string
	Key      *FlowKey
	Cause    []string
	Then     []string
	// Links are Cause and Then with what the detail names them by.
	Links   []FlowLink
	Problem *FlowProblem
}

// FlowLink is a "Caused by" or "Led to" button under a step.
type FlowLink struct {
	ID    string
	Kind  string
	At    time.Time
	Label string
}

// FlowGroup is the thing a run of steps is about.
type FlowGroup struct {
	N      int
	Colour int
	Name   string
	Steps  int
}

// FlowRow is one line of the sheet: a group heading, or a step.
type FlowRow struct {
	Head *FlowHead
	Step *FlowStep
}

// FlowHead starts a run of steps about one thing.
type FlowHead struct {
	Group FlowGroup
	// Gap is how long after the previous step this run starts; zero when
	// it follows at once.
	Gap time.Duration
}

// CallFlow is everything the flow draws.
type CallFlow struct {
	ID       string
	Admin    bool
	Steps    []*FlowStep
	Groups   []FlowGroup
	Rows     []FlowRow
	Selected string
	Problems int
	// OneGroup is true when the flow is about a single thing, so it shows
	// no group switches.
	OneGroup bool
	First    time.Time
	Last     time.Time
}

// flowInput is what buildCallFlow needs beyond the rows themselves.
type flowInput struct {
	Events   []SyncEvent
	Admin    bool
	Selected int64
	OneGroup bool
	Keys     map[int]FlowKey
	// Repeats holds, per failed row id, the same failures lately.
	Repeats     map[int64][]SyncEvent
	OnlyThese   func(ev SyncEvent) string
	OpenHistory string
}

// flowGroupKey says which thing a row is about. A page and its character
// fields are the same thing; the calendar is one thing whatever the row
// calls it; connects, pulls and notes from the module are the connection.
func flowGroupKey(ev SyncEvent) string {
	switch ev.Kind {
	case "page", "character":
		if ev.ResourceID != "" {
			return "page|" + ev.ResourceID
		}
		return "page"
	case historyKindCalendar:
		return historyKindCalendar
	case "connection", "sync", "link", "":
		return "connection"
	}
	if ev.ResourceID != "" {
		return ev.Kind + "|" + ev.ResourceID
	}
	return ev.Kind
}

// flowNoun is the word for a kind of thing, on the admin pages where names
// are hidden.
func flowNoun(kind string) string {
	switch kind {
	case "page", "character":
		return "Page"
	case "map":
		return "Map"
	case "note":
		return "Note"
	case "stash":
		return "Stash"
	}
	return capitalise(kind)
}

func flowGroupName(ev SyncEvent, admin bool, counts map[string]int) string {
	switch flowGroupKey(ev) {
	case historyKindCalendar:
		return "Calendar"
	case "connection":
		return "Connection"
	}
	if admin {
		noun := flowNoun(ev.Kind)
		counts[noun]++
		return noun + " " + strconv.Itoa(counts[noun])
	}
	if ev.ResourceName != "" && ev.Kind != historyKindCalendar {
		return ev.ResourceName
	}
	return historyName(ev)
}

// flowSubject is how a step's sentences name its thing.
func flowSubject(ev SyncEvent, admin bool) string {
	switch ev.Kind {
	case historyKindCalendar:
		return "the calendar"
	case "map":
		if admin || ev.ResourceName == "" {
			return "a map"
		}
	case "note":
		if admin || ev.ResourceName == "" {
			return "a note"
		}
	case "stash":
		return "a stash"
	default:
		if admin || ev.ResourceName == "" {
			return "a page"
		}
	}
	return "“" + ev.ResourceName + "”"
}

func capitalise(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

// callVerbs are the arrow labels for calls Foundry makes, by history action.
var callVerbs = map[string]string{
	"page created":   "Creates a page",
	"page updated":   "Saves the page",
	"page deleted":   "Deletes the page",
	"fields updated": "Saves the character",
	"access changed": "Changes who can see it",
	"tags changed":   "Changes tags",
	"link added":     "Adds a link",
	"date set":       "Moves the date",
	"event created":  "Adds an event",
	"event updated":  "Changes an event",
	"event deleted":  "Deletes an event",
	"pin added":      "Adds a pin",
	"pin updated":    "Moves a pin",
	"pin removed":    "Removes a pin",
	"note created":   "Adds a note",
	"note updated":   "Saves a note",
	"note deleted":   "Deletes a note",
	"linked":         "Links a journal",
	"unlinked":       "Unlinks a journal",
	"item moved":     "Moves an item",
}

// webVerbs are the labels for the edit in Chronicle behind a change Foundry
// applied.
var webVerbs = map[string]string{
	"page created": "Creates a page",
	"page updated": "Saves the page",
	"page deleted": "Deletes the page",
	"date moved":   "Sets the date",
}

// appliedWords are the labels for what Foundry did with a change, by kind.
var appliedWords = map[string]string{
	"page":              "Journal updated",
	"character":         "Actor updated",
	"map":               "Map updated",
	"note":              "Note updated",
	"stash":             "Inventory updated",
	historyKindCalendar: "Calendar moved",
}

func callLabel(ev SyncEvent) string {
	if v, ok := callVerbs[ev.Action]; ok {
		return v
	}
	return capitalise(ev.Action)
}

// answerLabel is the dashed reply Chronicle sends back.
func answerLabel(ev SyncEvent) string {
	if !ev.OK {
		return shortProblem(ev)
	}
	if strings.HasPrefix(ev.Call, "DELETE ") {
		return "Deleted"
	}
	return "Saved"
}

// flowMs reports whether a duration is worth showing on a step.
func flowMs(ms int) bool { return ms > 0 }

// buildCallFlow turns history rows, oldest first, into the drawing.
func buildCallFlow(in flowInput) *CallFlow {
	f := &CallFlow{Admin: in.Admin, OneGroup: in.OneGroup}
	groupIdx := map[string]int{}
	nameCounts := map[string]int{}
	byEvent := map[int64][]*FlowStep{}

	for _, ev := range in.Events {
		gk := flowGroupKey(ev)
		gi, ok := groupIdx[gk]
		if !ok {
			gi = len(f.Groups)
			groupIdx[gk] = gi
			f.Groups = append(f.Groups, FlowGroup{
				N:      gi + 1,
				Colour: gi%flowGroupColours + 1,
				Name:   flowGroupName(ev, in.Admin, nameCounts),
			})
		}
		steps := eventSteps(ev, in)
		for i, s := range steps {
			s.ID = "s" + strconv.FormatInt(ev.ID, 10) + "-" + strconv.Itoa(i)
			s.EventID = ev.ID
			s.At = ev.OccurredAt
			s.Group = gi + 1
			if s.Fail {
				f.Problems++
			}
			// The steps of one row are one exchange: each led to the next.
			if i > 0 {
				prev := steps[i-1]
				prev.Then = append(prev.Then, s.ID)
				s.Cause = append(s.Cause, prev.ID)
			}
		}
		f.Groups[gi].Steps += len(steps)
		byEvent[ev.ID] = steps
		f.Steps = append(f.Steps, steps...)
	}

	// Rows about the same thing close together are one story: the last
	// step of one led to the first step of the next.
	lastOf := map[int]*FlowStep{}
	lastAt := map[int]time.Time{}
	for _, ev := range in.Events {
		steps := byEvent[ev.ID]
		if len(steps) == 0 {
			continue
		}
		g := steps[0].Group
		if prev, ok := lastOf[g]; ok && ev.OccurredAt.Sub(lastAt[g]) <= flowChainGap {
			first := steps[0]
			prev.Then = append(prev.Then, first.ID)
			first.Cause = append(first.Cause, prev.ID)
		}
		lastOf[g] = steps[len(steps)-1]
		lastAt[g] = ev.OccurredAt
	}

	// A problem fixed later says by whom and when.
	for i, s := range f.Steps {
		if s.Problem == nil {
			continue
		}
		for _, later := range f.Steps[i+1:] {
			if later.Group == s.Group && !later.Fail && later.From == laneBrowser {
				s.Problem.FixedBy, s.Problem.FixedByWho, s.Problem.FixedByAt = later.ID, later.Who, later.At
				break
			}
		}
	}

	byID := make(map[string]*FlowStep, len(f.Steps))
	for _, s := range f.Steps {
		byID[s.ID] = s
	}
	for _, s := range f.Steps {
		for _, id := range s.Cause {
			s.Links = append(s.Links, FlowLink{ID: id, Kind: "Caused by", At: byID[id].At, Label: byID[id].Label})
		}
		for _, id := range s.Then {
			s.Links = append(s.Links, FlowLink{ID: id, Kind: "Led to", At: byID[id].At, Label: byID[id].Label})
		}
	}

	if len(f.Steps) > 0 {
		f.First, f.Last = f.Steps[0].At, f.Steps[len(f.Steps)-1].At
	}
	if steps := byEvent[in.Selected]; len(steps) > 0 {
		f.Selected = steps[len(steps)-1].ID
		for _, s := range steps {
			if s.Fail {
				f.Selected = s.ID
				break
			}
		}
	}

	var lastGroup int
	var prevAt time.Time
	for _, s := range f.Steps {
		if s.Group != lastGroup {
			head := &FlowHead{Group: f.Groups[s.Group-1]}
			if !prevAt.IsZero() {
				head.Gap = s.At.Sub(prevAt)
			}
			f.Rows = append(f.Rows, FlowRow{Head: head})
			lastGroup = s.Group
		}
		prevAt = s.At
		f.Rows = append(f.Rows, FlowRow{Step: s})
	}
	return f
}

// eventSteps draws one history row as its arrows.
func eventSteps(ev SyncEvent, in flowInput) []*FlowStep {
	subject := flowSubject(ev, in.Admin)
	recorded := "Chronicle"
	if ev.ReportedBy == reportedByClient {
		recorded = "Foundry"
	}
	ms, showMs := ev.DurationMs, flowMs(ev.DurationMs)

	switch {
	case ev.Direction == DirToChronicle && ev.ReportedBy == reportedByChronicle:
		call := &FlowStep{
			From: laneFoundry, To: laneServer, Line: lineToChronicle,
			Label: callLabel(ev), Who: ev.UserName, Code: ev.Call, Recorded: recorded,
			What: callWhat(ev, subject),
		}
		answer := &FlowStep{
			From: laneServer, To: laneFoundry, Line: lineToFoundry, Reply: true,
			Label: answerLabel(ev), Code: ev.Status, Result: ev.Status, Ms: ms, ShowMs: showMs,
			Recorded: recorded, Fail: !ev.OK,
		}
		if ev.APIKeyID != nil {
			if k, ok := in.Keys[*ev.APIKeyID]; ok {
				call.Key, answer.Key = &k, &k
			}
		}
		if ev.OK {
			answer.What = "Chronicle accepted it."
			if strings.HasPrefix(ev.Call, "DELETE ") {
				answer.What = "Chronicle deleted it."
			}
		} else {
			answer.Line = lineBad
			if ev.Message != "" {
				answer.Code = ev.Status + " " + ev.Message
			}
			answer.Problem = explainProblem(ev, answer.Key, in)
			answer.What = answer.Problem.What
		}
		return []*FlowStep{call, answer}

	case ev.Direction == DirToFoundry && !isConnectionKind(ev.Kind):
		var steps []*FlowStep
		if ev.UserName != "" {
			label := webVerbs[ev.Action]
			if label == "" {
				label = "Edits on the web"
			}
			steps = append(steps, &FlowStep{
				From: laneBrowser, To: laneServer, Line: lineWeb,
				Label: label, Who: ev.UserName, Code: "edit on the web", Result: "saved", Recorded: "Chronicle",
				What: fmt.Sprintf("%s changed %s in Chronicle.", ev.UserName, subject),
			})
		}
		tell := "Tells Foundry it changed"
		if ev.Kind == historyKindCalendar {
			tell = "Tells Foundry"
		}
		steps = append(steps, &FlowStep{
			From: laneServer, To: laneFoundry, Line: lineToFoundry,
			Label: tell, Code: strings.TrimPrefix(ev.Call, "ws "), Result: "sent", Recorded: recorded,
			What: fmt.Sprintf("Chronicle told Foundry that %s changed.", subject),
		})
		applied := &FlowStep{
			From: laneFoundry, To: laneFoundry, Line: lineToFoundry,
			Label: appliedLabel(ev), Code: "applied in Foundry", Result: ev.Status, Ms: ms, ShowMs: showMs,
			Recorded: recorded, Fail: !ev.OK,
			What: "Foundry applied the change.",
		}
		if !ev.OK {
			applied.Line = lineBad
			applied.Label = "Couldn’t apply it"
			applied.Problem = explainProblem(ev, nil, in)
			applied.What = applied.Problem.What
			if !in.Admin && ev.Message != "" {
				applied.Code = ev.Message
			}
		}
		return append(steps, applied)
	}

	// Connects, pulls, links and the module's own notes happen in Foundry.
	step := &FlowStep{
		From: laneFoundry, To: laneFoundry, Line: lineToFoundry,
		Label: capitalise(ev.Action), Code: strings.TrimPrefix(ev.Call, "foundry "), Result: ev.Status,
		Ms: ms, ShowMs: showMs, Recorded: recorded, Fail: !ev.OK,
		What: "Foundry: " + ev.Action + ".",
	}
	if !in.Admin && ev.ResourceName != "" && isConnectionKind(ev.Kind) {
		step.What = ev.ResourceName
	}
	if !ev.OK {
		step.Line = lineBad
		step.Problem = explainProblem(ev, nil, in)
		step.What = step.Problem.What
	}
	return []*FlowStep{step}
}

func isConnectionKind(kind string) bool {
	switch kind {
	case "connection", "sync", "link", "":
		return true
	}
	return false
}

func appliedLabel(ev SyncEvent) string {
	if ev.Kind == historyKindCalendar && strings.HasPrefix(ev.Action, "event") {
		return "Calendar event updated"
	}
	if w, ok := appliedWords[ev.Kind]; ok {
		return w
	}
	return "Applied in Foundry"
}

func callWhat(ev SyncEvent, subject string) string {
	if ev.Kind == historyKindCalendar && ev.Action == "date set" && ev.ResourceName != "" {
		return fmt.Sprintf("Foundry asked Chronicle to move the calendar to %s.", ev.ResourceName)
	}
	return fmt.Sprintf("Foundry sent a change to %s to Chronicle (%s).", subject, ev.Action)
}

// flowSort orders rows oldest first, a run's steps right after it.
func flowSort(events []SyncEvent) {
	sort.SliceStable(events, func(i, j int) bool {
		if !events[i].OccurredAt.Equal(events[j].OccurredAt) {
			return events[i].OccurredAt.Before(events[j].OccurredAt)
		}
		return events[i].ID < events[j].ID
	})
}

// flowPick keeps the rows about the chosen thing (all of them when key is
// empty), at most flowMaxEvents, the closest to the centre.
func flowPick(events []SyncEvent, key string, centre time.Time) []SyncEvent {
	out := make([]SyncEvent, 0, len(events))
	for _, ev := range events {
		if key == "" || flowGroupKey(ev) == key {
			out = append(out, ev)
		}
	}
	if len(out) > flowMaxEvents {
		sort.SliceStable(out, func(i, j int) bool {
			return absDur(out[i].OccurredAt.Sub(centre)) < absDur(out[j].OccurredAt.Sub(centre))
		})
		out = out[:flowMaxEvents]
	}
	flowSort(out)
	return out
}

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
