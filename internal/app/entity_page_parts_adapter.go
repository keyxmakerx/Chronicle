package app

import (
	"context"
	"encoding/json"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
	"github.com/keyxmakerx/chronicle/internal/widgets/tags"
)

// entityPagePartsAdapter lets the entity page send its tags and connections
// with the page. It goes through the two widgets' own handlers, which hold the
// visibility rules, so a part sent with the page is exactly what its endpoint
// would have answered and never a second copy of those rules.
type entityPagePartsAdapter struct {
	tags *tags.Handler
	rels *relations.Handler
}

// Tags answers {"all":[...],"mine":[...]}.
func (a *entityPagePartsAdapter) Tags(ctx context.Context, cc *campaigns.CampaignContext, entityID string) (json.RawMessage, error) {
	all, mine, err := a.tags.FirstPaint(ctx, cc, entityID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"all": all, "mine": mine})
}

// Relations answers {"relations":[...],"types":[...]}.
func (a *entityPagePartsAdapter) Relations(ctx context.Context, cc *campaigns.CampaignContext, entityID, userID string, withTypes bool) (json.RawMessage, error) {
	rels, types, err := a.rels.FirstPaint(ctx, cc, entityID, userID, withTypes)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"relations": rels}
	if types != nil {
		out["types"] = types
	}
	return json.Marshal(out)
}
