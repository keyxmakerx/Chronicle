package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
	"github.com/keyxmakerx/chronicle/internal/plugins/systemstate"
)

func init() {
	addWorkshopSource("creature variants", func(reg *homebrew.Registry, d workshopDeps) error {
		if d.SystemState == nil {
			return nil
		}
		return registerVariantParts(reg, d.SystemState,
			&systemStateEntityLookup{entities: d.Entities},
			func(ctx context.Context, campaignID string) string {
				if sys := d.Systems.EnabledSystem(ctx, campaignID); sys != nil {
					return sys.Info().ID
				}
				return ""
			})
	})
}

// variantKey is the system-state key a game system keeps its creature variant
// recipes and links under. The record itself belongs to the system package;
// Chronicle reads only whether a page is a recipe and which part it is.
const variantKey = "variant"

// Limits on a creature mark, so a recipe stays well inside the Workshop's
// 16 KB part data: a recipe has at most six add-ons (the package's own
// limit), and a mark covers at most one campaign listing's worth of pages.
const (
	maxMarkAddons = 6
	maxMarkPages  = systemstate.MaxListItems
)

// variantStates reads a page's system state, already scoped to the campaign.
type variantStates interface {
	Get(ctx context.Context, campaignID, entityID, systemID, key string) (systemstate.State, error)
}

// variantPages tells which campaign a page belongs to.
type variantPages interface {
	EntityCampaignID(ctx context.Context, entityID string) (string, error)
}

// variantSystem names the campaign's enabled game system, or "" when none.
type variantSystem func(ctx context.Context, campaignID string) string

// registerVariantParts plugs creature variants into the Homebrew Workshop: a
// link to a variant recipe page, a mark that makes creatures rise as a variant
// of themselves when they die, the event that fires on a marked death, and the
// action that raises one. The recipe's steps stay in the game system's own
// record and are never copied into a Workshop recipe. Rising runs in the
// Director's browser, because only the system package can work its stat block
// out again.
func registerVariantParts(reg *homebrew.Registry, states variantStates, pages variantPages, system variantSystem) error {
	v := variantChecker{states: states, pages: pages, system: system}
	none := func(_ context.Context, _ string, raw json.RawMessage) (json.RawMessage, error) {
		var empty struct{}
		return json.RawMessage(`{}`), strictJSON(raw, &empty)
	}
	marks := []string{"creature-mark"}
	regs := []func() error{
		func() error {
			return reg.RegisterPart(homebrew.PartKind{Name: "creature-recipe", Label: "Creature recipe", ForceSee: "director", Check: v.checkRecipeLink})
		},
		func() error {
			return reg.RegisterPart(homebrew.PartKind{Name: "creature-mark", Label: "Creature mark", ForceSee: "director", Check: v.checkMark})
		},
		func() error {
			return reg.RegisterEvent(homebrew.EventDef{Name: "creature.dies", Label: "A marked creature dies", PartKinds: marks, Check: none})
		},
		func() error {
			return reg.RegisterAction(homebrew.ActionDef{Name: "rise-as-variant", Label: "Rise as its variant", PartKinds: marks, ClientRun: true, Check: none})
		},
	}
	for _, r := range regs {
		if err := r(); err != nil {
			return err
		}
	}
	return nil
}

type variantChecker struct {
	states variantStates
	pages  variantPages
	system variantSystem
}

// recipeLink is a creature-recipe part's data.
type recipeLink struct {
	Page string `json:"page"`
}

// creatureMark is a creature-mark part's data. Variants is written by the
// rise action, not by the Director, and maps a marked page to its variant.
type creatureMark struct {
	Recipe   string            `json:"recipe"`
	Addons   []string          `json:"addons,omitempty"`
	Marked   []string          `json:"marked,omitempty"`
	Variants map[string]string `json:"variants,omitempty"`
}

func (v variantChecker) checkRecipeLink(ctx context.Context, campaignID string, raw json.RawMessage) (json.RawMessage, error) {
	var d recipeLink
	if err := strictJSON(raw, &d); err != nil {
		return nil, err
	}
	sys, err := v.systemFor(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if _, err := v.recipePart(ctx, campaignID, sys, d.Page); err != nil {
		return nil, err
	}
	return json.Marshal(d)
}

func (v variantChecker) checkMark(ctx context.Context, campaignID string, raw json.RawMessage) (json.RawMessage, error) {
	var d creatureMark
	if err := strictJSON(raw, &d); err != nil {
		return nil, err
	}
	sys, err := v.systemFor(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	part, err := v.recipePart(ctx, campaignID, sys, d.Recipe)
	if err != nil {
		return nil, err
	}
	if part != "base" {
		return nil, apperror.NewValidation("the recipe is an add-on; pick the recipe it adds to")
	}
	if len(d.Addons) > maxMarkAddons {
		return nil, apperror.NewValidation(fmt.Sprintf("has more than %d add-ons", maxMarkAddons))
	}
	addons, err := unique(d.Addons, "add-on")
	if err != nil {
		return nil, err
	}
	for _, id := range addons {
		part, err := v.recipePart(ctx, campaignID, sys, id)
		if err != nil {
			return nil, err
		}
		if part != "addon" {
			return nil, apperror.NewValidation("an add-on is a whole recipe, not an add-on")
		}
	}
	if len(d.Marked) > maxMarkPages || len(d.Variants) > maxMarkPages {
		return nil, apperror.NewValidation(fmt.Sprintf("marks more than %d creatures", maxMarkPages))
	}
	marked, err := unique(d.Marked, "marked creature")
	if err != nil {
		return nil, err
	}
	isMarked := map[string]bool{}
	for _, id := range marked {
		if err := v.inCampaign(ctx, campaignID, id); err != nil {
			return nil, err
		}
		isMarked[id] = true
	}
	for from, to := range d.Variants {
		if !isMarked[from] {
			return nil, apperror.NewValidation("lists a variant for a creature that isn't marked")
		}
		if err := v.inCampaign(ctx, campaignID, to); err != nil {
			return nil, err
		}
	}
	d.Addons, d.Marked = addons, marked
	return json.Marshal(d)
}

// systemFor is the campaign's game system; variants need one.
func (v variantChecker) systemFor(ctx context.Context, campaignID string) (string, error) {
	sys := v.system(ctx, campaignID)
	if sys == "" {
		return "", apperror.NewValidation("needs a game system enabled for this campaign")
	}
	return sys, nil
}

// recipePart reads a page's variant record and says which part of a recipe it
// is ("base" or "addon"). A page that is not a recipe in this campaign is a
// validation error, worded the same whether it is missing or elsewhere.
func (v variantChecker) recipePart(ctx context.Context, campaignID, sys, page string) (string, error) {
	if page == "" {
		return "", apperror.NewValidation("needs a recipe page")
	}
	st, err := v.states.Get(ctx, campaignID, page, sys, variantKey)
	if err != nil {
		return "", notHere(err, "points at a page that isn't in this campaign")
	}
	var doc struct {
		Recipe *struct {
			Part string `json:"part"`
		} `json:"recipe"`
	}
	if json.Unmarshal(st.GM, &doc) != nil || doc.Recipe == nil {
		return "", apperror.NewValidation("points at a page that isn't a variant recipe")
	}
	if doc.Recipe.Part == "addon" {
		return "addon", nil
	}
	return "base", nil
}

func (v variantChecker) inCampaign(ctx context.Context, campaignID, page string) error {
	if page == "" {
		return apperror.NewValidation("has an empty page in its list")
	}
	owner, err := v.pages.EntityCampaignID(ctx, page)
	if err != nil {
		return notHere(err, "lists a page that isn't in this campaign")
	}
	if owner != campaignID {
		return apperror.NewValidation("lists a page that isn't in this campaign")
	}
	return nil
}

// notHere turns a not-found (or a refused id) into the Director's validation
// message and passes anything else through, so a database failure is still
// reported as one.
func notHere(err error, msg string) error {
	var ae *apperror.AppError
	if errors.As(err, &ae) && ae.Code >= http.StatusBadRequest && ae.Code < http.StatusInternalServerError {
		return apperror.NewValidation(msg)
	}
	return err
}

// unique refuses a list that names an id twice, so a mark can't count one
// creature or add-on more than once.
func unique(ids []string, what string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if seen[id] {
			return nil, apperror.NewValidation("names the same " + what + " twice")
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, nil
}
