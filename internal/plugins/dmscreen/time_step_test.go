package dmscreen

import (
	"context"
	"testing"
)

type fakeWorld struct {
	canStep     bool
	hours, days int
	advanced    bool
}

func (f *fakeWorld) World(context.Context, string, Viewer) (*WorldView, error) {
	return &WorldView{CalendarID: "cal", DateLabel: "1 Frost", CanStep: f.canStep}, nil
}

func (f *fakeWorld) Advance(_ context.Context, _ string, _ Viewer, hours, days int) error {
	f.advanced, f.hours, f.days = true, hours, days
	return nil
}

func TestStepTime(t *testing.T) {
	tests := []struct {
		name        string
		role        int
		key         string
		wantErr     bool
		hours, days int
	}{
		{"plus one hour", 3, "1h", false, 1, 0},
		{"plus eight hours", 3, "8h", false, 8, 0},
		{"next day is one calendar day, not 24 hours", 3, "1d", false, 0, 1},
		{"scribe refused", 2, "1h", true, 0, 0},
		{"unknown step refused", 3, "999h", true, 0, 0},
		{"empty step refused", 3, "", true, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &fakeWorld{canStep: true}
			err := NewService(Sources{World: w}).StepTime(context.Background(), "c1", Viewer{UserID: "u", Role: tt.role}, tt.key)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v", err)
			}
			if tt.wantErr {
				if w.advanced {
					t.Error("a refused step moved the calendar")
				}
				return
			}
			if w.hours != tt.hours || w.days != tt.days {
				t.Errorf("advanced %dh %dd, want %dh %dd", w.hours, w.days, tt.hours, tt.days)
			}
		})
	}
}

func TestBuild_CanStep(t *testing.T) {
	tests := []struct {
		name     string
		role     int
		calendar bool
		want     bool
	}{
		{"owner on a world calendar", 3, true, true},
		{"owner on a real-time calendar hides the chips", 3, false, false},
		{"scribe never sees them", 2, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := NewService(Sources{World: &fakeWorld{canStep: tt.calendar}}).Build(context.Background(), "c1", Viewer{Role: tt.role})
			if err != nil || v.World == nil {
				t.Fatalf("err %v world %+v", err, v.World)
			}
			if v.World.CanStep != tt.want {
				t.Errorf("CanStep = %v, want %v", v.World.CanStep, tt.want)
			}
		})
	}
}
