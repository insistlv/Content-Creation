package app

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	videoSoundtrackSourceAI     = "ai"
	videoSoundtrackSourceUpload = "upload"
	soundtrackGenerationCredits = 1
	maxSoundtrackUploadBytes    = 50 * 1024 * 1024
)

type videoSoundtrackGenerateRequest struct {
	Variation string `json:"variation"`
}

type videoSoundtrackPayload struct {
	ID                uint      `json:"id"`
	VideoWorkID       uint      `json:"video_work_id"`
	AudioWorkID       uint      `json:"audio_work_id"`
	Source            string    `json:"source"`
	ProviderRequestID string    `json:"provider_request_id"`
	AudioURL          string    `json:"audio_url"`
	DownloadURL       string    `json:"download_url"`
	MIMEType          string    `json:"mime_type"`
	Title             string    `json:"title"`
	CreatedAt         time.Time `json:"created_at"`
}

func (a *App) handleGenerateVideoSoundtrack(c *gin.Context) {
	user := currentUser(c)
	video, ok := a.findOwnedSucceededVideoWork(c, user.ID)
	if !ok {
		return
	}
	var req videoSoundtrackGenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	variation := normalizeSoundtrackVariation(req.Variation)

	balance, err := a.lookupBalance(user.ID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "balance_load_failed", "账户读取失败")
		return
	}
	if balance.AvailableCredits < soundtrackGenerationCredits {
		writeError(c, http.StatusConflict, "credits_insufficient", "点数不足，请先充值")
		return
	}

	settings, err := a.loadSettings()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "settings_load_failed", "配置读取失败")
		return
	}
	candidate, err := a.defaultMusicModelCandidate(settings)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_load_failed", "模型中心配置读取失败")
		return
	}
	if candidate == nil {
		writeError(c, http.StatusInternalServerError, "model_center_load_failed", "未配置音频模型")
		return
	}

	input, err := a.buildMusicProviderInput(video, *candidate, variation)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "soundtrack_video_read_failed", "视频文件读取失败")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(settings.RequestTimeoutSeconds)*time.Second)
	defer cancel()
	result, providerErr := a.musicProvider.GenerateMusic(ctx, input)
	if providerErr != nil {
		if strings.TrimSpace(providerErr.Code) == "provider_empty_audio" {
			writeError(c, http.StatusBadGateway, "soundtrack_empty_audio", "智能配乐结果为空")
			return
		}
		writeError(c, http.StatusBadGateway, "soundtrack_provider_failed", "智能配乐服务暂时不可用，请稍后重试")
		return
	}
	audioBase64 := strings.TrimSpace(result.AudioBase64)
	if audioBase64 == "" {
		writeError(c, http.StatusBadGateway, "soundtrack_empty_audio", "智能配乐结果为空")
		return
	}
	audioBytes, err := base64.StdEncoding.DecodeString(audioBase64)
	if err != nil || len(audioBytes) == 0 {
		writeError(c, http.StatusBadGateway, "soundtrack_empty_audio", "智能配乐结果为空")
		return
	}

	assetKey, mimeType, err := a.assetStore.SaveBytes(audioBytes, fallbackString(result.MIMEType, "audio/mpeg"))
	if err != nil {
		writeError(c, http.StatusInternalServerError, "asset_store_failed", "音乐保存失败")
		return
	}

	payload, err := a.persistVideoSoundtrack(user.ID, video, assetKey, mimeType, videoSoundtrackSourceAI, fallbackString(result.ProviderRequestID, providerErrRequestID(providerErr)), soundtrackGenerationCredits)
	if err != nil {
		if errors.Is(err, errCreditsInsufficient) {
			writeError(c, http.StatusConflict, "credits_insufficient", "点数不足，请先充值")
			return
		}
		writeError(c, http.StatusInternalServerError, "soundtrack_persist_failed", "音乐保存失败")
		return
	}
	writeJSON(c, http.StatusOK, payload)
}

func (a *App) handleUploadVideoSoundtrack(c *gin.Context) {
	user := currentUser(c)
	video, ok := a.findOwnedSucceededVideoWork(c, user.ID)
	if !ok {
		return
	}
	file, err := c.FormFile("file")
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请选择音乐文件")
		return
	}
	if file.Size > maxSoundtrackUploadBytes {
		writeError(c, http.StatusRequestEntityTooLarge, "soundtrack_upload_too_large", "音乐文件不能超过 50MB")
		return
	}
	mimeType := soundtrackUploadMimeType(file.Header.Get("Content-Type"), file.Filename)
	if !validSoundtrackMimeType(mimeType) {
		writeError(c, http.StatusBadRequest, "soundtrack_upload_invalid_type", "不支持的音乐格式")
		return
	}
	opened, err := file.Open()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "soundtrack_upload_failed", "音乐读取失败")
		return
	}
	defer opened.Close()
	content, err := io.ReadAll(io.LimitReader(opened, maxSoundtrackUploadBytes+1))
	if err != nil {
		writeError(c, http.StatusInternalServerError, "soundtrack_upload_failed", "音乐读取失败")
		return
	}
	if int64(len(content)) > maxSoundtrackUploadBytes {
		writeError(c, http.StatusRequestEntityTooLarge, "soundtrack_upload_too_large", "音乐文件不能超过 50MB")
		return
	}
	if len(content) == 0 {
		writeError(c, http.StatusBadRequest, "soundtrack_empty_audio", "音乐文件为空")
		return
	}
	assetKey, normalizedMIME, err := a.assetStore.SaveBytes(content, mimeType)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "asset_store_failed", "音乐保存失败")
		return
	}
	payload, err := a.persistVideoSoundtrack(user.ID, video, assetKey, normalizedMIME, videoSoundtrackSourceUpload, "", 0)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "soundtrack_persist_failed", "音乐保存失败")
		return
	}
	writeJSON(c, http.StatusOK, payload)
}

func (a *App) handleListVideoSoundtracks(c *gin.Context) {
	user := currentUser(c)
	video, ok := a.findOwnedSucceededVideoWork(c, user.ID)
	if !ok {
		return
	}
	payloads, err := a.videoSoundtrackPayloads(video.ID, user.ID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "soundtracks_load_failed", "配乐读取失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{"items": payloads})
}

func (a *App) findOwnedSucceededVideoWork(c *gin.Context, userID uint) (Work, bool) {
	var work Work
	result := a.db.Where("id = ? AND user_id = ?", c.Param("work_id"), userID).Limit(1).Find(&work)
	if result.Error != nil {
		writeError(c, http.StatusInternalServerError, "work_load_failed", "作品读取失败")
		return Work{}, false
	}
	if result.RowsAffected == 0 {
		writeError(c, http.StatusNotFound, "work_not_found", "作品不存在")
		return Work{}, false
	}
	if normalizeWorkCategory(work.Category) != WorkCategoryVideo || work.Status != GenerationStatusSucceeded {
		writeError(c, http.StatusBadRequest, "soundtrack_video_invalid", "只能为已完成视频添加配乐")
		return Work{}, false
	}
	return work, true
}

func (a *App) defaultMusicModelCandidate(settings AppSettings) (*modelCenterCandidate, error) {
	candidates, err := a.modelCenterCandidatesForGeneration(settings, ModelConfigTypeAudio, 0)
	if err != nil {
		return nil, err
	}
	if len(candidates) > 0 {
		return &candidates[0], nil
	}
	var configuredChannelCount int64
	if err := a.db.Model(&ModelChannel{}).
		Joins("JOIN model_catalogs ON model_catalogs.id = model_channels.model_id").
		Where("model_catalogs.modality = ?", ModelConfigTypeAudio).
		Count(&configuredChannelCount).Error; err != nil {
		return nil, err
	}
	if configuredChannelCount > 0 {
		return nil, nil
	}
	return &modelCenterCandidate{
		Model: ModelCatalog{
			Name:               "智能配乐",
			Modality:           ModelConfigTypeAudio,
			Status:             ModelCenterStatusOnline,
			Visibility:         ModelCenterVisibilityPublic,
			DefaultCreditsCost: soundtrackGenerationCredits,
		},
		Channel: ModelChannel{
			Name:         "默认音频通道",
			RuntimeModel: "music-for-video",
			Endpoint:     "/v1/audio/soundtracks",
			Status:       ModelCenterStatusOnline,
			HealthStatus: ModelChannelHealthHealthy,
		},
	}, nil
}

func (a *App) buildMusicProviderInput(video Work, candidate modelCenterCandidate, variation string) (MusicGenerationInput, error) {
	videoURL := strings.TrimSpace(a.assetStore.PublicURL(video.AssetKey))
	videoBase64 := ""
	if videoURL == "" {
		content, err := a.assetStore.Read(video.AssetKey)
		if err != nil {
			return MusicGenerationInput{}, err
		}
		videoBase64 = base64.StdEncoding.EncodeToString(content)
	}
	return MusicGenerationInput{
		Model:               fallbackString(modelCenterRuntimeModel(&candidate), "music-for-video"),
		VideoURL:            videoURL,
		VideoBase64:         videoBase64,
		VideoMIMEType:       fallbackString(video.MIMEType, "video/mp4"),
		Prompt:              video.Prompt,
		Duration:            a.soundtrackVideoDuration(video),
		AspectRatio:         video.AspectRatio,
		Variation:           variation,
		ProviderBaseURL:     modelCenterProviderBaseURL(&candidate),
		ProviderAPIKey:      modelCenterProviderAPIKey(&candidate),
		ProviderAPIEndpoint: modelCenterProviderEndpoint(&candidate),
	}, nil
}

func (a *App) persistVideoSoundtrack(userID uint, video Work, assetKey, mimeType, source, providerRequestID string, creditsCost int) (videoSoundtrackPayload, error) {
	var soundtrack VideoSoundtrack
	var audio Work
	err := a.db.Transaction(func(tx *gorm.DB) error {
		audio = Work{
			UserID:            userID,
			Prompt:            video.Prompt,
			AspectRatio:       video.AspectRatio,
			Category:          WorkCategoryAudio,
			Model:             "music-for-video",
			Status:            GenerationStatusSucceeded,
			Visibility:        WorkVisibilityPrivate,
			AssetKey:          assetKey,
			MIMEType:          normalizeAssetMimeType(mimeType),
			ProviderRequestID: providerRequestID,
		}
		if err := tx.Create(&audio).Error; err != nil {
			return err
		}
		if publicURL := a.assetStore.PublicURL(assetKey); publicURL != "" {
			audio.PreviewURL = publicURL
			audio.DownloadURL = publicURL
		} else {
			audio.PreviewURL = fmt.Sprintf("/api/works/%d/file", audio.ID)
			audio.DownloadURL = fmt.Sprintf("/api/works/%d/download", audio.ID)
		}
		if err := tx.Save(&audio).Error; err != nil {
			return err
		}
		soundtrack = VideoSoundtrack{
			UserID:            userID,
			VideoWorkID:       video.ID,
			AudioWorkID:       audio.ID,
			Source:            source,
			ProviderRequestID: providerRequestID,
		}
		if err := tx.Create(&soundtrack).Error; err != nil {
			return err
		}
		if creditsCost > 0 {
			remainingCredits, err := deductGenerationCredits(tx, userID, creditsCost)
			if err != nil {
				return err
			}
			if err := tx.Create(&CreditTransaction{
				UserID:       userID,
				Type:         CreditTransactionTypeGenerationCharge,
				Amount:       -creditsCost,
				BalanceAfter: remainingCredits,
				Reason:       "视频智能配乐扣点",
				RelatedType:  "video_soundtrack",
				RelatedID:    soundtrack.ID,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return videoSoundtrackPayload{}, err
	}
	return a.videoSoundtrackPayload(soundtrack, audio), nil
}

func (a *App) videoSoundtrackPayloads(videoWorkID, userID uint) ([]videoSoundtrackPayload, error) {
	var soundtracks []VideoSoundtrack
	if err := a.db.Where("video_work_id = ? AND user_id = ?", videoWorkID, userID).Order("created_at desc, id desc").Find(&soundtracks).Error; err != nil {
		return nil, err
	}
	if len(soundtracks) == 0 {
		return []videoSoundtrackPayload{}, nil
	}
	audioIDs := make([]uint, 0, len(soundtracks))
	for _, soundtrack := range soundtracks {
		audioIDs = append(audioIDs, soundtrack.AudioWorkID)
	}
	var works []Work
	if err := a.db.Where("id IN ? AND user_id = ?", audioIDs, userID).Find(&works).Error; err != nil {
		return nil, err
	}
	byID := make(map[uint]Work, len(works))
	for _, work := range works {
		a.applyWorkPublicURL(&work)
		byID[work.ID] = work
	}
	items := make([]videoSoundtrackPayload, 0, len(soundtracks))
	for _, soundtrack := range soundtracks {
		audio, ok := byID[soundtrack.AudioWorkID]
		if !ok {
			continue
		}
		items = append(items, a.videoSoundtrackPayload(soundtrack, audio))
	}
	return items, nil
}

func (a *App) videoSoundtrackPayload(soundtrack VideoSoundtrack, audio Work) videoSoundtrackPayload {
	a.applyWorkPublicURL(&audio)
	return videoSoundtrackPayload{
		ID:                soundtrack.ID,
		VideoWorkID:       soundtrack.VideoWorkID,
		AudioWorkID:       soundtrack.AudioWorkID,
		Source:            soundtrack.Source,
		ProviderRequestID: soundtrack.ProviderRequestID,
		AudioURL:          audio.PreviewURL,
		DownloadURL:       audio.DownloadURL,
		MIMEType:          normalizeAssetMimeType(audio.MIMEType),
		Title:             soundtrackTitle(soundtrack.Source),
		CreatedAt:         soundtrack.CreatedAt,
	}
}

func normalizeSoundtrackVariation(value string) string {
	if strings.TrimSpace(value) == "replace" {
		return "replace"
	}
	return "smart"
}

func (a *App) soundtrackVideoDuration(video Work) string {
	var record GenerationRecord
	if video.GenerationRecordID != 0 {
		if err := a.db.First(&record, video.GenerationRecordID).Error; err == nil {
			if value := strings.TrimSuffix(strings.TrimSpace(record.StylePreset), "s"); value != "" {
				return value
			}
		}
	}
	return "10"
}

func soundtrackTitle(source string) string {
	if source == videoSoundtrackSourceUpload {
		return "上传音乐"
	}
	return "智能配乐"
}

func soundtrackUploadMimeType(contentType, filename string) string {
	contentType = strings.TrimSpace(strings.Split(contentType, ";")[0])
	if validSoundtrackMimeType(contentType) {
		return normalizeAssetMimeType(contentType)
	}
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".m4a", ".mp4":
		return "audio/mp4"
	case ".aac":
		return "audio/aac"
	case ".ogg":
		return "audio/ogg"
	default:
		return contentType
	}
}

func validSoundtrackMimeType(mimeType string) bool {
	switch normalizeAssetMimeType(mimeType) {
	case "audio/mpeg", "audio/wav", "audio/mp4", "audio/aac", "audio/ogg":
		return true
	default:
		return false
	}
}

func providerErrRequestID(err *ProviderError) string {
	if err == nil {
		return ""
	}
	return err.ProviderRequestID
}
