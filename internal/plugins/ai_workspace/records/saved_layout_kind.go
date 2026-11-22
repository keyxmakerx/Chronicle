package records

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// SavedLayoutsAPI is the campaign's saved layouts: the named arrangements an
// owner starts a category's page layout from in Customize. The built-in ones
// ship with every campaign and are never changed or removed.
type SavedLayoutsAPI interface {
	// IsOwner reports whether userID owns the campaign; saved layouts are
	// managed by the owner.
	IsOwner(ctx context.Context, campaignID, userID string) (bool, error)
	// List is the campaign's saved layouts, built-in ones included.
	List(ctx context.Context, campaignID string) ([]entities.LayoutPreset, error)
	// BlockTypes are the blocks a layout can place in this campaign.
	BlockTypes(ctx context.Context, campaignID string) []LayoutBlockInfo
	// Check runs the rules a layout must meet, without saving.
	Check(ctx context.Context, campaignID string, l entities.EntityTypeLayout) error
	Create(ctx context.Context, campaignID string, in entities.CreateLayoutPresetInput) (*entities.LayoutPreset, error)
	// Update sends only the fields the input holds, as the route does.
	Update(ctx context.Context, campaignID string, id int, in entities.UpdateLayoutPresetInput) error
	Delete(ctx context.Context, campaignID string, id int) error
}

// Limits the layout preset service enforces, repeated so a plan can refuse
// what an apply would.
const (
	maxSavedLayoutName = 200
	maxSavedLayoutDesc = 500
)

// SavedLayoutKind makes, rearranges, renames and removes the campaign's saved
// layouts. A saved layout is a copy an owner applies to a category; changing
// one never changes a category that already used it.
type SavedLayoutKind struct{ Svc SavedLayoutsAPI }

func (SavedLayoutKind) Name() string  { return "saved-layout" }
func (SavedLayoutKind) Label() string { return "Saved layout" }
func (SavedLayoutKind) Brief() string {
	return "the campaign's own saved layouts, the named page arrangements an owner starts a category's layout from: make, rearrange, rename or remove one (the built-in ones stay as they are; owner only)"
}
func (SavedLayoutKind) Doc() string {
	return "One of the campaign's saved layouts, matched by `name`, ignoring capitals. A saved layout is a page arrangement an owner picks as the starting point of a category's layout; changing one does not change a category that already used it. Keys: `rows` (the arrangement: a list of rows, each a list of `columns` with a `width` from 1 to 12 and `blocks`, written as in `kind: layout`, where a block is its type or `{type: text_block, text: \"words\"}` and may carry `directors_only`), `description` (up to 500 characters), `icon` (a Font Awesome name such as `fa-id-card`), and `rename_to`. `create` needs `rows`; on `update` a key left out keeps what is saved, and `rows`, when given, replaces the whole arrangement (a block already in it keeps its settings when listed again; one left out is removed; a row `- tiles: kept` keeps a tile section it already has). `delete` removes the saved layout, never a category's own layout. The built-in layouts (Standard, Wide Content, Sidebar Left, Compact Profile) can't be changed or removed; to start from one, create a saved layout under a new name. Only the campaign's owner can change saved layouts. `what: layouts` with `part: saved-layouts` lists them all, written so they can be sent back changed.\n\n```\n---\nkind: saved-layout\nname: Hero sheet\ndescription: Portrait on the right, the story on the left.\nicon: fa-id-card\nrows:\n  - columns:\n      - width: 8\n        blocks: [title, entry]\n      - width: 4\n        blocks: [image, details]\n---\n```"
}

func (k SavedLayoutKind) owner(ctx context.Context, campaignID string, a Actor) error {
	if k.Svc == nil {
		return badRequestf("saved layouts can't be changed on this server")
	}
	if a.CanAuthorDmOnly() {
		if ok, err := k.Svc.IsOwner(ctx, campaignID, a.UserID); err != nil {
			return err
		} else if ok {
			return nil
		}
	}
	return badRequestf("only the campaign's owner can change saved layouts")
}

// savedLayoutTarget is a record resolved against the saved layout it names.
type savedLayoutTarget struct {
	hit   *entities.LayoutPreset // nil when no saved layout has the name
	all   []entities.LayoutPreset
	was   []gridRow
	next  []gridRow // set when the record writes `rows`
	words []homeWordsChange
	// rename, desc and icon are what the record sets; has* tells "left out,
	// keep it" from a value.
	rename, desc, icon        string
	hasRename, hasDesc, hasIc bool
}

// presetNamed is the saved layout with the name, nil for none. The
// campaign's own wins over a built-in of the same name, so the one the owner
// can change is the one a record reaches.
func presetNamed(all []entities.LayoutPreset, name string) *entities.LayoutPreset {
	var builtin *entities.LayoutPreset
	for i := range all {
		if !sameName(all[i].Name, name) {
			continue
		}
		if !all[i].IsBuiltin {
			return &all[i]
		}
		if builtin == nil {
			builtin = &all[i]
		}
	}
	return builtin
}

func presetRows(p *entities.LayoutPreset) ([]gridRow, error) {
	var l entities.EntityTypeLayout
	if err := json.Unmarshal([]byte(p.LayoutJSON), &l); err != nil {
		return nil, badRequestf("the saved layout %s can't be read", quote(p.Name))
	}
	return gridFromLayout(l), nil
}

func (k SavedLayoutKind) resolve(ctx context.Context, campaignID string, r Record) (savedLayoutTarget, error) {
	var t savedLayoutTarget
	name := strings.TrimSpace(r.Name)
	if name == "" {
		return t, badRequestf("name the saved layout in `name:`")
	}
	var err error
	if t.all, err = k.Svc.List(ctx, campaignID); err != nil {
		return t, err
	}
	t.hit = presetNamed(t.all, name)
	if t.hit != nil {
		if t.was, err = presetRows(t.hit); err != nil && r.Action != ActionDelete {
			return t, err
		}
	}
	if r.Action == ActionDelete {
		return t, nil
	}
	if t.hasRename = r.Has("rename_to"); t.hasRename {
		t.rename = strings.TrimSpace(r.Str("rename_to"))
	}
	if t.hasDesc = r.Has("description"); t.hasDesc {
		t.desc = strings.TrimSpace(r.Str("description"))
	}
	if t.hasIc = r.Has("icon"); t.hasIc {
		if t.icon, err = sanitize.ValidateIcon(r.Str("icon")); err != nil {
			return t, badRequestf("icon must be a Font Awesome icon name such as fa-id-card")
		}
	}
	if items := r.List("rows"); len(items) > 0 {
		palette := map[string]bool{}
		for _, b := range k.Svc.BlockTypes(ctx, campaignID) {
			palette[b.Type] = true
		}
		onLayout := map[string]bool{}
		for _, row := range t.was {
			for _, c := range row.Columns {
				for _, b := range c.Blocks {
					onLayout[b.Type] = true
				}
			}
		}
		rules := gridRules{
			valid:    func(typ string) bool { return palette[typ] || onLayout[typ] },
			notBlock: "a block this campaign's pages can hold",
			words:    layoutWords,
			tileRow:  keepTiles(t.was),
			block:    layoutBlockRule,
		}
		if t.next, t.words, err = parseGrid(items, t.was, rules); err != nil {
			return t, err
		}
	}
	return t, nil
}

// checkText bounds the words a saved layout's metadata takes, as the service does.
func (t savedLayoutTarget) checkText() error {
	if len(t.rename) > maxSavedLayoutName {
		return badRequestf("a saved layout's name is at most %d characters", maxSavedLayoutName)
	}
	if utf8.RuneCountInString(t.desc) > maxSavedLayoutDesc || len(t.desc) > maxSavedLayoutDesc {
		return badRequestf("a description is at most %d characters", maxSavedLayoutDesc)
	}
	return nil
}

func (k SavedLayoutKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	if err := k.owner(ctx, campaignID, a); err != nil {
		return Plan{Error: planError(err)}
	}
	t, err := k.resolve(ctx, campaignID, r)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	name := quote(strings.TrimSpace(r.Name))
	if t.hit != nil && t.hit.IsBuiltin {
		return Plan{Error: "the saved layout " + name + " is built in and can't be changed or removed; create a saved layout under another name to start from it"}
	}
	if err := t.checkText(); err != nil {
		return Plan{Error: planError(err)}
	}
	switch r.Action {
	case ActionCreate:
		if t.hit != nil {
			return Plan{Error: "a saved layout called " + name + " already exists; use action: update"}
		}
		if t.hasRename {
			return Plan{Error: "a new saved layout has nothing to rename; leave out `rename_to`"}
		}
		if len(t.next) == 0 {
			return Plan{Error: "list the saved layout's `rows:`, each with its `columns`"}
		}
		if len(strings.TrimSpace(r.Name)) > maxSavedLayoutName {
			return Plan{Error: "a saved layout's name is at most 200 characters"}
		}
		if err := k.Svc.Check(ctx, campaignID, layoutFromGrid(stripGridMarkdown(t.next))); err != nil {
			return Plan{Error: planError(err)}
		}
		return Plan{Summary: "adds the saved layout " + name + ": " + gridSummary(t.next) + gridWordsSummary(t.words)}
	case ActionUpdate:
		if t.hit == nil {
			return Plan{Error: "no saved layout called " + name + "; use action: create to add one"}
		}
		var parts []string
		if t.hasRename && t.rename == "" {
			return Plan{Error: "`rename_to` needs a name"}
		}
		if t.hasRename && t.rename != t.hit.Name {
			if o := presetNamed(t.all, t.rename); o != nil && o.ID != t.hit.ID {
				return Plan{Error: "a saved layout called " + quote(t.rename) + " already exists"}
			}
			parts = append(parts, "is renamed "+quote(t.rename))
		}
		if t.hasDesc && t.desc != t.hit.Description {
			parts = append(parts, "gets the description "+quoted(t.desc))
		}
		if t.hasIc && t.icon != "" && t.icon != t.hit.Icon {
			parts = append(parts, "gets the icon "+t.icon)
		}
		var warnings []string
		if len(t.next) > 0 {
			if err := k.Svc.Check(ctx, campaignID, layoutFromGrid(stripGridMarkdown(t.next))); err != nil {
				return Plan{Error: planError(err)}
			}
			if gridShape(t.next) != gridShape(t.was) || len(t.words) > 0 {
				parts = append(parts, "becomes: "+gridSummary(t.next)+" (now: "+gridSummary(t.was)+")"+gridWordsSummary(t.words))
				if dropped := gridDropped(t.was, t.next); len(dropped) > 0 {
					warnings = append(warnings, "left out, so removed: "+strings.Join(dropped, ", "))
				}
			}
		}
		if len(parts) == 0 {
			return Plan{Error: "the saved layout " + name + " is already like this"}
		}
		return Plan{Summary: "the saved layout " + name + " " + strings.Join(parts, "; "), Warnings: warnings}
	default:
		if t.hit == nil {
			return Plan{Error: "no saved layout called " + name}
		}
		return Plan{Summary: "removes the saved layout " + name + " (" + gridSummary(t.was) + "); categories that started from it keep their own layouts"}
	}
}

func (k SavedLayoutKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	if p := k.Plan(ctx, campaignID, a, r); p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	t, err := k.resolve(ctx, campaignID, r)
	if err != nil {
		return err
	}
	if r.Action == ActionDelete {
		return k.Svc.Delete(ctx, campaignID, t.hit.ID)
	}
	if err := renderGridWords(t.next, r.Text, "saved layout"); err != nil {
		return err
	}
	var layoutJSON string
	if len(t.next) > 0 {
		b, err := json.Marshal(layoutFromGrid(t.next))
		if err != nil {
			return err
		}
		layoutJSON = string(b)
	}
	if r.Action == ActionCreate {
		_, err := k.Svc.Create(ctx, campaignID, entities.CreateLayoutPresetInput{
			CampaignID: campaignID, Name: strings.TrimSpace(r.Name), Description: t.desc, LayoutJSON: layoutJSON, Icon: t.icon,
		})
		return err
	}
	// Only what the record sets goes in the update, so a rename never
	// rewrites the arrangement and a new arrangement never touches the name.
	var in entities.UpdateLayoutPresetInput
	if t.hasRename && t.rename != t.hit.Name {
		in.Name = patch.Of(t.rename)
	}
	if t.hasDesc && t.desc != t.hit.Description {
		in.Description = patch.Of(t.desc)
	}
	if t.hasIc && t.icon != "" && t.icon != t.hit.Icon {
		in.Icon = patch.Of(t.icon)
	}
	if layoutJSON != "" && (gridShape(t.next) != gridShape(t.was) || len(t.words) > 0) {
		in.LayoutJSON = patch.Of(layoutJSON)
	}
	return k.Svc.Update(ctx, campaignID, t.hit.ID, in)
}

// savedLayoutRecord writes one saved layout as the `kind: saved-layout` block
// that sends it back.
func (k SavedLayoutKind) savedLayoutRecord(b *strings.Builder, p entities.LayoutPreset, rows []gridRow) {
	b.WriteString("---\nkind: saved-layout\nname: " + quote(p.Name) + "\n")
	if p.Description != "" {
		b.WriteString("description: " + quote(p.Description) + "\n")
	}
	b.WriteString("icon: " + quote(p.Icon) + "\n")
	writeGrid(b, rows, LayoutKind{}.blockSpec)
	b.WriteString("---\n\n")
}

// Export writes the campaign's own saved layouts, for the owner only. The
// built-in ones are not written: they can't be sent back.
func (k SavedLayoutKind) Export(ctx context.Context, campaignID string, a Actor) (string, error) {
	if k.owner(ctx, campaignID, a) != nil {
		return "", nil
	}
	all, err := k.Svc.List(ctx, campaignID)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for i := range all {
		if all[i].IsBuiltin {
			continue
		}
		if rows, err := presetRows(&all[i]); err == nil && len(rows) > 0 {
			k.savedLayoutRecord(&b, all[i], rows)
		}
	}
	return b.String(), nil
}

// savedState is a saved layout's own fields, which is what an undo puts back
// and what its fingerprint covers; its dates are left out.
type savedState struct {
	ID          int    `json:"id,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	LayoutJSON  string `json:"layoutJson"`
}

func savedStateOf(p *entities.LayoutPreset) savedState {
	if p == nil {
		return savedState{}
	}
	return savedState{ID: p.ID, Name: p.Name, Description: p.Description, Icon: p.Icon, LayoutJSON: p.LayoutJSON}
}

var _ aiplug.Undoable = SavedLayoutKind{}

// The snapshot's target says what an undo has to do: remove a layout the
// record made ("made:<name>"), put an edited one back ("edit:<id>") or bring
// a removed one back ("gone:<id>").
const (
	savedMade = "made"
	savedEdit = "edit"
	savedGone = "gone"
)

// Before keeps the saved layout as it is now. A layout the record makes has
// nothing to keep: its undo removes it again.
func (k SavedLayoutKind) Before(ctx context.Context, campaignID string, _ Actor, r Record) (*aiplug.Snapshot, error) {
	if k.Svc == nil {
		return nil, nil
	}
	all, err := k.Svc.List(ctx, campaignID)
	if err != nil {
		return nil, nil
	}
	name := strings.TrimSpace(r.Name)
	hit := presetNamed(all, name)
	if hit == nil {
		if r.Action != ActionCreate {
			return nil, nil
		}
		return &aiplug.Snapshot{Target: savedMade + ":" + name, Name: "the saved layout " + quote(name), Data: "{}"}, nil
	}
	if hit.IsBuiltin || r.Action == ActionCreate {
		return nil, nil
	}
	data, err := json.Marshal(savedStateOf(hit))
	if err != nil {
		return nil, err
	}
	mode := savedEdit
	if r.Action == ActionDelete {
		mode = savedGone
	}
	return &aiplug.Snapshot{Target: mode + ":" + strconv.Itoa(hit.ID), Name: "the saved layout " + quote(hit.Name), Data: string(data)}, nil
}

// current is the saved layout a snapshot's target names now, nil when gone.
func (k SavedLayoutKind) current(ctx context.Context, campaignID, target string) (mode string, p *entities.LayoutPreset, err error) {
	mode, key, _ := strings.Cut(target, ":")
	all, err := k.Svc.List(ctx, campaignID)
	if err != nil {
		return mode, nil, err
	}
	if mode == savedMade {
		return mode, presetNamed(all, key), nil
	}
	id, convErr := strconv.Atoi(key)
	if convErr != nil {
		return mode, nil, badRequestf("this change can't be undone: its saved layout is not known")
	}
	for i := range all {
		if all[i].ID == id {
			return mode, &all[i], nil
		}
	}
	return mode, nil, nil
}

// Fingerprint is the saved layout now; one that is gone fingerprints as none.
func (k SavedLayoutKind) Fingerprint(ctx context.Context, campaignID string, _ Actor, s aiplug.Snapshot) (string, error) {
	_, p, err := k.current(ctx, campaignID, s.Target)
	if err != nil {
		return "", err
	}
	return aiplug.Hash(savedStateOf(p)), nil
}

// Restore takes a made layout away, puts an edited one's fields back, or
// makes a removed one again (the new copy sorts last, as any new layout does).
func (k SavedLayoutKind) Restore(ctx context.Context, campaignID string, _ Actor, s aiplug.Snapshot) error {
	mode, p, err := k.current(ctx, campaignID, s.Target)
	if err != nil {
		return err
	}
	var was savedState
	if err := json.Unmarshal([]byte(s.Data), &was); err != nil {
		return err
	}
	switch mode {
	case savedMade:
		if p == nil {
			return nil
		}
		if p.IsBuiltin {
			return badRequestf("this change can't be undone: a built-in layout can't be removed")
		}
		return k.Svc.Delete(ctx, campaignID, p.ID)
	case savedGone:
		if p != nil {
			return nil
		}
		_, err := k.Svc.Create(ctx, campaignID, entities.CreateLayoutPresetInput{
			CampaignID: campaignID, Name: was.Name, Description: was.Description, LayoutJSON: was.LayoutJSON, Icon: was.Icon,
		})
		return err
	}
	if p == nil {
		return badRequestf("this change can't be undone: the saved layout is gone")
	}
	return k.Svc.Update(ctx, campaignID, p.ID, entities.UpdateLayoutPresetInput{
		Name: patch.Of(was.Name), Description: patch.Of(was.Description),
		Icon: patch.Of(was.Icon), LayoutJSON: patch.Of(was.LayoutJSON),
	})
}
