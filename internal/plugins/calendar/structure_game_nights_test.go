package calendar

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type recordingRemapper struct {
	calls   int
	gotCamp string
	gotCal  string
	gotMap  map[int]int
	err     error
}

func (r *recordingRemapper) RemapMonthPositions(_ context.Context, campaignID, calendarID string, remap map[int]int) error {
	r.calls++
	r.gotCamp, r.gotCal, r.gotMap = campaignID, calendarID, remap
	return r.err
}

// A structure save hands the sessions side exactly the plan's old -> new
// positions, once, after the write; it is skipped when nothing moved or the
// save conflicted, and a remapper failure never fails the save.
func TestApplyStructureEdit_MovesGameNightsWithMonths(t *testing.T) {
	ctx := context.Background()
	reorder := func(cal *Calendar) StructureEdit {
		edit := editFrom(cal)
		edit.Months = []MonthInput{edit.Months[2], edit.Months[0]} // Gamma first, Beta gone
		return edit
	}
	tests := []struct {
		name      string
		edit      func(*Calendar) StructureEdit
		stale     bool
		remapErr  error
		unwired   bool
		wantCalls int
		wantMap   map[int]int
		wantErr   bool
	}{
		{"reordered months are passed through", reorder, false, nil, false, 1, map[int]int{1: 2, 3: 1}, false},
		{"no month moved means no call", editFrom, false, nil, false, 0, nil, false},
		{"a stale fingerprint writes nothing and moves nothing", reorder, true, nil, false, 0, nil, true},
		{"a remapper failure does not fail the saved structure", reorder, false, errors.New("sessions down"), false, 1, map[int]int{1: 2, 3: 1}, false},
		{"unwired is a plain save", reorder, false, nil, true, 0, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cal := structureFixture()
			var wrote StructureWrite
			calls := 0
			svc := structureService(cal, nil, &wrote, &calls)
			rem := &recordingRemapper{err: tt.remapErr}
			if !tt.unwired {
				svc.SetSessionMonthRemapper(rem)
			}
			edit := tt.edit(cal)
			preview, err := svc.PreviewStructureEdit(ctx, cal.ID, cal.CampaignID, edit)
			if err != nil {
				t.Fatalf("PreviewStructureEdit: %v", err)
			}
			fp := preview.Fingerprint
			if tt.stale {
				fp = "stale"
			}
			_, err = svc.ApplyStructureEdit(ctx, cal.ID, cal.CampaignID, fp, edit)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if rem.calls != tt.wantCalls {
				t.Fatalf("remapper called %d times, want %d", rem.calls, tt.wantCalls)
			}
			if tt.wantCalls > 0 {
				if !reflect.DeepEqual(rem.gotMap, tt.wantMap) || rem.gotCal != cal.ID || rem.gotCamp != cal.CampaignID {
					t.Errorf("remapper got campaign %q calendar %q map %v; want %q %q %v",
						rem.gotCamp, rem.gotCal, rem.gotMap, cal.CampaignID, cal.ID, tt.wantMap)
				}
			}
		})
	}
}
