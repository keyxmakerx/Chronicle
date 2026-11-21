package entities

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// PageParts supplies the first content of the page's tags and connections, so
// the page can carry it instead of leaving two empty boxes to fetch after it
// shows. It is an interface so this plugin never reaches into the tags or
// relations repositories. Each answer is the JSON the widget would otherwise
// have fetched, so the widget takes it over unchanged.
type PageParts interface {
	// Tags answers {"all":[...],"mine":[...]}: every tag the viewer may see
	// and this page's own.
	Tags(ctx context.Context, cc *campaigns.CampaignContext, entityID string) (json.RawMessage, error)
	// Relations answers {"relations":[...],"types":[...]}; types is left out
	// unless withTypes, which only someone who can add a relation needs.
	Relations(ctx context.Context, cc *campaigns.CampaignContext, entityID, userID string, withTypes bool) (json.RawMessage, error)
}

// SetPageParts wires the tags and connections supplier. Without it those two
// widgets fetch for themselves, as they always did.
func (h *Handler) SetPageParts(p PageParts) { h.pageParts = p }

// firstPaint holds what a page sends with itself for its four late-filling
// widgets. Each part is worked out only if a block on the page asks for it,
// so a layout without attributes never pays for them. An empty answer means
// the widget fetches as before: a failed part must never fail the page.
type firstPaint struct {
	once    [4]sync.Once
	parts   [4]json.RawMessage
	compute [4]func() json.RawMessage
}

const (
	fpEntry = iota
	fpFields
	fpTags
	fpRelations
)

func (f *firstPaint) get(i int) string {
	if f == nil || f.compute[i] == nil {
		return ""
	}
	f.once[i].Do(func() { f.parts[i] = f.compute[i]() })
	return string(f.parts[i])
}

type firstPaintKey struct{}

// sendsFirstPaint reports whether the page being drawn carries its widgets'
// first content. Only such a page waits to show whole; one that does not
// (the DM Screen's sheet, an era version) shows at once and fills as before.
func sendsFirstPaint(ctx context.Context) bool {
	f, _ := ctx.Value(firstPaintKey{}).(*firstPaint)
	return f != nil
}

func withFirstPaint(ctx context.Context, f *firstPaint) context.Context {
	return context.WithValue(ctx, firstPaintKey{}, f)
}

// firstPaintOf is the payload for the widget at index i on the page being
// drawn, or "" when the page sends none.
func firstPaintOf(ctx context.Context, i int) string {
	f, _ := ctx.Value(firstPaintKey{}).(*firstPaint)
	return f.get(i)
}

// newFirstPaint prepares the parts for the page being shown. Anything that
// changes what a widget fetches (an era version laid over the page) sends
// nothing, because those widgets read a different address.
func (h *Handler) newFirstPaint(c *echo.Context, cc *campaigns.CampaignContext, entity *Entity, userID string, showAttributes bool) *firstPaint {
	if eraViewOf(c.Request().Context()) != nil {
		return nil
	}
	marshal := func(what string, v any, err error) json.RawMessage {
		if err != nil {
			slog.Warn("page part not sent with the page; its widget will fetch it",
				slog.String("part", what), slog.String("entity_id", entity.ID), slog.Any("error", err))
			return nil
		}
		raw, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		return raw
	}
	ctx := c.Request().Context()
	f := &firstPaint{}
	f.compute[fpEntry] = func() json.RawMessage {
		return marshal("entry", h.entryPayload(c, cc, entity), nil)
	}
	if showAttributes {
		f.compute[fpFields] = func() json.RawMessage {
			v, err := h.fieldsPayload(c, cc, entity, userID)
			return marshal("fields", v, err)
		}
	}
	if h.pageParts != nil {
		f.compute[fpTags] = func() json.RawMessage {
			raw, err := h.pageParts.Tags(ctx, cc, entity.ID)
			if err != nil {
				return marshal("tags", nil, err)
			}
			return raw
		}
		f.compute[fpRelations] = func() json.RawMessage {
			raw, err := h.pageParts.Relations(ctx, cc, entity.ID, userID, cc.MemberRole >= campaigns.RoleScribe)
			if err != nil {
				return marshal("relations", nil, err)
			}
			return raw
		}
	}
	return f
}
