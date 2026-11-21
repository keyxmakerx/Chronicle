package records

import (
	"fmt"
	"strings"
)

// home answers `what: home`: each audience's home page as the owner has it,
// written as the `kind: home` blocks that send it back changed. Unlike the
// export it also says what an unsaved home borrows and what a home with a tile
// section holds, so an AI planning a new home sees where it is starting from.
// Owner only, as writing a home is: the arrangements name Director-only
// blocks and a co-DM may not change them.
func (run *lookupRun) home(audience string) (LookupAnswer, error) {
	ans := LookupAnswer{Chip: "Home pages"}
	k := run.l.Homes
	if k == nil || k.Svc == nil {
		return ans, badRequestf("home pages can't be looked up on this server")
	}
	if !run.director() {
		return ans, badRequestf("home pages are left out in Safe privacy; choose Permitted to include them")
	}
	owner, err := k.isOwner(run.ctx, run.campaignID, run.a)
	if err != nil {
		return ans, err
	}
	if !owner {
		return ans, badRequestf("only the campaign's owner can look at the home pages")
	}
	only := ""
	if strings.TrimSpace(audience) != "" {
		_, word, ok := audienceKey(audience)
		if !ok {
			return ans, badRequestf("%s is not an audience; use everyone, players, scribes, directors or guests", quote(audience))
		}
		only = word
	}

	// The Everyone home is what every unsaved home falls back to, so its state
	// is read first even when only another audience was asked for.
	everyone, everyoneSaved, err := k.Svc.Home(run.ctx, run.campaignID, homeAudiences[0].key)
	if err != nil {
		return ans, err
	}

	var b strings.Builder
	b.WriteString("## Home pages\n\nEach audience's home page as it is now. Send a `kind: home` block back, changed, to rearrange a home or rewrite its words: `create` and `update` replace the whole arrangement (list every block to keep), and `delete` takes an audience's own home away so it uses the Everyone home again.\n")
	saved := 0
	for _, aud := range homeAudiences {
		if only != "" && aud.word != only {
			continue
		}
		l, own, err := k.Svc.Home(run.ctx, run.campaignID, aud.key)
		if err != nil {
			return ans, err
		}
		fmt.Fprintf(&b, "\n### %s home\n\n", strings.ToUpper(aud.word[:1])+aud.word[1:])
		switch {
		case !own:
			b.WriteString(borrowedHome(aud.word, l, everyoneSaved, everyone))
		case l.Tiles:
			saved++
			b.WriteString(tileHome(l))
		case len(l.Rows) == 0:
			saved++
			b.WriteString("Saved with no blocks. Send a `home` block with `rows` to fill it.\n")
		default:
			saved++
			var blk strings.Builder
			k.writeHome(run.ctx, run.campaignID, &blk, aud.word, l)
			b.WriteString("Its own arrangement, saved:\n\n```\n" + strings.TrimRight(blk.String(), "\n") + "\n```\n")
		}
	}
	if only != "" {
		ans.Chip = "Home · " + only
	} else {
		ans.Chip = fmt.Sprintf("Home pages · %d saved", saved)
	}
	ans.Text = b.String()
	return ans, nil
}

// borrowedHome says an audience has no home of its own and what it shows
// instead: the Everyone home, or Chronicle's built-in one while that is not
// saved either.
func borrowedHome(word string, l HomeLayout, everyoneSaved bool, everyone HomeLayout) string {
	var b strings.Builder
	switch {
	case word != "everyone" && everyoneSaved:
		b.WriteString("Not saved, so it uses the Everyone home.")
	case word != "everyone":
		b.WriteString("Not saved, so it uses the Everyone home, which is not saved either, so everyone sees Chronicle's built-in home.")
	default:
		b.WriteString("Not saved, so everyone sees Chronicle's built-in home.")
	}
	// What it shows is worth naming only when the owner could start from it;
	// a borrowed tile section is described under the home it belongs to.
	if !l.Tiles && len(l.Rows) > 0 && (word == "everyone" || !everyoneSaved || everyone.Tiles) {
		b.WriteString(" It shows: " + homeSummary(l) + " (rows split by /, columns by |).")
	}
	b.WriteString(" A `home` block for this audience gives it its own.\n")
	return b.String()
}

// tileHome describes a saved home with a tile section: only the Customizer
// arranges tiles, so nothing here is written as a block to send back.
func tileHome(l HomeLayout) string {
	var b strings.Builder
	b.WriteString("Arranged in Customize; can't be written back.")
	if len(l.Rows) > 0 {
		b.WriteString(" Its rows hold: " + homeSummary(l) + ".")
	}
	if len(l.TileBlocks) > 0 {
		b.WriteString(" Its tile section holds: " + strings.Join(l.TileBlocks, ", ") + ".")
	}
	b.WriteString(" A `home` block for this audience would be refused.\n")
	return b.String()
}
