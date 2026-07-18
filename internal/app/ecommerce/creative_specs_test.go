package ecommerce

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestLatestCreativeSpecUsesProjectBoundaryAndStableOrder(t *testing.T) {
	ctx := context.Background()
	service, db := newCommerceServiceTest(t)
	product, _ := service.CreateProduct(ctx, 80, CreateProductInput{Name: "Restore"})
	project, _ := service.CreateProject(ctx, 80, CreateProjectInput{ProductID: product.ID, Pipeline: "general"})
	otherProject, _ := service.CreateProject(ctx, 80, CreateProjectInput{ProductID: product.ID, Pipeline: "general"})
	emptyProject, _ := service.CreateProject(ctx, 80, CreateProjectInput{ProductID: product.ID, Pipeline: "general"})

	base := time.Date(2026, 7, 11, 8, 0, 0, 0, time.UTC)
	firstAtLatestTime := CommerceCreativeSpec{UserID: 80, ProjectID: project.ID, Version: 1, Source: "vision", Status: "analyzing", ProductFactsJSON: `{}`, CreatedAt: base.Add(time.Minute), UpdatedAt: base.Add(time.Minute)}
	if err := db.Create(&firstAtLatestTime).Error; err != nil {
		t.Fatal(err)
	}
	higherIDButOlder := CommerceCreativeSpec{UserID: 80, ProjectID: project.ID, Version: 1, Source: "manual", Status: "draft", ProductFactsJSON: `{}`, CreatedAt: base, UpdatedAt: base}
	if err := db.Create(&higherIDButOlder).Error; err != nil {
		t.Fatal(err)
	}
	want := CommerceCreativeSpec{UserID: 80, ProjectID: project.ID, Version: 2, Source: "manual", Status: "draft", ProductFactsJSON: `{"name":"latest"}`, CreatedAt: base.Add(time.Minute), UpdatedAt: base.Add(time.Minute)}
	if err := db.Create(&want).Error; err != nil {
		t.Fatal(err)
	}
	other := CommerceCreativeSpec{UserID: 80, ProjectID: otherProject.ID, Version: 1, Source: "vision", Status: "analyzing", ProductFactsJSON: `{}`, CreatedAt: base.Add(time.Hour), UpdatedAt: base.Add(time.Hour)}
	if err := db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}

	got, err := service.GetLatestCreativeSpec(ctx, 80, project.ID)
	if err != nil || got.ID != want.ID {
		t.Fatalf("latest spec = %#v, err=%v, want id %d", got, err, want.ID)
	}
	if _, err := service.GetLatestCreativeSpec(ctx, 81, project.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user error = %v, want ErrNotFound", err)
	}
	if _, err := service.GetLatestCreativeSpec(ctx, 80, emptyProject.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty/cross-project error = %v, want ErrNotFound", err)
	}
	deletionRequestedAt := base.Add(2 * time.Hour)
	if err := db.Model(&CommerceProject{}).Where("id = ?", project.ID).Updates(map[string]any{"status": "deletion_requested", "deletion_requested_at": deletionRequestedAt}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetLatestCreativeSpec(ctx, 80, project.ID); !errors.Is(err, ErrProjectDeletionRequested) {
		t.Fatalf("deletion-requested error = %v, want ErrProjectDeletionRequested", err)
	}
	if err := db.Delete(&project).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetLatestCreativeSpec(ctx, 80, project.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted-project error = %v, want ErrNotFound", err)
	}
}

func TestCreativeSpecManualLifecycle(t *testing.T) {
	ctx := context.Background()
	service, _ := newCommerceServiceTest(t)
	product, err := service.CreateProduct(ctx, 10, CreateProductInput{Name: "Bottle"})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	project, err := service.CreateProject(ctx, 10, CreateProjectInput{ProductID: product.ID, Pipeline: "general"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	spec, err := service.CreateManualCreativeSpec(ctx, 10, project.ID, ManualCreativeSpecInput{
		ProductFacts:  []byte(`{"name":"Bottle","material":"steel"}`),
		SellingPoints: []byte(`["durable"]`),
	})
	if err != nil {
		t.Fatalf("CreateManualCreativeSpec: %v", err)
	}
	if spec.Source != "manual" || spec.Status != "draft" || spec.Version != 1 {
		t.Fatalf("unexpected initial spec: %#v", spec)
	}
	if _, err := service.BuildConfirmedCreativeSpecSnapshot(ctx, 10, project.ID, spec.ID); !errors.Is(err, ErrCreativeSpecNotConfirmed) {
		t.Fatalf("draft snapshot error = %v, want ErrCreativeSpecNotConfirmed", err)
	}
	if _, err := service.GetCreativeSpec(ctx, 11, spec.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user GetCreativeSpec error = %v, want ErrNotFound", err)
	}
	if _, err := service.PatchCreativeSpec(ctx, 11, spec.ID, PatchCreativeSpecInput{ExpectedVersion: 1}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user PatchCreativeSpec error = %v, want ErrNotFound", err)
	}

	confirmed, err := service.ConfirmCreativeSpec(ctx, 10, spec.ID)
	if err != nil {
		t.Fatalf("ConfirmCreativeSpec: %v", err)
	}
	if confirmed.Status != "confirmed" || confirmed.LockedAt == nil {
		t.Fatalf("unexpected confirmed spec: %#v", confirmed)
	}
	snapshot, err := service.BuildConfirmedCreativeSpecSnapshot(ctx, 10, project.ID, spec.ID)
	if err != nil {
		t.Fatalf("BuildConfirmedCreativeSpecSnapshot: %v", err)
	}
	if snapshot.Status != "confirmed" || snapshot.ContentSHA256 == "" || snapshot.ID != spec.ID {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}

	copyBlocks := []byte(`[{"text":"New copy"}]`)
	edited, err := service.PatchCreativeSpec(ctx, 10, spec.ID, PatchCreativeSpecInput{
		ExpectedVersion: confirmed.Version,
		CopyBlocks:      &copyBlocks,
	})
	if err != nil {
		t.Fatalf("PatchCreativeSpec: %v", err)
	}
	if edited.Status != "draft" || edited.Version != confirmed.Version+1 || edited.LockedAt != nil {
		t.Fatalf("unexpected edited spec: %#v", edited)
	}
	loadedProject, err := service.GetProject(ctx, 10, project.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if loadedProject.ActiveCreativeSpecID != nil {
		t.Fatalf("active spec not cleared after edit: %v", *loadedProject.ActiveCreativeSpecID)
	}
}

func TestManualCreativeSpecConfirmMergesOverridesAndValidatesRequiredMissing(t *testing.T) {
	ctx := context.Background()
	service, db := newCommerceServiceTest(t)
	product, _ := service.CreateProduct(ctx, 12, CreateProductInput{Name: "Manual fallback"})
	project, _ := service.CreateProject(ctx, 12, CreateProjectInput{ProductID: product.ID, Pipeline: "general"})
	spec, err := service.CreateManualCreativeSpec(ctx, 12, project.ID, ManualCreativeSpecInput{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&CommerceCreativeSpec{}).Where("id = ?", spec.ID).Update("missing_fields_json", `["material"]`).Error; err != nil {
		t.Fatal(err)
	}
	overrides := []byte(`{"name":"手工杯子","category":"家居"}`)
	patched, err := service.PatchCreativeSpec(ctx, 12, spec.ID, PatchCreativeSpecInput{ExpectedVersion: 1, UserOverrides: &overrides})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmCreativeSpec(ctx, 12, spec.ID); err == nil || !strings.Contains(err.Error(), "material must be resolved") {
		t.Fatalf("missing material confirm error = %v", err)
	}
	overrides = []byte(`{"name":"手工杯子","category":"家居","material":"不锈钢"}`)
	if _, err := service.PatchCreativeSpec(ctx, 12, spec.ID, PatchCreativeSpecInput{ExpectedVersion: patched.Version, UserOverrides: &overrides}); err != nil {
		t.Fatal(err)
	}
	confirmed, err := service.ConfirmCreativeSpec(ctx, 12, spec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.Status != "confirmed" || !strings.Contains(confirmed.ProductFactsJSON, `"name":"手工杯子"`) || !strings.Contains(confirmed.ProductFactsJSON, `"material":"不锈钢"`) {
		t.Fatalf("confirmed=%#v", confirmed)
	}
}

func TestCreativeSpecConfirmReplacesActiveSpec(t *testing.T) {
	ctx := context.Background()
	service, _ := newCommerceServiceTest(t)
	product, _ := service.CreateProduct(ctx, 20, CreateProductInput{Name: "Watch"})
	project, _ := service.CreateProject(ctx, 20, CreateProjectInput{ProductID: product.ID, Pipeline: "mixed"})

	first, err := service.CreateManualCreativeSpec(ctx, 20, project.ID, ManualCreativeSpecInput{ProductFacts: []byte(`{"name":"watch"}`)})
	if err != nil {
		t.Fatalf("create first spec: %v", err)
	}
	second, err := service.CreateManualCreativeSpec(ctx, 20, project.ID, ManualCreativeSpecInput{ProductFacts: []byte(`{"name":"watch v2"}`)})
	if err != nil {
		t.Fatalf("create second spec: %v", err)
	}
	if _, err := service.ConfirmCreativeSpec(ctx, 20, first.ID); err != nil {
		t.Fatalf("confirm first: %v", err)
	}
	if _, err := service.ConfirmCreativeSpec(ctx, 20, second.ID); err != nil {
		t.Fatalf("confirm second: %v", err)
	}
	loaded, err := service.GetProject(ctx, 20, project.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if loaded.ActiveCreativeSpecID == nil || *loaded.ActiveCreativeSpecID != second.ID {
		t.Fatalf("active spec = %v, want %d", loaded.ActiveCreativeSpecID, second.ID)
	}
}

func TestCreativeSpecConfirmCASConflict(t *testing.T) {
	ctx := context.Background()
	service, db := newCommerceServiceTest(t)
	product, _ := service.CreateProduct(ctx, 31, CreateProductInput{Name: "CAS"})
	project, _ := service.CreateProject(ctx, 31, CreateProjectInput{ProductID: product.ID, Pipeline: "general"})
	spec, err := service.CreateManualCreativeSpec(ctx, 31, project.ID, ManualCreativeSpecInput{ProductFacts: []byte(`{"name":"cas"}`)})
	if err != nil {
		t.Fatalf("CreateManualCreativeSpec: %v", err)
	}
	registerCommerceQueryMutation(t, db, "confirm-spec-cas", "commerce_creative_specs", func(tx *gorm.DB) {
		if err := tx.Exec("UPDATE commerce_creative_specs SET version = ?, copy_blocks_json = ? WHERE id = ?", 2, `[{"text":"patched"}]`, spec.ID).Error; err != nil {
			t.Fatalf("interleave spec patch: %v", err)
		}
	})
	if _, err := service.ConfirmCreativeSpec(ctx, 31, spec.ID); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("ConfirmCreativeSpec error = %v, want ErrVersionConflict", err)
	}
	var stored CommerceCreativeSpec
	if err := db.First(&stored, spec.ID).Error; err != nil {
		t.Fatalf("load spec: %v", err)
	}
	if stored.Version != 2 || stored.Status != "draft" || stored.CopyBlocksJSON != `[{"text":"patched"}]` {
		t.Fatalf("conflicting confirm overwrote concurrent patch: %#v", stored)
	}
}

func TestCreativeSpecConfirmDoesNotRestoreDeletionState(t *testing.T) {
	ctx := context.Background()
	service, db := newCommerceServiceTest(t)
	product, _ := service.CreateProduct(ctx, 32, CreateProductInput{Name: "Delete race"})
	project, _ := service.CreateProject(ctx, 32, CreateProjectInput{ProductID: product.ID, Pipeline: "general"})
	spec, _ := service.CreateManualCreativeSpec(ctx, 32, project.ID, ManualCreativeSpecInput{ProductFacts: []byte(`{"name":"race"}`)})
	registerCommerceQueryMutation(t, db, "confirm-project-delete", "commerce_projects", func(tx *gorm.DB) {
		if err := tx.Exec("UPDATE commerce_projects SET status = ?, deletion_requested_at = ? WHERE id = ?", "deletion_requested", "2026-07-10 12:00:00", project.ID).Error; err != nil {
			t.Fatalf("interleave project deletion: %v", err)
		}
	})
	if _, err := service.ConfirmCreativeSpec(ctx, 32, spec.ID); !errors.Is(err, ErrProjectDeletionRequested) {
		t.Fatalf("ConfirmCreativeSpec error = %v, want ErrProjectDeletionRequested", err)
	}
	var stored CommerceProject
	if err := db.First(&stored, project.ID).Error; err != nil {
		t.Fatalf("load project: %v", err)
	}
	if stored.Status != "deletion_requested" || stored.DeletionRequestedAt == nil {
		t.Fatalf("confirm restored deletion state: %#v", stored)
	}
}
