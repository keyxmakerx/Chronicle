// desk_fold.go builds what a desk line shows when it unfolds. People is
// campaigns' own; the other lines are read from the plugin that owns them
// through the optional DeskFolder seam, only when the owner opens the line,
// so the desk itself never pays for them. Everything a fold offers calls a
// route that already exists with its own owner check; a fold never adds a
// way to write.

package campaigns

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// deskFoldRows is how many rows a fold shows before "and N more": a sub menu
// is for the few things people come for, and the full page holds the rest.
const deskFoldRows = 6

// deskFoldSlots are the lines whose fold another plugin fills.
var deskFoldSlots = map[string]DeskSlot{
	"foundry": DeskFoundry,
	"trash":   DeskTrash,
}

// foldable says whether a line unfolds: People always, and a slot line only
// while a provider that can fill its fold is registered.
func (s *deskService) foldable(id string) bool {
	if id == "people" {
		return true
	}
	slot, ok := deskFoldSlots[id]
	if !ok {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok = s.providers[slot].(DeskFolder)
	return ok
}

// roleChoices are the words of a member's role blank. The co-Director word is
// the campaign's own term for its game master.
func roleChoices(gm string) []DeskOption {
	return []DeskOption{
		{Value: AccessPlayer, Label: "player"},
		{Value: AccessScribe, Label: "scribe"},
		{Value: AccessCoDM, Label: "co-" + gm},
	}
}

func roleShown(c *Campaign, m CampaignMember, gm string) (value, shown string) {
	switch MemberAccess(c, m) {
	case AccessCoDM:
		return AccessCoDM, "co-" + gm
	case AccessScribe:
		return AccessScribe, "scribe"
	}
	return AccessPlayer, "player"
}

// peopleFold lists the owner and then each member with a role that changes
// where it stands, and the invite field. The owner's own role is not
// changeable: the role route refuses it, so it is plain words here.
func peopleFold(c *Campaign, in DeskInputs) *DeskFold {
	gm := in.GMTerm
	if gm == "" {
		gm = "GM"
	}
	f := &DeskFold{MoreHref: deskHref(c.ID, "/members")}
	f.Rows = append(f.Rows, DeskFoldRow{Initial: "Y", Lead: "You", Meta: "Owner"})
	others := make([]CampaignMember, 0, len(in.Members))
	for _, m := range in.Members {
		if m.Role != RoleOwner {
			others = append(others, m)
		}
	}
	for i, m := range others {
		if len(f.Rows) >= deskFoldRows {
			f.More = len(others) - i
			break
		}
		name := m.DisplayName
		if name == "" {
			name = "Someone"
		}
		v, shown := roleShown(c, m, gm)
		b := &DeskBlank{
			Kind: DeskBlankChoice, ID: "role-" + m.UserID, Label: name + "'s role",
			Value: v, Shown: shown, Options: roleChoices(gm),
			URL: deskHref(c.ID, "/members/"+m.UserID+"/role"), Method: "PUT", Field: "role", Expect: "redirect",
		}
		offWhenArchived(c, b)
		meta := "no character yet"
		if m.CharacterName != nil && *m.CharacterName != "" {
			meta = *m.CharacterName
		}
		f.Rows = append(f.Rows, DeskFoldRow{Initial: strings.ToUpper(string([]rune(name)[:1])), Lead: name, Meta: meta, Blank: b})
	}
	if !c.IsArchived() {
		f.Invite = &DeskInvite{
			URL:   deskHref(c.ID, "/invites"),
			Roles: []DeskOption{{Value: "player", Label: "player"}, {Value: "scribe", Label: "scribe"}},
		}
	}
	return f
}

// foundryFold says why Foundry is silent, or how it stands, from the
// same status the line's sentence uses. It claims only what the connector
// measured: no count of missed tries, no guess at a cause.
func foundryFold(conn FoundryConnection, now time.Time) *DeskFold {
	f := &DeskFold{}
	st := ComputeFoundryStatus(now, conn)
	seen := foundrySeen(conn)
	switch st.Level {
	case FoundryConnectedNow:
		f.Rows = append(f.Rows, DeskFoldRow{Text: "Foundry is connected and answering now."})
	case FoundrySeenRecently, FoundrySeenLongAgo:
		w := fmt.Sprintf("Nothing has come from Foundry since %s.", seen.UTC().Format("Monday 2 January at 15:04 UTC"))
		if st.Level == FoundrySeenLongAgo {
			w += " It may be closed, or its link may need a new key."
		} else {
			w += " A Foundry world is often closed between sessions."
		}
		f.Rows = append(f.Rows, DeskFoldRow{Text: w})
	default:
		if conn.HasKey {
			f.Rows = append(f.Rows, DeskFoldRow{Text: "A key was made, but Foundry has not used it yet."})
		} else {
			f.Rows = append(f.Rows, DeskFoldRow{Text: "No key has been made for Foundry yet."})
		}
	}
	if conn.ModuleVersion != "" {
		w := "The module there is " + conn.ModuleVersion
		if conn.ServedVersion != "" && conn.ServedVersion != conn.ModuleVersion {
			w += ", and Chronicle serves " + conn.ServedVersion
		}
		f.Rows = append(f.Rows, DeskFoldRow{Text: w + "."})
	}
	if conn.KeyLastUsed != nil {
		f.Rows = append(f.Rows, DeskFoldRow{Text: "Its key was last used " + relativeAgo(now.Sub(*conn.KeyLastUsed)) + "."})
	}
	return f
}

// Fold reads the foundry slot's sub menu from the same connector the line
// reads.
func (p foundryDeskProvider) Fold(ctx context.Context, req DeskRequest) (*DeskFold, error) {
	conn, err := p.c.FoundryConnection(ctx, req.CampaignID)
	if err != nil {
		return nil, err
	}
	return foundryFold(conn, req.Now), nil
}

// fallbackFold is a fold that could not be read: the line's plain sentence
// and the link to the full page. It says nothing about numbers it lacks.
func fallbackFold(l DeskLine) *DeskFold {
	var b strings.Builder
	for _, p := range l.Parts {
		b.WriteString(p.Text)
	}
	f := &DeskFold{Fallback: true, Note: b.String()}
	f.FullHref, f.FullLabel = l.Href, "Open "+l.Name
	return f
}
