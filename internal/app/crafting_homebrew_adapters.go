package app

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/crafting"
	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
)

// The Crafting recipes part: a homebrew holds a set of crafting recipes
// (an herbalist's cures) as one part, drawn in the Workshop by crafting's
// own widget as a quick table. Each row is an ordinary crafting recipe the
// Crafting app and the item's page show too; the part keeps only which
// recipes it holds and how new rows start. A rule can teach one of them.
func init() {
	addWorkshopSource("crafting recipes", func(reg *homebrew.Registry, d workshopDeps) error {
		if d.Crafting == nil {
			return nil
		}
		return registerCraftingParts(reg, d.Crafting)
	})
}

// craftingPartKind is the part's kind, and the widget that draws it.
const (
	craftingPartKind   = "crafting-recipes"
	craftingPartWidget = "crafting-recipes-part"
	maxPartRecipes     = 200
)

// craftingPartData is the part's data. Who is how the part's recipes may be
// made ("all", "learned", or "director", which is "learned" with nobody
// taught yet); Start is how a new row starts ("draft" or "ready").
type craftingPartData struct {
	Who     string   `json:"who,omitempty"`
	Start   string   `json:"start,omitempty"`
	Recipes []string `json:"recipes,omitempty"`
}

// craftingPartViewer is the Director the Workshop already checked: a
// part's check runs only inside the Director's own save.
var craftingPartViewer = crafting.Viewer{Director: true}

func registerCraftingParts(reg *homebrew.Registry, svc crafting.Service) error {
	check := func(ctx context.Context, campaignID string, raw json.RawMessage) (json.RawMessage, error) {
		var d craftingPartData
		if err := strictJSON(raw, &d); err != nil {
			return nil, err
		}
		switch d.Who {
		case "", "all", "learned", "director":
		default:
			return nil, apperror.NewValidation(`who can make them is "all", "learned" or "director"`)
		}
		switch d.Start {
		case "", "draft", "ready":
		default:
			return nil, apperror.NewValidation(`new rows start as "draft" or "ready"`)
		}
		if d.Who == "" {
			d.Who = "all"
		}
		if d.Start == "" {
			d.Start = "draft"
		}
		if len(d.Recipes) > maxPartRecipes {
			return nil, apperror.NewValidation("one part holds at most 200 crafting recipes")
		}
		// A recipe deleted in the Crafting app simply drops out of the list.
		kept, err := svc.KnownRecipes(ctx, campaignID, craftingPartViewer, d.Recipes)
		if err != nil {
			return nil, err
		}
		d.Recipes = kept
		return json.Marshal(d)
	}
	checkTeach := func(_ context.Context, _ string, raw json.RawMessage) (json.RawMessage, error) {
		var a struct {
			Recipe string `json:"recipe"`
		}
		if err := strictJSON(raw, &a); err != nil {
			return nil, err
		}
		a.Recipe = strings.TrimSpace(a.Recipe)
		if a.Recipe == "" {
			return nil, apperror.NewValidation("name the crafting recipe to teach")
		}
		if len([]rune(a.Recipe)) > 120 {
			return nil, apperror.NewValidation("a recipe's name is at most 120 characters")
		}
		return json.Marshal(a)
	}
	if err := reg.RegisterPart(homebrew.PartKind{Name: craftingPartKind, Label: "Crafting recipes", ForceSee: "director",
		Widget: craftingPartWidget, Check: check}); err != nil {
		return err
	}
	return reg.RegisterAction(homebrew.ActionDef{
		Name: "crafting.teach", Label: "Teach the party a crafting recipe", PartKinds: []string{craftingPartKind},
		Fields: []homebrew.Field{{Name: "recipe", Label: "Recipe", Type: homebrew.FieldText}},
		Check:  checkTeach,
		Run: func(ctx context.Context, campaignID string, a homebrew.Act) (string, error) {
			var args struct {
				Recipe string `json:"recipe"`
			}
			if err := json.Unmarshal(a.Args, &args); err != nil {
				return "", apperror.NewValidation("the rule's recipe can't be read")
			}
			return svc.TeachParty(ctx, campaignID, crafting.Viewer{UserID: a.By.UserID, Director: a.By.Director, VisibilityRole: a.By.VisibilityRole}, args.Recipe)
		},
	})
}
