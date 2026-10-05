// share.go lets a player share a hidden item their character holds with the
// rest of the party: the "Share..." link on a line of the character panel.
//
// Sharing only ever adds or takes back plain view grants on the item's allow
// list, and only for players of the campaign's other characters. Every grant
// it adds is recorded (ShareStore) so taking a share back removes exactly
// what sharing added: never the holder's own access, a grant the GM set by
// hand, or the access a give or another holder's share relies on.
package armory

import (
	"context"
	"log/slog"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// ShareAuditor records shares in the campaign's activity log. Optional.
type ShareAuditor interface {
	LogEvent(ctx context.Context, campaignID, userID, action string, details map[string]any) error
}

// shareAuditAction is the activity log action for a change to who shares an item.
const shareAuditAction = "item.shared"

// ShareMember is one other player in the Share box: their character, their
// name, and whether the item is already shared with them.
type ShareMember struct {
	UserID    string
	Character string
	Player    string
	Checked   bool
}

// ShareBoxView feeds the "Who else can see <item>?" box.
type ShareBoxView struct {
	CampaignID string
	Character  NamedRef
	Item       NamedRef
	// IsMap marks a map handout, whose hint says the map opens too.
	IsMap bool
	Party []ShareMember
}

// ShareOutcome reports who the item is shared with after a save, by
// character name, for the toast and the panel line.
type ShareOutcome struct {
	SharedWith []string
}

// shareTarget is a checked request: the holder, the item, and who may be
// offered.
type shareTarget struct {
	char  *EntityRef
	item  *EntityRef
	party []ShareMember
}

// loadShareTarget checks that a may share itemID held by characterID and
// gathers the party to offer. Only the holder's player or Owner visibility
// may share; only an item the character holds, the actor can see, and not
// everyone can already see.
func (s *stashService) loadShareTarget(ctx context.Context, campaignID string, a Actor, characterID, itemID string) (*shareTarget, error) {
	if s.Shares == nil || s.Handouts == nil {
		return nil, apperror.NewBadRequest("Sharing isn't available right now.")
	}
	char, err := s.loadCharacter(ctx, campaignID, strings.TrimSpace(characterID))
	if err != nil {
		return nil, err
	}
	holderPlayer := char.OwnerUserID != "" && char.OwnerUserID == a.UserID
	if !holderPlayer && !a.IsOwner() {
		return nil, forbidden()
	}
	item, err := s.loadItem(ctx, campaignID, itemID)
	if err != nil {
		return nil, err
	}
	rel, err := s.hasItem(ctx, campaignID, char.ID, item.ID, a.IsGM())
	if err != nil {
		return nil, err
	}
	if rel == nil {
		return nil, notFound("item")
	}
	if q, _ := parseCarried(rel.Metadata); q < 1 {
		return nil, notFound("item")
	}
	ok, err := s.viewable(ctx, campaignID, a, item.ID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, notFound("item")
	}
	if !item.Restricted {
		return nil, apperror.NewBadRequest("Everyone can already see " + item.Name + ".")
	}
	party, err := s.shareParty(ctx, campaignID, a, char)
	if err != nil {
		return nil, err
	}
	return &shareTarget{char: char, item: item, party: party}, nil
}

// shareParty lists the players of the campaign's other characters that the
// actor can see, one row per player (their first character by name), never
// the holder's own player or the actor. A user who is no longer a member
// drops out because they have no name.
func (s *stashService) shareParty(ctx context.Context, campaignID string, a Actor, char *EntityRef) ([]ShareMember, error) {
	chars, err := s.Directory.ListCharacters(ctx, campaignID, a.Role, a.UserID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(chars, func(i, j int) bool { return strings.ToLower(chars[i].Name) < strings.ToLower(chars[j].Name) })
	var owners []string
	for _, c := range chars {
		owners = append(owners, c.OwnerUserID)
	}
	names := s.playerNames(ctx, campaignID, owners)
	seen := map[string]bool{"": true, char.OwnerUserID: true, a.UserID: true}
	var out []ShareMember
	for _, c := range chars {
		if c.ID == char.ID || seen[c.OwnerUserID] || names[c.OwnerUserID] == "" {
			continue
		}
		seen[c.OwnerUserID] = true
		out = append(out, ShareMember{UserID: c.OwnerUserID, Character: c.Name, Player: names[c.OwnerUserID]})
	}
	return out, nil
}

func (s *stashService) ShareBox(ctx context.Context, campaignID string, a Actor, characterID, itemID string) (*ShareBoxView, error) {
	t, err := s.loadShareTarget(ctx, campaignID, a, characterID, itemID)
	if err != nil {
		return nil, err
	}
	shared, err := s.sharedUsers(ctx, campaignID, t.char.ID, t.item.ID)
	if err != nil {
		return nil, err
	}
	for i := range t.party {
		t.party[i].Checked = shared[t.party[i].UserID]
	}
	return &ShareBoxView{
		CampaignID: campaignID,
		Character:  NamedRef{ID: t.char.ID, Name: t.char.Name},
		Item:       NamedRef{ID: t.item.ID, Name: t.item.Name},
		IsMap:      t.item.HandoutMapID != "",
		Party:      t.party,
	}, nil
}

// sharedUsers is who this holder shares the item with.
func (s *stashService) sharedUsers(ctx context.Context, campaignID, characterID, itemID string) (map[string]bool, error) {
	rows, err := s.Shares.ListByCharacter(ctx, campaignID, characterID)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, r := range rows {
		if r.ItemID == itemID {
			out[r.UserID] = true
		}
	}
	return out, nil
}

func (s *stashService) Share(ctx context.Context, campaignID string, a Actor, characterID, itemID string, userIDs []string) (*ShareOutcome, error) {
	t, err := s.loadShareTarget(ctx, campaignID, a, characterID, itemID)
	if err != nil {
		return nil, err
	}
	offered := map[string]ShareMember{}
	for _, m := range t.party {
		offered[m.UserID] = m
	}
	want := map[string]bool{}
	for _, id := range userIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := offered[id]; !ok {
			return nil, apperror.NewBadRequest("You can only share with the other players in this campaign.")
		}
		want[id] = true
	}

	// Under the campaign lock so two saves, or a save and a give, can't
	// interleave their reads and writes of the allow list.
	unlock := s.locks.lock(campaignID)
	added, removed, err := s.applyShares(ctx, campaignID, a, t, want)
	unlock()
	if err != nil {
		return nil, err
	}

	out := &ShareOutcome{}
	for _, m := range t.party {
		if want[m.UserID] {
			out.SharedWith = append(out.SharedWith, m.Character)
		}
	}
	if len(added)+len(removed) > 0 && s.Auditor != nil {
		if err := s.Auditor.LogEvent(ctx, campaignID, a.UserID, shareAuditAction, map[string]any{
			"item_id": t.item.ID, "item_name": t.item.Name, "character_id": t.char.ID,
			"shared_with": added, "unshared_from": removed,
		}); err != nil {
			slog.Warn("share: could not write the activity log", slog.Any("error", err))
		}
	}
	return out, nil
}

// applyShares moves the holder's shares of the item to exactly want and
// returns who was added and who was taken off.
func (s *stashService) applyShares(ctx context.Context, campaignID string, a Actor, t *shareTarget, want map[string]bool) (added, removed []string, err error) {
	rows, err := s.Shares.ListByItem(ctx, campaignID, t.item.ID)
	if err != nil {
		return nil, nil, err
	}
	mine := map[string]ItemShare{}
	for _, r := range rows {
		if r.CharacterID == t.char.ID {
			mine[r.UserID] = r
		}
	}
	for id := range want {
		if _, ok := mine[id]; !ok {
			added = append(added, id)
		}
	}
	for id := range mine {
		if !want[id] {
			removed = append(removed, id)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)

	if len(added) > 0 {
		granted, err := s.Handouts.AllowViewers(ctx, campaignID, t.item.ID, added)
		if err != nil {
			return nil, nil, err
		}
		made := map[string]bool{}
		for _, id := range granted {
			made[id] = true
		}
		for _, id := range added {
			if err := s.Shares.Add(ctx, ItemShare{
				CampaignID: campaignID, CharacterID: t.char.ID, ItemID: t.item.ID, UserID: id,
				MadeGrant: made[id], SharedBy: a.UserID,
			}); err != nil {
				return nil, nil, err
			}
		}
	}
	for _, id := range removed {
		if err := s.unshare(ctx, campaignID, t, mine[id], rows); err != nil {
			return nil, nil, err
		}
	}
	return added, removed, nil
}

// unshare forgets one share and, when that share added the player's grant,
// takes the grant back unless something else still needs it: another
// holder's share (which then owns the grant) or the player holding the item
// on a character of their own.
func (s *stashService) unshare(ctx context.Context, campaignID string, t *shareTarget, row ItemShare, all []ItemShare) error {
	if err := s.Shares.Remove(ctx, campaignID, t.char.ID, t.item.ID, row.UserID); err != nil {
		return err
	}
	if !row.MadeGrant {
		return nil
	}
	for _, other := range all {
		if other.UserID == row.UserID && other.CharacterID != t.char.ID {
			return s.Shares.SetMadeGrant(ctx, campaignID, other.CharacterID, t.item.ID, row.UserID)
		}
	}
	holds, err := s.userHolds(ctx, campaignID, row.UserID, t.item.ID)
	if err != nil {
		return err
	}
	if holds {
		return nil
	}
	return s.Handouts.RevokeViewers(ctx, campaignID, t.item.ID, []string{row.UserID})
}

// userHolds reports whether any character the user has claimed holds the item.
func (s *stashService) userHolds(ctx context.Context, campaignID, userID, itemID string) (bool, error) {
	owned, err := s.Directory.OwnedCharacterIDs(ctx, campaignID, userID)
	if err != nil {
		return false, err
	}
	for cid := range owned {
		rel, err := s.hasItem(ctx, campaignID, cid, itemID, true)
		if err != nil {
			return false, err
		}
		if rel != nil {
			if q, _ := parseCarried(rel.Metadata); q > 0 {
				return true, nil
			}
		}
	}
	return false, nil
}

// markShares fills the panel's share facts: who each line is shared with, and
// whether the viewer, as the holder's player, gets a Share link on it. A
// failure only costs the share line and the link; the panel still shows.
func (s *stashService) markShares(ctx context.Context, campaignID string, a Actor, char *EntityRef, held []HeldItem) {
	if s.Shares == nil || len(held) == 0 {
		return
	}
	rows, err := s.Shares.ListByCharacter(ctx, campaignID, char.ID)
	if err != nil {
		slog.Warn("panel: could not read item shares", slog.Any("error", err))
		return
	}
	var names map[string]string
	if len(rows) > 0 {
		party, err := s.shareParty(ctx, campaignID, Actor{UserID: char.OwnerUserID, Role: a.Role}, char)
		if err != nil {
			slog.Warn("panel: could not read the party", slog.Any("error", err))
			return
		}
		names = map[string]string{}
		for _, m := range party {
			names[m.UserID] = m.Character
		}
	}
	byItem := map[string][]string{}
	for _, r := range rows {
		if n := names[r.UserID]; n != "" {
			byItem[r.ItemID] = append(byItem[r.ItemID], n)
		}
	}
	holderPlayer := char.OwnerUserID != "" && char.OwnerUserID == a.UserID
	for i := range held {
		held[i].SharedWith = byItem[held[i].ItemID]
		sort.Strings(held[i].SharedWith)
		if !holderPlayer {
			continue
		}
		ref, err := s.Directory.GetEntity(ctx, campaignID, held[i].ItemID)
		if err == nil && ref != nil && ref.Restricted {
			held[i].CanShare = true
		}
	}
}

// sharedMessage is the toast after a save.
func sharedMessage(o *ShareOutcome) string {
	if len(o.SharedWith) == 0 {
		return "Only you can see it now"
	}
	return "Shared with " + strings.Join(o.SharedWith, ", ")
}
