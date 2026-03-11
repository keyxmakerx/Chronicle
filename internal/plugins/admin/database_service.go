// Package admin — database_service.go implements schema introspection
// and migration status for the admin database explorer page.
// Queries MariaDB information_schema for table/column/FK metadata and
// uses the plugin migration system to show pending migrations.
package admin

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/database"
)

// DatabaseExplorer provides schema introspection and migration status
// for the admin database explorer page.
type DatabaseExplorer interface {
	// GetSchema returns all tables, columns, and foreign key relationships.
	GetSchema(ctx context.Context) (*DatabaseSchema, error)

	// GetMigrationStatus returns the migration status for each plugin.
	GetMigrationStatus(ctx context.Context) ([]PluginMigrationStatus, error)

	// ApplyPendingMigrations runs all pending plugin migrations and returns results.
	ApplyPendingMigrations(ctx context.Context) ([]database.PluginMigrationResult, error)
}

// DatabaseSchema holds the full schema graph for visualization.
type DatabaseSchema struct {
	Tables      []TableInfo      `json:"tables"`
	ForeignKeys []ForeignKeyInfo `json:"foreignKeys"`
}

// TableInfo describes a single database table.
type TableInfo struct {
	Name        string       `json:"name"`
	RowCount    int64        `json:"rowCount"`
	DataSizeKB  int64        `json:"dataSizeKB"`
	ColumnCount int          `json:"columnCount"`
	Columns     []ColumnInfo `json:"columns"`
	Plugin      string       `json:"plugin"`
}

// ColumnInfo describes a single table column.
type ColumnInfo struct {
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Nullable bool    `json:"nullable"`
	Default  *string `json:"default"`
	Key      string  `json:"key"` // PRI, MUL, UNI, or ""
}

// ForeignKeyInfo describes a foreign key relationship between two tables.
type ForeignKeyInfo struct {
	FromTable  string `json:"fromTable"`
	FromColumn string `json:"fromColumn"`
	ToTable    string `json:"toTable"`
	ToColumn   string `json:"toColumn"`
}

// PluginMigrationStatus describes the migration state of a single plugin.
type PluginMigrationStatus struct {
	Slug           string             `json:"slug"`
	CurrentVersion int                `json:"currentVersion"`
	LatestVersion  int                `json:"latestVersion"`
	Pending        int                `json:"pending"`
	Healthy        bool               `json:"healthy"`
	Error          string             `json:"error,omitempty"`
	History        []MigrationHistory `json:"history,omitempty"`
}

// MigrationHistory records when a specific migration version was applied.
type MigrationHistory struct {
	Version   int    `json:"version"`
	AppliedAt string `json:"appliedAt"` // RFC3339
}

// databaseExplorer implements DatabaseExplorer with direct DB access.
type databaseExplorer struct {
	db           *sql.DB
	pluginHealth *database.PluginHealthRegistry
}

// NewDatabaseExplorer creates a new database explorer service.
func NewDatabaseExplorer(db *sql.DB, pluginHealth *database.PluginHealthRegistry) DatabaseExplorer {
	return &databaseExplorer{
		db:           db,
		pluginHealth: pluginHealth,
	}
}

// pluginTablePrefixes maps table name prefixes to plugin slugs.
// Tables that don't match any prefix are classified as "core".
var pluginTablePrefixes = []struct {
	prefix string
	slug   string
}{
	{"calendar_", "calendar"},
	{"map_", "maps"},
	{"maps_", "maps"},
	{"session", "sessions"},
	{"timeline_", "timeline"},
	{"sync_api_", "syncapi"},
	{"ext_", "extensions"},
	{"extension_", "extensions"},
	{"plugin_schema_", "system"},
}

// classifyTable returns the plugin slug that owns a table based on its name.
func classifyTable(name string) string {
	for _, p := range pluginTablePrefixes {
		if strings.HasPrefix(name, p.prefix) {
			return p.slug
		}
	}
	return "core"
}

// GetSchema queries information_schema to build the full schema graph.
func (e *databaseExplorer) GetSchema(ctx context.Context) (*DatabaseSchema, error) {
	schema := &DatabaseSchema{}

	// 1. Get all tables with row counts and sizes.
	tableRows, err := e.db.QueryContext(ctx, `
		SELECT TABLE_NAME, TABLE_ROWS, COALESCE(DATA_LENGTH, 0)
		FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_TYPE = 'BASE TABLE'
		ORDER BY TABLE_NAME
	`)
	if err != nil {
		return nil, fmt.Errorf("querying tables: %w", err)
	}
	defer tableRows.Close()

	tableMap := make(map[string]*TableInfo)
	for tableRows.Next() {
		var t TableInfo
		var dataBytes int64
		if err := tableRows.Scan(&t.Name, &t.RowCount, &dataBytes); err != nil {
			return nil, fmt.Errorf("scanning table row: %w", err)
		}
		t.DataSizeKB = dataBytes / 1024
		t.Plugin = classifyTable(t.Name)
		tableMap[t.Name] = &t
		schema.Tables = append(schema.Tables, t)
	}
	if err := tableRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating tables: %w", err)
	}

	// 2. Get all columns for each table.
	colRows, err := e.db.QueryContext(ctx, `
		SELECT TABLE_NAME, COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_DEFAULT, COLUMN_KEY
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE()
		ORDER BY TABLE_NAME, ORDINAL_POSITION
	`)
	if err != nil {
		return nil, fmt.Errorf("querying columns: %w", err)
	}
	defer colRows.Close()

	// Build column lists per table, then assign to schema.Tables.
	tableCols := make(map[string][]ColumnInfo)
	for colRows.Next() {
		var tableName, colName, colType, nullable, key string
		var colDefault *string
		if err := colRows.Scan(&tableName, &colName, &colType, &nullable, &colDefault, &key); err != nil {
			return nil, fmt.Errorf("scanning column row: %w", err)
		}
		tableCols[tableName] = append(tableCols[tableName], ColumnInfo{
			Name:     colName,
			Type:     colType,
			Nullable: nullable == "YES",
			Default:  colDefault,
			Key:      key,
		})
	}
	if err := colRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating columns: %w", err)
	}

	// Assign columns and column counts to tables.
	for i := range schema.Tables {
		t := &schema.Tables[i]
		t.Columns = tableCols[t.Name]
		t.ColumnCount = len(t.Columns)
	}

	// 3. Get foreign key relationships.
	fkRows, err := e.db.QueryContext(ctx, `
		SELECT TABLE_NAME, COLUMN_NAME, REFERENCED_TABLE_NAME, REFERENCED_COLUMN_NAME
		FROM information_schema.KEY_COLUMN_USAGE
		WHERE TABLE_SCHEMA = DATABASE()
		  AND REFERENCED_TABLE_NAME IS NOT NULL
		ORDER BY TABLE_NAME, COLUMN_NAME
	`)
	if err != nil {
		return nil, fmt.Errorf("querying foreign keys: %w", err)
	}
	defer fkRows.Close()

	for fkRows.Next() {
		var fk ForeignKeyInfo
		if err := fkRows.Scan(&fk.FromTable, &fk.FromColumn, &fk.ToTable, &fk.ToColumn); err != nil {
			return nil, fmt.Errorf("scanning FK row: %w", err)
		}
		schema.ForeignKeys = append(schema.ForeignKeys, fk)
	}
	if err := fkRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating foreign keys: %w", err)
	}

	return schema, nil
}

// GetMigrationStatus computes the migration status for each registered plugin.
func (e *databaseExplorer) GetMigrationStatus(ctx context.Context) ([]PluginMigrationStatus, error) {
	plugins := database.RegisteredPlugins()
	statuses := make([]PluginMigrationStatus, 0, len(plugins))

	for _, p := range plugins {
		status := PluginMigrationStatus{
			Slug:    p.Slug,
			Healthy: true,
		}

		// Get health and version info from registry (populated at startup
		// when migration files are scanned — avoids re-reading disk at runtime).
		if h := e.pluginHealth.Get(p.Slug); h != nil {
			status.Healthy = h.Healthy
			status.CurrentVersion = h.Version
			status.LatestVersion = h.LatestVersion
			status.Error = h.Error
		}
		status.Pending = status.LatestVersion - status.CurrentVersion
		if status.Pending < 0 {
			status.Pending = 0
		}

		// Fetch applied migration timestamps.
		history, err := e.getMigrationHistory(ctx, p.Slug)
		if err != nil {
			slog.Warn("failed to read migration history",
				slog.String("plugin", p.Slug),
				slog.Any("error", err),
			)
		}
		status.History = history

		statuses = append(statuses, status)
	}

	return statuses, nil
}

// getMigrationHistory returns the applied migration versions and timestamps for a plugin.
func (e *databaseExplorer) getMigrationHistory(ctx context.Context, slug string) ([]MigrationHistory, error) {
	rows, err := e.db.QueryContext(ctx,
		`SELECT version, applied_at FROM plugin_schema_versions WHERE plugin_slug = ? ORDER BY version`,
		slug,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []MigrationHistory
	for rows.Next() {
		var h MigrationHistory
		var appliedAt time.Time
		if err := rows.Scan(&h.Version, &appliedAt); err != nil {
			return nil, err
		}
		h.AppliedAt = appliedAt.Format(time.RFC3339)
		history = append(history, h)
	}
	return history, rows.Err()
}

// ApplyPendingMigrations runs all pending plugin migrations and updates the
// health registry. Returns the results for each plugin.
func (e *databaseExplorer) ApplyPendingMigrations(ctx context.Context) ([]database.PluginMigrationResult, error) {
	plugins := database.RegisteredPlugins()
	results := database.RunPluginMigrations(e.db, plugins)

	// Update the health registry with new results.
	for _, r := range results {
		e.pluginHealth.Register(r.Slug, r.Healthy, r.Error, r.Version, r.LatestVersion)
	}

	return results, nil
}
