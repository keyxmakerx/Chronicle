package updatehealth

import (
	"context"
	"log/slog"
	"sort"
	"time"
)

// The passing rule the operator signed: a changed feature passes on its own
// after this many uses with no errors, and one still short of that this long
// after the update is brought to a site admin as not used yet.
const (
	PassUses       = 10
	UnusedAfter    = 14 * 24 * time.Hour
	passedShownFor = 30 * 24 * time.Hour
)

// Counts are a feature's stored totals.
type Counts struct {
	FirstSeen    time.Time
	Uses         int
	Errors       int
	LastError    *time.Time
	LastErrorMsg string
}

// Store keeps the counts. EnsureSeen records the first time this server ran
// with each feature and leaves an existing row alone.
type Store interface {
	EnsureSeen(ctx context.Context, ids []string, now time.Time) error
	Add(ctx context.Context, id string, uses, errors int, lastError *time.Time, lastErrorMsg string) error
	All(ctx context.Context) (map[string]Counts, error)
}

// State is where a changed feature stands.
type State string

const (
	// Failing: it has answered with a server error since the update.
	Failing State = "failing"
	// Unused: it is past UnusedAfter with fewer than PassUses uses.
	Unused State = "unused"
	// Counting: it is still collecting uses; nobody needs to look yet.
	Counting State = "counting"
	// Passed: PassUses uses and no errors.
	Passed State = "passed"
)

// Status is one changed feature's standing, as Site admin Home shows it.
type Status struct {
	Feature
	State        State
	FirstSeen    time.Time
	Uses         int
	Errors       int
	LastError    *time.Time
	LastErrorMsg string
}

// Classify applies the passing rule. An error always outranks the use count:
// ten good uses do not hide one failure.
func Classify(c Counts, now time.Time) State {
	switch {
	case c.Errors > 0:
		return Failing
	case c.Uses >= PassUses:
		return Passed
	case !c.FirstSeen.IsZero() && now.Sub(c.FirstSeen) >= UnusedAfter:
		return Unused
	default:
		return Counting
	}
}

// Service reads the standing of every changed feature and moves the
// recorder's counts into the store.
type Service struct {
	rec   *Recorder
	store Store
	now   func() time.Time
}

// NewService builds the service.
func NewService(rec *Recorder, store Store) *Service {
	return &Service{rec: rec, store: store, now: time.Now}
}

// Start records when this server first ran with each feature.
func (s *Service) Start(ctx context.Context) error {
	ids := make([]string, 0, len(s.rec.Features()))
	for _, f := range s.rec.Features() {
		ids = append(ids, f.ID)
	}
	if len(ids) == 0 {
		return nil
	}
	return s.store.EnsureSeen(ctx, ids, s.now())
}

// Flush stores the counts gathered since the last flush. Counts it cannot
// store are kept for the next try.
func (s *Service) Flush(ctx context.Context) error {
	pending := s.rec.take()
	failed := map[string]*delta{}
	var firstErr error
	for id, d := range pending {
		var at *time.Time
		if !d.lastError.IsZero() {
			t := d.lastError
			at = &t
		}
		if err := s.store.Add(ctx, id, d.uses, d.errors, at, d.lastErrorMsg); err != nil {
			failed[id] = d
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	if len(failed) > 0 {
		s.rec.giveBack(failed)
	}
	return firstErr
}

// flushEvery is how often counts reach the database: often enough that a
// restart loses little, rarely enough to cost nothing.
const flushEvery = time.Minute

// Run records first sightings, then flushes on a timer until ctx ends, with
// one last flush on the way out.
func (s *Service) Run(ctx context.Context) {
	if err := s.Start(ctx); err != nil {
		slog.Warn("update health: noting changed features failed", slog.Any("error", err))
	}
	t := time.NewTicker(flushEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			done, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = s.Flush(done)
			cancel()
			return
		case <-t.C:
			if err := s.Flush(ctx); err != nil {
				slog.Warn("update health: storing counts failed", slog.Any("error", err))
			}
		}
	}
}

// Report returns every changed feature's standing, most urgent first. A
// feature that passed more than a month after the update drops out; one that
// fails never does.
func (s *Service) Report(ctx context.Context) ([]Status, error) {
	stored, err := s.store.All(ctx)
	if err != nil {
		return nil, err
	}
	return buildReport(s.rec.Features(), stored, s.rec.peek(), s.now()), nil
}

// buildReport is Report without the store, so the rules can be tested.
func buildReport(features []Feature, stored map[string]Counts, pending map[string]delta, now time.Time) []Status {
	out := make([]Status, 0, len(features))
	for _, f := range features {
		c, ok := stored[f.ID]
		if !ok {
			// Not yet noted (the first flush has not run): count from now.
			c.FirstSeen = now
		}
		if d, ok := pending[f.ID]; ok {
			c.Uses += d.uses
			c.Errors += d.errors
			if !d.lastError.IsZero() && (c.LastError == nil || d.lastError.After(*c.LastError)) {
				t := d.lastError
				c.LastError, c.LastErrorMsg = &t, d.lastErrorMsg
			}
		}
		st := Classify(c, now)
		if st == Passed && now.Sub(c.FirstSeen) > passedShownFor {
			continue
		}
		out = append(out, Status{
			Feature: f, State: st, FirstSeen: c.FirstSeen,
			Uses: c.Uses, Errors: c.Errors, LastError: c.LastError, LastErrorMsg: c.LastErrorMsg,
		})
	}
	rank := map[State]int{Failing: 0, Unused: 1, Counting: 2, Passed: 3}
	sort.SliceStable(out, func(i, j int) bool { return rank[out[i].State] < rank[out[j].State] })
	return out
}
