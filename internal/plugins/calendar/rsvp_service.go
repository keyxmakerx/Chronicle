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

	// --- schedule-ask rate limit, the two PERSISTED layers (C-CALV4-RSVP-P8B) ---

	// ScheduleAskState is the per-campaign cooldown, read for the send AND for
	// the Bench's "last asked" readout — one predicate, so the control and the
	// endpoint can never disagree about whether an ask is allowed.
	ScheduleAskState(ctx context.Context, campaignID string) (ScheduleAskState, error)

	// RecentlyAskedRecipients is the per-recipient floor: the members this
	// campaign has emailed inside the floor window, as a skip set. It SKIPS
	// those members rather than refusing the send, so a second ask after
	// somebody joins mails the new member and nobody else.
	RecentlyAskedRecipients(ctx context.Context, campaignID string) (map[string]bool, error)

	// RecordScheduleAsk appends one send row, called by the fan-out goroutine as
	// each send succeeds — never optimistically up front.
	RecordScheduleAsk(ctx context.Context, campaignID, eventID, recipientUserID, actorUserID string) error

	// ValidateToken resolves a token WITHOUT consuming it — the GET confirm
	// interstitial's pure read, so a mail scanner's prefetch changes nothing.
	ValidateToken(ctx context.Context, token string) (*EventRSVPToken, error)

	// AnsweredToken resolves a token that is ALREADY SPENT, together with the
	// answer standing on record for it (C-CALV4-GAMEREADY §5 [GR-8]).
	//
	// It is the read behind "you've already answered — you're down as Going",
	// and it exists because ValidateToken cannot serve it: ValidateToken's whole
	// contract is that spent, expired and never-existed are INDISTINGUISHABLE,
	// which is right for a link that may still be redeemable and wrong for one
	// the member has already used. Both halves must be present — a spent token
	// with no stored answer is not "already answered", it is a link whose write
	// did not happen, and that is the generic page's business.
	//
	// IT AUTHORISES NOTHING. The handler still runs the SAME membership and
	// event-visibility re-check resolveToken runs before it prints a word; this
	// method only reports what the two rows say.
	AnsweredToken(ctx context.Context, token string) (*EventRSVPToken, *EventRSVP, error)

	// ApplyToken consumes a token and writes the RSVP status it implies.
	//
	// The two richer actions' side-effects are NOT applied here — "out this
	// week" needs the scheduler and "suggest another time" needs the submitted
	// windows/note, neither of which the service has an edge to. It consumes the
	// token and reports which action it carried; the handler does the rest.
	ApplyToken(ctx context.Context, token string) (*EventRSVPToken, error)

	// CanUserViewEvent is the shared visibility predicate the handler needs for
	// the email fan-out (never email a hidden event's title) and the token flow.
	CanUserViewEvent(evt *Event, role int, userID string) bool

	// AnswersByUser returns EVERY stored answer to one event, keyed by user id.
	//
	// WHY THIS IS NOT Summary's Responders (C-CALV4-RSVP-P8 §4). Responders is
	// Owner/co-DM only, because the JSON API's audience question is "who may see
	// the breakdown of a count". The Bench RSVP panel answers a DIFFERENT and
	// separately signed question, and the signed contract answers it directly:
	// rsvpPanel() renders the availability lanes under `${GM ? lanes : ''}` but
	// renders the full member table — every member's role, zone, local clock and
	// ANSWER — unconditionally, and v4-bench-player-light.png shows a player
	// receiving exactly that. The law:
	//
	//	RSVP answers, roles, zones and per-member local clocks are
	//	party-visible. Per-member availability LANES are owner / co-DM only.
	//	The aggregate density row is everyone's.
	//
	// So: statuses for everyone, and NOTES FOR NOBODY. A free-text "these times
	// work better" note is not covered by that ruling, is the most personal
	// thing in the table, and the Bench has nowhere to print it — it stays on
	// Summary, behind the detail gate.
	//
	// The event-visibility gate is unchanged: a caller who cannot see the event
	// gets not-found, exactly as Summary does, so an RSVP read still cannot
	// confirm a hidden event's existence.
	AnswersByUser(ctx context.Context, evt *Event, userID string, role int) (map[string]string, error)
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

// AnswersByUser returns every stored answer to the event, keyed by user id. See
// the interface doc for why the audience is the whole party and why notes are
// excluded.
//
// The returned map is the RAW STORED SET, ex-members included. That is
// deliberate: filtering by membership is the CALLER'S job, because the caller is
// the one that knows which roster it is printing beside, and a store-side filter
// would hide the very disagreement the Bench's count oracle exists to catch —
// a stored row belonging to somebody who has left the campaign.
func (s *rsvpService) AnswersByUser(ctx context.Context, evt *Event, userID string, role int) (map[string]string, error) {
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
	out := make(map[string]string, len(rows))
	for i := range rows {
		out[rows[i].UserID] = rows[i].Status
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

// AnsweredToken reports a SPENT token plus the answer already on record.
//
// THE NARROWNESS IS THE SAFETY. Three conditions must ALL hold, and a miss on
// any of them returns the same not-found the generic page is built on:
//
//	the token row exists            — a guessed 64-hex string resolves nothing
//	used_at IS NOT NULL             — a LIVE token is ValidateToken's business,
//	                                  and answering here would let a redeemable
//	                                  link be read without being redeemed
//	the (event, user) row exists    — "spent with nothing written" is a failure,
//	                                  not an answer, and must not claim otherwise
//
// Expiry is deliberately NOT re-checked: a spent link is spent, and the answer
// it recorded is just as true a week later. What the member is told is a fact
// about their own row, not a permission to do anything.
func (s *rsvpService) AnsweredToken(ctx context.Context, token string) (*EventRSVPToken, *EventRSVP, error) {
	if token == "" {
		return nil, nil, apperror.NewBadRequest(rsvpBadTokenMsg)
	}
	t, err := s.repo.FindRSVPToken(ctx, token)
	if err != nil {
		return nil, nil, apperror.NewInternal(err)
	}
	if t == nil || t.UsedAt == nil {
		return nil, nil, apperror.NewBadRequest(rsvpBadTokenMsg)
	}
	answer, err := s.repo.GetUserRSVP(ctx, t.EventID, t.UserID)
	if err != nil {
		return nil, nil, apperror.NewInternal(err)
	}
	if answer == nil {
		return nil, nil, apperror.NewBadRequest(rsvpBadTokenMsg)
	}
	return t, answer, nil
}

// ApplyToken consumes a token and writes its effect. State-changing, so it only
// ever runs from the POST route (the GET half is ValidateToken) — a mail
// scanner prefetching the link records nothing.
//
// Consumption happens FIRST: MarkRSVPTokenUsed's `used_at IS NULL` predicate is
// the atomic winner-takes-it, so two concurrent submits of the same link apply
// the action exactly once.
//
// Side-effects the service has no edge to are NOT performed here: "out this
// week" needs the scheduler, and "suggest another time" needs the note and
// windows the member submitted with the POST. Both are the handler's job, so
// this consumes the token, writes the status the action implies (if any), and
// reports which action it carried.
//
// "suggest" carries NO status — offering an alternative is not itself an answer,
// and silently flipping someone to "no" because they proposed a better slot
// would misreport the count to the Director. It returns early, before the status
// write, with the token consumed.
func (s *rsvpService) ApplyToken(ctx context.Context, token string) (*EventRSVPToken, error) {
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

	status := statusForAction(t.Action)
	if status == "" {
		if t.Action == RSVPActionSuggest {
			return t, nil
		}
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

// --- the schedule ask's persisted rate limit (C-CALV4-RSVP-P8B, [PB-4]) -----

// scheduleAskCampaignCooldown is the minimum gap between two asking sends to
// the SAME campaign. Six hours: long enough that a table cannot be mailed twice
// in one evening's fiddling, short enough that a Director who genuinely needs
// to re-ask after a scheduling collapse can do so the same day.
const scheduleAskCampaignCooldown = 6 * time.Hour

// scheduleAskRecipientFloor is the minimum gap between two asking emails to the
// same PERSON, across sends. Deliberately longer than the campaign cooldown:
// the campaign limit protects the roster from a burst, this one protects an
// individual inbox from being the collateral of a legitimate re-ask.
const scheduleAskRecipientFloor = 24 * time.Hour

// ScheduleAskState is everything the caller needs to decide — and to EXPLAIN —
// whether this campaign may be asked right now.
//
// It exists because a limit whose only expression is an error page is a limit
// the operator hits blind. The Bench renders the same three fields as a
// sentence ("Asked 2 hours ago. You can ask again in 4 hours.") and disables
// the control, so the refusal is visible before the click rather than after it.
type ScheduleAskState struct {
	// LastAskedAt is when this campaign's roster was last mailed. The ZERO
	// VALUE means nobody has ever been asked, which is a different fact from
	// "asked a long time ago" and is printed differently.
	LastAskedAt time.Time
	// Ready is whether a send is permitted now.
	Ready bool
	// RetryAfter is how long until Ready flips true. Zero when Ready.
	RetryAfter time.Duration
}

// ScheduleAskState reads the campaign cooldown.
//
// FAIL-CLOSED. A read failure refuses the send rather than falling through to
// "askable": if we cannot tell whether this roster was mailed twenty minutes
// ago, an unretractable email is the wrong thing to guess about.
func (s *rsvpService) ScheduleAskState(ctx context.Context, campaignID string) (ScheduleAskState, error) {
	last, err := s.repo.LastScheduleAskAt(ctx, campaignID)
	if err != nil {
		return ScheduleAskState{}, apperror.NewInternal(err)
	}
	st := ScheduleAskState{LastAskedAt: last}
	if last.IsZero() {
		st.Ready = true
		return st, nil
	}
	if elapsed := time.Since(last); elapsed < scheduleAskCampaignCooldown {
		st.RetryAfter = scheduleAskCampaignCooldown - elapsed
		return st, nil
	}
	st.Ready = true
	return st, nil
}

// RecentlyAskedRecipients reads the per-recipient floor as a skip set.
//
// The window is measured back from NOW, not from the campaign's last send: the
// question is "has this person heard from us lately", and it has to give the
// same answer whether or not somebody else was mailed in between.
func (s *rsvpService) RecentlyAskedRecipients(ctx context.Context, campaignID string) (map[string]bool, error) {
	ids, err := s.repo.ScheduleAskRecipientsSince(ctx, campaignID,
		time.Now().UTC().Add(-scheduleAskRecipientFloor))
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	skip := make(map[string]bool, len(ids))
	for _, id := range ids {
		skip[id] = true
	}
	return skip, nil
}

// RecordScheduleAsk appends one send row.
//
// eventID is optional — "" records an ask that mentioned no session, which is
// the normal case in a campaign that has scheduled nothing. The campaign and
// the recipient are not optional: a row that cannot say who was mailed is not
// bookkeeping, it is a cooldown with no subject, so it is refused here rather
// than written and puzzled over later.
func (s *rsvpService) RecordScheduleAsk(ctx context.Context, campaignID, eventID, recipientUserID, actorUserID string) error {
	if campaignID == "" || recipientUserID == "" {
		return apperror.NewInternal(fmt.Errorf("schedule ask row needs a campaign and a recipient"))
	}
	row := &ScheduleAsk{
		ID:              generateRSVPID(),
		CampaignID:      campaignID,
		RecipientUserID: recipientUserID,
		ActorUserID:     actorUserID,
		SentAt:          time.Now().UTC(),
	}
	if eventID != "" {
		row.EventID = &eventID
	}
	if err := s.repo.RecordScheduleAsk(ctx, row); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}
