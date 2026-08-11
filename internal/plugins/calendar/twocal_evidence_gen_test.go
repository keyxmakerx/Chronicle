package calendar

// twocal_evidence_gen_test.go — SCRATCH EVIDENCE GENERATOR (delete after use).
//
// Renders THE SAME real-world calendar twice, through the real
// BlockService.Block with the Bench's own arguments (bench.go:1143 / 1258):
//
//	the real-world seat  →  ShelfHidden: true,  SkyOn: false   ([SKY-1] SIGNED)
//	the primary seat     →  ShelfHidden: false, SkyOn: true
//
// and writes each one as a standalone page for a browser to photograph. Inert
// unless TWOCAL_EVIDENCE names an output directory.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	calblock "github.com/keyxmakerx/chronicle/internal/widgets/calendar_block"
)

var twoCalLinkRe = regexp.MustCompile(`<link[^>]*>`)

// twoCalSeatBlock reproduces benchBlock's body exactly for one seat.
func twoCalSeatBlock(t *testing.T, spine *BlockService, cal *Calendar,
	viewer BlockViewer, activeID string, noShelf, sky bool) calblock.BlockData {
	t.Helper()
	var prefs blockLayerPrefs // a viewer who has never opened the switchboard
	d, err := spine.Block(context.Background(), BlockRequest{
		CalendarID:  cal.ID,
		CampaignID:  cal.CampaignID,
		Viewer:      viewer,
		View:        BlockDate{},
		IsActive:    cal.ID == activeID,
		ShelfHidden: noShelf,
		SkyOn:       sky,
		MoonCap:     benchMoonCap,
		LayerPrefs:  prefs,
	})
	if err != nil {
		t.Fatalf("Block: %v", err)
	}
	d.Layers = benchBlockLayers(prefs)
	return d
}

func twoCalPage(t *testing.T, title string, d calblock.BlockData, hostPx int) string {
	t.Helper()
	var sb strings.Builder
	if err := calblock.Block(d).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render block: %v", err)
	}
	markup := twoCalLinkRe.ReplaceAllString(sb.String(), "")

	css := benchBlockSheet(t) + twoCalBenchSheet(t)

	// The app shell, reproduced: <main class="px-3"> at a phone width, the
	// Bench's own .cal-bench box, its .bsurf surface and the .stack/.benchblock
	// wrappers the Primary and real-world Blocks actually sit in (bench.templ
	// :101, :178, :471). [MOB-10]: a camera pointed at a page production does
	// not render is worth nothing.
	return `<!doctype html><html lang="en"><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width,initial-scale=1">` +
		`<title>` + title + `</title><style>` +
		`html,body{margin:0;padding:0}` +
		`body{background:#f9fafb;color:#111827;` +
		`font-family:"Inter",ui-sans-serif,system-ui,-apple-system,"Segoe UI",sans-serif}` +
		`main{padding:12px}` + // <main class="px-3">
		fmt.Sprintf(`.cal-bench{width:%dpx;max-width:none}`, hostPx) +
		css +
		`</style></head><body><main>` +
		`<div class="cal-bench"><div class="bsurf" data-bench-surface>` +
		`<div class="stack" data-bench-stack>` +
		`<div class="benchblock" data-bench-block="primary">` + markup + `</div>` +
		`</div></div></div></main></body></html>`
}

func twoCalBenchSheet(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "..", "static", "css", "calendar-bench.css"))
	if err != nil {
		t.Fatalf("read calendar-bench.css: %v", err)
	}
	return string(body)
}

func TestTwoCalEvidencePages(t *testing.T) {
	outDir := os.Getenv("TWOCAL_EVIDENCE")
	if outDir == "" {
		t.Skip("set TWOCAL_EVIDENCE=<dir> to write the two-calendar decision pages")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// THE OPERATOR'S OWN CAMPAIGN SHAPE: an in-world calendar AND a real-world
	// one. The real-world calendar is the shape seedDefaults actually builds,
	// with no authored moons — so the Block spine's own EagerLoadCalendars
	// grows THE Moon on it (block_service.go:537).
	inWorld := blockTenDayCal()
	realWorld := moonFallbackGregorianCal("cal-real", 2026, 8, 11)
	spine := NewBlockService(newBlockFakeRepo(inWorld, realWorld))
	viewer := BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner}

	// THE SEAT, DECIDED BY THE PRODUCT and not by this file. benchClassify is
	// what bench.go calls; it is run here so the assignment is measured.
	cals, err := spine.EagerLoadCampaignCalendars(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("eager load: %v", err)
	}
	primary, rw, _ := benchClassify(cals, "")
	t.Logf("benchClassify → primary=%q realWorld=%q", primary.Name, rw.Name)
	if rw.ID != realWorld.ID {
		t.Fatalf("the real-world calendar did not take the real-world seat")
	}

	// Seat A — the real-world Block as the Bench renders it TODAY.
	seatA := twoCalSeatBlock(t, spine, realWorld, viewer, "", true, false)
	// Seat B — the SAME calendar as PRIMARY, which is what a single-calendar
	// campaign gets.
	seatB := twoCalSeatBlock(t, spine, realWorld, viewer, "", false, true)

	t.Logf("seat A (real-world): ShelfHidden=true SkyOn=false → Almanac lanes=%d, "+
		"MoonsDeclared=%d, SkyGradient=%q",
		len(seatA.Month.Almanac), seatA.Month.MoonsDeclared, seatA.SkyGradient)
	t.Logf("seat B (primary):    ShelfHidden=false SkyOn=true → Almanac lanes=%d, "+
		"MoonsDeclared=%d, SkyGradient=%q",
		len(seatB.Month.Almanac), seatB.Month.MoonsDeclared, seatB.SkyGradient)

	pages := []struct {
		file, title string
		d           calblock.BlockData
	}{
		{"seat-realworld.html", "real-world seat — noShelf=true, sky=false", seatA},
		{"seat-primary.html", "primary seat — shelf shown, sky on", seatB},
	}
	for _, p := range pages {
		dst := filepath.Join(outDir, p.file)
		if err := os.WriteFile(dst, []byte(twoCalPage(t, p.title, p.d, 366)), 0o644); err != nil { //nolint:gosec // scratch artefact
			t.Fatalf("write %s: %v", dst, err)
		}
		t.Logf("wrote %s", dst)
	}

	// THE MARKUP DIFFERENCE, COUNTED — so the caption can state it rather than
	// claim it. `.phctl` is the control branch; a bare `.phrow` is the inert one.
	for _, p := range pages {
		body, err := os.ReadFile(filepath.Join(outDir, p.file)) //nolint:gosec // just written
		if err != nil {
			t.Fatal(err)
		}
		s := string(body)
		t.Logf("%-22s phctl=%d  moonpick=%d  mpan=%d  phrow=%d",
			p.file,
			strings.Count(s, `class="phrow phctl"`),
			strings.Count(s, `class="moonpick vhctl"`),
			strings.Count(s, `class="mpan"`),
			strings.Count(s, `class="phrow"`))
	}
}
