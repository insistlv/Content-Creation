package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	articleImagesDefaultCount = 4
	articleImagesMaxCount     = 9
	articleImagesMaxRefs      = 4
)

type articleImagesPlanRequest struct {
	Title             string `json:"title"`
	Body              string `json:"body"`
	ArticleType       string `json:"article_type"`
	Audience          string `json:"audience"`
	Style             string `json:"style"`
	ImageCount        int    `json:"image_count"`
	IncludeCover      bool   `json:"include_cover"`
	ReferenceAssetIDs []uint `json:"reference_asset_ids"`
}

type articleImagesPlanResponse struct {
	ArticleSummary string                   `json:"article_summary"`
	ImageCards     []articleImagesImageCard `json:"image_cards"`
	SafetyNotes    []string                 `json:"safety_notes,omitempty"`
}

type articleImagesImageCard struct {
	Slot         int    `json:"slot"`
	Role         string `json:"role"`
	Placement    string `json:"placement"`
	Caption      string `json:"caption"`
	VisualPrompt string `json:"visual_prompt"`
	AspectRatio  string `json:"aspect_ratio"`
	OverlayTitle string `json:"overlay_title"`
	Layout       string `json:"layout"`
}

func (a *App) handlePlanArticleImages(c *gin.Context) {
	user := currentUser(c)

	var req articleImagesPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	normalized, err := a.normalizeArticleImagesPlanRequest(c.Request.Context(), user.ID, req)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_article_images_parameter", err.Error())
		return
	}
	if strings.TrimSpace(a.cfg.DeepSeekAPIKey) == "" {
		writeError(c, http.StatusServiceUnavailable, "article_images_plan_not_configured", "公众号配图规划服务暂未配置")
		return
	}

	timeoutSeconds := fallbackPositiveInt(a.cfg.DeepSeekPromptTimeoutSeconds, 45)
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	plan, err := a.planArticleImagesWithDeepSeek(ctx, normalized)
	if err != nil {
		writeError(c, http.StatusBadGateway, "article_images_plan_failed", "公众号配图方案暂时不可用，请稍后重试")
		return
	}
	c.JSON(http.StatusOK, plan)
}

func (a *App) normalizeArticleImagesPlanRequest(ctx context.Context, userID uint, req articleImagesPlanRequest) (articleImagesPlanRequest, error) {
	req.Title = strings.TrimSpace(req.Title)
	req.Body = strings.TrimSpace(req.Body)
	req.ArticleType = strings.TrimSpace(req.ArticleType)
	req.Audience = strings.TrimSpace(req.Audience)
	req.Style = strings.TrimSpace(req.Style)
	req.ReferenceAssetIDs = uniqueUintIDs(req.ReferenceAssetIDs)

	if req.Body == "" {
		return articleImagesPlanRequest{}, errors.New("请粘贴公众号文章正文")
	}
	if req.ImageCount == 0 {
		req.ImageCount = articleImagesDefaultCount
	}
	if req.ImageCount < 1 || req.ImageCount > articleImagesMaxCount {
		return articleImagesPlanRequest{}, fmt.Errorf("图片数量必须在 1 到 %d 之间", articleImagesMaxCount)
	}
	if req.ArticleType == "" {
		req.ArticleType = "知识科普"
	}
	if req.Audience == "" {
		req.Audience = "公众号读者"
	}
	if req.Style == "" {
		req.Style = "清爽专业"
	}
	if len(req.ReferenceAssetIDs) > articleImagesMaxRefs {
		return articleImagesPlanRequest{}, fmt.Errorf("参考图最多上传 %d 张", articleImagesMaxRefs)
	}
	if len(req.ReferenceAssetIDs) > 0 {
		var count int64
		if err := a.db.WithContext(ctx).Model(&ReferenceAsset{}).
			Where("user_id = ? AND id IN ?", userID, req.ReferenceAssetIDs).
			Count(&count).Error; err != nil {
			return articleImagesPlanRequest{}, errors.New("参考图读取失败")
		}
		if count != int64(len(req.ReferenceAssetIDs)) {
			return articleImagesPlanRequest{}, errors.New("参考图不存在或无权使用")
		}
	}

	return req, nil
}

func (a *App) planArticleImagesWithDeepSeek(ctx context.Context, req articleImagesPlanRequest) (articleImagesPlanResponse, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(a.cfg.DeepSeekBaseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.deepseek.com"
	}
	model := fallbackString(strings.TrimSpace(a.cfg.DeepSeekPromptModel), "deepseek-v4")

	userPayload, err := json.Marshal(buildArticleImagesPlannerPayload(req))
	if err != nil {
		return articleImagesPlanResponse{}, err
	}
	payload := map[string]any{
		"model":       model,
		"stream":      false,
		"temperature": 0.32,
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": articleImagesSystemPrompt(),
			},
			{
				"role":    "user",
				"content": string(userPayload),
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return articleImagesPlanResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return articleImagesPlanResponse{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(a.cfg.DeepSeekAPIKey))
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: time.Duration(fallbackPositiveInt(a.cfg.DeepSeekPromptTimeoutSeconds, 45)) * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return articleImagesPlanResponse{}, err
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return articleImagesPlanResponse{}, fmt.Errorf("deepseek article images plan failed: status=%d body=%s", resp.StatusCode, string(rawBody))
	}
	content, err := deepSeekMessageContent(rawBody)
	if err != nil {
		return articleImagesPlanResponse{}, err
	}
	return parseArticleImagesPlanResponse(content, req.ImageCount)
}

func articleImagesSystemPrompt() string {
	return strings.Join([]string{
		"你是公众号文章视觉策划助手，只负责把文章拆成配图方案，不直接生成图片。",
		"必须返回严格 JSON，不要 Markdown，不要解释。",
		"JSON 字段必须包含 article_summary、image_cards；可选 safety_notes。",
		"image_cards 长度必须等于 image_count；每项包含 slot、role、placement、caption、visual_prompt、aspect_ratio、overlay_title、layout。",
		"role 只能从封面图、段落配图、金句卡片、流程/步骤图、场景氛围图中选择；include_cover 为 true 时第 1 张必须是封面图。",
		"visual_prompt 必须适合图片生成模型，描述画面、主体、环境、构图、光线、风格；必须明确无文字、无水印、无二维码、无 logo 乱入。",
		"不要让图片模型生成中文大字、标题、金句或排版文字；这些文字只写在 overlay_title 或 caption，供前端 canvas 叠加。",
		"aspect_ratio 只能使用 16:9、3:4、1:1；封面优先 16:9，金句卡片优先 1:1，段落配图优先 16:9。",
		"placement 要给出建议插入位置，例如文章开头、某个小标题后、结尾前。",
		"避免医疗功效、金融收益、绝对化承诺、侵权人物和误导性广告表达；必要时在 safety_notes 说明安全改写。",
	}, "\n")
}

func buildArticleImagesPlannerPayload(req articleImagesPlanRequest) map[string]any {
	return map[string]any{
		"title":               req.Title,
		"body":                req.Body,
		"article_type":        req.ArticleType,
		"audience":            req.Audience,
		"style":               req.Style,
		"image_count":         req.ImageCount,
		"include_cover":       req.IncludeCover,
		"reference_asset_ids": req.ReferenceAssetIDs,
		"reference_note":      "参考图仅用于后续图片生成，规划时不要假装已经识别图片内容；可在 visual_prompt 中保留品牌/产品/人物一致性要求。",
		"output_json_schema":  "article_summary:string, image_cards:[{slot:int, role:string, placement:string, caption:string, visual_prompt:string, aspect_ratio:string, overlay_title:string, layout:string}], safety_notes:string[]",
		"text_overlay_note":   "标题、金句和说明文字由前端 canvas 叠加，不要要求图片模型生成中文文字。",
	}
}

func parseArticleImagesPlanResponse(content string, imageCount int) (articleImagesPlanResponse, error) {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(strings.TrimPrefix(content, "json"))
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end < start {
		return articleImagesPlanResponse{}, errors.New("article images plan JSON object not found")
	}
	var plan articleImagesPlanResponse
	if err := json.Unmarshal([]byte(content[start:end+1]), &plan); err != nil {
		return articleImagesPlanResponse{}, err
	}
	return normalizeArticleImagesPlanResponse(plan, imageCount)
}

func normalizeArticleImagesPlanResponse(plan articleImagesPlanResponse, imageCount int) (articleImagesPlanResponse, error) {
	plan.ArticleSummary = strings.TrimSpace(plan.ArticleSummary)
	if plan.ArticleSummary == "" {
		return articleImagesPlanResponse{}, errors.New("article images plan missing article_summary")
	}
	if len(plan.ImageCards) != imageCount {
		return articleImagesPlanResponse{}, fmt.Errorf("article images plan image_cards length %d does not match image_count %d", len(plan.ImageCards), imageCount)
	}
	plan.SafetyNotes = uniqueNonEmptyStrings(plan.SafetyNotes)
	for index := range plan.ImageCards {
		card := &plan.ImageCards[index]
		if card.Slot <= 0 {
			card.Slot = index + 1
		}
		card.Role = normalizeArticleImageRole(card.Role, index)
		card.Placement = strings.TrimSpace(card.Placement)
		card.Caption = strings.TrimSpace(card.Caption)
		card.VisualPrompt = strings.TrimSpace(card.VisualPrompt)
		card.AspectRatio = normalizeArticleImageAspectRatio(card.AspectRatio, card.Role)
		card.OverlayTitle = strings.TrimSpace(card.OverlayTitle)
		card.Layout = normalizeArticleImageLayout(card.Layout)
		if card.Placement == "" {
			card.Placement = fallbackArticleImagePlacement(index)
		}
		if card.Caption == "" {
			card.Caption = card.Role
		}
		if card.OverlayTitle == "" {
			card.OverlayTitle = card.Caption
		}
		if card.VisualPrompt == "" {
			return articleImagesPlanResponse{}, fmt.Errorf("article images plan image card %d missing visual_prompt", index+1)
		}
		if !strings.Contains(card.VisualPrompt, "无文字") {
			card.VisualPrompt += "，无文字"
		}
		if !strings.Contains(card.VisualPrompt, "无水印") {
			card.VisualPrompt += "，无水印"
		}
	}
	sort.SliceStable(plan.ImageCards, func(i, j int) bool {
		return plan.ImageCards[i].Slot < plan.ImageCards[j].Slot
	})
	return plan, nil
}

func normalizeArticleImageRole(value string, index int) string {
	clean := strings.TrimSpace(value)
	switch clean {
	case "封面图", "段落配图", "金句卡片", "流程/步骤图", "场景氛围图":
		return clean
	default:
		if index == 0 {
			return "封面图"
		}
		return "段落配图"
	}
}

func normalizeArticleImageAspectRatio(value string, role string) string {
	switch strings.TrimSpace(value) {
	case "16:9", "3:4", "1:1":
		return strings.TrimSpace(value)
	default:
		if role == "金句卡片" {
			return "1:1"
		}
		if role == "段落配图" || role == "流程/步骤图" {
			return "16:9"
		}
		return "16:9"
	}
}

func normalizeArticleImageLayout(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "cover_overlay", "quote_card", "step_card", "clean_overlay", "split_caption":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "clean_overlay"
	}
}

func fallbackArticleImagePlacement(index int) string {
	if index == 0 {
		return "文章开头"
	}
	return fmt.Sprintf("第 %d 个小标题后", index)
}
