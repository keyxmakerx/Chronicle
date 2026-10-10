package backup

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// memStore is an in-memory SettingsStore with the repository's NotFound.
type memStore struct {
	vals   map[string]string
	getErr error
}

func newMemStore(kv ...string) *memStore {
	m := &memStore{vals: map[string]string{}}
	for i := 0; i+1 < len(kv); i += 2 {
		m.vals[kv[i]] = kv[i+1]
	}
	return m
}

func (m *memStore) Get(_ context.Context, key string) (string, error) {
	if m.getErr != nil {
		return "", m.getErr
	}
	v, ok := m.vals[key]
	if !ok {
		return "", apperror.NewNotFound("setting not found")
	}
	return v, nil
}

func (m *memStore) Set(_ context.Context, key, value string) error {
	m.vals[key] = value
	return nil
}

func TestLoadSchedule(t *testing.T) {
	tests := []struct {
		name string
		kv   []string
		want Schedule
	}{
		{"never saved is off with defaults", nil, Schedule{Hour: DefaultScheduleHour, KeepDays: DefaultKeepDays}},
		{"saved values", []string{keyScheduleEnabled, "true", keyScheduleHour, "22", keyScheduleKeep, "14"}, Schedule{Enabled: true, Hour: 22, KeepDays: 14}},
		{"out of range falls back", []string{keyScheduleEnabled, "true", keyScheduleHour, "24", keyScheduleKeep, "0"}, Schedule{Enabled: true, Hour: DefaultScheduleHour, KeepDays: DefaultKeepDays}},
		{"garbage falls back", []string{keyScheduleEnabled, "yes", keyScheduleHour, "x", keyScheduleKeep, "-"}, Schedule{Hour: DefaultScheduleHour, KeepDays: DefaultKeepDays}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LoadSchedule(context.Background(), newMemStore(tt.kv...))
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestLoadSchedule_StoreErrorSurfaces(t *testing.T) {
	st := &memStore{getErr: errors.New("db down")}
	if _, err := LoadSchedule(context.Background(), st); err == nil {
		t.Fatal("want error when the store fails, got nil")
	}
}

func TestSaveSchedule_Validates(t *testing.T) {
	tests := []struct {
		name    string
		s       Schedule
		wantErr bool
	}{
		{"ok", Schedule{Enabled: true, Hour: 3, KeepDays: 7}, false},
		{"hour too big", Schedule{Hour: 24, KeepDays: 7}, true},
		{"hour negative", Schedule{Hour: -1, KeepDays: 7}, true},
		{"keep zero", Schedule{Hour: 3, KeepDays: 0}, true},
		{"keep too long", Schedule{Hour: 3, KeepDays: MaxKeepDays + 1}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := newMemStore()
			err := SaveSchedule(context.Background(), st, tt.s)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && len(st.vals) != 0 {
				t.Errorf("invalid schedule was stored: %v", st.vals)
			}
			if !tt.wantErr {
				got, _ := LoadSchedule(context.Background(), st)
				if got != tt.s {
					t.Errorf("round trip got %+v, want %+v", got, tt.s)
				}
			}
		})
	}
}

func TestDue(t *testing.T) {
	at := func(h int) time.Time { return time.Date(2026, 10, 10, h, 30, 0, 0, time.UTC) }
	on := Schedule{Enabled: true, Hour: 3, KeepDays: 7}
	today := &ScheduledRun{Day: "2026-10-10"}
	yesterday := &ScheduledRun{Day: "2026-10-09"}
	tests := []struct {
		name string
		now  time.Time
		s    Schedule
		last *ScheduledRun
		want bool
	}{
		{"off", at(4), Schedule{Hour: 3, KeepDays: 7}, nil, false},
		{"before the hour", at(2), on, yesterday, false},
		{"at the hour, never run", at(3), on, nil, true},
		{"after the hour, ran yesterday", at(15), on, yesterday, true},
		{"already ran today", at(15), on, today, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := due(tt.now, tt.s, tt.last); got != tt.want {
				t.Errorf("due = %v, want %v", got, tt.want)
			}
		})
	}
}

// fakeNotifier records failure notices.
type fakeNotifier struct{ runs []ScheduledRun }

func (f *fakeNotifier) BackupFailed(_ context.Context, r ScheduledRun) error {
	f.runs = append(f.runs, r)
	return nil
}

func TestSchedulerCheck(t *testing.T) {
	now := time.Date(2026, 10, 10, 4, 0, 0, 0, time.UTC)
	enabled := []string{keyScheduleEnabled, "true", keyScheduleHour, "3", keyScheduleKeep, "10"}
	tests := []struct {
		name       string
		kv         []string
		run        func(context.Context) (*RunResult, error)
		wantCalls  int
		wantStored bool
		wantOK     bool
		wantNotice int
	}{
		{
			name:      "off does nothing",
			run:       func(context.Context) (*RunResult, error) { return &RunResult{}, nil },
			wantCalls: 0,
		},
		{
			name:       "success is recorded",
			kv:         enabled,
			run:        func(context.Context) (*RunResult, error) { return &RunResult{}, nil },
			wantCalls:  1,
			wantStored: true,
			wantOK:     true,
		},
		{
			name:       "failure is recorded and admins are told",
			kv:         enabled,
			run:        func(context.Context) (*RunResult, error) { return &RunResult{ExitCode: 3}, nil },
			wantCalls:  1,
			wantStored: true,
			wantNotice: 1,
		},
		{
			name:      "an admin's run in progress is retried later, not recorded",
			kv:        enabled,
			run:       func(context.Context) (*RunResult, error) { return nil, ErrAlreadyRunning },
			wantCalls: 1,
		},
		{
			name:      "already ran today",
			kv:        append(append([]string{}, enabled...), keyScheduleLast, `{"day":"2026-10-10","ok":true}`),
			run:       func(context.Context) (*RunResult, error) { return &RunResult{}, nil },
			wantCalls: 0,
			// The stored row is the pre-existing one; nothing new written.
			wantStored: true,
			wantOK:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls, keep := 0, 0
			svc := &stubService{runFn: func(ctx context.Context) (*RunResult, error) { calls++; return tt.run(ctx) }}
			st := newMemStore(tt.kv...)
			n := &fakeNotifier{}
			s := NewScheduler(keepRecorder{svc, &keep}, st, n)
			s.now = func() time.Time { return now }
			s.check(context.Background())

			if calls != tt.wantCalls {
				t.Errorf("backup ran %d times, want %d", calls, tt.wantCalls)
			}
			if calls > 0 && keep != 10 {
				t.Errorf("keep days passed = %d, want 10", keep)
			}
			raw, stored := st.vals[keyScheduleLast]
			if stored != tt.wantStored {
				t.Fatalf("last run stored = %v, want %v", stored, tt.wantStored)
			}
			if stored {
				var r ScheduledRun
				if err := json.Unmarshal([]byte(raw), &r); err != nil {
					t.Fatal(err)
				}
				if r.OK != tt.wantOK || r.Day != "2026-10-10" {
					t.Errorf("stored run %+v, want ok=%v day 2026-10-10", r, tt.wantOK)
				}
			}
			if len(n.runs) != tt.wantNotice {
				t.Errorf("notices = %d, want %d", len(n.runs), tt.wantNotice)
			}
		})
	}
}

// keepRecorder wraps a stub and records the keep-days the scheduler passed.
type keepRecorder struct {
	*stubService
	keep *int
}

func (k keepRecorder) RunBackupKeeping(ctx context.Context, days int) (*RunResult, error) {
	*k.keep = days
	return k.RunBackup(ctx)
}

func TestSaveScheduleHandler(t *testing.T) {
	tests := []struct {
		name     string
		form     url.Values
		wantCode int
		wantSet  bool
	}{
		{"saves", url.Values{"enabled": {"on"}, "hour": {"2"}, "keep_days": {"14"}}, http.StatusSeeOther, true},
		{"turning off keeps hour and days", url.Values{"hour": {"2"}, "keep_days": {"14"}}, http.StatusSeeOther, true},
		{"bad hour", url.Values{"enabled": {"on"}, "hour": {"25"}, "keep_days": {"14"}}, http.StatusUnprocessableEntity, false},
		{"missing days", url.Values{"enabled": {"on"}, "hour": {"2"}}, http.StatusBadRequest, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(&stubService{dir: t.TempDir()})
			st := newMemStore()
			h.SetScheduleStore(st)
			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, "/admin/backup/schedule", strings.NewReader(tt.form.Encode()))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
			rec := httptest.NewRecorder()
			err := h.SaveSchedule(e.NewContext(req, rec))
			code := rec.Code
			var he *echo.HTTPError
			if errors.As(err, &he) {
				code = he.Code
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if code != tt.wantCode {
				t.Fatalf("code = %d, want %d", code, tt.wantCode)
			}
			if _, set := st.vals[keyScheduleHour]; set != tt.wantSet {
				t.Errorf("stored = %v, want %v", set, tt.wantSet)
			}
		})
	}
}
