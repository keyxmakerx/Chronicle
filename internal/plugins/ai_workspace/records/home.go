package records

import (
	"context"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// HomeAPI is a campaign's home pages, one arrangement per audience, as the
// owner arranges them in Customize › Home pages.
type HomeAPI interface {
	// IsOwner reports whether userID owns the campaign; only the owner
	// arranges the homes.
	IsOwner(ctx context.Context, campaignID, userID string) (bool, error)
	// Home is what the audience sees; saved is false when it borrows another
	// home (the Everyone home or the built-in one).
	Home(ctx context.Context, campaignID, audience string) (layout HomeLayout, saved bool, err error)
	// Check runs the rules a saved home must meet, without saving.
	Check(ctx context.Context, campaignID string, layout HomeLayout) error
	// Save makes layout the audience's own home.
	Save(ctx context.Context, campaignID, audience string, layout HomeLayout) error
	// Reset takes the audience's own home away, so it borrows again.
	Reset(ctx context.Context, campaignID, audience string) error
	// BlockTypes are the widgets a home can hold.
	BlockTypes() []string
}

// HomeLayout is a home as rows of columns of blocks. Tiles is set when the
// home holds a tile section, which only the Customizer arranges.
type HomeLayout struct {
	Rows  []HomeRow
	Tiles bool
}

// HomeRow is one row of columns.
type HomeRow struct {
	ID      string
	Columns []HomeColumn
}

// HomeColumn is a column of Width twelfths.
type HomeColumn struct {
	ID     string
	Width  int
	Blocks []HomeBlock
}

// HomeBlock is one widget; Config keeps what the owner set on it. A Text
// Block's words are Config["content"], as sanitized HTML.
type HomeBlock struct {
	ID     string
	Type   string
	Config map[string]any
}

// homeAudiences are the audience words a record may use, in the order they are
// listed, each with the key the campaign stores it under.
var homeAudiences = []struct{ word, key string }{
	{"everyone", "default"}, {"players", "player"}, {"scribes", "scribe"}, {"directors", "director"}, {"guests", "guest"},
}

// maxHomeText bounds one Text Block's words, as the page editor does for a
// short note.
const maxHomeText = 4000

// HomeKind reads, rearranges and rewrites the text of a campaign's home
// pages: which blocks each audience's home holds, in which rows and columns.
// A block already on the home keeps its settings when the record lists it
// again, so rearranging never loses a choice the owner made.
type HomeKind struct {
	Svc HomeAPI
}

func (HomeKind) Name() string  { return "home" }
func (HomeKind) Label() string { return "Home page" }
func (HomeKind) Brief() string {
	return "the home page each audience sees: which blocks it holds and where, and the words in its Text Blocks (owner only)"
}
func (k HomeKind) Doc() string {
	types := "the Customizer's blocks"
	if k.Svc != nil {
		types = "`" + strings.Join(k.Svc.BlockTypes(), "`, `") + "`"
	}
	return "The home page one audience sees, matched by `audience`: `everyone`, `players`, `scribes`, `directors` or `guests`. Key: `rows`, a list of rows, each a list of `columns`; a column has a `width` (1 to 12, a row's widths add to 12 at most) and `blocks`, in the order shown. A block is its type written alone, or `{type: text_block, text: \"words\"}` to write a Text Block's words in markdown. Block types: " + types + ". A block already on the home keeps its settings when listed again, so list every block the home should keep; one left out is removed. `create` and `update` replace the whole arrangement; `delete` takes the audience's own home away so it shows the Everyone home again. Only the campaign's owner can change a home. Some blocks show only what that audience may see, or only to the DM team, and the review says when a block is placed where it can't draw. A home with a tile section can only be arranged in Customize.\n\n```\n---\nkind: home\naudience: players\nrows:\n  - columns:\n      - width: 12\n        blocks: [welcome_banner]\n  - columns:\n      - width: 7\n        blocks:\n          - start_here\n          - {type: text_block, text: \"Bring dice and snacks.\"}\n          - recent_pages\n      - width: 5\n        blocks: [coming_up, party]\n---\n```"
}

// homeBlocks that draw nothing for anyone but the DM team.
var dmOnlyHomeBlocks = map[string]bool{"activity_feed": true, "quick_actions": true, "sync_status": true}

// homeTarget is a record resolved against the home it replaces.
type homeTarget struct {
	key, word string
	was       HomeLayout
	saved     bool
	next      HomeLayout
}

func audienceKey(word string) (key, canonical string, ok bool) {
	w := strings.ToLower(strings.TrimSpace(word))
	for _, a := range homeAudiences {
		if w == a.word || w == a.key {
			return a.key, a.word, true
		}
	}
	return "", "", false
}

func (k HomeKind) resolve(ctx context.Context, campaignID string, r Record) (homeTarget, error) {
	var t homeTarget
	word := r.Str("audience")
	if word == "" {
		return t, badRequestf("name the audience in `audience:` (everyone, players, scribes, directors or guests)")
	}
	var ok bool
	if t.key, t.word, ok = audienceKey(word); !ok {
		return t, badRequestf("%s is not an audience; use everyone, players, scribes, directors or guests", quote(word))
	}
	var err error
	if t.was, t.saved, err = k.Svc.Home(ctx, campaignID, t.key); err != nil {
		return t, err
	}
	if r.Action == ActionDelete {
		return t, nil
	}
	if t.was.Tiles {
		return t, badRequestf("this home has a tile section, which can only be arranged in Customize › Home pages")
	}
	rows := r.List("rows")
	if len(rows) == 0 {
		return t, badRequestf("list the home's `rows:`, each with its `columns`")
	}
	valid := map[string]bool{}
	for _, bt := range k.Svc.BlockTypes() {
		valid[bt] = true
	}
	// Blocks already on the home, by type in the order found, so a listed
	// block takes over the settings of the one it stands for.
	pool := map[string][]HomeBlock{}
	for _, row := range t.was.Rows {
		for _, col := range row.Columns {
			for _, b := range col.Blocks {
				pool[b.Type] = append(pool[b.Type], b)
			}
		}
	}
	used := map[string]int{}
	for ri, rv := range rows {
		rm, ok := rv.(map[string]any)
		if !ok {
			return t, badRequestf("row %d: write it as `columns:` and a list of columns", ri+1)
		}
		cols, _ := rm["columns"].([]any)
		if len(cols) == 0 {
			return t, badRequestf("row %d: list its `columns`", ri+1)
		}
		row := HomeRow{ID: fmt.Sprintf("row-ai-%d", ri+1)}
		for ci, cv := range cols {
			cm, ok := cv.(map[string]any)
			if !ok {
				return t, badRequestf("row %d, column %d: write it as `width:` and `blocks:`", ri+1, ci+1)
			}
			w, err := wholeNumber(cm["width"])
			if err != nil || w < 1 || w > 12 {
				return t, badRequestf("row %d, column %d: `width` is a whole number from 1 to 12", ri+1, ci+1)
			}
			col := HomeColumn{ID: fmt.Sprintf("col-ai-%d-%d", ri+1, ci+1), Width: w}
			blocks, _ := cm["blocks"].([]any)
			for bi, bv := range blocks {
				typ, text, hasText, err := homeBlockSpec(bv)
				if err != nil {
					return t, badRequestf("row %d, column %d, block %d: %v", ri+1, ci+1, bi+1, err)
				}
				if !valid[typ] {
					return t, badRequestf("row %d, column %d: %s is not a home block", ri+1, ci+1, quote(typ))
				}
				b := HomeBlock{ID: fmt.Sprintf("blk-ai-%d-%d-%d", ri+1, ci+1, bi+1), Type: typ}
				if old := pool[typ]; used[typ] < len(old) {
					b.ID, b.Config = old[used[typ]].ID, cloneConfig(old[used[typ]].Config)
				}
				used[typ]++
				if hasText {
					if typ != "text_block" {
						return t, badRequestf("only a text_block takes `text`; %s does not", quote(typ))
					}
					if len([]rune(text)) > maxHomeText {
						return t, badRequestf("a text block's words run over %d characters", maxHomeText)
					}
					if b.Config == nil {
						b.Config = map[string]any{}
					}
					b.Config["_md"] = text // turned into HTML at Apply
				}
				col.Blocks = append(col.Blocks, b)
			}
			row.Columns = append(row.Columns, col)
		}
		t.next.Rows = append(t.next.Rows, row)
	}
	return t, nil
}

// homeBlockSpec reads a block written as its type or as {type, text}.
func homeBlockSpec(v any) (typ, text string, hasText bool, err error) {
	switch b := v.(type) {
	case string:
		return strings.TrimSpace(b), "", false, nil
	case map[string]any:
		typ, _ = b["type"].(string)
		if strings.TrimSpace(typ) == "" {
			return "", "", false, fmt.Errorf("a block with `text` needs a `type`")
		}
		if tv, ok := b["text"]; ok {
			return strings.TrimSpace(typ), strings.TrimSpace(fmt.Sprint(tv)), true, nil
		}
		return strings.TrimSpace(typ), "", false, nil
	}
	return "", "", false, fmt.Errorf("write a block as its type, or `{type: text_block, text: \"…\"}`")
}

func wholeNumber(v any) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case float64:
		if n == float64(int(n)) {
			return int(n), nil
		}
	}
	return 0, fmt.Errorf("not a whole number")
}

func cloneConfig(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (k HomeKind) isOwner(ctx context.Context, campaignID string, a Actor) (bool, error) {
	if !a.CanAuthorDmOnly() {
		return false, nil
	}
	return k.Svc.IsOwner(ctx, campaignID, a.UserID)
}

func (k HomeKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	if owner, err := k.isOwner(ctx, campaignID, a); err != nil {
		return Plan{Error: planError(err)}
	} else if !owner {
		return Plan{Error: "only the campaign's owner can change the home pages"}
	}
	t, err := k.resolve(ctx, campaignID, r)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	if r.Action == ActionDelete {
		if !t.saved {
			return Plan{Error: "the " + t.word + " home has no arrangement of its own to remove"}
		}
		return Plan{Summary: "the " + t.word + " home goes back to the Everyone home (now: " + homeSummary(t.was) + ")"}
	}
	// A plan's text is still markdown; the rules are checked on a stand-in
	// that holds the same blocks, so a bad shape is caught at review.
	probe := t.next
	if err := k.Svc.Check(ctx, campaignID, stripMarkdown(probe)); err != nil {
		return Plan{Error: planError(err)}
	}
	if homeShape(t.next) == homeShape(t.was) && !homeHasText(t.next) {
		return Plan{Error: "the " + t.word + " home already has these blocks in these places"}
	}
	p := Plan{Summary: "the " + t.word + " home becomes: " + homeSummary(t.next) + " (now: " + homeSummary(t.was) + ")"}
	if dropped := droppedBlocks(t.was, t.next); len(dropped) > 0 {
		p.Warnings = append(p.Warnings, "left out, so removed: "+strings.Join(dropped, ", "))
	}
	if t.key != "director" {
		var dm []string
		for _, row := range t.next.Rows {
			for _, c := range row.Columns {
				for _, b := range c.Blocks {
					if dmOnlyHomeBlocks[b.Type] {
						dm = append(dm, b.Type)
					}
				}
			}
		}
		if len(dm) > 0 {
			p.Warnings = append(p.Warnings, strings.Join(dm, ", ")+" show only to the DM team, so the "+t.word+" home will show nothing there")
		}
	}
	return p
}

func (k HomeKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	if p := k.Plan(ctx, campaignID, a, r); p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	t, err := k.resolve(ctx, campaignID, r)
	if err != nil {
		return err
	}
	if r.Action == ActionDelete {
		return k.Svc.Reset(ctx, campaignID, t.key)
	}
	for ri := range t.next.Rows {
		for ci := range t.next.Rows[ri].Columns {
			blocks := t.next.Rows[ri].Columns[ci].Blocks
			for bi := range blocks {
				md, ok := blocks[bi].Config["_md"].(string)
				if !ok {
					continue
				}
				delete(blocks[bi].Config, "_md")
				if r.Text == nil {
					return fmt.Errorf("home: a text block was applied without its Text")
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
	return k.Svc.Save(ctx, campaignID, t.key, t.next)
}

// stripMarkdown is the layout without the unrendered text, for the rules check.
func stripMarkdown(l HomeLayout) HomeLayout {
	out := HomeLayout{}
	for _, row := range l.Rows {
		nr := HomeRow{ID: row.ID}
		for _, c := range row.Columns {
			nc := HomeColumn{ID: c.ID, Width: c.Width}
			for _, b := range c.Blocks {
				cfg := cloneConfig(b.Config)
				delete(cfg, "_md")
				nc.Blocks = append(nc.Blocks, HomeBlock{ID: b.ID, Type: b.Type, Config: cfg})
			}
			nr.Columns = append(nr.Columns, nc)
		}
		out.Rows = append(out.Rows, nr)
	}
	return out
}

func homeHasText(l HomeLayout) bool {
	for _, row := range l.Rows {
		for _, c := range row.Columns {
			for _, b := range c.Blocks {
				if _, ok := b.Config["_md"]; ok {
					return true
				}
			}
		}
	}
	return false
}

// homeShape is every column's width and blocks, to tell a change from none.
func homeShape(l HomeLayout) string {
	var b strings.Builder
	for _, row := range l.Rows {
		b.WriteString("/")
		for _, c := range row.Columns {
			fmt.Fprintf(&b, "|%d:", c.Width)
			for _, bl := range c.Blocks {
				b.WriteString(bl.Type + ",")
			}
		}
	}
	return b.String()
}

// homeSummary is a home on one line: rows split by " / ", columns by " | ".
func homeSummary(l HomeLayout) string {
	var rows []string
	for _, row := range l.Rows {
		var cols []string
		for _, c := range row.Columns {
			var types []string
			for _, b := range c.Blocks {
				types = append(types, b.Type)
			}
			cols = append(cols, strings.Join(types, ", "))
		}
		rows = append(rows, strings.Join(cols, " | "))
	}
	return strings.Join(rows, " / ")
}

// droppedBlocks lists the block types the old home held more of than the new.
func droppedBlocks(was, next HomeLayout) []string {
	count := func(l HomeLayout) map[string]int {
		m := map[string]int{}
		for _, row := range l.Rows {
			for _, c := range row.Columns {
				for _, b := range c.Blocks {
					m[b.Type]++
				}
			}
		}
		return m
	}
	w, n := count(was), count(next)
	var out []string
	for _, row := range was.Rows {
		for _, c := range row.Columns {
			for _, b := range c.Blocks {
				if w[b.Type] > n[b.Type] {
					out = append(out, b.Type)
					w[b.Type] = n[b.Type]
				}
			}
		}
	}
	return out
}

// Export writes each audience's own saved home as it stands, for the owner
// only. A home with a tile section is left out: it can't be written back.
func (k HomeKind) Export(ctx context.Context, campaignID string, a Actor) (string, error) {
	if owner, err := k.isOwner(ctx, campaignID, a); err != nil || !owner {
		return "", err
	}
	var b strings.Builder
	for _, aud := range homeAudiences {
		l, saved, err := k.Svc.Home(ctx, campaignID, aud.key)
		if err != nil {
			return "", err
		}
		if !saved || l.Tiles || len(l.Rows) == 0 {
			continue
		}
		b.WriteString("---\nkind: home\naudience: " + aud.word + "\nrows:\n")
		for _, row := range l.Rows {
			b.WriteString("  - columns:\n")
			for _, c := range row.Columns {
				fmt.Fprintf(&b, "      - width: %d\n        blocks:\n", c.Width)
				for _, bl := range c.Blocks {
					if bl.Type == "text_block" {
						if content, _ := bl.Config["content"].(string); content != "" {
							b.WriteString("          - {type: text_block, text: " + quote(textOf(content)) + "}\n")
							continue
						}
					}
					b.WriteString("          - " + bl.Type + "\n")
				}
			}
		}
		b.WriteString("---\n\n")
	}
	return b.String(), nil
}

// textOf is a Text Block's HTML as plain words, tags dropped, for an export an
// AI can rewrite.
func textOf(html string) string {
	var out strings.Builder
	in := false
	for _, r := range html {
		switch {
		case r == '<':
			in = true
			out.WriteByte(' ')
		case r == '>':
			in = false
		case !in:
			out.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(out.String()), " ")
}
