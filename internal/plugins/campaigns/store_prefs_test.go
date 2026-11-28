package campaigns

import (
	"context"
	"errors"
	"testing"
)

// storePrefsStub answers one person's Store icon choice, or fails.
type storePrefsStub struct {
	hidden bool
	err    error
	asked  bool
}

func (s *storePrefsStub) StoreHidden(context.Context, string, string) (bool, error) {
	s.asked = true
	return s.hidden, s.err
}

// The desk shows the person's own choice, and anything it cannot read (no
// reader wired, no signed-in person, a failed read) leaves the icon shown.
func TestHandler_StoreHidden(t *testing.T) {
	tests := []struct {
		name      string
		prefs     *storePrefsStub
		user      string
		want      bool
		wantAsked bool
	}{
		{"put away", &storePrefsStub{hidden: true}, "u1", true, true},
		{"shown", &storePrefsStub{}, "u1", false, true},
		{"a failed read leaves it shown", &storePrefsStub{hidden: true, err: errors.New("down")}, "u1", false, true},
		{"no signed-in person is never asked", &storePrefsStub{hidden: true}, "", false, false},
		{"no reader wired", nil, "u1", false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handler{}
			if tc.prefs != nil {
				h.SetStorePrefs(tc.prefs)
			}
			if got := h.storeHidden(context.Background(), tc.user, "c1"); got != tc.want {
				t.Errorf("storeHidden = %v, want %v", got, tc.want)
			}
			if tc.prefs != nil && tc.prefs.asked != tc.wantAsked {
				t.Errorf("asked = %v, want %v", tc.prefs.asked, tc.wantAsked)
			}
		})
	}
}
