package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	imageCompositionPlanSourceDeepSeek = "deepseek"
	imageCompositionPlanSourceFallback = "fallback"
)

type imageCompositionReferenceDescriptor struct {
	Number       int    `json:"number"`
	Type         string `json:"type"`
	ImageSummary string `json:"image_summary"`
}

type deepSeekImageCompositionPlan struct {
	BackgroundReferenceIndex  *int `json:"background_reference_index"`
	BackgroundReferenceNumber *int `json:"background_reference_number"`
	ReferenceRoles            []struct {
		ReferenceIndex  int    `json:"reference_index"`
		ReferenceNumber int    `json:"reference_number"`
		Use             string `json:"use"`
		Role            string `json:"role"`
	} `json:"reference_roles"`
	FinalPrompt string `json:"final_prompt"`
}

func (a *App) planImageComposition(ctx context.Context, job *generationJob, imageCount int) *ImageCompositionPlan {
	if job == nil || job.Request.ReferenceIntent != GenerationReferenceIntentCompose || imageCount < 2 {
		return nil
	}
	if strings.TrimSpace(a.cfg.DeepSeekAPIKey) == "" {
		return fallbackImageCompositionPlan(job.Request, imageCount, "deepseek_not_configured")
	}

	timeoutSeconds := fallbackPositiveInt(a.cfg.DeepSeekComposePlanTimeoutSeconds, 12)
	planCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	plan, err := a.planImageCompositionWithDeepSeek(planCtx, job, imageCount)
	if err != nil {
		return fallbackImageCompositionPlan(job.Request, imageCount, "deepseek_failed")
	}
	return plan
}

func (a *App) planImageCompositionWithDeepSeek(ctx context.Context, job *generationJob, imageCount int) (*ImageCompositionPlan, error) {
	if job == nil {
		return nil, errors.New("nil generation job")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(a.cfg.DeepSeekBaseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.deepseek.com"
	}
	model := fallbackString(strings.TrimSpace(a.cfg.DeepSeekPromptModel), "deepseek-v4")
	references := imageCompositionReferenceDescriptors(job, imageCount)
	planningContext := map[string]any{
		"user_prompt":       job.Request.Prompt,
		"negative_prompt":   job.Request.NegativePrompt,
		"reference_intent":  job.Request.ReferenceIntent,
		"reference_count":   imageCount,
		"references":        references,
		"image_summaries":   emptyImageSummaries(imageCount),
		"indexing_rule":     "用户和输出 JSON 中的图号均为 1-based，例如图2输出 2。",
		"important_warning": "不要假装识别图片内容；只能根据用户提示词、参考图顺序和引用类型制定合成计划。",
		"required_behavior": "必须选择背景图号；必须禁止新增人物、路人、人群、无关主体和额外肢体；final_prompt 必须可直接给图片生成模型使用。",
	}
	if job.Request.BackgroundReferenceIndex != nil {
		planningContext["explicit_background_reference_number"] = *job.Request.BackgroundReferenceIndex + 1
		planningContext["explicit_background_rule"] = "这是用户/API 强制覆盖字段，最终计划必须以该图号作为背景。"
	}
	contextJSON, err := json.Marshal(planningContext)
	if err != nil {
		return nil, err
	}

	payload := map[string]any{
		"model":       model,
		"stream":      false,
		"temperature": 0.1,
		"messages": []map[string]string{
			{
				"role": "system",
				"content": strings.Join([]string{
					"你是图生图多参考图合成规划助手。",
					"你不会接收图片本体，只能读取用户提示词、参考图顺序、引用类型和可选 image_summaries。",
					"请返回严格 JSON，不要 Markdown，不要解释。",
					"JSON 字段：background_reference_index 为 1-based 图号；reference_roles 为数组，每项含 reference_index(1-based) 和 use；final_prompt 为最终图片生成提示词。",
					"若用户/API 显式指定背景图号，必须遵从；否则根据用户提示词判断；无法判断时选择最后一张参考图为背景。",
					"final_prompt 必须包含参考图编号映射、背景图约束、人物保留规则，并明确禁止新增人物、路人、人群、无关主体或额外肢体。",
				}, "\n"),
			},
			{
				"role":    "user",
				"content": string(contextJSON),
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(a.cfg.DeepSeekAPIKey))
	httpReq.Header.Set("Content-Type", "application/json")

	timeoutSeconds := fallbackPositiveInt(a.cfg.DeepSeekComposePlanTimeoutSeconds, 12)
	client := &http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("deepseek compose plan failed: status=%d body=%s", resp.StatusCode, string(rawBody))
	}

	content, err := deepSeekMessageContent(rawBody)
	if err != nil {
		return nil, err
	}
	rawPlan, err := parseDeepSeekImageCompositionPlan(content)
	if err != nil {
		return nil, err
	}
	return buildDeepSeekImageCompositionPlan(job.Request, imageCount, rawPlan)
}

func deepSeekMessageContent(rawBody []byte) (string, error) {
	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(rawBody, &apiResp); err != nil {
		return "", err
	}
	for _, choice := range apiResp.Choices {
		if content := strings.TrimSpace(choice.Message.Content); content != "" {
			return content, nil
		}
	}
	return "", errors.New("deepseek returned empty compose plan")
}

func parseDeepSeekImageCompositionPlan(content string) (deepSeekImageCompositionPlan, error) {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(strings.TrimPrefix(content, "json"))
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end < start {
		return deepSeekImageCompositionPlan{}, errors.New("compose plan JSON object not found")
	}
	var plan deepSeekImageCompositionPlan
	if err := json.Unmarshal([]byte(content[start:end+1]), &plan); err != nil {
		return deepSeekImageCompositionPlan{}, err
	}
	return plan, nil
}

func buildDeepSeekImageCompositionPlan(req generationRequest, imageCount int, rawPlan deepSeekImageCompositionPlan) (*ImageCompositionPlan, error) {
	if strings.TrimSpace(rawPlan.FinalPrompt) == "" {
		return nil, errors.New("deepseek returned empty compose prompt")
	}
	backgroundIndex, ok := explicitBackgroundReferenceIndex(req, imageCount)
	if !ok {
		var err error
		backgroundIndex, err = deepSeekBackgroundReferenceIndex(rawPlan, imageCount)
		if err != nil {
			return nil, err
		}
	}
	usages := normalizeDeepSeekReferenceUsages(rawPlan, imageCount)
	prompt := buildImageCompositionProviderPrompt(req, imageCount, backgroundIndex, usages, rawPlan.FinalPrompt)
	if strings.TrimSpace(prompt) == "" {
		return nil, errors.New("deepseek returned empty compose prompt")
	}
	return &ImageCompositionPlan{
		Prompt:                   prompt,
		Source:                   imageCompositionPlanSourceDeepSeek,
		BackgroundReferenceIndex: intPointer(backgroundIndex),
		ReferenceUsages:          usages,
	}, nil
}

func fallbackImageCompositionPlan(req generationRequest, imageCount int, reason string) *ImageCompositionPlan {
	if imageCount < 1 {
		return nil
	}
	backgroundIndex, ok := explicitBackgroundReferenceIndex(req, imageCount)
	if !ok {
		if promptIndex := backgroundReferenceIndexFromPrompt(req.Prompt, imageCount); promptIndex >= 0 {
			backgroundIndex = promptIndex
		} else {
			backgroundIndex = imageCount - 1
		}
	}
	prompt := buildImageCompositionProviderPrompt(req, imageCount, backgroundIndex, nil, req.Prompt)
	return &ImageCompositionPlan{
		Prompt:                   prompt,
		Source:                   imageCompositionPlanSourceFallback,
		FallbackReason:           reason,
		BackgroundReferenceIndex: intPointer(backgroundIndex),
	}
}

func explicitBackgroundReferenceIndex(req generationRequest, imageCount int) (int, bool) {
	if req.BackgroundReferenceIndex == nil {
		return 0, false
	}
	index := *req.BackgroundReferenceIndex
	if index < 0 || index >= imageCount {
		return 0, false
	}
	return index, true
}

func deepSeekBackgroundReferenceIndex(plan deepSeekImageCompositionPlan, imageCount int) (int, error) {
	if plan.BackgroundReferenceNumber != nil {
		return oneBasedReferenceIndex(*plan.BackgroundReferenceNumber, imageCount)
	}
	if plan.BackgroundReferenceIndex != nil {
		return oneBasedReferenceIndex(*plan.BackgroundReferenceIndex, imageCount)
	}
	return -1, errors.New("deepseek compose plan missing background index")
}

func normalizeDeepSeekReferenceUsages(plan deepSeekImageCompositionPlan, imageCount int) []ImageCompositionReferenceUsage {
	usages := make([]ImageCompositionReferenceUsage, 0, len(plan.ReferenceRoles))
	for _, role := range plan.ReferenceRoles {
		number := role.ReferenceNumber
		if number == 0 {
			number = role.ReferenceIndex
		}
		index, err := oneBasedReferenceIndex(number, imageCount)
		if err != nil {
			continue
		}
		use := strings.TrimSpace(role.Use)
		if use == "" {
			use = strings.TrimSpace(role.Role)
		}
		if use == "" {
			continue
		}
		usages = append(usages, ImageCompositionReferenceUsage{ReferenceIndex: index, Use: use})
	}
	return usages
}

func oneBasedReferenceIndex(number, imageCount int) (int, error) {
	if number < 1 || number > imageCount {
		return -1, fmt.Errorf("reference number %d out of range 1..%d", number, imageCount)
	}
	return number - 1, nil
}

func buildImageCompositionProviderPrompt(req generationRequest, imageCount, backgroundIndex int, usages []ImageCompositionReferenceUsage, basePrompt string) string {
	parts := []string{strings.TrimSpace(basePrompt)}
	if strings.TrimSpace(req.NegativePrompt) != "" {
		parts = append(parts, "反向提示词："+strings.TrimSpace(req.NegativePrompt))
	}
	if imageCount > 0 {
		labels := make([]string, 0, imageCount)
		for index := 0; index < imageCount; index++ {
			labels = append(labels, fmt.Sprintf("【图%d】", index+1))
		}
		parts = append(parts, "参考图按输入顺序编号为"+strings.Join(labels, "、")+"；后续约束中的编号必须与该顺序一一对应。")
	}
	if backgroundIndex >= 0 && backgroundIndex < imageCount {
		parts = append(parts, fmt.Sprintf("背景/场景严格取【图%d】，保留该图的空间结构、环境、光线和视角，不要替换成其他背景。", backgroundIndex+1))
	}
	for _, usage := range usages {
		if usage.ReferenceIndex < 0 || usage.ReferenceIndex >= imageCount || strings.TrimSpace(usage.Use) == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("【图%d】用途：%s", usage.ReferenceIndex+1, strings.TrimSpace(usage.Use)))
	}
	parts = append(parts, "只保留参考图中已经出现的人物，按参考图主体进行合成；不要新增人物、路人、人群、额外肢体或无关主体。")
	parts = append(parts, "在满足用户提示词的前提下进行精准合成，优先保持人物身份特征、服装和背景来源的清晰映射。")
	return strings.Join(uniqueNonEmptyPromptLines(parts), "\n")
}

func uniqueNonEmptyPromptLines(values []string) []string {
	seen := map[string]bool{}
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		filtered = append(filtered, trimmed)
	}
	return filtered
}

func backgroundReferenceIndexFromPrompt(prompt string, imageCount int) int {
	text := strings.TrimSpace(prompt)
	if text == "" || imageCount < 1 {
		return -1
	}
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?:图|第)\s*([0-9一二三四五六七八九十]+)\s*(?:张|号)?[^，。；;,.!\n]{0,16}(?:背景|场景|环境)`),
		regexp.MustCompile(`(?:背景|场景|环境)[^，。；;,.!\n]{0,16}(?:图|第)\s*([0-9一二三四五六七八九十]+)\s*(?:张|号)?`),
	}
	for _, pattern := range patterns {
		matches := pattern.FindStringSubmatch(text)
		if len(matches) < 2 {
			continue
		}
		number, ok := parseReferenceNumber(matches[1])
		if !ok {
			continue
		}
		if index, err := oneBasedReferenceIndex(number, imageCount); err == nil {
			return index
		}
	}
	return -1
}

func parseReferenceNumber(value string) (int, bool) {
	text := strings.TrimSpace(value)
	if text == "" {
		return 0, false
	}
	if number, err := strconv.Atoi(text); err == nil {
		return number, true
	}
	numerals := map[rune]int{
		'一': 1,
		'二': 2,
		'两': 2,
		'三': 3,
		'四': 4,
		'五': 5,
		'六': 6,
		'七': 7,
		'八': 8,
		'九': 9,
	}
	runes := []rune(text)
	if len(runes) == 1 {
		number, ok := numerals[runes[0]]
		return number, ok
	}
	if text == "十" {
		return 10, true
	}
	if strings.HasPrefix(text, "十") && len(runes) == 2 {
		if ones, ok := numerals[runes[1]]; ok {
			return 10 + ones, true
		}
	}
	if strings.HasSuffix(text, "十") && len(runes) == 2 {
		if tens, ok := numerals[runes[0]]; ok {
			return tens * 10, true
		}
	}
	if strings.Contains(text, "十") && len(runes) == 3 {
		tens, tensOK := numerals[runes[0]]
		ones, onesOK := numerals[runes[2]]
		if tensOK && onesOK {
			return tens*10 + ones, true
		}
	}
	return 0, false
}

func imageCompositionReferenceDescriptors(job *generationJob, imageCount int) []imageCompositionReferenceDescriptor {
	descriptors := make([]imageCompositionReferenceDescriptor, 0, imageCount)
	number := 1
	if job.SourceWork != nil && number <= imageCount {
		descriptors = append(descriptors, imageCompositionReferenceDescriptor{
			Number:       number,
			Type:         "source_work",
			ImageSummary: "",
		})
		number++
	}
	for range job.ReferenceAssets {
		if number > imageCount {
			break
		}
		descriptors = append(descriptors, imageCompositionReferenceDescriptor{
			Number:       number,
			Type:         "reference_asset",
			ImageSummary: "",
		})
		number++
	}
	for range job.ReferenceWorks {
		if number > imageCount {
			break
		}
		descriptors = append(descriptors, imageCompositionReferenceDescriptor{
			Number:       number,
			Type:         "reference_work",
			ImageSummary: "",
		})
		number++
	}
	for number <= imageCount {
		descriptors = append(descriptors, imageCompositionReferenceDescriptor{
			Number:       number,
			Type:         "reference_image",
			ImageSummary: "",
		})
		number++
	}
	return descriptors
}

func emptyImageSummaries(imageCount int) []string {
	summaries := make([]string, imageCount)
	for index := range summaries {
		summaries[index] = ""
	}
	return summaries
}

func intPointer(value int) *int {
	return &value
}
