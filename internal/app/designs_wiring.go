package app

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/designs"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// designsDirectory answers the saved-designs plugin's questions about
// campaigns over the campaigns service, so that plugin never imports it.
type designsDirectory struct{ svc campaigns.CampaignService }

var _ designs.CampaignDirectory = designsDirectory{}

func (d designsDirectory) Campaign(ctx context.Context, id string) (*designs.CampaignInfo, error) {
	c, err := d.svc.GetByID(ctx, id)
	if err != nil {
		// A missing or trashed campaign is a clean "none", not a failure.
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &designs.CampaignInfo{ID: c.ID, Name: c.Name, SystemID: c.ParseSettings().SystemID}, nil
}

// Live is true for a campaign that exists, is not in the Trash and is not
// archived.
func (d designsDirectory) Live(ctx context.Context, id string) (bool, error) {
	c, err := d.svc.GetByID(ctx, id)
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return !c.IsArchived(), nil
}

// Member counts only the Owner role. A co-DM shares the Owner's visibility
// role but is not the campaign's Owner, and saved designs are the Owner's.
func (d designsDirectory) Member(ctx context.Context, campaignID, userID string) (string, bool, error) {
	m, err := d.svc.GetMember(ctx, campaignID, userID)
	if err != nil {
		if isNotFound(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return m.DisplayName, m.Role >= campaigns.RoleOwner, nil
}

func (d designsDirectory) OwnedCampaigns(ctx context.Context, userID string) ([]designs.CampaignInfo, error) {
	owned, err := d.svc.OwnedCampaigns(ctx, userID)
	if err != nil {
		return nil, err
	}
	var out []designs.CampaignInfo
	for _, o := range owned {
		// OwnedCampaigns also lists campaigns the user created but no longer
		// owns, and archived ones, which are read-only; neither can take a design.
		if _, isOwner, err := d.Member(ctx, o.ID, userID); err != nil || !isOwner {
			continue
		}
		c, err := d.svc.GetByID(ctx, o.ID)
		if err != nil || c.ArchivedAt != nil {
			continue
		}
		out = append(out, designs.CampaignInfo{ID: c.ID, Name: c.Name, SystemID: c.ParseSettings().SystemID})
	}
	return out, nil
}

// galleryUserNames gives the gallery's admin queue a publisher's real display
// name, which can differ from the shown-as name they chose.
type galleryUserNames struct{ auth auth.AuthService }

var _ designs.UserNames = galleryUserNames{}

func (n galleryUserNames) DisplayName(ctx context.Context, userID string) (string, error) {
	u, err := n.auth.GetUser(ctx, userID)
	if err != nil || u == nil {
		return "", err
	}
	return u.DisplayName, nil
}

// roomThemeKind is the armory's say over designs of kind room-theme: the same
// counter-theme rules a saved room is held to. It has no needs of the
// receiving campaign, so Lacks says nothing.
type roomThemeKind struct{}

var _ designs.Kind = roomThemeKind{}

func (roomThemeKind) Validate(raw json.RawMessage) (json.RawMessage, error) {
	return armory.NormalizeShopRoomCounter(raw)
}

func (roomThemeKind) Lacks(context.Context, string, json.RawMessage) ([]string, error) {
	return nil, nil
}

// newDesignKinds is the registry every feature adds its design kinds to.
// Crafting registers crafting-circle, crafting-item-look and crafting-bench on
// it when its plugin is up (see the designs plugin's .ai.md).
func newDesignKinds() *designs.Kinds {
	k := designs.NewKinds()
	k.Register(designs.KindRoomTheme, roomThemeKind{})
	return k
}

// designsPackageSource reads the designs a game system package ships, over
// the systems package's validated loader, so the designs plugin never imports
// it. Package designs are only read for a loaded system.
type designsPackageSource struct{}

var _ designs.PackageSource = designsPackageSource{}

func (designsPackageSource) Package(ctx context.Context, systemID string) (string, []designs.PackageItem) {
	m := systems.Find(systemID)
	if m == nil {
		return "", nil
	}
	loaded, _ := systems.PackageDesignsFor(ctx, systemID)
	items := make([]designs.PackageItem, len(loaded))
	for i, d := range loaded {
		items[i] = designs.PackageItem{Kind: d.Kind, Name: d.Name, Note: d.Note, Maker: d.Maker, File: d.File, Data: d.Data}
	}
	return m.Name, items
}

// designKindChecker lends the designs kind registry to the systems loader, so
// a package's design files are held to the same rules as a saved one.
type designKindChecker struct{ kinds designs.KindRegistry }

var _ systems.DesignKindChecker = designKindChecker{}

func (c designKindChecker) ValidateDesign(_ context.Context, kind string, raw json.RawMessage) (json.RawMessage, error) {
	k, ok := c.kinds.Kind(kind)
	if !ok {
		return nil, fmt.Errorf("%q is not a kind of design this server knows", kind)
	}
	return k.Validate(raw)
}
