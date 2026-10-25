package connector

import (
	"testing"
	"time"
)

// Calls split into sittings at every quiet gap longer than SittingGap, and
// each sitting counts what kind of calls it holds.
func TestSittings(t *testing.T) {
	t0 := time.Date(2026, 10, 24, 14, 0, 0, 0, time.UTC)
	at := func(min int, tool string, ok bool) Call {
		return Call{At: t0.Add(time.Duration(min) * time.Minute), Tool: tool, OK: ok}
	}
	calls := []Call{ // newest first
		at(200, toolAsk, true),
		at(190, "read_page", true),
		at(185, "read_page", false),
		at(160, "campaign_outline", true), // 25 quiet minutes: same sitting
		at(100, "list_tags", true),        // 60 quiet minutes: a new sitting
		at(70, "players", true),           // exactly SittingGap: same sitting
		at(69, toolConnect, true),         // counted as nothing
	}
	got := Sittings(calls)
	if len(got) != 2 {
		t.Fatalf("got %d sittings, want 2: %+v", len(got), got)
	}
	a, b := got[0], got[1]
	if !a.Start.Equal(calls[3].At) || !a.End.Equal(calls[0].At) || len(a.Calls) != 4 {
		t.Errorf("first sitting %v to %v with %d calls", a.Start, a.End, len(a.Calls))
	}
	if a.Asks != 1 || a.Reads != 1 || a.Refused != 1 || a.Lookups != 1 {
		t.Errorf("first sitting counts %+v", a)
	}
	if !b.Start.Equal(calls[6].At) || len(b.Calls) != 3 || b.Lookups != 2 || b.Refused+b.Asks+b.Reads != 0 {
		t.Errorf("second sitting %+v", b)
	}
	if Sittings(nil) != nil {
		t.Error("no calls should be no sittings")
	}
}
