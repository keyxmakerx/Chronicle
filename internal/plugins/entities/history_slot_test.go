package entities

import (
	"context"
	"testing"
)

func TestOpenHistoryURL(t *testing.T) {
	tests := []struct {
		name, q, want string
	}{
		{"not asked", "", ""},
		{"newest", "1", "/campaigns/c1/entities/e1/history"},
		{"a version", "v 9&x", "/campaigns/c1/entities/e1/history?v=v+9%26x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := openHistoryURL(withOpenHistory(context.Background(), tt.q), "c1", "e1"); got != tt.want {
				t.Errorf("openHistoryURL = %q, want %q", got, tt.want)
			}
		})
	}
}
