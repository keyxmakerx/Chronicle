package auth

import (
	"context"
	"strings"
	"testing"
	"time"
)

type fixedSiteName string

func (n fixedSiteName) SiteName(ctx context.Context) string { return string(n) }

// TestPasswordResetEmail_UsesSiteName pins that account emails say the name
// set under Site look, and "Chronicle" when none is wired or it is empty.
func TestPasswordResetEmail_UsesSiteName(t *testing.T) {
	cases := []struct {
		name string
		src  SiteNameSource
		want string
	}{
		{"site name set", fixedSiteName("Harbour Games"), "Harbour Games"},
		{"empty name", fixedSiteName(""), "Chronicle"},
		{"not wired", nil, "Chronicle"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mail := &mockMailSender{}
			repo := &mockUserRepo{
				findByEmailFn: func(ctx context.Context, email string) (*User, error) {
					return &User{ID: "user-123", Email: "alice@example.com"}, nil
				},
				createResetTokenFn: func(ctx context.Context, userID, email, tokenHash string, expiresAt time.Time) error {
					return nil
				},
			}
			svc := newTestAuthService(repo)
			svc.mail = mail
			svc.baseURL = "https://chronicle.example.com"
			if tc.src != nil {
				ConfigureSiteName(svc, tc.src)
			}

			if err := svc.InitiatePasswordReset(context.Background(), "alice@example.com"); err != nil {
				t.Fatalf("InitiatePasswordReset: %v", err)
			}
			if mail.lastSubject != "Password Reset — "+tc.want {
				t.Errorf("subject = %q, want it to name %q", mail.lastSubject, tc.want)
			}
			if !strings.Contains(mail.lastBody, "your "+tc.want+" account") {
				t.Errorf("body does not name %q:\n%s", tc.want, mail.lastBody)
			}
		})
	}
}
