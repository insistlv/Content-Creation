package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMomentsMarketingPlanRequiresLogin(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/marketing/moments/plan", map[string]any{
		"input_mode":   "text",
		"output_type":  "copy_image_separate",
		"image_count":  3,
		"product_name": "社区咖啡店",
	}, nil)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated request to return 401, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestMomentsMarketingPlanValidatesRequiredFieldsAndImageCount(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	_, cookies := createLoggedInUser(t, testApp, "moments_validation", "test-password")

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/marketing/moments/plan", map[string]any{
		"input_mode":   "story",
		"output_type":  "copy_image_separate",
		"image_count":  10,
		"product_name": "社区咖啡店",
	}, cookies)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid request 400, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "invalid_marketing_parameter") {
		t.Fatalf("expected invalid_marketing_parameter, got %s", resp.Body.String())
	}
}

func TestMomentsMarketingPlanRejectsInvalidDeepSeekJSON(t *testing.T) {
	deepSeek := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"不是 JSON"}}]}`))
	}))
	defer deepSeek.Close()

	testApp, _ := newTestApp(t, &stubProvider{})
	testApp.cfg.DeepSeekAPIKey = "deepseek-test"
	testApp.cfg.DeepSeekBaseURL = deepSeek.URL
	_, cookies := createLoggedInUser(t, testApp, "moments_bad_json", "test-password")

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/marketing/moments/plan", map[string]any{
		"input_mode":      "text",
		"output_type":     "poster_overlay",
		"image_count":     2,
		"product_name":    "巷口咖啡",
		"selling_points":  "现磨、低糖甜点",
		"target_audience": "附近上班族",
		"promotion":       "第二杯半价",
		"cta":             "私信预约",
	}, cookies)

	if resp.Code != http.StatusBadGateway {
		t.Fatalf("expected invalid DeepSeek JSON 502, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "marketing_plan_failed") {
		t.Fatalf("expected marketing_plan_failed, got %s", resp.Body.String())
	}
}

func TestMomentsMarketingPlanReturnsRequestedImageCards(t *testing.T) {
	var captured map[string]any
	deepSeek := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode deepseek request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"moments_text\":\"今天想把这家巷口咖啡推荐给附近朋友。\",\"hashtags\":[\"附近好店\"],\"image_cards\":[{\"slot\":1,\"role\":\"开场\",\"caption\":\"门店氛围\",\"visual_prompt\":\"温暖社区咖啡店门头，真实商业摄影，无文字\",\"overlay_title\":\"巷口咖啡\",\"overlay_subtitle\":\"现磨咖啡和低糖甜点\",\"overlay_badge\":\"第二杯半价\",\"cta\":\"私信预约\",\"layout\":\"bottom_gradient\"},{\"slot\":2,\"role\":\"产品\",\"caption\":\"招牌拿铁\",\"visual_prompt\":\"木桌上的热拿铁和甜点，自然光，无文字\",\"overlay_title\":\"招牌拿铁\",\"overlay_subtitle\":\"工作日下午补能\",\"overlay_badge\":\"限时优惠\",\"cta\":\"到店试试\",\"layout\":\"bottom_gradient\"}],\"safety_notes\":[\"文案避免绝对化承诺\"]}"}}]}`))
	}))
	defer deepSeek.Close()

	testApp, _ := newTestApp(t, &stubProvider{})
	testApp.cfg.DeepSeekAPIKey = "deepseek-test"
	testApp.cfg.DeepSeekBaseURL = deepSeek.URL
	_, cookies := createLoggedInUser(t, testApp, "moments_success", "test-password")

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/marketing/moments/plan", map[string]any{
		"input_mode":      "text",
		"output_type":     "poster_overlay",
		"image_count":     2,
		"brief":           "帮我发朋友圈推广社区咖啡店",
		"product_name":    "巷口咖啡",
		"selling_points":  "现磨、低糖甜点",
		"target_audience": "附近上班族",
		"promotion":       "第二杯半价",
		"tone":            "自然亲切",
		"cta":             "私信预约",
	}, cookies)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected plan 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload momentsMarketingPlanResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.MomentsText == "" || len(payload.ImageCards) != 2 {
		t.Fatalf("expected moments text and 2 image cards, got %+v", payload)
	}
	if payload.ImageCards[0].Slot != 1 || payload.ImageCards[1].VisualPrompt == "" {
		t.Fatalf("expected normalized image cards, got %+v", payload.ImageCards)
	}
	messages, _ := captured["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("expected DeepSeek chat messages, got %+v", captured)
	}
}
