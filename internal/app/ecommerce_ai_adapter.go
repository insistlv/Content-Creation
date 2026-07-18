package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"image-agent/internal/app/ecommerce"
)

type commerceVisionAnalyzerAdapter struct{ app *App }

type commerceVisionAdapterError struct {
	message   string
	retryable bool
}

func (e *commerceVisionAdapterError) Error() string   { return e.message }
func (e *commerceVisionAdapterError) Retryable() bool { return e.retryable }

func newCommerceVisionAnalyzerAdapter(app *App) *commerceVisionAnalyzerAdapter {
	return &commerceVisionAnalyzerAdapter{app: app}
}

func (a *commerceVisionAnalyzerAdapter) CommerceVisionConfigured(context.Context) (bool, error) {
	candidates, err := a.app.commerceVisionModelCandidates()
	return len(candidates) > 0, err
}

func (a *App) commerceVisionModelCandidates() ([]modelCenterCandidate, error) {
	settings, err := a.loadSettings()
	if err != nil {
		return nil, err
	}
	candidates, err := a.modelCenterCandidatesForGeneration(settings, ModelConfigTypeChat, 0)
	if err != nil {
		return nil, err
	}
	filtered := make([]modelCenterCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		tags := map[string]bool{}
		for _, tag := range candidate.Model.CapabilityTags {
			tags[strings.TrimSpace(tag)] = true
		}
		if candidate.Model.Modality == ModelConfigTypeChat && tags["vision"] && tags["commerce_vision"] && candidate.Channel.Status == ModelCenterStatusOnline && candidate.Provider.Status == ModelCenterStatusOnline {
			filtered = append(filtered, candidate)
		}
	}
	return filtered, nil
}

func (a *commerceVisionAnalyzerAdapter) AnalyzeProduct(ctx context.Context, input ecommerce.ProductAnalysisRequest) (string, error) {
	if a == nil || a.app == nil || input.JobID == 0 || input.UserID == 0 || input.ProjectID == 0 || input.CreativeSpecID == 0 {
		return "", &commerceVisionAdapterError{message: "product analysis invocation context is invalid"}
	}
	assets, err := a.signedProductAssets(input)
	if err != nil {
		return "", err
	}
	candidates, err := a.app.commerceVisionModelCandidates()
	if err != nil {
		return "", err
	}
	if len(candidates) == 0 {
		return "", &commerceVisionAdapterError{message: "commerce vision channel is unavailable", retryable: true}
	}
	assetIDs, _ := json.Marshal(input.SourceAssetIDs)
	for _, candidate := range candidates {
		started := time.Now()
		raw, requestID, callErr := callCommerceVisionChannel(ctx, candidate, input, assets)
		latency := time.Since(started).Milliseconds()
		if latency < 0 {
			latency = 0
		}
		invocation := ecommerce.CommerceAIInvocation{JobID: input.JobID, UserID: input.UserID, ProjectID: input.ProjectID, Purpose: "product_analysis", ModelID: candidate.Model.ID, ChannelID: candidate.Channel.ID, LatencyMS: latency, ProviderRequestID: requestID, RequestAssetIDsJSON: string(assetIDs), ResponseSchemaVersion: 2, CreatedAt: time.Now().UTC()}
		if callErr != nil {
			invocation.Status = "failed"
			invocation.ErrorCode = "provider_failed"
			invocation.ErrorMessage = "commerce vision provider failed"
			if err := a.app.db.Create(&invocation).Error; err != nil {
				return "", err
			}
			continue
		}
		invocation.Status = "succeeded"
		if err := a.app.db.Create(&invocation).Error; err != nil {
			return "", err
		}
		return raw, nil
	}
	return "", &commerceVisionAdapterError{message: "commerce vision provider failed", retryable: true}
}

type commerceSignedProductAsset struct {
	ID                  uint
	Role, URL, MIMEType string
}

func (a *commerceVisionAnalyzerAdapter) signedProductAssets(input ecommerce.ProductAnalysisRequest) ([]commerceSignedProductAsset, error) {
	var commerceAssets []ecommerce.CommerceAsset
	if err := a.app.db.Where("id IN ? AND user_id = ? AND project_id = ? AND role IN ?", input.SourceAssetIDs, input.UserID, input.ProjectID, ecommerce.ProductAnalysisAssetRoles()).Find(&commerceAssets).Error; err != nil {
		return nil, err
	}
	if len(commerceAssets) != len(input.SourceAssetIDs) {
		return nil, ecommerce.ErrOwnershipMismatch
	}
	byID := make(map[uint]ecommerce.CommerceAsset, len(commerceAssets))
	for _, asset := range commerceAssets {
		byID[asset.ID] = asset
	}
	result := make([]commerceSignedProductAsset, 0, len(input.SourceAssetIDs))
	for _, id := range input.SourceAssetIDs {
		asset := byID[id]
		var reference ReferenceAsset
		if err := a.app.db.Where("id = ? AND user_id = ?", asset.ReferenceAssetID, input.UserID).First(&reference).Error; err != nil {
			return nil, err
		}
		url, err := a.app.referenceAssetAccessURL(reference, "image/png", true, false)
		if err != nil {
			return nil, err
		}
		result = append(result, commerceSignedProductAsset{ID: id, Role: asset.Role, URL: url, MIMEType: fallbackString(reference.MIMEType, "image/png")})
	}
	return result, nil
}

func callCommerceVisionChannel(ctx context.Context, candidate modelCenterCandidate, input ecommerce.ProductAnalysisRequest, assets []commerceSignedProductAsset) (string, string, error) {
	endpoint := strings.TrimSpace(candidate.Channel.Endpoint)
	baseURL := strings.TrimRight(strings.TrimSpace(candidate.Provider.BaseURL), "/")
	if baseURL == "" {
		return "", "", errors.New("commerce vision provider base URL is empty")
	}
	if endpoint == "" {
		endpoint = "/v1/chat/completions"
	}
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = baseURL + "/" + strings.TrimLeft(endpoint, "/")
	}
	contextJSON, _ := json.Marshal(input.AssetContexts)
	prompt := "Analyze only directly observable product facts. Never infer or guess color, size, capacity, material, price, certification, or efficacy from an SKU code or identifier. Shared assets have shared=true; SKU-specific assets carry sku_id. Put facts supported only by shared assets in common_facts. Put facts supported by SKU-specific assets only in sku_overrides keyed by the decimal SKU ID; never leak an SKU-specific fact into common_facts or another SKU. observed_facts must contain the union of common_facts and sku_overrides for backward compatibility. If an SKU has no specific image, inherit shared visual evidence and report the missing SKU-specific evidence in risk_notices. Return strict JSON with observed_facts, common_facts, sku_overrides, selling_points, forbidden_changes, brand_tone, missing_fields, risk_notices, suggested_sections; never return user_overrides. Only observed_facts.value, selling_points, forbidden_changes, brand_tone.description, and risk_notices must use Simplified Chinese. observed_facts.field, missing_fields, suggested_sections, all JSON keys, and enum values must remain the English values defined by the schema. Every fact must keep source_asset_ids using these commerce asset IDs. Asset contexts: " + string(contextJSON) + ". User requirements: " + input.UserRequirements
	var payload any
	if strings.Contains(strings.ToLower(endpoint), "responses") {
		content := []map[string]any{{"type": "input_text", "text": prompt}}
		for _, asset := range assets {
			content = append(content, map[string]any{"type": "input_image", "image_url": asset.URL})
		}
		payload = map[string]any{"model": candidate.Channel.RuntimeModel, "input": []map[string]any{{"role": "user", "content": content}}, "text": map[string]any{"format": commerceVisionResponsesFormat()}}
	} else {
		content := []map[string]any{{"type": "text", "text": prompt}}
		for _, asset := range assets {
			content = append(content, map[string]any{"type": "image_url", "image_url": map[string]any{"url": asset.URL}})
		}
		payload = map[string]any{"model": candidate.Channel.RuntimeModel, "messages": []map[string]any{{"role": "user", "content": content}}, "response_format": commerceVisionChatResponseFormat()}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", "", err
	}
	timeout := candidate.Provider.DefaultTimeoutSeconds
	if timeout <= 0 {
		timeout = 45
	}
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if key := strings.TrimSpace(candidate.Provider.APIKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode >= 400 {
		return "", "", fmt.Errorf("commerce vision provider status %d", resp.StatusCode)
	}
	return parseCommerceVisionProviderResponse(responseBody)
}

func commerceVisionResponsesFormat() map[string]any {
	return map[string]any{"type": "json_schema", "name": "commerce_product_report", "strict": true, "schema": commerceVisionReportSchema()}
}

func commerceVisionChatResponseFormat() map[string]any {
	return map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "commerce_product_report", "strict": true, "schema": commerceVisionReportSchema()}}
}

func commerceVisionReportSchema() map[string]any {
	stringArray := func() map[string]any {
		return map[string]any{"type": "array", "items": map[string]any{"type": "string", "minLength": 1}}
	}
	schema := map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"observed_facts", "common_facts", "sku_overrides", "selling_points", "forbidden_changes", "brand_tone", "missing_fields", "risk_notices", "suggested_sections"},
		"properties": map[string]any{
			"common_facts": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object", "additionalProperties": false,
					"required": []string{"field", "value", "confidence", "source_asset_ids"},
					"properties": map[string]any{
						"field":            map[string]any{"type": "string", "minLength": 1},
						"value":            map[string]any{"type": "string"},
						"confidence":       map[string]any{"type": "number", "minimum": 0, "maximum": 1},
						"source_asset_ids": map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"type": "integer", "minimum": 1}},
					},
				},
			},
			"sku_overrides":     map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "array", "items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"field", "value", "confidence", "source_asset_ids"}, "properties": map[string]any{"field": map[string]any{"type": "string", "minLength": 1}, "value": map[string]any{"type": "string"}, "confidence": map[string]any{"type": "number", "minimum": 0, "maximum": 1}, "source_asset_ids": map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"type": "integer", "minimum": 1}}}}}},
			"selling_points":    stringArray(),
			"forbidden_changes": stringArray(),
			"brand_tone": map[string]any{
				"type": "object", "additionalProperties": false,
				"required":   []string{"description"},
				"properties": map[string]any{"description": map[string]any{"type": "string"}},
			},
			"missing_fields":     stringArray(),
			"risk_notices":       stringArray(),
			"suggested_sections": map[string]any{"type": "array", "items": map[string]any{"type": "string", "minLength": 1, "enum": []string{"hero", "selling_points", "material", "detail", "usage", "specification", "closing"}}},
		},
	}
	properties := schema["properties"].(map[string]any)
	properties["observed_facts"] = properties["common_facts"]
	return schema
}

func parseCommerceVisionProviderResponse(body []byte) (string, string, error) {
	var response struct {
		ID      string `json:"id"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Output []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", "", err
	}
	for _, choice := range response.Choices {
		if text := strings.TrimSpace(choice.Message.Content); text != "" {
			if strings.Contains(text, "```") {
				return "", response.ID, errors.New("markdown response is not allowed")
			}
			return text, response.ID, nil
		}
	}
	for _, output := range response.Output {
		for _, content := range output.Content {
			if text := strings.TrimSpace(content.Text); text != "" {
				if strings.Contains(text, "```") {
					return "", response.ID, errors.New("markdown response is not allowed")
				}
				return text, response.ID, nil
			}
		}
	}
	return "", response.ID, errors.New("commerce vision response is empty")
}
