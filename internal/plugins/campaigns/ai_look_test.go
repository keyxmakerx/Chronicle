package campaigns

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// fakeLookStore holds one campaign and saves through the real validator, so
// a save here stores what the screen's save would.
type fakeLookStore struct {
	c       *Campaign
	roles   map[string]Role
	saves   int
	saveErr error
}

func (f *fakeLookStore) GetByID(context.Context, string) (*Campaign, error) {
	cp := *f.c
	return &cp, nil
}

func (f *fakeLookStore) GetMember(_ context.Context, _, userID string) (*CampaignMember, error) {
	r, ok := f.roles[userID]
	if !ok {
		return nil, apperror.NewNotFound("no such member")
	}
	return &CampaignMember{UserID: userID, Role: r}, nil
}

func (f *fakeLookStore) SaveAppearance(_ context.Context, _ string, in AppearanceInput) error {
	f.saves++
	if f.saveErr != nil {
		return f.saveErr
	}
	s := f.c.ParseSettings()
	backdrop, err := applyAppearance(&s, in)
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(s)
	f.c.Settings = string(raw)
	f.c.BackdropPath = backdrop
	return nil
}

type fakeLookDrafts struct{ draft string }

func (f fakeLookDrafts) Get(context.Context, string, string) (*CustomizeDraft, error) {
	if f.draft == "" {
		return nil, nil
	}
	return &CustomizeDraft{Version: 2, Draft: json.RawMessage(f.draft)}, nil
}

func lookFixture(t *testing.T, s CampaignSettings) *fakeLookStore {
	t.Helper()
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return &fakeLookStore{
		c:     &Campaign{ID: "c1", Name: "Therin", Settings: string(raw)},
		roles: map[string]Role{"own": RoleOwner, "scribe": RoleScribe, "pl": RolePlayer},
	}
}

func lookRec(action string, kv ...any) aiplug.Record {
	f := map[string]any{}
	for i := 0; i < len(kv); i += 2 {
		f[kv[i].(string)] = kv[i+1]
	}
	return aiplug.Record{Kind: "look", Action: action, Fields: f}
}

var (
	lookOwner       = aiplug.Actor{UserID: "own", Role: 3}
	lookPlayersView = aiplug.Actor{UserID: "own", Role: 1}
)

func richSettings() CampaignSettings {
	return CampaignSettings{
		BrandName:      "Therin",
		AccentColor:    "#7a5cff",
		AccentSurface1: "#3b82a0",
		TopbarStyle:    &TopbarStyle{Mode: "gradient", GradientFrom: "#0f172a", GradientTo: "#1e2a5a", GradientDir: "to-br"},
		TopbarContent:  &TopbarContent{Mode: "widgets", Widgets: []string{"links", "date"}, Links: []TopbarLink{{Label: "Wiki", URL: "https://example.com/wiki"}}},
		Appearance: &Appearance{
			Look: "midnight", NavStyle: "tab", BodyFont: "literata", HeadingFont: "cinzel", ButtonStyle: "glow",
			Elevation: "flat", SidebarColour: "own", SidebarOwn: "#1a2a22", SidebarCorner: "subtitle", SidebarSubtitle: "Of Therin",
			PeekGlow: "own", PeekGlowColour: "#3b9fb5", HoverCard: "night", SheetStyle: "ledger", PageLook: "annals",
			StyleKit: "ornate", Entrance: "bloom", MomentSheen: true, ReduceMotion: true,
		},
	}
}

// Writing the stored look as the whole save the screen would send must change
// nothing the screen shows, or a block's few keys would drag the rest along.
func TestAppearanceInputOfKeepsTheLook(t *testing.T) {
	for name, s := range map[string]CampaignSettings{
		"never customized": {},
		"rich":             richSettings(),
		"older font":       {FontFamily: "georgia", AccentApp: "#336699"},
	} {
		t.Run(name, func(t *testing.T) {
			st := lookFixture(t, s)
			c, _ := st.GetByID(context.Background(), "c1")
			before, err := lookState(context.Background(), c, c.ParseSettings(), c.BackdropPath)
			if err != nil {
				t.Fatal(err)
			}
			next := c.ParseSettings()
			backdrop, err := applyAppearance(&next, appearanceInputOf(next, c.BackdropPath))
			if err != nil {
				t.Fatal(err)
			}
			after, err := lookState(context.Background(), c, next, backdrop)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Errorf("look changed on a save of itself:\nbefore %+v\nafter  %+v", before, after)
			}
		})
	}
}

func TestAILookPlan(t *testing.T) {
	tests := []struct {
		name    string
		actor   aiplug.Actor
		rec     aiplug.Record
		summary string
		err     string
		warn    string
	}{
		{"changes keys", lookOwner, lookRec("update", "look", "forest", "body_font", "Lora", "reduce_motion", false),
			"the look changes: look from midnight to forest, text font from literata to lora, calmer motion for everyone from on to off", "", "every member"},
		{"create reads as update", lookOwner, lookRec("create", "button_style", "ink"), "button style from glow to ink", "", ""},
		{"same value is no change", lookOwner, lookRec("update", "look", "midnight", "accent", "#7A5CFF"), "no change", "", ""},
		{"colour is toned like the screen's", lookOwner, lookRec("update", "tags_colour", "#ff0000"), "tags and marks colour from the menus and links colour to #", "", ""},
		{"empty colour follows the accent", lookOwner, lookRec("update", "headers_colour", ""), "headers and events colour from #3b82a0 to the menus and links colour", "", ""},
		{"page look by name", lookOwner, lookRec("update", "page_look", "standard"), "page look from annals to standard", "", ""},
		{"companion kept only while selected", lookOwner, lookRec("update", "sidebar_colour", "ink", "sidebar_own", "#112233"), "menu colour from own to ink", "", "sidebar_own is kept only"},
		{"own colour needs one", lookOwner, lookRec("update", "peek_glow_colour", "#abcdef", "sidebar_colour", "own", "sidebar_own", ""), "", "needs a colour", ""},
		{"a co-DM is not the owner", aiplug.Actor{UserID: "scribe", Role: 3}, lookRec("update", "look", "forest"), "", "only the campaign's owner", ""},
		{"a player is not the owner", aiplug.Actor{UserID: "pl", Role: 1}, lookRec("update", "look", "forest"), "", "only the campaign's owner", ""},
		{"players-view key with the owner's id", lookPlayersView, lookRec("update", "look", "forest"), "", "only the campaign's owner", ""},
		{"stranger", aiplug.Actor{UserID: "nobody", Role: 3}, lookRec("update", "look", "forest"), "", "only the campaign's owner", ""},
		{"unknown key", lookOwner, lookRec("update", "logo", "x.png"), "", `"logo" is not a key of the look`, ""},
		{"value outside the list", lookOwner, lookRec("update", "look", "neon"), "", `look: "neon" is not one of classic, parchment, midnight`, ""},
		{"flag that is not one", lookOwner, lookRec("update", "reduce_motion", "maybe"), "", "write true or false", ""},
		{"colour not a colour", lookOwner, lookRec("update", "accent", "blue"), "", "expected #RRGGBB", ""},
		{"amber is refused", lookOwner, lookRec("update", "accent", "#ffbf00"), "", "amber", ""},
		{"delete is refused", lookOwner, lookRec("delete"), "", "the look can't be removed", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			k := NewAILook(lookFixture(t, richSettings()), nil)
			p := k.Plan(context.Background(), "c1", tc.actor, tc.rec)
			if tc.err != "" {
				if !strings.Contains(p.Error, tc.err) {
					t.Fatalf("error %q, want %q", p.Error, tc.err)
				}
				return
			}
			if p.Error != "" || !strings.Contains(p.Summary, tc.summary) {
				t.Fatalf("plan %+v, want summary with %q", p, tc.summary)
			}
			if got := strings.Join(p.Warnings, "|"); !strings.Contains(got, tc.warn) {
				t.Errorf("warnings %q, want %q", got, tc.warn)
			}
		})
	}
}

// A block sets only its keys: everything else, the brand, the header and the
// untouched choices, comes out of Apply as it went in.
func TestAILookApplyIsPartial(t *testing.T) {
	st := lookFixture(t, richSettings())
	k := NewAILook(st, nil)
	ctx := context.Background()
	before, _ := lookState(ctx, st.c, st.c.ParseSettings(), st.c.BackdropPath)
	if err := k.Apply(ctx, "c1", lookOwner, lookRec("update", "sheet_style", "night", "accent", "#336699")); err != nil {
		t.Fatal(err)
	}
	after, _ := lookState(ctx, st.c, st.c.ParseSettings(), st.c.BackdropPath)
	want := before
	want.Sheet.Style, want.Colours.Accent = "night", "#336699"
	if !reflect.DeepEqual(after, want) {
		t.Errorf("after the save:\n%+v\nwant\n%+v", after, want)
	}
	s := st.c.ParseSettings()
	if s.TopbarContent == nil || len(s.TopbarContent.Links) != 1 || s.BrandName != "Therin" {
		t.Errorf("the header and brand were not kept: %+v", s)
	}
}

func TestAILookApplyChecksAgain(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name  string
		actor aiplug.Actor
		rec   aiplug.Record
	}{
		{"non-owner", aiplug.Actor{UserID: "scribe", Role: 3}, lookRec("update", "look", "forest")},
		{"players-view key", lookPlayersView, lookRec("update", "look", "forest")},
		{"bad value", lookOwner, lookRec("update", "style_kit", "gaudy")},
		{"delete", lookOwner, lookRec("delete")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := lookFixture(t, richSettings())
			if err := NewAILook(st, nil).Apply(ctx, "c1", tc.actor, tc.rec); err == nil || st.saves != 0 {
				t.Errorf("Apply = %v with %d saves, want a refusal and no save", err, st.saves)
			}
		})
	}
	st := lookFixture(t, richSettings())
	if err := NewAILook(st, nil).Apply(ctx, "c1", lookOwner, lookRec("update", "look", "midnight")); err != nil || st.saves != 0 {
		t.Errorf("a block that changes nothing saved %d times (err %v)", st.saves, err)
	}
}

// Saving over a look waiting in Customize would be overwritten by its Save,
// so the block is refused until the owner publishes or discards it.
func TestAILookRefusesOverAnUnpublishedLook(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name, draft string
		refused     bool
	}{
		{"no draft", "", false},
		{"look waiting", `{"look":{"draft":{},"input":null}}`, true},
		{"only another section", `{"categories":{"x":1}}`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := lookFixture(t, richSettings())
			k := NewAILook(st, fakeLookDrafts{tc.draft})
			rec := lookRec("update", "look", "forest")
			p := k.Plan(ctx, "c1", lookOwner, rec)
			err := k.Apply(ctx, "c1", lookOwner, rec)
			if tc.refused {
				if !strings.Contains(p.Error, "publish or discard that draft first") || err == nil || st.saves != 0 {
					t.Errorf("plan %+v, apply %v, saves %d: want a refusal", p, err, st.saves)
				}
				// Nothing to change is not worth refusing.
				if p := k.Plan(ctx, "c1", lookOwner, lookRec("update", "look", "midnight")); p.Summary != "no change" {
					t.Errorf("no-change plan = %+v", p)
				}
				return
			}
			if p.Error != "" || err != nil || st.saves != 1 {
				t.Errorf("plan %+v, apply %v, saves %d: want a save", p, err, st.saves)
			}
		})
	}
}

// parseBlock reads the lookup's block back the way the engine parses one.
func parseBlock(t *testing.T, text string) aiplug.Record {
	t.Helper()
	f := map[string]any{}
	in := false
	for _, line := range strings.Split(text, "\n") {
		switch {
		case line == "---":
			in = !in
		case in:
			k, v, _ := strings.Cut(line, ": ")
			if u, err := strconv.Unquote(v); err == nil {
				f[k] = u
			} else {
				f[k] = v
			}
		}
	}
	return aiplug.Record{Kind: "look", Action: "update", Fields: f}
}

func TestAILookLookupRoundTripsToNoChange(t *testing.T) {
	ctx := context.Background()
	for name, s := range map[string]CampaignSettings{"never customized": {}, "rich": richSettings()} {
		t.Run(name, func(t *testing.T) {
			k := NewAILook(lookFixture(t, s), nil)
			_, text, err := k.Answer(ctx, "c1", lookOwner, aiplug.Record{})
			if err != nil {
				t.Fatal(err)
			}
			rec := parseBlock(t, text)
			if len(rec.Fields) < 20 {
				t.Fatalf("the block carries %d keys:\n%s", len(rec.Fields), text)
			}
			if p := k.Plan(ctx, "c1", lookOwner, rec); p.Error != "" || p.Summary != "no change" {
				t.Errorf("plan of the unchanged block = %+v\n%s", p, text)
			}
		})
	}
}

func TestAILookLookupIsOwnerOnly(t *testing.T) {
	k := NewAILook(lookFixture(t, richSettings()), nil)
	for name, a := range map[string]aiplug.Actor{
		"co-DM": {UserID: "scribe", Role: 3}, "player": {UserID: "pl", Role: 1},
		"players-view key with the owner's id": lookPlayersView,
	} {
		t.Run(name, func(t *testing.T) {
			if _, text, err := k.Answer(context.Background(), "c1", a, aiplug.Record{}); err == nil || text != "" {
				t.Errorf("answered %q (err %v)", text, err)
			}
		})
	}
	if !k.LookupInfo().Tool.Owner || k.LookupInfo().Tool.Name != "campaign_look" {
		t.Errorf("tool = %+v", k.LookupInfo().Tool)
	}
}

// The doc names every value the save accepts, from the same lists, and has no
// key for a picture or file.
func TestAILookDocListsEveryAllowedValue(t *testing.T) {
	doc := (&AILook{}).Doc()
	for _, lk := range lookKeys {
		if !strings.Contains(doc, "`"+lk.key+"`") {
			t.Errorf("doc misses key %s", lk.key)
		}
		for _, v := range lk.allowed {
			if !strings.Contains(doc, v) {
				t.Errorf("doc misses %s value %q", lk.key, v)
			}
		}
		for _, banned := range []string{"logo", "backdrop", "banner", "image", "picture_"} {
			if strings.Contains(lk.key, banned) {
				t.Errorf("key %s sets a picture", lk.key)
			}
		}
	}
	for _, line := range strings.Split(doc, "\n") {
		if strings.HasPrefix(line, "## ") {
			t.Errorf("doc has a section heading: %q", line)
		}
	}
}

// fullLook sets every look field to a non-default value under the given header
// background, so a field the rebuild forgets to carry shows up as a reset.
func fullLook(top *TopbarStyle) CampaignSettings {
	return CampaignSettings{
		BrandName: "Therin", BrandLogo: "brand/logo.png", WelcomeMessage: "Welcome, travellers.",
		AccentColor: "#7a5cff", AccentSurface1: "#3b82a0", AccentSurface2: "#a05c3b",
		TopbarStyle:   top,
		TopbarContent: &TopbarContent{Mode: "links", Quote: "Onward", Widgets: []string{"links", "text"}, Links: []TopbarLink{{Label: "Wiki", URL: "https://example.com/wiki"}}},
		Appearance: &Appearance{
			Look: "midnight", NavStyle: "tab", NavStrength: "lively", NavPageName: "hidden", PageTone: "paper", Contrast: "high",
			BodyFont: "lora", HeadingFont: "cinzel", TypeScale: "roomy", ButtonStyle: "ink", IconAmount: "all", IconColour: "category",
			Elevation: "dramatic", MotionSpeed: "snappy", ReduceMotion: true,
			SidebarColour: "own", SidebarOwn: "#1a2a22", SidebarCorner: "subtitle", SidebarSubtitle: "Of Therin",
			PeekGlow: "own", PeekGlowColour: "#3b9fb5", HeaderHeight: "tall", HoverCard: "night", SheetStyle: "ledger",
			CraftArt: "lifelike", PageLook: "annals", TimelinePopup: "corner", StyleKit: "ornate", Entrance: "bloom",
			MomentSheen: true, MomentCascade: true, MomentSpark: true, DmScreenKit: "sharp",
			EmailLayout: AppearanceEmailLayouts[len(AppearanceEmailLayouts)-1], BuilderLook: "paper",
			AccessSheet: LookAccessOwner, AccessPage: LookAccessEveryone, AccessHover: LookAccessEveryone, AccessDmScreen: LookAccessOwner,
		},
	}
}

// A block that names one unrelated key must leave every other stored field
// alone, for each kind of header the Look screen can save. Compared on the
// whole stored settings, not the screen's reading of them.
func TestAILookApplyKeepsEveryStoredField(t *testing.T) {
	ctx := context.Background()
	headers := map[string]*TopbarStyle{
		"no header":       nil,
		"solid":           {Mode: "solid", Color: "#223344"},
		"gradient":        {Mode: "gradient", GradientFrom: "#0f172a", GradientTo: "#1e2a5a", GradientDir: "to-br"},
		"moving":          {Mode: "moving", GradientFrom: "#0f172a", GradientTo: "#1e2a5a", GradientDir: "to-r"},
		"sky":             {Mode: "sky"},
		"image and shade": {Mode: "image", ImagePath: "headers/dusk.png", Scrim: "strong"},
	}
	for name, top := range headers {
		t.Run(name, func(t *testing.T) {
			st := lookFixture(t, fullLook(top))
			backdrop := "backdrops/hall.png"
			st.c.BackdropPath = &backdrop
			before := st.c.ParseSettings()
			if err := NewAILook(st, nil).Apply(ctx, "c1", lookOwner, lookRec("update", "button_style", "press")); err != nil {
				t.Fatal(err)
			}
			if st.saves != 1 {
				t.Fatalf("saves = %d, want 1", st.saves)
			}
			want := before
			ap := *before.Appearance
			ap.ButtonStyle = "press"
			want.Appearance = &ap
			// JSON so a pointer field prints its contents in the failure.
			gotJSON, _ := json.Marshal(st.c.ParseSettings())
			wantJSON, _ := json.Marshal(want)
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("stored settings differ beyond button_style:\ngot  %s\nwant %s", gotJSON, wantJSON)
			}
			if st.c.BackdropPath == nil || *st.c.BackdropPath != backdrop {
				t.Errorf("backdrop = %v, want %q", st.c.BackdropPath, backdrop)
			}
		})
	}
	// A block naming only what is already stored writes nothing.
	st := lookFixture(t, fullLook(&TopbarStyle{Mode: "sky"}))
	if err := NewAILook(st, nil).Apply(ctx, "c1", lookOwner, lookRec("update", "button_style", "ink")); err != nil || st.saves != 0 {
		t.Errorf("same value: err %v, %d saves, want none", err, st.saves)
	}
}
