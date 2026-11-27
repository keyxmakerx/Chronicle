package entities

import (
	"encoding/json"
	"strings"
	"testing"
)

// A page in a DM-only category says so on the wire, so the Foundry module,
// which reads with the Director's key, can keep it from players too. An
// ordinary page carries no such key.
func TestEntityJSON_TypeDMOnly(t *testing.T) {
	tests := []struct {
		name   string
		dmOnly bool
		want   string
		absent bool
	}{
		{"DM-only category", true, `"type_dm_only":true`, false},
		{"ordinary category", false, `"type_dm_only"`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(Entity{ID: "e1", TypeDMOnly: tt.dmOnly})
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(string(b), tt.want); got == tt.absent {
				t.Errorf("json %s: contains %s = %v, want %v", b, tt.want, got, !tt.absent)
			}
		})
	}
}
