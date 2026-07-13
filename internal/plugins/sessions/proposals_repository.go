package sessions

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// errProposalAlreadyClosed signals that a concurrent confirm already closed the
// proposal — the conditional close matched zero rows. The service maps it to a
// clean "already confirmed" and, crucially, does NOT create a duplicate session.
var errProposalAlreadyClosed = errors.New("proposal already closed")

// Slot-proposal persistence on the existing sessionRepository (C-SCHED-P2).
// Proposals/options/responses/tokens live in their OWN tables — never
// session_attendees — so they stay out of export egress by construction
// (own-tables egress test, extended in 0b).

// CreateProposal inserts a proposal and its options atomically.
func (r *sessionRepository) CreateProposal(ctx context.Context, p *SlotProposal, options []SlotProposalOption) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin proposal tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO slot_proposals (id, campaign_id, created_by, title, note, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.CampaignID, p.CreatedBy, p.Title, p.Note, p.Status, p.CreatedAt, p.UpdatedAt); err != nil {
		return fmt.Errorf("inserting proposal: %w", err)
	}

	const ins = `INSERT INTO slot_proposal_options (id, proposal_id, starts_at_utc, ends_at_utc, ordinal, is_winner)
	             VALUES (?, ?, ?, ?, ?, ?)`
	for _, o := range options {
		if _, err := tx.ExecContext(ctx, ins,
			o.ID, o.ProposalID, o.StartsAtUTC.UTC(), o.EndsAtUTC.UTC(), o.Ordinal, o.IsWinner); err != nil {
			return fmt.Errorf("inserting proposal option: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit proposal tx: %w", err)
	}
	return nil
}

// GetProposal fetches a proposal scoped to its campaign (IDOR guard) plus its
// options in display order.
func (r *sessionRepository) GetProposal(ctx context.Context, campaignID, proposalID string) (*SlotProposal, []SlotProposalOption, error) {
	var p SlotProposal
	var note sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT id, campaign_id, created_by, title, note, status, created_at, updated_at
		 FROM slot_proposals WHERE id = ? AND campaign_id = ?`,
		proposalID, campaignID).Scan(&p.ID, &p.CampaignID, &p.CreatedBy, &p.Title, &note, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil, apperror.NewNotFound("proposal not found")
	}
	if err != nil {
		return nil, nil, fmt.Errorf("loading proposal: %w", err)
	}
	if note.Valid {
		p.Note = &note.String
	}
	opts, err := r.ListProposalOptions(ctx, proposalID)
	if err != nil {
		return nil, nil, err
	}
	return &p, opts, nil
}

// FindProposalByID loads a proposal by id ALONE — no campaign scope. Used by the
// emailed-token path (C-SCHED-P3 0a), where the campaign is DERIVED from the
// proposal (the token, not a URL campaign id, is the credential) so the redeem
// can recheck the proposal is still open + the user still a member.
func (r *sessionRepository) FindProposalByID(ctx context.Context, proposalID string) (*SlotProposal, error) {
	var p SlotProposal
	var note sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT id, campaign_id, created_by, title, note, status, created_at, updated_at
		 FROM slot_proposals WHERE id = ?`, proposalID).
		Scan(&p.ID, &p.CampaignID, &p.CreatedBy, &p.Title, &note, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, apperror.NewNotFound("proposal not found")
	}
	if err != nil {
		return nil, fmt.Errorf("finding proposal by id: %w", err)
	}
	if note.Valid {
		p.Note = &note.String
	}
	return &p, nil
}

// SetProposalWinnerAndClose marks one option the winner (clearing any other) and
// closes the proposal, atomically (C-SCHED-P3 confirm-winner). The CONDITIONAL
// close (`WHERE status = 'open'`) runs FIRST and is the serialization point:
// exactly one confirm can flip open→closed, so a concurrent double-confirm has
// the loser match zero rows and return errProposalAlreadyClosed (rolled back) —
// the service then skips session creation, so a proposal never mints two
// sessions. `is_winner = (id = ?)` sets 1 on the winner and 0 on every sibling in
// one UPDATE.
func (r *sessionRepository) SetProposalWinnerAndClose(ctx context.Context, proposalID, winningOptionID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin confirm tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit

	res, err := tx.ExecContext(ctx,
		`UPDATE slot_proposals SET status = ?, updated_at = ? WHERE id = ? AND status = ?`,
		ProposalClosed, time.Now().UTC(), proposalID, ProposalOpen)
	if err != nil {
		return fmt.Errorf("closing proposal: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errProposalAlreadyClosed // already closed (concurrent confirm) — no duplicate session
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE slot_proposal_options SET is_winner = (id = ?) WHERE proposal_id = ?`,
		winningOptionID, proposalID); err != nil {
		return fmt.Errorf("setting winner: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit confirm tx: %w", err)
	}
	return nil
}

// ListProposals returns a campaign's proposals, newest first.
func (r *sessionRepository) ListProposals(ctx context.Context, campaignID string) ([]SlotProposal, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, campaign_id, created_by, title, note, status, created_at, updated_at
		 FROM slot_proposals WHERE campaign_id = ? ORDER BY created_at DESC`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing proposals: %w", err)
	}
	defer rows.Close()
	var out []SlotProposal
	for rows.Next() {
		var p SlotProposal
		var note sql.NullString
		if err := rows.Scan(&p.ID, &p.CampaignID, &p.CreatedBy, &p.Title, &note, &p.Status, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning proposal: %w", err)
		}
		if note.Valid {
			p.Note = &note.String
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ListProposalOptions returns a proposal's options in display order.
func (r *sessionRepository) ListProposalOptions(ctx context.Context, proposalID string) ([]SlotProposalOption, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, proposal_id, starts_at_utc, ends_at_utc, ordinal, is_winner
		 FROM slot_proposal_options WHERE proposal_id = ? ORDER BY ordinal`, proposalID)
	if err != nil {
		return nil, fmt.Errorf("listing proposal options: %w", err)
	}
	defer rows.Close()
	var out []SlotProposalOption
	for rows.Next() {
		var o SlotProposalOption
		if err := rows.Scan(&o.ID, &o.ProposalID, &o.StartsAtUTC, &o.EndsAtUTC, &o.Ordinal, &o.IsWinner); err != nil {
			return nil, fmt.Errorf("scanning proposal option: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// FindOption returns a single option by id (used to resolve the proposal + IDOR
// scope for a response or token redemption).
func (r *sessionRepository) FindOption(ctx context.Context, optionID string) (*SlotProposalOption, error) {
	var o SlotProposalOption
	err := r.db.QueryRowContext(ctx,
		`SELECT id, proposal_id, starts_at_utc, ends_at_utc, ordinal, is_winner
		 FROM slot_proposal_options WHERE id = ?`, optionID).
		Scan(&o.ID, &o.ProposalID, &o.StartsAtUTC, &o.EndsAtUTC, &o.Ordinal, &o.IsWinner)
	if err == sql.ErrNoRows {
		return nil, apperror.NewNotFound("option not found")
	}
	if err != nil {
		return nil, fmt.Errorf("finding option: %w", err)
	}
	return &o, nil
}

// UpsertProposalResponse records (or replaces) a member's response to an option.
func (r *sessionRepository) UpsertProposalResponse(ctx context.Context, resp *SlotProposalResponse) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO slot_proposal_responses (id, option_id, user_id, response, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE response = VALUES(response), updated_at = VALUES(updated_at)`,
		resp.ID, resp.OptionID, resp.UserID, resp.Response, resp.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upserting response: %w", err)
	}
	return nil
}

// ListProposalResponses returns every response across all of a proposal's
// options (joined so a single query serves the tally).
func (r *sessionRepository) ListProposalResponses(ctx context.Context, proposalID string) ([]SlotProposalResponse, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT rp.id, rp.option_id, rp.user_id, rp.response, rp.updated_at
		 FROM slot_proposal_responses rp
		 JOIN slot_proposal_options o ON rp.option_id = o.id
		 WHERE o.proposal_id = ?`, proposalID)
	if err != nil {
		return nil, fmt.Errorf("listing proposal responses: %w", err)
	}
	defer rows.Close()
	var out []SlotProposalResponse
	for rows.Next() {
		var resp SlotProposalResponse
		if err := rows.Scan(&resp.ID, &resp.OptionID, &resp.UserID, &resp.Response, &resp.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning response: %w", err)
		}
		out = append(out, resp)
	}
	return out, rows.Err()
}

// CreateProposalToken stores a one-click response token.
func (r *sessionRepository) CreateProposalToken(ctx context.Context, token *SlotProposalToken) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO slot_proposal_tokens (token, option_id, user_id, response, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		token.Token, token.OptionID, token.UserID, token.Response, token.ExpiresAt, token.CreatedAt)
	if err != nil {
		return fmt.Errorf("creating proposal token: %w", err)
	}
	return nil
}

// FindProposalToken looks up a response token by its string.
func (r *sessionRepository) FindProposalToken(ctx context.Context, tokenStr string) (*SlotProposalToken, error) {
	var t SlotProposalToken
	var usedAt sql.NullTime
	err := r.db.QueryRowContext(ctx,
		`SELECT id, token, option_id, user_id, response, used_at, expires_at, created_at
		 FROM slot_proposal_tokens WHERE token = ?`, tokenStr).
		Scan(&t.ID, &t.Token, &t.OptionID, &t.UserID, &t.Response, &usedAt, &t.ExpiresAt, &t.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, apperror.NewNotFound("token not found")
	}
	if err != nil {
		return nil, fmt.Errorf("finding proposal token: %w", err)
	}
	if usedAt.Valid {
		t.UsedAt = &usedAt.Time
	}
	return &t, nil
}

// MarkProposalTokenUsed stamps a token as consumed (single-use).
func (r *sessionRepository) MarkProposalTokenUsed(ctx context.Context, tokenStr string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE slot_proposal_tokens SET used_at = ? WHERE token = ?`, time.Now().UTC(), tokenStr)
	if err != nil {
		return fmt.Errorf("marking proposal token used: %w", err)
	}
	return nil
}
