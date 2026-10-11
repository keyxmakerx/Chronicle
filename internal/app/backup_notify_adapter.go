package app

import (
	"context"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/backup"
)

// backupAdminSearch is the slice of the user repository the failure email
// needs: the site admins.
type backupAdminSearch interface {
	SearchUsers(ctx context.Context, opts auth.UserSearchOptions) ([]auth.User, error)
}

// backupMailer is the slice of the SMTP service the failure email needs.
type backupMailer interface {
	IsConfigured(ctx context.Context) bool
	SendMail(ctx context.Context, to []string, subject, body string) error
}

// backupFailureMailer emails every active site admin when a scheduled
// backup fails. It lives here so the backup plugin imports neither auth nor
// smtp. No email set up means no email; the Backup page still shows it.
type backupFailureMailer struct {
	users backupAdminSearch
	mail  backupMailer
}

// backupAdminPageSize bounds the admin lookup; a site has a handful.
const backupAdminPageSize = 100

// BackupFailed implements backup.FailureNotifier.
func (m backupFailureMailer) BackupFailed(ctx context.Context, run backup.ScheduledRun) error {
	if !m.mail.IsConfigured(ctx) {
		return nil
	}
	admins, err := m.users.SearchUsers(ctx, auth.UserSearchOptions{Filter: auth.UserFilterAdmins, PerPage: backupAdminPageSize})
	if err != nil {
		return fmt.Errorf("listing site admins: %w", err)
	}
	var to []string
	for _, u := range admins {
		if !u.IsDisabled && u.Email != "" {
			to = append(to, u.Email)
		}
	}
	if len(to) == 0 {
		return nil
	}
	body := fmt.Sprintf("Chronicle's daily backup on %s failed: %s.\n\n"+
		"Open Site admin > Backups & restore to run one now, and check the server log for the details.\n",
		run.At.Format("2 Jan 2006 at 15:04 MST"), run.Detail)
	return m.mail.SendMail(ctx, to, "Chronicle: the daily backup failed", body)
}
