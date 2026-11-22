// Hand-written SQL for decks, cards, copies and the move log.
//
// SECURITY: every statement filters on campaign_id (directly or through the
// deck), so an id from another campaign never reads or changes anything here.
// Every move claims its copy with a conditional update and checks the rows
// it changed, so two people can never take the same copy.
package decks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

// Place is where a copy sits: a zone, the hero page holding it (hand or play
// in front of a hero; "" is nobody, the whole table, or on a party deck the
// party's own hand) and which way up. Open is a face-up card in a hand shown
// to the table; every Place that leaves a hand has it false.
type Place struct {
	Zone           string
	HolderEntityID string
	FaceUp         bool
	Open           bool
}

// AskedChange says what a move does to a copy's waiting-for-a-Director mark.
type AskedChange int

const (
	// AskedKeep leaves the mark as it is.
	AskedKeep AskedChange = iota
	// AskedMark stamps it: a player asked to play an ask-first card.
	AskedMark
	// AskedClear removes it.
	AskedClear
)

// Move is one claimed change of a copy. It applies only while the copy is
// still exactly in From, so a lost race changes nothing.
type Move struct {
	CampaignID string
	DeckID     string
	CopyID     string
	From       Place
	To         Place
	// ExpectAsked requires the copy to be waiting for a Director (approve and
	// turn down).
	ExpectAsked bool
	Asked       AskedChange
	// Turned stamps turned_at.
	Turned bool
	// DealtBy stamps dealt_by and dealt_at; "" leaves them (and clears them
	// when the copy goes back to the pile or the shown spot).
	DealtBy string
	// Log is written in the same transaction; CardID and CopyID are filled
	// from the copy.
	Log LogLine
}

// PickChange says which of a deck's two draw nudges an update changes, so a
// save that names neither never writes them back over a draw that cleared one.
type PickChange struct {
	SetNext, SetLast bool
	Next, Last       string
}

// DrawPlan is one random draw of Count copies, done in one transaction.
type DrawPlan struct {
	CampaignID string
	DeckID     string
	// Count is how many copies to take; 0 is one.
	Count int
	// To is where each drawn copy lands.
	To Place
	// DealtBy stamps a copy that lands in a hand or in play.
	DealtBy string
	// CardID limits the draw to one card's copies in the pile ("deal a named
	// card").
	CardID string
	// PutBackCopyID returns that shown copy to the pile first and never draws
	// it again when another copy exists.
	PutBackCopyID string
	// Reshuffle moves the used pile into the pile when the pile has nothing
	// to draw.
	Reshuffle bool
	// SpendUserID spends one of that player's draws; "" spends nothing.
	SpendUserID string
	// Pick returns a random index below n; the service supplies crypto
	// randomness and tests supply a fixed one.
	Pick func(n int) int
	Log  LogLine
}

// KeepPlan copies one card into another deck and sends the source copy to
// gone, in one transaction.
type KeepPlan struct {
	CampaignID   string
	DeckID       string
	CopyID       string
	From         Place
	TargetDeckID string
	// TargetKinds is the target deck's kind keys, so the card's kind carries
	// over only where the target has one by that key.
	TargetKinds map[string]bool
	Log         LogLine
}

// ResizeResult reports a copies change.
type ResizeResult struct{ Added, Removed int }

// Repository is the persistence boundary for decks.
type Repository interface {
	CountDecks(ctx context.Context, campaignID string) (int, error)
	// ListDecks lists the campaign's decks with their chosen players.
	ListDecks(ctx context.Context, campaignID string) ([]Deck, error)
	GetDeck(ctx context.Context, campaignID, id string) (Deck, bool, error)
	// CreateDeck returns errDuplicate when the name is taken.
	CreateDeck(ctx context.Context, d Deck) error
	// UpdateDeck writes d if its stored version is still expected; ok is
	// false on a lost race. errDuplicate when the new name is taken. The draw
	// nudges are written only as picks says.
	UpdateDeck(ctx context.Context, d Deck, expected int, picks PickChange) (ok bool, err error)
	DeleteDeck(ctx context.Context, campaignID, id string) error
	// SetPlayers replaces the chosen players if the stored version is still
	// expected, and bumps it.
	SetPlayers(ctx context.Context, campaignID, deckID string, players []Player, expected int) (ok bool, err error)

	ListCards(ctx context.Context, campaignID, deckID string) ([]Card, error)
	GetCard(ctx context.Context, campaignID, deckID, id string) (Card, bool, error)
	// CreateCard stores the card and its copies, all in the pile.
	// errDuplicate when the name is taken.
	CreateCard(ctx context.Context, c Card) error
	// UpdateCard writes the card and brings its copies to c.Copies: it adds
	// pile copies, or removes them from the pile, then used, then gone. It
	// returns a *heldError, changing nothing, when more would have to come
	// out of a hand or out of play.
	UpdateCard(ctx context.Context, c Card) (ResizeResult, error)
	// DeleteCard removes the card and its copies, and clears the deck's next
	// or last card when it was this one.
	DeleteCard(ctx context.Context, campaignID, deckID, id string) error

	ListCopies(ctx context.Context, campaignID, deckID string) ([]Copy, error)
	GetCopy(ctx context.Context, campaignID, deckID, id string) (Copy, bool, error)
	// ListHeldBy lists the copies in hand or in play held by these pages.
	ListHeldBy(ctx context.Context, campaignID string, holderIDs []string) ([]Copy, error)
	// ZoneCounts counts every deck's copies by zone.
	ZoneCounts(ctx context.Context, campaignID string) (map[string]ZoneCounts, error)
	// ListTable lists the face-up copies in play in front of nobody, with the
	// face of their card and their deck's name and look.
	ListTable(ctx context.Context, campaignID string) ([]TableRow, error)

	// Draw takes p.Count copies (one by default) or none at all.
	Draw(ctx context.Context, p DrawPlan) ([]Copy, error)
	Move(ctx context.Context, m Move) error
	// Keep returns errLostClaim when the copy moved first, and errTargetFull,
	// errTargetCopies or errCardCopies, changing nothing, when the target
	// deck's limits would be passed.
	Keep(ctx context.Context, k KeepPlan) error
	// Shuffle moves the used pile back to the pile and returns how many.
	Shuffle(ctx context.Context, campaignID, deckID string, log LogLine) (int, error)
	// Gather moves every copy to the pile and returns how many moved.
	Gather(ctx context.Context, campaignID, deckID string, log LogLine) (int, error)
	ListLog(ctx context.Context, campaignID, deckID string, limit int) ([]LogLine, error)
}

// TableRow is a card on the table with what a viewer needs to draw it. It
// holds no Director's note or condition because the query never reads them.
type TableRow struct {
	CopyID   string
	DeckID   string
	DeckName string
	DeckKind string
	Look     Look
	Face     Face
}

type sqlRepository struct{ db *sql.DB }

// NewRepository creates a MariaDB-backed repository.
func NewRepository(db *sql.DB) Repository { return &sqlRepository{db: db} }

func isDuplicate(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func fromNull(n sql.NullString) string {
	if n.Valid {
		return n.String
	}
	return ""
}

func timePtr(n sql.NullTime) *time.Time {
	if n.Valid {
		t := n.Time
		return &t
	}
	return nil
}

func rollback(tx *sql.Tx) { _ = tx.Rollback() }

// ---- decks ----------------------------------------------------------------

const deckColumns = `id, campaign_id, name, kind, holder_entity_id, next_card_id, last_card_id, settings, look, version, sort_order, created_at, updated_at`

type scanner interface{ Scan(...any) error }

func scanDeck(sc scanner) (Deck, error) {
	var d Deck
	var holder, next, last sql.NullString
	var settings, look string
	if err := sc.Scan(&d.ID, &d.CampaignID, &d.Name, &d.Kind, &holder, &next, &last, &settings, &look,
		&d.Version, &d.SortOrder, &d.CreatedAt, &d.UpdatedAt); err != nil {
		return Deck{}, err
	}
	d.HolderEntityID = fromNull(holder)
	d.NextCardID, d.LastCardID = fromNull(next), fromNull(last)
	d.Settings = readSettings(d.Kind, settings)
	d.Look = readLook(look)
	d.Players = []Player{}
	return d, nil
}

func (s *sqlRepository) CountDecks(ctx context.Context, campaignID string) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM decks WHERE campaign_id = ?`, campaignID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count decks: %w", err)
	}
	return n, nil
}

func (s *sqlRepository) ListDecks(ctx context.Context, campaignID string) ([]Deck, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+deckColumns+` FROM decks WHERE campaign_id = ? ORDER BY sort_order, name, id`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list decks: %w", err)
	}
	defer rows.Close()
	var out []Deck
	index := map[string]int{}
	for rows.Next() {
		d, err := scanDeck(rows)
		if err != nil {
			return nil, fmt.Errorf("scan deck: %w", err)
		}
		index[d.ID] = len(out)
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list decks: %w", err)
	}
	prow, err := s.db.QueryContext(ctx,
		`SELECT p.deck_id, p.user_id, p.draws_left, p.created_at
		 FROM deck_players p JOIN decks d ON d.id = p.deck_id
		 WHERE d.campaign_id = ? ORDER BY p.created_at, p.user_id`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list deck players: %w", err)
	}
	defer prow.Close()
	for prow.Next() {
		deckID, p, err := scanPlayer(prow)
		if err != nil {
			return nil, err
		}
		if i, ok := index[deckID]; ok {
			out[i].Players = append(out[i].Players, p)
		}
	}
	return out, prow.Err()
}

func scanPlayer(sc scanner) (string, Player, error) {
	var deckID string
	var p Player
	var left sql.NullInt64
	if err := sc.Scan(&deckID, &p.UserID, &left, &p.GrantedAt); err != nil {
		return "", Player{}, fmt.Errorf("scan deck player: %w", err)
	}
	if left.Valid {
		n := int(left.Int64)
		p.DrawsLeft = &n
	}
	return deckID, p, nil
}

func (s *sqlRepository) GetDeck(ctx context.Context, campaignID, id string) (Deck, bool, error) {
	d, err := scanDeck(s.db.QueryRowContext(ctx,
		`SELECT `+deckColumns+` FROM decks WHERE id = ? AND campaign_id = ?`, id, campaignID))
	if errors.Is(err, sql.ErrNoRows) {
		return Deck{}, false, nil
	}
	if err != nil {
		return Deck{}, false, fmt.Errorf("get deck: %w", err)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT deck_id, user_id, draws_left, created_at FROM deck_players WHERE deck_id = ? ORDER BY created_at, user_id`, id)
	if err != nil {
		return Deck{}, false, fmt.Errorf("get deck players: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		_, p, err := scanPlayer(rows)
		if err != nil {
			return Deck{}, false, err
		}
		d.Players = append(d.Players, p)
	}
	return d, true, rows.Err()
}

func (s *sqlRepository) CreateDeck(ctx context.Context, d Deck) error {
	settings, err := json.Marshal(d.Settings)
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	look, err := json.Marshal(d.Look)
	if err != nil {
		return fmt.Errorf("encode look: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO decks (id, campaign_id, name, kind, holder_entity_id, settings, look, sort_order)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.CampaignID, d.Name, d.Kind, nullString(d.HolderEntityID), string(settings), string(look), d.SortOrder)
	if isDuplicate(err) {
		return errDuplicate
	}
	if err != nil {
		return fmt.Errorf("create deck: %w", err)
	}
	return nil
}

func (s *sqlRepository) UpdateDeck(ctx context.Context, d Deck, expected int, picks PickChange) (bool, error) {
	settings, err := json.Marshal(d.Settings)
	if err != nil {
		return false, fmt.Errorf("encode settings: %w", err)
	}
	look, err := json.Marshal(d.Look)
	if err != nil {
		return false, fmt.Errorf("encode look: %w", err)
	}
	sets := `name = ?, kind = ?, holder_entity_id = ?, settings = ?, look = ?, version = version + 1`
	args := []any{d.Name, d.Kind, nullString(d.HolderEntityID), string(settings), string(look)}
	if picks.SetNext {
		sets += `, next_card_id = ?`
		args = append(args, nullString(picks.Next))
	}
	if picks.SetLast {
		sets += `, last_card_id = ?`
		args = append(args, nullString(picks.Last))
	}
	args = append(args, d.ID, d.CampaignID, expected)
	res, err := s.db.ExecContext(ctx,
		`UPDATE decks SET `+sets+` WHERE id = ? AND campaign_id = ? AND version = ?`, args...)
	if isDuplicate(err) {
		return false, errDuplicate
	}
	if err != nil {
		return false, fmt.Errorf("update deck: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("update deck: %w", err)
	}
	return n == 1, nil
}

func (s *sqlRepository) DeleteDeck(ctx context.Context, campaignID, id string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM decks WHERE id = ? AND campaign_id = ?`, id, campaignID); err != nil {
		return fmt.Errorf("delete deck: %w", err)
	}
	return nil
}

func (s *sqlRepository) SetPlayers(ctx context.Context, campaignID, deckID string, players []Player, expected int) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("set players: %w", err)
	}
	defer rollback(tx)
	res, err := tx.ExecContext(ctx,
		`UPDATE decks SET version = version + 1 WHERE id = ? AND campaign_id = ? AND version = ?`, deckID, campaignID, expected)
	if err != nil {
		return false, fmt.Errorf("set players: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM deck_players WHERE deck_id = ?`, deckID); err != nil {
		return false, fmt.Errorf("clear deck players: %w", err)
	}
	for _, p := range players {
		var left any
		if p.DrawsLeft != nil {
			left = *p.DrawsLeft
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO deck_players (deck_id, user_id, draws_left, created_at) VALUES (?, ?, ?, ?)`,
			deckID, p.UserID, left, p.GrantedAt.UTC()); err != nil {
			return false, fmt.Errorf("add deck player: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("set players: %w", err)
	}
	return true, nil
}

// ---- cards ----------------------------------------------------------------

const cardColumns = `id, deck_id, campaign_id, name, kind_key, words, director_note, picture, corner, rare, copies, fate, when_json, when_open, ask_first, sort_order, created_at, updated_at`

func scanCard(sc scanner) (Card, error) {
	var c Card
	var when sql.NullString
	if err := sc.Scan(&c.ID, &c.DeckID, &c.CampaignID, &c.Name, &c.KindKey, &c.Words, &c.DirectorNote,
		&c.Picture, &c.Corner, &c.Rare, &c.Copies, &c.Fate, &when, &c.WhenOpen, &c.AskFirst, &c.SortOrder,
		&c.CreatedAt, &c.UpdatedAt); err != nil {
		return Card{}, err
	}
	c.When = fromNull(when)
	return c, nil
}

func (s *sqlRepository) ListCards(ctx context.Context, campaignID, deckID string) ([]Card, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+cardColumns+` FROM deck_cards WHERE deck_id = ? AND campaign_id = ? ORDER BY sort_order, name, id`,
		deckID, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list cards: %w", err)
	}
	defer rows.Close()
	var out []Card
	for rows.Next() {
		c, err := scanCard(rows)
		if err != nil {
			return nil, fmt.Errorf("scan card: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *sqlRepository) GetCard(ctx context.Context, campaignID, deckID, id string) (Card, bool, error) {
	c, err := scanCard(s.db.QueryRowContext(ctx,
		`SELECT `+cardColumns+` FROM deck_cards WHERE id = ? AND deck_id = ? AND campaign_id = ?`, id, deckID, campaignID))
	if errors.Is(err, sql.ErrNoRows) {
		return Card{}, false, nil
	}
	if err != nil {
		return Card{}, false, fmt.Errorf("get card: %w", err)
	}
	return c, true, nil
}

func (s *sqlRepository) CreateCard(ctx context.Context, c Card) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("create card: %w", err)
	}
	defer rollback(tx)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO deck_cards (id, deck_id, campaign_id, name, kind_key, words, director_note, picture, corner, rare, copies, fate, when_json, when_open, ask_first, sort_order)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.DeckID, c.CampaignID, c.Name, c.KindKey, c.Words, c.DirectorNote, c.Picture, c.Corner, c.Rare,
		c.Copies, c.Fate, nullString(c.When), c.WhenOpen, c.AskFirst, c.SortOrder)
	if isDuplicate(err) {
		return errDuplicate
	}
	if err != nil {
		return fmt.Errorf("create card: %w", err)
	}
	if err := addCopies(ctx, tx, c, c.Copies); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("create card: %w", err)
	}
	return nil
}

func addCopies(ctx context.Context, tx *sql.Tx, c Card, n int) error {
	for i := 0; i < n; i++ {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO deck_copies (id, deck_id, campaign_id, card_id, zone) VALUES (?, ?, ?, ?, 'pile')`,
			uuid.NewString(), c.DeckID, c.CampaignID, c.ID); err != nil {
			return fmt.Errorf("add copy: %w", err)
		}
	}
	return nil
}

func (s *sqlRepository) UpdateCard(ctx context.Context, c Card) (ResizeResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ResizeResult{}, fmt.Errorf("update card: %w", err)
	}
	defer rollback(tx)
	// The deck row is the lock every draw takes, so a resize never runs
	// beside a draw that is choosing among the same copies.
	if err := lockDeck(ctx, tx, c.CampaignID, c.DeckID); err != nil {
		return ResizeResult{}, err
	}
	_, err = tx.ExecContext(ctx,
		`UPDATE deck_cards SET name = ?, kind_key = ?, words = ?, director_note = ?, picture = ?, corner = ?, rare = ?,
		   copies = ?, fate = ?, when_json = ?, when_open = ?, ask_first = ?
		 WHERE id = ? AND deck_id = ? AND campaign_id = ?`,
		c.Name, c.KindKey, c.Words, c.DirectorNote, c.Picture, c.Corner, c.Rare, c.Copies, c.Fate,
		nullString(c.When), c.WhenOpen, c.AskFirst, c.ID, c.DeckID, c.CampaignID)
	if isDuplicate(err) {
		return ResizeResult{}, errDuplicate
	}
	if err != nil {
		return ResizeResult{}, fmt.Errorf("update card: %w", err)
	}
	rows, err := tx.QueryContext(ctx,
		`SELECT id, zone FROM deck_copies WHERE card_id = ? AND deck_id = ? ORDER BY id FOR UPDATE`, c.ID, c.DeckID)
	if err != nil {
		return ResizeResult{}, fmt.Errorf("read copies: %w", err)
	}
	byZone := map[string][]string{}
	have := 0
	for rows.Next() {
		var id, zone string
		if err := rows.Scan(&id, &zone); err != nil {
			rows.Close()
			return ResizeResult{}, fmt.Errorf("scan copy: %w", err)
		}
		byZone[zone] = append(byZone[zone], id)
		have++
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return ResizeResult{}, fmt.Errorf("read copies: %w", err)
	}
	rows.Close()

	var res ResizeResult
	switch {
	case c.Copies > have:
		if err := addCopies(ctx, tx, c, c.Copies-have); err != nil {
			return ResizeResult{}, err
		}
		res.Added = c.Copies - have
	case c.Copies < have:
		remove := have - c.Copies
		var doomed []string
		for _, zone := range []string{ZonePile, ZoneUsed, ZoneGone} {
			for _, id := range byZone[zone] {
				if len(doomed) < remove {
					doomed = append(doomed, id)
				}
			}
		}
		if len(doomed) < remove {
			held := len(byZone[ZoneShown]) + len(byZone[ZoneHand]) + len(byZone[ZonePlay])
			return ResizeResult{}, &heldError{N: held}
		}
		for _, id := range doomed {
			if _, err := tx.ExecContext(ctx, `DELETE FROM deck_copies WHERE id = ?`, id); err != nil {
				return ResizeResult{}, fmt.Errorf("remove copy: %w", err)
			}
		}
		res.Removed = remove
	}
	if err := tx.Commit(); err != nil {
		return ResizeResult{}, fmt.Errorf("update card: %w", err)
	}
	return res, nil
}

func (s *sqlRepository) DeleteCard(ctx context.Context, campaignID, deckID, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("delete card: %w", err)
	}
	defer rollback(tx)
	if err := lockDeck(ctx, tx, campaignID, deckID); err != nil {
		if errors.Is(err, errLostClaim) {
			return nil
		}
		return err
	}
	// The deck's next and last card carry no foreign key, so they are cleared
	// here, in the transaction that removes the card they name.
	if _, err := tx.ExecContext(ctx,
		`UPDATE decks SET next_card_id = IF(next_card_id = ?, NULL, next_card_id),
		   last_card_id = IF(last_card_id = ?, NULL, last_card_id)
		 WHERE id = ? AND campaign_id = ?`, id, id, deckID, campaignID); err != nil {
		return fmt.Errorf("clear draw nudges: %w", err)
	}
	// The copies go with the card through the foreign key.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM deck_cards WHERE id = ? AND deck_id = ? AND campaign_id = ?`, id, deckID, campaignID); err != nil {
		return fmt.Errorf("delete card: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("delete card: %w", err)
	}
	return nil
}

// ---- copies ---------------------------------------------------------------

const copyColumns = `id, deck_id, campaign_id, card_id, zone, holder_entity_id, face_up, is_open, asked_at, dealt_by, dealt_at, turned_at, moved_at`

func scanCopy(sc scanner) (Copy, error) {
	var c Copy
	var holder, dealtBy sql.NullString
	var asked, dealt, turned sql.NullTime
	if err := sc.Scan(&c.ID, &c.DeckID, &c.CampaignID, &c.CardID, &c.Zone, &holder, &c.FaceUp, &c.Open,
		&asked, &dealtBy, &dealt, &turned, &c.MovedAt); err != nil {
		return Copy{}, err
	}
	c.HolderEntityID = fromNull(holder)
	c.DealtBy = fromNull(dealtBy)
	c.AskedAt, c.DealtAt, c.TurnedAt = timePtr(asked), timePtr(dealt), timePtr(turned)
	return c, nil
}

func (s *sqlRepository) ListCopies(ctx context.Context, campaignID, deckID string) ([]Copy, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+copyColumns+` FROM deck_copies WHERE deck_id = ? AND campaign_id = ? ORDER BY moved_at, id`, deckID, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list copies: %w", err)
	}
	defer rows.Close()
	var out []Copy
	for rows.Next() {
		c, err := scanCopy(rows)
		if err != nil {
			return nil, fmt.Errorf("scan copy: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *sqlRepository) GetCopy(ctx context.Context, campaignID, deckID, id string) (Copy, bool, error) {
	c, err := scanCopy(s.db.QueryRowContext(ctx,
		`SELECT `+copyColumns+` FROM deck_copies WHERE id = ? AND deck_id = ? AND campaign_id = ?`, id, deckID, campaignID))
	if errors.Is(err, sql.ErrNoRows) {
		return Copy{}, false, nil
	}
	if err != nil {
		return Copy{}, false, fmt.Errorf("get copy: %w", err)
	}
	return c, true, nil
}

func (s *sqlRepository) ListHeldBy(ctx context.Context, campaignID string, holderIDs []string) ([]Copy, error) {
	if len(holderIDs) == 0 {
		return nil, nil
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(holderIDs)), ",")
	args := []any{campaignID}
	for _, id := range holderIDs {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+copyColumns+` FROM deck_copies
		 WHERE campaign_id = ? AND zone IN ('hand','play') AND holder_entity_id IN (`+marks+`)
		 ORDER BY moved_at, id`, args...)
	if err != nil {
		return nil, fmt.Errorf("list held copies: %w", err)
	}
	defer rows.Close()
	var out []Copy
	for rows.Next() {
		c, err := scanCopy(rows)
		if err != nil {
			return nil, fmt.Errorf("scan copy: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *sqlRepository) ZoneCounts(ctx context.Context, campaignID string) (map[string]ZoneCounts, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT deck_id, zone, COUNT(*) FROM deck_copies WHERE campaign_id = ? GROUP BY deck_id, zone`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("count copies: %w", err)
	}
	defer rows.Close()
	out := map[string]ZoneCounts{}
	for rows.Next() {
		var deckID, zone string
		var n int
		if err := rows.Scan(&deckID, &zone, &n); err != nil {
			return nil, fmt.Errorf("scan counts: %w", err)
		}
		c := out[deckID]
		c.add(zone, n)
		out[deckID] = c
	}
	return out, rows.Err()
}

func (z *ZoneCounts) add(zone string, n int) {
	switch zone {
	case ZonePile:
		z.Pile += n
	case ZoneShown:
		z.Shown += n
	case ZoneHand:
		z.Hand += n
	case ZonePlay:
		z.Play += n
	case ZoneUsed:
		z.Used += n
	case ZoneGone:
		z.Gone += n
	}
}

func (s *sqlRepository) ListTable(ctx context.Context, campaignID string) ([]TableRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT c.id, d.id, d.name, d.kind, d.look, k.id, k.name, k.kind_key, k.words, k.picture, k.corner, k.rare
		 FROM deck_copies c
		 JOIN decks d ON d.id = c.deck_id
		 JOIN deck_cards k ON k.id = c.card_id
		 WHERE c.campaign_id = ? AND c.zone = 'play' AND c.holder_entity_id IS NULL AND c.face_up = 1
		 ORDER BY d.sort_order, d.name, c.moved_at, c.id`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list table: %w", err)
	}
	defer rows.Close()
	var out []TableRow
	for rows.Next() {
		var r TableRow
		var look string
		if err := rows.Scan(&r.CopyID, &r.DeckID, &r.DeckName, &r.DeckKind, &look,
			&r.Face.CardID, &r.Face.Name, &r.Face.KindKey, &r.Face.Words, &r.Face.Picture, &r.Face.Corner, &r.Face.Rare); err != nil {
			return nil, fmt.Errorf("scan table row: %w", err)
		}
		r.Look = readLook(look)
		out = append(out, r)
	}
	return out, rows.Err()
}

// ---- moves ----------------------------------------------------------------

// lockDeck takes the deck row's lock, so every draw, shuffle, gather and
// resize on one deck runs one after another.
func lockDeck(ctx context.Context, tx *sql.Tx, campaignID, deckID string) error {
	var id string
	err := tx.QueryRowContext(ctx, `SELECT id FROM decks WHERE id = ? AND campaign_id = ? FOR UPDATE`, deckID, campaignID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return errLostClaim
	}
	if err != nil {
		return fmt.Errorf("lock deck: %w", err)
	}
	return nil
}

// resetFields puts a copy's per-hand state back to blank, for a copy that
// returns to the pile. The copy also gets a fresh id in the same statement:
// players see copy ids beside faces, and an id seen on a face must never come
// back attached to a hidden card.
const resetFields = `id = UUID(), zone = 'pile', holder_entity_id = NULL, face_up = 0, is_open = 0, asked_at = NULL, dealt_by = NULL, dealt_at = NULL, turned_at = NULL, moved_at = NOW()`

// resetFieldsAs is resetFields with an id chosen by the caller, for a copy
// the same transaction goes on to refer to.
const resetFieldsAs = `id = ?, zone = 'pile', holder_entity_id = NULL, face_up = 0, is_open = 0, asked_at = NULL, dealt_by = NULL, dealt_at = NULL, turned_at = NULL, moved_at = NOW()`

func insertLog(ctx context.Context, tx *sql.Tx, campaignID string, l LogLine) error {
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO deck_log (deck_id, campaign_id, card_id, card_name, copy_id, action, actor_user_id, holder_entity_id, seen)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		l.DeckID, campaignID, nullString(l.CardID), l.CardName, nullString(l.CopyID), l.Action, l.ActorUserID,
		nullString(l.HolderEntityID), l.Seen); err != nil {
		return fmt.Errorf("write log: %w", err)
	}
	var cut int64
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM deck_log WHERE deck_id = ? ORDER BY id DESC LIMIT 1 OFFSET ?`, l.DeckID, LogKept).Scan(&cut)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("trim log: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM deck_log WHERE deck_id = ? AND id <= ?`, l.DeckID, cut); err != nil {
		return fmt.Errorf("trim log: %w", err)
	}
	return nil
}

func (s *sqlRepository) Move(ctx context.Context, m Move) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("move copy: %w", err)
	}
	defer rollback(tx)

	// An open mark only makes sense on a face-up card still in a hand, so a
	// move that leaves either clears it.
	open := m.To.Open && m.To.FaceUp && m.To.Zone == ZoneHand
	sets := []string{"zone = ?", "holder_entity_id = ?", "face_up = ?", "is_open = ?", "moved_at = NOW()"}
	args := []any{m.To.Zone, nullString(m.To.HolderEntityID), m.To.FaceUp, open}
	switch m.Asked {
	case AskedMark:
		sets = append(sets, "asked_at = NOW()")
	case AskedClear:
		sets = append(sets, "asked_at = NULL")
	}
	switch {
	case m.DealtBy != "":
		sets = append(sets, "dealt_by = ?", "dealt_at = NOW()")
		args = append(args, m.DealtBy)
	case m.To.Zone == ZonePile || m.To.Zone == ZoneShown:
		sets = append(sets, "dealt_by = NULL", "dealt_at = NULL", "turned_at = NULL")
	}
	if m.Turned {
		sets = append(sets, "turned_at = NOW()")
	}
	// The card is read first because a copy going to the pile gets a new id.
	var cardID, cardName string
	err = tx.QueryRowContext(ctx,
		`SELECT c.card_id, k.name FROM deck_copies c JOIN deck_cards k ON k.id = c.card_id
		 WHERE c.id = ? AND c.deck_id = ? AND c.campaign_id = ? FOR UPDATE`,
		m.CopyID, m.DeckID, m.CampaignID).Scan(&cardID, &cardName)
	if errors.Is(err, sql.ErrNoRows) {
		return errLostClaim
	}
	if err != nil {
		return fmt.Errorf("read moved copy: %w", err)
	}
	if m.To.Zone == ZonePile {
		sets = append(sets, "id = ?")
		args = append(args, uuid.NewString())
	}
	where := `id = ? AND deck_id = ? AND campaign_id = ? AND zone = ? AND holder_entity_id <=> ? AND face_up = ? AND is_open = ?`
	args = append(args, m.CopyID, m.DeckID, m.CampaignID, m.From.Zone, nullString(m.From.HolderEntityID), m.From.FaceUp, m.From.Open)
	if m.ExpectAsked {
		where += ` AND asked_at IS NOT NULL`
	}
	res, err := tx.ExecContext(ctx, `UPDATE deck_copies SET `+strings.Join(sets, ", ")+` WHERE `+where, args...)
	if err != nil {
		return fmt.Errorf("move copy: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("move copy: %w", err)
	}
	if n != 1 {
		return errLostClaim
	}
	m.Log.DeckID, m.Log.CardID, m.Log.CardName, m.Log.CopyID = m.DeckID, cardID, cardName, m.CopyID
	if err := insertLog(ctx, tx, m.CampaignID, m.Log); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("move copy: %w", err)
	}
	return nil
}

type pileCopy struct{ id, cardID, cardName string }

func pileCopies(ctx context.Context, tx *sql.Tx, p DrawPlan) ([]pileCopy, error) {
	q := `SELECT c.id, c.card_id, k.name FROM deck_copies c JOIN deck_cards k ON k.id = c.card_id
	      WHERE c.deck_id = ? AND c.campaign_id = ? AND c.zone = 'pile'`
	args := []any{p.DeckID, p.CampaignID}
	if p.CardID != "" {
		q += ` AND c.card_id = ?`
		args = append(args, p.CardID)
	}
	rows, err := tx.QueryContext(ctx, q+` ORDER BY c.id FOR UPDATE`, args...)
	if err != nil {
		return nil, fmt.Errorf("read pile: %w", err)
	}
	defer rows.Close()
	var out []pileCopy
	for rows.Next() {
		var c pileCopy
		if err := rows.Scan(&c.id, &c.cardID, &c.cardName); err != nil {
			return nil, fmt.Errorf("scan pile: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// onlyCard keeps the candidates that are copies of one card.
func onlyCard(cands []pileCopy, cardID string) []pileCopy {
	var out []pileCopy
	for _, c := range cands {
		if c.cardID == cardID {
			out = append(out, c)
		}
	}
	return out
}

// withoutCard drops the candidates that are copies of one card.
func withoutCard(cands []pileCopy, cardID string) []pileCopy {
	var out []pileCopy
	for _, c := range cands {
		if c.cardID != cardID {
			out = append(out, c)
		}
	}
	return out
}

// deckPicks reads the deck's next and last card. It is called with the deck
// row already locked, so no draw sees a value another draw is about to clear.
func deckPicks(ctx context.Context, tx *sql.Tx, deckID string) (next, last string, err error) {
	var n, l sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT next_card_id, last_card_id FROM decks WHERE id = ?`, deckID).Scan(&n, &l); err != nil {
		return "", "", fmt.Errorf("read draw nudges: %w", err)
	}
	return fromNull(n), fromNull(l), nil
}

func (s *sqlRepository) Draw(ctx context.Context, p DrawPlan) ([]Copy, error) {
	count := p.Count
	if count < 1 {
		count = 1
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("draw: %w", err)
	}
	defer rollback(tx)
	if err := lockDeck(ctx, tx, p.CampaignID, p.DeckID); err != nil {
		return nil, err
	}

	if p.SpendUserID != "" {
		var left sql.NullInt64
		err := tx.QueryRowContext(ctx,
			`SELECT draws_left FROM deck_players WHERE deck_id = ? AND user_id = ? FOR UPDATE`, p.DeckID, p.SpendUserID).Scan(&left)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errNoDraws
		}
		if err != nil {
			return nil, fmt.Errorf("read draws: %w", err)
		}
		if left.Valid {
			res, err := tx.ExecContext(ctx,
				`UPDATE deck_players SET draws_left = draws_left - 1 WHERE deck_id = ? AND user_id = ? AND draws_left > 0`,
				p.DeckID, p.SpendUserID)
			if err != nil {
				return nil, fmt.Errorf("spend draw: %w", err)
			}
			if n, err := res.RowsAffected(); err != nil || n != 1 {
				if err != nil {
					return nil, fmt.Errorf("spend draw: %w", err)
				}
				return nil, errNoDraws
			}
		}
	}

	exclude := ""
	if p.PutBackCopyID != "" {
		var cardID, cardName string
		err := tx.QueryRowContext(ctx,
			`SELECT c.card_id, k.name FROM deck_copies c JOIN deck_cards k ON k.id = c.card_id
			 WHERE c.id = ? AND c.deck_id = ? AND c.campaign_id = ? AND c.zone = 'shown' FOR UPDATE`,
			p.PutBackCopyID, p.DeckID, p.CampaignID).Scan(&cardID, &cardName)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errLostClaim
		}
		if err != nil {
			return nil, fmt.Errorf("read shown: %w", err)
		}
		fresh := uuid.NewString()
		if _, err := tx.ExecContext(ctx, `UPDATE deck_copies SET `+resetFieldsAs+` WHERE id = ?`, fresh, p.PutBackCopyID); err != nil {
			return nil, fmt.Errorf("put back shown: %w", err)
		}
		exclude = fresh
		if err := insertLog(ctx, tx, p.CampaignID, LogLine{DeckID: p.DeckID, CardID: cardID, CardName: cardName,
			CopyID: p.PutBackCopyID, Action: ActPutBack, ActorUserID: p.Log.ActorUserID}); err != nil {
			return nil, err
		}
	}
	if p.To.Zone == ZoneShown {
		var n int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM deck_copies WHERE deck_id = ? AND campaign_id = ? AND zone = 'shown'`, p.DeckID, p.CampaignID).Scan(&n); err != nil {
			return nil, fmt.Errorf("read shown: %w", err)
		}
		if n+count > MaxShown {
			return nil, errShownFull
		}
	}

	// A named deal ignores the Director's next and last card: naming a card is
	// already the stronger instruction.
	next, last := "", ""
	if p.CardID == "" {
		if next, last, err = deckPicks(ctx, tx, p.DeckID); err != nil {
			return nil, err
		}
	}
	nextUsed, lastUsed := false, false

	candidates := func() ([]pileCopy, error) {
		all, err := pileCopies(ctx, tx, p)
		if err != nil {
			return nil, err
		}
		if exclude == "" {
			return all, nil
		}
		var rest []pileCopy
		for _, c := range all {
			if c.id != exclude {
				rest = append(rest, c)
			}
		}
		if len(rest) == 0 {
			// Nothing else exists, so the same copy is the only honest draw.
			return all, nil
		}
		return rest, nil
	}

	var out []Copy
	for i := 0; i < count; i++ {
		cands, err := candidates()
		if err != nil {
			return nil, err
		}
		if p.Reshuffle && p.CardID == "" && (len(cands) == 0 || (exclude != "" && len(cands) == 1 && cands[0].id == exclude)) {
			res, err := tx.ExecContext(ctx,
				`UPDATE deck_copies SET `+resetFields+` WHERE deck_id = ? AND campaign_id = ? AND zone = 'used'`, p.DeckID, p.CampaignID)
			if err != nil {
				return nil, fmt.Errorf("reshuffle: %w", err)
			}
			if n, _ := res.RowsAffected(); n > 0 {
				if cands, err = candidates(); err != nil {
					return nil, err
				}
			}
		}
		if len(cands) == 0 {
			return nil, errEmptyPile
		}
		// The next card is the first card of a draw, and only if a copy of it is
		// in the pile; the last card is left out while anything else is.
		pool, tookNext := cands, false
		if i == 0 && next != "" {
			if sub := onlyCard(cands, next); len(sub) > 0 {
				pool, tookNext = sub, true
			}
		}
		if !tookNext && last != "" {
			if rest := withoutCard(cands, last); len(rest) > 0 {
				pool = rest
			}
		}
		pick := pool[p.Pick(len(pool))]
		if tookNext {
			nextUsed, next = true, ""
		}
		if last != "" && pick.cardID == last {
			lastUsed, last = true, ""
		}

		sets := `zone = ?, holder_entity_id = ?, face_up = ?, is_open = 0, asked_at = NULL, turned_at = NULL, moved_at = NOW()`
		args := []any{p.To.Zone, nullString(p.To.HolderEntityID), p.To.FaceUp}
		if p.DealtBy != "" {
			sets += `, dealt_by = ?, dealt_at = NOW()`
			args = append(args, p.DealtBy)
		}
		args = append(args, pick.id)
		res, err := tx.ExecContext(ctx, `UPDATE deck_copies SET `+sets+` WHERE id = ? AND zone = 'pile'`, args...)
		if err != nil {
			return nil, fmt.Errorf("claim copy: %w", err)
		}
		if n, err := res.RowsAffected(); err != nil || n != 1 {
			if err != nil {
				return nil, fmt.Errorf("claim copy: %w", err)
			}
			return nil, errLostClaim
		}
		line := p.Log
		line.DeckID, line.CardID, line.CardName, line.CopyID = p.DeckID, pick.cardID, pick.cardName, pick.id
		if err := insertLog(ctx, tx, p.CampaignID, line); err != nil {
			return nil, err
		}
		got, err := scanCopy(tx.QueryRowContext(ctx, `SELECT `+copyColumns+` FROM deck_copies WHERE id = ?`, pick.id))
		if err != nil {
			return nil, fmt.Errorf("read drawn copy: %w", err)
		}
		out = append(out, got)
	}
	if nextUsed {
		if _, err := tx.ExecContext(ctx, `UPDATE decks SET next_card_id = NULL WHERE id = ?`, p.DeckID); err != nil {
			return nil, fmt.Errorf("clear next card: %w", err)
		}
	}
	if lastUsed {
		if _, err := tx.ExecContext(ctx, `UPDATE decks SET last_card_id = NULL WHERE id = ?`, p.DeckID); err != nil {
			return nil, fmt.Errorf("clear last card: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("draw: %w", err)
	}
	return out, nil
}

func (s *sqlRepository) Keep(ctx context.Context, k KeepPlan) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("keep card: %w", err)
	}
	defer rollback(tx)
	// Both decks are locked in id order, so two keeps in opposite directions
	// cannot wait on each other.
	ids := []string{k.DeckID, k.TargetDeckID}
	if ids[1] < ids[0] {
		ids[0], ids[1] = ids[1], ids[0]
	}
	for _, id := range ids {
		if err := lockDeck(ctx, tx, k.CampaignID, id); err != nil {
			return err
		}
	}
	var src Card
	err = tx.QueryRowContext(ctx,
		`SELECT k.id, k.name, k.kind_key, k.words, k.director_note, k.picture, k.corner, k.rare, k.fate, k.ask_first
		 FROM deck_copies c JOIN deck_cards k ON k.id = c.card_id
		 WHERE c.id = ? AND c.deck_id = ? AND c.campaign_id = ? FOR UPDATE`,
		k.CopyID, k.DeckID, k.CampaignID).Scan(&src.ID, &src.Name, &src.KindKey, &src.Words, &src.DirectorNote,
		&src.Picture, &src.Corner, &src.Rare, &src.Fate, &src.AskFirst)
	if errors.Is(err, sql.ErrNoRows) {
		return errLostClaim
	}
	if err != nil {
		return fmt.Errorf("read kept card: %w", err)
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE deck_copies SET zone = 'gone', holder_entity_id = NULL, face_up = 1, is_open = 0, asked_at = NULL, moved_at = NOW()
		 WHERE id = ? AND deck_id = ? AND campaign_id = ? AND zone = ? AND holder_entity_id <=> ? AND face_up = ? AND is_open = ?`,
		k.CopyID, k.DeckID, k.CampaignID, k.From.Zone, nullString(k.From.HolderEntityID), k.From.FaceUp, k.From.Open)
	if err != nil {
		return fmt.Errorf("keep card: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		if err != nil {
			return fmt.Errorf("keep card: %w", err)
		}
		return errLostClaim
	}

	var cards, copies int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(copies), 0) FROM deck_cards WHERE deck_id = ? AND campaign_id = ?`,
		k.TargetDeckID, k.CampaignID).Scan(&cards, &copies); err != nil {
		return fmt.Errorf("count target cards: %w", err)
	}
	if copies+1 > MaxCopiesPerDeck {
		return errTargetCopies
	}
	var targetID string
	var have int
	err = tx.QueryRowContext(ctx,
		`SELECT id, copies FROM deck_cards WHERE deck_id = ? AND campaign_id = ? AND name = ? FOR UPDATE`,
		k.TargetDeckID, k.CampaignID, src.Name).Scan(&targetID, &have)
	switch {
	case err == nil:
		if have+1 > MaxCopiesPerCard {
			return errCardCopies
		}
		if _, err := tx.ExecContext(ctx, `UPDATE deck_cards SET copies = copies + 1 WHERE id = ?`, targetID); err != nil {
			return fmt.Errorf("add kept copy: %w", err)
		}
	case errors.Is(err, sql.ErrNoRows):
		if cards >= MaxCardsPerDeck {
			return errTargetFull
		}
		kind := ""
		if k.TargetKinds[src.KindKey] {
			kind = src.KindKey
		}
		targetID = uuid.NewString()
		// The condition (when_json, when_open) is left at its default: it was
		// written against the source deck's cards and means nothing in another.
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO deck_cards (id, deck_id, campaign_id, name, kind_key, words, director_note, picture, corner, rare, copies, fate, ask_first, sort_order)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)`,
			targetID, k.TargetDeckID, k.CampaignID, src.Name, kind, src.Words, src.DirectorNote, src.Picture, src.Corner,
			src.Rare, src.Fate, src.AskFirst, cards); err != nil {
			return fmt.Errorf("add kept card: %w", err)
		}
	default:
		return fmt.Errorf("find kept card: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO deck_copies (id, deck_id, campaign_id, card_id, zone) VALUES (?, ?, ?, ?, 'pile')`,
		uuid.NewString(), k.TargetDeckID, k.CampaignID, targetID); err != nil {
		return fmt.Errorf("add kept copy: %w", err)
	}
	k.Log.DeckID, k.Log.CardID, k.Log.CardName, k.Log.CopyID = k.DeckID, src.ID, src.Name, k.CopyID
	if err := insertLog(ctx, tx, k.CampaignID, k.Log); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("keep card: %w", err)
	}
	return nil
}

func (s *sqlRepository) bulk(ctx context.Context, campaignID, deckID, where string, log LogLine) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("move copies: %w", err)
	}
	defer rollback(tx)
	if err := lockDeck(ctx, tx, campaignID, deckID); err != nil {
		return 0, err
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE deck_copies SET `+resetFields+` WHERE deck_id = ? AND campaign_id = ? AND `+where, deckID, campaignID)
	if err != nil {
		return 0, fmt.Errorf("move copies: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("move copies: %w", err)
	}
	log.DeckID = deckID
	if err := insertLog(ctx, tx, campaignID, log); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("move copies: %w", err)
	}
	return int(n), nil
}

func (s *sqlRepository) Shuffle(ctx context.Context, campaignID, deckID string, log LogLine) (int, error) {
	return s.bulk(ctx, campaignID, deckID, `zone = 'used'`, log)
}

func (s *sqlRepository) Gather(ctx context.Context, campaignID, deckID string, log LogLine) (int, error) {
	return s.bulk(ctx, campaignID, deckID, `zone <> 'pile'`, log)
}

func (s *sqlRepository) ListLog(ctx context.Context, campaignID, deckID string, limit int) ([]LogLine, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, deck_id, card_id, card_name, copy_id, action, actor_user_id, holder_entity_id, seen, created_at
		 FROM deck_log WHERE deck_id = ? AND campaign_id = ? ORDER BY id DESC LIMIT ?`, deckID, campaignID, limit)
	if err != nil {
		return nil, fmt.Errorf("list log: %w", err)
	}
	defer rows.Close()
	var out []LogLine
	for rows.Next() {
		var l LogLine
		var card, cp, holder sql.NullString
		if err := rows.Scan(&l.ID, &l.DeckID, &card, &l.CardName, &cp, &l.Action, &l.ActorUserID, &holder, &l.Seen, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan log: %w", err)
		}
		l.CardID, l.CopyID, l.HolderEntityID = fromNull(card), fromNull(cp), fromNull(holder)
		out = append(out, l)
	}
	return out, rows.Err()
}
