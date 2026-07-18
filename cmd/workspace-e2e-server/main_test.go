package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"testing"
	"time"

	"image-agent/internal/app"
	"image-agent/internal/app/ecommerce"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCommerceFoundationSeed(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:commerce-foundation-seed?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&app.User{}, &app.UserRole{}, &app.CreditBalance{}, &app.CreditTransaction{}, &app.GenerationRecord{}, &app.Work{}, &app.ReferenceAsset{}); err != nil {
		t.Fatal(err)
	}
	if err := ecommerce.MigrateSQLiteFoundationSchema(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	role := app.UserRole{Code: "standard_user", Name: "标准用户"}
	if err := db.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	if err := seedWorkspaceE2E(db, t.TempDir()); err != nil {
		t.Fatal(err)
	}

	for table, want := range map[string]int64{
		"commerce_projects":           1,
		"commerce_generation_batches": 1,
		"commerce_generation_items":   2,
		"commerce_credit_settlements": 1,
		"commerce_assets":             1,
		"commerce_creative_specs":     1,
	} {
		var got int64
		if err := db.Table(table).Count(&got).Error; err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s count=%d, want %d", table, got, want)
		}
	}
	var failed ecommerce.CommerceGenerationItem
	if err := db.Where("status = ?", "failed").First(&failed).Error; err != nil {
		t.Fatal(err)
	}
	if failed.ReleasedCredits != failed.ReservedCredits {
		t.Fatalf("failed item released/reserved credits=%d/%d, want full release", failed.ReleasedCredits, failed.ReservedCredits)
	}
}

func TestCommerceFoundationSubmitTriggersFakeExecution(t *testing.T) {
	t.Setenv("PORT", "18989")
	go main()
	base := "http://127.0.0.1:18989"
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 3 * time.Second}
	deadline := time.Now().Add(20 * time.Second)
	for {
		resp, err := client.Get(base + "/api/workspace/discovery")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("e2e server did not start: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	doJSON := func(method, path string, input any, output any, headers map[string]string) int {
		t.Helper()
		payload, _ := json.Marshal(input)
		req, _ := http.NewRequest(method, base+path, bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Image-Agent-Client", "mp-weixin")
		for key, value := range headers {
			req.Header.Set(key, value)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if output != nil {
			_ = json.NewDecoder(resp.Body).Decode(output)
		}
		return resp.StatusCode
	}
	if status := doJSON(http.MethodPost, "/api/auth/login", map[string]any{"username": e2eUsername, "password": e2ePassword}, nil, nil); status != http.StatusOK {
		t.Fatalf("login status=%d", status)
	}
	var projects struct {
		Items []struct {
			ID                   uint  `json:"id"`
			DefaultSKUID         uint  `json:"default_sku_id"`
			ActiveCreativeSpecID *uint `json:"active_creative_spec_id"`
		} `json:"items"`
	}
	if status := doJSON(http.MethodGet, "/api/ecommerce/projects", nil, &projects, nil); status != http.StatusOK || len(projects.Items) == 0 {
		t.Fatalf("projects status=%d items=%d", status, len(projects.Items))
	}
	project := projects.Items[0]
	input := map[string]any{"recipe_key": "workspace-e2e-recipe", "recipe_version": 1, "output_count": 2, "creative_spec_id": *project.ActiveCreativeSpecID, "primary_sku_id": project.DefaultSKUID, "quality_tier": "standard", "aspect_ratio": "1:1"}
	var estimate struct {
		PricingSnapshotID string `json:"pricing_snapshot_id"`
	}
	if status := doJSON(http.MethodPost, fmt.Sprintf("/api/ecommerce/projects/%d/batches/estimate", project.ID), input, &estimate, nil); status != http.StatusOK {
		t.Fatalf("estimate status=%d", status)
	}
	input["pricing_snapshot_id"] = estimate.PricingSnapshotID
	var submitted struct {
		Batch struct {
			ID uint `json:"id"`
		} `json:"batch"`
	}
	if status := doJSON(http.MethodPost, fmt.Sprintf("/api/ecommerce/projects/%d/batches", project.ID), input, &submitted, map[string]string{"Idempotency-Key": "server-test-submit"}); status != http.StatusCreated {
		t.Fatalf("submit status=%d", status)
	}
	for time.Now().Before(deadline) {
		var snapshot struct {
			Batch struct {
				Status    string `json:"status"`
				Succeeded int    `json:"succeeded_items"`
				Failed    int    `json:"failed_items"`
			} `json:"batch"`
		}
		if status := doJSON(http.MethodGet, fmt.Sprintf("/api/ecommerce/batches/%d", submitted.Batch.ID), nil, &snapshot, nil); status == http.StatusOK && snapshot.Batch.Status == "partial_succeeded" {
			if snapshot.Batch.Succeeded != 1 || snapshot.Batch.Failed != 1 {
				t.Fatalf("fake execution counts=%d/%d", snapshot.Batch.Succeeded, snapshot.Batch.Failed)
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("submitted batch was not processed by fake executor")
}

func TestCommerceFoundationFakeExecutorLifecycle(t *testing.T) {
	executor := &e2eCommerceExecutor{workID: 42}
	success, failure := executor.Execute(context.Background(), ecommerce.ItemExecutionRequest{Compiled: ecommerce.CompiledGenerationItem{SlotKey: "foundation-0"}})
	if failure != nil || success.WorkID != 42 {
		t.Fatalf("success execution = %#v, failure=%#v", success, failure)
	}
	_, failure = executor.Execute(context.Background(), ecommerce.ItemExecutionRequest{Compiled: ecommerce.CompiledGenerationItem{SlotKey: "foundation-1"}})
	if failure == nil || failure.Code != "e2e_expected_failure" {
		t.Fatalf("first failure = %#v", failure)
	}
	parentID := uint(1)
	retried, failure := executor.Execute(context.Background(), ecommerce.ItemExecutionRequest{Item: ecommerce.CommerceGenerationItem{ParentItemID: &parentID}, Compiled: ecommerce.CompiledGenerationItem{SlotKey: "foundation-1"}})
	if failure != nil || retried.WorkID != 42 {
		t.Fatalf("retry execution = %#v, failure=%#v", retried, failure)
	}
}
