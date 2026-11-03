package campaigns

import (
	"context"
	"strings"
	"testing"
)

// captureInviteMailer keeps the last email so a test can read both bodies.
type captureInviteMailer struct {
	subject, plain, html string
	sent                 int
}

func (m *captureInviteMailer) SendHTMLMail(_ context.Context, _ []string, subject, plain, html string) error {
	m.subject, m.plain, m.html = subject, plain, html
	m.sent++
	return nil
}

func (m *captureInviteMailer) IsConfigured(context.Context) bool { return true }

func TestCreateInvite_Email(t *testing.T) {
	for _, tc := range []struct {
		name     string
		campaign string
		settings string
		want     []string
		refuse   []string
	}{
		{
			name:     "carries the accept link, the campaign colour and the expiry",
			campaign: "Therin",
			settings: `{"accent_color":"#2563eb"}`,
			want:     []string{"You&#39;re invited to join Therin", "Accept the invitation", "/invites/accept?token=", "color:#2563eb", "expires in 7 days"},
		},
		{
			name:     "drawn in the layout the owner picked",
			campaign: "Therin",
			settings: `{"appearance":{"email_layout":"poster"}}`,
			want:     []string{"#0f1420", "Accept the invitation"},
		},
		{
			name:     "escapes the campaign name",
			campaign: `<img src=x onerror=alert(1)>`,
			want:     []string{"&lt;img src=x"},
			refuse:   []string{"<img src=x"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockCampaignRepoForInvites()
			repo.campaigns["camp-1"] = &Campaign{ID: "camp-1", Name: tc.campaign, Settings: tc.settings}
			mailer := &captureInviteMailer{}
			svc := NewInviteService(newMockInviteRepo(), repo, mailer, "https://x.test")
			if _, err := svc.CreateInvite(context.Background(), "camp-1", "user-1", CreateInviteInput{Email: "p@example.com", Role: "player"}); err != nil {
				t.Fatal(err)
			}
			if mailer.sent != 1 {
				t.Fatalf("sent %d emails, want 1", mailer.sent)
			}
			for _, w := range tc.want {
				if !strings.Contains(mailer.html, w) {
					t.Errorf("HTML is missing %q:\n%s", w, mailer.html)
				}
			}
			for _, bad := range tc.refuse {
				if strings.Contains(mailer.html, bad) {
					t.Errorf("HTML carries %q", bad)
				}
			}
			if !strings.Contains(mailer.plain, "Accept the invitation: https://x.test/invites/accept?token=") {
				t.Errorf("plain text has no accept link:\n%s", mailer.plain)
			}
			if strings.Contains(mailer.plain, "Change what reaches you") {
				t.Error("an invite to a non-member offers notification choices")
			}
		})
	}
}
