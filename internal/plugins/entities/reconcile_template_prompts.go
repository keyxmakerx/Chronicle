package entities

import (
	"context"
	"fmt"
)

// TemplatePromptStore is what ReconcileTemplatePrompts needs. Satisfied by
// the repository returned from NewContentTemplateRepository.
type TemplatePromptStore interface {
	// ReplaceTemplateContent rewrites every content template whose JSON is
	// exactly oldJSON and returns how many it changed.
	ReplaceTemplateContent(ctx context.Context, oldJSON, newJSON, newHTML string) (int64, error)
}

// ReconcileTemplatePrompts gives campaigns seeded before template prompts
// became hints the new built-in templates, whose prompts vanish when written
// over instead of staying on the page as text. Only a copy still matching the
// old seed exactly is changed, so an edited template is left alone, and a
// second run finds nothing. The count is templates changed.
func ReconcileTemplatePrompts(ctx context.Context, store TemplatePromptStore) (int64, error) {
	var total int64
	for _, l := range legacyTemplateContent {
		n, err := store.ReplaceTemplateContent(ctx, l.oldJSON, l.newJSON, l.newHTML)
		if err != nil {
			return total, fmt.Errorf("entities.ReconcileTemplatePrompts: %w", err)
		}
		total += n
	}
	return total, nil
}

// ReplaceTemplateContent implements TemplatePromptStore.
func (r *contentTemplateRepository) ReplaceTemplateContent(ctx context.Context, oldJSON, newJSON, newHTML string) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE content_templates SET content_json = ?, content_html = ? WHERE content_json = ?`,
		newJSON, newHTML, oldJSON)
	if err != nil {
		return 0, fmt.Errorf("replacing template content: %w", err)
	}
	return res.RowsAffected()
}
