// import_memory_repository.go holds the SQL for what AI Import keeps per
// campaign: the parts of a split answer, waiting in the tray or applied
// (ai_import_parts), and a fingerprint of each block applied
// (ai_import_blocks), both plugin migrations. Every statement is scoped by
// campaign_id.
package ai_workspace

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// StoredPart is one labelled part of an answer: waiting in the tray (its
// Markdown set), applied, or both when an applied part was pasted again.
type StoredPart struct {
	AnswerKey  string
	AnswerName string
	Number     int
	Of         int
	Needs      []int
	// Markdown is the part as pasted, label first; "" when not in the tray.
	Markdown string
	Applied  bool
	AddedBy  string
	AddedAt  time.Time
}

// ImportMemory is what AI Import keeps per campaign.
type ImportMemory interface {
	// Parts lists every stored part of one answer.
	Parts(ctx context.Context, campaignID, answerKey string) ([]StoredPart, error)
	// Tray lists the parts waiting in the tray, of every answer.
	Tray(ctx context.Context, campaignID string) ([]StoredPart, error)
	// PutInTray stores a part to wait for the rest of its answer.
	PutInTray(ctx context.Context, campaignID string, p StoredPart) error
	// MarkApplied records a part as applied and takes it out of the tray.
	MarkApplied(ctx context.Context, campaignID string, p StoredPart) error
	// ClearTray takes an answer's waiting parts out of the tray.
	ClearTray(ctx context.Context, campaignID, answerKey string) error
	// Seen returns, for each fingerprint already applied, when it was.
	Seen(ctx context.Context, campaignID string, hashes []string) (map[string]time.Time, error)
	// Remember records fingerprints as applied now.
	Remember(ctx context.Context, campaignID string, hashes []string) error
}

type importMemoryRepository struct{ db *sql.DB }

// NewImportMemoryRepository creates the import memory repository.
func NewImportMemoryRepository(db *sql.DB) ImportMemory { return &importMemoryRepository{db: db} }

func memoryDBError(what string, err error) error {
	return apperror.NewInternal(fmt.Errorf("%s: %w", what, err))
}

// answerKeyMax fits the key column; longer names still match on their start.
const answerKeyMax = 191

func clipKey(k string) string {
	if len(k) > answerKeyMax {
		return k[:answerKeyMax]
	}
	return k
}

func clipName(n string) string {
	if len(n) > 255 {
		return n[:255]
	}
	return n
}

const partColumns = `answer_key, answer_name, part_no, part_of, needs, markdown, applied_at IS NOT NULL, added_by, added_at`

func (r *importMemoryRepository) listParts(ctx context.Context, query string, args ...any) ([]StoredPart, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, memoryDBError("listing answer parts", err)
	}
	defer rows.Close()
	var out []StoredPart
	for rows.Next() {
		var p StoredPart
		var needs string
		var md sql.NullString
		if err := rows.Scan(&p.AnswerKey, &p.AnswerName, &p.Number, &p.Of, &needs, &md, &p.Applied, &p.AddedBy, &p.AddedAt); err != nil {
			return nil, memoryDBError("reading an answer part", err)
		}
		p.Needs = splitNeeds(needs)
		p.Markdown = md.String
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, memoryDBError("listing answer parts", err)
	}
	return out, nil
}

func (r *importMemoryRepository) Parts(ctx context.Context, campaignID, answerKey string) ([]StoredPart, error) {
	return r.listParts(ctx, `SELECT `+partColumns+` FROM ai_import_parts
		WHERE campaign_id = ? AND answer_key = ? ORDER BY part_no`, campaignID, clipKey(answerKey))
}

func (r *importMemoryRepository) Tray(ctx context.Context, campaignID string) ([]StoredPart, error) {
	return r.listParts(ctx, `SELECT `+partColumns+` FROM ai_import_parts
		WHERE campaign_id = ? AND markdown IS NOT NULL ORDER BY added_at, answer_key, part_no`, campaignID)
}

func (r *importMemoryRepository) PutInTray(ctx context.Context, campaignID string, p StoredPart) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO ai_import_parts
		(campaign_id, answer_key, part_no, answer_name, part_of, needs, markdown, added_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE answer_name = VALUES(answer_name), part_of = VALUES(part_of),
			needs = VALUES(needs), markdown = VALUES(markdown), added_by = VALUES(added_by),
			added_at = CURRENT_TIMESTAMP`,
		campaignID, clipKey(p.AnswerKey), p.Number, clipName(p.AnswerName), p.Of, joinNeeds(p.Needs), p.Markdown, p.AddedBy)
	if err != nil {
		return memoryDBError("putting a part in the tray", err)
	}
	return nil
}

func (r *importMemoryRepository) MarkApplied(ctx context.Context, campaignID string, p StoredPart) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO ai_import_parts
		(campaign_id, answer_key, part_no, answer_name, part_of, needs, markdown, added_by, applied_at)
		VALUES (?, ?, ?, ?, ?, ?, NULL, ?, CURRENT_TIMESTAMP)
		ON DUPLICATE KEY UPDATE answer_name = VALUES(answer_name), part_of = VALUES(part_of),
			needs = VALUES(needs), markdown = NULL, applied_at = CURRENT_TIMESTAMP`,
		campaignID, clipKey(p.AnswerKey), p.Number, clipName(p.AnswerName), p.Of, joinNeeds(p.Needs), p.AddedBy)
	if err != nil {
		return memoryDBError("recording an applied part", err)
	}
	return nil
}

func (r *importMemoryRepository) ClearTray(ctx context.Context, campaignID, answerKey string) error {
	key := clipKey(answerKey)
	// A part that was never applied has nothing left to remember.
	if _, err := r.db.ExecContext(ctx, `DELETE FROM ai_import_parts
		WHERE campaign_id = ? AND answer_key = ? AND applied_at IS NULL`, campaignID, key); err != nil {
		return memoryDBError("emptying the tray", err)
	}
	if _, err := r.db.ExecContext(ctx, `UPDATE ai_import_parts SET markdown = NULL
		WHERE campaign_id = ? AND answer_key = ?`, campaignID, key); err != nil {
		return memoryDBError("emptying the tray", err)
	}
	return nil
}

// seenBatch keeps each IN list a reasonable size.
const seenBatch = 200

func (r *importMemoryRepository) Seen(ctx context.Context, campaignID string, hashes []string) (map[string]time.Time, error) {
	out := map[string]time.Time{}
	for start := 0; start < len(hashes); start += seenBatch {
		batch := hashes[start:min(start+seenBatch, len(hashes))]
		args := make([]any, 0, len(batch)+1)
		args = append(args, campaignID)
		for _, h := range batch {
			args = append(args, h)
		}
		rows, err := r.db.QueryContext(ctx, `SELECT block_hash, applied_at FROM ai_import_blocks
			WHERE campaign_id = ? AND block_hash IN (?`+strings.Repeat(",?", len(batch)-1)+`)`, args...)
		if err != nil {
			return nil, memoryDBError("looking up applied blocks", err)
		}
		for rows.Next() {
			var h string
			var at time.Time
			if err := rows.Scan(&h, &at); err != nil {
				rows.Close()
				return nil, memoryDBError("reading an applied block", err)
			}
			out[h] = at
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, memoryDBError("looking up applied blocks", err)
		}
	}
	return out, nil
}

func (r *importMemoryRepository) Remember(ctx context.Context, campaignID string, hashes []string) error {
	for _, h := range hashes {
		if _, err := r.db.ExecContext(ctx, `INSERT INTO ai_import_blocks (campaign_id, block_hash) VALUES (?, ?)
			ON DUPLICATE KEY UPDATE applied_at = CURRENT_TIMESTAMP`, campaignID, h); err != nil {
			return memoryDBError("remembering an applied block", err)
		}
	}
	return nil
}

func joinNeeds(n []int) string {
	s := make([]string, len(n))
	for i, v := range n {
		s[i] = strconv.Itoa(v)
	}
	return strings.Join(s, ",")
}

func splitNeeds(s string) []int {
	var out []int
	for _, f := range strings.Split(s, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(f)); err == nil {
			out = append(out, n)
		}
	}
	return out
}
