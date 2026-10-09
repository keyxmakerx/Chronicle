package app

import (
	"context"
	"math"
	"strconv"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
	"github.com/keyxmakerx/chronicle/internal/plugins/quests"
	"github.com/keyxmakerx/chronicle/internal/plugins/syncapi"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// syncQuestAPIAdapter implements syncapi.QuestAPIService over the quests and
// armory services. The sync API routes are registered before the quests
// plugin is built, so the quest services are attached afterwards; until then
// (or when the plugin is degraded) every call reads as not found.
//
// The sync API handler has already checked that the caller is the owner or a
// co-DM, so the caller reads as the DM; the players view is quests.PlainPlayer.
type syncQuestAPIAdapter struct {
	quests quests.QuestService
	boards quests.BoardService
	picker quests.PickerService
	stash  armory.StashService
	actors *armory.StashAPI
}

var _ syncapi.QuestAPIService = (*syncQuestAPIAdapter)(nil)

func (a *syncQuestAPIAdapter) attach(q quests.QuestService, b quests.BoardService, p quests.PickerService) {
	a.quests, a.boards, a.picker = q, b, p
}

func (a *syncQuestAPIAdapter) ready() error {
	if a.quests == nil || a.boards == nil || a.picker == nil {
		return apperror.NewNotFound("quests are not available")
	}
	return nil
}

func questAPIViewer(userID string, players bool) quests.Viewer {
	if players {
		return quests.PlainPlayer()
	}
	return quests.Viewer{UserID: userID, MemberRole: permissions.RoleOwner, VisibilityRole: permissions.RoleOwner, IsDM: true}
}

func (a *syncQuestAPIAdapter) Homes(ctx context.Context, campaignID, userID string, players bool) (any, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.boards.Homes(ctx, campaignID, questAPIViewer(userID, players))
}

func (a *syncQuestAPIAdapter) Boards(ctx context.Context, campaignID, userID string, home syncapi.QuestHome, players bool) (any, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	h := quests.PageHome(home.ID)
	if home.Kind == "category" {
		tid, err := strconv.Atoi(home.ID)
		if err != nil || tid <= 0 {
			return nil, apperror.NewNotFound("category not found")
		}
		h = quests.TypeHome(tid)
	}
	out, err := a.boards.View(ctx, campaignID, h, questAPIViewer(userID, players))
	if err != nil {
		return nil, err
	}
	// The same picture links the web board gets.
	for bi := range out.Boards {
		for ii := range out.Boards[bi].Items {
			it := &out.Boards[bi].Items[ii]
			if it.ImagePath != "" {
				it.ImageURL = layouts.MediaThumbURL(ctx, it.ImagePath, "300")
			}
		}
	}
	return out, nil
}

func (a *syncQuestAPIAdapter) Quest(ctx context.Context, campaignID, userID, entityID string, players bool) (any, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.quests.Get(ctx, campaignID, entityID, questAPIViewer(userID, players))
}

func (a *syncQuestAPIAdapter) PutQuest(ctx context.Context, campaignID, userID, entityID string, body []byte) (any, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	p, err := quests.DecodePatch(body)
	if err != nil {
		return nil, err
	}
	return a.quests.Put(ctx, campaignID, entityID, questAPIViewer(userID, false), p)
}

func (a *syncQuestAPIAdapter) Party(ctx context.Context, campaignID, userID string) (any, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.picker.Search(ctx, campaignID, questAPIViewer(userID, false), "character", "")
}

func (a *syncQuestAPIAdapter) Pay(ctx context.Context, campaignID, userID string, in syncapi.QuestPay) (any, error) {
	if math.IsNaN(in.Amount) || math.IsInf(in.Amount, 0) {
		return nil, apperror.NewBadRequest("Enter an amount from 0.01 to 1,000,000.")
	}
	actor, err := a.actors.ActorFor(ctx, campaignID, userID, "")
	if err != nil {
		return nil, err
	}
	return a.stash.Pay(ctx, campaignID, actor, armory.PayInput{
		CharacterID: in.CharacterID,
		Amount:      armory.Cents(math.Round(in.Amount * 100)),
		Reason:      in.Reason,
	})
}

type questGiveAnswer struct {
	ItemName      string `json:"itemName"`
	CharacterName string `json:"characterName"`
}

func (a *syncQuestAPIAdapter) Give(ctx context.Context, campaignID, userID string, in syncapi.QuestGive) (any, error) {
	actor, err := a.actors.ActorFor(ctx, campaignID, userID, "")
	if err != nil {
		return nil, err
	}
	out, err := a.stash.Give(ctx, campaignID, actor, armory.GiveInput{CharacterID: in.CharacterID, ItemID: in.ItemID, Quantity: 1})
	if err != nil {
		return nil, err
	}
	return questGiveAnswer{ItemName: out.ItemName, CharacterName: out.CharacterName}, nil
}
