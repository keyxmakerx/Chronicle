// theater_test.go — C-CALV4-THEATER (slice R2-3).
//
// WHY THESE ASSERTIONS PARSE THE HTML RATHER THAN GREP IT. Three of the claims
// this slice turns on are STRUCTURAL — "no `id` appears twice in the rendered
// entity page", "no radio `name` is shared across the two Block subtrees", and
// "the scaffold is outside every `.cal-block-host`" — and none of them can be
// stated as a substring. `golang.org/x/net/html` is already a production
// dependency of this repo (internal/plugins/ai_workspace/importer/htmlconv),
// so the parse costs nothing new.
package calendar

import (
	"context"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	calblock "github.com/keyxmakerx/chronicle/internal/widgets/calendar_block"
)

// ── the parse helpers ───────────────────────────────────────────────────────

func theaterParse(t *testing.T, markup string) *html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(markup))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return doc
}

func theaterAttr(n *html.Node, name string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val, true
		}
	}
	return "", false
}

func theaterHasClass(n *html.Node, want string) bool {
	v, _ := theaterAttr(n, "class")
	for _, f := range strings.Fields(v) {
		if f == want {
			return true
		}
	}
	return false
}

// theaterWalk visits every element node, depth first.
func theaterWalk(n *html.Node, fn func(*html.Node)) {
	if n.Type == html.ElementNode {
		fn(n)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		theaterWalk(c, fn)
	}
}

// theaterFind returns the first element for which pred is true.
func theaterFind(root *html.Node, pred func(*html.Node) bool) *html.Node {
	var hit *html.Node
	theaterWalk(root, func(n *html.Node) {
		if hit == nil && pred(n) {
			hit = n
		}
	})
	return hit
}

func theaterFindAll(root *html.Node, pred func(*html.Node) bool) []*html.Node {
	var out []*html.Node
	theaterWalk(root, func(n *html.Node) {
		if pred(n) {
			out = append(out, n)
		}
	})
	return out
}

func theaterHasAncestor(n *html.Node, pred func(*html.Node) bool) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Type == html.ElementNode && pred(p) {
			return true
		}
	}
	return false
}

func isBlockHost(n *html.Node) bool { return theaterHasClass(n, "cal-block-host") }

func isTheaterScaffold(n *html.Node) bool {
	_, ok := theaterAttr(n, "data-cal-theater")
	return ok
}

// renderEntityCalAs renders the entity embed for an arbitrary viewer, including
// the anonymous one ([TH-9]: the control is emitted for EVERY viewer who
// receives the embed, so the anonymous render has to be reachable from a test).
func renderEntityCalAs(t *testing.T, svc CalendarService, role campaigns.Role, userID string) string {
	t.Helper()
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: role}
	var sb strings.Builder
	if err := EntityCalendarBlock(svc, cc, "ent-1", userID, "", "").Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	return sb.String()
}

// ── the seed ────────────────────────────────────────────────────────────────

// [TH-2] §4.1 SIGNED: the theater's seed is the Bench's own FIVE keys, not a
// sixth invented set — it restores exactly what [BR2-8] removed and nothing
// else. And it is a SEED, not an override: it goes through resolveBlockLayers,
// so a viewer who switched `shelf` off in the switchboard gets a theater with
// no Shelf. That is the one place a reader will expect an override and not get
// one, so it is asserted rather than described.
func TestTheaterSeed_IsTheBenchsFiveAndTheStoreStillWins(t *testing.T) {
	seeded := theaterBlockLayers(blockLayerPrefs{})
	want := []string{"moons", "eras", "weeknums", "ledger", "shelf"}
	if strings.Join(seeded.Enabled, ",") != strings.Join(want, ",") {
		t.Errorf("theater seed = %v, want the Bench's five %v", seeded.Enabled, want)
	}
	// The two keys [BR2-8] took off the entity embed are exactly the two this
	// puts back, and the embed's own seed is UNTOUCHED by this slice.
	embed := entityBlockLayers(blockLayerPrefs{})
	if strings.Join(embed.Enabled, ",") != "moons,eras,weeknums" {
		t.Errorf("the entity embed's seed changed (%v) — no layer seed is this slice's to touch", embed.Enabled)
	}

	// THE SWITCHBOARD STILL WINS. A stored set replaces the seed entirely,
	// including "chose nothing" — which is what keeps the theater from being a
	// second place layers are chosen (L29).
	stored := theaterBlockLayers(blockLayerPrefs{Stored: []string{"moons"}})
	if strings.Join(stored.Enabled, ",") != "moons" {
		t.Errorf("the stored set did not win: %v — the theater's seed is a SEED, never an override", stored.Enabled)
	}
	if bare := theaterBlockLayers(blockLayerPrefs{Stored: []string{}}); len(bare.Enabled) != 0 {
		t.Errorf("a viewer who chose NOTHING got %v — collapsing that into the seed makes the bare month unreachable", bare.Enabled)
	}
}

// ── the copy ────────────────────────────────────────────────────────────────

// [TH-13] SIGNED: the EMBED keeps its switchboard and the theater's copy
// renders WITHOUT one — suppressed through the two HOST-OWNED LayerState fields
// switchboardLive reads, so no widget file is opened.
func TestTheaterCopy_SuppressesTheSwitchboardAtTheProducer(t *testing.T) {
	prefs := blockLayerPrefs{PersistURL: "/campaigns/camp-1/calendar/prefs"}
	embed := calblock.BlockData{CalendarSlug: "harptos"}
	embed.Viewer.HostEntity = "ent-1"
	embed.Layers = entityBlockLayers(prefs)

	if !embed.Layers.HasSwitchboard || embed.Layers.PersistURL == "" {
		t.Fatal("the fixture's EMBED has no switchboard, so the assertion below would be vacuous")
	}
	cp := theaterBlockCopy(embed, prefs)
	if cp.Layers.HasSwitchboard {
		t.Error("the theater's copy kept HasSwitchboard — two live switchboards on one page, both writing the same per-viewer preference, is the state fork [TH-13] refuses")
	}
	if cp.Layers.PersistURL != "" {
		t.Error("the theater's copy kept PersistURL — the invariant is HasSwitchboard == (PersistURL != \"\") and both are cleared TOGETHER")
	}
	// And the EMBED is untouched by the copy. It loses nothing: it keeps the
	// affordance it ships today and stays the one place a viewer says "I want
	// the Ledger permanently".
	if !embed.Layers.HasSwitchboard || embed.Layers.PersistURL == "" {
		t.Error("taking the copy mutated the EMBED's LayerState")
	}
}

// [TH-2] SIGNED: the copy re-namespaces `Viewer.HostEntity`, and that is CORE
// rather than polish — without it the theater's tie toggle is dead and pressing
// it silently re-inks the embed behind the backdrop.
func TestTheaterCopy_ReNamespacesTheHostEntity(t *testing.T) {
	embed := calblock.BlockData{CalendarSlug: "harptos"}
	embed.Viewer.HostEntity = "ent-1"

	cp := theaterBlockCopy(embed, blockLayerPrefs{})
	if cp.Viewer.HostEntity == embed.Viewer.HostEntity {
		t.Fatal("the copy shares the embed's HostEntity — every id and every radio-group name the Block emits is a pure function of (CalendarSlug, HostEntity), so the two Blocks would collide in all six namespaces")
	}
	if !strings.HasPrefix(cp.Viewer.HostEntity, embed.Viewer.HostEntity) {
		t.Errorf("the copy's host token %q no longer derives from the embed's %q", cp.Viewer.HostEntity, embed.Viewer.HostEntity)
	}
	// The counts, the mode and the identity are computed BEFORE the copy, so
	// they must survive it byte-identically — that is what makes the
	// re-namespace a DOM change and not a data change.
	if cp.Viewer.TiedCount != embed.Viewer.TiedCount || cp.Viewer.WholeCount != embed.Viewer.WholeCount {
		t.Error("the copy changed a tie count — the counts are the spine's, from one viewer-filtered pass")
	}
	if cp.CalendarSlug != embed.CalendarSlug {
		t.Error("the copy changed the calendar identity")
	}
}

// [TH-15] SIGNED: once per page means once per (calendar, entity), and the id
// is namespaced per block so a second registry invocation for a DIFFERENT
// calendar or entity gets its own uniquely-named scaffold rather than a
// duplicate id. The shipped precedent is the worldstate seed's
// `"cal-v2-worldstate-cal-" + entityID` one line above the theater's mount.
func TestTheaterScaffoldID_IsNamespacedPerCalendarAndEntity(t *testing.T) {
	a := theaterScaffoldID("harptos", "ent-1")
	for _, other := range []string{
		theaterScaffoldID("harptos", "ent-2"),
		theaterScaffoldID("elven", "ent-1"),
	} {
		if a == other {
			t.Errorf("two (calendar, entity) pairs share the scaffold id %q — the first Expand button on the page would drive the wrong theater", a)
		}
	}
	if theaterScaffoldID("harptos", "ent-1") != a {
		t.Error("the scaffold id is not a pure function of its inputs")
	}
	// The heading id hangs off the scaffold id, so `aria-labelledby` cannot
	// drift from the element it names.
	if !strings.HasPrefix(theaterHeadingID("harptos", "ent-1"), a) {
		t.Error("the heading id does not derive from the scaffold id")
	}
	// A slug that is not id-safe cannot break the markup.
	if strings.ContainsAny(theaterScaffoldID("a b/c", "x\"y"), " /\"") {
		t.Error("the scaffold id carries characters that are unsafe in an id attribute")
	}
}

// ── the rendered page ───────────────────────────────────────────────────────

// [TH-2]'s pin (b) — "assertion (b) is the one that would have caught this, and
// it is cheap". Two Blocks for one calendar on one page emit identical ids
// unless the copy is re-namespaced.
func TestTheater_EntityPageEmitsNoDuplicateID(t *testing.T) {
	svc := entityHostSpine(t, blockTenDayCal())
	doc := theaterParse(t, renderEntityCalAs(t, svc, campaigns.RoleOwner, "user-1"))

	seen := map[string]int{}
	theaterWalk(doc, func(n *html.Node) {
		if id, ok := theaterAttr(n, "id"); ok && id != "" {
			seen[id]++
		}
	})
	if len(seen) < 2 {
		t.Fatal("fewer than two ids in the whole page — the parser stopped reading and every assertion here is vacuous")
	}
	for id, n := range seen {
		if n > 1 {
			t.Errorf("id %q appears %d times in the rendered entity page — a duplicate id means a <label for=…> resolves by document order, which is how the theater's tie toggle goes silently dead", id, n)
		}
	}
}

// [TH-2]'s pin (c). A shared radio `name` is worse than a duplicate id: the two
// Blocks' radios join ONE group, two of them carry markup `checked`, and
// pressing the theater's control re-inks the embed behind the backdrop.
func TestTheater_NoRadioNameIsSharedAcrossTheTwoBlocks(t *testing.T) {
	svc := entityHostSpine(t, blockTenDayCal())
	doc := theaterParse(t, renderEntityCalAs(t, svc, campaigns.RoleOwner, "user-1"))

	scaffold := theaterFind(doc, isTheaterScaffold)
	if scaffold == nil {
		t.Fatal("no theater scaffold in the render")
	}
	inTheater, outside := map[string]bool{}, map[string]bool{}
	theaterWalk(doc, func(n *html.Node) {
		if n.Data != "input" {
			return
		}
		if v, _ := theaterAttr(n, "type"); v != "radio" {
			return
		}
		name, ok := theaterAttr(n, "name")
		if !ok || name == "" {
			return
		}
		if theaterHasAncestor(n, isTheaterScaffold) {
			inTheater[name] = true
		} else {
			outside[name] = true
		}
	})
	if len(inTheater) == 0 {
		t.Fatal("the theater's Block emitted no radio at all — with the five-key seed it must, and without one this test proves nothing")
	}
	if len(outside) == 0 {
		t.Fatal("the embed's Block emitted no radio at all — this test needs both sides")
	}
	for name := range inTheater {
		if outside[name] {
			t.Errorf("radio group %q spans both Blocks — four radios in one group with two markup-checked, and the theater's control drives the embed", name)
		}
	}
}

// [TH-14]'s constraints 1 and 2, which the 2026-08-08 re-sign left UNCHANGED.
func TestTheater_ScaffoldSitsOutsideEveryBlockHostAndCarriesTheBenchRoot(t *testing.T) {
	svc := entityHostSpine(t, blockTenDayCal())
	doc := theaterParse(t, renderEntityCalAs(t, svc, campaigns.RoleOwner, "user-1"))

	scaffolds := theaterFindAll(doc, isTheaterScaffold)
	if len(scaffolds) != 1 {
		t.Fatalf("%d theater scaffolds, want exactly 1 ([TH-15]: once per (calendar, entity))", len(scaffolds))
	}
	s := scaffolds[0]

	// Constraint 1: outside every .cal-block-host. `.cal-block-host .block` is
	// overflow:hidden, so a scaffold inside the Block is CLIPPED — the same
	// fact that forces the top layer in the first place.
	if theaterHasAncestor(s, isBlockHost) {
		t.Error("the scaffold sits INSIDE a .cal-block-host — the Block clips it (calendar-block.css:503, overflow:hidden)")
	}
	// Constraint 2: inside a cal-bench root. Top-layer promotion does not
	// change DOM ancestry, so a <dialog> with no `.cal-bench` above or on it
	// gets no disclosure register at all.
	if !(theaterHasClass(s, "cal-bench") || theaterHasAncestor(s, func(n *html.Node) bool { return theaterHasClass(n, "cal-bench") })) {
		t.Error("the scaffold carries no `cal-bench` scope root — the register's every prelude names .cal-bench, so this theater would move with no motion at all")
	}
	if s.Data != "dialog" {
		t.Errorf("the scaffold is a <%s>; [TH-1] SIGNED rules a <dialog> opened with showModal(), for the focus containment and the inertness a 1180px overlay needs and a 340px card did not", s.Data)
	}
	if _, ok := theaterAttr(s, "open"); ok {
		t.Error("the scaffold ships OPEN — a page that opens modal over its own content on load is [TH-5]'s refused shape")
	}
	// The ARIA shape ([TH-4]), and the accessible name really resolves.
	if v, _ := theaterAttr(s, "aria-modal"); v != "true" {
		t.Error("the scaffold is missing aria-modal")
	}
	labelledBy, ok := theaterAttr(s, "aria-labelledby")
	if !ok {
		t.Fatal("the scaffold is missing aria-labelledby — the theater's accessible name comes from its own heading")
	}
	if theaterFind(doc, func(n *html.Node) bool { v, k := theaterAttr(n, "id"); return k && v == labelledBy }) == nil {
		t.Errorf("aria-labelledby=%q names an element that does not exist", labelledBy)
	}
	// [TH-16]: the scroll container is the theater's CONTENT REGION, and it is
	// inside .tbox — never .tbox itself (the register animates .tbox's
	// block-size, and a clamp there animates against a clamped box), and never
	// .cal-block-host (that is the container-query container, and clamping it
	// is a tier hazard dressed as a layout choice).
	scroll := theaterFind(doc, func(n *html.Node) bool { _, k := theaterAttr(n, "data-theater-scroll"); return k })
	if scroll == nil {
		t.Fatal("no content region marked data-theater-scroll")
	}
	if isBlockHost(scroll) {
		t.Error("the scroll container IS the .cal-block-host — [TH-16] refuses it by name")
	}
	if !theaterHasAncestor(scroll, func(n *html.Node) bool { return theaterHasClass(n, "tbox") }) {
		t.Error("the scroll container is not inside .tbox — the reveal's animated box must stay unclamped and the scroll must sit within it")
	}
	if theaterFind(scroll, isBlockHost) == nil {
		t.Error("the theater's Block is not inside the scroll container")
	}
	// The close control sits OUTSIDE the scroll region, which is what makes it
	// reachable without scrolling at the 768px floor and on a phone ([TH-7],
	// [TH-16]).
	closeBtn := theaterFind(doc, func(n *html.Node) bool { _, k := theaterAttr(n, "data-theater-close"); return k })
	if closeBtn == nil {
		t.Fatal("no close control")
	}
	if theaterHasAncestor(closeBtn, func(n *html.Node) bool { _, k := theaterAttr(n, "data-theater-scroll"); return k }) {
		t.Error("the close control is inside the scrolling region — at the 768px viewport floor it would not be reachable without scrolling")
	}
}

// [TH-1] SIGNED, and it is load-bearing rather than tidy: calendar_daycard.js
// rides the same body-script registry this module does, so it is ALREADY LOADED
// on every entity page. It is dormant only because of three markers. Emitting
// any one of them wakes the card on a page that then holds two Blocks with two
// different Ledger states — silently re-opening the SIGNED STOP-AND-FLAG
// C-CALV4-CARD-CROSSBLOCK-LEDGER on a surface it was never measured against.
func TestTheater_EmitsNoneOfTheDayCardsThreeMarkers(t *testing.T) {
	svc := entityHostSpine(t, blockTenDayCal())
	html := renderEntityCalAs(t, svc, campaigns.RoleOwner, "user-1")
	for _, marker := range []string{"data-bench-block", "data-cal-daycard", "data-cal-daycard-payload"} {
		if strings.Contains(html, marker) {
			t.Errorf("the entity page emits %q — the day card's dormancy on this surface is by CONSTRUCTION, and this is the marker that ends it", marker)
		}
	}
}

// [TH-13]: the suppression is a REAL ABSENCE in the render, never CSS hiding. A
// display:none switchboard is a control that exists, is reachable by keyboard
// and by screen reader, and STILL WRITES the preference.
func TestTheater_TheSwitchboardIsARealAbsence(t *testing.T) {
	svc := entityHostSpine(t, blockTenDayCal())
	doc := theaterParse(t, renderEntityCalAs(t, svc, campaigns.RoleOwner, "user-1"))

	popovers := theaterFindAll(doc, func(n *html.Node) bool { _, k := theaterAttr(n, "popover"); return k })
	inside := 0
	for _, p := range popovers {
		if theaterHasAncestor(p, isTheaterScaffold) {
			inside++
		}
	}
	if inside != 0 {
		t.Errorf("%d [popover] elements inside the theater — the layer sheet's id and CSS anchor-name are (CalendarSlug, HostEntity), so the theater's ⋯ would open the EMBED's sheet and anchor to the EMBED's month", inside)
	}
	if len(popovers) == 0 {
		t.Fatal("the EMBED has no [popover] either, so this assertion is vacuous — the embed must keep its switchboard ([TH-13])")
	}
}

// [TH-9]: emitted for EVERY viewer who receives the embed, including anonymous.
// A control present for a GM and absent for a Player would be a permission
// signal where no permission difference exists.
func TestTheater_TheOpenerIsEmittedForEveryViewerThatGetsTheEmbed(t *testing.T) {
	svc := entityHostSpine(t, blockTenDayCal())
	for _, v := range []struct {
		name   string
		role   campaigns.Role
		userID string
	}{
		{"owner", campaigns.RoleOwner, "user-1"},
		{"player", campaigns.RolePlayer, "user-2"},
		{"anonymous", campaigns.RoleNone, ""},
	} {
		doc := theaterParse(t, renderEntityCalAs(t, svc, v.role, v.userID))
		btn := theaterFind(doc, func(n *html.Node) bool { _, k := theaterAttr(n, "data-theater-pick"); return k })
		if btn == nil {
			t.Errorf("%s: no Expand control — the theater shows only what this viewer's own projection already contains, so there is no permission difference to signal", v.name)
			continue
		}
		if btn.Data != "button" {
			t.Errorf("%s: the control is a <%s>; [TH-9] rules a real <button>", v.name, btn.Data)
		}
		if got, _ := theaterAttr(btn, "aria-haspopup"); got != "dialog" {
			t.Errorf("%s: aria-haspopup is %q, want \"dialog\"", v.name, got)
		}
		if got, _ := theaterAttr(btn, "aria-expanded"); got != "false" {
			t.Errorf("%s: the closed theater's opener must report aria-expanded=false, got %q", v.name, got)
		}
		// The button points at the scaffold it actually owns ([TH-15]: an
		// Expand button whose aria-controls resolves to another block's
		// scaffold is a STOP-AND-FLAG).
		controls, _ := theaterAttr(btn, "aria-controls")
		pick, _ := theaterAttr(btn, "data-theater-pick")
		if controls == "" || controls != pick {
			t.Errorf("%s: aria-controls=%q does not match data-theater-pick=%q", v.name, controls, pick)
		}
		if theaterFind(doc, func(n *html.Node) bool { id, k := theaterAttr(n, "id"); return k && id == controls }) == nil {
			t.Errorf("%s: aria-controls=%q resolves to nothing", v.name, controls)
		}
		// The label is `Expand`, not `Full calendar` — no phone container
		// reaches the 900px full-tier floor, so the other word would be false
		// there (§2's third consequence).
		var text strings.Builder
		theaterWalk(btn, func(n *html.Node) {})
		for c := btn.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.TextNode {
				text.WriteString(c.Data)
			}
		}
		if !strings.Contains(text.String(), "Expand") {
			t.Errorf("%s: the control's label is %q, want \"Expand\"", v.name, strings.TrimSpace(text.String()))
		}
		if strings.Contains(text.String(), "Full calendar") {
			t.Errorf("%s: the control promises a tier a phone cannot deliver", v.name)
		}
	}
}

// [TH-2]'s pre-authorised mitigation: no Block to expand means no scaffold, no
// button and no second render. It is also W5a — the theater is emitted from the
// same branch as the Block or not at all, so it can never become a side channel
// that says "there is a calendar here you may not see."
func TestTheater_NoBlockMeansNoScaffoldAndNoButton(t *testing.T) {
	// The spine is absent → the degrade ladder omits the Block.
	prev := BlockSpine()
	InstallBlockSpine(nil)
	t.Cleanup(func() { blockSpine.Store(prev) })

	markup := renderEntityCalAs(t, sampleEmbedSvc(), campaigns.RoleOwner, "user-1")
	if !strings.Contains(markup, "data-entity-calendar") {
		t.Fatal("the embed itself did not render, so this assertion is vacuous")
	}
	for _, gone := range []string{"data-cal-theater", "data-theater-pick", "calendar-theater.css"} {
		if strings.Contains(markup, gone) {
			t.Errorf("with no Block to expand the page still emits %q", gone)
		}
	}
}

// [TH-6]'s separate obligation, which the reflect pin does not imply: the
// theater's seed adds `shelf`, and the Shelf has stub states, so this is the
// first thing on an entity page able to dock one.
func TestTheater_APlayerRenderSaysNeedsBackendZeroTimes(t *testing.T) {
	svc := entityHostSpine(t, blockTenDayCal())
	markup := renderEntityCalAs(t, svc, campaigns.RolePlayer, "user-2")
	if n := strings.Count(strings.ToLower(markup), "needs backend"); n != 0 {
		t.Errorf("a player render of the entity page contains \"needs backend\" %d times, theater included", n)
	}
	if !strings.Contains(markup, "data-cal-theater") {
		t.Fatal("the player got no theater at all, so the count above proves nothing")
	}
}

// ── subtree helpers, shared with entity_calendar_block_test.go ──────────────
//
// THE ENTITY PAGE NOW CARRIES TWO BLOCKS, so a whole-page `strings.Contains`
// can no longer answer "does the EMBED render a Ledger". These two helpers keep
// the older assertions honest by scoping them to a subtree instead of softening
// them — and they make the pair possible: absent HERE, present THERE, which is
// a strictly stronger claim than the single negative it replaces.

func theaterRenderNode(t *testing.T, n *html.Node) string {
	t.Helper()
	var sb strings.Builder
	if err := html.Render(&sb, n); err != nil {
		t.Fatalf("render node: %v", err)
	}
	return sb.String()
}

// entityEmbedSubtree is the EMBED's Block — the `.cal-block-host` that is NOT
// inside the theater's scaffold.
func entityEmbedSubtree(t *testing.T, markup string) string {
	t.Helper()
	doc := theaterParse(t, markup)
	for _, h := range theaterFindAll(doc, isBlockHost) {
		if !theaterHasAncestor(h, isTheaterScaffold) {
			return theaterRenderNode(t, h)
		}
	}
	t.Fatal("no embed Block outside the theater — the glanceable surface is gone")
	return ""
}

// entityTheaterSubtree is the theater scaffold, Block and all.
func entityTheaterSubtree(t *testing.T, markup string) string {
	t.Helper()
	doc := theaterParse(t, markup)
	s := theaterFind(doc, isTheaterScaffold)
	if s == nil {
		t.Fatal("no theater scaffold in the render")
	}
	return theaterRenderNode(t, s)
}
