package admin

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// ErrCheckupBusy is what a CheckupBackup returns when a backup is already
// running; the check-up then tries again on its next tick.
var ErrCheckupBusy = errors.New("a backup is already running")

// CheckupBackup reads the newest backup back to prove it restorable, making
// one first when none is recent. Wired from the backup plugin in internal/app.
type CheckupBackup interface {
	CheckBackup(ctx context.Context) (BackupCheckResult, error)
}

// ChronicleVersionChecker finds out whether a newer Chronicle is published.
type ChronicleVersionChecker interface {
	CheckChronicle(ctx context.Context) VersionCheckResult
}

// PackageUpdateSource refreshes the game systems' release lists and counts
// the updates ready to install. Wired from the packages plugin.
type PackageUpdateSource interface {
	CheckAllForUpdates(ctx context.Context) error
	CountUpdatesReady(ctx context.Context) (int, error)
}

// CheckupStore is the site settings key/value store. Get returns an apperror
// NotFound for a key never set.
type CheckupStore interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
}

// CheckupDeps is what the check-up reads. Registration and EmailReady come
// from the settings and SMTP services the admin handler already holds.
type CheckupDeps struct {
	Store     CheckupStore
	Backup    CheckupBackup
	Chronicle ChronicleVersionChecker // nil: Chronicle's version is not checked
	// ChronicleCheckOn is the admin's switch for asking GitHub about
	// Chronicle's version; nil counts as on.
	ChronicleCheckOn func(ctx context.Context) bool
	Packages         PackageUpdateSource // nil: game systems are not checked
	Backups          BackupLister
	Registration     func(ctx context.Context) (string, error)
	EmailReady       func(ctx context.Context) bool
}

// Site-setting keys. Per-admin keys end in the user's id.
const (
	keyCheckupLast       = "checkup.last"
	keyCheckupOpenSince  = "checkup.open_since"
	keyCheckupSnoozePre  = "checkup.snooze."
	keyCheckupPeekedPre  = "checkup.peeked."
	checkupStripAfter    = 7 * 24 * time.Hour
	checkupSnoozeFor     = 24 * time.Hour
	checkupRunTimeout    = 30 * time.Minute
	checkupSchedulerTick = time.Minute
)

// CheckupView is the check-up for one admin on one page load, with the
// decisions about nagging already made.
type CheckupView struct {
	Checkup
	// Peek opens the sheet by itself for a few seconds: once per sign-in.
	Peek bool
	// Strip puts the reminder strip under the top bar: at once for a failed
	// backup, or once rows have been open for a week.
	Strip bool
}

// CheckupService runs the check-up and decides when it nags. "Remind me
// tomorrow" is the only snooze, and it never hides the shield's count.
type CheckupService struct {
	deps CheckupDeps
	now  func() time.Time

	mu      sync.Mutex
	running bool
}

// NewCheckupService builds the service.
func NewCheckupService(deps CheckupDeps) *CheckupService {
	return &CheckupService{deps: deps, now: time.Now}
}

// View builds the check-up for userID, whose session began at signedInAt.
// A peek is recorded as it is handed out, so it happens once per sign-in
// whichever page it lands on. noPeek is for the shield's own refreshes.
func (s *CheckupService) View(ctx context.Context, userID string, signedInAt time.Time, noPeek bool) CheckupView {
	now := s.now()
	v := CheckupView{Checkup: buildCheckup(s.inputs(ctx, now))}
	worst := v.Worst()
	openSince := s.trackOpenSince(ctx, worst != CheckOK, now)
	if worst == CheckOK || s.snoozed(ctx, userID, now) {
		return v
	}
	v.Strip = worst == CheckBad || (!openSince.IsZero() && now.Sub(openSince) >= checkupStripAfter)
	if !noPeek && userID != "" {
		peeked, _ := s.loadTime(ctx, keyCheckupPeekedPre+userID)
		if peeked.IsZero() || signedInAt.After(peeked) {
			v.Peek = true
			s.saveTime(ctx, keyCheckupPeekedPre+userID, now)
		}
	}
	return v
}

// inputs gathers the live figures and the last run. A figure that can't be
// read leaves its row out or neutral; the check-up never fails a page.
func (s *CheckupService) inputs(ctx context.Context, now time.Time) checkupInputs {
	in := checkupInputs{Now: now, Running: s.Running(), Record: s.LastRecord(ctx), PackageUpdates: -1}
	if in.Record != nil && in.Record.Chronicle != nil && !s.chronicleCheckOn(ctx) {
		// A result from before the check was turned off is no longer kept up.
		rec := *in.Record
		rec.Chronicle = nil
		in.Record = &rec
	}
	if s.deps.Backups != nil {
		if bi, err := s.deps.Backups.BackupInfo(ctx); err == nil && bi.ScheduleKnown {
			in.BackupKnown, in.DailyOn, in.LastRunFailed = true, bi.ScheduleOn, bi.LastRunFailed
		}
	}
	if s.deps.Registration != nil {
		if mode, err := s.deps.Registration(ctx); err == nil {
			in.Registration = mode
		}
	}
	if s.deps.EmailReady != nil {
		in.EmailReady = s.deps.EmailReady(ctx)
	}
	if s.deps.Packages != nil {
		if n, err := s.deps.Packages.CountUpdatesReady(ctx); err == nil {
			in.PackageUpdates = n
		}
	}
	return in
}

func (s *CheckupService) chronicleCheckOn(ctx context.Context) bool {
	return s.deps.ChronicleCheckOn == nil || s.deps.ChronicleCheckOn(ctx)
}

// Running reports whether a full check-up is in progress.
func (s *CheckupService) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// Start begins a full check-up in the background and reports whether it
// started (false: one is already running). The run outlives the request
// that asked for it; ctx should be the server's lifetime context.
func (s *CheckupService) Start(ctx context.Context) bool {
	if !s.claim() {
		return false
	}
	go func() {
		defer s.release()
		if _, err := s.run(ctx); err != nil {
			slog.Warn("site check-up: run did not finish", slog.Any("error", err))
		}
	}()
	return true
}

func (s *CheckupService) claim() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return false
	}
	s.running = true
	return true
}

func (s *CheckupService) release() {
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
}

// run does the slow checks and stores the record. A backup already running
// leaves the record as it was and returns ErrCheckupBusy.
func (s *CheckupService) run(ctx context.Context) (*CheckupRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, checkupRunTimeout)
	defer cancel()
	rec := &CheckupRecord{At: s.now()}
	if s.deps.Backup != nil {
		res, err := s.deps.Backup.CheckBackup(ctx)
		if err != nil {
			return nil, err
		}
		rec.Backup = &res
	}
	if s.deps.Chronicle != nil && s.chronicleCheckOn(ctx) {
		v := s.deps.Chronicle.CheckChronicle(ctx)
		rec.Chronicle = &v
	}
	if s.deps.Packages != nil {
		if err := s.deps.Packages.CheckAllForUpdates(ctx); err != nil {
			slog.Warn("site check-up: could not refresh game-system releases", slog.Any("error", err))
		}
	}
	rec.At = s.now()
	b, err := json.Marshal(rec)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Store.Set(ctx, keyCheckupLast, string(b)); err != nil {
		return nil, err
	}
	if rec.Backup != nil && !rec.Backup.OK {
		slog.Error("site check-up: backup check failed", slog.String("problem", rec.Backup.Problem))
	}
	return rec, nil
}

// RunScheduler starts the weekly check-up when it is due. Call it in its own
// goroutine; it returns when ctx ends.
func (s *CheckupService) RunScheduler(ctx context.Context) {
	t := time.NewTicker(checkupSchedulerTick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if !checkupDue(s.now(), s.LastRecord(ctx)) || !s.claim() {
			continue
		}
		_, err := s.run(ctx)
		s.release()
		if err != nil && !errors.Is(err, ErrCheckupBusy) {
			slog.Warn("site check-up: weekly run did not finish", slog.Any("error", err))
		}
	}
}

// Snooze hides the peek and the strip from userID until this time tomorrow.
func (s *CheckupService) Snooze(ctx context.Context, userID string) error {
	return s.deps.Store.Set(ctx, keyCheckupSnoozePre+userID, s.now().Add(checkupSnoozeFor).UTC().Format(time.RFC3339))
}

func (s *CheckupService) snoozed(ctx context.Context, userID string, now time.Time) bool {
	if userID == "" {
		return false
	}
	until, _ := s.loadTime(ctx, keyCheckupSnoozePre+userID)
	return now.Before(until)
}

// LastRecord returns the stored last run, or nil when there is none.
func (s *CheckupService) LastRecord(ctx context.Context) *CheckupRecord {
	v, ok := s.get(ctx, keyCheckupLast)
	if !ok {
		return nil
	}
	var rec CheckupRecord
	if json.Unmarshal([]byte(v), &rec) != nil || rec.At.IsZero() {
		return nil
	}
	return &rec
}

// trackOpenSince keeps the time rows first went unresolved, cleared once all
// pass, and returns it. The week before the strip counts from then.
func (s *CheckupService) trackOpenSince(ctx context.Context, open bool, now time.Time) time.Time {
	since, set := s.loadTime(ctx, keyCheckupOpenSince)
	switch {
	case open && !set:
		s.saveTime(ctx, keyCheckupOpenSince, now)
		return now
	case !open && set:
		_ = s.deps.Store.Set(ctx, keyCheckupOpenSince, "")
		return time.Time{}
	}
	return since
}

func (s *CheckupService) loadTime(ctx context.Context, key string) (time.Time, bool) {
	v, ok := s.get(ctx, key)
	if !ok || v == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func (s *CheckupService) saveTime(ctx context.Context, key string, t time.Time) {
	if err := s.deps.Store.Set(ctx, key, t.UTC().Format(time.RFC3339)); err != nil {
		slog.Warn("site check-up: could not save", slog.String("key", key), slog.Any("error", err))
	}
}

// get reads one setting; see readSetting.
func (s *CheckupService) get(ctx context.Context, key string) (string, bool) {
	return readSetting(ctx, s.deps.Store, key)
}

// readSetting reads one site setting; unset and unreadable both come back as
// not found, since a notice that can't read its own state should stay quiet.
func readSetting(ctx context.Context, store CheckupStore, key string) (string, bool) {
	if store == nil {
		return "", false
	}
	v, err := store.Get(ctx, key)
	if err != nil {
		var ae *apperror.AppError
		if !errors.As(err, &ae) || ae.Code != http.StatusNotFound {
			slog.Warn("admin setting: could not read", slog.String("key", key), slog.Any("error", err))
		}
		return "", false
	}
	return v, true
}
