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

type promptOptimizationRequest struct {
	Prompt           string                 `json:"prompt"`
	Mode             string                 `json:"mode"`
	AspectRatio      string                 `json:"aspect_ratio"`
	StylePreset      string                 `json:"style_preset"`
	Message          string                 `json:"message"`
	History          []promptChatMessage    `json:"history"`
	StructuredPrompt promptStructuredFields `json:"structured_prompt"`
	Action           string                 `json:"action"`
}

type promptOptimizationResponse struct {
	OriginalPrompt   string                  `json:"original_prompt"`
	Reply            string                  `json:"reply,omitempty"`
	OptimizedPrompt  string                  `json:"optimized_prompt"`
	StructuredPrompt promptStructuredFields  `json:"structured_prompt"`
	Directions       []promptDirectionOption `json:"directions,omitempty"`
	SafetyNotes      []string                `json:"safety_notes"`
	Model            string                  `json:"model"`
}

type promptChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type promptStructuredFields struct {
	Subject string `json:"subject"`
	Scene   string `json:"scene"`
	Style   string `json:"style"`
	Usage   string `json:"usage"`
}

type promptDirectionOption struct {
	Title            string                 `json:"title"`
	Summary          string                 `json:"summary"`
	Prompt           string                 `json:"prompt"`
	StructuredPrompt promptStructuredFields `json:"structured_prompt"`
}

type promptOptimizationModelResponse struct {
	Reply            string                  `json:"reply"`
	OptimizedPrompt  string                  `json:"optimized_prompt"`
	StructuredPrompt promptStructuredFields  `json:"structured_prompt"`
	Directions       []promptDirectionOption `json:"directions"`
	SafetyNotes      []string                `json:"safety_notes"`
}

func (a *App) handleOptimizePrompt(c *gin.Context) {
	var req promptOptimizationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	req.Prompt = strings.TrimSpace(req.Prompt)
	req.Mode = normalizePromptOptimizationMode(req.Mode)
	req.AspectRatio = strings.TrimSpace(req.AspectRatio)
	req.StylePreset = strings.TrimSpace(req.StylePreset)
	req.Message = strings.TrimSpace(req.Message)
	req.Action = normalizePromptOptimizationAction(req.Action, req.Mode)
	req.History = normalizePromptChatHistory(req.History)
	req.StructuredPrompt = normalizePromptStructuredFields(req.StructuredPrompt)
	if req.Prompt == "" {
		writeError(c, http.StatusBadRequest, "prompt_required", "提示词不能为空")
		return
	}
	if strings.TrimSpace(a.cfg.DeepSeekAPIKey) == "" {
		writeError(c, http.StatusServiceUnavailable, "prompt_optimizer_not_configured", "提示词优化服务暂未配置")
		return
	}

	timeoutSeconds := a.cfg.DeepSeekPromptTimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 45
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	modelResult, err := a.optimizePromptWithDeepSeek(ctx, req)
	if err != nil {
		writeError(c, http.StatusBadGateway, "prompt_optimizer_failed", "AI 提示词优化暂时不可用，请稍后重试")
		return
	}
	optimizedPrompt, notes := sanitizePromptForImageGeneration(modelResult.OptimizedPrompt, req.Mode)
	if optimizedPrompt == "" {
		optimizedPrompt, notes = sanitizePromptForImageGeneration(req.Prompt, req.Mode)
	}
	notes = append(modelResult.SafetyNotes, notes...)
	if len(notes) == 0 {
		notes = append(notes, "已按图片生成场景优化为可直接使用的安全描述")
	}
	structuredPrompt, structuredNotes := sanitizeStructuredPromptFields(modelResult.StructuredPrompt, req.Mode)
	notes = append(notes, structuredNotes...)
	directions, directionNotes := sanitizePromptDirections(modelResult.Directions, req.Mode)
	notes = append(notes, directionNotes...)

	c.JSON(http.StatusOK, promptOptimizationResponse{
		OriginalPrompt:   req.Prompt,
		Reply:            strings.TrimSpace(modelResult.Reply),
		OptimizedPrompt:  optimizedPrompt,
		StructuredPrompt: structuredPrompt,
		Directions:       directions,
		SafetyNotes:      uniqueStrings(notes),
		Model:            fallbackString(strings.TrimSpace(a.cfg.DeepSeekPromptModel), "deepseek-v4"),
	})
}

func (a *App) optimizePromptWithDeepSeek(ctx context.Context, req promptOptimizationRequest) (promptOptimizationModelResponse, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(a.cfg.DeepSeekBaseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.deepseek.com"
	}
	model := fallbackString(strings.TrimSpace(a.cfg.DeepSeekPromptModel), "deepseek-v4")

	payload := map[string]any{
		"model":       model,
		"stream":      false,
		"temperature": 0.35,
		"messages": []map[string]string{
			{
				"role": "system",
				"content": strings.Join([]string{
					"你是图片生成的 AI 提示词助手。",
					"你可以用对话方式追问和整理需求，也可以把已有描述改写为可直接提交给图片生成模型的中文提示词。",
					"必须规避可能导致平台安全策略拒绝的违禁词、露骨成人内容、血腥暴力、违法危险、仇恨歧视和敏感政治煽动表达。",
					"必须规避受版权保护角色、知名角色、影视动画角色或品牌 IP 的姓名、标志性外观、服装配色、发型、同伴道具等组合相似性。",
					"用安全、视觉化、商业可用的替代表达保留画面意图。",
					"当原始描述接近知名角色时，改写成原创人物、原创服装和泛化场景元素。",
					"当 mode=portrait_detail 时，执行人脸高清优化：仅对成年写实人像强化真实皮肤毛孔、细微面部绒毛、自然皮肤质感、电影级布光、超写实 RAW 摄影质感；不要对未成年人、非人像、商品、风景、插画或漫画主体强行加入皮肤微距细节。",
					"当 action=change_direction 或 mode=direction 时，返回 3 个不同方向方案。",
					"必须返回 JSON，不要 Markdown。JSON 字段必须包含 reply、optimized_prompt、structured_prompt；structured_prompt 包含 subject、scene、style、usage。",
					"请基于原始描述推断结构化字段：subject、scene、style 要结合原始提示词、用户最新补充、history 和 current_structured_prompt 尽量给出可编辑草稿。",
					"structured_prompt 字段必须与 optimized_prompt 保持一致；不要返回占位文案，不要使用“待补充”“未知”“不确定”等机械占位；确实无法判断时返回空字符串。",
					"usage 用途采用温和推断：能判断时给宽泛用途，例如角色素材、社交媒体配图、商品图、海报、概念图；低置信可空，不要强行编造具体业务场景。",
					"需要方向方案时包含 directions 数组，每项包含 title、summary、prompt、structured_prompt；每个方向的 structured_prompt 必须与该方向 prompt 保持一致。",
					"可选 safety_notes。",
					"如果信息不足，reply 先追问关键问题，同时仍基于已有信息给出 optimized_prompt 和 structured_prompt 草稿。",
				}, "\n"),
			},
			{
				"role":    "user",
				"content": buildPromptOptimizationUserContent(req),
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return promptOptimizationModelResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return promptOptimizationModelResponse{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(a.cfg.DeepSeekAPIKey))
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: time.Duration(fallbackPositiveInt(a.cfg.DeepSeekPromptTimeoutSeconds, 45)) * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return promptOptimizationModelResponse{}, err
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return promptOptimizationModelResponse{}, fmt.Errorf("deepseek prompt optimization failed: status=%d body=%s", resp.StatusCode, string(rawBody))
	}

	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(rawBody, &apiResp); err != nil {
		return promptOptimizationModelResponse{}, err
	}
	for _, choice := range apiResp.Choices {
		if content := cleanOptimizedPromptText(choice.Message.Content); content != "" {
			return parsePromptOptimizationModelResponse(content), nil
		}
	}
	return promptOptimizationModelResponse{}, fmt.Errorf("deepseek returned empty optimized prompt")
}

func normalizePromptOptimizationMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "chat", "rewrite", "realistic", "direction", "commercial", "detail", "safe", "portrait_detail":
		return strings.ToLower(strings.TrimSpace(mode))
	case "portrait", "face", "face_detail":
		return "portrait_detail"
	default:
		return "balanced"
	}
}

func normalizePromptOptimizationAction(action, mode string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "start", "continue", "rewrite", "make_realistic", "change_direction":
		return strings.ToLower(strings.TrimSpace(action))
	}
	switch normalizePromptOptimizationMode(mode) {
	case "rewrite":
		return "rewrite"
	case "realistic":
		return "make_realistic"
	case "direction":
		return "change_direction"
	default:
		return "continue"
	}
}

func normalizePromptChatHistory(history []promptChatMessage) []promptChatMessage {
	if len(history) == 0 {
		return nil
	}
	start := 0
	if len(history) > 16 {
		start = len(history) - 16
	}
	result := make([]promptChatMessage, 0, len(history)-start)
	for _, item := range history[start:] {
		role := strings.ToLower(strings.TrimSpace(item.Role))
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		if role != "user" && role != "assistant" {
			continue
		}
		result = append(result, promptChatMessage{Role: role, Content: content})
	}
	return result
}

func normalizePromptStructuredFields(value promptStructuredFields) promptStructuredFields {
	return promptStructuredFields{
		Subject: strings.TrimSpace(value.Subject),
		Scene:   strings.TrimSpace(value.Scene),
		Style:   strings.TrimSpace(value.Style),
		Usage:   strings.TrimSpace(value.Usage),
	}
}

func buildPromptOptimizationUserContent(req promptOptimizationRequest) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "mode=%s\n", req.Mode)
	fmt.Fprintf(&builder, "action=%s\n", req.Action)
	fmt.Fprintf(&builder, "aspect_ratio=%s\n", fallbackString(req.AspectRatio, "1:1"))
	fmt.Fprintf(&builder, "style_preset=%s\n", req.StylePreset)
	fmt.Fprintf(&builder, "原始提示词：%s\n", req.Prompt)
	if req.Message != "" {
		fmt.Fprintf(&builder, "用户最新补充：%s\n", req.Message)
	}
	if len(req.History) > 0 {
		if rawHistory, err := json.Marshal(req.History); err == nil {
			fmt.Fprintf(&builder, "history=%s\n", string(rawHistory))
		}
	}
	if !isZeroPromptStructuredFields(req.StructuredPrompt) {
		if rawStructured, err := json.Marshal(req.StructuredPrompt); err == nil {
			fmt.Fprintf(&builder, "current_structured_prompt=%s\n", string(rawStructured))
		}
	}
	return strings.TrimSpace(builder.String())
}

func parsePromptOptimizationModelResponse(content string) promptOptimizationModelResponse {
	text := strings.TrimSpace(content)
	if text == "" {
		return promptOptimizationModelResponse{}
	}
	var parsed promptOptimizationModelResponse
	if err := json.Unmarshal([]byte(text), &parsed); err == nil {
		return normalizePromptOptimizationModelResponse(parsed)
	}
	if start, end := strings.Index(text, "{"), strings.LastIndex(text, "}"); start >= 0 && end > start {
		if err := json.Unmarshal([]byte(text[start:end+1]), &parsed); err == nil {
			return normalizePromptOptimizationModelResponse(parsed)
		}
	}
	return promptOptimizationModelResponse{
		OptimizedPrompt: text,
	}
}

func normalizePromptOptimizationModelResponse(value promptOptimizationModelResponse) promptOptimizationModelResponse {
	value.Reply = strings.TrimSpace(value.Reply)
	value.OptimizedPrompt = strings.TrimSpace(value.OptimizedPrompt)
	value.StructuredPrompt = normalizePromptStructuredFields(value.StructuredPrompt)
	value.SafetyNotes = uniqueStrings(value.SafetyNotes)
	if len(value.Directions) > 0 {
		directions := make([]promptDirectionOption, 0, len(value.Directions))
		for _, direction := range value.Directions {
			direction.Title = strings.TrimSpace(direction.Title)
			direction.Summary = strings.TrimSpace(direction.Summary)
			direction.Prompt = strings.TrimSpace(direction.Prompt)
			direction.StructuredPrompt = normalizePromptStructuredFields(direction.StructuredPrompt)
			if direction.Title == "" && direction.Summary == "" && direction.Prompt == "" {
				continue
			}
			directions = append(directions, direction)
		}
		value.Directions = directions
	}
	return value
}

func cleanOptimizedPromptText(value string) string {
	text := strings.TrimSpace(value)
	if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```")
		if newline := strings.Index(text, "\n"); newline >= 0 {
			text = text[newline+1:]
		}
		text = strings.TrimSuffix(strings.TrimSpace(text), "```")
	}
	text = strings.Trim(text, "\"'` \n\t")
	return strings.TrimSpace(text)
}

func sanitizeStructuredPromptFields(value promptStructuredFields, mode string) (promptStructuredFields, []string) {
	fields := normalizePromptStructuredFields(value)
	var notes []string
	fields.Subject, notes = appendPromptSanitizeResult(fields.Subject, mode, notes)
	fields.Scene, notes = appendPromptSanitizeResult(fields.Scene, mode, notes)
	fields.Style, notes = appendPromptSanitizeResult(fields.Style, mode, notes)
	fields.Usage, notes = appendPromptSanitizeResult(fields.Usage, mode, notes)
	return fields, notes
}

func appendPromptSanitizeResult(value, mode string, notes []string) (string, []string) {
	cleaned, fieldNotes := sanitizePromptForImageGeneration(value, mode)
	if cleaned == "" {
		cleaned = strings.TrimSpace(value)
	}
	return cleaned, append(notes, fieldNotes...)
}

func sanitizePromptDirections(values []promptDirectionOption, mode string) ([]promptDirectionOption, []string) {
	if len(values) == 0 {
		return nil, nil
	}
	directions := make([]promptDirectionOption, 0, len(values))
	var notes []string
	for _, value := range values {
		title := strings.TrimSpace(value.Title)
		summary := strings.TrimSpace(value.Summary)
		prompt, promptNotes := sanitizePromptForImageGeneration(value.Prompt, mode)
		notes = append(notes, promptNotes...)
		if prompt == "" {
			prompt = strings.TrimSpace(value.Prompt)
		}
		structuredPrompt, structuredNotes := sanitizeStructuredPromptFields(value.StructuredPrompt, mode)
		notes = append(notes, structuredNotes...)
		if title == "" && summary == "" && prompt == "" {
			continue
		}
		directions = append(directions, promptDirectionOption{
			Title:            title,
			Summary:          summary,
			Prompt:           prompt,
			StructuredPrompt: structuredPrompt,
		})
	}
	return directions, notes
}

func isZeroPromptStructuredFields(value promptStructuredFields) bool {
	return strings.TrimSpace(value.Subject) == "" &&
		strings.TrimSpace(value.Scene) == "" &&
		strings.TrimSpace(value.Style) == "" &&
		strings.TrimSpace(value.Usage) == ""
}

func sanitizePromptForImageGeneration(prompt, mode string) (string, []string) {
	text := strings.TrimSpace(prompt)
	if text == "" {
		return "", nil
	}
	replacements := []struct {
		term        string
		replacement string
	}{
		{"血腥", "戏剧化红色光影"},
		{"暴力", "强烈张力构图"},
		{"裸露", "优雅服饰造型"},
		{"裸体", "优雅服饰造型"},
		{"色情", "时尚视觉"},
		{"成人内容", "成熟商业视觉"},
		{"毒品", "抽象道具"},
		{"仇恨", "冷峻情绪"},
		{"歧视", "多元包容氛围"},
		{"违法", "合规场景"},
		{"枪支", "未来感道具"},
		{"爆炸", "强烈光效"},
	}
	notes := make([]string, 0)
	for _, item := range replacements {
		if strings.Contains(text, item.term) {
			text = strings.ReplaceAll(text, item.term, item.replacement)
			notes = append(notes, "已替换可能导致生成失败的敏感描述")
		}
	}
	var ipNotes []string
	text, ipNotes = sanitizeRecognizableCharacterSignals(text)
	notes = append(notes, ipNotes...)
	var portraitNotes []string
	text, portraitNotes = applyPortraitDetailRule(text, mode)
	notes = append(notes, portraitNotes...)
	text = strings.Join(strings.Fields(text), " ")
	text = strings.Trim(text, "，,。 ")
	return text, uniqueStrings(notes)
}

func applyPortraitDetailRule(prompt, mode string) (string, []string) {
	if normalizePromptOptimizationMode(mode) != "portrait_detail" {
		return prompt, nil
	}
	text := strings.TrimSpace(prompt)
	if text == "" {
		return text, nil
	}
	if containsChildSubject(text) {
		return text, []string{"已避免对未成年人主体添加皮肤微距细节"}
	}
	if !looksLikePortraitPrompt(text) || looksLikeNonPhotorealStyle(text) {
		return text, nil
	}
	details := []string{
		"极近距离人像特写",
		"真实皮肤毛孔清晰可见",
		"细微面部绒毛被逆光勾勒",
		"自然微光泽皮肤质感",
		"电影级柔和布光",
		"超写实 RAW 摄影质感",
	}
	missing := make([]string, 0, len(details))
	for _, detail := range details {
		if !strings.Contains(text, detail) {
			missing = append(missing, detail)
		}
	}
	if len(missing) == 0 {
		return text, []string{"已加入人脸高清细节优化"}
	}
	separator := "，"
	if strings.HasSuffix(text, "，") || strings.HasSuffix(text, ",") {
		separator = ""
	}
	return text + separator + strings.Join(missing, "，"), []string{"已加入人脸高清细节优化"}
}

func looksLikePortraitPrompt(text string) bool {
	return containsAny(text,
		"人像", "肖像", "头像", "脸部", "面部", "五官", "portrait", "face", "facial",
		"成年女性", "成年男性", "女性", "男性", "女人", "男人", "女孩", "男孩",
	)
}

func containsChildSubject(text string) bool {
	return containsAny(text,
		"儿童", "孩子", "小孩", "小朋友", "宝宝", "婴儿", "幼儿", "未成年",
		"小女孩", "小男孩", "少年", "少女", "child", "children", "kid", "teen",
	)
}

func looksLikeNonPhotorealStyle(text string) bool {
	return containsAny(text, "漫画", "插画", "动漫", "卡通", "二次元", "3D渲染", "3D 渲染", "illustration", "anime", "cartoon")
}

func sanitizeRecognizableCharacterSignals(prompt string) (string, []string) {
	text := prompt
	notes := make([]string, 0)
	if looksLikeRecognizablePrincessIP(text) {
		replacements := []struct {
			term        string
			replacement string
		}{
			{"年轻美丽的公主", "原创童话人物"},
			{"肌肤白皙细腻", "自然柔和肤色"},
			{"黑色短发", "深棕色微卷长发"},
			{"黑短发", "深棕色微卷长发"},
			{"红唇", "柔和自然唇色"},
			{"蓝黄配色经典公主裙", "淡蓝与象牙白配色的原创童话礼裙"},
			{"蓝黄配色", "淡蓝与象牙白配色"},
			{"经典公主裙", "原创童话礼裙"},
			{"公主", "原创童话人物"},
			{"小动物环绕", "花草与柔和微光环绕"},
			{"周围小动物环绕", "周围花草与柔和微光环绕"},
		}
		for _, item := range replacements {
			text = strings.ReplaceAll(text, item.term, item.replacement)
		}
		notes = append(notes, "已弱化可能指向知名角色或版权 IP 的标志性组合")
		return text, notes
	}
	if strings.Contains(text, "经典公主裙") {
		text = strings.ReplaceAll(text, "经典公主裙", "原创童话礼裙")
		notes = append(notes, "已弱化可能指向知名角色或版权 IP 的标志性组合")
	}
	return text, notes
}

func looksLikeRecognizablePrincessIP(text string) bool {
	if !strings.Contains(text, "公主") {
		return false
	}
	signals := 0
	for _, group := range [][]string{
		{"黑色短发", "黑短发"},
		{"红唇"},
		{"蓝黄配色", "蓝黄", "蓝黄裙"},
		{"经典公主裙"},
		{"森林"},
		{"小动物环绕", "小动物"},
	} {
		if containsAny(text, group...) {
			signals++
		}
	}
	return signals >= 4
}

func containsAny(text string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func fallbackPositiveInt(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}
