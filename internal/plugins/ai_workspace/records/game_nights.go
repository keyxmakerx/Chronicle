package records

import (
	"context"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// GameNightsAPI is the sessions service as the game-night kind needs it,
// adapted in app wiring so this package does not import the plugin. The
// service keeps its own rules (statuses, soft delete, recap sanitizing).
type GameNightsAPI interface {
	List(ctx context.Context, campaignID string) ([]GameNightRef, error)
	// Create makes the night and invites every member, as the Game nights
	// page does; it sends no email.
	Create(ctx context.Context, campaignID string, a Actor, in GameNightInput) (string, error)
	Update(ctx context.Context, id string, in GameNightInput) error
	SetRecap(ctx context.Context, id string, recapJSON, recapHTML *string) error
	// Delete moves the night to the restorable trash.
	Delete(ctx context.Context, id string) error
}

// GameNightRef is a stored game night.
type GameNightRef struct {
	ID, Name, Date, Time, Status, Summary string
}

// GameNightInput is the night's details; an empty pointer leaves a field as
// it is on update.
type GameNightInput struct {
	Name, Date, Time, Status, Summary *string
}

// GameNightKind adds, changes and removes game nights (play sessions),
// matched by name, with `date` telling apart two nights of the same name.
// The block's body is the night's recap, which every member can read.
type GameNightKind struct {
	Svc GameNightsAPI
}

func (GameNightKind) Name() string  { return "game-night" }
func (GameNightKind) Label() string { return "Game night" }
func (GameNightKind) Doc() string {
	return "A game night (a play session), matched by `name`; when two share a name, `date` picks one. Keys: `date` (the real-world day, YYYY-MM-DD), `time` (24-hour HH:MM), `summary` (one line), `status` (planned, played or cancelled), `rename_to`. The body is the night's recap, which every member of the campaign can read; on update a body replaces the recap and no body keeps it. A new night invites every member, as the Game nights page does, and sends no email. Removing a night moves it to the trash, where the owner can restore it.\n\n```\n---\nkind: game-night\nname: Session 13\ndate: 2026-10-24\ntime: \"19:30\"\nsummary: The siege of Harrowmere\n---\n```\n\n```\n---\nkind: game-night\naction: update\nname: Session 12\nstatus: played\n---\nThe party broke the siege at dawn and Corvin lost his shield.\n```"
}

// nightStatus maps the words an AI may write onto the stored statuses.
var nightStatus = map[string]string{
	"planned": "planned", "scheduled": "planned",
	"played": "completed", "completed": "completed", "done": "completed",
	"cancelled": "cancelled", "canceled": "cancelled",
}

// nightWords is the plain word for a stored status.
var nightWords = map[string]string{"planned": "planned", "completed": "played", "cancelled": "cancelled"}

// dateField reads `date`, which YAML may hand over as a timestamp.
func (r Record) dateField(key string) string {
	if t, ok := r.Fields[key].(time.Time); ok {
		return t.Format("2006-01-02")
	}
	return r.Str(key)
}

// input reads and checks the night's keys; only written keys are set.
func (k GameNightKind) input(r Record) (GameNightInput, error) {
	var in GameNightInput
	if r.Has("date") {
		d := r.dateField("date")
		if _, err := time.Parse("2006-01-02", d); err != nil {
			return in, badRequestf("date: %q is not a day written YYYY-MM-DD", d)
		}
		in.Date = &d
	}
	if r.Has("time") {
		t := r.Str("time")
		if _, err := time.Parse("15:04", t); err != nil {
			return in, badRequestf("time: %q is not a 24-hour time written HH:MM", t)
		}
		in.Time = &t
	}
	if r.Has("status") {
		s, ok := nightStatus[strings.ToLower(r.Str("status"))]
		if !ok {
			return in, badRequestf("status: %q is not planned, played or cancelled", r.Str("status"))
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
func (k GameNightKind) find(ctx context.Context, campaignID string, r Record) (*GameNightRef, error) {
	list, err := k.Svc.List(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	var hits []GameNightRef
	for _, n := range list {
		if sameName(n.Name, r.Name) {
			hits = append(hits, n)
		}
	}
	if len(hits) > 1 && r.Has("date") {
		d := r.dateField("date")
		var same []GameNightRef
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
		return nil, badRequestf("more than one game night is called %s on %s", quote(r.Name), r.dateField("date"))
	}
	return nil, badRequestf("more than one game night is called %s; add `date:` to pick one", quote(r.Name))
}

func nightLine(in GameNightInput, existing *GameNightRef) string {
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
		parts = append(parts, "summary "+quote(*in.Summary))
	}
	if in.Name != nil {
		parts = append(parts, "renamed to "+quote(*in.Name))
	}
	return strings.Join(parts, ", ")
}

func (k GameNightKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	if !a.CanAuthorDmOnly() {
		return Plan{Error: "only the owner or a co-DM can change game nights"}
	}
	if r.Name == "" {
		return Plan{Error: "a game night needs a name"}
	}
	in, err := k.input(r)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	hit, err := k.find(ctx, campaignID, r)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	recap := strings.TrimSpace(r.Body) != ""
	switch r.Action {
	case ActionCreate:
		if hit != nil && (in.Date == nil || hit.Date == *in.Date) {
			return Plan{Error: "a game night called " + quote(r.Name) + " already exists; use action: update"}
		}
		s := nightLine(in, nil)
		if s == "" {
			s = "no date yet"
		}
		s += "; invites every member"
		if recap {
			s += "; with a recap every member can read"
		}
		return Plan{Summary: s}
	case ActionUpdate:
		if hit == nil {
			return Plan{Error: "no game night called " + quote(r.Name)}
		}
		s := nightLine(in, hit)
		if recap {
			if s != "" {
				s += "; "
			}
			s += "replaces the recap, which every member can read"
		}
		if s == "" {
			return Plan{Error: "nothing to change: name a key to change, or write the recap below the front matter"}
		}
		return Plan{Summary: s}
	default:
		if hit == nil {
			return Plan{Error: "no game night called " + quote(r.Name)}
		}
		return Plan{Summary: "moves it to the trash, where the owner can restore it"}
	}
}

func (k GameNightKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	if p := k.Plan(ctx, campaignID, a, r); p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	in, err := k.input(r)
	if err != nil {
		return err
	}
	j, h, err := entry(r.Body, r.Links)
	if err != nil {
		return err
	}
	if r.Action == ActionCreate {
		name := r.Name
		in.Name = &name
		id, err := k.Svc.Create(ctx, campaignID, a, in)
		if err != nil || h == nil {
			return err
		}
		return k.Svc.SetRecap(ctx, id, j, h)
	}
	hit, err := k.find(ctx, campaignID, r)
	if err != nil {
		return err
	}
	if hit == nil {
		return apperror.NewBadRequest("it changed while you were reviewing; check it again")
	}
	if r.Action == ActionDelete {
		return k.Svc.Delete(ctx, hit.ID)
	}
	if in != (GameNightInput{}) {
		if err := k.Svc.Update(ctx, hit.ID, in); err != nil {
			return err
		}
	}
	if h != nil {
		return k.Svc.SetRecap(ctx, hit.ID, j, h)
	}
	return nil
}

// Export lists every game night in this kind's own format, so an AI can
// update or remove one by name. Recaps are left out: the export's Sessions
// section already carries them.
func (k GameNightKind) Export(ctx context.Context, campaignID string, a Actor) (string, error) {
	if !a.CanAuthorDmOnly() {
		return "", nil
	}
	list, err := k.Svc.List(ctx, campaignID)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, n := range list {
		b.WriteString("---\nkind: game-night\nname: " + quote(n.Name) + "\n")
		if n.Date != "" {
			b.WriteString("date: " + n.Date + "\n")
		}
		if n.Time != "" {
			b.WriteString("time: " + quote(n.Time) + "\n")
		}
		if w := nightWords[n.Status]; w != "" {
			b.WriteString("status: " + w + "\n")
		}
		if n.Summary != "" {
			b.WriteString("summary: " + quote(n.Summary) + "\n")
		}
		b.WriteString("---\n\n")
	}
	return b.String(), nil
}
