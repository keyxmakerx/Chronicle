package records

import (
	"context"
	"encoding/json"
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
	"visibility": true, "tags": true, "description": true, "reason": true,
	"for": true, "in_place_of": true,
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
	return systemEntryDoc + " " + systemEntryInPlaceDoc + "\n\nCard borders: " + systems.CardBorderGuide() + "\n\n" + systemEntryExample
}

// systemEntryDoc and systemEntryExample are the kind's notes; the card-border
// note goes between them so the example stays last.
const systemEntryDoc = "An entry the campaign adds to its game system's pick list for a character field (an ancestry, kit, culture, career, class, race...). `entry` is the character field's key (for example `ancestry`; ask `what: system-entries` with no `field` for the keys that have entries); matched by `entry` + `name`. Give a one-line `summary`. The body is the description. Optional `director: true` keeps it to Directors; `for: <character name>` instead keeps it to Directors and whoever plays that character page (a penciled option shown to one player), and `director: false` on an update opens it to everyone; `rename_to`. Any other key (for example `size`, `speed`) is stored as a detail and must be plain text, a number or yes/no, or a list of items that each have a `name` and optionally a whole-number `cost` and a plain-text `description` (for traits a hero buys with points, shape it like the system's own entries: a `*_points` budget beside the list). It never changes the game system's own entries; to take one of those (or a campaign entry) off players' pick list, use `kind: hero-choice-rule`."

const systemEntryExample = "```\n---\nkind: system-entry\nentry: ancestry\nname: Stoneborn\nsummary: Hardy folk of the deep mountains.\nsize: 1M\nspeed: 5\nancestry_points: 3\nsignature_traits:\n  - name: Rooted\n    description: You can't be pushed while standing on stone.\npurchased_traits:\n  - name: Deep Sight\n    cost: 1\n    description: You see in darkness as well as in dim light.\n  - name: Stone Skin\n    cost: 2\n    description: The first strike against you each round deals 2 less damage.\n---\nStoneborn are shaped by lives lived under rock.\n```"

// systemEntryInPlaceDoc is the note on `in_place_of`, kept apart from
// systemEntryDoc so each can change on its own.
const systemEntryInPlaceDoc = "Optional `in_place_of: <name of the game system's own entry in the same field>` makes the entry take that printed entry's place in the pick list and on the rules pages (for example a homebrew `Sylvan Elf` with `in_place_of: High Elf`); ask `what: system-entries` with `field:` for the printed names. A new entry in place of a printed one is still shown to everyone unless `director: true`, which keeps it the Directors' draft while players go on reading the printed entry. One printed entry takes one stand-in, a printed entry the campaign left out cannot be replaced, and `in_place_of: none` on an update makes the entry the campaign's own again (its text stays)."

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

// forCharacter resolves `for:` to the character page's id; "" when the
// block names none. A name that matches no single character page is an error.
func (k SystemEntryKind) forCharacter(ctx context.Context, campaignID string, r Record) (string, error) {
	name := r.Str("for")
	if name == "" {
		return "", nil
	}
	c, err := k.Svc.FindCharacter(ctx, campaignID, name)
	if err != nil {
		return "", err
	}
	if c == nil {
		return "", apperror.NewValidation("no single character page is called " + quote(name))
	}
	return c.ID, nil
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
	if v, ok := r.Bool("director"); ok && v && r.Str("for") != "" {
		return Plan{Error: "use either `director: true` or `for:`, not both"}
	}
	forID, err := k.forCharacter(ctx, campaignID, r)
	if err != nil {
		return Plan{Error: planError(err)}
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
		target, _, err := k.inPlace(ctx, campaignID, a, r, nil)
		if err != nil {
			return Plan{Error: planError(err)}
		}
		in := k.createInput(r, forID)
		if err := k.Svc.CheckCreate(ctx, campaignID, k.actor(a), in); err != nil {
			return Plan{Error: planError(err)}
		}
		sum := "new " + r.Str("entry") + " entry"
		var warn []string
		if target != nil {
			sum += ", in place of " + target.credited()
			if in.Visibility == systems.EntryVisibilityEveryone {
				warn = append(warn, "everyone reads it instead of the printed "+target.name+" once this is applied; `director: true` keeps it the Directors' draft")
			}
		}
		if forID != "" {
			sum += ", shown only to " + r.Str("for") + "'s player and Directors"
		}
		return Plan{Summary: sum, Warnings: warn}
	case ActionUpdate:
		if cur == nil {
			return Plan{Error: "no " + r.Str("entry") + " entry called " + quote(r.Name)}
		}
		// Checked onto what is stored, as the save will be, so a rewritten
		// entry's carried package values do not refuse the review.
		if err := systems.ValidateEntryPropertiesOnto(k.merged(cur, r), cur.Properties); err != nil {
			return Plan{Error: planError(err)}
		}
		target, untie, err := k.inPlace(ctx, campaignID, a, r, cur)
		if err != nil {
			return Plan{Error: planError(err)}
		}
		switch {
		case target != nil && cur.StandsFor == target.slug:
			return Plan{Summary: "changes the entry; it already takes the place of " + target.name}
		case target != nil:
			return Plan{Summary: "changes the entry; takes the place of " + target.name}
		case untie:
			return Plan{Summary: "changes the entry; it no longer takes the place of " + standsForName(cur)}
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

func (k SystemEntryKind) createInput(r Record, forID string) systems.CreateSystemEntryInput {
	vis := systems.EntryVisibilityEveryone
	if v, ok := r.Bool("director"); ok && v {
		vis = systems.EntryVisibilityDirectors
	}
	if forID != "" {
		vis = systems.EntryVisibilityCharacter
	}
	return systems.CreateSystemEntryInput{
		FieldKey: r.Str("entry"), Name: r.Name, Summary: r.Str("summary"),
		Description: strings.TrimSpace(r.Body), Properties: k.details(r), Visibility: vis, ForEntityID: forID,
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
	forID, err := k.forCharacter(ctx, campaignID, r)
	if err != nil {
		return err
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
		target, _, err := k.inPlace(ctx, campaignID, a, r, nil)
		if err != nil {
			return err
		}
		e, err := k.Svc.Create(ctx, campaignID, k.actor(a), k.createInput(r, forID))
		if err != nil || target == nil {
			return err
		}
		// The tie needs the new entry's id, so it can only follow the create.
		// Plan ran the same refusals, so a failure here means the campaign
		// changed during the review; the entry stays as the campaign's own
		// and the error says so, rather than deleting what was just written.
		if _, err := k.Svc.Tie(ctx, campaignID, k.actor(a), e.ID, target.slug); err != nil {
			return tieAfterCreate(r.Name, target, err)
		}
		return nil
	}
	if cur == nil {
		return apperror.NewBadRequest("it changed while you were reviewing; check it again")
	}
	if r.Action == ActionDelete {
		return k.Svc.Delete(ctx, campaignID, k.actor(a), cur.ID)
	}
	target, untie, err := k.inPlace(ctx, campaignID, a, r, cur)
	if err != nil {
		return err
	}
	// Partial update: only what the block names is sent.
	var in systems.UpdateSystemEntryInput
	sends := false
	if s := r.Str("rename_to"); s != "" {
		in.Name, sends = patch.Of(s), true
	}
	if s := r.Str("summary"); s != "" {
		in.Summary, sends = patch.Of(s), true
	}
	if body := strings.TrimSpace(r.Body); body != "" {
		in.Description, sends = patch.Of(body), true
	}
	if v, ok := r.Bool("director"); ok {
		vis := systems.EntryVisibilityEveryone
		if v {
			vis = systems.EntryVisibilityDirectors
		}
		in.Visibility, sends = patch.Of(vis), true
	}
	if forID != "" {
		in.Visibility = patch.Of(systems.EntryVisibilityCharacter)
		in.ForEntityID, sends = patch.Of(forID), true
	}
	if len(k.details(r)) > 0 {
		in.Properties, sends = patch.Of(k.merged(cur, r)), true
	}
	// A block that only ties or unties has nothing to save first.
	if sends || (target == nil && !untie) {
		if _, err := k.Svc.Update(ctx, campaignID, k.actor(a), cur.ID, in); err != nil {
			return err
		}
	}
	switch {
	case target != nil:
		_, err = k.Svc.Tie(ctx, campaignID, k.actor(a), cur.ID, target.slug)
	case untie:
		_, err = k.Svc.Untie(ctx, campaignID, k.actor(a), cur.ID)
	}
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
		switch e.Visibility {
		case systems.EntryVisibilityDirectors:
			b.WriteString(" (Directors only)")
		case systems.EntryVisibilityCharacter:
			b.WriteString(" (for " + e.ForName + ")")
		}
		if e.StandsFor != "" {
			b.WriteString(" (in place of " + standsForName(&e) + ")")
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

// inPlaceNone is the `in_place_of` value that makes an entry the campaign's
// own again.
const inPlaceNone = "none"

// printedEntry is the game system's own entry an `in_place_of` names.
type printedEntry struct {
	slug, name string
	// system is the game system's name; "" when the catalog has none.
	system string
	// heldBy is the campaign entry already in its place; 0 for none.
	heldBy     int64
	heldByName string
}

// credited names the printed entry with its system, as the review reads it.
func (p *printedEntry) credited() string {
	if p.system == "" {
		return "the printed " + p.name
	}
	return p.system + "'s " + p.name
}

// inPlace reads `in_place_of` against the entry as stored (nil on a create):
// the printed entry to tie to, or untie for `none`. Both are zero when the
// block asks for no change, and Plan and Apply both call it so they agree.
// The refusals mirror the service's Tie in plain words for the review; Tie
// still makes them itself at apply time.
func (k SystemEntryKind) inPlace(ctx context.Context, campaignID string, a Actor, r Record, cur *systems.SystemEntry) (*printedEntry, bool, error) {
	v := r.Str("in_place_of")
	if v == "" {
		return nil, false, nil
	}
	if strings.EqualFold(v, inPlaceNone) {
		// A new entry takes no place yet, so there is nothing to undo.
		return nil, cur != nil && cur.StandsFor != "", nil
	}
	p, err := k.printed(ctx, campaignID, a, r.Str("entry"), v)
	if err != nil {
		return nil, false, err
	}
	switch {
	case cur != nil && cur.StandsFor == p.slug:
		return p, false, nil
	case p.heldBy != 0:
		return nil, false, apperror.NewConflict(p.heldByName + " already takes " + p.name + "'s place; send it with `in_place_of: none` first")
	case cur != nil && cur.StandsFor != "":
		return nil, false, apperror.NewConflict(cur.Name + " already takes the place of " + standsForName(cur) + "; send `in_place_of: none` first")
	case cur != nil && cur.Status != "" && cur.Status != systems.EntryStatusAccepted:
		return nil, false, apperror.NewValidation("accept " + cur.Name + " before it takes " + p.name + "'s place")
	}
	return p, false, nil
}

// printed finds the printed entry of the field called name, ignoring case,
// as a Director reads the field: offered, already replaced, or left out.
func (k SystemEntryKind) printed(ctx context.Context, campaignID string, a Actor, field, name string) (*printedEntry, error) {
	list, err := k.fieldCatalog(ctx, campaignID, a, field)
	if err != nil {
		return nil, err
	}
	system := ""
	if list != nil {
		system = list.SystemName
		for _, e := range list.Entries {
			switch {
			case e.Origin == systems.OriginStandard && strings.EqualFold(e.Name, name):
				return &printedEntry{slug: e.Slug, name: e.Name, system: system}, nil
			case e.Origin == systems.OriginRewritten && e.StandsFor != "" && strings.EqualFold(e.StandsForName, name):
				return &printedEntry{slug: e.StandsFor, name: e.StandsForName, system: system, heldBy: e.EntryID, heldByName: e.Name}, nil
			}
		}
		for _, e := range list.LeftOut {
			if strings.EqualFold(e.Name, name) {
				return nil, apperror.NewValidation(e.Name + " is left out of this campaign, so nothing can take its place; bring it back on its rules page first")
			}
		}
	}
	if system == "" {
		system = "the game system"
	}
	return nil, apperror.NewValidation(system + " has no " + field + " entry called " + quote(name) + " to take the place of; ask `what: system-entries` with `field: " + field + "` for its names")
}

// fieldCatalog is the field's pick list as a Director reads it, printed
// entries included; nil when the campaign has no such field.
func (k SystemEntryKind) fieldCatalog(ctx context.Context, campaignID string, a Actor, field string) (*systems.CatalogList, error) {
	list, err := k.Svc.FieldCatalog(ctx, campaignID, field, k.actor(a))
	if err != nil || list == nil || list.FieldKey != field {
		return nil, err
	}
	return list, nil
}

// standsForName is the printed entry's name as the tie last saw it.
func standsForName(e *systems.SystemEntry) string {
	var snap systems.StandardSnapshot
	if json.Unmarshal([]byte(e.StandsForSeen), &snap) == nil && snap.Name != "" {
		return snap.Name
	}
	return "a printed entry"
}

// tieAfterCreate says the entry was added though the tie was refused, so the
// review does not read as nothing written.
func tieAfterCreate(name string, p *printedEntry, err error) error {
	ae, ok := err.(*apperror.AppError)
	if !ok || ae.Code >= 500 {
		return err
	}
	return &apperror.AppError{Code: ae.Code, Type: ae.Type,
		Message: name + " was added, but it could not take " + p.name + "'s place: " + ae.Message}
}
