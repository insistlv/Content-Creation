package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

type stubMusicProvider struct {
	results []MusicGenerationResult
	err     *ProviderError
	inputs  []MusicGenerationInput
}

func (s *stubMusicProvider) GenerateMusic(ctx context.Context, input MusicGenerationInput) (MusicGenerationResult, *ProviderError) {
	s.inputs = append(s.inputs, input)
	if s.err != nil {
		return MusicGenerationResult{}, s.err
	}
	if len(s.results) > 0 {
		result := s.results[0]
		s.results = s.results[1:]
		return result, nil
	}
	return MusicGenerationResult{
		AudioBase64:       base64.StdEncoding.EncodeToString([]byte("music-bytes")),
		MIMEType:          "audio/mpeg",
		ProviderRequestID: "music-req-1",
	}, nil
}

func seedSucceededVideoWork(t *testing.T, app *App, userID uint, prompt, aspectRatio, duration string) Work {
	t.Helper()
	assetKey, _, err := app.assetStore.SaveBytes([]byte("video-bytes"), "video/mp4")
	if err != nil {
		t.Fatalf("save fixture video bytes: %v", err)
	}
	record := GenerationRecord{
		UserID:            userID,
		Prompt:            prompt,
		AspectRatio:       aspectRatio,
		Model:             "sora-2",
		Status:            GenerationStatusSucceeded,
		Stage:             GenerationStageSucceeded,
		ToolMode:          "video",
		StylePreset:       duration + "s",
		MIMEType:          "video/mp4",
		AssetKey:          assetKey,
		CreditsDeducted:   true,
		ProviderRequestID: "video-req-1",
	}
	if err := app.db.Create(&record).Error; err != nil {
		t.Fatalf("seed video record: %v", err)
	}
	work := Work{
		UserID:             userID,
		GenerationRecordID: record.ID,
		Prompt:             prompt,
		AspectRatio:        aspectRatio,
		Category:           WorkCategoryVideo,
		Model:              record.Model,
		Status:             GenerationStatusSucceeded,
		Visibility:         WorkVisibilityPrivate,
		AssetKey:           assetKey,
		MIMEType:           "video/mp4",
		ProviderRequestID:  "video-req-1",
	}
	if err := app.db.Create(&work).Error; err != nil {
		t.Fatalf("seed video work: %v", err)
	}
	work.PreviewURL = fmt.Sprintf("/api/works/%d/file", work.ID)
	work.DownloadURL = fmt.Sprintf("/api/works/%d/download", work.ID)
	if err := app.db.Save(&work).Error; err != nil {
		t.Fatalf("save video urls: %v", err)
	}
	record.WorkID = &work.ID
	record.PreviewURL = work.PreviewURL
	record.DownloadURL = work.DownloadURL
	if err := app.db.Save(&record).Error; err != nil {
		t.Fatalf("link video record: %v", err)
	}
	return work
}

func TestGenerateVideoSoundtrackCreatesAudioWorkAndChargesOneCredit(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	musicProvider := &stubMusicProvider{}
	testApp.musicProvider = musicProvider
	user, cookies := createLoggedInUser(t, testApp, "video_soundtrack_user", "test-password")
	setUserCredits(t, testApp, user.ID, 3)
	video := seedSucceededVideoWork(t, testApp, user.ID, "发布会快剪", "16:9", "10")

	resp := performJSONRequest(t, testApp, http.MethodPost, fmt.Sprintf("/api/videos/%d/soundtracks/generate", video.ID), map[string]any{
		"variation": "smart",
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected soundtrack generate 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload videoSoundtrackPayload
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode soundtrack payload: %v", err)
	}
	if payload.ID == 0 || payload.VideoWorkID != video.ID || payload.AudioWorkID == 0 || payload.Source != "ai" {
		t.Fatalf("unexpected soundtrack payload: %+v", payload)
	}
	if payload.MIMEType != "audio/mpeg" || payload.AudioURL == "" || payload.DownloadURL == "" {
		t.Fatalf("expected audio payload urls and mime type, got %+v", payload)
	}
	if len(musicProvider.inputs) != 1 {
		t.Fatalf("expected one music provider call, got %d", len(musicProvider.inputs))
	}
	input := musicProvider.inputs[0]
	if input.Prompt != "发布会快剪" || input.AspectRatio != "16:9" || input.Duration != "10" || input.Variation != "smart" {
		t.Fatalf("unexpected music provider input: %+v", input)
	}
	if input.Model == "" {
		t.Fatalf("expected audio model from model center")
	}

	var audio Work
	if err := db.First(&audio, payload.AudioWorkID).Error; err != nil {
		t.Fatalf("load audio work: %v", err)
	}
	if audio.Category != WorkCategoryAudio || audio.MIMEType != "audio/mpeg" || audio.Status != GenerationStatusSucceeded {
		t.Fatalf("expected audio work, got %+v", audio)
	}
	var balance CreditBalance
	if err := db.Where("user_id = ?", user.ID).First(&balance).Error; err != nil {
		t.Fatalf("load balance: %v", err)
	}
	if balance.AvailableCredits != 2 {
		t.Fatalf("expected one credit charged, got %d", balance.AvailableCredits)
	}
}

func TestDefaultMusicModelCandidateFallsBackOnlyWithoutAudioRouting(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	var policies []ModelRoutingPolicy
	if err := db.Where("modality = ?", ModelConfigTypeAudio).Find(&policies).Error; err != nil {
		t.Fatalf("load audio policies: %v", err)
	}
	for _, policy := range policies {
		if err := db.Where("policy_id = ?", policy.ID).Delete(&ModelRoutingEntry{}).Error; err != nil {
			t.Fatalf("delete audio entries: %v", err)
		}
	}
	if err := db.Where("modality = ?", ModelConfigTypeAudio).Delete(&ModelRoutingPolicy{}).Error; err != nil {
		t.Fatalf("delete audio policy: %v", err)
	}
	if err := db.Where("type = ?", ModelConfigTypeAudio).Delete(&ModelConfig{}).Error; err != nil {
		t.Fatalf("delete audio legacy config: %v", err)
	}
	var audioModels []ModelCatalog
	if err := db.Where("modality = ?", ModelConfigTypeAudio).Find(&audioModels).Error; err != nil {
		t.Fatalf("load audio models: %v", err)
	}
	for _, model := range audioModels {
		if err := db.Where("model_id = ?", model.ID).Delete(&ModelChannel{}).Error; err != nil {
			t.Fatalf("delete audio channels: %v", err)
		}
		if err := db.Delete(&model).Error; err != nil {
			t.Fatalf("delete audio model: %v", err)
		}
	}
	var settings AppSettings
	if err := db.First(&settings, 1).Error; err != nil {
		t.Fatalf("load settings: %v", err)
	}
	candidate, err := testApp.defaultMusicModelCandidate(settings)
	if err != nil || candidate == nil {
		t.Fatalf("expected default audio fallback, got %+v err=%v", candidate, err)
	}
	if candidate.Channel.RuntimeModel != "music-for-video" || candidate.Channel.Endpoint != "/v1/audio/soundtracks" {
		t.Fatalf("unexpected default audio fallback: %+v", candidate)
	}

	policy := ModelRoutingPolicy{Modality: ModelConfigTypeAudio, RoutingStrategy: ModelRoutingStrategyDefault, Source: ModelRoutingSourceModelCenter}
	if err := db.Create(&policy).Error; err != nil {
		t.Fatalf("create empty audio policy: %v", err)
	}
	model := ModelCatalog{Name: "已配置音频模型", Modality: ModelConfigTypeAudio, Status: ModelCenterStatusOffline}
	if err := db.Create(&model).Error; err != nil {
		t.Fatalf("create configured audio model: %v", err)
	}
	provider := ModelProvider{Name: "已配置音频供应商", Provider: "configured-audio", Status: ModelCenterStatusOffline}
	if err := db.Create(&provider).Error; err != nil {
		t.Fatalf("create configured audio provider: %v", err)
	}
	channel := ModelChannel{ModelID: model.ID, ProviderID: provider.ID, Name: "已配置音频渠道", Status: ModelCenterStatusOffline}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("create configured audio channel: %v", err)
	}
	candidate, err = testApp.defaultMusicModelCandidate(settings)
	if err != nil || candidate != nil {
		t.Fatalf("expected configured empty audio policy to disable fallback, got %+v err=%v", candidate, err)
	}
}

func TestReplaceVideoSoundtrackCreatesNewBinding(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	testApp.musicProvider = &stubMusicProvider{results: []MusicGenerationResult{
		{AudioBase64: base64.StdEncoding.EncodeToString([]byte("first-music")), MIMEType: "audio/mpeg", ProviderRequestID: "music-1"},
		{AudioBase64: base64.StdEncoding.EncodeToString([]byte("second-music")), MIMEType: "audio/mpeg", ProviderRequestID: "music-2"},
	}}
	user, cookies := createLoggedInUser(t, testApp, "video_soundtrack_replace", "test-password")
	setUserCredits(t, testApp, user.ID, 5)
	video := seedSucceededVideoWork(t, testApp, user.ID, "城市夜景短片", "9:16", "15")

	firstResp := performJSONRequest(t, testApp, http.MethodPost, fmt.Sprintf("/api/videos/%d/soundtracks/generate", video.ID), map[string]any{"variation": "smart"}, cookies)
	if firstResp.Code != http.StatusOK {
		t.Fatalf("expected first soundtrack 200, got %d: %s", firstResp.Code, firstResp.Body.String())
	}
	secondResp := performJSONRequest(t, testApp, http.MethodPost, fmt.Sprintf("/api/videos/%d/soundtracks/generate", video.ID), map[string]any{"variation": "replace"}, cookies)
	if secondResp.Code != http.StatusOK {
		t.Fatalf("expected replace soundtrack 200, got %d: %s", secondResp.Code, secondResp.Body.String())
	}

	var soundtracks []VideoSoundtrack
	if err := db.Where("video_work_id = ?", video.ID).Order("id asc").Find(&soundtracks).Error; err != nil {
		t.Fatalf("load soundtracks: %v", err)
	}
	if len(soundtracks) != 2 || soundtracks[0].AudioWorkID == soundtracks[1].AudioWorkID {
		t.Fatalf("expected two distinct soundtrack bindings, got %+v", soundtracks)
	}
	var balance CreditBalance
	if err := db.Where("user_id = ?", user.ID).First(&balance).Error; err != nil {
		t.Fatalf("load balance: %v", err)
	}
	if balance.AvailableCredits != 3 {
		t.Fatalf("expected two credits charged, got %d", balance.AvailableCredits)
	}
}

func TestGenerateVideoSoundtrackProviderFailureDoesNotCharge(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	testApp.musicProvider = &stubMusicProvider{err: &ProviderError{Code: "provider_down", Message: "provider unavailable"}}
	user, cookies := createLoggedInUser(t, testApp, "video_soundtrack_provider_fail", "test-password")
	setUserCredits(t, testApp, user.ID, 2)
	video := seedSucceededVideoWork(t, testApp, user.ID, "失败不扣点", "16:9", "10")

	resp := performJSONRequest(t, testApp, http.MethodPost, fmt.Sprintf("/api/videos/%d/soundtracks/generate", video.ID), map[string]any{"variation": "smart"}, cookies)
	if resp.Code != http.StatusBadGateway || !bytes.Contains(resp.Body.Bytes(), []byte(`"soundtrack_provider_failed"`)) {
		t.Fatalf("expected provider failure 502, got %d: %s", resp.Code, resp.Body.String())
	}
	if bytes.Contains(resp.Body.Bytes(), []byte("provider unavailable")) || !bytes.Contains(resp.Body.Bytes(), []byte("智能配乐服务暂时不可用")) {
		t.Fatalf("expected localized provider failure without raw details, got %s", resp.Body.String())
	}
	var balance CreditBalance
	if err := db.Where("user_id = ?", user.ID).First(&balance).Error; err != nil {
		t.Fatalf("load balance: %v", err)
	}
	if balance.AvailableCredits != 2 {
		t.Fatalf("provider failure should not charge credits, got %d", balance.AvailableCredits)
	}
	var count int64
	if err := db.Model(&VideoSoundtrack{}).Where("video_work_id = ?", video.ID).Count(&count).Error; err != nil {
		t.Fatalf("count soundtracks: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no soundtrack binding on provider failure, got %d", count)
	}
}

func TestGenerateVideoSoundtrackProviderEmptyAudioUsesEmptyAudioError(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	testApp.musicProvider = &stubMusicProvider{err: &ProviderError{Code: "provider_empty_audio", Message: "provider returned no audio data"}}
	user, cookies := createLoggedInUser(t, testApp, "video_soundtrack_empty_audio", "test-password")
	setUserCredits(t, testApp, user.ID, 2)
	video := seedSucceededVideoWork(t, testApp, user.ID, "空音频不扣点", "16:9", "10")

	resp := performJSONRequest(t, testApp, http.MethodPost, fmt.Sprintf("/api/videos/%d/soundtracks/generate", video.ID), map[string]any{"variation": "smart"}, cookies)
	if resp.Code != http.StatusBadGateway || !bytes.Contains(resp.Body.Bytes(), []byte(`"soundtrack_empty_audio"`)) {
		t.Fatalf("expected empty audio 502, got %d: %s", resp.Code, resp.Body.String())
	}
	var balance CreditBalance
	if err := db.Where("user_id = ?", user.ID).First(&balance).Error; err != nil {
		t.Fatalf("load balance: %v", err)
	}
	if balance.AvailableCredits != 2 {
		t.Fatalf("empty audio should not charge credits, got %d", balance.AvailableCredits)
	}
}

func TestUploadVideoSoundtrackValidatesTypeSizeAndOwnership(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	owner, ownerCookies := createLoggedInUser(t, testApp, "video_soundtrack_upload_owner", "test-password")
	_, otherCookies := createLoggedInUser(t, testApp, "video_soundtrack_upload_other", "test-password")
	setUserCredits(t, testApp, owner.ID, 1)
	video := seedSucceededVideoWork(t, testApp, owner.ID, "上传配乐", "16:9", "10")

	otherResp := performMultipartRequest(t, testApp, http.MethodPost, fmt.Sprintf("/api/videos/%d/soundtracks/upload", video.ID), "file", "song.mp3", []byte("mp3 bytes"), otherCookies)
	if otherResp.Code != http.StatusNotFound {
		t.Fatalf("expected other user 404, got %d: %s", otherResp.Code, otherResp.Body.String())
	}

	invalidResp := performMultipartRequest(t, testApp, http.MethodPost, fmt.Sprintf("/api/videos/%d/soundtracks/upload", video.ID), "file", "note.txt", []byte("not audio"), ownerCookies)
	if invalidResp.Code != http.StatusBadRequest || !strings.Contains(invalidResp.Body.String(), "soundtrack_upload_invalid_type") {
		t.Fatalf("expected invalid upload type, got %d: %s", invalidResp.Code, invalidResp.Body.String())
	}

	tooLarge := bytes.Repeat([]byte("a"), int(maxSoundtrackUploadBytes)+1)
	largeResp := performMultipartRequest(t, testApp, http.MethodPost, fmt.Sprintf("/api/videos/%d/soundtracks/upload", video.ID), "file", "large.mp3", tooLarge, ownerCookies)
	if largeResp.Code != http.StatusRequestEntityTooLarge || !strings.Contains(largeResp.Body.String(), "soundtrack_upload_too_large") {
		t.Fatalf("expected upload too large, got %d: %s", largeResp.Code, largeResp.Body.String())
	}

	uploadResp := performMultipartRequest(t, testApp, http.MethodPost, fmt.Sprintf("/api/videos/%d/soundtracks/upload", video.ID), "file", "song.mp3", []byte("mp3 bytes"), ownerCookies)
	if uploadResp.Code != http.StatusOK {
		t.Fatalf("expected upload 200, got %d: %s", uploadResp.Code, uploadResp.Body.String())
	}
	var payload videoSoundtrackPayload
	if err := json.Unmarshal(uploadResp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode upload payload: %v", err)
	}
	if payload.Source != "upload" || payload.MIMEType != "audio/mpeg" || payload.AudioWorkID == 0 {
		t.Fatalf("unexpected upload payload: %+v", payload)
	}
	var balance CreditBalance
	if err := db.Where("user_id = ?", owner.ID).First(&balance).Error; err != nil {
		t.Fatalf("load balance: %v", err)
	}
	if balance.AvailableCredits != 1 {
		t.Fatalf("upload should not charge credits, got %d", balance.AvailableCredits)
	}
}

func TestListVideoSoundtracksAndServeAudioContentType(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	testApp.musicProvider = &stubMusicProvider{}
	user, cookies := createLoggedInUser(t, testApp, "video_soundtrack_list", "test-password")
	setUserCredits(t, testApp, user.ID, 3)
	video := seedSucceededVideoWork(t, testApp, user.ID, "列出配乐", "16:9", "10")

	generateResp := performJSONRequest(t, testApp, http.MethodPost, fmt.Sprintf("/api/videos/%d/soundtracks/generate", video.ID), map[string]any{"variation": "smart"}, cookies)
	if generateResp.Code != http.StatusOK {
		t.Fatalf("expected soundtrack 200, got %d: %s", generateResp.Code, generateResp.Body.String())
	}
	var generated videoSoundtrackPayload
	if err := json.Unmarshal(generateResp.Body.Bytes(), &generated); err != nil {
		t.Fatalf("decode generated payload: %v", err)
	}

	listResp := performJSONRequest(t, testApp, http.MethodGet, fmt.Sprintf("/api/videos/%d/soundtracks", video.ID), nil, cookies)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d: %s", listResp.Code, listResp.Body.String())
	}
	var listPayload struct {
		Items []videoSoundtrackPayload `json:"items"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode list payload: %v", err)
	}
	if len(listPayload.Items) != 1 || listPayload.Items[0].AudioWorkID != generated.AudioWorkID {
		t.Fatalf("unexpected list payload: %+v", listPayload)
	}

	fileResp := performJSONRequest(t, testApp, http.MethodGet, fmt.Sprintf("/api/works/%d/file", generated.AudioWorkID), nil, cookies)
	if fileResp.Code != http.StatusOK {
		t.Fatalf("expected audio file 200, got %d: %s", fileResp.Code, fileResp.Body.String())
	}
	if contentType := fileResp.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "audio/mpeg") {
		t.Fatalf("expected audio/mpeg content type, got %q", contentType)
	}
}
