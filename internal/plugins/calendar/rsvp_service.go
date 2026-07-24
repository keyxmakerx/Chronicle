package calendar

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html"
	"log/slog"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Calendar-event RSVP business logic (C-CAL-RSVP-P1). A separate RSVPService —
// NOT an extension of CalendarService — keeps this lane disjoint from the
// shared calendar interfaces (and from the parallel entity-ties leak fix).
//
// Cross-plugin dependencies are all narrow interfaces wired via post-
// construction setters in internal/app/routes.go (rule 8). Every one is
// nil-safe: without SMTP the in-app RSVP still works; without the notifier the
// bell is simply silent; without the availability writer "out this week" still
// records the decline.

// MailSender sends email notifications. Copied verbatim from the sessions
// plugin (sessions/handler.go:21-24) so the same SMTP service satisfies both.
type MailSender interface {
	SendHTMLMail(ctx context.Context, to []string, subject, plainBody, htmlBody string) error
	IsConfigured(ctx context.Context) bool
}

// RSVPNotifier writes in-app (bell) notifications to specific users. Backed by
// a generic NotifyUsers method on the sessions notifications service via an
// adapter in app/routes.go — the notifications store is documented generic
// (T-B2), so a second feature writing to it is by-design.
type RSVPNotifier interface {
	NotifyUsers(ctx context.Context, userIDs []string, campaignID, ntype, message, link string) error
}

// AvailabilityExceptionWriter writes the ACTING user's own availability
// exceptions — the "out this week" action. SELF-write only: userID is always the
// redeemed token's user, never a caller-controlled parameter. Backed by the
// sessions availability service (AddMyException / ListMyExceptions) via an
// adapter in app/routes.go.
type AvailabilityExceptionWriter interface {
	// ListMyExceptionDates returns the YYYY-MM-DD dates the user already has ANY
	// exception on, so out-this-week can skip hand-authored days.
	ListMyExceptionDates(ctx context.Context, campaignID, userID string) (map[string]bool, error)
	// AddFullDayUnavailable writes a full-day (0–1440) 'unavailable' exception
	// for onDate (YYYY-MM-DD) in the user's own recurring pattern.
	AddFullDayUnavailable(ctx context.Context, campaignID, userID, onDate string) error
}

// EventReader is the narrow read seam over the calendar service (GetEvent +
// GetCalendarByID). Kept narrow so the RSVP lane never extends CalendarService.
type EventReader interface {
	GetEvent(ctx context.Context, eventID string) (*Event, error)
	GetCalendarByID(ctx context.Context, calendarID string) (*Calendar, error)
}

// RSVPService is the calendar-event RSVP business boundary.
type RSVPService interface {
	// ApplyAction records the caller's own response for one of the five actions
	// (yes|maybe|no|out_week|suggest) and runs its side effects (out_week writes
	// the caller's availability; suggest notifies the owner). SELF only: userID is
	// the authenticated caller. Shared with the emailed-token path.
	ApplyAction(ctx context.Context, eventID, userID, action string, note *string) error
	// GetSummary returns counts + the caller's status; People is populated only
	// when includeDetail (Owner/co-DM detail gating done by the handler).
	GetSummary(ctx context.Context, eventID, viewerUserID string, includeDetail bool) (*RSVPSummary, error)
	// Collection opt-in (the per-event "Collect RSVPs" toggle).
	IsCollectionEnabled(ctx context.Context, eventID string) (bool, error)
	// EnableCollection turns the toggle on, fans out invite emails to viewable
	// members, and rings the bell for them. Idempotent-ish: re-enabling re-sends.
	EnableCollection(ctx context.Context, eventID string) error
	// DisableCollection turns the toggle off (no email/bell).
	DisableCollection(ctx context.Context, eventID string) error

	// Emailed-token flow. ValidateToken is the read-only GET-confirm check;
	// ApplyToken is the state-changing POST that consumes the token and records
	// the response (note used only for the "suggest another time" action).
	ValidateToken(ctx context.Context, tokenStr string) (*EventRSVPToken, error)
	ApplyToken(ctx context.Context, tokenStr string, note *string) (*EventRSVPToken, error)
}

// rsvpService implements RSVPService. Cross-plugin deps are nil until wired.
type rsvpService struct {
	repo         RSVPRepository
	events       EventReader
	members      campaigns.MemberLister
	mailer       MailSender
	notifier     RSVPNotifier
	availability AvailabilityExceptionWriter
	baseURL      string
	now          func() time.Time
}

// NewRSVPService constructs the RSVP service. The event reader (calendarService)
// and member lister (campaignService) are required; the mailer/notifier/
// availability writer are wired later via setters and stay nil-safe.
func NewRSVPService(repo RSVPRepository, events EventReader, members campaigns.MemberLister, baseURL string) *rsvpService {
	return &rsvpService{
		repo:    repo,
		events:  events,
		members: members,
		baseURL: baseURL,
		now:     time.Now,
	}
}

// SetMailSender wires the SMTP mail sender (nil-safe: no SMTP → in-app only).
func (s *rsvpService) SetMailSender(m MailSender) { s.mailer = m }

// SetNotifier wires the in-app bell notifier (nil-safe).
func (s *rsvpService) SetNotifier(n RSVPNotifier) { s.notifier = n }

// SetAvailabilityExceptionWriter wires the self-write availability adapter (nil-safe).
func (s *rsvpService) SetAvailabilityExceptionWriter(w AvailabilityExceptionWriter) {
	s.availability = w
}

// clockNow reads the injectable wall clock, defaulting to time.Now for a
// directly-constructed service (test fixtures may leave now nil).
func (s *rsvpService) clockNow() time.Time {
	if s.now == nil {
		return time.Now()
	}
	return s.now()
}

// ApplyAction validates the action then applies it for the authenticated
// caller. A bad action is a validation error.
func (s *rsvpService) ApplyAction(ctx context.Context, eventID, userID, action string, note *string) error {
	if !validRSVPAction(action) {
		return apperror.NewValidation("rsvp action must be yes, maybe, no, out_week, or suggest")
	}
	return s.applyAction(ctx, eventID, userID, action, note)
}

// applyAction is the shared core (in-app + emailed token): map the action to an
// RSVP status, upsert, then run action-specific side effects. The note is stored
// only for the "suggest" action; out_week writes the caller's availability.
func (s *rsvpService) applyAction(ctx context.Context, eventID, userID, action string, note *string) error {
	status := rsvpStatusForAction(action)
	var upNote *string
	if action == RSVPActionSuggest {
		upNote = note
	}
	if err := s.repo.UpsertRSVP(ctx, eventID, userID, status, upNote); err != nil {
		return apperror.NewInternal(err)
	}
	switch action {
	case RSVPActionOutWeek:
		s.applyOutThisWeek(ctx, eventID, userID)
		s.notifyOwnerOfResponse(ctx, eventID, userID, status)
	case RSVPActionSuggest:
		s.notifyOwnerOfSuggestion(ctx, eventID, userID, note)
	default:
		s.notifyOwnerOfResponse(ctx, eventID, userID, status)
	}
	return nil
}

// GetSummary builds the read model. Counts + MyStatus for everyone; People only
// when includeDetail is true (the handler passes Owner/co-DM only).
func (s *rsvpService) GetSummary(ctx context.Context, eventID, viewerUserID string, includeDetail bool) (*RSVPSummary, error) {
	counts, err := s.repo.CountRSVPs(ctx, eventID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	sum := &RSVPSummary{EventID: eventID, Counts: counts}
	if viewerUserID != "" {
		if mine, err := s.repo.GetMyRSVP(ctx, eventID, viewerUserID); err == nil && mine != nil {
			sum.MyStatus = mine.Status
		}
	}
	if includeDetail {
		people, err := s.repo.ListRSVPs(ctx, eventID)
		if err != nil {
			return nil, apperror.NewInternal(err)
		}
		sum.People = people
	}
	return sum, nil
}

// IsCollectionEnabled reads the per-event opt-in flag.
func (s *rsvpService) IsCollectionEnabled(ctx context.Context, eventID string) (bool, error) {
	enabled, err := s.repo.GetCollectRSVPs(ctx, eventID)
	if err != nil {
		return false, apperror.NewInternal(err)
	}
	return enabled, nil
}

// DisableCollection turns the toggle off. No fan-out.
func (s *rsvpService) DisableCollection(ctx context.Context, eventID string) error {
	if err := s.repo.SetCollectRSVPs(ctx, eventID, false); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

// generateRSVPToken returns a 64-hex-char opaque token (crypto/rand, house
// pattern — DB-stored, not HMAC-signed).
func generateRSVPToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// viewableMembers returns the campaign members who can VIEW the event — the
// audience for email fan-out and the collection-enabled bell. Visibility-gated
// so a hidden event's title never reaches a member who couldn't already see it
// (the same leak class the parallel entity-ties lane fixes). Owner/co-DM see
// everything; others go through canUserView.
func (s *rsvpService) viewableMembers(ctx context.Context, evt *Event, campaignID string) ([]campaigns.CampaignMember, error) {
	if s.members == nil {
		return nil, nil
	}
	all, err := s.members.ListMembers(ctx, campaignID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	out := make([]campaigns.CampaignMember, 0, len(all))
	for _, m := range all {
		role := int(m.Role)
		if permissions.CanSeeDmOnly(role) || canUserView(evt.Visibility, evt.VisibilityRules, role, m.UserID) {
			out = append(out, m)
		}
	}
	return out, nil
}

// notifyOwnerOfResponse rings the event owner's bell when a member responds
// (owner-only, best-effort — a notifier error never fails the RSVP). Skips the
// self-notify case where the owner RSVPs to their own event.
func (s *rsvpService) notifyOwnerOfResponse(ctx context.Context, eventID, responderID, status string) {
	if s.notifier == nil {
		return
	}
	evt, err := s.events.GetEvent(ctx, eventID)
	if err != nil || evt == nil || evt.CreatedBy == nil || *evt.CreatedBy == "" || *evt.CreatedBy == responderID {
		return
	}
	cal, err := s.events.GetCalendarByID(ctx, evt.CalendarID)
	if err != nil || cal == nil {
		return
	}
	msg := fmt.Sprintf("A member responded %q to %q", status, evt.Name)
	link := eventCalendarLink(cal.CampaignID, cal.ID)
	if err := s.notifier.NotifyUsers(ctx, []string{*evt.CreatedBy}, cal.CampaignID, NotifCalendarRSVP, msg, link); err != nil {
		slog.Warn("rsvp owner notify failed", slog.Any("error", err), slog.String("event_id", eventID))
	}
}

// eventCalendarLink is the in-app URL an RSVP notification points to (the V2
// calendar shell for the event's calendar).
func eventCalendarLink(campaignID, calendarID string) string {
	return fmt.Sprintf("/campaigns/%s/calendar/v2/%s", campaignID, calendarID)
}

// escapeForEmail escapes an operator-authored string so it can't inject markup
// into an HTML email body (mirrors the sessions invite-email hardening).
func escapeForEmail(s string) string { return html.EscapeString(s) }

// EnableCollection flips the per-event opt-in on, then fans out invite emails +
// bell notifications to every member who can VIEW the event. All fan-out is
// best-effort and nil-safe: the toggle persists even with no SMTP / no notifier.
func (s *rsvpService) EnableCollection(ctx context.Context, eventID string) error {
	evt, err := s.events.GetEvent(ctx, eventID)
	if err != nil {
		return err
	}
	if evt == nil {
		return apperror.NewNotFound("event not found")
	}
	cal, err := s.events.GetCalendarByID(ctx, evt.CalendarID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if cal == nil {
		return apperror.NewNotFound("calendar not found")
	}
	if err := s.repo.SetCollectRSVPs(ctx, eventID, true); err != nil {
		return apperror.NewInternal(err)
	}

	members, err := s.viewableMembers(ctx, evt, cal.CampaignID)
	if err != nil {
		return err
	}
	// Bell: tell every viewable member RSVPs just opened.
	if s.notifier != nil && len(members) > 0 {
		ids := make([]string, 0, len(members))
		for _, m := range members {
			ids = append(ids, m.UserID)
		}
		msg := fmt.Sprintf("RSVPs are open for %q", evt.Name)
		if err := s.notifier.NotifyUsers(ctx, ids, cal.CampaignID, NotifCalendarRSVP, msg, eventCalendarLink(cal.CampaignID, cal.ID)); err != nil {
			slog.Warn("rsvp collection-enabled notify failed", slog.Any("error", err), slog.String("event_id", eventID))
		}
	}
	// Email: per-recipient single-use token links (skipped entirely when SMTP
	// is unconfigured — in-app RSVP still works).
	s.fanOutInviteEmails(ctx, evt, cal, members)
	return nil
}

// fanOutInviteEmails sends one invite email per viewable member with per-action
// single-use token links. Members without an email are skipped. A per-recipient
// token-creation or send failure is logged and skipped — one bad address never
// blocks the rest.
func (s *rsvpService) fanOutInviteEmails(ctx context.Context, evt *Event, cal *Calendar, members []campaigns.CampaignMember) {
	if s.mailer == nil || !s.mailer.IsConfigured(ctx) {
		return
	}
	dateLine := fantasyDateLabel(evt, cal)
	expires := s.clockNow().UTC().Add(rsvpTokenTTLDays * 24 * time.Hour)
	actions := []string{RSVPActionYes, RSVPActionMaybe, RSVPActionNo, RSVPActionOutWeek, RSVPActionSuggest}
	for _, m := range members {
		if m.Email == "" {
			continue
		}
		tokens := make(map[string]string, len(actions))
		for _, action := range actions {
			tok := generateRSVPToken()
			if err := s.repo.CreateToken(ctx, evt.ID, m.UserID, action, tok, expires); err != nil {
				slog.Warn("rsvp token create failed", slog.Any("error", err), slog.String("user_id", m.UserID), slog.String("action", action))
				continue
			}
			tokens[action] = tok
		}
		subject, plain, htmlBody := s.composeInviteEmail(evt, cal, dateLine, tokens)
		if err := s.mailer.SendHTMLMail(ctx, []string{m.Email}, subject, plain, htmlBody); err != nil {
			slog.Warn("rsvp invite email failed", slog.Any("error", err), slog.String("to", m.Email), slog.String("event_id", evt.ID))
		}
	}
}

// composeInviteEmail builds the subject + plain + HTML bodies for an RSVP invite.
// Structure reuses the sessions invite email; five action buttons map to the
// five token verbs. All operator-authored values are escaped; the token URLs are
// same-origin hex paths.
func (s *rsvpService) composeInviteEmail(evt *Event, cal *Calendar, dateLine string, tokens map[string]string) (subject, plain, htmlBody string) {
	url := func(action string) string {
		if t := tokens[action]; t != "" {
			return fmt.Sprintf("%s/calendar-rsvp/%s", s.baseURL, t)
		}
		return ""
	}
	name := evt.Name
	subject = fmt.Sprintf("RSVP: %s — %s", name, cal.Name)

	plain = fmt.Sprintf(`You're invited to respond to a calendar event.

Event: %s
When: %s
Calendar: %s

Going:            %s
Maybe:            %s
Can't make it:    %s
Out this week:    %s
Suggest a time:   %s

These links expire in %d days.
`, name, dateLine, cal.Name,
		url(RSVPActionYes), url(RSVPActionMaybe), url(RSVPActionNo),
		url(RSVPActionOutWeek), url(RSVPActionSuggest), rsvpTokenTTLDays)

	btn := func(action, bg, label string) string {
		u := url(action)
		if u == "" {
			return ""
		}
		return fmt.Sprintf(`<a href="%s" style="display:inline-block;padding:9px 18px;background:%s;color:#fff;text-decoration:none;border-radius:6px;font-weight:600;margin:4px">%s</a>`,
			u, bg, label)
	}
	htmlBody = fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="utf-8"></head><body style="font-family:system-ui,-apple-system,sans-serif;max-width:480px;margin:0 auto;padding:20px;color:#333">
<div style="text-align:center;margin-bottom:24px">
  <div style="font-size:32px;margin-bottom:8px">📅</div>
  <h1 style="font-size:20px;margin:0">You're invited to RSVP</h1>
</div>
<div style="background:#f8f9fa;border-radius:8px;padding:20px;margin-bottom:20px">
  <h2 style="font-size:16px;margin:0 0 8px">%s</h2>
  <p style="margin:4px 0;color:#666;font-size:14px"><strong>When:</strong> %s</p>
  <p style="margin:4px 0;color:#666;font-size:14px"><strong>Calendar:</strong> %s</p>
</div>
<div style="text-align:center;margin-bottom:16px">
  %s%s%s
</div>
<div style="text-align:center;margin-bottom:20px">
  %s%s
</div>
<p style="text-align:center;color:#999;font-size:12px">These links expire in %d days.</p>
</body></html>`,
		escapeForEmail(name), escapeForEmail(dateLine), escapeForEmail(cal.Name),
		btn(RSVPActionYes, "#22c55e", "✓ Going"),
		btn(RSVPActionMaybe, "#f59e0b", "? Maybe"),
		btn(RSVPActionNo, "#ef4444", "✗ Can't make it"),
		btn(RSVPActionOutWeek, "#64748b", "✈ Out this week"),
		btn(RSVPActionSuggest, "#6366f1", "✎ Suggest a time"),
		rsvpTokenTTLDays)
	return subject, plain, htmlBody
}

// ValidateToken is the read-only GET-confirm check: the token must exist, be
// unused, and be unexpired. No state change (a mail scanner's prefetch GET must
// never record a response).
func (s *rsvpService) ValidateToken(ctx context.Context, tokenStr string) (*EventRSVPToken, error) {
	t, err := s.repo.GetToken(ctx, tokenStr)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if t == nil {
		return nil, apperror.NewNotFound("this RSVP link is invalid")
	}
	if t.UsedAt != nil {
		return nil, apperror.NewBadRequest("this RSVP link has already been used")
	}
	if s.clockNow().After(t.ExpiresAt) {
		return nil, apperror.NewBadRequest("this RSVP link has expired")
	}
	return t, nil
}

// ApplyToken consumes a token and records the response (POST only). Consume-
// first (scoped to the still-unused row) closes the double-submit window; then
// it upserts the RSVP and runs the action-specific side effects.
func (s *rsvpService) ApplyToken(ctx context.Context, tokenStr string, note *string) (*EventRSVPToken, error) {
	t, err := s.ValidateToken(ctx, tokenStr)
	if err != nil {
		return nil, err
	}
	if err := s.repo.MarkTokenUsed(ctx, t.ID); err != nil {
		return nil, apperror.NewInternal(err)
	}
	// Reuse the shared apply path (note is the free-text for "suggest").
	if err := s.applyAction(ctx, t.EventID, t.UserID, t.Action, note); err != nil {
		return nil, err
	}
	return t, nil
}

// applyOutThisWeek writes full-day (0–1440) 'unavailable' exceptions for the
// acting user across the current real week, skipping any day that already
// carries a hand-authored exception (mirrors static/js/availability.js:1086+).
// SELF-write only — userID comes from the redeemed token. Best-effort: an
// availability error never fails the RSVP the token already recorded. On a read
// failure it writes nothing (never clobber days it can't see).
func (s *rsvpService) applyOutThisWeek(ctx context.Context, eventID, userID string) {
	if s.availability == nil {
		return
	}
	evt, err := s.events.GetEvent(ctx, eventID)
	if err != nil || evt == nil {
		return
	}
	cal, err := s.events.GetCalendarByID(ctx, evt.CalendarID)
	if err != nil || cal == nil {
		return
	}
	existing, err := s.availability.ListMyExceptionDates(ctx, cal.CampaignID, userID)
	if err != nil {
		slog.Warn("out-this-week: could not read existing exceptions; skipping write", slog.Any("error", err), slog.String("user_id", userID))
		return
	}
	for _, iso := range realWeekDates(s.clockNow()) {
		if existing[iso] {
			continue // hand-authored day — leave it untouched (the key pin)
		}
		if err := s.availability.AddFullDayUnavailable(ctx, cal.CampaignID, userID, iso); err != nil {
			slog.Warn("out-this-week: add exception failed", slog.Any("error", err), slog.String("user_id", userID), slog.String("date", iso))
		}
	}
}

// notifyOwnerOfSuggestion rings the event owner's bell for a "suggest another
// time" response, carrying a truncated preview of the note (best-effort).
func (s *rsvpService) notifyOwnerOfSuggestion(ctx context.Context, eventID, responderID string, note *string) {
	if s.notifier == nil {
		return
	}
	evt, err := s.events.GetEvent(ctx, eventID)
	if err != nil || evt == nil || evt.CreatedBy == nil || *evt.CreatedBy == "" {
		return
	}
	cal, err := s.events.GetCalendarByID(ctx, evt.CalendarID)
	if err != nil || cal == nil {
		return
	}
	preview := ""
	if note != nil && *note != "" {
		preview = ": " + truncateNote(*note)
	}
	msg := fmt.Sprintf("A member suggested another time for %q%s", evt.Name, preview)
	if err := s.notifier.NotifyUsers(ctx, []string{*evt.CreatedBy}, cal.CampaignID, NotifCalendarRSVP, msg, eventCalendarLink(cal.CampaignID, cal.ID)); err != nil {
		slog.Warn("rsvp suggestion notify failed", slog.Any("error", err), slog.String("event_id", eventID))
	}
}

// fantasyDateLabel renders an event's in-world date using the calendar's month
// names + epoch when available, falling back to numeric fields. Appends the
// wall-clock time when the event has one.
func fantasyDateLabel(evt *Event, cal *Calendar) string {
	var label string
	if cal != nil && evt.Month >= 1 && evt.Month <= len(cal.Months) {
		label = fmt.Sprintf("%s %d, Year %d", cal.Months[evt.Month-1].Name, evt.Day, evt.Year)
	} else {
		label = fmt.Sprintf("Year %d, Month %d, Day %d", evt.Year, evt.Month, evt.Day)
	}
	if cal != nil && cal.EpochName != nil && *cal.EpochName != "" {
		label += " " + *cal.EpochName
	}
	if evt.HasTime() {
		label += fmt.Sprintf(" · %02d:%02d", *evt.StartHour, *evt.StartMinute)
	}
	return label
}

// realWeekDates returns the seven YYYY-MM-DD dates of the UTC week (Mon–Sun)
// containing now — the "this week" the availability.js one-click anchors on.
func realWeekDates(now time.Time) []string {
	d := now.UTC()
	offset := (int(d.Weekday()) + 6) % 7 // days since Monday (Go Sunday=0)
	monday := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -offset)
	out := make([]string, 7)
	for i := 0; i < 7; i++ {
		out[i] = monday.AddDate(0, 0, i).Format("2006-01-02")
	}
	return out
}

// truncateNote clips a free-text note to a bell-friendly length.
func truncateNote(s string) string {
	const max = 120
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
