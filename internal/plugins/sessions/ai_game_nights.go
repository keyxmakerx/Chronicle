package sessions

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// AIGameNights is the game nights' part of the AI engine (aiplug): the
// `game-night` block an AI can ask to write and the `game-nights` lookup.
// Matched by name, with `date` telling apart two nights of the same name;
// the block's body is the night's recap, which every member can read.
type AIGameNights struct {
	store aiNightStore
}

// NewAIGameNights builds the AI part over the sessions service. members
// lets a new night invite everyone, as the Game nights page does.
func NewAIGameNights(svc SessionService, members campaigns.MemberLister) *AIGameNights {
	return &AIGameNights{store: serviceNights{svc: svc, members: members}}
}

var (
	_ aiplug.Kind    = (*AIGameNights)(nil)
	_ aiplug.Briefed = (*AIGameNights)(nil)
	_ aiplug.Lookup  = (*AIGameNights)(nil)
)

// aiNightStore is the service as the AI part needs it. It keeps the
// service's own rules (statuses, soft delete, recap sanitizing); the AI part
// only hands it IDs it listed for the same campaign.
type aiNightStore interface {
	List(ctx context.Context, campaignID string) ([]aiNight, error)
	// Create makes the night and invites every member; it sends no email.
	Create(ctx context.Context, campaignID string, a aiplug.Actor, in aiNightInput) (string, error)
	Update(ctx context.Context, id string, in aiNightInput) error
	SetRecap(ctx context.Context, id string, recapJSON, recapHTML *string) error
	// Delete moves the night to the restorable trash.
	Delete(ctx context.Context, id string) error
}

// aiNight is a stored game night as the AI sees it.
type aiNight struct {
	ID, Name, Date, Time, Status, Summary string
}

// aiNightInput is the night's details; a nil field is left as it is on update.
type aiNightInput struct {
	Name, Date, Time, Status, Summary *string
}

func (*AIGameNights) Name() string  { return "game-night" }
func (*AIGameNights) Label() string { return "Game night" }
func (*AIGameNights) Brief() string {
	return "game nights (play sessions): the real-world date and time, a one-line summary, played or cancelled, and the recap every member reads"
}
func (*AIGameNights) Doc() string {
	return "A game night (a play session), matched by `name`; when two share a name, `date` picks one. Keys: `date` (the real-world day, YYYY-MM-DD), `time` (24-hour HH:MM), `summary` (one line), `status` (planned, played or cancelled), `rename_to`. The body is the night's recap, which every member of the campaign can read; on update a body replaces the recap and no body keeps it. A new night invites every member, as the Game nights page does, and sends no email. Removing a night moves it to the trash, where the owner can restore it.\n\n```\n---\nkind: game-night\nname: Session 13\ndate: 2026-10-24\ntime: \"19:30\"\nsummary: The siege of Harrowmere\n---\n```\n\n```\n---\nkind: game-night\naction: update\nname: Session 12\nstatus: played\n---\nThe party broke the siege at dawn and Corvin lost his shield.\n```"
}

// nightStatus maps the words an AI may write onto the stored statuses.
var nightStatus = map[string]string{
	"planned": StatusPlanned, "scheduled": StatusPlanned,
	"played": StatusCompleted, "completed": StatusCompleted, "done": StatusCompleted,
	"cancelled": StatusCancelled, "canceled": StatusCancelled,
}

// nightWords is the plain word for a stored status.
var nightWords = map[string]string{StatusPlanned: "planned", StatusCompleted: "played", StatusCancelled: "cancelled"}

// input reads and checks the night's keys; only written keys are set.
func (k *AIGameNights) input(r aiplug.Record) (aiNightInput, error) {
	var in aiNightInput
	if r.Has("date") {
		d := r.Date("date")
		if _, err := time.Parse("2006-01-02", d); err != nil {
			return in, aiplug.BadRequestf("date: %q is not a day written YYYY-MM-DD", d)
		}
		in.Date = &d
	}
	if r.Has("time") {
		t := r.Str("time")
		if _, err := time.Parse("15:04", t); err != nil {
			return in, aiplug.BadRequestf("time: %q is not a 24-hour time written HH:MM", t)
		}
		in.Time = &t
	}
	if r.Has("status") {
		s, ok := nightStatus[strings.ToLower(r.Str("status"))]
		if !ok {
			return in, aiplug.BadRequestf("status: %q is not planned, played or cancelled", r.Str("status"))
		}
		in.Status = &s
	}
	if r.Has("summary") {
		s := r.Str("summary")
		in.Summary = &s
	}
	if s := r.Str("rename_to"); s != "" {
		in.Name = &s
	}
	return in, nil
}

// find is the stored night the record names, or nil. Two nights of the same
// name need `date` to choose.
func (k *AIGameNights) find(ctx context.Context, campaignID string, r aiplug.Record) (*aiNight, error) {
	list, err := k.store.List(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	var hits []aiNight
	for _, n := range list {
		if aiplug.SameName(n.Name, r.Name) {
			hits = append(hits, n)
		}
	}
	if len(hits) > 1 && r.Has("date") {
		d := r.Date("date")
		var same []aiNight
		for _, n := range hits {
			if n.Date == d {
				same = append(same, n)
			}
		}
		hits = same
	}
	switch len(hits) {
	case 0:
		return nil, nil
	case 1:
		return &hits[0], nil
	}
	if r.Has("date") {
		return nil, aiplug.BadRequestf("more than one game night is called %s on %s", aiplug.Quote(r.Name), r.Date("date"))
	}
	return nil, aiplug.BadRequestf("more than one game night is called %s; add `date:` to pick one", aiplug.Quote(r.Name))
}

func nightLine(in aiNightInput, existing *aiNight) string {
	var parts []string
	if in.Date != nil {
		w := "on " + *in.Date
		if in.Time != nil {
			w += " at " + *in.Time
		}
		parts = append(parts, w)
	} else if in.Time != nil {
		parts = append(parts, "at "+*in.Time)
	}
	if in.Status != nil && (existing == nil || existing.Status != *in.Status) {
		parts = append(parts, "marked "+nightWords[*in.Status])
	}
	if in.Summary != nil {
		parts = append(parts, "summary "+aiplug.Quote(*in.Summary))
	}
	if in.Name != nil {
		parts = append(parts, "renamed to "+aiplug.Quote(*in.Name))
	}
	return strings.Join(parts, ", ")
}

// Plan checks a block for the review screen. Game nights are the
// Director's, so only the owner or a co-DM may change them.
func (k *AIGameNights) Plan(ctx context.Context, campaignID string, a aiplug.Actor, r aiplug.Record) aiplug.Plan {
	if !a.CanAuthorDmOnly() {
		return aiplug.Plan{Error: "only the owner or a co-DM can change game nights"}
	}
	if r.Name == "" {
		return aiplug.Plan{Error: "a game night needs a name"}
	}
	in, err := k.input(r)
	if err != nil {
		return aiplug.Plan{Error: aiplug.PlanError(err)}
	}
	hit, err := k.find(ctx, campaignID, r)
	if err != nil {
		return aiplug.Plan{Error: aiplug.PlanError(err)}
	}
	recap := strings.TrimSpace(r.Body) != ""
	switch r.Action {
	case aiplug.ActionCreate:
		if hit != nil && (in.Date == nil || hit.Date == *in.Date) {
			return aiplug.Plan{Error: "a game night called " + aiplug.Quote(r.Name) + " already exists; use action: update"}
		}
		s := nightLine(in, nil)
		if s == "" {
			s = "no date yet"
		}
		s += "; invites every member"
		if recap {
			s += "; with a recap every member can read"
		}
		return aiplug.Plan{Summary: s}
	case aiplug.ActionUpdate:
		if hit == nil {
			return aiplug.Plan{Error: "no game night called " + aiplug.Quote(r.Name)}
		}
		s := nightLine(in, hit)
		if recap {
			if s != "" {
				s += "; "
			}
			s += "replaces the recap, which every member can read"
		}
		if s == "" {
			return aiplug.Plan{Error: "nothing to change: name a key to change, or write the recap below the front matter"}
		}
		return aiplug.Plan{Summary: s}
	default:
		if hit == nil {
			return aiplug.Plan{Error: "no game night called " + aiplug.Quote(r.Name)}
		}
		return aiplug.Plan{Summary: "moves it to the trash, where the owner can restore it"}
	}
}

// Apply writes an approved block, re-planning it first.
func (k *AIGameNights) Apply(ctx context.Context, campaignID string, a aiplug.Actor, r aiplug.Record) error {
	if p := k.Plan(ctx, campaignID, a, r); p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	in, err := k.input(r)
	if err != nil {
		return err
	}
	j, h, err := r.BodyEntry()
	if err != nil {
		return err
	}
	if r.Action == aiplug.ActionCreate {
		name := r.Name
		in.Name = &name
		id, err := k.store.Create(ctx, campaignID, a, in)
		if err != nil || h == nil {
			return err
		}
		return k.store.SetRecap(ctx, id, j, h)
	}
	hit, err := k.find(ctx, campaignID, r)
	if err != nil {
		return err
	}
	if hit == nil {
		return apperror.NewBadRequest("it changed while you were reviewing; check it again")
	}
	if r.Action == aiplug.ActionDelete {
		return k.store.Delete(ctx, hit.ID)
	}
	if in != (aiNightInput{}) {
		if err := k.store.Update(ctx, hit.ID, in); err != nil {
			return err
		}
	}
	if h != nil {
		return k.store.SetRecap(ctx, hit.ID, j, h)
	}
	return nil
}

// Export lists every game night in the block's own format, so an AI can
// update or remove one by name. Recaps are left out: the export's Sessions
// section already carries them.
func (k *AIGameNights) Export(ctx context.Context, campaignID string, a aiplug.Actor) (string, error) {
	if !a.CanAuthorDmOnly() {
		return "", nil
	}
	list, err := k.store.List(ctx, campaignID)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, n := range list {
		b.WriteString("---\nkind: game-night\nname: " + aiplug.Quote(n.Name) + "\n")
		if n.Date != "" {
			b.WriteString("date: " + n.Date + "\n")
		}
		if n.Time != "" {
			b.WriteString("time: " + aiplug.Quote(n.Time) + "\n")
		}
		if w := nightWords[n.Status]; w != "" {
			b.WriteString("status: " + w + "\n")
		}
		if n.Summary != "" {
			b.WriteString("summary: " + aiplug.Quote(n.Summary) + "\n")
		}
		b.WriteString("---\n\n")
	}
	return b.String(), nil
}

// LookupInfo describes `what: game-nights` and its tool.
func (*AIGameNights) LookupInfo() aiplug.LookupInfo {
	return aiplug.LookupInfo{
		What: "game-nights", Aliases: []string{"game-night"}, Keys: "none",
		Brief: "the campaign's game nights: name, date, time, status and summary (owner and co-DM only)",
		Tool: aiplug.Tool{
			Name: "game_nights", Title: "Game nights", Did: "Listed the game nights",
			Desc: "The campaign's game nights: name, date, time, status and summary.",
		},
	}
}

// Answer lists the game nights. They are Director content, as the block
// is, so a player's view gets a refusal.
func (k *AIGameNights) Answer(ctx context.Context, campaignID string, a aiplug.Actor, _ aiplug.Record) (string, string, error) {
	chip := "Game nights"
	if !a.CanAuthorDmOnly() {
		return chip, "", aiplug.BadRequestf("game nights are left out in Safe privacy; choose Permitted to include them")
	}
	list, err := k.store.List(ctx, campaignID)
	if err != nil {
		return chip, "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Game nights (%d)\n\n", len(list))
	var lines []string
	for _, n := range list {
		line := "- " + n.Name
		var parts []string
		if when := strings.TrimSpace(n.Date + " " + n.Time); when != "" {
			parts = append(parts, when)
		}
		if w := nightWords[n.Status]; w != "" {
			parts = append(parts, w)
		}
		if len(parts) > 0 {
			line += ": " + strings.Join(parts, ", ")
		}
		if s := strings.TrimSpace(n.Summary); s != "" {
			line += ". " + aiplug.OneLine(s, 140)
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		b.WriteString("None yet.\n")
	}
	aiplug.WriteLines(&b, lines)
	return fmt.Sprintf("Game nights · %d", len(list)), b.String(), nil
}

// serviceNights is aiNightStore over the sessions service.
type serviceNights struct {
	svc     SessionService
	members campaigns.MemberLister
}

func (g serviceNights) List(ctx context.Context, campaignID string) ([]aiNight, error) {
	list, err := g.svc.ListSessions(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	out := make([]aiNight, 0, len(list))
	for _, s := range list {
		n := aiNight{ID: s.ID, Name: s.Name, Status: s.Status}
		if s.ScheduledDate != nil {
			n.Date = *s.ScheduledDate
		}
		if s.ScheduledTime != nil {
			n.Time = *s.ScheduledTime
		}
		if s.Summary != nil {
			n.Summary = *s.Summary
		}
		out = append(out, n)
	}
	return out, nil
}

// Create invites every member, as the Game nights page does when a night is
// made there. It sends no RSVP email: an AI's change never mails people.
func (g serviceNights) Create(ctx context.Context, campaignID string, a aiplug.Actor, in aiNightInput) (string, error) {
	var name string
	if in.Name != nil {
		name = *in.Name
	}
	s, err := g.svc.CreateSession(ctx, campaignID, CreateSessionInput{
		Name: name, Summary: in.Summary, ScheduledDate: in.Date, ScheduledTime: in.Time, CreatedBy: a.UserID,
	})
	if err != nil {
		return "", err
	}
	if in.Status != nil && *in.Status != StatusPlanned {
		if _, err := g.svc.UpdateSession(ctx, s.ID, UpdateSessionInput{Status: patch.Of(*in.Status)}); err != nil {
			return s.ID, err
		}
	}
	if g.members != nil {
		if members, err := g.members.ListMembers(ctx, campaignID); err == nil {
			ids := make([]string, 0, len(members))
			for _, m := range members {
				ids = append(ids, m.UserID)
			}
			_ = g.svc.InviteAll(ctx, s.ID, ids)
		}
	}
	return s.ID, nil
}

func (g serviceNights) Update(ctx context.Context, id string, in aiNightInput) error {
	_, err := g.svc.UpdateSession(ctx, id, UpdateSessionInput{
		Name:          setIfGiven(in.Name),
		Summary:       setIfGiven(in.Summary),
		ScheduledDate: setIfGiven(in.Date),
		ScheduledTime: setIfGiven(in.Time),
		Status:        setIfGiven(in.Status),
	})
	return err
}

// setIfGiven leaves a key the AI did not write as it is; patch.FromPtr would
// clear it instead.
func setIfGiven(p *string) patch.Field[string] {
	if p == nil {
		return patch.Absent[string]()
	}
	return patch.Of(*p)
}

func (g serviceNights) SetRecap(ctx context.Context, id string, recapJSON, recapHTML *string) error {
	return g.svc.UpdateSessionRecap(ctx, id, recapJSON, recapHTML)
}

func (g serviceNights) Delete(ctx context.Context, id string) error {
	_, err := g.svc.DeleteSession(ctx, id)
	return err
}
