package records

import (
	"context"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
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
// home holds a tile section, which only the Customizer arranges; TileBlocks
// names the widgets in it, so a lookup can say what the section holds without
// being able to write it back.
type HomeLayout struct {
	Rows       []HomeRow
	Tiles      bool
	TileBlocks []string
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
	// Pages names the page a Brochure block points at, both ways. Nil leaves
	// a brochure block without a page to point at.
	Pages HomePages
}

// HomePages finds a campaign page by name (to write a brochure's page) and by
// id (to export it back as a name). Both are campaign-scoped by the caller.
type HomePages interface {
	GetBySlug(ctx context.Context, campaignID, slug string) (*entities.Entity, error)
	GetByID(ctx context.Context, id string) (*entities.Entity, error)
}

func (HomeKind) Name() string  { return "home" }
func (HomeKind) Label() string { return "Home page" }
func (HomeKind) Brief() string {
	return "the home page each audience sees: which blocks it holds and where, and the words in its Text, Table agreement and Making a hero blocks and its Where we play address (owner only)"
}
func (k HomeKind) Doc() string {
	types := "the Customizer's blocks"
	if k.Svc != nil {
		types = "`" + strings.Join(k.Svc.BlockTypes(), "`, `") + "`"
	}
	return "The home page one audience sees, matched by `audience`: `everyone`, `players`, `scribes`, `directors` or `guests`. Key: `rows`, a list of rows, each a list of `columns`; a column has a `width` (1 to 12, a row's widths add to 12 at most) and `blocks`, in the order shown. A block is its type written alone, `{type: text_block, text: \"words\"}` to write a Text Block's words in markdown, `{type: table_agreement, text: \"words\"}` or `{type: making_a_hero, text: \"words\"}` to write those blocks' words the same way, `{type: where_we_play, foundry_url: \"foundry.example.com\", voice: \"Discord, the Ember Coast channel\"}` to set the Foundry address and voice channel (each optional; an empty one clears it), or `{type: brochure, page: \"Page name\", text: \"words on the box\"}` to point a Brochure block at the campaign page that is the new-player brochure (the page must exist; `text` is optional, and without it the box shows the page's first line). A brochure written without `page` or `text` keeps the ones it has, and so does any block written without the words it takes. Block types: " + types + ". A block already on the home keeps its settings when listed again, so list every block the home should keep; one left out is removed. `create` and `update` replace the whole arrangement; `delete` takes the audience's own home away so it shows the Everyone home again. Only the campaign's owner can change a home. Some blocks show only what that audience may see, or only to the DM team, and the review says when a block is placed where it can't draw. A home with a tile section can only be arranged in Customize. `what: home` lists every audience's home as it is now, including the ones that borrow the Everyone home, written so it can be sent back changed.\n\n```\n---\nkind: home\naudience: players\nrows:\n  - columns:\n      - width: 12\n        blocks: [welcome_banner]\n  - columns:\n      - width: 7\n        blocks:\n          - start_here\n          - {type: text_block, text: \"Bring dice and snacks.\"}\n          - {type: brochure, page: \"New player brochure\", text: \"Read this before your first night.\"}\n          - recent_pages\n      - width: 5\n        blocks: [coming_up, party]\n---\n```"
}

// homeBlocks that draw nothing for anyone but the DM team.
var dmOnlyHomeBlocks = map[string]bool{"activity_feed": true, "quick_actions": true, "sync_status": true}

// homeTarget is a record resolved against the home it replaces.
type homeTarget struct {
	key, word string
	was       HomeLayout
	saved     bool
	next      HomeLayout
	// words are the written blocks whose words really change, for the review.
	words []homeWordsChange
}

// homeWordsChange is one written block's words before and after, as markdown.
type homeWordsChange struct{ typ, was, now string }

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
	pool := map[string][]*poolBlock{}
	for _, row := range t.was.Rows {
		for _, col := range row.Columns {
			for _, b := range col.Blocks {
				pool[b.Type] = append(pool[b.Type], &poolBlock{block: b})
			}
		}
	}
	claimWordMatches(rows, pool)
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
				spec, err := homeBlockSpec(bv)
				typ := spec.typ
				if err != nil {
					return t, badRequestf("row %d, column %d, block %d: %v", ri+1, ci+1, bi+1, err)
				}
				if !valid[typ] {
					return t, badRequestf("row %d, column %d: %s is not a home block", ri+1, ci+1, quote(typ))
				}
				b := HomeBlock{ID: fmt.Sprintf("blk-ai-%d-%d-%d", ri+1, ci+1, bi+1), Type: typ}
				if old := takePoolBlock(pool[typ], blockKey(ri, ci, bi)); old != nil {
					b.ID, b.Config = old.ID, cloneConfig(old.Config)
				}
				if spec.hasText && !homeWordsBlocks[typ] && typ != "brochure" {
					return t, badRequestf("only a text_block, a brochure, a table_agreement or a making_a_hero takes `text`; %s does not", quote(typ))
				}
				if spec.hasPage && typ != "brochure" {
					return t, badRequestf("only a brochure takes `page`; %s does not", quote(typ))
				}
				if (spec.hasFoundryURL || spec.hasVoice) && typ != "where_we_play" {
					return t, badRequestf("only a where_we_play takes `foundry_url` and `voice`; %s does not", quote(typ))
				}
				switch {
				case homeWordsBlocks[typ]:
					if spec.hasText {
						if len([]rune(spec.text)) > maxHomeText {
							return t, badRequestf("the words in a %s block run over %d characters", typ, maxHomeText)
						}
						// Words sent back as they were read leave the stored
						// block alone, so a lookup sent back never re-renders
						// (and so never reshapes) what the owner wrote.
						was := storedWords(b.Config)
						if now := normalizeWords(spec.text); now != was {
							if b.Config == nil {
								b.Config = map[string]any{}
							}
							b.Config["_md"] = spec.text // turned into HTML at Apply
							t.words = append(t.words, homeWordsChange{typ: typ, was: was, now: now})
						}
					}
				case typ == "brochure":
					if err := k.setBrochure(ctx, campaignID, &b, spec); err != nil {
						return t, badRequestf("row %d, column %d: %v", ri+1, ci+1, err)
					}
				case typ == "where_we_play":
					if err := setWhereWePlay(&b, spec); err != nil {
						return t, badRequestf("row %d, column %d: %v", ri+1, ci+1, err)
					}
				}
				col.Blocks = append(col.Blocks, b)
			}
			row.Columns = append(row.Columns, col)
		}
		t.next.Rows = append(t.next.Rows, row)
	}
	return t, nil
}

// homeSpec is a block as written: its type and the words or page it carries.
type homeSpec struct {
	typ              string
	text             string
	hasText, hasPage bool
	page             string
	// foundryURL and voice are a where_we_play block's two typed lines; has*
	// tells "left out, keep it" from "empty, clear it".
	foundryURL, voice       string
	hasFoundryURL, hasVoice bool
}

// poolBlock is a block already on the home, waiting to be taken over by the
// listed block that stands for it.
type poolBlock struct {
	block HomeBlock
	// claim is the listed block that wrote this block's own words back, and
	// used says it has been taken.
	claim string
	used  bool
}

func blockKey(ri, ci, bi int) string { return fmt.Sprintf("%d.%d.%d", ri, ci, bi) }

// claimWordMatches pairs each listed written block that carries words with the
// block on the home holding exactly those words, wherever it now sits. Pairing
// by position alone would hand a moved column's words to another block of the
// same type, so a lookup sent back with a column moved would rewrite them.
func claimWordMatches(rows []any, pool map[string][]*poolBlock) {
	for ri, rv := range rows {
		rm, _ := rv.(map[string]any)
		cols, _ := rm["columns"].([]any)
		for ci, cv := range cols {
			cm, _ := cv.(map[string]any)
			blocks, _ := cm["blocks"].([]any)
			for bi, bv := range blocks {
				spec, err := homeBlockSpec(bv)
				if err != nil || !spec.hasText || !homeWordsBlocks[spec.typ] {
					continue
				}
				want := normalizeWords(spec.text)
				for _, pb := range pool[spec.typ] {
					if pb.claim == "" && storedWords(pb.block.Config) == want {
						pb.claim = blockKey(ri, ci, bi)
						break
					}
				}
			}
		}
	}
}

// takePoolBlock gives the listed block at key the block it claimed, else the
// next block of its type nobody claimed, in the order found.
func takePoolBlock(blocks []*poolBlock, key string) *HomeBlock {
	for _, pb := range blocks {
		if !pb.used && pb.claim == key {
			pb.used = true
			return &pb.block
		}
	}
	for _, pb := range blocks {
		if !pb.used && pb.claim == "" {
			pb.used = true
			return &pb.block
		}
	}
	return nil
}

// storedWords is a written block's stored words in the markdown a lookup
// writes, so words sent back can be compared with what is there.
func storedWords(cfg map[string]any) string {
	content, _ := cfg["content"].(string)
	return normalizeWords(writtenMarkdown(content))
}

// normalizeWords makes line endings and outer blank space irrelevant to
// whether words changed.
func normalizeWords(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
}

// homeWordsBlocks are the blocks whose words are one markdown `text`, kept as
// sanitized HTML in config["content"].
var homeWordsBlocks = map[string]bool{"text_block": true, "table_agreement": true, "making_a_hero": true}

// maxWhereWePlayLine bounds the Foundry address and the voice line a
// where_we_play block holds; the block shows an address cut to 300 anyway.
const maxWhereWePlayLine = 300

// maxBrochureWords bounds the words on a brochure's box, which the box
// shortens anyway.
const maxBrochureWords = 300

// setBrochure writes a brochure block's page and words into its config. A
// field the record leaves out keeps the block's existing one, so listing the
// block again never loses a choice the owner made.
func (k HomeKind) setBrochure(ctx context.Context, campaignID string, b *HomeBlock, spec homeSpec) error {
	if b.Config == nil {
		b.Config = map[string]any{}
	}
	if spec.hasPage {
		name := strings.TrimSpace(spec.page)
		if name == "" {
			return fmt.Errorf("`page` names the brochure's page")
		}
		if k.Pages == nil {
			return fmt.Errorf("a brochure's page can't be looked up here")
		}
		e, err := k.Pages.GetBySlug(ctx, campaignID, entities.Slugify(name))
		if err != nil || e == nil || e.CampaignID != campaignID {
			return fmt.Errorf("no page called %s; the brochure's page must exist", quote(name))
		}
		b.Config["entity_id"] = e.ID
	}
	if spec.hasText {
		if len([]rune(spec.text)) > maxBrochureWords {
			return fmt.Errorf("the words on a brochure run over %d characters", maxBrochureWords)
		}
		if spec.text == "" {
			delete(b.Config, "content")
		} else {
			b.Config["content"] = spec.text
		}
	}
	return nil
}

// setWhereWePlay writes a where_we_play block's address and voice line into
// its config. A line the record leaves out keeps the block's existing one,
// and an empty one clears it, as for a brochure's words.
func setWhereWePlay(b *HomeBlock, spec homeSpec) error {
	if b.Config == nil {
		b.Config = map[string]any{}
	}
	for _, f := range []struct {
		key, val string
		has      bool
	}{{"foundry_url", spec.foundryURL, spec.hasFoundryURL}, {"voice", spec.voice, spec.hasVoice}} {
		if !f.has {
			continue
		}
		if len([]rune(f.val)) > maxWhereWePlayLine {
			return fmt.Errorf("`%s` runs over %d characters", f.key, maxWhereWePlayLine)
		}
		if f.val == "" {
			delete(b.Config, f.key)
		} else {
			b.Config[f.key] = f.val
		}
	}
	return nil
}

// homeBlockSpec reads a block written as its type or as {type, text, page,
// foundry_url, voice}.
func homeBlockSpec(v any) (homeSpec, error) {
	switch b := v.(type) {
	case string:
		return homeSpec{typ: strings.TrimSpace(b)}, nil
	case map[string]any:
		typ, _ := b["type"].(string)
		if strings.TrimSpace(typ) == "" {
			return homeSpec{}, fmt.Errorf("a block with `text`, `page`, `foundry_url` or `voice` needs a `type`")
		}
		spec := homeSpec{typ: strings.TrimSpace(typ)}
		if tv, ok := b["text"]; ok {
			spec.text, spec.hasText = strings.TrimSpace(fmt.Sprint(tv)), true
		}
		if pv, ok := b["page"]; ok {
			spec.page, spec.hasPage = fmt.Sprint(pv), true
		}
		if fv, ok := b["foundry_url"]; ok {
			spec.foundryURL, spec.hasFoundryURL = strings.TrimSpace(fmt.Sprint(fv)), true
		}
		if vv, ok := b["voice"]; ok {
			spec.voice, spec.hasVoice = strings.TrimSpace(fmt.Sprint(vv)), true
		}
		return spec, nil
	}
	return homeSpec{}, fmt.Errorf("write a block as its type, or `{type: text_block, text: \"…\"}`")
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
	if homeShape(t.next) == homeShape(t.was) && len(t.words) == 0 {
		return Plan{Error: "the " + t.word + " home already has these blocks in these places"}
	}
	p := Plan{Summary: "the " + t.word + " home becomes: " + homeSummary(t.next) + " (now: " + homeSummary(t.was) + ")"}
	for _, w := range t.words {
		// Only words that really change are shown; the rest stay as written.
		if w.was == "" {
			p.Summary += "; " + w.typ + " words: " + quoted(w.now)
		} else {
			p.Summary += "; " + w.typ + " words: " + quoted(w.now) + " (was " + quoted(w.was) + ")"
		}
	}
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

// homeShape is every column's width and blocks, to tell a change from none.
func homeShape(l HomeLayout) string {
	var b strings.Builder
	for _, row := range l.Rows {
		b.WriteString("/")
		for _, c := range row.Columns {
			fmt.Fprintf(&b, "|%d:", c.Width)
			for _, bl := range c.Blocks {
				b.WriteString(bl.Type)
				switch bl.Type {
				case "brochure":
					// Its page and words are the block's whole content, so a
					// change to them is a change to the home.
					fmt.Fprintf(&b, "(%v,%v)", bl.Config["entity_id"], bl.Config["content"])
				case "where_we_play":
					fmt.Fprintf(&b, "(%v,%v)", bl.Config["foundry_url"], bl.Config["voice"])
				}
				b.WriteString(",")
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
		k.writeHome(ctx, campaignID, &b, aud.word, l)
	}
	return b.String(), nil
}

// writeHome writes one home as the `kind: home` block that sends it back.
func (k HomeKind) writeHome(ctx context.Context, campaignID string, b *strings.Builder, audience string, l HomeLayout) {
	b.WriteString("---\nkind: home\naudience: " + audience + "\nrows:\n")
	for _, row := range l.Rows {
		b.WriteString("  - columns:\n")
		for _, c := range row.Columns {
			fmt.Fprintf(b, "      - width: %d\n        blocks:\n", c.Width)
			for _, bl := range c.Blocks {
				b.WriteString("          - " + k.blockSpec(ctx, campaignID, bl) + "\n")
			}
		}
	}
	b.WriteString("---\n\n")
}

// blockSpec writes one block in the form a record reads: its bare type, or
// the type with the words and settings the record can change. A block with
// none of them is written bare, which keeps whatever it holds.
func (k HomeKind) blockSpec(ctx context.Context, campaignID string, bl HomeBlock) string {
	switch bl.Type {
	case "text_block":
		if content, _ := bl.Config["content"].(string); content != "" {
			return "{type: text_block, text: " + quote(writtenMarkdown(content)) + "}"
		}
	case "table_agreement", "making_a_hero":
		if content, _ := bl.Config["content"].(string); strings.TrimSpace(content) != "" {
			return "{type: " + bl.Type + ", text: " + quote(writtenMarkdown(content)) + "}"
		}
	case "where_we_play":
		var parts []string
		if v, _ := bl.Config["foundry_url"].(string); strings.TrimSpace(v) != "" {
			parts = append(parts, "foundry_url: "+quote(strings.TrimSpace(v)))
		}
		if v, _ := bl.Config["voice"].(string); strings.TrimSpace(v) != "" {
			parts = append(parts, "voice: "+quote(strings.TrimSpace(v)))
		}
		if len(parts) > 0 {
			return "{type: where_we_play, " + strings.Join(parts, ", ") + "}"
		}
	case "brochure":
		return k.brochureSpec(ctx, campaignID, bl)
	}
	return bl.Type
}

// brochureSpec writes a brochure block in the form a record reads: the page by
// name and the words on the box. A page that is gone, or a block with neither,
// is written as the bare type, which keeps whatever the block holds.
func (k HomeKind) brochureSpec(ctx context.Context, campaignID string, b HomeBlock) string {
	var parts []string
	if id, _ := b.Config["entity_id"].(string); id != "" && k.Pages != nil {
		if e, err := k.Pages.GetByID(ctx, id); err == nil && e != nil && e.CampaignID == campaignID {
			parts = append(parts, "page: "+quote(e.Name))
		}
	}
	if words, _ := b.Config["content"].(string); strings.TrimSpace(words) != "" {
		parts = append(parts, "text: "+quote(words))
	}
	if len(parts) == 0 {
		return "brochure"
	}
	return "{type: brochure, " + strings.Join(parts, ", ") + "}"
}
