package tags

import (
	"context"
	"log/slog"
)

// DefaultTagColour is what a tag with no colour of its own wears when the
// campaign has picked neither a Tags and marks colour nor an accent: the
// accent every campaign starts with.
const DefaultTagColour = "#6366f1"

// CampaignColours tells the tags widget which colours a campaign has picked,
// so a tag with no colour of its own can wear them. It is implemented by an
// adapter over the campaigns service in internal/app; the widget never reads
// campaign settings itself.
type CampaignColours interface {
	// TagColours returns the campaign's Tags and marks colour and its accent,
	// each "" when the owner has not picked one.
	TagColours(ctx context.Context, campaignID string) (marks, accent string, err error)
}

// tagPainter fills in the colour of tags that follow their campaign. It
// remembers each campaign's colour, so one read path asks once per campaign
// however many tags it returns. Build one per call; it is not shared.
type tagPainter struct {
	src  CampaignColours
	memo map[string]string
}

func newTagPainter(src CampaignColours) *tagPainter {
	return &tagPainter{src: src}
}

// colourOf is the campaign's colour for following tags: Tags and marks, else
// the accent, else Chronicle's default. A failed lookup is logged and falls
// back to the default rather than failing the read, so tags still draw.
func (p *tagPainter) colourOf(ctx context.Context, campaignID string) string {
	if c, ok := p.memo[campaignID]; ok {
		return c
	}
	out := DefaultTagColour
	if p.src != nil {
		marks, accent, err := p.src.TagColours(ctx, campaignID)
		switch {
		case err != nil:
			slog.Warn("tags: reading the campaign's tag colour", slog.String("campaign_id", campaignID), slog.Any("error", err))
		case hexColorPattern.MatchString(marks):
			out = marks
		case hexColorPattern.MatchString(accent):
			out = accent
		}
	}
	if p.memo == nil {
		p.memo = map[string]string{}
	}
	p.memo[campaignID] = out
	return out
}

// paint turns a tag as stored into a tag as shown: OwnColor says whether it
// has a colour of its own, and Color is always a real hex.
func (p *tagPainter) paint(ctx context.Context, t *Tag) {
	if t == nil {
		return
	}
	t.OwnColor = t.Color != ""
	if !t.OwnColor {
		t.Color = p.colourOf(ctx, t.CampaignID)
	}
}

func (p *tagPainter) paintAll(ctx context.Context, ts []Tag) {
	for i := range ts {
		p.paint(ctx, &ts[i])
	}
}

// StoredColor is the colour a tag stores: its own hex, or "" when it follows
// the campaign. Exports and AI records write this, never the shown colour, so
// a following tag still follows wherever it is brought back.
func (t Tag) StoredColor() string {
	if t.OwnColor {
		return t.Color
	}
	return ""
}
