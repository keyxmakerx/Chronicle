package syncapi

import (
	"testing"
	"time"
)

func TestInboundBuffer_NewestFirstAndFilter(t *testing.T) {
	resetInboundBuffer()
	base := time.Unix(1000, 0)
	recordInbound("e1", "fields", map[string]any{"a": 1}, base)
	recordInbound("e2", "fields", map[string]any{"b": 2}, base.Add(time.Second))
	recordInbound("e1", "fields", map[string]any{"a": 3}, base.Add(2*time.Second))

	// Across all, newest first.
	all := RecentInbound("", 10)
	if len(all) != 3 {
		t.Fatalf("want 3, got %d", len(all))
	}
	if all[0].At.Before(all[1].At) {
		t.Error("records should be newest-first")
	}

	// Filter by entity.
	e1 := RecentInbound("e1", 10)
	if len(e1) != 2 {
		t.Fatalf("want 2 for e1, got %d", len(e1))
	}
	for _, r := range e1 {
		if r.EntityID != "e1" {
			t.Errorf("filter leaked %s", r.EntityID)
		}
	}

	// Limit honored.
	if got := RecentInbound("", 1); len(got) != 1 {
		t.Errorf("limit not honored: %d", len(got))
	}
}

func TestInboundBuffer_CapAndCopy(t *testing.T) {
	resetInboundBuffer()
	for i := 0; i < inboundBufferCap+50; i++ {
		recordInbound("e", "fields", map[string]any{"i": i}, time.Unix(int64(i), 0))
	}
	got := RecentInbound("e", inboundBufferCap+100)
	if len(got) != inboundBufferCap {
		t.Errorf("buffer should cap at %d, got %d", inboundBufferCap, len(got))
	}

	// Stored map is a copy: mutating the source after recording must not change it.
	src := map[string]any{"k": "v"}
	resetInboundBuffer()
	recordInbound("e", "fields", src, time.Unix(1, 0))
	src["k"] = "MUTATED"
	if RecentInbound("e", 1)[0].Fields["k"] != "v" {
		t.Error("recordInbound must store a copy of the fields map")
	}
}

// resetInboundBuffer clears the ring for deterministic tests.
func resetInboundBuffer() {
	inboundMu.Lock()
	inboundBuf = nil
	inboundMu.Unlock()
}
