package campaigns

import "testing"

func TestHeroChangeRuleValidate(t *testing.T) {
	cases := []struct {
		name string
		rule *HeroChangeRule
		ok   bool
	}{
		{"nil is any time", nil, true},
		{"days after making", &HeroChangeRule{AfterMakingDays: 8}, true},
		{"days after a level", &HeroChangeRule{AfterLevel: true, FromLevel: 2, AfterLevelDays: 8}, true},
		{"both", &HeroChangeRule{AfterMakingDays: 3, AfterLevel: true, AfterLevelDays: 8}, true},
		{"opens nothing", &HeroChangeRule{}, false},
		{"a level window with no length", &HeroChangeRule{AfterLevel: true, FromLevel: 2}, false},
		{"too many days", &HeroChangeRule{AfterMakingDays: HeroChangeMaxDays + 1}, false},
		{"negative days", &HeroChangeRule{AfterMakingDays: -1}, false},
		{"level out of range", &HeroChangeRule{AfterLevel: true, FromLevel: HeroChangeMaxLevel + 1, AfterLevelDays: 1}, false},
	}
	for _, tc := range cases {
		if err := tc.rule.Validate(); (err == nil) != tc.ok {
			t.Errorf("%s: err = %v", tc.name, err)
		}
	}
	r := &HeroChangeRule{AfterMakingDays: 2, FromLevel: 5, AfterLevelDays: 9}
	if err := r.Validate(); err != nil || r.FromLevel != 0 || r.AfterLevelDays != 0 {
		t.Fatalf("level numbers without the level window must be dropped: %+v %v", r, err)
	}
}
