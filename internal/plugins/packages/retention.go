// retention.go — the automatic old-version rule: its settings, validation,
// the pure "which folders go" selection, and the service entry points.
//
// The default is manual: nothing is removed unless an admin clicks Clean up,
// so an install that never opens the new settings behaves exactly as before.
// Automatic runs and the manual card share one deletion path (prune.go) so
// there is a single protected set; this file only decides WHICH folders a
// rule nominates, never whether a protected one may go.
package packages

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// RetentionMode is the site-wide old-version rule.
type RetentionMode string

const (
	// RetentionManual removes nothing automatically (the default).
	RetentionManual RetentionMode = "manual"
	// RetentionKeepNewest keeps the newest N versions of each package.
	RetentionKeepNewest RetentionMode = "keep_newest"
	// RetentionUnusedDays removes versions untouched for N days.
	RetentionUnusedDays RetentionMode = "unused_days"
)

const (
	// DefaultRetentionKeepNewest and DefaultRetentionUnusedDays pre-fill the
	// number boxes; they take effect only once the admin picks that mode.
	DefaultRetentionKeepNewest = 2
	DefaultRetentionUnusedDays = 30
	// maxRetentionN stops a typo (or a hostile form) from storing a value
	// that can never match anything and so silently disables the rule.
	maxRetentionN = 1000

	settingRetentionMode       = "packages.retention_mode"
	settingRetentionKeepNewest = "packages.retention_keep_newest"
	settingRetentionUnusedDays = "packages.retention_unused_days"
)

// RetentionSettings is the site-wide rule. Both numbers are always kept so
// switching mode back and forth does not lose what the admin typed.
type RetentionSettings struct {
	Mode       RetentionMode
	KeepNewest int
	UnusedDays int
}

// DefaultRetentionSettings is the rule for installs that never set one.
func DefaultRetentionSettings() RetentionSettings {
	return RetentionSettings{
		Mode:       RetentionManual,
		KeepNewest: DefaultRetentionKeepNewest,
		UnusedDays: DefaultRetentionUnusedDays,
	}
}

// ValidateRetentionCount rejects N outside 1..maxRetentionN. N < 1 would
// mean "keep nothing", which the safety rules forbid anyway.
func ValidateRetentionCount(n int, what string) error {
	if n < 1 || n > maxRetentionN {
		return apperror.NewValidation(fmt.Sprintf("%s must be a whole number from 1 to %d", what, maxRetentionN))
	}
	return nil
}

// Validate checks the mode and the number the mode actually uses; the other
// number is not required to be sensible while its mode is not selected.
func (r RetentionSettings) Validate() error {
	switch r.Mode {
	case RetentionManual:
		return nil
	case RetentionKeepNewest:
		return ValidateRetentionCount(r.KeepNewest, "Versions to keep")
	case RetentionUnusedDays:
		return ValidateRetentionCount(r.UnusedDays, "Days unused")
	default:
		return apperror.NewValidation("unknown clean-up rule")
	}
}

// ParseRetentionSettings builds the rule from submitted form text, starting
// from current so a blank or garbled number in a mode that is NOT selected
// keeps its stored value instead of rejecting the whole save.
func ParseRetentionSettings(mode, keepNewest, unusedDays string, current RetentionSettings) (RetentionSettings, error) {
	out := current
	out.Mode = RetentionMode(strings.TrimSpace(mode))
	if n, err := strconv.Atoi(strings.TrimSpace(keepNewest)); err == nil {
		out.KeepNewest = n
	} else if out.Mode == RetentionKeepNewest {
		return current, apperror.NewValidation("Versions to keep must be a whole number")
	}
	if n, err := strconv.Atoi(strings.TrimSpace(unusedDays)); err == nil {
		out.UnusedDays = n
	} else if out.Mode == RetentionUnusedDays {
		return current, apperror.NewValidation("Days unused must be a whole number")
	}
	if err := out.Validate(); err != nil {
		return current, err
	}
	// An out-of-range number in the mode that is not selected is dropped
	// rather than stored, so a later switch cannot activate a bad value.
	if out.Mode != RetentionKeepNewest && ValidateRetentionCount(out.KeepNewest, "") != nil {
		out.KeepNewest = current.KeepNewest
	}
	if out.Mode != RetentionUnusedDays && ValidateRetentionCount(out.UnusedDays, "") != nil {
		out.UnusedDays = current.UnusedDays
	}
	return out, nil
}

// retentionRule is the resolved rule for one package.
type retentionRule struct {
	Mode       RetentionMode
	KeepNewest int
	UnusedDays int
}

// effectiveRetention resolves a package's rule: a per-package override wins
// over the site rule (even when the site rule is manual), and foundry
// modules are always manual because their folders are served to campaigns
// by historical pin.
func effectiveRetention(site RetentionSettings, pkg *Package) retentionRule {
	if pkg.Type == PackageTypeFoundryModule {
		return retentionRule{Mode: RetentionManual}
	}
	if pkg.RetentionKeepNewest != nil {
		return retentionRule{Mode: RetentionKeepNewest, KeepNewest: *pkg.RetentionKeepNewest}
	}
	return retentionRule(site)
}

// RetentionSummary is the plain-words sentence the Versions tab shows.
func RetentionSummary(site RetentionSettings, pkg *Package) string {
	if pkg.Type == PackageTypeFoundryModule {
		return "Old versions of this package are never removed automatically: Foundry worlds may still load them."
	}
	if pkg.RetentionKeepNewest != nil {
		return fmt.Sprintf("Old versions follow this package's own rule: keep the newest %d.", *pkg.RetentionKeepNewest)
	}
	switch site.Mode {
	case RetentionKeepNewest:
		return fmt.Sprintf("Old versions follow the site rule: keep the newest %d.", site.KeepNewest)
	case RetentionUnusedDays:
		return fmt.Sprintf("Old versions follow the site rule: remove versions unused for %d days.", site.UnusedDays)
	default:
		return "Old versions are only removed when you click Clean up on the Settings tab."
	}
}

// versionFolder is what the selector knows about one on-disk version folder.
type versionFolder struct {
	Name string
	// Published is the release date from the catalog; zero when the folder
	// has no catalog row (e.g. a manual upload).
	Published time.Time
	// Modified is when the folder was last installed or last stopped being
	// the installed version (see touchPreviousInstall).
	Modified time.Time
	// Served marks a folder the systems loader is serving right now.
	Served bool
}

// newerFolder orders folders newest first. Catalog dates decide when both
// folders have one; a folder without a date sorts as NEWER than any dated one
// so that uncertainty keeps a folder rather than deleting it.
func newerFolder(a, b versionFolder) bool {
	switch {
	case a.Published.IsZero() && !b.Published.IsZero():
		return true
	case !a.Published.IsZero() && b.Published.IsZero():
		return false
	case !a.Published.IsZero() && !a.Published.Equal(b.Published):
		return a.Published.After(b.Published)
	}
	return pruneVersionLess(b.Name, a.Name)
}

// selectRemovable returns the folder names the rule nominates for deletion.
// It is the single place the never-remove set lives: the installed version,
// the pinned version and anything the loader serves are skipped under every
// rule, manual clean-up included. A manual rule nominates nothing.
func selectRemovable(folders []versionFolder, installed, pinned string, rule retentionRule, now time.Time) []string {
	if rule.Mode != RetentionKeepNewest && rule.Mode != RetentionUnusedDays {
		return nil
	}
	ordered := append([]versionFolder(nil), folders...)
	sort.SliceStable(ordered, func(i, j int) bool { return newerFolder(ordered[i], ordered[j]) })

	var out []string
	for rank, f := range ordered {
		if f.Name == installed || (pinned != "" && f.Name == pinned) || f.Served {
			continue
		}
		switch rule.Mode {
		case RetentionKeepNewest:
			if rank < rule.KeepNewest {
				continue
			}
		case RetentionUnusedDays:
			if f.Modified.IsZero() || now.Sub(f.Modified) < time.Duration(rule.UnusedDays)*24*time.Hour {
				continue
			}
		}
		out = append(out, f.Name)
	}
	return out
}

// GetRetentionSettings returns the site rule, defaulting to manual when
// nothing is stored or a stored value is unusable.
func (s *packageService) GetRetentionSettings(ctx context.Context) (*RetentionSettings, error) {
	out := DefaultRetentionSettings()
	if s.settings == nil {
		return &out, nil
	}
	if v, err := s.settings.Get(ctx, settingRetentionMode); err == nil {
		switch m := RetentionMode(v); m {
		case RetentionKeepNewest, RetentionUnusedDays:
			out.Mode = m
		}
	}
	// A stored number that is unusable leaves the default in place for the
	// form, but a rule whose own number is unusable must not run at all.
	keepOK, daysOK := true, true
	if v, err := s.settings.Get(ctx, settingRetentionKeepNewest); err == nil {
		n, perr := strconv.Atoi(v)
		keepOK = perr == nil && ValidateRetentionCount(n, "") == nil
		if keepOK {
			out.KeepNewest = n
		}
	}
	if v, err := s.settings.Get(ctx, settingRetentionUnusedDays); err == nil {
		n, perr := strconv.Atoi(v)
		daysOK = perr == nil && ValidateRetentionCount(n, "") == nil
		if daysOK {
			out.UnusedDays = n
		}
	}
	if (out.Mode == RetentionKeepNewest && !keepOK) || (out.Mode == RetentionUnusedDays && !daysOK) {
		out.Mode = RetentionManual
	}
	return &out, nil
}

// SaveRetentionSettings validates and stores the site rule.
func (s *packageService) SaveRetentionSettings(ctx context.Context, r RetentionSettings) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if s.settingsWriter == nil {
		return fmt.Errorf("settings writer not configured")
	}
	pairs := map[string]string{
		settingRetentionMode:       string(r.Mode),
		settingRetentionKeepNewest: strconv.Itoa(r.KeepNewest),
		settingRetentionUnusedDays: strconv.Itoa(r.UnusedDays),
	}
	for k, v := range pairs {
		if err := s.settingsWriter.Set(ctx, k, v); err != nil {
			return fmt.Errorf("saving %s: %w", k, err)
		}
	}
	return nil
}

// SetPackageRetention sets (keepNewest != nil) or clears (nil) a package's
// own rule. Foundry modules refuse: their folders are never removed, so an
// override would promise something that cannot happen.
func (s *packageService) SetPackageRetention(ctx context.Context, packageID string, keepNewest *int) error {
	if keepNewest != nil {
		if err := ValidateRetentionCount(*keepNewest, "Versions to keep"); err != nil {
			return err
		}
	}
	pkg, err := s.repo.GetPackage(ctx, packageID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if pkg == nil {
		return apperror.NewNotFound("package not found")
	}
	if pkg.Type == PackageTypeFoundryModule {
		return apperror.NewBadRequest("Foundry module versions are never removed automatically")
	}
	if err := s.repo.SetRetention(ctx, packageID, keepNewest); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

// RunRetention applies the automatic rule to every package once. Idempotent:
// a second run finds nothing left to remove. Returns an error only when the
// run could not start (unwired loader signal, unreadable catalog); a single
// folder failing to delete is logged and skipped by the shared prune path.
func (s *packageService) RunRetention(ctx context.Context) (*PruneResult, error) {
	site, err := s.GetRetentionSettings(ctx)
	if err != nil {
		return nil, err
	}
	res, err := s.prune(ctx, func(p *Package) retentionRule { return effectiveRetention(*site, p) }, false)
	if err != nil {
		return nil, err
	}
	if len(res.Removed) > 0 {
		slog.Info("automatic old-version clean-up finished",
			slog.Int("removed", len(res.Removed)),
			slog.Int64("bytes_freed", res.BytesFreed))
	}
	return res, nil
}
