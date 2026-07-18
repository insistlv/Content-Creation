package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	videoConversationPageSize       = 20
	videoAssistantMaxInputLength    = 2000
	videoAssistantRequestsPerMinute = 12
)

type videoConversationMessageRequest struct {
	Content         string         `json:"content"`
	ComposerContext map[string]any `json:"composer_context"`
}

type videoAssistantReply struct {
	Reply           string   `json:"reply"`
	SuggestedPrompt string   `json:"suggested_prompt"`
	ReadyToGenerate bool     `json:"ready_to_generate"`
	QuickReplies    []string `json:"quick_replies"`
}

func parseVideoConversationID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		writeError(c, http.StatusNotFound, "video_conversation_not_found", "视频会话不存在")
		return 0, false
	}
	return uint(id), true
}

func (a *App) ownedVideoConversation(c *gin.Context) (VideoConversation, bool) {
	id, ok := parseVideoConversationID(c)
	if !ok {
		return VideoConversation{}, false
	}
	var item VideoConversation
	if err := a.db.Where("id = ? AND user_id = ?", id, currentUser(c).ID).First(&item).Error; err != nil {
		writeError(c, http.StatusNotFound, "video_conversation_not_found", "视频会话不存在")
		return VideoConversation{}, false
	}
	return item, true
}

func (a *App) handleCreateVideoConversation(c *gin.Context) {
	var req struct {
		Title string `json:"title"`
	}
	if err := c.ShouldBindJSON(&req); err != nil && err != io.EOF {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	now := time.Now()
	item := VideoConversation{UserID: currentUser(c).ID, Title: fallbackString(strings.TrimSpace(req.Title), "新对话"), LastActivityAt: now}
	if err := a.db.Create(&item).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "video_conversation_create_failed", "视频会话创建失败")
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (a *App) handleListVideoConversations(c *gin.Context) {
	userID := currentUser(c).ID
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", strconv.Itoa(videoConversationPageSize)))
	if pageSize < 1 || pageSize > 50 {
		pageSize = videoConversationPageSize
	}
	query := a.db.Model(&VideoConversation{}).Where("user_id = ?", userID)
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		pattern := "%" + q + "%"
		query = query.Where(`title LIKE ?
			OR EXISTS (SELECT 1 FROM video_conversation_messages m WHERE m.conversation_id = video_conversations.id AND m.user_id = ? AND m.content LIKE ?)
			OR EXISTS (SELECT 1 FROM video_generation_records v WHERE v.conversation_id = video_conversations.id AND v.user_id = ? AND v.prompt LIKE ?)`, pattern, userID, pattern, userID, pattern)
	}
	if c.Query("favorite") == "true" {
		query = query.Where("is_favorite = ?", true)
	}
	switch c.Query("range") {
	case "today":
		now := time.Now()
		startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		query = query.Where("last_activity_at >= ?", startOfDay)
	case "7d":
		query = query.Where("last_activity_at >= ?", time.Now().AddDate(0, 0, -7))
	case "30d":
		query = query.Where("last_activity_at >= ?", time.Now().AddDate(0, 0, -30))
	}
	if status := strings.TrimSpace(c.Query("status")); status != "" && status != "all" {
		if status == "running" {
			query = query.Where("EXISTS (SELECT 1 FROM video_generation_records v WHERE v.conversation_id = video_conversations.id AND v.status IN ?)", []string{"queued", "running", "saving"})
		} else {
			query = query.Where("EXISTS (SELECT 1 FROM video_generation_records v WHERE v.conversation_id = video_conversations.id AND v.status = ?)", status)
		}
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		writeError(c, 500, "video_conversations_load_failed", "视频会话读取失败")
		return
	}
	var items []VideoConversation
	if err := query.Order("last_activity_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error; err != nil {
		writeError(c, 500, "video_conversations_load_failed", "视频会话读取失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "page": page, "page_size": pageSize})
}

func (a *App) handlePatchVideoConversation(c *gin.Context) {
	item, ok := a.ownedVideoConversation(c)
	if !ok {
		return
	}
	var req struct {
		Title      *string `json:"title"`
		IsFavorite *bool   `json:"is_favorite"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, 400, "invalid_request", "请求格式错误")
		return
	}
	updates := map[string]any{}
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" {
			writeError(c, 422, "video_conversation_title_required", "会话标题不能为空")
			return
		}
		updates["title"] = title
	}
	if req.IsFavorite != nil {
		updates["is_favorite"] = *req.IsFavorite
	}
	if len(updates) > 0 {
		updates["updated_at"] = time.Now()
		if err := a.db.Model(&item).Updates(updates).Error; err != nil {
			writeError(c, 500, "video_conversation_save_failed", "视频会话保存失败")
			return
		}
		_ = a.db.First(&item, item.ID).Error
	}
	c.JSON(http.StatusOK, item)
}

func (a *App) handleGetVideoConversation(c *gin.Context) {
	item, ok := a.ownedVideoConversation(c)
	if !ok {
		return
	}
	var messages []VideoConversationMessage
	if err := a.db.Where("conversation_id = ? AND user_id = ?", item.ID, item.UserID).Order("created_at ASC, id ASC").Find(&messages).Error; err != nil {
		writeError(c, 500, "video_conversation_load_failed", "视频会话读取失败")
		return
	}
	var generations []VideoGenerationRecord
	if err := a.db.Where("conversation_id = ? AND user_id = ?", item.ID, item.UserID).Order("created_at ASC, id ASC").Find(&generations).Error; err != nil {
		writeError(c, 500, "video_conversation_load_failed", "视频会话读取失败")
		return
	}
	timeline := make([]gin.H, 0, len(messages)+len(generations))
	mi, gi := 0, 0
	for mi < len(messages) || gi < len(generations) {
		if gi >= len(generations) || (mi < len(messages) && !generations[gi].CreatedAt.Before(messages[mi].CreatedAt)) {
			timeline = append(timeline, gin.H{"type": "message", "message": messages[mi]})
			mi++
		} else {
			timeline = append(timeline, gin.H{"type": "generation", "generation": generations[gi]})
			gi++
		}
	}
	c.JSON(http.StatusOK, gin.H{"conversation": item, "timeline": timeline})
}

func (a *App) handleCreateVideoConversationMessage(c *gin.Context) {
	conversation, ok := a.ownedVideoConversation(c)
	if !ok {
		return
	}
	var req videoConversationMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, 400, "invalid_request", "请求格式错误")
		return
	}
	req.Content = strings.TrimSpace(req.Content)
	if req.Content == "" || len([]rune(req.Content)) > videoAssistantMaxInputLength {
		writeError(c, 422, "video_assistant_content_invalid", "请输入 1 至 2000 字的视频创意需求")
		return
	}
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		key = fmt.Sprintf("auto-%d-%x", currentUser(c).ID, sha256.Sum256([]byte(req.Content+time.Now().String())))
	}
	fpRaw, _ := json.Marshal(req)
	sum := sha256.Sum256(fpRaw)
	fingerprint := hex.EncodeToString(sum[:])
	var existing VideoConversationMessage
	if err := a.db.Where("conversation_id = ? AND idempotency_key = ? AND role = ?", conversation.ID, key, "user").First(&existing).Error; err == nil {
		if existing.RequestFingerprint != fingerprint {
			writeError(c, 409, "idempotency_conflict", "重复请求标识与原内容不一致")
			return
		}
		var reply VideoConversationMessage
		_ = a.db.Where("reply_to_message_id = ?", existing.ID).First(&reply).Error
		c.JSON(200, gin.H{"message": existing, "reply": reply})
		return
	}
	var recentCount int64
	a.db.Model(&VideoConversationMessage{}).Where("user_id = ? AND role = ? AND created_at >= ?", conversation.UserID, "user", time.Now().Add(-time.Minute)).Count(&recentCount)
	if recentCount >= videoAssistantRequestsPerMinute {
		writeError(c, 429, "video_assistant_rate_limited", "请求过于频繁，请稍后再试")
		return
	}
	userMessage := VideoConversationMessage{ConversationID: conversation.ID, UserID: conversation.UserID, Role: "user", Content: req.Content, Status: "pending", IdempotencyKey: key, RequestFingerprint: fingerprint}
	if err := a.db.Create(&userMessage).Error; err != nil {
		writeError(c, 500, "video_message_create_failed", "消息保存失败")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Duration(fallbackPositiveInt(a.cfg.DeepSeekPromptTimeoutSeconds, 45))*time.Second)
	defer cancel()
	started := time.Now()
	assistantReply, err := a.replyVideoAssistant(ctx, conversation.ID, req)
	if err != nil {
		a.db.Model(&userMessage).Updates(map[string]any{"status": "failed", "error_code": "video_assistant_unavailable"})
		writeError(c, 502, "video_assistant_unavailable", "视频策划助手暂时不可用，请稍后重试")
		return
	}
	if len(assistantReply.QuickReplies) > 3 {
		assistantReply.QuickReplies = assistantReply.QuickReplies[:3]
	}
	quickJSON, _ := json.Marshal(assistantReply.QuickReplies)
	reply := VideoConversationMessage{ConversationID: conversation.ID, UserID: conversation.UserID, Role: "assistant", Content: assistantReply.Reply, Status: "answered", ReplyToMessageID: &userMessage.ID, SuggestedPrompt: assistantReply.SuggestedPrompt, ReadyToGenerate: assistantReply.ReadyToGenerate, QuickRepliesJSON: string(quickJSON), QuickReplies: assistantReply.QuickReplies, LatencyMS: time.Since(started).Milliseconds()}
	if err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&reply).Error; err != nil {
			return err
		}
		title := conversation.Title
		if title == "新对话" {
			r := []rune(req.Content)
			if len(r) > 18 {
				r = r[:18]
			}
			title = string(r)
		}
		return tx.Model(&conversation).Updates(map[string]any{"title": title, "last_activity_at": time.Now()}).Error
	}); err != nil {
		writeError(c, 500, "video_message_create_failed", "消息保存失败")
		return
	}
	a.db.Model(&userMessage).Update("status", "answered")
	c.JSON(http.StatusCreated, gin.H{"message": userMessage, "reply": reply})
}

func (a *App) replyVideoAssistant(ctx context.Context, conversationID uint, req videoConversationMessageRequest) (videoAssistantReply, error) {
	if strings.TrimSpace(a.cfg.DeepSeekAPIKey) == "" {
		return videoAssistantReply{}, fmt.Errorf("not configured")
	}
	var history []VideoConversationMessage
	a.db.Where("conversation_id = ?", conversationID).Order("created_at DESC").Limit(20).Find(&history)
	messages := []map[string]string{{"role": "system", "content": "你是白霖 AI 的视频策划助手。只做创意策划和可直接用于视频生成的提示词，不声称已经生成视频。请严格返回 JSON：reply、suggested_prompt、ready_to_generate、quick_replies。quick_replies 最多3条。"}}
	for i := len(history) - 1; i >= 0; i-- {
		role := history[i].Role
		if role != "user" && role != "assistant" {
			continue
		}
		messages = append(messages, map[string]string{"role": role, "content": history[i].Content})
	}
	ctxJSON, _ := json.Marshal(req.ComposerContext)
	messages = append(messages, map[string]string{"role": "user", "content": req.Content + "\n当前生成器参数：" + string(ctxJSON)})
	payload, _ := json.Marshal(map[string]any{"model": fallbackString(strings.TrimSpace(a.cfg.DeepSeekPromptModel), "deepseek-v4"), "stream": false, "temperature": 0.6, "max_tokens": 800, "messages": messages})
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(a.cfg.DeepSeekBaseURL, "/")+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return videoAssistantReply{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+a.cfg.DeepSeekAPIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return videoAssistantReply{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return videoAssistantReply{}, fmt.Errorf("assistant status %d", resp.StatusCode)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(raw, &out) != nil || len(out.Choices) == 0 {
		return videoAssistantReply{}, fmt.Errorf("empty reply")
	}
	content := cleanOptimizedPromptText(out.Choices[0].Message.Content)
	var parsed videoAssistantReply
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return videoAssistantReply{}, err
	}
	if strings.TrimSpace(parsed.Reply) == "" {
		return videoAssistantReply{}, fmt.Errorf("empty reply")
	}
	return parsed, nil
}

func (a *App) backfillVideoConversations() error {
	if !a.db.Migrator().HasTable(&VideoConversation{}) || !a.db.Migrator().HasTable(&VideoGenerationRecord{}) {
		return nil
	}
	for {
		var records []VideoGenerationRecord
		if err := a.db.Where("conversation_id IS NULL AND (source IS NULL OR source = '' OR source <> ?)", "novel_video").Order("id ASC").Limit(200).Find(&records).Error; err != nil {
			return err
		}
		if len(records) == 0 {
			return nil
		}
		for _, record := range records {
			if err := a.db.Transaction(func(tx *gorm.DB) error {
				titleRunes := []rune(strings.TrimSpace(record.Prompt))
				if len(titleRunes) > 28 {
					titleRunes = titleRunes[:28]
				}
				title := string(titleRunes)
				if title == "" {
					title = "历史视频创作"
				}
				generationID := record.GenerationRecordID
				conversation := VideoConversation{UserID: record.UserID, Title: title, LastGenerationID: &generationID, LastActivityAt: record.UpdatedAt, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}
				if conversation.LastActivityAt.IsZero() {
					conversation.LastActivityAt = record.CreatedAt
				}
				if err := tx.Create(&conversation).Error; err != nil {
					return err
				}
				return tx.Model(&VideoGenerationRecord{}).Where("id = ? AND conversation_id IS NULL", record.ID).Update("conversation_id", conversation.ID).Error
			}); err != nil {
				return err
			}
		}
	}
}
