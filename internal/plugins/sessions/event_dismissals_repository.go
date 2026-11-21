package sessions

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/go-sql-driver/mysql"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// isMissingEventLink reports a foreign-key refusal (1452) on the named
// constraint: the calendar event was deleted between the handler reading it
// and this insert, which is a missing event, not a server fault.
func isMissingEventLink(err error, constraint string) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1452 && strings.Contains(me.Message, constraint)
}

// ListDismissedEventIDs implements SessionRepository.
func (r *sessionRepository) ListDismissedEventIDs(ctx context.Context, campaignID string) ([]string, error) {
	return r.stringColumn(ctx, "listing dismissed events",
		`SELECT calendar_event_id FROM session_event_dismissals WHERE campaign_id = ?`, campaignID)
}

// DismissEvent implements SessionRepository. A second press keeps the first
// presser's name, so a row that is already there is left as it is. That is a
// no-op ON DUPLICATE KEY UPDATE and not INSERT IGNORE, which would also turn a
// foreign-key refusal into a silent warning and answer success for an event
// that no longer exists.
func (r *sessionRepository) DismissEvent(ctx context.Context, campaignID, eventID, userID string) error {
	var by any
	if userID != "" {
		by = userID
	}
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO session_event_dismissals (campaign_id, calendar_event_id, dismissed_by) VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE campaign_id = campaign_id`,
		campaignID, eventID, by); err != nil {
		if isMissingEventLink(err, "fk_session_event_dismissal_event") {
			return apperror.NewNotFound("that event is no longer on the calendar")
		}
		return fmt.Errorf("dismissing event: %w", err)
	}
	return nil
}

// RestoreEvent implements SessionRepository. Deleting a row that is not there
// is a success, so an Undo pressed twice, or after another Scribe already put
// the event back, is harmless. The campaign id is part of the key, so one
// campaign can never un-hide another's event.
func (r *sessionRepository) RestoreEvent(ctx context.Context, campaignID, eventID string) error {
	if _, err := r.db.ExecContext(ctx,
		`DELETE FROM session_event_dismissals WHERE campaign_id = ? AND calendar_event_id = ?`,
		campaignID, eventID); err != nil {
		return fmt.Errorf("restoring event: %w", err)
	}
	return nil
}
