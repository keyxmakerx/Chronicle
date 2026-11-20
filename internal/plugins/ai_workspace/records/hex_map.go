package records

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/hexsketch"
)

// HexMapAPI is what a hex map needs from the maps plugin, in this package's
// own types (adapted in app wiring).
type HexMapAPI interface {
	// CreateHexMap makes a blank map, with no picture, whose hex field holds at
	// least cols by rows hexes, and returns its id.
	CreateHexMap(ctx context.Context, campaignID, name string, cols, rows int) (string, error)
	// UsesHexGrid reports whether the map's grid is hexes.
	UsesHexGrid(ctx context.Context, mapID string) (bool, error)
	// Cells reads the hexes stored on the map as the actor sees them; the
	// import's actor is the owner, who sees every hex.
	Cells(ctx context.Context, campaignID, mapID string, a Actor) ([]HexCellState, error)
	// PaintCells writes one batch through the hex layer's own checks.
	PaintCells(ctx context.Context, campaignID, mapID string, a Actor, cells []HexWrite) error
}

// HexCellState is a stored hex, as far as an update needs it: that it exists,
// and the exits of its lines.
type HexCellState struct {
	Col, Row           int
	Road, River, Trail *int
}

// HexWrite is one hex a block paints. Unset fields leave what is stored: an
// empty Terrain, a nil line mask, and Place false. A place writes Name and
// Notes together, so empty Notes clears them.
type HexWrite struct {
	Col, Row           int
	Terrain            string
	Road, River, Trail *int
	Place              bool
	Name, Notes        string
}

// HexBatch is how many hexes go to the maps plugin in one call; it is the hex
// layer's own cap on a batch, pinned by a test in app wiring.
const HexBatch = 500

// HexMapKind makes a new blank hex map and paints it, or paints onto a map
// that already uses a hex grid: land, roads, rivers, trails and named places,
// each an ordinary hex the map page's Paint tools can change afterwards.
type HexMapKind struct {
	Svc  HexMapAPI
	Maps MapsAPI
}

var _ aiplug.Staged = HexMapKind{}

func (HexMapKind) Name() string  { return "hex-map" }
func (HexMapKind) Label() string { return "Hex map" }
func (HexMapKind) Doc() string {
	return "A hex map painted hex by hex: a new blank map (`action: create`), or paint added onto a map that already uses a hex grid (`action: update`, the map named in `name` or `map`; a map on a square grid is refused). Keys: `name`, `size` (hexes across x down, 4 to 100 each way, like `30 x 20`). On an update `size` is only the bounds for the lines in this block: hexes it does not mention are left alone, a road, river or trail is added to the ones already on a hex, land replaces land, and a `place` replaces the hex's name and notes.\n\n" +
		"Hexes are written `col,row`, 0,0 at the top-left. They are pointy-top and odd rows sit half a hex to the right, so neighbours differ by row: 4,2 touches 3,2 5,2 3,1 4,1 3,3 4,3 but 4,3 touches 3,3 5,3 4,2 5,2 4,4 5,4; lean on `to`, `around` and `along` instead of counting. One thing per line, `#` lines are skipped, later lines paint over earlier ones:\n\n" +
		"- `fill <land>`: every hex\n" +
		"- `<land> [\"Name\"] c,r`: one hex; `<land> c,r to c,r`: a box between two corners; `<land> around c,r radius n` (n 0 to 30); `<land> along c,r c,r c,r …`: the hexes on the straight runs between the points, such as a mountain range\n" +
		"- `road|river|trail [\"Name\"] c,r c,r …`: a line through the straight runs between the points; it lies on top of the land and does not change it\n" +
		"- `place \"Name\" c,r[: notes]`: names a hex\n\n" +
		"`<land>` is plains, forest, hills, mountain, water, swamp, desert, snow, town, jungle, farmland, scrub, badlands, waste, volcanic, coast, deep, ice or cavern (also mountains, deep sea, caverns, sea, lake, city, village). `road` is a line, never land. A \"Name\" on land or a line is only a label at review and is not saved. A box or `around` is clipped to the size; a single hex, line point or place outside it leaves that line out. A line that cannot be read is shown at review and left out; the rest is still made.\n\n" +
		"```\n---\nkind: hex-map\nname: The Ashen Coast\nsize: 16 x 10\n---\nfill water\nplains 2,1 to 12,7\nforest around 6,4 radius 2\nmountain along 12,1 14,3 14,6\nriver \"Slow Run\" 6,4 4,7 3,9\nroad 2,3 6,4 11,5\ntown 2,3\nplace \"Harbor\" 2,3: The only deep-water port.\n```\n\n" +
		"To paint onto a hex map that already exists:\n\n```\n---\nkind: hex-map\naction: update\nname: Imix\nsize: 30 x 20\n---\ntrail 4,4 9,6\nplace \"Old Shrine\" 9,6: Candles still burn.\n```"
}

// hexName is the map a record names: `map:` on an update, else `name`.
func hexName(r Record) string {
	if m := r.Str("map"); m != "" && r.Action == ActionUpdate {
		return m
	}
	return r.Name
}

// plan parses the record and checks what blocks it. Bad lines are only
// warnings, so a mostly good block still makes its map.
func (k HexMapKind) plan(r Record) (hexsketch.Result, string) {
	if r.Action != ActionCreate && r.Action != ActionUpdate {
		return hexsketch.Result{}, "a hex map can only be made (`action: create`) or painted onto with `action: update`; delete a map on its map page"
	}
	if hexName(r) == "" {
		return hexsketch.Result{}, "a hex map needs `name`"
	}
	if r.Str("size") == "" {
		return hexsketch.Result{}, "a hex map needs `size`, like 30 x 20"
	}
	cols, rows, err := hexsketch.ParseSize(r.Str("size"))
	if err != nil {
		return hexsketch.Result{}, err.Error()
	}
	res := hexsketch.Parse(r.Body, cols, rows)
	if len(res.Parts) == 0 {
		return res, "there is nothing on this hex map that can be drawn"
	}
	return res, ""
}

func problemTexts(res hexsketch.Result) []string {
	var out []string
	for _, p := range res.Problems {
		out = append(out, p.String())
	}
	return out
}

// findTarget finds the named map in this campaign and checks it is a hex map;
// a map that is not there is a plain refusal, never a new map.
func (k HexMapKind) findTarget(ctx context.Context, campaignID string, r Record) (MapRef, error) {
	name := hexName(r)
	ms, err := k.Maps.ListMaps(ctx, campaignID)
	if err != nil {
		return MapRef{}, badRequestf("could not read the maps")
	}
	var hits []MapRef
	for _, m := range ms {
		if m.CampaignID == campaignID && sameName(m.Name, name) {
			hits = append(hits, m)
		}
	}
	switch len(hits) {
	case 0:
		return MapRef{}, badRequestf("no map called %s to paint onto", quote(name))
	case 1:
	default:
		return MapRef{}, badRequestf("more than one map is called %s", quote(name))
	}
	hex, err := k.Svc.UsesHexGrid(ctx, hits[0].ID)
	if err != nil {
		return MapRef{}, badRequestf("could not read the map %s", quote(name))
	}
	if !hex {
		return MapRef{}, badRequestf("the %s map does not use a hex grid; switch the map's grid to hexes on its map page first, then paint onto it", hits[0].Name)
	}
	return hits[0], nil
}

// stored reads the hexes already on the map, by position.
func (k HexMapKind) stored(ctx context.Context, campaignID string, m MapRef, a Actor) (map[hexsketch.Hex]HexCellState, error) {
	cells, err := k.Svc.Cells(ctx, campaignID, m.ID, a)
	if err != nil {
		return nil, badRequestf("could not read the hexes on the %s map", m.Name)
	}
	out := make(map[hexsketch.Hex]HexCellState, len(cells))
	for _, c := range cells {
		out[hexsketch.Hex{Col: c.Col, Row: c.Row}] = c
	}
	return out, nil
}

// Stage notes a map this row creates, so a pin after it may name it.
func (HexMapKind) Stage(ctx context.Context, _ string, _ Actor, r Record) context.Context {
	if r.Action != ActionCreate {
		return ctx
	}
	return withPendingMap(ctx, mapKey(hexName(r)))
}

func (k HexMapKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	res, blocked := k.plan(r)
	if blocked != "" {
		return Plan{Error: blocked, Warnings: problemTexts(res)}
	}
	warnings := problemTexts(res)
	if r.Action == ActionUpdate {
		m, err := k.findTarget(ctx, campaignID, r)
		if err != nil {
			return Plan{Error: planError(err), Warnings: warnings}
		}
		have, err := k.stored(ctx, campaignID, m, a)
		if err != nil {
			return Plan{Error: planError(err), Warnings: warnings}
		}
		over := 0
		for _, c := range res.Cells {
			if _, ok := have[hexsketch.Hex{Col: c.Col, Row: c.Row}]; ok {
				over++
			}
		}
		sum := fmt.Sprintf("paints %s on the %s map: %d new, %d painted over",
			plural(len(res.Cells), "hex", "hexes"), m.Name, len(res.Cells)-over, over)
		if l := lineSummary(res); l != "" {
			sum += "; adds " + l
		}
		if n := namedPlaces(res); n > 0 {
			sum += "; names " + plural(n, "place", "places")
		}
		return Plan{Summary: sum, Warnings: warnings}
	}

	items := []string{}
	if land := landSummary(res); land != "" {
		items = append(items, land)
	}
	items = append(items, lineItems(res)...)
	if n := namedPlaces(res); n > 0 {
		items = append(items, plural(n, "named place", "named places"))
	}
	sum := fmt.Sprintf("makes a hex map %d by %d", res.Cols, res.Rows)
	if len(items) > 0 {
		sum += " with " + joinAnd(items)
	}
	out := Plan{Summary: sum, Warnings: warnings}
	if ms, err := k.Maps.ListMaps(ctx, campaignID); err == nil {
		for _, m := range ms {
			if m.CampaignID == campaignID && sameName(m.Name, r.Name) {
				out.Warnings = append(out.Warnings, "A map with this name already exists; this makes another")
				break
			}
		}
	}
	return out
}

// landSummary is "412 hexes of land (plains 200, forest 120, …)", the kinds by
// how many hexes each holds.
func landSummary(res hexsketch.Result) string {
	counts := map[string]int{}
	total := 0
	for _, c := range res.Cells {
		if c.Terrain != "" {
			counts[c.Terrain]++
			total++
		}
	}
	if total == 0 {
		return ""
	}
	kinds := make([]string, 0, len(counts))
	for k := range counts {
		kinds = append(kinds, k)
	}
	sort.Slice(kinds, func(i, j int) bool {
		if counts[kinds[i]] != counts[kinds[j]] {
			return counts[kinds[i]] > counts[kinds[j]]
		}
		return kinds[i] < kinds[j]
	})
	const shown = 5
	var parts []string
	for i, k := range kinds {
		if i == shown {
			parts = append(parts, "…")
			break
		}
		parts = append(parts, fmt.Sprintf("%s %d", k, counts[k]))
	}
	return fmt.Sprintf("%s (%s)", plural(total, "hex of land", "hexes of land"), strings.Join(parts, ", "))
}

// lineItems counts the lines written, one item per kind of line, rivers first
// as they are what shapes a map most.
func lineItems(res hexsketch.Result) []string {
	var out []string
	for _, w := range []string{"river", "road", "trail"} {
		n := 0
		for _, p := range res.Parts {
			if p.Kind == hexsketch.PartLine && p.Word == w {
				n++
			}
		}
		if n > 0 {
			out = append(out, plural(n, w, w+"s"))
		}
	}
	return out
}

func lineSummary(res hexsketch.Result) string { return joinAnd(lineItems(res)) }

func namedPlaces(res hexsketch.Result) int {
	n := 0
	for _, c := range res.Cells {
		if c.Name != "" {
			n++
		}
	}
	return n
}

// orMask adds the exits already stored on a hex to the ones the block draws,
// so a new line never takes an old one away.
func orMask(added, have *int) *int {
	if added == nil {
		return nil
	}
	v := *added
	if have != nil {
		v |= *have
	}
	return &v
}

// writesFor turns the block's final cells into hexes to write. have is empty
// for a new map.
func writesFor(cells []hexsketch.Cell, have map[hexsketch.Hex]HexCellState) []HexWrite {
	out := make([]HexWrite, 0, len(cells))
	for _, c := range cells {
		old := have[hexsketch.Hex{Col: c.Col, Row: c.Row}]
		out = append(out, HexWrite{
			Col: c.Col, Row: c.Row, Terrain: c.Terrain,
			Road: orMask(c.Road, old.Road), River: orMask(c.River, old.River), Trail: orMask(c.Trail, old.Trail),
			Place: c.Name != "", Name: c.Name, Notes: c.Notes,
		})
	}
	return out
}

// Apply re-reads the block, makes the map (or finds the one to paint onto),
// and writes the hexes in batches. Batches before a failed one stay written,
// so the error says how many hexes were not painted.
func (k HexMapKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	res, blocked := k.plan(r)
	if blocked != "" {
		return apperror.NewBadRequest(blocked)
	}
	var (
		mapID  string
		have   map[hexsketch.Hex]HexCellState
		update = r.Action == ActionUpdate
		name   = hexName(r)
	)
	if update {
		m, err := k.findTarget(ctx, campaignID, r)
		if err != nil {
			return err
		}
		if have, err = k.stored(ctx, campaignID, m, a); err != nil {
			return err
		}
		mapID, name = m.ID, m.Name
	} else {
		id, err := k.Svc.CreateHexMap(ctx, campaignID, r.Name, res.Cols, res.Rows)
		if err != nil {
			return err
		}
		mapID = id
	}
	writes := writesFor(res.Cells, have)
	painted := 0
	for painted < len(writes) {
		batch := writes[painted:min(painted+HexBatch, len(writes))]
		if err := k.Svc.PaintCells(ctx, campaignID, mapID, a, batch); err != nil {
			left := len(writes) - painted
			if update {
				return badRequestf("painted only %d of the %d hexes onto the %s map: the other %d were not painted (%s)",
					painted, len(writes), name, left, planError(err))
			}
			return badRequestf("made the %s map, but painted only %d of its %d hexes: the other %d were not painted (%s)",
				name, painted, len(writes), left, planError(err))
		}
		painted += len(batch)
	}
	return nil
}

// Export lists nothing: hex maps are written, not read back.
func (HexMapKind) Export(context.Context, string, Actor) (string, error) { return "", nil }
