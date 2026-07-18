package ecommerce

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func runConcurrentMatrix(t *testing.T, calls ...func() error) []error {
	t.Helper()
	start := make(chan struct{})
	results := make([]error, len(calls))
	var wg sync.WaitGroup
	for i, call := range calls {
		wg.Add(1)
		go func(i int, call func() error) { defer wg.Done(); <-start; results[i] = call() }(i, call)
	}
	close(start)
	wg.Wait()
	return results
}

func newSKUMatrixTestService(t *testing.T) (*Service, *gorm.DB, CommerceProduct) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := MigrateSQLiteFoundationSchema(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	product := CommerceProduct{UserID: 7, Name: "测试商品", Status: "active"}
	if err := db.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	return NewService(NewRepository(db)), db, product
}

func matrixInput(version int, dimensions ...SKUDimensionInput) SKUMatrixInput {
	return SKUMatrixInput{ExpectedVersion: version, Dimensions: dimensions}
}

func TestPreviewSKUMatrixBuildsCartesianProductWithoutWriting(t *testing.T) {
	s, db, product := newSKUMatrixTestService(t)
	in := matrixInput(0,
		SKUDimensionInput{Name: "颜色", Values: []SKUValueInput{{Name: "红"}, {Name: "蓝"}}},
		SKUDimensionInput{Name: "尺码", Values: []SKUValueInput{{Name: "S"}, {Name: "M"}, {Name: "L"}}},
	)
	preview, err := s.PreviewSKUMatrix(context.Background(), 7, product.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Add) != 6 || len(preview.Keep) != 0 || len(preview.Disable) != 0 {
		t.Fatalf("preview=%+v", preview)
	}
	for _, model := range []any{&CommerceSKUDimension{}, &CommerceSKUValue{}, &CommerceSKUValueLink{}, &CommerceSKU{}} {
		var count int64
		if err := db.Model(model).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("%T count=%d err=%v", model, count, err)
		}
	}
}

func TestPreviewSKUMatrixRejectsCombinationLimit(t *testing.T) {
	s, _, product := newSKUMatrixTestService(t)
	values := func(prefix string, n int) []SKUValueInput {
		out := make([]SKUValueInput, n)
		for i := range out {
			out[i] = SKUValueInput{Name: prefix + string(rune('A'+i))}
		}
		return out
	}
	_, err := s.PreviewSKUMatrix(context.Background(), 7, product.ID, matrixInput(0,
		SKUDimensionInput{Name: "甲", Values: values("a", 11)}, SKUDimensionInput{Name: "乙", Values: values("b", 10)},
	))
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err=%v", err)
	}
}

func TestPreviewSKUMatrixLimitsOnlyEnabledDimensionsAndValues(t *testing.T) {
	s, _, product := newSKUMatrixTestService(t)
	input := matrixInput(0,
		SKUDimensionInput{Name: "停用甲", Status: "disabled", Values: []SKUValueInput{{Name: "一"}}},
		SKUDimensionInput{Name: "颜色", Values: []SKUValueInput{{Name: "红"}, {Name: "废弃", Status: "disabled"}}},
		SKUDimensionInput{Name: "尺码", Values: []SKUValueInput{{Name: "M"}}},
		SKUDimensionInput{Name: "材质", Values: []SKUValueInput{{Name: "棉"}}},
	)
	preview, err := s.PreviewSKUMatrix(context.Background(), 7, product.ID, input)
	if err != nil {
		t.Fatalf("disabled dimensions must not count: %v", err)
	}
	if len(preview.Add) != 1 {
		t.Fatalf("add=%d", len(preview.Add))
	}
}

func TestPreviewSKUMatrixValueLimitCountsOnlyEnabledValues(t *testing.T) {
	s, _, product := newSKUMatrixTestService(t)
	values := make([]SKUValueInput, 0, 21)
	for i := 0; i < 20; i++ {
		values = append(values, SKUValueInput{Name: fmt.Sprintf("值%02d", i)})
	}
	values = append(values, SKUValueInput{Name: "停用值", Status: "disabled"})
	if _, err := s.PreviewSKUMatrix(context.Background(), 7, product.ID, matrixInput(0, SKUDimensionInput{Name: "规格", Values: values})); err != nil {
		t.Fatalf("20 enabled + 1 disabled must pass: %v", err)
	}
	values[20].Status = "active"
	if _, err := s.PreviewSKUMatrix(context.Background(), 7, product.ID, matrixInput(0, SKUDimensionInput{Name: "规格", Values: values})); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("21 enabled err=%v", err)
	}
}

func TestApplySKUMatrixConcurrentVersionCAS(t *testing.T) {
	s, _, product := newSKUMatrixTestService(t)
	input := matrixInput(0, SKUDimensionInput{Name: "颜色", Values: []SKUValueInput{{Name: "红"}}})
	errs := runConcurrentMatrix(t, func() error { _, e := s.ApplySKUMatrix(context.Background(), 7, product.ID, "cas-a", input); return e }, func() error { _, e := s.ApplySKUMatrix(context.Background(), 7, product.ID, "cas-b", input); return e })
	success, conflict := 0, 0
	for _, e := range errs {
		if e == nil {
			success++
		} else if errors.Is(e, ErrSKUVersionConflict) {
			conflict++
		} else {
			t.Fatalf("unexpected concurrent error: %v", e)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d errs=%v", success, conflict, errs)
	}
}

func TestApplySKUMatrixConcurrentIdempotencyClaim(t *testing.T) {
	t.Run("same digest replays", func(t *testing.T) {
		s, _, p := newSKUMatrixTestService(t)
		in := matrixInput(0, SKUDimensionInput{Name: "颜色", Values: []SKUValueInput{{Name: "红"}}})
		errs := runConcurrentMatrix(t, func() error { _, e := s.ApplySKUMatrix(context.Background(), 7, p.ID, "same", in); return e }, func() error { _, e := s.ApplySKUMatrix(context.Background(), 7, p.ID, "same", in); return e })
		for _, e := range errs {
			if e != nil {
				t.Fatalf("err=%v", e)
			}
		}
	})
	t.Run("different digest conflicts", func(t *testing.T) {
		s, _, p := newSKUMatrixTestService(t)
		a := matrixInput(0, SKUDimensionInput{Name: "颜色", Values: []SKUValueInput{{Name: "红"}}})
		b := matrixInput(0, SKUDimensionInput{Name: "颜色", Values: []SKUValueInput{{Name: "蓝"}}})
		errs := runConcurrentMatrix(t, func() error { _, e := s.ApplySKUMatrix(context.Background(), 7, p.ID, "different", a); return e }, func() error { _, e := s.ApplySKUMatrix(context.Background(), 7, p.ID, "different", b); return e })
		success, conflict := 0, 0
		for _, e := range errs {
			if e == nil {
				success++
			} else if errors.Is(e, ErrIdempotencyConflict) {
				conflict++
			} else {
				t.Fatalf("err=%v", e)
			}
		}
		if success != 1 || conflict != 1 {
			t.Fatalf("errs=%v", errs)
		}
	})
}

func TestSKUConfigPropagatesAssetCountQueryError(t *testing.T) {
	s, db, p := newSKUMatrixTestService(t)
	if err := db.Create(&CommerceSKU{UserID: 7, ProductID: p.ID, Code: "OLD", Status: "active"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Migrator().DropTable(&CommerceAsset{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSKUConfig(context.Background(), 7, p.ID); err == nil {
		t.Fatal("want asset count query error")
	}
}

func TestSKUMatrixCrossUserPreviewAndPutAreIsolated(t *testing.T) {
	s, _, p := newSKUMatrixTestService(t)
	in := matrixInput(0, SKUDimensionInput{Name: "颜色", Values: []SKUValueInput{{Name: "红"}}})
	if _, e := s.PreviewSKUMatrix(context.Background(), 8, p.ID, in); !errors.Is(e, ErrOwnershipMismatch) {
		t.Fatalf("preview=%v", e)
	}
	if _, e := s.ApplySKUMatrix(context.Background(), 8, p.ID, "foreign", in); !errors.Is(e, ErrOwnershipMismatch) {
		t.Fatalf("put=%v", e)
	}
}

func TestPreviewSKUMatrixReportsCodeConflictWithExistingSKU(t *testing.T) {
	s, db, product := newSKUMatrixTestService(t)
	if err := db.Create(&CommerceSKU{UserID: 7, ProductID: product.ID, Code: "TAKEN", Status: "active", AttributesJSON: "{}"}).Error; err != nil {
		t.Fatal(err)
	}
	preview, err := s.PreviewSKUMatrix(context.Background(), 7, product.ID, matrixInput(0, SKUDimensionInput{Name: "颜色", Values: []SKUValueInput{{Name: "红", Code: "TAKEN"}}}))
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Conflicts) != 1 || len(preview.Add) != 0 {
		t.Fatalf("preview=%+v", preview)
	}
}

func TestApplySKUMatrixIsAtomicVersionedAndIdempotent(t *testing.T) {
	s, db, product := newSKUMatrixTestService(t)
	in := matrixInput(0, SKUDimensionInput{Name: "颜色", Values: []SKUValueInput{{Name: "红"}, {Name: "蓝"}}})
	first, err := s.ApplySKUMatrix(context.Background(), 7, product.ID, "matrix-key", in)
	if err != nil {
		t.Fatal(err)
	}
	if first.Version != 1 || len(first.SKUs) != 2 || first.SKUs[0].Code != "SKU-"+itoa(product.ID)+"-1" {
		t.Fatalf("first=%+v", first)
	}
	replay, err := s.ApplySKUMatrix(context.Background(), 7, product.ID, "matrix-key", in)
	if err != nil || replay.Version != 1 || len(replay.SKUs) != 2 {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	changed := in
	changed.Dimensions[0].Values = append(changed.Dimensions[0].Values, SKUValueInput{Name: "绿"})
	if _, err := s.ApplySKUMatrix(context.Background(), 7, product.ID, "matrix-key", changed); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("digest conflict=%v", err)
	}
	if _, err := s.ApplySKUMatrix(context.Background(), 7, product.ID, "other-key", in); !errors.Is(err, ErrSKUVersionConflict) {
		t.Fatalf("version conflict=%v", err)
	}
	var dimensions int64
	db.Model(&CommerceSKUDimension{}).Where("product_id = ?", product.ID).Count(&dimensions)
	if dimensions != 1 {
		t.Fatalf("transaction residue dimensions=%d", dimensions)
	}
}

func TestApplySKUMatrixRejectsDuplicateCodeAndRollsBack(t *testing.T) {
	s, db, product := newSKUMatrixTestService(t)
	in := matrixInput(0, SKUDimensionInput{Name: "颜色", Values: []SKUValueInput{{Name: "红", Code: "DUP"}, {Name: "蓝", Code: "DUP"}}})
	if _, err := s.ApplySKUMatrix(context.Background(), 7, product.ID, "dup", in); !errors.Is(err, ErrConflict) {
		t.Fatalf("err=%v", err)
	}
	for _, model := range []any{&CommerceSKUDimension{}, &CommerceSKUValue{}, &CommerceSKU{}} {
		var n int64
		db.Model(model).Count(&n)
		if n != 0 {
			t.Fatalf("%T count=%d", model, n)
		}
	}
}

func TestApplySKUMatrixRollsBackOnWriteFailure(t *testing.T) {
	s, db, product := newSKUMatrixTestService(t)
	if err := db.Exec(`CREATE TRIGGER fail_sku_insert BEFORE INSERT ON commerce_skus BEGIN SELECT RAISE(ABORT, 'injected failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	_, err := s.ApplySKUMatrix(context.Background(), 7, product.ID, "rollback", matrixInput(0, SKUDimensionInput{Name: "颜色", Values: []SKUValueInput{{Name: "红"}}}))
	if err == nil {
		t.Fatal("want injected failure")
	}
	for _, model := range []any{&CommerceSKUDimension{}, &CommerceSKUValue{}, &CommerceSKUValueLink{}, &CommerceSKUMatrixRequest{}} {
		var n int64
		db.Model(model).Count(&n)
		if n != 0 {
			t.Fatalf("%T count=%d", model, n)
		}
	}
	var stored CommerceProduct
	if err := db.First(&stored, product.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.SKUVersion != 0 {
		t.Fatalf("version=%d", stored.SKUVersion)
	}
}

func TestApplySKUMatrixAtomicallySwitchesDefaultSKUAndOwnership(t *testing.T) {
	s, db, product := newSKUMatrixTestService(t)
	first, err := s.ApplySKUMatrix(context.Background(), 7, product.ID, "one", matrixInput(0, SKUDimensionInput{Name: "颜色", Values: []SKUValueInput{{Name: "红"}, {Name: "蓝"}}}))
	if err != nil {
		t.Fatal(err)
	}
	project := CommerceProject{UserID: 7, ProductID: product.ID, DefaultSKUID: &first.SKUs[0].ID, Title: "项目", Pipeline: "general", Status: "active"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	second, err := s.ApplySKUMatrix(context.Background(), 7, product.ID, "two", matrixInput(1, SKUDimensionInput{Name: "颜色", Values: []SKUValueInput{{Name: "蓝"}}}))
	if err != nil {
		t.Fatalf("apply matrix: %v", err)
	}
	if second.DefaultSKUID == 0 || len(second.SKUs) == 0 {
		t.Fatalf("config=%+v", second)
	}
	var updated CommerceProject
	if err := db.First(&updated, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.DefaultSKUID == nil || *updated.DefaultSKUID != second.DefaultSKUID || *updated.DefaultSKUID == first.SKUs[0].ID {
		t.Fatalf("project=%+v config=%+v", updated, second)
	}
	if _, err := s.GetSKUConfig(context.Background(), 8, product.ID); !errors.Is(err, ErrOwnershipMismatch) {
		t.Fatalf("ownership err=%v", err)
	}
}

func TestSKUConfigAndApplyPreserveActiveNonFirstProjectDefault(t *testing.T) {
	s, db, product := newSKUMatrixTestService(t)
	first, err := s.ApplySKUMatrix(context.Background(), 7, product.ID, "seed-default", matrixInput(0, SKUDimensionInput{Name: "颜色", Values: []SKUValueInput{{Name: "红"}, {Name: "蓝"}}}))
	if err != nil {
		t.Fatal(err)
	}
	wanted := first.SKUs[1].ID
	project := CommerceProject{UserID: 7, ProductID: product.ID, DefaultSKUID: &wanted, Title: "项目", Pipeline: "general", Status: "active"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	got, err := s.GetSKUConfig(context.Background(), 7, product.ID)
	if err != nil || !got.DefaultKnown || got.DefaultSKUID != wanted {
		t.Fatalf("get=%+v err=%v", got, err)
	}
	applied, err := s.ApplySKUMatrix(context.Background(), 7, product.ID, "keep-default", matrixInput(1, SKUDimensionInput{Name: "颜色", Values: []SKUValueInput{{Name: "红"}, {Name: "蓝"}}}))
	if err != nil || !applied.DefaultKnown || applied.DefaultSKUID != wanted {
		t.Fatalf("apply=%+v err=%v", applied, err)
	}
}

func TestSKUConfigOmitsAmbiguousProjectDefault(t *testing.T) {
	s, db, product := newSKUMatrixTestService(t)
	config, err := s.ApplySKUMatrix(context.Background(), 7, product.ID, "seed-ambiguous", matrixInput(0, SKUDimensionInput{Name: "颜色", Values: []SKUValueInput{{Name: "红"}, {Name: "蓝"}}}))
	if err != nil {
		t.Fatal(err)
	}
	for i := range config.SKUs {
		id := config.SKUs[i].ID
		if err := db.Create(&CommerceProject{UserID: 7, ProductID: product.ID, DefaultSKUID: &id, Title: fmt.Sprintf("项目%d", i), Pipeline: "general", Status: "active"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.GetSKUConfig(context.Background(), 7, product.ID)
	if err != nil || got.DefaultKnown {
		t.Fatalf("get=%+v err=%v", got, err)
	}
}

func TestCreateAndPatchSKUAdvanceVersionAndInvalidateMatrixVersion(t *testing.T) {
	s, db, product := newSKUMatrixTestService(t)
	sku, err := s.CreateSKU(context.Background(), 7, product.ID, CreateSKUInput{Code: "MANUAL"})
	if err != nil {
		t.Fatal(err)
	}
	color := "blue"
	if _, err := s.PatchSKU(context.Background(), 7, sku.ID, PatchSKUInput{Color: &color}); err != nil {
		t.Fatal(err)
	}
	disabled := "disabled"
	if _, err := s.PatchSKU(context.Background(), 7, sku.ID, PatchSKUInput{Status: &disabled}); err != nil {
		t.Fatal(err)
	}
	active := "active"
	if _, err := s.PatchSKU(context.Background(), 7, sku.ID, PatchSKUInput{Status: &active}); err != nil {
		t.Fatal(err)
	}
	attributes := `{"material":"cotton"}`
	if _, err := s.PatchSKU(context.Background(), 7, sku.ID, PatchSKUInput{AttributesJSON: &attributes}); err != nil {
		t.Fatal(err)
	}
	var stored CommerceProduct
	if err := db.First(&stored, product.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.SKUVersion != 5 {
		t.Fatalf("version=%d", stored.SKUVersion)
	}
	if _, err := s.ApplySKUMatrix(context.Background(), 7, product.ID, "stale-after-patch", matrixInput(0)); !errors.Is(err, ErrSKUVersionConflict) {
		t.Fatalf("apply err=%v", err)
	}
}

func TestPatchProjectCannotSetDisabledSKUAsDefault(t *testing.T) {
	s, db, p := newSKUMatrixTestService(t)
	sku := CommerceSKU{UserID: 7, ProductID: p.ID, Code: "OFF", Status: "disabled"}
	if err := db.Create(&sku).Error; err != nil {
		t.Fatal(err)
	}
	project := CommerceProject{UserID: 7, ProductID: p.ID, Title: "项目", Pipeline: "general", Status: "active"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.PatchProject(context.Background(), 7, project.ID, PatchProjectInput{DefaultSKUID: &sku.ID}); !errors.Is(err, ErrDefaultSKUDisable) {
		t.Fatalf("err=%v", err)
	}
}

func TestCreateProjectRacesMatrixDisableWithoutInvalidDefault(t *testing.T) {
	s, db, p := newSKUMatrixTestService(t)
	first, err := s.ApplySKUMatrix(context.Background(), 7, p.ID, "seed-race", matrixInput(0, SKUDimensionInput{Name: "颜色", Values: []SKUValueInput{{Name: "红"}}}))
	if err != nil {
		t.Fatal(err)
	}
	skuID := first.SKUs[0].ID
	errs := runConcurrentMatrix(t, func() error {
		_, e := s.CreateProject(context.Background(), 7, CreateProjectInput{ProductID: p.ID, DefaultSKUID: &skuID, Title: "并发项目", Pipeline: "general"})
		return e
	}, func() error {
		_, e := s.ApplySKUMatrix(context.Background(), 7, p.ID, "disable-race", matrixInput(1))
		return e
	})
	assertNoDisabledDefault(t, db, p.ID, errs)
}

func TestPatchSKURacesDefaultSwitchWithoutInvalidDefault(t *testing.T) {
	s, db, p := newSKUMatrixTestService(t)
	sku := CommerceSKU{UserID: 7, ProductID: p.ID, Code: "RACE", Status: "active"}
	if err := db.Create(&sku).Error; err != nil {
		t.Fatal(err)
	}
	project := CommerceProject{UserID: 7, ProductID: p.ID, Title: "项目", Pipeline: "general", Status: "active"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	disabled := "disabled"
	errs := runConcurrentMatrix(t, func() error {
		_, e := s.PatchProject(context.Background(), 7, project.ID, PatchProjectInput{DefaultSKUID: &sku.ID})
		return e
	}, func() error {
		_, e := s.PatchSKU(context.Background(), 7, sku.ID, PatchSKUInput{Status: &disabled})
		return e
	})
	assertNoDisabledDefault(t, db, p.ID, errs)
}

func assertNoDisabledDefault(t *testing.T, db *gorm.DB, productID uint, errs []error) {
	t.Helper()
	for _, e := range errs {
		if e != nil && !errors.Is(e, ErrDefaultSKUDisable) {
			t.Fatalf("unexpected race error=%v all=%v", e, errs)
		}
	}
	var count int64
	if err := db.Table("commerce_projects p").Joins("JOIN commerce_skus s ON s.id=p.default_sku_id").Where("p.product_id=? AND s.status='disabled'", productID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("disabled defaults=%d errors=%v", count, errs)
	}
}

func itoa(value uint) string {
	const digits = "0123456789"
	if value == 0 {
		return "0"
	}
	b := make([]byte, 0, 10)
	for value > 0 {
		b = append([]byte{digits[value%10]}, b...)
		value /= 10
	}
	return string(b)
}
