package systems

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// Pins put a few entries of a hero pick list first, in the order the Director
// team pinned them; the rest keep the list's own order. Players see the order
// only; who may pin is decided here, not by the screen that offers it.

// MaxChoicePins bounds how many entries one list may pin, so a pinned group
// stays a short lead rather than a second list.
const MaxChoicePins = 50

// maxPinKeyLen is the entry_key column's width.
const maxPinKeyLen = 160

// choicePinKey is the stored key of an entry: its source and key together, so
// a package slug and a campaign entry id can never name each other.
func choicePinKey(source, key string) string { return source + ":" + key }

// orderPinned moves the pinned entries to the front in pin order and marks
// them; every other entry keeps its place relative to the rest. A pin naming
// no entry on the list (one deleted, or left out for this reader) is skipped.
func orderPinned(choices []Choice, pins []string) []Choice {
	if len(pins) == 0 || len(choices) == 0 {
		return choices
	}
	at := make(map[string]int, len(pins))
	for i, p := range pins {
		if _, dup := at[p]; !dup {
			at[p] = i
		}
	}
	lead := make([]Choice, len(pins))
	has := make([]bool, len(pins))
	rest := make([]Choice, 0, len(choices))
	for _, c := range choices {
		if i, ok := at[choicePinKey(c.Source, c.Key)]; ok && c.Key != "" && !has[i] {
			c.Pinned = true
			lead[i], has[i] = c, true
			continue
		}
		rest = append(rest, c)
	}
	out := make([]Choice, 0, len(choices))
	for i := range lead {
		if has[i] {
			out = append(out, lead[i])
		}
	}
	return append(out, rest...)
}

// ChoicePinReader is what the pick list reads: one list's pins, in order.
type ChoicePinReader interface {
	ChoicePins(ctx context.Context, campaignID, systemID, fieldKey string) ([]string, error)
}

// ChoicePinRepository stores pins. Every method is scoped by campaign.
type ChoicePinRepository interface {
	// List returns one list's pinned entry keys in pin order.
	List(ctx context.Context, campaignID, systemID, fieldKey string) ([]string, error)
	// Replace writes the list's pins as given, in one transaction, so a
	// change never half-lands.
	Replace(ctx context.Context, campaignID, systemID, fieldKey string, keys []string, updatedBy string) error
}

type choicePinRepository struct{ db *sql.DB }

// NewChoicePinRepository creates the MariaDB-backed repository.
func NewChoicePinRepository(db *sql.DB) ChoicePinRepository { return &choicePinRepository{db: db} }

func (r *choicePinRepository) List(ctx context.Context, campaignID, systemID, fieldKey string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT entry_key FROM campaign_choice_pins
		  WHERE campaign_id = ? AND system_id = ? AND field_key = ?
		  ORDER BY position, entry_key`, campaignID, systemID, fieldKey)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, apperror.NewInternal(err)
		}
		out = append(out, k)
	}
	if err := rows.Err(); err != nil {
		return nil, apperror.NewInternal(err)
	}
	return out, nil
}

func (r *choicePinRepository) Replace(ctx context.Context, campaignID, systemID, fieldKey string, keys []string, updatedBy string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return apperror.NewInternal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM campaign_choice_pins WHERE campaign_id = ? AND system_id = ? AND field_key = ?`,
		campaignID, systemID, fieldKey); err != nil {
		return apperror.NewInternal(err)
	}
	for i, k := range keys {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO campaign_choice_pins (campaign_id, system_id, field_key, entry_key, position, updated_by)
			 VALUES (?, ?, ?, ?, ?, ?)`, campaignID, systemID, fieldKey, k, i, updatedBy); err != nil {
			return apperror.NewInternal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

// ChoicePinService keeps the Director team's pins.
type ChoicePinService interface {
	ChoicePinReader
	// SetPinned pins an entry (last among the pins) or unpins it, and returns
	// the list's pins after the change. director is the caller's standing:
	// anyone below the Director team is refused. An entry that is not on the
	// field's list cannot be pinned.
	SetPinned(ctx context.Context, campaignID, userID string, director bool, fieldKey, source, key string, pinned bool) ([]string, error)
}

type choicePinService struct {
	repo    ChoicePinRepository
	choices CharacterChoiceService
}

// NewChoicePinService wires the service. choices is read as a Director sees
// the list, to find the system and which entries exist.
func NewChoicePinService(repo ChoicePinRepository, choices CharacterChoiceService) ChoicePinService {
	return &choicePinService{repo: repo, choices: choices}
}

func (s *choicePinService) ChoicePins(ctx context.Context, campaignID, systemID, fieldKey string) ([]string, error) {
	return s.repo.List(ctx, campaignID, systemID, fieldKey)
}

func (s *choicePinService) SetPinned(ctx context.Context, campaignID, userID string, director bool, fieldKey, source, key string, pinned bool) ([]string, error) {
	if !director {
		return nil, apperror.NewForbidden("only the Director team can pin entries")
	}
	if !ValidChoiceFieldKey(fieldKey) {
		return nil, apperror.NewBadRequest("invalid field key")
	}
	if source != ChoiceSourcePackage && source != ChoiceSourceCampaign {
		return nil, apperror.NewBadRequest("unknown entry source")
	}
	key = strings.TrimSpace(key)
	pin := choicePinKey(source, key)
	if key == "" || len(pin) > maxPinKeyLen {
		return nil, apperror.NewBadRequest("invalid entry key")
	}
	list, err := s.choices.CharacterChoiceList(WithChoiceViewerUser(WithChoiceViewer(ctx, true), userID), campaignID, fieldKey)
	if err != nil {
		return nil, err
	}
	if list.SystemID == "" {
		return nil, apperror.NewNotFound("this list has no entries")
	}
	current, err := s.repo.List(ctx, campaignID, list.SystemID, fieldKey)
	if err != nil {
		return nil, err
	}
	next := make([]string, 0, len(current)+1)
	for _, k := range current {
		if k != pin {
			next = append(next, k)
		}
	}
	if pinned {
		found := false
		for _, c := range list.Choices {
			if c.Source == source && c.Key == key {
				found = true
				break
			}
		}
		if !found {
			return nil, apperror.NewNotFound("that entry is not on this list")
		}
		if len(next) >= MaxChoicePins {
			return nil, apperror.NewBadRequest("this list already has the most pins it can hold")
		}
		next = append(next, pin)
	}
	if err := s.repo.Replace(ctx, campaignID, list.SystemID, fieldKey, next, userID); err != nil {
		return nil, err
	}
	return next, nil
}

// applyPins puts the list's pinned entries first. A failed read leaves the
// list in its own order: a pin is an arrangement, not a rule, and must not
// take the pick list down.
func (s *characterChoiceService) applyPins(ctx context.Context, campaignID string, l *ChoiceList) {
	if s.pins == nil || l == nil || l.SystemID == "" || len(l.Choices) == 0 {
		return
	}
	pins, err := s.pins.ChoicePins(ctx, campaignID, l.SystemID, l.FieldKey)
	if err != nil {
		slog.Warn("pins could not be read; serving the list unpinned",
			slog.String("field", l.FieldKey), slog.Any("error", err))
		return
	}
	l.Choices = orderPinned(l.Choices, pins)
}
