package campaigns

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// yourLookOpening is the Your look page's opening. It has no action: a pick
// is made on the cards below. With nothing open to the viewer the sentence
// says who decides, and offers nothing to do. (The menu link is hidden in the
// same case, so this is only reached by a typed address.)
func yourLookOpening(ctx context.Context, parts int) layouts.AppOpeningData {
	return layouts.AppOpeningData{
		Name:  "Your look",
		Empty: parts == 0,
		Intros: layouts.AppIntros{
			Player: "The owner chooses which parts of the look you can change for yourself. None are open to you yet.",
			CoDM:   "The owner chooses which parts of the look you can change for yourself. None are open to you yet.",
			Owner:  "The campaign's look is yours to set. What a member picks here changes only what they see.",
		},
	}
}
