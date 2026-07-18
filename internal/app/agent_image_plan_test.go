package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAgentImagePlanRequiresLogin(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/agent/image-plan", map[string]any{
		"message": "帮我生成一张商品主图",
	}, nil)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated request to return 401, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestAgentImagePlanReturnsStructuredPlanAndCandidates(t *testing.T) {
	var captured struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	deepSeek := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode deepseek request: %v", err)
		}
		response := map[string]any{
			"reply":               "我整理了两个可执行方向，建议先做电商主图。",
			"needs_clarification": false,
			"plan": map[string]any{
				"title":                 "玻璃香薰电商主图",
				"intent":                "text_to_image",
				"tool_mode":             "generate",
				"prompt":                "透明玻璃香薰瓶居中，浅色背景，商业摄影布光",
				"negative_prompt":       "文字，水印",
				"aspect_ratio":          "1:1",
				"style_preset":          "电商",
				"quality":               "high",
				"reference_weight":      75,
				"tool_options":          map[string]any{},
				"edit_instruction":      "",
				"requires_confirmation": true,
			},
			"candidates": []map[string]any{
				{
					"id":               "commerce-main",
					"title":            "电商主图",
					"prompt":           "透明玻璃香薰瓶居中，浅色背景，商业摄影布光",
					"aspect_ratio":     "1:1",
					"style_preset":     "电商",
					"quality":          "high",
					"tool_mode":        "generate",
					"tool_options":     map[string]any{},
					"reference_weight": 75,
				},
				{
					"id":           "poster-kv",
					"title":        "海报 KV",
					"prompt":       "透明玻璃香薰瓶与光影背景，海报构图",
					"aspect_ratio": "3:4",
					"style_preset": "海报",
					"quality":      "medium",
					"tool_mode":    "generate",
				},
			},
			"safety_notes": []string{"已规避品牌标识"},
		}
		content, _ := json.Marshal(response)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, string(content))
	}))
	defer deepSeek.Close()

	testApp, _ := newTestApp(t, &stubProvider{})
	testApp.cfg.DeepSeekAPIKey = "deepseek-test"
	testApp.cfg.DeepSeekBaseURL = deepSeek.URL
	testApp.cfg.DeepSeekPromptModel = "deepseek-v4"
	_, cookies := createLoggedInUser(t, testApp, "agent_plan_success", "test-password")

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/agent/image-plan", map[string]any{
		"message":             "帮我做一张香薰电商主图",
		"reference_asset_ids": []uint{42},
		"history": []map[string]string{
			{"role": "user", "content": "先做商品图"},
		},
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected plan 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload imageAgentPlanResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if captured.Model != "deepseek-v4" || len(captured.Messages) != 2 {
		t.Fatalf("expected DeepSeek chat request, got %+v", captured)
	}
	if !strings.Contains(captured.Messages[0].Content, "创作任务代理") ||
		!strings.Contains(captured.Messages[0].Content, "未经确认不得自动扣点") ||
		!strings.Contains(captured.Messages[1].Content, "reference_asset_ids") {
		t.Fatalf("expected agent planning and safety instructions, got %+v", captured.Messages)
	}
	if payload.Plan.Title != "玻璃香薰电商主图" || payload.Plan.ToolMode != GenerationToolModeGenerate {
		t.Fatalf("unexpected plan: %+v", payload.Plan)
	}
	if len(payload.Candidates) != 2 || payload.Candidates[1].AspectRatio != "3:4" {
		t.Fatalf("unexpected candidates: %+v", payload.Candidates)
	}
	if len(payload.SafetyNotes) == 0 {
		t.Fatalf("expected safety notes")
	}
}

func TestAgentImagePlanSanitizesUnsafePlanPrompt(t *testing.T) {
	deepSeek := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]any{
			"reply": "已改写为安全方案。",
			"plan": map[string]any{
				"title":        "安全海报",
				"intent":       "text_to_image",
				"tool_mode":    "generate",
				"prompt":       "血腥暴力 裸露 商品海报",
				"aspect_ratio": "1:1",
				"quality":      "medium",
			},
		}
		content, _ := json.Marshal(response)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, string(content))
	}))
	defer deepSeek.Close()

	testApp, _ := newTestApp(t, &stubProvider{})
	testApp.cfg.DeepSeekAPIKey = "deepseek-test"
	testApp.cfg.DeepSeekBaseURL = deepSeek.URL
	_, cookies := createLoggedInUser(t, testApp, "agent_plan_safety", "test-password")

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/agent/image-plan", map[string]any{
		"message": "血腥暴力 裸露 商品海报",
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected plan 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload imageAgentPlanResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	for _, unsafe := range []string{"血腥", "暴力", "裸露"} {
		if strings.Contains(payload.Plan.Prompt, unsafe) {
			t.Fatalf("plan prompt still contains unsafe term %q: %q", unsafe, payload.Plan.Prompt)
		}
	}
	if len(payload.SafetyNotes) == 0 {
		t.Fatalf("expected sanitizer notes")
	}
}

func TestAgentImagePlanReportsDeepSeekFailureWithoutCreatingGeneration(t *testing.T) {
	deepSeek := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	}))
	defer deepSeek.Close()

	testApp, _ := newTestApp(t, &stubProvider{})
	testApp.cfg.DeepSeekAPIKey = "deepseek-test"
	testApp.cfg.DeepSeekBaseURL = deepSeek.URL
	_, cookies := createLoggedInUser(t, testApp, "agent_plan_failure", "test-password")

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/agent/image-plan", map[string]any{
		"message": "帮我做一张商品主图",
	}, cookies)

	if resp.Code != http.StatusBadGateway {
		t.Fatalf("expected DeepSeek failure 502, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "agent_image_plan_failed") {
		t.Fatalf("expected agent_image_plan_failed, got %s", resp.Body.String())
	}
}
