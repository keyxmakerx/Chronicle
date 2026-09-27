// calendar_import_body_limit_test.go pins isCalendarImportPath: the global
// 2MB BodyLimit's skip list for the calendar plugin's upload-heavy routes.
// A false negative here silently caps a real ~10MB import at 2MB; a false
// positive exempts an unrelated route from the global body limit — both are
// worth a direct test, not just an end-to-end one.
package app

import "testing"

func TestIsCalendarImportPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/campaigns/abc-123/calendars/wizard/import/preview", true},
		{"/campaigns/abc-123/calendars/wizard/create", true},
		{"/campaigns/abc-123/calendars/import/preview", true},
		{"/campaigns/abc-123/calendars/import", true},
		// A different campaign id segment shape must still match — :id is
		// any single path segment, not specifically a UUID.
		{"/campaigns/1/calendars/import", true},

		// Must NOT match: routes that don't take a file upload, or don't
		// belong to the calendar plugin at all.
		{"/campaigns/abc-123/calendars/presets/blank", false},
		{"/campaigns/abc-123/calendars", false},
		{"/campaigns/abc-123/calendars/wizard/import", false}, // GET step, no upload
		{"/media/upload", false},
		{"/campaigns/abc-123/calendars/import/preview/extra", false},
		{"/campaigns/abc-123/apps/calendar", false},
	}
	for _, tt := range tests {
		if got := isCalendarImportPath(tt.path); got != tt.want {
			t.Errorf("isCalendarImportPath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}
