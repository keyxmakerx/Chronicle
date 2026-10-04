// owner_updates.go — what a campaign owner sees and does about a new Foundry
// module version.
//
// A new version never moves a campaign: it waits until the owner presses
// Update. The packages plugin owns the rules (who may move, what is waiting,
// the admin's hold); this file only shapes that standing into what the two
// owner screens show, so the handlers stay bind/call/render.
package foundry_vtt

import (
	"context"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/packages"
)

// OwnerUpdateView is one campaign's standing on the Foundry module, shaped for
// the dashboard line and the Foundry page's Module version card.
type OwnerUpdateView struct {
	CampaignID   string
	CampaignName string

	// Package is the module's display name.
	Package string

	// Running is the version the campaign's Foundry world loads.
	Running string

	// Ready is the newer version waiting for the owner; "" when up to date.
	Ready string

	// Notes are the first lines of Ready's release notes, and NotesURL the
	// page they come from; both empty when the release carries none.
	Notes    []string
	NotesURL string

	// Dismissed is true once the owner answered "Later" to Ready.
	Dismissed bool

	// AdminHold is true while a site admin keeps the campaign on Running.
	AdminHold bool

	// Mode is how new versions reach the campaign: automatic, approve_first
	// or pinned (packages.UpdateMode values).
	Mode string

	// Versions are the installed versions the owner may go back to.
	Versions []string

	// Done is a one-line result shown in place of the prompt right after an
	// action ("Updated to ...").
	Done string
}

// OwnerUpdates is the owner's side of the update flow.
type OwnerUpdates interface {
	// View returns the campaign's standing, or nil when there is no module
	// installed to talk about.
	View(ctx context.Context, campaignID string) (*OwnerUpdateView, error)

	// Update moves the campaign onto the waiting version, which the caller
	// names so a stale page cannot approve a different one.
	Update(ctx context.Context, campaignID, version string, actor packages.ActorInfo) error

	// Later hides the prompt for the waiting version.
	Later(ctx context.Context, campaignID, version string) error

	// Switch keeps the campaign on another installed version.
	Switch(ctx context.Context, campaignID, version string, actor packages.ActorInfo) error

	// SetMode changes how new versions reach the campaign. An admin hold
	// refuses it, as it refuses every other owner move.
	SetMode(ctx context.Context, campaignID, mode string, actor packages.ActorInfo) error
}

type ownerUpdates struct {
	updates packages.CampaignUpdateService
	reg     *PackageRegistry
	pkgs    PackageReader
}

// NewOwnerUpdates wires the owner flow to the packages update service.
func NewOwnerUpdates(updates packages.CampaignUpdateService, pkgs PackageReader) OwnerUpdates {
	return &ownerUpdates{updates: updates, reg: NewPackageRegistry(pkgs), pkgs: pkgs}
}

// modulePackage finds the installed, approved module, or nil.
func (o *ownerUpdates) modulePackage(ctx context.Context) (*packages.Package, error) {
	pkg, err := o.reg.FoundryPackage(ctx)
	if err != nil {
		return nil, err
	}
	if pkg == nil || pkg.InstalledVersion == "" || pkg.Status != packages.StatusApproved {
		return nil, nil
	}
	return pkg, nil
}

func (o *ownerUpdates) View(ctx context.Context, campaignID string) (*OwnerUpdateView, error) {
	pkg, err := o.modulePackage(ctx)
	if err != nil || pkg == nil {
		return nil, err
	}
	st, err := o.updates.CampaignState(ctx, campaignID, pkg.ID)
	if err != nil {
		return nil, err
	}
	v := &OwnerUpdateView{
		CampaignID: campaignID,
		Package:    pkg.Name,
		Running:    st.EffectiveVersion,
		Ready:      st.HeldVersion,
		Dismissed:  st.HeldVersion != "" && st.DismissedVersion == st.HeldVersion,
		AdminHold:  st.AdminHold,
		Mode:       string(st.Mode),
	}
	if v.Ready != "" {
		o.addNotes(ctx, pkg, v)
	}
	// The picker is only needed by an owner who can use it.
	if !v.AdminHold {
		if v.Versions, err = o.updates.InstalledVersions(ctx, pkg.ID); err != nil {
			return nil, err
		}
	}
	return v, nil
}

// addNotes attaches the waiting release's notes. Notes are a courtesy: a
// failed lookup leaves the line without them rather than hiding the update.
func (o *ownerUpdates) addNotes(ctx context.Context, pkg *packages.Package, v *OwnerUpdateView) {
	vers, err := o.pkgs.ListVersions(ctx, pkg.ID)
	if err != nil {
		return
	}
	for _, pv := range vers {
		if pv.Version != v.Ready {
			continue
		}
		v.Notes = packages.ReleaseNoteLines(pv.ReleaseNotes, 3)
		// Only a web link is offered: the address comes from the release feed.
		if strings.HasPrefix(pv.ReleaseURL, "https://") {
			v.NotesURL = pv.ReleaseURL
		}
		return
	}
}

func (o *ownerUpdates) Update(ctx context.Context, campaignID, version string, actor packages.ActorInfo) error {
	pkg, err := o.modulePackage(ctx)
	if err != nil {
		return err
	}
	if pkg == nil {
		return apperror.NewNotFound("the Foundry module is not installed")
	}
	_, err = o.updates.ApproveVersion(ctx, campaignID, pkg.ID, version, actor)
	return err
}

func (o *ownerUpdates) Later(ctx context.Context, campaignID, version string) error {
	pkg, err := o.modulePackage(ctx)
	if err != nil {
		return err
	}
	if pkg == nil {
		return apperror.NewNotFound("the Foundry module is not installed")
	}
	_, err = o.updates.DismissHeld(ctx, campaignID, pkg.ID, version)
	return err
}

func (o *ownerUpdates) Switch(ctx context.Context, campaignID, version string, actor packages.ActorInfo) error {
	pkg, err := o.modulePackage(ctx)
	if err != nil {
		return err
	}
	if pkg == nil {
		return apperror.NewNotFound("the Foundry module is not installed")
	}
	_, err = o.updates.SwitchVersion(ctx, campaignID, pkg.ID, version, actor)
	return err
}

func (o *ownerUpdates) SetMode(ctx context.Context, campaignID, mode string, actor packages.ActorInfo) error {
	pkg, err := o.modulePackage(ctx)
	if err != nil {
		return err
	}
	if pkg == nil {
		return apperror.NewNotFound("the Foundry module is not installed")
	}
	// "Stay on one version" stays on the version running now; the version
	// list is for Use another version.
	_, err = o.updates.SetMode(ctx, packages.SetUpdateModeInput{CampaignID: campaignID, PackageID: pkg.ID, Mode: mode}, actor)
	return err
}
