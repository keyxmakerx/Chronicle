// prune.go — stale-version cleanup for installed SYSTEM packages.
//
// Every InstallVersion leaves the previous version's dir on disk, so
// media/packages/systems/<slug>/ accumulates one folder per release ever
// installed. This file implements the safe reclaim: a dry-run scan (the
// admin wizard's preview) and an execute path that deletes only folders
// provably not in use.
//
// Scope: SYSTEM packages ONLY. Foundry-module version dirs are served to
// campaigns via historical pins (resolveCampaignManifest hard-errors with
// ErrPinnedVersionNotInstalled if a pinned dir is missing), so they are
// never touched here.
package packages

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// StaleVersion is one on-disk version folder the cleanup may reclaim.
type StaleVersion struct {
	Slug    string
	Version string
	Path    string
	Size    int64 // bytes
}

// PruneResult summarizes a prune run (dry-run or executed).
type PruneResult struct {
	Reclaimable []StaleVersion // what would be / was deleted
	Removed     []StaleVersion // actually deleted (empty on dry-run)
	BytesFreed  int64
	DryRun      bool
}

// SetLoadedDirsProvider wires the systems loader's live served-dir set into
// the prune safety check (dependency inversion — packages must not import
// systems). Pruning FAILS CLOSED when unset: with no signal about what the
// loader is serving, nothing is deleted.
func SetLoadedDirsProvider(svc PackageService, fn func() map[string]bool) {
	if s, ok := svc.(*packageService); ok {
		s.loadedDirsFn = fn
	}
}

// PruneStaleVersions is the admin's manual clean-up: keep the newest
// keepNewest folders of every system package and delete the rest that are
// safe. keepNewest < 1 is treated as 1. dryRun previews without deleting.
func (s *packageService) PruneStaleVersions(ctx context.Context, keepNewest int, dryRun bool) (*PruneResult, error) {
	if keepNewest < 1 {
		keepNewest = 1
	}
	rule := retentionRule{Mode: RetentionKeepNewest, KeepNewest: keepNewest}
	return s.prune(ctx, func(*Package) retentionRule { return rule }, dryRun)
}

// prune is the one deletion path for both the manual card and the automatic
// rule, so there is exactly one protected set. Never deleted, whatever the
// rule: the DB-installed version, a pinned version, any dir the loader is
// serving, every version a campaign is on or holds for approval, and every
// foundry-module folder (pin-served to campaigns).
// ruleFor returns the rule per package; a manual rule skips the package.
// Fails closed when the loader signal or the campaign versions provider is
// unwired, or when reading campaign versions errors: a campaign's version
// missing from the answer would be deleted from under it.
func (s *packageService) prune(ctx context.Context, ruleFor func(*Package) retentionRule, dryRun bool) (*PruneResult, error) {
	if s.loadedDirsFn == nil {
		return nil, fmt.Errorf("cannot prune: loaded-dirs provider not wired (fail closed)")
	}
	loaded := s.loadedDirsFn()

	if s.campaignVersionsFn == nil {
		return nil, fmt.Errorf("cannot prune: campaign versions provider not wired (fail closed)")
	}
	kept, err := s.campaignVersionsFn(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot prune: reading campaign versions failed, keeping everything: %w", err)
	}

	pkgs, err := s.repo.ListPackages(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing packages: %w", err)
	}

	res := &PruneResult{DryRun: dryRun}
	now := time.Now()
	for i := range pkgs {
		pkg := &pkgs[i]
		if pkg.Type == PackageTypeFoundryModule || pkg.InstalledVersion == "" {
			continue // foundry dirs are pin-served; uninstalled packages have nothing to keep safe
		}
		rule := ruleFor(pkg)
		if rule.Mode == RetentionManual {
			continue
		}
		s.pruneOnePackage(ctx, pkg, rule, now, dryRun, loaded, kept[pkg.Slug], res)
	}

	if !dryRun && len(res.Removed) > 0 && s.onServeInvalidate != nil {
		s.onServeInvalidate()
	}
	return res, nil
}

// pruneOnePackage scans and (unless dryRun) reclaims one system package's
// stale version folders, appending to res. It holds that package's install
// mutex for the whole scan+delete so a concurrent InstallVersion (rollback,
// auto-update worker, admin double-click) can never RemoveAll a dir the
// install is mid-extract into — and a version still being installed is
// either fully committed (then protected as InstalledVersion) or not yet on
// disk before we ReadDir. Different packages don't contend; the lock is
// released (deferred) before the caller moves to the next package.
func (s *packageService) pruneOnePackage(ctx context.Context, pkg *Package, rule retentionRule, now time.Time, dryRun bool, loaded map[string]bool, campaignKept map[string]bool, res *PruneResult) {
	mu := s.lockForPackage(pkg.ID)
	mu.Lock()
	defer mu.Unlock()

	// Re-read under the lock: the row the caller listed may predate an
	// install or pin that finished while we waited for the mutex.
	if fresh, err := s.repo.GetPackage(ctx, pkg.ID); err == nil && fresh != nil {
		pkg = fresh
	}

	slugDir := filepath.Join(s.packagesDir(), "systems", pkg.Slug)
	entries, err := os.ReadDir(slugDir)
	if err != nil {
		return // no dir / unreadable → nothing to reclaim
	}

	published := map[string]time.Time{}
	if vs, err := s.repo.ListVersions(ctx, pkg.ID); err == nil {
		for _, v := range vs {
			published[v.Version] = v.PublishedAt
		}
	}

	var folders []versionFolder
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		f := versionFolder{
			Name:      e.Name(),
			Published: published[e.Name()],
			Served:    loaded[filepath.Join(slugDir, e.Name())],
		}
		if info, err := e.Info(); err == nil {
			f.Modified = info.ModTime()
		}
		folders = append(folders, f)
	}

	for _, v := range selectRemovable(folders, pkg.InstalledVersion, pkg.PinnedVersion, rule, now) {
		if campaignKept[v] {
			continue // a campaign is on this version or holds it for approval
		}
		full := filepath.Join(slugDir, v)
		sv := StaleVersion{Slug: pkg.Slug, Version: v, Path: full, Size: dirSize(full)}
		res.Reclaimable = append(res.Reclaimable, sv)
		if dryRun {
			continue
		}
		// Re-assert protection immediately before deletion (defense in
		// depth against a concurrent install changing the picture).
		// Re-read the row so a pin made after the scan is honoured; if it
		// cannot be read, fail closed and keep the folder.
		cur, err := s.repo.GetPackage(ctx, pkg.ID)
		if err != nil || cur == nil {
			continue
		}
		if v == cur.InstalledVersion || (cur.PinnedVersion != "" && v == cur.PinnedVersion) || s.loadedDirsFn()[full] {
			continue
		}
		if err := os.RemoveAll(full); err != nil {
			slog.Warn("prune: failed to remove stale version dir",
				slog.String("dir", full), slog.Any("error", err))
			continue
		}
		slog.Info("prune: removed stale package version",
			slog.String("package", pkg.Slug), slog.String("version", v),
			slog.String("rule", string(rule.Mode)), slog.Int64("bytes", sv.Size))
		res.Removed = append(res.Removed, sv)
		res.BytesFreed += sv.Size
	}
}

// prettyBytes renders a byte count for the cleanup card ("312.4 MB").
func prettyBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// pruneTotal sums the reclaimable bytes for the wizard's headline/confirm.
func pruneTotal(res *PruneResult) int64 {
	var t int64
	for _, s := range res.Reclaimable {
		t += s.Size
	}
	return t
}

// pruneVersionLess is a numeric-aware semver comparator (local copy of the
// systems loader's versionLess — packages must not import systems; keep in
// sync with internal/systems/registry.go). Non-numeric segments compare
// lexically; missing segments count as 0.
func pruneVersionLess(a, b string) bool {
	as := strings.Split(strings.TrimPrefix(a, "v"), ".")
	bs := strings.Split(strings.TrimPrefix(b, "v"), ".")
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		var av, bv string
		if i < len(as) {
			av = as[i]
		}
		if i < len(bs) {
			bv = bs[i]
		}
		ai, aerr := strconv.Atoi(av)
		bi, berr := strconv.Atoi(bv)
		switch {
		case aerr == nil && berr == nil:
			if ai != bi {
				return ai < bi
			}
		default:
			if av != bv {
				return av < bv
			}
		}
	}
	return false
}
