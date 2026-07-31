// bench_sections.go — the Bench's disclosure REGISTRY and its resolution, in
// one place (C-CALV4-BENCH-R2 slice R2-1, [BR2-3] and [BR2-5] SIGNED).
//
// It is deliberately the sibling of block_layers.go and should be read beside
// it: same seed-vs-store shape, same nil-vs-empty discipline, same
// reject-on-write / filter-on-read validation split. One discipline, learned
// once, in two places that both needed it.
//
// WHAT A DISCLOSURE IS HERE. Four sections of the Bench collapse — the ribbon
// (whole, not per tile), the RSVP panel, the NEXT UP index and the subordinate
// row grid. They are native <details>/<summary>, so they open and close with no
// JavaScript at all; only the REMEMBERING travels over the wire, and it travels
// as one optional field on a route that already existed.
//
// WHAT NEVER COLLAPSES, and this file is the registry that makes it structural
// rather than a convention: the two Blocks (.stack), the page head (.phead),
// the "The bench" label (.sechead) and the closing .caption. [BR2-3 SIGNED].
// The subject of the page may not be inside a disclosure — "5-6 blocks of data
// before you get to the calendar" is not answered by a calendar you have to
// open.
package calendar

import (
	"context"
	"log/slog"
	"slices"
)

// benchSectionKeys is the CLOSED registry of collapsible Bench sections, IN THE
// PAGE'S CONTRACT ORDER (benchSurface's child order, bench.templ).
//
// The order is load-bearing in exactly one place: SetBenchSections writes the
// stored list in this order rather than in click order, so two viewers in the
// same state store the same bytes and the column stays greppable in an
// incident. It is otherwise presentation-neutral.
//
// A fifth key is a product decision, not a typo — see [BR2-3]'s refusal of the
// per-tile ribbon grain, which would have made this six keys longer for a
// preference nobody expressed. That grain books as C-CALV4-RIBBON-R2b and is
// cheap then, because this registry and its store already exist.
var benchSectionKeys = []string{"ribbon", "rsvp", "nextup", "rows"}

// resolveBenchSections turns the STORED CLOSED SET into the per-key closed
// answer the template renders.
//
// THE DEFAULT IS CLOSED, AT EVERY WIDTH ([BR2-4 SIGNED], Option A), and the nil
// branch below is where that lives:
//
//	stored == nil        never chosen  → all four CLOSED
//	stored == []string{} closed nothing → all four OPEN
//	stored == [rsvp rows] → those two closed, the other two open
//
// Why closed everywhere rather than "desktop open, mobile closed", which is
// what the operator literally asked for: a server-rendered attribute cannot
// vary by viewport (ADR-048 §13, first met over `checked` and met a second time
// here over `open`), and a per-viewer store does not beat that bound — it
// changes where the single value comes from, not how many values one attribute
// can be. The honest ways out were one default, or a script that mutates `open`
// after paint. The script is refused: htmx.config.allowEval is false
// (static/js/boot.js:168), so a scripted default would also force the WRITE to
// carry an explicit state via hx-vals='js:…', which is dead there. The mobile
// half of the complaint is answered instead by CSS `order` lifting the calendar
// above the ribbon at ≤640px, which needs no state at all.
//
// An unknown stored key resolves to nothing and takes nothing down with it: a
// registry that SHRANK after a write is a deploy, not a caller, and bricking a
// viewer's Bench over the product's own history would be the wrong failure.
func resolveBenchSections(stored []string) map[string]bool {
	closed := make(map[string]bool, len(benchSectionKeys))
	if stored == nil {
		for _, k := range benchSectionKeys {
			closed[k] = true
		}
		return closed
	}
	for _, k := range stored {
		if slices.Contains(benchSectionKeys, k) {
			closed[k] = true
		}
	}
	return closed
}

// benchSectionPrefs is what a producer needs from the request to render the
// disclosures, carried as one value so a caller cannot supply half of it — the
// same construction blockLayerPrefs uses, for the same reason.
//
// Stored is the viewer's own CLOSED set, or nil when they have never chosen.
// PersistURL is empty when there is nowhere to persist: an anonymous viewer, or
// a Bench composed outside a campaign. A disclosure with an empty PersistURL
// emits NO hx-* at all rather than a dead control — it still opens and closes
// natively, it simply forgets, and that is the correct degradation.
type benchSectionPrefs struct {
	Stored     []string
	PersistURL string
}

// benchSectionsPath is the campaign-scoped endpoint a <details> posts its flip
// to. It is the SAME endpoint the Block's switchboard already posts to — the
// route grows one optional field rather than a sibling ([BR2-5] SIGNED), which
// is why internal/wire/routes_snapshot.txt is byte-identical after this slice.
func benchSectionsPath(campaignID string) string {
	return blockPrefsPath(campaignID)
}

// benchSectionPrefsFor reads the viewer's stored closed set and builds the
// endpoint, ONCE per page.
//
// A read failure is NOT fatal and NOT a fallback to "no disclosures": the
// preference is display state, and a degraded read should give the viewer the
// ruled default with working controls rather than a page of dead chevrons. Same
// shape as blockLayerPrefsFor and the sync pill's degraded probe, for the same
// reason — a degraded read must never blank a surface.
func benchSectionPrefsFor(ctx context.Context, svc CalendarService, userID, campaignID string) benchSectionPrefs {
	if svc == nil || userID == "" || campaignID == "" {
		// Nowhere to persist: the ruled default renders (nil → all four closed)
		// and no hx-* is emitted.
		return benchSectionPrefs{}
	}
	p := benchSectionPrefs{PersistURL: benchSectionsPath(campaignID)}
	if stored, err := svc.GetBenchSections(ctx, userID, campaignID); err == nil {
		p.Stored = stored
	} else {
		slog.Warn("calendar: Bench disclosure preference read failed; falling back to the ruled default",
			slog.String("user_id", userID),
			slog.String("campaign_id", campaignID),
			slog.Any("error", err))
	}
	return p
}
