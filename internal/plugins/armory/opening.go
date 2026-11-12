package armory

import (
	"context"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// armoryFiltered is true when the viewer narrowed the list, so an empty
// result means "nothing matched", not "nothing here yet".
func armoryFiltered(opts ItemListOptions) bool {
	return opts.Search != "" || opts.Tag != "" || opts.TypeID > 0 || opts.InstanceID > 0
}

// armoryOpening is the Armory's opening. Items are pages of an item type, so
// the page offers no create action of its own; the sentences say where they
// come from. A chosen collection is named as the title.
func armoryOpening(ctx context.Context, total int, opts ItemListOptions, selected *InventoryInstance) layouts.AppOpeningData {
	name := "Armory"
	if selected != nil {
		name = selected.Name
	}
	return layouts.AppOpeningData{
		Name:  name,
		Empty: total == 0 && !armoryFiltered(opts),
		Intros: layouts.AppIntros{
			Player: fmt.Sprintf("Items your %s shares will show here.", layouts.GMTerm(ctx)),
			CoDM:   "Items the owner adds will show here.",
			Owner:  "Items come from the item types in your campaign. Add item pages and they will show here.",
		},
	}
}

// armoryCount is the count beside the title.
func armoryCount(n int) string {
	if n == 1 {
		return "1 item"
	}
	return fmt.Sprintf("%d items", n)
}
