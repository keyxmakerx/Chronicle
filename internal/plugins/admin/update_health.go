package admin

import (
	"context"
	"fmt"
	"time"

	"github.com/keyxmakerx/chronicle/internal/updatehealth"
)

// UpdateHealthReporter reports where each feature the running release
// changed stands: failing, not used yet, still counting or passed.
type UpdateHealthReporter interface {
	Report(ctx context.Context) ([]updatehealth.Status, error)
}

// SetUpdateHealth wires update health into Site admin Home.
func (h *Handler) SetUpdateHealth(r UpdateHealthReporter) { h.updateHealth = r }

// updateHealthErrorsHref is where "See the errors" goes: the AI helper,
// whose server-errors check lists the recent failures by route.
const updateHealthErrorsHref = "/admin/diagnostics/workspace"

// updateHealthNeeds turns the changed features that need a site admin into
// Needs you rows: a failing feature is a fault, shown in red with the number
// of failures; one not used yet is a chore, shown in amber. Features still
// counting or passed make no row.
func updateHealthNeeds(statuses []updatehealth.Status, now time.Time) []NeedsItem {
	var items []NeedsItem
	for _, s := range statuses {
		switch s.State {
		case updatehealth.Failing:
			it := NeedsItem{
				Text:   fmt.Sprintf("%s has failed since the update on %s", s.Name, s.FirstSeen.Format("Mon 2 Jan")),
				Count:  s.Errors,
				Href:   updateHealthErrorsHref,
				Button: "See the errors",
				rank:   1,
			}
			if s.LastError != nil && s.LastErrorMsg != "" {
				it.Detail = "Last: " + s.LastErrorMsg + ", " + agoText(now.Sub(*s.LastError))
			}
			items = append(items, it)
		case updatehealth.Unused:
			days := int(now.Sub(s.FirstSeen).Hours() / 24)
			text := fmt.Sprintf("%s changed %d days ago and nobody has used it yet", s.Name, days)
			if s.Uses > 0 {
				text = fmt.Sprintf("%s changed %d days ago and has been used only %s", s.Name, days, timesText(s.Uses))
			}
			items = append(items, NeedsItem{Text: text, Count: 1, Href: s.Try, Button: "Try it now", rank: 4})
		}
	}
	return items
}

// UpdateHealthView is what Home shows under Needs you about the changed
// features that need nobody: the ones that passed, and how many are still
// being counted.
type UpdateHealthView struct {
	Passed   []updatehealth.Status
	Counting int
}

// Line is the quiet sentence for the view.
func (v UpdateHealthView) Line() string { return passedLine(len(v.Passed), v.Counting) }

// passedFeatures are the changed features that passed by use, for the quiet
// line under Needs you.
func passedFeatures(statuses []updatehealth.Status) []updatehealth.Status {
	var out []updatehealth.Status
	for _, s := range statuses {
		if s.State == updatehealth.Passed {
			out = append(out, s)
		}
	}
	return out
}

// countingFeatures is how many changed features are still collecting uses.
func countingFeatures(statuses []updatehealth.Status) int {
	n := 0
	for _, s := range statuses {
		if s.State == updatehealth.Counting {
			n++
		}
	}
	return n
}

// passedLine is the quiet sentence under Needs you, or "" when no changed
// feature has passed or is still being counted.
func passedLine(passed, counting int) string {
	switch {
	case passed == 0 && counting == 0:
		return ""
	case passed == 0:
		return fmt.Sprintf("%s still being counted after the update.", featuresText(counting))
	case counting == 0:
		return fmt.Sprintf("%s passed by use.", featuresText(passed))
	default:
		return fmt.Sprintf("%s passed by use; %d still being counted.", featuresText(passed), counting)
	}
}

func featuresText(n int) string {
	if n == 1 {
		return "1 changed feature"
	}
	return fmt.Sprintf("%d changed features", n)
}

func timesText(n int) string {
	if n == 1 {
		return "once"
	}
	return fmt.Sprintf("%d times", n)
}

// agoText is a short "how long ago" for the last failure.
func agoText(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d h ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	}
}

// usesLabel is a passed feature's count, e.g. "12 uses".
func usesLabel(n int) string {
	if n == 1 {
		return "1 use"
	}
	return fmt.Sprintf("%d uses", n)
}
