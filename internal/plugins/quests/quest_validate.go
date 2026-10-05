package quests

import (
	"math"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Limits keep one page's sheet bounded; it is prep notes, not a content store.
const (
	MaxBodyBytes  = 256 * 1024
	maxTitle      = 120
	maxPlate      = 60
	maxKicker     = 40
	maxBlurb      = 300
	maxParagraph  = 2000
	maxParagraphs = 10
	maxShortField = 160 // postedBy, reward, due
	maxLineText   = 200 // steps, rewards, foes, foe notes
	maxLabel      = 120
	maxListItems  = 50
	maxAmount     = 1e12
	defaultKicker = "Help wanted"
)

// idPattern is the shape of a client-made id; the server makes one when empty.
var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)

func validStatus(s string) bool {
	switch s {
	case StatusNotStarted, StatusActive, StatusDone, StatusFailed:
		return true
	}
	return false
}

func validLook(s string) bool {
	switch s {
	case LookLit, LookPlain, LookParchment, LookMidnight:
		return true
	}
	return false
}

// defaultLayout matches the signed mockup.
func defaultLayout() Layout {
	return Layout{
		Notice: Rect{X: 7, Y: 8, W: 58, R: -2.5},
		Map:    Rect{X: 61, Y: 44, W: 34, R: 7},
		Tag:    Rect{X: 9, Y: 66, W: 30, R: -8},
	}
}

// defaultQuest is what a page with no saved sheet reads as.
func defaultQuest(entityName string) Quest {
	return Quest{
		Notice: Notice{Kicker: defaultKicker, Title: entityName, Body: []string{}},
		Status: StatusNotStarted,
		Steps:  []Step{}, Rewards: []Reward{}, Foes: []Foe{}, Links: []Link{},
		Layout: defaultLayout(),
		Looks:  Looks{Board: LookLit, Ledger: LookLit},
	}
}

func clamp(v, lo, hi float64) float64 {
	if math.IsNaN(v) {
		return lo
	}
	return math.Max(lo, math.Min(hi, v))
}

// clampRect keeps a piece on the board and legible.
func clampRect(r Rect) Rect {
	r.X = clamp(r.X, 0, 100)
	r.Y = clamp(r.Y, 0, 100)
	r.W = clamp(r.W, 5, 70)
	r.R = clamp(r.R, -15, 15)
	return r
}

// text trims s and refuses one longer than max runes: truncating silently
// would lose what the DM typed without telling them.
func text(field, s string, max int) (string, error) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > max {
		return "", errInvalid(field + " is too long")
	}
	return s, nil
}

// itemID returns a valid unique id, making one when the client sent none.
func itemID(field, id string, seen map[string]bool) (string, error) {
	if id == "" {
		id = strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	} else if !idPattern.MatchString(id) {
		return "", errInvalid(field + " id is not valid")
	}
	if seen[id] {
		return "", errInvalid(field + " ids must be unique")
	}
	seen[id] = true
	return id, nil
}

func normalizeNotice(n Notice) (Notice, error) {
	var out Notice
	var err error
	for _, f := range []struct {
		name string
		in   string
		max  int
		dst  *string
	}{
		{"plate", n.Plate, maxPlate, &out.Plate},
		{"kicker", n.Kicker, maxKicker, &out.Kicker},
		{"title", n.Title, maxTitle, &out.Title},
		{"blurb", n.Blurb, maxBlurb, &out.Blurb},
		{"postedBy", n.PostedBy, maxShortField, &out.PostedBy},
		{"reward", n.Reward, maxShortField, &out.Reward},
		{"due", n.Due, maxShortField, &out.Due},
	} {
		if *f.dst, err = text(f.name, f.in, f.max); err != nil {
			return Notice{}, err
		}
	}
	if len(n.Body) > maxParagraphs {
		return Notice{}, errInvalid("body has too many paragraphs")
	}
	out.Body = make([]string, 0, len(n.Body))
	for _, p := range n.Body {
		p, err := text("body paragraph", p, maxParagraph)
		if err != nil {
			return Notice{}, err
		}
		out.Body = append(out.Body, p)
	}
	return out, nil
}

func normalizeSteps(in []Step) ([]Step, error) {
	if len(in) > maxListItems {
		return nil, errInvalid("too many steps")
	}
	seen := map[string]bool{}
	out := make([]Step, 0, len(in))
	for _, s := range in {
		id, err := itemID("step", s.ID, seen)
		if err != nil {
			return nil, err
		}
		t, err := text("step", s.Text, maxLineText)
		if err != nil {
			return nil, err
		}
		out = append(out, Step{ID: id, Text: t, Done: s.Done, Shown: s.Shown})
	}
	return out, nil
}

func normalizeRewards(in []Reward) ([]Reward, error) {
	if len(in) > maxListItems {
		return nil, errInvalid("too many rewards")
	}
	seen := map[string]bool{}
	out := make([]Reward, 0, len(in))
	for _, r := range in {
		id, err := itemID("reward", r.ID, seen)
		if err != nil {
			return nil, err
		}
		switch r.Kind {
		case "money", "item", "other":
		default:
			return nil, errInvalid("reward kind must be money, item or other")
		}
		t, err := text("reward", r.Text, maxLineText)
		if err != nil {
			return nil, err
		}
		if r.Amount != nil && (math.IsNaN(*r.Amount) || math.IsInf(*r.Amount, 0) || *r.Amount < 0 || *r.Amount > maxAmount) {
			return nil, errInvalid("reward amount is out of range")
		}
		if r.EntityID != "" && !idPattern.MatchString(r.EntityID) {
			return nil, errInvalid("reward page id is not valid")
		}
		out = append(out, Reward{ID: id, Kind: r.Kind, Text: t, Amount: r.Amount, EntityID: r.EntityID})
	}
	return out, nil
}

func normalizeFoes(in []Foe) ([]Foe, error) {
	if len(in) > maxListItems {
		return nil, errInvalid("too many foes")
	}
	seen := map[string]bool{}
	out := make([]Foe, 0, len(in))
	for _, f := range in {
		id, err := itemID("foe", f.ID, seen)
		if err != nil {
			return nil, err
		}
		t, err := text("foe", f.Text, maxLineText)
		if err != nil {
			return nil, err
		}
		n, err := text("foe note", f.Note, maxLineText)
		if err != nil {
			return nil, err
		}
		if f.EntityID != "" && !idPattern.MatchString(f.EntityID) {
			return nil, errInvalid("foe page id is not valid")
		}
		out = append(out, Foe{ID: id, Text: t, Note: n, EntityID: f.EntityID})
	}
	return out, nil
}

func normalizeLinks(in []Link) ([]Link, error) {
	if len(in) > maxListItems {
		return nil, errInvalid("too many links")
	}
	seen := map[string]bool{}
	out := make([]Link, 0, len(in))
	for _, l := range in {
		id, err := itemID("link", l.ID, seen)
		if err != nil {
			return nil, err
		}
		if l.Kind != "entity" && l.Kind != "map" {
			return nil, errInvalid("link kind must be entity or map")
		}
		if !idPattern.MatchString(l.RefID) {
			return nil, errInvalid("link target id is not valid")
		}
		label, err := text("link label", l.Label, maxLabel)
		if err != nil {
			return nil, err
		}
		out = append(out, Link{ID: id, Kind: l.Kind, RefID: l.RefID, Label: label})
	}
	return out, nil
}
