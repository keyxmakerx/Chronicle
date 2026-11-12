package campaigns

import (
	"context"
	"fmt"
	"html"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// setupStepsBlockID names the setup steps block a new campaign's DM team
// home starts with, so tests and the owner's later edits can find it.
const setupStepsBlockID = "blk-setup-steps"

// starterDirectorHome is the home a new campaign's owner and co-DMs land on:
// the built-in home with an ordinary Text Block of setup steps under the
// welcome banner. It is
// a normal layout, so the owner edits or removes the block in Customize →
// Home pages like any other; nothing tracks or re-adds it. Players never see
// it: the DM team's home is theirs alone.
func starterDirectorHome(campaignID string) *DashboardLayout {
	layout := DefaultDashboardLayout()
	steps := DashboardRow{
		ID: "row-setup-steps",
		Columns: []DashboardColumn{{
			ID:    "col-setup-steps",
			Width: 12,
			Blocks: []DashboardBlock{{
				ID:     setupStepsBlockID,
				Type:   BlockTextBlock,
				Config: map[string]any{"content": setupStepsHTML(campaignID)},
			}},
		}},
	}
	// Under the welcome banner, so the campaign's name still leads the page.
	rows := append([]DashboardRow{}, layout.Rows[:1]...)
	rows = append(rows, steps)
	layout.Rows = append(rows, layout.Rows[1:]...)
	return layout
}

// setupStepsHTML is the setup block's text: four steps, each a link to the
// page that does it, and where to remove the block. The wording avoids the
// game master's title because no game system is chosen yet.
func setupStepsHTML(campaignID string) string {
	base := "/campaigns/" + html.EscapeString(campaignID)
	return fmt.Sprintf(`<h3>Get your table ready</h3>
<p>Only the people running this campaign see this note.</p>
<ol>
<li><a href="%[1]s/extensions">Choose your game</a> and turn on the parts you'll use, like the calendar and maps.</li>
<li><a href="%[1]s/members">Invite your players</a>. One link works for new and returning players.</li>
<li><a href="%[1]s/customize/look">Pick a look</a> for your campaign.</li>
<li><a href="%[1]s/entities/new">Write your first page</a>: a place, a villain, a hook. New pages start hidden from players until you show them.</li>
</ol>
<p>Done? Remove this note in <a href="%[1]s/customize/home/director">Customize → Home pages</a>.</p>`, base)
}

// ownerOnlyPaths are the pages the owner's setup steps link to that a co-DM
// is refused.
var ownerOnlyPaths = []string{"/extensions", "/members", "/customize"}

// codmNoteHTML is the setup block's text for a co-DM: what the role gives them
// and where to find it, with every link one they can open.
func codmNoteHTML(ctx context.Context, campaignID string) string {
	base := "/campaigns/" + html.EscapeString(campaignID)
	gm := html.EscapeString(layouts.GMTerm(ctx))
	return fmt.Sprintf(`<h3>Running this table with the owner</h3>
<p>Only the people running this campaign see this note.</p>
<ul>
<li>You're a co-%[2]s: you can see and write what players can't. Anything marked with the amber edge is hidden from players.</li>
<li><a href="%[1]s/entities/new">Write a page</a>. New pages start hidden from players until you show them.</li>
<li>To see the campaign as a player does, use the eye button at the top right of the header. Press it again to come back.</li>
<li>The owner sets the game, the look and who's invited.</li>
</ul>`, base, gm)
}

// setupNoteFor decides what the setup block shows the person looking at it.
// The owner sees the block as stored. A co-DM sees their own note in place of
// text that still links to owner-only pages (text the owner rewrote without
// those links is theirs to keep). Anyone else sees nothing: the note is for
// the people running the campaign. Reports false when nothing is drawn.
func setupNoteFor(ctx context.Context, cc *CampaignContext, block DashboardBlock) (DashboardBlock, bool) {
	if block.ID != setupStepsBlockID {
		return block, true
	}
	switch {
	case cc.MemberRole >= RoleOwner:
		return block, true
	case cc.IsDmGranted:
		content, _ := block.Config["content"].(string)
		for _, p := range ownerOnlyPaths {
			if strings.Contains(content, p) {
				block.Config = map[string]any{"content": codmNoteHTML(ctx, cc.Campaign.ID)}
				break
			}
		}
		return block, true
	}
	return block, false
}
