package app

import (
	"context"
	"path/filepath"
	"testing"
)

func TestMigrateImageGenerationQueueSchemaRequiresIdleLegacyTasks(t *testing.T) {
	db := openTestSQLiteDB(t, filepath.Join(t.TempDir(), "migration-active.db"))
	if err := db.AutoMigrate(&GenerationRecord{}, &VideoGenerationRecord{}, &AppSettings{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&AppSettings{ID: 1, GenerationConcurrencyLimit: 4}).Error; err != nil {
		t.Fatal(err)
	}
	record := GenerationRecord{UserID: 1, Status: GenerationStatusRunning, Stage: GenerationStageRequestingProvider}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	_, err := MigrateImageGenerationQueueSchema(context.Background(), db)
	if !IsActiveImageGenerationMigrationError(err) {
		t.Fatalf("expected active image migration guard, got %v", err)
	}
	if db.Migrator().HasTable(&ImageGenerationJob{}) {
		t.Fatal("queue table must not be created while legacy image tasks are active")
	}
}

func TestMigrateImageGenerationQueueSchemaCreatesQueueAndStartsAtTwo(t *testing.T) {
	db := openTestSQLiteDB(t, filepath.Join(t.TempDir(), "migration-idle.db"))
	if err := db.AutoMigrate(&GenerationRecord{}, &VideoGenerationRecord{}, &AppSettings{}); err != nil {
		t.Fatal(err)
	}
	settings := AppSettings{ID: 1, GenerationConcurrencyLimit: 4}
	if err := db.Create(&settings).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE model_channels (id integer primary key)").Error; err != nil {
		t.Fatal(err)
	}
	videoRecord := GenerationRecord{UserID: 1, Status: GenerationStatusRunning, Stage: GenerationStageRequestingProvider}
	if err := db.Create(&videoRecord).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&VideoGenerationRecord{GenerationRecordID: videoRecord.ID, UserID: 1, Status: GenerationStatusRunning}).Error; err != nil {
		t.Fatal(err)
	}
	report, err := MigrateImageGenerationQueueSchema(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if report.ActiveImageGenerations != 0 || report.QueueTableExisted || report.ConcurrencyLimit != 2 {
		t.Fatalf("unexpected migration report: %+v", report)
	}
	if !db.Migrator().HasTable(&ImageGenerationJob{}) || !db.Migrator().HasTable(&ImageExecutionLease{}) {
		t.Fatal("durable image queue tables were not created")
	}
	if !db.Migrator().HasColumn(&ModelChannel{}, "ConsecutiveFailureCount") {
		t.Fatal("existing model channel table must receive the circuit breaker counter column")
	}
	if err := db.First(&settings, 1).Error; err != nil {
		t.Fatal(err)
	}
	if settings.GenerationConcurrencyLimit != 2 {
		t.Fatalf("expected first rollout concurrency 2, got %d", settings.GenerationConcurrencyLimit)
	}
	report, err = MigrateImageGenerationQueueSchema(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if !report.QueueTableExisted || report.ConcurrencyLimit != 2 {
		t.Fatalf("repeat migration must preserve configured concurrency: %+v", report)
	}
}
