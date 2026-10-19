package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

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
		return func(_ context.Context, _ string, raw json.RawMessage) (json.RawMessage, error) {
			var s trackers.Shape
			if err := strictJSON(raw, &s); err != nil {
				return nil, err
			}
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
				Fields:  []homebrew.Field{{Name: "value", Label: "Reaches", Type: homebrew.FieldWhole}},
				Preview: &homebrew.Preview{Op: homebrew.PreviewReach, Arg: "value"},
				Check: needInt("the value", func(raw json.RawMessage) (*int, error) {
					var a valueArgs
					return a.Value, strictJSON(raw, &a)
				})})
		},
		func() error {
			return reg.RegisterAction(homebrew.ActionDef{Name: "step", Label: "Step a tracker", PartKinds: both,
				Fields:  []homebrew.Field{{Name: "by", Label: "By", Type: homebrew.FieldWhole}},
				Preview: &homebrew.Preview{Op: homebrew.PreviewAdd, Arg: "by"},
				Check: needInt("the step", func(raw json.RawMessage) (*int, error) {
					var a byArgs
					return a.By, strictJSON(raw, &a)
				})})
		},
		func() error {
			return reg.RegisterAction(homebrew.ActionDef{Name: "set", Label: "Set a tracker", PartKinds: both,
				Fields:  []homebrew.Field{{Name: "to", Label: "To", Type: homebrew.FieldWhole}},
				Preview: &homebrew.Preview{Op: homebrew.PreviewSet, Arg: "to"},
				Check: needInt("the value", func(raw json.RawMessage) (*int, error) {
					var a toArgs
					return a.To, strictJSON(raw, &a)
				})})
		},
		func() error {
			return reg.RegisterAction(homebrew.ActionDef{Name: "tick", Label: "Tick a clock", PartKinds: []string{"clock"},
				Preview: &homebrew.Preview{Op: homebrew.PreviewAdd},
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
		prefix := "homebrew:" + recipeID + ":" + kind + ":"
		out := make([]trackers.OwnedKind, 0, len(parts))
		for _, p := range parts {
			var s trackers.Shape
			if err := json.Unmarshal(p.Data, &s); err != nil {
				return "", nil, fmt.Errorf("tracker part %s: %w", p.Key, err)
			}
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
