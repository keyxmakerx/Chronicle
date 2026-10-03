package smtp

import (
	"context"
	"strings"
	"testing"
)

func TestEmailStatus(t *testing.T) {
	tests := []struct {
		name     string
		in       SMTPSettings
		wantText string
		wantOK   bool
	}{
		{"enabled with host", SMTPSettings{Host: "smtp.example.com", Enabled: true}, "Email is working.", true},
		{"saved but disabled", SMTPSettings{Host: "smtp.example.com"}, "Email is set up.", false},
		{"nothing saved", SMTPSettings{}, "Email isn't set up yet.", false},
		{"enabled without host", SMTPSettings{Enabled: true}, "Email isn't set up yet.", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			text, ok := emailStatus(&tc.in)
			if text != tc.wantText || ok != tc.wantOK {
				t.Errorf("emailStatus = (%q, %v), want (%q, %v)", text, ok, tc.wantText, tc.wantOK)
			}
		})
	}
}

// The test controls must live outside the settings form so Enter there cannot
// save, and the settings form must offer Save only.
func TestSMTPFormComponentLayout(t *testing.T) {
	var sb strings.Builder
	if err := SMTPFormComponent(&SMTPSettings{Host: "h", Enabled: true}, "tok", "boom", "").Render(context.Background(), &sb); err != nil {
		t.Fatal(err)
	}
	out := sb.String()

	first := strings.Index(out, `hx-put="/admin/smtp"`)
	settingsEnd := strings.Index(out[first:], "</form>") + first
	settingsForm := out[first:settingsEnd]
	rest := out[settingsEnd:]

	for _, banned := range []string{"send-test", "/admin/smtp/test", "Cancel", "test_email"} {
		if strings.Contains(settingsForm, banned) {
			t.Errorf("settings form contains %q", banned)
		}
	}
	for _, want := range []string{`hx-post="/admin/smtp/send-test"`, `hx-post="/admin/smtp/test"`, "Send a test", "Uses the saved settings"} {
		if !strings.Contains(rest, want) {
			t.Errorf("test card missing %q", want)
		}
	}
	if !strings.Contains(out, "Email is working.") {
		t.Error("missing status line")
	}
	if !strings.Contains(out, "boom") {
		t.Error("error message not rendered")
	}
	if strings.Contains(out, "last sent") || strings.Contains(out, "Last sent") {
		t.Error("claims a last-sent time that is not recorded")
	}
}
