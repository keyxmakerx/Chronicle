package records

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/tilegrid"
)

// CategoryHomesAPI is each category's own landing page, as the owner arranges
// it in Customize › Categories. A category with none shows Chronicle's
// built-in landing page.
type CategoryHomesAPI interface {
	// IsOwner reports whether userID owns the campaign; the Customizer's
	// categories, and so their homes, are the owner's.
	IsOwner(ctx context.Context, campaignID, userID string) (bool, error)
	// Categories are the campaign's categories with their saved homes.
	Categories(ctx context.Context, campaignID string) ([]entities.EntityType, error)
	// BlockTypes are the blocks a category home can place in this campaign.
	BlockTypes(ctx context.Context, campaignID string) []LayoutBlockInfo
	// Check runs the rules a saved category home must meet, without saving.
	Check(ctx context.Context, campaignID string, typeID int, l campaigns.DashboardLayout) error
	// Save makes l the category's own home.
	Save(ctx context.Context, campaignID string, typeID int, l campaigns.DashboardLayout) error
	// Reset takes the category's own home away, so it shows the built-in one.
	Reset(ctx context.Context, campaignID string, typeID int) error
}

// CategoryHomeKind reads and rearranges the landing page of a category: which
// blocks it holds, in which rows and columns, and the words in its Text
// blocks. It is the home page's arrangement (rows of columns of blocks) for
// one category, so a block already on it keeps its settings when the record
// lists it again, and a tile section stays as the Customizer arranged it.
type CategoryHomeKind struct{ Svc CategoryHomesAPI }

func (CategoryHomeKind) Name() string  { return "category-home" }
func (CategoryHomeKind) Label() string { return "Category home" }
func (CategoryHomeKind) Brief() string {
	return "the landing page of a category: which blocks it holds and where, and the words in its Text blocks; `delete` brings back the built-in one (owner only)"
}
func (CategoryHomeKind) Doc() string {
	return "The landing page of one category (the page its sidebar entry opens, with its list of pages), matched by `category` (its name). Key: `rows`, a list of rows, each a list of `columns`; a column has a `width` (1 to 12) and `blocks`, in the order shown. A block is its type written alone, or `{type: text_block, text: \"words\"}` to write a Text block's words in markdown. A block already on the home keeps its settings when listed again, so list every block to keep; one left out is removed. A row `- tiles: kept` keeps a tile section the home already has, unchanged and in order: tiles are arranged only in Customize, and a tile section left out is removed. `create` and `update` replace the whole arrangement and give the category a landing page of its own; `delete` takes it away so the category shows Chronicle's built-in landing page again. Only the campaign's owner can change a category home. `what: layouts` with `part: category-homes` lists every category's home as it is now, with the blocks it can place, written so it can be sent back changed.\n\n```\n---\nkind: category-home\ncategory: Locations\nrows:\n  - columns:\n      - width: 12\n        blocks: [category_header, search_bar]\n  - columns:\n      - width: 8\n        blocks: [entity_grid]\n      - width: 4\n        blocks:\n          - pinned_pages\n          - {type: text_block, text: \"Start with the **capital**.\"}\n---\n```"
}

func (k CategoryHomeKind) Title(r Record) string { return r.Str("category") + " · home" }

func (k CategoryHomeKind) owner(ctx context.Context, campaignID string, a Actor) error {
	if k.Svc == nil {
		return badRequestf("category homes can't be changed on this server")
	}
	if a.CanAuthorDmOnly() {
		if ok, err := k.Svc.IsOwner(ctx, campaignID, a.UserID); err != nil {
			return err
		} else if ok {
			return nil
		}
	}
	return badRequestf("only the campaign's owner can change category homes")
}

// categoryHomeRows is a category's own saved home as rows; saved is false
// while it shows the built-in one.
func categoryHomeRows(et *entities.EntityType) (rows []gridRow, saved bool) {
	l := et.ParseCategoryDashboardLayout()
	if l == nil {
		return nil, false
	}
	return gridFromDashboard(*l), true
}

// categoryHomeTarget is a record resolved against the home it replaces.
type categoryHomeTarget struct {
	et    *entities.EntityType
	was   []gridRow
	saved bool
	next  []gridRow
	words []homeWordsChange
}

func (k CategoryHomeKind) locate(ctx context.Context, campaignID string, r Record) (categoryHomeTarget, error) {
	var t categoryHomeTarget
	name := strings.TrimSpace(r.Str("category"))
	if name == "" {
		return t, badRequestf("name the category in `category:`")
	}
	types, err := k.Svc.Categories(ctx, campaignID)
	if err != nil {
		return t, err
	}
	if t.et = typeNamed(types, name); t.et == nil {
		return t, badRequestf("there is no category called %s", quote(name))
	}
	t.was, t.saved = categoryHomeRows(t.et)
	return t, nil
}

func (k CategoryHomeKind) resolve(ctx context.Context, campaignID string, r Record) (categoryHomeTarget, error) {
	t, err := k.locate(ctx, campaignID, r)
	if err != nil || r.Action == ActionDelete {
		return t, err
	}
	items := r.List("rows")
	if len(items) == 0 {
		return t, badRequestf("list the home's `rows:`, each with its `columns`")
	}
	palette := map[string]bool{}
	for _, b := range k.Svc.BlockTypes(ctx, campaignID) {
		palette[b.Type] = true
	}
	// A block the editor no longer offers can still be kept where it is.
	onHome := map[string]bool{}
	for _, row := range t.was {
		for _, c := range row.Columns {
			for _, b := range c.Blocks {
				onHome[b.Type] = true
			}
		}
	}
	rules := gridRules{
		valid:    func(typ string) bool { return palette[typ] || onHome[typ] },
		notBlock: "a block a category home can hold",
		words:    layoutWords,
		tileRow:  keepTiles(t.was),
	}
	t.next, t.words, err = parseGrid(items, t.was, rules)
	return t, err
}

func (k CategoryHomeKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	if err := k.owner(ctx, campaignID, a); err != nil {
		return Plan{Error: planError(err)}
	}
	t, err := k.resolve(ctx, campaignID, r)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	where := "the " + t.et.NamePlural + " home"
	if r.Action == ActionDelete {
		if !t.saved {
			return Plan{Error: where + " has no arrangement of its own to remove"}
		}
		return Plan{Summary: where + " goes back to Chronicle's built-in landing page (now: " + gridSummary(t.was) + ")"}
	}
	// A plan's text is still markdown; the rules are checked on a stand-in
	// that holds the same blocks, so a bad shape is caught at review.
	if err := k.Svc.Check(ctx, campaignID, t.et.ID, dashboardFromGrid(stripGridMarkdown(t.next))); err != nil {
		return Plan{Error: planError(err)}
	}
	if t.saved && gridShape(t.next) == gridShape(t.was) && len(t.words) == 0 {
		return Plan{Error: where + " already has these blocks in these places"}
	}
	var p Plan
	if t.saved {
		p.Summary = where + " becomes: " + gridSummary(t.next) + " (now: " + gridSummary(t.was) + ")"
	} else {
		p.Summary = where + " gets its own arrangement: " + gridSummary(t.next) + " (now: the built-in landing page)"
	}
	p.Summary += gridWordsSummary(t.words)
	if dropped := gridDropped(t.was, t.next); len(dropped) > 0 {
		p.Warnings = append(p.Warnings, "left out, so removed: "+strings.Join(dropped, ", "))
	}
	return p
}

func (k CategoryHomeKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	if p := k.Plan(ctx, campaignID, a, r); p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	t, err := k.resolve(ctx, campaignID, r)
	if err != nil {
		return err
	}
	if r.Action == ActionDelete {
		return k.Svc.Reset(ctx, campaignID, t.et.ID)
	}
	if err := renderGridWords(t.next, r.Text, "category home"); err != nil {
		return err
	}
	return k.Svc.Save(ctx, campaignID, t.et.ID, dashboardFromGrid(t.next))
}

// blockSpec writes one block in the form a record reads: its bare type, or
// the type with its words. A block with no words is written bare, which keeps
// whatever it holds.
func (k CategoryHomeKind) blockSpec(bl HomeBlock) string {
	if bl.Type == "text_block" {
		if content, _ := bl.Config["content"].(string); content != "" {
			return "{type: text_block, text: " + quote(writtenMarkdown(content)) + "}"
		}
	}
	return bl.Type
}

// homeRecord writes one category's home as the `kind: category-home` block
// that sends it back.
func (k CategoryHomeKind) homeRecord(b *strings.Builder, category string, rows []gridRow) {
	b.WriteString("---\nkind: category-home\ncategory: " + quote(category) + "\n")
	writeGrid(b, rows, k.blockSpec)
	b.WriteString("---\n\n")
}

// Export writes every category's own home as it stands, for the owner only.
func (k CategoryHomeKind) Export(ctx context.Context, campaignID string, a Actor) (string, error) {
	if k.owner(ctx, campaignID, a) != nil {
		return "", nil
	}
	types, err := k.Svc.Categories(ctx, campaignID)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for i := range types {
		if rows, saved := categoryHomeRows(&types[i]); saved && len(rows) > 0 {
			k.homeRecord(&b, types[i].Name, rows)
		}
	}
	return b.String(), nil
}

// categoryHomeBefore is a home as it was: its stored layout, or none while
// the category showed the built-in landing page.
type categoryHomeBefore struct {
	Layout *campaigns.DashboardLayout `json:"layout,omitempty"`
}

var _ aiplug.Undoable = CategoryHomeKind{}

// Before keeps the home as it is now, whatever the record does to it.
func (k CategoryHomeKind) Before(ctx context.Context, campaignID string, _ Actor, r Record) (*aiplug.Snapshot, error) {
	if k.Svc == nil {
		return nil, nil
	}
	t, err := k.locate(ctx, campaignID, r)
	if err != nil {
		return nil, nil
	}
	data, err := json.Marshal(categoryHomeBefore{Layout: t.et.ParseCategoryDashboardLayout()})
	if err != nil {
		return nil, err
	}
	return &aiplug.Snapshot{Target: strconv.Itoa(t.et.ID), Name: "the " + t.et.NamePlural + " home", Data: string(data)}, nil
}

func (k CategoryHomeKind) category(ctx context.Context, campaignID, target string) (*entities.EntityType, error) {
	id, err := strconv.Atoi(target)
	if err != nil {
		return nil, badRequestf("this change can't be undone: its category is not known")
	}
	types, err := k.Svc.Categories(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	for i := range types {
		if types[i].ID == id {
			return &types[i], nil
		}
	}
	return nil, badRequestf("this change can't be undone: the category is gone")
}

// Fingerprint is the home now.
func (k CategoryHomeKind) Fingerprint(ctx context.Context, campaignID string, _ Actor, s aiplug.Snapshot) (string, error) {
	et, err := k.category(ctx, campaignID, s.Target)
	if err != nil {
		return "", err
	}
	return aiplug.Hash(categoryHomeBefore{Layout: et.ParseCategoryDashboardLayout()}), nil
}

// Restore saves the kept home again, or takes the home away when the category
// had none.
func (k CategoryHomeKind) Restore(ctx context.Context, campaignID string, _ Actor, s aiplug.Snapshot) error {
	var b categoryHomeBefore
	if err := json.Unmarshal([]byte(s.Data), &b); err != nil {
		return err
	}
	et, err := k.category(ctx, campaignID, s.Target)
	if err != nil {
		return err
	}
	if b.Layout == nil {
		return k.Svc.Reset(ctx, campaignID, et.ID)
	}
	return k.Svc.Save(ctx, campaignID, et.ID, *b.Layout)
}

func gridFromDashboard(l campaigns.DashboardLayout) []gridRow {
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

func dashboardFromGrid(rows []gridRow) campaigns.DashboardLayout {
	out := campaigns.DashboardLayout{Rows: []campaigns.DashboardRow{}}
	for _, row := range rows {
		if row.IsTiles {
			// The editor writes a tile section with an empty column list.
			out.Rows = append(out.Rows, campaigns.DashboardRow{ID: row.ID, Kind: tilegrid.KindTiles, Columns: []campaigns.DashboardColumn{}, Tiles: row.Tiles})
			continue
		}
		r := campaigns.DashboardRow{ID: row.ID}
		for _, c := range row.Columns {
			col := campaigns.DashboardColumn{ID: c.ID, Width: c.Width, Blocks: []campaigns.DashboardBlock{}}
			for _, b := range c.Blocks {
				col.Blocks = append(col.Blocks, campaigns.DashboardBlock{ID: b.ID, Type: b.Type, Config: b.Config})
			}
			r.Columns = append(r.Columns, col)
		}
		out.Rows = append(out.Rows, r)
	}
	return out
}
