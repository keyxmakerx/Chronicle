// campaign_binding.go — the Foundry module's side of the per-campaign update
// modes.
//
// The packages plugin owns what a mode means; this file only says where the
// Foundry module keeps a campaign's choice. That is the existing pin and pin
// mode in the campaign's settings, which resolveCampaignManifest already
// serves from, so the modes are a view over them (see UpdateModeFromLegacy)
// rather than a second copy that could drift from what is actually served.
package foundry_vtt

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/plugins/packages"
)

// CampaignPinRow is one campaign's stored pin and pin mode.
type CampaignPinRow struct {
	CampaignID   string
	CampaignName string
	OwnerName    string
	Pin          string
	PinMode      string
}

// CampaignPinLister lists every campaign with its stored pin and pin mode.
// A separate narrow interface (not a Repository method) so it adds no burden
// to the Repository fakes.
type CampaignPinLister interface {
	AllCampaignPins(ctx context.Context) ([]CampaignPinRow, error)
}

type campaignPinLister struct {
	db *sql.DB
}

// NewCampaignPinLister builds the MariaDB implementation.
func NewCampaignPinLister(db *sql.DB) CampaignPinLister {
	return &campaignPinLister{db: db}
}

// AllCampaignPins reads the two settings keys straight from the campaigns
// settings JSON, like the other campaign queries in this plugin. A JSON null
// can unquote to the literal text "null", which is read as unset.
func (l *campaignPinLister) AllCampaignPins(ctx context.Context) ([]CampaignPinRow, error) {
	rows, err := l.db.QueryContext(ctx, `
		SELECT c.id, c.name, COALESCE(u.display_name, ''),
		       COALESCE(JSON_UNQUOTE(JSON_EXTRACT(c.settings, '$.foundry_module_pin')), ''),
		       COALESCE(JSON_UNQUOTE(JSON_EXTRACT(c.settings, '$.foundry_module_pin_mode')), '')
		  FROM campaigns c
		  LEFT JOIN users u ON u.id = c.created_by
		  ORDER BY c.name`)
	if err != nil {
		return nil, fmt.Errorf("listing campaign pins: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []CampaignPinRow
	for rows.Next() {
		var r CampaignPinRow
		if err := rows.Scan(&r.CampaignID, &r.CampaignName, &r.OwnerName, &r.Pin, &r.PinMode); err != nil {
			return nil, fmt.Errorf("scanning campaign pin: %w", err)
		}
		if r.Pin == "null" {
			r.Pin = ""
		}
		if r.PinMode == "null" {
			r.PinMode = ""
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// campaignBinding implements packages.CampaignBinding for the Foundry module.
type campaignBinding struct {
	svc      Service
	settings CampaignSettingsAdapter
	pins     CampaignPinLister
}

// NewCampaignBinding returns the packages.CampaignBinding for the Foundry
// module. Register it with the packages update service at boot.
func NewCampaignBinding(svc Service, settings CampaignSettingsAdapter, pins CampaignPinLister) packages.CampaignBinding {
	return &campaignBinding{svc: svc, settings: settings, pins: pins}
}

func (b *campaignBinding) PackageType() packages.PackageType {
	return packages.PackageTypeFoundryModule
}

// UsedBy is true for any existing campaign: the module is offered to all of
// them through their own install URL.
func (b *campaignBinding) UsedBy(ctx context.Context, campaignID string, _ *packages.Package) (bool, error) {
	return b.settings.CampaignExists(ctx, campaignID)
}

func (b *campaignBinding) State(ctx context.Context, campaignID string, _ *packages.Package) (packages.CampaignPackageState, error) {
	pin, err := b.settings.GetFoundryModulePin(ctx, campaignID)
	if err != nil {
		return packages.CampaignPackageState{}, err
	}
	// A pin_mode read failure must not hide a real pin; the pin alone still
	// maps to pinned.
	pinMode, _ := b.settings.GetFoundryModulePinMode(ctx, campaignID)
	mode, version := UpdateModeFromLegacy(pin, pinMode)
	return packages.CampaignPackageState{CampaignID: campaignID, Mode: mode, Version: version, Explicit: pinMode == PinModeApproveFirst}, nil
}

func (b *campaignBinding) Campaigns(ctx context.Context, _ *packages.Package) ([]packages.CampaignPackageState, error) {
	rows, err := b.pins.AllCampaignPins(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]packages.CampaignPackageState, 0, len(rows))
	for _, r := range rows {
		mode, version := UpdateModeFromLegacy(r.Pin, r.PinMode)
		out = append(out, packages.CampaignPackageState{
			CampaignID: r.CampaignID, CampaignName: r.CampaignName, OwnerName: r.OwnerName,
			Mode: mode, Version: version, Explicit: r.PinMode == PinModeApproveFirst,
		})
	}
	return out, nil
}

// Apply writes the pin through the existing Service paths, so the on-disk
// check and the admin force-pin audit trail are the ones already in use, and
// then records the mode. Automatic clears the pin: that is what "latest
// tracking" has always meant here.
func (b *campaignBinding) Apply(ctx context.Context, campaignID string, _ *packages.Package, mode packages.UpdateMode, version string, actor packages.ActorInfo) error {
	switch mode {
	case packages.UpdateModeAutomatic:
		if err := b.svc.SetPinnedVersion(ctx, campaignID, ""); err != nil {
			return err
		}
		return b.settings.SetFoundryModulePinMode(ctx, campaignID, PinModePromote)
	case packages.UpdateModePinned, packages.UpdateModeApproveFirst:
		var err error
		if actor.Admin {
			err = b.svc.ForcePinCampaign(ctx, campaignID, version, actor.UserID, actor.IP, actor.UserAgent)
		} else {
			err = b.svc.SetPinnedVersion(ctx, campaignID, version)
		}
		if err != nil {
			return err
		}
		stored := PinModePinned
		if mode == packages.UpdateModeApproveFirst {
			stored = PinModeApproveFirst
		}
		return b.settings.SetFoundryModulePinMode(ctx, campaignID, stored)
	default:
		return fmt.Errorf("unknown update mode %q", mode)
	}
}

// Freeze sets only the pin. The stored pin mode is left exactly as it was, so
// a "preserve" campaign stays "preserve".
func (b *campaignBinding) Freeze(ctx context.Context, campaignID string, _ *packages.Package, _ packages.UpdateMode, version string) error {
	return b.svc.SetPinnedVersion(ctx, campaignID, version)
}
