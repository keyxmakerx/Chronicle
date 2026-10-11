package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// Site-setting keys for the daily backup. Values are plain strings so the
// generic site_settings table holds them without a migration.
const (
	keyScheduleEnabled = "backup.schedule.enabled"
	keyScheduleHour    = "backup.schedule.hour"
	keyScheduleKeep    = "backup.schedule.keep_days"
	keyScheduleLast    = "backup.schedule.last"
)

// Defaults and bounds for the daily backup. Keep is in days because the
// script prunes by the date in each file name; with one run a day that is
// the same as "the last N backups".
const (
	DefaultScheduleHour = 3
	DefaultKeepDays     = 7
	MinKeepDays         = 1
	MaxKeepDays         = 90
)

// Schedule is the owner's daily backup choice.
type Schedule struct {
	Enabled  bool
	Hour     int // 0-23, server time
	KeepDays int
}

// Validate rejects values the form can't produce, so a crafted POST can't
// store an hour that never comes round or a retention that prunes everything.
func (s Schedule) Validate() error {
	if s.Hour < 0 || s.Hour > 23 {
		return apperror.NewValidation("Pick an hour between 0 and 23.")
	}
	if s.KeepDays < MinKeepDays || s.KeepDays > MaxKeepDays {
		return apperror.NewValidation(fmt.Sprintf("Keep between %d and %d days of backups.", MinKeepDays, MaxKeepDays))
	}
	return nil
}

// ScheduledRun is the outcome of the last scheduled backup. It is stored so
// the Backup page can show it after a restart, and so a restart inside the
// chosen hour doesn't run the same day's backup twice.
type ScheduledRun struct {
	At     time.Time `json:"at"`
	Day    string    `json:"day"` // server-local YYYY-MM-DD the run counted for
	OK     bool      `json:"ok"`
	Detail string    `json:"detail,omitempty"`
}

// SettingsStore is the slice of the site settings repository this package
// uses. Get returns an apperror NotFound for a key never set.
type SettingsStore interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
}

// FailureNotifier tells site admins a scheduled backup failed. Optional.
type FailureNotifier interface {
	BackupFailed(ctx context.Context, run ScheduledRun) error
}

// LoadSchedule reads the schedule, falling back to defaults (off) for keys
// never saved or holding values out of range.
func LoadSchedule(ctx context.Context, st SettingsStore) (Schedule, error) {
	s := Schedule{Hour: DefaultScheduleHour, KeepDays: DefaultKeepDays}
	v, ok, err := getSetting(ctx, st, keyScheduleEnabled)
	if err != nil {
		return s, err
	}
	s.Enabled = ok && v == "true"
	if v, ok, err = getSetting(ctx, st, keyScheduleHour); err != nil {
		return s, err
	} else if n, convErr := strconv.Atoi(v); ok && convErr == nil && n >= 0 && n <= 23 {
		s.Hour = n
	}
	if v, ok, err = getSetting(ctx, st, keyScheduleKeep); err != nil {
		return s, err
	} else if n, convErr := strconv.Atoi(v); ok && convErr == nil && n >= MinKeepDays && n <= MaxKeepDays {
		s.KeepDays = n
	}
	return s, nil
}

// SaveSchedule validates and stores the schedule.
func SaveSchedule(ctx context.Context, st SettingsStore, s Schedule) error {
	if err := s.Validate(); err != nil {
		return err
	}
	for k, v := range map[string]string{
		keyScheduleEnabled: strconv.FormatBool(s.Enabled),
		keyScheduleHour:    strconv.Itoa(s.Hour),
		keyScheduleKeep:    strconv.Itoa(s.KeepDays),
	} {
		if err := st.Set(ctx, k, v); err != nil {
			return err
		}
	}
	return nil
}

// LoadLastScheduledRun returns the stored last scheduled run, or nil.
func LoadLastScheduledRun(ctx context.Context, st SettingsStore) (*ScheduledRun, error) {
	v, ok, err := getSetting(ctx, st, keyScheduleLast)
	if err != nil || !ok || v == "" {
		return nil, err
	}
	var r ScheduledRun
	if json.Unmarshal([]byte(v), &r) != nil {
		return nil, nil
	}
	return &r, nil
}

func getSetting(ctx context.Context, st SettingsStore, key string) (string, bool, error) {
	v, err := st.Get(ctx, key)
	if err != nil {
		var ae *apperror.AppError
		if errors.As(err, &ae) && ae.Code == http.StatusNotFound {
			return "", false, nil
		}
		return "", false, err
	}
	return v, true, nil
}

// due reports whether a scheduled backup should start now: the schedule is
// on, the clock is at or past the chosen hour, and today's run hasn't been
// recorded. Starting any time after the hour (not only inside it) means a
// server that was down at 03:00 still backs up when it comes back that day.
func due(now time.Time, s Schedule, last *ScheduledRun) bool {
	if !s.Enabled || now.Hour() < s.Hour {
		return false
	}
	return last == nil || last.Day != now.Format("2006-01-02")
}

// Scheduler runs the daily backup from inside the server process.
type Scheduler struct {
	svc      Service
	store    SettingsStore
	notifier FailureNotifier
	now      func() time.Time
	tick     time.Duration
}

// NewScheduler builds a Scheduler. notifier may be nil.
func NewScheduler(svc Service, store SettingsStore, notifier FailureNotifier) *Scheduler {
	return &Scheduler{svc: svc, store: store, notifier: notifier, now: time.Now, tick: time.Minute}
}

// Run checks once a minute until ctx ends. Call it in its own goroutine.
func (s *Scheduler) Run(ctx context.Context) {
	t := time.NewTicker(s.tick)
	defer t.Stop()
	for {
		s.check(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// check starts today's backup if it is due. A backup already running (an
// admin's click) is not today's run: the next tick tries again.
func (s *Scheduler) check(ctx context.Context) {
	sched, err := LoadSchedule(ctx, s.store)
	if err != nil {
		slog.Warn("backup schedule: could not read settings", slog.Any("error", err))
		return
	}
	last, err := LoadLastScheduledRun(ctx, s.store)
	if err != nil {
		slog.Warn("backup schedule: could not read last run", slog.Any("error", err))
		return
	}
	now := s.now()
	if !due(now, sched, last) {
		return
	}

	res, err := s.svc.RunBackupKeeping(ctx, sched.KeepDays)
	if errors.Is(err, ErrAlreadyRunning) || ctx.Err() != nil {
		return
	}
	run := ScheduledRun{At: s.now(), Day: now.Format("2006-01-02")}
	switch {
	case err != nil:
		run.Detail = err.Error()
	case res == nil:
		run.Detail = "no result"
	case res.Succeeded():
		run.OK = true
	default:
		run.Detail = failureDetail(res)
	}
	if b, mErr := json.Marshal(run); mErr == nil {
		if sErr := s.store.Set(ctx, keyScheduleLast, string(b)); sErr != nil {
			slog.Error("backup schedule: could not record run", slog.Any("error", sErr))
		}
	}
	if run.OK {
		slog.Info("backup schedule: daily backup finished")
		return
	}
	slog.Error("backup schedule: daily backup failed", slog.String("detail", run.Detail))
	if s.notifier != nil {
		if nErr := s.notifier.BackupFailed(ctx, run); nErr != nil {
			slog.Warn("backup schedule: could not notify admins", slog.Any("error", nErr))
		}
	}
}

// failureDetail is a one-line reason for the page and the admin email; the
// full script output stays in the server log and on the run card.
func failureDetail(r *RunResult) string {
	if r.TimedOut || r.ErrorString != "" {
		return r.ErrorString
	}
	return fmt.Sprintf("the backup script stopped with exit code %d", r.ExitCode)
}
