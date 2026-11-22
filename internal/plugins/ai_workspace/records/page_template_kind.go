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
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// PageTemplatesAPI is the campaign's page templates: the starter text a new
// page begins with, optionally for one category's pages. The ones every
// campaign shares (global) are never changed here.
type PageTemplatesAPI interface {
	// IsOwner reports whether userID owns the campaign; page templates are
	// managed by the owner.
	IsOwner(ctx context.Context, campaignID, userID string) (bool, error)
	// Categories are the campaign's categories, for a template's `category`.
	Categories(ctx context.Context, campaignID string) ([]entities.EntityType, error)
	// List is every template the campaign can use, global ones included.
	List(ctx context.Context, campaignID string) ([]entities.ContentTemplate, error)
	Create(ctx context.Context, campaignID string, in entities.CreateContentTemplateInput) (*entities.ContentTemplate, error)
	// Update sends only the fields the input holds, as the route does.
	Update(ctx context.Context, campaignID string, id int, in entities.UpdateContentTemplateInput) error
	Delete(ctx context.Context, campaignID string, id int) error
}

// maxPageTemplateText bounds a template's markdown, so the editor document it
// turns into stays inside the size the template service accepts.
const maxPageTemplateText = 20000

// PageTemplateKind makes, rewrites, renames and removes the campaign's page
// templates. A template is only a starting point copied into a new page, so
// changing or removing one never changes a page already written from it.
type PageTemplateKind struct{ Svc PageTemplatesAPI }

func (PageTemplateKind) Name() string  { return "page-template" }
func (PageTemplateKind) Label() string { return "Page template" }
func (PageTemplateKind) Brief() string {
	return "the campaign's page templates, the starter text a new page begins with, for every page or for one category's: make, rewrite, rename or remove one (the ones every campaign shares stay as they are; owner only)"
}
func (PageTemplateKind) Doc() string {
	return "One of the campaign's page templates, matched by `name`, ignoring capitals (and by `category` when two share a name). A page template is the starter text a new page begins with, and the text after the block is its words, written in markdown (headings, lists and bold work; GM-only markers are refused). It copies into a new page, so changing or removing one never changes a page already written from it. Keys: `category` (the category whose new pages are offered it; leave out for every category; it is chosen when the template is made and can't be changed later), `description` (up to 500 characters), `icon` (a Font Awesome name such as `fa-scroll`), `rename_to`, and the markdown words (up to 20000 characters). `create` needs the words; on `update` a key left out keeps what is saved, and words, when given, replace the template's text. A template's grey writing prompts (the hints under its headings) can't be written in markdown: sending its text back unchanged keeps them, and changing the text replaces them with the text. `delete` removes the template. The templates every campaign shares can't be changed. Only the campaign's owner can change page templates. `what: layouts` with `part: page-templates` lists them all, written so they can be sent back changed.\n\n```\n---\nkind: page-template\nname: Tavern\ncategory: Locations\ndescription: A place to drink and hear rumours.\nicon: fa-beer-mug-empty\n---\n## The room\n\nWho is here, and what do they want?\n\n## Rumours\n\n- A rumour the party can follow\n```"
}

// pageTemplateTarget is a record resolved against the template it names.
type pageTemplateTarget struct {
	hit *entities.ContentTemplate // nil when none matches
	cat *entities.EntityType      // the `category`, nil when the record names none
	all []entities.ContentTemplate
	// rename, desc and icon are what the record sets; has* tells "left out,
	// keep it" from a value.
	rename, desc, icon        string
	hasRename, hasDesc, hasIc bool
}

func (k PageTemplateKind) owner(ctx context.Context, campaignID string, a Actor) error {
	if k.Svc == nil {
		return badRequestf("page templates can't be changed on this server")
	}
	if a.CanAuthorDmOnly() {
		if ok, err := k.Svc.IsOwner(ctx, campaignID, a.UserID); err != nil {
			return err
		} else if ok {
			return nil
		}
	}
	return badRequestf("only the campaign's owner can change page templates")
}

// templateCategory is the id of the category a template is for; 0 for all.
func templateCategory(t *entities.ContentTemplate) int {
	if t.EntityTypeID == nil {
		return 0
	}
	return *t.EntityTypeID
}

func (k PageTemplateKind) resolve(ctx context.Context, campaignID string, r Record) (pageTemplateTarget, error) {
	var t pageTemplateTarget
	name := strings.TrimSpace(r.Name)
	if name == "" {
		return t, badRequestf("name the page template in `name:`")
	}
	var err error
	if t.all, err = k.Svc.List(ctx, campaignID); err != nil {
		return t, err
	}
	if c := strings.TrimSpace(r.Str("category")); c != "" {
		types, err := k.Svc.Categories(ctx, campaignID)
		if err != nil {
			return t, err
		}
		if t.cat = typeNamed(types, c); t.cat == nil {
			return t, badRequestf("there is no category called %s", quote(c))
		}
	}
	// A record that names a category finds the template of that category; one
	// that names none finds the only template of the name.
	var found []*entities.ContentTemplate
	for i := range t.all {
		x := &t.all[i]
		if !sameName(x.Name, name) {
			continue
		}
		if t.cat != nil && templateCategory(x) != t.cat.ID {
			continue
		}
		found = append(found, x)
	}
	switch len(found) {
	case 0:
	case 1:
		t.hit = found[0]
	default:
		return t, badRequestf("more than one page template is called %s; add `category:` to say which", quote(name))
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
			return t, badRequestf("icon must be a Font Awesome icon name such as fa-scroll")
		}
	}
	return t, nil
}

// sameWords says whether the markdown is what the template's saved text
// already reads as, so words sent back as they were read change nothing.
func sameWords(body string, t *entities.ContentTemplate) bool {
	return normalizeWords(body) == normalizeWords(writtenMarkdown(t.ContentHTML))
}

func (k PageTemplateKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	if err := k.owner(ctx, campaignID, a); err != nil {
		return Plan{Error: planError(err)}
	}
	t, err := k.resolve(ctx, campaignID, r)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	name := quote(strings.TrimSpace(r.Name))
	if utf8.RuneCountInString(r.Name) > 200 || utf8.RuneCountInString(t.rename) > 200 {
		return Plan{Error: "a page template's name is at most 200 characters"}
	}
	if len(t.desc) > 500 {
		return Plan{Error: "a description is at most 500 characters"}
	}
	if utf8.RuneCountInString(r.Body) > maxPageTemplateText {
		return Plan{Error: "the words run over " + strconv.Itoa(maxPageTemplateText) + " characters"}
	}
	if t.hit != nil && t.hit.IsGlobal {
		return Plan{Error: "the page template " + name + " is shared by every campaign and can't be changed or removed; create one of your own under another name"}
	}
	if importer.HasSecretMarkerText(r.Body) {
		return Plan{Error: "a page template is starter text every new page begins with, so it can't hold GM-only text"}
	}
	where := ""
	if t.cat != nil {
		where = " for " + t.cat.NamePlural
	}
	switch r.Action {
	case ActionCreate:
		if t.hit != nil {
			return Plan{Error: "a page template called " + name + where + " already exists; use action: update"}
		}
		if t.hasRename {
			return Plan{Error: "a new page template has nothing to rename; leave out `rename_to`"}
		}
		if strings.TrimSpace(r.Body) == "" {
			return Plan{Error: "write the template's words in markdown after the block"}
		}
		return Plan{Summary: "adds the page template " + name + where + ": " + quoted(r.Body)}
	case ActionUpdate:
		if t.hit == nil {
			if t.cat != nil {
				return Plan{Error: "no page template called " + name + " for " + t.cat.NamePlural + "; a template's category can't be changed once it is made, so create a new one"}
			}
			return Plan{Error: "no page template called " + name + "; use action: create to add one"}
		}
		var parts []string
		var warnings []string
		if t.hasRename && t.rename == "" {
			return Plan{Error: "`rename_to` needs a name"}
		}
		if t.hasRename && t.rename != t.hit.Name {
			parts = append(parts, "is renamed "+quote(t.rename))
		}
		if t.hasDesc && t.desc != t.hit.Description {
			parts = append(parts, "gets the description "+quoted(t.desc))
		}
		if t.hasIc && t.icon != "" && t.icon != t.hit.Icon {
			parts = append(parts, "gets the icon "+t.icon)
		}
		if strings.TrimSpace(r.Body) != "" && !sameWords(r.Body, t.hit) {
			parts = append(parts, "has its text replaced: "+quoted(r.Body)+" (was "+quoted(writtenMarkdown(t.hit.ContentHTML))+")")
			if strings.Contains(t.hit.ContentJSON, `"prompt"`) {
				warnings = append(warnings, "its grey writing prompts are replaced by this text")
			}
		}
		if len(parts) == 0 {
			return Plan{Error: "the page template " + name + " is already like this"}
		}
		return Plan{Summary: "the page template " + name + " " + strings.Join(parts, "; "), Warnings: warnings}
	default:
		if t.hit == nil {
			return Plan{Error: "no page template called " + name}
		}
		return Plan{Summary: "removes the page template " + name + where + "; pages already written from it keep their text"}
	}
}

func (k PageTemplateKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
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
	var doc, html *string
	if strings.TrimSpace(r.Body) != "" && (r.Action == ActionCreate || !sameWords(r.Body, t.hit)) {
		if r.Text == nil {
			return badRequestf("page template: the words were applied without their Text")
		}
		if doc, html, err = r.Text.Entry(r.Body); err != nil {
			return err
		}
		if doc == nil || html == nil {
			return badRequestf("write the template's words in markdown after the block")
		}
	}
	if r.Action == ActionCreate {
		in := entities.CreateContentTemplateInput{
			Name: strings.TrimSpace(r.Name), Description: t.desc, Icon: t.icon,
			ContentJSON: *doc, ContentHTML: *html,
		}
		if t.cat != nil {
			in.EntityTypeID = t.cat.ID
		}
		_, err := k.Svc.Create(ctx, campaignID, in)
		return err
	}
	// Only what the record sets goes in the update, so a rename never
	// rewrites the text and new text never touches the name.
	var in entities.UpdateContentTemplateInput
	if t.hasRename && t.rename != t.hit.Name {
		in.Name = patch.Of(t.rename)
	}
	if t.hasDesc && t.desc != t.hit.Description {
		in.Description = patch.Of(t.desc)
	}
	if t.hasIc && t.icon != "" && t.icon != t.hit.Icon {
		in.Icon = patch.Of(t.icon)
	}
	if doc != nil {
		in.ContentJSON, in.ContentHTML = patch.Of(*doc), patch.Of(*html)
	}
	return k.Svc.Update(ctx, campaignID, t.hit.ID, in)
}

// templateRecord writes one page template as the `kind: page-template` block
// that sends it back, its words as the markdown after the block.
func (k PageTemplateKind) templateRecord(b *strings.Builder, t entities.ContentTemplate, category string) {
	b.WriteString("---\nkind: page-template\nname: " + quote(t.Name) + "\n")
	if category != "" {
		b.WriteString("category: " + quote(category) + "\n")
	}
	if t.Description != "" {
		b.WriteString("description: " + quote(t.Description) + "\n")
	}
	b.WriteString("icon: " + quote(t.Icon) + "\n---\n")
	if words := writtenMarkdown(t.ContentHTML); words != "" {
		b.WriteString(words + "\n")
	}
	b.WriteString("\n")
}

// categoryName is the name of the category with the id, "" for none.
func categoryName(types []entities.EntityType, id int) string {
	for _, et := range types {
		if et.ID == id {
			return et.Name
		}
	}
	return ""
}

// Export writes the campaign's own page templates, for the owner only. The
// ones every campaign shares are not written: they can't be sent back.
func (k PageTemplateKind) Export(ctx context.Context, campaignID string, a Actor) (string, error) {
	if k.owner(ctx, campaignID, a) != nil {
		return "", nil
	}
	all, err := k.Svc.List(ctx, campaignID)
	if err != nil {
		return "", err
	}
	types, err := k.Svc.Categories(ctx, campaignID)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, t := range all {
		if !t.IsGlobal {
			k.templateRecord(&b, t, categoryName(types, templateCategory(&t)))
		}
	}
	return b.String(), nil
}

// templateState is a page template's own fields, which is what an undo puts
// back and what its fingerprint covers; its dates are left out.
type templateState struct {
	ID           int    `json:"id,omitempty"`
	EntityTypeID int    `json:"entityTypeId,omitempty"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Icon         string `json:"icon"`
	ContentJSON  string `json:"contentJson"`
	ContentHTML  string `json:"contentHtml"`
}

func templateStateOf(t *entities.ContentTemplate) templateState {
	if t == nil {
		return templateState{}
	}
	return templateState{ID: t.ID, EntityTypeID: templateCategory(t), Name: t.Name, Description: t.Description, Icon: t.Icon, ContentJSON: t.ContentJSON, ContentHTML: t.ContentHTML}
}

var _ aiplug.Undoable = PageTemplateKind{}

// The snapshot's target says what an undo has to do: remove a template the
// record made ("made:<category id>:<name>"), put an edited one back
// ("edit:<id>") or bring a removed one back ("gone:<id>").
const (
	templateMade = "made"
	templateEdit = "edit"
	templateGone = "gone"
)

// Before keeps the template as it is now. One the record makes has nothing to
// keep: its undo removes it again.
func (k PageTemplateKind) Before(ctx context.Context, campaignID string, _ Actor, r Record) (*aiplug.Snapshot, error) {
	if k.Svc == nil {
		return nil, nil
	}
	t, err := k.resolve(ctx, campaignID, r)
	if err != nil {
		return nil, nil
	}
	name := strings.TrimSpace(r.Name)
	if t.hit == nil {
		if r.Action != ActionCreate {
			return nil, nil
		}
		cat := 0
		if t.cat != nil {
			cat = t.cat.ID
		}
		return &aiplug.Snapshot{Target: templateMade + ":" + strconv.Itoa(cat) + ":" + name, Name: "the page template " + quote(name), Data: "{}"}, nil
	}
	if t.hit.IsGlobal || r.Action == ActionCreate {
		return nil, nil
	}
	data, err := json.Marshal(templateStateOf(t.hit))
	if err != nil {
		return nil, err
	}
	mode := templateEdit
	if r.Action == ActionDelete {
		mode = templateGone
	}
	return &aiplug.Snapshot{Target: mode + ":" + strconv.Itoa(t.hit.ID), Name: "the page template " + quote(t.hit.Name), Data: string(data)}, nil
}

// current is the template a snapshot's target names now, nil when gone.
func (k PageTemplateKind) current(ctx context.Context, campaignID, target string) (string, *entities.ContentTemplate, error) {
	parts := strings.SplitN(target, ":", 3)
	mode := parts[0]
	all, err := k.Svc.List(ctx, campaignID)
	if err != nil {
		return mode, nil, err
	}
	if mode == templateMade && len(parts) == 3 {
		cat, _ := strconv.Atoi(parts[1])
		for i := range all {
			if !all[i].IsGlobal && templateCategory(&all[i]) == cat && sameName(all[i].Name, parts[2]) {
				return mode, &all[i], nil
			}
		}
		return mode, nil, nil
	}
	id, convErr := strconv.Atoi(parts[len(parts)-1])
	if convErr != nil || len(parts) != 2 {
		return mode, nil, badRequestf("this change can't be undone: its page template is not known")
	}
	for i := range all {
		if all[i].ID == id {
			return mode, &all[i], nil
		}
	}
	return mode, nil, nil
}

// Fingerprint is the template now; one that is gone fingerprints as none.
func (k PageTemplateKind) Fingerprint(ctx context.Context, campaignID string, _ Actor, s aiplug.Snapshot) (string, error) {
	_, t, err := k.current(ctx, campaignID, s.Target)
	if err != nil {
		return "", err
	}
	return aiplug.Hash(templateStateOf(t)), nil
}

// Restore takes a made template away, puts an edited one's fields back, or
// makes a removed one again (the new copy sorts last, as any new template does).
func (k PageTemplateKind) Restore(ctx context.Context, campaignID string, _ Actor, s aiplug.Snapshot) error {
	mode, t, err := k.current(ctx, campaignID, s.Target)
	if err != nil {
		return err
	}
	var was templateState
	if err := json.Unmarshal([]byte(s.Data), &was); err != nil {
		return err
	}
	switch mode {
	case templateMade:
		if t == nil {
			return nil
		}
		if t.IsGlobal {
			return badRequestf("this change can't be undone: a shared template can't be removed")
		}
		return k.Svc.Delete(ctx, campaignID, t.ID)
	case templateGone:
		if t != nil {
			return nil
		}
		_, err := k.Svc.Create(ctx, campaignID, entities.CreateContentTemplateInput{
			EntityTypeID: was.EntityTypeID, Name: was.Name, Description: was.Description, Icon: was.Icon,
			ContentJSON: was.ContentJSON, ContentHTML: was.ContentHTML,
		})
		return err
	}
	if t == nil {
		return badRequestf("this change can't be undone: the page template is gone")
	}
	return k.Svc.Update(ctx, campaignID, t.ID, entities.UpdateContentTemplateInput{
		Name: patch.Of(was.Name), Description: patch.Of(was.Description), Icon: patch.Of(was.Icon),
		ContentJSON: patch.Of(was.ContentJSON), ContentHTML: patch.Of(was.ContentHTML),
	})
}
