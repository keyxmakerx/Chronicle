package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
	"github.com/keyxmakerx/chronicle/internal/plugins/trackers"
)

func init() {
	addWorkshopSource("trackers", func(reg *homebrew.Registry, d workshopDeps) error {
		if !d.Health.IsHealthy(trackers.PluginSlug) {
			return nil
		}
		return registerTrackerParts(reg, d.Trackers)
	})
}

// registerTrackerParts plugs trackers into the Homebrew Workshop: a tracker
// part carries a kind's shape, a clock part is that shape fixed to a clock,
// and rules can react to a tracker reaching a value and step, set or tick
// one. The shapes are checked by the trackers plugin's own rules, so a
// recipe never holds one the Trackers block would refuse. With svc, each
// tracker or clock part is also saved as a tracker kind the recipe keeps in
// step, so it can be placed on pages and keeps a log like any other.
func registerTrackerParts(reg *homebrew.Registry, svc trackers.Service) error {
	shapeCheck := func(clock bool) homebrew.CheckFunc {
		return func(ctx context.Context, campaignID string, raw json.RawMessage) (json.RawMessage, error) {
			var d trackerPartData
			if err := strictJSON(raw, &d); err != nil {
				return nil, err
			}
			if strings.TrimSpace(d.Link) != "" {
				return linkCheck(ctx, svc, campaignID, d.Link, clock)
			}
			s := d.Shape
			if clock {
				if s.Shape != "" && s.Shape != trackers.ShapeClock {
					return nil, apperror.NewValidation("a clock part is always a clock")
				}
				s.Shape = trackers.ShapeClock
			} else if s.Shape == "" {
				s.Shape = trackers.ShapeCount
			}
			out, err := trackers.CheckShape(s)
			if err != nil {
				return nil, err
			}
			return json.Marshal(out)
		}
	}
	type valueArgs struct {
		Value *int `json:"value"`
		Every *int `json:"every"`
	}
	type byArgs struct {
		By *int `json:"by"`
	}
	type toArgs struct {
		To *int `json:"to"`
	}
	needInt := func(field string, get func(raw json.RawMessage) (*int, error)) homebrew.CheckFunc {
		return func(_ context.Context, _ string, raw json.RawMessage) (json.RawMessage, error) {
			n, err := get(raw)
			if err != nil {
				return nil, err
			}
			if n == nil {
				return nil, apperror.NewValidation(fmt.Sprintf("give a whole number for %s", field))
			}
			if *n < -trackers.MaxAbsValue || *n > trackers.MaxAbsValue {
				return nil, apperror.NewValidation(field + " is out of range")
			}
			return raw, nil
		}
	}
	both := []string{"tracker", "clock"}
	regs := []func() error{
		func() error {
			return reg.RegisterPart(homebrew.PartKind{Name: "tracker", Label: "Tracker", Preview: homebrew.PreviewShape, Check: shapeCheck(false),
				Sync: trackerKindSync(svc, "tracker")})
		},
		func() error {
			return reg.RegisterPart(homebrew.PartKind{Name: "clock", Label: "Clock", Preview: homebrew.PreviewShape, Check: shapeCheck(true),
				Sync: trackerKindSync(svc, "clock")})
		},
		func() error {
			return reg.RegisterEvent(homebrew.EventDef{Name: "tracker.reached", Label: "A tracker reaches", PartKinds: both,
				Fields: []homebrew.Field{{Name: "value", Label: "Reaches", Type: homebrew.FieldWhole},
					{Name: "every", Label: "Again every", Type: homebrew.FieldWhole, Optional: true}},
				Preview: &homebrew.Preview{Op: homebrew.PreviewReach, Arg: "value", Every: "every"},
				Match:   trackerReached,
				Check: func(ctx context.Context, campaignID string, raw json.RawMessage) (json.RawMessage, error) {
					var a valueArgs
					if _, err := needInt("the value", func(raw json.RawMessage) (*int, error) {
						return a.Value, strictJSON(raw, &a)
					})(ctx, campaignID, raw); err != nil {
						return nil, err
					}
					if a.Every != nil && (*a.Every < 1 || *a.Every > trackers.MaxAbsValue) {
						return nil, apperror.NewValidation("\"again every\" is a whole number from 1 up; leave it empty to reach the value once")
					}
					return raw, nil
				}})
		},
		func() error {
			return reg.RegisterAction(homebrew.ActionDef{Name: "step", Label: "Step a tracker", PartKinds: both,
				Fields:  []homebrew.Field{{Name: "by", Label: "By", Type: homebrew.FieldWhole}},
				Preview: &homebrew.Preview{Op: homebrew.PreviewAdd, Arg: "by"},
				Run:     trackerRun(svc, trackers.OpAdd, "by"),
				Check: needInt("the step", func(raw json.RawMessage) (*int, error) {
					var a byArgs
					return a.By, strictJSON(raw, &a)
				})})
		},
		func() error {
			return reg.RegisterAction(homebrew.ActionDef{Name: "set", Label: "Set a tracker", PartKinds: both,
				Fields:  []homebrew.Field{{Name: "to", Label: "To", Type: homebrew.FieldWhole}},
				Preview: &homebrew.Preview{Op: homebrew.PreviewSet, Arg: "to"},
				Run:     trackerRun(svc, trackers.OpSet, "to"),
				Check: needInt("the value", func(raw json.RawMessage) (*int, error) {
					var a toArgs
					return a.To, strictJSON(raw, &a)
				})})
		},
		func() error {
			return reg.RegisterAction(homebrew.ActionDef{Name: "tick", Label: "Tick a clock", PartKinds: []string{"clock"},
				Preview: &homebrew.Preview{Op: homebrew.PreviewAdd},
				Run:     trackerRun(svc, trackers.OpAdd, ""),
				Check: func(_ context.Context, _ string, raw json.RawMessage) (json.RawMessage, error) {
					var none struct{}
					return json.RawMessage(`{}`), strictJSON(raw, &none)
				}})
		},
	}
	for _, r := range regs {
		if err := r(); err != nil {
			return err
		}
	}
	return nil
}

// trackerPartData is a tracker or clock part's data: a shape the recipe
// keeps as its own kind, or Link, the name of a hand-made tracker kind the
// campaign already keeps on pages. A linked part carries a copy of that
// kind's shape, refreshed on every save, only so the Workshop's preview can
// act it out; the kind itself stays the truth.
type trackerPartData struct {
	trackers.Shape
	Link string `json:"link,omitempty"`
}

// linkKey is how a link appears in a part's stored data, so the recipe store
// can skip recipes with none before anything is decoded.
const linkKey = `"link":`

// partLink is the hand-made kind a part links to, or "".
func partLink(p *homebrew.Part) string {
	if p == nil || len(p.Data) == 0 {
		return ""
	}
	var d trackerPartData
	if json.Unmarshal(p.Data, &d) != nil {
		return ""
	}
	return d.Link
}

// linkCheck accepts a link to a hand-made kind of the right sort and returns
// the part's data with the kind's name as the campaign spells it and its
// current shape. A kind a recipe keeps is refused: it changes when that
// recipe is saved, so two recipes would fight over it.
func linkCheck(ctx context.Context, svc trackers.Service, campaignID, name string, clock bool) (json.RawMessage, error) {
	name = strings.TrimSpace(name)
	if len([]rune(name)) > trackers.MaxNameChars {
		return nil, apperror.NewValidation("the tracker to use has too long a name")
	}
	if svc == nil {
		return nil, apperror.NewValidation("trackers can't be read just now, so a part can't use one")
	}
	k, ok, err := svc.KindNamed(ctx, campaignID, name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperror.NewValidation("there is no tracker called \"" + name + "\" in this campaign")
	}
	if k.Managed || k.Owner != "" {
		return nil, apperror.NewValidation(k.Name + " is kept by a recipe; use a tracker made on a page")
	}
	if clock != (k.Shape == trackers.ShapeClock) {
		if clock {
			return nil, apperror.NewValidation(k.Name + " is not a clock; use it from a tracker part")
		}
		return nil, apperror.NewValidation(k.Name + " is a clock; use it from a clock part")
	}
	return json.Marshal(trackerPartData{Link: k.Name, Shape: trackers.Shape{Shape: k.Shape, Min: k.Min, Max: k.Max, Steps: k.Steps, Bands: k.Bands}})
}

// trackerKindSync saves one kind of part as tracker kinds owned by the
// recipe, keyed "homebrew:<recipe>:<kind>:<part key>" so tracker and clock
// parts of one recipe never sync over each other. Name-only parts become
// Director-only kinds: the Trackers block has no name-only view, and
// showing less is the safe side.
func trackerKindSync(svc trackers.Service, kind string) *homebrew.PartSync {
	if svc == nil {
		return nil
	}
	want := func(recipeID string, parts []homebrew.Part) (string, []trackers.OwnedKind, error) {
		prefix := recipeOwnerPrefix(recipeID) + kind + ":"
		out := make([]trackers.OwnedKind, 0, len(parts))
		for _, p := range parts {
			var d trackerPartData
			if err := json.Unmarshal(p.Data, &d); err != nil {
				return "", nil, fmt.Errorf("tracker part %s: %w", p.Key, err)
			}
			if d.Link != "" {
				// A linked part uses the page tracker it names, so the
				// recipe keeps no kind of its own for it.
				continue
			}
			s := d.Shape
			see := trackers.SeeDirectors
			if p.See == homebrew.SeeParty {
				see = trackers.SeeEveryone
			}
			out = append(out, trackers.OwnedKind{Owner: prefix + p.Key, Name: p.Label, Shape: s, Visibility: see})
		}
		return prefix, out, nil
	}
	return &homebrew.PartSync{
		Check: func(ctx context.Context, campaignID, recipeID string, parts []homebrew.Part) error {
			prefix, w, err := want(recipeID, parts)
			if err != nil {
				return err
			}
			return svc.CheckOwned(ctx, campaignID, prefix, w)
		},
		Apply: func(ctx context.Context, campaignID, recipeID string, parts []homebrew.Part) error {
			prefix, w, err := want(recipeID, parts)
			if err != nil {
				return err
			}
			return svc.SyncOwned(ctx, campaignID, prefix, w)
		},
	}
}

// strictJSON decodes an owner's JSON, refusing fields the owner doesn't know,
// so a typo in a recipe is reported rather than silently dropped.
func strictJSON(raw json.RawMessage, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return apperror.NewValidation("has a setting it doesn't recognise or of the wrong type")
	}
	return nil
}
