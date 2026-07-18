package ecommerce

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPostgresQueueConcurrentClaimNoDuplicates(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("CI") == "true" {
			t.Fatal("TEST_DATABASE_URL is required in CI")
		}
		t.Skip("TEST_DATABASE_URL is not set")
	}
	adminDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	schema := fmt.Sprintf("commerce_queue_%d", time.Now().UnixNano())
	if err := adminDB.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatalf("create isolated schema: %v", err)
	}
	t.Cleanup(func() { _ = adminDB.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE").Error })
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	db, err := gorm.Open(postgres.Open(parsed.String()), &gorm.Config{})
	if err != nil {
		t.Fatalf("open isolated PostgreSQL schema: %v", err)
	}
	if err := ApplyFoundationMigrations(context.Background(), db); err != nil {
		t.Fatalf("apply foundation migration: %v", err)
	}
	prefix := fmt.Sprintf("task5-pg-%d", time.Now().UnixNano())
	product := CommerceProduct{UserID: 91001, Name: prefix, Status: "active"}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}
	project := CommerceProject{UserID: product.UserID, ProductID: product.ID, Title: prefix, Pipeline: "general", Status: "active"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	batch := CommerceGenerationBatch{UserID: project.UserID, ProjectID: project.ID, PrimarySKUID: 1, Pipeline: "general", RecipeKey: "poster", RecipeVersion: 1, Status: CommerceBatchQueued, IdempotencyKey: prefix + ":batch", TotalItems: 10, QueuedItems: 10}
	if err := db.Create(&batch).Error; err != nil {
		t.Fatalf("create batch: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Where("project_id = ?", project.ID).Delete(&CommerceEvent{}).Error
		_ = db.Where("project_id = ?", project.ID).Delete(&CommerceJob{}).Error
		_ = db.Where("project_id = ?", project.ID).Delete(&CommerceGenerationItem{}).Error
		_ = db.Where("project_id = ?", project.ID).Delete(&CommerceGenerationBatch{}).Error
		_ = db.Unscoped().Where("id = ? AND title LIKE ?", project.ID, prefix+"%").Delete(&CommerceProject{}).Error
		_ = db.Unscoped().Where("id = ? AND name LIKE ?", product.ID, prefix+"%").Delete(&CommerceProduct{}).Error
	})
	compiled, err := EncodeJSON(CompiledGenerationItem{SKUID: 1, Pipeline: "general", RecipeKey: "poster", RecipeVersion: 1})
	if err != nil {
		t.Fatalf("encode item: %v", err)
	}
	for index := 0; index < 10; index++ {
		item := CommerceGenerationItem{UserID: project.UserID, ProjectID: project.ID, BatchID: batch.ID, ReservationID: 1, SKUID: 1, Pipeline: "general", RecipeKey: "poster", RecipeVersion: 1, Status: CommerceItemQueued, IdempotencyKey: fmt.Sprintf("%s:item:%d", prefix, index), OutputSpecJSON: compiled}
		if err := db.Create(&item).Error; err != nil {
			t.Fatalf("create item %d: %v", index, err)
		}
		batchID, itemID := batch.ID, item.ID
		job := CommerceJob{UserID: project.UserID, ProjectID: project.ID, BatchID: &batchID, GenerationItemID: &itemID, Kind: CommerceJobKindGenerateItem, Pipeline: "general", RecipeKey: "poster", Status: CommerceJobQueued, IdempotencyKey: fmt.Sprintf("%s:job:%d", prefix, index), MaxAttempts: 3}
		if err := db.Create(&job).Error; err != nil {
			t.Fatalf("create job %d: %v", index, err)
		}
	}

	start := make(chan struct{})
	results := make(chan []JobSnapshot, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, workerID := range []string{"postgres-worker-a", "postgres-worker-b"} {
		workerID := workerID
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			claimed, claimErr := NewQueue(db, nil, workerID).Claim(context.Background(), 5, time.Minute)
			results <- claimed
			errs <- claimErr
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	for claimErr := range errs {
		if claimErr != nil {
			t.Fatalf("concurrent Claim: %v", claimErr)
		}
	}
	seen := make(map[uint]string, 10)
	for claimed := range results {
		if len(claimed) != 5 {
			t.Fatalf("worker claimed %d jobs, want 5", len(claimed))
		}
		for _, snapshot := range claimed {
			if previous, duplicate := seen[snapshot.Job.ID]; duplicate {
				t.Fatalf("job %d claimed twice by %s and %s", snapshot.Job.ID, previous, snapshot.Job.LeaseOwner)
			}
			seen[snapshot.Job.ID] = snapshot.Job.LeaseOwner
		}
	}
	if len(seen) != 10 {
		t.Fatalf("unique claimed jobs = %d, want 10", len(seen))
	}
}

func TestPostgresSKUMatrixDownMigrationIsGuardedAndRepeatable(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("CI") == "true" {
			t.Fatal("TEST_DATABASE_URL is required in CI")
		}
		t.Skip("TEST_DATABASE_URL is not set")
	}
	adminDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("commerce_sku_down_%d", time.Now().UnixNano())
	if err := adminDB.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = adminDB.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE").Error })
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	db, err := gorm.Open(postgres.Open(parsed.String()), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyFoundationMigrations(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	product := CommerceProduct{UserID: 1, Name: "rollback guard", Status: "active"}
	if err := db.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&CommerceSKUDimension{UserID: 1, ProductID: product.ID, Name: "color", Status: "active"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(skuMatrixDownSQL).Error; err == nil || !strings.Contains(err.Error(), "contains business data") {
		t.Fatalf("guarded down error=%v", err)
	}
	if err := db.Exec("DELETE FROM commerce_sku_dimensions").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(skuMatrixDownSQL).Error; err != nil {
		t.Fatalf("empty down: %v", err)
	}
	if err := db.Exec(skuMatrixDownSQL).Error; err != nil {
		t.Fatalf("repeat down: %v", err)
	}
	if db.Migrator().HasTable("commerce_sku_dimensions") || db.Migrator().HasColumn("commerce_products", "sku_version") {
		t.Fatal("SKU matrix down migration left schema artifacts")
	}
}
