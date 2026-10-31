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

// PlayerSheet is one character a player claimed, as their AI sees it.
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
	// value must be one of them.
	Picks map[string][]string
}

// PlayerSheets is the door a player's AI changes go through. internal/app
// implements it over the entities plugin and the hero builder.
type PlayerSheets interface {
	// Sheets lists the characters userID claimed in the campaign and can
	// still open.
	Sheets(ctx context.Context, campaignID, userID string) ([]PlayerSheet, error)
	// PatchSheet checks patch against what userID may change on their own
	// character entityID, and writes it when apply is true. label names
	// the AI app, for the page's history. The error says, in the player's
	// terms, why a change isn't allowed.
	PatchSheet(ctx context.Context, campaignID, userID, entityID string, patch map[string]any, label string, apply bool) error
}

// SetPlayerSheets wires the door for players' AI changes. Unwired, a
// player's AI can look things up but every change is held back.
func (h *Handler) SetPlayerSheets(s PlayerSheets) { h.playerSheets = s }

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

// planPlayerChanges reads a player's blocks. Each kept block has passed the
// sheet's own checks without writing; the caller applies the ones chosen,
// and PatchSheet checks each again then.
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
		if ch.Held == "" {
			if err := sheets.PatchSheet(ctx, campaignID, userID, sheet.ID, p.FrontMatter.Fields, "", false); err != nil {
				ch.Held = playerRefusal(err)
			}
		}
		if ch.Held == "" {
			ch.SheetID, ch.Page, ch.Fields = sheet.ID, sheet.Name, p.FrontMatter.Fields
			ch.Lines = changeLines(sheet, p.FrontMatter.Fields)
		}
		out = append(out, ch)
	}
	return out, nil
}

// holdPlayerBlock is why a block can't be a change to a player's own sheet,
// judged from the block alone; "" when it can be.
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
	var extra []string
	for k := range p.Fields {
		if !playerBlockKeys[strings.ToLower(k)] {
			extra = append(extra, k)
		}
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		return "only sheet fields can change here, so this can't set " + strings.Join(extra, ", ")
	}
	if strings.TrimSpace(p.Body) != "" {
		return "only the sheet can change here, not the page's text"
	}
	if len(p.FrontMatter.Fields) == 0 {
		return "it changes no sheet fields"
	}
	return ""
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

// changeLines lists what a change sets, in key order, from the sheet's
// current values.
func changeLines(s *PlayerSheet, fields map[string]any) []PlayerChangeLine {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]PlayerChangeLine, 0, len(keys))
	for _, k := range keys {
		to := ""
		if v, ok := fields[k].(string); ok {
			to = v
		}
		out = append(out, PlayerChangeLine{Key: k, From: s.Values[k], To: to})
	}
	return out
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
// each may change, and the one block format that can change it.
func playerInstructions(campaignName string, sheets []PlayerSheet) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Changing your character in %s\n\n", oneLineText(campaignName, 120))
	b.WriteString("You are helping a player in a tabletop roleplaying campaign kept in Chronicle. ")
	b.WriteString("You can change only that player's own character's sheet: the identity fields, which take short text, and the hero builder's choices, which take one name from the list given. ")
	b.WriteString("Everything else (other pages, the page's text, its picture, numbers the Director keeps) is held back if you ask for it. ")
	b.WriteString("Nothing changes until the player says yes to each change in Chronicle.\n\n")
	b.WriteString("## How to write a change\n\nOne block per character, like this:\n\n")
	b.WriteString("```\n---\naction: update\nname: <the character's name, exactly>\nreason: <one line saying why>\nfields:\n  <key>: <new value>\n---\n```\n\n")
	b.WriteString("Write only the keys you change. Every value is text. Write nothing after the closing `---`.\n\n")
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
		if len(s.Free) > 0 {
			b.WriteString("Short text:\n")
			for _, k := range s.Free {
				fmt.Fprintf(&b, "- %s: %s\n", k, currentText(s.Values[k]))
			}
		}
		steps := make([]string, 0, len(s.Picks))
		for k := range s.Picks {
			steps = append(steps, k)
		}
		sort.Strings(steps)
		if len(steps) > 0 {
			b.WriteString("Hero builder choices (one of the names listed):\n")
			for _, k := range steps {
				fmt.Fprintf(&b, "- %s: %s. Choose from: %s\n", k, currentText(s.Values[k]), strings.Join(s.Picks[k], ", "))
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
