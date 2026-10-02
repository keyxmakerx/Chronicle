package campaigns

import "context"

// needsYou gathers what is waiting on the owner for the Overview card. It may
// only use data campaigns already reaches through its own service and
// interfaces, never another plugin's repository. Today none of the candidates
// (join requests, a game system update awaiting approval, Foundry not seen
// for a week, storage near its limit) has such a source: campaigns has no
// join-request concept, and the Foundry and media plugins expose no interface
// to it. Each lands here when its source exists.
//
// TODO(#851): add entries as their data becomes reachable through an interface.
func (h *Handler) needsYou(_ context.Context, _ *CampaignContext) []NeedsYouItem {
	return nil
}
