// system_binding.go — the game-system CampaignBinding.
//
// A game system has no per-campaign state anywhere else, so its mode and
// version live in campaign_package_updates. A campaign with no row is
// automatic, which is how every campaign followed a system before modes.
package packages

import (
	"context"
	"fmt"
)

type systemBinding struct {
	repo CampaignUpdateRepository
	pkgs updatePackageSource
}

func (b *systemBinding) PackageType() PackageType { return PackageTypeSystem }

// stateFromRow maps a stored row to the binding's view. A mode that keeps no
// version drops any stray one so automatic always reads as "follows installed".
func stateFromRow(row *CampaignUpdateRow) CampaignPackageState {
	if row == nil || !row.Mode.Valid() {
		return CampaignPackageState{Mode: UpdateModeAutomatic}
	}
	st := CampaignPackageState{Mode: row.Mode, Explicit: true}
	if row.Mode.KeepsVersion() {
		st.Version = row.Version
	}
	return st
}

func (b *systemBinding) State(ctx context.Context, campaignID string, pkg *Package) (CampaignPackageState, error) {
	row, err := b.repo.Get(ctx, campaignID, pkg.ID)
	if err != nil {
		return CampaignPackageState{}, err
	}
	return stateFromRow(row), nil
}

func (b *systemBinding) UsedBy(ctx context.Context, campaignID string, pkg *Package) (bool, error) {
	states, err := b.Campaigns(ctx, pkg)
	if err != nil {
		return false, err
	}
	for _, st := range states {
		if st.CampaignID == campaignID {
			return true, nil
		}
	}
	return false, nil
}

// Campaigns is everyone who has the system's addon enabled (automatic unless
// a row says otherwise) plus anyone with a stored choice, so a campaign that
// later turned the addon off still keeps its pin protected and listed.
func (b *systemBinding) Campaigns(ctx context.Context, pkg *Package) ([]CampaignPackageState, error) {
	usage, err := b.pkgs.GetUsage(ctx, pkg.ID)
	if err != nil {
		return nil, fmt.Errorf("listing system usage: %w", err)
	}
	rows, err := b.repo.ListByPackage(ctx, pkg.ID)
	if err != nil {
		return nil, err
	}
	byID := map[string]*CampaignPackageState{}
	var order []string
	for _, u := range usage {
		if _, dup := byID[u.CampaignID]; dup {
			continue
		}
		byID[u.CampaignID] = &CampaignPackageState{
			CampaignID: u.CampaignID, CampaignName: u.CampaignName, Mode: UpdateModeAutomatic,
		}
		order = append(order, u.CampaignID)
	}
	for i := range rows {
		r := &rows[i]
		st := stateFromRow(r)
		if cur, ok := byID[r.CampaignID]; ok {
			st.CampaignID, st.CampaignName = cur.CampaignID, cur.CampaignName
		} else {
			st.CampaignID, st.CampaignName = r.CampaignID, r.CampaignName
			order = append(order, r.CampaignID)
		}
		byID[r.CampaignID] = &st
	}
	out := make([]CampaignPackageState, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out, nil
}

func (b *systemBinding) Apply(ctx context.Context, campaignID string, pkg *Package, mode UpdateMode, version string, _ ActorInfo) error {
	if !mode.KeepsVersion() {
		version = ""
	}
	return b.repo.UpsertModeVersion(ctx, campaignID, pkg.ID, mode, version)
}

// Freeze stores the version a campaign was running without touching anything
// else about its choice.
func (b *systemBinding) Freeze(ctx context.Context, campaignID string, pkg *Package, mode UpdateMode, version string) error {
	return b.repo.UpsertModeVersion(ctx, campaignID, pkg.ID, mode, version)
}
