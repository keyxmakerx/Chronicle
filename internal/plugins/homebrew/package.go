package homebrew

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// A packaged recipe is the Store's form of a recipe: the Workshop's own
// shape, plus what it needs, with everything that belongs to the sharing
// campaign left out. The payload is data the owners validate; it carries no
// markup or script, and every consumer treats it as untrusted, including one
// this server wrote earlier.

// PackageNeeds is what a campaign must have to run a packaged recipe. It is
// derived from the recipe, never believed from the sender.
type PackageNeeds struct {
	PartKinds []string `json:"partKinds"`
	Events    []string `json:"events"`
	Actions   []string `json:"actions"`
	// System is the game system the recipe was made on, recorded only when a
	// step runs in the Director's browser, since only that system's package
	// can do such a step.
	System string `json:"system,omitempty"`
}

// packageDoc is a recipe's parts and rules as shared. It has no Off, so a
// package can't ask to arrive switched on or claim a state of its own.
type packageDoc struct {
	Parts  []Part         `json:"parts"`
	Rules  []Rule         `json:"rules"`
	Layout map[string]Pos `json:"layout,omitempty"`
}

type packagedRecipe struct {
	Name  string       `json:"name"`
	Doc   packageDoc   `json:"doc"`
	Needs PackageNeeds `json:"needs"`
}

// AddedRecipe is what adding a packaged recipe made. LeftToPoint has one
// plain sentence for each link the Director still has to choose, and the
// recipe stays switched off until then.
type AddedRecipe struct {
	ID          string   `json:"id"`
	LeftToPoint []string `json:"leftToPoint"`
}

func (d packageDoc) doc() Doc { return Doc{Parts: d.Parts, Rules: d.Rules, Layout: d.Layout} }

// parsePackage reads a package strictly: an unknown field anywhere, a second
// document after the first, or an oversized body is refused.
func parsePackage(raw json.RawMessage) (packagedRecipe, error) {
	var p packagedRecipe
	if len(raw) > MaxBodyBytes {
		return p, errInvalid("this recipe is too large to share")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return p, errInvalid("this isn't a recipe package this server can read")
	}
	if dec.More() {
		return p, errInvalid("this isn't a recipe package this server can read")
	}
	return p, nil
}

// needsOf works out what a doc needs from the registry. System is kept only
// when a client-run step is in use and the sender named a plain id.
func (s *service) needsOf(d packageDoc, system string) PackageNeeds {
	kinds, events, actions := map[string]bool{}, map[string]bool{}, map[string]bool{}
	clientRun := false
	for _, p := range d.Parts {
		kinds[p.Kind] = true
	}
	for _, r := range d.Rules {
		events[r.When.Event] = true
		for _, st := range r.Then {
			actions[st.Action] = true
			if a, ok := s.reg.action(st.Action); ok && a.ClientRun {
				clientRun = true
			}
		}
	}
	n := PackageNeeds{PartKinds: sortedKeys(kinds), Events: sortedKeys(events), Actions: sortedKeys(actions)}
	if clientRun && validToken(system) {
		n.System = system
	}
	return n
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Package writes a recipe as a shareable package.
func (s *service) Package(ctx context.Context, campaignID, recipeID string, v Viewer) (json.RawMessage, error) {
	if err := requireDirector(v); err != nil {
		return nil, err
	}
	r, err := s.get(ctx, campaignID, recipeID)
	if err != nil {
		return nil, err
	}
	// The recipe's own copy is never edited: parts are copied before the
	// owners strip them. Notes and the run state of a recipe are not in the
	// package type at all.
	p := packagedRecipe{Name: r.Name, Doc: packageDoc{Parts: append([]Part{}, r.Doc.Parts...), Rules: r.Doc.Rules, Layout: r.Doc.Layout}}
	p.Needs.System = s.reg.systemOf(ctx, campaignID)
	out, err := s.strip(p)
	if err != nil {
		return nil, err
	}
	stripped, err := json.Marshal(out)
	if err != nil {
		return nil, errInternal(err)
	}
	// Written in the canonical form, so a package this server made is its own
	// ValidatePackage.
	return s.ValidatePackage(stripped)
}

// StripPackage drops from a package everything that stays with the sharing
// campaign: whatever the owners of its parts say is Director-only, each link
// to a record of that campaign (which becomes an unlinked marker saying what
// to pick), and every field a package does not have. It is run on share and
// again on add, because the sender is never trusted. It is idempotent.
func (s *service) StripPackage(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) > MaxBodyBytes {
		return nil, errInvalid("this recipe is too large to share")
	}
	var p packagedRecipe
	// Unknown fields are dropped here, not refused: stripping is what makes
	// a package fit to validate, and ValidatePackage refuses what is left.
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, errInvalid("this isn't a recipe package this server can read")
	}
	out, err := s.strip(p)
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

// shareArgs runs an event's or action's Share over its args.
func shareArgs(where string, share ShareArgsFunc, args json.RawMessage) (json.RawMessage, error) {
	data, err := shapeJSON(where, args, MaxArgsBytes)
	if err != nil {
		return nil, err
	}
	clean, err := share(data)
	if err != nil {
		return nil, errInvalid(where + ": " + shareMessage(err))
	}
	return shapeJSON(where, clean, MaxArgsBytes)
}

func (s *service) strip(p packagedRecipe) (packagedRecipe, error) {
	name, err := cleanName("name", p.Name)
	if err != nil {
		return p, err
	}
	if len(p.Doc.Parts) > MaxParts {
		return p, errInvalid(fmt.Sprintf("a recipe holds at most %d parts", MaxParts))
	}
	if len(p.Doc.Rules) > MaxRules {
		return p, errInvalid(fmt.Sprintf("a recipe holds at most %d rules", MaxRules))
	}
	out := packagedRecipe{Name: name, Doc: packageDoc{Parts: make([]Part, 0, len(p.Doc.Parts)), Rules: make([]Rule, 0, len(p.Doc.Rules)), Layout: p.Doc.Layout}}
	for i, part := range p.Doc.Parts {
		label := strings.TrimSpace(part.Label)
		if label == "" {
			label = fmt.Sprintf("part %d", i+1)
		}
		waiting, err := cleanUnlinked(label, part.Unlinked)
		if err != nil {
			return p, err
		}
		data, err := shapeJSON(label, part.Data, MaxDataBytes)
		if err != nil {
			return p, err
		}
		kept := []Unlinked(nil)
		if k, ok := s.reg.part(part.Kind); ok && k.Share != nil {
			clean, picks, err := k.Share(data, waiting)
			if err != nil {
				return p, errInvalid(label + ": " + shareMessage(err))
			}
			// Canonical key order, so stripping what is already stripped
			// gives the same bytes whatever order the owner writes in.
			if data, err = shapeJSON(label, clean, MaxDataBytes); err != nil {
				return p, err
			}
			// Only what the owner returns is a marker; the sender's own are
			// input to it, never kept as written.
			if kept, err = cleanUnlinked(label, picks); err != nil {
				return p, err
			}
		} else if len(waiting) > 0 {
			return p, errInvalid(label + " can't have a link to choose")
		}
		part.Data = data
		part.Unlinked = kept
		out.Doc.Parts = append(out.Doc.Parts, part)
	}
	for i, r := range p.Doc.Rules {
		where := fmt.Sprintf("rule %d", i+1)
		nr := Rule{When: r.When, Timing: r.Timing, Then: make([]Do, 0, len(r.Then))}
		var err error
		if ev, ok := s.reg.event(r.When.Event); ok && ev.Share != nil {
			if nr.When.Args, err = shareArgs(where+"'s when", ev.Share, r.When.Args); err != nil {
				return p, err
			}
		}
		for j, d := range r.Then {
			if act, ok := s.reg.action(d.Action); ok && act.Share != nil {
				if d.Args, err = shareArgs(fmt.Sprintf("%s, step %d", where, j+1), act.Share, d.Args); err != nil {
					return p, err
				}
			}
			nr.Then = append(nr.Then, d)
		}
		out.Doc.Rules = append(out.Doc.Rules, nr)
	}
	out.Needs = s.needsOf(out.Doc, p.Needs.System)
	return out, nil
}

// ValidatePackage returns a package in its canonical form, or an error that
// names the part it refuses. It needs no campaign: it holds the package to
// the Workshop's limits and references, refuses unknown fields and anything
// an owner would still strip, and recomputes what the package needs. The
// owners' own Check, which reads the campaign's records, runs when the
// package is added (AddPackaged), not here.
func (s *service) ValidatePackage(raw json.RawMessage) (json.RawMessage, error) {
	p, err := parsePackage(raw)
	if err != nil {
		return nil, err
	}
	name, err := cleanName("name", p.Name)
	if err != nil {
		return nil, err
	}
	doc, err := validateDocMode(context.Background(), s.reg, "", p.Doc.doc(), modeShape)
	if err != nil {
		return nil, err
	}
	out := packagedRecipe{Name: name, Doc: packageDoc{Parts: doc.Parts, Rules: doc.Rules, Layout: doc.Layout}}
	out.Needs = s.needsOf(out.Doc, p.Needs.System)
	return json.Marshal(out)
}

// PackageLacks says, in sentences, what the campaign lacks to run a package:
// a kind of part, a When or a Then that comes from an add-on or game system
// it doesn't use, the game system a browser-run step needs, and anything an
// owner refuses once it checks the package against the campaign's own records.
func (s *service) PackageLacks(ctx context.Context, campaignID string, raw json.RawMessage) ([]string, error) {
	p, err := parsePackage(raw)
	if err != nil {
		return nil, err
	}
	needs := s.needsOf(p.Doc, p.Needs.System)
	var out []string
	for _, k := range needs.PartKinds {
		if _, ok := s.reg.part(k); !ok {
			out = append(out, fmt.Sprintf("This campaign has no %q kind of part; it comes from an add-on or game system it doesn't use.", k))
		}
	}
	for _, e := range needs.Events {
		if _, ok := s.reg.event(e); !ok {
			out = append(out, fmt.Sprintf("This campaign has no %q When; it comes from an add-on or game system it doesn't use.", e))
		}
	}
	for _, a := range needs.Actions {
		if _, ok := s.reg.action(a); !ok {
			out = append(out, fmt.Sprintf("This campaign has no %q Then; it comes from an add-on or game system it doesn't use.", a))
		}
	}
	if needs.System != "" {
		if have := s.reg.systemOf(ctx, campaignID); have != needs.System {
			now := "no game system"
			if have != "" {
				now = "the " + have + " game system"
			}
			out = append(out, fmt.Sprintf("Some steps run in the Director's browser through the %s game system's package, and this campaign uses %s.", needs.System, now))
		}
	}
	if len(out) > 0 {
		return out, nil
	}
	if _, err := validateDocMode(ctx, s.reg, campaignID, p.Doc.doc(), modeAdd); err != nil {
		var ae *apperror.AppError
		if errors.As(err, &ae) && ae.Code < 500 {
			return []string{"It won't fit this campaign as it is: " + strings.TrimRight(ae.Message, ".") + "."}, nil
		}
		return nil, err
	}
	return out, nil
}

// AddPackaged makes a new recipe from a package, in the campaign and switched
// off, with each unlinked marker kept so the Director points it at one of this
// campaign's own records before the recipe is switched on. Only the Director
// team may add. The package is stripped and validated again here, so a package
// that skipped the share step, or was edited on the way, is held to the same
// rules, and every owner checks the rest against this campaign as a save would.
func (s *service) AddPackaged(ctx context.Context, campaignID string, v Viewer, raw json.RawMessage) (AddedRecipe, error) {
	if err := requireDirector(v); err != nil {
		return AddedRecipe{}, err
	}
	stripped, err := s.StripPackage(raw)
	if err != nil {
		return AddedRecipe{}, err
	}
	canon, err := s.ValidatePackage(stripped)
	if err != nil {
		return AddedRecipe{}, err
	}
	p, err := parsePackage(canon)
	if err != nil {
		return AddedRecipe{}, err
	}
	doc, err := validateDocMode(ctx, s.reg, campaignID, p.Doc.doc(), modeAdd)
	if err != nil {
		return AddedRecipe{}, err
	}
	n, err := s.repo.Count(ctx, campaignID)
	if err != nil {
		return AddedRecipe{}, errInternal(err)
	}
	if n >= MaxRecipes {
		return AddedRecipe{}, errInvalid(fmt.Sprintf("a campaign holds at most %d recipes", MaxRecipes))
	}
	r := Recipe{ID: s.newID(), CampaignID: campaignID, Doc: doc, CreatedBy: v.UserID}
	// A name the campaign already uses gets a number, since the Director did
	// not choose this name and the add should not fail on it.
	for i := 1; ; i++ {
		r.Name = numberedName(p.Name, i)
		err := s.repo.Create(ctx, r)
		if err == nil {
			break
		}
		if !errors.Is(err, errDuplicate) || i >= 20 {
			return AddedRecipe{}, dupName(err, r.Name)
		}
	}
	// No owner's Sync runs: the recipe is off, so none of its records should
	// appear in the campaign. The first save that switches it on syncs them.
	notes := []string{}
	for _, part := range doc.Parts {
		for _, u := range part.Unlinked {
			notes = append(notes, fmt.Sprintf("%s: choose %s.", part.Label, u.Pick))
		}
	}
	return AddedRecipe{ID: r.ID, LeftToPoint: notes}, nil
}

// numberedName is name for the first try and "name (2)" and on after, kept
// within the name limit.
func numberedName(name string, i int) string {
	if i <= 1 {
		return name
	}
	suffix := fmt.Sprintf(" (%d)", i)
	rs := []rune(name)
	if len(rs)+len([]rune(suffix)) > MaxNameChars {
		rs = rs[:MaxNameChars-len([]rune(suffix))]
	}
	return string(rs) + suffix
}
