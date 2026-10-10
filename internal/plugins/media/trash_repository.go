package media

import (
	"context"
	"fmt"
	"strings"
)

// This file is the SQL behind the site Trash's file clean-ups. A trashed file
// keeps its row and its place on disk; only trash_batch_id marks it, so Undo
// is one update and storage keeps counting it until the final delete.

// trashFilesChunk bounds the placeholders in one TrashFiles statement.
const trashFilesChunk = 500

// TrashFiles marks the given unused uploads as one clean-up batch. The
// conditions are the same ones the unused-uploads scan uses (no campaign, not
// an avatar or backdrop), repeated here so an id that stopped qualifying
// between the scan and this call is simply left alone. It returns how many
// rows and bytes the batch holds afterwards.
func (r *mediaRepository) TrashFiles(ctx context.Context, batchID string, ids []string) (int, int64, error) {
	for start := 0; start < len(ids); start += trashFilesChunk {
		end := start + trashFilesChunk
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[start:end]
		args := make([]any, 0, len(chunk)+1)
		args = append(args, batchID)
		for _, id := range chunk {
			args = append(args, id)
		}
		if _, err := r.db.ExecContext(ctx,
			`UPDATE media_files SET trash_batch_id = ?
			 WHERE trash_batch_id IS NULL AND campaign_id IS NULL
			   AND usage_type NOT IN ('avatar', 'backdrop')
			   AND id IN (`+strings.TrimSuffix(strings.Repeat("?,", len(chunk)), ",")+`)`, args...); err != nil {
			return 0, 0, fmt.Errorf("moving files to trash: %w", err)
		}
	}
	var count int
	var size int64
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(file_size), 0) FROM media_files WHERE trash_batch_id = ?`, batchID,
	).Scan(&count, &size); err != nil {
		return 0, 0, fmt.Errorf("counting trashed files: %w", err)
	}
	return count, size, nil
}

// RestoreTrashedFiles clears the batch mark; the files never left disk.
// Repeating it is harmless.
func (r *mediaRepository) RestoreTrashedFiles(ctx context.Context, batchID string) (int, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE media_files SET trash_batch_id = NULL WHERE trash_batch_id = ?`, batchID)
	if err != nil {
		return 0, fmt.Errorf("restoring trashed files: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ListTrashedFileIDs lists a batch's files.
func (r *mediaRepository) ListTrashedFileIDs(ctx context.Context, batchID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM media_files WHERE trash_batch_id = ? ORDER BY id`, batchID)
	if err != nil {
		return nil, fmt.Errorf("listing trashed files: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning trashed file id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
