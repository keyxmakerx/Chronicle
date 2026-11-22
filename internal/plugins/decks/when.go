package decks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// A card says when it can come up: up to MaxTests plain tests that must all
// hold. There are no formulas and no "any of". Each test is either printed on
// the card (anyone who can read the face reads it) or the Director's own and
// never sent to a player.

// Kinds of test.
const (
	TestNumber  = "number"
	TestTracker = "tracker"
	TestTag     = "tag"
	TestLayer   = "layer"
	TestSay     = "say"
)

// Who a test is about. Anything else is a page id of the campaign.
const (
	// WhoHolder is the hero a hero deck is for.
	WhoHolder = "holder"
	// WhoAny is any hero of the campaign; every "any" test of one card must hold
	// for the same hero, which is the moment's subject.
	WhoAny = "any"
)

// Limits of a card's tests.
const (
	MaxTests          = 4
	MaxLayerNameChars = 80
	// maxTestNumber keeps a typed number to something every comparison can hold.
	maxTestNumber = 1e12
)

// WhenTest is one test. Only the keys of its kind are used; ParseWhen refuses
// any other.
type WhenTest struct {
	Test string `json:"test"`
	// Open says the test is printed on the card.
	Open bool   `json:"open"`
	Who  string `json:"who,omitempty"`
	// Field, Op, Value, Of and Share are a number test; a tracker test uses Op and
	// Value too.
	Field string   `json:"field,omitempty"`
	Op    string   `json:"op,omitempty"`
	Value *float64 `json:"value,omitempty"`
	Of    string   `json:"of,omitempty"`
	Share string   `json:"share,omitempty"`
	// Kind is a tracker kind id.
	Kind string `json:"kind,omitempty"`
	// Tag is a tag id and Has whether the page has it.
	Tag *int  `json:"tag,omitempty"`
	Has *bool `json:"has,omitempty"`
	// Name is a layer's name or source and On whether it is on the page.
	Name string `json:"name,omitempty"`
	On   *bool  `json:"on,omitempty"`
}

// WhenView is a test as a Director reads it back: the test, its sentence and
// whether every name in it still resolves.
type WhenView struct {
	WhenTest
	Sentence string `json:"sentence"`
	// Broken is true when a hero, field, tracker kind or tag the test names no
	// longer exists; such a test can never hold.
	Broken bool `json:"broken"`
}

// Comparison operators and their words.
var (
	opPhrases = []OptionWord{
		{"lte", "at or below"}, {"lt", "below"}, {"gte", "at or above"}, {"gt", "above"}, {"eq", "exactly"},
	}
	sharePhrases = []OptionWord{
		{"quarter", "a quarter of"}, {"third", "a third of"}, {"half", "half of"},
		{"two_thirds", "two thirds of"}, {"three_quarters", "three quarters of"}, {"all", ""},
	}
	testKindWords = []OptionWord{
		{TestNumber, "a number on the page"}, {TestTracker, "a tracker on the page"}, {TestTag, "a tag on the page"},
		{TestLayer, "a transformation on the page"}, {TestSay, "the Director says so"},
	}
)

func opPhrase(op string) string {
	for _, o := range opPhrases {
		if o.Key == op {
			return o.Phrase
		}
	}
	return ""
}

func validOp(op string) bool { return opPhrase(op) != "" }

// shareRatio is a share as a whole-number ratio, so a third of 30 is exactly 10.
func shareRatio(share string) (num, den float64, ok bool) {
	switch share {
	case "quarter":
		return 1, 4, true
	case "third":
		return 1, 3, true
	case "half":
		return 1, 2, true
	case "two_thirds":
		return 2, 3, true
	case "three_quarters":
		return 3, 4, true
	case "all":
		return 1, 1, true
	}
	return 0, 0, false
}

func sharePhrase(share string) string {
	for _, o := range sharePhrases {
		if o.Key == share {
			return o.Phrase
		}
	}
	return ""
}

// keysOf is the keys a kind of test may carry besides "test" and "open".
var keysOf = map[string][]string{
	TestNumber:  {"who", "field", "op", "value", "of", "share"},
	TestTracker: {"who", "kind", "op", "value"},
	TestTag:     {"who", "tag", "has"},
	TestLayer:   {"who", "name", "on"},
	TestSay:     {},
}

// WhenList is a card's tests as they arrive in a request. Its decoder refuses
// an unknown key or kind by name, so a typo is never stored as a test that
// quietly does nothing.
type WhenList []WhenTest

// UnmarshalJSON accepts a list of tests, and "", null or [] for none.
func (w *WhenList) UnmarshalJSON(b []byte) error {
	tests, err := ParseWhen(b)
	if err != nil {
		return err
	}
	*w = tests
	return nil
}

// ParseWhen reads a list of tests strictly: every test has a known kind and
// only the keys of that kind. It checks the shape, not the names; ValidateWhen
// checks those against the campaign.
func ParseWhen(raw []byte) ([]WhenTest, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) || bytes.Equal(raw, []byte(`""`)) {
		return nil, nil
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, errInvalid("a card's tests must be a list")
	}
	if len(items) > MaxTests {
		return nil, errInvalid(fmt.Sprintf("a card can have at most %d tests", MaxTests))
	}
	out := make([]WhenTest, 0, len(items))
	for i, item := range items {
		n := i + 1
		var kind string
		if v, ok := item["test"]; !ok || json.Unmarshal(v, &kind) != nil || kind == "" {
			return nil, errInvalid(fmt.Sprintf(`test %d needs a "test" saying what kind it is`, n))
		}
		allowed, known := keysOf[kind]
		if !known {
			return nil, errInvalid(fmt.Sprintf("test %d has an unknown kind %q", n, kind))
		}
		keys := make([]string, 0, len(item))
		for k := range item {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if k == "test" || k == "open" || contains(allowed, k) {
				continue
			}
			return nil, errInvalid(fmt.Sprintf("test %d (%s) has an unknown key %q", n, kind, k))
		}
		var t WhenTest
		one, err := json.Marshal(item)
		if err != nil {
			return nil, errInvalid(fmt.Sprintf("test %d can't be read", n))
		}
		if err := json.Unmarshal(one, &t); err != nil {
			var te *json.UnmarshalTypeError
			if errors.As(err, &te) && te.Field != "" {
				return nil, errInvalid(fmt.Sprintf("test %d: %q has the wrong kind of value", n, te.Field))
			}
			return nil, errInvalid(fmt.Sprintf("test %d can't be read", n))
		}
		out = append(out, t)
	}
	return out, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// derivedOpen is true when at least one test is printed on the card.
func derivedOpen(tests []WhenTest) bool {
	for _, t := range tests {
		if t.Open {
			return true
		}
	}
	return false
}

// hasHidden is true when a test is the Director's own.
func hasHidden(tests []WhenTest) bool {
	for _, t := range tests {
		if !t.Open {
			return true
		}
	}
	return false
}

func encodeWhen(tests []WhenTest) string {
	if len(tests) == 0 {
		return ""
	}
	b, err := json.Marshal(tests)
	if err != nil {
		// A plain struct of strings, numbers and bools cannot fail to marshal.
		return ""
	}
	return string(b)
}

// decodeWhen reads a stored list. A stored list that no longer parses reads as
// none: the card then has no test, which only ever lets a card be drawn that a
// Director meant to hold back, so the repository never refuses to list a deck
// over it. The save path is strict and cannot write one.
func decodeWhen(stored string) []WhenTest {
	if stored == "" {
		return nil
	}
	var out []WhenTest
	if err := json.Unmarshal([]byte(stored), &out); err != nil {
		return nil
	}
	if len(out) > MaxTests {
		out = out[:MaxTests]
	}
	return out
}

// ---- the one check ----------------------------------------------------------

// ValidateWhen checks a card's tests against the campaign and returns them
// normalized. CreateCard, UpdateCard and, later, the AI call this one function.
// Its messages are for a Director and name the part that is wrong.
func (s *service) ValidateWhen(ctx context.Context, campaignID string, d Deck, tests []WhenTest) ([]WhenTest, error) {
	if len(tests) > MaxTests {
		return nil, errInvalid(fmt.Sprintf("a card can have at most %d tests", MaxTests))
	}
	c := s.newTestChecker(ctx, campaignID, d)
	out := make([]WhenTest, 0, len(tests))
	for i, t := range tests {
		n, err := c.check(t)
		if err != nil {
			return nil, errInvalid(fmt.Sprintf("test %d: %s", i+1, err.Error()))
		}
		out = append(out, n)
	}
	return out, nil
}

// BrokenTests says, for a stored list, which tests no longer resolve: the hero,
// field, tracker kind or tag they name is gone. A test that cannot be checked
// because the facts cannot be read is not called broken.
func (s *service) BrokenTests(ctx context.Context, campaignID string, d Deck, tests []WhenTest) []bool {
	out := make([]bool, len(tests))
	if len(tests) == 0 {
		return out
	}
	c := s.newTestChecker(ctx, campaignID, d)
	for i, t := range tests {
		_, err := c.check(t)
		out[i] = err != nil && !errors.Is(err, errCannotCheck)
	}
	return out
}

// errCannotCheck marks a test that could not be checked for a reason that is
// not the Director's: the facts are not wired or cannot be read.
var errCannotCheck = errors.New("the facts cannot be read")

type testChecker struct {
	s      *service
	ctx    context.Context
	cid    string
	deck   Deck
	opts   *WhenOptions
	optErr error
	fields map[string][]NumberField
}

func (s *service) newTestChecker(ctx context.Context, campaignID string, d Deck) *testChecker {
	return &testChecker{s: s, ctx: ctx, cid: campaignID, deck: d, fields: map[string][]NumberField{}}
}

func (c *testChecker) options() (WhenOptions, error) {
	if c.opts == nil && c.optErr == nil {
		if c.s.facts == nil {
			c.optErr = errCannotCheck
		} else if o, err := c.s.facts.Options(c.ctx, c.cid); err != nil {
			c.optErr = fmt.Errorf("%w: %v", errCannotCheck, err)
		} else {
			c.opts = &o
		}
	}
	if c.optErr != nil {
		return WhenOptions{}, c.optErr
	}
	return *c.opts, nil
}

// fieldsFor are the number fields a test's "who" may be asked about.
func (c *testChecker) fieldsFor(who string) ([]NumberField, error) {
	if who == WhoAny {
		o, err := c.options()
		return o.Fields, err
	}
	page := who
	if who == WhoHolder {
		page = c.deck.HolderEntityID
	}
	if f, ok := c.fields[page]; ok {
		return f, nil
	}
	if c.s.facts == nil {
		return nil, errCannotCheck
	}
	f, err := c.s.facts.Fields(c.ctx, c.cid, page)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errCannotCheck, err)
	}
	c.fields[page] = f
	return f, nil
}

func hasField(list []NumberField, key string) bool {
	for _, f := range list {
		if f.Key == key {
			return true
		}
	}
	return false
}

func (c *testChecker) checkWho(t WhenTest) error {
	switch t.Who {
	case "":
		return errors.New(`"who" is needed: "holder", "any" or a page`)
	case WhoAny:
		return nil
	case WhoHolder:
		if c.deck.Kind != KindHero || c.deck.HolderEntityID == "" {
			return errors.New(`"holder" is only for a hero deck that belongs to a hero`)
		}
		return nil
	}
	if !validToken(t.Who) {
		return errors.New("that page is not in this campaign")
	}
	_, ok, err := c.s.heroes.InCampaign(c.ctx, c.cid, t.Who)
	if err != nil {
		return fmt.Errorf("%w: %v", errCannotCheck, err)
	}
	if !ok {
		return errors.New("that page is not in this campaign")
	}
	return nil
}

// check validates one test and returns it normalized: only the keys of its
// kind, names trimmed, and the share filled in.
func (c *testChecker) check(t WhenTest) (WhenTest, error) {
	out := WhenTest{Test: t.Test, Open: t.Open}
	if t.Test == TestSay {
		if t.Who != "" || t.Field != "" || t.Op != "" || t.Value != nil || t.Of != "" || t.Share != "" ||
			t.Kind != "" || t.Tag != nil || t.Has != nil || t.Name != "" || t.On != nil {
			return WhenTest{}, errors.New(`"say" takes no other keys`)
		}
		return out, nil
	}
	if _, known := keysOf[t.Test]; !known {
		return WhenTest{}, fmt.Errorf("unknown kind %q", t.Test)
	}
	if c.s.facts == nil {
		return WhenTest{}, errors.New("this kind of test is not available on this server right now; only \"say\" is")
	}
	if err := c.checkWho(t); err != nil {
		return WhenTest{}, err
	}
	out.Who = t.Who
	switch t.Test {
	case TestNumber:
		return c.checkNumber(t, out)
	case TestTracker:
		return c.checkTracker(t, out)
	case TestTag:
		if t.Tag == nil || t.Has == nil {
			return WhenTest{}, errors.New(`a tag test needs "tag" and "has"`)
		}
		o, err := c.options()
		if err != nil {
			return WhenTest{}, err
		}
		found := false
		for _, g := range o.Tags {
			found = found || g.ID == *t.Tag
		}
		if !found {
			return WhenTest{}, errors.New("that tag does not exist in this campaign")
		}
		tag, has := *t.Tag, *t.Has
		out.Tag, out.Has = &tag, &has
	case TestLayer:
		if t.On == nil {
			return WhenTest{}, errors.New(`a layer test needs "name" and "on"`)
		}
		name, err := plainText("the layer's name", t.Name, MaxLayerNameChars)
		if err != nil {
			return WhenTest{}, err
		}
		if name == "" {
			return WhenTest{}, errors.New("a layer test needs the layer's name")
		}
		on := *t.On
		out.Name, out.On = name, &on
	}
	return out, nil
}

func (c *testChecker) checkNumber(t, out WhenTest) (WhenTest, error) {
	if t.Field == "" || !validOp(t.Op) {
		return WhenTest{}, errors.New(`a number test needs "field" and a "op" of lte, lt, gte, gt or eq`)
	}
	if (t.Value == nil) == (t.Of == "") {
		return WhenTest{}, errors.New(`a number test compares to either "value" or "of", not both and not neither`)
	}
	if t.Value != nil && (math.IsNaN(*t.Value) || math.Abs(*t.Value) > maxTestNumber) {
		return WhenTest{}, errors.New(`"value" is not a usable number`)
	}
	if t.Of == "" && t.Share != "" {
		return WhenTest{}, errors.New(`"share" goes with "of"`)
	}
	fields, err := c.fieldsFor(t.Who)
	if err != nil {
		return WhenTest{}, err
	}
	if !hasField(fields, t.Field) {
		return WhenTest{}, errors.New("that is not a number field of the page")
	}
	out.Field, out.Op = t.Field, t.Op
	if t.Value != nil {
		v := *t.Value
		out.Value = &v
		return out, nil
	}
	if !hasField(fields, t.Of) {
		return WhenTest{}, errors.New(`"of" is not a number field of the page`)
	}
	share := t.Share
	if share == "" {
		share = "all"
	}
	if _, _, ok := shareRatio(share); !ok {
		return WhenTest{}, fmt.Errorf("unknown share %q", share)
	}
	out.Of, out.Share = t.Of, share
	return out, nil
}

func (c *testChecker) checkTracker(t, out WhenTest) (WhenTest, error) {
	if t.Kind == "" || !validOp(t.Op) || t.Value == nil {
		return WhenTest{}, errors.New(`a tracker test needs "kind", a "op" of lte, lt, gte, gt or eq, and a "value"`)
	}
	v := *t.Value
	if math.IsNaN(v) || math.Abs(v) > maxTestNumber || v != math.Trunc(v) {
		return WhenTest{}, errors.New(`a tracker's "value" must be a whole number`)
	}
	o, err := c.options()
	if err != nil {
		return WhenTest{}, err
	}
	found := false
	for _, k := range o.Trackers {
		found = found || k.ID == t.Kind
	}
	if !found {
		return WhenTest{}, errors.New("that tracker kind does not exist in this campaign")
	}
	out.Kind, out.Op = t.Kind, t.Op
	out.Value = &v
	return out, nil
}

// ---- sentences --------------------------------------------------------------

// namer turns ids into the words a sentence says. A name that can no longer be
// read falls back to plain words, never to an id. A hero is named to a player
// only when they may open the hero's page, so a printed line cannot name a page
// that is hidden from the reader.
type namer struct {
	s      *service
	ctx    context.Context
	cid    string
	deck   Deck
	viewer Viewer
	opts   *WhenOptions
	tried  bool
	fields map[string]map[string]string
	heroes map[string]string
}

func (s *service) newNamer(ctx context.Context, cid string, d Deck, v Viewer) *namer {
	return &namer{s: s, ctx: ctx, cid: cid, deck: d, viewer: v, fields: map[string]map[string]string{}, heroes: map[string]string{}}
}

func (n *namer) options() *WhenOptions {
	if !n.tried {
		n.tried = true
		if n.s.facts != nil {
			if o, err := n.s.facts.Options(n.ctx, n.cid); err == nil {
				n.opts = &o
			}
		}
	}
	return n.opts
}

func (n *namer) hero(id string) string {
	if name, ok := n.heroes[id]; ok {
		return name
	}
	name := ""
	if h, ok, err := n.s.heroes.InCampaign(n.ctx, n.cid, id); err == nil && ok {
		name = h.Name
		if !n.viewer.Director {
			if may, err := n.s.heroes.Viewable(n.ctx, n.cid, id, n.viewer); err != nil || !may {
				name = ""
			}
		}
	}
	n.heroes[id] = name
	return name
}

// who is the subject of a sentence: "Orrin", "any hero" or, for a page that can't
// be read, "a page".
func (n *namer) who(w string) string {
	switch w {
	case WhoAny:
		return "any hero"
	case WhoHolder:
		w = n.deck.HolderEntityID
	}
	if name := n.hero(w); name != "" {
		return name
	}
	return "a page"
}

func possessive(who string) string {
	if strings.HasSuffix(who, "s") {
		return who + "'"
	}
	return who + "'s"
}

func (n *namer) fieldLabel(who, key string) string {
	page := who
	switch who {
	case WhoAny:
		if o := n.options(); o != nil {
			for _, f := range o.Fields {
				if f.Key == key {
					return f.Label
				}
			}
		}
		return "a number"
	case WhoHolder:
		page = n.deck.HolderEntityID
	}
	labels, ok := n.fields[page]
	if !ok {
		labels = map[string]string{}
		if n.s.facts != nil && page != "" {
			if list, err := n.s.facts.Fields(n.ctx, n.cid, page); err == nil {
				for _, f := range list {
					labels[f.Key] = f.Label
				}
			}
		}
		n.fields[page] = labels
	}
	if l := labels[key]; l != "" {
		return l
	}
	return "a number"
}

func (n *namer) trackerName(kind string) string {
	if o := n.options(); o != nil {
		for _, k := range o.Trackers {
			if k.ID == kind {
				return k.Name
			}
		}
	}
	return "a tracker"
}

func (n *namer) tagName(id int) string {
	if o := n.options(); o != nil {
		for _, t := range o.Tags {
			if t.ID == id {
				return t.Name
			}
		}
	}
	return "a tag"
}

func fmtNum(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// sentence says one test in words: "Orrin's Stamina is at or below half of
// Stamina max", "any hero's Hunger is at or above 4", "Orrin has the tag
// Cursed", "Orrin is not Vampire Scion", "the Director says so".
func (n *namer) sentence(t WhenTest) string {
	switch t.Test {
	case TestSay:
		return "the Director says so"
	case TestNumber:
		who := n.who(t.Who)
		s := possessive(who) + " " + n.fieldLabel(t.Who, t.Field) + " is " + opPhrase(t.Op) + " "
		if t.Value != nil {
			return s + fmtNum(*t.Value)
		}
		if p := sharePhrase(t.Share); p != "" {
			s += p + " "
		}
		return s + n.fieldLabel(t.Who, t.Of)
	case TestTracker:
		v := 0.0
		if t.Value != nil {
			v = *t.Value
		}
		return possessive(n.who(t.Who)) + " " + n.trackerName(t.Kind) + " is " + opPhrase(t.Op) + " " + fmtNum(v)
	case TestTag:
		if t.Has != nil && !*t.Has {
			return n.who(t.Who) + " does not have the tag " + n.tagName(intOf(t.Tag))
		}
		return n.who(t.Who) + " has the tag " + n.tagName(intOf(t.Tag))
	case TestLayer:
		if t.On != nil && !*t.On {
			return n.who(t.Who) + " is not " + t.Name
		}
		return n.who(t.Who) + " is " + t.Name
	}
	return "a test"
}

func intOf(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// printedLine joins a card's printed tests: "When A and B." It is empty when no
// test is printed, and it never carries a test the Director kept to themselves.
func (n *namer) printedLine(tests []WhenTest) string {
	var parts []string
	for _, t := range tests {
		if t.Open {
			parts = append(parts, n.sentence(t))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "When " + strings.Join(parts, " and ") + "."
}

// viewsWith is a card's tests with their sentences and whether each still
// resolves, for a Director. The namer and checker are shared so a deck's cards
// read the facts once between them.
func (s *service) viewsWith(n *namer, c *testChecker, tests []WhenTest) []WhenView {
	out := make([]WhenView, 0, len(tests))
	for _, t := range tests {
		_, err := c.check(t)
		out = append(out, WhenView{WhenTest: t, Sentence: n.sentence(t), Broken: err != nil && !errors.Is(err, errCannotCheck)})
	}
	return out
}

// printLines fills each card's printed line as the viewer may read it, in
// place. It reads the facts' names only when some card prints a test.
func (s *service) printLines(ctx context.Context, cid string, d Deck, v Viewer, cards []Card) {
	var n *namer
	for i := range cards {
		if !cards[i].WhenOpen {
			continue
		}
		if n == nil {
			n = s.newNamer(ctx, cid, d, v)
		}
		cards[i].Printed = n.printedLine(cards[i].When)
	}
}

// printedFrom is the printed line of a Director's card, from its views.
func printedFrom(views []WhenView) string {
	var parts []string
	for _, w := range views {
		if w.Open {
			parts = append(parts, w.Sentence)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "When " + strings.Join(parts, " and ") + "."
}
