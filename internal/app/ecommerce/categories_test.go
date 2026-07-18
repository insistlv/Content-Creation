package ecommerce

import (
	"context"
	"errors"
	"testing"
)

func TestCommerceCategoryCatalogSeedsTwoLevelChineseDefaults(t *testing.T) {
	service, db := newCommerceServiceTest(t)
	catalog, err := service.ListCategories(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListCategories: %v", err)
	}
	if catalog.Version != "cn-commerce-v1" || len(catalog.SystemCategories) != 16 {
		t.Fatalf("catalog version/roots = %q/%d", catalog.Version, len(catalog.SystemCategories))
	}
	var cup CommerceSystemCategory
	if err := db.Where("name = ? AND level = ?", "杯壶餐具", 2).First(&cup).Error; err != nil {
		t.Fatalf("seed 杯壶餐具: %v", err)
	}
	if cup.ParentID == nil || cup.SearchAliasesJSON == "" {
		t.Fatalf("cup category missing parent/aliases: %#v", cup)
	}
	var root CommerceSystemCategory
	if err := db.Where("seed_key = ?", "root-04").First(&root).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&root).Update("name", "居家百货").Error; err != nil {
		t.Fatal(err)
	}
	if err := SeedDefaultCategories(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var rootCount int64
	if err := db.Model(&CommerceSystemCategory{}).Where("level = ?", 1).Count(&rootCount).Error; err != nil || rootCount != 16 {
		t.Fatalf("seed after rename root count = %d, %v", rootCount, err)
	}
}

func TestCommerceCustomCategoriesArePrivateAndLifecycleManaged(t *testing.T) {
	service, db := newCommerceServiceTest(t)
	var parent CommerceSystemCategory
	if err := db.Where("name = ? AND level = ?", "家居日用", 1).First(&parent).Error; err != nil {
		t.Fatal(err)
	}
	created, err := service.CreateCustomCategory(context.Background(), 10, CreateCustomCategoryInput{ParentID: parent.ID, Name: "咖啡器具"})
	if err != nil {
		t.Fatalf("CreateCustomCategory: %v", err)
	}
	if _, err := service.CreateCustomCategory(context.Background(), 10, CreateCustomCategoryInput{ParentID: parent.ID, Name: " 咖啡器具 "}); !errors.Is(err, ErrCategoryConflict) {
		t.Fatalf("duplicate error = %v", err)
	}
	ownerCatalog, _ := service.ListCategories(context.Background(), 10)
	otherCatalog, _ := service.ListCategories(context.Background(), 11)
	if len(ownerCatalog.CustomCategories) != 1 || len(otherCatalog.CustomCategories) != 0 {
		t.Fatalf("custom visibility owner=%d other=%d", len(ownerCatalog.CustomCategories), len(otherCatalog.CustomCategories))
	}
	name, status := "手冲咖啡器具", CategoryStatusInactive
	updated, err := service.PatchCustomCategory(context.Background(), 10, created.ID, PatchCustomCategoryInput{Name: &name, Status: &status})
	if err != nil || updated.Name != name || updated.Status != CategoryStatusInactive {
		t.Fatalf("PatchCustomCategory = %#v, %v", updated, err)
	}
	if _, err := service.PatchCustomCategory(context.Background(), 11, created.ID, PatchCustomCategoryInput{Name: &name}); !errors.Is(err, ErrOwnershipMismatch) {
		t.Fatalf("cross-user patch error = %v", err)
	}
}

func TestCommerceBootstrapValidatesCategorySelectionAndKeepsLegacyText(t *testing.T) {
	service, db := newCommerceServiceTest(t)
	var child CommerceSystemCategory
	if err := db.Preload("Parent").Where("name = ? AND level = ?", "杯壶餐具", 2).First(&child).Error; err != nil {
		t.Fatal(err)
	}
	selected, err := service.BootstrapProject(context.Background(), 20, "category-selected", BootstrapProjectInput{
		Title: "保温杯", Category: "伪造路径", CategoryID: child.ID, CategorySource: CategorySourceSystem, CategoryPath: "伪造路径", Pipeline: "general",
	})
	if err != nil {
		t.Fatalf("selected bootstrap: %v", err)
	}
	if selected.Product.Category != "家居日用 / 杯壶餐具" || selected.Product.CategoryID == nil || selected.Product.CategorySource != CategorySourceSystem {
		t.Fatalf("selected product category = %#v", selected.Product)
	}
	legacy, err := service.BootstrapProject(context.Background(), 20, "category-legacy", BootstrapProjectInput{Title: "旧商品", Category: "旧自由文本", Pipeline: "general"})
	if err != nil || legacy.Product.Category != "旧自由文本" || legacy.Product.CategoryID != nil {
		t.Fatalf("legacy bootstrap = %#v, %v", legacy.Product, err)
	}
	if _, err := service.BootstrapProject(context.Background(), 20, "category-invalid", BootstrapProjectInput{Title: "错误", CategoryID: 999999, CategorySource: CategorySourceSystem, Pipeline: "general"}); !errors.Is(err, ErrCategoryUnavailable) {
		t.Fatalf("invalid category error = %v", err)
	}
}

func TestCommerceProductPatchMapsCategorySelection(t *testing.T) {
	service, db := newCommerceServiceTest(t)
	product, err := service.CreateProduct(context.Background(), 30, CreateProductInput{Name: "旧水杯", Category: "旧分类"})
	if err != nil {
		t.Fatal(err)
	}
	var child CommerceSystemCategory
	if err := db.Where("name = ? AND level = ?", "杯壶餐具", 2).First(&child).Error; err != nil {
		t.Fatal(err)
	}
	source := CategorySourceSystem
	updated, err := service.PatchProduct(context.Background(), 30, product.ID, PatchProductInput{CategoryID: &child.ID, CategorySource: &source})
	if err != nil {
		t.Fatalf("PatchProduct: %v", err)
	}
	if updated.Category != "家居日用 / 杯壶餐具" || updated.CategoryPath != updated.Category || updated.CategoryID == nil {
		t.Fatalf("updated category = %#v", updated)
	}
}
