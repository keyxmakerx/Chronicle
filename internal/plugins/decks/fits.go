package decks

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// Judging a card's tests against what the campaign holds now. A fact that
// cannot be read is a missing fact and a test on it does not hold, so a failing
// source can never make a card come up.

// Fit is the verdict on one card.
type Fit struct {
	// OK is true when every test holds. A card with no test always fits.
	OK bool
	// Subject is the hero the "any" tests hold for; "" when the card has none.
	Subject string
	// Reading says what the first number or tracker test is now.
	Reading Reading
	// Why is the first test that fails, as a sentence with the current value.
	Why string
}

// judge reads each fact once per run, so judging every card of a deck asks for
// a page's numbers a single time.
type judge struct {
	ctx   context.Context
	cid   string
	facts Facts
	deck  Deck
	// party lists the heroes an "any" test may be about, read when first needed.
	party   func() ([]Hero, error)
	partyV  []Hero
	partyOK bool
	names   *namer

	nums   map[string]numberRead
	trks   map[string]trackerRead
	tags   map[string]tagRead
	lays   map[string]layerRead
	warned bool

	// todayAs is who the calendar is read as: the Director, except for a
	// player's own draw, which judges the calendar as that player sees it.
	todayAs    Viewer
	todayV     TodayFact
	todayErr   error
	todayTried bool
}

type numberRead struct {
	set NumberSet
	err error
}
type trackerRead struct {
	list []TrackerFact
	err  error
}
type tagRead struct {
	list []TagFact
	err  error
}
type layerRead struct {
	list []LayerFact
	err  error
}

func newJudge(ctx context.Context, cid string, f Facts, d Deck, party func() ([]Hero, error), names *namer) *judge {
	return &judge{ctx: ctx, cid: cid, facts: f, deck: d, party: party, names: names, todayAs: judgeDirector,
		nums: map[string]numberRead{}, trks: map[string]trackerRead{}, tags: map[string]tagRead{}, lays: map[string]layerRead{}}
}

// failed logs a read failure once per run; a plugin that is simply not running
// is quiet.
func (j *judge) failed(what string, err error) {
	if err == nil || errors.Is(err, ErrUnavailable) || j.warned {
		return
	}
	j.warned = true
	slog.Warn("decks: a fact could not be read, so tests on it do not hold",
		slog.String("campaign_id", j.cid), slog.String("fact", what), slog.Any("error", err))
}

func (j *judge) numbers(page string) (NumberSet, error) {
	if r, ok := j.nums[page]; ok {
		return r.set, r.err
	}
	var r numberRead
	if j.facts == nil {
		r.err = ErrUnavailable
	} else {
		r.set, r.err = j.facts.Numbers(j.ctx, j.cid, page)
		j.failed("numbers", r.err)
	}
	j.nums[page] = r
	return r.set, r.err
}

func (j *judge) trackers(page string) ([]TrackerFact, error) {
	if r, ok := j.trks[page]; ok {
		return r.list, r.err
	}
	var r trackerRead
	if j.facts == nil {
		r.err = ErrUnavailable
	} else {
		r.list, r.err = j.facts.Trackers(j.ctx, j.cid, page)
		j.failed("trackers", r.err)
	}
	j.trks[page] = r
	return r.list, r.err
}

func (j *judge) tagsOf(page string) ([]TagFact, error) {
	if r, ok := j.tags[page]; ok {
		return r.list, r.err
	}
	var r tagRead
	if j.facts == nil {
		r.err = ErrUnavailable
	} else {
		r.list, r.err = j.facts.Tags(j.ctx, j.cid, page)
		j.failed("tags", r.err)
	}
	j.tags[page] = r
	return r.list, r.err
}

func (j *judge) layersOf(page string) ([]LayerFact, error) {
	if r, ok := j.lays[page]; ok {
		return r.list, r.err
	}
	var r layerRead
	if j.facts == nil {
		r.err = ErrUnavailable
	} else {
		r.list, r.err = j.facts.Layers(j.ctx, j.cid, page)
		j.failed("layers", r.err)
	}
	j.lays[page] = r
	return r.list, r.err
}

// heroes is the party, in a fixed order (name, then id) so which hero is the
// subject never depends on the order a store returned them in.
func (j *judge) heroes() []Hero {
	if !j.partyOK {
		j.partyOK = true
		if j.party != nil {
			list, err := j.party()
			j.failed("party", err)
			if err == nil {
				j.partyV = append([]Hero{}, list...)
				sort.SliceStable(j.partyV, func(a, b int) bool {
					if j.partyV[a].Name != j.partyV[b].Name {
						return j.partyV[a].Name < j.partyV[b].Name
					}
					return j.partyV[a].ID < j.partyV[b].ID
				})
			}
		}
	}
	return j.partyV
}

// today is the main calendar as todayAs sees it, read once per run.
func (j *judge) today() (TodayFact, error) {
	if !j.todayTried {
		j.todayTried = true
		if j.facts == nil {
			j.todayErr = ErrUnavailable
		} else {
			j.todayV, j.todayErr = j.facts.Today(j.ctx, j.cid, j.todayAs)
			j.failed("today", j.todayErr)
		}
	}
	return j.todayV, j.todayErr
}

// compare applies an operator with a hair of tolerance, so a third of 30 holds
// "exactly 10".
func compare(op string, a, b float64) bool {
	const eps = 1e-9
	switch op {
	case "lte":
		return a <= b+eps
	case "lt":
		return a < b-eps
	case "gte":
		return a >= b-eps
	case "gt":
		return a > b+eps
	case "eq":
		return math.Abs(a-b) <= eps
	}
	return false
}

// verdict is one test judged for one page.
type verdict struct {
	holds   bool
	why     string
	reading Reading
	named   bool // reading is set
}

// test judges one test about one page. page is "" for a test with no page.
func (j *judge) test(t WhenTest, page string, card Card) verdict {
	switch t.Test {
	case TestSay:
		if card.SaidAt != nil {
			return verdict{holds: true}
		}
		return verdict{why: "the Director has not said so yet"}
	case TestNumber:
		return j.number(t, page)
	case TestTracker:
		return j.tracker(t, page)
	case TestTag:
		return j.tag(t, page)
	case TestLayer:
		return j.layer(t, page)
	case TestMoon, TestEvent, TestTime:
		return j.calendar(t)
	}
	return verdict{why: "a test this server does not know"}
}

// calendar judges a moon, event or time test against today. A calendar that
// can't be read, a moon that is not on it, events that can't be read and a
// time of day the calendar can't say are all missing facts, so the test does
// not hold in either its plain or its "not" form.
func (j *judge) calendar(t WhenTest) verdict {
	td, err := j.today()
	if err != nil {
		return verdict{why: "the calendar can't be read"}
	}
	switch t.Test {
	case TestMoon:
		m, ok := td.Moons[calendarKey(t.Name)]
		if !ok {
			return verdict{why: j.names.moon(t.Name) + " is not on the calendar"}
		}
		why := j.names.moon(t.Name) + " is " + moonNowWords(m)
		return verdict{holds: moonMatches(m, t.Phase, t.From, t.To) == (t.Is != nil && *t.Is), why: why}
	case TestEvent:
		if td.Events == nil {
			return verdict{why: "today's events can't be read"}
		}
		on := td.Events[calendarKey(t.Name)]
		why := t.Name + " is not today"
		if on {
			why = t.Name + " is today"
		}
		return verdict{holds: on == (t.Has != nil && *t.Has), why: why}
	case TestTime:
		if td.DayPart == "" {
			return verdict{why: "the calendar can't say the time of day"}
		}
		is := strings.EqualFold(td.DayPart, t.Time)
		return verdict{holds: is == (t.Is != nil && *t.Is), why: "it is " + td.DayPart}
	}
	return verdict{why: "a test this server does not know"}
}

// moonNowWords says a moon's state now for a reason line: "Waxing Gibbous,
// day 12 of its cycle".
func moonNowWords(m MoonFact) string {
	s := "in no phase"
	if len(m.Words) > 0 {
		s = m.Words[0]
	}
	if m.Day > 0 {
		s += ", day " + strconv.Itoa(m.Day) + " of its cycle"
	}
	return s
}

// moonMatches is the moon matching worked-out sheet fields use (a field that
// follows `moon:<name>`), kept here because plugins never import each other:
// a phase matches any of the moon's words today, ignoring case; a range counts
// days of the cycle and may wrap (23 to 1). A hand-set moon has words but no
// days, so a range never matches it.
func moonMatches(m MoonFact, phase string, from, to *int) bool {
	if phase != "" {
		for _, w := range m.Words {
			if strings.EqualFold(strings.TrimSpace(phase), w) {
				return true
			}
		}
		return false
	}
	if m.Day == 0 || from == nil || to == nil {
		return false
	}
	d, f, e := m.Day, *from, *to
	if f > e {
		return d >= f || d <= e
	}
	return d >= f && d <= e
}

func (j *judge) number(t WhenTest, page string) verdict {
	who := possessive(j.names.who(t.Who))
	label := j.names.fieldLabel(t.Who, t.Field)
	missing := verdict{why: who + " " + label + " can't be read"}
	set, err := j.numbers(page)
	if err != nil || !set.Sheet {
		return missing
	}
	a, ok := set.Values[t.Field]
	if !ok {
		return missing
	}
	var b float64
	text := who + " " + label + " is " + fmtNum(a)
	if t.Value != nil {
		b = *t.Value
	} else {
		of, ok := set.Values[t.Of]
		if !ok || of == 0 {
			return verdict{why: who + " " + label + " can't be compared: " + j.names.fieldLabel(t.Who, t.Of) + " is missing or zero"}
		}
		num, den, ok := shareRatio(t.Share)
		if !ok {
			return missing
		}
		b = of * num / den
		text += " of " + fmtNum(of)
	}
	v := verdict{holds: compare(t.Op, a, b), why: text, named: true,
		reading: Reading{Text: text, Source: set.Sources[t.Field], At: set.ChangedAt}}
	return v
}

func (j *judge) tracker(t WhenTest, page string) verdict {
	who := possessive(j.names.who(t.Who))
	name := j.names.trackerName(t.Kind)
	list, err := j.trackers(page)
	if err != nil {
		return verdict{why: who + " " + name + " can't be read"}
	}
	for _, k := range list {
		if k.KindID != t.Kind {
			continue
		}
		text := who + " " + name + " is at " + fmtNum(float64(k.Value))
		want := 0.0
		if t.Value != nil {
			want = *t.Value
		}
		return verdict{holds: compare(t.Op, float64(k.Value), want), why: text, named: true, reading: Reading{Text: text}}
	}
	return verdict{why: who + " " + name + " is not on the page"}
}

func (j *judge) tag(t WhenTest, page string) verdict {
	list, err := j.tagsOf(page)
	who := j.names.who(t.Who)
	tag := j.names.tagName(intOf(t.Tag))
	if err != nil {
		return verdict{why: "the tags of " + who + " can't be read"}
	}
	has := false
	for _, g := range list {
		has = has || g.ID == intOf(t.Tag)
	}
	want := t.Has != nil && *t.Has
	why := who + " does not have the tag " + tag
	if has {
		why = who + " has the tag " + tag
	}
	return verdict{holds: has == want, why: why}
}

func (j *judge) layer(t WhenTest, page string) verdict {
	list, err := j.layersOf(page)
	who := j.names.who(t.Who)
	if err != nil {
		return verdict{why: "the layers on " + who + " can't be read"}
	}
	on := false
	for _, l := range list {
		on = on || strings.EqualFold(l.Name, t.Name) || (l.Source != "" && strings.EqualFold(l.Source, t.Name))
	}
	want := t.On != nil && *t.On
	why := who + " is not " + t.Name
	if on {
		why = who + " is " + t.Name
	}
	return verdict{holds: on == want, why: why}
}

// pageFor resolves a test's "who" for one candidate hero.
func (j *judge) pageFor(t WhenTest, hero string) string {
	switch t.Who {
	case WhoAny:
		return hero
	case WhoHolder:
		return j.deck.HolderEntityID
	}
	return t.Who
}

// fits judges a card. A card with no test fits. All "any" tests must hold for
// the same hero; the first hero (by name) for whom every test holds is the
// subject.
func (j *judge) fits(card Card) Fit {
	if len(card.When) == 0 {
		return Fit{OK: true}
	}
	usesAny := false
	for _, t := range card.When {
		usesAny = usesAny || (t.Test != TestSay && t.Who == WhoAny)
	}
	if !usesAny {
		return j.fitsFor(card, "")
	}
	heroes := j.heroes()
	if len(heroes) == 0 {
		return Fit{Why: "there is no hero in the party yet"}
	}
	var first Fit
	for i, h := range heroes {
		f := j.fitsFor(card, h.ID)
		if f.OK {
			f.Subject = h.ID
			return f
		}
		if i == 0 {
			first = f
		}
	}
	return first
}

// fitsFor judges every test of the card about one candidate hero.
func (j *judge) fitsFor(card Card, hero string) Fit {
	out := Fit{OK: true}
	for _, t := range card.When {
		v := j.test(t, j.pageFor(t, hero), card)
		if v.named && out.Reading.Text == "" {
			out.Reading = v.reading
		}
		if !v.holds && out.OK {
			out.OK, out.Why = false, v.why
		}
	}
	return out
}

// ---- service helpers ---------------------------------------------------------

// judgeFor builds a judge for one deck, as a Director would read the facts.
func (s *service) judgeFor(ctx context.Context, cid string, d Deck) *judge {
	return newJudge(ctx, cid, s.facts, d, func() ([]Hero, error) {
		return s.heroes.Party(ctx, cid, judgeDirector)
	}, s.newNamer(ctx, cid, d, judgeDirector))
}

// judgeDirector is who a deck's tests are judged as.
var judgeDirector = Viewer{Director: true, MemberRole: permissions.RoleOwner, VisibilityRole: permissions.RoleOwner}

// cardsWithTests reports whether any card of the list has a test.
func cardsWithTests(cards []Card) bool {
	for _, c := range cards {
		if len(c.When) > 0 {
			return true
		}
	}
	return false
}

// fittingCards says which of the deck's cards fit now, for a Director's draw:
// a card with no test always fits. nil means nothing is restricted (no card has
// a test), so the draw does not even read the facts.
func (s *service) fittingCards(ctx context.Context, cid string, d Deck, cards []Card) map[string]bool {
	if !cardsWithTests(cards) {
		return nil
	}
	j := s.judgeFor(ctx, cid, d)
	out := make(map[string]bool, len(cards))
	for _, c := range cards {
		if len(c.When) == 0 || j.fits(c).OK {
			out[c.ID] = true
		}
	}
	return out
}

// eligibleForPlayer says which cards a player's own draw may pull: a card with
// no test, or one whose tests are all printed and all hold. A card with any
// test the Director kept to themselves is never in it, whether or not that test
// holds, and its facts are not even read, so what a player is sent cannot
// depend on a hidden fact. A printed test about a page this player cannot open
// keeps the card out the same way: the draw would otherwise say how that page
// stands. The calendar is read as this player sees it, so a moon hidden from
// them is a missing fact (the card stays out) and an event they cannot see is
// not today: the draw never says how a hidden moon or event stands.
func (s *service) eligibleForPlayer(ctx context.Context, cid string, d Deck, cards []Card, v Viewer) map[string]bool {
	if !cardsWithTests(cards) {
		return nil
	}
	j := s.judgeFor(ctx, cid, d)
	j.todayAs = v
	out := make(map[string]bool, len(cards))
	for _, c := range cards {
		switch {
		case len(c.When) == 0:
			out[c.ID] = true
		case hasHidden(c.When) || !s.pagesOpenTo(ctx, cid, c.When, v):
		case j.fits(c).OK:
			out[c.ID] = true
		}
	}
	return out
}

// pagesOpenTo says whether the viewer may open every page the tests name by
// id. A page that cannot be asked about counts as closed.
func (s *service) pagesOpenTo(ctx context.Context, cid string, tests []WhenTest, v Viewer) bool {
	for _, t := range tests {
		if t.Who == "" || t.Who == WhoHolder || t.Who == WhoAny {
			continue
		}
		ok, err := s.heroes.Viewable(ctx, cid, t.Who, v)
		if err != nil || !ok {
			return false
		}
	}
	return true
}

// fittingWithTests says which cards that have tests fit now, hidden tests
// included, with the verdict for each. It is what a moment and an invited draw
// pick among.
func (s *service) fittingWithTests(ctx context.Context, cid string, d Deck, cards []Card) map[string]Fit {
	out := map[string]Fit{}
	var j *judge
	for _, c := range cards {
		if len(c.When) == 0 {
			continue
		}
		if j == nil {
			j = s.judgeFor(ctx, cid, d)
		}
		if f := j.fits(c); f.OK {
			out[c.ID] = f
		}
	}
	return out
}
