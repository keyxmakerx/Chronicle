package entities

// era_pages.go: pages that change with the era. The campaign's main
// calendar's current date picks an era; a page with a version for it is
// shown, edited and previewed as that version. Only the Owner and members
// with DM access see other eras, through the Eras panel; everyone else sees
// the page as it is now. What a version stores and which era applies belong
// to the calendar plugin, reached through EraPages.

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// PageEra is one era of the main calendar as the Eras panel draws it.
type PageEra struct {
	ID     int
	Name   string
	Color  string
	Color2 string
	// Span says when it runs, in the calendar's own years.
	Span string
	// Hidden is set while the era is still hidden from players.
	Hidden bool
	// Weight is its share of the ribbon (square-root lengths, with a floor).
	Weight float64
	// Here is where "you are here" sits along the current era (0 to 1), or
	// -1 for every other era.
	Here float64
}

// PageEraSet is the main calendar's eras for one viewer, oldest first.
type PageEraSet struct {
	CalendarName string
	Eras         []PageEra
	// CurrentID is the era the calendar's date is in; 0 when none.
	CurrentID int
}

// Era returns the era with id, or nil.
func (s *PageEraSet) Era(id int) *PageEra {
	if s == nil {
		return nil
	}
	for i := range s.Eras {
		if s.Eras[i].ID == id {
			return &s.Eras[i]
		}
	}
	return nil
}

// PageEraVersion is a page's version for one era: a nil field is the usual
// page's, and FieldsData holds only the values that differ from it.
type PageEraVersion struct {
	Name       *string
	ImagePath  *string
	Entry      *string
	EntryHTML  *string
	EntryRev   int
	FieldsData map[string]any
	UpdatedBy  string
	UpdatedAt  time.Time
}

// EraEntryClash is the stored text an era version's save collided with.
type EraEntryClash struct {
	Rev       int
	Entry     *string
	EntryHTML *string
	ByUserID  string
	At        time.Time
}

// EraPages is the calendar plugin's side of pages by era, wired in
// internal/app. Writes are checked against the campaign there too.
type EraPages interface {
	PageEras(ctx context.Context, campaignID string, v permissions.Viewer) (*PageEraSet, error)
	Version(ctx context.Context, entityID string, eraID int) (*PageEraVersion, error)
	VersionEraIDs(ctx context.Context, entityID string) ([]int, error)
	VersionsFor(ctx context.Context, entityIDs []string, eraID int) (map[string]*PageEraVersion, error)
	CreateVersion(ctx context.Context, campaignID, entityID string, eraID int, userID string) error
	SetVersionIdentity(ctx context.Context, campaignID, entityID string, eraID int, name, imagePath patch.Field[string], userID string) error
	SaveVersionEntry(ctx context.Context, campaignID, entityID string, eraID int, entry, entryHTML string, baseRev *int, userID string) (int, *EraEntryClash, error)
	SaveVersionFields(ctx context.Context, campaignID, entityID string, eraID int, changed map[string]any, userID string) error
	DeleteVersion(ctx context.Context, campaignID, entityID string, eraID int) error
}

// SetEraPages wires pages by era. Unset, every page is its usual self.
func (h *Handler) SetEraPages(p EraPages) { h.eraPages = p }

// eraUsual is the Eras panel's choice of the page with no era applied.
const eraUsual = -1

// eraView is how one request shows a page through the eras.
type eraView struct {
	Set *PageEraSet
	// Selected is the era picked in the Eras panel (eraUsual for the usual
	// page); 0 when the page is shown as it is now.
	Selected int
	// ShownID is the era whose version is on the page; 0 for the usual page.
	ShownID int
	Version *PageEraVersion
	// HasVersion lists the eras this page has a version for (DM only).
	HasVersion map[int]bool
	// DM may switch eras and make versions.
	DM bool
	// Highlight marks what the shown version changes from the usual page.
	Highlight bool
	// Usual is the page before the version was laid over it.
	Usual *Entity
	// Diff is what the shown version changes (DM only).
	Diff *eraDiff
}

// Open reports whether the Eras panel is open (an era or the usual page was
// picked).
func (v *eraView) Open() bool { return v != nil && v.Selected != 0 }

// Shown reports whether a version is on the page.
func (v *eraView) Shown() bool { return v != nil && v.Version != nil }

// ShownEra is the era whose version is on the page, or nil.
func (v *eraView) ShownEra() *PageEra {
	if v == nil || v.ShownID == 0 {
		return nil
	}
	return v.Set.Era(v.ShownID)
}

// SelectedEra is the era picked in the panel, or nil.
func (v *eraView) SelectedEra() *PageEra {
	if v == nil || v.Selected <= 0 {
		return nil
	}
	return v.Set.Era(v.Selected)
}

func (v *eraView) currentEra() *PageEra {
	if v == nil || v.Set == nil {
		return nil
	}
	return v.Set.Era(v.Set.CurrentID)
}

// resolveEraView works out which version a page is shown with. Everyone gets
// the current era's version when the page has one; a DM can pick another
// era (?era=<id>) or the usual page (?era=usual), and turn highlighting off
// with hl=0. A failure to read eras shows the usual page.
func (h *Handler) resolveEraView(ctx context.Context, cc *campaigns.CampaignContext, entity *Entity, userID, eraParam, hlParam string) *eraView {
	if h.eraPages == nil {
		return nil
	}
	v := permissions.RequestViewer(int(cc.VisibilityRole()), userID)
	set, err := h.eraPages.PageEras(ctx, cc.Campaign.ID, v)
	if err != nil || set == nil || len(set.Eras) == 0 {
		return nil
	}
	view := &eraView{Set: set, DM: cc.CanAuthorDmOnly()}
	if view.DM {
		view.HasVersion = map[int]bool{}
		if ids, err := h.eraPages.VersionEraIDs(ctx, entity.ID); err == nil {
			for _, id := range ids {
				view.HasVersion[id] = true
			}
		}
		switch {
		case eraParam == "usual":
			view.Selected = eraUsual
		case eraParam != "":
			if id, err := strconv.Atoi(eraParam); err == nil && set.Era(id) != nil {
				view.Selected = id
			}
		}
	}
	target := set.CurrentID
	if view.Selected != 0 {
		target = view.Selected
	}
	if target > 0 && (!view.DM || view.HasVersion[target]) {
		if ver, err := h.eraPages.Version(ctx, entity.ID, target); err == nil && ver != nil {
			view.Version = ver
			view.ShownID = target
		}
	}
	view.Highlight = view.DM && view.Selected > 0 && view.Version != nil && hlParam != "0"
	return view
}

// withEraVersion returns a copy of e with v laid over it.
func withEraVersion(e *Entity, v *PageEraVersion) *Entity {
	out := *e
	if v == nil {
		return &out
	}
	if v.Name != nil {
		out.Name = *v.Name
	}
	if v.ImagePath != nil {
		out.ImagePath = v.ImagePath
	}
	if v.Entry != nil {
		out.Entry = v.Entry
		out.EntryHTML = v.EntryHTML
	}
	out.FieldsData = mergeEraFields(e.FieldsData, v.FieldsData)
	return &out
}

// mergeEraFields lays a version's changed values over the usual ones. A key
// the version holds as null is cleared in that era.
func mergeEraFields(usual, changed map[string]any) map[string]any {
	out := make(map[string]any, len(usual)+len(changed))
	for k, val := range usual {
		out[k] = val
	}
	for k, val := range changed {
		if val == nil {
			delete(out, k)
			continue
		}
		out[k] = val
	}
	return out
}

// eraFieldChanges keeps only the values in full that differ from usual, so a
// field edited back to its usual value stops being a change. A usual value
// missing from full is stored as null (cleared in that era).
func eraFieldChanges(usual, full map[string]any) map[string]any {
	out := map[string]any{}
	for k, val := range full {
		if !sameFieldValue(val, usual[k]) {
			out[k] = val
		}
	}
	for k, val := range usual {
		if _, ok := full[k]; !ok && !isBlankField(val) {
			out[k] = nil
		}
	}
	return out
}

// sameFieldValue compares two field values the way they would be stored, so
// 3 and 3.0, or "" and an absent value, count as the same.
func sameFieldValue(a, b any) bool {
	if isBlankField(a) && isBlankField(b) {
		return true
	}
	ja, errA := json.Marshal(a)
	jb, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return reflect.DeepEqual(a, b)
	}
	var na, nb any
	_ = json.Unmarshal(ja, &na)
	_ = json.Unmarshal(jb, &nb)
	return reflect.DeepEqual(na, nb)
}

func isBlankField(v any) bool {
	if v == nil {
		return true
	}
	s, ok := v.(string)
	return ok && s == ""
}

// mayWriteEraVersion reports whether this member may change a page's version
// for eraID. A DM may change any era's; anyone else who can edit the page
// only the current era's, which is the version they see on the page.
func mayWriteEraVersion(cc *campaigns.CampaignContext, set *PageEraSet, eraID int) bool {
	if cc.CanAuthorDmOnly() {
		return set.Era(eraID) != nil
	}
	return set != nil && eraID > 0 && eraID == set.CurrentID
}

// eraDiff is what the shown version changes from the usual page, worked out
// for the DM's Eras panel and highlights.
type eraDiff struct {
	Name   bool
	Image  bool
	Fields int
	// Text is the version's text block by block; nil when the version has
	// no text of its own.
	Text []EraDiffBlock
}

// Count is how many things differ from the usual page.
func (d *eraDiff) Count() int {
	if d == nil {
		return 0
	}
	n := d.Fields + EraDiffCount(d.Text)
	if d.Name {
		n++
	}
	if d.Image {
		n++
	}
	return n
}

// diffEraVersion compares a version with the usual page.
func diffEraVersion(usual *Entity, v *PageEraVersion) *eraDiff {
	d := &eraDiff{Fields: len(v.FieldsData)}
	d.Name = v.Name != nil && *v.Name != usual.Name
	d.Image = v.ImagePath != nil && (usual.ImagePath == nil || *v.ImagePath != *usual.ImagePath)
	if v.Entry != nil {
		d.Text = EraDiff(derefStr(usual.EntryHTML), derefStr(v.EntryHTML))
	}
	return d
}

type eraViewKey struct{}

// withEraView carries the request's era view to the page templates.
func withEraView(ctx context.Context, v *eraView) context.Context {
	return context.WithValue(ctx, eraViewKey{}, v)
}

// eraViewOf returns the era view the show handler resolved, or nil when the
// page is drawn as its usual self.
func eraViewOf(ctx context.Context) *eraView {
	v, _ := ctx.Value(eraViewKey{}).(*eraView)
	return v
}

// applyEraView resolves the era view for the show page and lays the shown
// version over the entity. Pages a game system draws keep their usual self:
// their sheets read and write the usual fields directly.
func (h *Handler) applyEraView(ctx context.Context, cc *campaigns.CampaignContext, entity *Entity, entityType *EntityType, userID, eraParam, hlParam string) (*Entity, *eraView) {
	if hasEntityShowRenderer(entityType) {
		return entity, nil
	}
	v := h.resolveEraView(ctx, cc, entity, userID, eraParam, hlParam)
	if v == nil {
		return entity, nil
	}
	v.Usual = entity
	if v.Version == nil {
		return entity, v
	}
	if v.DM {
		v.Diff = diffEraVersion(entity, v.Version)
	}
	return withEraVersion(entity, v.Version), v
}

// versionPath is the shown version's route under the page, or "" when the
// page is shown as its usual self.
func (v *eraView) versionPath(campaignID, entityID string) string {
	if !v.Shown() {
		return ""
	}
	return fmt.Sprintf("/campaigns/%s/entities/%s/eras/%d", campaignID, entityID, v.ShownID)
}

// widgetEndpoint is where a page widget reads and saves: the shown version's
// route when a version is on the page, else the usual one.
func widgetEndpoint(ctx context.Context, campaignID, entityID, leaf string) string {
	if p := eraViewOf(ctx).versionPath(campaignID, entityID); p != "" {
		return p + "/" + leaf
	}
	return fmt.Sprintf("/campaigns/%s/entities/%s/%s", campaignID, entityID, leaf)
}

// usualEntity is the page before any era version was laid over it, for the
// controls that change the usual page (the Edit name fold).
func usualEntity(ctx context.Context, e *Entity) *Entity {
	if v := eraViewOf(ctx); v != nil && v.Usual != nil {
		return v.Usual
	}
	return e
}

// HighlightText reports whether the text is drawn as highlights instead of
// the editor.
func (v *eraView) HighlightText() bool {
	return v != nil && v.Highlight && v.Diff != nil && v.Diff.Text != nil
}

// Editing reports whether the DM opened a version to edit (highlights off).
func (v *eraView) Editing() bool {
	return v != nil && v.DM && v.Selected > 0 && v.Version != nil && !v.Highlight
}

// FocusID is the era the panel's details describe: the one picked, else the
// current one.
func (v *eraView) FocusID() int {
	if v == nil {
		return 0
	}
	if v.Selected > 0 {
		return v.Selected
	}
	if v.Selected == eraUsual {
		return 0
	}
	return v.Set.CurrentID
}

// VersionCount is how many eras the page has a version for.
func (v *eraView) VersionCount() int {
	if v == nil {
		return 0
	}
	return len(v.HasVersion)
}

// hereLeft is where "you are here" sits along the whole ribbon, in percent,
// or -1 when the calendar's date is in no era.
func (s *PageEraSet) hereLeft() float64 {
	total, before := 0.0, 0.0
	at := -1.0
	for _, e := range s.Eras {
		if e.Here >= 0 {
			at = before + e.Here*e.Weight
		}
		before += e.Weight
		total += e.Weight
	}
	if at < 0 || total <= 0 {
		return -1
	}
	return at / total * 100
}

// eraPageLink is the page opened on a choice in the Eras panel.
func eraPageLink(campaignID, entityID, era string, extra string) templ.SafeURL {
	u := fmt.Sprintf("/campaigns/%s/entities/%s", campaignID, entityID)
	if era != "" {
		u += "?era=" + era
		if extra != "" {
			u += "&" + extra
		}
	}
	return templ.SafeURL(u)
}

// currentEraEntity is the page as everyone sees it now: with its version for
// the main calendar's era laid over, when it has one. Hover cards and peeks
// use it so they match the page.
func (h *Handler) currentEraEntity(ctx context.Context, cc *campaigns.CampaignContext, entity *Entity, entityType *EntityType, userID string) *Entity {
	if h.eraPages == nil || hasEntityShowRenderer(entityType) {
		return entity
	}
	set, err := h.eraPages.PageEras(ctx, cc.Campaign.ID, permissions.RequestViewer(int(cc.VisibilityRole()), userID))
	if err != nil || set == nil || set.CurrentID == 0 {
		return entity
	}
	v, err := h.eraPages.Version(ctx, entity.ID, set.CurrentID)
	if err != nil || v == nil {
		return entity
	}
	return withEraVersion(entity, v)
}

// eraDiffSummary says how many things a version changes.
func eraDiffSummary(n int) string {
	switch n {
	case 0:
		return "The same as the usual page so far."
	case 1:
		return "1 difference from the usual page."
	}
	return strconv.Itoa(n) + " differences from the usual page."
}

// highlightQuery is the query that turns highlights on or off.
func highlightQuery(on bool) string {
	if on {
		return ""
	}
	return "hl=0"
}

// eraFieldsAttrs tells the attributes widget it shows an era version, and
// whether to mark what the version changes.
func eraFieldsAttrs(ctx context.Context) templ.Attributes {
	v := eraViewOf(ctx)
	if !v.Shown() {
		return templ.Attributes{}
	}
	a := templ.Attributes{"data-era": "true"}
	if v.Open() {
		a["class"] = "era-settle"
	}
	if v.Highlight {
		a["data-era-highlight"] = "true"
	}
	return a
}
