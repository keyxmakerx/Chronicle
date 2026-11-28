package app

import (
	"context"
	"encoding/json"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/designs"
	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
)

// homebrewRecipeKind is the Store kind a shared Workshop recipe travels as.
const homebrewRecipeKind = "homebrew-recipe"

func init() {
	// A packaged recipe records the game system only when a step runs in the
	// Director's browser, so the Workshop is told how to read a campaign's.
	addWorkshopSource("recipe sharing", func(reg *homebrew.Registry, d workshopDeps) error {
		if d.Systems == nil {
			return nil
		}
		reg.SetSystem(func(ctx context.Context, campaignID string) string {
			if sys := d.Systems.EnabledSystem(ctx, campaignID); sys != nil {
				return sys.Info().ID
			}
			return ""
		})
		return nil
	})
}

// storeMembers says who is on a campaign's Director team, the way a request
// would, for calls that arrive with a user id and no request.
type storeMembers interface {
	GetMember(ctx context.Context, campaignID, userID string) (*campaigns.CampaignMember, error)
	IsUserDmGranted(ctx context.Context, campaignID, userID string) (bool, error)
}

// homebrewAddResult is what adding a recipe made: its id, and one plain
// sentence for each link the Director still has to choose.
type homebrewAddResult struct {
	ID          string
	LeftToPoint []string
}

// homebrewStoreKind is the Workshop's say over designs of kind
// homebrew-recipe. It only passes through to the Workshop's service, which
// holds every rule, so the Store never reads a recipe.
type homebrewStoreKind struct {
	svc     homebrew.Service
	members storeMembers
}

var _ designs.Kind = homebrewStoreKind{}

func (k homebrewStoreKind) Validate(raw json.RawMessage) (json.RawMessage, error) {
	return k.svc.ValidatePackage(raw)
}

func (k homebrewStoreKind) Lacks(ctx context.Context, campaignID string, raw json.RawMessage) ([]string, error) {
	return k.svc.PackageLacks(ctx, campaignID, raw)
}

// Strip drops what stays with the sharing campaign; it runs on share and
// again on add.
func (k homebrewStoreKind) Strip(raw json.RawMessage) (json.RawMessage, error) {
	return k.svc.StripPackage(raw)
}

// Package writes one of the campaign's recipes as a package. Only the
// Director team may.
func (k homebrewStoreKind) Package(ctx context.Context, campaignID, userID, recipeID string) (json.RawMessage, error) {
	v, err := k.viewer(ctx, campaignID, userID)
	if err != nil {
		return nil, err
	}
	return k.svc.Package(ctx, campaignID, recipeID, v)
}

// Add makes a new recipe in the campaign, switched off. Only the Director
// team may.
func (k homebrewStoreKind) Add(ctx context.Context, campaignID, userID string, raw json.RawMessage) (homebrewAddResult, error) {
	v, err := k.viewer(ctx, campaignID, userID)
	if err != nil {
		return homebrewAddResult{}, err
	}
	added, err := k.svc.AddPackaged(ctx, campaignID, v, raw)
	if err != nil {
		return homebrewAddResult{}, err
	}
	return homebrewAddResult{ID: added.ID, LeftToPoint: added.LeftToPoint}, nil
}

// viewer is the user as the Workshop's routes would see them: a Director is
// the campaign's Owner or someone the Owner granted the Director's view. A
// stranger is no one's Director, and the service refuses them.
func (k homebrewStoreKind) viewer(ctx context.Context, campaignID, userID string) (homebrew.Viewer, error) {
	v := homebrew.Viewer{UserID: userID}
	m, err := k.members.GetMember(ctx, campaignID, userID)
	if err != nil {
		if isNotFound(err) {
			return v, nil
		}
		return v, err
	}
	if m.Role >= campaigns.RoleOwner {
		v.Director = true
		return v, nil
	}
	granted, err := k.members.IsUserDmGranted(ctx, campaignID, userID)
	if err != nil {
		return v, err
	}
	v.Director = granted
	return v, nil
}
