package records

import (
	"context"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// LayoutsLookup answers `what: layouts`: each category's page layouts as the
// owner has them, written as the `kind: layout` blocks that send them back
// changed, with the blocks the campaign can place. It is a plugged lookup so
// the connector gets its tool from LookupInfo; later arrangements that live
// beside page layouts (category homes, presets, starter text) can join this
// answer or sit next to it. Owner only, as writing a layout is: the layouts
// name Director-only blocks, and a co-DM may not change them.
type LayoutsLookup struct{ Svc LayoutsAPI }

func (LayoutsLookup) LookupInfo() aiplug.LookupInfo {
	return aiplug.LookupInfo{
		What: "layouts", Aliases: []string{"layout", "page-layouts", "page-layout", "templates"},
		Keys:  "optional `category` (its name) and `layout` (a layout's name)",
		Brief: "the page layout each category's pages wear, written as `layout` blocks to send back changed: its rows, columns and blocks, which blocks only the DM team sees, the words in Text blocks, and what each block the campaign can place is (owner only)",
		Tool: aiplug.Tool{
			Name: "page_layouts", Title: "Page layouts", Did: "Looked at the page layouts", Owner: true,
			Desc: "The page layout each category's pages wear: rows, columns and blocks, which pages use which layout, and a block to send back changed. Also lists the blocks a layout can place, and what each one is.",
			Args: []aiplug.Arg{
				{Key: "category", Desc: "A category's name, to answer for that category only."},
				{Key: "layout", Desc: "A layout's name within the category, to answer for that layout only."},
			},
		},
	}
}

func (l LayoutsLookup) Answer(ctx context.Context, campaignID string, a Actor, r Record) (string, string, error) {
	chip := "Page layouts"
	k := LayoutKind(l)
	if k.Svc == nil {
		return chip, "", badRequestf("page layouts can't be looked up on this server")
	}
	if err := k.owner(ctx, campaignID, a); err != nil {
		return chip, "", badRequestf("page layouts are the campaign owner's, and are left out in Safe privacy")
	}
	types, err := k.Svc.Categories(ctx, campaignID)
	if err != nil {
		return chip, "", err
	}
	var only *entities.EntityType
	if c := strings.TrimSpace(r.Str("category")); c != "" {
		if only = typeNamed(types, c); only == nil {
			return chip, "", badRequestf("there is no category called %s", quote(c))
		}
	}
	wantLayout := strings.TrimSpace(r.Str("layout"))
	if wantLayout != "" && only == nil {
		return chip, "", badRequestf("name the `category` that holds the layout %s", quote(wantLayout))
	}

	var b strings.Builder
	b.WriteString("## Page layouts\n\nThe layout a category's pages wear, as it is now. Send a `kind: layout` block back, changed, to rearrange it: `create` and `update` replace the whole arrangement (list every block to keep; one left out is removed), a block that is listed again keeps its settings, and `directors_only: true` hides a block from players. A `create` with a layout name the category does not have adds a new layout. Tile sections are arranged only in Customize; write `- tiles: kept` to keep one. To remove a layout, use Customize › Categories.\n")
	shown, cats := 0, 0
	for i := range types {
		et := &types[i]
		if only != nil && et.ID != only.ID {
			continue
		}
		cats++
		pl, named := k.Svc.Named(ctx, et)
		fmt.Fprintf(&b, "\n### %s\n\n", et.NamePlural)
		b.WriteString(layoutDefaults(et, pl, named))
		type one struct {
			key, name, label string
			rows             []gridRow
		}
		layouts := []one{{entities.MainLayoutKey, "", pl.MainLabel(et), gridFromLayout(et.Layout)}}
		for _, x := range pl.Extra {
			layouts = append(layouts, one{x.Key, x.Name, x.Name, gridFromLayout(x.Layout)})
		}
		wantKey, _ := pl.KeyByName(et, wantLayout)
		for i, ly := range layouts {
			if wantLayout != "" && ly.key != wantKey {
				continue
			}
			shown++
			head := ly.label + " layout"
			if i == 0 {
				head += " (main)"
			}
			if len(ly.rows) == 0 {
				fmt.Fprintf(&b, "**%s**: no blocks yet. A `layout` block with `rows` fills it.\n\n", head)
				continue
			}
			var blk strings.Builder
			k.layoutRecord(&blk, et.Name, ly.name, ly.rows)
			fmt.Fprintf(&b, "**%s**: %s.\n\n```\n%s\n```\n\n", head, gridSummary(ly.rows), strings.TrimRight(blk.String(), "\n"))
			for _, row := range ly.rows {
				if row.IsTiles {
					fmt.Fprintf(&b, "Its tile section holds %s; it is arranged in Customize and written back as `tiles: kept`.\n\n", strings.Join(tileTypes(row), ", "))
				}
			}
		}
	}
	if wantLayout != "" && shown == 0 {
		return chip, "", badRequestf("%s has no layout called %s", only.Name, quote(wantLayout))
	}

	if palette := k.Svc.BlockTypes(ctx, campaignID); len(palette) > 0 {
		b.WriteString("\n### Blocks a layout can place\n\n")
		for _, p := range palette {
			line := "- `" + p.Type + "`"
			if p.Label != "" {
				line += " — " + p.Label
			}
			if p.Description != "" {
				line += ": " + aiplug.OneLine(p.Description, 200)
			}
			b.WriteString(line + "\n")
		}
	}
	if only != nil {
		chip = "Page layouts · " + only.NamePlural
	} else {
		chip = fmt.Sprintf("Page layouts · %d categories", cats)
	}
	return chip, b.String(), nil
}

// layoutDefaults says which layout a category's pages wear by default and
// which a player's character starts on.
func layoutDefaults(et *entities.EntityType, pl entities.PageLayouts, named bool) string {
	main := pl.MainLabel(et)
	s := "Pages wear the " + main + " layout (the main one) unless the Director picks another for a page."
	if c, ok := pl.Find(pl.Claimed); ok && pl.Claimed != "" {
		s += " A player's character starts on the " + c.Name + " layout."
	}
	if !named {
		s += " This campaign keeps one layout per category for now."
	}
	return s + "\n\n"
}
