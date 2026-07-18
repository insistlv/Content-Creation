package app

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAsyncVideoGenerationWritesVideoGenerationRecord(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	videoProvider := &stubVideoProvider{
		submitResult: VideoSubmitResult{TaskID: "video-task-123", ProviderRequestID: "provider-submit-123"},
		pollResults: []VideoTaskResult{
			{TaskID: "video-task-123", Status: VideoTaskSucceeded, OutputBase64: base64.StdEncoding.EncodeToString([]byte("video-bytes")), MIMEType: "video/mp4", ProviderRequestID: "provider-poll-123"},
		},
	}
	testApp.videoProvider = videoProvider

	user, cookies := createLoggedInUser(t, testApp, "video_record_user", "test-password")
	setUserCredits(t, testApp, user.ID, 20)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/generations/async", map[string]any{
		"prompt":       "product launch video",
		"aspect_ratio": "16:9",
		"duration":     "10",
		"model":        "sora-2",
	}, cookies)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected accepted video generation, got %d: %s", resp.Code, resp.Body.String())
	}
	var created struct {
		GenerationID uint `json:"generation_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	var videoRecord VideoGenerationRecord
	waitFor(t, time.Second, func() bool {
		err := db.Where("generation_record_id = ?", created.GenerationID).First(&videoRecord).Error
		return err == nil && videoRecord.Status == GenerationStatusSucceeded
	})

	if videoRecord.UserID != user.ID || videoRecord.Source != VideoGenerationSourceWorkspace {
		t.Fatalf("unexpected user/source in video record: %+v", videoRecord)
	}
	if videoRecord.DurationSeconds != 10 || videoRecord.AspectRatio != "16:9" || videoRecord.InputImageCount != 0 || videoRecord.ReferenceAssetCount != 0 {
		t.Fatalf("unexpected video parameters: %+v", videoRecord)
	}
	if videoRecord.ProviderRequestID != "provider-poll-123" || videoRecord.WorkID == nil || videoRecord.PreviewURL == "" || videoRecord.DownloadURL == "" || videoRecord.MIMEType != "video/mp4" {
		t.Fatalf("unexpected provider/result fields: %+v", videoRecord)
	}
	if !videoRecord.CreditsDeducted || videoRecord.CreditsCost <= 0 || videoRecord.LatencyMS <= 0 {
		t.Fatalf("unexpected billing/latency fields: %+v", videoRecord)
	}
}

func TestNovelVideoRenderApprovedShotsWritesNovelShotVideoRecord(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	testApp.videoProvider = &stubVideoProvider{
		submitResult: VideoSubmitResult{TaskID: "novel-video-task-1"},
		pollResults: []VideoTaskResult{
			{TaskID: "novel-video-task-1", Status: VideoTaskSucceeded, OutputBase64: base64.StdEncoding.EncodeToString([]byte("novel-video-bytes")), MIMEType: "video/mp4", ProviderRequestID: "novel-provider-1"},
		},
	}

	user, cookies := createLoggedInUser(t, testApp, "novel_video_record_user", "test-password")
	setUserCredits(t, testApp, user.ID, 20)
	project := NovelVideoProject{UserID: user.ID, Title: "Novel Project", SourceText: "story", AspectRatio: "9:16", Duration: "6", VideoModel: "sora-2", Status: NovelVideoProjectStatusPlanned}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	episode := NovelVideoEpisode{ProjectID: project.ID, UserID: user.ID, Number: 1, Title: "Episode 1", Status: NovelVideoReviewStatusApproved}
	if err := db.Create(&episode).Error; err != nil {
		t.Fatalf("create episode: %v", err)
	}
	shot := NovelVideoShot{ProjectID: project.ID, EpisodeID: episode.ID, UserID: user.ID, Number: 1, Title: "Shot 1", Prompt: "camera move", Status: NovelVideoReviewStatusApproved}
	if err := db.Create(&shot).Error; err != nil {
		t.Fatalf("create shot: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects/"+itoa(project.ID)+"/render-approved-shots", map[string]any{}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected render 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var videoRecord VideoGenerationRecord
	waitFor(t, 3*time.Second, func() bool {
		err := db.Where("novel_video_shot_id = ?", shot.ID).First(&videoRecord).Error
		return err == nil && videoRecord.Status == GenerationStatusSucceeded
	})
	if videoRecord.Source != VideoGenerationSourceNovelShot || videoRecord.NovelVideoProjectID == nil || *videoRecord.NovelVideoProjectID != project.ID || videoRecord.NovelVideoEpisodeID == nil || *videoRecord.NovelVideoEpisodeID != episode.ID {
		t.Fatalf("unexpected novel video linkage: %+v", videoRecord)
	}
	if videoRecord.NovelVideoShotID == nil || *videoRecord.NovelVideoShotID != shot.ID || videoRecord.NovelVideoAttemptID == nil {
		t.Fatalf("expected shot and attempt linkage: %+v", videoRecord)
	}
}

func TestAdminVideoGenerationsListDetailExportAndMenu(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)
	user, _ := createLoggedInUser(t, testApp, "video_admin_user", "test-password")
	model := ModelConfig{Name: "Grok Imagine", Type: ModelConfigTypeVideo, Provider: "Wuyin", RuntimeModel: "grok-imagine-video-1.5-preview", Status: ModelConfigStatusOnline}
	if err := db.Create(&model).Error; err != nil {
		t.Fatalf("create model: %v", err)
	}
	generation := seedAdminGenerationRecord(t, testApp, GenerationRecord{
		UserID:             user.ID,
		Prompt:             "admin searchable video",
		AspectRatio:        "16:9",
		StylePreset:        "6s",
		ToolMode:           "video",
		ModelConfigID:      model.ID,
		ModelName:          model.Name,
		RuntimeModel:       model.RuntimeModel,
		Model:              model.RuntimeModel,
		Status:             GenerationStatusSucceeded,
		Stage:              GenerationStageSucceeded,
		PreviewURL:         "https://cdn.example.com/video.mp4",
		DownloadURL:        "https://cdn.example.com/video.mp4",
		MIMEType:           "video/mp4",
		ProviderRequestID:  "provider-admin-1",
		CreditsCost:        5,
		CreditsDeducted:    true,
		ProviderHTTPStatus: http.StatusOK,
		CreatedAt:          time.Now().Add(-time.Hour),
	})
	work := Work{UserID: user.ID, GenerationRecordID: generation.ID, Prompt: generation.Prompt, AspectRatio: generation.AspectRatio, Category: WorkCategoryVideo, Model: generation.Model, Status: GenerationStatusSucceeded, PreviewURL: generation.PreviewURL, DownloadURL: generation.DownloadURL, MIMEType: "video/mp4", ProviderRequestID: generation.ProviderRequestID}
	if err := db.Create(&work).Error; err != nil {
		t.Fatalf("create work: %v", err)
	}
	generation.WorkID = &work.ID
	if err := db.Save(&generation).Error; err != nil {
		t.Fatalf("link generation work: %v", err)
	}
	videoRecord := VideoGenerationRecord{
		GenerationRecordID:   generation.ID,
		UserID:               user.ID,
		WorkID:               &work.ID,
		Source:               VideoGenerationSourceWorkspace,
		Prompt:               generation.Prompt,
		AspectRatio:          "16:9",
		DurationSeconds:      6,
		ModelConfigID:        model.ID,
		ModelName:            model.Name,
		RuntimeModel:         model.RuntimeModel,
		Provider:             model.Provider,
		ProviderRequestID:    generation.ProviderRequestID,
		Status:               GenerationStatusSucceeded,
		Stage:                GenerationStageSucceeded,
		LatencyMS:            1200,
		CreditsCost:          5,
		CreditsDeducted:      true,
		PreviewURL:           generation.PreviewURL,
		DownloadURL:          generation.DownloadURL,
		MIMEType:             "video/mp4",
		ProviderHTTPStatus:   generation.ProviderHTTPStatus,
		ProviderFailureStage: generation.ProviderFailureStage,
		CreatedAt:            generation.CreatedAt,
	}
	if err := db.Create(&videoRecord).Error; err != nil {
		t.Fatalf("create video generation record: %v", err)
	}

	meResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/me", nil, adminCookies)
	if meResp.Code != http.StatusOK {
		t.Fatalf("expected admin me 200, got %d: %s", meResp.Code, meResp.Body.String())
	}
	if !strings.Contains(meResp.Body.String(), `"/admin/generations"`) || !strings.Contains(meResp.Body.String(), `"/admin/video-generations"`) || strings.Index(meResp.Body.String(), `"/admin/generations"`) > strings.Index(meResp.Body.String(), `"/admin/video-generations"`) {
		t.Fatalf("expected video menu after generation menu, got %s", meResp.Body.String())
	}

	listResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/video-generations?q=searchable&source=workspace&provider=Wuyin&status=succeeded&page=1&page_size=10", nil, adminCookies)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d: %s", listResp.Code, listResp.Body.String())
	}
	var listPayload struct {
		Items []struct {
			ID                 uint   `json:"id"`
			GenerationRecordID uint   `json:"generation_record_id"`
			Source             string `json:"source"`
			ProviderRequestID  string `json:"provider_request_id"`
			DurationSeconds    int    `json:"duration_seconds"`
			MIMEType           string `json:"mime_type"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if listPayload.Total != 1 || len(listPayload.Items) != 1 || listPayload.Items[0].GenerationRecordID != generation.ID || listPayload.Items[0].ProviderRequestID != "provider-admin-1" || listPayload.Items[0].DurationSeconds != 6 || listPayload.Items[0].MIMEType != "video/mp4" {
		t.Fatalf("unexpected list payload: %+v", listPayload)
	}

	detailResp := performJSONRequest(t, testApp, http.MethodGet, fmt.Sprintf("/api/admin/video-generations/%d", videoRecord.ID), nil, adminCookies)
	if detailResp.Code != http.StatusOK {
		t.Fatalf("expected detail 200, got %d: %s", detailResp.Code, detailResp.Body.String())
	}
	if !strings.Contains(detailResp.Body.String(), `"provider_request_id":"provider-admin-1"`) || !strings.Contains(detailResp.Body.String(), `"result_video"`) {
		t.Fatalf("unexpected detail payload: %s", detailResp.Body.String())
	}

	exportResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/video-generations/export?q=searchable", nil, adminCookies)
	if exportResp.Code != http.StatusOK {
		t.Fatalf("expected export 200, got %d: %s", exportResp.Code, exportResp.Body.String())
	}
	if !strings.Contains(exportResp.Body.String(), "provider-admin-1") || !strings.Contains(exportResp.Body.String(), "admin searchable video") {
		t.Fatalf("unexpected export payload: %s", exportResp.Body.String())
	}
}

func TestUserVideoGenerationHistoryListsOwnRecordsWithEnhancementTags(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "video_history_owner", "test-password")
	otherUser, _ := createLoggedInUser(t, testApp, "video_history_other", "test-password")

	reference := seedReferenceAsset(t, testApp, user.ID, "history-ref.png", "image/png", []byte("ref-image"))
	ownerGeneration := seedAdminGenerationRecord(t, testApp, GenerationRecord{
		UserID:          user.ID,
		Prompt:          "owner searchable launch video",
		AspectRatio:     "16:9",
		Quality:         GenerationQualityHigh,
		ToolMode:        "video",
		StylePreset:     "Brand Film",
		ModelName:       "Sora2 Pro",
		RuntimeModel:    "sora-2-pro",
		Model:           "sora-2-pro",
		Status:          GenerationStatusSucceeded,
		Stage:           GenerationStageSucceeded,
		PreviewURL:      "/api/works/901/file",
		DownloadURL:     "/api/works/901/download",
		MIMEType:        "video/mp4",
		CreditsCost:     8,
		CreditsDeducted: true,
		CreatedAt:       time.Now().Add(-10 * time.Minute),
	})
	if err := db.Create(&GenerationReferenceAsset{
		GenerationRecordID: ownerGeneration.ID,
		ReferenceAssetID:   reference.ID,
		SortOrder:          0,
	}).Error; err != nil {
		t.Fatalf("seed generation reference: %v", err)
	}
	work := Work{
		UserID:             user.ID,
		GenerationRecordID: ownerGeneration.ID,
		Prompt:             ownerGeneration.Prompt,
		AspectRatio:        ownerGeneration.AspectRatio,
		Category:           WorkCategoryVideo,
		Model:              ownerGeneration.Model,
		Status:             GenerationStatusSucceeded,
		PreviewURL:         ownerGeneration.PreviewURL,
		DownloadURL:        ownerGeneration.DownloadURL,
		MIMEType:           "video/mp4",
	}
	if err := db.Create(&work).Error; err != nil {
		t.Fatalf("seed work: %v", err)
	}
	ownerGeneration.WorkID = &work.ID
	if err := db.Save(&ownerGeneration).Error; err != nil {
		t.Fatalf("link work: %v", err)
	}
	ownerRecord := VideoGenerationRecord{
		GenerationRecordID:  ownerGeneration.ID,
		UserID:              user.ID,
		WorkID:              &work.ID,
		Source:              VideoGenerationSourceWorkspace,
		Prompt:              ownerGeneration.Prompt,
		AspectRatio:         ownerGeneration.AspectRatio,
		StylePreset:         ownerGeneration.StylePreset,
		DurationSeconds:     10,
		InputImageCount:     1,
		ReferenceAssetCount: 1,
		ModelName:           ownerGeneration.ModelName,
		RuntimeModel:        ownerGeneration.RuntimeModel,
		Provider:            "GPT-Best",
		Status:              GenerationStatusSucceeded,
		Stage:               GenerationStageSucceeded,
		CreditsCost:         ownerGeneration.CreditsCost,
		CreditsDeducted:     true,
		PreviewURL:          ownerGeneration.PreviewURL,
		DownloadURL:         ownerGeneration.DownloadURL,
		MIMEType:            "video/mp4",
		CreatedAt:           ownerGeneration.CreatedAt,
		UpdatedAt:           ownerGeneration.UpdatedAt,
	}
	if err := db.Create(&ownerRecord).Error; err != nil {
		t.Fatalf("seed owner video record: %v", err)
	}
	otherGeneration := seedAdminGenerationRecord(t, testApp, GenerationRecord{
		UserID:      otherUser.ID,
		Prompt:      "other user private video",
		AspectRatio: "9:16",
		ToolMode:    "video",
		Model:       "sora-2",
		Status:      GenerationStatusSucceeded,
		Stage:       GenerationStageSucceeded,
		PreviewURL:  "/api/works/902/file",
		DownloadURL: "/api/works/902/download",
		MIMEType:    "video/mp4",
		CreditsCost: 5,
		CreatedAt:   time.Now().Add(-5 * time.Minute),
	})
	if err := db.Create(&VideoGenerationRecord{
		GenerationRecordID: otherGeneration.ID,
		UserID:             otherUser.ID,
		Source:             VideoGenerationSourceWorkspace,
		Prompt:             otherGeneration.Prompt,
		AspectRatio:        otherGeneration.AspectRatio,
		DurationSeconds:    6,
		RuntimeModel:       otherGeneration.Model,
		Status:             GenerationStatusSucceeded,
		Stage:              GenerationStageSucceeded,
		PreviewURL:         otherGeneration.PreviewURL,
		DownloadURL:        otherGeneration.DownloadURL,
		MIMEType:           "video/mp4",
		CreatedAt:          otherGeneration.CreatedAt,
	}).Error; err != nil {
		t.Fatalf("seed other video record: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/videos/generations?q=searchable&enhancement=%E9%AB%98%E6%B8%85&page=1&page_size=10", nil, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected user video history 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Items []struct {
			ID                  uint     `json:"id"`
			GenerationID        uint     `json:"generation_id"`
			WorkID              *uint    `json:"work_id"`
			Prompt              string   `json:"prompt"`
			PromptSummary       string   `json:"prompt_summary"`
			Status              string   `json:"status"`
			PreviewURL          string   `json:"preview_url"`
			DownloadURL         string   `json:"download_url"`
			AspectRatio         string   `json:"aspect_ratio"`
			DurationSeconds     int      `json:"duration_seconds"`
			RuntimeModel        string   `json:"runtime_model"`
			CreditsCost         int      `json:"credits_cost"`
			ReferenceAssetIDs   []uint   `json:"reference_asset_ids"`
			ReferenceAssetCount int      `json:"reference_asset_count"`
			HD                  bool     `json:"hd"`
			EnhancementTags     []string `json:"enhancement_tags"`
		} `json:"items"`
		Page     int   `json:"page"`
		PageSize int   `json:"page_size"`
		Total    int64 `json:"total"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode user video history: %v", err)
	}
	if payload.Total != 1 || len(payload.Items) != 1 || payload.Page != 1 || payload.PageSize != 10 {
		t.Fatalf("expected only owner filtered record, got %+v", payload)
	}
	item := payload.Items[0]
	if item.GenerationID != ownerGeneration.ID || item.WorkID == nil || *item.WorkID != work.ID || item.Prompt != ownerGeneration.Prompt || item.PromptSummary == "" {
		t.Fatalf("unexpected owner history item identity: %+v", item)
	}
	if item.Status != GenerationStatusSucceeded || item.PreviewURL != ownerGeneration.PreviewURL || item.DownloadURL != ownerGeneration.DownloadURL || item.AspectRatio != "16:9" || item.DurationSeconds != 10 || item.RuntimeModel != "sora-2-pro" || item.CreditsCost != 8 {
		t.Fatalf("unexpected owner history item fields: %+v", item)
	}
	if !item.HD || item.ReferenceAssetCount != 1 || len(item.ReferenceAssetIDs) != 1 || item.ReferenceAssetIDs[0] != reference.ID {
		t.Fatalf("expected hd and reference metadata, got %+v", item)
	}
	for _, expected := range []string{"高清", "参考图", "风格模板", "Pro"} {
		if !containsVideoHistoryTag(item.EnhancementTags, expected) {
			t.Fatalf("expected enhancement tag %q in %+v", expected, item.EnhancementTags)
		}
	}
	if strings.Contains(resp.Body.String(), "other user private video") {
		t.Fatalf("history leaked another user's record: %s", resp.Body.String())
	}
}

func containsVideoHistoryTag(tags []string, expected string) bool {
	for _, tag := range tags {
		if tag == expected {
			return true
		}
	}
	return false
}

func TestBackfillVideoGenerationRecordsCreatesProjectionAndIsIdempotent(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, _ := createLoggedInUser(t, testApp, "video_backfill_user", "test-password")
	generation := seedAdminGenerationRecord(t, testApp, GenerationRecord{
		UserID:            user.ID,
		Prompt:            "legacy video",
		AspectRatio:       "9:16",
		StylePreset:       "15s",
		ToolMode:          "video",
		Model:             "sora-2",
		Status:            GenerationStatusSucceeded,
		Stage:             GenerationStageSucceeded,
		ProviderRequestID: "legacy-provider-1",
		MIMEType:          "video/mp4",
		PreviewURL:        "/api/works/1/file",
		DownloadURL:       "/api/works/1/download",
		CreditsCost:       5,
		CreditsDeducted:   true,
		CreatedAt:         time.Now().Add(-2 * time.Hour),
	})
	work := Work{UserID: user.ID, GenerationRecordID: generation.ID, Prompt: generation.Prompt, AspectRatio: generation.AspectRatio, Category: WorkCategoryVideo, Model: generation.Model, Status: GenerationStatusSucceeded, PreviewURL: generation.PreviewURL, DownloadURL: generation.DownloadURL, MIMEType: "video/mp4", ProviderRequestID: generation.ProviderRequestID}
	if err := db.Create(&work).Error; err != nil {
		t.Fatalf("create work: %v", err)
	}
	generation.WorkID = &work.ID
	if err := db.Save(&generation).Error; err != nil {
		t.Fatalf("link work: %v", err)
	}

	if err := testApp.backfillVideoGenerationRecords(); err != nil {
		t.Fatalf("backfill video records: %v", err)
	}
	if err := testApp.backfillVideoGenerationRecords(); err != nil {
		t.Fatalf("second backfill video records: %v", err)
	}

	var count int64
	if err := db.Model(&VideoGenerationRecord{}).Where("generation_record_id = ?", generation.ID).Count(&count).Error; err != nil {
		t.Fatalf("count video records: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected idempotent single projection, got %d", count)
	}
	var videoRecord VideoGenerationRecord
	if err := db.Where("generation_record_id = ?", generation.ID).First(&videoRecord).Error; err != nil {
		t.Fatalf("load video record: %v", err)
	}
	if videoRecord.DurationSeconds != 15 || videoRecord.Source != VideoGenerationSourceWorkspace || videoRecord.WorkID == nil || *videoRecord.WorkID != work.ID {
		t.Fatalf("unexpected backfilled projection: %+v", videoRecord)
	}
}
