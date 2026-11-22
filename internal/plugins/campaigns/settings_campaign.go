// settings_campaign.go builds Campaign settings' Campaign tab: the campaign's
// name and description, who can see it, where new pages start, how it looks
// (with the way into the Customizer and a line per Customize area), and the
// card people see on My campaigns. Each setting reuses the desk's blank for
// it, so the tab, the board's flips and the old links save through the same
// routes with the same words.

package campaigns

import (
	"strings"
)

// CampaignTab is the Campaign tab's page.
type CampaignTab struct {
	Campaign *Campaign
	// Name and Description are typed into fields that save when left.
	Name, Description *DeskBlank
	// Access and Pages are two-way choices that save when pressed.
	Access, Pages *DeskBlank
	// Preview opens Home as a player or a guest sees it.
	Preview DeskLine
	Look    LookSummary
	// CustomizerHref opens the Customizer at its front page and PicturesHref
	// at Look, where the emblem and the cover are chosen; Customize are the
	// other Customize areas and the header brush, one row each.
	CustomizerHref, PicturesHref string
	Customize                    []DeskLine
	// Card is what the campaign's card on My campaigns shows.
	Card CampaignTabCard
}

// CampaignTabCard is the campaign as its card shows it to people.
type CampaignTabCard struct {
	Name, Description string
	// Cover and Emblem are media paths; empty draws them from the colour and
	// the name's first letter, as the banner does.
	Cover, Emblem string
	Public        bool
	Faces         []DeskFace
}

// campaignCardFaces is how many faces the card's row has room for.
const campaignCardFaces = 5

// campaignTabOf builds the tab from the desk's inputs. The Look row is the
// strip at the top of Look, so the Customize list starts after it.
func campaignTabOf(c *Campaign, in DeskInputs) CampaignTab {
	s := c.ParseSettings()
	t := CampaignTab{
		Campaign:       c,
		Name:           nameLine(c).Values[0].Blank,
		Description:    descriptionLine(c).Values[0].Blank,
		Access:         publicLine(c).Values[0].Blank,
		Pages:          pagesLine(c).Values[0].Blank,
		Preview:        previewLine(c),
		Look:           in.Look,
		CustomizerHref: deskHref(c.ID, "/customize"),
		PicturesHref:   deskHref(c.ID, "/customize/look"),
		Card: CampaignTabCard{
			Name:   c.Name,
			Emblem: strings.TrimSpace(s.BrandLogo),
			Public: c.IsPublic,
		},
	}
	if lines := customizeLines(c, in); len(lines) > 1 {
		t.Customize = lines[1:]
	}
	if c.Description != nil {
		t.Card.Description = strings.TrimSpace(*c.Description)
	}
	if c.BackdropPath != nil {
		t.Card.Cover = strings.TrimSpace(*c.BackdropPath)
	}
	for _, m := range in.Members {
		if len(t.Card.Faces) == campaignCardFaces {
			break
		}
		f := DeskFace{Name: memberShownName(m), Hue: faceHue(m.UserID)}
		if m.AvatarPath != nil {
			f.Avatar = strings.TrimSpace(*m.AvatarPath)
		}
		t.Card.Faces = append(t.Card.Faces, f)
	}
	return t
}

// choiceOn reports whether a choice's option is the one stored.
func choiceOn(b *DeskBlank, o DeskOption) bool { return b != nil && b.Value == o.Value }
