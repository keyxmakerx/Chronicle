package bestiary

import (
	"context"
	"io/fs"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/database/dbtest"
)

// TestGetBestiaryStats_EmptyTable pins the fresh-site case: SUM over zero rows
// is NULL in SQL, and scanning NULL into an int made the admin stats endpoint
// answer 500 until the first creature was published.
func TestGetBestiaryStats_EmptyTable(t *testing.T) {
	sub, err := fs.Sub(MigrationsFS, database.PluginMigrationsSubdir)
	if err != nil {
		t.Fatalf("sub-FS: %v", err)
	}
	db := dbtest.Open(t, dbtest.Options{
		Prefix:           "chronicle_bst_",
		RequireDSN:       true,
		Plugins:          []dbtest.PluginMigrations{{Slug: "bestiary", FS: sub}},
		StrictMigrations: true,
	})

	stats, err := NewBestiaryRepository(db).GetBestiaryStats(context.Background())
	if err != nil {
		t.Fatalf("GetBestiaryStats on an empty table: %v", err)
	}
	if stats.TotalPublications != 0 || stats.PublishedCount != 0 || stats.FlaggedCount != 0 {
		t.Errorf("want all zero counts, got %+v", *stats)
	}
}
