package campaigns

import (
	"strings"
	"testing"
	"time"
)

func TestFoundryHealthChecks(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	old := now.Add(-72 * time.Hour)
	recent := now.Add(-time.Hour)
	cases := []struct {
		name       string
		conn       FoundryConnection
		wantLevels []FoundryCheckLevel
		wantTitle  string // substring of the first row's title
		wantDetail string // substring of the last row's detail
	}{
		{"no key, never seen", FoundryConnection{}, []FoundryCheckLevel{FoundryCheckBad}, "isn't connected yet", ""},
		{"key turned off after use is still bad", FoundryConnection{KeyLastUsed: &old}, []FoundryCheckLevel{FoundryCheckBad}, "no working connect line", "turned off"},
		{"connected", FoundryConnection{HasKey: true, Connected: true}, []FoundryCheckLevel{FoundryCheckOK}, "is connected", ""},
		{"seen recently", FoundryConnection{HasKey: true, KeyLastUsed: &recent}, []FoundryCheckLevel{FoundryCheckOK}, "connected recently", ""},
		{"stale", FoundryConnection{HasKey: true, KeyLastUsed: &old}, []FoundryCheckLevel{FoundryCheckWarn}, "in a while", ""},
		{"key but never connected", FoundryConnection{HasKey: true}, []FoundryCheckLevel{FoundryCheckWarn}, "never connected", ""},
		{"versions match with a v prefix", FoundryConnection{HasKey: true, Connected: true, ModuleVersion: "2.0.2", ServedVersion: "v2.0.2"},
			[]FoundryCheckLevel{FoundryCheckOK, FoundryCheckOK}, "is connected", "v2.0.2"},
		{"versions match the other way round", FoundryConnection{HasKey: true, Connected: true, ModuleVersion: "v2.0.2", ServedVersion: "2.0.2"},
			[]FoundryCheckLevel{FoundryCheckOK, FoundryCheckOK}, "is connected", ""},
		{"older module warns to restart", FoundryConnection{HasKey: true, Connected: true, ModuleVersion: "2.0.1", ServedVersion: "2.0.2"},
			[]FoundryCheckLevel{FoundryCheckOK, FoundryCheckWarn}, "is connected", "Restart"},
		{"unknown module version adds no version row", FoundryConnection{HasKey: true, Connected: true, ServedVersion: "2.0.2"},
			[]FoundryCheckLevel{FoundryCheckOK}, "is connected", ""},
		{"nothing served adds no version row", FoundryConnection{HasKey: true, Connected: true, ModuleVersion: "2.0.2"},
			[]FoundryCheckLevel{FoundryCheckOK}, "is connected", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FoundryHealthChecks(now, tc.conn)
			if len(got) != len(tc.wantLevels) {
				t.Fatalf("got %d rows %+v, want %d", len(got), got, len(tc.wantLevels))
			}
			for i, l := range tc.wantLevels {
				if got[i].Level != l {
					t.Fatalf("row %d level %q, want %q (%+v)", i, got[i].Level, l, got[i])
				}
			}
			if !strings.Contains(got[0].Title, tc.wantTitle) {
				t.Fatalf("title %q lacks %q", got[0].Title, tc.wantTitle)
			}
			if last := got[len(got)-1]; !strings.Contains(last.Detail, tc.wantDetail) {
				t.Fatalf("detail %q lacks %q", last.Detail, tc.wantDetail)
			}
		})
	}
}

func TestFoundryHealthChecks_VersionMismatchNamesBoth(t *testing.T) {
	got := FoundryHealthChecks(time.Now(), FoundryConnection{HasKey: true, Connected: true, ModuleVersion: "2.0.1", ServedVersion: "2.0.2"})
	row := got[len(got)-1]
	if !strings.Contains(row.Title, "2.0.1") || !strings.Contains(row.Title, "2.0.2") || !strings.Contains(row.Detail, "Restart your Foundry world") {
		t.Fatalf("got %+v", row)
	}
}
