package campaigns

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

func TestHeroChangeRuleSummary(t *testing.T) {
	cases := []struct {
		rule *HeroChangeRule
		want string
	}{
		{nil, "Players can change their hero's picks any time."},
		{&HeroChangeRule{AfterMakingDays: 1}, "Players can change their hero's picks for the first 1 day after making it."},
		{&HeroChangeRule{AfterLevel: true, FromLevel: 2, AfterLevelDays: 8}, "Players can change their hero's picks for 8 days after each new level."},
		{&HeroChangeRule{AfterMakingDays: 7, AfterLevel: true, FromLevel: 4, AfterLevelDays: 3},
			"Players can change their hero's picks for the first 7 days after making it, and for 3 days after each new level from level 4 on."},
	}
	for _, tc := range cases {
		if got := tc.rule.Summary(); got != tc.want {
			t.Errorf("got %q\nwant %q", got, tc.want)
		}
	}
}

func TestHeroChangeForm(t *testing.T) {
	cases := []struct {
		name    string
		form    HeroChangeForm
		want    *HeroChangeRule
		refused bool
	}{
		{"any time", HeroChangeForm{When: "any", Making: "1", MakingDays: "5"}, nil, false},
		{"nothing chosen is any time", HeroChangeForm{}, nil, false},
		{"after making", HeroChangeForm{When: "set", Making: "1", MakingDays: " 5 "}, &HeroChangeRule{AfterMakingDays: 5}, false},
		{"after a level", HeroChangeForm{When: "set", Level: "1", FromLevel: "3", LevelDays: "4"}, &HeroChangeRule{AfterLevel: true, FromLevel: 3, AfterLevelDays: 4}, false},
		{"unticked numbers are ignored", HeroChangeForm{When: "set", MakingDays: "5", Level: "1", LevelDays: "2"}, &HeroChangeRule{AfterLevel: true, AfterLevelDays: 2}, false},
		{"after making with no days", HeroChangeForm{When: "set", Making: "1", MakingDays: "x"}, nil, true},
	}
	for _, tc := range cases {
		got, err := tc.form.Rule()
		if (err != nil) != tc.refused {
			t.Errorf("%s: err = %v", tc.name, err)
			continue
		}
		if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestHeroChangeFormValues(t *testing.T) {
	if v := (*HeroChangeRule)(nil).FormValues(); v.Ruled || v.MakingDays != HeroChangeDefaultDays || v.FromLevel != HeroChangeDefaultLevel || v.LevelDays != HeroChangeDefaultDays {
		t.Errorf("no rule suggests the defaults: %+v", v)
	}
	v := (&HeroChangeRule{AfterLevel: true, FromLevel: 0, AfterLevelDays: 3}).FormValues()
	if !v.Ruled || v.AfterMaking || v.MakingDays != HeroChangeDefaultDays || !v.AfterLevel || v.FromLevel != HeroChangeDefaultLevel || v.LevelDays != 3 {
		t.Errorf("a level rule keeps its numbers and suggests the rest: %+v", v)
	}
}

// heroRuleService records the rule People stores.
type heroRuleService struct {
	deskStubService
	saved *HeroChangeRule
	calls int
}

func (s *heroRuleService) UpdateHeroChanges(_ context.Context, _ string, rule *HeroChangeRule) error {
	if err := rule.Validate(); err != nil {
		return err
	}
	s.calls++
	s.saved = rule
	return nil
}

// heroChangesWhen is a lock that applies to every campaign, or to none.
func heroChangesWhen(on bool) func(context.Context, string) bool {
	return func(context.Context, string) bool { return on }
}

func heroSettingRequest(h *Handler, role Role, form url.Values, htmx bool) (*httptest.ResponseRecorder, error) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/campaigns/c1/members/hero-changes", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "c1"}})
	c.Set(contextKeyCampaign, &CampaignContext{Campaign: &Campaign{ID: "c1", Name: "Therin", Settings: "{}"}, MemberRole: role, IsMember: true})
	auth.SetSession(c, &auth.Session{UserID: "u1"})
	return rec, h.SaveHeroChangesSetting(c)
}

func TestSaveHeroChangesSetting(t *testing.T) {
	week := url.Values{"when": {"set"}, "after_making": {"1"}, "making_days": {"7"}}

	t.Run("the owner saves and the section redraws", func(t *testing.T) {
		svc := &heroRuleService{}
		h := NewHandler(svc)
		h.SetHeroChanges(heroChangesWhen(true))
		rec, err := heroSettingRequest(h, RoleOwner, week, true)
		if err != nil || rec.Code != http.StatusOK {
			t.Fatalf("err=%v code=%d", err, rec.Code)
		}
		if svc.saved == nil || svc.saved.AfterMakingDays != 7 {
			t.Fatalf("stored %+v", svc.saved)
		}
		body := rec.Body.String()
		for _, want := range []string{`id="hero-changes"`, "for the first 7 days after making it", "Saved.", "is-glow", "/characters#hero-changes"} {
			if !strings.Contains(body, want) {
				t.Errorf("section is missing %q", want)
			}
		}
	})

	t.Run("any time clears the rule", func(t *testing.T) {
		svc := &heroRuleService{saved: &HeroChangeRule{AfterMakingDays: 3}}
		h := NewHandler(svc)
		h.SetHeroChanges(heroChangesWhen(true))
		rec, err := heroSettingRequest(h, RoleOwner, url.Values{"when": {"any"}}, true)
		if err != nil || svc.calls != 1 || svc.saved != nil {
			t.Fatalf("err=%v calls=%d saved=%+v", err, svc.calls, svc.saved)
		}
		if strings.Contains(rec.Body.String(), "/characters#hero-changes") {
			t.Error("with no rule there is no hero to reopen, so no way to the Characters page is offered")
		}
	})

	t.Run("a refused rule says why and keeps what was typed", func(t *testing.T) {
		svc := &heroRuleService{}
		h := NewHandler(svc)
		h.SetHeroChanges(heroChangesWhen(true))
		rec, err := heroSettingRequest(h, RoleOwner, url.Values{"when": {"set"}, "after_level": {"1"}, "from_level": {"4"}, "level_days": {"0"}}, true)
		if err != nil || rec.Code != http.StatusOK || svc.calls != 0 {
			t.Fatalf("err=%v code=%d calls=%d", err, rec.Code, svc.calls)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `role="alert"`) || !strings.Contains(body, "Say how many days a new level keeps the picks open.") {
			t.Errorf("no refusal shown: %s", body)
		}
		if !strings.Contains(body, `name="from_level" min="2" max="100" value="4"`) {
			t.Error("the typed level is lost")
		}
	})

	t.Run("a refusal the form itself makes keeps the ticks", func(t *testing.T) {
		svc := &heroRuleService{}
		h := NewHandler(svc)
		h.SetHeroChanges(heroChangesWhen(true))
		rec, err := heroSettingRequest(h, RoleOwner, url.Values{"when": {"set"}, "after_making": {"1"}, "making_days": {""}, "after_level": {"1"}, "level_days": {"3"}, "from_level": {"5"}}, true)
		if err != nil || svc.calls != 0 {
			t.Fatalf("err=%v calls=%d", err, svc.calls)
		}
		body := rec.Body.String()
		for _, want := range []string{"Say how many days after making a hero the picks stay open.",
			`name="after_making" value="1" checked`, `name="after_level" value="1" checked`, `name="level_days" min="1" max="365" value="3"`, `name="from_level" min="2" max="100" value="5"`, `value="set" checked`} {
			if !strings.Contains(body, want) {
				t.Errorf("redraw is missing %q", want)
			}
		}
	})

	t.Run("without HTMX a refusal draws the page with its sentence", func(t *testing.T) {
		h := NewHandler(&peopleRuleService{})
		h.SetHeroChanges(heroChangesWhen(true))
		rec, err := heroSettingRequest(h, RoleOwner, url.Values{"when": {"set"}}, false)
		if err != nil || rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Choose after making the hero, after a new level, or both.") {
			t.Fatalf("err=%v code=%d", err, rec.Code)
		}
	})

	t.Run("without HTMX it goes back to the section", func(t *testing.T) {
		h := NewHandler(&heroRuleService{})
		h.SetHeroChanges(heroChangesWhen(true))
		rec, err := heroSettingRequest(h, RoleOwner, week, false)
		if err != nil || rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/campaigns/c1/members#hero-changes" {
			t.Fatalf("err=%v code=%d location=%q", err, rec.Code, rec.Header().Get("Location"))
		}
	})

	t.Run("only the owner, and only while the lock is wired", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			role Role
			on   bool
		}{{"player", RolePlayer, true}, {"scribe", RoleScribe, true}, {"owner with the lock off", RoleOwner, false}} {
			svc := &heroRuleService{}
			h := NewHandler(svc)
			h.SetHeroChanges(heroChangesWhen(tc.on))
			_, err := heroSettingRequest(h, tc.role, week, true)
			var ae *apperror.AppError
			if !errors.As(err, &ae) || ae.Code != http.StatusNotFound || svc.calls != 0 {
				t.Errorf("%s: err=%v calls=%d", tc.name, err, svc.calls)
			}
		}
	})
}

// peopleStubService answers what the People page reads.
type peopleStubService struct{ deskStubService }

func (peopleStubService) ListRemovedMembers(context.Context, string) ([]MemberUser, error) {
	return nil, nil
}

func (peopleStubService) GetPendingTransfer(context.Context, string) (*OwnershipTransfer, error) {
	return nil, nil
}

// peopleRuleService is heroRuleService with what the People page reads.
type peopleRuleService struct{ heroRuleService }

func (*peopleRuleService) ListRemovedMembers(context.Context, string) ([]MemberUser, error) {
	return nil, nil
}

func (*peopleRuleService) GetPendingTransfer(context.Context, string) (*OwnershipTransfer, error) {
	return nil, nil
}

// People shows the section to the owner only where the lock applies (its
// table is healthy and players can claim heroes), and Find lists it only then.
func TestPeoplePage_ChangingHeroes(t *testing.T) {
	for _, on := range []bool{true, false} {
		h := NewHandler(peopleStubService{})
		h.SetHeroChanges(heroChangesWhen(on))
		rec, err := settingsOwnerRequest(h, http.MethodGet, "/campaigns/c1/members", RoleOwner, (*Handler).Members)
		if err != nil || rec.Code != http.StatusOK {
			t.Fatalf("on=%v: err=%v code=%d", on, err, rec.Code)
		}
		if got := strings.Contains(rec.Body.String(), `id="hero-changes"`); got != on {
			t.Errorf("on=%v: section shown = %v", on, got)
		}
		if got := hasLabel(buildDeskFind(deskFindInput{Base: "/campaigns/c1", HeroChanges: on}), "Changing heroes"); got != on {
			t.Errorf("on=%v: Find lists it = %v", on, got)
		}
	}
}
