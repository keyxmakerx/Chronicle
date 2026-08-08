// entity_ledger_geometry_test.go — §9(d)'s MEASUREMENT, in executable form
// (C-CALV4-BENCH-R2 slice R2-1, [BR2-8] SIGNED).
//
// THE CLAIM UNDER TEST. entity_calendar_block.go carried a warning, for three
// waves, that dropping the `ledger` key from the entity host's seed would break
// geometry: "the full-tier column arithmetic subtracts the Ledger's 300px
// unconditionally (sizing.go), so an entity Block that skipped the zone would
// measure its own columns wrong and flip density at the wrong host width."
//
// [BR2-8] SIGNED requires that warning to be MEASURED and then rewritten, and
// declares a measurement that CONTRADICTS the scout a STOP-AND-FLAG. This file
// is the measurement. It confirms the scout on three independent legs, and it
// stays in the tree so the finding cannot silently rot back:
//
//  1. sizing.go's ColWidth / IsNamed / IsNamedCSS have ZERO non-test callers
//     anywhere under internal/. Nothing in the render path ever evaluates the
//     arithmetic the warning is about.
//  2. The shipped full-tier body grid is `minmax(0, 1fr) auto`, so an ABSENT
//     Ledger collapses its own track rather than leaving a 300px hole.
//  3. An entity Block rendered WITHOUT `ledger` emits no Ledger DOM at all —
//     no placeholder, no reserved box, nothing to occupy the auto track.
//
// The three together are the whole of the warning's premise, and all three
// falsify it. The comment at entity_calendar_block.go was rewritten to what is
// actually true, and this file is why that rewrite is a finding rather than an
// opinion.
package calendar

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	calblock "github.com/keyxmakerx/chronicle/internal/widgets/calendar_block"
)

// entityFiveKeyStub renders the WAVE-1 five-key set through the store rather
// than through the seed: [LYR-3]'s stored set wins at every host, so this is the
// honest way to obtain the "before" render without editing the producer.
type entityFiveKeyStub struct{ *entityCalBlockStub }

func (s *entityFiveKeyStub) GetBlockLayers(context.Context, string, string) ([]string, error) {
	return []string{"moons", "eras", "weeknums", "ledger", "shelf"}, nil
}

func renderEntityCalLayers(t *testing.T, svc *entityCalBlockStub) string {
	t.Helper()
	return renderEntityCal(t, &entityFiveKeyStub{svc}, campaigns.RoleOwner, false)
}

// repoRootFrom resolves the repository root from this test file's own location,
// the same way readRepoFile does.
func repoRootFrom(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
}

// LEG 1 — the arithmetic has no caller in the render path.
//
// sizing.go says so about itself at :143-151 ("IsNamedCSS is what the shipped
// stylesheet actually implements … Kept beside IsNamed so the two can be diffed
// by a test rather than by eye"), but a comment is not a measurement. This
// walks every non-test .go file under internal/ and proves it.
func TestLedgerGeometry_TheColumnArithmeticHasNoRenderPathCaller(t *testing.T) {
	root := repoRootFrom(t)
	callRe := regexp.MustCompile(`\b(ColWidth|IsNamed|IsNamedCSS)\s*\(`)
	var callers []string

	err := filepath.Walk(filepath.Join(root, "internal"), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		// sizing.go DEFINES them and IsNamed calls ColWidth; that is the
		// definition site, not the render path.
		if strings.HasSuffix(path, "widgets/calendar_block/sizing.go") {
			return nil
		}
		body, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		if callRe.Match(body) {
			rel, _ := filepath.Rel(root, path)
			callers = append(callers, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal/: %v", err)
	}
	if len(callers) != 0 {
		t.Fatalf("STOP AND FLAG — the full-tier column arithmetic now HAS render-path callers %v. "+
			"§9(d)'s finding is falsified and the entity seed decision must be re-taken before it ships.", callers)
	}
}

// LEG 2 — the shipped full-tier grid collapses an absent Ledger's track.
//
// The density decision is made by a CONTAINER QUERY against real layout
// (`@container cal-cell (min-width: 84px)`), not by the Go arithmetic, and the
// body's track set is `auto` on the Ledger side. An `auto` track with no item
// in it resolves to zero — there is no 300px hole to leave.
func TestLedgerGeometry_FullTierTrackIsAutoNotAFixedThreeHundred(t *testing.T) {
	css := readRepoFile(t, "static/css/calendar-block.css")
	if !strings.Contains(css, "@container cal-block (min-width: 900px)") {
		t.Fatal("the full-tier container query moved; re-measure before trusting anything below")
	}
	if !strings.Contains(css, "grid-template-columns: minmax(0, 1fr) auto") {
		t.Error("STOP AND FLAG — the full-tier body grid is no longer `minmax(0, 1fr) auto`. " +
			"If the Ledger's side became a FIXED track, an absent Ledger would leave a real hole " +
			"and §9(d)'s rewritten comment would be false.")
	}
	if !strings.Contains(css, "@container cal-cell (min-width: 84px)") {
		t.Error("the density flip is no longer a cal-cell container query; the Go arithmetic " +
			"would then matter again and the seed decision must be re-taken")
	}
}

// LEG 3 — a Block rendered without `ledger` emits no Ledger DOM at all.
//
// This is the direct observation: render the entity host's Block with the seed
// this slice ships and with the wave-1 five-key seed, and compare. The
// "without" render must contain no ledger zone and no shelf zone, and must
// still contain everything the three kept keys draw.
func TestLedgerGeometry_WithoutLedgerThereIsNoLedgerDOMToLeaveAVoid(t *testing.T) {
	svc := entityHostSpine(t, blockTenDayCal())
	with := renderEntityCalLayers(t, svc)
	without := renderEntityCal(t, svc, campaigns.RoleOwner, false)

	// SCOPED TO THE EMBED'S BLOCK, AND PAIRED WITH THE THEATER'S — C-CALV4-THEATER
	// (R2-3). The entity page now carries TWO Blocks for one calendar, so a
	// whole-page `strings.Contains` can no longer answer "does the EMBED render a
	// Ledger": the theater's copy is seeded with the Bench's five keys and
	// legitimately renders both zones inside a closed <dialog>. Narrowing the
	// negative on its own would be a guard that stopped looking at half the page,
	// so it is paired with the positive — absent HERE, present THERE — which is a
	// strictly stronger claim than the single negative it replaces, and it is the
	// booking [BR2-8] made turned into something a test checks.
	withEmbed := entityEmbedSubtree(t, with)
	withoutEmbed := entityEmbedSubtree(t, without)
	withoutTheater := entityTheaterSubtree(t, without)
	for _, zone := range []string{`data-zone="ledger"`, `data-zone="shelf"`} {
		if !strings.Contains(withEmbed, zone) {
			t.Fatalf("fixture invariant broken: the five-key seed did not render %s", zone)
		}
		if strings.Contains(withoutEmbed, zone) {
			t.Errorf("the three-key seed still renders %s", zone)
		}
		if !strings.Contains(withoutTheater, zone) {
			t.Errorf("the THEATER is missing %s — the depth R2-1 removed comes back there, "+
				"and a theater without the two zone-adding keys is a stretched glanceable "+
				"month rather than a full-tier Block", zone)
		}
	}
	// Nothing was left behind in its place: the "without" render is SHORTER,
	// which is what "the track collapsed" looks like from the DOM's side.
	if len(without) >= len(with) {
		t.Errorf("the three-key render is %d bytes against the five-key render's %d — "+
			"dropping two zones must remove markup, not substitute a placeholder for it",
			len(without), len(with))
	}
	// And the three KEPT keys are all still drawn, so the embed lost zones, not
	// fiction.
	// data-weeknums is the W1/W2/W3 gutter's own marker on the instrument; the
	// era bands and moon discs are drawn inside the month and have no zone of
	// their own, which is exactly WHY they were the keys kept.
	if !strings.Contains(without, "data-weeknums") {
		t.Error("the three-key seed dropped the W1/W2/W3 gutter; the keys kept are the ones INSIDE the month")
	}
	for _, layer := range []string{`data-layer="legend"`, `data-layer="horizon"`, `data-layer="moongraph"`} {
		if strings.Contains(without, layer) {
			t.Errorf("the three-key seed picked up %s; the seed SHRANK, it did not change shape", layer)
		}
	}
}

// THE ONE REAL BEHAVIOUR CHANGE, stated as arithmetic so the report can quote a
// number rather than an impression.
//
// Without the Ledger beside it the month's cells are WIDER at the same host
// width, so `@container cal-cell (min-width: 84px)` flips named columns ON at a
// NARROWER host. The entity month becomes richer, not poorer — a good outcome
// and an unexpected one.
//
// For the ten-day week the fixtures use, with the shipped constants
// (instPad 24, blockPad 16, weekGutter 18, ledgerDock 300, NamedColWidthMin 84):
//
//	with    ledger: host ≥ 24 + 300 + 16 + 18 + 10×84 = 1198px
//	without ledger: host ≥ 24 +   0 + 16 + 18 + 10×84 =  898px
//
// a 300px shift, which is exactly the Ledger's dock width. This test pins the
// constants the arithmetic rests on, so the report's numbers cannot go stale
// without something turning red.
func TestLedgerGeometry_NamedColumnsFlipMovesByExactlyTheDockWidth(t *testing.T) {
	const week = 10
	if got := calblock.NamedColWidthMin; got != 84 {
		t.Fatalf("NamedColWidthMin = %v, want 84 — the report's flip arithmetic is stated against it", got)
	}
	// ColWidth is the model of the arithmetic; it has no render-path caller
	// (leg 1) but it is the shipped statement of the constants, so it is the
	// honest place to read them from.
	flip := func(hostAdjust int) int {
		for host := 400; host <= 3000; host++ {
			// The no-dock case is the full-tier arithmetic with the 300px given
			// back: the body's `auto` track collapses, so the month gets exactly
			// the dock's width in addition. Modelling it as TierStd would be
			// WRONG — std also drops the instrument's own 24px padding, which an
			// entity Block at full tier still has.
			if calblock.ColWidth(calblock.TierFull, host+hostAdjust, week) >= calblock.NamedColWidthMin {
				return host
			}
		}
		return 0
	}
	withLedger := flip(0)
	withoutLedger := flip(300)
	if withLedger != 1198 {
		t.Errorf("with the Ledger the flip is at host %dpx; the report states 1198px", withLedger)
	}
	if withoutLedger != 898 {
		t.Errorf("without the Ledger the flip is at host %dpx; the report states 898px", withoutLedger)
	}
	if withLedger-withoutLedger != 300 {
		t.Errorf("the flip moved by %dpx; it must move by the dock's own 300px", withLedger-withoutLedger)
	}
}
