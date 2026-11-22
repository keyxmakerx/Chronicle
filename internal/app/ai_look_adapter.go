package app

import (
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// The look sits right after the home pages (same rank, so file order), the other
// thing the owner sets in Customize.
func init() {
	addAIKinds(186, func(d *aiDeps) []records.Kind {
		return []records.Kind{d.look()}
	})
}

// look is the campaign look's AI part, over the campaign service and the
// Customize draft store, which it reads to leave an unpublished look alone.
func (d *aiDeps) look() *campaigns.AILook {
	if d.LookDrafts == nil {
		return campaigns.NewAILook(d.Campaigns, nil)
	}
	return campaigns.NewAILook(d.Campaigns, d.LookDrafts)
}
