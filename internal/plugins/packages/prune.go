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
	"sort"
	"strconv"
	"strings"
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

// PruneStaleVersions scans each installed system package's version folders
// and (unless dryRun) deletes those that are safe to reclaim. Protected —
// never deleted — are: the top keepNewest versions by semver (always
// includes the newest), the DB-installed version, and any dir the loader
// is currently serving. keepNewest < 1 is treated as 1. Idempotent: after
// an execute, a re-run reclaims nothing.
func (s *packageService) PruneStaleVersions(ctx context.Context, keepNewest int, dryRun bool) (*PruneResult, error) {
	if s.loadedDirsFn == nil {
		return nil, fmt.Errorf("cannot prune: loaded-dirs provider not wired (fail closed)")
	}
	if keepNewest < 1 {
		keepNewest = 1
	}
	loaded := s.loadedDirsFn()

	pkgs, err := s.repo.ListPackages(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing packages: %w", err)
	}

	res := &PruneResult{DryRun: dryRun}
	for i := range pkgs {
		pkg := &pkgs[i]
		if pkg.Type == PackageTypeFoundryModule || pkg.InstalledVersion == "" {
			continue // foundry dirs are pin-served; uninstalled packages have nothing to keep safe
		}
		slugDir := filepath.Join(s.packagesDir(), "systems", pkg.Slug)
		entries, err := os.ReadDir(slugDir)
		if err != nil {
			continue // no dir / unreadable → nothing to reclaim
		}
		var vers []string
		for _, e := range entries {
			if e.IsDir() {
				vers = append(vers, e.Name())
			}
		}
		if len(vers) <= keepNewest {
			continue
		}
		sort.Slice(vers, func(a, b int) bool { return pruneVersionLess(vers[b], vers[a]) })

		protected := make(map[string]bool, keepNewest+2)
		for j := 0; j < keepNewest && j < len(vers); j++ {
			protected[vers[j]] = true
		}
		protected[pkg.InstalledVersion] = true

		for _, v := range vers {
			full := filepath.Join(slugDir, v)
			if protected[v] || loaded[full] {
				continue
			}
			sv := StaleVersion{Slug: pkg.Slug, Version: v, Path: full, Size: dirSize(full)}
			res.Reclaimable = append(res.Reclaimable, sv)
			if dryRun {
				continue
			}
			// Re-assert protection immediately before deletion (defense in
			// depth against a concurrent install changing the picture).
			if protected[v] || s.loadedDirsFn()[full] {
				continue
			}
			if err := os.RemoveAll(full); err != nil {
				slog.Warn("prune: failed to remove stale version dir",
					slog.String("dir", full), slog.Any("error", err))
				continue
			}
			slog.Info("prune: removed stale package version",
				slog.String("package", pkg.Slug), slog.String("version", v),
				slog.Int64("bytes", sv.Size))
			res.Removed = append(res.Removed, sv)
			res.BytesFreed += sv.Size
		}
	}

	if !dryRun && len(res.Removed) > 0 && s.onServeInvalidate != nil {
		s.onServeInvalidate()
	}
	return res, nil
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
