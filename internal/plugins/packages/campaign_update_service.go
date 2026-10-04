// campaign_update_service.go — per-campaign update modes for packages.
//
// The packages plugin stays type-agnostic: it decides what a mode MEANS
// (hold, freeze, approve) and asks a per-type CampaignBinding where a
// campaign's mode and version are stored. Game systems have no per-campaign
// state anywhere else, so their binding keeps it in campaign_package_updates.
// The Foundry module already keeps a pin and pin mode in the campaign's
// settings (and its manifest endpoint serves from them), so its binding, in
// the foundry_vtt plugin, maps that onto these modes instead of creating a
// second copy. The held version of a newly installed release lives in
// campaign_package_updates for every type.
package packages

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"sync"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// Audit event types written when a campaign's update choice changes. Same
// shape and sink as the Foundry force-pin events.
const (
	EventCampaignUpdateModeSet  = "packages.campaign_update_mode_set"
	EventCampaignUpdateHeld     = "packages.campaign_update_held"
	EventCampaignUpdateApproved = "packages.campaign_update_approved"

	// EventCampaignUpdateSwitched is a move to a version the owner or an admin
	// picked rather than the one that was waiting.
	EventCampaignUpdateSwitched  = "packages.campaign_update_switched"
	EventCampaignUpdateDismissed = "packages.campaign_update_dismissed"
	EventCampaignUpdateAdminHold = "packages.campaign_update_admin_hold"
)

// AuditLogger writes one audit row per action. Same shape as the security
// event logger so the app can pass it straight through; nil disables it.
type AuditLogger interface {
	LogEvent(ctx context.Context, eventType, userID, actorID, ip, userAgent string, details map[string]any) error
}

// updatePackageSource is the part of PackageService the update modes need,
// kept narrow so tests can fake it.
type updatePackageSource interface {
	ListPackages(ctx context.Context) ([]Package, error)
	GetPackage(ctx context.Context, id string) (*Package, error)
	GetUsage(ctx context.Context, packageID string) ([]PackageUsage, error)
	InstallDirForVersion(pkgType PackageType, slug, version string) string
	ListVersions(ctx context.Context, packageID string) ([]PackageVersion, error)
	// lockForPackage is the per-package mutex install and clean-up already
	// share; taking it keeps a check-then-write on a version folder atomic
	// against clean-up.
	lockForPackage(packageID string) *sync.Mutex
}

// pkgSource adapts a PackageService to updatePackageSource. Only the real
// service has the shared lock; any other implementation gets a private one.
type pkgSource struct {
	PackageService
	fallback sync.Mutex
}

func (p *pkgSource) lockForPackage(id string) *sync.Mutex {
	if s, ok := p.PackageService.(*packageService); ok {
		return s.lockForPackage(id)
	}
	return &p.fallback
}

// CampaignBinding is how one package type stores a campaign's mode and
// version. Implemented by the packages plugin for game systems and by the
// foundry_vtt plugin for the Foundry module.
type CampaignBinding interface {
	// PackageType is the type this binding serves.
	PackageType() PackageType

	// UsedBy reports whether the campaign uses the package at all.
	UsedBy(ctx context.Context, campaignID string, pkg *Package) (bool, error)

	// State reports the campaign's stored mode and explicit version. Mode and
	// Version are the only fields it must fill.
	State(ctx context.Context, campaignID string, pkg *Package) (CampaignPackageState, error)

	// Campaigns lists every campaign that uses the package with its mode,
	// version and campaign identity (id, name, owner).
	Campaigns(ctx context.Context, pkg *Package) ([]CampaignPackageState, error)

	// Apply stores mode and version. The service has already validated them
	// (mode known, version installed on disk); version is "" for automatic.
	Apply(ctx context.Context, campaignID string, pkg *Package, mode UpdateMode, version string, actor ActorInfo) error

	// Freeze records the version a campaign was running at install time and
	// nothing else: it must not rewrite any stored mode, so a campaign whose
	// mode is derived from older data keeps that data as it was.
	Freeze(ctx context.Context, campaignID string, pkg *Package, mode UpdateMode, version string) error
}

// AsksByDefaultBinding is implemented by a binding whose package type never
// moves a campaign to a new version on its own. For such a type a campaign
// with no choice of its own is frozen where it is the first time a version
// would move it, and a pinned campaign is offered the waiting version too,
// so every campaign ends up either on its version or asked. Game systems do
// not implement it and keep following the installed version.
type AsksByDefaultBinding interface {
	AsksByDefault() bool
}

// SetUpdateModeInput is the owner's request to change one package's mode.
type SetUpdateModeInput struct {
	CampaignID string
	PackageID  string
	Mode       string
	// Version is the version to stay on. Only read for "pinned"; empty there
	// means "the version the campaign is on now".
	Version string
}

// ServedVersion is the version (and folder) a campaign's files should be
// served from, and whether that is the campaign's own choice or the fallback.
type ServedVersion struct {
	Version string
	Dir     string
	// FellBack is true when the campaign's own version could not be used (its
	// folder is gone, or the lookup failed) and the installed version is
	// served instead. Reason says why.
	FellBack bool
	Reason   string
}

// ReconcileResult counts what the boot reconciler changed.
type ReconcileResult struct {
	HeldSet     int
	HeldCleared int
	RowsRemoved int64
}

// CampaignUpdateService owns the per-campaign update modes.
//
// It never checks that the caller may act: the owner/admin gate belongs to
// the handler that will sit on top, which passes ActorInfo.Admin to say which.
type CampaignUpdateService interface {
	// CampaignState returns one campaign's mode, version and held update for
	// one package.
	CampaignState(ctx context.Context, campaignID, packageID string) (*CampaignPackageState, error)

	// CampaignStates returns the campaign's standing on every installed
	// package it uses (its game-system package and the Foundry module).
	CampaignStates(ctx context.Context, campaignID string) ([]CampaignPackageState, error)

	// SetMode changes a campaign's mode for one package (owner action).
	SetMode(ctx context.Context, in SetUpdateModeInput, actor ActorInfo) (*CampaignPackageState, error)

	// ApproveHeld moves an approve_first campaign onto its held version. The
	// owner calls it with actor.Admin false; a site admin approving on the
	// owner's behalf passes true.
	ApproveHeld(ctx context.Context, campaignID, packageID string, actor ActorInfo) (*CampaignPackageState, error)

	// ApproveVersion is ApproveHeld for a caller that names the version it was
	// shown: it fails if a different one is waiting by now.
	ApproveVersion(ctx context.Context, campaignID, packageID, version string, actor ActorInfo) (*CampaignPackageState, error)

	// SwitchVersion keeps a campaign on a version the caller picked (an owner
	// going back from a new version, or an admin moving it). The version must
	// be a known release whose folder is on disk. An owner is refused while a
	// site admin holds the campaign; an admin (actor.Admin) is not.
	SwitchVersion(ctx context.Context, campaignID, packageID, version string, actor ActorInfo) (*CampaignPackageState, error)

	// DismissHeld records that the owner answered "Later" to the waiting
	// version, which must still be the one waiting.
	DismissHeld(ctx context.Context, campaignID, packageID, version string) (*CampaignPackageState, error)

	// SetAdminHold keeps a campaign on its current version against its
	// owner's Update, or lets go. A campaign that has no version of its own
	// yet is frozen on the one it runs first.
	SetAdminHold(ctx context.Context, campaignID, packageID string, hold bool, actor ActorInfo) (*CampaignPackageState, error)

	// InstalledVersions lists the package's known releases whose folder is on
	// disk, newest first: the versions a campaign can be moved to.
	InstalledVersions(ctx context.Context, packageID string) ([]string, error)

	// CampaignsOnPackage lists every campaign on a package with its mode,
	// version and any held update (the admin panel's Campaigns tab).
	CampaignsOnPackage(ctx context.Context, packageID string) ([]CampaignPackageState, error)

	// HeldUpdates lists every campaign currently holding an update.
	HeldUpdates(ctx context.Context) ([]CampaignPackageState, error)

	// ServedVersion resolves the version a campaign's files come from. It
	// never errors: anything it cannot honour falls back to the installed
	// version, which is what every campaign was served before modes existed.
	ServedVersion(ctx context.Context, campaignID string, pkg *Package) ServedVersion

	// OnInstall runs before an install is committed: pinned and approve_first
	// campaigns are frozen on the version they were running. It writes no
	// holds and no audit rows, so a refused install leaves none behind. A
	// campaign whose mode was set explicitly and cannot be frozen fails the
	// call so the install is refused rather than silently moving it; a
	// campaign whose mode is derived from older data is logged and skipped,
	// as before.
	OnInstall(ctx context.Context, pkg *Package, previousVersion, newVersion string) error

	// OnInstalled runs after the install is committed and hands approve_first
	// campaigns the new version to approve (writing the hold and its audit
	// event). pkg.InstalledVersion is the new version.
	OnInstalled(ctx context.Context, pkg *Package, previousVersion, newVersion string) error

	// KeptVersions is every system version some campaign is on or holds,
	// keyed by package slug, for the clean-up protected set.
	KeptVersions(ctx context.Context) (map[string]map[string]bool, error)

	// Reconcile is the idempotent boot pass that makes held versions agree
	// with the installed version and drops rows that say nothing.
	Reconcile(ctx context.Context) (ReconcileResult, error)

	// RegisterBinding attaches a package type's binding (boot-time wiring).
	RegisterBinding(b CampaignBinding)
}

type campaignUpdateService struct {
	repo     CampaignUpdateRepository
	pkgs     updatePackageSource
	audit    AuditLogger
	bindings map[PackageType]CampaignBinding
}

// NewCampaignUpdateService wires the service with the game-system binding
// already registered. audit may be nil.
func NewCampaignUpdateService(repo CampaignUpdateRepository, pkgs PackageService, audit AuditLogger) CampaignUpdateService {
	return newCampaignUpdateService(repo, &pkgSource{PackageService: pkgs}, audit)
}

func newCampaignUpdateService(repo CampaignUpdateRepository, pkgs updatePackageSource, audit AuditLogger) *campaignUpdateService {
	s := &campaignUpdateService{
		repo:     repo,
		pkgs:     pkgs,
		audit:    audit,
		bindings: map[PackageType]CampaignBinding{},
	}
	s.RegisterBinding(&systemBinding{repo: repo, pkgs: pkgs})
	return s
}

func (s *campaignUpdateService) RegisterBinding(b CampaignBinding) {
	s.bindings[b.PackageType()] = b
}

// loadPackage fetches a package and its binding, mapping absence to the
// errors a handler can show.
func (s *campaignUpdateService) loadPackage(ctx context.Context, packageID string) (*Package, CampaignBinding, error) {
	pkg, err := s.pkgs.GetPackage(ctx, packageID)
	if err != nil {
		return nil, nil, apperror.NewInternal(fmt.Errorf("loading package: %w", err))
	}
	if pkg == nil {
		return nil, nil, apperror.NewNotFound("package not found")
	}
	b, ok := s.bindings[pkg.Type]
	if !ok {
		return nil, nil, apperror.NewBadRequest("this package type has no update choice")
	}
	return pkg, b, nil
}

// stateOf assembles the full state: the binding's mode/version, the package
// identity, the effective version, and the held version (only meaningful
// while the mode is approve_first).
func (s *campaignUpdateService) stateOf(ctx context.Context, b CampaignBinding, campaignID string, pkg *Package) (CampaignPackageState, error) {
	st, err := b.State(ctx, campaignID, pkg)
	if err != nil {
		return CampaignPackageState{}, apperror.NewInternal(fmt.Errorf("reading campaign mode: %w", err))
	}
	st.CampaignID = campaignID
	if st.Mode == "" {
		st.Mode = UpdateModeAutomatic
	}
	row, err := s.repo.Get(ctx, campaignID, pkg.ID)
	if err != nil {
		return CampaignPackageState{}, apperror.NewInternal(err)
	}
	if row != nil {
		applyRow(&st, b, row)
	}
	decorate(&st, pkg)
	return st, nil
}

// asksByDefault reports whether the binding's package type never moves a
// campaign on its own.
func asksByDefault(b CampaignBinding) bool {
	a, ok := b.(AsksByDefaultBinding)
	return ok && a.AsksByDefault()
}

// holds reports whether a campaign in this mode is offered a waiting version:
// ask-first always, and pinned for a type that asks by default (an owner who
// pinned a version still wants to know a newer one is ready).
func holds(b CampaignBinding, mode UpdateMode) bool {
	return mode == UpdateModeApproveFirst || (mode == UpdateModePinned && asksByDefault(b))
}

// applyRow copies the stored row's waiting version, dismissal and admin hold
// onto a state. A waiting version only counts while the mode still holds.
func applyRow(st *CampaignPackageState, b CampaignBinding, row *CampaignUpdateRow) {
	if holds(b, st.Mode) {
		st.HeldVersion = row.HeldVersion
		st.HeldAt = row.HeldAt
	}
	st.DismissedVersion = row.DismissedVersion
	st.AdminHold = row.AdminHold
	st.AdminHoldAt = row.AdminHoldAt
}

// ownerMayMove refuses an owner's move while a site admin holds the campaign.
// The admin's own actions pass.
func ownerMayMove(st CampaignPackageState, actor ActorInfo) error {
	if st.AdminHold && !actor.Admin {
		return apperror.NewForbidden(fmt.Sprintf("The site admin is keeping this campaign on %s.", st.EffectiveVersion))
	}
	return nil
}

// decorate fills the package-derived fields of a state.
func decorate(st *CampaignPackageState, pkg *Package) {
	st.PackageID = pkg.ID
	st.PackageSlug = pkg.Slug
	st.PackageType = pkg.Type
	if st.Mode == "" {
		st.Mode = UpdateModeAutomatic
	}
	st.EffectiveVersion = st.Version
	if st.EffectiveVersion == "" {
		st.EffectiveVersion = pkg.InstalledVersion
	}
}

func (s *campaignUpdateService) CampaignState(ctx context.Context, campaignID, packageID string) (*CampaignPackageState, error) {
	pkg, b, err := s.loadPackage(ctx, packageID)
	if err != nil {
		return nil, err
	}
	st, err := s.stateOf(ctx, b, campaignID, pkg)
	if err != nil {
		return nil, err
	}
	return &st, nil
}

func (s *campaignUpdateService) CampaignStates(ctx context.Context, campaignID string) ([]CampaignPackageState, error) {
	pkgs, err := s.pkgs.ListPackages(ctx)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("listing packages: %w", err))
	}
	var out []CampaignPackageState
	for i := range pkgs {
		pkg := &pkgs[i]
		b, ok := s.bindings[pkg.Type]
		if !ok || pkg.InstalledVersion == "" || pkg.Status != StatusApproved {
			continue
		}
		used, err := b.UsedBy(ctx, campaignID, pkg)
		if err != nil {
			return nil, apperror.NewInternal(fmt.Errorf("checking package use: %w", err))
		}
		if !used {
			continue
		}
		st, err := s.stateOf(ctx, b, campaignID, pkg)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, nil
}

// versionOnDisk reports whether a version's folder exists.
func (s *campaignUpdateService) versionOnDisk(pkg *Package, version string) bool {
	if !ValidVersionString(version) {
		return false
	}
	dir := s.pkgs.InstallDirForVersion(pkg.Type, pkg.Slug, version)
	if dir == "" {
		return false
	}
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}

// knownRelease reports whether version is one of the package's known releases
// (package_versions), so a free-form string never reaches a path.
func (s *campaignUpdateService) knownRelease(ctx context.Context, pkg *Package, version string) (bool, error) {
	vers, err := s.pkgs.ListVersions(ctx, pkg.ID)
	if err != nil {
		return false, err
	}
	for _, v := range vers {
		if v.Version == version {
			return true, nil
		}
	}
	return false, nil
}

func (s *campaignUpdateService) SetMode(ctx context.Context, in SetUpdateModeInput, actor ActorInfo) (*CampaignPackageState, error) {
	mode, err := ParseUpdateMode(in.Mode)
	if err != nil {
		return nil, apperror.NewValidation(err.Error())
	}
	pkg, b, err := s.loadPackage(ctx, in.PackageID)
	if err != nil {
		return nil, err
	}
	// The folder check and the write are one step as far as clean-up is
	// concerned: it takes this lock too.
	mu := s.pkgs.lockForPackage(pkg.ID)
	mu.Lock()
	defer mu.Unlock()

	cur, err := s.stateOf(ctx, b, in.CampaignID, pkg)
	if err != nil {
		return nil, err
	}

	version := ""
	switch mode {
	case UpdateModePinned:
		version = in.Version
		if version == "" {
			version = cur.EffectiveVersion
		} else {
			if !ValidVersionString(version) {
				return nil, apperror.NewValidation("that is not a valid version")
			}
			known, err := s.knownRelease(ctx, pkg, version)
			if err != nil {
				return nil, apperror.NewInternal(fmt.Errorf("listing versions: %w", err))
			}
			if !known {
				return nil, apperror.NewBadRequest(fmt.Sprintf("version %s is not a known release of this package", version))
			}
		}
	case UpdateModeApproveFirst:
		// Ask-first holds the campaign where it is; the version it is
		// asked to move to arrives with the next install.
		version = cur.EffectiveVersion
	}
	if mode.KeepsVersion() {
		if version != "" && !ValidVersionString(version) {
			return nil, apperror.NewValidation("that is not a valid version")
		}
		if version == "" {
			return nil, apperror.NewBadRequest("this package has no installed version to stay on")
		}
		if !s.versionOnDisk(pkg, version) {
			return nil, apperror.NewBadRequest(fmt.Sprintf("version %s is not installed on this server", version))
		}
	}

	if err := b.Apply(ctx, in.CampaignID, pkg, mode, version, actor); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("storing campaign mode: %w", err))
	}
	// A pending hold only survives staying in ask-first; anything else
	// answers it.
	if mode != UpdateModeApproveFirst || cur.Mode != UpdateModeApproveFirst {
		if err := s.repo.SetHeld(ctx, in.CampaignID, pkg.ID, ""); err != nil {
			return nil, apperror.NewInternal(err)
		}
	}
	s.log(ctx, EventCampaignUpdateModeSet, actor, map[string]any{
		"campaign_id": in.CampaignID, "package_id": pkg.ID, "package": pkg.Slug,
		"from_mode": string(cur.Mode), "to_mode": string(mode), "version": version,
	})
	return s.CampaignState(ctx, in.CampaignID, in.PackageID)
}

func (s *campaignUpdateService) ApproveHeld(ctx context.Context, campaignID, packageID string, actor ActorInfo) (*CampaignPackageState, error) {
	return s.approve(ctx, campaignID, packageID, "", actor)
}

func (s *campaignUpdateService) ApproveVersion(ctx context.Context, campaignID, packageID, version string, actor ActorInfo) (*CampaignPackageState, error) {
	if version == "" {
		return nil, apperror.NewValidation("say which version to update to")
	}
	return s.approve(ctx, campaignID, packageID, version, actor)
}

// approve moves a campaign onto its waiting version. When expect is set the
// waiting version must still be that one, so a page that was open while a
// newer version arrived cannot approve a version its owner never saw.
func (s *campaignUpdateService) approve(ctx context.Context, campaignID, packageID, expect string, actor ActorInfo) (*CampaignPackageState, error) {
	pkg, b, err := s.loadPackage(ctx, packageID)
	if err != nil {
		return nil, err
	}
	mu := s.pkgs.lockForPackage(pkg.ID)
	mu.Lock()
	defer mu.Unlock()

	cur, err := s.stateOf(ctx, b, campaignID, pkg)
	if err != nil {
		return nil, err
	}
	if err := ownerMayMove(cur, actor); err != nil {
		return nil, err
	}
	if !holds(b, cur.Mode) || cur.HeldVersion == "" {
		return nil, apperror.NewConflict("no update is waiting for approval")
	}
	if expect != "" && cur.HeldVersion != expect {
		return nil, apperror.NewConflict("a different version is waiting now. Reload the page to see it")
	}
	if !ValidVersionString(cur.HeldVersion) {
		return nil, apperror.NewValidation("the held version is not a valid version")
	}
	known, err := s.knownRelease(ctx, pkg, cur.HeldVersion)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("listing versions: %w", err))
	}
	if !known {
		return nil, apperror.NewBadRequest(fmt.Sprintf("version %s is not a known release of this package", cur.HeldVersion))
	}
	// Moving onto a folder that is gone would break the campaign; the update
	// stays held instead.
	if !s.versionOnDisk(pkg, cur.HeldVersion) {
		return nil, apperror.NewBadRequest(fmt.Sprintf("version %s is not installed on this server", cur.HeldVersion))
	}
	// The campaign keeps the mode it had: a pinned campaign stays pinned.
	if err := b.Apply(ctx, campaignID, pkg, cur.Mode, cur.HeldVersion, actor); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("applying held version: %w", err))
	}
	// If clearing fails the campaign is already on the version, and the
	// reconciler drops the stale hold.
	if err := s.repo.SetHeld(ctx, campaignID, pkg.ID, ""); err != nil {
		slog.Warn("update modes: approved but could not clear the held version",
			slog.String("campaign_id", campaignID), slog.Any("error", err))
	}
	s.log(ctx, EventCampaignUpdateApproved, actor, map[string]any{
		"campaign_id": campaignID, "package_id": pkg.ID, "package": pkg.Slug,
		"from": cur.EffectiveVersion, "to": cur.HeldVersion, "by_admin": actor.Admin,
	})
	return s.CampaignState(ctx, campaignID, packageID)
}

func (s *campaignUpdateService) SwitchVersion(ctx context.Context, campaignID, packageID, version string, actor ActorInfo) (*CampaignPackageState, error) {
	if !ValidVersionString(version) {
		return nil, apperror.NewValidation("that is not a valid version")
	}
	pkg, b, err := s.loadPackage(ctx, packageID)
	if err != nil {
		return nil, err
	}
	mu := s.pkgs.lockForPackage(pkg.ID)
	mu.Lock()
	defer mu.Unlock()

	if err := s.requireUses(ctx, b, campaignID, pkg); err != nil {
		return nil, err
	}
	cur, err := s.stateOf(ctx, b, campaignID, pkg)
	if err != nil {
		return nil, err
	}
	if err := ownerMayMove(cur, actor); err != nil {
		return nil, err
	}
	if err := s.requireInstalled(ctx, pkg, version); err != nil {
		return nil, err
	}
	// Going to a version keeps the campaign there, so a campaign that followed
	// the installed version now asks first like every other.
	mode := cur.Mode
	if !mode.KeepsVersion() {
		mode = UpdateModeApproveFirst
	}
	if err := b.Apply(ctx, campaignID, pkg, mode, version, actor); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("moving campaign to %s: %w", version, err))
	}
	// Newer versions stay on offer, and the one it just moved onto no longer is.
	if err := s.repo.SetHeld(ctx, campaignID, pkg.ID, wantHeld(b, pkg, mode, version)); err != nil {
		return nil, apperror.NewInternal(err)
	}
	s.log(ctx, EventCampaignUpdateSwitched, actor, map[string]any{
		"campaign_id": campaignID, "package_id": pkg.ID, "package": pkg.Slug,
		"from": cur.EffectiveVersion, "to": version, "by_admin": actor.Admin,
	})
	return s.CampaignState(ctx, campaignID, packageID)
}

func (s *campaignUpdateService) DismissHeld(ctx context.Context, campaignID, packageID, version string) (*CampaignPackageState, error) {
	pkg, b, err := s.loadPackage(ctx, packageID)
	if err != nil {
		return nil, err
	}
	mu := s.pkgs.lockForPackage(pkg.ID)
	mu.Lock()
	defer mu.Unlock()

	cur, err := s.stateOf(ctx, b, campaignID, pkg)
	if err != nil {
		return nil, err
	}
	// Answering a version that is no longer the waiting one (the page was
	// stale) must not silence the one that is.
	if cur.HeldVersion == "" || cur.HeldVersion != version {
		return nil, apperror.NewConflict("that update is no longer waiting")
	}
	if err := s.repo.SetDismissed(ctx, campaignID, pkg.ID, version); err != nil {
		return nil, apperror.NewInternal(err)
	}
	s.log(ctx, EventCampaignUpdateDismissed, ActorInfo{}, map[string]any{
		"campaign_id": campaignID, "package_id": pkg.ID, "package": pkg.Slug, "version": version,
	})
	return s.CampaignState(ctx, campaignID, packageID)
}

func (s *campaignUpdateService) SetAdminHold(ctx context.Context, campaignID, packageID string, hold bool, actor ActorInfo) (*CampaignPackageState, error) {
	pkg, b, err := s.loadPackage(ctx, packageID)
	if err != nil {
		return nil, err
	}
	mu := s.pkgs.lockForPackage(pkg.ID)
	mu.Lock()
	defer mu.Unlock()

	if err := s.requireUses(ctx, b, campaignID, pkg); err != nil {
		return nil, err
	}
	cur, err := s.stateOf(ctx, b, campaignID, pkg)
	if err != nil {
		return nil, err
	}
	if hold && (!cur.Mode.KeepsVersion() || cur.Version == "") {
		// A hold is a version the campaign stays on, so one that still
		// follows the installed version is frozen on it first.
		mode := cur.Mode
		if !mode.KeepsVersion() {
			mode = UpdateModeApproveFirst
		}
		if err := s.requireInstalled(ctx, pkg, cur.EffectiveVersion); err != nil {
			return nil, err
		}
		if err := b.Apply(ctx, campaignID, pkg, mode, cur.EffectiveVersion, actor); err != nil {
			return nil, apperror.NewInternal(fmt.Errorf("freezing campaign before hold: %w", err))
		}
		if err := s.repo.SetHeld(ctx, campaignID, pkg.ID, wantHeld(b, pkg, mode, cur.EffectiveVersion)); err != nil {
			return nil, apperror.NewInternal(err)
		}
	}
	if err := s.repo.SetAdminHold(ctx, campaignID, pkg.ID, hold); err != nil {
		return nil, apperror.NewInternal(err)
	}
	s.log(ctx, EventCampaignUpdateAdminHold, actor, map[string]any{
		"campaign_id": campaignID, "package_id": pkg.ID, "package": pkg.Slug,
		"hold": hold, "version": cur.EffectiveVersion,
	})
	return s.CampaignState(ctx, campaignID, packageID)
}

// requireUses refuses a campaign id the package does not apply to, so an id
// typed into a request cannot create a row for something that is not there.
func (s *campaignUpdateService) requireUses(ctx context.Context, b CampaignBinding, campaignID string, pkg *Package) error {
	used, err := b.UsedBy(ctx, campaignID, pkg)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("checking package use: %w", err))
	}
	if !used {
		return apperror.NewNotFound("that campaign does not use this package")
	}
	return nil
}

// requireInstalled checks a requested version before it is stored: a valid
// string, a known release, and a folder that is still on disk (so clean-up has
// not removed it).
func (s *campaignUpdateService) requireInstalled(ctx context.Context, pkg *Package, version string) error {
	if !ValidVersionString(version) {
		return apperror.NewValidation("that is not a valid version")
	}
	known, err := s.knownRelease(ctx, pkg, version)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("listing versions: %w", err))
	}
	if !known {
		return apperror.NewBadRequest(fmt.Sprintf("version %s is not a known release of this package", version))
	}
	if !s.versionOnDisk(pkg, version) {
		return apperror.NewBadRequest(fmt.Sprintf("version %s is not installed on this server", version))
	}
	return nil
}

func (s *campaignUpdateService) InstalledVersions(ctx context.Context, packageID string) ([]string, error) {
	pkg, err := s.pkgs.GetPackage(ctx, packageID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("loading package: %w", err))
	}
	if pkg == nil {
		return nil, apperror.NewNotFound("package not found")
	}
	vers, err := s.pkgs.ListVersions(ctx, pkg.ID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("listing versions: %w", err))
	}
	seen := map[string]bool{}
	var out []string
	for _, v := range vers {
		if seen[v.Version] || !s.versionOnDisk(pkg, v.Version) {
			continue
		}
		seen[v.Version] = true
		out = append(out, v.Version)
	}
	sort.Slice(out, func(i, j int) bool { return versionLess(out[j], out[i]) })
	return out, nil
}

// wantHeld is the one correct waiting version for a campaign on version in
// mode: the installed version when the campaign is offered updates and is
// behind it, otherwise nothing.
func wantHeld(b CampaignBinding, pkg *Package, mode UpdateMode, version string) string {
	if holds(b, mode) && version != "" && versionLess(version, pkg.InstalledVersion) {
		return pkg.InstalledVersion
	}
	return ""
}

func (s *campaignUpdateService) CampaignsOnPackage(ctx context.Context, packageID string) ([]CampaignPackageState, error) {
	pkg, b, err := s.loadPackage(ctx, packageID)
	if err != nil {
		return nil, err
	}
	states, err := b.Campaigns(ctx, pkg)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("listing campaigns: %w", err))
	}
	rows, err := s.repo.ListByPackage(ctx, pkg.ID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	byCampaign := map[string]*CampaignUpdateRow{}
	for i := range rows {
		byCampaign[rows[i].CampaignID] = &rows[i]
	}
	for i := range states {
		if states[i].Mode == "" {
			states[i].Mode = UpdateModeAutomatic
		}
		if row := byCampaign[states[i].CampaignID]; row != nil {
			applyRow(&states[i], b, row)
		}
		decorate(&states[i], pkg)
	}
	return states, nil
}

func (s *campaignUpdateService) HeldUpdates(ctx context.Context) ([]CampaignPackageState, error) {
	rows, err := s.repo.ListHeld(ctx)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	var out []CampaignPackageState
	for _, r := range rows {
		pkg, b, err := s.loadPackage(ctx, r.PackageID)
		if err != nil {
			continue // package removed or type unbound: nothing to approve
		}
		st, err := s.stateOf(ctx, b, r.CampaignID, pkg)
		if err != nil {
			return nil, err
		}
		if st.HeldVersion == "" {
			continue // stale hold, the reconciler clears it
		}
		st.CampaignName = r.CampaignName
		out = append(out, st)
	}
	return out, nil
}

func (s *campaignUpdateService) ServedVersion(ctx context.Context, campaignID string, pkg *Package) ServedVersion {
	installed := ServedVersion{
		Version: pkg.InstalledVersion,
		Dir:     s.pkgs.InstallDirForVersion(pkg.Type, pkg.Slug, pkg.InstalledVersion),
	}
	b, ok := s.bindings[pkg.Type]
	if !ok {
		return installed
	}
	st, err := b.State(ctx, campaignID, pkg)
	if err != nil {
		slog.Warn("update modes: could not read campaign version, serving the installed one",
			slog.String("campaign_id", campaignID), slog.String("package", pkg.Slug), slog.Any("error", err))
		installed.FellBack, installed.Reason = true, "campaign version unreadable"
		return installed
	}
	if !st.Mode.KeepsVersion() || st.Version == "" || st.Version == pkg.InstalledVersion {
		return installed
	}
	if !s.versionOnDisk(pkg, st.Version) {
		slog.Warn("update modes: campaign version folder is missing, serving the installed version",
			slog.String("campaign_id", campaignID), slog.String("package", pkg.Slug),
			slog.String("campaign_version", st.Version), slog.String("installed", pkg.InstalledVersion))
		installed.FellBack, installed.Reason = true, "campaign version folder missing"
		return installed
	}
	return ServedVersion{Version: st.Version, Dir: s.pkgs.InstallDirForVersion(pkg.Type, pkg.Slug, st.Version)}
}

func (s *campaignUpdateService) OnInstall(ctx context.Context, pkg *Package, previousVersion, newVersion string) error {
	if previousVersion == "" || previousVersion == newVersion {
		return nil // first install, or a reinstall: nothing moved
	}
	b, ok := s.bindings[pkg.Type]
	if !ok {
		return nil
	}
	states, err := b.Campaigns(ctx, pkg)
	if err != nil {
		return fmt.Errorf("listing campaigns to hold: %w", err)
	}

	var failures []error
	for _, st := range states {
		if asksByDefault(b) && st.Mode == UpdateModeAutomatic {
			// Nothing was chosen for it: it asks like the rest, from the
			// version it was running. A previous version that is no longer on
			// disk was not being served, so there is nothing to hold it on.
			if !s.versionOnDisk(pkg, previousVersion) {
				slog.Warn("update modes: previous version is not on disk, leaving a campaign as it was",
					slog.String("campaign_id", st.CampaignID), slog.String("package", pkg.Slug))
				continue
			}
			if err := b.Apply(ctx, st.CampaignID, pkg, UpdateModeApproveFirst, previousVersion, ActorInfo{}); err != nil {
				failures = append(failures, fmt.Errorf("campaign %s: %w", st.CampaignID, err))
			}
			continue
		}
		if !st.Mode.KeepsVersion() || st.Version != "" {
			continue // follows the installed version, or already on its own
		}
		// Not frozen yet: freeze on what it was running.
		var ferr error
		if !s.versionOnDisk(pkg, previousVersion) {
			ferr = fmt.Errorf("previous version %s is not on disk", previousVersion)
		} else {
			ferr = b.Freeze(ctx, st.CampaignID, pkg, st.Mode, previousVersion)
		}
		if ferr == nil {
			continue
		}
		if st.Explicit {
			failures = append(failures, fmt.Errorf("campaign %s: %w", st.CampaignID, ferr))
			continue
		}
		// Derived from older stored data: the older best-effort rule applies.
		slog.Warn("update modes: could not freeze a campaign on install, leaving it as before",
			slog.String("campaign_id", st.CampaignID), slog.String("package", pkg.Slug), slog.Any("error", ferr))
	}
	if len(failures) > 0 {
		return fmt.Errorf("could not hold %d campaign(s) on %s, install refused so none moves silently: %w",
			len(failures), previousVersion, errors.Join(failures...))
	}
	return nil
}

func (s *campaignUpdateService) OnInstalled(ctx context.Context, pkg *Package, previousVersion, newVersion string) error {
	if previousVersion == "" || previousVersion == newVersion {
		return nil
	}
	_, set, err := s.reconcilePackage(ctx, pkg)
	for _, h := range set {
		s.log(ctx, EventCampaignUpdateHeld, ActorInfo{}, map[string]any{
			"campaign_id": h.campaignID, "package_id": pkg.ID, "package": pkg.Slug,
			"on": h.on, "held": h.held,
		})
	}
	return err
}

func (s *campaignUpdateService) KeptVersions(ctx context.Context) (map[string]map[string]bool, error) {
	return s.repo.ListKeptVersions(ctx)
}

// holdSet is one hold written by reconcilePackage.
type holdSet struct{ campaignID, on, held string }

func (s *campaignUpdateService) Reconcile(ctx context.Context) (ReconcileResult, error) {
	var res ReconcileResult
	pkgs, err := s.pkgs.ListPackages(ctx)
	if err != nil {
		return res, fmt.Errorf("listing packages: %w", err)
	}
	for i := range pkgs {
		pkg := &pkgs[i]
		if _, ok := s.bindings[pkg.Type]; !ok || pkg.InstalledVersion == "" {
			continue
		}
		s.adoptDefaults(ctx, pkg)
		r, _, err := s.reconcilePackage(ctx, pkg)
		res.HeldSet += r.HeldSet
		res.HeldCleared += r.HeldCleared
		if err != nil {
			return res, err
		}
	}
	n, err := s.repo.DeleteDefaultRows(ctx)
	if err != nil {
		return res, err
	}
	res.RowsRemoved = n
	return res, nil
}

// adoptDefaults moves every campaign of an ask-by-default package that has no
// choice of its own onto ask-first, frozen on the installed version, so a
// campaign created or restored since the last install is covered too. It is
// best effort: a campaign it cannot freeze is logged and retried next boot.
func (s *campaignUpdateService) adoptDefaults(ctx context.Context, pkg *Package) {
	b, ok := s.bindings[pkg.Type]
	if !ok || !asksByDefault(b) || !s.versionOnDisk(pkg, pkg.InstalledVersion) {
		return
	}
	states, err := b.Campaigns(ctx, pkg)
	if err != nil {
		slog.Warn("update modes: could not list campaigns to adopt", slog.String("package", pkg.Slug), slog.Any("error", err))
		return
	}
	for _, st := range states {
		if st.Mode != UpdateModeAutomatic {
			continue
		}
		if err := b.Apply(ctx, st.CampaignID, pkg, UpdateModeApproveFirst, pkg.InstalledVersion, ActorInfo{}); err != nil {
			slog.Warn("update modes: could not freeze a campaign on the installed version",
				slog.String("campaign_id", st.CampaignID), slog.String("package", pkg.Slug), slog.Any("error", err))
		}
	}
}

// reconcilePackage makes one package's holds agree with its installed
// version and reports the holds it set.
func (s *campaignUpdateService) reconcilePackage(ctx context.Context, pkg *Package) (ReconcileResult, []holdSet, error) {
	var res ReconcileResult
	var set []holdSet
	b, ok := s.bindings[pkg.Type]
	if !ok || pkg.InstalledVersion == "" {
		return res, nil, nil
	}
	states, err := b.Campaigns(ctx, pkg)
	if err != nil {
		return res, nil, fmt.Errorf("listing campaigns for %s: %w", pkg.Slug, err)
	}
	rows, err := s.repo.ListByPackage(ctx, pkg.ID)
	if err != nil {
		return res, nil, err
	}
	held := map[string]string{}
	for _, r := range rows {
		held[r.CampaignID] = r.HeldVersion
	}
	seen := map[string]bool{}
	for _, st := range states {
		seen[st.CampaignID] = true
		// The one correct value: the installed version, if this campaign is
		// asking first and is behind it; otherwise nothing.
		want := wantHeld(b, pkg, st.Mode, st.Version)
		if want == held[st.CampaignID] {
			continue
		}
		if err := s.repo.SetHeld(ctx, st.CampaignID, pkg.ID, want); err != nil {
			return res, set, err
		}
		if want == "" {
			res.HeldCleared++
		} else {
			res.HeldSet++
			set = append(set, holdSet{st.CampaignID, st.Version, want})
		}
	}
	for id, h := range held {
		if h != "" && !seen[id] {
			if err := s.repo.SetHeld(ctx, id, pkg.ID, ""); err != nil {
				return res, set, err
			}
			res.HeldCleared++
		}
	}
	return res, set, nil
}

// log writes an audit row; a missing or failing sink never blocks the action.
func (s *campaignUpdateService) log(ctx context.Context, event string, actor ActorInfo, details map[string]any) {
	if s.audit == nil {
		return
	}
	if err := s.audit.LogEvent(ctx, event, "", actor.UserID, actor.IP, actor.UserAgent, details); err != nil {
		slog.Warn("update modes: audit write failed", slog.String("event", event), slog.Any("error", err))
	}
}

// updateModeHook adapts OnInstall to the install pipeline for one package
// type.
type updateModeHook struct {
	svc CampaignUpdateService
	typ PackageType
}

// NewUpdateModeHook returns the PostInstallHook that applies update modes
// when a package of the given type installs. Register it AFTER any
// type-specific hook that also freezes campaigns (the Foundry auto-pin),
// since it reads the state that hook leaves behind.
func NewUpdateModeHook(svc CampaignUpdateService, typ PackageType) PostInstallHook {
	return &updateModeHook{svc: svc, typ: typ}
}

func (h *updateModeHook) PackageType() PackageType { return h.typ }

func (h *updateModeHook) AfterInstall(ctx context.Context, pkg *Package, version, previousVersion, _ string) error {
	return h.svc.OnInstall(ctx, pkg, previousVersion, version)
}

// AfterCommit hands approve_first campaigns the new version once the install
// is committed (see PostCommitHook).
func (h *updateModeHook) AfterCommit(ctx context.Context, pkg *Package, version, previousVersion string) error {
	return h.svc.OnInstalled(ctx, pkg, previousVersion, version)
}

// SetCampaignVersionsProvider wires the set of versions campaigns are on into
// clean-up (see prune.go). Clean-up refuses to run until it is wired.
func SetCampaignVersionsProvider(svc PackageService, fn func(ctx context.Context) (map[string]map[string]bool, error)) {
	if s, ok := svc.(*packageService); ok {
		s.campaignVersionsFn = fn
	}
}
