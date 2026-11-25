package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
	"github.com/keyxmakerx/chronicle/internal/plugins/trackers"
)

// respiteEvent is the Homebrew event a logged respite fires.
const respiteEvent = "respite.logged"

func init() {
	addWorkshopSource("respite", func(reg *homebrew.Registry, d workshopDeps) error {
		def := homebrew.EventDef{Name: respiteEvent, Label: "A respite is logged", Press: "Log a respite",
			Check: func(_ context.Context, _ string, raw json.RawMessage) (json.RawMessage, error) {
				var none struct{}
				return json.RawMessage(`{}`), strictJSON(raw, &none)
			}}
		// One press is one respite for crafting too: every project in
		// progress gets its roll.
		if d.Respites != nil {
			def.Pressed = func(ctx context.Context, campaignID string, by homebrew.Viewer) error {
				return d.Respites.RespitePressed(ctx, campaignID, by)
			}
		}
		return reg.RegisterEvent(def)
	})
}

// trackerMove is the payload of tracker.reached: the value before and after.
type trackerMove struct {
	From int `json:"from"`
	To   int `json:"to"`
}

// trackerReached matches a rule when the tracker lands on its value or
// moves past it, either way, as the Workshop's preview does. With every, the
// value repeats upward (3, again every 3: 3, 6, 9 and on), and one move that
// passes several of them is one firing.
func trackerReached(args, payload json.RawMessage) (bool, error) {
	var a struct {
		Value *int `json:"value"`
		Every *int `json:"every"`
	}
	var m trackerMove
	if err := json.Unmarshal(args, &a); err != nil || a.Value == nil {
		return false, fmt.Errorf("tracker.reached args: %v", err)
	}
	if err := json.Unmarshal(payload, &m); err != nil {
		return false, fmt.Errorf("tracker.reached payload: %w", err)
	}
	every := 0
	if a.Every != nil && *a.Every > 0 {
		every = *a.Every
	}
	return crossesMark(m.From, m.To, *a.Value, every), nil
}

// crossesMark says whether moving from one value to another lands on or
// passes at, or with every > 0 any of at, at+every, at+2*every and on.
func crossesMark(from, to, at, every int) bool {
	switch {
	case from < to:
		next := at
		if every > 0 && from >= at {
			next = at + ((from-at)/every+1)*every
		}
		return from < next && next <= to
	case from > to:
		if from <= at {
			return false
		}
		prev := at
		if every > 0 {
			prev = at + ((from-at-1)/every)*every
		}
		return to <= prev && prev < from
	}
	return false
}

// trackerRun is a tracker action carried out by a rule: one log line,
// marked as the rule's, on the page the event came from, or on every page
// that has the tracker when it came from none. arg names the amount; ""
// is 1. Without svc the action has no Run, so rules using it wait for the
// Director.
func trackerRun(svc trackers.Service, op, arg string) func(ctx context.Context, campaignID string, a homebrew.Act) (string, error) {
	if svc == nil {
		return nil
	}
	return func(ctx context.Context, campaignID string, a homebrew.Act) (string, error) {
		if a.Part == nil {
			return "", fmt.Errorf("tracker action with no part")
		}
		n := 1
		if arg != "" {
			var args map[string]*int
			if err := json.Unmarshal(a.Args, &args); err != nil || args[arg] == nil {
				return "", fmt.Errorf("tracker action args: %v", err)
			}
			n = *args[arg]
		}
		v := trackers.Viewer{UserID: a.By.UserID, VisibilityRole: a.By.VisibilityRole, Director: a.By.Director}
		name := a.Part.Label
		var views []trackers.TrackerView
		var err error
		if link := partLink(a.Part); link != "" {
			name = link
			k, ok, kerr := svc.KindNamed(ctx, campaignID, link)
			if kerr != nil {
				return "", kerr
			}
			if !ok || k.Owner != "" {
				// Renamed or removed since the recipe was saved.
				return "", apperror.NewValidation(a.Part.Label + " uses " + link + ", which is no longer a tracker made on a page; choose it again in the Workshop")
			}
			views, err = svc.LogKind(ctx, campaignID, a.Scope, link, v, op, n, a.Note)
		} else {
			owner := recipeOwnerPrefix(a.RecipeID) + a.Part.Kind + ":" + a.Part.Key
			views, err = svc.LogOwned(ctx, campaignID, a.Scope, owner, v, op, n, a.Note)
		}
		if err != nil {
			return "", err
		}
		switch len(views) {
		case 0:
			if a.Scope != "" && partLink(a.Part) != "" {
				return name + " is not on that page, so nothing changed.", nil
			}
			return name + " is on no page yet, so nothing changed.", nil
		case 1:
			line := fmt.Sprintf("%s is now %d", name, views[0].Value)
			if views[0].Now != "" {
				line += ": " + views[0].Now
			}
			return line + ".", nil
		default:
			return fmt.Sprintf("%s changed on %d pages.", name, len(views)), nil
		}
	}
}

// trackerRules hears tracker changes and fires tracker.reached on the
// recipe that owns the tracker's kind, or on every recipe part linked to a
// hand-made kind by its name.
type trackerRules struct {
	svc homebrew.Service
}

func (t *trackerRules) TrackerChanged(ctx context.Context, campaignID string, v trackers.Viewer, c trackers.Change) error {
	payload, err := json.Marshal(trackerMove{From: c.From, To: c.To})
	if err != nil {
		return err
	}
	hv := homebrew.Viewer{UserID: v.UserID, Director: v.Director, VisibilityRole: v.VisibilityRole}
	if !v.Director {
		// A chosen player's change sets off the Director's own recipe: its
		// rules run with the Director's standing, since a player has none
		// and Fire refuses them. The player stays the one who set it off.
		hv.Director, hv.VisibilityRole = true, permissions.RoleOwner
	}
	fire := func(recipeID, part string) error {
		return t.svc.Fire(ctx, campaignID, hv, homebrew.Fired{Event: "tracker.reached", RecipeID: recipeID, Part: part, Scope: c.EntityID, Payload: payload})
	}
	if c.Owner != "" {
		rest, ok := strings.CutPrefix(c.Owner, "homebrew:")
		if !ok {
			return nil
		}
		parts := strings.SplitN(rest, ":", 3)
		if len(parts) != 3 {
			return nil
		}
		return fire(parts[0], parts[2])
	}
	if c.KindName == "" {
		return nil
	}
	linked, err := t.svc.PartsOfKind(ctx, campaignID, hv, linkKey, "tracker", "clock")
	if err != nil {
		return err
	}
	// The parts linked to one kind share one budget, as a chain of rules
	// does, so several recipes on one tracker still stop after 50 runs.
	ctx = t.svc.ShareBudget(ctx)
	var errs []error
	for _, rp := range linked {
		if strings.EqualFold(partLink(&rp.Part), c.KindName) {
			errs = append(errs, fire(rp.RecipeID, rp.Part.Key))
		}
	}
	return errors.Join(errs...)
}
