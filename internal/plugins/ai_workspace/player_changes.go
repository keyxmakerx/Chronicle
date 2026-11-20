// player_changes.go is what a player's AI may change: the sheet of a
// character that player claimed, and nothing else. A player's request or
// paste is read here, each block either kept as a sheet change or held back
// with a reason, and every kept change goes through PlayerSheets, which
// makes the same checks the sheet's own editor makes for that player. So a
// player's AI can change exactly what the player could change by hand.

package ai_workspace

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
)

// PlayerSheet is one character a player claimed, as their AI sees it. Every
// key listed is a field of the character's category that the player may
// change, each listed once, with the rule the server applies to it.
type PlayerSheet struct {
	ID   string
	Name string
	// Type is the page's category, for the prompt.
	Type string
	// Values is the sheet's current value for each key the player may
	// change, as text ("" for empty).
	Values map[string]string
	// Free lists the keys that take short free text (the identity fields).
	Free []string
	// Picks maps each hero builder step's key to the names it offers; the
	// value must be one of them, unless Own says the player may also write
	// their own.
	Picks map[string][]string
	Own   map[string]bool
	// Ask maps a step's key to the entries the player may pick only with
	// their Director's yes. The AI can't ask for them, so the brief says to
	// leave them to the player, and the server refuses a write of one.
	Ask map[string][]string
	// Required marks a step whose list is a short list the Director set: it
	// can't be emptied. Fixed holds the one entry such a step is filled in
	// with, which can't be changed either.
	Required map[string]bool
	Fixed    map[string]string
	// Story lists the keys that take the player's long text (a backstory).
	Story []string
	// Lists lists the keys that take a JSON list of short names (skills,
	// languages, perks).
	Lists []string
	// Buys maps a key keeping what the hero bought from an entry
	// (ancestry_choices_json) to what that entry offers.
	Buys map[string]PlayerBuy
	// Shown is the rest of the sheet as the player sees it, key to value,
	// for the AI to recommend from; none of it can be changed.
	Shown map[string]string
}

// PlayerBuy is what one entry lets a hero buy, and the points to spend.
type PlayerBuy struct {
	Entry  string
	Budget float64
	Names  []string
	Costs  map[string]float64
}

// PlayerSheets is the door a player's AI changes go through. internal/app
// implements it over the entities plugin and the hero builder.
type PlayerSheets interface {
	// Sheets lists the characters userID claimed in the campaign and can
	// still open.
	Sheets(ctx context.Context, campaignID, userID string) ([]PlayerSheet, error)
	// HeldFields judges each key of patch against what userID may change on
	// their own character entityID and returns, for each key they may not,
	// why, in the player's terms. err is for a change they may not make at
	// all (not their character, one they can't open).
	HeldFields(ctx context.Context, campaignID, userID, entityID string, patch map[string]any) (map[string]string, error)
	// PatchSheet checks patch against what userID may change on their own
	// character entityID, and writes it when apply is true. label names
	// the AI app, for the page's history. The error says, in the player's
	// terms, why a change isn't allowed.
	PatchSheet(ctx context.Context, campaignID, userID, entityID string, patch map[string]any, label string, apply bool) error
	// GMTerm is what the campaign calls its game master.
	GMTerm(ctx context.Context, campaignID string) string
}

// SetPlayerSheets wires the door for players' AI changes. Unwired, a
// player's AI can look things up but every change is held back.
func (h *Handler) SetPlayerSheets(s PlayerSheets) { h.playerSheets = s }

// playerGMTerm is the campaign's word for its game master, for the brief.
func (h *Handler) playerGMTerm(ctx context.Context, campaignID string) string {
	if h.playerSheets == nil {
		return ""
	}
	return h.playerSheets.GMTerm(ctx, campaignID)
}

// PlayerChange is one block of a player's request, and what happens to it.
type PlayerChange struct {
	// Block is the block's place in the request, from 1.
	Block int
	Page  string
	// SheetID is the character it changes; "" when held back.
	SheetID string
	Fields  map[string]any
	Lines   []PlayerChangeLine
	// Reason is the AI's own one line about the change (untrusted, shown
	// in quotes).
	Reason string
	// Held says why the block can't be applied; "" when it can.
	Held string
}

// PlayerChangeLine is one field a change sets, from its current value.
type PlayerChangeLine struct {
	Key, From, To string
}

// playerBlockKeys are the front-matter keys a sheet change may carry. type
// and kind are tolerated because AIs write them out of habit; neither can
// move or retype the page here.
var playerBlockKeys = map[string]bool{"name": true, "action": true, "reason": true, "fields": true, "type": true, "kind": true}

// planPlayerChanges reads a player's blocks. A block's fields are judged one
// by one: the ones the player may change are kept as one change, and each
// one held back is its own "left out" entry with its reason, so one field
// the AI may not write never hides the rest. The caller applies the kept
// changes chosen, and PatchSheet checks each again then.
func planPlayerChanges(ctx context.Context, sheets PlayerSheets, campaignID, userID, changes string) ([]PlayerChange, error) {
	var mine []PlayerSheet
	if sheets != nil {
		var err error
		if mine, err = sheets.Sheets(ctx, campaignID, userID); err != nil {
			return nil, err
		}
	}
	parsed := importer.Parse(changes)
	out := make([]PlayerChange, 0, len(parsed))
	for i, p := range parsed {
		ch := PlayerChange{Block: i + 1, Page: p.Name, Reason: oneLineText(p.FrontMatter.Reason, 200)}
		ch.Held = holdPlayerBlock(p)
		if ch.Held == "" && sheets == nil {
			ch.Held = "changes from a player's AI can't be applied on this server"
		}
		var sheet *PlayerSheet
		if ch.Held == "" {
			for j := range mine {
				if strings.EqualFold(strings.TrimSpace(mine[j].Name), strings.TrimSpace(p.Name)) {
					sheet = &mine[j]
					break
				}
			}
			if sheet == nil {
				ch.Held = fmt.Sprintf("%q isn't a character you claimed; your AI can only change your own", p.Name)
			}
		}
		if ch.Held != "" {
			out = append(out, ch)
			continue
		}
		left := playerBlockParts(p, sheet.Name, ch.Block)
		fields := p.FrontMatter.Fields
		if len(fields) == 0 {
			if len(left) == 0 {
				ch.Held = "it changes no sheet fields"
				out = append(out, ch)
			}
			out = append(out, left...)
			continue
		}
		held, err := sheets.HeldFields(ctx, campaignID, userID, sheet.ID, fields)
		if err != nil {
			ch.Held = playerRefusal(err)
			out = append(out, ch)
			continue
		}
		kept := map[string]any{}
		for k, v := range fields {
			if why, no := held[k]; no {
				left = append(left, PlayerChange{Block: ch.Block, Page: sheet.Name, Held: why})
			} else {
				kept[k] = v
			}
		}
		sort.SliceStable(left, func(a, b int) bool { return left[a].Held < left[b].Held })
		if len(kept) > 0 {
			// Only fields that differ are offered and written, so a yes never
			// records a change that changed nothing.
			if ch.Lines, ch.Fields = changeLines(sheet, kept); len(ch.Lines) == 0 {
				ch.Held, ch.Fields = "your character's sheet already says this", nil
			} else {
				ch.SheetID, ch.Page = sheet.ID, sheet.Name
			}
			out = append(out, ch)
		}
		out = append(out, left...)
	}
	return out, nil
}

// holdPlayerBlock is why a block can't be a change to a player's own sheet
// at all, judged from the block alone; "" when it can be.
func holdPlayerBlock(p importer.ParsedPage) string {
	switch {
	case p.Status == importer.StatusParseError:
		return "Chronicle couldn't read this block: " + strings.TrimRight(p.ParseError, ".")
	case strings.EqualFold(strings.TrimSpace(p.FrontMatter.Kind), records.KindLookup):
		return "lookups aren't answered here; connect your AI to look things up"
	case p.IsRecord():
		return fmt.Sprintf("this is a %s; your AI can only change your character's sheet", strings.TrimSpace(p.FrontMatter.Kind))
	case !strings.EqualFold(strings.TrimSpace(p.FrontMatter.Action), "update"):
		return "your AI can only change a character you already have; it can't add or remove pages"
	case p.FrontMatter.IsEra():
		return "your AI can't write an era's version of a page"
	}
	return ""
}

// playerBlockParts is the parts of a block besides its fields that a
// player's AI can't set (other front matter, the page's text), each left
// out with its reason while the fields go on.
func playerBlockParts(p importer.ParsedPage, page string, block int) []PlayerChange {
	var out []PlayerChange
	var extra []string
	for k := range p.Fields {
		if !playerBlockKeys[strings.ToLower(k)] {
			extra = append(extra, k)
		}
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		out = append(out, PlayerChange{Block: block, Page: page,
			Held: "only sheet fields can change here, so this can't set " + strings.Join(extra, ", ")})
	}
	if strings.TrimSpace(p.Body) != "" {
		out = append(out, PlayerChange{Block: block, Page: page,
			Held: "only the sheet can change here, not the page's text; write a backstory as the backstory field when the sheet has one"})
	}
	return out
}

// playerRefusal turns a refused change into the player's words. An
// unexpected failure is not described, so nothing internal leaks.
func playerRefusal(err error) string {
	var ae *apperror.AppError
	if errors.As(err, &ae) && ae.Code < 500 {
		return strings.TrimRight(ae.Message, ".")
	}
	return "Chronicle couldn't check this change just now"
}

// changeLines lists the fields a change would alter, in key order, from
// the sheet's current values, and the part of fields that does so.
func changeLines(s *PlayerSheet, fields map[string]any) ([]PlayerChangeLine, map[string]any) {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]PlayerChangeLine, 0, len(keys))
	changed := map[string]any{}
	for _, k := range keys {
		to := valueText(fields[k])
		if to == s.Values[k] {
			continue
		}
		out = append(out, PlayerChangeLine{Key: k, From: s.Values[k], To: to})
		changed[k] = fields[k]
	}
	return out, changed
}

// valueText is a field value as the review shows it and as the sheet's
// current value is read.
func valueText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	default:
		return strings.TrimSpace(fmt.Sprint(x))
	}
}

// playerCounts is Chronicle's own line about a player's request.
func playerCounts(changes []PlayerChange) string {
	ok, held := 0, 0
	for _, c := range changes {
		if c.Held == "" {
			ok++
		} else {
			held++
		}
	}
	switch {
	case held == 0:
		return fmt.Sprintf("%d to change", ok)
	case ok == 0:
		return fmt.Sprintf("%d held back", held)
	}
	return fmt.Sprintf("%d to change, %d held back", ok, held)
}

// playerInstructions is how_to_write_changes for a player's AI, and the
// text a player copies for an AI they paste into: their characters, what
// each may change with the rule the server applies, the rest of each sheet
// to recommend from, and the one block format that can change it.
func playerInstructions(campaignName, gmTerm string, sheets []PlayerSheet) string {
	if strings.TrimSpace(gmTerm) == "" {
		gmTerm = "GM"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Changing your character in %s\n\n", oneLineText(campaignName, 120))
	b.WriteString("You are helping a player in a tabletop roleplaying campaign kept in Chronicle. ")
	b.WriteString("You can change only the fields listed below on that player's own character, each by the rule given with it. ")
	fmt.Fprintf(&b, "Everything else (other pages, the page's text, its picture, the numbers the %s keeps) is held back if you ask for it. ", gmTerm)
	b.WriteString("Nothing changes until the player says yes to each change in Chronicle.\n\n")
	b.WriteString("## How to write a change\n\nOne block per character, like this:\n\n")
	b.WriteString("```\n---\naction: update\nname: <the character's name, exactly>\nreason: <one line saying why>\nfields:\n  <key>: <new value>\n---\n```\n\n")
	b.WriteString("Write only the keys you change. Every value is text; a list is written as JSON text, like `'[\"Sneak\", \"Lie\"]'`. Write nothing after the closing `---`.\n\n")
	if len(sheets) == 0 {
		b.WriteString("## Your characters\n\nThe player hasn't claimed a character in this campaign yet, so there is nothing you can change.\n")
		return b.String()
	}
	b.WriteString("## Your characters\n")
	for _, s := range sheets {
		fmt.Fprintf(&b, "\n### %s", s.Name)
		if s.Type != "" {
			fmt.Fprintf(&b, " (%s)", s.Type)
		}
		b.WriteString("\n\n")
		steps := make([]string, 0, len(s.Picks))
		for k := range s.Picks {
			steps = append(steps, k)
		}
		sort.Strings(steps)
		if len(steps) > 0 {
			b.WriteString("Choices (one of the names listed):\n")
			for _, k := range steps {
				own := ""
				if s.Own[k] {
					own = ", or a short name of the player's own"
				}
				if fixed := s.Fixed[k]; fixed != "" {
					fmt.Fprintf(&b, "- %s: %s. It is fixed for everyone in this campaign: it must be %s, so leave it alone.\n", k, currentText(s.Values[k]), fixed)
					continue
				}
				extra := ""
				if s.Required[k] {
					extra += " Your Director set this short list, so the field can't be emptied."
				}
				if ask := s.Ask[k]; len(ask) > 0 {
					extra += fmt.Sprintf(" Ask your Director first for: %s. Don't write those yourself; tell the player to ask their Director.", strings.Join(ask, ", "))
				}
				fmt.Fprintf(&b, "- %s: %s. Choose from: %s%s.%s\n", k, currentText(s.Values[k]), strings.Join(s.Picks[k], ", "), own, extra)
			}
		}
		if len(s.Free) > 0 {
			b.WriteString("Short text (up to 200 characters):\n")
			for _, k := range s.Free {
				fmt.Fprintf(&b, "- %s: %s\n", k, currentText(s.Values[k]))
			}
		}
		if len(s.Story) > 0 {
			b.WriteString("The player's own words (up to 20000 characters; write what the player tells you, and suggest, never decide, their story):\n")
			for _, k := range s.Story {
				fmt.Fprintf(&b, "- %s: %s\n", k, currentText(s.Values[k]))
			}
		}
		if len(s.Lists) > 0 {
			b.WriteString("Lists of names (a JSON list, up to 40 names of up to 64 characters each; write the whole list):\n")
			for _, k := range s.Lists {
				fmt.Fprintf(&b, "- %s: %s\n", k, currentText(s.Values[k]))
			}
		}
		buys := make([]string, 0, len(s.Buys))
		for k := range s.Buys {
			buys = append(buys, k)
		}
		sort.Strings(buys)
		for _, k := range buys {
			by := s.Buys[k]
			items := make([]string, 0, len(by.Names))
			for _, n := range by.Names {
				items = append(items, fmt.Sprintf("%s (%g)", n, by.Costs[n]))
			}
			fmt.Fprintf(&b, "Bought from %s (a JSON list of names; their costs may add up to %g points):\n- %s: %s. Choose from: %s\n",
				by.Entry, by.Budget, k, currentText(s.Values[k]), strings.Join(items, ", "))
		}
		if len(s.Shown) > 0 {
			keys := make([]string, 0, len(s.Shown))
			for k := range s.Shown {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			b.WriteString("The rest of the sheet, to read and recommend from (you can't change these):\n")
			for _, k := range keys {
				fmt.Fprintf(&b, "- %s: %s\n", k, oneLineText(s.Shown[k], 200))
			}
		}
	}
	return b.String()
}

func currentText(v string) string {
	if strings.TrimSpace(v) == "" {
		return "(empty)"
	}
	return "now " + oneLineText(v, 200)
}

// oneLineText collapses s onto one line and cuts it to n characters.
func oneLineText(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}
