// page_model.go holds the view model and the pure helpers behind the admin
// Packages page. Everything here is plain data in, plain data out: the page
// state lives in the URL (?tab, ?f, ?q, ?pkg, ?ptab) so every view is a link,
// works without JavaScript, and can be tested without a request.

package packages

import (
	"encoding/json"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Top-level tabs of the Packages page.
const (
	PackagesTabInstalled = "installed"
	PackagesTabUpdates   = "updates"
	PackagesTabReview    = "review"
	PackagesTabSettings  = "settings"
)

// Tabs of the open package's side panel.
const (
	PanelTabOverview  = "overview"
	PanelTabVersions  = "versions"
	PanelTabCampaigns = "campaigns"
	PanelTabSettings  = "settings"
)

// Filters of the Installed list. The empty filter is "Active": everything
// that is not retired.
const (
	FilterActive   = ""
	FilterSystems  = "systems"
	FilterFoundry  = "foundry"
	FilterProblems = "problems"
	FilterRetired  = "retired"
)

// maxSearchLen bounds the search box so a hand-built URL cannot make the
// page echo an arbitrarily long string back.
const maxSearchLen = 100

// normalizePackagesTab whitelists ?tab=; anything unknown falls back to the
// Installed list so a hand-edited URL never renders a blank page.
func normalizePackagesTab(raw string) string {
	switch raw {
	case PackagesTabUpdates, PackagesTabReview, PackagesTabSettings:
		return raw
	default:
		return PackagesTabInstalled
	}
}

// normalizePanelTab whitelists ?ptab= the same way.
func normalizePanelTab(raw string) string {
	switch raw {
	case PanelTabVersions, PanelTabCampaigns, PanelTabSettings:
		return raw
	default:
		return PanelTabOverview
	}
}

// normalizeFilter whitelists ?f=; unknown values mean "Active".
func normalizeFilter(raw string) string {
	switch raw {
	case FilterSystems, FilterFoundry, FilterProblems, FilterRetired:
		return raw
	default:
		return FilterActive
	}
}

// packagesQuery is the whole page state, already whitelisted.
type packagesQuery struct {
	Tab      string
	Filter   string
	Search   string
	PkgID    string
	PanelTab string
}

// parsePackagesQuery builds the page state from raw query values. The package
// id is not validated here: it is only ever compared against loaded rows.
func parsePackagesQuery(tab, filter, search, pkg, ptab string) packagesQuery {
	search = strings.TrimSpace(search)
	if len(search) > maxSearchLen {
		search = search[:maxSearchLen]
	}
	return packagesQuery{
		Tab:      normalizePackagesTab(tab),
		Filter:   normalizeFilter(filter),
		Search:   search,
		PkgID:    strings.TrimSpace(pkg),
		PanelTab: normalizePanelTab(ptab),
	}
}

// pkgHref builds a page link, leaving out every value that is the default so
// the bare page stays /admin/packages.
func pkgHref(tab, filter, search, pkg, ptab string) string {
	v := url.Values{}
	if tab != "" && tab != PackagesTabInstalled {
		v.Set("tab", tab)
	}
	if filter != FilterActive {
		v.Set("f", filter)
	}
	if search != "" {
		v.Set("q", search)
	}
	if pkg != "" {
		v.Set("pkg", pkg)
		if ptab != "" && ptab != PanelTabOverview {
			v.Set("ptab", ptab)
		}
	}
	if len(v) == 0 {
		return "/admin/packages"
	}
	return "/admin/packages?" + v.Encode()
}

// tabHref links to a top-level tab with no package open.
func tabHref(tab string) string { return pkgHref(tab, FilterActive, "", "", "") }

// filterHref links to the Installed list with a filter, keeping the search.
func (q packagesQuery) filterHref(filter string) string {
	return pkgHref(PackagesTabInstalled, filter, q.Search, "", "")
}

// rowHref opens a package in the side panel, keeping the list's filter and search.
func (q packagesQuery) rowHref(id string) string {
	return pkgHref(PackagesTabInstalled, q.Filter, q.Search, id, PanelTabOverview)
}

// panelHref switches the open package's panel tab.
func (q packagesQuery) panelHref(ptab string) string {
	return pkgHref(PackagesTabInstalled, q.Filter, q.Search, q.PkgID, ptab)
}

// closeHref closes the side panel, keeping the list's filter and search.
func (q packagesQuery) closeHref() string {
	return pkgHref(PackagesTabInstalled, q.Filter, q.Search, "", "")
}

// rowStatus is the one state a package row shows in its pill.
type rowStatus string

const (
	statusUpToDate     rowStatus = "up_to_date"
	statusUpdateReady  rowStatus = "update_ready"
	statusProblem      rowStatus = "problem"
	statusPinned       rowStatus = "pinned"
	statusNotInstalled rowStatus = "not_installed"
	statusRetired      rowStatus = "retired"
	statusDeclined     rowStatus = "declined"
)

// newerVersion returns the version an admin could update to, or nil. A
// package that was never installed has no "newer": it is simply not installed.
// Versions compare numerically, so 0.9.0 -> 0.10.0 counts as newer.
func newerVersion(pkg Package, versions []PackageVersion) *PackageVersion {
	if pkg.InstalledVersion == "" {
		return nil
	}
	latest := latestStableVersion(versions)
	if latest == nil || latest.Version == pkg.InstalledVersion {
		return nil
	}
	if !pruneVersionLess(pkg.InstalledVersion, latest.Version) {
		return nil
	}
	return latest
}

// derivePackageStatus picks the pill. Order matters: a retired package's old
// error is moot, and a failure outranks a pending update because the admin
// must see it first. A pinned package never reports an update, matching the
// auto-update worker, which skips pins.
func derivePackageStatus(pkg Package, newer *PackageVersion) rowStatus {
	switch pkg.Status {
	case StatusRejected:
		return statusDeclined
	case StatusArchived, StatusDeprecated:
		return statusRetired
	}
	switch {
	case pkg.LastError != "":
		return statusProblem
	case pkg.InstalledVersion == "":
		return statusNotInstalled
	case pkg.PinnedVersion != "":
		return statusPinned
	case newer != nil:
		return statusUpdateReady
	default:
		return statusUpToDate
	}
}

// PackageRow is one package with everything the page shows about it, loaded
// once so the list, the strip and the panel never disagree.
type PackageRow struct {
	Package
	Status   rowStatus
	Newer    *PackageVersion // non-nil only when Status == statusUpdateReady
	Versions []PackageVersion
	Usage    []PackageUsage
	// UsageKnown is false when the usage lookup failed: the page then says
	// nothing about campaign counts rather than showing a wrong zero.
	UsageKnown bool

	// ActionsFragmentURL is the type-registered lazy-load slot URL, "" for none.
	ActionsFragmentURL string

	// Campaigns is each campaign's standing on the package, loaded for a type
	// whose owners are asked before a campaign moves (the Foundry module).
	// CampaignsKnown is false when that is not wired or the lookup failed, and
	// the page then shows the plain usage list instead.
	Campaigns      []CampaignPackageState
	CampaignsKnown bool

	// Versions the admin may move a campaign to: installed, not cleaned up.
	MovableVersions []string
}

// IsFoundryModule reports whether the row is the Foundry module type.
func (r PackageRow) IsFoundryModule() bool { return r.Type == PackageTypeFoundryModule }

// IsRetired reports whether the row belongs under the Retired filter.
func (r PackageRow) IsRetired() bool {
	return r.Status == statusRetired || r.Status == statusDeclined
}

// statusLabel is the pill text. Deprecated and archived differ in wording
// because only one of them still runs.
func statusLabel(r PackageRow) string {
	switch r.Status {
	case statusUpdateReady:
		if r.Newer != nil {
			return "Update ready · " + r.Newer.Version
		}
		return "Update ready"
	case statusProblem:
		return "Problem"
	case statusPinned:
		return "Pinned · " + r.PinnedVersion
	case statusNotInstalled:
		return "Not installed"
	case statusDeclined:
		return "Declined"
	case statusRetired:
		if r.Package.Status == StatusDeprecated {
			return "Deprecated"
		}
		return "Retired"
	default:
		return "Up to date"
	}
}

// statusClass is the badge style of the pill for a status; the badge-*
// classes are the app's own, so light and dark themes come for free.
func statusClass(s rowStatus) string {
	switch s {
	case statusUpdateReady:
		return "badge-amber"
	case statusProblem:
		return "badge-red"
	case statusPinned:
		return "badge-primary"
	case statusNotInstalled, statusRetired, statusDeclined:
		return "badge-gray"
	default:
		return "badge-green"
	}
}

// jsonAttr marshals v for an hx-vals / hx-headers attribute. Marshalling,
// rather than splicing strings into JSON, keeps a version tag or token with a
// quote in it from breaking the attribute.
func jsonAttr(v map[string]string) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// csrfHeaders is the hx-headers value that carries the CSRF token.
func csrfHeaders(token string) string {
	return jsonAttr(map[string]string{"X-CSRF-Token": token})
}

// versionVals is the hx-vals value that names a version.
func versionVals(version string) string {
	return jsonAttr(map[string]string{"version": version})
}

// firstFoundryRow returns the Foundry-module package the page can target with
// its hidden versions trigger, or nil when there is none.
func firstFoundryRow(rows []PackageRow) *PackageRow {
	for i := range rows {
		if rows[i].IsFoundryModule() {
			return &rows[i]
		}
	}
	return nil
}

// packagesReturnURL is where a write sends the admin back to: the page they
// were on, so an Install from the open panel does not close it. The address
// comes from the HX-Current-URL header, which the client controls, so only
// the page's own whitelisted parameters are carried over and the path is
// always /admin/packages; anything else falls back to the bare page.
func packagesReturnURL(current string) string {
	u, err := url.Parse(current)
	if err != nil || u.Path != "/admin/packages" {
		return "/admin/packages"
	}
	v := u.Query()
	q := parsePackagesQuery(v.Get("tab"), v.Get("f"), v.Get("q"), v.Get("pkg"), v.Get("ptab"))
	return pkgHref(q.Tab, q.Filter, q.Search, q.PkgID, q.PanelTab)
}

// updatePolicyLabel names an UpdatePolicy in plain words. An unknown value
// reads as Off, the safe reading: nothing updates by itself.
func updatePolicyLabel(p UpdatePolicy) string {
	switch p {
	case UpdateNightly:
		return "Once a day"
	case UpdateWeekly:
		return "Once a week"
	case UpdateOnRelease:
		return "As soon as it is released"
	default:
		return "Off"
	}
}

// updatePolicyHint is the one-line explanation under each policy choice.
func updatePolicyHint(p UpdatePolicy) string {
	switch p {
	case UpdateOff:
		return "Only when you press Check now."
	case UpdateOnRelease:
		return "Checked every hour."
	default:
		return ""
	}
}

// updatePolicies lists the choices in the order the settings tab shows them.
var updatePolicies = []UpdatePolicy{UpdateOff, UpdateNightly, UpdateWeekly, UpdateOnRelease}

// typeLabel is the plain name of a package type.
func typeLabel(t PackageType) string {
	if t == PackageTypeFoundryModule {
		return "Foundry module"
	}
	return "Game system"
}

// iconLetter is the single letter on a package's icon tile.
func iconLetter(name string) string {
	for _, r := range name {
		if r != ' ' {
			return strings.ToUpper(string(r))
		}
	}
	return "?"
}

// filterCounts are the numbers on the filter chips.
type filterCounts struct {
	Active   int
	Systems  int
	Foundry  int
	Problems int
	Retired  int
}

// countFilters tallies the chips over every manageable package. System and
// Foundry counts cover active packages only, so each chip counts what it
// would show.
func countFilters(rows []PackageRow) filterCounts {
	var c filterCounts
	for _, r := range rows {
		switch {
		case r.IsRetired():
			c.Retired++
		default:
			c.Active++
			if r.IsFoundryModule() {
				c.Foundry++
			} else {
				c.Systems++
			}
			if r.Status == statusProblem {
				c.Problems++
			}
		}
	}
	return c
}

// filterRows applies the chip and the search box. Search matches the name and
// the repository address, case-insensitively.
func filterRows(rows []PackageRow, filter, search string) []PackageRow {
	needle := strings.ToLower(strings.TrimSpace(search))
	out := make([]PackageRow, 0, len(rows))
	for _, r := range rows {
		var keep bool
		switch filter {
		case FilterRetired:
			keep = r.IsRetired()
		case FilterProblems:
			keep = !r.IsRetired() && r.Status == statusProblem
		case FilterSystems:
			keep = !r.IsRetired() && !r.IsFoundryModule()
		case FilterFoundry:
			keep = !r.IsRetired() && r.IsFoundryModule()
		default:
			keep = !r.IsRetired()
		}
		if !keep {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(r.Name+" "+r.RepoURL), needle) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// updateRows returns the rows that have a newer version ready, by name.
func updateRows(rows []PackageRow) []PackageRow {
	var out []PackageRow
	for _, r := range rows {
		if r.Status == statusUpdateReady {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// repoLabel shortens a repository URL to owner/repo for the list; anything
// that does not parse is shown as stored.
func repoLabel(repoURL string) string {
	if p := repoPath(repoURL); p != "" {
		return p
	}
	return repoURL
}

// agoLabel renders a past time as "12 minutes ago", or "never" for nil.
func agoLabel(now time.Time, t *time.Time) string {
	if t == nil || t.IsZero() {
		return "never"
	}
	d := now.Sub(*t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return plural(int(d.Minutes()), "minute") + " ago"
	case d < 24*time.Hour:
		return plural(int(d.Hours()), "hour") + " ago"
	case d < 60*24*time.Hour:
		return plural(int(d.Hours()/24), "day") + " ago"
	default:
		return t.Format("2 Jan 2006")
	}
}

// dateLabel renders a calendar date, or "unknown" for nil.
func dateLabel(t *time.Time) string {
	if t == nil || t.IsZero() {
		return "unknown"
	}
	return t.Format("2 Jan 2006")
}

// plural renders "1 minute" / "2 minutes".
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

// ownKeepNewest pre-fills the per-package number box: the stored value, or the
// site default when the package follows the site rule.
func ownKeepNewest(v *int) string {
	if v == nil {
		return strconv.Itoa(DefaultRetentionKeepNewest)
	}
	return strconv.Itoa(*v)
}

// campaignCount renders "3 campaigns" for the usage count of a row.
func campaignCount(n int) string { return plural(n, "campaign") }

// releaseNoteLines pulls the first n meaningful lines out of a release body.
// Only the changes themselves are kept: headings and GitHub's generated
// "Full Changelog" line are dropped, and the " by @author in <pull URL>"
// tail GitHub adds to each generated entry is cut, so a line reads as one
// short sentence. Bullet markers are stripped since the page draws its own.
func releaseNoteLines(notes string, n int) []string {
	var out []string
	for _, line := range strings.Split(notes, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimPrefix(line, "* ")
		line = strings.TrimSpace(generatedNoteTail.ReplaceAllString(line, ""))
		if line == "" || strings.Contains(line, "Full Changelog") {
			continue
		}
		out = append(out, line)
		if len(out) == n {
			break
		}
	}
	return out
}

// generatedNoteTail matches the credit GitHub appends to each entry of
// generated release notes.
var generatedNoteTail = regexp.MustCompile(`\s+by @\S+ in https?://\S+$`)

// ReleaseNoteLines is releaseNoteLines for another plugin's page, so a
// campaign owner reads release notes the same way the admin does.
func ReleaseNoteLines(notes string, n int) []string { return releaseNoteLines(notes, n) }

// latestCheck is the most recent update check across all rows, or nil if none
// has ever run.
func latestCheck(rows []PackageRow) *time.Time {
	var best *time.Time
	for i := range rows {
		t := rows[i].LastCheckedAt
		if t != nil && !t.IsZero() && (best == nil || t.After(*best)) {
			best = t
		}
	}
	return best
}

// installedVersionSize is the download size recorded for the running version,
// or 0 when it is not known (a version the check never recorded a size for).
func installedVersionSize(r PackageRow) int64 {
	for _, v := range r.Versions {
		if v.Version == r.InstalledVersion {
			return v.FileSize
		}
	}
	return 0
}

// versionIsNewerThanInstalled tells the Versions tab whether a row is an
// upgrade (Install) or a step back (Switch to this).
func versionIsNewerThanInstalled(installed, candidate string) bool {
	return installed != "" && pruneVersionLess(installed, candidate)
}

// PackagesPageData is everything PackagesPage renders.
type PackagesPageData struct {
	Query     packagesQuery
	CSRFToken string
	Now       time.Time

	// Rows is every manageable package (pending submissions are in Pending);
	// Visible is Rows after the chip and the search box.
	Rows    []PackageRow
	Visible []PackageRow
	Counts  filterCounts
	// Selected is the open package, nil when none is open or the id is unknown.
	Selected *PackageRow

	Updates      []PackageRow
	Pending      []Package
	PendingCount int
	// Settings is loaded for the Settings tab only.
	Settings *PackageSecuritySettings
	// Retention is the site-wide old-version rule, loaded for every tab.
	Retention RetentionSettings
	// CanRemind is true when an owner-notification path is wired, so the
	// Remind owner action is offered.
	CanRemind bool
}

// chipState is what the Campaigns tab says about one campaign's update.
type chipState struct {
	Label string
	Class string
	// Hint is the "since ..." text after the chip; empty when there is none.
	Hint string
	// Asked is true while an owner has a version waiting that the admin has
	// not held back, so Remind owner makes sense.
	Asked bool
}

// campaignChip picks the one chip a campaign row shows. A hold outranks
// everything, since it is what the owner cannot override.
func campaignChip(st CampaignPackageState, now time.Time) chipState {
	switch {
	case st.AdminHold:
		return chipState{Label: "Held by you", Class: "badge-primary", Hint: "since " + dateLabel(st.AdminHoldAt)}
	case st.HeldVersion != "" && st.DismissedVersion == st.HeldVersion:
		return chipState{Label: "Owner chose Later", Class: "badge-amber", Asked: true,
			Hint: st.HeldVersion + " offered " + agoLabel(now, st.HeldAt)}
	case st.HeldVersion != "":
		return chipState{Label: "Owner asked", Class: "badge-amber", Asked: true, Hint: "since " + dateLabel(st.HeldAt)}
	default:
		return chipState{Label: "Up to date", Class: "badge-green"}
	}
}

// askedAndHeld counts the owners who will be asked about the next version and
// the campaigns the admin is holding back (which are not asked).
func askedAndHeld(camps []CampaignPackageState) (asked, held int) {
	for _, c := range camps {
		if c.AdminHold {
			held++
		} else {
			asked++
		}
	}
	return asked, held
}

// installConfirmText is the confirm shown before an install. For a type whose
// owners are asked, it says nothing moves until they press Update.
func installConfirmText(t PackageType, version string) string {
	msg := "Install version " + version + "?"
	if t == PackageTypeFoundryModule {
		msg += " Each campaign keeps its current version until its owner presses Update."
	}
	return msg
}
