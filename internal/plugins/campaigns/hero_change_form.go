package campaigns

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// hero_change_form.go is what both places that set the hero change rule
// share: the rule in one sentence, and the form both post. The People page
// (owner) and the Characters page (every Director, Co-DMs included, who
// cannot open People) draw the same form, so a rule reads and saves the
// same wherever it is set.

// HeroChangeDefaultDays and HeroChangeDefaultLevel are the numbers the form
// suggests before a rule exists: a week after making, and a week after each
// new level.
const (
	HeroChangeDefaultDays  = 7
	HeroChangeDefaultLevel = 2
)

// Summary says the rule in one sentence, as a Director reads it. A nil rule
// is any time.
func (r *HeroChangeRule) Summary() string {
	if r == nil {
		return "Players can change their hero's picks any time."
	}
	var parts []string
	if r.AfterMakingDays > 0 {
		parts = append(parts, "for the first "+HeroDays(r.AfterMakingDays)+" after making it")
	}
	if r.AfterLevel {
		p := "for " + HeroDays(r.AfterLevelDays) + " after each new level"
		if r.FromLevel > 2 {
			p += fmt.Sprintf(" from level %d on", r.FromLevel)
		}
		parts = append(parts, p)
	}
	return "Players can change their hero's picks " + strings.Join(parts, ", and ") + "."
}

// HeroDays is a count of days in words: "1 day", "7 days".
func HeroDays(n int) string {
	if n == 1 {
		return "1 day"
	}
	return strconv.Itoa(n) + " days"
}

// HeroChangeForm is the rule's form as posted. A ticked box sends any
// non-empty value; numbers arrive as typed.
type HeroChangeForm struct {
	When       string `form:"when"`
	Making     string `form:"after_making"`
	MakingDays string `form:"making_days"`
	Level      string `form:"after_level"`
	FromLevel  string `form:"from_level"`
	LevelDays  string `form:"level_days"`
}

// Rule reads the form into a rule; nil is any time. It refuses only what the
// form alone can tell (a ticked "after making" with no days); the range
// checks are HeroChangeRule.Validate's, run when the rule is stored.
func (f HeroChangeForm) Rule() (*HeroChangeRule, error) {
	if f.When != "set" {
		return nil, nil
	}
	rule := &HeroChangeRule{}
	if f.Making != "" {
		rule.AfterMakingDays = formNumber(f.MakingDays)
		if rule.AfterMakingDays == 0 {
			return nil, apperror.NewBadRequest("say how many days after making a hero the picks stay open")
		}
	}
	if f.Level != "" {
		rule.AfterLevel = true
		rule.FromLevel = formNumber(f.FromLevel)
		rule.AfterLevelDays = formNumber(f.LevelDays)
	}
	return rule, nil
}

// Values is the form redrawn as it was posted, after a refusal, so what was
// typed can be put right. A number box left blank or unreadable keeps the
// number it showed before (from), unless its box was ticked, where it shows
// 0 so the blank stays visible.
func (f HeroChangeForm) Values(from HeroChangeFormValues) HeroChangeFormValues {
	v := from
	v.Ruled = f.When == "set"
	v.AfterMaking, v.AfterLevel = f.Making != "", f.Level != ""
	typed := func(raw string, ticked bool, into *int) {
		if n := formNumber(raw); n != 0 || ticked {
			*into = n
		}
	}
	typed(f.MakingDays, v.AfterMaking, &v.MakingDays)
	typed(f.LevelDays, v.AfterLevel, &v.LevelDays)
	typed(f.FromLevel, v.AfterLevel, &v.FromLevel)
	return v
}

// HeroChangeFormValues is the form's starting state: the stored rule, or the
// suggested numbers where it sets none.
type HeroChangeFormValues struct {
	Ruled       bool
	AfterMaking bool
	MakingDays  int
	AfterLevel  bool
	FromLevel   int
	LevelDays   int
}

// FormValues is the form's starting state for this rule.
func (r *HeroChangeRule) FormValues() HeroChangeFormValues {
	v := HeroChangeFormValues{
		Ruled:      r != nil,
		MakingDays: HeroChangeDefaultDays, FromLevel: HeroChangeDefaultLevel, LevelDays: HeroChangeDefaultDays,
	}
	if r == nil {
		return v
	}
	v.AfterMaking, v.AfterLevel = r.AfterMakingDays > 0, r.AfterLevel
	if r.AfterMakingDays > 0 {
		v.MakingDays = r.AfterMakingDays
	}
	if r.AfterLevel {
		v.FromLevel, v.LevelDays = max(r.FromLevel, HeroChangeDefaultLevel), r.AfterLevelDays
	}
	return v
}

// formNumber reads a whole number from a form field; 0 when it is not one.
func formNumber(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}
