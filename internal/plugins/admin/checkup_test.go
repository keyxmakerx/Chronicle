package admin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// A Wednesday, so the week's boundary is three days back.
var ckNow = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

func okRecord() *CheckupRecord {
	return &CheckupRecord{
		At:        ckNow.Add(-time.Hour),
		Backup:    &BackupCheckResult{OK: true, BackupAt: ckNow.Add(-9 * time.Hour)},
		Chronicle: &VersionCheckResult{Known: true},
	}
}

func allGood() checkupInputs {
	return checkupInputs{Now: ckNow, Record: okRecord(), BackupKnown: true, DailyOn: true,
		Registration: "invite", EmailReady: true, PackageUpdates: 0}
}

func item(t *testing.T, c Checkup, key string) CheckItem {
	t.Helper()
	for _, it := range c.Items {
		if it.Key == key {
			return it
		}
	}
	t.Fatalf("no %q row", key)
	return CheckItem{}
}

func TestBuildCheckup(t *testing.T) {
	tests := []struct {
		name      string
		edit      func(*checkupInputs)
		key       string
		state     CheckState
		fix       string // FixHref or FixPost
		detailHas string
	}{
		{"all good backup", nil, "backup", CheckOK, "", "read back in full"},
		{"daily backups off", func(in *checkupInputs) { in.DailyOn = false }, "backup", CheckWarn, checkupBackupHref, "Daily backups are off"},
		{"never checked", func(in *checkupInputs) { in.Record = nil }, "backup", CheckWarn, "", "Not checked yet"},
		{"read-back failed", func(in *checkupInputs) {
			in.Record.Backup = &BackupCheckResult{Problem: "The backup can't be restored: the database dump is incomplete."}
		}, "backup", CheckBad, checkupBackupHref, "incomplete"},
		{"a later backup failed", func(in *checkupInputs) { in.LastRunFailed = true }, "backup", CheckBad, checkupBackupHref, "last backup failed"},
		{"a failure beats backups being off", func(in *checkupInputs) { in.LastRunFailed, in.DailyOn = true, false }, "backup", CheckBad, checkupBackupHref, ""},
		{"open sign-up", func(in *checkupInputs) { in.Registration = "open" }, "signup", CheckWarn, checkupSignupPost, "Anyone"},
		{"closed sign-up", func(in *checkupInputs) { in.Registration = "closed" }, "signup", CheckOK, "", "Closed"},
		{"no email", func(in *checkupInputs) { in.EmailReady = false }, "reset", CheckWarn, checkupEmailHref, "dead end"},
		{"game-system updates", func(in *checkupInputs) { in.PackageUpdates = 2 }, "update", CheckWarn, checkupPackagesHref, "2 game-system updates"},
		{"newer Chronicle", func(in *checkupInputs) { in.Record.Chronicle = &VersionCheckResult{Known: true, Behind: 12} }, "update", CheckWarn, checkupUpdateGuide, "12 changes"},
		{"Chronicle unknown is not a fault", func(in *checkupInputs) {
			in.Record.Chronicle = &VersionCheckResult{Problem: "GitHub couldn't be reached."}
		}, "update", CheckOK, "", "couldn't be checked"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := allGood()
			if tt.edit != nil {
				tt.edit(&in)
			}
			it := item(t, buildCheckup(in), tt.key)
			if it.State != tt.state {
				t.Errorf("state = %s, want %s (%q)", it.State, tt.state, it.Detail)
			}
			if got := it.FixHref + it.FixPost; got != tt.fix {
				t.Errorf("fix = %q, want %q", got, tt.fix)
			}
			if !strings.Contains(it.Detail, tt.detailHas) {
				t.Errorf("detail %q should mention %q", it.Detail, tt.detailHas)
			}
		})
	}
}

func TestBuildCheckup_SignupLeftOutWhenUnreadable(t *testing.T) {
	in := allGood()
	in.Registration = ""
	for _, it := range buildCheckup(in).Items {
		if it.Key == "signup" {
			t.Fatal("an unreadable registration mode must not be shown as a verdict")
		}
	}
}

func TestCheckupWording(t *testing.T) {
	good := buildCheckup(allGood())
	if good.Worst() != CheckOK || good.Title() != "All good" {
		t.Fatalf("worst=%s title=%q", good.Worst(), good.Title())
	}
	if got := good.Subtitle(); got != "Checked 1 hour ago. Next check-up Sunday at 04:00." {
		t.Errorf("subtitle = %q", got)
	}

	in := allGood()
	in.Registration, in.EmailReady = "open", false
	two := buildCheckup(in)
	if two.Title() != "2 things need you" {
		t.Errorf("title = %q", two.Title())
	}
	if got := two.StripText(); got != "2 check-up items have been waiting a week: who can sign up, players can get back in." {
		t.Errorf("strip = %q", got)
	}

	in = allGood()
	in.Record.Chronicle = &VersionCheckResult{Known: true, Behind: 3}
	if got := buildCheckup(in).StripText(); got != "1 check-up item has been waiting a week: Chronicle and game systems up to date." {
		t.Errorf("strip = %q", got)
	}

	in = allGood()
	in.LastRunFailed = true
	bad := buildCheckup(in)
	if bad.Worst() != CheckBad || bad.Title() != "A backup failed" || !strings.Contains(bad.StripText(), "not backed up") {
		t.Errorf("worst=%s title=%q strip=%q", bad.Worst(), bad.Title(), bad.StripText())
	}
}

func TestCheckupSchedule(t *testing.T) {
	sunday4 := time.Date(2026, 10, 18, 4, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		now  time.Time
		last *CheckupRecord
		want bool
	}{
		{"never run, mid-week: wait for Sunday", ckNow, nil, false},
		{"never run, Sunday 04:10", sunday4.Add(10 * time.Minute), nil, true},
		{"never run, Sunday 03:59", sunday4.Add(-time.Minute), nil, false},
		{"ran this week", ckNow, &CheckupRecord{At: sunday4.Add(time.Minute)}, false},
		{"missed Sunday, made up", ckNow, &CheckupRecord{At: sunday4.Add(-24 * time.Hour)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkupDue(tt.now, tt.last); got != tt.want {
				t.Fatalf("checkupDue = %v, want %v", got, tt.want)
			}
		})
	}
	if got := nextCheckupAt(ckNow); !got.Equal(sunday4.AddDate(0, 0, 7)) {
		t.Errorf("next = %v", got)
	}
	if got := nextCheckupLabel(sunday4.Add(-time.Hour), sunday4); got != "today at 04:00" {
		t.Errorf("label = %q", got)
	}
}

// ckStore is an in-memory settings store with the repository's NotFound.
type ckStore map[string]string

func (m ckStore) Get(_ context.Context, k string) (string, error) {
	v, ok := m[k]
	if !ok {
		return "", apperror.NewNotFound("setting not found")
	}
	return v, nil
}
func (m ckStore) Set(_ context.Context, k, v string) error { m[k] = v; return nil }

func newCkService(store ckStore, mode string, now *time.Time) *CheckupService {
	s := NewCheckupService(CheckupDeps{
		Store:        store,
		Registration: func(context.Context) (string, error) { return mode, nil },
		EmailReady:   func(context.Context) bool { return true },
	})
	s.now = func() time.Time { return *now }
	return s
}

func saveRecord(t *testing.T, store ckStore, rec *CheckupRecord) {
	t.Helper()
	b, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	store[keyCheckupLast] = string(b)
}

func TestCheckupView_Nagging(t *testing.T) {
	ctx := context.Background()
	now := ckNow
	signIn := ckNow.Add(-time.Minute)

	t.Run("peeks once per sign-in, strip only after a week", func(t *testing.T) {
		store := ckStore{}
		saveRecord(t, store, okRecord())
		s := newCkService(store, "open", &now)
		v := s.View(ctx, "u1", signIn, false)
		if !v.Peek || v.Strip {
			t.Fatalf("first view: peek=%v strip=%v", v.Peek, v.Strip)
		}
		if v = s.View(ctx, "u1", signIn, false); v.Peek {
			t.Fatal("a second page in the same sign-in must not peek again")
		}
		now = ckNow.Add(8 * 24 * time.Hour)
		saveRecord(t, store, &CheckupRecord{At: now.Add(-time.Hour), Backup: &BackupCheckResult{OK: true, BackupAt: now}})
		if v = s.View(ctx, "u1", now.Add(-time.Minute), false); !v.Peek || !v.Strip {
			t.Fatalf("a week on, new sign-in: peek=%v strip=%v", v.Peek, v.Strip)
		}
		now = ckNow
	})

	t.Run("a failed backup puts the strip up at once", func(t *testing.T) {
		store := ckStore{}
		saveRecord(t, store, &CheckupRecord{At: ckNow, Backup: &BackupCheckResult{Problem: "x"}})
		if v := newCkService(store, "invite", &now).View(ctx, "u1", signIn, true); !v.Strip || v.Peek {
			t.Fatalf("strip=%v peek=%v", v.Strip, v.Peek)
		}
	})

	t.Run("remind me tomorrow hides the peek and strip but not the count", func(t *testing.T) {
		store := ckStore{}
		saveRecord(t, store, &CheckupRecord{At: ckNow, Backup: &BackupCheckResult{Problem: "x"}})
		s := newCkService(store, "invite", &now)
		if err := s.Snooze(ctx, "u1"); err != nil {
			t.Fatal(err)
		}
		v := s.View(ctx, "u1", signIn, false)
		if v.Strip || v.Peek || len(v.Open()) == 0 {
			t.Fatalf("snoozed: strip=%v peek=%v open=%d", v.Strip, v.Peek, len(v.Open()))
		}
		if v = s.View(ctx, "u2", signIn, false); !v.Strip {
			t.Fatal("one admin's snooze must not hide the strip from another")
		}
		later := ckNow.Add(25 * time.Hour)
		s.now = func() time.Time { return later }
		if v = s.View(ctx, "u1", signIn, true); !v.Strip {
			t.Fatal("the snooze lasts a day")
		}
	})

	t.Run("all good never nags and clears the week's clock", func(t *testing.T) {
		store := ckStore{keyCheckupOpenSince: ckNow.Add(-30 * 24 * time.Hour).Format(time.RFC3339)}
		saveRecord(t, store, okRecord())
		v := newCkService(store, "invite", &now).View(ctx, "u1", signIn, false)
		if v.Peek || v.Strip || v.Worst() != CheckOK {
			t.Fatalf("peek=%v strip=%v worst=%s", v.Peek, v.Strip, v.Worst())
		}
		if store[keyCheckupOpenSince] != "" {
			t.Fatal("open-since should be cleared once everything passes")
		}
	})
}

// busyBackup is a backup check that finds a backup already running.
type busyBackup struct{}

func (busyBackup) CheckBackup(context.Context) (BackupCheckResult, error) {
	return BackupCheckResult{}, ErrCheckupBusy
}

type okBackup struct{}

func (okBackup) CheckBackup(context.Context) (BackupCheckResult, error) {
	return BackupCheckResult{OK: true, BackupAt: ckNow}, nil
}

func TestCheckupRun(t *testing.T) {
	ctx := context.Background()
	store := ckStore{}
	s := NewCheckupService(CheckupDeps{Store: store, Backup: busyBackup{}})
	if _, err := s.run(ctx); err != ErrCheckupBusy {
		t.Fatalf("err = %v", err)
	}
	if s.LastRecord(ctx) != nil {
		t.Fatal("a busy run must leave the last record alone")
	}

	s = NewCheckupService(CheckupDeps{Store: store, Backup: okBackup{}})
	if _, err := s.run(ctx); err != nil {
		t.Fatal(err)
	}
	if rec := s.LastRecord(ctx); rec == nil || rec.Backup == nil || !rec.Backup.OK {
		t.Fatalf("record = %+v", rec)
	}
	if !s.claim() || s.claim() {
		t.Fatal("only one run at a time")
	}
	s.release()
}
