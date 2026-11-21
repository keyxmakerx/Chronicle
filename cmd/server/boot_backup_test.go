package main

import "testing"

func TestBackupRequired(t *testing.T) {
	tests := []struct {
		name, raw, env string
		want           bool
	}{
		{"unset in development", "", "development", false},
		{"unset in production", "", "production", true},
		{"explicit on in development", "1", "development", true},
		{"explicit true", "TRUE", "development", true},
		{"explicit off in production", "0", "production", false},
		{"explicit false in production", "false", "production", false},
		{"unknown value falls back to env", "maybe", "production", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := backupRequired(tt.raw, tt.env); got != tt.want {
				t.Fatalf("backupRequired(%q, %q) = %v, want %v", tt.raw, tt.env, got, tt.want)
			}
		})
	}
}
