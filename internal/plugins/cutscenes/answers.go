package cutscenes

import (
	"sort"
	"time"
)

// What became of an answer. Only a waiting answer can be decided, once.
const (
	AnswerWaiting  = "waiting"
	AnswerAdded    = "added"    // joined the story as the player wrote it
	AnswerChanged  = "changed"  // joined the story in the Director's words
	AnswerDeclined = "declined" // turned down; it never joins the story
)

// What the Director can do with a waiting answer, by its form value.
const (
	ResolveAdd     = "add"
	ResolveChange  = "change"
	ResolveDecline = "decline"
)

// MaxAnswer is the longest answer a player can write, and the longest line
// the Director can put in its place.
const MaxAnswer = 280

// Answer is what one viewer answered at one moment of a showing. Text is
// what they sent and stays as they wrote it; Line is what joined the story,
// on PageID, once the Director added it.
type Answer struct {
	ID         string
	ShowingID  string
	Moment     int
	UserID     string
	Name       string // filled by the service from the member list
	Way        string
	Text       string
	Status     string
	Line       string
	PageID     string
	CreatedAt  time.Time
	ResolvedAt *time.Time
}

// InStory reports whether the answer joined the story.
func (a Answer) InStory() bool { return a.Status == AnswerAdded || a.Status == AnswerChanged }

// AnswerInput is what a viewer sends at a moment.
type AnswerInput struct {
	Way  string
	Text string
}

// ResolveInput is what the Director decides about one answer. Line is the
// changed words (change only); Page names the page the line joins, by id or
// by its exact name.
type ResolveInput struct {
	Action string
	Line   string
	Page   string
}

// MyAnswer is a viewer's own answer as their screen reads it, so a screen
// that opens mid-hold shows what they already sent. What the Director did
// with it is not theirs to read here: a line that joined the story reaches
// them when the scene goes on.
type MyAnswer struct {
	Moment int    `json:"moment"`
	Way    string `json:"way"`
	Text   string `json:"text"`
}

// ShownLine is a line that joined the story at a moment, as every viewer's
// screen shows it when the scene goes on.
type ShownLine struct {
	Line string `json:"line"`
	From string `json:"from"`
}

// PageRef is a page the Director can add a line to.
type PageRef struct {
	ID   string
	Name string
}

// momentOrder lists a scene's moment indexes in the order the scene reaches
// them: by time, then by list order.
func momentOrder(ms []Moment) []int {
	out := make([]int, len(ms))
	for i := range ms {
		out[i] = i
	}
	sort.SliceStable(out, func(a, b int) bool { return ms[out[a]].At < ms[out[b]].At })
	return out
}

// IsReleased reports whether the Director let moment i go.
func (s Showing) IsReleased(i int) bool { return i >= 0 && i < 31 && s.Released&(1<<uint(i)) != 0 }

// Current is the moment the showing holds on, or waits to reach: the first
// one, in the scene's order, the Director has not let go. -1 when there is
// none.
func (s Showing) Current() int {
	for _, i := range momentOrder(s.Scene.Moments) {
		if !s.IsReleased(i) {
			return i
		}
	}
	return -1
}

// Holding reports whether a moment of the showing still waits on the
// Director: not stopped, inside its window, with a moment not let go.
func (s Showing) Holding(now time.Time) bool {
	return s.StoppedAt == nil && now.Before(s.RewatchUntil) && s.Current() >= 0
}

// ReleasedList is the moments let go, for a screen that opens late.
func (s Showing) ReleasedList() []int {
	var out []int
	for i := range s.Scene.Moments {
		if s.IsReleased(i) {
			out = append(out, i)
		}
	}
	return out
}

// LiveMoment is one live showing of a scene as the Director's panel shows
// it: the moment it holds on, with the Director's own note, and what the
// viewers answered there.
type LiveMoment struct {
	Showing Showing
	Current int    // -1 once every moment is let go
	Moment  Moment // the current moment; zero when Current is -1
	Answers []Answer
	Pages   []PageRef
}

// Waiting counts the answers the Director has not decided yet.
func (l LiveMoment) Waiting() int {
	n := 0
	for _, a := range l.Answers {
		if a.Status == AnswerWaiting {
			n++
		}
	}
	return n
}
