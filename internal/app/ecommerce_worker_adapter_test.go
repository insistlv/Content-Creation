package app

import (
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCommerceWorkerLifecycleStartsOnlyWhenEnabledAndStopsOnClose(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "worker.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	cfg := commerceWorkerTestConfig(t)
	cfg.StartupDatabaseMigrations = StartupDatabaseMigrationsBootstrap
	cfg.StartupDatabaseBootstrap = true
	application, err := NewWithDependencies(cfg, db, &stubProvider{})
	if err != nil {
		t.Fatalf("NewWithDependencies: %v", err)
	}
	if application.commerceWorker == nil || !application.commerceWorker.Running() {
		t.Fatal("commerce worker did not start")
	}
	worker := application.commerceWorker
	if err := application.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if worker.Running() {
		t.Fatal("commerce worker still running after App.Close")
	}
}

func TestCommerceWorkerLifecycleFailsFastWhenSchemaMissing(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "missing.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	cfg := commerceWorkerTestConfig(t)
	cfg.StartupDatabaseMigrations = StartupDatabaseMigrationsSkip
	cfg.StartupDatabaseBootstrap = false
	_, err = NewWithDependencies(cfg, db, &stubProvider{})
	if err == nil || !strings.Contains(err.Error(), "commerce worker schema readiness") {
		t.Fatalf("missing schema error = %v", err)
	}
	sqlDB, sqlErr := db.DB()
	if sqlErr != nil {
		t.Fatalf("db.DB: %v", sqlErr)
	}
	if closeErr := sqlDB.Close(); closeErr != nil {
		t.Fatalf("close database: %v", closeErr)
	}
}

func TestCommerceWorkerLifecycleFailsFastWhenRequiredJobColumnMissing(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "missing-worker-column.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	bootstrapCfg := testConfig(t)
	bootstrapCfg.StartupDatabaseMigrations = StartupDatabaseMigrationsBootstrap
	bootstrapCfg.StartupDatabaseBootstrap = true
	bootstrapApp, err := NewWithDependencies(bootstrapCfg, db, &stubProvider{})
	if err != nil {
		t.Fatalf("bootstrap schema: %v", err)
	}
	bootstrapApp.cleanupStopOnce.Do(func() { close(bootstrapApp.cleanupStop) })
	if err := db.Exec("ALTER TABLE commerce_jobs RENAME COLUMN heartbeat_at TO missing_heartbeat_at").Error; err != nil {
		t.Fatalf("rename heartbeat column: %v", err)
	}
	cfg := commerceWorkerTestConfig(t)
	cfg.StartupDatabaseMigrations = StartupDatabaseMigrationsSkip
	cfg.StartupDatabaseBootstrap = false
	_, err = NewWithDependencies(cfg, db, &stubProvider{})
	if err == nil || !strings.Contains(err.Error(), "commerce worker schema readiness") || !strings.Contains(err.Error(), "heartbeat_at") {
		t.Fatalf("missing worker column error = %v", err)
	}
	sqlDB, sqlErr := db.DB()
	if sqlErr != nil {
		t.Fatalf("db.DB: %v", sqlErr)
	}
	if closeErr := sqlDB.Close(); closeErr != nil {
		t.Fatalf("close database: %v", closeErr)
	}
}

func commerceWorkerTestConfig(t *testing.T) Config {
	t.Helper()
	cfg := testConfig(t)
	cfg.AICommerceEnabled = true
	cfg.AICommerceWorkerEnabled = true
	cfg.AICommercePrivateStorageType = "oss"
	cfg.AICommerceOSSEndpoint = "https://oss.example.com"
	cfg.AICommerceOSSAccessKeyID = "test-access-key"
	cfg.AICommerceOSSAccessKeySecret = "test-secret"
	cfg.AICommerceOSSBucket = "test-bucket"
	cfg.AICommerceOSSBasePath = "commerce/"
	return cfg
}
