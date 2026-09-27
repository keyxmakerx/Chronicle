package sessions

// Game-night RSVP business logic (issue #741 Part C): soft delete/restore
// with "needs to check again" flagging, per-occurrence RSVPs on a recurring
// session, the Director's own tally-exclusion switch, the "suggest another
// time" validate-before-consume flow, and the private calendar feed.
//
// Division of labor matches this plugin's existing pattern (see
// ConfirmProposalWinner's own doc comment): this file enumerates WHO needs
// telling and returns their ids; the HANDLER is the one that calls
// NotifyUsers / sends email, because member-directory and notification
// wording are handler-layer, campaign-facing concerns.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/timeutil"
)

// --- InviteAllWithStatus ---------------------------------------------------

// InviteAllWithStatus is InviteAll generalized to any starting status. Used
// to carry a confirmed proposal's winning-option yes-voters in as
// "carried_yes" (they still owe an explicit confirm) rather than plain
// "invited".
func (s *sessionService) InviteAllWithStatus(ctx context.Context, sessionID string, userIDs []string, status string) error {
	for _, userID := range userIDs {
		if err := s.repo.AddAttendee(ctx, sessionID, userID, status); err != nil {
			return apperror.NewInternal(fmt.Errorf("inviting user %s as %s: %w", userID, status, err))
		}
	}
	return nil
}

// --- soft delete / restore / move, with needs-recheck flagging -----------

// cancelOrDeleteSession is DeleteSession's body, named to make its actual
// effect (soft-delete, not destroy) obvious at the call site.
//
// A recurring session's responses live in session_occurrence_rsvps (one row
// per night) rather than session_attendees, so BOTH stores are checked and
// flagged; a manual/non-recurring session only ever populates the first.
func (s *sessionService) cancelOrDeleteSession(ctx context.Context, id string) ([]string, error) {
	session, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	responded, err := s.respondedUserIDs(ctx, session)
	if err != nil {
		return nil, err
	}
	if err := s.flagNeedsRecheck(ctx, session); err != nil {
		return nil, err
	}

	if err := s.repo.SoftDeleteSession(ctx, id); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("deleting session: %w", err))
	}
	return responded, nil
}

// RestoreSession undoes a soft delete and returns the same "who had
// responded" set as DeleteSession, for a symmetrical re-notify ("this session
// is back on").
func (s *sessionService) RestoreSession(ctx context.Context, id string) (*Session, []string, error) {
	if err := s.repo.RestoreSession(ctx, id); err != nil {
		return nil, nil, apperror.NewInternal(fmt.Errorf("restoring session: %w", err))
	}
	session, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	responded, err := s.respondedUserIDs(ctx, session)
	if err != nil {
		return nil, nil, err
	}
	return session, responded, nil
}

// MoveSession performs UpdateSession's exact partial update, then — only when
// the stored schedule actually changed AND the session already carries
// responses — flags those responses "needs to check again" (never clears
// them) and returns who to notify. It never fires on an edit that leaves the
// schedule untouched (e.g. a recap save or a Mark Complete), because nobody
// needs to re-check an answer to a night that didn't move.
func (s *sessionService) MoveSession(ctx context.Context, id string, input UpdateSessionInput) (*Session, []string, error) {
	before, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	beforeDate, beforeTime := strPtrVal(before.ScheduledDate), strPtrVal(before.ScheduledTime)

	next, err := s.UpdateSession(ctx, id, input)
	if err != nil {
		return nil, nil, err
	}

	after, err := s.repo.FindByID(ctx, id)
	if err != nil {
		// The row is gone (e.g. a "Mark Complete" that generated a NEXT
		// occurrence still leaves the original row in place, so this really
		// only happens on a genuine lookup failure) — nothing to flag.
		return next, nil, nil
	}
	if strPtrVal(after.ScheduledDate) == beforeDate && strPtrVal(after.ScheduledTime) == beforeTime {
		return next, nil, nil
	}

	responded, err := s.respondedUserIDs(ctx, after)
	if err != nil {
		return next, nil, err
	}
	if err := s.flagNeedsRecheck(ctx, after); err != nil {
		return next, nil, err
	}
	return next, responded, nil
}

// respondedUserIDs collects who has already answered a session, from BOTH
// stores it might use. A recurring session's answers are NOT exclusively in
// session_occurrence_rsvps: the shipped RSVP control (a bare {status} POST)
// still writes to session_attendees regardless of IsRecurring — only a
// caller that explicitly sends note/occurrenceDate engages the per-occurrence
// path (see UpdateRSVPDetailed) — so checking occurrence rows alone would
// silently miss every answer given through the ordinary RSVP buttons on a
// recurring session, and a cancel/delete would notify nobody.
func (s *sessionService) respondedUserIDs(ctx context.Context, session *Session) ([]string, error) {
	attendeeIDs, err := s.repo.ListRespondedUserIDs(ctx, session.ID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("listing responders: %w", err))
	}
	if !session.IsRecurring {
		return attendeeIDs, nil
	}
	occurrenceIDs, err := s.repo.ListAllOccurrenceRespondedUserIDs(ctx, session.ID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("listing occurrence responders: %w", err))
	}
	seen := make(map[string]bool, len(attendeeIDs)+len(occurrenceIDs))
	out := make([]string, 0, len(attendeeIDs)+len(occurrenceIDs))
	for _, id := range attendeeIDs {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, id := range occurrenceIDs {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out, nil
}

// flagNeedsRecheck marks every existing answer "needs to check again" in
// BOTH stores — see respondedUserIDs' doc comment for why a recurring
// session's answers are not exclusively in the per-occurrence table.
func (s *sessionService) flagNeedsRecheck(ctx context.Context, session *Session) error {
	if err := s.repo.MarkSeriesNeedsRecheck(ctx, session.ID); err != nil {
		return apperror.NewInternal(fmt.Errorf("flagging responses: %w", err))
	}
	if !session.IsRecurring {
		return nil
	}
	if err := s.repo.MarkAllOccurrencesNeedsRecheck(ctx, session.ID); err != nil {
		return apperror.NewInternal(fmt.Errorf("flagging occurrence responses: %w", err))
	}
	return nil
}

func strPtrVal(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// --- per-occurrence RSVP + notes + tally exclusion ------------------------

// notePatchToRepoNote converts the service-boundary patch.Field into the
// repository's plain *string note contract: nil means the caller never
// mentioned a note at all (repo methods that accept it treat that as
// "preserve"), a non-nil pointer means the caller means to set-or-clear it
// (an explicit null becomes a pointer to "", which every repo note-setter
// treats as a clear).
func notePatchToRepoNote(note patch.Field[string]) *string {
	if !note.Present() {
		return nil
	}
	if note.IsNull() {
		empty := ""
		return &empty
	}
	v, _ := note.Get()
	return &v
}

// nextUpcomingOccurrenceDate returns the next occurrence date on/after today
// for a recurring session, reusing computeNextOccurrence's own per-type step
// rather than re-deriving the recurrence math. The walk is capped (520 steps
// — roughly ten years of weekly steps) so a pathological recurrence config
// can never spin unbounded; it is not expected to run more than once or
// twice in practice, since a session left unanswered for years is not the
// case this exists for.
func nextUpcomingOccurrenceDate(session *Session, today string) string {
	if session.ScheduledDate == nil || *session.ScheduledDate == "" || session.RecurrenceType == nil {
		return ""
	}
	cur := *session.ScheduledDate
	if cur >= today {
		return cur
	}
	probe := &Session{
		ScheduledDate:      &cur,
		RecurrenceType:     session.RecurrenceType,
		RecurrenceInterval: session.RecurrenceInterval,
	}
	for i := 0; i < 520; i++ {
		next := computeNextOccurrence(probe)
		if next == "" {
			return cur
		}
		cur = next
		probe.ScheduledDate = &cur
		if cur >= today {
			return cur
		}
	}
	return cur
}

// UpdateRSVPDetailed is the game-night-aware sibling of UpdateRSVP (which is
// untouched — this does not replace it, and the two share no code, so
// UpdateRSVP's existing test coverage keeps proving its exact prior
// behavior). A non-recurring session's answer still lands in
// session_attendees exactly as before; a recurring session's lands in
// session_occurrence_rsvps, keyed to the given night (or the next upcoming
// one when the caller doesn't say which).
func (s *sessionService) UpdateRSVPDetailed(ctx context.Context, sessionID, userID, status string, occurrenceDate *string, note patch.Field[string]) error {
	switch status {
	case RSVPAccepted, RSVPDeclined, RSVPTentative:
		// Valid — same accepted set as UpdateRSVP. "invited" and
		// "carried_yes" are system-assigned states, never a member-submitted
		// value, exactly like today's "invited" restriction.
	default:
		return apperror.NewBadRequest("invalid RSVP status: must be accepted, declined, or tentative")
	}

	noteVal := notePatchToRepoNote(note)
	// Matches session_attendees.note / session_occurrence_rsvps.note
	// VARCHAR(140) — reject an over-length note here, before any write,
	// rather than letting the column reject it after the status write has
	// already committed.
	if noteVal != nil && len(*noteVal) > 140 {
		return apperror.NewValidation("the note must be at most 140 characters")
	}
	if occurrenceDate != nil && *occurrenceDate != "" {
		if _, err := time.Parse("2006-01-02", *occurrenceDate); err != nil {
			return apperror.NewValidation("occurrenceDate must be a valid YYYY-MM-DD date")
		}
	}

	session, err := s.repo.FindByID(ctx, sessionID)
	if err != nil {
		return err
	}

	if session.IsRecurring {
		occDate := ""
		if occurrenceDate != nil && *occurrenceDate != "" {
			occDate = *occurrenceDate
		} else {
			occDate = nextUpcomingOccurrenceDate(session, time.Now().UTC().Format("2006-01-02"))
		}
		if occDate == "" {
			return apperror.NewBadRequest("this session has no upcoming occurrence to answer for")
		}
		return s.repo.UpsertOccurrenceRSVP(ctx, sessionID, userID, occDate, status, noteVal)
	}

	if err := s.repo.UpdateAttendeeStatus(ctx, sessionID, userID, status); err != nil {
		return err
	}
	if noteVal != nil {
		if err := s.repo.SetAttendeeNote(ctx, sessionID, userID, noteVal); err != nil {
			return err
		}
	}
	return nil
}

// ListOccurrenceAttendees returns one night's roster for a recurring session.
func (s *sessionService) ListOccurrenceAttendees(ctx context.Context, sessionID, occurrenceDate string) ([]OccurrenceRSVP, error) {
	rsvps, err := s.repo.ListOccurrenceRSVPs(ctx, sessionID, occurrenceDate)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("listing occurrence rsvps: %w", err))
	}
	return rsvps, nil
}

// SetExcludedFromCount is the Director's own "I still count as a member of
// the table, but leave me out of the going/total tally" switch (operator
// answer #3: "The Director counts in a game night's answers like any member,
// with a switch to leave themselves out."). Self-only and organizer/owner-only
// by design — this is not a way to exclude ANOTHER member's answer from the
// count, which would misrepresent who is actually coming.
func (s *sessionService) SetExcludedFromCount(ctx context.Context, sessionID, userID string, isCampaignOwner, excluded bool, occurrenceDate *string) error {
	session, err := s.repo.FindByID(ctx, sessionID)
	if err != nil {
		return err
	}
	if session.CreatedBy != userID && !isCampaignOwner {
		return apperror.NewForbidden("only the session organizer or the campaign owner may set this")
	}

	if session.IsRecurring {
		occDate := ""
		if occurrenceDate != nil && *occurrenceDate != "" {
			occDate = *occurrenceDate
		} else {
			occDate = nextUpcomingOccurrenceDate(session, time.Now().UTC().Format("2006-01-02"))
		}
		if occDate == "" {
			return apperror.NewBadRequest("this session has no upcoming occurrence")
		}
		return s.repo.SetOccurrenceExcluded(ctx, sessionID, userID, occDate, excluded)
	}
	return s.repo.SetAttendeeExcluded(ctx, sessionID, userID, excluded)
}

// --- "suggest another time" ------------------------------------------------

// ValidateAndRecordSuggestion is the fix for the historical bug where a
// suggestion token was consumed BEFORE the suggested date/time was validated
// — a bad submission (a malformed or past date) burned the single-use link
// with nothing to show for it, forcing the member to ask the Director for a
// fresh invite just to try again.
//
// Order, mirroring ApplyRSVPToken's own consume-then-apply reasoning but with
// validation moved in FRONT of consumption:
//  1. resolve the token and check it exists, is unexpired, unused, and is
//     actually a "suggest" action token — a read, nothing written yet.
//  2. validate the submitted date/time — a read, nothing written yet.
//  3. ONLY once both pass, mark the token used (the same atomic
//     `used_at IS NULL` guard every other token route relies on).
//  4. record the suggestion.
//
// A failure at step 1 or 2 leaves the token untouched, so the member can
// correct their input and resubmit the SAME link. A lost race at step 3
// (ErrRSVPTokenSpent) stops before step 4, so a spent token can never gain a
// second suggestion row.
func (s *sessionService) ValidateAndRecordSuggestion(ctx context.Context, tokenStr, suggestedDate string, suggestedTime, note *string) (*RescheduleSuggestion, error) {
	token, err := s.repo.FindRSVPToken(ctx, tokenStr)
	if err != nil {
		return nil, err
	}
	if token.Action != RSVPActionSuggest {
		return nil, apperror.NewBadRequest("this link cannot be used to suggest a different time")
	}
	if token.UsedAt != nil {
		return nil, apperror.NewBadRequest("this link has already been used")
	}
	if time.Now().UTC().After(token.ExpiresAt) {
		return nil, apperror.NewBadRequest("this link has expired")
	}

	parsedDate, err := time.Parse("2006-01-02", strings.TrimSpace(suggestedDate))
	if err != nil {
		return nil, apperror.NewValidation("please enter a valid date")
	}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	if parsedDate.Before(today) {
		return nil, apperror.NewValidation("the suggested date can't be in the past")
	}
	// Normalize (trim) BEFORE both validating and storing — the earlier
	// version validated a trimmed copy but stored the caller's raw,
	// untrimmed pointer, so a value like " 19:00 " passed validation here and
	// then failed the VARCHAR(5) column write AFTER the token was already
	// consumed, which is exactly the bug this method exists to avoid.
	var cleanTime *string
	if suggestedTime != nil {
		if t := strings.TrimSpace(*suggestedTime); t != "" {
			if _, err := time.Parse("15:04", t); err != nil {
				return nil, apperror.NewValidation("please enter a valid time")
			}
			cleanTime = &t
		}
	}
	var cleanNote *string
	if note != nil {
		if n := strings.TrimSpace(*note); n != "" {
			// Matches session_reschedule_suggestions.note VARCHAR(140) — an
			// over-length note must fail HERE, before the token is
			// consumed, not as a column-width error after it.
			if len(n) > 140 {
				return nil, apperror.NewValidation("the note must be at most 140 characters")
			}
			cleanNote = &n
		}
	}

	if err := s.repo.MarkRSVPTokenUsed(ctx, tokenStr); err != nil {
		if err == ErrRSVPTokenSpent {
			return nil, err
		}
		return nil, apperror.NewInternal(fmt.Errorf("marking suggestion token used: %w", err))
	}

	suggestion := &RescheduleSuggestion{
		SessionID:     token.SessionID,
		UserID:        token.UserID,
		SuggestedDate: parsedDate.Format("2006-01-02"),
		SuggestedTime: cleanTime,
		Note:          cleanNote,
		CreatedAt:     time.Now().UTC(),
	}
	if err := s.repo.CreateRescheduleSuggestion(ctx, suggestion); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("recording suggestion: %w", err))
	}
	return suggestion, nil
}

// --- calendar feed ----------------------------------------------------------

func (s *sessionService) GetOrCreateFeedToken(ctx context.Context, campaignID, userID string) (*CalendarFeedToken, error) {
	tok, err := s.repo.GetOrCreateFeedToken(ctx, campaignID, userID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("getting feed token: %w", err))
	}
	return tok, nil
}

func (s *sessionService) ReplaceFeedToken(ctx context.Context, campaignID, userID string) (*CalendarFeedToken, error) {
	tok, err := s.repo.ReplaceFeedToken(ctx, campaignID, userID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("replacing feed token: %w", err))
	}
	return tok, nil
}

func (s *sessionService) IsCalendarFeedEnabled(ctx context.Context, campaignID string) (bool, error) {
	enabled, err := s.repo.IsCalendarFeedEnabled(ctx, campaignID)
	if err != nil {
		return false, apperror.NewInternal(fmt.Errorf("reading feed settings: %w", err))
	}
	return enabled, nil
}

func (s *sessionService) SetCalendarFeedEnabled(ctx context.Context, campaignID string, enabled bool) error {
	if err := s.repo.SetCalendarFeedEnabled(ctx, campaignID, enabled); err != nil {
		return apperror.NewInternal(fmt.Errorf("setting feed enabled: %w", err))
	}
	return nil
}

// ResolveFeedToken is the read behind the public feed route's credential
// check: the token IS the credential (mirrors /rsvp/:token), so a missing or
// unknown token and a campaign-disabled feed both fail the SAME way —
// apperror.NewNotFound — so the route can return one plain 404 either way
// rather than distinguishing "wrong token" from "turned off" to an
// unauthenticated caller.
//
// Deliberately does NOT list any session data: unlike a session-scoped RSVP
// token, a feed token has no natural expiry, so the HANDLER must re-check the
// token's user is STILL a campaign member before this method's sibling,
// ListFeedSessions, is ever called — a removed member's subscribed calendar
// app would otherwise keep pulling every planned session's name, date and
// time indefinitely, since nothing else revokes the token when membership
// ends. Splitting the credential check from the data read means that failure
// mode can't reach the data at all, not just "the handler happens to discard
// it before responding".
func (s *sessionService) ResolveFeedToken(ctx context.Context, tokenStr string) (campaignID, userID string, err error) {
	tok, err := s.repo.FindFeedToken(ctx, tokenStr)
	if err != nil {
		return "", "", err
	}
	enabled, err := s.repo.IsCalendarFeedEnabled(ctx, tok.CampaignID)
	if err != nil {
		return "", "", apperror.NewInternal(fmt.Errorf("reading feed settings: %w", err))
	}
	if !enabled {
		return "", "", apperror.NewNotFound("this campaign's calendar feed is turned off")
	}
	return tok.CampaignID, tok.UserID, nil
}

// ListFeedSessions returns campaignID's upcoming sessions to render as an ICS
// feed. Call only AFTER ResolveFeedToken has succeeded AND the caller has
// re-checked the token's user is still a campaign member — see
// ResolveFeedToken's doc comment.
func (s *sessionService) ListFeedSessions(ctx context.Context, campaignID string) ([]Session, error) {
	// A generous forward window (2 years) plus everything already past that
	// is still planned: an ICS feed subscriber's calendar app re-fetches
	// periodically, so this does not need to be exhaustive, only sufficient
	// for a subscribed app to show "what's next".
	start := time.Now().UTC().Format("2006-01-02")
	end := time.Now().UTC().AddDate(2, 0, 0).Format("2006-01-02")
	list, err := s.repo.ListByDateRange(ctx, campaignID, start, end)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("listing feed sessions: %w", err))
	}
	return list, nil
}

// BuildFeedICS renders a minimal, valid RFC 5545 VCALENDAR/VEVENT block, one
// VEVENT per session. No third-party ICS library is imported anywhere in this
// repo (checked go.mod), matching the project's low-dependency style, so this
// is hand-built rather than pulling one in for ~20 lines of text.
//
// DTSTART/DTEND are computed in UTC from ScheduledDate + ScheduledTime +
// ScheduledTZ; a session with no ScheduledTZ falls back to UTC (documented
// here rather than guessing a campaign zone, since a session is not
// necessarily tied to one calendar row). A session with no ScheduledTime
// becomes an all-day VEVENT (DTSTART;VALUE=DATE), and a session with no
// ScheduledDate at all is skipped — there is nothing to put on a calendar.
func (s *sessionService) BuildFeedICS(sessions []Session) string {
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\n")
	b.WriteString("VERSION:2.0\r\n")
	b.WriteString("PRODID:-//Chronicle//Game Nights//EN\r\n")
	b.WriteString("CALSCALE:GREGORIAN\r\n")
	now := time.Now().UTC().Format("20060102T150405Z")
	for _, sess := range sessions {
		if sess.ScheduledDate == nil || *sess.ScheduledDate == "" {
			continue
		}
		b.WriteString("BEGIN:VEVENT\r\n")
		fmt.Fprintf(&b, "UID:session-%s@chronicle\r\n", sess.ID)
		fmt.Fprintf(&b, "DTSTAMP:%s\r\n", now)
		switch {
		case sess.ScheduledTime == nil || *sess.ScheduledTime == "":
			day := strings.ReplaceAll(*sess.ScheduledDate, "-", "")
			fmt.Fprintf(&b, "DTSTART;VALUE=DATE:%s\r\n", day)
		case sess.ScheduledTZ != nil && *sess.ScheduledTZ != "":
			// A known zone converts cleanly to a real UTC instant.
			loc := timeutil.LoadLocation(*sess.ScheduledTZ)
			start, err := time.ParseInLocation("2006-01-02 15:04", *sess.ScheduledDate+" "+*sess.ScheduledTime, loc)
			if err != nil {
				day := strings.ReplaceAll(*sess.ScheduledDate, "-", "")
				fmt.Fprintf(&b, "DTSTART;VALUE=DATE:%s\r\n", day)
				break
			}
			end := start.Add(3 * time.Hour) // no stored duration; a typical session-length default
			fmt.Fprintf(&b, "DTSTART:%s\r\n", start.UTC().Format("20060102T150405Z"))
			fmt.Fprintf(&b, "DTEND:%s\r\n", end.UTC().Format("20060102T150405Z"))
		default:
			// No stored zone (every pre-Part-C session, and any manual
			// session whose creator's zone wasn't captured): a session with
			// a time but no zone is NOT UTC — treating it as UTC would put
			// e.g. a Chicago Director's "7:00 PM" at 2:00 PM for a subscriber
			// six hours east. RFC 5545 "floating" time (no Z, no TZID) is the
			// honest representation: each subscriber's calendar app shows it
			// at face value, in ITS OWN local time, same as the organizer
			// typed it — no worse than the UTC guess, and never confidently
			// wrong by a fixed offset.
			start, err := time.Parse("2006-01-02 15:04", *sess.ScheduledDate+" "+*sess.ScheduledTime)
			if err != nil {
				day := strings.ReplaceAll(*sess.ScheduledDate, "-", "")
				fmt.Fprintf(&b, "DTSTART;VALUE=DATE:%s\r\n", day)
				break
			}
			end := start.Add(3 * time.Hour)
			fmt.Fprintf(&b, "DTSTART:%s\r\n", start.Format("20060102T150405"))
			fmt.Fprintf(&b, "DTEND:%s\r\n", end.Format("20060102T150405"))
		}
		fmt.Fprintf(&b, "SUMMARY:%s\r\n", icsEscape(sess.Name))
		b.WriteString("END:VEVENT\r\n")
	}
	b.WriteString("END:VCALENDAR\r\n")
	return b.String()
}

// icsEscape escapes the handful of characters RFC 5545 requires escaped in a
// TEXT value, so an operator-authored session name can't break the feed's
// line structure.
func icsEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `,`, `\,`, `;`, `\;`, "\n", `\n`)
	return r.Replace(s)
}
