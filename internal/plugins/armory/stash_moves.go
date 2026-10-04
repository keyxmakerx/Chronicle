// stash_moves.go moves items and money between characters and stashes.
//
// One function (run) applies a move and is used by an immediate move, by a
// GM's approval and by the sweep that happens when downtime opens, so the
// three paths cannot drift apart. Every apply holds the campaign lock and
// debits the source first with a check that it holds enough; only then does it
// credit the destination, and a failed credit puts the source back. A move is
// therefore never applied in part.
package armory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

const (
	sourceShortReason = "The source no longer has enough."
	creditFailReason  = "Could not add it to the destination, so nothing was moved."
	debitFailReason   = "Could not take it from the source, so nothing was moved."
	autoAppliedReason = "Applied when downtime opened."
	// relationRetries bounds how often a compare-and-set on a Has Item
	// relation is retried when the inventory widget changes it concurrently.
	relationRetries = 4
	// maxPendingPerUser bounds how many requests one player can leave waiting.
	maxPendingPerUser = 20
)

// moveRefs are the character entities a move touches (nil for stash ends).
type moveRefs struct {
	from, to *EntityRef
}

// refsFor loads the character ends of a move, scoped to the campaign.
func (s *stashService) refsFor(ctx context.Context, m *Move) (moveRefs, error) {
	var r moveRefs
	var err error
	if m.From.Kind == EndpointCharacter {
		if r.from, err = s.loadCharacter(ctx, m.CampaignID, m.From.ID); err != nil {
			return r, err
		}
	}
	if m.To.Kind == EndpointCharacter {
		if r.to, err = s.loadCharacter(ctx, m.CampaignID, m.To.ID); err != nil {
			return r, err
		}
	}
	return r, nil
}

// --- Has Item relation helpers ---

// parseCarried returns the quantity and the whole stored object of a Has Item
// relation's metadata. A missing or unreadable quantity means one, matching the
// inventory widget.
func parseCarried(raw []byte) (int, map[string]json.RawMessage) {
	fields := map[string]json.RawMessage{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
			fields = map[string]json.RawMessage{}
		}
	}
	qty := 1
	if v, ok := fields["quantity"]; ok {
		var f float64
		if err := json.Unmarshal(v, &f); err == nil && f >= 0 && f == math.Trunc(f) && f < 1e9 {
			qty = int(f)
		}
	}
	return qty, fields
}

// withCarriedQuantity rewrites only "quantity", keeping every other key
// (equipped, attuned, notes) the inventory widget stores.
func withCarriedQuantity(fields map[string]json.RawMessage, q int) ([]byte, error) {
	out := make(map[string]json.RawMessage, len(fields)+1)
	for k, v := range fields {
		out[k] = v
	}
	out["quantity"] = json.RawMessage(strconv.Itoa(q))
	return json.Marshal(out)
}

// hasItem finds the character's Has Item line for an item. A DM-only line is
// invisible to anyone but the GM: for them it is as if the item isn't held, so
// a refusal can't reveal it exists.
func (s *stashService) hasItem(ctx context.Context, campaignID, characterID, itemID string, allowDM bool) (*HasItemRelation, error) {
	rels, err := s.Relations.ListByCharacter(ctx, campaignID, characterID)
	if err != nil {
		return nil, err
	}
	for i := range rels {
		if rels[i].DmOnly && !allowDM {
			continue
		}
		if rels[i].ItemEntityID == itemID {
			return &rels[i], nil
		}
	}
	return nil, nil
}

// carried lists what a character holds, for the panel.
func (s *stashService) carried(ctx context.Context, campaignID string, a Actor, characterID string) ([]HeldItem, error) {
	rels, err := s.Relations.ListByCharacter(ctx, campaignID, characterID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(rels))
	for _, r := range rels {
		if r.DmOnly && !a.IsGM() {
			continue
		}
		ids = append(ids, r.ItemEntityID)
	}
	var viewable map[string]bool
	if !a.IsOwner() && len(ids) > 0 {
		if viewable, err = s.Visibility.FilterViewableEntityIDs(ctx, campaignID, ids, a.Role, a.UserID); err != nil {
			return nil, err
		}
	}
	var out []HeldItem
	for _, r := range rels {
		if r.DmOnly && !a.IsGM() {
			continue
		}
		if viewable != nil && !viewable[r.ItemEntityID] {
			continue
		}
		q, _ := parseCarried(r.Metadata)
		if q < 1 {
			continue
		}
		out = append(out, HeldItem{ItemID: r.ItemEntityID, Name: r.ItemName, Quantity: q})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}

// dropEmptyCarried removes a line left at quantity 0 by a take that kept it.
func (s *stashService) dropEmptyCarried(ctx context.Context, campaignID, characterID, itemID string) {
	rel, err := s.hasItem(ctx, campaignID, characterID, itemID, true)
	if err != nil || rel == nil {
		return
	}
	if q, _ := parseCarried(rel.Metadata); q != 0 {
		return
	}
	if err := s.Relations.Delete(ctx, rel.ID); err != nil {
		// Harmless: a 0-quantity line is hidden from the panel and re-used by
		// the next credit.
		slog.Warn("stash move: could not remove an emptied line", slog.Any("error", err))
	}
}

// adjustCarried changes a character's quantity of an item by delta (negative
// takes). Taking reports false, changing nothing, when they hold too little.
// keepZero leaves a line that reaches 0 in place (quantity 0) instead of
// deleting it, so the original metadata (notes, attuned, equipped, dm_only)
// survives until the move is certain; dropEmptyCarried removes it afterwards.
// allowDM lets the call see DM-only lines (GM only).
// The write is a compare-and-set on the relation's metadata so the inventory
// widget editing the same line at the same moment cannot be overwritten.
func (s *stashService) adjustCarried(ctx context.Context, campaignID, characterID, itemID, createdBy string, delta int, allowDM, keepZero bool) (bool, error) {
	for attempt := 0; attempt < relationRetries; attempt++ {
		rel, err := s.hasItem(ctx, campaignID, characterID, itemID, allowDM)
		if err != nil {
			return false, err
		}
		if rel == nil {
			if delta < 0 {
				return false, nil
			}
			meta, _ := json.Marshal(map[string]any{"quantity": delta, "equipped": false})
			if _, err := s.Relations.Create(ctx, campaignID, characterID, itemID, createdBy, meta); err != nil {
				// A concurrent add of the same line makes Create conflict;
				// the next pass finds it and increments instead.
				var ae *apperror.AppError
				if errors.As(err, &ae) && ae.Code == 409 {
					continue
				}
				return false, err
			}
			return true, nil
		}

		held, fields := parseCarried(rel.Metadata)
		next := held + delta
		if next < 0 {
			return false, nil
		}
		if next == 0 && !keepZero {
			if err := s.Relations.Delete(ctx, rel.ID); err != nil {
				return false, err
			}
			return true, nil
		}
		meta, err := withCarriedQuantity(fields, next)
		if err != nil {
			return false, err
		}
		if len(rel.Metadata) == 0 {
			// Nothing stored to compare against; the campaign lock keeps
			// this plugin's own writers out.
			if err := s.Relations.UpdateMetadata(ctx, rel.ID, meta); err != nil {
				return false, err
			}
			return true, nil
		}
		wrote, err := s.Relations.UpdateMetadataIf(ctx, rel.ID, rel.Metadata, meta)
		if err != nil {
			return false, err
		}
		if wrote {
			return true, nil
		}
	}
	return false, apperror.NewConflict("That character's items are being edited. Try again.")
}

// --- character money ---

// toCents reads a numeric sheet value (a number, or a string numeral) as cents.
func toCents(v any) (Cents, error) {
	switch n := v.(type) {
	case nil:
		return 0, nil
	case float64:
		return Cents(math.Round(n * 100)), nil
	case float32:
		return Cents(math.Round(float64(n) * 100)), nil
	case int:
		return Cents(n) * 100, nil
	case int64:
		return Cents(n) * 100, nil
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return 0, err
		}
		return Cents(math.Round(f * 100)), nil
	case string:
		t := strings.TrimSpace(n)
		if t == "" {
			return 0, nil
		}
		f, err := strconv.ParseFloat(t, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, fmt.Errorf("money value %q is not a number", n)
		}
		return Cents(math.Round(f * 100)), nil
	}
	return 0, fmt.Errorf("money value of type %T is not a number", v)
}

func (s *stashService) characterMoney(ctx context.Context, ref *EntityRef) (Cents, error) {
	if s.Fields == nil || ref.MoneyKey == "" {
		return 0, apperror.NewBadRequest(noMoneyMessage)
	}
	fields, err := s.Fields.GetEntityFields(ctx, ref.ID)
	if err != nil {
		return 0, err
	}
	c, err := toCents(fields[ref.MoneyKey])
	if err != nil {
		return 0, apperror.NewBadRequest("That character's money isn't a number, so it can't be moved.")
	}
	return c, nil
}

// adjustCharacterMoney adds delta (negative takes) to the sheet field, never
// letting it go below zero. Taking reports false when the character has less.
func (s *stashService) adjustCharacterMoney(ctx context.Context, ref *EntityRef, delta Cents) (bool, error) {
	cur, err := s.characterMoney(ctx, ref)
	if err != nil {
		return false, err
	}
	next := cur + delta
	if next < 0 {
		return false, nil
	}
	// The field stays numeric: a number, not the text form.
	if err := s.Fields.UpdateEntityFields(ctx, ref.ID, map[string]any{ref.MoneyKey: float64(next) / 100}); err != nil {
		return false, err
	}
	return true, nil
}

// purseCounts reads the character's whole-coin counts for every coin the sheet
// has. A fractional coin count (2.5 gp) is floored: payment works in whole
// coins and adjustPurse keeps the fraction when it writes.
func (s *stashService) purseCounts(ctx context.Context, ref *EntityRef) (map[string]int64, error) {
	counts, _, err := s.readPurse(ctx, ref)
	return counts, err
}

// readPurse returns the whole-coin counts and the raw cents behind them.
func (s *stashService) readPurse(ctx context.Context, ref *EntityRef) (counts map[string]int64, cents map[string]Cents, err error) {
	if s.Fields == nil || len(ref.Purse) == 0 {
		return nil, nil, apperror.NewBadRequest(noMoneyMessage)
	}
	fields, err := s.Fields.GetEntityFields(ctx, ref.ID)
	if err != nil {
		return nil, nil, err
	}
	counts, cents, err = coinCounts(ref.Purse, fields)
	if err != nil {
		return nil, nil, apperror.NewBadRequest("That character's coins aren't numbers, so they can't be spent.")
	}
	return counts, cents, nil
}

// adjustPurse adds deltas (whole coins, negative spends) to the character's coin
// fields. It re-reads the purse and refuses, writing nothing, if any coin would
// go below zero. Every changed coin is written in one field update so a purse
// is never left half-changed.
func (s *stashService) adjustPurse(ctx context.Context, ref *EntityRef, deltas map[string]int64) (bool, error) {
	_, cents, err := s.readPurse(ctx, ref)
	if err != nil {
		return false, err
	}
	write := make(map[string]any, len(deltas))
	for coin, d := range deltas {
		key, ok := ref.Purse[coin]
		if !ok {
			return false, nil
		}
		next := cents[coin] + Cents(d)*100
		if next < 0 {
			return false, nil
		}
		write[key] = float64(next) / 100
	}
	if len(write) == 0 {
		return true, nil
	}
	if err := s.Fields.UpdateEntityFields(ctx, ref.ID, write); err != nil {
		return false, err
	}
	return true, nil
}

// --- holdings, debit and credit ---

// holds reports whether the source currently has enough for m.
func (s *stashService) holds(ctx context.Context, m *Move, refs moveRefs) (bool, error) {
	switch {
	case m.Kind == MoveKindItem && m.From.Kind == EndpointCharacter:
		rel, err := s.hasItem(ctx, m.CampaignID, m.From.ID, m.ItemEntityID, m.byGM)
		if err != nil || rel == nil {
			return false, err
		}
		q, _ := parseCarried(rel.Metadata)
		return q >= m.Quantity, nil
	case m.Kind == MoveKindItem:
		sid, _ := m.From.StashID()
		all, err := s.Repo.ListItems(ctx, m.CampaignID)
		if err != nil {
			return false, err
		}
		for _, r := range all[sid] {
			if r.ItemEntityID == m.ItemEntityID {
				return r.Quantity >= m.Quantity, nil
			}
		}
		return false, nil
	case m.From.Kind == EndpointCharacter:
		bal, err := s.characterMoney(ctx, refs.from)
		return bal >= m.Amount, err
	default:
		sid, _ := m.From.StashID()
		st, err := s.Repo.GetStash(ctx, m.CampaignID, sid)
		if err != nil {
			return false, err
		}
		return st.Money >= m.Amount, nil
	}
}

// debit takes m's contents from end. It reports false when end holds too little.
func (s *stashService) debit(ctx context.Context, m *Move, end Endpoint, ref *EntityRef) (bool, error) {
	switch {
	case m.Kind == MoveKindItem && end.Kind == EndpointCharacter:
		return s.adjustCarried(ctx, m.CampaignID, end.ID, m.ItemEntityID, m.RequestedBy, -m.Quantity, m.byGM, true)
	case m.Kind == MoveKindItem:
		sid, _ := end.StashID()
		return s.Repo.DebitItem(ctx, m.CampaignID, sid, m.ItemEntityID, m.Quantity)
	case end.Kind == EndpointCharacter:
		return s.adjustCharacterMoney(ctx, ref, -m.Amount)
	default:
		sid, _ := end.StashID()
		return s.Repo.DebitMoney(ctx, m.CampaignID, sid, m.Amount)
	}
}

// credit adds m's contents to end.
func (s *stashService) credit(ctx context.Context, m *Move, end Endpoint, ref *EntityRef) error {
	switch {
	case m.Kind == MoveKindItem && end.Kind == EndpointCharacter:
		ok, err := s.adjustCarried(ctx, m.CampaignID, end.ID, m.ItemEntityID, m.RequestedBy, m.Quantity, m.byGM, false)
		if err == nil && !ok {
			err = errors.New("credit refused")
		}
		return err
	case m.Kind == MoveKindItem:
		sid, _ := end.StashID()
		return s.Repo.CreditItem(ctx, m.CampaignID, sid, m.ItemEntityID, m.Quantity)
	case end.Kind == EndpointCharacter:
		ok, err := s.adjustCharacterMoney(ctx, ref, m.Amount)
		if err == nil && !ok {
			err = errors.New("credit refused")
		}
		return err
	default:
		sid, _ := end.StashID()
		return s.Repo.CreditMoney(ctx, m.CampaignID, sid, m.Amount)
	}
}

// run applies a move and returns its final status and reason. The caller holds
// the campaign lock. It never applies a move in part: if the destination can't
// take it, the source is restored and the move is reported failed.
func (s *stashService) run(ctx context.Context, m *Move, refs moveRefs) (status, reason string) {
	ok, err := s.debit(ctx, m, m.From, refs.from)
	if err != nil {
		slog.Error("stash move: debit failed", slog.Int64("move_id", m.ID), slog.Any("error", err))
		return MoveFailed, debitFailReason
	}
	if !ok {
		return MoveFailed, sourceShortReason
	}
	if err := s.credit(ctx, m, m.To, refs.to); err != nil {
		slog.Error("stash move: credit failed, restoring source", slog.Int64("move_id", m.ID), slog.Any("error", err))
		if rerr := s.credit(ctx, m, m.From, refs.from); rerr != nil {
			// The source can't be put back. Losing track silently would be
			// worse than a loud log: someone has to look at this move.
			slog.Error("stash move: RESTORE FAILED, source is short", slog.Int64("move_id", m.ID),
				slog.String("campaign_id", m.CampaignID), slog.Any("error", rerr))
		}
		return MoveFailed, creditFailReason
	}
	if m.Kind == MoveKindItem && m.From.Kind == EndpointCharacter {
		s.dropEmptyCarried(ctx, m.CampaignID, m.From.ID, m.ItemEntityID)
	}
	return MoveApplied, ""
}

// --- validation ---

func insufficientMessage(m *Move) string {
	if m.Kind == MoveKindMoney {
		return "There isn't that much money to move."
	}
	return "There isn't enough of that item to move."
}

// validate checks the request and who is asking, returning the move to run.
// Nothing here changes any data.
func (s *stashService) validate(ctx context.Context, campaignID string, a Actor, in MoveInput) (*Move, moveRefs, error) {
	var refs moveRefs
	m := &Move{CampaignID: campaignID, Kind: in.Kind, From: in.From, To: in.To, RequestedBy: a.UserID, byGM: a.IsGM()}

	if !in.From.Valid() || !in.To.Valid() {
		return nil, refs, apperror.NewBadRequest("Choose where it is going.")
	}
	if in.From == in.To {
		return nil, refs, apperror.NewBadRequest("It is already there. Choose somewhere else.")
	}
	switch in.Kind {
	case MoveKindItem:
		if in.Quantity < 1 || in.Quantity > maxMoveQuantity {
			return nil, refs, apperror.NewBadRequest("Enter a quantity of at least 1.")
		}
		m.Quantity, m.ItemEntityID = in.Quantity, strings.TrimSpace(in.ItemEntityID)
		if m.ItemEntityID == "" {
			return nil, refs, apperror.NewBadRequest("Choose an item.")
		}
	case MoveKindMoney:
		c, err := ParseCents(in.Amount)
		if err != nil {
			return nil, refs, err
		}
		m.Amount = c
	default:
		return nil, refs, apperror.NewBadRequest("Choose an item or money to move.")
	}

	// Source: a character the actor can act as, or a stash they can see.
	if in.From.Kind == EndpointCharacter {
		ref, err := s.loadCharacter(ctx, campaignID, in.From.ID)
		if err != nil {
			return nil, refs, err
		}
		ok, err := s.canAct(ctx, campaignID, a, ref.ID)
		if err != nil {
			return nil, refs, err
		}
		if !ok {
			return nil, refs, forbidden()
		}
		refs.from = ref
	} else if err := s.requireVisibleStash(ctx, campaignID, a, in.From); err != nil {
		return nil, refs, err
	}

	// Destination: any character the actor can view, or a stash they can see.
	if in.To.Kind == EndpointCharacter {
		ref, err := s.loadCharacter(ctx, campaignID, in.To.ID)
		if err != nil {
			return nil, refs, err
		}
		ok, err := s.viewable(ctx, campaignID, a, ref.ID)
		if err != nil {
			return nil, refs, err
		}
		if !ok {
			return nil, refs, notFound("character")
		}
		refs.to = ref
	} else if err := s.requireVisibleStash(ctx, campaignID, a, in.To); err != nil {
		return nil, refs, err
	}

	if in.Kind == MoveKindItem {
		ref, err := s.Directory.GetEntity(ctx, campaignID, m.ItemEntityID)
		if err != nil {
			return nil, refs, err
		}
		if ref == nil || !ref.IsItem {
			return nil, refs, notFound("item")
		}
		ok, err := s.viewable(ctx, campaignID, a, ref.ID)
		if err != nil {
			return nil, refs, err
		}
		if !ok {
			return nil, refs, notFound("item")
		}
	} else {
		for _, r := range []*EntityRef{refs.from, refs.to} {
			if r != nil && r.MoneyKey == "" {
				return nil, refs, apperror.NewBadRequest(noMoneyMessage)
			}
		}
	}
	return m, refs, nil
}

func (s *stashService) requireVisibleStash(ctx context.Context, campaignID string, a Actor, e Endpoint) error {
	id, ok := e.StashID()
	if !ok {
		return notFound("stash")
	}
	_, err := s.visibleStash(ctx, campaignID, a, id)
	return err
}

// --- the operations ---

func (s *stashService) Move(ctx context.Context, campaignID string, a Actor, in MoveInput) (*MoveOutcome, error) {
	m, refs, err := s.validate(ctx, campaignID, a, in)
	if err != nil {
		return nil, err
	}

	// Registered before the lock so the events go out after it is released.
	var ev eventBatch
	defer ev.flush(s.Events, campaignID)
	unlock := s.locks.lock(campaignID)
	defer unlock()

	// Read the switch under the lock: opening downtime sets it and sweeps the
	// queue under the same lock, so a request can't slip in after the sweep and
	// wait for a downtime that already opened.
	open, err := s.IsDowntimeOpen(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	enough, err := s.holds(ctx, m, refs)
	if err != nil {
		return nil, err
	}
	if !enough {
		return nil, apperror.NewBadRequest(insufficientMessage(m))
	}

	if a.IsGM() || open {
		// Record the move as pending BEFORE touching any balance, then settle
		// it. If the insert fails nothing has moved; if the settle fails after
		// the move, it is logged loudly rather than silently unrecorded.
		m.Status = MovePending
		if err := s.Repo.InsertMove(ctx, m); err != nil {
			return nil, err
		}
		m.Status, m.Reason = s.run(ctx, m, refs)
		wrote, err := s.Repo.SettleMove(ctx, campaignID, m.ID, m.Status, m.Reason, "")
		if err == nil && !wrote {
			err = errors.New("move was no longer pending")
		}
		if err != nil {
			// One retry; after that the row stays pending although the move
			// ran, so say so where an operator will see it.
			if wrote, err = s.Repo.SettleMove(ctx, campaignID, m.ID, m.Status, m.Reason, ""); err != nil || !wrote {
				slog.Error("stash move: APPLIED BUT NOT RECORDED, move row still pending",
					slog.Int64("move_id", m.ID), slog.String("campaign_id", campaignID),
					slog.String("status", m.Status), slog.Any("error", err))
				return nil, apperror.NewInternal(fmt.Errorf("recording move %d: %w", m.ID, err))
			}
		}
		ev.moved(m)
		if m.Status != MoveApplied {
			return nil, apperror.NewConflict(m.Reason)
		}
		return &MoveOutcome{Move: *m, Applied: true}, nil
	}

	mine, err := s.Repo.ListMoves(ctx, campaignID, MoveFilter{RequestedBy: a.UserID, Status: MovePending, Limit: maxPendingPerUser + 1})
	if err != nil {
		return nil, err
	}
	if len(mine) >= maxPendingPerUser {
		return nil, apperror.NewBadRequest(fmt.Sprintf("You have %d requests waiting for the GM already.", maxPendingPerUser))
	}

	m.Status = MovePending
	if err := s.Repo.InsertMove(ctx, m); err != nil {
		return nil, err
	}
	ev.requested(m)
	return &MoveOutcome{Move: *m}, nil
}

// answerable loads a pending request, enforcing Owner visibility.
func (s *stashService) answerable(ctx context.Context, campaignID string, a Actor, id int64) (*Move, error) {
	if !a.IsOwner() {
		return nil, forbidden()
	}
	m, err := s.Repo.GetMove(ctx, campaignID, id)
	if err != nil {
		return nil, err
	}
	if m.Status != MovePending {
		return nil, apperror.NewConflict("That request has already been answered.")
	}
	return m, nil
}

func (s *stashService) Approve(ctx context.Context, campaignID string, a Actor, moveID int64) (*Move, error) {
	if _, err := s.answerable(ctx, campaignID, a, moveID); err != nil {
		return nil, err
	}
	var ev eventBatch
	defer ev.flush(s.Events, campaignID)
	unlock := s.locks.lock(campaignID)
	defer unlock()
	// Re-read under the lock: another answer may have landed while we waited.
	m, err := s.answerable(ctx, campaignID, a, moveID)
	if err != nil {
		return nil, err
	}
	if err := s.settlePending(ctx, m, a.UserID, ""); err != nil {
		return nil, err
	}
	ev.moved(m)
	ev.settled(m)
	return m, nil
}

// settlePending runs a pending move now and records how it went.
func (s *stashService) settlePending(ctx context.Context, m *Move, decidedBy, okReason string) error {
	refs, err := s.refsFor(ctx, m)
	if err != nil {
		m.Status, m.Reason = MoveFailed, "That character no longer exists."
	} else {
		m.Status, m.Reason = s.run(ctx, m, refs)
		if m.Status == MoveApplied {
			m.Reason = okReason
		}
	}
	m.DecidedBy = decidedBy
	wrote, err := s.Repo.SettleMove(ctx, m.CampaignID, m.ID, m.Status, m.Reason, decidedBy)
	if err != nil {
		return err
	}
	if !wrote {
		return apperror.NewConflict("That request has already been answered.")
	}
	return nil
}

func (s *stashService) Decline(ctx context.Context, campaignID string, a Actor, moveID int64) (*Move, error) {
	if _, err := s.answerable(ctx, campaignID, a, moveID); err != nil {
		return nil, err
	}
	var ev eventBatch
	defer ev.flush(s.Events, campaignID)
	unlock := s.locks.lock(campaignID)
	defer unlock()
	m, err := s.answerable(ctx, campaignID, a, moveID)
	if err != nil {
		return nil, err
	}
	wrote, err := s.Repo.SettleMove(ctx, campaignID, moveID, MoveDeclined, "", a.UserID)
	if err != nil {
		return nil, err
	}
	if !wrote {
		return nil, apperror.NewConflict("That request has already been answered.")
	}
	m.Status, m.DecidedBy = MoveDeclined, a.UserID
	ev.settled(m)
	return m, nil
}

func (s *stashService) SetDowntime(ctx context.Context, campaignID string, a Actor, open bool) (DowntimeResult, error) {
	var res DowntimeResult
	if !a.IsOwner() {
		return res, forbidden()
	}
	var ev eventBatch
	defer ev.flush(s.Events, campaignID)
	unlock := s.locks.lock(campaignID)
	defer unlock()
	if err := s.Repo.SetDowntime(ctx, campaignID, open, a.UserID); err != nil {
		return res, err
	}
	ev.downtimeChanged(open)
	if !open {
		return res, nil
	}
	pending, err := s.Repo.ListPending(ctx, campaignID)
	if err != nil {
		return res, err
	}
	// Oldest first, each through the same apply path. One request that can't
	// go through is marked failed and the rest carry on.
	for i := range pending {
		m := pending[i]
		if err := s.settlePending(ctx, &m, a.UserID, autoAppliedReason); err != nil {
			slog.Error("downtime sweep: settling a request failed", slog.Int64("move_id", m.ID), slog.Any("error", err))
			res.Failed++
			continue
		}
		ev.moved(&m)
		ev.settled(&m)
		if m.Status == MoveApplied {
			res.Applied++
		} else {
			res.Failed++
		}
	}
	// Waiting purchase requests go through the same lock and the same Buy
	// logic, after the moves so any coins a move delivers are counted.
	if s.purchases != nil {
		applied, failed := s.purchases.sweepRequests(ctx, campaignID, a.UserID)
		res.Applied += applied
		res.Failed += failed
	}
	return res, nil
}

// --- move dialog ---

func (s *stashService) MoveDialog(ctx context.Context, campaignID string, a Actor, kind string, from Endpoint, itemID string) (*MoveDialogView, error) {
	if kind != MoveKindItem && kind != MoveKindMoney {
		return nil, apperror.NewBadRequest("Choose an item or money to move.")
	}
	if !from.Valid() {
		return nil, notFound("source")
	}
	view := &MoveDialogView{CampaignID: campaignID, Kind: kind, From: from}
	var fromRef *EntityRef

	if from.Kind == EndpointCharacter {
		ref, err := s.loadCharacter(ctx, campaignID, from.ID)
		if err != nil {
			return nil, err
		}
		ok, err := s.canAct(ctx, campaignID, a, ref.ID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, forbidden()
		}
		fromRef, view.FromName = ref, ref.Name
	} else {
		id, ok := from.StashID()
		if !ok {
			return nil, notFound("stash")
		}
		st, err := s.visibleStash(ctx, campaignID, a, id)
		if err != nil {
			return nil, err
		}
		view.FromName = st.Name
	}

	m := &Move{CampaignID: campaignID, Kind: kind, From: from}
	if kind == MoveKindItem {
		ref, err := s.Directory.GetEntity(ctx, campaignID, itemID)
		if err != nil {
			return nil, err
		}
		if ref == nil || !ref.IsItem {
			return nil, notFound("item")
		}
		if ok, err := s.viewable(ctx, campaignID, a, ref.ID); err != nil {
			return nil, err
		} else if !ok {
			return nil, notFound("item")
		}
		view.ItemID, view.ItemName = ref.ID, ref.Name
		m.ItemEntityID, m.Quantity = ref.ID, 1
		max, err := s.heldItemQuantity(ctx, campaignID, from, ref.ID, a.IsGM())
		if err != nil {
			return nil, err
		}
		view.Max = max
	} else {
		bal, err := s.heldMoney(ctx, campaignID, from, fromRef)
		if err != nil {
			return nil, err
		}
		view.MaxMoney = bal
	}

	dests, err := s.Destinations(ctx, campaignID, a, kind, from)
	if err != nil {
		return nil, err
	}
	view.Destinations = dests

	open, err := s.IsDowntimeOpen(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	view.Immediate = a.IsGM() || open
	return view, nil
}

// Destinations lists where the actor may send a move from `from`: stashes they
// see, then characters they can view. For money, characters with no money
// field are left out because the move would be refused.
func (s *stashService) Destinations(ctx context.Context, campaignID string, a Actor, kind string, from Endpoint) ([]MoveDestination, error) {
	var out []MoveDestination
	stashes, err := s.Repo.ListStashes(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	viewers, err := s.Repo.ListViewers(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	for _, st := range stashes {
		ep := StashEndpoint(st.ID)
		if ep == from {
			continue
		}
		if ok, err := s.stashVisible(ctx, campaignID, a, viewers[st.ID]); err != nil {
			return nil, err
		} else if !ok {
			continue
		}
		out = append(out, MoveDestination{Endpoint: ep, Label: st.Name, Group: "Stashes"})
	}
	chars, err := s.Directory.ListCharacters(ctx, campaignID, a.Role, a.UserID)
	if err != nil {
		return nil, err
	}
	for _, c := range chars {
		if from.Kind == EndpointCharacter && from.ID == c.ID {
			continue
		}
		if kind == MoveKindMoney && c.MoneyKey == "" {
			continue
		}
		out = append(out, MoveDestination{
			Endpoint: Endpoint{Kind: EndpointCharacter, ID: c.ID}, Label: c.Name, Group: "Characters"})
	}
	return out, nil
}

func (s *stashService) heldItemQuantity(ctx context.Context, campaignID string, from Endpoint, itemID string, allowDM bool) (int, error) {
	if from.Kind == EndpointCharacter {
		rel, err := s.hasItem(ctx, campaignID, from.ID, itemID, allowDM)
		if err != nil || rel == nil {
			return 0, err
		}
		q, _ := parseCarried(rel.Metadata)
		return q, nil
	}
	sid, _ := from.StashID()
	all, err := s.Repo.ListItems(ctx, campaignID)
	if err != nil {
		return 0, err
	}
	for _, r := range all[sid] {
		if r.ItemEntityID == itemID {
			return r.Quantity, nil
		}
	}
	return 0, nil
}

func (s *stashService) heldMoney(ctx context.Context, campaignID string, from Endpoint, ref *EntityRef) (Cents, error) {
	if from.Kind == EndpointCharacter {
		if ref == nil || ref.MoneyKey == "" {
			return 0, apperror.NewBadRequest(noMoneyMessage)
		}
		return s.characterMoney(ctx, ref)
	}
	sid, _ := from.StashID()
	st, err := s.Repo.GetStash(ctx, campaignID, sid)
	if err != nil {
		return 0, err
	}
	return st.Money, nil
}
