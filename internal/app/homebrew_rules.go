package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
	"github.com/keyxmakerx/chronicle/internal/plugins/trackers"
)

func init() {
	addWorkshopSource("respite", func(reg *homebrew.Registry, _ workshopDeps) error {
		return reg.RegisterEvent(homebrew.EventDef{Name: "respite.logged", Label: "A respite is logged", Press: "Log a respite",
			Check: func(_ context.Context, _ string, raw json.RawMessage) (json.RawMessage, error) {
				var none struct{}
				return json.RawMessage(`{}`), strictJSON(raw, &none)
			}})
	})
}

// trackerMove is the payload of tracker.reached: the value before and after.
type trackerMove struct {
	From int `json:"from"`
	To   int `json:"to"`
}

// trackerReached matches a rule when the tracker lands on its value or
// moves past it, either way, as the Workshop's preview does.
func trackerReached(args, payload json.RawMessage) (bool, error) {
	var a struct {
		Value *int `json:"value"`
	}
	var m trackerMove
	if err := json.Unmarshal(args, &a); err != nil || a.Value == nil {
		return false, fmt.Errorf("tracker.reached args: %v", err)
	}
	if err := json.Unmarshal(payload, &m); err != nil {
		return false, fmt.Errorf("tracker.reached payload: %w", err)
	}
	at := *a.Value
	return (m.From < at && m.To >= at) || (m.From > at && m.To <= at), nil
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
		owner := recipeOwnerPrefix(a.RecipeID) + a.Part.Kind + ":" + a.Part.Key
		v := trackers.Viewer{UserID: a.By.UserID, VisibilityRole: a.By.VisibilityRole, Director: a.By.Director}
		views, err := svc.LogOwned(ctx, campaignID, a.Scope, owner, v, op, n, a.Note)
		if err != nil {
			return "", err
		}
		switch len(views) {
		case 0:
			return a.Part.Label + " is on no page yet, so nothing changed.", nil
		case 1:
			line := fmt.Sprintf("%s is now %d", a.Part.Label, views[0].Value)
			if views[0].Now != "" {
				line += ": " + views[0].Now
			}
			return line + ".", nil
		default:
			return fmt.Sprintf("%s changed on %d pages.", a.Part.Label, len(views)), nil
		}
	}
}

// trackerRules hears tracker changes and fires tracker.reached on the
// recipe that owns the tracker's kind.
type trackerRules struct {
	svc homebrew.Service
}

func (t *trackerRules) TrackerChanged(ctx context.Context, campaignID string, v trackers.Viewer, c trackers.Change) error {
	rest, ok := strings.CutPrefix(c.Owner, "homebrew:")
	if !ok {
		return nil
	}
	parts := strings.SplitN(rest, ":", 3)
	if len(parts) != 3 {
		return nil
	}
	payload, err := json.Marshal(trackerMove{From: c.From, To: c.To})
	if err != nil {
		return err
	}
	hv := homebrew.Viewer{UserID: v.UserID, Director: v.Director, VisibilityRole: v.VisibilityRole}
	return t.svc.Fire(ctx, campaignID, hv, homebrew.Fired{Event: "tracker.reached", RecipeID: parts[0], Part: parts[2], Scope: c.EntityID, Payload: payload})
}
