package campaigns

import "context"

// foundryDeskProvider fills the desk's Foundry line from the same
// FoundryConnector the Foundry page reads, which the app wires over the sync
// keys and the socket hub; campaigns never reaches those plugins itself.
type foundryDeskProvider struct{ c FoundryConnector }

func (foundryDeskProvider) Slot() DeskSlot { return DeskFoundry }

func (p foundryDeskProvider) Facts(ctx context.Context, req DeskRequest) (DeskFacts, error) {
	conn, err := p.c.FoundryConnection(ctx, req.CampaignID)
	if err != nil {
		return DeskFacts{}, err
	}
	return DeskFacts{Foundry: &DeskFoundryFacts{Connection: conn}}, nil
}
