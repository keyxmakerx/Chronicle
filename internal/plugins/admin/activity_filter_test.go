package admin

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestActivityAreaOf(t *testing.T) {
	tests := []struct{ action, want string }{
		{"user.admin_granted", AreaPeople},
		{"session.terminated", AreaPeople},
		{"campaign.deleted", AreaCampaigns},
		{"package.added", AreaPackages},
		{"foundry.campaign_notified", AreaPackages},
		{"addon.changed", AreaFeatures},
		{"extension.installed", AreaFeatures},
		{"registration.mode_changed", AreaSecurity},
		{"apikey.changed", AreaSecurity},
		{"smtp.saved", AreaSite},
		{"sitelook.saved", AreaSite},
		{"backup.run", AreaSite},
		{"storage.user_limit_set", AreaSite},
		{"media.deleted", AreaSite},
		{"brand_new.thing", AreaOther},
		{"", AreaOther},
	}
	for _, tc := range tests {
		t.Run(tc.action, func(t *testing.T) {
			if got := ActivityAreaOf(tc.action); got != tc.want {
				t.Errorf("ActivityAreaOf(%q) = %q, want %q", tc.action, got, tc.want)
			}
		})
	}
}

// Every action with a sentence must land in a real area, so a recorder added
// without a mapping shows up here rather than silently under Other.
func TestActivityAreaOf_AllPhrasedActionsMapped(t *testing.T) {
	for action := range activityPhrases {
		if ActivityAreaOf(action) == AreaOther {
			t.Errorf("action %q has no area mapping", action)
		}
	}
}

func TestBuildActivityWhere(t *testing.T) {
	since := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	tests := []struct {
		name     string
		f        ActivityFilter
		wantSQL  string
		wantArgs []any
	}{
		{"empty", ActivityFilter{}, "", nil},
		{"actor", ActivityFilter{ActorID: "u1"}, " WHERE a.actor_user_id = ?", []any{"u1"}},
		{"since", ActivityFilter{Since: since}, " WHERE a.created_at >= ?", []any{since}},
		{"area", ActivityFilter{Area: AreaPeople},
			" WHERE SUBSTRING_INDEX(a.action, '.', 1) IN (?,?)", []any{"session", "user"}},
		{"all three", ActivityFilter{ActorID: "u1", Area: AreaCampaigns, Since: since},
			" WHERE a.actor_user_id = ? AND a.created_at >= ? AND SUBSTRING_INDEX(a.action, '.', 1) IN (?)",
			[]any{"u1", since, "campaign"}},
		{"unknown area ignored", ActivityFilter{Area: "bogus"}, "", nil},
		{"injection stays an argument", ActivityFilter{ActorID: "x' OR 1=1 --"},
			" WHERE a.actor_user_id = ?", []any{"x' OR 1=1 --"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sql, args := buildActivityWhere(tc.f)
			if sql != tc.wantSQL {
				t.Errorf("sql = %q, want %q", sql, tc.wantSQL)
			}
			if !reflect.DeepEqual(args, tc.wantArgs) {
				t.Errorf("args = %v, want %v", args, tc.wantArgs)
			}
			if strings.Count(sql, "?") != len(args) {
				t.Errorf("placeholder/arg mismatch: %q vs %v", sql, args)
			}
		})
	}
}

func TestBuildActivityWhere_Other(t *testing.T) {
	sql, args := buildActivityWhere(ActivityFilter{Area: AreaOther})
	if !strings.Contains(sql, "NOT IN (") || len(args) != len(activityResourceArea) {
		t.Errorf("other = %q with %d args", sql, len(args))
	}
}

func TestResolveActivityFilter(t *testing.T) {
	now := time.Date(2026, 5, 10, 15, 30, 0, 0, time.UTC)
	tests := []struct {
		name      string
		q         ActivityQuery
		wantSince time.Time
		wantArea  string
		wantActor string
	}{
		{"default is 7 days", ActivityQuery{}, now.AddDate(0, 0, -7), "", ""},
		{"today", ActivityQuery{When: WhenToday}, time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC), "", ""},
		{"30 days", ActivityQuery{When: When30}, now.AddDate(0, 0, -30), "", ""},
		{"all time", ActivityQuery{When: WhenAll}, time.Time{}, "", ""},
		{"bad when falls back", ActivityQuery{When: "yesterday"}, now.AddDate(0, 0, -7), "", ""},
		{"bad area dropped", ActivityQuery{Area: "nope", When: WhenAll}, time.Time{}, "", ""},
		{"area and actor kept", ActivityQuery{Area: AreaSite, Actor: "u1", When: WhenAll}, time.Time{}, AreaSite, "u1"},
		{"oversized actor dropped", ActivityQuery{Actor: strings.Repeat("a", 40), When: WhenAll}, time.Time{}, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := ResolveActivityFilter(tc.q, now)
			if !f.Since.Equal(tc.wantSince) || f.Area != tc.wantArea || f.ActorID != tc.wantActor {
				t.Errorf("got %+v", f)
			}
		})
	}
}
