package records

import (
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// Who may see a page, marked for a Director's AI. A key reading as the owner
// gets GM-only pages and secret text in the same answers as everything else;
// without a mark it can't tell what must not be repeated to players. Only a
// Director's answers carry marks: anyone else never reads what they mark.

// Marks written after a page's name, and after a kind of page whose category
// is for the DM team only. They are words, not part of the name.
const (
	markGMOnly = " (GM only)"
	markSome   = " (shared with some)"
)

// sightLegend explains the marks wherever a list carries one, so an AI
// neither repeats a marked page to players nor writes a mark into a name.
const sightLegend = "(GM only) marks a page or kind of page players never see, and (shared with some) a page only the people it is shared with see. " +
	"Neither is part of the name. Don't repeat what they hold to players.\n"

// pageSightMark is the mark for a page's own visibility setting. A page of a
// DM-only category is marked through its group (kindSightMark) instead, so a
// list of adventures doesn't repeat the mark on every line.
func pageSightMark(e *entities.Entity) string {
	switch {
	case e.TypeDMOnly:
		return ""
	case e.Visibility == entities.VisibilityCustom:
		return markSome
	case e.IsPrivate:
		return markGMOnly
	}
	return ""
}

// kindSightMark is the mark for a group of pages whose category is for the
// DM team only.
func kindSightMark(e *entities.Entity) string {
	if e.TypeDMOnly {
		return markGMOnly
	}
	return ""
}

// restrictedFieldKeys are the page's filled sheet fields players can't read:
// GM-only ones, and owner-only ones only the claiming player and the GM see.
func restrictedFieldKeys(e *entities.Entity, types []entities.EntityType) (gm, own []string) {
	var defs []entities.FieldDefinition
	for _, t := range types {
		if t.ID == e.EntityTypeID {
			defs = t.Fields
		}
	}
	for _, d := range entities.RestrictionDefs(defs, e.FieldOverrides) {
		v, ok := e.FieldsData[d.Key]
		if !ok || v == nil || v == "" {
			continue
		}
		switch {
		case d.GMOnly:
			gm = append(gm, "`"+d.Key+"`")
		case d.OwnerOnly:
			own = append(own, "`"+d.Key+"`")
		}
	}
	return gm, own
}

// pageSecretsNote is the line a Director's read of one page opens with,
// naming what on it players never see; "" when nothing is hidden. md is the
// page as already written, whose {gm{…}gm} markers are the GM-only passages.
func pageSecretsNote(e *entities.Entity, types []entities.EntityType, md string) string {
	var parts []string
	switch {
	case e.TypeDMOnly:
		parts = append(parts, "players never see this page: its category is for the DM team only.")
	case e.Visibility == entities.VisibilityCustom:
		parts = append(parts, "only the people it is shared with see this page.")
	case e.IsPrivate:
		parts = append(parts, "players never see this page.")
	}
	if strings.Contains(md, "{gm{") {
		parts = append(parts, "Text between {gm{ and }gm} is hidden from players.")
	}
	gm, own := restrictedFieldKeys(e, types)
	if len(gm) > 0 {
		parts = append(parts, "Sheet fields hidden from players: "+strings.Join(gm, ", ")+".")
	}
	if len(own) > 0 {
		parts = append(parts, "Sheet fields only its player and the GM see: "+strings.Join(own, ", ")+".")
	}
	if len(parts) == 0 {
		return ""
	}
	return "**For the GM only:** " + strings.Join(parts, " ") + " Don't repeat these to players."
}
