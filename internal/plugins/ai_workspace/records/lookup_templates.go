package records

import (
	"context"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// otherParts names the parts of the answer that sit beside the page layouts,
// for the ones this server has, so an AI that asked for layouts learns the rest
// exist without receiving them all in one answer.
func (l LayoutsLookup) otherParts() string {
	var parts []string
	if l.Homes != nil {
		parts = append(parts, "`part: category-homes` (each category's own landing page, `category-home` blocks)")
	}
	if l.Saved != nil {
		parts = append(parts, "`part: saved-layouts` (the arrangements a category's layout starts from, `saved-layout` blocks)")
	}
	if l.Templates != nil {
		parts = append(parts, "`part: page-templates` (the starter text a new page begins with, `page-template` blocks)")
	}
	if len(parts) == 0 {
		return ""
	}
	return "\n### More templates\n\nAsk again with " + strings.Join(parts, ", ") + ".\n"
}

// categoryHomes answers `part: category-homes`: each category's landing page
// as a `kind: category-home` block, or what an unsaved one starts from.
func (l LayoutsLookup) categoryHomes(ctx context.Context, campaignID string, a Actor, r Record) (string, string, error) {
	chip := "Category homes"
	k := CategoryHomeKind{Svc: l.Homes}
	if k.Svc == nil {
		return chip, "", badRequestf("category homes can't be looked up on this server")
	}
	if err := k.owner(ctx, campaignID, a); err != nil {
		return chip, "", badRequestf("category homes are the campaign owner's, and are left out in Safe privacy")
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
	var b strings.Builder
	b.WriteString("## Category homes\n\nThe landing page each category's sidebar entry opens, as it is now. Send a `kind: category-home` block back, changed, to rearrange one or rewrite its Text blocks: `create` and `update` replace the whole arrangement (list every block to keep; one left out is removed) and a block that is listed again keeps its settings. `delete` takes a category's own home away so it shows Chronicle's built-in landing page again. Tile sections are arranged only in Customize; write `- tiles: kept` to keep one.\n")
	saved, cats := 0, 0
	for i := range types {
		et := &types[i]
		if only != nil && et.ID != only.ID {
			continue
		}
		cats++
		fmt.Fprintf(&b, "\n### %s\n\n", et.NamePlural)
		rows, own := categoryHomeRows(et)
		if own {
			saved++
		}
		var blk strings.Builder
		switch {
		case own && len(rows) == 0:
			b.WriteString("Saved with no blocks. A `category-home` block with `rows` fills it.\n")
		case own:
			k.homeRecord(&blk, et.Name, rows)
			fmt.Fprintf(&b, "Its own arrangement: %s.\n\n```\n%s\n```\n", gridSummary(rows), strings.TrimRight(blk.String(), "\n"))
			for _, row := range rows {
				if row.IsTiles {
					fmt.Fprintf(&b, "\nIts tile section holds %s; it is arranged in Customize and written back as `tiles: kept`.\n", strings.Join(tileTypes(row), ", "))
				}
			}
		default:
			start := gridFromDashboard(*entities.DefaultCategoryHomeLayout())
			k.homeRecord(&blk, et.Name, start)
			fmt.Fprintf(&b, "Not saved, so it shows Chronicle's built-in landing page. Customize starts from: %s. A `category-home` block for it gives it its own:\n\n```\n%s\n```\n", gridSummary(start), strings.TrimRight(blk.String(), "\n"))
		}
	}
	if palette := k.Svc.BlockTypes(ctx, campaignID); len(palette) > 0 {
		b.WriteString("\n### Blocks a category home can place\n\n")
		writePalette(&b, palette)
	}
	if only != nil {
		chip = "Category homes · " + only.NamePlural
	} else {
		chip = fmt.Sprintf("Category homes · %d of %d saved", saved, cats)
	}
	return chip, b.String(), nil
}

// writePalette lists the blocks a layout can place, one line each.
func writePalette(b *strings.Builder, palette []LayoutBlockInfo) {
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

// savedLayouts answers `part: saved-layouts`: each saved layout, the built-in
// ones named as read-only and the campaign's own as `kind: saved-layout`
// blocks.
func (l LayoutsLookup) savedLayouts(ctx context.Context, campaignID string, a Actor, r Record) (string, string, error) {
	chip := "Saved layouts"
	k := SavedLayoutKind{Svc: l.Saved}
	if k.Svc == nil {
		return chip, "", badRequestf("saved layouts can't be looked up on this server")
	}
	if err := k.owner(ctx, campaignID, a); err != nil {
		return chip, "", badRequestf("saved layouts are the campaign owner's, and are left out in Safe privacy")
	}
	all, err := k.Svc.List(ctx, campaignID)
	if err != nil {
		return chip, "", err
	}
	want := strings.TrimSpace(r.Str("name"))
	if want != "" && presetNamed(all, want) == nil {
		return chip, "", badRequestf("there is no saved layout called %s", quote(want))
	}
	var b strings.Builder
	b.WriteString("## Saved layouts\n\nThe arrangements an owner starts a category's page layout from. Send a `kind: saved-layout` block back, changed, to rearrange one, rename it or change its description or icon: `update` with `rows` replaces the whole arrangement (list every block to keep), and a key left out keeps what is saved. `create` makes a new one and `delete` removes one; a category that started from a saved layout keeps its own layout either way. The built-in ones can't be changed or removed: create a saved layout under a new name to start from one.\n")
	shown := 0
	for i := range all {
		p := &all[i]
		if want != "" && !sameName(p.Name, want) {
			continue
		}
		shown++
		rows, rerr := presetRows(p)
		fmt.Fprintf(&b, "\n### %s\n\n", p.Name)
		if p.Description != "" {
			b.WriteString(p.Description + "\n\n")
		}
		switch {
		case rerr != nil:
			b.WriteString("Its stored arrangement can't be read, so it is not written here.\n")
		case p.IsBuiltin:
			fmt.Fprintf(&b, "Built in, so it can't be changed or removed. It holds: %s.\n", gridSummary(rows))
		case len(rows) == 0:
			b.WriteString("It holds no blocks. A `saved-layout` block with `rows` fills it.\n")
		default:
			var blk strings.Builder
			k.savedLayoutRecord(&blk, *p, rows)
			fmt.Fprintf(&b, "%s.\n\n```\n%s\n```\n", gridSummary(rows), strings.TrimRight(blk.String(), "\n"))
		}
	}
	if len(all) == 0 {
		b.WriteString("\nThis campaign has no saved layouts. A `saved-layout` block with `name` and `rows` makes one.\n")
	}
	if palette := k.Svc.BlockTypes(ctx, campaignID); len(palette) > 0 {
		b.WriteString("\n### Blocks a layout can place\n\n")
		writePalette(&b, palette)
	}
	if want != "" {
		chip = "Saved layouts · " + want
	} else {
		chip = fmt.Sprintf("Saved layouts · %d", shown)
	}
	return chip, b.String(), nil
}

// pageTemplates answers `part: page-templates`: each template, the shared ones
// named as read-only and the campaign's own as `kind: page-template` blocks
// with their words.
func (l LayoutsLookup) pageTemplates(ctx context.Context, campaignID string, a Actor, r Record) (string, string, error) {
	chip := "Page templates"
	k := PageTemplateKind{Svc: l.Templates}
	if k.Svc == nil {
		return chip, "", badRequestf("page templates can't be looked up on this server")
	}
	if err := k.owner(ctx, campaignID, a); err != nil {
		return chip, "", badRequestf("page templates are the campaign owner's, and are left out in Safe privacy")
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
	all, err := k.Svc.List(ctx, campaignID)
	if err != nil {
		return chip, "", err
	}
	want := strings.TrimSpace(r.Str("name"))
	var b strings.Builder
	b.WriteString("## Page templates\n\nThe starter text a new page begins with. Send a `kind: page-template` block back, changed, to rewrite its words (the markdown after the block), rename it or change its description or icon: a key left out keeps what is saved. `create` makes a new one and `delete` removes one; pages already written from a template keep their text either way. A template's category is chosen when it is made and can't be changed. A template's grey writing prompts are not written in markdown: sending its text back unchanged keeps them, and changed text replaces them. The ones every campaign shares can't be changed.\n")
	shown := 0
	for i := range all {
		t := &all[i]
		if want != "" && !sameName(t.Name, want) {
			continue
		}
		if only != nil && templateCategory(t) != 0 && templateCategory(t) != only.ID {
			continue
		}
		shown++
		scope := "every category"
		for _, et := range types {
			if et.ID == templateCategory(t) {
				scope = et.NamePlural
			}
		}
		fmt.Fprintf(&b, "\n### %s\n\nFor %s.", t.Name, scope)
		if t.Description != "" {
			b.WriteString(" " + t.Description)
		}
		if t.IsGlobal {
			b.WriteString(" Shared by every campaign, so it can't be changed or removed.\n")
			continue
		}
		var blk strings.Builder
		k.templateRecord(&blk, *t, categoryName(types, templateCategory(t)))
		fmt.Fprintf(&b, "\n\n```\n%s\n```\n", strings.TrimRight(blk.String(), "\n"))
	}
	if shown == 0 {
		switch {
		case want != "":
			return chip, "", badRequestf("there is no page template called %s", quote(want))
		case only != nil:
			fmt.Fprintf(&b, "\n%s has no page templates of its own or for every category. A `page-template` block with `category` and words makes one.\n", only.NamePlural)
		default:
			b.WriteString("\nThis campaign has no page templates. A `page-template` block with a `name` and words makes one.\n")
		}
	}
	switch {
	case want != "":
		chip = "Page templates · " + want
	case only != nil:
		chip = "Page templates · " + only.NamePlural
	default:
		chip = fmt.Sprintf("Page templates · %d", shown)
	}
	return chip, b.String(), nil
}
