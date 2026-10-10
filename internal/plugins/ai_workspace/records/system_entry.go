package records

import (
	"context"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// systemEntryReserved are the front-matter keys with a meaning of their own
// (including the page keys the parser always knows); every other key is a
// detail stored in the entry's properties.
var systemEntryReserved = map[string]bool{
	"kind": true, "action": true, "name": true, "entry": true, "summary": true,
	"director": true, "rename_to": true, "type": true, "subcategory": true,
	"visibility": true, "tags": true, "description": true,
}

// SystemEntryKind adds, changes and removes the campaign's own pick-list
// entries (an ancestry, kit, culture...). It goes through the systems
// service, which only ever sees the campaign's own rows, so the game
// system package's entries cannot be changed from here.
type SystemEntryKind struct {
	Svc systems.SystemEntryService
}

func (SystemEntryKind) Name() string  { return "system-entry" }
func (SystemEntryKind) Label() string { return "System entry" }
func (SystemEntryKind) Doc() string {
	return "An entry the campaign adds to its game system's pick list for a character field (an ancestry, kit, culture, career, class, race...). `entry` is the character field's key (for example `ancestry`); matched by `entry` + `name`. Give a one-line `summary`. The body is the description. Optional `director: true` keeps it to Directors; `rename_to`. Any other key (for example `size`, `speed`) is stored as a detail and must be plain text, a number or yes/no. It never changes the game system's own entries.\n\n```\n---\nkind: system-entry\nentry: ancestry\nname: Stoneborn\nsummary: Hardy folk of the deep mountains.\nsize: Medium\nspeed: 5\n---\nStoneborn are shaped by lives lived under rock.\n```"
}

func (k SystemEntryKind) actor(a Actor) systems.EntryActor {
	return systems.EntryActor{UserID: a.UserID, IsDirector: a.CanAuthorDmOnly()}
}

func (k SystemEntryKind) details(r Record) map[string]any {
	out := map[string]any{}
	for key, v := range r.Fields {
		if !systemEntryReserved[key] {
			out[key] = v
		}
	}
	return out
}

func (k SystemEntryKind) find(ctx context.Context, campaignID string, a Actor, r Record) (*systems.SystemEntry, error) {
	return k.Svc.FindByName(ctx, campaignID, r.Str("entry"), r.Name, k.actor(a))
}

func (k SystemEntryKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	if !a.CanAuthorDmOnly() {
		return Plan{Error: "only the campaign's Directors can change its system entries"}
	}
	if r.Name == "" {
		return Plan{Error: "a system entry needs a name"}
	}
	if r.Str("entry") == "" {
		return Plan{Error: "a system entry needs `entry:` (the character field it belongs to, for example ancestry)"}
	}
	cur, err := k.find(ctx, campaignID, a, r)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	switch r.Action {
	case ActionCreate:
		if cur != nil {
			return Plan{Error: "a " + r.Str("entry") + " called " + quote(r.Name) + " already exists; use action: update"}
		}
		in := k.createInput(r)
		if err := k.Svc.CheckCreate(ctx, campaignID, k.actor(a), in); err != nil {
			return Plan{Error: planError(err)}
		}
		return Plan{Summary: "new " + r.Str("entry") + " entry"}
	case ActionUpdate:
		if cur == nil {
			return Plan{Error: "no " + r.Str("entry") + " entry called " + quote(r.Name)}
		}
		if err := systems.ValidateEntryProperties(k.merged(cur, r)); err != nil {
			return Plan{Error: planError(err)}
		}
		return Plan{Summary: "changes the entry"}
	default:
		if cur == nil {
			return Plan{Error: "no " + r.Str("entry") + " entry called " + quote(r.Name)}
		}
		return Plan{
			Summary:  "removes the entry from the pick list",
			Warnings: []string{"characters that already picked it keep the name they stored"},
		}
	}
}

func (k SystemEntryKind) createInput(r Record) systems.CreateSystemEntryInput {
	vis := systems.EntryVisibilityEveryone
	if v, ok := r.Bool("director"); ok && v {
		vis = systems.EntryVisibilityDirectors
	}
	return systems.CreateSystemEntryInput{
		FieldKey: r.Str("entry"), Name: r.Name, Summary: r.Str("summary"),
		Description: strings.TrimSpace(r.Body), Properties: k.details(r), Visibility: vis,
	}
}

// merged overlays the record's details on the stored ones: an update that
// names `speed` must not forget `size`.
func (k SystemEntryKind) merged(cur *systems.SystemEntry, r Record) map[string]any {
	out := map[string]any{}
	for key, v := range cur.Properties {
		out[key] = v
	}
	for key, v := range k.details(r) {
		out[key] = v
	}
	return out
}

func (k SystemEntryKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	if p := k.Plan(ctx, campaignID, a, r); p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	cur, err := k.find(ctx, campaignID, a, r)
	if err != nil {
		return err
	}
	switch r.Action {
	case ActionCreate:
		if cur != nil {
			return apperror.NewBadRequest("it changed while you were reviewing; check it again")
		}
		_, err = k.Svc.Create(ctx, campaignID, k.actor(a), k.createInput(r))
		return err
	}
	if cur == nil {
		return apperror.NewBadRequest("it changed while you were reviewing; check it again")
	}
	if r.Action == ActionDelete {
		return k.Svc.Delete(ctx, campaignID, k.actor(a), cur.ID)
	}
	// Partial update: only what the block names is sent.
	var in systems.UpdateSystemEntryInput
	if s := r.Str("rename_to"); s != "" {
		in.Name = patch.Of(s)
	}
	if s := r.Str("summary"); s != "" {
		in.Summary = patch.Of(s)
	}
	if body := strings.TrimSpace(r.Body); body != "" {
		in.Description = patch.Of(body)
	}
	if v, ok := r.Bool("director"); ok {
		vis := systems.EntryVisibilityEveryone
		if v {
			vis = systems.EntryVisibilityDirectors
		}
		in.Visibility = patch.Of(vis)
	}
	if len(k.details(r)) > 0 {
		in.Properties = patch.Of(k.merged(cur, r))
	}
	_, err = k.Svc.Update(ctx, campaignID, k.actor(a), cur.ID, in)
	return err
}

func (k SystemEntryKind) Export(ctx context.Context, campaignID string, a Actor) (string, error) {
	if !a.CanAuthorDmOnly() {
		return "", nil
	}
	list, err := k.Svc.List(ctx, campaignID, "", k.actor(a))
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, e := range list {
		b.WriteString("- " + e.FieldKey + ": " + e.Name)
		if e.Visibility == systems.EntryVisibilityDirectors {
			b.WriteString(" (Directors only)")
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}
