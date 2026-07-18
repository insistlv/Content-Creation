package ecommerce

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type fakeReferenceAssetOwnershipResolver func(context.Context, uint, uint) (bool, error)

func (f fakeReferenceAssetOwnershipResolver) OwnsReferenceAsset(ctx context.Context, userID, assetID uint) (bool, error) {
	return f(ctx, userID, assetID)
}

func newCommerceServiceTest(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := MigrateSQLiteFoundationSchema(context.Background(), db); err != nil {
		t.Fatalf("migrate foundation schema: %v", err)
	}
	return NewService(NewRepository(db)), db
}

func TestProjectServiceOwnership(t *testing.T) {
	ctx := context.Background()
	service, _ := newCommerceServiceTest(t)

	product, err := service.CreateProduct(ctx, 1, CreateProductInput{Name: "Lamp", Category: "home"})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	sku, err := service.CreateSKU(ctx, 1, product.ID, CreateSKUInput{Code: "LAMP-WHITE"})
	if err != nil {
		t.Fatalf("CreateSKU: %v", err)
	}
	project, err := service.CreateProject(ctx, 1, CreateProjectInput{
		ProductID:    product.ID,
		DefaultSKUID: &sku.ID,
		Title:        "Listing",
		Pipeline:     "general",
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	if _, err := service.GetProject(ctx, 2, project.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user GetProject error = %v, want ErrNotFound", err)
	}

	otherProduct, err := service.CreateProduct(ctx, 1, CreateProductInput{Name: "Chair"})
	if err != nil {
		t.Fatalf("CreateProduct other: %v", err)
	}
	if _, err := service.CreateProject(ctx, 1, CreateProjectInput{
		ProductID:    otherProduct.ID,
		DefaultSKUID: &sku.ID,
		Title:        "Mismatch",
		Pipeline:     "general",
	}); !errors.Is(err, ErrOwnershipMismatch) {
		t.Fatalf("mismatched default SKU error = %v, want ErrOwnershipMismatch", err)
	}
}

func TestOwnershipAndProjectConstraints(t *testing.T) {
	ctx := context.Background()
	service, _ := newCommerceServiceTest(t)
	product, err := service.CreateProduct(ctx, 7, CreateProductInput{Name: "Dress"})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	sku1, err := service.CreateSKU(ctx, 7, product.ID, CreateSKUInput{Code: "DRESS-S"})
	if err != nil {
		t.Fatalf("CreateSKU: %v", err)
	}
	if _, err := service.CreateSKU(ctx, 7, product.ID, CreateSKUInput{Code: "DRESS-S"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate SKU error = %v, want ErrConflict", err)
	}
	otherProduct, err := service.CreateProduct(ctx, 7, CreateProductInput{Name: "Shoes"})
	if err != nil {
		t.Fatalf("CreateProduct other: %v", err)
	}
	sku2, err := service.CreateSKU(ctx, 7, otherProduct.ID, CreateSKUInput{Code: "SHOE-1"})
	if err != nil {
		t.Fatalf("CreateSKU other: %v", err)
	}

	if _, err := service.CreateProject(ctx, 7, CreateProjectInput{ProductID: product.ID, Pipeline: "apparel"}); !errors.Is(err, ErrInvalidPipeline) {
		t.Fatalf("invalid pipeline error = %v, want ErrInvalidPipeline", err)
	}
	if err := service.ValidateProjectSKUs(ctx, 7, product.ID, []uint{sku1.ID}); err != nil {
		t.Fatalf("ValidateProjectSKUs same product: %v", err)
	}
	if err := service.ValidateProjectSKUs(ctx, 7, product.ID, []uint{sku1.ID, sku2.ID}); !errors.Is(err, ErrOwnershipMismatch) {
		t.Fatalf("mixed-product selected SKU error = %v, want ErrOwnershipMismatch", err)
	}
}

func TestProjectServiceDeletionGate(t *testing.T) {
	ctx := context.Background()
	service, _ := newCommerceServiceTest(t)
	product, err := service.CreateProduct(ctx, 30, CreateProductInput{Name: "Desk"})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	project, err := service.CreateProject(ctx, 30, CreateProjectInput{ProductID: product.ID, Pipeline: "general"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	deleted, err := service.RequestProjectDeletion(ctx, 30, project.ID)
	if err != nil {
		t.Fatalf("RequestProjectDeletion: %v", err)
	}
	if deleted.Status != "deletion_requested" || deleted.DeletionRequestedAt == nil {
		t.Fatalf("unexpected deletion request state: %#v", deleted)
	}
	if _, err := service.ValidateProjectWritable(ctx, 30, project.ID); !errors.Is(err, ErrProjectDeletionRequested) {
		t.Fatalf("ValidateProjectWritable error = %v, want ErrProjectDeletionRequested", err)
	}
}

func TestRequestProjectDeletionReturnsAcceptedSnapshotWhenReconcileDeletesAfterCommit(t *testing.T) {
	ctx := context.Background()
	service, db := newCommerceServiceTest(t)
	product, err := service.CreateProduct(ctx, 31, CreateProductInput{Name: "Immediate delete"})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	project, err := service.CreateProject(ctx, 31, CreateProjectInput{ProductID: product.ID, Pipeline: "general"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	queue := NewQueue(db, service, "deletion-reconciler")
	var projectQueries atomic.Int32
	const callbackName = "test:reconcile-project-deletion-after-commit"
	if err := db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.Table != "commerce_projects" {
			return
		}
		if projectQueries.Add(1) != 2 {
			return
		}
		if reconcileErr := queue.ReconcileProjectDeletions(ctx, 10); reconcileErr != nil {
			tx.AddError(reconcileErr)
		}
	}); err != nil {
		t.Fatalf("register deletion reconcile callback: %v", err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove(callbackName) })

	accepted, err := service.RequestProjectDeletion(ctx, project.UserID, project.ID)
	if err != nil {
		t.Fatalf("RequestProjectDeletion returned post-commit not found: %v", err)
	}
	if accepted.ID != project.ID || accepted.Status != "deletion_requested" || accepted.DeletionRequestedAt == nil {
		t.Fatalf("accepted deletion snapshot = %#v", accepted)
	}
	var stored CommerceProject
	if err := db.Unscoped().First(&stored, project.ID).Error; err != nil {
		t.Fatalf("load reconciled project: %v", err)
	}
	if !stored.DeletedAt.Valid {
		t.Fatalf("project was not reconciled after commit: %#v", stored)
	}
	if err := queue.ReconcileProjectDeletions(ctx, 10); err != nil {
		t.Fatalf("repeat ReconcileProjectDeletions: %v", err)
	}
	var rows int64
	if err := db.Unscoped().Model(&CommerceProject{}).Where("id = ?", project.ID).Count(&rows).Error; err != nil {
		t.Fatalf("count project rows: %v", err)
	}
	if rows != 1 {
		t.Fatalf("deletion side effect rows = %d, want 1", rows)
	}
}

func TestOwnershipLogoReferenceAsset(t *testing.T) {
	ctx := context.Background()
	_, db := newCommerceServiceTest(t)
	logoID := uint(44)
	resolver := fakeReferenceAssetOwnershipResolver(func(_ context.Context, userID, assetID uint) (bool, error) {
		return userID == 1 && assetID == 55, nil
	})
	service := NewService(NewRepository(db), resolver)

	if _, err := service.CreateBrand(ctx, 1, CreateBrandInput{Name: "Wrong logo", LogoReferenceAssetID: &logoID}); !errors.Is(err, ErrOwnershipMismatch) {
		t.Fatalf("foreign logo CreateBrand error = %v, want ErrOwnershipMismatch", err)
	}
	var count int64
	if err := db.Model(&CommerceBrand{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("brands after rejected logo = %d, err=%v", count, err)
	}

	ownedLogoID := uint(55)
	brand, err := service.CreateBrand(ctx, 1, CreateBrandInput{Name: "Owned logo", LogoReferenceAssetID: &ownedLogoID})
	if err != nil {
		t.Fatalf("CreateBrand owned logo: %v", err)
	}
	if brand.LogoReferenceAssetID == nil || *brand.LogoReferenceAssetID != ownedLogoID {
		t.Fatalf("created logo = %v, want %d", brand.LogoReferenceAssetID, ownedLogoID)
	}
	if _, err := NewService(NewRepository(db)).PatchBrand(ctx, 1, brand.ID, PatchBrandInput{LogoReferenceAssetID: &logoID}); err == nil {
		t.Fatal("PatchBrand accepted non-nil logo without ownership resolver")
	}
}

func TestProjectServiceInterleavedUpdates(t *testing.T) {
	ctx := context.Background()
	service, db := newCommerceServiceTest(t)
	product, err := service.CreateProduct(ctx, 70, CreateProductInput{Name: "Interleave"})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	project, err := service.CreateProject(ctx, 70, CreateProjectInput{ProductID: product.ID, Pipeline: "general", Title: "before"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	registerCommerceQueryMutation(t, db, "patch-project-interleave", "commerce_projects", func(tx *gorm.DB) {
		timestamp := "2026-07-10 12:00:00"
		if err := tx.Exec("UPDATE commerce_projects SET status = ?, deletion_requested_at = ? WHERE id = ?", "deletion_requested", timestamp, project.ID).Error; err != nil {
			t.Fatalf("interleave deletion request: %v", err)
		}
	})
	title := "after"
	if _, err := service.PatchProject(ctx, 70, project.ID, PatchProjectInput{Title: &title}); !errors.Is(err, ErrProjectDeletionRequested) {
		t.Fatalf("interleaved PatchProject error = %v, want ErrProjectDeletionRequested", err)
	}
	var stored CommerceProject
	if err := db.First(&stored, project.ID).Error; err != nil {
		t.Fatalf("load project: %v", err)
	}
	if stored.Status != "deletion_requested" || stored.DeletionRequestedAt == nil || stored.Title != "before" {
		t.Fatalf("patch restored deletion state or changed title: %#v", stored)
	}
}

func TestProjectServiceDeletionDoesNotRestoreActiveSpec(t *testing.T) {
	ctx := context.Background()
	service, db := newCommerceServiceTest(t)
	product, _ := service.CreateProduct(ctx, 71, CreateProductInput{Name: "Delete"})
	project, _ := service.CreateProject(ctx, 71, CreateProjectInput{ProductID: product.ID, Pipeline: "general"})
	activeID := uint(777)
	registerCommerceQueryMutation(t, db, "delete-project-interleave", "commerce_projects", func(tx *gorm.DB) {
		if err := tx.Exec("UPDATE commerce_projects SET active_creative_spec_id = ? WHERE id = ?", activeID, project.ID).Error; err != nil {
			t.Fatalf("interleave active spec: %v", err)
		}
	})
	if _, err := service.RequestProjectDeletion(ctx, 71, project.ID); err != nil {
		t.Fatalf("RequestProjectDeletion: %v", err)
	}
	var stored CommerceProject
	if err := db.First(&stored, project.ID).Error; err != nil {
		t.Fatalf("load project: %v", err)
	}
	if stored.ActiveCreativeSpecID == nil || *stored.ActiveCreativeSpecID != activeID {
		t.Fatalf("delete restored active spec: %v, want %d", stored.ActiveCreativeSpecID, activeID)
	}
	if stored.Status != "deletion_requested" || stored.DeletionRequestedAt == nil {
		t.Fatalf("unexpected deletion state: %#v", stored)
	}
}

func TestProjectRowLockSerializesDeletionOrchestration(t *testing.T) {
	ctx := context.Background()
	service, db := newCommerceServiceTest(t)
	product, err := service.CreateProduct(ctx, 72, CreateProductInput{Name: "Lock"})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	project, err := service.CreateProject(ctx, 72, CreateProjectInput{ProductID: product.ID, Pipeline: "general"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin holder transaction: %v", tx.Error)
	}
	if _, err := lockWritableProjectTx(ctx, tx, project.UserID, project.ID); err != nil {
		_ = tx.Rollback()
		t.Fatalf("lock writable project: %v", err)
	}
	deletionDone := make(chan error, 1)
	go func() {
		_, deleteErr := service.RequestProjectDeletion(ctx, project.UserID, project.ID)
		deletionDone <- deleteErr
	}()
	earlySQLiteConflict := false
	select {
	case err := <-deletionDone:
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "locked") {
			_ = tx.Rollback()
			t.Fatalf("deletion bypassed held project lock: %v", err)
		}
		earlySQLiteConflict = true
	case <-time.After(100 * time.Millisecond):
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit holder transaction: %v", err)
	}
	if earlySQLiteConflict {
		if _, err := service.RequestProjectDeletion(ctx, project.UserID, project.ID); err != nil {
			t.Fatalf("RequestProjectDeletion retry after CAS conflict: %v", err)
		}
	} else {
		select {
		case err := <-deletionDone:
			if err != nil {
				t.Fatalf("RequestProjectDeletion after unlock: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("deletion did not resume after project lock release")
		}
	}
}

func registerCommerceQueryMutation(t *testing.T, db *gorm.DB, name, table string, mutate func(*gorm.DB)) {
	t.Helper()
	var fired atomic.Bool
	if err := db.Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) {
		if fired.Load() || tx.Error != nil || tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.Table != table {
			return
		}
		if !fired.CompareAndSwap(false, true) {
			return
		}
		mutate(tx.Session(&gorm.Session{NewDB: true}))
	}); err != nil {
		t.Fatalf("register query mutation: %v", err)
	}
}
