// foundry_monsters.go — the monsters a campaign's Foundry world can place,
// as the GM's Foundry client reports them (POST /sync/monsters): its world's
// NPC actors and its Actor compendiums' entries, with the few numbers a DM
// builds a fight from. The AI connection reads it so it plans fights with
// monsters Foundry already has instead of writing copies of them.
//
// Everything in a report is the client's word: each field is capped and
// stripped of control characters, the list is capped, and only an owner or
// DM-access key may send it, so a bad client can at worst mislabel its own
// campaign's list.

package syncapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// FoundryMonster is one actor Foundry can place.
type FoundryMonster struct {
	Name string `json:"name"`
	// Pack is the compendium's label; empty for a world actor.
	Pack         string `json:"pack,omitempty"`
	Level        string `json:"level,omitempty"`
	Role         string `json:"role,omitempty"`
	Organization string `json:"organization,omitempty"`
	EV           string `json:"ev,omitempty"`
	// Linked marks an actor creature sync made from a Chronicle page, and
	// Page is that page's id.
	Linked bool   `json:"linked,omitempty"`
	Page   string `json:"page,omitempty"`
}

// FoundryMonsterList is a campaign's last report.
type FoundryMonsterList struct {
	Monsters   []FoundryMonster
	Truncated  bool
	ReportedAt time.Time
}

// Limits on one report. A big compendium set is a few thousand entries; past
// the cap the client sends what fits and says so.
const (
	foundryMonstersMax     = 5000
	foundryMonsterNameMax  = 120
	foundryMonsterPackMax  = 120
	foundryMonsterShortMax = 40
	foundryMonsterIDMax    = 36
	// foundryMonstersRetention matches the players list's.
	foundryMonstersRetention = foundryPlayersRetention
)

// FoundryMonsterRepository owns the foundry_monster_lists table.
type FoundryMonsterRepository interface {
	// Replace makes list the campaign's whole list.
	Replace(ctx context.Context, campaignID string, list FoundryMonsterList) error
	// Get is the campaign's list; nil when Foundry never sent one.
	Get(ctx context.Context, campaignID string) (*FoundryMonsterList, error)
	// PruneOlderThan deletes lists last reported before cutoff.
	PruneOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
}

type foundryMonsterRepo struct{ db *sql.DB }

// NewFoundryMonsterRepository creates the MariaDB-backed repository.
func NewFoundryMonsterRepository(db *sql.DB) FoundryMonsterRepository {
	return &foundryMonsterRepo{db: db}
}

func (r *foundryMonsterRepo) Replace(ctx context.Context, campaignID string, list FoundryMonsterList) error {
	data, err := json.Marshal(list.Monsters)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("encode foundry monsters: %w", err))
	}
	if _, err := r.db.ExecContext(ctx, `INSERT INTO foundry_monster_lists (campaign_id, monsters, truncated, reported_at)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE monsters = VALUES(monsters), truncated = VALUES(truncated), reported_at = VALUES(reported_at)`,
		campaignID, string(data), list.Truncated, list.ReportedAt.UTC()); err != nil {
		return apperror.NewInternal(fmt.Errorf("store foundry monsters: %w", err))
	}
	return nil
}

func (r *foundryMonsterRepo) Get(ctx context.Context, campaignID string) (*FoundryMonsterList, error) {
	var data string
	out := &FoundryMonsterList{}
	err := r.db.QueryRowContext(ctx, `SELECT monsters, truncated, reported_at FROM foundry_monster_lists WHERE campaign_id = ?`,
		campaignID).Scan(&data, &out.Truncated, &out.ReportedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("read foundry monsters: %w", err))
	}
	if err := json.Unmarshal([]byte(data), &out.Monsters); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("decode foundry monsters: %w", err))
	}
	return out, nil
}

func (r *foundryMonsterRepo) PruneOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM foundry_monster_lists WHERE reported_at < ?`, cutoff.UTC())
	if err != nil {
		return 0, apperror.NewInternal(fmt.Errorf("prune foundry monsters: %w", err))
	}
	return res.RowsAffected()
}

type reportMonstersRequest struct {
	Monsters  []FoundryMonster `json:"monsters"`
	Truncated bool             `json:"truncated"`
}

// cleanMonsters caps and strips a report. An entry with no name is dropped;
// a duplicate of the same name in the same place keeps its first entry.
func cleanMonsters(in reportMonstersRequest, now time.Time) (FoundryMonsterList, error) {
	if len(in.Monsters) > foundryMonstersMax {
		return FoundryMonsterList{}, apperror.NewBadRequest(fmt.Sprintf("too many monsters in one report (at most %d)", foundryMonstersMax))
	}
	field := func(s string, n int) string { return truncate(strings.TrimSpace(stripControl(s)), n) }
	seen := make(map[string]bool, len(in.Monsters))
	out := FoundryMonsterList{Monsters: make([]FoundryMonster, 0, len(in.Monsters)), Truncated: in.Truncated, ReportedAt: now}
	for _, m := range in.Monsters {
		c := FoundryMonster{
			Name:         field(m.Name, foundryMonsterNameMax),
			Pack:         field(m.Pack, foundryMonsterPackMax),
			Level:        field(m.Level, foundryMonsterShortMax),
			Role:         field(m.Role, foundryMonsterShortMax),
			Organization: field(m.Organization, foundryMonsterShortMax),
			EV:           field(m.EV, foundryMonsterShortMax),
			Linked:       m.Linked,
		}
		if c.Linked {
			c.Page = field(m.Page, foundryMonsterIDMax)
		}
		if c.Name == "" {
			continue
		}
		key := strings.ToLower(c.Pack) + "\x00" + strings.ToLower(c.Name)
		if seen[key] {
			continue
		}
		seen[key] = true
		out.Monsters = append(out.Monsters, c)
	}
	return out, nil
}

// SetFoundryMonsters wires the monster store. Without it the report endpoint
// answers 404.
func (h *SyncHistoryHandler) SetFoundryMonsters(repo FoundryMonsterRepository) {
	h.monsters = repo
}

// ReportMonsters replaces the campaign's Foundry monster list with the GM
// client's report. Owner or DM-access keys only: the list includes the GM's
// own actors, which players may not know exist.
// POST /api/v1/campaigns/:id/sync/monsters
func (h *SyncHistoryHandler) ReportMonsters(c echo.Context) error {
	if h.monsters == nil {
		return apperror.NewNotFound("monster reports are not available")
	}
	if !h.isDMEquivalent(c) {
		return apperror.NewForbidden("reporting Foundry monsters requires owner or DM access")
	}
	var req reportMonstersRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	list, err := cleanMonsters(req, h.now())
	if err != nil {
		return err
	}
	if err := h.monsters.Replace(c.Request().Context(), c.Param("id"), list); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]int{"stored": len(list.Monsters)})
}

// pruneMonsters drops lists Foundry has not refreshed in a while.
func pruneMonsters(ctx context.Context, repo FoundryMonsterRepository, now time.Time) (int64, error) {
	if repo == nil {
		return 0, nil
	}
	return repo.PruneOlderThan(ctx, now.Add(-foundryMonstersRetention))
}
