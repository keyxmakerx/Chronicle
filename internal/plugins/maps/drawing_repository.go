package maps

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// DrawingRepository defines persistence for map drawings, tokens, layers, and fog.
type DrawingRepository interface {
	// Drawing CRUD.
	CreateDrawing(ctx context.Context, d *Drawing) error
	GetDrawing(ctx context.Context, id string) (*Drawing, error)
	UpdateDrawing(ctx context.Context, d *Drawing) error
	DeleteDrawing(ctx context.Context, id string) error
	ListDrawings(ctx context.Context, mapID string, role int) ([]Drawing, error)

	// Token CRUD.
	CreateToken(ctx context.Context, t *Token) error
	GetToken(ctx context.Context, id string) (*Token, error)
	UpdateToken(ctx context.Context, t *Token) error
	UpdateTokenPosition(ctx context.Context, id string, x, y float64) error
	DeleteToken(ctx context.Context, id string) error
	ListTokens(ctx context.Context, mapID string, role int) ([]Token, error)

	// Layer CRUD.
	CreateLayer(ctx context.Context, l *Layer) error
	GetLayer(ctx context.Context, id string) (*Layer, error)
	UpdateLayer(ctx context.Context, l *Layer) error
	DeleteLayer(ctx context.Context, id string) error
	ListLayers(ctx context.Context, mapID string) ([]Layer, error)

	// Fog CRUD.
	CreateFog(ctx context.Context, f *FogRegion) error
	DeleteFog(ctx context.Context, id string) error
	ListFog(ctx context.Context, mapID string) ([]FogRegion, error)
	ResetFog(ctx context.Context, mapID string) error
}

// drawingRepo implements DrawingRepository with MariaDB.
type drawingRepo struct {
	db *sql.DB
}

// NewDrawingRepository creates a new drawing repository.
func NewDrawingRepository(db *sql.DB) DrawingRepository {
	return &drawingRepo{db: db}
}

// --- Drawing ---

// CreateDrawing inserts a new drawing.
func (r *drawingRepo) CreateDrawing(ctx context.Context, d *Drawing) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO map_drawings (id, map_id, layer_id, drawing_type, points,
			stroke_color, stroke_width, fill_color, fill_alpha, text_content,
			font_size, rotation, visibility, created_by, foundry_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.MapID, d.LayerID, d.DrawingType, d.Points,
		d.StrokeColor, d.StrokeWidth, d.FillColor, d.FillAlpha, d.TextContent,
		d.FontSize, d.Rotation, d.Visibility, d.CreatedBy, d.FoundryID,
	)
	if err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

// GetDrawing retrieves a drawing by ID.
func (r *drawingRepo) GetDrawing(ctx context.Context, id string) (*Drawing, error) {
	var d Drawing
	err := r.db.QueryRowContext(ctx, `
		SELECT id, map_id, layer_id, drawing_type, points, stroke_color,
			stroke_width, fill_color, fill_alpha, text_content, font_size,
			rotation, visibility, created_by, foundry_id, created_at, updated_at
		FROM map_drawings WHERE id = ?`, id,
	).Scan(
		&d.ID, &d.MapID, &d.LayerID, &d.DrawingType, &d.Points,
		&d.StrokeColor, &d.StrokeWidth, &d.FillColor, &d.FillAlpha,
		&d.TextContent, &d.FontSize, &d.Rotation, &d.Visibility,
		&d.CreatedBy, &d.FoundryID, &d.CreatedAt, &d.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, apperror.NewNotFound("drawing", id)
	}
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return &d, nil
}

// UpdateDrawing updates a drawing.
func (r *drawingRepo) UpdateDrawing(ctx context.Context, d *Drawing) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE map_drawings SET points = ?, stroke_color = ?, stroke_width = ?,
			fill_color = ?, fill_alpha = ?, text_content = ?, font_size = ?,
			rotation = ?, visibility = ?
		WHERE id = ?`,
		d.Points, d.StrokeColor, d.StrokeWidth, d.FillColor, d.FillAlpha,
		d.TextContent, d.FontSize, d.Rotation, d.Visibility, d.ID,
	)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return apperror.NewNotFound("drawing", d.ID)
	}
	return nil
}

// DeleteDrawing removes a drawing.
func (r *drawingRepo) DeleteDrawing(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM map_drawings WHERE id = ?`, id)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return apperror.NewNotFound("drawing", id)
	}
	return nil
}

// ListDrawings returns all drawings for a map, filtered by role visibility.
func (r *drawingRepo) ListDrawings(ctx context.Context, mapID string, role int) ([]Drawing, error) {
	query := `
		SELECT id, map_id, layer_id, drawing_type, points, stroke_color,
			stroke_width, fill_color, fill_alpha, text_content, font_size,
			rotation, visibility, created_by, foundry_id, created_at, updated_at
		FROM map_drawings WHERE map_id = ?`
	if role < 3 { // Non-owners don't see dm_only drawings.
		query += ` AND visibility != 'dm_only'`
	}
	query += ` ORDER BY created_at ASC`

	rows, err := r.db.QueryContext(ctx, query, mapID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	defer rows.Close()

	var drawings []Drawing
	for rows.Next() {
		var d Drawing
		err := rows.Scan(
			&d.ID, &d.MapID, &d.LayerID, &d.DrawingType, &d.Points,
			&d.StrokeColor, &d.StrokeWidth, &d.FillColor, &d.FillAlpha,
			&d.TextContent, &d.FontSize, &d.Rotation, &d.Visibility,
			&d.CreatedBy, &d.FoundryID, &d.CreatedAt, &d.UpdatedAt,
		)
		if err != nil {
			return nil, apperror.NewInternal(err)
		}
		drawings = append(drawings, d)
	}
	return drawings, nil
}

// --- Token ---

// CreateToken inserts a new token.
func (r *drawingRepo) CreateToken(ctx context.Context, t *Token) error {
	statusJSON, _ := json.Marshal(t.StatusEffects)
	flagsJSON, _ := json.Marshal(t.Flags)
	if len(statusJSON) == 0 {
		statusJSON = []byte("null")
	}
	if len(flagsJSON) == 0 {
		flagsJSON = []byte("null")
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO map_tokens (id, map_id, layer_id, entity_id, name, image_path,
			x, y, width, height, rotation, scale, is_hidden, is_locked,
			bar1_value, bar1_max, bar2_value, bar2_max,
			aura_radius, aura_color, light_radius, light_dim_radius, light_color,
			vision_enabled, vision_range, elevation, sort_order,
			status_effects, flags, foundry_id, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.MapID, t.LayerID, t.EntityID, t.Name, t.ImagePath,
		t.X, t.Y, t.Width, t.Height, t.Rotation, t.Scale, t.IsHidden, t.IsLocked,
		t.Bar1Value, t.Bar1Max, t.Bar2Value, t.Bar2Max,
		t.AuraRadius, t.AuraColor, t.LightRadius, t.LightDimRadius, t.LightColor,
		t.VisionEnabled, t.VisionRange, t.Elevation, t.SortOrder,
		statusJSON, flagsJSON, t.FoundryID, t.CreatedBy,
	)
	if err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

// GetToken retrieves a token by ID.
func (r *drawingRepo) GetToken(ctx context.Context, id string) (*Token, error) {
	var t Token
	var statusJSON, flagsJSON []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT id, map_id, layer_id, entity_id, name, image_path,
			x, y, width, height, rotation, scale, is_hidden, is_locked,
			bar1_value, bar1_max, bar2_value, bar2_max,
			aura_radius, aura_color, light_radius, light_dim_radius, light_color,
			vision_enabled, vision_range, elevation, sort_order,
			status_effects, flags, foundry_id, created_by, created_at, updated_at
		FROM map_tokens WHERE id = ?`, id,
	).Scan(
		&t.ID, &t.MapID, &t.LayerID, &t.EntityID, &t.Name, &t.ImagePath,
		&t.X, &t.Y, &t.Width, &t.Height, &t.Rotation, &t.Scale, &t.IsHidden, &t.IsLocked,
		&t.Bar1Value, &t.Bar1Max, &t.Bar2Value, &t.Bar2Max,
		&t.AuraRadius, &t.AuraColor, &t.LightRadius, &t.LightDimRadius, &t.LightColor,
		&t.VisionEnabled, &t.VisionRange, &t.Elevation, &t.SortOrder,
		&statusJSON, &flagsJSON, &t.FoundryID, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, apperror.NewNotFound("token", id)
	}
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	t.StatusEffects = statusJSON
	t.Flags = flagsJSON
	return &t, nil
}

// UpdateToken updates a token's full state.
func (r *drawingRepo) UpdateToken(ctx context.Context, t *Token) error {
	statusJSON, _ := json.Marshal(t.StatusEffects)
	flagsJSON, _ := json.Marshal(t.Flags)

	result, err := r.db.ExecContext(ctx, `
		UPDATE map_tokens SET name = ?, image_path = ?, x = ?, y = ?,
			width = ?, height = ?, rotation = ?, scale = ?,
			is_hidden = ?, is_locked = ?,
			bar1_value = ?, bar1_max = ?, bar2_value = ?, bar2_max = ?,
			aura_radius = ?, aura_color = ?,
			light_radius = ?, light_dim_radius = ?, light_color = ?,
			vision_enabled = ?, vision_range = ?, elevation = ?,
			status_effects = ?, flags = ?
		WHERE id = ?`,
		t.Name, t.ImagePath, t.X, t.Y,
		t.Width, t.Height, t.Rotation, t.Scale,
		t.IsHidden, t.IsLocked,
		t.Bar1Value, t.Bar1Max, t.Bar2Value, t.Bar2Max,
		t.AuraRadius, t.AuraColor,
		t.LightRadius, t.LightDimRadius, t.LightColor,
		t.VisionEnabled, t.VisionRange, t.Elevation,
		statusJSON, flagsJSON, t.ID,
	)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return apperror.NewNotFound("token", t.ID)
	}
	return nil
}

// UpdateTokenPosition updates only a token's x,y coordinates.
func (r *drawingRepo) UpdateTokenPosition(ctx context.Context, id string, x, y float64) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE map_tokens SET x = ?, y = ? WHERE id = ?`, x, y, id)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return apperror.NewNotFound("token", id)
	}
	return nil
}

// DeleteToken removes a token.
func (r *drawingRepo) DeleteToken(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM map_tokens WHERE id = ?`, id)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return apperror.NewNotFound("token", id)
	}
	return nil
}

// ListTokens returns all tokens for a map, filtered by role.
func (r *drawingRepo) ListTokens(ctx context.Context, mapID string, role int) ([]Token, error) {
	query := `
		SELECT id, map_id, layer_id, entity_id, name, image_path,
			x, y, width, height, rotation, scale, is_hidden, is_locked,
			bar1_value, bar1_max, bar2_value, bar2_max,
			aura_radius, aura_color, light_radius, light_dim_radius, light_color,
			vision_enabled, vision_range, elevation, sort_order,
			status_effects, flags, foundry_id, created_by, created_at, updated_at
		FROM map_tokens WHERE map_id = ?`
	if role < 3 { // Non-owners don't see hidden tokens.
		query += ` AND is_hidden = FALSE`
	}
	query += ` ORDER BY sort_order ASC, created_at ASC`

	rows, err := r.db.QueryContext(ctx, query, mapID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	defer rows.Close()

	var tokens []Token
	for rows.Next() {
		var t Token
		var statusJSON, flagsJSON []byte
		err := rows.Scan(
			&t.ID, &t.MapID, &t.LayerID, &t.EntityID, &t.Name, &t.ImagePath,
			&t.X, &t.Y, &t.Width, &t.Height, &t.Rotation, &t.Scale, &t.IsHidden, &t.IsLocked,
			&t.Bar1Value, &t.Bar1Max, &t.Bar2Value, &t.Bar2Max,
			&t.AuraRadius, &t.AuraColor, &t.LightRadius, &t.LightDimRadius, &t.LightColor,
			&t.VisionEnabled, &t.VisionRange, &t.Elevation, &t.SortOrder,
			&statusJSON, &flagsJSON, &t.FoundryID, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt,
		)
		if err != nil {
			return nil, apperror.NewInternal(err)
		}
		t.StatusEffects = statusJSON
		t.Flags = flagsJSON
		tokens = append(tokens, t)
	}
	return tokens, nil
}

// --- Layer ---

// CreateLayer inserts a new layer.
func (r *drawingRepo) CreateLayer(ctx context.Context, l *Layer) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO map_layers (id, map_id, name, layer_type, sort_order,
			is_visible, opacity, is_locked)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		l.ID, l.MapID, l.Name, l.LayerType, l.SortOrder,
		l.IsVisible, l.Opacity, l.IsLocked,
	)
	if err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

// GetLayer retrieves a layer by ID.
func (r *drawingRepo) GetLayer(ctx context.Context, id string) (*Layer, error) {
	var l Layer
	err := r.db.QueryRowContext(ctx, `
		SELECT id, map_id, name, layer_type, sort_order, is_visible, opacity,
			is_locked, created_at
		FROM map_layers WHERE id = ?`, id,
	).Scan(&l.ID, &l.MapID, &l.Name, &l.LayerType, &l.SortOrder,
		&l.IsVisible, &l.Opacity, &l.IsLocked, &l.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, apperror.NewNotFound("layer", id)
	}
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return &l, nil
}

// UpdateLayer updates a layer.
func (r *drawingRepo) UpdateLayer(ctx context.Context, l *Layer) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE map_layers SET name = ?, sort_order = ?, is_visible = ?,
			opacity = ?, is_locked = ?
		WHERE id = ?`,
		l.Name, l.SortOrder, l.IsVisible, l.Opacity, l.IsLocked, l.ID,
	)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return apperror.NewNotFound("layer", l.ID)
	}
	return nil
}

// DeleteLayer removes a layer (drawings/tokens on this layer get NULL layer_id).
func (r *drawingRepo) DeleteLayer(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM map_layers WHERE id = ?`, id)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return apperror.NewNotFound("layer", id)
	}
	return nil
}

// ListLayers returns all layers for a map ordered by sort_order.
func (r *drawingRepo) ListLayers(ctx context.Context, mapID string) ([]Layer, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, map_id, name, layer_type, sort_order, is_visible, opacity,
			is_locked, created_at
		FROM map_layers WHERE map_id = ?
		ORDER BY sort_order ASC`, mapID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	defer rows.Close()

	var layers []Layer
	for rows.Next() {
		var l Layer
		err := rows.Scan(&l.ID, &l.MapID, &l.Name, &l.LayerType, &l.SortOrder,
			&l.IsVisible, &l.Opacity, &l.IsLocked, &l.CreatedAt)
		if err != nil {
			return nil, apperror.NewInternal(err)
		}
		layers = append(layers, l)
	}
	return layers, nil
}

// --- Fog ---

// CreateFog inserts a new fog of war region.
func (r *drawingRepo) CreateFog(ctx context.Context, f *FogRegion) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO map_fog (id, map_id, points, is_explored)
		VALUES (?, ?, ?, ?)`,
		f.ID, f.MapID, f.Points, f.IsExplored,
	)
	if err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

// DeleteFog removes a fog region.
func (r *drawingRepo) DeleteFog(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM map_fog WHERE id = ?`, id)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return apperror.NewNotFound("fog region", id)
	}
	return nil
}

// ListFog returns all fog regions for a map.
func (r *drawingRepo) ListFog(ctx context.Context, mapID string) ([]FogRegion, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, map_id, points, is_explored, created_at
		FROM map_fog WHERE map_id = ?
		ORDER BY created_at ASC`, mapID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	defer rows.Close()

	var regions []FogRegion
	for rows.Next() {
		var f FogRegion
		err := rows.Scan(&f.ID, &f.MapID, &f.Points, &f.IsExplored, &f.CreatedAt)
		if err != nil {
			return nil, apperror.NewInternal(err)
		}
		regions = append(regions, f)
	}
	return regions, nil
}

// ResetFog removes all fog regions for a map.
func (r *drawingRepo) ResetFog(ctx context.Context, mapID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM map_fog WHERE map_id = ?`, mapID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}
