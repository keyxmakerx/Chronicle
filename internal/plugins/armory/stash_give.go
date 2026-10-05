// stash_give.go lets the GM hand a character an Armory item or a map.
//
// A give credits the character's "Has Item" relation exactly as a stash move
// that credits a character does (adjustCarried), under the same campaign
// lock, so the relation events that already reach Foundry fire unchanged. It
// is recorded in item_moves as an applied item row whose two ends are the same
// character: the table's ENUMs stay as they are, and every history reader sees
// it through Move.IsGive instead of treating it as a move between two places.
//
// A map is given as an item. The map itself is not an item, so a handout
// entity called "Map: <name>" stands for it; it points at the map and is shown
// only to the players who hold it.
package armory

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

const (
	// giveReplayReason is what run answers when handed a give.
	giveReplayReason = "A gift is already given and can't be moved again."
	// handoutPrefix names a map handout item. The prefix plus the map name is
	// how an earlier handout for the same map is found again.
	handoutPrefix = "Map: "
	// maxHandoutMapName leaves room for the prefix inside the 200-byte entity
	// name limit, whatever the map's name is made of.
	maxHandoutMapName = 150
	// NotifItemGiven is the notification type a player gets when a GM gives
	// their character something.
	NotifItemGiven = "item_given"
	// giveListPageSize caps the item list a dialog shows; the search narrows it.
	giveListPageSize = 50
	// giveQuantityMessage is the answer to a quantity outside the allowed range.
	giveQuantityMessage = "Enter a quantity from 1 to 1,000,000."
)

// HandoutStore is what giving a map needs from the maps and entities plugins.
// Implemented in internal/app over their services; nil means maps cannot be
// given, and the dialog leaves that tab out.
type HandoutStore interface {
	// ListMaps returns the campaign's maps, by name.
	ListMaps(ctx context.Context, campaignID string) ([]NamedRef, error)
	// FindMap returns the map, or nil (and no error) when it does not exist in
	// campaignID, so a foreign id and a missing id look the same.
	FindMap(ctx context.Context, campaignID, mapID string) (*NamedRef, error)
	// FindHandout returns the item entity called name that is assigned mapID,
	// or nil when there is none.
	FindHandout(ctx context.Context, campaignID, name, mapID string) (*EntityRef, error)
	// CreateHandout makes an item entity called name, hidden from players,
	// assigned to the map, whose entry links the map's page.
	CreateHandout(ctx context.Context, campaignID, createdBy, name string, m NamedRef) (*EntityRef, error)
	// AllowViewers adds the users to the entity's allow list so they, besides
	// the GM, can open it. Existing grants stay; a public entity is left alone.
	// It returns the users it added, leaving out those who already had a
	// grant of their own.
	AllowViewers(ctx context.Context, campaignID, entityID string, userIDs []string) ([]string, error)
	// RevokeViewers takes the users' view grants off the entity's allow list.
	// A grant above view (set by hand) and every other grant stay.
	RevokeViewers(ctx context.Context, campaignID, entityID string, userIDs []string) error
}

// GiveNotifier tells players they were given something. Optional.
type GiveNotifier interface {
	// ItemGiven notifies the users, with a link to where to look. detail is
	// the short line under the message ("On Bren").
	ItemGiven(ctx context.Context, campaignID string, userIDs []string, message, detail, link string) error
}

// GiveInput is what the GM asks for. Exactly one of ItemID and MapID is set.
type GiveInput struct {
	CharacterID string
	ItemID      string
	MapID       string
	Quantity    int
}

// GiveOutcome reports what Give did, for the confirmation toast.
type GiveOutcome struct {
	Move          Move
	ItemName      string
	CharacterName string
}

// GiveDialogView feeds the Give box. Opened from a character page the
// character is fixed and the item (or map) is picked; opened from an item card
// the item is fixed and the character is picked.
type GiveDialogView struct {
	CampaignID string
	Character  *NamedRef
	// Player is the fixed character's player's name, empty when nobody has
	// claimed the character; the box's hint names them.
	Player string
	Item   *NamedRef
	// Items is the page of Armory items matching Query (at most
	// giveListPageSize); ItemTotal counts every match so the box can say
	// when it is showing only some.
	Items     []NamedRef
	ItemTotal int
	Query     string
	// Characters is the pick-list of the item flow.
	Characters []GiveCharacter
	// MapsOn is true when maps can be given at all (the box shows the map
	// tab); Maps lists them and HeldMaps marks those the character holds.
	MapsOn   bool
	Maps     []NamedRef
	HeldMaps map[string]bool
}

// GiveCharacter is one character in the item flow's list, with its player's
// name (empty when unclaimed) shown at the right of the row.
type GiveCharacter struct {
	ID     string
	Name   string
	Player string
}

// MoreItems reports whether the list shows only some of the matches.
func (v *GiveDialogView) MoreItems() bool { return v.ItemTotal > len(v.Items) }

func (s *stashService) requireOwner(a Actor) error {
	if !a.IsOwner() {
		return forbidden()
	}
	return nil
}

// loadItem returns an item-category entity in the campaign, else NotFound.
func (s *stashService) loadItem(ctx context.Context, campaignID, id string) (*EntityRef, error) {
	ref, err := s.Directory.GetEntity(ctx, campaignID, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	if ref == nil || !ref.IsItem {
		return nil, notFound("item")
	}
	return ref, nil
}

func (s *stashService) GiveDialog(ctx context.Context, campaignID string, a Actor, characterID, itemID, query string) (*GiveDialogView, error) {
	if err := s.requireOwner(a); err != nil {
		return nil, err
	}
	view := &GiveDialogView{CampaignID: campaignID, Query: strings.TrimSpace(query)}
	switch {
	case strings.TrimSpace(characterID) != "":
		ref, err := s.loadCharacter(ctx, campaignID, strings.TrimSpace(characterID))
		if err != nil {
			return nil, err
		}
		view.Character = &NamedRef{ID: ref.ID, Name: ref.Name}
		view.Player = s.playerNames(ctx, campaignID, []string{ref.OwnerUserID})[ref.OwnerUserID]
		items, err := s.Directory.ListItems(ctx, campaignID, a.Role, a.UserID, 0)
		if err != nil {
			return nil, err
		}
		view.Items, view.ItemTotal = matchItems(items, view.Query, giveListPageSize)
		if s.Handouts != nil {
			view.MapsOn = true
			if view.Maps, err = s.Handouts.ListMaps(ctx, campaignID); err != nil {
				return nil, err
			}
			held, err := s.carried(ctx, campaignID, a, ref.ID)
			if err != nil {
				return nil, err
			}
			view.HeldMaps = s.markMaps(ctx, campaignID, held)
		}
	case strings.TrimSpace(itemID) != "":
		ref, err := s.loadItem(ctx, campaignID, itemID)
		if err != nil {
			return nil, err
		}
		view.Item = &NamedRef{ID: ref.ID, Name: ref.Name, Restricted: ref.Restricted}
		chars, err := s.Directory.ListCharacters(ctx, campaignID, a.Role, a.UserID)
		if err != nil {
			return nil, err
		}
		owners := make([]string, 0, len(chars))
		for _, c := range chars {
			owners = append(owners, c.OwnerUserID)
		}
		names := s.playerNames(ctx, campaignID, owners)
		for _, c := range chars {
			view.Characters = append(view.Characters, GiveCharacter{ID: c.ID, Name: c.Name, Player: names[c.OwnerUserID]})
		}
	default:
		return nil, apperror.NewBadRequest("Choose a character or an item to give.")
	}
	return view, nil
}

// playerNames resolves the claimed players' display names. A failed lookup
// only costs the names: the box then reads without them.
func (s *stashService) playerNames(ctx context.Context, campaignID string, userIDs []string) map[string]string {
	var ids []string
	for _, id := range userIDs {
		if id != "" {
			ids = append(ids, id)
		}
	}
	if s.UserNames == nil || len(ids) == 0 {
		return map[string]string{}
	}
	names, err := s.UserNames.DisplayNames(ctx, campaignID, ids)
	if err != nil {
		slog.Warn("give: could not read player names", slog.Any("error", err))
		return map[string]string{}
	}
	return names
}

// markMaps flags the held lines that are map handouts and returns the maps
// they stand for. Only a name with the handout prefix is looked up, so a
// character's ordinary items cost nothing.
func (s *stashService) markMaps(ctx context.Context, campaignID string, held []HeldItem) map[string]bool {
	maps := map[string]bool{}
	for i := range held {
		if !strings.HasPrefix(held[i].Name, handoutPrefix) {
			continue
		}
		ref, err := s.Directory.GetEntity(ctx, campaignID, held[i].ItemID)
		if err != nil || ref == nil || ref.HandoutMapID == "" {
			continue
		}
		held[i].IsMap = true
		maps[ref.HandoutMapID] = true
	}
	return maps
}

// matchItems returns the first limit items whose name contains query (any case,
// by name), and how many matched in all.
func matchItems(items []EntityRef, query string, limit int) ([]NamedRef, int) {
	q := strings.ToLower(query)
	var all []NamedRef
	for _, it := range items {
		if q == "" || strings.Contains(strings.ToLower(it.Name), q) {
			all = append(all, NamedRef{ID: it.ID, Name: it.Name, Restricted: it.Restricted})
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return strings.ToLower(all[i].Name) < strings.ToLower(all[j].Name) })
	total := len(all)
	if total > limit {
		all = all[:limit]
	}
	return all, total
}

// handoutName is the item name a map's handout carries.
func handoutName(mapName string) string {
	mapName = strings.TrimSpace(mapName)
	if utf8.RuneCountInString(mapName) > maxHandoutMapName {
		mapName = string([]rune(mapName)[:maxHandoutMapName])
	}
	return handoutPrefix + mapName
}

func (s *stashService) Give(ctx context.Context, campaignID string, a Actor, in GiveInput) (*GiveOutcome, error) {
	if err := s.requireOwner(a); err != nil {
		return nil, err
	}
	in.CharacterID, in.ItemID, in.MapID = strings.TrimSpace(in.CharacterID), strings.TrimSpace(in.ItemID), strings.TrimSpace(in.MapID)
	if in.Quantity < 1 || in.Quantity > maxMoveQuantity {
		return nil, apperror.NewBadRequest(giveQuantityMessage)
	}
	if (in.ItemID == "") == (in.MapID == "") {
		return nil, apperror.NewBadRequest("Choose an item or a map to give.")
	}
	char, err := s.loadCharacter(ctx, campaignID, in.CharacterID)
	if err != nil {
		return nil, err
	}

	var item *EntityRef
	var mp *NamedRef
	if in.MapID != "" {
		if s.Handouts == nil {
			return nil, apperror.NewBadRequest("Maps can't be given here.")
		}
		if mp, err = s.Handouts.FindMap(ctx, campaignID, in.MapID); err != nil {
			return nil, err
		}
		if mp == nil {
			return nil, notFound("map")
		}
	} else if item, err = s.loadItem(ctx, campaignID, in.ItemID); err != nil {
		return nil, err
	}

	// Registered before the lock so the events go out after it is released.
	var ev eventBatch
	defer ev.flush(s.Events, campaignID)
	unlock := s.locks.lock(campaignID)
	out, err := s.give(ctx, campaignID, a, char, item, mp, in.Quantity, &ev)
	unlock()
	if err != nil {
		return nil, err
	}
	s.notifyGiven(ctx, campaignID, a, char, out, in.Quantity)
	return out, nil
}

// give does the work of Give under the campaign lock. The handout is found or
// made inside the lock so two gives of one map cannot make two items.
func (s *stashService) give(ctx context.Context, campaignID string, a Actor, char, item *EntityRef, mp *NamedRef, qty int, ev *eventBatch) (*GiveOutcome, error) {
	if mp != nil {
		var err error
		if item, err = s.handoutFor(ctx, campaignID, a, *mp); err != nil {
			return nil, err
		}
	}

	// Let the holder see the item before they hold it, so a player is never
	// handed something they cannot open (a private item would otherwise show
	// as "someone" in their history and be missing from their panel). A
	// character with no player keeps a private item GM-only.
	if s.Handouts != nil && char.OwnerUserID != "" {
		if _, err := s.Handouts.AllowViewers(ctx, campaignID, item.ID, []string{char.OwnerUserID}); err != nil {
			return nil, err
		}
	}

	ok, err := s.adjustCarried(ctx, campaignID, char.ID, item.ID, a.UserID, qty, true, false)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperror.NewConflict("Could not add it to that character.")
	}

	self := Endpoint{Kind: EndpointCharacter, ID: char.ID}
	m := &Move{
		CampaignID: campaignID, Kind: MoveKindItem, ItemEntityID: item.ID, Quantity: qty,
		From: self, To: self, Status: MoveApplied, RequestedBy: a.UserID, byGM: true,
	}
	if err := s.Repo.InsertMove(ctx, m); err != nil {
		// The item is already on the character and its relation events have
		// gone out. Failing the request would invite a second give, so say so
		// where an operator will see it and report success.
		slog.Error("give: GIVEN BUT NOT RECORDED in the move history",
			slog.String("campaign_id", campaignID), slog.String("character_id", char.ID),
			slog.String("item_id", item.ID), slog.Any("error", err))
	} else {
		ev.moved(m)
	}
	return &GiveOutcome{Move: *m, ItemName: item.Name, CharacterName: char.Name}, nil
}

// handoutFor returns the item that stands for the map, making it the first
// time. It reuses an item with the same name that points at the same map.
func (s *stashService) handoutFor(ctx context.Context, campaignID string, a Actor, mp NamedRef) (*EntityRef, error) {
	name := handoutName(mp.Name)
	ref, err := s.Handouts.FindHandout(ctx, campaignID, name, mp.ID)
	if err != nil {
		return nil, err
	}
	if ref != nil {
		return ref, nil
	}
	return s.Handouts.CreateHandout(ctx, campaignID, a.UserID, name, mp)
}

// notifyGiven tells the character's player. A failure is logged and dropped:
// the item is already given and a missed bell must not undo it. The player
// reads "Your GM", whoever of the GMs gave it, as in their history.
func (s *stashService) notifyGiven(ctx context.Context, campaignID string, a Actor, char *EntityRef, out *GiveOutcome, qty int) {
	if s.Notifier == nil || char.OwnerUserID == "" || char.OwnerUserID == a.UserID {
		return
	}
	thing := out.ItemName
	if qty > 1 {
		thing = fmt.Sprintf("%d × %s", qty, out.ItemName)
	}
	link := "/campaigns/" + campaignID + "/entities/" + char.ID
	if err := s.Notifier.ItemGiven(ctx, campaignID, []string{char.OwnerUserID}, "Your GM gave you "+thing, "On "+char.Name, link); err != nil {
		slog.Warn("give: could not notify the player", slog.Any("error", err))
	}
}

// giveSummary is the history sentence for a give. A GM reads who gave what to
// whom; the character's own player reads "Your GM gave you", as the
// notification says; anyone else who can see the line reads it from the
// outside. requester is empty when the giver's name is not known.
func giveSummary(l MoveLine, viewerIsGM, viewerIsRecipient bool, viewerID, requester string) string {
	thing := thingText(l)
	if viewerIsGM {
		who := "You"
		if l.RequestedBy != viewerID {
			who = requester
			if who == "" {
				who = "A GM"
			}
		}
		return fmt.Sprintf("%s gave %s %s", who, l.ToName, thing)
	}
	if viewerIsRecipient {
		return "Your GM gave you " + thing
	}
	who := requester
	if who == "" {
		who = "A GM"
	}
	return fmt.Sprintf("%s gave %s %s", who, l.ToName, thing)
}
