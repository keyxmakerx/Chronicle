package decks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// States of a moment. A moment is open while waiting or invited.
const (
	MomentWaiting   = "waiting"
	MomentInvited   = "invited"
	MomentDealt     = "dealt"
	MomentDrawn     = "drawn"
	MomentDismissed = "dismissed"
	MomentLapsed    = "lapsed"
)

// closedBySystem is who closes a moment that lapses.
const closedBySystem = "system"

// momentsKept is how many closed moments a deck keeps.
const momentsKept = 50

// quietWindow is how soon after one moment closed the next one is quiet, so a
// number that wobbles does not slide in again and again.
const quietWindow = 5 * time.Minute

// Moment is one waiting notice for one deck: the card the deck picked blind
// among the cards that fit, remembered and not dealt. Nothing leaves the pile
// until a Director deals.
type Moment struct {
	ID         string
	CampaignID string
	DeckID     string
	CardID     string
	// SubjectID is the hero the card's "any" tests held for; "" when it has none.
	SubjectID string
	// FitCount is how many cards with tests fit.
	FitCount int
	Reading  Reading
	State    string
	// Quiet is a moment that follows another too closely to be announced loudly.
	Quiet         bool
	InvitedUserID string
	CreatedAt     time.Time
	ClosedAt      *time.Time
	ClosedBy      string
}

// Open is a moment a Director can still answer.
func (m Moment) Open() bool { return m.State == MomentWaiting || m.State == MomentInvited }

// MomentClaim is a state change of a moment made inside another transaction,
// so an answer and the move it causes are one step.
type MomentClaim struct {
	ID string
	// From lists the states the moment may be in.
	From []string
	To   string
	By   string
	// InvitedUserID, when set, must be the invited player.
	InvitedUserID string
}

// MomentRepository is the persistence boundary for moments and for what is
// remembered about whether a card's tests hold.
type MomentRepository interface {
	// WatchedDecks lists the decks of a campaign that need a look: those with a
	// card that has tests and a notice that is not quiet, and those with an open
	// moment. It is one indexed question and nothing else, because it runs after
	// every page save in every campaign.
	WatchedDecks(ctx context.Context, campaignID string) ([]string, error)
	// PileCounts is how many copies of each card are in the pile.
	PileCounts(ctx context.Context, campaignID, deckID string) (map[string]int, error)
	// SetFits starts fits_since on the cards in start (where it is not set) and
	// clears it on the cards in stop. started is how many cards it really
	// started: a look that read the cards before another look marked them
	// starts none, so it raises no second moment.
	SetFits(ctx context.Context, campaignID, deckID string, start, stop []string) (started int, err error)
	// ClearFits clears fits_since on every card of the deck.
	ClearFits(ctx context.Context, campaignID, deckID string) error
	// SetSaid sets or clears "the Director says so"; ok is false for a card that
	// is not in the deck.
	SetSaid(ctx context.Context, campaignID, deckID, cardID string, said bool) (ok bool, err error)

	// Open finds the deck's open moment.
	Open(ctx context.Context, campaignID, deckID string) (Moment, bool, error)
	// ListOpen lists the campaign's open moments, oldest first.
	ListOpen(ctx context.Context, campaignID string) ([]Moment, error)
	Get(ctx context.Context, campaignID, id string) (Moment, bool, error)
	// InvitedFor finds the moment that invites this player to draw from the deck.
	InvitedFor(ctx context.Context, campaignID, deckID, userID string) (Moment, bool, error)
	// Create stores a moment; created is false when the deck already has an open
	// one (the unique key), which is not an error.
	Create(ctx context.Context, m Moment) (created bool, err error)
	// Repick changes the card of a waiting moment, only while it still waits on
	// fromCard.
	Repick(ctx context.Context, campaignID, id, fromCard string, to Moment) (bool, error)
	// Refresh updates what an open moment says about its card without changing
	// the card, only while it still holds forCard: a look that began before the
	// moment was picked again must not write the old card's hero onto the new one.
	Refresh(ctx context.Context, campaignID, id, forCard string, to Moment) (bool, error)
	// Close moves an open moment to a closed state, only from one of from.
	Close(ctx context.Context, campaignID, id string, from []string, to, by string) (bool, error)
	// Invite moves a waiting moment to invited.
	Invite(ctx context.Context, campaignID, id, userID string) (bool, error)
	// LastClosed is when the deck's most recent moment closed.
	LastClosed(ctx context.Context, campaignID, deckID string) (time.Time, bool, error)
}

type sqlMoments struct{ db *sql.DB }

// NewMomentRepository creates the MariaDB-backed moment store.
func NewMomentRepository(db *sql.DB) MomentRepository { return &sqlMoments{db: db} }

// quietNotice matches a deck whose stored settings say notice "quiet". The
// settings are the re-marshal of the typed struct, so the text is exact.
const quietNotice = `%"notice":"quiet"%`

func (s *sqlMoments) WatchedDecks(ctx context.Context, campaignID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT k.deck_id FROM deck_cards k JOIN decks d ON d.id = k.deck_id
		 WHERE k.campaign_id = ? AND k.has_when = 1 AND d.settings NOT LIKE ?
		 UNION
		 SELECT m.deck_id FROM deck_moments m WHERE m.campaign_id = ? AND m.open_deck_id IS NOT NULL`,
		campaignID, quietNotice, campaignID)
	if err != nil {
		return nil, fmt.Errorf("watched decks: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan watched deck: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *sqlMoments) PileCounts(ctx context.Context, campaignID, deckID string) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT card_id, COUNT(*) FROM deck_copies WHERE deck_id = ? AND campaign_id = ? AND zone = 'pile' GROUP BY card_id`,
		deckID, campaignID)
	if err != nil {
		return nil, fmt.Errorf("pile counts: %w", err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, fmt.Errorf("scan pile count: %w", err)
		}
		out[id] = n
	}
	return out, rows.Err()
}

// clipReading keeps a reading inside its column, by characters, so a very long
// page name cannot fail the write.
func clipReading(s string) string {
	const max = 500
	if r := []rune(s); len(r) > max {
		return string(r[:max])
	}
	return s
}

func inList(ids []string) (string, []any) {
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	return strings.TrimSuffix(strings.Repeat("?,", len(ids)), ","), args
}

func (s *sqlMoments) SetFits(ctx context.Context, campaignID, deckID string, start, stop []string) (int, error) {
	started := 0
	if len(start) > 0 {
		marks, args := inList(start)
		args = append([]any{deckID, campaignID}, args...)
		res, err := s.db.ExecContext(ctx,
			`UPDATE deck_cards SET fits_since = NOW() WHERE deck_id = ? AND campaign_id = ? AND fits_since IS NULL AND id IN (`+marks+`)`, args...)
		if err != nil {
			return 0, fmt.Errorf("start fits: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("start fits: %w", err)
		}
		started = int(n)
	}
	if len(stop) > 0 {
		marks, args := inList(stop)
		args = append([]any{deckID, campaignID}, args...)
		if _, err := s.db.ExecContext(ctx,
			`UPDATE deck_cards SET fits_since = NULL WHERE deck_id = ? AND campaign_id = ? AND fits_since IS NOT NULL AND id IN (`+marks+`)`, args...); err != nil {
			return started, fmt.Errorf("stop fits: %w", err)
		}
	}
	return started, nil
}

func (s *sqlMoments) ClearFits(ctx context.Context, campaignID, deckID string) error {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE deck_cards SET fits_since = NULL WHERE deck_id = ? AND campaign_id = ? AND fits_since IS NOT NULL`, deckID, campaignID); err != nil {
		return fmt.Errorf("clear fits: %w", err)
	}
	return nil
}

func (s *sqlMoments) SetSaid(ctx context.Context, campaignID, deckID, cardID string, said bool) (bool, error) {
	// Saying it again keeps the first time.
	res, err := s.db.ExecContext(ctx,
		`UPDATE deck_cards SET said_at = IF(?, COALESCE(said_at, NOW()), NULL) WHERE id = ? AND deck_id = ? AND campaign_id = ?`,
		said, cardID, deckID, campaignID)
	if err != nil {
		return false, fmt.Errorf("set said: %w", err)
	}
	// A repeat changes no row, so the card is looked up separately.
	if n, _ := res.RowsAffected(); n > 0 {
		return true, nil
	}
	var id string
	err = s.db.QueryRowContext(ctx, `SELECT id FROM deck_cards WHERE id = ? AND deck_id = ? AND campaign_id = ?`, cardID, deckID, campaignID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("set said: %w", err)
	}
	return true, nil
}

const momentColumns = `id, campaign_id, deck_id, card_id, subject_entity_id, fit_count, reading_text, reading_source, read_at,
	state, quiet, invited_user_id, created_at, closed_at, closed_by`

func scanMoment(sc scanner) (Moment, error) {
	var m Moment
	var subject, invited, closedBy sql.NullString
	var readAt, closedAt sql.NullTime
	if err := sc.Scan(&m.ID, &m.CampaignID, &m.DeckID, &m.CardID, &subject, &m.FitCount, &m.Reading.Text, &m.Reading.Source,
		&readAt, &m.State, &m.Quiet, &invited, &m.CreatedAt, &closedAt, &closedBy); err != nil {
		return Moment{}, err
	}
	m.SubjectID, m.InvitedUserID, m.ClosedBy = fromNull(subject), fromNull(invited), fromNull(closedBy)
	if readAt.Valid {
		m.Reading.At = readAt.Time
	}
	m.ClosedAt = timePtr(closedAt)
	return m, nil
}

func (s *sqlMoments) one(ctx context.Context, where string, args ...any) (Moment, bool, error) {
	m, err := scanMoment(s.db.QueryRowContext(ctx, `SELECT `+momentColumns+` FROM deck_moments WHERE `+where, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return Moment{}, false, nil
	}
	if err != nil {
		return Moment{}, false, fmt.Errorf("read moment: %w", err)
	}
	return m, true, nil
}

func (s *sqlMoments) Open(ctx context.Context, campaignID, deckID string) (Moment, bool, error) {
	return s.one(ctx, `campaign_id = ? AND open_deck_id = ?`, campaignID, deckID)
}

func (s *sqlMoments) Get(ctx context.Context, campaignID, id string) (Moment, bool, error) {
	return s.one(ctx, `campaign_id = ? AND id = ?`, campaignID, id)
}

func (s *sqlMoments) InvitedFor(ctx context.Context, campaignID, deckID, userID string) (Moment, bool, error) {
	return s.one(ctx, `campaign_id = ? AND open_deck_id = ? AND state = 'invited' AND invited_user_id = ?`, campaignID, deckID, userID)
}

func (s *sqlMoments) ListOpen(ctx context.Context, campaignID string) ([]Moment, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+momentColumns+` FROM deck_moments WHERE campaign_id = ? AND open_deck_id IS NOT NULL ORDER BY created_at, id`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list moments: %w", err)
	}
	defer rows.Close()
	var out []Moment
	for rows.Next() {
		m, err := scanMoment(rows)
		if err != nil {
			return nil, fmt.Errorf("scan moment: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *sqlMoments) Create(ctx context.Context, m Moment) (bool, error) {
	var readAt any
	if !m.Reading.At.IsZero() {
		readAt = m.Reading.At.UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO deck_moments (id, campaign_id, deck_id, card_id, subject_entity_id, fit_count, reading_text, reading_source, read_at, state, quiet, open_deck_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'waiting', ?, ?)`,
		m.ID, m.CampaignID, m.DeckID, m.CardID, nullString(m.SubjectID), m.FitCount, clipReading(m.Reading.Text), m.Reading.Source, readAt, m.Quiet, m.DeckID)
	if isDuplicate(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("create moment: %w", err)
	}
	// Closed moments are history; the newest few are kept.
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM deck_moments WHERE deck_id = ? AND closed_at IS NOT NULL AND id NOT IN
		   (SELECT id FROM (SELECT id FROM deck_moments WHERE deck_id = ? AND closed_at IS NOT NULL ORDER BY closed_at DESC, id LIMIT ?) kept)`,
		m.DeckID, m.DeckID, momentsKept); err != nil {
		return true, fmt.Errorf("trim moments: %w", err)
	}
	return true, nil
}

func (s *sqlMoments) Repick(ctx context.Context, campaignID, id, fromCard string, to Moment) (bool, error) {
	var readAt any
	if !to.Reading.At.IsZero() {
		readAt = to.Reading.At.UTC()
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE deck_moments SET card_id = ?, subject_entity_id = ?, fit_count = ?, reading_text = ?, reading_source = ?, read_at = ?
		 WHERE id = ? AND campaign_id = ? AND state = 'waiting' AND card_id = ?`,
		to.CardID, nullString(to.SubjectID), to.FitCount, clipReading(to.Reading.Text), to.Reading.Source, readAt, id, campaignID, fromCard)
	if err != nil {
		return false, fmt.Errorf("repick moment: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("repick moment: %w", err)
	}
	return n == 1, nil
}

func (s *sqlMoments) Refresh(ctx context.Context, campaignID, id, forCard string, to Moment) (bool, error) {
	var readAt any
	if !to.Reading.At.IsZero() {
		readAt = to.Reading.At.UTC()
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE deck_moments SET subject_entity_id = ?, fit_count = ?, reading_text = ?, reading_source = ?, read_at = ?
		 WHERE id = ? AND campaign_id = ? AND open_deck_id IS NOT NULL AND card_id = ?`,
		nullString(to.SubjectID), to.FitCount, clipReading(to.Reading.Text), to.Reading.Source, readAt, id, campaignID, forCard)
	if err != nil {
		return false, fmt.Errorf("refresh moment: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("refresh moment: %w", err)
	}
	return n == 1, nil
}

func (s *sqlMoments) Close(ctx context.Context, campaignID, id string, from []string, to, by string) (bool, error) {
	marks, args := inList(from)
	args = append([]any{to, by, id, campaignID}, args...)
	res, err := s.db.ExecContext(ctx,
		`UPDATE deck_moments SET state = ?, closed_at = NOW(), closed_by = ?, open_deck_id = NULL
		 WHERE id = ? AND campaign_id = ? AND state IN (`+marks+`)`, args...)
	if err != nil {
		return false, fmt.Errorf("close moment: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("close moment: %w", err)
	}
	return n == 1, nil
}

func (s *sqlMoments) Invite(ctx context.Context, campaignID, id, userID string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE deck_moments SET state = 'invited', invited_user_id = ? WHERE id = ? AND campaign_id = ? AND state = 'waiting'`,
		userID, id, campaignID)
	if err != nil {
		return false, fmt.Errorf("invite moment: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("invite moment: %w", err)
	}
	return n == 1, nil
}

func (s *sqlMoments) LastClosed(ctx context.Context, campaignID, deckID string) (time.Time, bool, error) {
	var t sql.NullTime
	if err := s.db.QueryRowContext(ctx,
		`SELECT MAX(closed_at) FROM deck_moments WHERE campaign_id = ? AND deck_id = ? AND closed_at IS NOT NULL`, campaignID, deckID).Scan(&t); err != nil {
		return time.Time{}, false, fmt.Errorf("last closed moment: %w", err)
	}
	return t.Time, t.Valid, nil
}

// claimMoment changes a moment's state inside a draw's transaction. It runs
// after the deck is locked; a moment that is not in one of the states, or whose
// invited player is someone else, is errMomentGone and the whole draw rolls back.
func claimMoment(ctx context.Context, tx *sql.Tx, campaignID, deckID string, c MomentClaim) error {
	marks, args := inList(c.From)
	q := `UPDATE deck_moments SET state = ?, closed_at = NOW(), closed_by = ?, open_deck_id = NULL
	      WHERE id = ? AND campaign_id = ? AND deck_id = ? AND state IN (` + marks + `)`
	full := append([]any{c.To, c.By, c.ID, campaignID, deckID}, args...)
	if c.InvitedUserID != "" {
		q += ` AND invited_user_id = ?`
		full = append(full, c.InvitedUserID)
	}
	res, err := tx.ExecContext(ctx, q, full...)
	if err != nil {
		return fmt.Errorf("claim moment: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("claim moment: %w", err)
	}
	if n != 1 {
		return errMomentGone
	}
	return nil
}
