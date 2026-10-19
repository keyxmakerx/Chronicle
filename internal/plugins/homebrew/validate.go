package homebrew

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// Limits. They keep one recipe a size the Workshop canvas can show and a
// request can carry.
const (
	MaxNameChars     = 80
	MaxTestChars     = 200
	MaxParts         = 60
	MaxRules         = 60
	MaxThen          = 12
	MaxRecipes       = 200
	MaxDataBytes     = 16 << 10
	MaxArgsBytes     = 4 << 10
	MaxBodyBytes     = 512 << 10
	MaxIDChars       = 64
	maxPartKeyLength = 40
	MaxCanvas        = 20000 // how far from the origin a card may sit
)

var keyRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// validToken reports whether s is a short safe id.
func validToken(s string) bool {
	if s == "" || len(s) > MaxIDChars {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

// cleanText trims s and checks it is single-line plain text within max
// characters, so stored text is always safe to show with textContent.
func cleanText(field, s string, max int) (string, error) {
	s = strings.TrimSpace(s)
	if !utf8.ValidString(s) {
		return "", errInvalid(field + " is not valid text")
	}
	if utf8.RuneCountInString(s) > max {
		return "", errInvalid(fmt.Sprintf("%s is too long (at most %d characters)", field, max))
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return "", errInvalid(field + " contains a character that is not allowed")
		}
	}
	return s, nil
}

func cleanName(field, s string) (string, error) {
	s, err := cleanText(field, s, MaxNameChars)
	if err != nil {
		return "", err
	}
	if s == "" {
		return "", errInvalid(field + " is required")
	}
	return s, nil
}

func validSee(s string) bool { return s == SeeParty || s == SeeName || s == SeeDirector }

func validTiming(s string) bool {
	return s == TimingNow || s == TimingEncounterEnd || s == TimingHeld
}

// emptyJSON stands in for absent data or args, so every owner's Check sees
// an object rather than having to handle nothing.
var emptyJSON = json.RawMessage(`{}`)

// runCheck calls an owner's Check, bounding the input and keeping the owner's
// message when it is one meant for the Director.
func runCheck(ctx context.Context, campaignID, where string, raw json.RawMessage, max int, check CheckFunc) (json.RawMessage, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		raw = emptyJSON
	}
	if len(raw) > max {
		return nil, errInvalid(fmt.Sprintf("%s is too large", where))
	}
	if !json.Valid(raw) {
		return nil, errInvalid(where + " is not valid JSON")
	}
	out, err := check(ctx, campaignID, raw)
	if err != nil {
		var ae *apperror.AppError
		if errors.As(err, &ae) && ae.Code < 500 {
			return nil, errInvalid(where + ": " + ae.Message)
		}
		return nil, err
	}
	if len(out) == 0 {
		out = raw
	}
	return out, nil
}

func listHas(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// validateDoc checks a recipe's envelope against the registry and hands each
// part's data and each rule's args to its owner. It returns the doc as it
// will be stored.
func validateDoc(ctx context.Context, reg *Registry, campaignID string, in Doc) (Doc, error) {
	out := Doc{Parts: []Part{}, Rules: []Rule{}}
	if len(in.Parts) > MaxParts {
		return out, errInvalid(fmt.Sprintf("a recipe holds at most %d parts", MaxParts))
	}
	if len(in.Rules) > MaxRules {
		return out, errInvalid(fmt.Sprintf("a recipe holds at most %d rules", MaxRules))
	}
	kinds := map[string]string{}
	for i, p := range in.Parts {
		where := fmt.Sprintf("part %d", i+1)
		if len(p.Key) > maxPartKeyLength || !keyRe.MatchString(p.Key) {
			return out, errInvalid(where + " needs a key of lower-case letters, digits, - or _")
		}
		if _, dup := kinds[p.Key]; dup {
			return out, errInvalid(fmt.Sprintf("two parts use the key %q", p.Key))
		}
		label, err := cleanName(where+"'s label", p.Label)
		if err != nil {
			return out, err
		}
		k, ok := reg.part(p.Kind)
		if !ok {
			return out, errInvalid(fmt.Sprintf("%s (%s) is a kind this campaign doesn't have", label, p.Kind))
		}
		see := p.See
		if see == "" {
			see = SeeDirector
		}
		if !validSee(see) {
			return out, errInvalid(label + ": who sees it must be party, name or director")
		}
		if k.ForceSee != "" {
			see = k.ForceSee
		}
		data, err := runCheck(ctx, campaignID, label, p.Data, MaxDataBytes, k.Check)
		if err != nil {
			return out, err
		}
		kinds[p.Key] = p.Kind
		out.Parts = append(out.Parts, Part{Key: p.Key, Kind: p.Kind, Label: label, See: see, Data: data})
	}

	for i, r := range in.Rules {
		where := fmt.Sprintf("rule %d", i+1)
		ev, ok := reg.event(r.When.Event)
		if !ok {
			return out, errInvalid(fmt.Sprintf("%s starts on %q, which this campaign doesn't have", where, r.When.Event))
		}
		if err := checkTarget(where+"'s when", r.When.Part, ev.PartKinds, kinds); err != nil {
			return out, err
		}
		test, err := cleanText(where+"'s test", r.When.Test, MaxTestChars)
		if err != nil {
			return out, err
		}
		args, err := runCheck(ctx, campaignID, where+"'s when", r.When.Args, MaxArgsBytes, ev.Check)
		if err != nil {
			return out, err
		}
		timing := r.Timing
		if timing == "" {
			timing = TimingNow
		}
		if !validTiming(timing) {
			return out, errInvalid(where + " runs now, at the end of the encounter, or when held")
		}
		if len(r.Then) == 0 || len(r.Then) > MaxThen {
			return out, errInvalid(fmt.Sprintf("%s needs 1 to %d things to do", where, MaxThen))
		}
		rule := Rule{When: When{Event: ev.Name, Part: r.When.Part, Args: args, Test: test}, Timing: timing, Then: []Do{}}
		for j, d := range r.Then {
			dw := fmt.Sprintf("%s, step %d", where, j+1)
			act, ok := reg.action(d.Action)
			if !ok {
				return out, errInvalid(fmt.Sprintf("%s does %q, which this campaign doesn't have", dw, d.Action))
			}
			if err := checkTarget(dw, d.Part, act.PartKinds, kinds); err != nil {
				return out, err
			}
			a, err := runCheck(ctx, campaignID, dw, d.Args, MaxArgsBytes, act.Check)
			if err != nil {
				return out, err
			}
			rule.Then = append(rule.Then, Do{Action: act.Name, Part: d.Part, Args: a})
		}
		out.Rules = append(out.Rules, rule)
	}
	out.Layout = cleanLayout(in.Layout, out)
	return out, nil
}

var ruleCardRe = regexp.MustCompile(`^r(0|[1-9]\d?)(?:\.(0|[1-9]\d?))?$`)

// cleanLayout keeps only the positions of cards the doc has, clamped to the
// canvas. A stale or unknown key is dropped rather than refused: the layout
// is the Workshop's own, and a card without a place is simply laid out again.
func cleanLayout(in map[string]Pos, d Doc) map[string]Pos {
	if len(in) == 0 {
		return nil
	}
	parts := map[string]bool{}
	for _, p := range d.Parts {
		parts[p.Key] = true
	}
	out := map[string]Pos{}
	for k, pos := range in {
		ok := false
		if key, isPart := strings.CutPrefix(k, "p:"); isPart {
			ok = parts[key]
		} else if m := ruleCardRe.FindStringSubmatch(k); m != nil {
			r, _ := strconv.Atoi(m[1]) // the pattern admits only digits
			ok = r < len(d.Rules)
			if ok && m[2] != "" {
				t, _ := strconv.Atoi(m[2])
				ok = t < len(d.Rules[r].Then)
			}
		}
		if ok {
			out[k] = Pos{X: clampCanvas(pos.X), Y: clampCanvas(pos.Y)}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func clampCanvas(n int) int {
	return max(-MaxCanvas, min(MaxCanvas, n))
}

// checkTarget checks a rule's part reference: it must name a part of this
// recipe of a kind the event or action works with, and must be present
// exactly when the event or action is tied to parts.
func checkTarget(where, key string, partKinds []string, kinds map[string]string) error {
	if len(partKinds) == 0 {
		if key != "" {
			return errInvalid(where + " doesn't take a part")
		}
		return nil
	}
	if key == "" {
		return errInvalid(where + " needs a part")
	}
	kind, ok := kinds[key]
	if !ok {
		return errInvalid(fmt.Sprintf("%s names a part %q this recipe doesn't have", where, key))
	}
	if !listHas(partKinds, kind) {
		return errInvalid(fmt.Sprintf("%s can't use a %s part", where, kind))
	}
	return nil
}
