package app

import (
	"context"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/backup"
)

type fakeAdminSearch struct {
	users []auth.User
	opts  auth.UserSearchOptions
}

func (f *fakeAdminSearch) SearchUsers(_ context.Context, opts auth.UserSearchOptions) ([]auth.User, error) {
	f.opts = opts
	return f.users, nil
}

type fakeBackupMailer struct {
	configured bool
	to         []string
	subject    string
	sent       int
}

func (f *fakeBackupMailer) IsConfigured(context.Context) bool { return f.configured }
func (f *fakeBackupMailer) SendMail(_ context.Context, to []string, subject, _ string) error {
	f.to, f.subject, f.sent = to, subject, f.sent+1
	return nil
}

func TestBackupFailureMailer(t *testing.T) {
	admins := []auth.User{
		{Email: "a@example.test"},
		{Email: "off@example.test", IsDisabled: true},
		{Email: ""},
		{Email: "b@example.test"},
	}
	tests := []struct {
		name       string
		configured bool
		users      []auth.User
		wantSent   int
		wantTo     int
	}{
		{"no email set up sends nothing", false, admins, 0, 0},
		{"active admins only", true, admins, 1, 2},
		{"no admins with an address", true, []auth.User{{IsDisabled: true, Email: "x@example.test"}}, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			users := &fakeAdminSearch{users: tt.users}
			mail := &fakeBackupMailer{configured: tt.configured}
			m := backupFailureMailer{users: users, mail: mail}
			if err := m.BackupFailed(context.Background(), backup.ScheduledRun{At: time.Now(), Detail: "exit 3"}); err != nil {
				t.Fatal(err)
			}
			if mail.sent != tt.wantSent || len(mail.to) != tt.wantTo {
				t.Errorf("sent %d to %v, want %d mails to %d people", mail.sent, mail.to, tt.wantSent, tt.wantTo)
			}
			if tt.configured && users.opts.Filter != auth.UserFilterAdmins {
				t.Errorf("searched with filter %v, want admins", users.opts.Filter)
			}
		})
	}
}

type fixedSiteName string

func (n fixedSiteName) SiteName(context.Context) string { return string(n) }

// The failure email names the site as set under Site look.
func TestBackupFailureMailer_SiteName(t *testing.T) {
	for _, tt := range []struct {
		name string
		site auth.SiteNameSource
		want string
	}{
		{"site name set", fixedSiteName("Harbour Games"), "Harbour Games: the daily backup failed"},
		{"not wired", nil, "Chronicle: the daily backup failed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mail := &fakeBackupMailer{configured: true}
			m := backupFailureMailer{users: &fakeAdminSearch{users: []auth.User{{Email: "a@example.test"}}}, mail: mail, site: tt.site}
			if err := m.BackupFailed(context.Background(), backup.ScheduledRun{At: time.Now(), Detail: "exit 3"}); err != nil {
				t.Fatal(err)
			}
			if mail.subject != tt.want {
				t.Errorf("subject = %q, want %q", mail.subject, tt.want)
			}
		})
	}
}
