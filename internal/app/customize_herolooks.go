package app

import (
	"context"
	"encoding/json"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// customizeSectionHeroLooks is the draft key for the owner's card borders in
// the hero creator.
const customizeSectionHeroLooks = "herolooks"

// heroLooksItem is one card's border in the draft. For a package card
// (source "package", key = the entry's slug, or parent--child for a derived
// entry) border "" means no border and reset:true gives back the game
// system's own look. For a campaign's own entry (source "campaign", key = the
// entry's numeric id) the choice is written into the entry itself. colour is
// "" (the border's own) or #rrggbb; border "" requires colour "".
type heroLooksItem struct {
	Field  string `json:"field"`
	Source string `json:"source"`
	Key    string `json:"key"`
	Border string `json:"border"`
	Colour string `json:"colour"`
	Reset  bool   `json:"reset"`
}

// heroLooksSectionValue is the draft value the page writes:
//
//	{"looks":[{"field":"ancestry","source":"package","key":"hakaan","border":"fire","colour":"#58c4d8"},
//	          {"field":"class","source":"package","key":"fury","reset":true}],
//	 "_changes":[{"label":"...","href":"..."}]}
//
// Keys it does not know, "_changes" included, are ignored. At most
// systems.MaxCardLookChanges items, one per (field, source, key).
type heroLooksSectionValue struct {
	Looks []heroLooksItem `json:"looks"`
}

// cardLooksApplier is the slice of the card look service the section needs.
type cardLooksApplier interface {
	Apply(ctx context.Context, campaignID, userID string, changes []systems.CardLookChange) error
}

// newHeroLooksSection builds the "herolooks" Customize section. Only the
// campaign's owner may save it, the campaign is the request's own (never the
// body's), and the service checks every item before anything is written.
func newHeroLooksSection(svc cardLooksApplier) campaigns.CustomizeSection {
	return campaigns.CustomizeSection{
		Key: customizeSectionHeroLooks,
		Apply: func(ctx context.Context, cc *campaigns.CampaignContext, userID string, raw json.RawMessage) (json.RawMessage, error) {
			if cc.MemberRole < campaigns.RoleOwner {
				return nil, apperror.NewForbidden("only campaign owners can change card borders")
			}
			var sec heroLooksSectionValue
			if err := json.Unmarshal(raw, &sec); err != nil {
				return nil, apperror.NewBadRequest("the card borders in the draft are not valid")
			}
			if len(sec.Looks) == 0 {
				return nil, nil
			}
			changes := make([]systems.CardLookChange, len(sec.Looks))
			for i, it := range sec.Looks {
				changes[i] = systems.CardLookChange{
					FieldKey: it.Field, Source: it.Source, Key: it.Key,
					Border: it.Border, Colour: it.Colour, Reset: it.Reset,
				}
			}
			// Nothing is left over: the batch applies whole or not at all.
			return nil, svc.Apply(ctx, cc.Campaign.ID, userID, changes)
		},
	}
}
