package ecommerce

import (
	"context"
	"errors"
	"testing"
	"time"
)

type assetTestReferenceAsset struct {
	ID           uint   `gorm:"primaryKey"`
	UserID       uint   `gorm:"index"`
	AssetKey     string `gorm:"size:255"`
	StorageScope string `gorm:"size:32"`
}

func (assetTestReferenceAsset) TableName() string { return "reference_assets" }

func newAssetServiceTest(t *testing.T) (*AssetService, *Repository, time.Time) {
	t.Helper()
	_, db := newCommerceServiceTest(t)
	if err := db.AutoMigrate(&assetTestReferenceAsset{}); err != nil {
		t.Fatalf("migrate reference assets: %v", err)
	}
	now := time.Date(2026, time.July, 10, 8, 0, 0, 0, time.UTC)
	service := NewAssetService(NewRepository(db))
	service.now = func() time.Time { return now }
	return service, NewRepository(db), now
}

func seedAssetProject(t *testing.T, repository *Repository, userID uint) CommerceProject {
	t.Helper()
	project := CommerceProject{UserID: userID, ProductID: 1, Title: "Assets", Pipeline: "general", Status: "active"}
	if err := repository.DB().Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	return project
}

func seedAssetReference(t *testing.T, repository *Repository, userID uint, key string) assetTestReferenceAsset {
	t.Helper()
	asset := assetTestReferenceAsset{UserID: userID, AssetKey: key, StorageScope: StorageScopeCommercePrivate}
	if err := repository.DB().Create(&asset).Error; err != nil {
		t.Fatalf("create reference asset: %v", err)
	}
	return asset
}

func TestAssetOwnership(t *testing.T) {
	ctx := context.Background()
	service, repository, _ := newAssetServiceTest(t)
	project := seedAssetProject(t, repository, 11)
	reference := seedAssetReference(t, repository, 11, "commerce/11/1/product.png")

	created, err := service.CreateAsset(ctx, CreateAssetInput{
		UserID:           11,
		ProjectID:        project.ID,
		ReferenceAssetID: reference.ID,
		Role:             "product",
		Lifecycle:        AssetLifecycleProject,
	})
	if err != nil {
		t.Fatalf("CreateAsset: %v", err)
	}
	if _, err := service.GetAsset(ctx, 12, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user GetAsset error = %v, want ErrNotFound", err)
	}
	if _, err := service.ListProjectAssets(ctx, 12, project.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user ListProjectAssets error = %v, want ErrNotFound", err)
	}
	if got, err := service.GetAsset(ctx, 11, created.ID); err != nil || got.ReferenceAssetID != reference.ID {
		t.Fatalf("owner GetAsset = %#v, err=%v", got, err)
	}
}

func TestAssetLifecycle(t *testing.T) {
	ctx := context.Background()
	service, repository, now := newAssetServiceTest(t)
	project := seedAssetProject(t, repository, 21)
	projectReference := seedAssetReference(t, repository, 21, "commerce/21/1/project.png")
	temporaryReference := seedAssetReference(t, repository, 21, "commerce/21/1/temp.png")

	projectAsset, err := service.CreateAsset(ctx, CreateAssetInput{
		UserID: 21, ProjectID: project.ID, ReferenceAssetID: projectReference.ID,
		Role: "product", Lifecycle: AssetLifecycleProject, TemporaryRetention: 7 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("CreateAsset project: %v", err)
	}
	if projectAsset.RetainUntil != nil {
		t.Fatalf("project asset retain_until = %v, want nil", projectAsset.RetainUntil)
	}

	temporaryAsset, err := service.CreateAsset(ctx, CreateAssetInput{
		UserID: 21, ProjectID: project.ID, ReferenceAssetID: temporaryReference.ID,
		Role: "candidate", Lifecycle: AssetLifecycleTemporary, TemporaryRetention: 7 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("CreateAsset temporary: %v", err)
	}
	wantRetainUntil := now.Add(7 * 24 * time.Hour)
	if temporaryAsset.RetainUntil == nil || !temporaryAsset.RetainUntil.Equal(wantRetainUntil) {
		t.Fatalf("temporary retain_until = %v, want %v", temporaryAsset.RetainUntil, wantRetainUntil)
	}
}

func TestAssetCleanup(t *testing.T) {
	ctx := context.Background()
	service, repository, now := newAssetServiceTest(t)
	project := seedAssetProject(t, repository, 31)
	reference := seedAssetReference(t, repository, 31, "commerce/31/1/expired.png")
	asset, err := service.CreateAsset(ctx, CreateAssetInput{
		UserID: 31, ProjectID: project.ID, ReferenceAssetID: reference.ID,
		Role: "candidate", Lifecycle: AssetLifecycleTemporary, TemporaryRetention: -time.Hour,
	})
	if err != nil {
		t.Fatalf("CreateAsset: %v", err)
	}

	queued, err := service.QueueExpiredTemporaryAssets(ctx)
	if err != nil {
		t.Fatalf("QueueExpiredTemporaryAssets: %v", err)
	}
	if queued != 1 {
		t.Fatalf("queued = %d, want 1", queued)
	}
	var cleanup CommerceObjectCleanup
	if err := repository.DB().Where("commerce_asset_id = ?", asset.ID).First(&cleanup).Error; err != nil {
		t.Fatalf("load cleanup: %v", err)
	}
	if cleanup.ObjectKey != reference.AssetKey || cleanup.StorageScope != StorageScopeCommercePrivate {
		t.Fatalf("cleanup object = %#v", cleanup)
	}

	service.now = func() time.Time { return now.Add(time.Hour) }
	if err := service.RecordCleanupFailure(ctx, cleanup.ID, errors.New("OSS unavailable")); err != nil {
		t.Fatalf("RecordCleanupFailure: %v", err)
	}
	if err := repository.DB().First(&cleanup, cleanup.ID).Error; err != nil {
		t.Fatalf("reload failed cleanup: %v", err)
	}
	if cleanup.AttemptCount != 1 || cleanup.NextAttemptAt == nil || !cleanup.NextAttemptAt.After(now.Add(time.Hour)) || cleanup.ObjectDeletedAt != nil {
		t.Fatalf("failed cleanup state = %#v", cleanup)
	}

	service.now = func() time.Time { return now.Add(2 * time.Hour) }
	if err := service.RecordCleanupSuccess(ctx, cleanup.ID); err != nil {
		t.Fatalf("RecordCleanupSuccess: %v", err)
	}
	if err := repository.DB().First(&cleanup, cleanup.ID).Error; err != nil {
		t.Fatalf("reload successful cleanup: %v", err)
	}
	if cleanup.ObjectDeletedAt == nil || cleanup.Status != CleanupStatusSucceeded {
		t.Fatalf("successful cleanup state = %#v", cleanup)
	}
}
