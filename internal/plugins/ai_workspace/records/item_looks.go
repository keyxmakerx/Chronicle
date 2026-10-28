package records

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// ItemLooksAPI is the item look service as the item-look kind and the
// crafting lookup need it, adapted in app wiring so this package does not
// import the plugin. The service keeps its own rules: the fixed lists of
// looks, colours, sounds and spots, the words' length, and who may see a
// page.
type ItemLooksAPI interface {
	// Get is the page's own look, or nil when it has none.
	Get(ctx context.Context, campaignID, pageID string, a Actor) (*ItemLookRef, error)
	// List is every look on a page the Actor can see, by page name, and
	// whether the campaign has more than were read.
	List(ctx context.Context, campaignID string, a Actor) ([]ItemLookRef, bool, error)
	// Check runs Save's checks without saving.
	Check(ctx context.Context, campaignID, pageID string, v ItemLookValue) error
	// Save replaces the page's look; a value with no look and no MadeFromID
	// clears it.
	Save(ctx context.Context, campaignID, pageID string, a Actor, v ItemLookValue) error
}

// ItemLookValue is a look as the service stores it. Tint is a hex colour.
type ItemLookValue struct {
	Look, Tint, Sound, Words string
	Spots                    map[string]ItemLookSpot
	// MadeFromID is the page the item was made from; it carries that
	// page's look.
	MadeFromID string
}

// ItemLookSpot is one gallery look placed on a spot of the stage.
type ItemLookSpot struct{ Look, Tint string }

// ItemLookRef is a stored look. MadeFrom names the page it was made from,
// or is "" when there is none or the Actor can't see it.
type ItemLookRef struct {
	PageID, Page string
	Value        ItemLookValue
	MadeFrom     string
}

// The fixed lists the look service accepts, with the names the page's Look
// block shows, so the AI can be told its choices. The service is the check;
// TestItemLookChoicesMatchService pins these to it.
var (
	itemLookNames = map[string]string{
		"ember": "Ember strike", "frost": "Frost shard", "storm": "Storm arc",
		"bloom": "Healing bloom", "dawn": "Dawn light", "rot": "Remnant rot",
		"venom": "Venom", "glass": "Glass and starlight", "quiet": "Quiet glint",
	}
	itemLookColors = map[string]string{
		"orange": "#ff7a2f", "gold": "#ffd66b", "green": "#8fe07a", "mint": "#7cf5c8",
		"sky": "#8fd8ff", "violet": "#a99bff", "pink": "#f59ac0", "bone": "#e8dcc0",
	}
	itemLookSounds = []string{"crackle", "ping", "zap", "chime", "bell", "thrum", "bubble", "hiss", "none"}
	itemLookSpots  = []string{"item", "trail", "impact", "body", "ground"}
)

// ItemLookChoices lists the looks, colours, sounds and spots the kind
// writes, for the test that pins them to the service.
func ItemLookChoices() (looks, colors, sounds, spots []string) {
	for k := range itemLookNames {
		looks = append(looks, k)
	}
	for _, v := range itemLookColors {
		colors = append(colors, v)
	}
	sort.Strings(looks)
	sort.Strings(colors)
	return looks, colors, itemLookSounds, itemLookSpots
}

// maxItemLookWords matches the Look block's input.
const maxItemLookWords = 24

// ItemLookKind sets the look an item shows when it is used: one look from
// a fixed gallery, a colour, a sound, words that float up, looks on spots of
// the stage, and the page it was made from (a Remnant, say), whose look it
// carries. Matched by `page`. Only the owner sets looks, as on the page.
type ItemLookKind struct {
	Svc      ItemLooksAPI
	Entities EntityLookup
	// Owners tells the owner from a co-DM, who shares the owner's
	// visibility role but may not set looks.
	Owners CampaignOwnerAPI
}

// CampaignOwnerAPI says whether a user owns the campaign.
type CampaignOwnerAPI interface {
	IsOwner(ctx context.Context, campaignID, userID string) (bool, error)
}

// isOwner is the look route's rule: the campaign's owner, never a co-DM.
func (k ItemLookKind) isOwner(ctx context.Context, campaignID string, a Actor) (bool, error) {
	if a.Role < permissions.RoleOwner || k.Owners == nil {
		return false, nil
	}
	return k.Owners.IsOwner(ctx, campaignID, a.UserID)
}

func (ItemLookKind) Name() string  { return "item-look" }
func (ItemLookKind) Label() string { return "Item look" }
func (ItemLookKind) Doc() string {
	return "How an item shows its magic when it is used, on the page's Look block, matched by `page` (the item's page name). Any page can hold a look; a material's page (a Remnant, say) holds the look the items made from it carry. Keys: " +
		"`look` (" + lookChoiceList() + "), " +
		"`color` (" + colorChoiceList() + "; leave it out for the look's own colour), " +
		"`sound` (" + strings.Join(itemLookSounds, ", ") + "; leave it out for the look's own sound, none is silent), " +
		fmt.Sprintf("`words` (at most %d characters that float up when it is used, for example \"6 fire\"), ", maxItemLookWords) +
		"`spots` (looks placed on spots of the stage: " + strings.Join(itemLookSpots, ", ") + "; each is a look, or `{look, color}`), " +
		"`made_from` (the page it was made from; its look comes along, as the item's look when it has none of its own). " +
		"An update changes only the keys written; `none` takes one away (a spot, the colour, made_from). Removing takes the look off the page. Ask `what: crafting` to see every look.\n\n" +
		"```\n---\nkind: item-look\npage: Flametongue\nlook: ember\ncolor: gold\nwords: 6 fire\nspots:\n  impact: storm\n  ground: {look: ember, color: orange}\nmade_from: Wyrm scale\n---\n```\n\n" +
		"```\n---\nkind: item-look\naction: update\npage: Flametongue\nsound: none\nspots:\n  ground: none\n---\n```"
}

func lookChoiceList() string {
	looks, _, _, _ := ItemLookChoices()
	parts := make([]string, len(looks))
	for i, k := range looks {
		parts[i] = k + " (" + itemLookNames[k] + ")"
	}
	return strings.Join(parts, ", ")
}

func colorChoiceList() string {
	names := make([]string, 0, len(itemLookColors))
	for k := range itemLookColors {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// colorName is the word for a stored colour, or the colour itself.
func colorName(hex string) string {
	for k, v := range itemLookColors {
		if strings.EqualFold(v, hex) {
			return k
		}
	}
	return hex
}

// readColor reads a colour written by name or as one of the hex colours.
func readColor(key, s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if hex, ok := itemLookColors[s]; ok {
		return hex, nil
	}
	for _, v := range itemLookColors {
		if v == s {
			return v, nil
		}
	}
	return "", badRequestf("%s: %q is not one of %s", key, s, colorChoiceList())
}

func readLook(key, s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if _, ok := itemLookNames[s]; ok {
		return s, nil
	}
	// The block shows the long names, so accept those too.
	for k, v := range itemLookNames {
		if strings.EqualFold(v, s) {
			return k, nil
		}
	}
	return "", badRequestf("%s: %q is not a look; the looks are %s", key, s, strings.Join(sortedKeys(itemLookNames), ", "))
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func isNone(s string) bool { return strings.EqualFold(strings.TrimSpace(s), "none") }

// Title names the review row: "Look of Flametongue".
func (ItemLookKind) Title(r Record) string {
	if p := itemLookPage(r); p != "" {
		return "Look of " + p
	}
	return ""
}

func itemLookPage(r Record) string {
	if p := r.Str("page"); p != "" {
		return p
	}
	return r.Name
}

func (k ItemLookKind) page(ctx context.Context, campaignID, name string) *entities.Entity {
	e, err := k.Entities.GetBySlug(ctx, campaignID, entities.Slugify(name))
	if err != nil || e == nil || e.CampaignID != campaignID || !sameName(e.Name, name) {
		return nil
	}
	return e
}

// value is the look the record leaves on the page: the written keys over
// the page's current look. madeFrom is the page `made_from` names, if any.
func (k ItemLookKind) value(ctx context.Context, campaignID string, r Record, cur ItemLookValue) (ItemLookValue, error) {
	v := cur
	if r.Has("look") {
		s := r.Str("look")
		if isNone(s) || s == "" {
			v.Look = ""
		} else {
			l, err := readLook("look", s)
			if err != nil {
				return v, err
			}
			v.Look = l
		}
	}
	if r.Has("color") {
		if s := r.Str("color"); isNone(s) || s == "" {
			v.Tint = ""
		} else {
			c, err := readColor("color", s)
			if err != nil {
				return v, err
			}
			v.Tint = c
		}
	}
	if r.Has("sound") {
		s := strings.ToLower(r.Str("sound"))
		switch {
		case s == "":
			v.Sound = ""
		case containsStr(itemLookSounds, s):
			v.Sound = s
		default:
			return v, badRequestf("sound: %q is not one of %s", s, strings.Join(itemLookSounds, ", "))
		}
	}
	if r.Has("words") {
		w := strings.Join(strings.Fields(r.Str("words")), " ")
		if len([]rune(w)) > maxItemLookWords {
			return v, badRequestf("words: at most %d characters", maxItemLookWords)
		}
		v.Words = w
	}
	if r.Has("spots") {
		spots, err := readSpots(r.Fields["spots"], v.Spots)
		if err != nil {
			return v, err
		}
		v.Spots = spots
	}
	if r.Has("made_from") {
		s := r.Str("made_from")
		if isNone(s) || s == "" {
			v.MadeFromID = ""
		} else {
			src := k.page(ctx, campaignID, s)
			if src == nil {
				return v, badRequestf("made_from: no page called %s", quote(s))
			}
			v.MadeFromID = src.ID
		}
	}
	return v, nil
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// readSpots merges the written spots over the current ones: a spot set to
// none is taken away, the rest are replaced.
func readSpots(raw any, cur map[string]ItemLookSpot) (map[string]ItemLookSpot, error) {
	out := map[string]ItemLookSpot{}
	for k, s := range cur {
		out[k] = s
	}
	if s, ok := raw.(string); ok && isNone(s) {
		return nil, nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, badRequestf("spots: write each spot on its own line, for example `impact: storm`")
	}
	for spot, val := range m {
		key := strings.ToLower(strings.TrimSpace(spot))
		if !containsStr(itemLookSpots, key) {
			return nil, badRequestf("spots: %q is not a spot; the spots are %s", spot, strings.Join(itemLookSpots, ", "))
		}
		var look, color string
		switch t := val.(type) {
		case string:
			look = t
		case map[string]any:
			look, _ = t["look"].(string)
			color, _ = t["color"].(string)
		case nil:
			look = "none"
		default:
			return nil, badRequestf("spots: %s should be a look, or {look, color}", key)
		}
		if isNone(look) {
			delete(out, key)
			continue
		}
		l, err := readLook("spots: "+key, look)
		if err != nil {
			return nil, err
		}
		p := ItemLookSpot{Look: l}
		if color != "" && !isNone(color) {
			if p.Tint, err = readColor("spots: "+key+" color", color); err != nil {
				return nil, err
			}
		}
		out[key] = p
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// lookLine is the review row's line: "Ember strike in gold, words "6 fire"".
func lookLine(v ItemLookValue, madeFrom string) string {
	var parts []string
	if v.Look != "" {
		s := itemLookNames[v.Look]
		if v.Tint != "" {
			s += " in " + colorName(v.Tint)
		}
		parts = append(parts, s)
	}
	if v.Sound != "" {
		parts = append(parts, "sound "+v.Sound)
	}
	if v.Words != "" {
		parts = append(parts, "words "+quote(v.Words))
	}
	if len(v.Spots) > 0 {
		parts = append(parts, plural(len(v.Spots), "spot", "spots"))
	}
	if madeFrom != "" {
		parts = append(parts, "made from "+madeFrom)
	}
	return strings.Join(parts, ", ")
}

// resolve is the page the record names and the look it leaves there.
func (k ItemLookKind) resolve(ctx context.Context, campaignID string, a Actor, r Record) (*entities.Entity, *ItemLookRef, ItemLookValue, error) {
	name := itemLookPage(r)
	e := k.page(ctx, campaignID, name)
	if e == nil {
		return nil, nil, ItemLookValue{}, badRequestf("no page called %s", quote(name))
	}
	cur, err := k.Svc.Get(ctx, campaignID, e.ID, a)
	if err != nil {
		return nil, nil, ItemLookValue{}, err
	}
	var base ItemLookValue
	if cur != nil && r.Action == ActionUpdate {
		base = cur.Value
	}
	v, err := k.value(ctx, campaignID, r, base)
	if err != nil {
		return nil, nil, ItemLookValue{}, err
	}
	if v.MadeFromID == e.ID {
		return nil, nil, ItemLookValue{}, badRequestf("a page can't be made from itself")
	}
	return e, cur, v, nil
}

func (k ItemLookKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	if owner, err := k.isOwner(ctx, campaignID, a); err != nil {
		return Plan{Error: planError(err)}
	} else if !owner {
		return Plan{Error: "only the campaign's owner can set item looks"}
	}
	name := itemLookPage(r)
	if name == "" {
		return Plan{Error: "an item look needs `page`: the item it is on"}
	}
	if k.page(ctx, campaignID, name) == nil && r.Action == ActionCreate && r.Made[entities.Slugify(name)] {
		if _, err := k.value(ctx, campaignID, r, ItemLookValue{}); err != nil {
			return Plan{Error: planError(err)}
		}
		return Plan{Summary: "gives the new page a look", Warnings: []string{quote(name) + " is made in this paste, so its look is checked when it saves"}}
	}
	e, cur, v, err := k.resolve(ctx, campaignID, a, r)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	switch r.Action {
	case ActionCreate:
		if cur != nil {
			return Plan{Error: e.Name + " already has a look; use action: update"}
		}
	case ActionUpdate:
		if cur == nil {
			return Plan{Error: e.Name + " has no look yet; use action: create"}
		}
	default:
		if cur == nil {
			return Plan{Error: e.Name + " has no look to take away"}
		}
		return Plan{Summary: "takes the look off the page"}
	}
	if v.Look == "" && v.MadeFromID == "" {
		return Plan{Error: "it needs `look`, or `made_from` a page whose look it carries; to take the look away, use action: delete"}
	}
	if err := k.Svc.Check(ctx, campaignID, e.ID, v); err != nil {
		return Plan{Error: planError(err)}
	}
	madeFrom := ""
	if v.MadeFromID != "" {
		madeFrom = r.Str("made_from")
		if madeFrom == "" && cur != nil {
			madeFrom = cur.MadeFrom
		}
	}
	s := lookLine(v, madeFrom)
	if r.Action == ActionUpdate {
		s = "now " + s
	}
	return Plan{Summary: s}
}

func (k ItemLookKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	if p := k.Plan(ctx, campaignID, a, r); p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	e, _, v, err := k.resolve(ctx, campaignID, a, r)
	if err != nil {
		return err
	}
	if r.Action == ActionDelete {
		v = ItemLookValue{}
	}
	return k.Svc.Save(ctx, campaignID, e.ID, a, v)
}

// Export lists every look in this kind's own format, for the owner.
func (k ItemLookKind) Export(ctx context.Context, campaignID string, a Actor) (string, error) {
	if owner, err := k.isOwner(ctx, campaignID, a); err != nil || !owner {
		return "", err
	}
	list, _, err := k.Svc.List(ctx, campaignID, a)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, l := range list {
		b.WriteString(itemLookBlock(l))
		b.WriteString("\n")
	}
	return b.String(), nil
}

// itemLookBlock writes one look as an item-look block.
func itemLookBlock(l ItemLookRef) string {
	var b strings.Builder
	b.WriteString("---\nkind: item-look\npage: " + quote(l.Page) + "\n")
	v := l.Value
	if v.Look != "" {
		b.WriteString("look: " + v.Look + "\n")
	}
	if v.Tint != "" {
		b.WriteString("color: " + colorName(v.Tint) + "\n")
	}
	if v.Sound != "" {
		b.WriteString("sound: " + v.Sound + "\n")
	}
	if v.Words != "" {
		b.WriteString("words: " + quote(v.Words) + "\n")
	}
	if len(v.Spots) > 0 {
		b.WriteString("spots:\n")
		for _, spot := range itemLookSpots {
			p, ok := v.Spots[spot]
			if !ok {
				continue
			}
			if p.Tint == "" {
				b.WriteString("  " + spot + ": " + p.Look + "\n")
			} else {
				b.WriteString("  " + spot + ": {look: " + p.Look + ", color: " + colorName(p.Tint) + "}\n")
			}
		}
	}
	if l.MadeFrom != "" {
		b.WriteString("made_from: " + quote(l.MadeFrom) + "\n")
	}
	b.WriteString("---\n")
	return b.String()
}

// pageLookText is the page's look as a block, for the `page` answer.
func (run *lookupRun) pageLookText(e *entities.Entity) string {
	if run.l.Looks == nil {
		return ""
	}
	l, err := run.l.Looks.Get(run.ctx, run.campaignID, e.ID, run.a)
	if err != nil || l == nil {
		return ""
	}
	return "\n\n### Look\n\nHow it shows its magic when used.\n\n```\n" + itemLookBlock(*l) + "```\n"
}
