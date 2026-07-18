package app

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestCommerceCategoryUserAPIs(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	_, cookies := createLoggedInUser(t, testApp, "category-user", "password123")

	listed := performJSONRequest(t, testApp, http.MethodGet, "/api/ecommerce/categories", nil, cookies)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), "家居日用") || !strings.Contains(listed.Body.String(), "杯壶餐具") {
		t.Fatalf("list categories = %d: %s", listed.Code, listed.Body.String())
	}
	var payload struct {
		SystemCategories []struct {
			ID   uint   `json:"id"`
			Name string `json:"name"`
		} `json:"system_categories"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	var parentID uint
	for _, item := range payload.SystemCategories {
		if item.Name == "家居日用" {
			parentID = item.ID
		}
	}
	created := performJSONRequest(t, testApp, http.MethodPost, "/api/ecommerce/categories/custom", map[string]any{"parent_id": parentID, "name": "咖啡器具"}, cookies)
	if created.Code != http.StatusCreated || !strings.Contains(created.Body.String(), "咖啡器具") {
		t.Fatalf("create custom = %d: %s", created.Code, created.Body.String())
	}
	var custom struct {
		ID uint `json:"id"`
	}
	_ = json.Unmarshal(created.Body.Bytes(), &custom)
	patched := performJSONRequest(t, testApp, http.MethodPatch, "/api/ecommerce/categories/custom/"+itoa(custom.ID), map[string]any{"status": "inactive"}, cookies)
	if patched.Code != http.StatusOK || !strings.Contains(patched.Body.String(), "inactive") {
		t.Fatalf("patch custom = %d: %s", patched.Code, patched.Body.String())
	}
}

func TestAdminCommerceCategoryAPIsAuditMutations(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	admin := createDatabaseAdminUser(t, db, "category-admin", "AdminPass123")
	assignAdminRoleByCode(t, db, &admin, "super_admin")
	cookies := loginAdminAs(t, testApp, admin.Username, "AdminPass123")

	listed := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/ecommerce/categories", nil, cookies)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), "家居日用") {
		t.Fatalf("admin list = %d: %s", listed.Code, listed.Body.String())
	}
	created := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/ecommerce/categories", map[string]any{"level": 1, "name": "测试大类", "sort_order": 999, "aliases": []string{"测试"}}, cookies)
	if created.Code != http.StatusCreated {
		t.Fatalf("admin create = %d: %s", created.Code, created.Body.String())
	}
	var category struct {
		ID uint `json:"id"`
	}
	_ = json.Unmarshal(created.Body.Bytes(), &category)
	patched := performJSONRequest(t, testApp, http.MethodPatch, "/api/admin/ecommerce/categories/"+itoa(category.ID), map[string]any{"status": "inactive"}, cookies)
	if patched.Code != http.StatusOK {
		t.Fatalf("admin patch = %d: %s", patched.Code, patched.Body.String())
	}
	var audits int64
	if err := db.Model(&AdminAuditLog{}).Where("target_type = ? AND target_id = ?", "commerce_category", category.ID).Count(&audits).Error; err != nil || audits != 2 {
		t.Fatalf("audit count = %d, %v", audits, err)
	}
}
