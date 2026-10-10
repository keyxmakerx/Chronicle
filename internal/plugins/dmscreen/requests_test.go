package dmscreen

import (
	"context"
	"testing"
	"time"
)

type fakeRequests struct {
	reqs []Request
	ok   bool
}

func (f fakeRequests) WaitingRequests(context.Context, string, Viewer) ([]Request, bool, error) {
	return f.reqs, f.ok, nil
}

func reqAt(id int64, kind string, minute int) Request {
	return Request{Kind: kind, ID: id, Text: "r", CreatedAt: time.Date(2026, 10, 10, 19, minute, 0, 0, time.UTC)}
}

func TestTrimRequests(t *testing.T) {
	seven := []Request{
		reqAt(7, RequestMove, 7), reqAt(1, RequestPurchase, 1), reqAt(3, RequestMove, 3), reqAt(2, RequestMove, 2),
		reqAt(6, RequestPurchase, 6), reqAt(4, RequestMove, 4), reqAt(5, RequestMove, 5),
	}
	tests := []struct {
		name      string
		in        []Request
		canAnswer bool
		wantNil   bool
		wantIDs   []int64
		wantMore  int
	}{
		{"nothing waiting leaves the block out", nil, true, true, nil, 0},
		{"fewer than five all shown", seven[:2], true, false, []int64{1, 7}, 0},
		{"exactly five, no more line", []Request{reqAt(5, RequestMove, 5), reqAt(1, RequestMove, 1), reqAt(2, RequestMove, 2), reqAt(3, RequestMove, 3), reqAt(4, RequestMove, 4)}, true, false, []int64{1, 2, 3, 4, 5}, 0},
		{"seven keeps the five oldest, mixed kinds", seven, true, false, []int64{1, 2, 3, 4, 5}, 2},
		{"scribe sees rows without answering", seven[:2], false, false, []int64{1, 7}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trimRequests(tt.in, tt.canAnswer)
			if (got == nil) != tt.wantNil {
				t.Fatalf("got %+v, wantNil %v", got, tt.wantNil)
			}
			if got == nil {
				return
			}
			var ids []int64
			for _, r := range got.Items {
				ids = append(ids, r.ID)
			}
			if len(ids) != len(tt.wantIDs) {
				t.Fatalf("ids = %v, want %v", ids, tt.wantIDs)
			}
			for i := range ids {
				if ids[i] != tt.wantIDs[i] {
					t.Fatalf("ids = %v, want %v", ids, tt.wantIDs)
				}
			}
			if got.More != tt.wantMore || got.CanAnswer != tt.canAnswer {
				t.Errorf("more %d canAnswer %v", got.More, got.CanAnswer)
			}
		})
	}
	// The input slice is not reordered.
	if seven[0].ID != 7 {
		t.Error("trimRequests mutated its input")
	}
}

func TestRequestPaths(t *testing.T) {
	tests := []struct {
		r               RequestView
		approve, refuse string
	}{
		{RequestView{Kind: RequestMove, ID: 4}, "/campaigns/c1/armory/moves/4/approve", "/campaigns/c1/armory/moves/4/decline"},
		{RequestView{Kind: RequestPurchase, ID: 9}, "/campaigns/c1/armory/purchase-requests/9/approve", "/campaigns/c1/armory/purchase-requests/9/decline"},
	}
	for _, tt := range tests {
		if got := tt.r.ApprovePath("c1"); got != tt.approve {
			t.Errorf("approve = %q", got)
		}
		if got := tt.r.DeclinePath("c1"); got != tt.refuse {
			t.Errorf("decline = %q", got)
		}
	}
}

func TestBuild_RequestsSource(t *testing.T) {
	src := Sources{Requests: fakeRequests{ok: true, reqs: []Request{reqAt(1, RequestMove, 1)}}}
	v, err := NewService(src).Build(context.Background(), "c1", Viewer{Role: 3})
	if err != nil || v.Requests == nil || !v.Requests.CanAnswer {
		t.Fatalf("owner: %v %+v", err, v.Requests)
	}
	v, _ = NewService(src).Build(context.Background(), "c1", Viewer{Role: 2})
	if v.Requests == nil || v.Requests.CanAnswer {
		t.Fatalf("scribe: %+v", v.Requests)
	}
	src.Requests = fakeRequests{ok: false}
	if v, _ = NewService(src).Build(context.Background(), "c1", Viewer{Role: 3}); v.Requests != nil {
		t.Fatalf("no armory: %+v", v.Requests)
	}
}
