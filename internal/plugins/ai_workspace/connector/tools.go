package connector

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
)

// argMax caps one tool argument unless it says otherwise; nearly every
// argument is a name or a date.
const argMax = 200

// arg is one input a tool takes. Key is the lookup field it fills, or "name"
// for the lookup's own name.
type arg struct {
	Key, Desc string
	Required  bool
	// Max overrides argMax. Keep keeps the text's lines as sent; other
	// arguments are folded onto one line.
	Max  int
	Keep bool
}

func (a arg) max() int {
	if a.Max > 0 {
		return a.Max
	}
	return argMax
}

// tool is one tool: a lookup the copy-and-paste flow already answers (What),
// the campaign outline (What == ""), or, for a connection that may ask for
// changes (Asks), the writing instructions and the request itself.
type tool struct {
	Name, Title, Desc string
	// Did starts the "What it did" line, in the past tense.
	Did  string
	What string
	Args []arg
	// Asks marks the tools only an "Ask for changes" connection lists.
	Asks bool
	// Owner marks the tools only a key reading as the owner lists: the
	// Director's preparation and queues, which a player's view has no
	// business in. A player's key and an owner's Safe key never list them.
	Owner bool
	// PlayerDesc replaces Desc for a player's key, where the owner's
	// wording would be wrong.
	PlayerDesc string
}

// pieceArg lets any lookup tool step through an answer too long for one read
// (or a list past its line cap). It is not called `page`, which several tools
// already use for a page's name.
var pieceArg = arg{Key: "piece", Desc: "For an answer cut into pieces: which piece to read, like 2. Leave out for the first."}

// withPiece is args plus the piece argument.
func withPiece(args []arg) []arg {
	return append(args[:len(args):len(args)], pieceArg)
}

func init() {
	for i := range tools {
		if tools[i].What != "" && !tools[i].Asks {
			tools[i].Args = withPiece(tools[i].Args)
		}
	}
}

// toolConnect logs an AI app connecting; it is no tool the AI calls.
const toolConnect = "connect"

// The tools an "Ask for changes" connection adds.
const (
	toolHowTo = "how_to_write_changes"
	toolAsk   = "ask_for_changes"
	// toolWithdraw takes back one of the AI's own waiting requests.
	toolWithdraw = "withdraw_request"
)

// tools is every tool, in the order an AI is shown them. Each maps onto one
// lookup, so answers follow the lookup's privacy and size caps exactly.
var tools = []tool{
	{Name: "campaign_outline", Title: "Campaign outline", Did: "Read the outline of your world",
		Desc:       "Start here. It opens with what this connection reads as (the owner, GM-only pages and secrets included, or only what the campaign's players see) and whether it may ask for changes. Then the campaign's name and every page it can see, grouped by kind (characters, places...), with pages players never see marked (GM only). Use read_page for one page's text.",
		PlayerDesc: "Start here. It opens with what this connection reads as (you, as a player: nothing GM-only) and whether it may ask for changes. Then every page you can see, grouped by kind (characters, places...). Use read_page for one page's text."},
	{Name: "list_pages", Title: "List pages", Did: "Listed pages", What: "pages",
		Desc: "Page names with their tags, optionally only one kind of page or one tag.",
		Args: []arg{{Key: "type", Desc: "A kind of page, for example \"Character\""}, {Key: "tag", Desc: "A tag's name"}}},
	{Name: "search", Title: "Search the campaign", Did: "Searched the campaign", What: "search",
		Desc: "Find pages by words: matches a page's name, other names, kind and text, and quotes a short line from each hit. Use it when you don't know a page's exact name; read_page then reads the hit in full. Only pages you can see are searched. A long list of hits comes in pieces: give piece for the next.",
		Args: []arg{{Key: "query", Desc: "The words to look for (at least two characters)", Required: true}, {Key: "type", Desc: "Only this kind of page, for example \"Character\""}}},
	{Name: "read_page", Title: "Read a page", Did: "Read the page", What: "page",
		Desc: "One page: its text, tags, links and sheet, and its map when it has one.",
		Args: []arg{{Key: "name", Desc: "The page's name", Required: true}}},
	{Name: "list_tags", Title: "Tags", Did: "Listed the tags", What: "tags",
		Desc: "Every tag, with how many pages carry it."},
	{Name: "rolling_tables", Owner: true, Title: "Rolling tables", Did: "Read rolling tables", What: "table",
		Desc: "The rolling tables, or one table's entries when given its name.",
		Args: []arg{{Key: "name", Desc: "A table's name, to read its entries"}}},
	{Name: "map_pins", Title: "Maps and pins", Did: "Looked at maps", What: "pins",
		Desc: "The maps, or one map's pins when given its name. A hex map also comes with its hexes (land, roads, rivers, trails, named hexes), written as a block you can send back changed.",
		Args: []arg{{Key: "map", Desc: "A map's name, to list its pins and hexes"}, {Key: "rows", Desc: "Rows of a big hex map to read, like 0-19"}}},
	{Name: "foundry_scenes", Owner: true, Title: "Foundry scenes", Did: "Looked at the Foundry scenes", What: "scenes",
		Desc: "The scenes in the campaign's Foundry world, which says which maps already exist there. Give a name to check for one.",
		Args: []arg{{Key: "name", Desc: "Part of a scene's name or folder, to check whether a map exists"}}},
	{Name: "shop_stock", Title: "What a shop sells", Did: "Looked at a shop", What: "stock",
		Desc: "What a shop page sells, with prices and how many and which goods turn up on today's shelf only some days, and its room (what kind of shop, its counter room, what the keeper says) and storefront on the Shops street, written as blocks you can send back changed.",
		Args: []arg{{Key: "shop", Desc: "The shop's page name", Required: true}}},
	{Name: "inventory", Title: "What a character carries", Did: "Looked at what someone carries", What: "inventory",
		Desc: "What a character carries.",
		Args: []arg{{Key: "character", Desc: "The character's page name", Required: true}}},
	{Name: "crafting", Title: "Crafting", Did: "Looked at crafting", What: "crafting",
		Desc: "The campaign's crafting: the game system's crafting rules, every recipe, every material (with the pages that drop it and the recipes that use it), every page's drop list, what waits in the spoils tray, heroes' projects and the benches. Each recipe comes with its \"When… then…\" rules. " +
			"Give part to get recipes, materials, drop lists or benches written as blocks you can send back changed, or looks for each item's look (how it shows its magic when used). A connection that sees what players see reads only the recipe book.",
		Args: []arg{
			{Key: "part", Desc: "recipes, materials, drops, benches or looks: that part, written as blocks"},
			{Key: "name", Desc: "One recipe, material, bench or page with a drop list"},
		}},
	{Name: "my_notes", Title: "My notes", Did: "Listed your notes", What: "notes",
		Desc:       "The campaign owner's Journal notes, page jots and Player Notes on pages, and the ones other members shared with them, game night entries among them: titles, folders, pages, nights, who wrote each and who it is shared with. Use read_note for one note's text.",
		PlayerDesc: "Your own Journal notes, page jots and Player Notes on pages, and the ones other members shared with you: titles, folders, pages, who wrote each and who it is shared with. Use read_note for one note's text."},
	{Name: "read_note", Title: "Read a note", Did: "Read a note", What: "note",
		Desc: "One note's full text, by its title as my_notes lists it. Notes other members wrote can be read, never changed.",
		Args: []arg{
			{Key: "name", Desc: "The note's title", Required: true},
			{Key: "page", Desc: "For a jot or Player Note on a page: the page's name"},
			{Key: "by", Desc: "Who wrote it, when two notes share a title"},
		}},
	{Name: "house_rules", Title: "House rules", Did: "Read the house rules", What: "house-rules",
		Desc: "The campaign's house rules, chapter by chapter. A rulebook too long for one answer lists every chapter's title; give name for any one chapter.",
		Args: []arg{{Key: "name", Desc: "One chapter's title"}}},
	{Name: "system_entries", Title: "Game system entries", Did: "Looked up game system entries", What: "system-entries",
		Desc: "The game system's entries for a character field (ancestries, kits...). Without field, the fields that have entries.",
		Args: []arg{{Key: "field", Desc: "A character field, for example \"ancestry\""}, {Key: "name", Desc: "Part of an entry's name, to narrow a long list"}}},
	{Name: "waiting_for_review", Owner: true, Title: "Waiting for review", Did: "Looked at what is waiting for review", What: "waiting",
		Desc: "What is waiting for the owner's yes or no: the changes you asked for that are still waiting, every answer the owner gave to yours in the last 7 days (newest first), entries players wrote and handed in, and players' stash moves and requests to buy. Read-only: only the owner can accept or turn these down, in Chronicle.",
		Args: []arg{{Key: "older", Desc: "How many days back to start from, to page through older answers in 7-day windows: 7 shows days 7 to 14, 14 shows days 14 to 21"}}},
	{Name: "players", Owner: true, Title: "Players", Did: "Looked at the players", What: "players",
		Desc: "Each player's character right now, what they carry, and whether they are coming to the next game night."},
	{Name: "home", Owner: true, Title: "Home pages", Did: "Looked at the home pages", What: "home",
		Desc: "The home page each audience sees (everyone, players, scribes, directors, guests), written as home blocks you can send back changed: its rows, columns and blocks, the words in its Text, Table agreement and Making a hero blocks, and the Where we play address. It says which audiences have no home of their own and use the Everyone home, and which are arranged in Customize and can't be written back.",
		Args: []arg{{Key: "audience", Desc: "One audience: everyone, players, scribes, directors or guests"}}},
	{Name: "encounters", Owner: true, Title: "Encounters", Did: "Looked at the encounters", What: "encounters",
		Desc: "The fights planned on pages, written as encounter blocks you can send back changed: heroes, creatures, groups, starts and versions, the version that plays now, any creature Foundry can't place, and the EV written on the creature pages added up (not the party's budget or the difficulty, which the encounter builder works out).",
		Args: []arg{{Key: "page", Desc: "Only the fights on this page"}}},
	{Name: "tests", Owner: true, Title: "Tests", Did: "Looked at the tests", What: "tests",
		Desc: "The montages and other tests on pages, with their results and the one recorded. An encounter version names one of these results to be picked by it.",
		Args: []arg{{Key: "page", Desc: "Only the tests on this page"}}},
	{Name: "trackers", Owner: true, Title: "Trackers", Did: "Looked at the trackers", What: "trackers",
		Desc: "The counters, named steps and clocks kept on pages, hidden ones included: each one's value, what it means now and its latest changes. Change one with a tracker block in ask_for_changes.",
		Args: []arg{{Key: "page", Desc: "Only the trackers on this page"}}},
	{Name: "foundry_monsters", Owner: true, Title: "Foundry's monsters", Did: "Looked at Foundry's monsters", What: "foundry-monsters",
		Desc: "The monsters the campaign's Foundry world can place, from its actors and compendiums, with level, role and EV. Check here before writing a Creature page: a monster Foundry has needs no page to be used in a fight.",
		Args: []arg{{Key: "name", Desc: "Part of a monster's name"}, {Key: "level", Desc: "A level, for example \"3\""}, {Key: "role", Desc: "A role or organization, for example \"Harrier\" or \"Horde\""}}},
	{Name: "block_format", Title: "How to write a block", Did: "Read how to write a block", What: "format",
		Desc: "How Chronicle writes one kind of block (an event, a calendar, a rolling table...).",
		Args: []arg{{Key: "block", Desc: "The kind of block, for example \"event\"", Required: true}}},
	{Name: toolHowTo, Title: "How to write changes", Did: "Read how to write changes", Asks: true,
		Desc:       "Read this before ask_for_changes: how Chronicle writes pages and every other kind of block. It comes in parts so each fits one read: without arguments the guide, then a category's sheet fields with category, and each kind of block's format from block_format.",
		PlayerDesc: "Read this before ask_for_changes: how to write a change to your own character's sheet, the sheets you may change, and the choices the hero builder offers.",
		Args: []arg{
			{Key: "category", Desc: "A category's name, for example \"Creature\": its sheet fields"},
			{Key: "part", Desc: "\"categories\" for every category's sheet fields at once"},
		}},
	{Name: toolAsk, Title: "Ask for changes", Did: "Asked for changes", Asks: true,
		Desc: "Send changes for the campaign's owner to review: new pages, edits, removals, events and the other blocks how_to_write_changes describes. " +
			"Nothing changes until the owner says yes to each one in Chronicle, so tell them it is waiting under the AI button in Chronicle; waiting_for_review shows later how they answered. Send one request per job, not one per block.",
		PlayerDesc: "Send changes to your own character's sheet for you to review: the identity fields and the hero builder's choices. " +
			"Nothing changes until you say yes to each one under the AI button in Chronicle. Changes to anything else are held back. Send one request per job.",
		Args: []arg{
			{Key: "summary", Required: true, Desc: "One line saying what this does, for example \"Five characters for Coral Court\""},
			{Key: "changes", Required: true, Max: changesMax, Keep: true, Desc: "The blocks, written as how_to_write_changes says, one after another"},
		}},
	{Name: toolWithdraw, Title: "Withdraw a request", Did: "Withdrew a request", Asks: true,
		Desc: "Take back one of your own requests for changes while it is still waiting, when it is wrong or no longer wanted. Give its number, as ask_for_changes and waiting_for_review show it, and a reason: the owner sees the reason in Chronicle. " +
			"Nothing in the campaign changes. A request the owner already answered, or is applying, can't be withdrawn, and neither can anyone else's.",
		PlayerDesc: "Take back one of your own requests for changes while it is still waiting, when it is wrong or no longer wanted. Give its number, as ask_for_changes showed it, and a reason, which is kept with it in Chronicle. Nothing changes.",
		Args: []arg{
			{Key: "request", Required: true, Desc: "The request's number, for example \"12\""},
			{Key: "reason", Required: true, Max: withdrawReasonMax, Desc: "Why it is withdrawn, in a sentence, for the owner's record"},
		}},
}

// toolsFor is the tools a connection may use, in order: the built-in ones,
// with the lookups features plug in (l.Plugged) before block_format. Only a
// key that reads as the owner lists the owner's tools, and one that isn't
// listed can't be called. Most of those lookups also refuse a player's view
// on their own; hiding them as well means a Safe key is never offered GM
// material, so the guarantee doesn't rest on every lookup's own check.
func toolsFor(c *Connection, l *records.Lookups) []*tool {
	out := make([]*tool, 0, len(tools))
	owner := readsAsOwner(c)
	add := func(t *tool) {
		if t.Asks && (c == nil || !c.CanAsk) {
			return
		}
		if t.Owner && !owner {
			return
		}
		out = append(out, t)
	}
	for i := range tools {
		if tools[i].What == "format" && l != nil {
			for _, p := range l.Plugged {
				if t := pluggedTool(p.LookupInfo()); t != nil {
					add(t)
				}
			}
		}
		add(&tools[i])
	}
	return out
}

// pluggedTool is a feature's lookup as a tool; nil when it lists none.
func pluggedTool(in aiplug.LookupInfo) *tool {
	if in.Tool.Name == "" {
		return nil
	}
	t := &tool{
		Name: in.Tool.Name, Title: in.Tool.Title, Did: in.Tool.Did, What: in.What,
		Desc: in.Tool.Desc, PlayerDesc: in.Tool.PlayerDesc, Owner: in.Tool.Owner,
	}
	for _, a := range in.Tool.Args {
		t.Args = append(t.Args, arg{Key: a.Key, Desc: a.Desc, Required: a.Required})
	}
	t.Args = withPiece(t.Args)
	return t
}

// findTool is the connection's tool by name; one it may not use is not found.
func findTool(c *Connection, l *records.Lookups, name string) *tool {
	for _, t := range toolsFor(c, l) {
		if t.Name == name {
			return t
		}
	}
	return nil
}

// definition is the tool as tools/list describes it. Every lookup is marked
// read-only, so an AI app may run it without asking. ask_for_changes and
// withdraw_request are not read-only, since they leave or settle a request
// for the owner, but they are not destructive: neither changes the campaign.
func (t *tool) definition(forPlayer bool) map[string]any {
	props := map[string]any{}
	required := []string{}
	for _, a := range t.Args {
		props[a.Key] = map[string]any{"type": "string", "description": a.Desc, "maxLength": a.max()}
		if a.Required {
			required = append(required, a.Key)
		}
	}
	return map[string]any{
		"name":        t.Name,
		"title":       t.Title,
		"description": t.describe(forPlayer),
		"inputSchema": map[string]any{
			"type":                 "object",
			"properties":           props,
			"required":             required,
			"additionalProperties": false,
		},
		"annotations": map[string]any{
			"title":           t.Title,
			"readOnlyHint":    !t.writes(),
			"destructiveHint": false,
			"idempotentHint":  !t.writes(),
			"openWorldHint":   false,
		},
	}
}

// writes reports a tool that leaves or settles a request for the owner.
func (t *tool) writes() bool { return t.Name == toolAsk || t.Name == toolWithdraw }

// describe is the tool's description for the key's maker.
func (t *tool) describe(forPlayer bool) string {
	if forPlayer && t.PlayerDesc != "" {
		return t.PlayerDesc
	}
	return t.Desc
}

// readArgs keeps only the tool's own arguments, each a string within its
// cap. A missing required one is an error the AI can fix.
// takes reports whether the tool has an argument called key.
func (t *tool) takes(key string) bool {
	for _, a := range t.Args {
		if a.Key == key {
			return true
		}
	}
	return false
}

// argList names the tool's arguments: "part, name".
func (t *tool) argList() string {
	keys := make([]string, len(t.Args))
	for i, a := range t.Args {
		keys[i] = a.Key
	}
	return strings.Join(keys, ", ")
}

func (t *tool) readArgs(in map[string]any) (map[string]string, error) {
	// An argument the tool doesn't take is refused, not dropped, so the AI
	// never believes a filter or a part it made up was applied.
	for k := range in {
		if !t.takes(k) {
			if len(t.Args) == 0 {
				return nil, fmt.Errorf("%s takes no arguments, so %s can't be used", t.Name, oneLine(k, 40))
			}
			return nil, fmt.Errorf("%s doesn't take %s; it takes %s", t.Name, oneLine(k, 40), t.argList())
		}
	}
	out := map[string]string{}
	for _, a := range t.Args {
		v, ok := in[a.Key]
		s := ""
		if ok && v != nil {
			switch x := v.(type) {
			case string:
				s = x
			case float64:
				s = strconv.FormatFloat(x, 'f', -1, 64)
			case bool:
				s = strconv.FormatBool(x)
			default:
				return nil, fmt.Errorf("%s should be text", a.Key)
			}
		}
		if a.Keep {
			s = strings.TrimSpace(s)
		} else {
			s = strings.Join(strings.Fields(s), " ")
		}
		if len(s) > a.max() {
			return nil, fmt.Errorf("%s is too long (at most %d characters)", a.Key, a.max())
		}
		if s == "" {
			if a.Required {
				return nil, fmt.Errorf("it needs %s", a.Key)
			}
			continue
		}
		out[a.Key] = s
	}
	return out, nil
}

// summary is the "What it did" line for one call. The AI's own words are
// quoted, so they can never read as Chronicle's.
func (t *tool) summary(args map[string]string) string {
	var parts []string
	// The piece argument rides on every lookup; it must not turn a
	// one-argument tool's "Read rolling tables: “Loot”" into a keyed line.
	own := len(t.Args)
	if t.takes(pieceArg.Key) {
		own--
	}
	for _, a := range t.Args {
		if v := args[a.Key]; v != "" {
			q := "“" + oneLine(v, 80) + "”"
			if own == 1 && a.Key != pieceArg.Key {
				parts = append(parts, q)
			} else {
				parts = append(parts, a.Key+" "+q)
			}
		}
	}
	if len(parts) == 0 {
		return t.Did
	}
	return t.Did + ": " + strings.Join(parts, ", ")
}

// answer runs the tool as the session's Actor. It returns the text for the
// AI, the note for the log, and whether it was answered.
func (t *tool) answer(ctx context.Context, l *records.Lookups, sess Session, args map[string]string) (text, note string, ok bool) {
	campaignID, a := "", sess.Actor
	if sess.Conn != nil {
		campaignID = sess.Conn.CampaignID
	}
	if t.What == "" {
		return outline(ctx, l, sess.Conn, campaignID, sess.CampaignName, a)
	}
	if l == nil {
		l = &records.Lookups{}
	}
	fields := map[string]any{"what": t.What}
	rec := records.Record{Kind: records.KindLookup, Fields: fields}
	for k, v := range args {
		if k == "name" {
			rec.Name = v
		} else {
			fields[k] = v
		}
	}
	ans := l.Answer(ctx, campaignID, a, []records.Record{rec})
	if len(ans) == 0 {
		return "Not answered.", "not answered", false
	}
	if ans[0].Error != "" {
		return "Not answered: " + strings.TrimRight(ans[0].Error, ".") + ".", ans[0].Error, false
	}
	return strings.TrimSpace(ans[0].Text), ans[0].Chip, true
}

// outline is the campaign's name, what the connection reads as, and its
// page index, as the copy-and-paste prompt's "What already exists" reads it.
func outline(ctx context.Context, l *records.Lookups, c *Connection, campaignID, campaignName string, a records.Actor) (string, string, bool) {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", oneLine(campaignName, 200))
	b.WriteString(accessSection(c))
	index := ""
	if l != nil {
		index = l.PageIndex(ctx, campaignID, a)
	}
	if index == "" {
		b.WriteString("No pages to show yet.\n")
		return b.String(), "no pages", true
	}
	b.WriteString("## Pages by kind\n\n")
	b.WriteString(index)
	b.WriteString("\nUse read_page for one page's text, and list_pages for the rest of a long list.\n")
	if menu := l.MenuOrder(ctx, campaignID, a); menu != "" {
		b.WriteString("\n## Sidebar menu\n\nThe categories in the order the menu shows them: " + menu +
			". A `menu-order` change can reorder them.\n")
	}
	return b.String(), "categories and names", true
}

// readsAsOwner reports whether the connection reads as the campaign's owner:
// an owner's key set to Permitted. It is actorFor's rule, so what a key is
// told and offered never drifts from what it reads.
func readsAsOwner(c *Connection) bool {
	return c != nil && !c.ForPlayer && c.Privacy != aiexport.PrivacyModeSafe
}

// accessSection is the outline's "This connection": who the key reads as and
// whether it may ask for changes. It is in a tool answer because many AI apps
// drop or summarise the initialize instructions, and the AI has to know
// whether what it reads may be repeated to players.
func accessSection(c *Connection) string {
	asks := "Changes: it only reads."
	if c != nil && c.CanAsk {
		asks = "Changes: it may ask for changes, which wait for the owner's yes in Chronicle."
	}
	var reads, who string
	switch {
	case c != nil && c.ForPlayer:
		reads = "you, as a player in this campaign: what players see, plus the pages you claimed and those shared with you. Nothing GM-only."
		if c.CanAsk {
			asks = "Changes: it may ask for changes to your own character's sheet, which wait for your yes under the AI button in Chronicle."
		}
		who = "You choose whether it may ask for changes in your AI panel in Chronicle; the campaign's owner decides whether players may use AI at all."
	case readsAsOwner(c):
		reads = "the campaign's owner: everything, GM-only pages and secret text included. Pages marked (GM only) and text between {gm{ and }gm} are secrets; never repeat them to players."
		who = "The owner sets both on the Your AI page in Chronicle."
	default:
		reads = "the campaign's players: only what players see. GM-only pages, secret text, notes and the Director's preparation are left out."
		who = "The owner sets both on the Your AI page in Chronicle."
	}
	return "## This connection\n\n- Reads as " + reads + "\n- " + asks + "\n- " + who + "\n\n"
}
