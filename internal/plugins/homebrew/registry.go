package homebrew

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// CheckFunc validates one owner's JSON and returns it normalised. Its error
// is shown to the Director, so it says what to fix in plain words.
type CheckFunc func(ctx context.Context, campaignID string, raw json.RawMessage) (json.RawMessage, error)

// PartKind is a kind of part, registered by the plugin that owns its data.
type PartKind struct {
	Name  string // e.g. "tracker"; what a part's kind field holds
	Label string // what the Workshop's palette calls it
	// ForceSee, when set, is the only audience the kind allows (a creature
	// recipe is always the Director's).
	ForceSee string
	// Preview, when PreviewShape, says data is a trackers shape, so the
	// Workshop's live preview can show the part as a number it can change.
	Preview string
	Check   CheckFunc
}

// EventDef is something that can start a rule.
type EventDef struct {
	Name  string // e.g. "tracker.reached"
	Label string
	// PartKinds are the kinds of part that fire it; empty means it comes from
	// outside any part (a moon changing, a respite).
	PartKinds []string
	Fields    []Field // the args the Workshop asks for; nil when there are none
	// Preview lets the live preview act the event out; nil makes it a
	// button the Director presses to say it happened.
	Preview *Preview
	Check   CheckFunc
}

// Field is one arg of an event or action, as the Workshop's inspector asks
// for it. The owner's Check still has the last word on its value.
type Field struct {
	Name  string `json:"name"`  // the arg's JSON key
	Label string `json:"label"` // what the inspector calls it
	Type  string `json:"type"`  // FieldWhole or FieldText
}

// Field types the inspector can draw.
const (
	FieldWhole = "whole" // a whole number
	FieldText  = "text"  // one line of text
)

// ActionDef is something a rule can do.
type ActionDef struct {
	Name  string // e.g. "step"
	Label string
	// PartKinds are the kinds of part it acts on; empty means it takes no part.
	PartKinds []string
	// ClientRun actions run in the Director's browser, through a handler a
	// system package registered there, only when the Director presses Run.
	// The server records and validates them but never runs them.
	ClientRun bool
	Fields    []Field
	// Preview lets the live preview act the action out; nil makes it a
	// line saying what would happen.
	Preview *Preview
	Check   CheckFunc
}

// Preview says how the Workshop's live preview acts out an event or action
// on a part's number, so a recipe can be tried without the server and
// without the preview knowing any owner by name.
type Preview struct {
	Op string `json:"op"`
	// Arg is the whole-number field holding the amount; empty means 1 for
	// PreviewAdd. PreviewReach and PreviewSet need one.
	Arg string `json:"arg,omitempty"`
}

// Preview ops. An event can only reach; an action can only add or set.
const (
	PreviewShape = "shape" // a part kind whose data is a trackers shape
	PreviewReach = "reach" // the part's number moves onto or past Arg
	PreviewAdd   = "add"   // add Arg (or 1) to the part's number
	PreviewSet   = "set"   // set the part's number to Arg
)

// checkPreview keeps a preview the Workshop can act out: an op that fits,
// over a part with a number, reading a whole-number field the def declares.
func checkPreview(what, name string, p *Preview, fields []Field, partKinds []string, ops ...string) error {
	if p == nil {
		return nil
	}
	ok := false
	for _, o := range ops {
		ok = ok || p.Op == o
	}
	if !ok {
		return fmt.Errorf("homebrew: %s %q has a preview op %q it can't use", what, name, p.Op)
	}
	if len(partKinds) == 0 {
		return fmt.Errorf("homebrew: %s %q previews a number but takes no part", what, name)
	}
	if p.Arg == "" {
		if p.Op != PreviewAdd {
			return fmt.Errorf("homebrew: %s %q preview %q needs an arg", what, name, p.Op)
		}
		return nil
	}
	for _, f := range fields {
		if f.Name == p.Arg && f.Type == FieldWhole {
			return nil
		}
	}
	return fmt.Errorf("homebrew: %s %q preview reads %q, which is not one of its whole-number fields", what, name, p.Arg)
}

// Registry holds what other plugins have plugged into the Workshop. It is
// filled once while the app starts and read for every request after.
type Registry struct {
	mu      sync.RWMutex
	parts   map[string]PartKind
	events  map[string]EventDef
	actions map[string]ActionDef
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{parts: map[string]PartKind{}, events: map[string]EventDef{}, actions: map[string]ActionDef{}}
}

// A name is lower case words joined by - or ., so it is safe in a URL, a log
// and a data attribute, and two owners can't register look-alikes.
var nameRe = regexp.MustCompile(`^[a-z][a-z0-9]*([.-][a-z0-9]+)*$`)

func checkName(what, name string, check CheckFunc) error {
	if len(name) > 40 || !nameRe.MatchString(name) {
		return fmt.Errorf("homebrew: %s name %q is not a lower-case dotted or dashed word", what, name)
	}
	if check == nil {
		return fmt.Errorf("homebrew: %s %q has no Check", what, name)
	}
	return nil
}

var fieldNameRe = regexp.MustCompile(`^[a-z][a-zA-Z0-9]{0,39}$`)

// checkFields keeps an owner's arg list drawable: camelCase keys, each once,
// with a label and a type the inspector knows.
func checkFields(what, name string, fields []Field) error {
	seen := map[string]bool{}
	for _, f := range fields {
		if !fieldNameRe.MatchString(f.Name) || seen[f.Name] {
			return fmt.Errorf("homebrew: %s %q has a bad or repeated field %q", what, name, f.Name)
		}
		if strings.TrimSpace(f.Label) == "" || (f.Type != FieldWhole && f.Type != FieldText) {
			return fmt.Errorf("homebrew: %s %q field %q needs a label and a known type", what, name, f.Name)
		}
		seen[f.Name] = true
	}
	return nil
}

// RegisterPart adds a kind of part. A second registration of a name is an
// error: two owners for one kind would make validation ambiguous.
func (r *Registry) RegisterPart(k PartKind) error {
	if err := checkName("part kind", k.Name, k.Check); err != nil {
		return err
	}
	if k.ForceSee != "" && !validSee(k.ForceSee) {
		return fmt.Errorf("homebrew: part kind %q forces an unknown audience %q", k.Name, k.ForceSee)
	}
	if k.Preview != "" && k.Preview != PreviewShape {
		return fmt.Errorf("homebrew: part kind %q has an unknown preview %q", k.Name, k.Preview)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.parts[k.Name]; dup {
		return fmt.Errorf("homebrew: part kind %q is already registered", k.Name)
	}
	r.parts[k.Name] = k
	return nil
}

// RegisterEvent adds an event.
func (r *Registry) RegisterEvent(e EventDef) error {
	if err := checkName("event", e.Name, e.Check); err != nil {
		return err
	}
	if err := checkFields("event", e.Name, e.Fields); err != nil {
		return err
	}
	if err := checkPreview("event", e.Name, e.Preview, e.Fields, e.PartKinds, PreviewReach); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.events[e.Name]; dup {
		return fmt.Errorf("homebrew: event %q is already registered", e.Name)
	}
	r.events[e.Name] = e
	return nil
}

// RegisterAction adds an action.
func (r *Registry) RegisterAction(a ActionDef) error {
	if err := checkName("action", a.Name, a.Check); err != nil {
		return err
	}
	if err := checkFields("action", a.Name, a.Fields); err != nil {
		return err
	}
	if a.ClientRun && a.Preview != nil {
		return fmt.Errorf("homebrew: action %q runs in the browser, so it can't preview", a.Name)
	}
	if err := checkPreview("action", a.Name, a.Preview, a.Fields, a.PartKinds, PreviewAdd, PreviewSet); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.actions[a.Name]; dup {
		return fmt.Errorf("homebrew: action %q is already registered", a.Name)
	}
	r.actions[a.Name] = a
	return nil
}

func (r *Registry) part(name string) (PartKind, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	k, ok := r.parts[name]
	return k, ok
}

func (r *Registry) event(name string) (EventDef, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.events[name]
	return e, ok
}

func (r *Registry) action(name string) (ActionDef, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.actions[name]
	return a, ok
}

// Catalog is what the Workshop's palette and When/Then lists offer.
type Catalog struct {
	Parts   []CatalogEntry `json:"parts"`
	Events  []CatalogEntry `json:"events"`
	Actions []CatalogEntry `json:"actions"`
}

// CatalogEntry is one registered name, as the Workshop shows it.
type CatalogEntry struct {
	Name      string   `json:"name"`
	Label     string   `json:"label"`
	ForceSee  string   `json:"forceSee,omitempty"`
	PartKinds []string `json:"partKinds,omitempty"`
	ClientRun bool     `json:"clientRun,omitempty"`
	Fields    []Field  `json:"fields,omitempty"`
	// Preview is a part kind's PreviewShape, or an event's or action's op.
	Preview any `json:"preview,omitempty"`
}

// Catalog lists everything registered, sorted by name so the palette is
// stable between restarts.
func (r *Registry) Catalog() Catalog {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c := Catalog{Parts: []CatalogEntry{}, Events: []CatalogEntry{}, Actions: []CatalogEntry{}}
	for _, k := range r.parts {
		e := CatalogEntry{Name: k.Name, Label: k.Label, ForceSee: k.ForceSee}
		if k.Preview != "" {
			e.Preview = k.Preview
		}
		c.Parts = append(c.Parts, e)
	}
	for _, e := range r.events {
		ce := CatalogEntry{Name: e.Name, Label: e.Label, PartKinds: e.PartKinds, Fields: e.Fields}
		if e.Preview != nil {
			ce.Preview = e.Preview
		}
		c.Events = append(c.Events, ce)
	}
	for _, a := range r.actions {
		ce := CatalogEntry{Name: a.Name, Label: a.Label, PartKinds: a.PartKinds, ClientRun: a.ClientRun, Fields: a.Fields}
		if a.Preview != nil {
			ce.Preview = a.Preview
		}
		c.Actions = append(c.Actions, ce)
	}
	for _, l := range [][]CatalogEntry{c.Parts, c.Events, c.Actions} {
		sort.Slice(l, func(i, j int) bool { return l[i].Name < l[j].Name })
	}
	return c
}
