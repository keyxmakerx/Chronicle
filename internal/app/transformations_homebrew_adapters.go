package app

import (
	"context"
	"encoding/json"

	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
	"github.com/keyxmakerx/chronicle/internal/plugins/transformations"
)

func init() {
	addWorkshopSource("transformations", func(reg *homebrew.Registry, d workshopDeps) error {
		// A preset nobody can put on is no use, so it waits for the store.
		if !d.Health.IsHealthy(transformations.PluginSlug) {
			return nil
		}
		return registerTransformationParts(reg)
	})
}

// registerTransformationParts lets a Director keep a transformation preset
// of their own in the Homebrew Workshop and have a rule offer to put it on.
// The part holds the layer itself (name, note, changes), checked as the
// layer store checks one, so a saved preset is always one Chronicle will
// store. Putting it on runs in the Director's browser: the game system's
// sheet opens the hero with the preset previewed, because nothing reaches a
// hero before the Director has looked at it and pressed Push.
func registerTransformationParts(reg *homebrew.Registry) error {
	kinds := []string{"transformation"}
	if err := reg.RegisterPart(homebrew.PartKind{Name: "transformation", Label: "Transformation", ForceSee: "director",
		Check: func(_ context.Context, _ string, raw json.RawMessage) (json.RawMessage, error) {
			var in transformations.AddInput
			if err := strictJSON(raw, &in); err != nil {
				return nil, err
			}
			out, err := transformations.CheckLayer(in)
			if err != nil {
				return nil, err
			}
			return json.Marshal(out)
		}}); err != nil {
		return err
	}
	return reg.RegisterAction(homebrew.ActionDef{Name: "put-on-transformation", Label: "Put the transformation on the hero",
		PartKinds: kinds, ClientRun: true,
		Check: func(_ context.Context, _ string, raw json.RawMessage) (json.RawMessage, error) {
			var empty struct{}
			return json.RawMessage(`{}`), strictJSON(raw, &empty)
		}})
}
