package calendar

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// errRSVPTokenSpent is the sentinel MarkRSVPTokenUsed returns when the atomic
// "consume if unused" UPDATE matched nothing — i.e. another request already
// redeemed this link. Kept unexported and translated at the service boundary so
// the public route only ever emits the generic invalid/expired message.
var errRSVPTokenSpent = errors.New("rsvp token already used")

// RSVPService owns event-RSVP business logic.
//
// A SEPARATE service from CalendarService by dispatch (C-CAL-RSVP-P1 "code
// isolation"). It reads the calendar aggregate through the narrow
// rsvpEventLookup below rather than depending on CalendarService, so this lane
// adds nothing to the interfaces the syncapi stub and other calendar lanes
// mirror.
type RSVPService interface {
	// EventContext resolves an event + its calendar, verifying the event belongs
	// to campaignID. Used by every caller to close the IDOR before doing anything.
	EventContext(ctx context.Context, eventID, campaignID string) (*Event, *Calendar, error)

	// SetMyRSVP records the CALLER'S OWN answer. userID is never a request
	// parameter at any call site — it comes from the session or a redeemed token.
	SetMyRSVP(ctx context.Context, evt *Event, userID string, role int, req SetRSVPRequest) error

	// SuggestTime attaches the caller's own free-text suggestion WITHOUT
	// changing their status. Same gates as SetMyRSVP.
	SuggestTime(ctx context.Context, evt *Event, userID string, role int, note string) error

	// Summary builds the counts (+ optional per-person detail) for one event.
	Summary(ctx context.Context, evt *Event, userID string, role int, includeDetail bool) (*EventRSVPSummary, error)

	// SetCollection flips the per-event opt-in (Scribe+ at the route).
	SetCollection(ctx context.Context, eventID string, enabled bool) error

	// MintActionTokens creates one single-use link per emailed action for a
	// recipient, returned as action → token.
	MintActionTokens(ctx context.Context, eventID, userID string) (map[string]string, error)

	// ValidateToken resolves a token WITHOUT consuming it — the GET confirm
	// interstitial's pure read, so a mail scanner's prefetch changes nothing.
	ValidateToken(ctx context.Context, token string) (*EventRSVPToken, error)

	// ApplyToken consumes a token and writes its effect. note is used only by
	// the "suggest another time" action and ignored otherwise.
	ApplyToken(ctx context.Context, token, note string) (*EventRSVPToken, error)

	// CanUserViewEvent is the shared visibility predicate the handler needs for
	// the email fan-out (never email a hidden event's title) and the token flow.
	CanUserViewEvent(evt *Event, role int, userID string) bool
}

// rsvpEventLookup is the narrow slice of the calendar aggregate the RSVP service
// reads. *calendarRepo already satisfies it, so wiring passes the same repo
// without widening CalendarRepository (which a hand-written ~60-method mock and
// the syncapi stub both mirror).
type rsvpEventLookup interface {
	GetEvent(ctx context.Context, id string) (*Event, error)
	GetByID(ctx context.Context, id string) (*Calendar, error)
}

// rsvpService is the concrete RSVPService.
type rsvpService struct {
	repo   RSVPRepository
	events rsvpEventLookup
}

// NewRSVPService creates the RSVP service.
func NewRSVPService(repo RSVPRepository, events rsvpEventLookup) RSVPService {
	return &rsvpService{repo: repo, events: events}
}

// EventContext loads the event and its calendar and asserts the calendar belongs
// to campaignID. Mirrors handler.requireEventInCampaign — the same IDOR close
// every other per-event calendar endpoint uses — but lives on the service so the
// public token route (which has no campaign in its path) can reach it too.
func (s *rsvpService) EventContext(ctx context.Context, eventID, campaignID string) (*Event, *Calendar, error) {
	evt, err := s.events.GetEvent(ctx, eventID)
	if err != nil {
		return nil, nil, fmt.Errorf("get event: %w", err)
	}
	if evt == nil {
		return nil, nil, apperror.NewNotFound("event not found")
	}
	cal, err := s.events.GetByID(ctx, evt.CalendarID)
	if err != nil || cal == nil {
		return nil, nil, apperror.NewNotFound("event not found")
	}
	// campaignID == "" means "caller has already established the campaign"
	// (the token flow resolves it FROM the calendar rather than the path).
	if campaignID != "" && cal.CampaignID != campaignID {
		return nil, nil, apperror.NewNotFound("event not found")
	}
	return evt, cal, nil
}

// CanUserViewEvent reuses the calendar's existing event-visibility predicate
// (service.go canUserView) rather than re-deriving one — the C-CAL-ENTITY-TIES-
// LEAK-FIX lesson was that a second, subtly different visibility path is how
// leaks happen. Same package, so no interface widening is needed.
func (s *rsvpService) CanUserViewEvent(evt *Event, role int, userID string) bool {
	if evt == nil {
		return false
	}
	return canUserView(evt.Visibility, evt.VisibilityRules, role, userID)
}

// SetMyRSVP validates and upserts the caller's own answer.
//
// Three gates, all fail-closed:
//  1. the event must be VISIBLE to this member (a dm_only event must not even
//     confirm its existence to a Player via an RSVP write);
//  2. collection must be switched on for the event;
//  3. the status must be a storable ENUM member.
func (s *rsvpService) SetMyRSVP(ctx context.Context, evt *Event, userID string, role int, req SetRSVPRequest) error {
	if evt == nil || userID == "" {
		return apperror.NewNotFound("event not found")
	}
	if !s.CanUserViewEvent(evt, role, userID) {
		// Same shape as a missing event: never disclose that a hidden event exists.
		return apperror.NewNotFound("event not found")
	}
	if !evt.CollectRSVPs {
		return apperror.NewBadRequest("this event is not collecting RSVPs")
	}
	if !ValidRSVPStatus(req.Status) {
		return apperror.NewValidation("status must be one of yes, maybe, no")
	}
	note, err := normalizeRSVPNote(req.Note)
	if err != nil {
		return err
	}
	rec := &EventRSVP{
		ID:        generateRSVPID(),
		EventID:   evt.ID,
		UserID:    userID,
		Status:    req.Status,
		Note:      note,
		UpdatedAt: time.Now().UTC(),
	}
	if err := s.repo.UpsertRSVP(ctx, rec); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

// SuggestTime records the caller's own "these times would work better" note.
//
// Status is deliberately left alone: suggesting an alternative is not itself an
// answer, and silently flipping someone to "no" because they offered a better
// slot would misreport the count to the Director.
func (s *rsvpService) SuggestTime(ctx context.Context, evt *Event, userID string, role int, note string) error {
	if evt == nil || userID == "" {
		return apperror.NewNotFound("event not found")
	}
	if !s.CanUserViewEvent(evt, role, userID) {
		return apperror.NewNotFound("event not found")
	}
	if !evt.CollectRSVPs {
		return apperror.NewBadRequest("this event is not collecting RSVPs")
	}
	clean, err := normalizeRSVPNote(&note)
	if err != nil {
		return err
	}
	if clean == nil {
		return apperror.NewValidation("please describe the times that would work better")
	}
	if err := s.repo.SetRSVPNote(ctx, evt.ID, userID, *clean); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

// Summary builds the RSVP payload for one event.
//
// Counts are visible to every member who can view the event; the per-person
// Responders list is populated ONLY when includeDetail is true (Owner/co-DM at
// the route), mirroring the scheduler overlay's density-for-all /
// identity-for-the-DM split. The caller's OWN answer is always included — it is
// their own data.
//
// Responders carry raw user IDs here; the handler resolves display names through
// campaigns.MemberLister so this service keeps no campaigns dependency.
func (s *rsvpService) Summary(ctx context.Context, evt *Event, userID string, role int, includeDetail bool) (*EventRSVPSummary, error) {
	if evt == nil {
		return nil, apperror.NewNotFound("event not found")
	}
	if !s.CanUserViewEvent(evt, role, userID) {
		return nil, apperror.NewNotFound("event not found")
	}
	rows, err := s.repo.ListRSVPsForEvent(ctx, evt.ID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	out := &EventRSVPSummary{
		EventID:       evt.ID,
		CollectRSVPs:  evt.CollectRSVPs,
		IncludeDetail: includeDetail,
	}
	for i := range rows {
		switch rows[i].Status {
		case RSVPYes:
			out.Counts.Yes++
		case RSVPMaybe:
			out.Counts.Maybe++
		case RSVPNo:
			out.Counts.No++
		}
		if rows[i].UserID == userID {
			out.MyStatus = rows[i].Status
			out.MyNote = rows[i].Note
		}
		if includeDetail {
			out.Responders = append(out.Responders, RSVPResponder{
				UserID: rows[i].UserID,
				Status: rows[i].Status,
				Note:   rows[i].Note,
			})
		}
	}
	return out, nil
}

// SetCollection flips the per-event opt-in.
func (s *rsvpService) SetCollection(ctx context.Context, eventID string, enabled bool) error {
	if err := s.repo.SetCollectRSVPs(ctx, eventID, enabled); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

// normalizeRSVPNote trims, length-checks, and nils-out an empty note. Returning
// nil (rather than an empty string) matters: UpsertRSVP's COALESCE(VALUES(note),
// note) treats NULL as "leave the existing note alone".
func normalizeRSVPNote(in *string) (*string, error) {
	if in == nil {
		return nil, nil
	}
	n := strings.TrimSpace(*in)
	if n == "" {
		return nil, nil
	}
	if len([]rune(n)) > maxRSVPNoteLen {
		return nil, apperror.NewValidation(fmt.Sprintf("note must be %d characters or fewer", maxRSVPNoteLen))
	}
	return &n, nil
}

// generateRSVPID mints a v4 UUID for an RSVP row. Local to this lane rather than
// shared with the calendar service's generator so the file set stays disjoint.
func generateRSVPID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// generateRSVPToken mints a 64-char hex opaque token (32 bytes of crypto/rand),
// matching the sessions token width so the CHAR(64) column is exact.
func generateRSVPToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// --- Emailed action tokens ---

// rsvpEmailActions is the ordered action set every recipient's email carries.
// Ordering is the render order of the buttons, so it lives with the data.
var rsvpEmailActions = []string{
	RSVPActionYes,
	RSVPActionMaybe,
	RSVPActionNo,
	RSVPActionOutWeek,
	RSVPActionSuggest,
}

// MintActionTokens creates one single-use link per action for one recipient.
//
// Per-ACTION tokens (rather than one token plus a chosen action) are what make
// the emailed buttons safe: the link itself encodes the intent, so redeeming it
// cannot be steered by anything the recipient sends, and a leaked link can do
// exactly one thing, once.
func (s *rsvpService) MintActionTokens(ctx context.Context, eventID, userID string) (map[string]string, error) {
	now := time.Now().UTC()
	expires := now.Add(rsvpTokenTTL)
	out := make(map[string]string, len(rsvpEmailActions))
	for _, action := range rsvpEmailActions {
		tok := generateRSVPToken()
		if err := s.repo.CreateRSVPToken(ctx, &EventRSVPToken{
			Token:     tok,
			EventID:   eventID,
			UserID:    userID,
			Action:    action,
			ExpiresAt: expires,
			CreatedAt: now,
		}); err != nil {
			return nil, apperror.NewInternal(err)
		}
		out[action] = tok
	}
	return out, nil
}

// ValidateToken resolves + checks a token WITHOUT consuming it.
//
// Every failure mode — unknown, spent, expired — returns the SAME message. The
// route is unauthenticated, so distinguishing them would turn it into an oracle
// for whether a given token string ever existed.
func (s *rsvpService) ValidateToken(ctx context.Context, token string) (*EventRSVPToken, error) {
	if token == "" {
		return nil, apperror.NewBadRequest(rsvpBadTokenMsg)
	}
	t, err := s.repo.FindRSVPToken(ctx, token)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if t == nil || t.UsedAt != nil || time.Now().UTC().After(t.ExpiresAt) {
		return nil, apperror.NewBadRequest(rsvpBadTokenMsg)
	}
	return t, nil
}

// rsvpBadTokenMsg is the single user-facing message for every token failure.
const rsvpBadTokenMsg = "this RSVP link is invalid or has expired"

// ApplyToken consumes a token and writes its effect. State-changing, so it only
// ever runs from the POST route (the GET half is ValidateToken) — a mail
// scanner prefetching the link records nothing.
//
// Consumption happens FIRST: MarkRSVPTokenUsed's `used_at IS NULL` predicate is
// the atomic winner-takes-it, so two concurrent submits of the same link apply
// the action exactly once.
//
// The availability side-effect of "out this week" is NOT performed here — it
// needs cross-plugin wiring the service has no edge to. ApplyToken records the
// decline and returns the token; the handler performs the availability write.
func (s *rsvpService) ApplyToken(ctx context.Context, token, note string) (*EventRSVPToken, error) {
	t, err := s.ValidateToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if err := s.repo.MarkRSVPTokenUsed(ctx, t.Token); err != nil {
		if errors.Is(err, errRSVPTokenSpent) {
			return nil, apperror.NewBadRequest(rsvpBadTokenMsg)
		}
		return nil, apperror.NewInternal(err)
	}

	if t.Action == RSVPActionSuggest {
		clean, err := normalizeRSVPNote(&note)
		if err != nil {
			return nil, err
		}
		if clean == nil {
			return nil, apperror.NewValidation("please describe the times that would work better")
		}
		if err := s.repo.SetRSVPNote(ctx, t.EventID, t.UserID, *clean); err != nil {
			return nil, apperror.NewInternal(err)
		}
		return t, nil
	}

	status := statusForAction(t.Action)
	if status == "" {
		return nil, apperror.NewBadRequest(rsvpBadTokenMsg)
	}
	if err := s.repo.UpsertRSVP(ctx, &EventRSVP{
		ID:        generateRSVPID(),
		EventID:   t.EventID,
		UserID:    t.UserID,
		Status:    status,
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		return nil, apperror.NewInternal(err)
	}
	return t, nil
}

// --- "Out this week" week resolution ---

// rsvpWeekDates returns the Monday-anchored ISO week (7 dates, Mon..Sun) that
// "out this week" should block, plus its Monday.
//
// Availability exceptions are REAL-WORLD dates, but a calendar event carries a
// date in its own (possibly fantasy) reckoning, so the two only line up when the
// calendar tracks real time — then Year/Month/Day IS the proleptic Gregorian
// date (Calendar.UsesRealTime, C-REAL-CALENDAR-P1). For a fantasy calendar there
// is no derivable real week, so we fall back to the week containing the moment
// of redemption — exactly what the scheduler's own "Out this week" button means
// (static/js/availability.js mondayOf(todayUTC())).
//
// The caller SHOWS the resolved week back to the member, so the action can never
// silently block a week they didn't intend.
func rsvpWeekDates(cal *Calendar, evt *Event, now time.Time) (monday string, dates []string) {
	anchor := now.UTC()
	if cal != nil && evt != nil && cal.UsesRealTime() {
		anchor = time.Date(evt.Year, time.Month(evt.Month), evt.Day, 0, 0, 0, 0, time.UTC)
	}
	anchor = time.Date(anchor.Year(), anchor.Month(), anchor.Day(), 0, 0, 0, 0, time.UTC)
	// (weekday + 6) % 7 = days since Monday, mirroring availability.js mondayOf.
	off := (int(anchor.Weekday()) + 6) % 7
	start := anchor.AddDate(0, 0, -off)
	dates = make([]string, 0, 7)
	for i := 0; i < 7; i++ {
		dates = append(dates, start.AddDate(0, 0, i).Format("2006-01-02"))
	}
	return start.Format("2006-01-02"), dates
}
