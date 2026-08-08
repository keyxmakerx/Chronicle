// block_sky_test.go — the sky header's PRODUCER side. C-CALV4-SKY (R2-5),
// [SKY-1], [SKY-6] and [SKY-7] SIGNED.
//
// THREE CLAIMS, AND THE THIRD IS THE ONE THAT NEEDED A DATABASE.
//
//  1. THE SEAT. The sky's two fields are filled on the Block that ASKED for a
//     sky and on no other — one sky per surface, never one per Block. The
//     Bench's Primary asks; the real-world Block, the subordinate rows, the
//     builder preview and the entity embed do not, and get the correct answer
//     by saying nothing at all.
//
//  2. THE FACTS. The gradient is `SkybandGradient(timeOfDayFraction(cal))`
//     REUSED rather than re-derived, the clock is the calendar's own
//     FormatCurrentTime as of render, and there is NO SUNRISE AND NO SUNSET in
//     any form ([SKY-6]) — which is asserted against the producer's own source
//     rather than promised, because the words exist in this package (import.go
//     parses them from two foreign formats and drops them) and a plausible
//     06:00/18:00 default is exactly the shape of defect that ships green.
//
//  3. THE GATE, ON A REAL MariaDB. [SKY-7] widens the Almanac register's build
//     gate BY NAME so the sky is a second reader beside the Shelf. That is a
//     PERSISTENCE claim in the only way that matters: the register is built
//     from `cal.Moons`, which is a hydrated table read, and a gate that is
//     right against a hand-built fixture and wrong against a loaded calendar is
//     a GM opening a sky that is blank on their page and nowhere else. So it
//     gets a database (`make test-db-up`), through the whole spine.
package calendar

import (
	"bytes"
	"context"
	"database/sql"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// --- 1. the seat ------------------------------------------------------------

// TestBlockSky_SeatsOnTheBlockThatAsked is the [SKY-1] pin at the producer.
//
// IT IS TWO-DIRECTIONAL ON PURPOSE. A guard that only proved the real-world
// Block carries no sky would stay green on a slice that shipped no sky at all,
// which is the same defect [SKY-11]'s guard exists to prevent one layer up.
func TestBlockSky_SeatsOnTheBlockThatAsked(t *testing.T) {
	cal := blockTenDayCal()
	cal.HoursPerDay, cal.MinutesPerHour = 24, 60
	cal.CurrentHour, cal.CurrentMinute = 19, 42

	on := projectBlock(BlockProjectionInput{
		Calendar: cal, Viewer: BlockViewer{UserID: "u-gm"}, MonthIndex: 0, Year: 1523,
		SkyOn: true,
	})
	if on.SkyGradient == "" {
		t.Error("a Block that asked for the sky carries no gradient — SkyGradient is the " +
			"seat gate the renderer reads, so an empty one is no sky at all")
	}
	if on.SkyClock != "19:42" {
		t.Errorf("SkyClock = %q, want %q — the in-world time is the calendar's own, "+
			"formatted at the producer because the widget has no calendar geometry",
			on.SkyClock, "19:42")
	}

	off := projectBlock(BlockProjectionInput{
		Calendar: cal, Viewer: BlockViewer{UserID: "u-gm"}, MonthIndex: 0, Year: 1523,
	})
	if off.SkyGradient != "" || off.SkyClock != "" {
		t.Errorf("a Block that did NOT ask for a sky carries gradient=%q clock=%q — "+
			"[SKY-1] is one sky per surface, and the zero value of SkyOn is the "+
			"answer every host but the Bench's Primary wants",
			off.SkyGradient, off.SkyClock)
	}
}

// TestBenchSky_OnlyThePrimaryBlockAsks walks the HOST's own call sites rather
// than a fixture, because the seat is decided at the one place that knows which
// Block is the Primary and a second `SkyOn: true` there would be a second sky
// nobody would notice until it was drawn.
func TestBenchSky_OnlyThePrimaryBlockAsks(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("bench.go"))
	if err != nil {
		t.Fatalf("read bench.go: %v", err)
	}
	code := string(src)
	// The two shipped call sites, by their exact argument tails: the Primary
	// asks (…, false, true, …) and the real-world Block does not (…, true,
	// false, …). Reading the calls rather than counting the literal `true`
	// keeps the assertion from being satisfied by any other boolean in the file.
	if n := strings.Count(code, "h.benchBlock(ctx, spine, primary, viewer, activeID, false, true,"); n != 1 {
		t.Errorf("%d Primary benchBlock calls asking for a sky, want exactly 1 — "+
			"[SKY-1] seats the sky on the PRIMARY Block only", n)
	}
	if n := strings.Count(code, "h.benchBlock(ctx, spine, realWorld, viewer, activeID, true, false,"); n != 1 {
		t.Errorf("%d real-world benchBlock calls refusing a sky, want exactly 1 — the "+
			"real-world Block's Almanac is empty for two independent reasons and a "+
			"sky there would carry a gradient and a clock and nothing else, forever", n)
	}
}

// --- 2. the facts -----------------------------------------------------------

// TestBlockSky_GradientIsTheShippedFunctionReused fails if the producer ever
// grows a gradient of its own. `SkybandGradient` is pinned by
// TestSkybandGradient_SnapsToKeyframes; re-deriving it here would be a second
// colour authority for one fact, and [SKY-6] says REUSED, not re-derived.
func TestBlockSky_GradientIsTheShippedFunctionReused(t *testing.T) {
	cal := blockTenDayCal()
	cal.HoursPerDay, cal.MinutesPerHour = 24, 60
	for _, hour := range []int{0, 6, 12, 19, 23} {
		cal.CurrentHour, cal.CurrentMinute = hour, 0
		got := projectBlock(BlockProjectionInput{
			Calendar: cal, Viewer: BlockViewer{UserID: "u-gm"}, MonthIndex: 0, Year: 1523,
			SkyOn: true,
		}).SkyGradient
		want := SkybandGradient(timeOfDayFraction(cal))
		if got != want {
			t.Errorf("hour %d: the Block's gradient is not the shipped one\n got %q\nwant %q",
				hour, got, want)
		}
	}
}

// TestBlockSky_NoSunriseAndNoSunset reads the producer's OWN source. [SKY-6]
// refuses a daylight boundary in every form — not a tick, not a chip, not a
// "needs backend" note, not a 06:00/18:00 default — because Chronicle does not
// persist one: `grep` over db/migrations/ and the plugin's migrations returns
// zero rows, and the only occurrences in this package are the parse-only DTOs
// in import.go that read the words from Simple Calendar / Fantasy Calendar
// exports and DROP them.
//
// IT READS THE SKY'S FILES BY NAME rather than the package, because import.go
// legitimately contains both words and a package-wide ban would either be red
// on day one or would have to carve import.go out and thereby stop being a ban.
//
// AND IT READS CODE, NOT COMMENTS. The producer's own doc comment explains at
// length WHY there is no sunrise — naming the thing it refuses, which is the
// only way that paragraph can be read — and a scanner that judged the comment
// would force the refusal to be undocumented in order to stay green. Go source
// is stripped by re-printing a comment-free AST; the .templ and .css files are
// read whole, because nothing legitimately names a daylight boundary there.
func TestBlockSky_NoSunriseAndNoSunset(t *testing.T) {
	for _, path := range []string{
		filepath.Join("block_projection.go"),
		filepath.Join("..", "..", "widgets", "calendar_block", "block.templ"),
		filepath.Join("..", "..", "..", "static", "css", "calendar-bench.css"),
		filepath.Join("..", "..", "..", "static", "css", "calendar-block.css"),
	} {
		body := skyTestSourceWithoutComments(t, path)
		low := strings.ToLower(body)
		for _, bad := range []string{"sunrise", "sunset"} {
			if strings.Contains(low, bad) {
				t.Errorf("%s names %q IN CODE — Chronicle does not persist a daylight "+
					"boundary (import.go parses it from two foreign formats and drops "+
					"it), and deriving one on a worldbuilding platform is the defect "+
					"WorldStateSun.Tint already refuses by shipping null ([SKY-6])",
					path, bad)
			}
		}
	}
}

// skyTestSourceWithoutComments returns a file's source with Go comments removed.
// Non-Go files come back verbatim.
func skyTestSourceWithoutComments(t *testing.T, path string) string {
	t.Helper()
	src, err := os.ReadFile(path) //nolint:gosec // test-local, fixed literal paths
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if filepath.Ext(path) != ".go" {
		return string(src)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, 0) // 0 == comments discarded
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, file); err != nil {
		t.Fatalf("print %s: %v", path, err)
	}
	return buf.String()
}

// --- 3. the gate, by name ---------------------------------------------------

// TestBlockSky_AlmanacGateNamesBothReaders is [SKY-7]'s whole claim, in the
// four combinations that matter.
//
// THE THIRD ROW IS THE BUG THIS SLICE FIXES: a viewer who turns the Shelf layer
// off, or a host that docks the Block without one, used to silently empty the
// sky's expansion — invisible until somebody reported "the sky is blank on my
// page". Either reader asking is now enough, and neither is subordinate.
func TestBlockSky_AlmanacGateNamesBothReaders(t *testing.T) {
	cal := blockTenDayCal()
	cal.Moons = blockFourMoons()

	for _, tc := range []struct {
		name        string
		shelfHidden bool
		skyHidden   bool
		want        bool
	}{
		{"shelf reads it, sky does not", false, true, true},
		{"both read it", false, false, true},
		{"only the sky reads it — the widened leg", true, false, true},
		{"neither reads it", true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			geo := buildMonthGeometry(cal, blockMonthGeometryInput{
				MonthIndex: 0, Year: 1523, ShowMoons: true, MoonCap: 3,
				ShelfHidden: tc.shelfHidden, SkyHidden: tc.skyHidden,
			})
			if built := len(geo.Almanac) > 0; built != tc.want {
				t.Errorf("Almanac built = %v, want %v (shelfHidden=%v skyHidden=%v)",
					built, tc.want, tc.shelfHidden, tc.skyHidden)
			}
		})
	}

	// …and the flag TRAVELS. A gate that is right in the geometry and unwired in
	// the projection is the shape of a fix that never reaches a page.
	got := projectBlock(BlockProjectionInput{
		Calendar: cal, Viewer: BlockViewer{UserID: "u-gm"}, MonthIndex: 0, Year: 1523,
		ShelfHidden: true, SkyOn: true, MoonCap: 3,
	})
	if len(got.Month.Almanac) == 0 {
		t.Error("projectBlock built no Almanac for a Block with no Shelf and a sky — " +
			"SkyOn does not reach blockMonthGeometryInput.SkyHidden")
	}
}

// TestBlockSky_NoMoonsDropsTheDiscsRatherThanBlankingThem is [SKY-7]'s closing
// clause and [SKY-6]'s "dropped entirely rather than blanked": a calendar that
// declares no moons still gets its gradient, its clock and its season word, and
// gets NO register — not an empty one, not a row of empty disc slots.
func TestBlockSky_NoMoonsDropsTheDiscsRatherThanBlankingThem(t *testing.T) {
	cal := blockTenDayCal()
	cal.HoursPerDay, cal.MinutesPerHour = 24, 60
	cal.CurrentHour, cal.CurrentMinute = 4, 5

	got := projectBlock(BlockProjectionInput{
		Calendar: cal, Viewer: BlockViewer{UserID: "u-gm"}, MonthIndex: 0, Year: 1523,
		SkyOn: true, MoonCap: 3,
	})
	if got.SkyGradient == "" || got.SkyClock != "04:05" {
		t.Errorf("a moonless calendar lost its sky: gradient=%q clock=%q",
			got.SkyGradient, got.SkyClock)
	}
	if len(got.Month.Almanac) != 0 {
		t.Errorf("a calendar declaring no moons built %d Almanac lanes — the expansion "+
			"offers the current sky only, on almanacReachable's own absence precedent",
			len(got.Month.Almanac))
	}
}

// --- the same claim, against a real MariaDB ---------------------------------

// TestBlockSky_ProducerAndGate_Integration drives the sky's producer through
// the SPINE against a real database, because both halves of this stage are
// persistence claims wearing render clothes:
//
//   - the clock and the gradient are computed from `current_hour` /
//     `current_minute` / `hours_per_day` / `minutes_per_hour`, four columns
//     that a hand-built fixture sets by assignment and a loaded calendar sets
//     by hydration — and `timeOfDayFraction` divides by the last two, so a
//     calendar whose clock geometry did not survive the round trip yields a
//     0.0 fraction and a midnight gradient at seven in the evening;
//   - the Almanac register is built from `cal.Moons`, a separate table read, so
//     [SKY-7]'s widened gate is only actually widened if the hydration that
//     feeds it happened.
func TestBlockSky_ProducerAndGate_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("sky producer integration test requires a database; skipped under -short")
	}
	db := newCalendarScratchSchema(t)
	ctx := context.Background()
	campaignID, cal := calTestSeedSkyCalendar(t, db)

	spine := NewBlockService(NewBlockRepository(db))
	viewer := BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner}

	// THE PRIMARY: a sky, and an Almanac built for the sky ALONE — the Shelf is
	// hidden on this request precisely so the widened leg of [SKY-7]'s gate is
	// the only thing that could have built it.
	primary, err := spine.Block(ctx, BlockRequest{
		CalendarID: cal.ID, CampaignID: campaignID, Viewer: viewer,
		ShelfHidden: true, SkyOn: true, MoonCap: 3,
	})
	if err != nil {
		t.Fatalf("Block(primary): %v", err)
	}
	if primary.SkyClock != "19:42" {
		t.Errorf("SkyClock = %q, want \"19:42\" — the clock geometry did not survive "+
			"the round trip, which is the failure a fixture cannot show", primary.SkyClock)
	}
	if want := SkybandGradient(timeOfDayFraction(cal)); primary.SkyGradient != want {
		t.Errorf("SkyGradient = %q, want %q", primary.SkyGradient, want)
	}
	if primary.SkyGradient == SkybandGradient(0) {
		t.Error("the gradient snapped to midnight on a calendar whose stored hour is 19 — " +
			"timeOfDayFraction divides by hours_per_day × minutes_per_hour, so this is " +
			"what a lost clock geometry looks like on the page")
	}
	if len(primary.Month.Almanac) != 2 {
		t.Fatalf("the sky's Almanac has %d lanes, want 2 — the Shelf is hidden on this "+
			"request, so [SKY-7]'s widened leg is the only thing that could build it",
			len(primary.Month.Almanac))
	}
	if primary.Month.Almanac[0].Name != "Selune" {
		t.Errorf("first lane is %q, want \"Selune\" — the register is the calendar's own "+
			"declared bodies, hydrated", primary.Month.Almanac[0].Name)
	}

	// THE NEIGHBOUR: same calendar, same database, no sky asked for. One sky per
	// surface is a producer fact here, not a template branch.
	neighbour, err := spine.Block(ctx, BlockRequest{
		CalendarID: cal.ID, CampaignID: campaignID, Viewer: viewer,
		ShelfHidden: true, MoonCap: 3,
	})
	if err != nil {
		t.Fatalf("Block(neighbour): %v", err)
	}
	if neighbour.SkyGradient != "" || neighbour.SkyClock != "" {
		t.Errorf("a Block that asked for no sky came back with gradient=%q clock=%q",
			neighbour.SkyGradient, neighbour.SkyClock)
	}
	if len(neighbour.Month.Almanac) != 0 {
		t.Errorf("a Block with neither Shelf nor sky built %d Almanac lanes — the gate "+
			"was widened for a named reader, not opened", len(neighbour.Month.Almanac))
	}
}

// calTestSeedSkyCalendar seeds an in-world calendar that actually declares
// moons and an evening hour, which the nav fixture deliberately does not.
func calTestSeedSkyCalendar(t *testing.T, db *sql.DB) (string, *Calendar) {
	t.Helper()
	ctx := context.Background()
	campaignID := calTestSeedCampaign(t, db)
	repo := NewCalendarRepository(db)

	cal := &Calendar{
		ID:         calTestID(t),
		CampaignID: campaignID, Name: "Hearthwane", Mode: ModeFantasy,
		IsDefault: true, CurrentYear: 1523, CurrentMonth: 1, CurrentDay: 14,
		HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60,
		CurrentHour: 19, CurrentMinute: 42,
	}
	if err := repo.Create(ctx, cal); err != nil {
		t.Fatalf("create calendar: %v", err)
	}
	if err := repo.SetMonths(ctx, cal.ID, []MonthInput{
		{Name: "Emberfall", Days: 30}, {Name: "Thawrun", Days: 30},
	}); err != nil {
		t.Fatalf("set months: %v", err)
	}
	if err := repo.SetWeekdays(ctx, cal.ID, []WeekdayInput{
		{Name: "Sul"}, {Name: "Mol"}, {Name: "Zor"}, {Name: "Ith"},
		{Name: "Ves"}, {Name: "Kar"}, {Name: "Fen"},
	}); err != nil {
		t.Fatalf("set weekdays: %v", err)
	}
	if err := repo.SetMoons(ctx, cal.ID, []MoonInput{
		{Name: "Selune", CycleDays: 29.5, Color: "#dfe6f5"},
		{Name: "Flint", CycleDays: 11.2, Color: "#f0c98a"},
	}); err != nil {
		t.Fatalf("set moons: %v", err)
	}
	// Re-read so the returned calendar carries what the spine will load, rather
	// than what this function typed.
	loaded, err := repo.GetByID(ctx, cal.ID)
	if err != nil {
		t.Fatalf("reload calendar: %v", err)
	}
	return campaignID, loaded
}
