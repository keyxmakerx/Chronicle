package cutscenes

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
)

// scenesLookup answers `what: cut-scenes` for a Director only: scenes are
// the Director's preparation, so a player's key and the owner's "what players
// see" key are refused rather than told what is in store for them.
type scenesLookup struct{ *aiCore }

func (*scenesLookup) LookupInfo() aiplug.LookupInfo {
	return aiplug.LookupInfo{
		What: "cut-scenes", Aliases: []string{"cutscenes", "cut-scene", "cutscene"}, Keys: "`scene` (a scene's name, optional)",
		Brief: "the campaign's cut scenes, each layer in words and in the cut-scene block's format (the same timeline the editor draws: every layer's start, length, arrival and leaving), and every background, animation, text style and ending a scene can use with what it looks like (Director only)",
		Tool: aiplug.Tool{
			Name: "cut_scenes", Title: "Cut scenes", Did: "Looked at the cut scenes", Owner: true,
			Desc: "The campaign's cut scenes: short full-screen moments the Director shows chosen players. Every scene with its background, the animations on top, its words and its ending, in words and as a ready cut-scene block to change, the block being the same timeline the editor draws (each layer a bar with a start, a length, how it arrives and how it leaves); and every kind of background, animation, text style and ending with what each looks like on screen, so a new scene can be written well. `scene` narrows it to one scene. Scenes are made, changed and deleted with cut-scene blocks in ask_for_changes; the AI never shows a scene to anyone.",
			Args: []aiplug.Arg{{Key: "scene", Desc: "A scene's name; leave out for every scene"}},
		},
	}
}

func (l *scenesLookup) Answer(ctx context.Context, campaignID string, a aiplug.Actor, r aiplug.Record) (string, string, error) {
	chip := "Cut scenes"
	if !l.on(ctx, campaignID) {
		return chip, "", aiplug.BadRequestf("cut scenes are switched off in this campaign")
	}
	if a.ReadsOwnNotes || !a.CanAuthorDmOnly() {
		return chip, "", aiplug.BadRequestf("cut scenes are the Director's; they are read with the Director's own key")
	}
	want := strings.TrimSpace(r.Str("scene"))
	if want == "" {
		want = strings.TrimSpace(r.Name)
	}
	list, err := l.svc.List(ctx, campaignID, directorViewer(a))
	if err != nil {
		return chip, "", err
	}
	var b strings.Builder
	b.WriteString("## Cut scenes\n\n")
	shown := 0
	for _, s := range list {
		if want != "" && !aiplug.SameName(s.Name, want) {
			continue
		}
		shown++
		writeScene(&b, s)
	}
	switch {
	case want != "" && shown == 0:
		fmt.Fprintf(&b, "No scene is called %s.\n\n", aiplug.Quote(want))
	case len(list) == 0:
		b.WriteString("The campaign has no cut scenes yet.\n\n")
	}
	writeCatalog(&b)
	return chip, b.String(), nil
}

// writeScene says one scene in words, then as the block that would make it.
func writeScene(b *strings.Builder, s Scene) {
	fmt.Fprintf(b, "### %s\n\n", aiplug.OneLine(s.Name, 120))
	lines := []string{"Background: " + BackgroundWords(s.Background)}
	lines = append(lines, layerLines(s)...)
	lines = append(lines, "Ending: "+EndingWords(s.Ending))
	for _, l := range lines {
		b.WriteString("- " + l + "\n")
	}
	b.WriteString("\n```\n---\n" + SceneBlock(s) + "---\n```\n\n")
}

// SceneBlock is the scene's front matter in the cut-scene block's format.
func SceneBlock(s Scene) string {
	var b strings.Builder
	fmt.Fprintf(&b, "kind: %s\nname: %s\n", KindName, aiplug.Quote(s.Name))
	// The background is written as the editor shows it: what is behind,
	// then how it arrives.
	arrive := ""
	if BackdropOf(s.Background.Kind) != BackgroundNone {
		arrive = ", in: " + ArrivalOf(s.Background.Kind)
	}
	fmt.Fprintf(&b, "background: {kind: %s%s, seconds: %s%s}\n", BackdropOf(s.Background.Kind), arrive, num(s.Background.Seconds), lookField(s.Background.Params.Look))
	if len(s.Overlays) == 0 {
		b.WriteString("overlays: []\n")
	} else {
		b.WriteString("overlays:\n")
		for _, o := range s.Overlays {
			color := ""
			if o.Params.Color != "" {
				color = ", color: " + strconv.Quote(o.Params.Color)
			}
			fmt.Fprintf(&b, "  - {kind: %s, at: %s, seconds: %s%s%s}\n", o.Kind, num(o.At), num(o.Seconds), color, lookField(o.Params.Look)+pupilField(o.Params.Pupil))
		}
	}
	if len(s.Text) == 0 {
		b.WriteString("text: []\n")
	} else {
		b.WriteString("text:\n")
		for _, t := range s.Text {
			fmt.Fprintf(&b, "  - {say: %s, at: %s, seconds: %s, style: %s%s%s%s}\n", aiplug.Quote(t.Say), num(t.At), num(t.Seconds), t.Style,
				optField("layout", t.Layout), optField("in", t.In), optField("out", t.Out))
		}
	}
	// Moments only when there are some, so an older scene's block reads as
	// it always did.
	if len(s.Moments) > 0 {
		b.WriteString("moments:\n")
		for _, m := range s.Moments {
			ways := make([]string, 0, len(m.Ways))
			for _, w := range m.Ways {
				ways = append(ways, w.Kind)
			}
			note := ""
			if m.Note != "" {
				note = ", note: " + aiplug.Quote(m.Note)
			}
			fmt.Fprintf(&b, "  - {at: %s, ask: %s, ways: [%s]%s%s}\n", num(m.At), aiplug.Quote(m.Ask), strings.Join(ways, ", "), optField("to", m.To), note)
		}
	}
	if s.Ending.Kind == EndingAuto {
		after := DefaultAfter
		if s.Ending.After != nil {
			after = *s.Ending.After
		}
		fmt.Fprintf(&b, "ending: {kind: auto, after: %s}\n", num(after))
	} else {
		b.WriteString("ending: {kind: tap}\n")
	}
	return b.String()
}

// optField is a block's `key: value` for a choice left at its default when
// empty, so pasting the block back keeps it so.
func optField(key, v string) string {
	if v == "" {
		return ""
	}
	return ", " + key + ": " + v
}

// pupilField is a block's `pupil:` for an eye that names one.
func pupilField(pupil string) string {
	if pupil == "" {
		return ""
	}
	return ", pupil: " + pupil
}

// lookField is a block's `look:` for a layer that names one; a layer playing
// its default look writes none, so pasting the block back keeps it so.
func lookField(look string) string {
	if look == "" {
		return ""
	}
	return ", look: " + look
}

// writeCatalog lists every choice with what it looks like.
func writeCatalog(b *strings.Builder) {
	b.WriteString("### What a scene can use\n\n")
	section := func(title string, list []Kind) {
		b.WriteString(title + ":\n")
		for _, k := range list {
			fmt.Fprintf(b, "- `%s` (%s): %s\n", k.Key, k.Name, k.Looks)
		}
		b.WriteString("\n")
	}
	section("Backgrounds (`background.kind`)", BackgroundKinds)
	looks := func(title, kind string) {
		b.WriteString(title + ":\n")
		for i, l := range Looks[kind] {
			def := ""
			if i == 0 {
				def = ", the default"
			}
			fmt.Fprintf(b, "- `%s` (%s%s): %s\n", l.Key, l.Name, def, l.Looks)
		}
		b.WriteString("\n")
	}
	looks("Looks of falling asleep (`background.look`)", BackgroundFallAsleep)
	section("Animations on top (`overlays[].kind`)", OverlayKinds)
	looks("Looks of the eye (`overlays[].look`)", OverlayEye)
	b.WriteString("Pupils of the eye (`overlays[].pupil`), any look; left out, the look's own:\n")
	for _, p := range Pupils {
		var own []string
		for _, l := range Looks[OverlayEye] {
			if l.Pupil == p.Key {
				own = append(own, l.Key)
			}
		}
		fmt.Fprintf(b, "- `%s` (%s, the own pupil of %s): %s\n", p.Key, p.Name, strings.Join(own, " and "), p.Looks)
	}
	b.WriteString("\n")
	section("Text styles (`text[].style`)", TextStyles)
	section("Where words sit (`text[].layout`); left out, the middle", TextLayouts)
	section("How words arrive (`text[].in`); left out, the style's own", TextArrivals)
	section("How words leave (`text[].out`); left out, a soft blur upwards", TextLeavings)
	section("Ways to answer a moment (`moments[].ways`); left out, they write it", WayKinds)
	section("Who answers a moment (`moments[].to`); left out, each person watching", Answerers)
	section("Endings (`ending.kind`)", EndingKinds)
	fmt.Fprintf(b, "Players can always close a scene with Esc or a small close mark after 2 seconds. A good scene is short: the background reaches dark, one or two things appear, a line or two of words, all within about %s.\n", Secs(20))
}
