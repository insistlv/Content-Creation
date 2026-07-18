package app

import (
	"path/filepath"
	"testing"
)

func TestEnsureCustomerServiceConfigColumnBackfillsExistingSettingsTable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "app.db")
	db := openTestSQLiteDB(t, dbPath)
	if err := db.Exec(`
		CREATE TABLE app_settings (
			id integer primary key autoincrement,
			default_model text,
			default_prompt text,
			created_at datetime,
			updated_at datetime
		)
	`).Error; err != nil {
		t.Fatalf("create legacy app_settings table: %v", err)
	}

	app := &App{db: db}
	if err := app.ensureCustomerServiceConfigColumn(); err != nil {
		t.Fatalf("ensureCustomerServiceConfigColumn() error = %v", err)
	}

	if !db.Migrator().HasColumn(&AppSettings{}, "CustomerServiceConfigJSON") {
		t.Fatal("expected migrated customer service config column")
	}
}
