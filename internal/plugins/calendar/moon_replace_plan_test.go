// moon_replace_plan_test.go — pins the diff-wise SetMoons planner that
// replaced the DELETE+INSERT footgun (moon edits were resetting the
// migration-008 render props and cascading away calendar_moon_phases rows).
package calendar

import (
	"reflect"
	"testing"
)

func TestPlanMoonReplace(t *testing.T) {
	moon := func(name string) MoonInput { return MoonInput{Name: name, CycleDays: 29.5} }

	cases := []struct {
		name     string
		existing []moonRef
		inputs   []MoonInput
		want     moonReplacePlan
	}{
		{
			name:     "edit in place keeps identity",
			existing: []moonRef{{ID: 1, Name: "Selune"}},
			inputs:   []MoonInput{{Name: "Selune", CycleDays: 30.4, Color: "#fff"}},
			want: moonReplacePlan{
				Updates: []moonUpdate{{ID: 1, Input: MoonInput{Name: "Selune", CycleDays: 30.4, Color: "#fff"}}},
			},
		},
		{
			name:     "add and remove",
			existing: []moonRef{{ID: 1, Name: "Selune"}, {ID: 2, Name: "Shar"}},
			inputs:   []MoonInput{moon("Selune"), moon("Luna")},
			want: moonReplacePlan{
				Updates:   []moonUpdate{{ID: 1, Input: moon("Selune")}},
				Inserts:   []MoonInput{moon("Luna")},
				DeleteIDs: []int{2},
			},
		},
		{
			name:     "rename reads as delete+add (no id on the wire)",
			existing: []moonRef{{ID: 1, Name: "Selune"}},
			inputs:   []MoonInput{moon("Selûne")},
			want: moonReplacePlan{
				Inserts:   []MoonInput{moon("Selûne")},
				DeleteIDs: []int{1},
			},
		},
		{
			name:     "duplicate names pair off one-to-one in order",
			existing: []moonRef{{ID: 1, Name: "Twin"}, {ID: 2, Name: "Twin"}},
			inputs:   []MoonInput{moon("Twin"), moon("Twin"), moon("Twin")},
			want: moonReplacePlan{
				Updates: []moonUpdate{{ID: 1, Input: moon("Twin")}, {ID: 2, Input: moon("Twin")}},
				Inserts: []MoonInput{moon("Twin")},
			},
		},
		{
			name:     "clear all",
			existing: []moonRef{{ID: 1, Name: "Selune"}},
			inputs:   nil,
			want:     moonReplacePlan{DeleteIDs: []int{1}},
		},
		{
			name:   "seed from empty",
			inputs: []MoonInput{moon("Selune")},
			want:   moonReplacePlan{Inserts: []MoonInput{moon("Selune")}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := planMoonReplace(tc.existing, tc.inputs)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("planMoonReplace:\n got %+v\nwant %+v", got, tc.want)
			}
		})
	}
}
