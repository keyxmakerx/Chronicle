package admin

import (
	"testing"
	"time"
)

func TestBuildNeedsYou(t *testing.T) {
	tests := []struct {
		name     string
		in       needsInput
		wantHref []string
	}{
		{"nothing", needsInput{SMTPKnown: true, SMTPConfigured: true}, nil},
		{"smtp service not wired", needsInput{}, nil},
		{"smtp missing", needsInput{SMTPKnown: true}, []string{"/admin/smtp"}},
		{
			"everything, most urgent first",
			needsInput{UnhealthyPlugins: 1, PendingMigrations: 2, APIAlerts: 3, PendingSubmissions: 4, SMTPKnown: true},
			[]string{"/admin/systems", "/admin/database", "/admin/api", "/admin/packages/pending", "/admin/smtp"},
		},
		{
			"backup failed is a fault, daily backups off a chore",
			needsInput{BackupFailed: true, DailyBackupOff: true, PendingMigrations: 1, PendingSubmissions: 1, SMTPKnown: true, SMTPConfigured: true},
			[]string{"/admin/backup", "/admin/database", "/admin/packages/pending", "/admin/backup"},
		},
		{
			"only some",
			needsInput{PendingSubmissions: 2, APIAlerts: 1, SMTPKnown: true, SMTPConfigured: true},
			[]string{"/admin/api", "/admin/packages/pending"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildNeedsYou(tc.in)
			if len(got) != len(tc.wantHref) {
				t.Fatalf("got %d items, want %d: %+v", len(got), len(tc.wantHref), got)
			}
			for i, h := range tc.wantHref {
				if got[i].Href != h {
					t.Errorf("item %d href = %q, want %q", i, got[i].Href, h)
				}
			}
		})
	}
}

func TestWithBackupNeeds(t *testing.T) {
	tests := []struct {
		name       string
		bi         BackupInfo
		wantFailed bool
		wantOff    bool
	}{
		{"no backup folder", BackupInfo{LastRunFailed: true, ScheduleKnown: true}, false, false},
		{"schedule unread", BackupInfo{Enabled: true}, false, false},
		{"daily off", BackupInfo{Enabled: true, ScheduleKnown: true}, false, true},
		{"daily on", BackupInfo{Enabled: true, ScheduleKnown: true, ScheduleOn: true}, false, false},
		{"last run failed", BackupInfo{Enabled: true, ScheduleKnown: true, ScheduleOn: true, LastRunFailed: true}, true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := withBackupNeeds(needsInput{}, tc.bi)
			if got.BackupFailed != tc.wantFailed || got.DailyBackupOff != tc.wantOff {
				t.Errorf("got failed=%v off=%v, want failed=%v off=%v", got.BackupFailed, got.DailyBackupOff, tc.wantFailed, tc.wantOff)
			}
		})
	}
}

func TestLastGoodBackup(t *testing.T) {
	t0 := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name          string
		manifests     []RestoreManifest
		want          time.Time
		wantPreUpdate bool
	}{
		{"none", nil, time.Time{}, false},
		{"empty-site capture before first setup is not a backup",
			[]RestoreManifest{{PreUpdate: true, MigrationVersion: "0", ModTime: t0}}, time.Time{}, false},
		{"unreadable manifest is not counted",
			[]RestoreManifest{{ParseError: "bad", ModTime: t0}}, time.Time{}, false},
		{"newest wins and says which kind",
			[]RestoreManifest{{ModTime: t0.Add(-time.Hour)}, {PreUpdate: true, MigrationVersion: "40", ModTime: t0}}, t0, true},
		{"own backup newer than the pre-update one",
			[]RestoreManifest{{ModTime: t0}, {PreUpdate: true, MigrationVersion: "40", ModTime: t0.Add(-time.Hour)}}, t0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, pre := lastGoodBackup(BackupInfo{Manifests: tc.manifests})
			if !got.Equal(tc.want) || pre != tc.wantPreUpdate {
				t.Errorf("got %v pre=%v, want %v pre=%v", got, pre, tc.want, tc.wantPreUpdate)
			}
		})
	}
}

func TestBuildNeedsYou_Wording(t *testing.T) {
	one := buildNeedsYou(needsInput{PendingSubmissions: 1})
	many := buildNeedsYou(needsInput{PendingSubmissions: 3})
	if one[0].Text == many[0].Text {
		t.Errorf("singular and plural sentences should differ, both %q", one[0].Text)
	}
}
