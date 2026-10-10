package admin

import (
	"sort"
	"strings"
	"time"
)

// Activity areas mirror the admin menu so a filter reads like the sidebar.
const (
	AreaPeople    = "people"
	AreaCampaigns = "campaigns"
	AreaPackages  = "game-systems"
	AreaFeatures  = "features"
	AreaSecurity  = "security"
	AreaSite      = "site"
	AreaOther     = "other"
)

// activityArea is one filter choice: a stable query value, the label shown in
// the menu and the row icon (Font Awesome class without the style prefix).
type activityArea struct {
	Key   string
	Label string
	Icon  string
}

// activityAreas lists the areas in menu order.
var activityAreas = []activityArea{
	{AreaPeople, "People", "fa-users"},
	{AreaCampaigns, "Campaigns", "fa-book-open"},
	{AreaPackages, "Packages", "fa-box"},
	{AreaFeatures, "Features & extensions", "fa-plug"},
	{AreaSecurity, "Security", "fa-shield-halved"},
	{AreaSite, "Site", "fa-hard-drive"},
	{AreaOther, "Other", "fa-circle-info"},
}

// activityResourceArea maps the part of an action before the dot to its area.
// Keyed on the resource so a new verb on a known resource lands in the right
// area without touching this table.
var activityResourceArea = map[string]string{
	"user":         AreaPeople,
	"session":      AreaPeople,
	"campaign":     AreaCampaigns,
	"package":      AreaPackages,
	"foundry":      AreaPackages,
	"addon":        AreaFeatures,
	"extension":    AreaFeatures,
	"apialert":     AreaSecurity,
	"apikey":       AreaSecurity,
	"cors":         AreaSecurity,
	"ipblock":      AreaSecurity,
	"registration": AreaSecurity,
	"media":        AreaSite,
	"hygiene":      AreaSite,
	"trash":        AreaSite,
	"migrations":   AreaSite,
	"backup":       AreaSite,
	"restore":      AreaSite,
	"smtp":         AreaSite,
	"sitelook":     AreaSite,
	"storage":      AreaSite,
}

// activityResource returns the resource part of "resource.verb".
func activityResource(action string) string {
	if i := strings.IndexByte(action, '.'); i >= 0 {
		return action[:i]
	}
	return action
}

// ActivityAreaOf names the area an action belongs to; unknown actions are
// Other so a recorder added later still shows up under "All areas".
func ActivityAreaOf(action string) string {
	if a, ok := activityResourceArea[activityResource(action)]; ok {
		return a
	}
	return AreaOther
}

// activityResourcesIn lists the resources mapped to area, sorted so the SQL
// and its arguments are stable.
func activityResourcesIn(area string) []string {
	var out []string
	for res, a := range activityResourceArea {
		if a == area {
			out = append(out, res)
		}
	}
	sort.Strings(out)
	return out
}

// allActivityResources lists every resource the table knows, sorted.
func allActivityResources() []string {
	out := make([]string, 0, len(activityResourceArea))
	for res := range activityResourceArea {
		out = append(out, res)
	}
	sort.Strings(out)
	return out
}

// validActivityArea reports whether key is a known filter value.
func validActivityArea(key string) bool {
	for _, a := range activityAreas {
		if a.Key == key {
			return true
		}
	}
	return false
}

// Area is this entry's area key.
func (e ActivityEntry) Area() string { return ActivityAreaOf(e.Action) }

// AreaMeta is this entry's area label and icon.
func (e ActivityEntry) AreaMeta() activityArea {
	k := e.Area()
	for _, a := range activityAreas {
		if a.Key == k {
			return a
		}
	}
	return activityAreas[len(activityAreas)-1]
}

// Time windows for the "when" filter.
const (
	WhenToday = "today"
	When7Days = "7d"
	When30    = "30d"
	WhenAll   = "all"
)

// activityWhenOption is one "when" choice.
type activityWhenOption struct{ Key, Label string }

var activityWhens = []activityWhenOption{
	{WhenToday, "Today"}, {When7Days, "Last 7 days"}, {When30, "Last 30 days"}, {WhenAll, "All time"},
}

// ActivityFilter narrows the change log. The zero value means everything.
type ActivityFilter struct {
	ActorID string
	Area    string
	// Since is an inclusive lower bound on created_at; zero means no bound.
	Since time.Time
}

// ActivityActor is one person who has made a logged change, for the "who" menu.
type ActivityActor struct {
	ID   string
	Name string
}

// ActivityQuery is the raw, untrusted filter from the query string.
type ActivityQuery struct {
	Actor, Area, When string
}

// Normalize drops unknown values to their defaults so a hand-edited URL never
// reaches the repository as an unexpected filter.
func (q ActivityQuery) Normalize() ActivityQuery {
	if !validActivityArea(q.Area) {
		q.Area = ""
	}
	ok := false
	for _, w := range activityWhens {
		if w.Key == q.When {
			ok = true
		}
	}
	if !ok {
		q.When = When7Days
	}
	if len(q.Actor) > 36 {
		q.Actor = ""
	}
	return q
}

// ResolveActivityFilter turns a query into a filter. "Today" is since UTC
// midnight because created_at is stored in UTC.
func ResolveActivityFilter(q ActivityQuery, now time.Time) ActivityFilter {
	q = q.Normalize()
	f := ActivityFilter{ActorID: q.Actor, Area: q.Area}
	now = now.UTC()
	switch q.When {
	case WhenToday:
		f.Since = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	case When7Days:
		f.Since = now.AddDate(0, 0, -7)
	case When30:
		f.Since = now.AddDate(0, 0, -30)
	}
	return f
}

// buildActivityWhere turns a filter into a WHERE clause with placeholders only;
// every value travels as an argument, never inside the SQL text. The clause is
// empty when the filter is.
func buildActivityWhere(f ActivityFilter) (string, []any) {
	var conds []string
	var args []any
	if f.ActorID != "" {
		conds = append(conds, "a.actor_user_id = ?")
		args = append(args, f.ActorID)
	}
	if !f.Since.IsZero() {
		conds = append(conds, "a.created_at >= ?")
		args = append(args, f.Since)
	}
	if validActivityArea(f.Area) {
		resources := activityResourcesIn(f.Area)
		not := ""
		if f.Area == AreaOther {
			// Other is whatever no known area claims.
			resources = allActivityResources()
			not = "NOT "
		}
		if len(resources) > 0 {
			conds = append(conds, "SUBSTRING_INDEX(a.action, '.', 1) "+not+"IN ("+strings.TrimSuffix(strings.Repeat("?,", len(resources)), ",")+")")
			for _, r := range resources {
				args = append(args, r)
			}
		}
	}
	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}
