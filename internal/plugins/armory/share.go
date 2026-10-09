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
	// Character is the holder, named in the toast when nothing is shared.
	Character  string
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

	out := &ShareOutcome{Character: t.char.Name}
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
		if err := s.addShares(ctx, campaignID, a, t, added); err != nil {
			return nil, nil, err
		}
	}
	for _, id := range removed {
		if err := s.unshare(ctx, campaignID, mine[id], rows); err != nil {
			return nil, nil, err
		}
	}
	return added, removed, nil
}

// addShares records the shares first and grants second, so a crash between
// the two can't leave a grant nothing tracks (and so nothing could ever take
// back). If the grant fails the new rows are dropped again; if recording
// which grants sharing made fails, those grants are revoked with them.
func (s *stashService) addShares(ctx context.Context, campaignID string, a Actor, t *shareTarget, users []string) error {
	var recorded []string
	rollback := func(granted []string) {
		if len(granted) > 0 {
			if err := s.Handouts.RevokeViewers(ctx, campaignID, t.item.ID, granted); err != nil {
				slog.Error("share: could not undo a grant", slog.String("item_id", t.item.ID), slog.Any("error", err))
				return // keep the rows so the grant stays tracked
			}
		}
		for _, id := range recorded {
			if err := s.Shares.Remove(ctx, campaignID, t.char.ID, t.item.ID, id); err != nil {
				slog.Error("share: could not undo a share", slog.String("item_id", t.item.ID), slog.Any("error", err))
			}
		}
	}
	for _, id := range users {
		if err := s.Shares.Add(ctx, ItemShare{
			CampaignID: campaignID, CharacterID: t.char.ID, ItemID: t.item.ID, UserID: id, SharedBy: a.UserID,
		}); err != nil {
			rollback(nil)
			return err
		}
		recorded = append(recorded, id)
	}
	granted, err := s.Handouts.AllowViewers(ctx, campaignID, t.item.ID, users)
	if err != nil {
		rollback(nil)
		return err
	}
	for _, id := range granted {
		if err := s.Shares.SetMadeGrant(ctx, campaignID, t.char.ID, t.item.ID, id); err != nil {
			rollback(granted)
			return err
		}
	}
	return nil
}

// unshare takes one share back. When that share added the player's grant, the
// grant is revoked first and the row forgotten after, so a failed revoke
// leaves the row to retry from. The grant stays when something else still
// needs it: another holder who still holds the item and shares it (the grant
// passes to that share) or the player holding the item on a character of
// their own.
func (s *stashService) unshare(ctx context.Context, campaignID string, row ItemShare, all []ItemShare) error {
	forget := func() error {
		return s.Shares.Remove(ctx, campaignID, row.CharacterID, row.ItemID, row.UserID)
	}
	if !row.MadeGrant {
		return forget()
	}
	for _, other := range all {
		if other.UserID != row.UserID || other.CharacterID == row.CharacterID {
			continue
		}
		// A share whose character let go of the item is stale: it can't
		// carry the grant.
		held, err := s.holdsVisible(ctx, campaignID, other.CharacterID, row.ItemID)
		if err != nil {
			return err
		}
		if held {
			if err := s.Shares.SetMadeGrant(ctx, campaignID, other.CharacterID, row.ItemID, row.UserID); err != nil {
				return err
			}
			return forget()
		}
	}
	holds, err := s.userHolds(ctx, campaignID, row.UserID, row.ItemID)
	if err != nil {
		return err
	}
	if !holds {
		if err := s.Handouts.RevokeViewers(ctx, campaignID, row.ItemID, []string{row.UserID}); err != nil {
			return err
		}
	}
	return forget()
}

// releaseShares takes back a character's shares of an item once the last
// unit has left them: a share must not outlive the holding. Callers hold the
// campaign lock. A failure is logged, not returned: the move already
// happened, and the stale row is skipped by later hand-overs.
func (s *stashService) releaseShares(ctx context.Context, campaignID, characterID, itemID string) {
	if s.Shares == nil || s.Handouts == nil {
		return
	}
	rows, err := s.Shares.ListByItem(ctx, campaignID, itemID)
	if err != nil {
		slog.Warn("share: could not read shares to release", slog.Any("error", err))
		return
	}
	for _, r := range rows {
		if r.CharacterID != characterID {
			continue
		}
		if err := s.unshare(ctx, campaignID, r, rows); err != nil {
			slog.Warn("share: could not release a share", slog.String("item_id", itemID), slog.Any("error", err))
		}
	}
}

// ReleaseLetGoShares takes back characterID's shares of every item it no
// longer holds where its player can see it. The armory's own moves release
// shares as they go; this catches a "Has Item" line lowered, hidden or
// deleted elsewhere (the inventory widget, the relations panel). It takes the
// campaign lock, so callers must not hold it; an id that is not a character
// with shares does nothing.
func (s *stashService) ReleaseLetGoShares(ctx context.Context, campaignID, characterID string) {
	if s.Shares == nil || s.Handouts == nil || campaignID == "" || characterID == "" {
		return
	}
	unlock := s.locks.lock(campaignID)
	defer unlock()
	rows, err := s.Shares.ListByCharacter(ctx, campaignID, characterID)
	if err != nil {
		slog.Warn("share: could not read shares to check", slog.Any("error", err))
		return
	}
	checked := make(map[string]bool, len(rows))
	for _, r := range rows {
		if checked[r.ItemID] {
			continue
		}
		checked[r.ItemID] = true
		held, err := s.holdsVisible(ctx, campaignID, characterID, r.ItemID)
		if err != nil {
			slog.Warn("share: could not check a holding", slog.String("item_id", r.ItemID), slog.Any("error", err))
			continue
		}
		if !held {
			s.releaseShares(ctx, campaignID, characterID, r.ItemID)
		}
	}
}

// holdsVisible reports whether the character holds at least one unit on a
// line a player can see. A GM-only line is hidden from players, so it never
// counts as them holding the item.
func (s *stashService) holdsVisible(ctx context.Context, campaignID, characterID, itemID string) (bool, error) {
	rel, err := s.hasItem(ctx, campaignID, characterID, itemID, false)
	if err != nil || rel == nil {
		return false, err
	}
	q, _ := parseCarried(rel.Metadata)
	return q > 0, nil
}

// userHolds reports whether any character the user has claimed holds the item
// where they can see it.
func (s *stashService) userHolds(ctx context.Context, campaignID, userID, itemID string) (bool, error) {
	owned, err := s.Directory.OwnedCharacterIDs(ctx, campaignID, userID)
	if err != nil {
		return false, err
	}
	for cid := range owned {
		held, err := s.holdsVisible(ctx, campaignID, cid, itemID)
		if err != nil || held {
			return held, err
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
		// Others may still see the item through another holder's share, a
		// grant the GM set or a role, so only this share is reported gone.
		return "No longer shared by " + o.Character
	}
	return "Shared with " + strings.Join(o.SharedWith, ", ")
}
