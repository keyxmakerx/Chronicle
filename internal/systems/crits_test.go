package systems

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestValidateCrits(t *testing.T) {
	ptr := func(r ...CritRule) *[]CritRule { return &r }
	tests := []struct {
		name    string
		rules   *[]CritRule
		wantErr string
	}{
		{"nil is fine", nil, ""},
		{"empty list is fine", ptr(), ""},
		{"d20 each", ptr(CritRule{Dice: "d20", Succ: []int{20}, Fail: []int{1}}), ""},
		{"draw steel total", ptr(CritRule{Dice: "2d10", Judge: "total", Succ: []int{19, 20}}), ""},
		{"bad dice", ptr(CritRule{Dice: "d7", Succ: []int{1}}), "dice"},
		{"five dice", ptr(CritRule{Dice: "5d6", Judge: "total", Succ: []int{6}}), "dice"},
		{"total needs a count", ptr(CritRule{Dice: "d10", Judge: "total", Succ: []int{5}}), "needs a count"},
		{"unknown judge", ptr(CritRule{Dice: "d20", Judge: "sum", Succ: []int{20}}), "judge"},
		{"nothing listed", ptr(CritRule{Dice: "d20"}), "at least one"},
		{"face above the die", ptr(CritRule{Dice: "d20", Succ: []int{21}}), "can't come up"},
		{"face of zero", ptr(CritRule{Dice: "d20", Fail: []int{0}}), "can't come up"},
		{"total below the pair's minimum", ptr(CritRule{Dice: "2d10", Judge: "total", Fail: []int{1}}), "can't come up"},
		{"total above the pair's maximum", ptr(CritRule{Dice: "2d10", Judge: "total", Succ: []int{21}}), "can't come up"},
		{"too many values", ptr(CritRule{Dice: "d20", Succ: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 20}}), "too many succ"},
		{"too many rules", ptr(make([]CritRule, 9)...), "too many rules"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCrits(tt.rules)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want one containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestCritRulesDefaultsAndEmpty(t *testing.T) {
	var absent, declaredEmpty, declared SystemManifest
	if err := json.Unmarshal([]byte(`{}`), &absent); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"crits": []}`), &declaredEmpty); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"crits":[{"dice":"2d10","judge":"total","succ":[19,20]}]}`), &declared); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		m    *SystemManifest
		want []CritRule
	}{
		{"nil manifest", nil, DefaultCritRules()},
		{"no block", &absent, DefaultCritRules()},
		{"empty block means no crits", &declaredEmpty, []CritRule{}},
		{"declared", &declared, []CritRule{{Dice: "2d10", Judge: "total", Succ: []int{19, 20}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.m.CritRules(); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestEmptyCritsSurviveRemarshal(t *testing.T) {
	var m SystemManifest
	if err := json.Unmarshal([]byte(`{"crits": []}`), &m); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(m)
	var back SystemManifest
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if got := back.CritRules(); len(got) != 0 {
		t.Fatalf("an empty crits block came back as %v", got)
	}
}
