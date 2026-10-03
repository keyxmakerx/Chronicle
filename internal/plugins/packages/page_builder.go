// page_builder.go assembles PackagesPageData from the service. It lives
// beside the model, not in the handler, so the handler stays bind/call/render
// and the assembly can be tested with a fake source.

package packages

import (
	"context"
	"log/slog"
	"time"
)

// pageSource is the slice of PackageService the Packages page reads.
type pageSource interface {
	ListPackages(ctx context.Context) ([]Package, error)
	ListVersions(ctx context.Context, packageID string) ([]PackageVersion, error)
	GetUsage(ctx context.Context, packageID string) ([]PackageUsage, error)
	ListPendingSubmissions(ctx context.Context) ([]Package, error)
	GetSecuritySettings(ctx context.Context) (*PackageSecuritySettings, error)
	GetRetentionSettings(ctx context.Context) (*RetentionSettings, error)
}

// campaignStateSource is the part of the update service the page reads for a
// package whose owners are asked before a campaign moves.
type campaignStateSource interface {
	CampaignsOnPackage(ctx context.Context, packageID string) ([]CampaignPackageState, error)
	InstalledVersions(ctx context.Context, packageID string) ([]string, error)
}

// buildPackagesPage loads what the requested tab needs. The summary strip and
// tab badges need every package's newest version on every tab, so versions and
// usage are loaded for all rows; a failed lookup for one package degrades that
// row (no update, no campaign count) instead of failing the page, because the
// admin must still be able to reach the Remove and Settings controls.
func buildPackagesPage(ctx context.Context, src pageSource, updates campaignStateSource, q packagesQuery, csrfToken string, now time.Time) (*PackagesPageData, error) {
	pkgs, err := src.ListPackages(ctx)
	if err != nil {
		return nil, err
	}
	pending, err := src.ListPendingSubmissions(ctx)
	if err != nil {
		return nil, err
	}

	data := &PackagesPageData{
		Query:        q,
		CSRFToken:    csrfToken,
		Now:          now,
		Pending:      pending,
		PendingCount: len(pending),
	}

	// The panel's Settings tab and the Versions hint name the site rule, so it
	// is needed on every tab; a failed read degrades to the manual default
	// (which deletes nothing) rather than failing the page.
	data.Retention = DefaultRetentionSettings()
	if r, err := src.GetRetentionSettings(ctx); err != nil {
		slog.Warn("packages page: reading old-version rule failed", slog.Any("error", err))
	} else if r != nil {
		data.Retention = *r
	}

	for _, p := range pkgs {
		// Pending submissions are reviewed, not managed: they live in the
		// Review tab until approved.
		if p.Status == StatusPending {
			continue
		}
		row := PackageRow{Package: p}

		versions, err := src.ListVersions(ctx, p.ID)
		if err != nil {
			slog.Warn("packages page: listing versions failed",
				slog.String("package", p.Slug), slog.Any("error", err))
		}
		row.Versions = versions
		row.Newer = newerVersion(p, versions)
		row.Status = derivePackageStatus(p, row.Newer)
		if row.Status != statusUpdateReady {
			row.Newer = nil
		}

		usage, err := src.GetUsage(ctx, p.ID)
		if err != nil {
			slog.Warn("packages page: reading usage failed",
				slog.String("package", p.Slug), slog.Any("error", err))
		} else {
			row.Usage = usage
			row.UsageKnown = true
		}

		if updates != nil && p.Type == PackageTypeFoundryModule && p.InstalledVersion != "" {
			loadCampaignStates(ctx, updates, &row)
		}

		data.Rows = append(data.Rows, row)
	}

	data.Counts = countFilters(data.Rows)
	data.Visible = filterRows(data.Rows, q.Filter, q.Search)
	data.Updates = updateRows(data.Rows)

	if q.PkgID != "" {
		for i := range data.Rows {
			if data.Rows[i].ID == q.PkgID {
				data.Selected = &data.Rows[i]
				break
			}
		}
	}

	if q.Tab == PackagesTabSettings {
		settings, err := src.GetSecuritySettings(ctx)
		if err != nil {
			return nil, err
		}
		data.Settings = settings
	}
	return data, nil
}

// loadCampaignStates fills a row's per-campaign standing. A failed lookup
// leaves the row on the plain usage list: it must not fail the page, because
// the admin still has to reach the other controls.
func loadCampaignStates(ctx context.Context, updates campaignStateSource, row *PackageRow) {
	camps, err := updates.CampaignsOnPackage(ctx, row.ID)
	if err != nil {
		slog.Warn("packages page: reading campaign update state failed",
			slog.String("package", row.Slug), slog.Any("error", err))
		return
	}
	versions, err := updates.InstalledVersions(ctx, row.ID)
	if err != nil {
		slog.Warn("packages page: listing installed versions failed",
			slog.String("package", row.Slug), slog.Any("error", err))
		return
	}
	row.Campaigns, row.MovableVersions, row.CampaignsKnown = camps, versions, true
}
