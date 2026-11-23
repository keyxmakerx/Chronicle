package app

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// fakeClimateEnsurer adds "climate" to every category that lacks it, as
// EnsureClimateFields does.
type fakeClimateEnsurer struct {
	has  map[int]bool
	runs int
	err  error
}

func (f *fakeClimateEnsurer) EnsureClimateFields(context.Context) (int, error) {
	f.runs++
	if f.err != nil {
		return 0, f.err
	}
	n := 0
	for id, ok := range f.has {
		if !ok {
			f.has[id] = true
			n++
		}
	}
	return n, nil
}

type failingSettings struct{}

func (failingSettings) Set(context.Context, string, string) error { return nil }

func (failingSettings) Get(context.Context, string) (string, error) {
	return "", apperror.NewInternal(errors.New("db down"))
}

func TestAddClimateFieldsOnce(t *testing.T) {
	tests := []struct {
		name      string
		settings  pageExtrasSettings
		ensureErr error
		wantN     int
		wantErr   bool
		wantRuns  int
		recorded  bool
	}{
		{name: "first boot adds and records", settings: &fakeExtrasSettings{vals: map[string]string{}}, wantN: 2, wantRuns: 1, recorded: true},
		{name: "already ran", settings: &fakeExtrasSettings{vals: map[string]string{climateFieldsKey: "1"}}, wantN: 0, wantRuns: 0, recorded: true},
		{name: "sweep fails, not recorded so it repeats", settings: &fakeExtrasSettings{vals: map[string]string{}}, ensureErr: errors.New("boom"), wantErr: true, wantRuns: 1},
		{name: "settings unreadable, sweep skipped", settings: failingSettings{}, wantErr: true, wantRuns: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ents := &fakeClimateEnsurer{has: map[int]bool{1: false, 2: false, 3: true}, err: tt.ensureErr}
			n, err := addClimateFieldsOnce(context.Background(), tt.settings, ents)
			if (err != nil) != tt.wantErr || n != tt.wantN || ents.runs != tt.wantRuns {
				t.Fatalf("n=%d err=%v runs=%d; want n=%d err=%v runs=%d", n, err, ents.runs, tt.wantN, tt.wantErr, tt.wantRuns)
			}
			if fs, ok := tt.settings.(*fakeExtrasSettings); ok && (fs.vals[climateFieldsKey] == "1") != tt.recorded {
				t.Errorf("recorded = %v, want %v", fs.vals[climateFieldsKey] == "1", tt.recorded)
			}
		})
	}
}

// An owner who deletes the Climate field after the first boot keeps it
// deleted on the next.
func TestAddClimateFieldsOnceKeepsADeletedField(t *testing.T) {
	st := &fakeExtrasSettings{vals: map[string]string{}}
	ents := &fakeClimateEnsurer{has: map[int]bool{1: false}}
	if n, err := addClimateFieldsOnce(context.Background(), st, ents); err != nil || n != 1 {
		t.Fatalf("first boot: n=%d err=%v", n, err)
	}
	ents.has[1] = false // the owner removes the field
	if n, err := addClimateFieldsOnce(context.Background(), st, ents); err != nil || n != 0 {
		t.Fatalf("second boot: n=%d err=%v", n, err)
	}
	if ents.has[1] {
		t.Error("the deleted Climate field came back")
	}
}
