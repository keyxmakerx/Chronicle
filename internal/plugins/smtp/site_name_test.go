package smtp

import (
	"context"
	"testing"
)

type fixedSiteName string

func (n fixedSiteName) SiteName(ctx context.Context) string { return string(n) }

// TestFromName pins the sender name: the stored default ("Chronicle" or
// empty) follows the Site look name; a name the admin typed is kept.
func TestFromName(t *testing.T) {
	cases := []struct {
		name  string
		site  SiteNameSource
		saved string
		want  string
	}{
		{"default follows site name", fixedSiteName("Harbour Games"), "Chronicle", "Harbour Games"},
		{"empty follows site name", fixedSiteName("Harbour Games"), "", "Harbour Games"},
		{"admin's own name kept", fixedSiteName("Harbour Games"), "The Keeper", "The Keeper"},
		{"no site name set", fixedSiteName(""), "", "Chronicle"},
		{"not wired", nil, "Chronicle", "Chronicle"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewSMTPService(nil, "k")
			if tc.site != nil {
				ConfigureSiteName(svc, tc.site)
			}
			if got := svc.(*smtpService).fromName(context.Background(), tc.saved); got != tc.want {
				t.Errorf("fromName(%q) = %q, want %q", tc.saved, got, tc.want)
			}
		})
	}
}
