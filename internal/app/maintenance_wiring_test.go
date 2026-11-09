package app

import (
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/maintenance"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// TestMaintenanceGateReadsTheAuthCookies pins that the gate, which cannot
// import the auth plugin, looks for the session under the same cookie names
// auth sets. A drift would silently lock the restoring admin out of their
// own progress page.
func TestMaintenanceGateReadsTheAuthCookies(t *testing.T) {
	if got, want := maintenance.SessionCookieNames(), auth.SessionCookieNames(); !reflect.DeepEqual(got, want) {
		t.Fatalf("gate cookie names %v != auth cookie names %v", got, want)
	}
}
