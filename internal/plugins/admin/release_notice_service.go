package admin

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// Site-setting keys for the new-version notice. The Later choice is per
// admin, like the check-up's snooze, so one admin's Later never hides the
// news from another.
const (
	keyReleaseLast     = "release_notice.last"
	keyReleaseLaterPre = "release_notice.later."
	// keyUpdateCheck turns every call to GitHub about Chronicle's own
	// version off ("off") or on (anything else, including unset).
	keyUpdateCheck = "updates.chronicle_check"

	releaseCheckEvery   = 24 * time.Hour
	releaseRetryAfter   = 6 * time.Hour
	releaseCheckTimeout = 30 * time.Second
	releaseTick         = 15 * time.Minute
)

// ReleaseNoticeService runs the daily release check and builds the notice.
type ReleaseNoticeService struct {
	store   CheckupStore
	source  ChronicleReleaseSource
	running func() string
	now     func() time.Time
}

// NewReleaseNoticeService builds the service. running reports this server's
// version (hostinfo.Version); source may be nil, which never checks.
func NewReleaseNoticeService(store CheckupStore, source ChronicleReleaseSource, running func() string) *ReleaseNoticeService {
	return &ReleaseNoticeService{store: store, source: source, running: running, now: time.Now}
}

// Notice is what userID sees on the dashboard, or nil. It never calls GitHub
// and never fails a page: anything it can't read means no notice.
func (s *ReleaseNoticeService) Notice(ctx context.Context, userID string) *ReleaseNotice {
	if s == nil || !s.CheckOn(ctx) {
		return nil
	}
	later, _ := readSetting(ctx, s.store, keyReleaseLaterPre+userID)
	return buildReleaseNotice(s.running(), s.last(ctx), later)
}

// Later hides version, and anything older, from userID until a newer
// release is out. Only a plain release number is stored.
func (s *ReleaseNoticeService) Later(ctx context.Context, userID, version string) error {
	if !IsStableVersion(version) {
		return apperror.NewBadRequest("That isn't a Chronicle version number.")
	}
	return s.store.Set(ctx, keyReleaseLaterPre+userID, strings.TrimPrefix(version, "v"))
}

// CheckOn reports whether Chronicle may ask GitHub about its own newer
// versions. It is on unless an admin turned it off.
func (s *ReleaseNoticeService) CheckOn(ctx context.Context) bool {
	v, _ := readSetting(ctx, s.store, keyUpdateCheck)
	return v != "off"
}

// SetCheckOn turns the check on or off. Turning it on checks at the next
// tick, since the last result may be days old.
func (s *ReleaseNoticeService) SetCheckOn(ctx context.Context, on bool) error {
	if on {
		if err := s.store.Set(ctx, keyReleaseLast, ""); err != nil {
			return err
		}
		return s.store.Set(ctx, keyUpdateCheck, "on")
	}
	return s.store.Set(ctx, keyUpdateCheck, "off")
}

// RunScheduler checks once a day. Call it in its own goroutine; it returns
// when ctx ends. The first check waits for the first tick, so a restart loop
// can't turn into a burst of calls to GitHub.
func (s *ReleaseNoticeService) RunScheduler(ctx context.Context) {
	t := time.NewTicker(releaseTick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		s.checkIfDue(ctx)
	}
}

// checkIfDue asks GitHub for the newest release when the last answer is a
// day old (six hours after a failure). A server that doesn't run a release
// never asks: it would have nothing honest to compare against.
func (s *ReleaseNoticeService) checkIfDue(ctx context.Context) {
	if s.source == nil || !IsReleaseVersion(s.running()) || !s.CheckOn(ctx) {
		return
	}
	now := s.now()
	prev := s.last(ctx)
	if prev != nil {
		wait := releaseCheckEvery
		if prev.Failed {
			wait = releaseRetryAfter
		}
		if now.Sub(prev.CheckedAt) < wait {
			return
		}
	}
	cctx, cancel := context.WithTimeout(ctx, releaseCheckTimeout)
	defer cancel()
	rec := releaseRecord{CheckedAt: now}
	rel, err := s.source.LatestChronicleRelease(cctx)
	if err != nil {
		slog.Warn("new-version check: GitHub couldn't be asked", slog.Any("error", err))
		rec.Failed = true
		if prev != nil {
			rec.Release = prev.Release
		}
	} else {
		rec.Release = rel
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return
	}
	if err := s.store.Set(ctx, keyReleaseLast, string(b)); err != nil {
		slog.Warn("new-version check: could not save", slog.Any("error", err))
	}
}

// last returns the stored check, or nil when there is none.
func (s *ReleaseNoticeService) last(ctx context.Context) *releaseRecord {
	v, ok := readSetting(ctx, s.store, keyReleaseLast)
	if !ok || v == "" {
		return nil
	}
	var rec releaseRecord
	if json.Unmarshal([]byte(v), &rec) != nil || rec.CheckedAt.IsZero() {
		return nil
	}
	return &rec
}
