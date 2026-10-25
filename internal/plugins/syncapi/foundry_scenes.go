// foundry_scenes.go — the scenes in a campaign's Foundry world, as the GM's
// Foundry client reports them (POST /sync/scenes).
//
// Chronicle has no other way to know which maps exist in Foundry. The GM's
// client sends the whole list when it connects and when scenes change; each
// report replaces the campaign's rows. Only names and a few flags travel,
// never pictures: most scenes are animated, and the list is for knowing what
// exists, not for showing it.
//
// Scene names can give away what is coming, so the list is read only by the
// owner and those with DM access, never players. Everything in a report is
// the client's word: names are capped and stripped of control characters.

package syncapi

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// FoundryScene is one scene of a campaign's Foundry world.
type FoundryScene struct {
	SceneID string
	Name    string
	// Folder is the scene's folder path in Foundry ("Act 1 / Caves"); empty
	// at the top level.
	Folder string
	// Active is the scene players are on; InNavigation is whether it is in
	// Foundry's scene bar.
	Active       bool
	InNavigation bool
}

// FoundrySceneReport is a campaign's last scene report.
type FoundrySceneReport struct {
	Scenes []FoundryScene
	// Truncated is true when the world had more scenes than one report holds.
	Truncated  bool
	ReportedAt time.Time
}

// Limits on one report.
const (
	foundryScenesMax      = 2000
	foundrySceneIDMax     = 64
	foundrySceneNameMax   = 255
	foundrySceneFolderMax = 255
)

// FoundrySceneRepository owns the foundry_scenes and foundry_scene_reports
// tables.
type FoundrySceneRepository interface {
	// Replace makes the report the campaign's whole list.
	Replace(ctx context.Context, campaignID string, report FoundrySceneReport) error
	// Get returns the campaign's last report, scenes in Foundry's sidebar
	// order; nil when Foundry has never reported.
	Get(ctx context.Context, campaignID string) (*FoundrySceneReport, error)
}

type foundrySceneRepo struct {
	db *sql.DB
}

// NewFoundrySceneRepository creates the MariaDB-backed repository.
func NewFoundrySceneRepository(db *sql.DB) FoundrySceneRepository {
	return &foundrySceneRepo{db: db}
}

func (r *foundrySceneRepo) Replace(ctx context.Context, campaignID string, report FoundrySceneReport) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("begin foundry scenes: %w", err))
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM foundry_scenes WHERE campaign_id = ?`, campaignID); err != nil {
		return apperror.NewInternal(fmt.Errorf("clear foundry scenes: %w", err))
	}
	for i, s := range report.Scenes {
		if _, err := tx.ExecContext(ctx, `INSERT INTO foundry_scenes
			(campaign_id, scene_id, name, folder, is_active, in_navigation, position)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			campaignID, s.SceneID, s.Name, s.Folder, s.Active, s.InNavigation, i); err != nil {
			return apperror.NewInternal(fmt.Errorf("insert foundry scene: %w", err))
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO foundry_scene_reports
		(campaign_id, scene_count, truncated, reported_at) VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE scene_count = VALUES(scene_count),
			truncated = VALUES(truncated), reported_at = VALUES(reported_at)`,
		campaignID, len(report.Scenes), report.Truncated, report.ReportedAt.UTC()); err != nil {
		return apperror.NewInternal(fmt.Errorf("record foundry scene report: %w", err))
	}
	if err := tx.Commit(); err != nil {
		return apperror.NewInternal(fmt.Errorf("commit foundry scenes: %w", err))
	}
	return nil
}

func (r *foundrySceneRepo) Get(ctx context.Context, campaignID string) (*FoundrySceneReport, error) {
	rep := &FoundrySceneReport{Scenes: []FoundryScene{}}
	err := r.db.QueryRowContext(ctx, `SELECT truncated, reported_at FROM foundry_scene_reports
		WHERE campaign_id = ?`, campaignID).Scan(&rep.Truncated, &rep.ReportedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("read foundry scene report: %w", err))
	}
	rows, err := r.db.QueryContext(ctx, `SELECT scene_id, name, folder, is_active, in_navigation
		FROM foundry_scenes WHERE campaign_id = ? ORDER BY position, scene_id`, campaignID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("list foundry scenes: %w", err))
	}
	defer rows.Close()
	for rows.Next() {
		var s FoundryScene
		if err := rows.Scan(&s.SceneID, &s.Name, &s.Folder, &s.Active, &s.InNavigation); err != nil {
			return nil, apperror.NewInternal(fmt.Errorf("scan foundry scene: %w", err))
		}
		rep.Scenes = append(rep.Scenes, s)
	}
	if err := rows.Err(); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("read foundry scenes: %w", err))
	}
	return rep, nil
}

// reportedScene is one entry of POST /sync/scenes, as the module sends it.
type reportedScene struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Folder     string `json:"folder"`
	Active     bool   `json:"active"`
	Navigation bool   `json:"navigation"`
}

type reportScenesRequest struct {
	Scenes    []reportedScene `json:"scenes"`
	Truncated bool            `json:"truncated"`
}

// cleanScenes validates and clamps a report, keeping the order it came in
// (Foundry's sidebar order, which the owner arranged). A duplicate scene id
// keeps its first entry; a scene with no name is stored as "Unnamed scene" so
// it still counts.
func cleanScenes(req reportScenesRequest, now time.Time) (FoundrySceneReport, error) {
	if len(req.Scenes) > foundryScenesMax {
		return FoundrySceneReport{}, apperror.NewBadRequest(fmt.Sprintf("too many scenes in one report (at most %d)", foundryScenesMax))
	}
	seen := make(map[string]bool, len(req.Scenes))
	out := FoundrySceneReport{Scenes: make([]FoundryScene, 0, len(req.Scenes)), Truncated: req.Truncated, ReportedAt: now}
	for _, rs := range req.Scenes {
		id := strings.TrimSpace(rs.ID)
		if id == "" || len(id) > foundrySceneIDMax || !isPrintableASCII(id) {
			return FoundrySceneReport{}, apperror.NewBadRequest("each scene needs an id of 1 to 64 plain characters")
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		name := truncate(strings.TrimSpace(stripControl(rs.Name)), foundrySceneNameMax)
		if name == "" {
			name = "Unnamed scene"
		}
		out.Scenes = append(out.Scenes, FoundryScene{
			SceneID:      id,
			Name:         name,
			Folder:       truncate(strings.TrimSpace(stripControl(rs.Folder)), foundrySceneFolderMax),
			Active:       rs.Active,
			InNavigation: rs.Navigation,
		})
	}
	return out, nil
}
