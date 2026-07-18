package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

func TestContentReportLifecycleAndAdminComplianceExport(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, userCookies := createLoggedInUser(t, testApp, "compliance-user", "test-password")
	adminCookies := createAdminSession(t, testApp)

	generation := GenerationRecord{
		UserID:            user.ID,
		Prompt:            "生成合规测试图片",
		Status:            GenerationStatusSucceeded,
		Stage:             GenerationStageSucceeded,
		Model:             "gpt-image-2",
		ProviderRequestID: "req-compliance-001",
		PreviewURL:        "/api/works/1/file",
		DownloadURL:       "/api/works/1/download",
	}
	if err := db.Create(&generation).Error; err != nil {
		t.Fatalf("seed generation: %v", err)
	}

	reportResp := performJSONRequest(t, testApp, http.MethodPost, "/api/content-reports", map[string]any{
		"target_type":          "generation",
		"target_id":            generation.ID,
		"reason":               "疑似违规内容",
		"description":          "公开分享页包含不适合公开传播的内容",
		"contact":              "creator@example.com",
		"generation_record_id": generation.ID,
	}, userCookies)
	if reportResp.Code != http.StatusCreated {
		t.Fatalf("expected content report create 201, got %d: %s", reportResp.Code, reportResp.Body.String())
	}
	var reportPayload struct {
		ID     uint   `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(reportResp.Body.Bytes(), &reportPayload); err != nil {
		t.Fatalf("decode report payload: %v", err)
	}
	if reportPayload.ID == 0 || reportPayload.Status != ContentReportStatusPending {
		t.Fatalf("unexpected report payload: %+v", reportPayload)
	}

	reviewsResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/content-reviews?status=pending&page=1&page_size=10", nil, adminCookies)
	if reviewsResp.Code != http.StatusOK {
		t.Fatalf("expected content reviews 200, got %d: %s", reviewsResp.Code, reviewsResp.Body.String())
	}
	if !bytes.Contains(reviewsResp.Body.Bytes(), []byte(`"risk_level":"medium"`)) ||
		!bytes.Contains(reviewsResp.Body.Bytes(), []byte(`"generation_record_id":`)) {
		t.Fatalf("expected pending review with generation trace, got %s", reviewsResp.Body.String())
	}

	var reviewList struct {
		Items []struct {
			ID uint `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(reviewsResp.Body.Bytes(), &reviewList); err != nil {
		t.Fatalf("decode review list: %v", err)
	}
	if len(reviewList.Items) != 1 {
		t.Fatalf("expected one review item, got %+v", reviewList.Items)
	}

	patchResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/admin/content-reviews/"+itoa(reviewList.Items[0].ID), map[string]any{
		"status":  "reject",
		"action":  "下架公开分享",
		"comment": "举报属实，已限制传播",
	}, adminCookies)
	if patchResp.Code != http.StatusOK {
		t.Fatalf("expected content review patch 200, got %d: %s", patchResp.Code, patchResp.Body.String())
	}

	reportsResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/content-reports?status=resolved&page=1&page_size=10", nil, adminCookies)
	if reportsResp.Code != http.StatusOK {
		t.Fatalf("expected admin reports 200, got %d: %s", reportsResp.Code, reportsResp.Body.String())
	}
	if !bytes.Contains(reportsResp.Body.Bytes(), []byte(`"status":"resolved"`)) ||
		!bytes.Contains(reportsResp.Body.Bytes(), []byte(`"resolution":"下架公开分享"`)) {
		t.Fatalf("expected resolved report payload, got %s", reportsResp.Body.String())
	}

	disclosureResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/algorithm-disclosure", nil, adminCookies)
	if disclosureResp.Code != http.StatusOK {
		t.Fatalf("expected disclosure 200, got %d: %s", disclosureResp.Code, disclosureResp.Body.String())
	}
	if !bytes.Contains(disclosureResp.Body.Bytes(), []byte("白霖共享图片生成合成服务算法")) {
		t.Fatalf("expected default disclosure, got %s", disclosureResp.Body.String())
	}

	updateDisclosureResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/admin/algorithm-disclosure", map[string]any{
		"algorithm_name":       "白霖共享图片生成合成服务算法",
		"algorithm_type":       "生成合成类",
		"service_description":  "面向用户提供文生图、参考图合成和局部编辑能力",
		"provider_description": "平台调用第三方生成模型，不训练基础模型",
		"disclosure_version":   "2026-06-04",
		"status":               "published",
	}, adminCookies)
	if updateDisclosureResp.Code != http.StatusOK {
		t.Fatalf("expected disclosure update 200, got %d: %s", updateDisclosureResp.Code, updateDisclosureResp.Body.String())
	}

	exportResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/algorithm-compliance/export", nil, adminCookies)
	if exportResp.Code != http.StatusOK {
		t.Fatalf("expected compliance export 200, got %d: %s", exportResp.Code, exportResp.Body.String())
	}
	if !bytes.Contains(exportResp.Body.Bytes(), []byte(`"content_review_summary"`)) ||
		!bytes.Contains(exportResp.Body.Bytes(), []byte(`"algorithm_disclosure"`)) ||
		!bytes.Contains(exportResp.Body.Bytes(), []byte(`"generation_trace_samples"`)) {
		t.Fatalf("expected compliance export evidence sections, got %s", exportResp.Body.String())
	}
}

func TestAlgorithmComplianceRBACSeedsAndMenus(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})

	if err := testApp.seedPermissionsAndRoles(); err != nil {
		t.Fatalf("seed permissions: %v", err)
	}

	expectedPermissions := []string{
		"content_reviews.read",
		"content_reviews.update",
		"content_reports.read",
		"content_reports.update",
		"algorithm_compliance.read",
		"algorithm_compliance.update",
		"algorithm_incidents.read",
		"algorithm_incidents.update",
	}
	for _, code := range expectedPermissions {
		var count int64
		if err := db.Model(&Permission{}).Where("code = ?", code).Count(&count).Error; err != nil {
			t.Fatalf("count permission %s: %v", code, err)
		}
		if count != 1 {
			t.Fatalf("expected permission %s to be seeded", code)
		}
	}

	adminCookies := createAdminSession(t, testApp)
	meResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/me", nil, adminCookies)
	if meResp.Code != http.StatusOK {
		t.Fatalf("expected admin me 200, got %d: %s", meResp.Code, meResp.Body.String())
	}
	for _, path := range []string{"/admin/content-reviews", "/admin/content-reports", "/admin/algorithm-compliance", "/admin/incidents"} {
		if !bytes.Contains(meResp.Body.Bytes(), []byte(path)) {
			t.Fatalf("expected admin menu to include %s, got %s", path, meResp.Body.String())
		}
	}
}

func TestWorkCannotBePublishedWhenComplianceReviewIsUnresolved(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	owner, ownerCookies := createLoggedInUser(t, testApp, "review_block_owner", "test-password")
	work := seedSucceededWork(t, testApp, owner.ID, "待复核公开图", "1:1")

	review := ContentSafetyReview{
		ReviewType:         ContentReviewTypeShare,
		Status:             ContentSafetyStatusPending,
		RiskLevel:          "high",
		Reason:             "公开分享前需要人工复核",
		TargetType:         "work",
		TargetID:           work.ID,
		GenerationRecordID: &work.GenerationRecordID,
		WorkID:             &work.ID,
		UserID:             &owner.ID,
		InputSummary:       work.Prompt,
	}
	if err := db.Create(&review).Error; err != nil {
		t.Fatalf("seed content review: %v", err)
	}

	publishResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/works/"+itoa(work.ID), map[string]any{
		"visibility": WorkVisibilityPublic,
	}, ownerCookies)
	if publishResp.Code != http.StatusForbidden {
		t.Fatalf("expected unresolved review to block publishing, got %d: %s", publishResp.Code, publishResp.Body.String())
	}
	if !bytes.Contains(publishResp.Body.Bytes(), []byte("work_review_required")) {
		t.Fatalf("expected work_review_required error, got %s", publishResp.Body.String())
	}

	if err := db.Model(&review).Update("status", ContentSafetyStatusPass).Error; err != nil {
		t.Fatalf("pass review: %v", err)
	}
	allowedResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/works/"+itoa(work.ID), map[string]any{
		"visibility": WorkVisibilityPublic,
	}, ownerCookies)
	if allowedResp.Code != http.StatusOK {
		t.Fatalf("expected passed review to allow publishing, got %d: %s", allowedResp.Code, allowedResp.Body.String())
	}
}

func TestRegistrationRecordsComplianceConsents(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, _ := createLoggedInUser(t, testApp, "consent_user", "test-password")

	var consents []UserConsent
	if err := db.Where("user_id = ?", user.ID).Order("consent_type asc").Find(&consents).Error; err != nil {
		t.Fatalf("load consents: %v", err)
	}
	if len(consents) != 3 {
		t.Fatalf("expected three compliance consents, got %+v", consents)
	}
	types := map[string]bool{}
	for _, consent := range consents {
		types[consent.ConsentType] = true
		if consent.Version != "2026-06-04" || consent.Source != "register" {
			t.Fatalf("unexpected consent payload: %+v", consent)
		}
	}
	for _, consentType := range []string{"terms", "privacy", "algorithm_disclosure"} {
		if !types[consentType] {
			t.Fatalf("missing consent type %s in %+v", consentType, consents)
		}
	}
}
