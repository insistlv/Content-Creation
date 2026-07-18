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
	momentsMarketingInputText  = "text"
	momentsMarketingInputPhoto = "photo"

	momentsMarketingOutputSeparate = "copy_image_separate"
	momentsMarketingOutputPoster   = "poster_overlay"
	momentsMarketingOutputNineGrid = "nine_grid_campaign"
)

type momentsMarketingPlanRequest struct {
	InputMode         string `json:"input_mode"`
	OutputType        string `json:"output_type"`
	ImageCount        int    `json:"image_count"`
	Brief             string `json:"brief"`
	ProductName       string `json:"product_name"`
	SellingPoints     string `json:"selling_points"`
	TargetAudience    string `json:"target_audience"`
	Promotion         string `json:"promotion"`
	Tone              string `json:"tone"`
	CTA               string `json:"cta"`
	ReferenceAssetIDs []uint `json:"reference_asset_ids"`
}

type momentsMarketingPlanResponse struct {
	MomentsText string                      `json:"moments_text"`
	Hashtags    []string                    `json:"hashtags,omitempty"`
	ImageCards  []momentsMarketingImageCard `json:"image_cards"`
	SafetyNotes []string                    `json:"safety_notes,omitempty"`
}

type momentsMarketingImageCard struct {
	Slot            int    `json:"slot"`
	Role            string `json:"role"`
	Caption         string `json:"caption"`
	VisualPrompt    string `json:"visual_prompt"`
	OverlayTitle    string `json:"overlay_title"`
	OverlaySubtitle string `json:"overlay_subtitle"`
	OverlayBadge    string `json:"overlay_badge"`
	CTA             string `json:"cta"`
	Layout          string `json:"layout"`
}

func (a *App) handlePlanMomentsMarketing(c *gin.Context) {
	user := currentUser(c)

	var req momentsMarketingPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	normalized, err := a.normalizeMomentsMarketingPlanRequest(c.Request.Context(), user.ID, req)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_marketing_parameter", err.Error())
		return
	}
	if strings.TrimSpace(a.cfg.DeepSeekAPIKey) == "" {
		writeError(c, http.StatusServiceUnavailable, "marketing_plan_not_configured", "朋友圈营销规划服务暂未配置")
		return
	}

	timeoutSeconds := fallbackPositiveInt(a.cfg.DeepSeekPromptTimeoutSeconds, 45)
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	plan, err := a.planMomentsMarketingWithDeepSeek(ctx, normalized)
	if err != nil {
		writeError(c, http.StatusBadGateway, "marketing_plan_failed", "朋友圈营销方案暂时不可用，请稍后重试")
		return
	}
	c.JSON(http.StatusOK, plan)
}

func (a *App) normalizeMomentsMarketingPlanRequest(ctx context.Context, userID uint, req momentsMarketingPlanRequest) (momentsMarketingPlanRequest, error) {
	req.InputMode = strings.ToLower(strings.TrimSpace(req.InputMode))
	if req.InputMode == "" {
		req.InputMode = momentsMarketingInputText
	}
	if req.InputMode != momentsMarketingInputText && req.InputMode != momentsMarketingInputPhoto {
		return momentsMarketingPlanRequest{}, errors.New("不支持的输入模式")
	}

	req.OutputType = strings.ToLower(strings.TrimSpace(req.OutputType))
	if req.OutputType == "" {
		req.OutputType = momentsMarketingOutputSeparate
	}
	switch req.OutputType {
	case momentsMarketingOutputSeparate, momentsMarketingOutputPoster, momentsMarketingOutputNineGrid:
	default:
		return momentsMarketingPlanRequest{}, errors.New("不支持的输出类型")
	}

	if req.ImageCount == 0 {
		if req.OutputType == momentsMarketingOutputNineGrid {
			req.ImageCount = 9
		} else {
			req.ImageCount = 3
		}
	}
	if req.ImageCount < 1 || req.ImageCount > 9 {
		return momentsMarketingPlanRequest{}, errors.New("图片数量必须在 1 到 9 之间")
	}

	req.Brief = strings.TrimSpace(req.Brief)
	req.ProductName = strings.TrimSpace(req.ProductName)
	req.SellingPoints = strings.TrimSpace(req.SellingPoints)
	req.TargetAudience = strings.TrimSpace(req.TargetAudience)
	req.Promotion = strings.TrimSpace(req.Promotion)
	req.Tone = strings.TrimSpace(req.Tone)
	req.CTA = strings.TrimSpace(req.CTA)
	req.ReferenceAssetIDs = uniqueUintIDs(req.ReferenceAssetIDs)

	if req.InputMode == momentsMarketingInputText && req.Brief == "" && req.ProductName == "" {
		return momentsMarketingPlanRequest{}, errors.New("请填写店铺或产品信息")
	}
	if req.InputMode == momentsMarketingInputPhoto {
		if len(req.ReferenceAssetIDs) == 0 {
			return momentsMarketingPlanRequest{}, errors.New("实图宣传模式需要上传 1 到 4 张实图")
		}
		if len(req.ReferenceAssetIDs) > 4 {
			return momentsMarketingPlanRequest{}, errors.New("实图宣传模式最多上传 4 张实图")
		}
		var count int64
		if err := a.db.WithContext(ctx).Model(&ReferenceAsset{}).
			Where("user_id = ? AND id IN ?", userID, req.ReferenceAssetIDs).
			Count(&count).Error; err != nil {
			return momentsMarketingPlanRequest{}, errors.New("参考图读取失败")
		}
		if count != int64(len(req.ReferenceAssetIDs)) {
			return momentsMarketingPlanRequest{}, errors.New("参考图不存在或无权使用")
		}
	}

	return req, nil
}

func (a *App) planMomentsMarketingWithDeepSeek(ctx context.Context, req momentsMarketingPlanRequest) (momentsMarketingPlanResponse, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(a.cfg.DeepSeekBaseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.deepseek.com"
	}
	model := fallbackString(strings.TrimSpace(a.cfg.DeepSeekPromptModel), "deepseek-v4")

	userPayload, err := json.Marshal(buildMomentsMarketingPlannerPayload(req))
	if err != nil {
		return momentsMarketingPlanResponse{}, err
	}
	payload := map[string]any{
		"model":       model,
		"stream":      false,
		"temperature": 0.35,
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": momentsMarketingSystemPrompt(),
			},
			{
				"role":    "user",
				"content": string(userPayload),
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return momentsMarketingPlanResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return momentsMarketingPlanResponse{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(a.cfg.DeepSeekAPIKey))
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: time.Duration(fallbackPositiveInt(a.cfg.DeepSeekPromptTimeoutSeconds, 45)) * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return momentsMarketingPlanResponse{}, err
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return momentsMarketingPlanResponse{}, fmt.Errorf("deepseek marketing plan failed: status=%d body=%s", resp.StatusCode, string(rawBody))
	}
	content, err := deepSeekMessageContent(rawBody)
	if err != nil {
		return momentsMarketingPlanResponse{}, err
	}
	return parseMomentsMarketingPlanResponse(content, req.ImageCount)
}

func momentsMarketingSystemPrompt() string {
	return strings.Join([]string{
		"你是朋友圈本地生活与产品营销策划助手。",
		"你只负责规划朋友圈正文和每张宣传图的视觉 brief，不直接生成图片。",
		"必须返回严格 JSON，不要 Markdown，不要解释。",
		"JSON 字段必须包含 moments_text、image_cards；可选 hashtags、safety_notes。",
		"image_cards 长度必须等于 image_count；每项包含 slot、role、caption、visual_prompt、overlay_title、overlay_subtitle、overlay_badge、cta、layout。",
		"visual_prompt 必须适合图片生成模型，明确要求无文字、无水印、无二维码、无 logo 乱入。",
		"overlay_title、overlay_subtitle、overlay_badge、cta 必须短，供前端 canvas 排版；不要让图片模型生成中文广告字。",
		"九宫格 campaign 要按传播角色拆分：开场、痛点、产品、场景、优惠、信任背书、细节、口碑、行动引导；少于 9 张时取前 N 个角色。",
		"避免绝对化、医疗功效、金融收益、虚假稀缺等高风险广告表述；必要时在 safety_notes 说明安全改写。",
	}, "\n")
}

func buildMomentsMarketingPlannerPayload(req momentsMarketingPlanRequest) map[string]any {
	return map[string]any{
		"input_mode":          req.InputMode,
		"output_type":         req.OutputType,
		"image_count":         req.ImageCount,
		"brief":               req.Brief,
		"product_name":        req.ProductName,
		"selling_points":      req.SellingPoints,
		"target_audience":     req.TargetAudience,
		"promotion":           req.Promotion,
		"tone":                req.Tone,
		"cta":                 req.CTA,
		"reference_asset_ids": req.ReferenceAssetIDs,
		"reference_note":      "实图模式只会传入素材 ID，规划时不要假装识别图片内容；请基于用户补充信息和素材顺序写保留真实产品/门店特征的视觉 brief。",
		"output_json_schema":  "moments_text:string, hashtags:string[], image_cards:[{slot:int, role:string, caption:string, visual_prompt:string, overlay_title:string, overlay_subtitle:string, overlay_badge:string, cta:string, layout:string}], safety_notes:string[]",
		"copywriting_context": "朋友圈营销，正文自然像真人分享，避免硬广堆砌；图片 brief 面向商业宣传图。",
	}
}

func parseMomentsMarketingPlanResponse(content string, imageCount int) (momentsMarketingPlanResponse, error) {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(strings.TrimPrefix(content, "json"))
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end < start {
		return momentsMarketingPlanResponse{}, errors.New("marketing plan JSON object not found")
	}
	var plan momentsMarketingPlanResponse
	if err := json.Unmarshal([]byte(content[start:end+1]), &plan); err != nil {
		return momentsMarketingPlanResponse{}, err
	}
	return normalizeMomentsMarketingPlanResponse(plan, imageCount)
}

func normalizeMomentsMarketingPlanResponse(plan momentsMarketingPlanResponse, imageCount int) (momentsMarketingPlanResponse, error) {
	plan.MomentsText = strings.TrimSpace(plan.MomentsText)
	if plan.MomentsText == "" {
		return momentsMarketingPlanResponse{}, errors.New("marketing plan missing moments_text")
	}
	if len(plan.ImageCards) != imageCount {
		return momentsMarketingPlanResponse{}, fmt.Errorf("marketing plan image_cards length %d does not match image_count %d", len(plan.ImageCards), imageCount)
	}
	plan.Hashtags = uniqueNonEmptyStrings(plan.Hashtags)
	plan.SafetyNotes = uniqueNonEmptyStrings(plan.SafetyNotes)
	for index := range plan.ImageCards {
		card := &plan.ImageCards[index]
		if card.Slot <= 0 {
			card.Slot = index + 1
		}
		card.Role = strings.TrimSpace(card.Role)
		card.Caption = strings.TrimSpace(card.Caption)
		card.VisualPrompt = strings.TrimSpace(card.VisualPrompt)
		card.OverlayTitle = strings.TrimSpace(card.OverlayTitle)
		card.OverlaySubtitle = strings.TrimSpace(card.OverlaySubtitle)
		card.OverlayBadge = strings.TrimSpace(card.OverlayBadge)
		card.CTA = strings.TrimSpace(card.CTA)
		card.Layout = normalizeMomentsMarketingLayout(card.Layout)
		if card.VisualPrompt == "" {
			return momentsMarketingPlanResponse{}, fmt.Errorf("marketing plan image card %d missing visual_prompt", index+1)
		}
		if !strings.Contains(card.VisualPrompt, "无文字") {
			card.VisualPrompt += "，无文字"
		}
	}
	sort.SliceStable(plan.ImageCards, func(i, j int) bool {
		return plan.ImageCards[i].Slot < plan.ImageCards[j].Slot
	})
	return plan, nil
}

func normalizeMomentsMarketingLayout(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "top_gradient", "center_focus", "split", "badge_focus":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "bottom_gradient"
	}
}

func uniqueNonEmptyStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		clean := strings.TrimSpace(value)
		if clean == "" || seen[clean] {
			continue
		}
		seen[clean] = true
		result = append(result, clean)
	}
	return result
}

func uniqueUintIDs(values []uint) []uint {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[uint]bool, len(values))
	result := make([]uint, 0, len(values))
	for _, value := range values {
		if value == 0 || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}
