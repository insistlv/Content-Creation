package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestVideoConversationCreateListPatchAndOwnership(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	_, cookies := createLoggedInUser(t, testApp, "video_conversation_user", "test-password")
	_, otherCookies := createLoggedInUser(t, testApp, "video_conversation_other", "test-password")

	created := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/conversations", map[string]any{"title": "香水产品片"}, cookies)
	if created.Code != http.StatusCreated {
		t.Fatalf("create conversation: %d %s", created.Code, created.Body.String())
	}
	var item VideoConversation
	if err := json.Unmarshal(created.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	if item.Title != "香水产品片" || item.UserID == 0 {
		t.Fatalf("unexpected conversation: %+v", item)
	}

	list := performJSONRequest(t, testApp, http.MethodGet, "/api/videos/conversations?q=香水", nil, cookies)
	if list.Code != http.StatusOK || !json.Valid(list.Body.Bytes()) {
		t.Fatalf("list conversation: %d %s", list.Code, list.Body.String())
	}
	if list.Body.String() == "" || !containsJSONText(list.Body.String(), "香水产品片") {
		t.Fatalf("missing conversation: %s", list.Body.String())
	}

	patched := performJSONRequest(t, testApp, http.MethodPatch, "/api/videos/conversations/"+strconv.FormatUint(uint64(item.ID), 10), map[string]any{"is_favorite": true, "title": "高端香水宣传片"}, cookies)
	if patched.Code != http.StatusOK || !containsJSONText(patched.Body.String(), "高端香水宣传片") {
		t.Fatalf("patch conversation: %d %s", patched.Code, patched.Body.String())
	}

	forbidden := performJSONRequest(t, testApp, http.MethodGet, "/api/videos/conversations/"+strconv.FormatUint(uint64(item.ID), 10), nil, otherCookies)
	if forbidden.Code != http.StatusNotFound {
		t.Fatalf("cross user access must be 404, got %d", forbidden.Code)
	}
}

func TestVideoConversationListFiltersSearchMessagesAndStatuses(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "video_filter_user", "test-password")
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	items := []VideoConversation{
		{UserID: user.ID, Title: "收藏任务", IsFavorite: true, LastActivityAt: now},
		{UserID: user.ID, Title: "消息搜索", LastActivityAt: now.Add(-time.Minute)},
		{UserID: user.ID, Title: "提示词搜索", LastActivityAt: now.Add(-2 * time.Minute)},
		{UserID: user.ID, Title: "昨天任务", LastActivityAt: startOfDay.Add(-time.Minute)},
	}
	for index := range items {
		if err := db.Create(&items[index]).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&VideoConversationMessage{ConversationID: items[1].ID, UserID: user.ID, Role: "user", Content: "消息命中关键字", Status: "answered"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, record := range []VideoGenerationRecord{
		{GenerationRecordID: 101, ConversationID: &items[0].ID, UserID: user.ID, Status: "queued", Prompt: "排队任务"},
		{GenerationRecordID: 102, ConversationID: &items[2].ID, UserID: user.ID, Status: "succeeded", Prompt: "提示词命中关键字"},
	} {
		if err := db.Create(&record).Error; err != nil {
			t.Fatal(err)
		}
	}

	for path, expected := range map[string]string{
		"/api/videos/conversations?q=消息命中":         "消息搜索",
		"/api/videos/conversations?q=提示词命中":        "提示词搜索",
		"/api/videos/conversations?favorite=true":  "收藏任务",
		"/api/videos/conversations?status=running": "收藏任务",
	} {
		resp := performJSONRequest(t, testApp, http.MethodGet, path, nil, cookies)
		if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), expected) {
			t.Fatalf("%s: %d %s", path, resp.Code, resp.Body.String())
		}
	}
	today := performJSONRequest(t, testApp, http.MethodGet, "/api/videos/conversations?range=today&page_size=1&page=2", nil, cookies)
	if today.Code != http.StatusOK || strings.Contains(today.Body.String(), "昨天任务") || !strings.Contains(today.Body.String(), `"page":2`) {
		t.Fatalf("today pagination: %d %s", today.Code, today.Body.String())
	}
}

func TestVideoConversationAssistantSuccessIdempotencyRateLimitAndFailure(t *testing.T) {
	assistant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"reply\":\"建议慢推镜头\",\"suggested_prompt\":\"产品慢推镜头\",\"ready_to_generate\":true,\"quick_replies\":[\"改成竖屏\"]}"}}]}`))
	}))
	defer assistant.Close()
	testApp, db := newTestApp(t, &stubProvider{})
	testApp.cfg.DeepSeekAPIKey = "test-key"
	testApp.cfg.DeepSeekBaseURL = assistant.URL
	user, cookies := createLoggedInUser(t, testApp, "video_assistant_user", "test-password")
	conversation := VideoConversation{UserID: user.ID, Title: "新对话", LastActivityAt: time.Now()}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatal(err)
	}
	path := "/api/videos/conversations/" + strconv.FormatUint(uint64(conversation.ID), 10) + "/messages"
	body := map[string]any{"content": "策划产品视频"}
	created := performJSONRequestWithHeaders(t, testApp, http.MethodPost, path, body, cookies, map[string]string{"Idempotency-Key": "assistant-key"})
	if created.Code != http.StatusCreated || !strings.Contains(created.Body.String(), "产品慢推镜头") {
		t.Fatalf("assistant success: %d %s", created.Code, created.Body.String())
	}
	replay := performJSONRequestWithHeaders(t, testApp, http.MethodPost, path, body, cookies, map[string]string{"Idempotency-Key": "assistant-key"})
	if replay.Code != http.StatusOK {
		t.Fatalf("assistant replay: %d %s", replay.Code, replay.Body.String())
	}
	conflict := performJSONRequestWithHeaders(t, testApp, http.MethodPost, path, map[string]any{"content": "不同内容"}, cookies, map[string]string{"Idempotency-Key": "assistant-key"})
	if conflict.Code != http.StatusConflict {
		t.Fatalf("assistant conflict: %d %s", conflict.Code, conflict.Body.String())
	}
	for index := 0; index < videoAssistantRequestsPerMinute; index++ {
		if err := db.Create(&VideoConversationMessage{ConversationID: conversation.ID, UserID: user.ID, Role: "user", Content: "rate", Status: "answered", IdempotencyKey: "rate-seed-" + strconv.Itoa(index), CreatedAt: time.Now()}).Error; err != nil {
			t.Fatal(err)
		}
	}
	limited := performJSONRequestWithHeaders(t, testApp, http.MethodPost, path, body, cookies, map[string]string{"Idempotency-Key": "rate-key"})
	if limited.Code != http.StatusTooManyRequests {
		t.Fatalf("assistant rate limit: %d %s", limited.Code, limited.Body.String())
	}
	invalid := performJSONRequestWithHeaders(t, testApp, http.MethodPost, path, map[string]any{"content": strings.Repeat("字", 2001)}, cookies, map[string]string{"Idempotency-Key": "long-key"})
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("assistant validation: %d %s", invalid.Code, invalid.Body.String())
	}
	testApp.cfg.DeepSeekBaseURL = "http://127.0.0.1:1"
	failureUser, failureCookies := createLoggedInUser(t, testApp, "video_assistant_failure", "test-password")
	failureConversation := VideoConversation{UserID: failureUser.ID, Title: "失败测试", LastActivityAt: time.Now()}
	if err := db.Create(&failureConversation).Error; err != nil {
		t.Fatal(err)
	}
	failurePath := "/api/videos/conversations/" + strconv.FormatUint(uint64(failureConversation.ID), 10) + "/messages"
	failed := performJSONRequestWithHeaders(t, testApp, http.MethodPost, failurePath, body, failureCookies, map[string]string{"Idempotency-Key": "failure-key"})
	if failed.Code != http.StatusBadGateway || !strings.Contains(failed.Body.String(), "video_assistant_unavailable") {
		t.Fatalf("assistant failure: %d %s", failed.Code, failed.Body.String())
	}
	var failedMessage VideoConversationMessage
	if err := db.Where("conversation_id = ? AND idempotency_key = ?", failureConversation.ID, "failure-key").First(&failedMessage).Error; err != nil || failedMessage.Status != "failed" {
		t.Fatalf("failed message not persisted: %+v %v", failedMessage, err)
	}
}

func containsJSONText(value, fragment string) bool {
	return len(value) >= len(fragment) && stringContains(value, fragment)
}
func stringContains(value, fragment string) bool {
	for i := 0; i+len(fragment) <= len(value); i++ {
		if value[i:i+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
