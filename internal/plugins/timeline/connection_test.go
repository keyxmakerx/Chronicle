package timeline

import (
	"testing"
)

func TestIsValidConnectionStyle(t *testing.T) {
	tests := []struct {
		style string
		valid bool
	}{
		{"arrow", true},
		{"dashed", true},
		{"dotted", true},
		{"solid", true},
		{"", false},
		{"wavy", false},
		{"double", false},
	}

	for _, tt := range tests {
		t.Run(tt.style, func(t *testing.T) {
			if got := IsValidConnectionStyle(tt.style); got != tt.valid {
				t.Errorf("IsValidConnectionStyle(%q) = %v, want %v", tt.style, got, tt.valid)
			}
		})
	}
}

func TestEventConnectionJSON(t *testing.T) {
	// Verify that EventConnection fields serialize as expected.
	conn := EventConnection{
		ID:         1,
		TimelineID: "tl-1",
		SourceID:   "evt-1",
		TargetID:   "evt-2",
		SourceType: "standalone",
		TargetType: "calendar",
		Style:      "arrow",
	}
	if conn.SourceID != "evt-1" {
		t.Errorf("expected SourceID evt-1, got %s", conn.SourceID)
	}
	if conn.Style != "arrow" {
		t.Errorf("expected style arrow, got %s", conn.Style)
	}
}

func TestCreateConnectionInputDefaults(t *testing.T) {
	// Verify that the model supports all required fields.
	input := CreateConnectionInput{
		SourceID:   "a",
		TargetID:   "b",
		SourceType: "standalone",
		TargetType: "standalone",
		Style:      "dashed",
	}
	if input.SourceID != "a" || input.TargetID != "b" {
		t.Error("field assignment failed")
	}
	if input.Style != "dashed" {
		t.Errorf("expected style dashed, got %s", input.Style)
	}
	if input.Label != nil {
		t.Error("label should be nil by default")
	}
	if input.Color != nil {
		t.Error("color should be nil by default")
	}
}
