package app

import (
	"path/filepath"
	"testing"
)

func TestEnsureModelConfigAPIColumnsBackfillsExistingTable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "app.db")
	db := openTestSQLiteDB(t, dbPath)
	if err := db.Exec(`
		CREATE TABLE model_configs (
			id integer primary key autoincrement,
			name text,
			type text,
			provider text,
			status text,
			priority integer,
			cost_label text,
			permission text,
			weight integer,
			sort_order integer,
			runtime_model text,
			created_at datetime,
			updated_at datetime
		)
	`).Error; err != nil {
		t.Fatalf("create legacy model_configs table: %v", err)
	}

	app := &App{db: db}
	if err := app.ensureModelConfigAPIColumns(); err != nil {
		t.Fatalf("ensureModelConfigAPIColumns() error = %v", err)
	}

	for _, field := range []string{"APIBaseURL", "APIEndpoint", "APIKey"} {
		if !db.Migrator().HasColumn(&ModelConfig{}, field) {
			t.Fatalf("expected migrated column for %s", field)
		}
	}
}
