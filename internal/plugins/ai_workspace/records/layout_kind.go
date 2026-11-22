package records

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/tilegrid"
)

// LayoutsAPI is a category's page layouts, as the owner arranges them in
// Customize › Categories: the main layout every page of the category wears
// by default, and the named layouts beside it.
type LayoutsAPI interface {
	// IsOwner reports whether userID owns the campaign; the Customizer's
	// categories are the owner's.
	IsOwner(ctx context.Context, campaignID, userID string) (bool, error)
	// Categories are the campaign's categories with their main layouts.
	Categories(ctx context.Context, campaignID string) ([]entities.EntityType, error)
	// Named is the category's named layouts; ok is false while the campaign
	// keeps one layout per category (the layouts store is degraded).
	Named(ctx context.Context, et *entities.EntityType) (pl entities.PageLayouts, ok bool)
	// BlockTypes are the blocks a page layout can place in this campaign.
	BlockTypes(ctx context.Context, campaignID string) []LayoutBlockInfo
	// Check runs the rules a saved layout must meet, without saving.
	Check(ctx context.Context, campaignID string, typeID int, l entities.EntityTypeLayout) error
	// SaveMain makes l the category's main layout.
	SaveMain(ctx context.Context, campaignID string, typeID int, l entities.EntityTypeLayout) error
	// SaveNamed stores the category's named layouts, all of them.
	SaveNamed(ctx context.Context, campaignID string, typeID int, p entities.PageLayouts) error
}

// LayoutBlockInfo is one block a layout can place, as the Customizer's palette
// names it.
type LayoutBlockInfo struct{ Type, Label, Description string }

// layoutWords are the layout blocks whose `text` is markdown kept as HTML.
var layoutWords = map[string]bool{"text_block": true}

// LayoutKind reads and rearranges a category's page layouts: which blocks the
// pages of a category hold, in which rows and columns, and which of them only
// the DM team sees. A block already on the layout keeps its settings when the
// record lists it again, so rearranging never loses a choice the owner made.
// Tile sections stay as the Customizer arranged them.
type LayoutKind struct{ Svc LayoutsAPI }

func (LayoutKind) Name() string  { return "layout" }
func (LayoutKind) Label() string { return "Page layout" }
func (LayoutKind) Brief() string {
	return "the page layout every page of a category wears: which blocks it holds and where, which are for the DM team only, and the words in its Text blocks; also adds a named layout (owner only)"
}
func (LayoutKind) Doc() string {
	return "The layout the pages of one category wear, matched by `category` (its name) and, for a named layout, `layout` (its name). Without `layout` it is the category's main layout. Key: `rows`, a list of rows, each a list of `columns`; a column has a `width` (1 to 12, a row's widths add to 12) and `blocks`, in the order shown. A block is its type written alone, or `{type: text_block, text: \"words\"}` to write a Text block's words in markdown; any block may carry `directors_only: true` (only the DM team sees it) or `false`, and leaving it out keeps the block's current setting. A block already on the layout keeps its settings when listed again, so list every block to keep; one left out is removed. A row `- tiles: kept` keeps a tile section the layout already has, unchanged and in order: tiles are arranged only in Customize, and a tile section left out is removed. `create` and `update` replace the whole arrangement; `create` with a `layout` name the category does not have adds a new named layout. To remove a layout, use Customize › Categories, since pages wearing it change too. Only the campaign's owner can change a layout. `what: layouts` lists every layout as it is now, with the blocks the campaign can place, written so it can be sent back changed.\n\n```\n---\nkind: layout\ncategory: Characters\nlayout: NPC\nrows:\n  - columns:\n      - width: 8\n        blocks: [title, entry, {type: trackers, directors_only: true}]\n      - width: 4\n        blocks: [image, details]\n  - tiles: kept\n---\n```"
}

func (k LayoutKind) Title(r Record) string {
	if l := r.Str("layout"); l != "" {
		return r.Str("category") + " · " + l
	}
	return r.Str("category") + " · main layout"
}

// layoutLoc is the layout a record names, found among the category's own.
type layoutLoc struct {
	et *entities.EntityType
	pl entities.PageLayouts
	// named is false while the campaign keeps one layout per category.
	named bool
	// key is "main" or a named layout's key; label is how it reads.
	key, label string
	// isNew marks a named layout the record adds; its key is made from label.
	isNew bool
	was   []gridRow
}

func (l layoutLoc) isMain() bool { return l.key == entities.MainLayoutKey }

// where names the layout in a summary: "the Characters NPC layout".
func (l layoutLoc) where() string {
	if l.isMain() && l.pl.MainName == "" {
		return "the " + l.et.NamePlural + " main layout"
	}
	return "the " + l.et.NamePlural + " " + l.label + " layout"
}

func (k LayoutKind) owner(ctx context.Context, campaignID string, a Actor) error {
	if k.Svc == nil {
		return badRequestf("page layouts can't be changed on this server")
	}
	if a.CanAuthorDmOnly() {
		if ok, err := k.Svc.IsOwner(ctx, campaignID, a.UserID); err != nil {
			return err
		} else if ok {
			return nil
		}
	}
	return badRequestf("only the campaign's owner can change page layouts")
}

// locate finds the category and layout a record names. The rows it carries
// are not read here, so Before can use it too.
func (k LayoutKind) locate(ctx context.Context, campaignID string, r Record) (layoutLoc, error) {
	var loc layoutLoc
	name := strings.TrimSpace(r.Str("category"))
	if name == "" {
		return loc, badRequestf("name the category in `category:`")
	}
	types, err := k.Svc.Categories(ctx, campaignID)
	if err != nil {
		return loc, err
	}
	if loc.et = typeNamed(types, name); loc.et == nil {
		return loc, badRequestf("there is no category called %s", quote(name))
	}
	loc.pl, loc.named = k.Svc.Named(ctx, loc.et)
	word := strings.TrimSpace(r.Str("layout"))
	if word == "" {
		word = entities.MainLayoutKey
	}
	if key, ok := loc.pl.KeyByName(loc.et, word); ok {
		loc.key, loc.label = key, loc.pl.Label(loc.et, key)
		if key == entities.MainLayoutKey {
			loc.was = gridFromLayout(loc.et.Layout)
		} else {
			f, _ := loc.pl.Find(key)
			loc.was = gridFromLayout(f.Layout)
		}
		return loc, nil
	}
	if r.Action != ActionCreate {
		return loc, badRequestf("%s has no layout called %s; use action: create to add one", loc.et.Name, quote(word))
	}
	if !loc.named {
		return loc, badRequestf("this campaign's categories have one layout each for now, so a new one can't be added")
	}
	if len(loc.pl.Extra) >= entities.MaxPageLayouts {
		return loc, badRequestf("%s already has %d layouts besides its main one, the most a category keeps", loc.et.Name, entities.MaxPageLayouts)
	}
	key, err := layoutKeyOf(word, loc.pl)
	if err != nil {
		return loc, err
	}
	loc.key, loc.label, loc.isNew = key, word, true
	return loc, nil
}

var layoutKeyJunk = regexp.MustCompile(`[^a-z0-9]+`)

// layoutKeyOf makes a new layout's key from its name: lowercase letters,
// digits and dashes, starting with a letter, and not one the category has.
func layoutKeyOf(name string, pl entities.PageLayouts) (string, error) {
	base := strings.Trim(layoutKeyJunk.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(base) > 28 {
		base = strings.Trim(base[:28], "-")
	}
	if base == "" || base[0] < 'a' || base[0] > 'z' {
		return "", badRequestf("a layout's name starts with a letter")
	}
	key := base
	for i := 2; ; i++ {
		_, taken := pl.Find(key)
		if !taken && key != entities.MainLayoutKey {
			return key, nil
		}
		key = base + "-" + strconv.Itoa(i)
	}
}

// layoutTarget is a record resolved against the layout it replaces.
type layoutTarget struct {
	loc   layoutLoc
	next  []gridRow
	words []homeWordsChange
}

func (k LayoutKind) resolve(ctx context.Context, campaignID string, r Record) (layoutTarget, error) {
	t := layoutTarget{}
	var err error
	if t.loc, err = k.locate(ctx, campaignID, r); err != nil {
		return t, err
	}
	items := r.List("rows")
	if len(items) == 0 {
		return t, badRequestf("list the layout's `rows:`, each with its `columns`")
	}
	palette := map[string]bool{}
	for _, b := range k.Svc.BlockTypes(ctx, campaignID) {
		palette[b.Type] = true
	}
	// A block the palette no longer offers (its add-on is off, or another
	// block replaced it) can still be kept where it is.
	onLayout := map[string]bool{}
	for _, row := range t.loc.was {
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
		tileRow:  keepTiles(t.loc.was),
		block:    layoutBlockRule,
	}
	t.next, t.words, err = parseGrid(items, t.loc.was, rules)
	return t, err
}

// layoutBlockRule is what a page layout adds to one block: an extension
// widget can only stay where it is, and `directors_only` sets or clears the
// block's DM-only visibility (left out keeps it).
func layoutBlockRule(raw any, spec homeSpec, b *HomeBlock, kept bool) error {
	if spec.typ == "ext_widget" && !kept {
		return fmt.Errorf("an extension widget block can only be kept where it is, not added")
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	dv, has := m["directors_only"]
	if !has {
		return nil
	}
	on, ok := dv.(bool)
	if !ok {
		return fmt.Errorf("`directors_only` is true or false")
	}
	if on {
		if b.Config == nil {
			b.Config = map[string]any{}
		}
		b.Config["visibility"] = "dm_only"
	} else {
		delete(b.Config, "visibility")
	}
	return nil
}

func (k LayoutKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	if err := k.owner(ctx, campaignID, a); err != nil {
		return Plan{Error: planError(err)}
	}
	if r.Action == ActionDelete {
		return Plan{Error: "to remove a layout, use Customize › Categories, since pages wearing it change too"}
	}
	t, err := k.resolve(ctx, campaignID, r)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	l := t.loc
	if l.isNew {
		pl := withNamedLayout(l, entities.EntityTypeLayout{})
		if err := pl.Check(); err != nil {
			return Plan{Error: planError(err)}
		}
	}
	// A plan's text is still markdown; the rules are checked on a stand-in
	// that holds the same blocks, so a bad shape is caught at review.
	if err := k.Svc.Check(ctx, campaignID, l.et.ID, layoutFromGrid(stripGridMarkdown(t.next))); err != nil {
		return Plan{Error: planError(err)}
	}
	if !l.isNew && gridShape(t.next) == gridShape(l.was) && len(t.words) == 0 {
		return Plan{Error: l.where() + " already has these blocks in these places"}
	}
	var p Plan
	if l.isNew {
		p.Summary = "adds " + l.where() + ": " + gridSummary(t.next)
	} else {
		p.Summary = l.where() + " becomes: " + gridSummary(t.next) + " (now: " + gridSummary(l.was) + ")"
	}
	for _, w := range t.words {
		if w.was == "" {
			p.Summary += "; " + w.typ + " words: " + quoted(w.now)
		} else {
			p.Summary += "; " + w.typ + " words: " + quoted(w.now) + " (was " + quoted(w.was) + ")"
		}
	}
	if dropped := gridDropped(l.was, t.next); len(dropped) > 0 {
		p.Warnings = append(p.Warnings, "left out, so removed: "+strings.Join(dropped, ", "))
	}
	return p
}

func (k LayoutKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	if p := k.Plan(ctx, campaignID, a, r); p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	t, err := k.resolve(ctx, campaignID, r)
	if err != nil {
		return err
	}
	for ri := range t.next {
		for ci := range t.next[ri].Columns {
			blocks := t.next[ri].Columns[ci].Blocks
			for bi := range blocks {
				md, ok := blocks[bi].Config["_md"].(string)
				if !ok {
					continue
				}
				delete(blocks[bi].Config, "_md")
				if r.Text == nil {
					return fmt.Errorf("layout: a text block was applied without its Text")
				}
				html, err := r.Text.HTML(md)
				if err != nil {
					return err
				}
				content := ""
				if html != nil {
					content = *html
				}
				blocks[bi].Config["content"] = content
			}
		}
	}
	layout := layoutFromGrid(t.next)
	if t.loc.isMain() {
		return k.Svc.SaveMain(ctx, campaignID, t.loc.et.ID, layout)
	}
	return k.Svc.SaveNamed(ctx, campaignID, t.loc.et.ID, withNamedLayout(t.loc, layout))
}

// withNamedLayout is the category's named layouts with the one at loc set to
// layout, added when it is new.
func withNamedLayout(loc layoutLoc, layout entities.EntityTypeLayout) entities.PageLayouts {
	pl := loc.pl
	pl.Extra = append([]entities.PageLayout(nil), pl.Extra...)
	if loc.isNew {
		pl.Extra = append(pl.Extra, entities.PageLayout{Key: loc.key, Name: loc.label, Layout: layout})
		return pl
	}
	for i := range pl.Extra {
		if pl.Extra[i].Key == loc.key {
			pl.Extra[i].Layout = layout
		}
	}
	return pl
}

func gridFromLayout(l entities.EntityTypeLayout) []gridRow {
	var out []gridRow
	for _, row := range l.Rows {
		if row.Kind == tilegrid.KindTiles {
			out = append(out, gridRow{ID: row.ID, IsTiles: true, Tiles: row.Tiles})
			continue
		}
		g := gridRow{ID: row.ID}
		for _, c := range row.Columns {
			col := HomeColumn{ID: c.ID, Width: c.Width}
			for _, b := range c.Blocks {
				col.Blocks = append(col.Blocks, HomeBlock{ID: b.ID, Type: b.Type, Config: b.Config})
			}
			g.Columns = append(g.Columns, col)
		}
		out = append(out, g)
	}
	return out
}

func layoutFromGrid(rows []gridRow) entities.EntityTypeLayout {
	out := entities.EntityTypeLayout{Rows: []entities.TemplateRow{}}
	for _, row := range rows {
		if row.IsTiles {
			// The editor writes a tile section with an empty column list.
			out.Rows = append(out.Rows, entities.TemplateRow{ID: row.ID, Kind: tilegrid.KindTiles, Columns: []entities.TemplateColumn{}, Tiles: row.Tiles})
			continue
		}
		r := entities.TemplateRow{ID: row.ID}
		for _, c := range row.Columns {
			col := entities.TemplateColumn{ID: c.ID, Width: c.Width, Blocks: []entities.TemplateBlock{}}
			for _, b := range c.Blocks {
				col.Blocks = append(col.Blocks, entities.TemplateBlock{ID: b.ID, Type: b.Type, Config: b.Config})
			}
			r.Columns = append(r.Columns, col)
		}
		out.Rows = append(out.Rows, r)
	}
	return out
}

// stripGridMarkdown is the rows without the unrendered text, for the rules check.
func stripGridMarkdown(rows []gridRow) []gridRow {
	out := make([]gridRow, 0, len(rows))
	for _, row := range rows {
		nr := gridRow{ID: row.ID, IsTiles: row.IsTiles, Tiles: row.Tiles}
		for _, c := range row.Columns {
			nc := HomeColumn{ID: c.ID, Width: c.Width}
			for _, b := range c.Blocks {
				cfg := cloneConfig(b.Config)
				delete(cfg, "_md")
				nc.Blocks = append(nc.Blocks, HomeBlock{ID: b.ID, Type: b.Type, Config: cfg})
			}
			nr.Columns = append(nr.Columns, nc)
		}
		out = append(out, nr)
	}
	return out
}

// blockSpec writes one block in the form a record reads: its bare type, or
// the type with the words and the DM-only setting a record can change. A block
// with neither is written bare, which keeps whatever it holds.
func (k LayoutKind) blockSpec(bl HomeBlock) string {
	parts := []string{"type: " + bl.Type}
	if bl.Type == "text_block" {
		if content, _ := bl.Config["content"].(string); content != "" {
			parts = append(parts, "text: "+quote(writtenMarkdown(content)))
		}
	}
	if gridDirectorsOnly(bl) {
		parts = append(parts, "directors_only: true")
	}
	if len(parts) == 1 {
		return bl.Type
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// layoutRecord writes one layout as the `kind: layout` block that sends it
// back; the main layout is the one written without a `layout:` name.
func (k LayoutKind) layoutRecord(b *strings.Builder, category, layout string, rows []gridRow) {
	b.WriteString("---\nkind: layout\ncategory: " + quote(category) + "\n")
	if layout != "" {
		b.WriteString("layout: " + quote(layout) + "\n")
	}
	writeGrid(b, rows, k.blockSpec)
	b.WriteString("---\n\n")
}

// Export writes every category's layouts as they stand, for the owner only.
func (k LayoutKind) Export(ctx context.Context, campaignID string, a Actor) (string, error) {
	if k.owner(ctx, campaignID, a) != nil {
		return "", nil
	}
	types, err := k.Svc.Categories(ctx, campaignID)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for i := range types {
		et := &types[i]
		if rows := gridFromLayout(et.Layout); len(rows) > 0 {
			k.layoutRecord(&b, et.Name, "", rows)
		}
		pl, ok := k.Svc.Named(ctx, et)
		if !ok {
			continue
		}
		for _, l := range pl.Extra {
			if rows := gridFromLayout(l.Layout); len(rows) > 0 {
				k.layoutRecord(&b, et.Name, l.Name, rows)
			}
		}
	}
	return b.String(), nil
}

// layoutBefore is a layout as it was: the main layout, or all the category's
// named layouts, which is what a named layout is stored with.
type layoutBefore struct {
	Main  *entities.EntityTypeLayout `json:"main,omitempty"`
	Named *entities.PageLayouts      `json:"named,omitempty"`
}

var _ aiplug.Undoable = LayoutKind{}

// Before keeps the layout as it is now. A new named layout needs no layout of
// its own kept: the named layouts before it was added are put back, which
// removes it.
func (k LayoutKind) Before(ctx context.Context, campaignID string, _ Actor, r Record) (*aiplug.Snapshot, error) {
	if k.Svc == nil || r.Action == ActionDelete {
		return nil, nil
	}
	loc, err := k.locate(ctx, campaignID, r)
	if err != nil {
		return nil, nil
	}
	var b layoutBefore
	if loc.isMain() {
		b.Main = &loc.et.Layout
	} else {
		b.Named = &loc.pl
	}
	data, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	return &aiplug.Snapshot{Target: strconv.Itoa(loc.et.ID) + ":" + loc.key, Name: loc.where(), Data: string(data)}, nil
}

// categoryOf finds the category a snapshot's target names.
func (k LayoutKind) categoryOf(ctx context.Context, campaignID, target string) (*entities.EntityType, string, error) {
	idText, key, _ := strings.Cut(target, ":")
	id, err := strconv.Atoi(idText)
	if err != nil {
		return nil, "", badRequestf("this change can't be undone: its category is not known")
	}
	types, err := k.Svc.Categories(ctx, campaignID)
	if err != nil {
		return nil, "", err
	}
	for i := range types {
		if types[i].ID == id {
			return &types[i], key, nil
		}
	}
	return nil, "", badRequestf("this change can't be undone: the category is gone")
}

// Fingerprint is the layout now: the main layout, or the named layouts.
func (k LayoutKind) Fingerprint(ctx context.Context, campaignID string, _ Actor, s aiplug.Snapshot) (string, error) {
	et, key, err := k.categoryOf(ctx, campaignID, s.Target)
	if err != nil {
		return "", err
	}
	if key == entities.MainLayoutKey {
		return aiplug.Hash(layoutBefore{Main: &et.Layout}), nil
	}
	pl, _ := k.Svc.Named(ctx, et)
	return aiplug.Hash(layoutBefore{Named: &pl}), nil
}

// Restore saves the kept layout again.
func (k LayoutKind) Restore(ctx context.Context, campaignID string, _ Actor, s aiplug.Snapshot) error {
	var b layoutBefore
	if err := json.Unmarshal([]byte(s.Data), &b); err != nil {
		return err
	}
	et, _, err := k.categoryOf(ctx, campaignID, s.Target)
	if err != nil {
		return err
	}
	if b.Main != nil {
		return k.Svc.SaveMain(ctx, campaignID, et.ID, *b.Main)
	}
	if b.Named != nil {
		return k.Svc.SaveNamed(ctx, campaignID, et.ID, *b.Named)
	}
	return nil
}
