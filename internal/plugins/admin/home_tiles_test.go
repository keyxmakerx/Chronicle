package admin

import (
	"testing"
	"time"
)

func TestBuildHomeGroups_OrderAndShape(t *testing.T) {
	groups := buildHomeGroups(homeInput{Now: time.Now()})
	var labels []string
	for _, g := range groups {
		labels = append(labels, g.Label)
		for _, tl := range g.Tiles {
			if tl.Big == "" || tl.Label == "" || tl.Line == "" || tl.Href == "" {
				t.Errorf("tile %q in %q is missing a part: %+v", tl.Label, g.Label, tl)
			}
		}
	}
	want := []string{"Community", "Packages & apps", "Security", "Site", "Tools"}
	if len(labels) != len(want) {
		t.Fatalf("groups = %v, want %v", labels, want)
	}
	for i := range want {
		if labels[i] != want[i] {
			t.Errorf("group %d = %q, want %q", i, labels[i], want[i])
		}
	}
}

func findTile(groups []HomeGroup, label string) HomeTile {
	for _, g := range groups {
		for _, tl := range g.Tiles {
			if tl.Label == label {
				return tl
			}
		}
	}
	return HomeTile{}
}

func TestBuildHomeGroups_Tiles(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		in       homeInput
		tile     string
		wantBig  string
		wantLine string
		wantAttn bool
	}{
		{"people count", homeInput{Users: 14}, "People", "14", "accounts on this site", false},
		{"people lookup failed shows dash", homeInput{Users: -1}, "People", "—", "accounts on this site", false},
		{"people disabled", homeInput{Users: 14, DisabledUsers: 2}, "People", "14", "2 disabled", false},
		{"packages pending", homeInput{RegisteredSystems: 3, PendingSubmissions: 1}, "Packages", "3", "1 to review", true},
		{"packages failed", homeInput{RegisteredSystems: 3, FailedSystems: 1}, "Packages", "3", "1 failed to load", true},
		{"packages fine", homeInput{RegisteredSystems: 3}, "Packages", "3", "game systems loaded", false},
		{"failed logins", homeInput{ActiveSessions: 6, FailedLogins24h: 4}, "Sign-ins & sessions", "6", "4 failed sign-ins in 24 hours", true},
		{"api alerts", homeInput{APIAlerts: 2}, "API & access", "2", "alerts not looked at", true},
		{"api clear", homeInput{}, "API & access", "0", "alerts not looked at", false},
		{"email not set up", homeInput{SMTPKnown: true}, "Email", "Not set up", "password resets and invites can't be sent", true},
		{"email set up", homeInput{SMTPKnown: true, SMTPConfigured: true}, "Email", "Set up", "mail server saved", false},
		{"email unknown", homeInput{}, "Email", "—", "status unavailable", false},
		{"health degraded", homeInput{DegradedParts: 2, RegisteredSystems: 3}, "Health & diagnostics", "2 need a look", "database and 3 game systems", true},
		{"health ok", homeInput{RegisteredSystems: 3}, "Health & diagnostics", "Healthy", "database and 3 game systems", false},
		{"backup age", homeInput{Now: now, BackupsKnown: true, BackupsEnabled: true, LastBackup: now.Add(-72 * time.Hour)}, "Backups & restore", "3 days", "since the last backup", false},
		{"backup none", homeInput{Now: now, BackupsKnown: true, BackupsEnabled: true}, "Backups & restore", "None yet", "no backup files found", true},
		{"backup off", homeInput{Now: now, BackupsKnown: true}, "Backups & restore", "Not set up", "no backup folder is configured", false},
		{"backup unknown", homeInput{Now: now}, "Backups & restore", "—", "status unavailable", false},
		{"storage", homeInput{StorageBytes: 1 << 30, MediaFiles: 412}, "Storage & cleanup", "1.0 GB", "in 412 files", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := findTile(buildHomeGroups(tc.in), tc.tile)
			if got.Big != tc.wantBig || got.Line != tc.wantLine || got.Attention != tc.wantAttn {
				t.Errorf("%s = %+v, want big=%q line=%q attn=%v", tc.tile, got, tc.wantBig, tc.wantLine, tc.wantAttn)
			}
		})
	}
}

func TestBackupAge(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{10 * time.Minute, "Under 1 hour"},
		{time.Hour, "1 hour"},
		{5 * time.Hour, "5 hours"},
		{47 * time.Hour, "47 hours"},
		{49 * time.Hour, "2 days"},
		{10 * 24 * time.Hour, "10 days"},
	}
	for _, tc := range tests {
		if got := backupAge(tc.d); got != tc.want {
			t.Errorf("backupAge(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}
