// foundry_page.go — the owner's Foundry page: whether the campaign's Foundry
// world is working, its connect line, the module version and the install
// link, in one place.
//
// The page stays VTT-agnostic in what it owns: the connection comes through
// the FoundryConnector adapter, and the version, install and problem cards
// are lazy-loaded fragments from the plugins that own them. The checklist
// wording is a pure function so it is table-tested without a database.

package campaigns

import (
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
)

// FoundryCheckLevel is how a checklist row is marked.
type FoundryCheckLevel string

const (
	FoundryCheckOK   FoundryCheckLevel = "ok"
	FoundryCheckWarn FoundryCheckLevel = "warn"
	FoundryCheckBad  FoundryCheckLevel = "bad"
	FoundryCheckInfo FoundryCheckLevel = "info"
)

// FoundryCheck is one row of "Is everything working?".
type FoundryCheck struct {
	Level  FoundryCheckLevel
	Title  string
	Detail string
}

// FoundryHealthChecks words what Chronicle knows about the campaign's Foundry
// link as a checklist. Only facts Chronicle records appear; each player's
// state is in the Players in Foundry card, from what Foundry reports.
func FoundryHealthChecks(now time.Time, c FoundryConnection) []FoundryCheck {
	st := ComputeFoundryStatus(now, c)
	var out []FoundryCheck
	switch {
	case !c.HasKey && st.Level == FoundryNeverSeen:
		out = append(out, FoundryCheck{FoundryCheckBad, "Foundry isn't connected yet",
			"Make a connect line below and paste it into Chronicle Sync's settings in your Foundry world."})
	case !c.HasKey:
		out = append(out, FoundryCheck{FoundryCheckBad, "There is no working connect line",
			"Foundry was " + lowerFirst(st.Text) + ", but its connect line has been turned off. Make a new one below and paste it into Chronicle Sync in Foundry."})
	case st.Level == FoundryConnectedNow:
		out = append(out, FoundryCheck{FoundryCheckOK, "Foundry is connected", "Your world is open and talking to Chronicle right now."})
	case st.Level == FoundrySeenRecently:
		out = append(out, FoundryCheck{FoundryCheckOK, "Foundry has connected recently",
			st.Text + ". Your world isn't open right now, which is normal between sessions."})
	case st.Level == FoundrySeenLongAgo:
		out = append(out, FoundryCheck{FoundryCheckWarn, "Foundry hasn't connected in a while",
			st.Text + ". If your world is open, check the connect line in Chronicle Sync's settings in Foundry."})
	default:
		out = append(out, FoundryCheck{FoundryCheckWarn, "Foundry has never connected",
			"Paste the connect line into Chronicle Sync's settings in your Foundry world."})
	}
	if c.ModuleVersion != "" && c.ServedVersion != "" {
		// The module reports "2.0.2"; a release tag may read "v2.0.2".
		if strings.TrimPrefix(c.ModuleVersion, "v") == strings.TrimPrefix(c.ServedVersion, "v") {
			out = append(out, FoundryCheck{FoundryCheckOK, "Foundry runs the version Chronicle serves", "Both are " + c.ServedVersion + "."})
		} else {
			out = append(out, FoundryCheck{FoundryCheckWarn,
				"Foundry last ran " + c.ModuleVersion + ", but Chronicle serves " + c.ServedVersion,
				"Restart your Foundry world so it loads " + c.ServedVersion + "."})
		}
	}
	return out
}

// lowerFirst lowercases an ASCII first letter so a status reads mid-sentence.
func lowerFirst(s string) string {
	if s == "" || s[0] < 'A' || s[0] > 'Z' {
		return s
	}
	return string(s[0]+'a'-'A') + s[1:]
}

// FoundryPage renders GET /campaigns/:id/foundry (owner only).
func (h *Handler) FoundryPage(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	ctx := c.Request().Context()
	row := h.foundryRow(ctx, cc.Campaign.ID)
	var checks []FoundryCheck
	if row.Wired {
		checks = FoundryHealthChecks(time.Now(), row.conn)
	}
	return middleware.Render(c, http.StatusOK,
		FoundryPageView(cc, row, checks, middleware.GetCSRFToken(c)))
}

// foundryChecksNeedingLook counts the rows marked warn or bad.
func foundryChecksNeedingLook(checks []FoundryCheck) int {
	n := 0
	for _, c := range checks {
		if c.Level == FoundryCheckWarn || c.Level == FoundryCheckBad {
			n++
		}
	}
	return n
}

func foundryCheckBadgeClass(l FoundryCheckLevel) string {
	switch l {
	case FoundryCheckOK:
		return "bg-emerald-500/15 text-emerald-700 dark:text-emerald-400"
	case FoundryCheckWarn:
		return "bg-amber-500/15 text-amber-700 dark:text-amber-400"
	case FoundryCheckBad:
		return "bg-red-500/15 text-red-700 dark:text-red-400"
	default:
		return "bg-sky-500/15 text-sky-700 dark:text-sky-400"
	}
}

func foundryCheckGlyph(l FoundryCheckLevel) string {
	switch l {
	case FoundryCheckOK:
		return "✓"
	case FoundryCheckInfo:
		return "i"
	default:
		return "!"
	}
}

// foundryCheckSR is the row's mark for screen readers, which skip the glyph.
func foundryCheckSR(l FoundryCheckLevel) string {
	switch l {
	case FoundryCheckOK:
		return "Working: "
	case FoundryCheckWarn:
		return "Worth a look: "
	case FoundryCheckBad:
		return "Not working: "
	default:
		return "Note: "
	}
}
