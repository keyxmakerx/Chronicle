package media

import (
	"testing"
)

func TestNewClamAVScanner_EmptyAddress(t *testing.T) {
	scanner := NewClamAVScanner("")
	if scanner != nil {
		t.Error("expected nil scanner for empty address")
	}
}

func TestNewClamAVScanner_WithAddress(t *testing.T) {
	scanner := NewClamAVScanner("localhost:3310")
	if scanner == nil {
		t.Fatal("expected non-nil scanner")
	}
	if scanner.Address != "localhost:3310" {
		t.Errorf("expected address localhost:3310, got %s", scanner.Address)
	}
	if scanner.Timeout == 0 {
		t.Error("expected non-zero timeout")
	}
}

func TestClamAVScanner_ScanUnavailable(t *testing.T) {
	// Scanner should fail-open when clamd is not reachable.
	scanner := NewClamAVScanner("localhost:19999") // Not a real clamd.
	err := scanner.Scan([]byte("test content"), "test.jpg")
	if err != nil {
		t.Errorf("expected nil error (fail-open), got: %v", err)
	}
}
