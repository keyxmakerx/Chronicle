package sessions

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

func TestMoveList(t *testing.T) {
	tests := []struct {
		name  string
		remap map[int]int
		want  []monthMove
	}{
		{"nil", nil, []monthMove{}},
		{"identity entries are dropped", map[int]int{2: 2, 3: 3}, []monthMove{}},
		{"non-positive positions are dropped", map[int]int{0: 1, 2: 0, -1: 3}, []monthMove{}},
		{"a swap keeps both halves, sorted by old position", map[int]int{4: 3, 3: 4}, []monthMove{{3, 4}, {4, 3}}},
		{"a shift after an insert", map[int]int{5: 6, 3: 4, 4: 5}, []monthMove{{3, 4}, {4, 5}, {5, 6}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := moveList(tc.remap); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("moveList(%v) = %v, want %v", tc.remap, got, tc.want)
			}
		})
	}
}

func TestRemapMonthPositions_Service(t *testing.T) {
	tests := []struct {
		name      string
		camp, cal string
		remap     map[int]int
		repoErr   error
		wantCalls int
		wantN     int
		wantCode  int
	}{
		{"passes real moves to the repository", "c1", "cal-1", map[int]int{3: 4, 4: 3}, nil, 1, 2, 0},
		{"an all-identity remap never reaches the repository", "c1", "cal-1", map[int]int{1: 1}, nil, 0, 0, 0},
		{"an empty remap is a no-op", "c1", "cal-1", nil, nil, 0, 0, 0},
		{"a missing calendar id is refused, never widened to every calendar", "c1", "", map[int]int{1: 2}, nil, 0, 0, 400},
		{"a missing campaign id is refused", "", "cal-1", map[int]int{1: 2}, nil, 0, 0, 400},
		{"a repository failure is wrapped, never returned raw", "c1", "cal-1", map[int]int{1: 2}, errors.New("db down"), 1, 0, 500},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			svc := NewSessionService(&mockSessionRepo{remapMonthPositionsFn: func(_ context.Context, camp, cal string, _ map[int]int) (int64, error) {
				calls++
				if camp != tc.camp || cal != tc.cal {
					t.Errorf("repo scoped to %q/%q, want %q/%q", camp, cal, tc.camp, tc.cal)
				}
				return 2, tc.repoErr
			}}, nil, nil)
			n, err := svc.RemapMonthPositions(context.Background(), tc.camp, tc.cal, tc.remap)
			if calls != tc.wantCalls {
				t.Errorf("repo calls = %d, want %d", calls, tc.wantCalls)
			}
			if tc.wantCode != 0 {
				var ae *apperror.AppError
				if !errors.As(err, &ae) || ae.Code != tc.wantCode {
					t.Fatalf("err = %v, want apperror code %d", err, tc.wantCode)
				}
				return
			}
			if err != nil || n != tc.wantN {
				t.Errorf("got %d, %v; want %d", n, err, tc.wantN)
			}
		})
	}
}

// The repository builds one CASE statement so a swap is applied from each
// row's OLD month; this pins the SQL shape and argument order without a DB.
func TestBuildRemapQuery(t *testing.T) {
	query, args := buildRemapQuery("camp", "cal", []monthMove{{3, 4}, {4, 3}})
	if !strings.Contains(query, "CASE calendar_month WHEN ? THEN ? WHEN ? THEN ? ELSE calendar_month END") ||
		!strings.Contains(query, "calendar_month IN (?,?)") {
		t.Errorf("query = %s", query)
	}
	want := []any{3, 4, 4, 3, "camp", "cal", 3, 4}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("args = %v, want %v", args, want)
	}
}
