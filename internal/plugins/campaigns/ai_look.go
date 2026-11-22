package campaigns

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// AILook is the campaign's look as the AI engine sees it (aiplug): the `look`
// block that changes the Customizer's Look choices and the `campaign-look`
// lookup that reads them back. A block is a partial update over the one look
// every campaign has, and is stored through SaveAppearance, the call the Look
// screen's Save makes, so every value passes the screen's own checks.
type AILook struct {
	store  aiLookStore
	drafts AILookDrafts
}

// aiLookStore is the campaign service as the look needs it.
type aiLookStore interface {
	GetByID(ctx context.Context, id string) (*Campaign, error)
	GetMember(ctx context.Context, campaignID, userID string) (*CampaignMember, error)
	SaveAppearance(ctx context.Context, campaignID string, in AppearanceInput) error
}

// AILookDrafts reads the owner's unpublished Customize draft. It may be nil
// (no drafts to protect).
type AILookDrafts interface {
	Get(ctx context.Context, campaignID, userID string) (*CustomizeDraft, error)
}

// NewAILook builds the look's AI part over the campaign service and the
// Customize draft store.
func NewAILook(store aiLookStore, drafts AILookDrafts) *AILook {
	return &AILook{store: store, drafts: drafts}
}

var (
	_ aiplug.Kind    = (*AILook)(nil)
	_ aiplug.Briefed = (*AILook)(nil)
	_ aiplug.Titled  = (*AILook)(nil)
	_ aiplug.Lookup  = (*AILook)(nil)
)

// lookKey is one key of a look block. The allowed lists are the ones the
// save validates against, so the doc and the check cannot drift apart. get
// reads the screen's own state for the key, which is also what a lookup
// writes and what a plan compares before and after.
type lookKey struct {
	key, words string
	allowed    []string // nil for a colour, a flag or text
	colour     bool
	flag       bool
	none       string // how an empty value is worded
	get        func(d *customizeDraft) string
	set        func(in *AppearanceInput, v string)
	note       string // appended to the key's line in the doc
}

func flagWord(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func choiceKey(key, words string, allowed []string, get func(d *customizeDraft) string, set func(in *AppearanceInput, v string)) lookKey {
	return lookKey{key: key, words: words, allowed: allowed, get: get, set: set}
}

func flagKey(key, words string, get func(d *customizeDraft) bool, set func(in *AppearanceInput, v bool)) lookKey {
	return lookKey{key: key, words: words, flag: true,
		get: func(d *customizeDraft) string { return flagWord(get(d)) },
		set: func(in *AppearanceInput, v string) { set(in, v == "true") }}
}

// lookDmKits are the DM Screen kit's values: the pages' kit or a kit of its own.
var lookDmKits = append([]string{"same"}, AppearanceStyleKits...)

// lookKeys are the keys of a look block in the order the doc and the lookup
// list them. Pictures, the brand's words, the header and who may pick their
// own look are not here: a block never uploads a file, and those parts are
// set on the screens.
var lookKeys = []lookKey{
	choiceKey("look", "look", AppearanceLooks, func(d *customizeDraft) string { return d.Look }, func(in *AppearanceInput, v string) { in.Look = v }),
	{key: "accent", words: "menus and links colour", colour: true, none: "the Classic colour",
		get: func(d *customizeDraft) string { return d.Colours.Accent }, set: func(in *AppearanceInput, v string) { in.Colours.Accent = v },
		note: "a colour written #RRGGBB, toned down as the colour picker does; one close to the Director's amber is refused"},
	{key: "headers_colour", words: "headers and events colour", colour: true, none: "the menus and links colour",
		get: func(d *customizeDraft) string { return deref(d.Colours.S1) }, set: func(in *AppearanceInput, v string) { in.Colours.S1 = v },
		note: "#RRGGBB, or an empty value to follow the menus and links colour"},
	{key: "tags_colour", words: "tags and marks colour", colour: true, none: "the menus and links colour",
		get: func(d *customizeDraft) string { return deref(d.Colours.S2) }, set: func(in *AppearanceInput, v string) { in.Colours.S2 = v },
		note: "#RRGGBB, or an empty value to follow the menus and links colour"},
	choiceKey("page_tone", "page tone", AppearancePageTones, func(d *customizeDraft) string { return d.Colours.Page }, func(in *AppearanceInput, v string) { in.Colours.Page = v }),
	choiceKey("contrast", "text contrast", AppearanceContrasts, func(d *customizeDraft) string { return d.Colours.Contrast }, func(in *AppearanceInput, v string) { in.Colours.Contrast = v }),
	choiceKey("body_font", "text font", AppearanceBodyFonts, func(d *customizeDraft) string { return d.Type.Body }, func(in *AppearanceInput, v string) { in.Type.Body = v }),
	choiceKey("heading_font", "heading font", AppearanceHeadingFonts, func(d *customizeDraft) string { return d.Type.Heading }, func(in *AppearanceInput, v string) { in.Type.Heading = v }),
	choiceKey("type_scale", "text size", AppearanceTypeScales, func(d *customizeDraft) string { return d.Type.Scale }, func(in *AppearanceInput, v string) { in.Type.Scale = v }),
	choiceKey("nav_style", "menu highlight", AppearanceNavStyles, func(d *customizeDraft) string { return d.Nav.Style }, func(in *AppearanceInput, v string) { in.Nav.Style = v }),
	choiceKey("nav_strength", "highlight strength", AppearanceNavStrengths, func(d *customizeDraft) string { return d.Nav.Strength }, func(in *AppearanceInput, v string) { in.Nav.Strength = v }),
	choiceKey("nav_page_name", "page name in the menu", AppearanceNavPageNames, func(d *customizeDraft) string { return d.Nav.PageName }, func(in *AppearanceInput, v string) { in.Nav.PageName = v }),
	choiceKey("button_style", "button style", AppearanceButtonStyles, func(d *customizeDraft) string { return d.Buttons.Style }, func(in *AppearanceInput, v string) { in.Buttons.Style = v }),
	choiceKey("icon_amount", "icon amount", AppearanceIconAmounts, func(d *customizeDraft) string { return d.Icons.Amount }, func(in *AppearanceInput, v string) { in.Icons.Amount = v }),
	choiceKey("icon_colour", "icon colour", AppearanceIconColours, func(d *customizeDraft) string { return d.Icons.Colour }, func(in *AppearanceInput, v string) { in.Icons.Colour = v }),
	choiceKey("elevation", "depth", AppearanceElevations, func(d *customizeDraft) string { return d.Motion.Elevation }, func(in *AppearanceInput, v string) { in.Motion.Elevation = v }),
	choiceKey("motion_speed", "motion speed", AppearanceSpeeds, func(d *customizeDraft) string { return d.Motion.Speed }, func(in *AppearanceInput, v string) { in.Motion.Speed = v }),
	flagKey("reduce_motion", "calmer motion for everyone", func(d *customizeDraft) bool { return d.Motion.ReduceAll }, func(in *AppearanceInput, v bool) { in.Motion.ReduceAll = v }),
	choiceKey("entrance", "card entrance", AppearanceEntrances, func(d *customizeDraft) string { return d.Motion.Entrance }, func(in *AppearanceInput, v string) { in.Motion.Entrance = v }),
	flagKey("moment_sheen", "sheen across a page header", func(d *customizeDraft) bool { return d.Motion.Sheen }, func(in *AppearanceInput, v bool) { in.Motion.Sheen = v }),
	flagKey("moment_cascade", "cards staggering in", func(d *customizeDraft) bool { return d.Motion.Cascade }, func(in *AppearanceInput, v bool) { in.Motion.Cascade = v }),
	flagKey("moment_spark", "sparks on a save", func(d *customizeDraft) bool { return d.Motion.Spark }, func(in *AppearanceInput, v bool) { in.Motion.Spark = v }),
	choiceKey("header_height", "header height", AppearanceHeights, func(d *customizeDraft) string { return d.Header.Height }, func(in *AppearanceInput, v string) { in.Header.Height = v }),
	choiceKey("sidebar_colour", "menu colour", AppearanceSidebars, func(d *customizeDraft) string { return d.Colours.Sidebar }, func(in *AppearanceInput, v string) { in.Sidebar.Colour = v }),
	{key: "sidebar_own", words: "your own menu colour", colour: true, none: "none",
		get: func(d *customizeDraft) string {
			if d.Colours.Sidebar == "own" {
				return d.Sidebar.Own
			}
			return ""
		},
		set:  func(in *AppearanceInput, v string) { in.Sidebar.Own = v },
		note: "#RRGGBB; needed when `sidebar_colour` is own, and kept only then"},
	choiceKey("sidebar_corner", "menu corner", AppearanceCorners, func(d *customizeDraft) string { return d.Sidebar.Corner }, func(in *AppearanceInput, v string) { in.Sidebar.Corner = v }),
	{key: "sidebar_subtitle", words: "menu subtitle", none: "none",
		get: func(d *customizeDraft) string {
			if d.Sidebar.Corner == "subtitle" {
				return d.Sidebar.Subtitle
			}
			return ""
		},
		set:  func(in *AppearanceInput, v string) { in.Sidebar.Subtitle = v },
		note: "one short line of text, at most 40 characters; kept only when `sidebar_corner` is subtitle"},
	choiceKey("peek_glow", "hidden-menu edge glow", AppearanceGlows, func(d *customizeDraft) string { return d.Sidebar.Glow }, func(in *AppearanceInput, v string) { in.Sidebar.Glow = v }),
	{key: "peek_glow_colour", words: "edge glow colour", colour: true, none: "none",
		get: func(d *customizeDraft) string {
			if d.Sidebar.Glow == "own" {
				return d.Sidebar.GlowColour
			}
			return ""
		},
		set:  func(in *AppearanceInput, v string) { in.Sidebar.GlowColour = v },
		note: "#RRGGBB; needed when `peek_glow` is own, and kept only then"},
	choiceKey("hover_card", "hover card", AppearanceHoverCards, func(d *customizeDraft) string { return d.Hover.Look }, func(in *AppearanceInput, v string) { in.Hover.Look = v }),
	choiceKey("timeline_popup", "time line popup", AppearanceTimelinePopups, func(d *customizeDraft) string { return d.Hover.TimelinePopup }, func(in *AppearanceInput, v string) { in.Hover.TimelinePopup = v }),
	choiceKey("sheet_style", "character sheet style", AppearanceSheetStyles, func(d *customizeDraft) string { return d.Sheet.Style }, func(in *AppearanceInput, v string) { in.Sheet.Style = v }),
	choiceKey("craft_art", "made item pictures", AppearanceCraftArts, func(d *customizeDraft) string { return d.Sheet.CraftArt }, func(in *AppearanceInput, v string) { in.Sheet.CraftArt = v }),
	{key: "page_look", words: "page look", allowed: AppearancePageLooks, none: "not chosen yet",
		get: func(d *customizeDraft) string { return d.Page.Look }, set: func(in *AppearanceInput, v string) { in.Page.Look = v },
		note: "the lookup leaves it out until the owner has chosen one"},
	choiceKey("style_kit", "style kit", AppearanceStyleKits, func(d *customizeDraft) string { return d.Page.Kit }, func(in *AppearanceInput, v string) { in.Page.Kit = v }),
	choiceKey("dmscreen_kit", "DM Screen kit", lookDmKits, func(d *customizeDraft) string { return d.Page.DmKit }, func(in *AppearanceInput, v string) { in.Page.DmKit = v }),
	choiceKey("email_layout", "email layout", AppearanceEmailLayouts, func(d *customizeDraft) string { return d.Email.Layout }, func(in *AppearanceInput, v string) { in.Email.Layout = v }),
	choiceKey("builder_look", "hero builder look", AppearanceBuilderLooks, func(d *customizeDraft) string { return d.Builder.Look }, func(in *AppearanceInput, v string) { in.Builder.Look = v }),
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// lookMeta are keys every block may carry that are not look settings.
var lookMeta = map[string]bool{"kind": true, "action": true, "name": true, "reason": true}

func (*AILook) Name() string  { return "look" }
func (*AILook) Label() string { return "Look" }
func (*AILook) Brief() string {
	return "the campaign's look: which look it wears, colours, fonts, menu, buttons, depth, motion, sheet, page and style kit (owner only)"
}
func (*AILook) Title(aiplug.Record) string { return "the campaign's look" }

// Doc lists every key with the values the save accepts, built from the same
// lists the save checks.
func (*AILook) Doc() string {
	var b strings.Builder
	b.WriteString("The campaign's look, the choices in Customize › Look. There is one look per campaign, so there is nothing to create or remove, and `action: delete` is refused. Only the keys you write change; a key you leave out keeps its value. Every value is checked as the Look screen's Save checks it, and the change takes effect for every member as soon as the owner approves it. Pictures and files are not set from here, nor the brand's words, the header or who may pick their own look. Only the campaign's owner can change it; `what: campaign-look` writes the current values as this block. Keys:\n")
	for _, k := range lookKeys {
		b.WriteString("- `" + k.key + "`: " + k.words + "; ")
		switch {
		case k.flag:
			b.WriteString("true or false")
		case k.allowed != nil:
			b.WriteString("one of " + strings.Join(k.allowed, ", "))
		}
		if k.note != "" {
			if k.allowed != nil || k.flag {
				b.WriteString("; ")
			}
			b.WriteString(k.note)
		}
		b.WriteString("\n")
	}
	b.WriteString("\n```\n---\nkind: look\nlook: midnight\naccent: \"#7a5cff\"\nbody_font: literata\nsidebar_colour: ink\n---\n```")
	return b.String()
}

// isOwner is the owner acting as the owner. An export for what players see
// carries the owner's id with a player's role, so the role gate comes first
// and membership second, as the menu and home kinds check it.
func (k *AILook) isOwner(ctx context.Context, campaignID string, a aiplug.Actor) (bool, error) {
	if !a.CanAuthorDmOnly() || a.UserID == "" {
		return false, nil
	}
	m, err := k.store.GetMember(ctx, campaignID, a.UserID)
	if err != nil {
		if apperror.SafeCode(err) == 404 {
			return false, nil
		}
		return false, err
	}
	return m.Role >= RoleOwner, nil
}

// lookState is the screen's own reading of a campaign's look, for settings
// that may not be saved yet.
func lookState(ctx context.Context, c *Campaign, s CampaignSettings, backdrop *string) (customizeDraft, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return customizeDraft{}, apperror.NewInternal(fmt.Errorf("marshaling settings: %w", err))
	}
	cp := *c
	cp.Settings = string(raw)
	cp.BackdropPath = backdrop
	return buildCustomizeState(ctx, &CampaignContext{Campaign: &cp}).Draft, nil
}

// appearanceInputOf writes stored settings as the whole Save the Look screen
// would send for them unchanged, which is what SaveAppearance takes: it has
// no partial form, so a block's few keys are laid over this.
func appearanceInputOf(s CampaignSettings, backdrop *string) AppearanceInput {
	var a Appearance
	if s.Appearance != nil {
		a = *s.Appearance
	}
	var in AppearanceInput
	in.Look = a.Look
	in.Brand = AppearanceBrandInput{Name: s.BrandName, Logo: s.BrandLogo, Welcome: s.WelcomeMessage}
	if backdrop != nil {
		in.Brand.Backdrop = *backdrop
	}
	h := &in.Header
	h.Height = a.HeaderHeight
	if ts := s.TopbarStyle; ts != nil {
		switch ts.Mode {
		case "solid":
			h.Bg, h.Color = "solid", ts.Color
		case "gradient", "moving":
			h.Bg, h.From, h.To, h.Dir = ts.Mode, ts.GradientFrom, ts.GradientTo, ts.GradientDir
		case "sky":
			h.Bg = "sky"
		case "image":
			h.Bg, h.Image, h.Scrim = "image", ts.ImagePath, ts.Scrim
		}
	}
	if tc := s.TopbarContent; tc != nil {
		h.Widgets = (&layouts.TopbarContentData{Mode: tc.Mode, Widgets: tc.Widgets}).TopbarWidgets()
		h.Links = append([]TopbarLink(nil), tc.Links...)
		h.Text = tc.Quote
	}
	in.Colours = AppearanceColourInput{Accent: s.AccentColor, S1: s.AccentSurface1, S2: s.AccentSurface2, Page: a.PageTone, Contrast: a.Contrast}
	if in.Colours.S1 == "" {
		in.Colours.S1 = s.AccentApp
	}
	// The screen sends the highlight it shows, so a change of look does not
	// swap it for the new look's default.
	in.Nav.Style, in.Nav.Strength, in.Nav.PageName = a.EffectiveNavStyle(), a.NavStrength, a.NavPageName
	in.Type.Body, in.Type.Heading, in.Type.Scale = a.BodyFont, a.HeadingFont, a.TypeScale
	if in.Type.Body == "" {
		in.Type.Body = legacyBodyFonts[s.FontFamily]
	}
	in.Buttons.Style = a.ButtonStyle
	in.Icons.Amount, in.Icons.Colour = a.IconAmount, a.IconColour
	in.Motion.Elevation, in.Motion.Speed, in.Motion.ReduceAll = a.Elevation, a.MotionSpeed, a.ReduceMotion
	in.Motion.Entrance = a.Entrance
	in.Motion.Sheen, in.Motion.Cascade, in.Motion.Spark = a.MomentSheen, a.MomentCascade, a.MomentSpark
	in.Sidebar = AppearanceSidebarInput{Colour: a.SidebarColour, Own: a.SidebarOwn, Corner: a.SidebarCorner,
		Subtitle: a.SidebarSubtitle, Banner: a.SidebarBanner, Glow: a.PeekGlow, GlowColour: a.PeekGlowColour}
	in.Hover.Look, in.Hover.TimelinePopup = a.HoverCard, a.TimelinePopup
	in.Sheet.Style, in.Sheet.CraftArt = a.SheetStyle, a.CraftArt
	in.Page.Look, in.Page.Kit, in.Page.DmKit = a.PageLook, a.StyleKit, a.DmScreenKit
	in.Email.Layout = a.EmailLayout
	in.Builder.Look = a.BuilderLook
	return in
}

// lookChange is a block resolved against the stored look.
type lookChange struct {
	in       AppearanceInput
	summary  []string // one phrase per key that changes
	warnings []string
}

// value reads a key's written value, checked against its list.
func lookValue(k lookKey, r aiplug.Record) (string, error) {
	switch {
	case k.flag:
		switch s := strings.ToLower(r.Str(k.key)); s {
		case "true", "yes", "on":
			return "true", nil
		case "false", "no", "off":
			return "false", nil
		default:
			return "", aiplug.BadRequestf("%s: write true or false", k.key)
		}
	case k.allowed != nil:
		v := strings.ToLower(r.Str(k.key))
		for _, a := range k.allowed {
			if v == a {
				return v, nil
			}
		}
		return "", aiplug.BadRequestf("%s: %q is not one of %s", k.key, r.Str(k.key), strings.Join(k.allowed, ", "))
	}
	return r.Str(k.key), nil
}

func lookWords(k lookKey, v string) string {
	switch {
	case v == "" && k.none != "":
		return k.none
	case k.flag && v == "true":
		return "on"
	case k.flag:
		return "off"
	}
	return v
}

// resolve checks a block against the stored look and says what it changes.
// Plan and Apply both run it, so nothing the review showed is trusted.
func (k *AILook) resolve(ctx context.Context, campaignID string, a aiplug.Actor, r aiplug.Record) (lookChange, error) {
	var ch lookChange
	if r.Action == aiplug.ActionDelete {
		return ch, aiplug.BadRequestf("the look can't be removed; write the keys to change instead")
	}
	owner, err := k.isOwner(ctx, campaignID, a)
	if err != nil {
		return ch, err
	}
	if !owner {
		return ch, aiplug.BadRequestf("only the campaign's owner can change the look")
	}
	known := map[string]bool{}
	var names []string
	for _, lk := range lookKeys {
		known[lk.key] = true
		names = append(names, lk.key)
	}
	for key := range r.Fields {
		if !known[key] && !lookMeta[key] {
			return ch, aiplug.BadRequestf("%q is not a key of the look; the keys are %s", key, strings.Join(names, ", "))
		}
	}

	c, err := k.store.GetByID(ctx, campaignID)
	if err != nil {
		return ch, err
	}
	s := c.ParseSettings()
	before, err := lookState(ctx, c, s, c.BackdropPath)
	if err != nil {
		return ch, err
	}
	ch.in = appearanceInputOf(s, c.BackdropPath)
	var written []lookKey
	for _, lk := range lookKeys {
		if !r.Has(lk.key) {
			continue
		}
		v, err := lookValue(lk, r)
		if err != nil {
			return ch, err
		}
		lk.set(&ch.in, v)
		written = append(written, lk)
	}

	// The save's own validator, on a copy: its refusals and its toning of
	// colours are the ones the screen gets.
	next := s
	backdrop, err := applyAppearance(&next, ch.in)
	if err != nil {
		return ch, err
	}
	after, err := lookState(ctx, c, next, backdrop)
	if err != nil {
		return ch, err
	}
	for _, lk := range written {
		was, now := lk.get(&before), lk.get(&after)
		if strings.EqualFold(was, now) {
			continue
		}
		ch.summary = append(ch.summary, fmt.Sprintf("%s from %s to %s", lk.words, lookWords(lk, was), lookWords(lk, now)))
	}
	for _, lk := range written {
		if lk.key == "sidebar_own" || lk.key == "peek_glow_colour" || lk.key == "sidebar_subtitle" {
			if lk.get(&after) == "" && r.Str(lk.key) != "" {
				ch.warnings = append(ch.warnings, lk.key+" is kept only while the choice that uses it is selected")
			}
		}
	}
	return ch, nil
}

// checkNoDraft refuses a change while the owner has a look waiting in
// Customize: the draft carries a whole look, so publishing it later would
// overwrite this change without a word.
func (k *AILook) checkNoDraft(ctx context.Context, campaignID string, a aiplug.Actor) error {
	if k.drafts == nil {
		return nil
	}
	d, err := k.drafts.Get(ctx, campaignID, a.UserID)
	if err != nil {
		return err
	}
	if d == nil || len(d.Draft) == 0 {
		return nil
	}
	sections, err := parseCustomizeDraft(d.Draft)
	if err != nil {
		return err
	}
	if _, waiting := sections[customizeSectionLook]; waiting {
		return aiplug.BadRequestf("you have a look waiting in Customize that is not published yet; publish or discard that draft first, then ask again")
	}
	return nil
}

// Plan checks a block for the review screen; it never writes.
func (k *AILook) Plan(ctx context.Context, campaignID string, a aiplug.Actor, r aiplug.Record) aiplug.Plan {
	ch, err := k.resolve(ctx, campaignID, a, r)
	if err != nil {
		return aiplug.Plan{Error: aiplug.PlanError(err)}
	}
	if len(ch.summary) == 0 {
		return aiplug.Plan{Summary: "no change", Warnings: ch.warnings}
	}
	if err := k.checkNoDraft(ctx, campaignID, a); err != nil {
		return aiplug.Plan{Error: aiplug.PlanError(err)}
	}
	return aiplug.Plan{
		Summary:  "the look changes: " + strings.Join(ch.summary, ", "),
		Warnings: append([]string{"This changes how the site looks for every member as soon as you approve it."}, ch.warnings...),
	}
}

// Apply saves the look through SaveAppearance, the Look screen's own save,
// after checking the block again against the look as it is now.
func (k *AILook) Apply(ctx context.Context, campaignID string, a aiplug.Actor, r aiplug.Record) error {
	ch, err := k.resolve(ctx, campaignID, a, r)
	if err != nil {
		return err
	}
	if len(ch.summary) == 0 {
		return nil
	}
	if err := k.checkNoDraft(ctx, campaignID, a); err != nil {
		return err
	}
	return k.store.SaveAppearance(ctx, campaignID, ch.in)
}

// Export is empty: the look is long and `what: campaign-look` answers it on
// request, so it does not ride along in every export.
func (*AILook) Export(context.Context, string, aiplug.Actor) (string, error) { return "", nil }

// LookupInfo describes `what: campaign-look` and its tool. It is not
// `looks`, which is the Armory's item looks.
func (*AILook) LookupInfo() aiplug.LookupInfo {
	return aiplug.LookupInfo{
		What: "campaign-look", Aliases: []string{"site-look"}, Keys: "none",
		Brief: "the campaign's look as set in Customize › Look, written as the `look` block that would send it back (owner only)",
		Tool: aiplug.Tool{
			Name: "campaign_look", Title: "The campaign's look", Did: "Looked at the campaign's look", Owner: true,
			Desc: "The campaign's look (its look, colours, fonts, menu, buttons, depth, motion, sheet and page styles), written as the look block that would send it back.",
		},
	}
}

// Answer writes the look as a block. It is owner-only like the block, and a
// players-view key never gets it.
func (k *AILook) Answer(ctx context.Context, campaignID string, a aiplug.Actor, _ aiplug.Record) (string, string, error) {
	chip := "The look"
	owner, err := k.isOwner(ctx, campaignID, a)
	if err != nil {
		return chip, "", err
	}
	if !owner {
		return chip, "", aiplug.BadRequestf("only the campaign's owner can look at the look")
	}
	c, err := k.store.GetByID(ctx, campaignID)
	if err != nil {
		return chip, "", err
	}
	d, err := lookState(ctx, c, c.ParseSettings(), c.BackdropPath)
	if err != nil {
		return chip, "", err
	}
	var b strings.Builder
	b.WriteString("## The look\n\nSend this `kind: look` block back with only the keys to change; a key left out keeps its value. A page look the owner has not chosen yet is left out.\n\n```\n---\nkind: look\n")
	for _, lk := range lookKeys {
		v := lk.get(&d)
		switch {
		case lk.key == "page_look" && v == "":
			continue
		case (lk.key == "sidebar_own" || lk.key == "peek_glow_colour" || lk.key == "sidebar_subtitle") && v == "":
			continue
		case lk.colour || lk.key == "sidebar_subtitle":
			v = aiplug.Quote(v)
		}
		b.WriteString(lk.key + ": " + v + "\n")
	}
	b.WriteString("---\n```\n")
	return chip, b.String(), nil
}
