package calendar

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestWizardReview_SaysWhyCreateIsUnavailable pins that the Review step
// names what is missing beside the Create button, which a preset without
// today's month and day leaves grey.
func TestWizardReview_SaysWhyCreateIsUnavailable(t *testing.T) {
	ir := &ImportResult{CalendarName: "Elven Reckoning", Months: []MonthInput{{Name: "Frostfall", Days: 30}}}
	var buf bytes.Buffer
	if err := wizardReviewBody("c1", "preset", "elven", "", "tok", "/back", "", ir).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	for _, want := range []string{
		`x-show="!m || !d"`,
		"Pick today's month and day to create it.",
		`aria-describedby="calv5-review-why"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("Review step lacks %q", want)
		}
	}
}
