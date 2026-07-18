package app

// 本文件从 generation.go 拆分：生成请求归一化、工具模式参数校验与状态归一化。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func normalizeGenerationQuality(quality string) string {
	quality = strings.ToLower(strings.TrimSpace(quality))
	if quality == "" {
		return GenerationQualityMedium
	}
	return quality
}

func normalizeGenerationNum(req *generationRequest) bool {
	if req == nil {
		return true
	}
	req.Num = 1
	if req.BatchID == "" {
		req.BatchIndex = 0
		req.BatchTotal = 0
	}
	return true
}

func normalizeGenerationBatchMetadata(req *generationRequest) {
	req.BatchID = strings.TrimSpace(req.BatchID)
	if req.BatchID == "" {
		req.BatchIndex = 0
		req.BatchTotal = 0
		return
	}
	if req.BatchIndex < 0 {
		req.BatchIndex = 0
	}
	if req.BatchTotal < req.BatchIndex+1 {
		req.BatchTotal = req.BatchIndex + 1
	}
	if req.BatchTotal < 1 {
		req.BatchTotal = 1
	}
	if req.BatchTotal > 16 {
		req.BatchTotal = 16
	}
	if req.BatchIndex >= req.BatchTotal {
		req.BatchIndex = req.BatchTotal - 1
	}
}

func normalizeGenerationVariationMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case "stable", "balanced", "bold":
		return strings.TrimSpace(mode)
	default:
		return ""
	}
}

func normalizeGenerationReferenceIntent(intent string) string {
	switch strings.ToLower(strings.TrimSpace(intent)) {
	case "":
		return ""
	case GenerationReferenceIntentCompose:
		return GenerationReferenceIntentCompose
	case GenerationReferenceIntentCharacter:
		return GenerationReferenceIntentCharacter
	case GenerationReferenceIntentCreative:
		return GenerationReferenceIntentCreative
	default:
		return strings.ToLower(strings.TrimSpace(intent))
	}
}

func isValidGenerationReferenceIntent(intent string) bool {
	switch intent {
	case "", GenerationReferenceIntentCompose, GenerationReferenceIntentCharacter, GenerationReferenceIntentCreative:
		return true
	default:
		return false
	}
}

func normalizeGenerationBackgroundReferenceIndex(req *generationRequest, referenceInputCount int) bool {
	if req == nil {
		return true
	}
	if req.BackgroundReferenceIndex != nil {
		index := *req.BackgroundReferenceIndex
		if index < 0 {
			return false
		}
		if referenceInputCount <= 0 || index >= referenceInputCount {
			return false
		}
		return true
	}
	return true
}

func normalizeGenerationToolMode(mode string) string {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return GenerationToolModeGenerate
	}
	return strings.ToLower(mode)
}

func normalizeGenerationToolOptions(options map[string]any) map[string]any {
	if len(options) == 0 {
		return nil
	}
	normalized := make(map[string]any, len(options))
	for key, value := range options {
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "" {
			continue
		}
		normalized[key] = value
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func normalizeGenerationToolOptionsForMode(mode string, options map[string]any) map[string]any {
	switch normalizeGenerationToolMode(mode) {
	case GenerationToolModeExpand:
		normalized, ok := normalizedExpandToolOptions(options)
		if !ok {
			return options
		}
		return map[string]any{
			"unit":   normalized.Unit,
			"top":    normalized.Top,
			"bottom": normalized.Bottom,
			"left":   normalized.Left,
			"right":  normalized.Right,
		}
	default:
		return options
	}
}

func validateGenerationToolOptions(req generationRequest) (bool, string) {
	switch normalizeGenerationToolMode(req.ToolMode) {
	case GenerationToolModeExpand:
		if !isValidExpandToolOptions(req.ToolOptions) {
			return false, "扩图边距百分比必须在 0 到 100 之间，且至少设置一个方向"
		}
	case GenerationToolModeUpscale:
		if normalizedUpscaleToolScale(req.ToolOptions) == "" {
			return false, "放大倍率必须为 2x、4x 或 8x"
		}
	case GenerationToolModeErase:
		if strings.TrimSpace(req.EditInstruction) == "" && req.MaskAssetID == nil && !hasEraseMaskRegion(req.ToolOptions) {
			return false, "移除物体需要填写移除说明或圈选区域"
		}
		if !hasValidEraseMaskRegions(req.ToolOptions) {
			return false, "圈选区域坐标必须在 0 到 1 之间"
		}
	case GenerationToolModePrecisionEdit:
		if strings.TrimSpace(req.EditInstruction) == "" {
			return false, "精细编辑需要填写局部编辑指令"
		}
		if req.MaskAssetID == nil && !hasPrecisionEditRegion(req.ToolOptions) {
			return false, "精细编辑需要上传蒙版或圈选区域"
		}
		if !hasValidEraseMaskRegions(req.ToolOptions) {
			return false, "圈选区域坐标必须在 0 到 1 之间"
		}
	}
	return true, ""
}

func hasEraseMaskRegion(options map[string]any) bool {
	return len(maskRegionsFromToolOptions(options)) > 0
}

func hasValidEraseMaskRegions(options map[string]any) bool {
	regions, ok := rawMaskRegions(options)
	if !ok {
		return true
	}
	if len(regions) == 0 {
		return true
	}
	return len(maskRegionsFromToolOptions(options)) == len(regions)
}

func maskRegionsFromToolOptions(options map[string]any) []ImageMaskRegion {
	regions, ok := rawMaskRegions(options)
	if !ok || len(regions) == 0 {
		return nil
	}
	normalized := make([]ImageMaskRegion, 0, len(regions))
	for _, item := range regions {
		values, ok := item.(map[string]any)
		if !ok {
			continue
		}
		region := ImageMaskRegion{
			X:      floatToolOption(values, "x"),
			Y:      floatToolOption(values, "y"),
			Width:  floatToolOption(values, "width"),
			Height: floatToolOption(values, "height"),
		}
		if isValidMaskRegion(region) {
			normalized = append(normalized, region)
		}
	}
	return normalized
}

func rawMaskRegions(options map[string]any) ([]any, bool) {
	if len(options) == 0 {
		return nil, false
	}
	raw, ok := options["mask_regions"]
	if !ok {
		raw, ok = options["regions"]
	}
	if !ok {
		return nil, false
	}
	switch regions := raw.(type) {
	case []any:
		return regions, true
	default:
		return nil, true
	}
}

func isValidMaskRegion(region ImageMaskRegion) bool {
	return region.X >= 0 && region.X <= 1 &&
		region.Y >= 0 && region.Y <= 1 &&
		region.Width > 0 && region.Width <= 1 &&
		region.Height > 0 && region.Height <= 1 &&
		region.X+region.Width <= 1.000001 &&
		region.Y+region.Height <= 1.000001
}

type expandToolOptions struct {
	Unit   string
	Top    int
	Bottom int
	Left   int
	Right  int
}

func normalizedExpandToolOptions(options map[string]any) (expandToolOptions, bool) {
	if len(options) == 0 {
		return expandToolOptions{}, false
	}
	unit := strings.ToLower(strings.TrimSpace(fmt.Sprint(options["unit"])))
	if unit == "" {
		unit = "percent"
	}
	normalized := expandToolOptions{
		Unit:   unit,
		Top:    intToolOption(options, "top"),
		Bottom: intToolOption(options, "bottom"),
		Left:   intToolOption(options, "left"),
		Right:  intToolOption(options, "right"),
	}
	return normalized, true
}

func isValidExpandToolOptions(options map[string]any) bool {
	normalized, ok := normalizedExpandToolOptions(options)
	if !ok || normalized.Unit != "percent" {
		return false
	}
	total := normalized.Top + normalized.Bottom + normalized.Left + normalized.Right
	for _, value := range []int{normalized.Top, normalized.Bottom, normalized.Left, normalized.Right} {
		if value < 0 || value > 100 {
			return false
		}
	}
	return total > 0
}

func normalizedUpscaleToolScale(options map[string]any) string {
	if len(options) == 0 || options["scale"] == nil || strings.TrimSpace(fmt.Sprint(options["scale"])) == "" {
		return "2x"
	}
	scale := strings.ToLower(strings.TrimSpace(fmt.Sprint(options["scale"])))
	switch scale {
	case "2x", "4x", "8x":
		return scale
	default:
		return ""
	}
}

func hasPrecisionEditRegion(options map[string]any) bool {
	if len(options) == 0 {
		return false
	}
	if regions, ok := options["regions"].([]any); ok && len(regions) > 0 {
		return true
	}
	if regions, ok := options["mask_regions"].([]any); ok && len(regions) > 0 {
		return true
	}
	return false
}

func intToolOption(options map[string]any, key string) int {
	value, ok := options[key]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case json.Number:
		next, _ := typed.Int64()
		return int(next)
	default:
		next, _ := strconv.Atoi(strings.TrimSpace(fmt.Sprint(value)))
		return next
	}
}

func floatToolOption(options map[string]any, key string) float64 {
	value, ok := options[key]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case json.Number:
		next, _ := typed.Float64()
		return next
	default:
		next, _ := strconv.ParseFloat(strings.TrimSpace(fmt.Sprint(value)), 64)
		return next
	}
}

func (a *App) validateMaskAsset(c *gin.Context, userID uint, maskAssetID *uint) bool {
	if maskAssetID == nil || *maskAssetID == 0 {
		return true
	}
	var count int64
	if err := a.db.Model(&ReferenceAsset{}).Where("id = ? AND user_id = ?", *maskAssetID, userID).Count(&count).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "reference_asset_load_failed", "蒙版素材读取失败")
		return false
	}
	if count == 0 {
		writeError(c, http.StatusBadRequest, "invalid_generation_parameter", "蒙版素材不存在")
		return false
	}
	return true
}

func encodeGenerationToolOptions(options map[string]any) string {
	if len(options) == 0 {
		return ""
	}
	payload, err := json.Marshal(options)
	if err != nil {
		return ""
	}
	return string(payload)
}

func decodeGenerationToolOptions(value string) map[string]any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	var options map[string]any
	if err := json.Unmarshal([]byte(value), &options); err != nil {
		return nil
	}
	return normalizeGenerationToolOptions(options)
}

func generationPromptForToolMode(req generationRequest) string {
	text := strings.TrimSpace(req.Prompt)
	toolMode := normalizeGenerationToolMode(req.ToolMode)
	if toolMode == GenerationToolModeRemoveBackground {
		instruction := "请移除图片背景，保留主体和主体边缘细节，输出透明背景 PNG。不要改变主体材质、颜色、比例或构图。"
		if text == "" {
			text = instruction
		} else {
			text += "\n" + instruction
		}
		if editInstruction := strings.TrimSpace(req.EditInstruction); editInstruction != "" {
			text += "\n主体保留说明：" + editInstruction
		}
	}
	if toolMode == GenerationToolModeErase {
		if instruction := strings.TrimSpace(req.EditInstruction); instruction != "" {
			text += "\n移除说明：" + instruction
		}
		text += "\n请移除指定物体或干扰元素，自然补全背景纹理、光线、阴影和透视，保持未指定主体、构图、画风和细节不变。"
		if req.MaskAssetID != nil || hasEraseMaskRegion(req.ToolOptions) {
			text += "\n如提供蒙版或圈选区域，仅处理圈选区域；蒙版白色区域为待移除区域，黑色或透明区域必须保持不变。"
		}
	}
	if toolMode == GenerationToolModeExpand {
		expandOptions, _ := normalizedExpandToolOptions(req.ToolOptions)
		text += "\n" + fmt.Sprintf("扩图设置：单位百分比，上 %d%%、下 %d%%、左 %d%%、右 %d%%。", expandOptions.Top, expandOptions.Bottom, expandOptions.Left, expandOptions.Right)
		text += "\n请只补全透明外扩区域，不要裁切、缩放或重绘原图主体；保持原图主体、光线、透视、材质、景深和画风一致，让新增边界自然衔接。"
	}
	if toolMode == GenerationToolModeUpscale {
		text += "\n" + "放大倍率：" + normalizedUpscaleToolScale(req.ToolOptions)
		if editInstruction := strings.TrimSpace(req.EditInstruction); editInstruction != "" {
			text += "\n增强说明：" + editInstruction
		}
		text += "\n请进行 AI 高清增强，按目标倍率增强清晰度、纹理细节和边缘质量；保持主体、颜色、构图和内容不变，不新增物体，不改变画风，不要重绘成新图。"
	}
	if toolMode == GenerationToolModePrecisionEdit {
		text += "\n局部编辑指令：" + strings.TrimSpace(req.EditInstruction)
		text += "\n仅修改圈选区域或蒙版白色区域，保持未选区域不变。严格保持未选区域的主体、颜色、构图、材质、文字和边缘不变，不要重绘整张图。"
	}
	return strings.TrimSpace(text)
}

func isValidGenerationQuality(quality string) bool {
	switch quality {
	case GenerationQualityLow, GenerationQualityMedium, GenerationQualityHigh, GenerationQualityUltra:
		return true
	default:
		return false
	}
}

func isValidGenerationToolMode(mode string) bool {
	switch mode {
	case GenerationToolModeGenerate, GenerationToolModeRedraw, GenerationToolModeErase, GenerationToolModeExpand, GenerationToolModeUpscale, GenerationToolModeRemoveBackground, GenerationToolModePrecisionEdit, GenerationToolModeVirtualTryOn:
		return true
	default:
		return false
	}
}

func isEditToolMode(mode string) bool {
	switch mode {
	case GenerationToolModeRedraw, GenerationToolModeErase, GenerationToolModeExpand, GenerationToolModeUpscale, GenerationToolModeRemoveBackground, GenerationToolModePrecisionEdit:
		return true
	default:
		return false
	}
}

func isPercentRange(value int) bool {
	return value >= 0 && value <= 100
}

func normalizeGenerationStatus(status string) string {
	switch status {
	case "", GenerationStatusQueued:
		return GenerationStatusQueued
	case GenerationStatusRunning:
		return GenerationStatusRunning
	case GenerationStatusSucceeded:
		return GenerationStatusSucceeded
	case GenerationStatusFailed:
		return GenerationStatusFailed
	default:
		return status
	}
}

func normalizeGenerationStage(status, stage string) string {
	switch {
	case strings.TrimSpace(stage) != "":
		return stage
	case normalizeGenerationStatus(status) == GenerationStatusQueued:
		return GenerationStageQueued
	case normalizeGenerationStatus(status) == GenerationStatusRunning:
		return GenerationStageRequestingProvider
	case normalizeGenerationStatus(status) == GenerationStatusSucceeded:
		return GenerationStageSucceeded
	case normalizeGenerationStatus(status) == GenerationStatusFailed:
		return GenerationStageFailed
	default:
		return GenerationStageQueued
	}
}
