package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"log"
	"math"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "image/gif"
	_ "image/jpeg"
)

type ImageProvider interface {
	Generate(ctx context.Context, input ImageGenerationInput) (ImageGenerationResult, *ProviderError)
}

type VideoProvider interface {
	SubmitVideo(ctx context.Context, input VideoGenerationInput) (VideoSubmitResult, *ProviderError)
	PollVideo(ctx context.Context, taskID string, input VideoGenerationInput) (VideoTaskResult, *ProviderError)
}

type MusicProvider interface {
	GenerateMusic(ctx context.Context, input MusicGenerationInput) (MusicGenerationResult, *ProviderError)
}

type OpenAIProvider struct {
	baseURL       string
	apiKey        string
	arkAPIKey     string
	zzAPIKey      string
	client        *http.Client
	spoolPath     string
	spoolMaxBytes int64
}

const (
	providerFailureStageImageGenerationRequest     = "image_generation_request"
	providerFailureStageResponsesGenerationRequest = "responses_generation_request"
	providerFailureStageProviderAssetFetch         = "provider_asset_fetch"
	providerFailureStageVideoSubmitRequest         = "video_submit_request"
	providerFailureStageVideoPollRequest           = "video_poll_request"
)

const (
	wuyinGrokImagineRuntimeModel    = "grok-imagine-video-1.5-preview"
	wuyinGrokImagineSubmitEndpoint  = "/api/async/video_grok_imagine"
	wuyinGrokImagineDetailEndpoint  = "/api/async/detail"
	wuyinGrokImagineProviderBaseURL = "https://api.wuyinkeji.com"
)

const (
	arkSeedanceMiniRuntimeModel    = "doubao-seed-2-0-mini-260428"
	arkSeedance2RuntimeModel       = "doubao-seedance-2-0-260128"
	arkVideoProviderName           = "Volcengine Ark"
	arkVideoProviderBaseURL        = "https://ark.cn-beijing.volces.com/api/v3"
	arkVideoTasksEndpoint          = "/contents/generations/tasks"
	arkVideoReferenceImageMaxCount = 9
	arkVideoReferenceMediaMaxCount = 3
)

const (
	zzVideoDSFastRuntimeModel = "video-ds-2.0-fast"
	zzVideoProviderName       = "ZZ API"
	zzVideoProviderCode       = "zz"
	zzVideoProviderBaseURL    = "https://zz1cc.cc.cd"
	zzVideoEndpoint           = "/v1/videos"
	zzVideoReferenceMaxCount  = 3
)

const (
	imageReferenceTransportModeNone                 = "none"
	imageReferenceTransportModeImagesDirect         = "images_direct"
	imageReferenceTransportModeImagesReferenceSheet = "images_reference_sheet"
	imageReferenceTransportModeImagesEditsMultipart = "images_edits_multipart"
	imageReferenceTransportModeResponsesMultiImage  = "responses_multi_image"
	imageReferenceTransportModeChatMultiImage       = "chat_multi_image"

	imageReferenceSheetPromptInstruction        = "参考图已合成为一张多图网格：请综合所有分区的主体、风格、材质和构图，不要只参考第一块。"
	imageComposeReferenceSheetPromptInstruction = "参考图已合成为一张带编号标签的多图网格：每个分区左上角标注【图1】【图2】等编号，请严格按编号理解参考图映射。"
	imageDefaultReferencePromptInstruction      = "参考图约束：上传图片是本次生成的主要视觉参考，请优先保留其中的主体、构图、风格或用户指定元素，并围绕用户提示词进行生成。"
)

func (p *OpenAIProvider) SubmitVideo(ctx context.Context, input VideoGenerationInput) (VideoSubmitResult, *ProviderError) {
	if isWuyinGrokImagineVideoInput(input) {
		return p.submitWuyinGrokImagineVideo(ctx, input)
	}
	if isArkSeedanceVideoInput(input) {
		return p.submitArkSeedanceVideo(ctx, input)
	}
	if isZZVideoInput(input) {
		return p.submitZZVideo(ctx, input)
	}

	payload := map[string]any{
		"prompt":       strings.TrimSpace(input.Prompt),
		"model":        fallbackString(strings.TrimSpace(input.Model), "sora-2"),
		"aspect_ratio": fallbackString(strings.TrimSpace(input.AspectRatio), "16:9"),
		"duration":     fallbackString(strings.TrimSpace(input.Duration), "10"),
		"hd":           input.HD,
		"watermark":    input.Watermark,
		"private":      input.Private,
	}
	if len(input.Images) > 0 {
		payload["images"] = input.Images
	}
	if hook := strings.TrimSpace(input.NotifyHook); hook != "" {
		payload["notify_hook"] = hook
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return VideoSubmitResult{}, &ProviderError{Code: "provider_request_build_failed", Message: err.Error()}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, providerBaseURL(p.baseURL, ImageGenerationInput{
		ProviderBaseURL: input.ProviderBaseURL,
	})+providerAPIEndpoint(ImageGenerationInput{ProviderAPIEndpoint: input.ProviderAPIEndpoint}, "/v2/videos/generations"), bytes.NewReader(body))
	if err != nil {
		return VideoSubmitResult{}, &ProviderError{Code: "provider_request_build_failed", Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+providerAPIKey(p.apiKey, ImageGenerationInput{ProviderAPIKey: input.ProviderAPIKey}))
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return VideoSubmitResult{}, &ProviderError{Code: "provider_request_failed", Message: err.Error(), FailureStage: providerFailureStageVideoSubmitRequest}
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(resp.Body)
	requestID := resp.Header.Get("x-request-id")
	if resp.StatusCode >= 400 {
		return VideoSubmitResult{}, providerHTTPError(resp.StatusCode, rawBody, requestID, providerFailureStageVideoSubmitRequest)
	}

	var apiResp struct {
		TaskID string `json:"task_id"`
	}
	if providerErr := decodeProviderJSON(rawBody, &apiResp, requestID, providerFailureStageVideoSubmitRequest); providerErr != nil {
		return VideoSubmitResult{}, providerErr
	}
	if strings.TrimSpace(apiResp.TaskID) == "" {
		return VideoSubmitResult{}, &ProviderError{Code: "provider_empty_task", Message: "provider returned no task_id", ProviderRequestID: requestID}
	}
	return VideoSubmitResult{TaskID: apiResp.TaskID, ProviderRequestID: requestID}, nil
}

func applyImageIdempotencyHeader(req *http.Request, input ImageGenerationInput) {
	if req != nil && input.SupportsIdempotencyKey && strings.TrimSpace(input.IdempotencyKey) != "" {
		req.Header.Set("Idempotency-Key", strings.TrimSpace(input.IdempotencyKey))
	}
}

func (p *OpenAIProvider) PollVideo(ctx context.Context, taskID string, input VideoGenerationInput) (VideoTaskResult, *ProviderError) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return VideoTaskResult{}, &ProviderError{Code: "provider_empty_task", Message: "task_id is required"}
	}
	if isWuyinGrokImagineVideoInput(input) {
		return p.pollWuyinGrokImagineVideo(ctx, taskID, input)
	}
	if isArkSeedanceVideoInput(input) {
		return p.pollArkSeedanceVideo(ctx, taskID, input)
	}
	if isZZVideoInput(input) {
		return p.pollZZVideo(ctx, taskID, input)
	}

	base := providerBaseURL(p.baseURL, ImageGenerationInput{ProviderBaseURL: input.ProviderBaseURL})
	endpoint := providerAPIEndpoint(ImageGenerationInput{ProviderAPIEndpoint: input.ProviderAPIEndpoint}, "/v2/videos/generations")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+strings.TrimRight(endpoint, "/")+"/"+taskID, nil)
	if err != nil {
		return VideoTaskResult{}, &ProviderError{Code: "provider_request_build_failed", Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+providerAPIKey(p.apiKey, ImageGenerationInput{ProviderAPIKey: input.ProviderAPIKey}))

	resp, err := p.client.Do(req)
	if err != nil {
		return VideoTaskResult{}, &ProviderError{Code: "provider_request_failed", Message: err.Error(), FailureStage: providerFailureStageVideoPollRequest}
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(resp.Body)
	requestID := resp.Header.Get("x-request-id")
	if resp.StatusCode >= 400 {
		return VideoTaskResult{}, providerHTTPError(resp.StatusCode, rawBody, requestID, providerFailureStageVideoPollRequest)
	}

	var apiResp struct {
		TaskID     string `json:"task_id"`
		Status     string `json:"status"`
		FailReason string `json:"fail_reason"`
		Progress   string `json:"progress"`
		Data       struct {
			Output string `json:"output"`
		} `json:"data"`
	}
	if providerErr := decodeProviderJSON(rawBody, &apiResp, requestID, providerFailureStageVideoPollRequest); providerErr != nil {
		return VideoTaskResult{}, providerErr
	}

	result := VideoTaskResult{
		TaskID:            fallbackString(apiResp.TaskID, taskID),
		Status:            strings.TrimSpace(apiResp.Status),
		Progress:          strings.TrimSpace(apiResp.Progress),
		FailReason:        strings.TrimSpace(apiResp.FailReason),
		OutputURL:         strings.TrimSpace(apiResp.Data.Output),
		MIMEType:          "video/mp4",
		ProviderRequestID: requestID,
	}
	if result.Status == VideoTaskSucceeded && result.OutputURL != "" {
		base64Video, mimeType, providerErr := p.fetchBinaryURL(ctx, result.OutputURL, requestID)
		if providerErr != nil {
			return VideoTaskResult{}, providerErr
		}
		result.OutputBase64 = base64Video
		result.MIMEType = mimeType
	}
	return result, nil
}

func (p *OpenAIProvider) submitArkSeedanceVideo(ctx context.Context, input VideoGenerationInput) (VideoSubmitResult, *ProviderError) {
	if len(input.Images) > arkVideoReferenceImageMaxCount {
		return VideoSubmitResult{}, &ProviderError{
			Code:         "reference_asset_limit_exceeded",
			Message:      "Ark video supports at most 9 reference images",
			FailureStage: providerFailureStageVideoSubmitRequest,
		}
	}
	if len(input.ReferenceVideos) > arkVideoReferenceMediaMaxCount {
		return VideoSubmitResult{}, &ProviderError{
			Code:         "reference_video_limit_exceeded",
			Message:      "Ark Seedance 2.0 supports at most 3 reference videos",
			FailureStage: providerFailureStageVideoSubmitRequest,
		}
	}
	if len(input.ReferenceAudios) > arkVideoReferenceMediaMaxCount {
		return VideoSubmitResult{}, &ProviderError{
			Code:         "reference_audio_limit_exceeded",
			Message:      "Ark Seedance 2.0 supports at most 3 reference audios",
			FailureStage: providerFailureStageVideoSubmitRequest,
		}
	}
	if (len(input.ReferenceVideos) > 0 || len(input.ReferenceAudios) > 0 || input.GenerateAudio) && arkSeedanceVideoRuntimeModel(input) != arkSeedance2RuntimeModel {
		return VideoSubmitResult{}, &ProviderError{
			Code:         "reference_media_unsupported",
			Message:      "Ark reference video/audio requires Seedance 2.0",
			FailureStage: providerFailureStageVideoSubmitRequest,
		}
	}
	apiKey := p.arkVideoAPIKey(input)
	if apiKey == "" {
		return VideoSubmitResult{}, &ProviderError{
			Code:         "provider_api_key_missing",
			Message:      "ARK_API_KEY or model API key is required",
			FailureStage: providerFailureStageVideoSubmitRequest,
		}
	}

	content := make([]map[string]any, 0, len(input.Images)+len(input.ReferenceVideos)+len(input.ReferenceAudios)+1)
	content = append(content, map[string]any{
		"type": "text",
		"text": strings.TrimSpace(input.Prompt),
	})
	for _, imageURL := range input.Images {
		imageURL = strings.TrimSpace(imageURL)
		if imageURL == "" {
			continue
		}
		content = append(content, map[string]any{
			"type": "image_url",
			"image_url": map[string]any{
				"url": imageURL,
			},
			"role": "reference_image",
		})
	}
	for _, videoURL := range input.ReferenceVideos {
		videoURL = strings.TrimSpace(videoURL)
		if videoURL == "" {
			continue
		}
		content = append(content, map[string]any{
			"type": "video_url",
			"video_url": map[string]any{
				"url": videoURL,
			},
			"role": "reference_video",
		})
	}
	for _, audioURL := range input.ReferenceAudios {
		audioURL = strings.TrimSpace(audioURL)
		if audioURL == "" {
			continue
		}
		content = append(content, map[string]any{
			"type": "audio_url",
			"audio_url": map[string]any{
				"url": audioURL,
			},
			"role": "reference_audio",
		})
	}

	payload := map[string]any{
		"model":          arkSeedanceVideoRuntimeModel(input),
		"content":        content,
		"ratio":          fallbackString(strings.TrimSpace(input.AspectRatio), "16:9"),
		"duration":       arkVideoDuration(input.Duration),
		"resolution":     arkVideoResolution(input),
		"watermark":      input.Watermark,
		"generate_audio": input.GenerateAudio,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return VideoSubmitResult{}, &ProviderError{Code: "provider_request_build_failed", Message: err.Error()}
	}

	endpointURL := arkVideoTasksURL(input)
	log.Printf("ark video submit request model=%q endpoint=%q ratio=%q duration=%v resolution=%q reference_images=%d reference_videos=%d reference_audios=%d generate_audio=%v",
		payload["model"],
		endpointURL,
		payload["ratio"],
		payload["duration"],
		payload["resolution"],
		len(input.Images),
		len(input.ReferenceVideos),
		len(input.ReferenceAudios),
		input.GenerateAudio,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, bytes.NewReader(body))
	if err != nil {
		return VideoSubmitResult{}, &ProviderError{Code: "provider_request_build_failed", Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return VideoSubmitResult{}, &ProviderError{Code: "provider_request_failed", Message: err.Error(), FailureStage: providerFailureStageVideoSubmitRequest}
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(resp.Body)
	requestID := resp.Header.Get("x-request-id")
	if resp.StatusCode >= 400 {
		return VideoSubmitResult{}, providerHTTPError(resp.StatusCode, rawBody, requestID, providerFailureStageVideoSubmitRequest)
	}

	var apiResp struct {
		ID     string `json:"id"`
		TaskID string `json:"task_id"`
	}
	if providerErr := decodeProviderJSON(rawBody, &apiResp, requestID, providerFailureStageVideoSubmitRequest); providerErr != nil {
		return VideoSubmitResult{}, providerErr
	}
	taskID := fallbackString(strings.TrimSpace(apiResp.ID), strings.TrimSpace(apiResp.TaskID))
	if taskID == "" {
		return VideoSubmitResult{}, &ProviderError{Code: "provider_empty_task", Message: "provider returned no id", ProviderRequestID: requestID}
	}
	return VideoSubmitResult{TaskID: taskID, ProviderRequestID: fallbackString(requestID, taskID)}, nil
}

func (p *OpenAIProvider) pollArkSeedanceVideo(ctx context.Context, taskID string, input VideoGenerationInput) (VideoTaskResult, *ProviderError) {
	apiKey := p.arkVideoAPIKey(input)
	if apiKey == "" {
		return VideoTaskResult{}, &ProviderError{
			Code:         "provider_api_key_missing",
			Message:      "ARK_API_KEY or model API key is required",
			FailureStage: providerFailureStageVideoPollRequest,
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(arkVideoTasksURL(input), "/")+"/"+taskID, nil)
	if err != nil {
		return VideoTaskResult{}, &ProviderError{Code: "provider_request_build_failed", Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return VideoTaskResult{}, &ProviderError{Code: "provider_request_failed", Message: err.Error(), FailureStage: providerFailureStageVideoPollRequest}
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(resp.Body)
	requestID := resp.Header.Get("x-request-id")
	if resp.StatusCode >= 400 {
		return VideoTaskResult{}, providerHTTPError(resp.StatusCode, rawBody, requestID, providerFailureStageVideoPollRequest)
	}

	var apiResp struct {
		ID      string `json:"id"`
		TaskID  string `json:"task_id"`
		Status  string `json:"status"`
		Content struct {
			VideoURL string `json:"video_url"`
		} `json:"content"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}
	if providerErr := decodeProviderJSON(rawBody, &apiResp, requestID, providerFailureStageVideoPollRequest); providerErr != nil {
		return VideoTaskResult{}, providerErr
	}

	result := VideoTaskResult{
		TaskID:            fallbackString(strings.TrimSpace(apiResp.ID), fallbackString(strings.TrimSpace(apiResp.TaskID), taskID)),
		Status:            arkVideoTaskStatus(apiResp.Status),
		FailReason:        arkVideoFailReason(apiResp.Error.Code, apiResp.Error.Message, apiResp.Status),
		OutputURL:         strings.TrimSpace(apiResp.Content.VideoURL),
		MIMEType:          "video/mp4",
		ProviderRequestID: fallbackString(requestID, taskID),
		UsageTotalTokens:  apiResp.Usage.TotalTokens,
	}
	if result.Status == VideoTaskSucceeded {
		if result.OutputURL == "" {
			return VideoTaskResult{}, &ProviderError{
				Code:              "provider_empty_video",
				Message:           "provider returned no video_url",
				ProviderRequestID: result.ProviderRequestID,
				FailureStage:      providerFailureStageVideoPollRequest,
			}
		}
		base64Video, mimeType, providerErr := p.fetchBinaryURL(ctx, result.OutputURL, result.ProviderRequestID)
		if providerErr != nil {
			return VideoTaskResult{}, providerErr
		}
		result.OutputBase64 = base64Video
		result.MIMEType = fallbackString(mimeType, "video/mp4")
	}
	return result, nil
}

func (p *OpenAIProvider) submitZZVideo(ctx context.Context, input VideoGenerationInput) (VideoSubmitResult, *ProviderError) {
	apiKey := p.zzVideoAPIKey(input)
	if apiKey == "" {
		return VideoSubmitResult{}, &ProviderError{
			Code:         "provider_api_key_missing",
			Message:      "ZZ_API_KEY or model API key is required",
			FailureStage: providerFailureStageVideoSubmitRequest,
		}
	}
	payload := map[string]any{
		"model":        fallbackString(strings.TrimSpace(input.Model), zzVideoDSFastRuntimeModel),
		"prompt":       strings.TrimSpace(input.Prompt),
		"seconds":      zzVideoSeconds(input.Duration),
		"aspect_ratio": fallbackString(strings.TrimSpace(input.AspectRatio), "16:9"),
	}
	if resolution := strings.ToLower(strings.TrimSpace(input.Resolution)); resolution != "" {
		payload["resolution"] = resolution
	}
	if values := cleanStringList(input.Images); len(values) > 0 {
		payload["images"] = values
	}
	if values := cleanStringList(input.ReferenceVideos); len(values) > 0 {
		payload["videos"] = values
	}
	if values := cleanStringList(input.ReferenceAudios); len(values) > 0 {
		payload["audios"] = values
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return VideoSubmitResult{}, &ProviderError{Code: "provider_request_build_failed", Message: err.Error()}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, zzVideoTasksURL(input), bytes.NewReader(body))
	if err != nil {
		return VideoSubmitResult{}, &ProviderError{Code: "provider_request_build_failed", Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return VideoSubmitResult{}, &ProviderError{Code: "provider_request_failed", Message: err.Error(), FailureStage: providerFailureStageVideoSubmitRequest}
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(resp.Body)
	requestID := resp.Header.Get("x-request-id")
	if resp.StatusCode >= 400 {
		return VideoSubmitResult{}, providerHTTPError(resp.StatusCode, rawBody, requestID, providerFailureStageVideoSubmitRequest)
	}

	var apiResp struct {
		ID     string `json:"id"`
		TaskID string `json:"task_id"`
	}
	if providerErr := decodeProviderJSON(rawBody, &apiResp, requestID, providerFailureStageVideoSubmitRequest); providerErr != nil {
		return VideoSubmitResult{}, providerErr
	}
	taskID := fallbackString(strings.TrimSpace(apiResp.ID), strings.TrimSpace(apiResp.TaskID))
	if taskID == "" {
		return VideoSubmitResult{}, &ProviderError{Code: "provider_empty_task", Message: "provider returned no id", ProviderRequestID: requestID}
	}
	return VideoSubmitResult{TaskID: taskID, ProviderRequestID: fallbackString(requestID, taskID)}, nil
}

func (p *OpenAIProvider) pollZZVideo(ctx context.Context, taskID string, input VideoGenerationInput) (VideoTaskResult, *ProviderError) {
	apiKey := p.zzVideoAPIKey(input)
	if apiKey == "" {
		return VideoTaskResult{}, &ProviderError{
			Code:         "provider_api_key_missing",
			Message:      "ZZ_API_KEY or model API key is required",
			FailureStage: providerFailureStageVideoPollRequest,
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(zzVideoTasksURL(input), "/")+"/"+taskID, nil)
	if err != nil {
		return VideoTaskResult{}, &ProviderError{Code: "provider_request_build_failed", Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return VideoTaskResult{}, &ProviderError{Code: "provider_request_failed", Message: err.Error(), FailureStage: providerFailureStageVideoPollRequest}
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(resp.Body)
	requestID := resp.Header.Get("x-request-id")
	if resp.StatusCode >= 400 {
		return VideoTaskResult{}, providerHTTPError(resp.StatusCode, rawBody, requestID, providerFailureStageVideoPollRequest)
	}

	var apiResp struct {
		ID     string          `json:"id"`
		TaskID string          `json:"task_id"`
		Status string          `json:"status"`
		Error  json.RawMessage `json:"error"`
	}
	if providerErr := decodeProviderJSON(rawBody, &apiResp, requestID, providerFailureStageVideoPollRequest); providerErr != nil {
		return VideoTaskResult{}, providerErr
	}
	result := VideoTaskResult{
		TaskID:            fallbackString(strings.TrimSpace(apiResp.ID), fallbackString(strings.TrimSpace(apiResp.TaskID), taskID)),
		Status:            zzVideoTaskStatus(apiResp.Status),
		FailReason:        zzVideoFailReason(apiResp.Error, apiResp.Status),
		MIMEType:          "video/mp4",
		ProviderRequestID: fallbackString(requestID, taskID),
	}
	if result.Status != VideoTaskSucceeded {
		return result, nil
	}
	outputBase64, mimeType, providerErr := p.fetchZZVideoContent(ctx, taskID, input, apiKey, result.ProviderRequestID)
	if providerErr != nil {
		return VideoTaskResult{}, providerErr
	}
	result.OutputBase64 = outputBase64
	result.MIMEType = fallbackString(mimeType, "video/mp4")
	return result, nil
}

func (p *OpenAIProvider) fetchZZVideoContent(ctx context.Context, taskID string, input VideoGenerationInput, apiKey, requestID string) (string, string, *ProviderError) {
	contentURL := strings.TrimRight(zzVideoTasksURL(input), "/") + "/" + strings.TrimSpace(taskID) + "/content"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, contentURL, nil)
	if err != nil {
		return "", "", &ProviderError{Code: "provider_request_build_failed", Message: err.Error(), ProviderRequestID: requestID}
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", "", &ProviderError{Code: "provider_request_failed", Message: err.Error(), ProviderRequestID: requestID, FailureStage: providerFailureStageVideoPollRequest}
	}
	defer resp.Body.Close()
	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", "", providerHTTPError(resp.StatusCode, rawBody, requestID, providerFailureStageVideoPollRequest)
	}
	mimeType := normalizeAssetMimeType(fallbackString(resp.Header.Get("Content-Type"), "video/mp4"))
	return base64.StdEncoding.EncodeToString(rawBody), mimeType, nil
}

func (p *OpenAIProvider) submitWuyinGrokImagineVideo(ctx context.Context, input VideoGenerationInput) (VideoSubmitResult, *ProviderError) {
	imageURLs, providerErr := wuyinVideoImageURLs(input.Images)
	if providerErr != nil {
		return VideoSubmitResult{}, providerErr
	}

	payload := map[string]any{
		"prompt":       strings.TrimSpace(input.Prompt),
		"duration":     fallbackString(strings.TrimSpace(input.Duration), "10"),
		"aspect_ratio": fallbackString(strings.TrimSpace(input.AspectRatio), "16:9"),
	}
	if len(imageURLs) > 0 {
		payload["image_urls"] = imageURLs
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return VideoSubmitResult{}, &ProviderError{Code: "provider_request_build_failed", Message: err.Error()}
	}

	providerInput := wuyinVideoProviderInput(input)
	apiKey := providerAPIKey(p.apiKey, providerInput)
	reqURL := appendProviderQueryParam(providerBaseURL(p.baseURL, providerInput)+providerAPIEndpoint(providerInput, wuyinGrokImagineSubmitEndpoint), "key", apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return VideoSubmitResult{}, &ProviderError{Code: "provider_request_build_failed", Message: err.Error()}
	}
	req.Header.Set("Authorization", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return VideoSubmitResult{}, &ProviderError{Code: "provider_request_failed", Message: err.Error(), FailureStage: providerFailureStageVideoSubmitRequest}
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(resp.Body)
	requestID := resp.Header.Get("x-request-id")
	if resp.StatusCode >= 400 {
		return VideoSubmitResult{}, providerHTTPError(resp.StatusCode, rawBody, requestID, providerFailureStageVideoSubmitRequest)
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if providerErr := decodeProviderJSON(rawBody, &apiResp, requestID, providerFailureStageVideoSubmitRequest); providerErr != nil {
		return VideoSubmitResult{}, providerErr
	}
	if apiResp.Code != 0 && apiResp.Code != http.StatusOK {
		return VideoSubmitResult{}, &ProviderError{
			Code:              "provider_wuyin_error",
			Message:           fallbackString(strings.TrimSpace(apiResp.Msg), "provider returned an error"),
			ProviderRequestID: requestID,
			FailureStage:      providerFailureStageVideoSubmitRequest,
		}
	}
	taskID := strings.TrimSpace(apiResp.Data.ID)
	if taskID == "" {
		return VideoSubmitResult{}, &ProviderError{Code: "provider_empty_task", Message: "provider returned no id", ProviderRequestID: requestID}
	}
	return VideoSubmitResult{TaskID: taskID, ProviderRequestID: fallbackString(requestID, taskID)}, nil
}

func (p *OpenAIProvider) pollWuyinGrokImagineVideo(ctx context.Context, taskID string, input VideoGenerationInput) (VideoTaskResult, *ProviderError) {
	providerInput := wuyinVideoProviderInput(input)
	base := providerBaseURL(p.baseURL, providerInput)
	apiKey := providerAPIKey(p.apiKey, providerInput)
	reqURL := appendProviderQueryParam(base+wuyinGrokImagineDetailEndpoint, "id", taskID)
	reqURL = appendProviderQueryParam(reqURL, "key", apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return VideoTaskResult{}, &ProviderError{Code: "provider_request_build_failed", Message: err.Error()}
	}
	req.Header.Set("Authorization", apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return VideoTaskResult{}, &ProviderError{Code: "provider_request_failed", Message: err.Error(), FailureStage: providerFailureStageVideoPollRequest}
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(resp.Body)
	requestID := resp.Header.Get("x-request-id")
	if resp.StatusCode >= 400 {
		return VideoTaskResult{}, providerHTTPError(resp.StatusCode, rawBody, requestID, providerFailureStageVideoPollRequest)
	}

	var apiResp struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if providerErr := decodeProviderJSON(rawBody, &apiResp, requestID, providerFailureStageVideoPollRequest); providerErr != nil {
		return VideoTaskResult{}, providerErr
	}
	if apiResp.Code != 0 && apiResp.Code != http.StatusOK {
		return VideoTaskResult{}, &ProviderError{
			Code:              "provider_wuyin_error",
			Message:           fallbackString(strings.TrimSpace(apiResp.Msg), "provider returned an error"),
			ProviderRequestID: requestID,
			FailureStage:      providerFailureStageVideoPollRequest,
		}
	}

	var data any
	if len(bytes.TrimSpace(apiResp.Data)) > 0 {
		if err := json.Unmarshal(apiResp.Data, &data); err != nil {
			return VideoTaskResult{}, &ProviderError{
				Code:              "provider_decode_failed",
				Message:           "provider returned invalid data JSON",
				ProviderRequestID: requestID,
				FailureStage:      providerFailureStageVideoPollRequest,
			}
		}
	}

	statusCode, ok := wuyinVideoStatus(data)
	if !ok {
		return VideoTaskResult{}, &ProviderError{
			Code:              "provider_empty_status",
			Message:           "provider returned no status",
			ProviderRequestID: requestID,
			FailureStage:      providerFailureStageVideoPollRequest,
		}
	}
	message := fallbackString(wuyinStringField(data, "message"), strings.TrimSpace(apiResp.Msg))
	result := VideoTaskResult{
		TaskID:            taskID,
		ProviderRequestID: fallbackString(requestID, taskID),
		MIMEType:          "video/mp4",
	}
	switch statusCode {
	case 0, 1:
		result.Status = VideoTaskInProgress
		return result, nil
	case 2:
		result.Status = VideoTaskSucceeded
	case 3:
		result.Status = VideoTaskFailed
		result.FailReason = strings.TrimSpace(message)
		return result, nil
	default:
		result.Status = VideoTaskInProgress
		return result, nil
	}

	result.OutputURL = extractWuyinVideoURL(data)
	if result.OutputURL == "" {
		return VideoTaskResult{}, &ProviderError{
			Code:              "provider_empty_video",
			Message:           "provider returned no video URL",
			ProviderRequestID: result.ProviderRequestID,
			FailureStage:      providerFailureStageVideoPollRequest,
		}
	}
	base64Video, mimeType, providerErr := p.fetchBinaryURL(ctx, result.OutputURL, result.ProviderRequestID)
	if providerErr != nil {
		return VideoTaskResult{}, providerErr
	}
	result.OutputBase64 = base64Video
	result.MIMEType = fallbackString(mimeType, "video/mp4")
	return result, nil
}

func arkVideoProviderInput(input VideoGenerationInput) ImageGenerationInput {
	return ImageGenerationInput{
		ProviderBaseURL:     input.ProviderBaseURL,
		ProviderAPIKey:      input.ProviderAPIKey,
		ProviderAPIEndpoint: input.ProviderAPIEndpoint,
	}
}

func arkVideoTasksURL(input VideoGenerationInput) string {
	providerInput := arkVideoProviderInput(input)
	return providerBaseURL(arkVideoProviderBaseURL, providerInput) + providerAPIEndpoint(providerInput, arkVideoTasksEndpoint)
}

func (p *OpenAIProvider) arkVideoAPIKey(input VideoGenerationInput) string {
	if apiKey := strings.TrimSpace(input.ProviderAPIKey); apiKey != "" {
		return apiKey
	}
	return strings.TrimSpace(p.arkAPIKey)
}

func zzVideoProviderInput(input VideoGenerationInput) ImageGenerationInput {
	return ImageGenerationInput{
		ProviderBaseURL:     input.ProviderBaseURL,
		ProviderAPIKey:      input.ProviderAPIKey,
		ProviderAPIEndpoint: input.ProviderAPIEndpoint,
	}
}

func zzVideoTasksURL(input VideoGenerationInput) string {
	providerInput := zzVideoProviderInput(input)
	return providerBaseURL(zzVideoProviderBaseURL, providerInput) + providerAPIEndpoint(providerInput, zzVideoEndpoint)
}

func (p *OpenAIProvider) zzVideoAPIKey(input VideoGenerationInput) string {
	if apiKey := strings.TrimSpace(input.ProviderAPIKey); apiKey != "" {
		return apiKey
	}
	return strings.TrimSpace(p.zzAPIKey)
}

func zzVideoSeconds(value string) int {
	seconds, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || seconds <= 0 {
		return 15
	}
	return seconds
}

func canonicalVideoRuntimeModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
	}
	if isArkSeedanceMiniRuntimeAlias(model) {
		return arkSeedanceMiniRuntimeModel
	}
	if isArkSeedance2RuntimeAlias(model) {
		return arkSeedance2RuntimeModel
	}
	if isZZVideoRuntimeAlias(model) {
		return zzVideoDSFastRuntimeModel
	}
	return model
}

func isArkSeedanceMiniRuntimeAlias(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case strings.ToLower(arkSeedanceMiniRuntimeModel), "doubao-seed-2-0-mini":
		return true
	default:
		return false
	}
}

func isArkSeedance2RuntimeAlias(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case strings.ToLower(arkSeedance2RuntimeModel), "doubao-seedance-2-0":
		return true
	default:
		return false
	}
}

func arkSeedanceVideoRuntimeModel(input VideoGenerationInput) string {
	if model := canonicalVideoRuntimeModel(input.Model); model != "" && isArkSeedanceVideoInput(VideoGenerationInput{Model: model}) {
		return model
	}
	return arkSeedanceMiniRuntimeModel
}

func arkVideoDuration(value string) int {
	trimmed := strings.TrimSpace(value)
	if trimmed == "-1" {
		return -1
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil || parsed < 4 || parsed > 15 {
		return 10
	}
	return parsed
}

func arkVideoResolution(input VideoGenerationInput) string {
	resolution := strings.ToLower(strings.TrimSpace(input.Resolution))
	switch canonicalVideoRuntimeModel(input.Model) {
	case arkSeedance2RuntimeModel:
		switch resolution {
		case "1080p":
			return "1080p"
		default:
			return "720p"
		}
	default:
		switch resolution {
		case "720p":
			return "720p"
		case "480p":
			return "480p"
		}
		if input.HD {
			return "720p"
		}
		return "480p"
	}
}

func arkVideoTaskStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "succeeded", "success":
		return VideoTaskSucceeded
	case "failed", "expired", "cancelled", "canceled":
		return VideoTaskFailed
	case "queued", "running", "in_progress", "processing":
		return VideoTaskInProgress
	default:
		return VideoTaskInProgress
	}
}

func arkVideoFailReason(code, message, status string) string {
	if arkVideoTaskStatus(status) != VideoTaskFailed {
		return ""
	}
	code = strings.TrimSpace(code)
	message = strings.TrimSpace(message)
	if code != "" && message != "" {
		return code + ": " + message
	}
	if message != "" {
		return message
	}
	if code != "" {
		return code
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "expired":
		return "task expired"
	case "cancelled", "canceled":
		return "task cancelled"
	default:
		return "provider video failed"
	}
}

func isArkSeedanceVideoInput(input VideoGenerationInput) bool {
	if isArkSeedanceMiniRuntimeAlias(input.Model) {
		return true
	}
	model := strings.ToLower(strings.TrimSpace(input.Model))
	if strings.Contains(model, "doubao-seed") || strings.Contains(model, "seedance") {
		return true
	}
	endpoint := strings.ToLower(strings.TrimSpace(input.ProviderAPIEndpoint))
	if strings.Contains(endpoint, "contents/generations/tasks") {
		return true
	}
	baseURL := strings.ToLower(strings.TrimSpace(input.ProviderBaseURL))
	return strings.Contains(baseURL, "ark.cn-beijing.volces.com")
}

func isArkSeedanceVideoModel(model string, config *ModelConfig) bool {
	if isArkSeedanceVideoInput(VideoGenerationInput{Model: model}) {
		return true
	}
	if config == nil {
		return false
	}
	return isArkSeedanceVideoInput(VideoGenerationInput{
		Model:               config.RuntimeModel,
		ProviderBaseURL:     config.APIBaseURL,
		ProviderAPIEndpoint: config.APIEndpoint,
	})
}

func isZZVideoRuntimeAlias(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case strings.ToLower(zzVideoDSFastRuntimeModel), "video-ds-2.0", "video ds 2.0", "ds 2.0", "ds-2.0", "zz api video ds 2.0 fast":
		return true
	default:
		return false
	}
}

func isZZVideoInput(input VideoGenerationInput) bool {
	if isZZVideoRuntimeAlias(input.Model) {
		return true
	}
	endpoint := strings.ToLower(strings.TrimSpace(input.ProviderAPIEndpoint))
	if strings.Trim(endpoint, "/") == strings.Trim(zzVideoEndpoint, "/") {
		return true
	}
	baseURL := strings.ToLower(strings.TrimSpace(input.ProviderBaseURL))
	return strings.Contains(baseURL, "zz1cc.cc.cd")
}

func isZZVideoModel(model string, config *ModelConfig) bool {
	if isZZVideoInput(VideoGenerationInput{Model: model}) {
		return true
	}
	if config == nil {
		return false
	}
	return isZZVideoInput(VideoGenerationInput{
		Model:               config.RuntimeModel,
		ProviderBaseURL:     config.APIBaseURL,
		ProviderAPIEndpoint: config.APIEndpoint,
	})
}

func zzVideoTaskStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "succeeded", "success":
		return VideoTaskSucceeded
	case "failed", "error", "cancelled", "canceled":
		return VideoTaskFailed
	case "queued", "running", "in_progress", "processing", "pending", "":
		return VideoTaskInProgress
	default:
		return VideoTaskInProgress
	}
}

func zzVideoFailReason(raw json.RawMessage, status string) string {
	if zzVideoTaskStatus(status) != VideoTaskFailed {
		return ""
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "provider video failed"
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return fallbackString(strings.TrimSpace(text), "provider video failed")
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err == nil {
		for _, key := range []string{"message", "msg", "error", "reason", "code"} {
			if value, ok := object[key]; ok {
				message := strings.TrimSpace(fmt.Sprint(value))
				if message != "" {
					return message
				}
			}
		}
	}
	return string(raw)
}

func cleanStringList(values []string) []string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			cleaned = append(cleaned, text)
		}
	}
	return cleaned
}

func wuyinVideoProviderInput(input VideoGenerationInput) ImageGenerationInput {
	return ImageGenerationInput{
		ProviderBaseURL:     input.ProviderBaseURL,
		ProviderAPIKey:      input.ProviderAPIKey,
		ProviderAPIEndpoint: input.ProviderAPIEndpoint,
	}
}

func appendProviderQueryParam(rawURL, key, value string) string {
	if strings.TrimSpace(value) == "" {
		return rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		separator := "?"
		if strings.Contains(rawURL, "?") {
			separator = "&"
		}
		return rawURL + separator + url.QueryEscape(key) + "=" + url.QueryEscape(value)
	}
	query := parsed.Query()
	query.Set(key, value)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func isWuyinGrokImagineVideoInput(input VideoGenerationInput) bool {
	if strings.EqualFold(strings.TrimSpace(input.Model), wuyinGrokImagineRuntimeModel) {
		return true
	}
	endpoint := strings.ToLower(strings.TrimSpace(input.ProviderAPIEndpoint))
	if strings.Contains(endpoint, "video_grok_imagine") {
		return true
	}
	baseURL := strings.ToLower(strings.TrimSpace(input.ProviderBaseURL))
	return strings.Contains(baseURL, "api.wuyinkeji.com")
}

func isWuyinGrokImagineModel(model string, config *ModelConfig) bool {
	if strings.EqualFold(strings.TrimSpace(model), wuyinGrokImagineRuntimeModel) {
		return true
	}
	if config == nil {
		return false
	}
	return isWuyinGrokImagineVideoInput(VideoGenerationInput{
		Model:               config.RuntimeModel,
		ProviderBaseURL:     config.APIBaseURL,
		ProviderAPIEndpoint: config.APIEndpoint,
	})
}

func wuyinVideoImageURLs(images []string) ([]string, *ProviderError) {
	urls := make([]string, 0, len(images))
	for _, image := range images {
		image = strings.TrimSpace(image)
		if image == "" {
			continue
		}
		parsed, err := url.Parse(image)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return nil, &ProviderError{
				Code:         "provider_reference_url_required",
				Message:      "Wuyin video reference images require public http(s) URLs",
				FailureStage: providerFailureStageVideoSubmitRequest,
			}
		}
		urls = append(urls, image)
	}
	return urls, nil
}

func wuyinVideoStatus(data any) (int, bool) {
	value, ok := wuyinField(data, "status")
	if !ok {
		return 0, false
	}
	return intFromProviderValue(value)
}

func wuyinStringField(data any, field string) string {
	value, ok := wuyinField(data, field)
	if !ok {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func wuyinField(data any, field string) (any, bool) {
	object, ok := data.(map[string]any)
	if !ok {
		return nil, false
	}
	for key, value := range object {
		if strings.EqualFold(strings.TrimSpace(key), field) {
			return value, true
		}
	}
	return nil, false
}

func intFromProviderValue(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		if parsed, err := typed.Int64(); err == nil {
			return int(parsed), true
		}
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func extractWuyinVideoURL(data any) string {
	urls := make([]string, 0, 2)
	collectWuyinVideoURLs(data, &urls)
	for _, candidate := range urls {
		if isLikelyVideoURL(candidate) {
			return candidate
		}
	}
	if len(urls) > 0 {
		return urls[0]
	}
	return ""
}

func collectWuyinVideoURLs(value any, urls *[]string) {
	switch typed := value.(type) {
	case string:
		if httpURL := extractHTTPURLFromText(typed); httpURL != "" {
			*urls = append(*urls, httpURL)
		}
	case []any:
		for _, item := range typed {
			collectWuyinVideoURLs(item, urls)
		}
	case map[string]any:
		seen := map[string]struct{}{}
		for _, key := range []string{"video_url", "video", "url", "output", "result", "file_url", "download_url", "data", "content"} {
			if nested, ok := typed[key]; ok {
				seen[key] = struct{}{}
				collectWuyinVideoURLs(nested, urls)
			}
		}
		for key, nested := range typed {
			if _, ok := seen[key]; ok {
				continue
			}
			collectWuyinVideoURLs(nested, urls)
		}
	}
}

func isLikelyVideoURL(candidate string) bool {
	withoutQuery := candidate
	if parsed, err := url.Parse(candidate); err == nil {
		withoutQuery = parsed.Path
	}
	withoutQuery = strings.ToLower(strings.TrimSpace(withoutQuery))
	for _, suffix := range []string{".mp4", ".mov", ".m4v", ".webm"} {
		if strings.HasSuffix(withoutQuery, suffix) {
			return true
		}
	}
	return false
}

func (p *OpenAIProvider) GenerateMusic(ctx context.Context, input MusicGenerationInput) (MusicGenerationResult, *ProviderError) {
	payload := map[string]any{
		"model":           strings.TrimSpace(input.Model),
		"video_mime_type": fallbackString(strings.TrimSpace(input.VideoMIMEType), "video/mp4"),
		"prompt":          strings.TrimSpace(input.Prompt),
		"duration":        strings.TrimSpace(input.Duration),
		"aspect_ratio":    strings.TrimSpace(input.AspectRatio),
		"variation":       fallbackString(strings.TrimSpace(input.Variation), "smart"),
	}
	if videoURL := strings.TrimSpace(input.VideoURL); videoURL != "" {
		payload["video_url"] = videoURL
	} else if videoBase64 := strings.TrimSpace(input.VideoBase64); videoBase64 != "" {
		payload["video_base64"] = videoBase64
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return MusicGenerationResult{}, &ProviderError{Code: "provider_request_build_failed", Message: err.Error()}
	}

	providerInput := ImageGenerationInput{
		ProviderBaseURL:     input.ProviderBaseURL,
		ProviderAPIKey:      input.ProviderAPIKey,
		ProviderAPIEndpoint: input.ProviderAPIEndpoint,
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, providerBaseURL(p.baseURL, providerInput)+providerAPIEndpoint(providerInput, "/v1/audio/soundtracks"), bytes.NewReader(body))
	if err != nil {
		return MusicGenerationResult{}, &ProviderError{Code: "provider_request_build_failed", Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+providerAPIKey(p.apiKey, providerInput))
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return MusicGenerationResult{}, &ProviderError{Code: "provider_request_failed", Message: err.Error(), FailureStage: "music_generation_request"}
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(resp.Body)
	requestID := resp.Header.Get("x-request-id")
	if resp.StatusCode >= 400 {
		return MusicGenerationResult{}, providerHTTPError(resp.StatusCode, rawBody, requestID, "music_generation_request")
	}

	var apiResp MusicGenerationResult
	if providerErr := decodeProviderJSON(rawBody, &apiResp, requestID, "music_generation_request"); providerErr != nil {
		return MusicGenerationResult{}, providerErr
	}
	apiResp.AudioURL = strings.TrimSpace(apiResp.AudioURL)
	apiResp.AudioBase64 = strings.TrimSpace(apiResp.AudioBase64)
	apiResp.MIMEType = normalizeAssetMimeType(fallbackString(apiResp.MIMEType, "audio/mpeg"))
	apiResp.ProviderRequestID = fallbackString(strings.TrimSpace(apiResp.ProviderRequestID), requestID)
	if apiResp.AudioBase64 == "" && apiResp.AudioURL != "" {
		audioBase64, mimeType, providerErr := p.fetchBinaryURL(ctx, apiResp.AudioURL, requestID)
		if providerErr != nil {
			return MusicGenerationResult{}, providerErr
		}
		apiResp.AudioBase64 = audioBase64
		apiResp.MIMEType = normalizeAssetMimeType(fallbackString(mimeType, apiResp.MIMEType))
	}
	if apiResp.AudioBase64 == "" {
		return MusicGenerationResult{}, &ProviderError{Code: "provider_empty_audio", Message: "provider returned no audio data", ProviderRequestID: requestID, FailureStage: "music_generation_request"}
	}
	return apiResp, nil
}

func NewOpenAIProvider(cfg Config) *OpenAIProvider {
	baseURL := strings.TrimRight(cfg.OpenAIBaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	return &OpenAIProvider{
		baseURL:       baseURL,
		apiKey:        cfg.OpenAIAPIKey,
		arkAPIKey:     cfg.ArkAPIKey,
		zzAPIKey:      cfg.ZZAPIKey,
		spoolPath:     cfg.GenerationSpoolPath,
		spoolMaxBytes: cfg.GenerationSpoolMaxBytes,
		client: &http.Client{
			Transport: &http.Transport{
				Proxy:             http.ProxyFromEnvironment,
				DisableKeepAlives: true,
				ForceAttemptHTTP2: false,
			},
		},
	}
}

func (p *OpenAIProvider) Generate(ctx context.Context, input ImageGenerationInput) (ImageGenerationResult, *ProviderError) {
	if useChatCompletionsImageGeneration(input) {
		return p.generateWithChatCompletionsAPI(ctx, input)
	}
	if !useResponsesImageGeneration(input) {
		return p.generateWithImagesAPI(ctx, input)
	}
	return p.generateWithResponsesAPI(ctx, input)
}

func (p *OpenAIProvider) generateWithChatCompletionsAPI(ctx context.Context, input ImageGenerationInput) (ImageGenerationResult, *ProviderError) {
	payload := map[string]any{
		"model":    fallbackString(strings.TrimSpace(input.Model), "gpt-image-2"),
		"stream":   false,
		"messages": []map[string]any{{"role": "user", "content": chatImageMessageContent(input)}},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return ImageGenerationResult{}, &ProviderError{Code: "provider_request_build_failed", Message: err.Error()}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, providerBaseURL(p.baseURL, input)+providerChatCompletionsEndpoint(input), bytes.NewReader(body))
	if err != nil {
		return ImageGenerationResult{}, &ProviderError{Code: "provider_request_build_failed", Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+providerAPIKey(p.apiKey, input))
	req.Header.Set("Content-Type", "application/json")
	applyImageIdempotencyHeader(req, input)

	resp, err := p.client.Do(req)
	if err != nil {
		return ImageGenerationResult{}, providerRequestError(err, providerFailureStageImageGenerationRequest)
	}
	defer resp.Body.Close()

	requestID := resp.Header.Get("x-request-id")
	if resp.StatusCode >= 400 {
		rawBody, responseErr := readLimitedImageProviderResponse(resp)
		if responseErr != nil {
			responseErr.ProviderRequestID = requestID
			responseErr.FailureStage = providerFailureStageImageGenerationRequest
			return ImageGenerationResult{}, responseErr
		}
		return ImageGenerationResult{}, providerHTTPErrorWithResponse(resp, rawBody, requestID, providerFailureStageImageGenerationRequest)
	}

	var apiResp struct {
		Choices []struct {
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if providerErr := decodeLimitedImageProviderSuccess(resp, &apiResp, requestID, providerFailureStageImageGenerationRequest); providerErr != nil {
		return ImageGenerationResult{}, providerErr
	}

	for _, choice := range apiResp.Choices {
		candidate, ok := extractChatImageCandidate(choice.Message.Content)
		if !ok {
			continue
		}
		if strings.TrimSpace(candidate.base64Image) != "" {
			return p.spoolImageGenerationResult(candidate.base64Image, fallbackString(candidate.mimeType, "image/png"), requestID)
		}
		if strings.TrimSpace(candidate.imageURL) != "" {
			base64Image, mimeType, providerErr := p.fetchImageURL(ctx, candidate.imageURL, requestID)
			if providerErr != nil {
				return ImageGenerationResult{}, providerErr
			}
			return p.spoolImageGenerationResult(base64Image, mimeType, requestID)
		}
	}

	return ImageGenerationResult{}, &ProviderError{
		Code:              "provider_empty_image",
		Message:           "provider returned no image data",
		ProviderRequestID: requestID,
		FailureStage:      providerFailureStageImageGenerationRequest,
	}
}

func (p *OpenAIProvider) generateWithImagesAPI(ctx context.Context, input ImageGenerationInput) (ImageGenerationResult, *ProviderError) {
	prompt := composeImagePrompt(input)
	images := orderedReferenceImageInputs(input)
	if len(images) > 0 {
		return p.generateWithImagesEditsAPI(ctx, input, prompt, images)
	}

	referenceImages, referenceTransportMode, err := imageGenerationReferencePayload(input)
	if err != nil {
		return ImageGenerationResult{}, &ProviderError{Code: "provider_request_build_failed", Message: err.Error()}
	}
	if referenceTransportMode == imageReferenceTransportModeImagesReferenceSheet {
		prompt = appendImageReferenceSheetPromptInstruction(prompt, input)
	}

	payload := map[string]any{
		"model":           strings.TrimSpace(input.Model),
		"prompt":          prompt,
		"size":            input.Size,
		"quality":         providerImageQuality(input.Quality),
		"response_format": "b64_json",
	}
	if payload["model"] == "" {
		payload["model"] = "gpt-image-2"
	}
	if strings.TrimSpace(input.AspectRatio) != "" {
		payload["aspect_ratio"] = strings.TrimSpace(input.AspectRatio)
	}
	if len(referenceImages) > 0 {
		payload["image"] = referenceImages
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return ImageGenerationResult{}, &ProviderError{Code: "provider_request_build_failed", Message: err.Error()}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, providerBaseURL(p.baseURL, input)+providerAPIEndpoint(input, "/v1/images/generations"), bytes.NewReader(body))
	if err != nil {
		return ImageGenerationResult{}, &ProviderError{Code: "provider_request_build_failed", Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+providerAPIKey(p.apiKey, input))
	req.Header.Set("Content-Type", "application/json")
	applyImageIdempotencyHeader(req, input)

	resp, err := p.client.Do(req)
	if err != nil {
		return ImageGenerationResult{}, providerRequestError(err, providerFailureStageImageGenerationRequest)
	}
	defer resp.Body.Close()

	requestID := resp.Header.Get("x-request-id")
	if resp.StatusCode >= 400 {
		rawBody, responseErr := readLimitedImageProviderResponse(resp)
		if responseErr != nil {
			responseErr.ProviderRequestID = requestID
			responseErr.FailureStage = providerFailureStageImageGenerationRequest
			return ImageGenerationResult{}, responseErr
		}
		return ImageGenerationResult{}, providerHTTPErrorWithResponse(resp, rawBody, requestID, providerFailureStageImageGenerationRequest)
	}

	var apiResp struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
			URL     string `json:"url"`
		} `json:"data"`
	}
	if providerErr := decodeLimitedImageProviderSuccess(resp, &apiResp, requestID, providerFailureStageImageGenerationRequest); providerErr != nil {
		return ImageGenerationResult{}, providerErr
	}
	for _, item := range apiResp.Data {
		if strings.TrimSpace(item.B64JSON) != "" {
			base64Image, mimeType := normalizeProviderBase64Image(item.B64JSON, "image/png")
			return p.spoolImageGenerationResult(base64Image, mimeType, requestID)
		}
		if strings.TrimSpace(item.URL) != "" {
			base64Image, mimeType, providerErr := p.fetchImageURL(ctx, item.URL, requestID)
			if providerErr != nil {
				return ImageGenerationResult{}, providerErr
			}
			return p.spoolImageGenerationResult(base64Image, mimeType, requestID)
		}
	}
	return ImageGenerationResult{}, &ProviderError{
		Code:              "provider_empty_image",
		Message:           "provider returned no image data",
		ProviderRequestID: requestID,
		FailureStage:      providerFailureStageImageGenerationRequest,
	}
}

func (p *OpenAIProvider) generateWithImagesEditsAPI(ctx context.Context, input ImageGenerationInput, prompt string, images []ReferenceImageInput) (ImageGenerationResult, *ProviderError) {
	body, contentType, cleanup, err := buildImagesEditsMultipartTempFile(p.spoolPath, input, prompt, images)
	if err != nil {
		return ImageGenerationResult{}, &ProviderError{Code: "provider_request_build_failed", Message: err.Error()}
	}
	defer cleanup()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, providerBaseURL(p.baseURL, input)+providerImagesEditsEndpoint(input), body)
	if err != nil {
		return ImageGenerationResult{}, &ProviderError{Code: "provider_request_build_failed", Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+providerAPIKey(p.apiKey, input))
	req.Header.Set("Content-Type", contentType)
	applyImageIdempotencyHeader(req, input)

	resp, err := p.client.Do(req)
	if err != nil {
		return ImageGenerationResult{}, providerRequestError(err, providerFailureStageImageGenerationRequest)
	}
	defer resp.Body.Close()

	requestID := resp.Header.Get("x-request-id")
	if resp.StatusCode >= 400 {
		rawBody, responseErr := readLimitedImageProviderResponse(resp)
		if responseErr != nil {
			responseErr.ProviderRequestID = requestID
			responseErr.FailureStage = providerFailureStageImageGenerationRequest
			return ImageGenerationResult{}, responseErr
		}
		return ImageGenerationResult{}, providerHTTPErrorWithResponse(resp, rawBody, requestID, providerFailureStageImageGenerationRequest)
	}

	var apiResp struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
			URL     string `json:"url"`
		} `json:"data"`
	}
	if providerErr := decodeLimitedImageProviderSuccess(resp, &apiResp, requestID, providerFailureStageImageGenerationRequest); providerErr != nil {
		return ImageGenerationResult{}, providerErr
	}
	for _, item := range apiResp.Data {
		if strings.TrimSpace(item.B64JSON) != "" {
			base64Image, mimeType := normalizeProviderBase64Image(item.B64JSON, "image/png")
			return p.spoolImageGenerationResult(base64Image, mimeType, requestID)
		}
		if strings.TrimSpace(item.URL) != "" {
			base64Image, mimeType, providerErr := p.fetchImageURL(ctx, item.URL, requestID)
			if providerErr != nil {
				return ImageGenerationResult{}, providerErr
			}
			return p.spoolImageGenerationResult(base64Image, mimeType, requestID)
		}
	}
	return ImageGenerationResult{}, &ProviderError{
		Code:              "provider_empty_image",
		Message:           "provider returned no image data",
		ProviderRequestID: requestID,
		FailureStage:      providerFailureStageImageGenerationRequest,
	}
}

func (p *OpenAIProvider) generateWithResponsesAPI(ctx context.Context, input ImageGenerationInput) (ImageGenerationResult, *ProviderError) {
	content := make([]map[string]any, 0, 1+len(input.ReferenceImages))
	content = append(content, map[string]any{
		"type": "input_text",
		"text": composeImagePrompt(input),
	})
	if input.SourceImage != nil {
		content = append(content, map[string]any{
			"type":      "input_image",
			"image_url": responsesReferenceImageURL(*input.SourceImage),
		})
	}
	for _, image := range input.ReferenceImages {
		content = append(content, map[string]any{
			"type":      "input_image",
			"image_url": responsesReferenceImageURL(image),
		})
	}

	payload := map[string]any{
		"model": fallbackString(strings.TrimSpace(input.Model), "gpt-image-2"),
		"input": []map[string]any{
			{
				"role":    "user",
				"content": content,
			},
		},
		"tool_choice": map[string]any{
			"type": "image_generation",
		},
		"tools": []map[string]any{
			{
				"type":          "image_generation",
				"size":          input.Size,
				"quality":       providerImageQuality(input.Quality),
				"output_format": "png",
				"action":        providerImageAction(input.ToolMode),
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return ImageGenerationResult{}, &ProviderError{Code: "provider_request_build_failed", Message: err.Error()}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, providerBaseURL(p.baseURL, input)+providerResponsesEndpoint(input), bytes.NewReader(body))
	if err != nil {
		return ImageGenerationResult{}, &ProviderError{Code: "provider_request_build_failed", Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+providerAPIKey(p.apiKey, input))
	req.Header.Set("Content-Type", "application/json")
	applyImageIdempotencyHeader(req, input)

	resp, err := p.client.Do(req)
	if err != nil {
		return ImageGenerationResult{}, providerRequestError(err, providerFailureStageResponsesGenerationRequest)
	}
	defer resp.Body.Close()

	requestID := resp.Header.Get("x-request-id")
	if resp.StatusCode >= 400 {
		rawBody, responseErr := readLimitedImageProviderResponse(resp)
		if responseErr != nil {
			responseErr.ProviderRequestID = requestID
			responseErr.FailureStage = providerFailureStageResponsesGenerationRequest
			return ImageGenerationResult{}, responseErr
		}
		return ImageGenerationResult{}, providerHTTPErrorWithResponse(resp, rawBody, requestID, providerFailureStageResponsesGenerationRequest)
	}

	var apiResp struct {
		Output []struct {
			Type   string `json:"type"`
			Result string `json:"result"`
		} `json:"output"`
	}
	if providerErr := decodeLimitedImageProviderSuccess(resp, &apiResp, requestID, providerFailureStageResponsesGenerationRequest); providerErr != nil {
		return ImageGenerationResult{}, providerErr
	}
	for _, item := range apiResp.Output {
		if item.Type == "image_generation_call" && strings.TrimSpace(item.Result) != "" {
			base64Image, mimeType := normalizeProviderBase64Image(item.Result, "image/png")
			return p.spoolImageGenerationResult(base64Image, mimeType, requestID)
		}
	}

	return ImageGenerationResult{}, &ProviderError{
		Code:              "provider_empty_image",
		Message:           "provider returned no image data",
		ProviderRequestID: requestID,
		FailureStage:      providerFailureStageResponsesGenerationRequest,
	}
}

func useResponsesImageGeneration(input ImageGenerationInput) bool {
	endpoint := strings.ToLower(strings.TrimSpace(input.ProviderAPIEndpoint))
	if endpoint != "" {
		if endpoint == "responses" || strings.Contains(endpoint, "/responses") {
			return true
		}
		return false
	}
	return false
}

func useChatCompletionsImageGeneration(input ImageGenerationInput) bool {
	endpoint := strings.ToLower(strings.TrimSpace(input.ProviderAPIEndpoint))
	return endpoint == "chat" || endpoint == "chat/completions" || strings.Contains(endpoint, "/chat/completions")
}

func imageGenerationReferencePayload(input ImageGenerationInput) ([]string, string, error) {
	orderedImages := orderedReferenceImageInputs(input)
	if len(orderedImages) == 0 {
		return nil, imageReferenceTransportModeNone, nil
	}
	if len(orderedImages) > 1 {
		sheetURL, err := referenceImageSheetDataURL(orderedImages, isComposeReferenceIntent(input))
		if err != nil {
			return nil, imageReferenceTransportModeImagesReferenceSheet, fmt.Errorf("build reference image sheet: %w", err)
		}
		return []string{sheetURL}, imageReferenceTransportModeImagesReferenceSheet, nil
	}

	images := make([]string, 0, len(orderedImages))
	for _, image := range orderedImages {
		if imageURL := referenceImageProviderURL(image); imageURL != "" {
			images = append(images, imageURL)
		}
	}
	if len(images) == 0 {
		return nil, imageReferenceTransportModeNone, nil
	}
	return images, imageReferenceTransportModeImagesDirect, nil
}

func orderedReferenceImageInputs(input ImageGenerationInput) []ReferenceImageInput {
	images := make([]ReferenceImageInput, 0, 2+len(input.ReferenceImages))
	if input.SourceImage != nil {
		images = append(images, *input.SourceImage)
	}
	images = append(images, input.ReferenceImages...)
	return images
}

func buildImagesEditsMultipartBody(input ImageGenerationInput, prompt string, images []ReferenceImageInput) ([]byte, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	fields := map[string]string{
		"model":           fallbackString(strings.TrimSpace(input.Model), "gpt-image-2"),
		"prompt":          strings.TrimSpace(prompt),
		"size":            strings.TrimSpace(input.Size),
		"quality":         providerImageQuality(input.Quality),
		"response_format": "b64_json",
	}
	for _, key := range []string{"model", "prompt", "size", "quality", "response_format"} {
		if value := strings.TrimSpace(fields[key]); value != "" {
			if err := writer.WriteField(key, value); err != nil {
				return nil, "", err
			}
		}
	}

	imageFieldName := "image"
	if len(images) > 1 {
		imageFieldName = "image[]"
	}
	for index, image := range images {
		if err := writeReferenceImageMultipartFile(writer, imageFieldName, image, index); err != nil {
			return nil, "", err
		}
	}
	if input.MaskImage != nil {
		if err := writeReferenceImageMultipartFile(writer, "mask", *input.MaskImage, 0); err != nil {
			return nil, "", err
		}
	}

	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return body.Bytes(), writer.FormDataContentType(), nil
}

func buildImagesEditsMultipartTempFile(spoolPath string, input ImageGenerationInput, prompt string, images []ReferenceImageInput) (*os.File, string, func(), error) {
	if strings.TrimSpace(spoolPath) == "" {
		spoolPath = os.TempDir()
	}
	if err := os.MkdirAll(spoolPath, 0o750); err != nil {
		return nil, "", func() {}, err
	}
	file, err := os.CreateTemp(spoolPath, "image-request-*.multipart.tmp")
	if err != nil {
		return nil, "", func() {}, err
	}
	cleanup := func() {
		name := file.Name()
		_ = file.Close()
		_ = os.Remove(name)
	}
	writer := multipart.NewWriter(file)
	fields := map[string]string{
		"model": fallbackString(strings.TrimSpace(input.Model), "gpt-image-2"), "prompt": strings.TrimSpace(prompt),
		"size": strings.TrimSpace(input.Size), "quality": providerImageQuality(input.Quality), "response_format": "b64_json",
	}
	for _, key := range []string{"model", "prompt", "size", "quality", "response_format"} {
		if value := strings.TrimSpace(fields[key]); value != "" {
			if err := writer.WriteField(key, value); err != nil {
				cleanup()
				return nil, "", func() {}, err
			}
		}
	}
	fieldName := "image"
	if len(images) > 1 {
		fieldName = "image[]"
	}
	for index, image := range images {
		if err := writeReferenceImageMultipartFile(writer, fieldName, image, index); err != nil {
			cleanup()
			return nil, "", func() {}, err
		}
	}
	if input.MaskImage != nil {
		if err := writeReferenceImageMultipartFile(writer, "mask", *input.MaskImage, 0); err != nil {
			cleanup()
			return nil, "", func() {}, err
		}
	}
	if err := writer.Close(); err != nil {
		cleanup()
		return nil, "", func() {}, err
	}
	if info, err := file.Stat(); err != nil || info.Size() > 64<<20 {
		cleanup()
		if err != nil {
			return nil, "", func() {}, err
		}
		return nil, "", func() {}, errGenerationPayloadTooLarge
	}
	contentType := writer.FormDataContentType()
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, "", func() {}, err
	}
	return file, contentType, cleanup, nil
}

func writeReferenceImageMultipartFile(writer *multipart.Writer, fieldName string, image ReferenceImageInput, index int) error {
	mimeType := normalizeImageMimeType(image.MIMEType)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="reference-%d%s"`, fieldName, index+1, imageFileExtension(mimeType)))
	header.Set("Content-Type", mimeType)
	part, err := writer.CreatePart(header)
	if err != nil {
		return err
	}
	if filePath := strings.TrimSpace(image.FilePath); filePath != "" {
		file, err := os.Open(filePath)
		if err != nil {
			return fmt.Errorf("prepare reference image %d: %w", index+1, err)
		}
		defer file.Close()
		_, err = io.Copy(part, io.LimitReader(file, (64<<20)+1))
		return err
	}
	rawImage, err := referenceImageInlineBytes(image)
	if err != nil {
		return fmt.Errorf("prepare reference image %d: %w", index+1, err)
	}
	_, err = part.Write(rawImage)
	return err
}

func imageFileExtension(mimeType string) string {
	if extensions, err := mime.ExtensionsByType(normalizeImageMimeType(mimeType)); err == nil && len(extensions) > 0 {
		return extensions[0]
	}
	switch normalizeImageMimeType(mimeType) {
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".png"
	}
}

func referenceImageProviderURL(image ReferenceImageInput) string {
	if inputURL := strings.TrimSpace(image.InputURL); inputURL != "" {
		return inputURL
	}
	base64Data := strings.TrimSpace(image.Base64Data)
	if base64Data == "" {
		if filePath := strings.TrimSpace(image.FilePath); filePath != "" {
			rawImage, err := os.ReadFile(filePath)
			if err != nil {
				return ""
			}
			base64Data = base64.StdEncoding.EncodeToString(rawImage)
		} else {
			return ""
		}
	}
	return fmt.Sprintf("data:%s;base64,%s", normalizeImageMimeType(image.MIMEType), base64Data)
}

func responsesReferenceImageURL(image ReferenceImageInput) string {
	if inputURL := strings.TrimSpace(image.InputURL); inputURL != "" {
		return inputURL
	}
	if base64Data := strings.TrimSpace(image.Base64Data); base64Data != "" {
		return fmt.Sprintf("data:%s;base64,%s", normalizeImageMimeType(image.MIMEType), base64Data)
	}
	return referenceImageProviderURL(image)
}

func appendImageReferenceSheetPromptInstruction(prompt string, input ImageGenerationInput) string {
	prompt = strings.TrimSpace(prompt)
	instruction := imageReferenceSheetPromptInstruction
	if isComposeReferenceIntent(input) {
		instruction = imageComposeReferenceSheetPromptInstruction
	}
	if strings.Contains(prompt, instruction) {
		return prompt
	}
	if prompt == "" {
		return instruction
	}
	return prompt + "\n" + instruction
}

func referenceImageSheetDataURL(images []ReferenceImageInput, labelReferences bool) (string, error) {
	if len(images) == 0 {
		return "", errors.New("no reference images")
	}

	decodedImages := make([]image.Image, 0, len(images))
	for index, referenceImage := range images {
		decodedImage, err := decodeReferenceImageInput(referenceImage)
		if err != nil {
			return "", fmt.Errorf("decode reference image %d: %w", index+1, err)
		}
		decodedImages = append(decodedImages, decodedImage)
	}

	sheet := drawReferenceImageSheet(decodedImages, labelReferences)
	var buf bytes.Buffer
	if err := png.Encode(&buf, sheet); err != nil {
		return "", fmt.Errorf("encode PNG reference sheet: %w", err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func decodeReferenceImageInput(referenceImage ReferenceImageInput) (image.Image, error) {
	rawImage, err := referenceImageInlineBytes(referenceImage)
	if err != nil {
		return nil, err
	}
	decodedImage, _, err := image.Decode(bytes.NewReader(rawImage))
	if err != nil {
		return nil, err
	}
	return decodedImage, nil
}

func referenceImageInlineBytes(referenceImage ReferenceImageInput) ([]byte, error) {
	if filePath := strings.TrimSpace(referenceImage.FilePath); filePath != "" {
		return os.ReadFile(filePath)
	}
	if base64Data := strings.TrimSpace(referenceImage.Base64Data); base64Data != "" {
		imageData, _ := normalizeProviderBase64Image(base64Data, normalizeImageMimeType(referenceImage.MIMEType))
		rawImage, err := base64.StdEncoding.DecodeString(compactBase64Data(imageData))
		if err != nil {
			return nil, err
		}
		return rawImage, nil
	}

	inputURL := strings.TrimSpace(referenceImage.InputURL)
	if !isDataImageURL(inputURL) {
		return nil, errors.New("reference image inline data is required")
	}
	_, imageData, ok := strings.Cut(inputURL, ",")
	if !ok {
		return nil, errors.New("invalid reference image data URL")
	}
	rawImage, err := base64.StdEncoding.DecodeString(compactBase64Data(imageData))
	if err != nil {
		return nil, err
	}
	return rawImage, nil
}

func compactBase64Data(value string) string {
	replacer := strings.NewReplacer("\n", "", "\r", "", "\t", "", " ", "")
	return replacer.Replace(strings.TrimSpace(value))
}

func drawReferenceImageSheet(images []image.Image, labelReferences bool) *image.RGBA {
	const (
		tileSize = 512
		padding  = 24
		gap      = 16
		inset    = 16
	)

	columns := referenceImageSheetColumns(len(images))
	rows := (len(images) + columns - 1) / columns
	width := padding*2 + columns*tileSize + (columns-1)*gap
	height := padding*2 + rows*tileSize + (rows-1)*gap
	sheet := image.NewRGBA(image.Rect(0, 0, width, height))

	draw.Draw(sheet, sheet.Bounds(), &image.Uniform{C: color.RGBA{R: 248, G: 250, B: 252, A: 255}}, image.Point{}, draw.Src)
	for index, img := range images {
		col := index % columns
		row := index / columns
		x := padding + col*(tileSize+gap)
		y := padding + row*(tileSize+gap)
		tileRect := image.Rect(x, y, x+tileSize, y+tileSize)
		draw.Draw(sheet, tileRect, &image.Uniform{C: color.RGBA{R: 255, G: 255, B: 255, A: 255}}, image.Point{}, draw.Src)
		drawReferenceImageSheetBorder(sheet, tileRect)
		drawImageContain(sheet, tileRect.Inset(inset), img)
		if labelReferences {
			drawReferenceImageSheetLabel(sheet, tileRect, index+1)
		}
	}
	return sheet
}

func drawReferenceImageSheetLabel(dst *image.RGBA, tileRect image.Rectangle, number int) {
	badge := image.Rect(tileRect.Min.X+14, tileRect.Min.Y+14, tileRect.Min.X+112, tileRect.Min.Y+62)
	fillRect(dst, badge, color.RGBA{R: 255, G: 255, B: 255, A: 245})
	fillRect(dst, image.Rect(badge.Min.X, badge.Min.Y, badge.Max.X, badge.Min.Y+2), color.RGBA{R: 15, G: 23, B: 42, A: 255})
	fillRect(dst, image.Rect(badge.Min.X, badge.Max.Y-2, badge.Max.X, badge.Max.Y), color.RGBA{R: 15, G: 23, B: 42, A: 255})
	fillRect(dst, image.Rect(badge.Min.X, badge.Min.Y, badge.Min.X+2, badge.Max.Y), color.RGBA{R: 15, G: 23, B: 42, A: 255})
	fillRect(dst, image.Rect(badge.Max.X-2, badge.Min.Y, badge.Max.X, badge.Max.Y), color.RGBA{R: 15, G: 23, B: 42, A: 255})

	labelColor := color.RGBA{R: 15, G: 23, B: 42, A: 255}
	drawReferenceImageSheetTuGlyph(dst, badge.Min.X+10, badge.Min.Y+10, 2, labelColor)
	drawReferenceImageSheetNumber(dst, badge.Min.X+52, badge.Min.Y+8, 4, number, labelColor)
}

func drawReferenceImageSheetTuGlyph(dst *image.RGBA, x, y, scale int, c color.RGBA) {
	stroke := max(1, scale)
	fillRect(dst, image.Rect(x, y, x+14*scale, y+stroke), c)
	fillRect(dst, image.Rect(x, y+16*scale-stroke, x+14*scale, y+16*scale), c)
	fillRect(dst, image.Rect(x, y, x+stroke, y+16*scale), c)
	fillRect(dst, image.Rect(x+14*scale-stroke, y, x+14*scale, y+16*scale), c)
	fillRect(dst, image.Rect(x+4*scale, y+5*scale, x+10*scale, y+7*scale), c)
	fillRect(dst, image.Rect(x+5*scale, y+8*scale, x+9*scale, y+10*scale), c)
	fillRect(dst, image.Rect(x+6*scale, y+11*scale, x+8*scale, y+13*scale), c)
}

func drawReferenceImageSheetNumber(dst *image.RGBA, x, y, scale, number int, c color.RGBA) {
	if number < 0 {
		number = 0
	}
	digits := strconv.Itoa(number)
	for index, digit := range digits {
		drawReferenceImageSheetDigit(dst, x+index*6*scale, y, scale, digit, c)
	}
}

func drawReferenceImageSheetDigit(dst *image.RGBA, x, y, scale int, digit rune, c color.RGBA) {
	patterns := map[rune][]string{
		'0': {"111", "101", "101", "101", "101", "101", "111"},
		'1': {"010", "110", "010", "010", "010", "010", "111"},
		'2': {"111", "001", "001", "111", "100", "100", "111"},
		'3': {"111", "001", "001", "111", "001", "001", "111"},
		'4': {"101", "101", "101", "111", "001", "001", "001"},
		'5': {"111", "100", "100", "111", "001", "001", "111"},
		'6': {"111", "100", "100", "111", "101", "101", "111"},
		'7': {"111", "001", "001", "010", "010", "010", "010"},
		'8': {"111", "101", "101", "111", "101", "101", "111"},
		'9': {"111", "101", "101", "111", "001", "001", "111"},
	}
	rows, ok := patterns[digit]
	if !ok {
		return
	}
	for rowIndex, row := range rows {
		for colIndex, cell := range row {
			if cell != '1' {
				continue
			}
			fillRect(dst, image.Rect(x+colIndex*scale, y+rowIndex*scale, x+(colIndex+1)*scale, y+(rowIndex+1)*scale), c)
		}
	}
}

func fillRect(dst *image.RGBA, rect image.Rectangle, c color.RGBA) {
	draw.Draw(dst, rect.Intersect(dst.Bounds()), &image.Uniform{C: c}, image.Point{}, draw.Src)
}

func referenceImageSheetColumns(count int) int {
	switch {
	case count <= 1:
		return 1
	case count <= 4:
		return 2
	default:
		return 3
	}
}

func drawReferenceImageSheetBorder(dst *image.RGBA, rect image.Rectangle) {
	borderColor := &image.Uniform{C: color.RGBA{R: 214, G: 222, B: 232, A: 255}}
	draw.Draw(dst, image.Rect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Min.Y+1), borderColor, image.Point{}, draw.Src)
	draw.Draw(dst, image.Rect(rect.Min.X, rect.Max.Y-1, rect.Max.X, rect.Max.Y), borderColor, image.Point{}, draw.Src)
	draw.Draw(dst, image.Rect(rect.Min.X, rect.Min.Y, rect.Min.X+1, rect.Max.Y), borderColor, image.Point{}, draw.Src)
	draw.Draw(dst, image.Rect(rect.Max.X-1, rect.Min.Y, rect.Max.X, rect.Max.Y), borderColor, image.Point{}, draw.Src)
}

func drawImageContain(dst *image.RGBA, dstBounds image.Rectangle, src image.Image) {
	srcBounds := src.Bounds()
	if srcBounds.Dx() <= 0 || srcBounds.Dy() <= 0 || dstBounds.Dx() <= 0 || dstBounds.Dy() <= 0 {
		return
	}

	scale := math.Min(float64(dstBounds.Dx())/float64(srcBounds.Dx()), float64(dstBounds.Dy())/float64(srcBounds.Dy()))
	width := max(1, int(math.Round(float64(srcBounds.Dx())*scale)))
	height := max(1, int(math.Round(float64(srcBounds.Dy())*scale)))
	target := image.Rect(0, 0, width, height)
	target = target.Add(image.Point{
		X: dstBounds.Min.X + (dstBounds.Dx()-width)/2,
		Y: dstBounds.Min.Y + (dstBounds.Dy()-height)/2,
	})

	drawScaledNearest(dst, target, src)
}

func drawScaledNearest(dst *image.RGBA, dstBounds image.Rectangle, src image.Image) {
	srcBounds := src.Bounds()
	dstWidth := dstBounds.Dx()
	dstHeight := dstBounds.Dy()
	if dstWidth <= 0 || dstHeight <= 0 {
		return
	}

	for y := 0; y < dstHeight; y++ {
		srcY := srcBounds.Min.Y + y*srcBounds.Dy()/dstHeight
		for x := 0; x < dstWidth; x++ {
			srcX := srcBounds.Min.X + x*srcBounds.Dx()/dstWidth
			dst.Set(dstBounds.Min.X+x, dstBounds.Min.Y+y, src.At(srcX, srcY))
		}
	}
}

func imageReferenceTransportMetadata(input ImageGenerationInput) map[string]any {
	mode, payloadCount := imageReferenceTransportForInput(input)
	return map[string]any{
		"reference_transport_mode":     mode,
		"provider_image_payload_count": payloadCount,
	}
}

func imageReferenceTransportForInput(input ImageGenerationInput) (string, int) {
	imageCount := len(orderedReferenceImageInputs(input))
	if imageCount == 0 {
		return imageReferenceTransportModeNone, 0
	}
	if useChatCompletionsImageGeneration(input) {
		return imageReferenceTransportModeChatMultiImage, imageCount
	}
	if useResponsesImageGeneration(input) {
		return imageReferenceTransportModeResponsesMultiImage, imageCount
	}
	return imageReferenceTransportModeImagesEditsMultipart, imageCount
}

func chatImageMessageContent(input ImageGenerationInput) any {
	if input.SourceImage == nil && len(input.ReferenceImages) == 0 {
		return composeImagePrompt(input)
	}

	content := make([]map[string]any, 0, 1+len(input.ReferenceImages))
	content = append(content, map[string]any{
		"type": "text",
		"text": composeImagePrompt(input),
	})
	if input.SourceImage != nil {
		content = append(content, chatImageURLContentItem(*input.SourceImage))
	}
	for _, image := range input.ReferenceImages {
		content = append(content, chatImageURLContentItem(image))
	}
	return content
}

func chatImageURLContentItem(image ReferenceImageInput) map[string]any {
	return map[string]any{
		"type": "image_url",
		"image_url": map[string]any{
			"url": fmt.Sprintf("data:%s;base64,%s", normalizeImageMimeType(image.MIMEType), image.Base64Data),
		},
	}
}

type chatImageCandidate struct {
	base64Image string
	mimeType    string
	imageURL    string
}

func extractChatImageCandidate(raw json.RawMessage) (chatImageCandidate, bool) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return chatImageCandidate{}, false
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return chatImageCandidate{}, false
	}
	return extractChatImageCandidateFromValue(value, "")
}

func extractChatImageCandidateFromValue(value any, keyHint string) (chatImageCandidate, bool) {
	switch typed := value.(type) {
	case string:
		return extractChatImageCandidateFromString(typed, keyHint)
	case []any:
		for _, item := range typed {
			if candidate, ok := extractChatImageCandidateFromValue(item, keyHint); ok {
				return candidate, true
			}
		}
	case map[string]any:
		for _, key := range []string{"b64_json", "base64", "b64", "url", "image_url", "images", "data", "content", "text"} {
			if nested, ok := typed[key]; ok {
				if candidate, found := extractChatImageCandidateFromValue(nested, key); found {
					return candidate, true
				}
			}
		}
		for key, nested := range typed {
			if candidate, ok := extractChatImageCandidateFromValue(nested, key); ok {
				return candidate, true
			}
		}
	}
	return chatImageCandidate{}, false
}

func extractChatImageCandidateFromString(value, keyHint string) (chatImageCandidate, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return chatImageCandidate{}, false
	}

	if isDataImageURL(trimmed) {
		base64Image, mimeType := normalizeProviderBase64Image(trimmed, "image/png")
		return chatImageCandidate{base64Image: base64Image, mimeType: mimeType}, true
	}

	if isChatImageBase64Key(keyHint) {
		base64Image, mimeType := normalizeProviderBase64Image(trimmed, "image/png")
		if strings.TrimSpace(base64Image) != "" {
			return chatImageCandidate{base64Image: base64Image, mimeType: mimeType}, true
		}
	}

	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		var nested any
		if err := json.Unmarshal([]byte(trimmed), &nested); err == nil {
			if candidate, ok := extractChatImageCandidateFromValue(nested, keyHint); ok {
				return candidate, true
			}
		}
	}

	if markdownURL := extractMarkdownImageURL(trimmed); markdownURL != "" {
		return extractChatImageCandidateFromString(markdownURL, "url")
	}
	if httpURL := extractHTTPURLFromText(trimmed); httpURL != "" {
		return chatImageCandidate{imageURL: httpURL}, true
	}

	return chatImageCandidate{}, false
}

func isDataImageURL(value string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "data:image/")
}

func isChatImageBase64Key(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "b64_json", "base64", "b64":
		return true
	default:
		return false
	}
}

func extractMarkdownImageURL(text string) string {
	search := text
	for {
		imageStart := strings.Index(search, "![")
		if imageStart < 0 {
			return ""
		}
		linkStartRelative := strings.Index(search[imageStart:], "](")
		if linkStartRelative < 0 {
			return ""
		}
		urlStart := imageStart + linkStartRelative + len("](")
		urlEndRelative := strings.Index(search[urlStart:], ")")
		if urlEndRelative < 0 {
			return ""
		}
		candidate := strings.Trim(strings.TrimSpace(search[urlStart:urlStart+urlEndRelative]), `"'`)
		if candidate != "" {
			return candidate
		}
		search = search[urlStart+urlEndRelative+1:]
	}
}

func extractHTTPURLFromText(text string) string {
	start := strings.Index(text, "https://")
	if start < 0 {
		start = strings.Index(text, "http://")
	}
	if start < 0 {
		return ""
	}
	end := len(text)
	for idx, r := range text[start:] {
		if idx == 0 {
			continue
		}
		if strings.ContainsRune(" \t\r\n)]>\"'`", r) {
			end = start + idx
			break
		}
	}
	return strings.Trim(strings.TrimSpace(text[start:end]), `"'`)
}

func normalizeProviderBase64Image(value, fallbackMIMEType string) (string, string) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(value), "data:") {
		header, data, ok := strings.Cut(value, ",")
		if ok {
			mimeType := fallbackMIMEType
			if mediaType := strings.TrimPrefix(strings.Split(strings.TrimPrefix(header, "data:"), ";")[0], "data:"); strings.TrimSpace(mediaType) != "" {
				mimeType = mediaType
			}
			return strings.TrimSpace(data), normalizeImageMimeType(mimeType)
		}
	}
	return value, normalizeImageMimeType(fallbackMIMEType)
}

func providerBaseURL(defaultBaseURL string, input ImageGenerationInput) string {
	baseURL := strings.TrimRight(strings.TrimSpace(input.ProviderBaseURL), "/")
	if baseURL != "" {
		return baseURL
	}
	return defaultBaseURL
}

func providerAPIKey(defaultAPIKey string, input ImageGenerationInput) string {
	apiKey := strings.TrimSpace(input.ProviderAPIKey)
	if apiKey != "" {
		return apiKey
	}
	return defaultAPIKey
}

func providerAPIEndpoint(input ImageGenerationInput, defaultEndpoint string) string {
	endpoint := strings.TrimSpace(input.ProviderAPIEndpoint)
	if endpoint == "" || endpoint == "responses" {
		return defaultEndpoint
	}
	if !strings.HasPrefix(endpoint, "/") {
		endpoint = "/" + endpoint
	}
	return endpoint
}

func providerImagesEditsEndpoint(input ImageGenerationInput) string {
	endpoint := providerAPIEndpoint(input, "/v1/images/edits")
	if strings.Contains(endpoint, "/images/generations") {
		return strings.Replace(endpoint, "/images/generations", "/images/edits", 1)
	}
	return endpoint
}

func providerChatCompletionsEndpoint(input ImageGenerationInput) string {
	endpoint := strings.ToLower(strings.TrimSpace(input.ProviderAPIEndpoint))
	if endpoint == "chat" {
		return "/v1/chat/completions"
	}
	return providerAPIEndpoint(input, "/v1/chat/completions")
}

func providerResponsesEndpoint(input ImageGenerationInput) string {
	endpoint := strings.ToLower(strings.TrimSpace(input.ProviderAPIEndpoint))
	if endpoint == "responses" || strings.Contains(endpoint, "/responses") {
		return providerAPIEndpoint(input, "/v1/responses")
	}
	return "/v1/responses"
}

func providerHTTPError(statusCode int, rawBody []byte, requestID, failureStage string) *ProviderError {
	message := http.StatusText(statusCode)
	var apiErr struct {
		Error struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"error"`
		Message string `json:"message"`
		Msg     string `json:"msg"`
		Code    string `json:"code"`
	}
	if err := json.Unmarshal(rawBody, &apiErr); err == nil {
		if strings.TrimSpace(apiErr.Error.Message) != "" {
			message = strings.TrimSpace(apiErr.Error.Message)
		}
		if strings.TrimSpace(apiErr.Error.Code) != "" {
			code, normalizedMessage := normalizeProviderHTTPErrorDetails(apiErr.Error.Code, message)
			return &ProviderError{
				HTTPStatus:        statusCode,
				Code:              code,
				Message:           normalizedMessage,
				ProviderRequestID: requestID,
				FailureStage:      failureStage,
			}
		}
		if strings.TrimSpace(apiErr.Message) != "" {
			message = strings.TrimSpace(apiErr.Message)
		}
		if strings.TrimSpace(apiErr.Msg) != "" {
			message = strings.TrimSpace(apiErr.Msg)
		}
		if strings.TrimSpace(apiErr.Code) != "" {
			code, normalizedMessage := normalizeProviderHTTPErrorDetails(apiErr.Code, message)
			return &ProviderError{
				HTTPStatus:        statusCode,
				Code:              code,
				Message:           normalizedMessage,
				ProviderRequestID: requestID,
				FailureStage:      failureStage,
			}
		}
		if message == http.StatusText(statusCode) {
			if summary := providerErrorBodySummary(rawBody); summary != "" {
				message = summary
			}
		}
	} else if summary := providerErrorBodySummary(rawBody); summary != "" {
		message = summary
	}
	code, normalizedMessage := normalizeProviderHTTPErrorDetails(fmt.Sprintf("provider_http_%d", statusCode), message)
	return &ProviderError{
		HTTPStatus:        statusCode,
		Code:              code,
		Message:           normalizedMessage,
		ProviderRequestID: requestID,
		FailureStage:      failureStage,
	}
}

func providerHTTPErrorWithResponse(resp *http.Response, rawBody []byte, requestID, failureStage string) *ProviderError {
	if resp == nil {
		return providerHTTPError(0, rawBody, requestID, failureStage)
	}
	err := providerHTTPError(resp.StatusCode, rawBody, requestID, failureStage)
	value := strings.TrimSpace(resp.Header.Get("Retry-After"))
	if seconds, parseErr := strconv.Atoi(value); parseErr == nil && seconds > 0 {
		err.RetryAfter = time.Duration(seconds) * time.Second
	} else if retryAt, parseErr := http.ParseTime(value); parseErr == nil && retryAt.After(time.Now()) {
		err.RetryAfter = time.Until(retryAt)
	}
	return err
}

func normalizeProviderHTTPErrorDetails(code, message string) (string, string) {
	code = sanitizeProviderErrorCode(code)
	message = strings.TrimSpace(message)
	if nestedCode, nestedMessage, traceSuffix := parseNestedProviderJSONMessage(message); nestedCode != "" || nestedMessage != "" {
		if nestedCode != "" {
			code = nestedCode
		}
		if nestedMessage != "" {
			message = nestedMessage + traceSuffix
		}
	}
	lower := strings.ToLower(strings.TrimSpace(code + " " + message))
	switch {
	case strings.Contains(lower, "token_invalidated") || strings.Contains(lower, "authentication token has been invalidated"):
		return "token_invalidated", "模型通道认证已失效，请检查 API Key 或切换可用通道" + providerTraceSuffix(message)
	case strings.Contains(lower, "no available channel"):
		return fallbackString(code, "provider_channel_unavailable"), "当前模型通道没有可用渠道，请切换可用通道或联系供应商" + providerRequestIDSuffix(message)
	default:
		return fallbackString(code, "provider_error"), message
	}
}

func sanitizeProviderErrorCode(code string) string {
	code = strings.TrimSpace(code)
	if code == "" || code == "<nil>" || strings.EqualFold(code, "null") {
		return ""
	}
	return code
}

func parseNestedProviderJSONMessage(message string) (string, string, string) {
	jsonText, suffix := splitProviderJSONPrefix(message)
	if jsonText == "" {
		return "", "", ""
	}
	var payload struct {
		Error struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"error"`
		Message string `json:"message"`
		Msg     string `json:"msg"`
		Code    string `json:"code"`
	}
	if err := json.Unmarshal([]byte(jsonText), &payload); err != nil {
		return "", "", ""
	}
	nestedCode := sanitizeProviderErrorCode(payload.Error.Code)
	if nestedCode == "" {
		nestedCode = sanitizeProviderErrorCode(payload.Code)
	}
	nestedMessage := strings.TrimSpace(payload.Error.Message)
	if nestedMessage == "" {
		nestedMessage = strings.TrimSpace(payload.Message)
	}
	if nestedMessage == "" {
		nestedMessage = strings.TrimSpace(payload.Msg)
	}
	return nestedCode, nestedMessage, suffix
}

func splitProviderJSONPrefix(message string) (string, string) {
	message = strings.TrimSpace(message)
	if !strings.HasPrefix(message, "{") && !strings.HasPrefix(message, "[") {
		return "", ""
	}
	for index := len(message); index > 0; index-- {
		candidate := strings.TrimSpace(message[:index])
		if candidate == "" {
			continue
		}
		var raw json.RawMessage
		if err := json.Unmarshal([]byte(candidate), &raw); err == nil {
			return candidate, strings.TrimSpace(message[index:])
		}
	}
	return "", ""
}

func providerTraceSuffix(message string) string {
	for _, marker := range []string{"（traceid:", "(traceid:"} {
		if index := strings.Index(message, marker); index >= 0 {
			return strings.TrimSpace(message[index:])
		}
	}
	return ""
}

func providerRequestIDSuffix(message string) string {
	for _, marker := range []string{"(request id:", "（request id:", "(请求ID:", "（请求ID:"} {
		if index := strings.Index(message, marker); index >= 0 {
			return strings.TrimSpace(message[index:])
		}
	}
	return ""
}

func decodeProviderJSON(rawBody []byte, target any, requestID, failureStage string) *ProviderError {
	if len(bytes.TrimSpace(rawBody)) == 0 {
		return &ProviderError{
			Code:              "provider_decode_failed",
			Message:           "provider returned empty response body",
			ProviderRequestID: requestID,
			FailureStage:      failureStage,
		}
	}
	if err := json.Unmarshal(rawBody, target); err != nil {
		return &ProviderError{
			Code:              "provider_decode_failed",
			Message:           "provider returned invalid JSON",
			ProviderRequestID: requestID,
			FailureStage:      failureStage,
		}
	}
	return nil
}

func providerRequestError(err error, failureStage string) *ProviderError {
	code := "provider_request_failed"
	if errors.Is(err, context.DeadlineExceeded) {
		code = "provider_timeout"
	}
	requestNotSent := false
	var operationError *net.OpError
	if errors.As(err, &operationError) && strings.EqualFold(strings.TrimSpace(operationError.Op), "dial") {
		requestNotSent = true
	}
	return &ProviderError{Code: code, Message: err.Error(), FailureStage: failureStage, RequestNotSent: requestNotSent}
}

const (
	maxImageProviderSuccessResponseBytes = 64 << 20
	maxImageProviderErrorResponseBytes   = 1 << 20
)

func readLimitedImageProviderResponse(resp *http.Response) ([]byte, *ProviderError) {
	if resp == nil || resp.Body == nil {
		return nil, &ProviderError{Code: "provider_decode_failed", Message: "provider response body is empty"}
	}
	limit := int64(maxImageProviderSuccessResponseBytes)
	if resp.StatusCode >= http.StatusBadRequest {
		limit = maxImageProviderErrorResponseBytes
	}
	if resp.ContentLength > limit {
		return nil, &ProviderError{HTTPStatus: resp.StatusCode, Code: "generation_payload_too_large", Message: "provider response exceeds size limit"}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, &ProviderError{HTTPStatus: resp.StatusCode, Code: "provider_decode_failed", Message: err.Error()}
	}
	if int64(len(body)) > limit {
		return nil, &ProviderError{HTTPStatus: resp.StatusCode, Code: "generation_payload_too_large", Message: "provider response exceeds size limit"}
	}
	return body, nil
}

func decodeLimitedImageProviderSuccess(resp *http.Response, target any, requestID, failureStage string) *ProviderError {
	if resp == nil || resp.Body == nil {
		return &ProviderError{Code: "provider_decode_failed", Message: "provider response body is empty", ProviderRequestID: requestID, FailureStage: failureStage}
	}
	if resp.ContentLength > maxImageProviderSuccessResponseBytes {
		return &ProviderError{HTTPStatus: resp.StatusCode, Code: "generation_payload_too_large", Message: "provider response exceeds size limit", ProviderRequestID: requestID, FailureStage: failureStage}
	}
	limited := &io.LimitedReader{R: resp.Body, N: maxImageProviderSuccessResponseBytes + 1}
	decoder := json.NewDecoder(limited)
	if err := decoder.Decode(target); err != nil {
		code := "provider_decode_failed"
		message := "provider returned invalid JSON"
		if errors.Is(err, io.EOF) {
			message = "provider returned empty response body"
		} else if limited.N == 0 {
			code = "generation_payload_too_large"
			message = "provider response exceeds size limit"
		}
		return &ProviderError{HTTPStatus: resp.StatusCode, Code: code, Message: message, ProviderRequestID: requestID, FailureStage: failureStage}
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		code := "provider_decode_failed"
		message := "provider returned invalid JSON"
		if limited.N == 0 {
			code = "generation_payload_too_large"
			message = "provider response exceeds size limit"
		}
		return &ProviderError{HTTPStatus: resp.StatusCode, Code: code, Message: message, ProviderRequestID: requestID, FailureStage: failureStage}
	}
	return nil
}

func (p *OpenAIProvider) spoolImageGenerationResult(value, mimeType, requestID string) (ImageGenerationResult, *ProviderError) {
	imageData, normalizedMIME := normalizeProviderBase64Image(value, mimeType)
	spoolPath := strings.TrimSpace(p.spoolPath)
	if spoolPath == "" {
		return ImageGenerationResult{Base64Image: imageData, MIMEType: normalizedMIME, ProviderRequestID: requestID}, nil
	}
	if err := os.MkdirAll(spoolPath, 0o750); err != nil {
		return ImageGenerationResult{}, &ProviderError{Code: "generation_spool_failed", Message: err.Error(), ProviderRequestID: requestID}
	}
	if p.spoolMaxBytes > 0 {
		var used int64
		_ = filepath.WalkDir(spoolPath, func(_ string, entry os.DirEntry, err error) error {
			if err == nil && !entry.IsDir() {
				if info, infoErr := entry.Info(); infoErr == nil {
					used += info.Size()
				}
			}
			return nil
		})
		if used >= p.spoolMaxBytes {
			return ImageGenerationResult{}, &ProviderError{Code: "generation_queue_full", Message: "generation spool capacity exceeded", ProviderRequestID: requestID}
		}
	}
	partial, err := os.CreateTemp(spoolPath, "generation-result-*.partial")
	if err != nil {
		return ImageGenerationResult{}, &ProviderError{Code: "generation_spool_failed", Message: err.Error(), ProviderRequestID: requestID}
	}
	partialName := partial.Name()
	finalName := strings.TrimSuffix(partialName, ".partial") + extensionForMimeType(normalizedMIME)
	decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(compactBase64Data(imageData)))
	written, copyErr := io.Copy(partial, io.LimitReader(decoder, maxImageProviderSuccessResponseBytes+1))
	closeErr := partial.Close()
	if copyErr != nil || closeErr != nil || written > maxImageProviderSuccessResponseBytes {
		_ = os.Remove(partialName)
		if copyErr != nil {
			return ImageGenerationResult{}, &ProviderError{Code: "provider_decode_failed", Message: copyErr.Error(), ProviderRequestID: requestID}
		}
		if closeErr != nil {
			return ImageGenerationResult{}, &ProviderError{Code: "generation_spool_failed", Message: closeErr.Error(), ProviderRequestID: requestID}
		}
		return ImageGenerationResult{}, &ProviderError{Code: "generation_payload_too_large", Message: "decoded image exceeds size limit", ProviderRequestID: requestID}
	}
	if err := os.Rename(partialName, finalName); err != nil {
		_ = os.Remove(partialName)
		return ImageGenerationResult{}, &ProviderError{Code: "generation_spool_failed", Message: err.Error(), ProviderRequestID: requestID}
	}
	return ImageGenerationResult{FilePath: finalName, MIMEType: normalizedMIME, ProviderRequestID: requestID}, nil
}

func providerErrorBodySummary(rawBody []byte) string {
	summary := strings.TrimSpace(strings.ToValidUTF8(string(rawBody), ""))
	if summary == "" {
		return ""
	}
	const maxProviderErrorSummaryLength = 500
	if len(summary) > maxProviderErrorSummaryLength {
		return summary[:maxProviderErrorSummaryLength]
	}
	return summary
}

func composeImagePrompt(input ImageGenerationInput) string {
	if isComposeReferenceIntent(input) {
		return composeImageCompositionPrompt(input)
	}
	if isCharacterReferenceIntent(input) {
		return composeCharacterReferencePrompt(input)
	}
	parts := []string{strings.TrimSpace(input.Prompt)}
	if strings.TrimSpace(input.NegativePrompt) != "" {
		parts = append(parts, "反向提示词："+strings.TrimSpace(input.NegativePrompt))
	}
	if strings.TrimSpace(input.ToolMode) != "" && strings.TrimSpace(input.ToolMode) != GenerationToolModeGenerate {
		parts = append(parts, "工具模式："+strings.TrimSpace(input.ToolMode))
	}
	if strings.TrimSpace(input.StylePreset) != "" {
		parts = append(parts, "风格预设："+strings.TrimSpace(input.StylePreset))
	}
	if input.StyleStrength > 0 {
		parts = append(parts, fmt.Sprintf("风格强度：%d/100", input.StyleStrength))
	}
	if input.ReferenceWeight > 0 {
		parts = append(parts, fmt.Sprintf("相似度/参考权重：%d/100", input.ReferenceWeight))
	}
	if usesDefaultReferenceIntent(input) {
		parts = append(parts, imageDefaultReferencePromptInstruction)
	}
	if strings.TrimSpace(input.VariationPrompt) != "" {
		if strings.TrimSpace(input.VariationMode) != "" {
			parts = append(parts, "变化模式："+strings.TrimSpace(input.VariationMode))
		}
		parts = append(parts, "本张变化方向："+strings.TrimSpace(input.VariationPrompt))
	}
	if input.MaskImage != nil {
		parts = append(parts, "蒙版参考图：白色区域为待移除区域，黑色或透明区域必须保持不变。")
	}
	if len(input.MaskRegions) > 0 {
		parts = append(parts, fmt.Sprintf("圈选区域数量：%d；仅处理圈选区域。", len(input.MaskRegions)))
	}
	if strings.TrimSpace(input.Seed) != "" {
		parts = append(parts, "随机种子："+strings.TrimSpace(input.Seed))
	}
	return strings.Join(parts, "\n")
}

func usesDefaultReferenceIntent(input ImageGenerationInput) bool {
	return strings.TrimSpace(input.ReferenceIntent) == "" && (input.SourceImage != nil || len(input.ReferenceImages) > 0)
}

func composeCharacterReferencePrompt(input ImageGenerationInput) string {
	parts := []string{strings.TrimSpace(input.Prompt)}
	if strings.TrimSpace(input.NegativePrompt) != "" {
		parts = append(parts, "反向提示词："+strings.TrimSpace(input.NegativePrompt))
	}
	parts = append(parts, characterReferenceConstraintPrompt(input)...)
	return strings.Join(nonEmptyStrings(parts), "\n")
}

func characterReferenceConstraintPrompt(input ImageGenerationInput) []string {
	imageCount := len(orderedReferenceImageInputs(input))
	parts := []string{}
	if imageCount > 0 {
		labels := make([]string, 0, imageCount)
		for index := 0; index < imageCount; index++ {
			labels = append(labels, fmt.Sprintf("【图%d】", index+1))
		}
		parts = append(parts, "参考图按输入顺序编号为"+strings.Join(labels, "、")+"；人物角色映射必须与该顺序一一对应。")
	}
	parts = append(parts, "参考图只作为人物身份参考：保留每张参考图中人物的身份、五官结构、脸型、发型、肤色、体态和整体气质。")
	parts = append(parts, "不要把不同参考图的人物特征混合成陌生人，不要换脸、不要互换角色，不要新增人物、第三人、路人、人群、额外肢体或无关主体。")
	parts = append(parts, "背景/场景、光线、构图和叙事内容按用户提示词生成，不从任一参考图抽取或固定背景。")
	return parts
}

func composeImageCompositionPrompt(input ImageGenerationInput) string {
	if input.CompositionPlan != nil && strings.TrimSpace(input.CompositionPlan.Prompt) != "" {
		return strings.TrimSpace(input.CompositionPlan.Prompt)
	}
	parts := []string{strings.TrimSpace(input.Prompt)}
	if strings.TrimSpace(input.NegativePrompt) != "" {
		parts = append(parts, "反向提示词："+strings.TrimSpace(input.NegativePrompt))
	}
	parts = append(parts, composeReferenceConstraintPrompt(input)...)
	return strings.Join(nonEmptyStrings(parts), "\n")
}

func composeReferenceConstraintPrompt(input ImageGenerationInput) []string {
	imageCount := len(orderedReferenceImageInputs(input))
	parts := []string{}
	if imageCount > 0 {
		labels := make([]string, 0, imageCount)
		for index := 0; index < imageCount; index++ {
			labels = append(labels, fmt.Sprintf("【图%d】", index+1))
		}
		parts = append(parts, "参考图按输入顺序编号为"+strings.Join(labels, "、")+"；后续约束中的编号必须与该顺序一一对应。")
	}
	if backgroundIndex := normalizedBackgroundReferenceIndex(input, imageCount); backgroundIndex >= 0 {
		parts = append(parts, fmt.Sprintf("背景/场景严格取【图%d】，保留该图的空间结构、环境、光线和视角，不要替换成其他背景。", backgroundIndex+1))
	}
	parts = append(parts, "只保留参考图中已经出现的人物，按参考图主体进行合成；不要新增人物、路人、人群、额外肢体或无关主体。")
	parts = append(parts, "在满足用户提示词的前提下进行精准合成，优先保持人物身份特征、服装和背景来源的清晰映射。")
	return parts
}

func nonEmptyStrings(values []string) []string {
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}
	return filtered
}

func isComposeReferenceIntent(input ImageGenerationInput) bool {
	return strings.EqualFold(strings.TrimSpace(input.ReferenceIntent), GenerationReferenceIntentCompose)
}

func isCharacterReferenceIntent(input ImageGenerationInput) bool {
	return strings.EqualFold(strings.TrimSpace(input.ReferenceIntent), GenerationReferenceIntentCharacter)
}

func isStrongMultiReferenceIntent(input ImageGenerationInput) bool {
	return isComposeReferenceIntent(input) || isCharacterReferenceIntent(input)
}

func normalizedBackgroundReferenceIndex(input ImageGenerationInput, imageCount int) int {
	if imageCount <= 0 {
		return -1
	}
	if input.BackgroundReferenceIndex != nil && *input.BackgroundReferenceIndex >= 0 && *input.BackgroundReferenceIndex < imageCount {
		return *input.BackgroundReferenceIndex
	}
	if imageCount == 1 {
		return 0
	}
	return imageCount - 1
}

func providerImageQuality(quality string) string {
	switch strings.TrimSpace(quality) {
	case GenerationQualityLow, GenerationQualityHigh:
		return strings.TrimSpace(quality)
	case GenerationQualityUltra:
		return GenerationQualityHigh
	default:
		return GenerationQualityMedium
	}
}

func providerImageAction(mode string) string {
	if isEditToolMode(normalizeGenerationToolMode(mode)) {
		return "edit"
	}
	return "generate"
}

var providerAssetFetchRetryBackoffs = []time.Duration{
	100 * time.Millisecond,
	300 * time.Millisecond,
}

func (p *OpenAIProvider) fetchImageURL(ctx context.Context, imageURL, requestID string) (string, string, *ProviderError) {
	attempts := 1 + len(providerAssetFetchRetryBackoffs)
	var lastErr *ProviderError
	for attempt := 1; attempt <= attempts; attempt++ {
		base64Image, mimeType, providerErr := p.fetchBinaryURL(ctx, imageURL, requestID)
		if providerErr == nil {
			return base64Image, mimeType, nil
		}
		providerErr.AttemptCount = attempt
		lastErr = providerErr
		if !isRetryableProviderAssetFetchError(providerErr) || attempt == attempts {
			return "", "", providerErr
		}

		timer := time.NewTimer(providerAssetFetchRetryBackoffs[attempt-1])
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", "", lastErr
		case <-timer.C:
		}
	}
	return "", "", lastErr
}

func isRetryableProviderAssetFetchError(err *ProviderError) bool {
	if err == nil {
		return false
	}
	switch strings.TrimSpace(err.Code) {
	case "provider_asset_fetch_failed", "provider_empty_asset":
		return true
	}
	switch err.HTTPStatus {
	case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func (p *OpenAIProvider) fetchBinaryURL(ctx context.Context, imageURL, requestID string) (string, string, *ProviderError) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return "", "", &ProviderError{
			Code:              "provider_asset_fetch_failed",
			Message:           err.Error(),
			ProviderRequestID: requestID,
			FailureStage:      providerFailureStageProviderAssetFetch,
		}
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return "", "", &ProviderError{
			Code:              "provider_asset_fetch_failed",
			Message:           err.Error(),
			ProviderRequestID: requestID,
			FailureStage:      providerFailureStageProviderAssetFetch,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		rawBody, _ := io.ReadAll(resp.Body)
		message := http.StatusText(resp.StatusCode)
		if summary := providerErrorBodySummary(rawBody); summary != "" {
			message = summary
		}
		return "", "", &ProviderError{
			HTTPStatus:        resp.StatusCode,
			Code:              fmt.Sprintf("provider_asset_http_%d", resp.StatusCode),
			Message:           message,
			ProviderRequestID: requestID,
			FailureStage:      providerFailureStageProviderAssetFetch,
		}
	}

	assetBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", &ProviderError{
			Code:              "provider_asset_fetch_failed",
			Message:           err.Error(),
			ProviderRequestID: requestID,
			FailureStage:      providerFailureStageProviderAssetFetch,
		}
	}
	if len(assetBytes) == 0 {
		return "", "", &ProviderError{
			Code:              "provider_empty_asset",
			Message:           "provider returned no asset data",
			ProviderRequestID: requestID,
			FailureStage:      providerFailureStageProviderAssetFetch,
		}
	}

	mimeType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	} else if mediaType, _, found := strings.Cut(mimeType, ";"); found {
		mimeType = strings.TrimSpace(mediaType)
	}

	return base64.StdEncoding.EncodeToString(assetBytes), mimeType, nil
}
