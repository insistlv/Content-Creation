package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type imageAgentPlanRequest struct {
	Message           string              `json:"message"`
	History           []promptChatMessage `json:"history"`
	ReferenceAssetIDs []uint              `json:"reference_asset_ids"`
	ReferenceWorkIDs  []uint              `json:"reference_work_ids"`
	CurrentPlan       *imageAgentPlan     `json:"current_plan"`
}

type imageAgentPlanResponse struct {
	Reply              string           `json:"reply"`
	NeedsClarification bool             `json:"needs_clarification"`
	Plan               imageAgentPlan   `json:"plan"`
	Candidates         []imageAgentPlan `json:"candidates"`
	SafetyNotes        []string         `json:"safety_notes"`
	Model              string           `json:"model"`
}

type imageAgentPlan struct {
	ID                   string         `json:"id,omitempty"`
	Title                string         `json:"title"`
	Intent               string         `json:"intent"`
	ToolMode             string         `json:"tool_mode"`
	Prompt               string         `json:"prompt"`
	NegativePrompt       string         `json:"negative_prompt,omitempty"`
	AspectRatio          string         `json:"aspect_ratio"`
	StylePreset          string         `json:"style_preset,omitempty"`
	Quality              string         `json:"quality"`
	ReferenceWeight      int            `json:"reference_weight"`
	ToolOptions          map[string]any `json:"tool_options,omitempty"`
	EditInstruction      string         `json:"edit_instruction,omitempty"`
	RequiresConfirmation bool           `json:"requires_confirmation"`
}

func (a *App) handlePlanImageAgent(c *gin.Context) {
	var req imageAgentPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	req.Message = strings.TrimSpace(req.Message)
	req.History = normalizePromptChatHistory(req.History)
	if req.CurrentPlan != nil {
		normalized := normalizeImageAgentPlan(*req.CurrentPlan)
		req.CurrentPlan = &normalized
	}
	if req.Message == "" && len(req.ReferenceAssetIDs) == 0 && len(req.ReferenceWorkIDs) == 0 && req.CurrentPlan == nil {
		writeError(c, http.StatusBadRequest, "agent_message_required", "请先描述创作目标或提供参考图")
		return
	}
	if strings.TrimSpace(a.cfg.DeepSeekAPIKey) == "" {
		writeError(c, http.StatusServiceUnavailable, "agent_image_plan_not_configured", "创作任务代理暂未配置")
		return
	}

	timeoutSeconds := fallbackPositiveInt(a.cfg.DeepSeekPromptTimeoutSeconds, 45)
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	result, err := a.planImageAgentWithDeepSeek(ctx, req)
	if err != nil {
		writeError(c, http.StatusBadGateway, "agent_image_plan_failed", "创作任务代理暂时不可用，请稍后重试")
		return
	}

	plan, notes := sanitizeImageAgentPlan(result.Plan, req.Message)
	candidates, candidateNotes := sanitizeImageAgentCandidates(result.Candidates)
	notes = append(notes, candidateNotes...)
	notes = append(result.SafetyNotes, notes...)
	if len(candidates) == 0 && plan.Prompt != "" {
		candidate := plan
		candidate.ID = "primary"
		candidates = append(candidates, candidate)
	}

	c.JSON(http.StatusOK, imageAgentPlanResponse{
		Reply:              strings.TrimSpace(result.Reply),
		NeedsClarification: result.NeedsClarification,
		Plan:               plan,
		Candidates:         candidates,
		SafetyNotes:        uniqueStrings(notes),
		Model:              fallbackString(strings.TrimSpace(a.cfg.DeepSeekPromptModel), "deepseek-v4"),
	})
}

func (a *App) planImageAgentWithDeepSeek(ctx context.Context, req imageAgentPlanRequest) (imageAgentPlanResponse, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(a.cfg.DeepSeekBaseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.deepseek.com"
	}
	model := fallbackString(strings.TrimSpace(a.cfg.DeepSeekPromptModel), "deepseek-v4")

	payload := map[string]any{
		"model":       model,
		"stream":      false,
		"temperature": 0.6,
		"messages": []map[string]string{
			{
				"role": "system",
				"content": strings.Join([]string{
					"你是白霖 AI 工作台的创作任务代理，负责把用户自然语言需求整理为可确认、可直接出图的高质量图片创作方案。",
					"v1 只做规划和单次或批量提交前的参数填充；未经确认不得自动扣点，不得自动发布、分享、删除或修改作品库可见性。",
					"你需要识别意图：文生图、图生图、局部编辑、扩图、抠图、高清放大，并据此选择 tool_mode。",
					"",
					"【提示词工程规范】prompt 必须是结构完整、细节丰富、可直接出图的描述，按以下五层组织并用中文逗号连接：",
					"1) 主体：主体对象、数量、姿态或状态、关键材质与质感；",
					"2) 场景环境：背景、空间关系、陪体与道具；",
					"3) 构图镜头：景别（特写/中景/全景）、视角（平视/俯拍/仰拍）、画幅或焦段语言；",
					"4) 光线色彩：光源类型与方向（柔光/硬光/逆光/伦勃朗光）、色温、主色调与配色；",
					"5) 风格画质：艺术风格或摄影流派，以及高细节、超清、商业摄影级、电影质感等画质关键词。",
					"prompt 控制在 60-180 字，宁可具体不要笼统，避免空泛堆砌与自相矛盾，必要时保留专业英文术语。",
					"negative_prompt 给出与主体相关的针对性排除项，例如 低分辨率、畸变、多余手指、文字水印、噪点、塑料感、过曝 等。",
					"reference_weight 取 0-100；提供参考图时给出合理强度，纯文生图可偏低。",
					"根据意图设置 aspect_ratio 与 quality：电商主图常用 1:1，竖版海报常用 3:4 或 9:16，横版banner常用 16:9。",
					"",
					"【候选方案】candidates 至少给 2-3 个有明显差异的方向（不同构图、风格或用途），不是同一方案的措辞改写；每个候选都是独立完整、可直接出图的 prompt，并配套各自的 title、aspect_ratio、quality。",
					"如信息不足，needs_clarification=true，reply 提出最关键的 1 个追问，但仍要给出一份可编辑、可直接出图的草稿方案，绝不能让 prompt 为空。",
					"",
					"tool_mode 只能使用 generate、redraw、erase、expand、upscale、remove_background、precision_edit。",
					"quality 只能使用 low、medium、high、ultra；aspect_ratio 使用 1:1、3:4、4:3、9:16、16:9、21:9、9:21 等常用比例。",
					"涉及参考图时只说明使用方式，不得伪造图像识别结果。",
					"",
					"【安全】必须规避成人、血腥暴力、违法危险、仇恨歧视和敏感政治煽动表达。",
					"必须规避知名 IP、品牌标识、受版权保护角色或可识别角色相似组合；用原创、泛化、商业可用表达替代。",
					"",
					"输出要求：必须输出严格的 JSON，不要 Markdown、不要代码块、不要注释。字段包含 reply、needs_clarification、plan、candidates、safety_notes。",
					"plan 和 candidates 的字段包含 title、intent、tool_mode、prompt、negative_prompt、aspect_ratio、style_preset、quality、reference_weight、tool_options、edit_instruction、requires_confirmation。",
				}, "\n"),
			},
			{
				"role":    "user",
				"content": buildImageAgentUserContent(req),
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return imageAgentPlanResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return imageAgentPlanResponse{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(a.cfg.DeepSeekAPIKey))
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: time.Duration(fallbackPositiveInt(a.cfg.DeepSeekPromptTimeoutSeconds, 45)) * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return imageAgentPlanResponse{}, err
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return imageAgentPlanResponse{}, fmt.Errorf("deepseek agent plan failed: status=%d body=%s", resp.StatusCode, string(rawBody))
	}

	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(rawBody, &apiResp); err != nil {
		return imageAgentPlanResponse{}, err
	}
	for _, choice := range apiResp.Choices {
		if content := cleanOptimizedPromptText(choice.Message.Content); content != "" {
			return parseImageAgentPlanResponse(content)
		}
	}
	return imageAgentPlanResponse{}, fmt.Errorf("deepseek returned empty agent plan")
}

func buildImageAgentUserContent(req imageAgentPlanRequest) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "message=%s\n", req.Message)
	if len(req.ReferenceAssetIDs) > 0 {
		raw, _ := json.Marshal(req.ReferenceAssetIDs)
		fmt.Fprintf(&builder, "reference_asset_ids=%s\n", string(raw))
	}
	if len(req.ReferenceWorkIDs) > 0 {
		raw, _ := json.Marshal(req.ReferenceWorkIDs)
		fmt.Fprintf(&builder, "reference_work_ids=%s\n", string(raw))
	}
	if len(req.History) > 0 {
		raw, _ := json.Marshal(req.History)
		fmt.Fprintf(&builder, "history=%s\n", string(raw))
	}
	if req.CurrentPlan != nil {
		raw, _ := json.Marshal(req.CurrentPlan)
		fmt.Fprintf(&builder, "current_plan=%s\n", string(raw))
	}
	return strings.TrimSpace(builder.String())
}

func parseImageAgentPlanResponse(content string) (imageAgentPlanResponse, error) {
	text := strings.TrimSpace(content)
	if text == "" {
		return imageAgentPlanResponse{}, fmt.Errorf("empty agent plan")
	}
	var parsed imageAgentPlanResponse
	if err := json.Unmarshal([]byte(text), &parsed); err == nil {
		return normalizeImageAgentPlanResponse(parsed), nil
	}
	if start, end := strings.Index(text, "{"), strings.LastIndex(text, "}"); start >= 0 && end > start {
		if err := json.Unmarshal([]byte(text[start:end+1]), &parsed); err == nil {
			return normalizeImageAgentPlanResponse(parsed), nil
		}
	}
	return imageAgentPlanResponse{}, fmt.Errorf("invalid agent plan json")
}

func normalizeImageAgentPlanResponse(value imageAgentPlanResponse) imageAgentPlanResponse {
	value.Reply = strings.TrimSpace(value.Reply)
	value.Plan = normalizeImageAgentPlan(value.Plan)
	if len(value.Candidates) > 0 {
		candidates := make([]imageAgentPlan, 0, len(value.Candidates))
		for _, candidate := range value.Candidates {
			candidate = normalizeImageAgentPlan(candidate)
			if candidate.Title == "" && candidate.Prompt == "" {
				continue
			}
			candidates = append(candidates, candidate)
		}
		value.Candidates = candidates
	}
	value.SafetyNotes = uniqueStrings(value.SafetyNotes)
	return value
}

func normalizeImageAgentPlan(value imageAgentPlan) imageAgentPlan {
	value.ID = strings.TrimSpace(value.ID)
	value.Title = strings.TrimSpace(value.Title)
	value.Intent = strings.TrimSpace(value.Intent)
	value.ToolMode = normalizeGenerationToolMode(value.ToolMode)
	value.Prompt = strings.TrimSpace(value.Prompt)
	value.NegativePrompt = strings.TrimSpace(value.NegativePrompt)
	value.AspectRatio = strings.TrimSpace(value.AspectRatio)
	if value.AspectRatio == "" {
		value.AspectRatio = "1:1"
	}
	value.StylePreset = strings.TrimSpace(value.StylePreset)
	value.Quality = normalizeGenerationQuality(value.Quality)
	value.EditInstruction = strings.TrimSpace(value.EditInstruction)
	if value.ReferenceWeight < 0 {
		value.ReferenceWeight = 0
	}
	if value.ReferenceWeight > 100 {
		value.ReferenceWeight = 100
	}
	if value.ReferenceWeight == 0 {
		value.ReferenceWeight = 75
	}
	value.RequiresConfirmation = true
	return value
}

func sanitizeImageAgentPlan(plan imageAgentPlan, fallbackPrompt string) (imageAgentPlan, []string) {
	plan = normalizeImageAgentPlan(plan)
	if plan.Prompt == "" {
		plan.Prompt = strings.TrimSpace(fallbackPrompt)
	}
	prompt, notes := sanitizePromptForImageGeneration(plan.Prompt, "safe")
	if prompt != "" {
		plan.Prompt = prompt
	}
	negativePrompt, negativeNotes := sanitizePromptForImageGeneration(plan.NegativePrompt, "safe")
	if negativePrompt != "" {
		plan.NegativePrompt = negativePrompt
	}
	notes = append(notes, negativeNotes...)
	if plan.Title == "" {
		plan.Title = "图片创作方案"
	}
	if plan.Intent == "" {
		plan.Intent = "text_to_image"
	}
	return plan, uniqueStrings(notes)
}

func sanitizeImageAgentCandidates(values []imageAgentPlan) ([]imageAgentPlan, []string) {
	if len(values) == 0 {
		return nil, nil
	}
	candidates := make([]imageAgentPlan, 0, len(values))
	var notes []string
	for index, candidate := range values {
		cleaned, candidateNotes := sanitizeImageAgentPlan(candidate, "")
		notes = append(notes, candidateNotes...)
		if cleaned.Prompt == "" {
			continue
		}
		if cleaned.ID == "" {
			cleaned.ID = fmt.Sprintf("candidate-%d", index+1)
		}
		candidates = append(candidates, cleaned)
	}
	return candidates, uniqueStrings(notes)
}
