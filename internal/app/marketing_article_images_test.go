package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestArticleImagesPlanRequiresLogin(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/marketing/article-images/plan", map[string]any{
		"title":       "新品发布文章",
		"body":        "这是一篇公众号正文，需要生成封面和段落配图。",
		"image_count": 3,
	}, nil)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated request to return 401, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestArticleImagesPlanValidatesBodyAndImageCount(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	_, cookies := createLoggedInUser(t, testApp, "article_images_validation", "test-password")

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/marketing/article-images/plan", map[string]any{
		"title":       "空正文",
		"body":        "",
		"image_count": 13,
	}, cookies)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid request 400, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "invalid_article_images_parameter") {
		t.Fatalf("expected invalid_article_images_parameter, got %s", resp.Body.String())
	}
}

func TestArticleImagesPlanRejectsInvalidDeepSeekJSON(t *testing.T) {
	deepSeek := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"不是 JSON"}}]}`))
	}))
	defer deepSeek.Close()

	testApp, _ := newTestApp(t, &stubProvider{})
	testApp.cfg.DeepSeekAPIKey = "deepseek-test"
	testApp.cfg.DeepSeekBaseURL = deepSeek.URL
	_, cookies := createLoggedInUser(t, testApp, "article_images_bad_json", "test-password")

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/marketing/article-images/plan", map[string]any{
		"title":         "活动复盘",
		"body":          "第一段介绍背景。第二段说明方法。第三段总结成果。",
		"article_type":  "教程攻略",
		"audience":      "品牌运营",
		"style":         "清爽专业",
		"image_count":   2,
		"include_cover": true,
	}, cookies)

	if resp.Code != http.StatusBadGateway {
		t.Fatalf("expected invalid DeepSeek JSON 502, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "article_images_plan_failed") {
		t.Fatalf("expected article_images_plan_failed, got %s", resp.Body.String())
	}
}

func TestArticleImagesPlanReturnsRequestedImageCards(t *testing.T) {
	var captured map[string]any
	deepSeek := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode deepseek request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"article_summary\":\"文章讲解品牌活动从预热到转化的完整方法。\",\"image_cards\":[{\"slot\":1,\"role\":\"封面图\",\"placement\":\"文章开头\",\"caption\":\"活动增长方法论\",\"visual_prompt\":\"清爽专业的公众号封面插画，品牌运营团队围绕增长看板讨论，蓝白配色，无文字，无水印\",\"aspect_ratio\":\"16:9\",\"overlay_title\":\"活动增长方法论\",\"layout\":\"cover_overlay\"},{\"slot\":2,\"role\":\"流程/步骤图\",\"placement\":\"第二个小标题后\",\"caption\":\"三步拆解活动流程\",\"visual_prompt\":\"极简流程图风格场景，预热、转化、复盘三个阶段的抽象图形表达，无中文文字，无水印\",\"aspect_ratio\":\"1:1\",\"overlay_title\":\"三步拆解活动流程\",\"layout\":\"step_card\"}],\"safety_notes\":[\"标题文字由前端叠加，图片 prompt 不要求生成中文大字\"]}"}}]}`))
	}))
	defer deepSeek.Close()

	testApp, _ := newTestApp(t, &stubProvider{})
	testApp.cfg.DeepSeekAPIKey = "deepseek-test"
	testApp.cfg.DeepSeekBaseURL = deepSeek.URL
	_, cookies := createLoggedInUser(t, testApp, "article_images_success", "test-password")

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/marketing/article-images/plan", map[string]any{
		"title":         "活动增长方法论",
		"body":          "第一段介绍活动背景。第二段说明三步流程。第三段总结复盘方法。",
		"article_type":  "教程攻略",
		"audience":      "品牌运营",
		"style":         "清爽专业",
		"image_count":   2,
		"include_cover": true,
	}, cookies)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected plan 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload articleImagesPlanResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.ArticleSummary == "" || len(payload.ImageCards) != 2 {
		t.Fatalf("expected summary and 2 image cards, got %+v", payload)
	}
	if payload.ImageCards[0].AspectRatio != "16:9" || payload.ImageCards[1].OverlayTitle == "" {
		t.Fatalf("expected normalized article image cards, got %+v", payload.ImageCards)
	}
	if strings.Contains(payload.ImageCards[0].VisualPrompt, "生成中文大字") {
		t.Fatalf("visual prompt should not ask image model to render Chinese text: %s", payload.ImageCards[0].VisualPrompt)
	}
	messages, _ := captured["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("expected DeepSeek chat messages, got %+v", captured)
	}
}

func TestArticleImagesPlanRejectsReferenceAssetsOwnedByOtherUsers(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	_, cookies := createLoggedInUser(t, testApp, "article_images_owner_a", "test-password")
	otherUser, _ := createLoggedInUser(t, testApp, "article_images_owner_b", "test-password")
	asset := ReferenceAsset{
		UserID:           otherUser.ID,
		AssetKey:         "reference-assets/other-user.png",
		PreviewURL:       "/api/reference-assets/99/file",
		MIMEType:         "image/png",
		OriginalFilename: "other-user.png",
		DisplayName:      "其他用户素材",
	}
	if err := testApp.db.Create(&asset).Error; err != nil {
		t.Fatalf("create reference asset: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/marketing/article-images/plan", map[string]any{
		"title":               "品牌故事",
		"body":                "这是一篇品牌故事文章，需要参考图保持人物与产品一致。",
		"image_count":         2,
		"reference_asset_ids": []uint{asset.ID},
	}, cookies)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid reference asset 400, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "invalid_article_images_parameter") {
		t.Fatalf("expected invalid_article_images_parameter, got %s", resp.Body.String())
	}
}
