package connector

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
)

// argMax caps one tool argument; every argument is a name or a date.
const argMax = 200

// arg is one input a tool takes. Key is the lookup field it fills, or "name"
// for the lookup's own name.
type arg struct {
	Key, Desc string
	Required  bool
}

// tool is one read-only tool: a lookup the copy-and-paste flow already
// answers (What), or the campaign outline (What == "").
type tool struct {
	Name, Title, Desc string
	// Did starts the "What it did" line, in the past tense.
	Did  string
	What string
	Args []arg
}

var dateArgs = []arg{
	{Key: "from", Desc: "First day, as the calendar writes it (for example \"Deepwinter 1 1492\")"},
	{Key: "to", Desc: "Last day, as the calendar writes it"},
	{Key: "year", Desc: "A year (instead of from and to), for example \"1492\""},
	{Key: "month", Desc: "A month of that year, by name or number"},
}

// tools is every tool, in the order an AI is shown them. Each maps onto one
// lookup, so answers follow the lookup's privacy and size caps exactly.
var tools = []tool{
	{Name: "campaign_outline", Title: "Campaign outline", Did: "Read the outline of your world",
		Desc: "Start here. The campaign's name and every page it can see, grouped by kind (characters, places...). Use read_page for one page's text."},
	{Name: "list_pages", Title: "List pages", Did: "Listed pages", What: "pages",
		Desc: "Page names with their tags, optionally only one kind of page or one tag.",
		Args: []arg{{Key: "type", Desc: "A kind of page, for example \"Character\""}, {Key: "tag", Desc: "A tag's name"}}},
	{Name: "read_page", Title: "Read a page", Did: "Read the page", What: "page",
		Desc: "One page: its text, tags, links and sheet, and its map when it has one.",
		Args: []arg{{Key: "name", Desc: "The page's name", Required: true}}},
	{Name: "list_tags", Title: "Tags", Did: "Listed the tags", What: "tags",
		Desc: "Every tag, with how many pages carry it."},
	{Name: "calendar", Title: "The calendar", Did: "Read the calendar", What: "calendar",
		Desc: "The main calendar: months, weekdays, seasons, moons and their phases, eras, festivals, today's date and the climate."},
	{Name: "weather_kinds", Title: "Kinds of weather", Did: "Read the kinds of weather", What: "weather-kinds",
		Desc: "Every climate and kind of weather the calendar knows."},
	{Name: "calendar_events", Title: "Calendar events", Did: "Looked up events", What: "events",
		Desc: "Calendar events in a range of dates: from and to, or a year and a month.", Args: dateArgs},
	{Name: "weather", Title: "Weather by day", Did: "Looked up the weather", What: "weather",
		Desc: "Each day's weather in a range of dates: from and to, or a year and a month.", Args: dateArgs},
	{Name: "rolling_tables", Title: "Rolling tables", Did: "Read rolling tables", What: "table",
		Desc: "The rolling tables, or one table's entries when given its name.",
		Args: []arg{{Key: "name", Desc: "A table's name, to read its entries"}}},
	{Name: "map_pins", Title: "Maps and pins", Did: "Looked at maps", What: "pins",
		Desc: "The maps, or one map's pins when given its name.",
		Args: []arg{{Key: "map", Desc: "A map's name, to list its pins"}}},
	{Name: "shop_stock", Title: "What a shop sells", Did: "Looked at a shop", What: "stock",
		Desc: "What a shop page sells, with prices and how many.",
		Args: []arg{{Key: "shop", Desc: "The shop's page name", Required: true}}},
	{Name: "inventory", Title: "What a character carries", Did: "Looked at what someone carries", What: "inventory",
		Desc: "What a character carries.",
		Args: []arg{{Key: "character", Desc: "The character's page name", Required: true}}},
	{Name: "my_notes", Title: "My notes", Did: "Listed your notes", What: "notes",
		Desc: "The titles of the campaign owner's own notes."},
	{Name: "house_rules", Title: "House rules", Did: "Read the house rules", What: "house-rules",
		Desc: "The campaign's house rules, chapter by chapter."},
	{Name: "system_entries", Title: "Game system entries", Did: "Looked up game system entries", What: "system-entries",
		Desc: "The game system's entries for a character field (ancestries, kits...). Without field, the fields that have entries.",
		Args: []arg{{Key: "field", Desc: "A character field, for example \"ancestry\""}}},
	{Name: "game_nights", Title: "Game nights", Did: "Listed the game nights", What: "game-nights",
		Desc: "The campaign's game nights: name, date, time, status and summary."},
	{Name: "players", Title: "Players", Did: "Looked at the players", What: "players",
		Desc: "Each player's character right now, what they carry, and whether they are coming to the next game night."},
	{Name: "block_format", Title: "How to write a block", Did: "Read how to write a block", What: "format",
		Desc: "How Chronicle writes one kind of block (an event, a calendar, a rolling table...), for answers the owner pastes in by hand. These tools can't make changes.",
		Args: []arg{{Key: "block", Desc: "The kind of block, for example \"event\"", Required: true}}},
}

func findTool(name string) *tool {
	for i := range tools {
		if tools[i].Name == name {
			return &tools[i]
		}
	}
	return nil
}

// definition is the tool as tools/list describes it. Every tool is marked
// read-only, so an AI app may run it without asking.
func (t *tool) definition() map[string]any {
	props := map[string]any{}
	required := []string{}
	for _, a := range t.Args {
		props[a.Key] = map[string]any{"type": "string", "description": a.Desc, "maxLength": argMax}
		if a.Required {
			required = append(required, a.Key)
		}
	}
	return map[string]any{
		"name":        t.Name,
		"title":       t.Title,
		"description": t.Desc,
		"inputSchema": map[string]any{
			"type":                 "object",
			"properties":           props,
			"required":             required,
			"additionalProperties": false,
		},
		"annotations": map[string]any{
			"title":           t.Title,
			"readOnlyHint":    true,
			"destructiveHint": false,
			"idempotentHint":  true,
			"openWorldHint":   false,
		},
	}
}

// readArgs keeps only the tool's own arguments, each a short string. A
// missing required one is an error the AI can fix.
func (t *tool) readArgs(in map[string]any) (map[string]string, error) {
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
		s = strings.Join(strings.Fields(s), " ")
		if len(s) > argMax {
			return nil, fmt.Errorf("%s is too long (at most %d characters)", a.Key, argMax)
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
	for _, a := range t.Args {
		if v := args[a.Key]; v != "" {
			q := "“" + oneLine(v, 80) + "”"
			if len(t.Args) == 1 {
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

// answer runs the tool as the connection's Actor. It returns the text for
// the AI, the note for the log, and whether it was answered.
func (t *tool) answer(ctx context.Context, l *records.Lookups, campaignID, campaignName string, a records.Actor, args map[string]string) (text, note string, ok bool) {
	if t.What == "" {
		return outline(ctx, l, campaignID, campaignName, a)
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

// outline is the campaign's name and its page index, as the copy-and-paste
// prompt's "What already exists" reads it.
func outline(ctx context.Context, l *records.Lookups, campaignID, campaignName string, a records.Actor) (string, string, bool) {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", oneLine(campaignName, 200))
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
	return b.String(), "categories and names", true
}
