package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const novelVideoMaxSourceChars = 50000
const novelVideoMissingSchemaMessage = "小说视频项目数据表未初始化，请联系管理员执行数据库迁移后重试"

type novelVideoProjectRequest struct {
	Title          string                       `json:"title"`
	SourceText     string                       `json:"source_text"`
	ContentMode    string                       `json:"content_mode"`
	GenerationMode string                       `json:"generation_mode"`
	GridSize       int                          `json:"grid_size"`
	StylePreset    string                       `json:"style_preset"`
	AspectRatio    string                       `json:"aspect_ratio"`
	Duration       string                       `json:"duration"`
	ImageModel     string                       `json:"image_model"`
	VideoModel     string                       `json:"video_model"`
	VideoSettings  novelVideoGenerationSettings `json:"video_settings"`
}

type novelVideoGenerationSettings struct {
	Model                  string `json:"model,omitempty"`
	AspectRatio            string `json:"aspect_ratio,omitempty"`
	Duration               string `json:"duration,omitempty"`
	Resolution             string `json:"resolution,omitempty"`
	VideoStylePresetID     uint   `json:"video_style_preset_id,omitempty"`
	CustomVideoStyleID     uint   `json:"custom_video_style_id,omitempty"`
	ReferenceAssetIDs      []uint `json:"reference_asset_ids,omitempty"`
	ReferenceVideoAssetIDs []uint `json:"reference_video_asset_ids,omitempty"`
	ReferenceAudioAssetIDs []uint `json:"reference_audio_asset_ids,omitempty"`
	GenerateAudio          bool   `json:"generate_audio"`
}

type novelVideoRenderPreflightShot struct {
	ShotID            uint                         `json:"shot_id"`
	EpisodeID         uint                         `json:"episode_id"`
	EpisodeNumber     int                          `json:"episode_number"`
	ShotNumber        int                          `json:"shot_number"`
	Number            int                          `json:"number"`
	Title             string                       `json:"title"`
	CanRender         bool                         `json:"can_render"`
	BlockReasons      []string                     `json:"block_reasons"`
	BlockedReason     string                       `json:"blocked_reason,omitempty"`
	RequiredCredits   int                          `json:"required_credits"`
	EffectiveSettings novelVideoGenerationSettings `json:"effective_settings"`
	Request           videoGenerationRequest       `json:"-"`
}

type novelVideoRenderPreflight struct {
	Status           string                          `json:"status"`
	Total            int                             `json:"total"`
	Renderable       int                             `json:"renderable"`
	Blocked          int                             `json:"blocked"`
	Skipped          int                             `json:"skipped"`
	RequiredCredits  int                             `json:"required_credits"`
	AvailableCredits int                             `json:"available_credits"`
	MissingCredits   int                             `json:"missing_credits"`
	Enough           bool                            `json:"enough"`
	Shots            []novelVideoRenderPreflightShot `json:"shots"`
}

type novelVideoShotGenerationPlan struct {
	Request               videoGenerationRequest
	AppSettings           AppSettings
	ModelConfig           *ModelConfig
	ModelCenterCandidates []modelCenterCandidate
	ReferenceAssets       []ReferenceAsset
	ReferenceVideoAssets  []ReferenceAsset
	ReferenceAudioAssets  []ReferenceAsset
}

type novelVideoShotImageGenerationDraft struct {
	Shot                  NovelVideoShot
	Request               generationRequest
	AppSettings           AppSettings
	ModelConfig           *ModelConfig
	ModelCenterCandidates []modelCenterCandidate
	ReferenceAssets       []ReferenceAsset
	SourceWork            *Work
	ActorIDs              []uint
	ReferenceAssetIDs     []uint
	ReferenceIntent       string
	Mode                  string
	LockLevel             string
	Version               int
}

type novelVideoShotImageGenerationTask struct {
	ImageID  uint
	Project  NovelVideoProject
	Record   GenerationRecord
	Job      *generationJob
	SlotWait time.Duration
}

type novelVideoCompositionResult struct {
	OutputURL    string
	SubtitleURL  string
	OutputBytes  []byte
	SubtitleText string
	ManifestJSON string
}

type FFmpegRunner interface {
	ComposeNovelVideo(ctx context.Context, project NovelVideoProject, clips []novelVideoComposeClip, assetStore AssetStore) (novelVideoCompositionResult, error)
}

type executableFFmpegRunner struct{}

type novelVideoComposeClip struct {
	Episode NovelVideoEpisode
	Shot    NovelVideoShot
	Work    Work
}

type novelVideoAssetRef struct {
	Type      string  `json:"type"`
	ID        uint    `json:"id"`
	Kind      string  `json:"kind,omitempty"`
	Name      string  `json:"name,omitempty"`
	Role      string  `json:"role,omitempty"`
	Weight    float64 `json:"weight,omitempty"`
	LockLevel string  `json:"lock_level,omitempty"`
}

func (executableFFmpegRunner) ComposeNovelVideo(ctx context.Context, project NovelVideoProject, clips []novelVideoComposeClip, assetStore AssetStore) (novelVideoCompositionResult, error) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return novelVideoCompositionResult{}, fmt.Errorf("ffmpeg unavailable: %w", err)
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return novelVideoCompositionResult{}, fmt.Errorf("ffprobe unavailable: %w", err)
	}
	if len(clips) == 0 {
		return novelVideoCompositionResult{}, errors.New("no rendered clips to compose")
	}
	tempDir, err := os.MkdirTemp("", "novel-video-compose-*")
	if err != nil {
		return novelVideoCompositionResult{}, err
	}
	defer os.RemoveAll(tempDir)
	var concat strings.Builder
	for index, clip := range clips {
		if strings.TrimSpace(clip.Work.AssetKey) == "" {
			return novelVideoCompositionResult{}, errors.New("rendered clip asset key missing")
		}
		content, err := assetStore.Read(clip.Work.AssetKey)
		if err != nil {
			return novelVideoCompositionResult{}, fmt.Errorf("read rendered clip: %w", err)
		}
		clipPath := filepath.Join(tempDir, fmt.Sprintf("clip-%03d.mp4", index+1))
		if err := os.WriteFile(clipPath, content, 0o644); err != nil {
			return novelVideoCompositionResult{}, err
		}
		concat.WriteString("file '")
		concat.WriteString(strings.ReplaceAll(filepath.ToSlash(clipPath), "'", "'\\''"))
		concat.WriteString("'\n")
	}
	concatPath := filepath.Join(tempDir, "concat.txt")
	outputPath := filepath.Join(tempDir, "output.mp4")
	if err := os.WriteFile(concatPath, []byte(concat.String()), 0o644); err != nil {
		return novelVideoCompositionResult{}, err
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-f", "concat", "-safe", "0", "-i", concatPath, "-c", "copy", outputPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return novelVideoCompositionResult{}, fmt.Errorf("ffmpeg compose failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	outputBytes, err := os.ReadFile(outputPath)
	if err != nil {
		return novelVideoCompositionResult{}, err
	}
	return novelVideoCompositionResult{
		OutputBytes:  outputBytes,
		SubtitleText: buildNovelVideoSRTFromClips(clips),
		ManifestJSON: encodeJSON(map[string]any{"schema_version": 2, "format": "novel-video-composition", "project_id": project.ID, "clips": len(clips)}),
	}, nil
}

func (a *App) handleCreateNovelVideoProject(c *gin.Context) {
	user := currentUser(c)
	req, ok := bindNovelVideoProjectRequest(c)
	if !ok {
		return
	}
	req.Title = fallbackString(strings.TrimSpace(req.Title), "未命名小说视频项目")
	req.SourceText = strings.TrimSpace(req.SourceText)
	req.ContentMode = normalizeNovelVideoContentMode(req.ContentMode)
	if req.ContentMode == "" {
		writeError(c, http.StatusBadRequest, "invalid_content_mode", "不支持的内容模式")
		return
	}
	req.GenerationMode = normalizeNovelVideoGenerationMode(req.GenerationMode)
	if req.GenerationMode == "" {
		writeError(c, http.StatusBadRequest, "invalid_generation_mode", "unsupported generation mode")
		return
	}
	req.GridSize = normalizeNovelVideoGridSize(req.GridSize)
	req.StylePreset = strings.TrimSpace(req.StylePreset)
	req.AspectRatio = normalizeNovelVideoAspectRatio(req.AspectRatio)
	req.Duration = normalizeVideoDuration(req.Duration)
	req.ImageModel = fallbackString(strings.TrimSpace(req.ImageModel), a.cfg.DefaultImageModel)
	req.VideoModel = fallbackString(strings.TrimSpace(req.VideoModel), wuyinGrokImagineRuntimeModel)
	req.VideoSettings = normalizeNovelVideoGenerationSettings(req.VideoSettings)
	if req.VideoSettings.AspectRatio != "" {
		req.AspectRatio = req.VideoSettings.AspectRatio
	}
	if req.VideoSettings.Duration != "" {
		req.Duration = req.VideoSettings.Duration
	}
	if req.VideoSettings.Model != "" {
		req.VideoModel = req.VideoSettings.Model
	}
	if req.SourceText == "" {
		writeError(c, http.StatusBadRequest, "source_text_required", "小说文本不能为空")
		return
	}
	if utf8.RuneCountInString(req.SourceText) > novelVideoMaxSourceChars {
		writeError(c, http.StatusBadRequest, "source_text_too_long", "小说文本最多 50000 字")
		return
	}

	project := NovelVideoProject{
		UserID:            user.ID,
		Title:             req.Title,
		SourceText:        req.SourceText,
		ContentMode:       req.ContentMode,
		SchemaVersion:     defaultNovelVideoSchemaVersion(req.ContentMode, req.GenerationMode),
		GenerationMode:    req.GenerationMode,
		GridSize:          req.GridSize,
		StylePreset:       req.StylePreset,
		AspectRatio:       req.AspectRatio,
		Duration:          req.Duration,
		ImageModel:        req.ImageModel,
		VideoModel:        req.VideoModel,
		VideoSettingsJSON: encodeNovelVideoGenerationSettings(req.VideoSettings),
		Status:            NovelVideoProjectStatusDraft,
	}
	if err := a.db.Create(&project).Error; err != nil {
		writeErrorWithLogDetail(c, http.StatusInternalServerError, "novel_video_project_create_failed", novelVideoProjectCreateErrorMessage(err), err.Error())
		return
	}
	writeJSON(c, http.StatusCreated, novelVideoProjectResponse(project, nil, nil))
}

func novelVideoProjectCreateErrorMessage(err error) string {
	if isDatabaseSchemaError(err) {
		return novelVideoMissingSchemaMessage
	}
	return "小说视频项目创建失败"
}

func isDatabaseSchemaError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	for _, marker := range []string{
		"sqlstate 42p01",
		"sqlstate 42703",
		"undefined_table",
		"undefined_column",
		"no such table",
		"no such column",
		"has no column named",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return (strings.Contains(text, "relation") || strings.Contains(text, "column")) && strings.Contains(text, "does not exist")
}

func bindNovelVideoProjectRequest(c *gin.Context) (novelVideoProjectRequest, bool) {
	var req novelVideoProjectRequest
	if strings.Contains(strings.ToLower(c.GetHeader("Content-Type")), "multipart/form-data") {
		req.Title = c.PostForm("title")
		req.ContentMode = c.PostForm("content_mode")
		req.GenerationMode = c.PostForm("generation_mode")
		if value := strings.TrimSpace(c.PostForm("grid_size")); value != "" {
			if parsed, err := strconv.Atoi(value); err == nil {
				req.GridSize = parsed
			}
		}
		req.StylePreset = c.PostForm("style_preset")
		req.AspectRatio = c.PostForm("aspect_ratio")
		req.Duration = c.PostForm("duration")
		req.ImageModel = c.PostForm("image_model")
		req.VideoModel = c.PostForm("video_model")
		req.SourceText = c.PostForm("source_text")
		if file, err := c.FormFile("file"); err == nil && file != nil {
			if !strings.HasSuffix(strings.ToLower(file.Filename), ".txt") {
				writeError(c, http.StatusBadRequest, "invalid_source_file", "仅支持 .txt 文本导入")
				return req, false
			}
			opened, err := file.Open()
			if err != nil {
				writeError(c, http.StatusBadRequest, "source_file_open_failed", "文本文件读取失败")
				return req, false
			}
			defer opened.Close()
			var builder strings.Builder
			buffer := make([]byte, 8192)
			for {
				n, readErr := opened.Read(buffer)
				if n > 0 {
					builder.Write(buffer[:n])
					if utf8.RuneCountInString(builder.String()) > novelVideoMaxSourceChars {
						break
					}
				}
				if readErr != nil {
					break
				}
			}
			if text := strings.TrimSpace(builder.String()); text != "" {
				req.SourceText = text
			}
		}
		return req, true
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return req, false
	}
	return req, true
}

func (a *App) handleListNovelVideoProjects(c *gin.Context) {
	user := currentUser(c)
	var projects []NovelVideoProject
	if err := a.db.Where("user_id = ?", user.ID).Order("updated_at desc, id desc").Limit(50).Find(&projects).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_projects_load_failed", "小说视频项目读取失败")
		return
	}
	items := make([]gin.H, 0, len(projects))
	for _, project := range projects {
		items = append(items, novelVideoProjectResponse(project, nil, nil))
	}
	writeJSON(c, http.StatusOK, gin.H{"items": items})
}

func (a *App) handleGetNovelVideoProject(c *gin.Context) {
	project, ok := a.loadNovelVideoProjectForUser(c)
	if !ok {
		return
	}
	creatures, episodes, err := a.loadNovelVideoProjectChildren(project)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_project_load_failed", "小说视频项目读取失败")
		return
	}
	response := novelVideoProjectResponse(project, creatures, episodes)
	if assets, err := a.loadNovelVideoAssets(project); err == nil {
		assetItems := make([]gin.H, 0, len(assets))
		for _, asset := range assets {
			assetItems = append(assetItems, novelVideoAssetResponse(asset))
		}
		response["assets"] = assetItems
	}
	var jobs []NovelVideoJob
	if err := a.db.Where("project_id = ? AND user_id = ?", project.ID, project.UserID).Order("updated_at desc, id desc").Limit(50).Find(&jobs).Error; err == nil {
		response["jobs"] = novelVideoJobResponses(jobs)
	}
	var compositions []NovelVideoComposition
	if err := a.db.Where("project_id = ? AND user_id = ?", project.ID, project.UserID).Order("id desc").Find(&compositions).Error; err == nil {
		response["compositions"] = novelVideoCompositionResponses(compositions)
	}
	writeJSON(c, http.StatusOK, response)
}

func (a *App) handlePatchNovelVideoProject(c *gin.Context) {
	project, ok := a.loadNovelVideoProjectForUser(c)
	if !ok {
		return
	}
	var req struct {
		Title              *string                       `json:"title"`
		StylePreset        *string                       `json:"style_preset"`
		AspectRatio        *string                       `json:"aspect_ratio"`
		Duration           *string                       `json:"duration"`
		ImageModel         *string                       `json:"image_model"`
		VideoModel         *string                       `json:"video_model"`
		ContentMode        *string                       `json:"content_mode"`
		GenerationMode     *string                       `json:"generation_mode"`
		GridSize           *int                          `json:"grid_size"`
		VideoSettings      *novelVideoGenerationSettings `json:"video_settings"`
		StoryBible         map[string]any                `json:"story_bible"`
		ContentRiskSummary *string                       `json:"content_risk_summary"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	if req.Title != nil {
		project.Title = strings.TrimSpace(*req.Title)
	}
	if project.Title == "" {
		project.Title = "未命名小说视频项目"
	}
	if utf8.RuneCountInString(project.Title) > 160 {
		writeError(c, http.StatusBadRequest, "title_too_long", "项目标题最多 160 字")
		return
	}
	if req.StylePreset != nil {
		project.StylePreset = strings.TrimSpace(*req.StylePreset)
	}
	if req.AspectRatio != nil {
		ratio := strings.TrimSpace(*req.AspectRatio)
		if ratio != "16:9" && ratio != "9:16" {
			writeError(c, http.StatusBadRequest, "invalid_aspect_ratio", "不支持的画幅比例")
			return
		}
		project.AspectRatio = ratio
	}
	if req.Duration != nil {
		duration := normalizeVideoDuration(*req.Duration)
		if duration == "" {
			writeError(c, http.StatusBadRequest, "invalid_generation_parameter", "不支持的视频时长")
			return
		}
		project.Duration = duration
	}
	if req.ImageModel != nil {
		project.ImageModel = strings.TrimSpace(*req.ImageModel)
	}
	if req.VideoModel != nil {
		project.VideoModel = strings.TrimSpace(*req.VideoModel)
	}
	if req.ContentMode != nil {
		mode := normalizeNovelVideoContentMode(*req.ContentMode)
		if mode == "" {
			writeError(c, http.StatusBadRequest, "invalid_content_mode", "不支持的内容模式")
			return
		}
		project.ContentMode = mode
	}
	if req.GenerationMode != nil {
		mode := normalizeNovelVideoGenerationMode(*req.GenerationMode)
		if mode == "" {
			writeError(c, http.StatusBadRequest, "invalid_generation_mode", "unsupported generation mode")
			return
		}
		project.GenerationMode = mode
	}
	if req.GridSize != nil {
		project.GridSize = normalizeNovelVideoGridSize(*req.GridSize)
	}
	if req.VideoSettings != nil {
		settings := normalizeNovelVideoGenerationSettings(*req.VideoSettings)
		if settings.AspectRatio != "" {
			project.AspectRatio = settings.AspectRatio
		}
		if settings.Duration != "" {
			project.Duration = settings.Duration
		}
		if settings.Model != "" {
			project.VideoModel = settings.Model
		}
		project.VideoSettingsJSON = encodeNovelVideoGenerationSettings(settings)
	}
	if req.StoryBible != nil {
		raw, _ := json.Marshal(req.StoryBible)
		project.StoryBibleJSON = string(raw)
	}
	if req.ContentRiskSummary != nil {
		project.ContentRiskSummary = strings.TrimSpace(*req.ContentRiskSummary)
	}
	if err := a.db.Save(&project).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_project_save_failed", "小说视频项目保存失败")
		return
	}
	creatures, episodes, err := a.loadNovelVideoProjectChildren(project)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_project_load_failed", "小说视频项目读取失败")
		return
	}
	response := novelVideoProjectResponse(project, creatures, episodes)
	assets, _ := a.loadNovelVideoAssets(project)
	assetItems := make([]gin.H, 0, len(assets))
	for _, asset := range assets {
		assetItems = append(assetItems, novelVideoAssetResponse(asset))
	}
	response["assets"] = assetItems
	writeJSON(c, http.StatusOK, response)
}

func (a *App) handleAnalyzeNovelVideoProject(c *gin.Context) {
	project, ok := a.loadNovelVideoProjectForUser(c)
	if !ok {
		return
	}
	plan := fallbackNovelVideoAnalysis(project)
	if deepSeekPlan, err := a.planNovelVideoAnalysisWithDeepSeek(c.Request.Context(), project); err == nil {
		plan = deepSeekPlan
	}
	storyBibleJSON, _ := json.Marshal(plan["story_bible"])
	draftJSON, _ := json.Marshal(plan)
	creatureDrafts, _ := plan["creatures"].([]novelVideoCreatureDraft)

	err := a.db.Transaction(func(tx *gorm.DB) error {
		project.StoryBibleJSON = string(storyBibleJSON)
		project.PlanningDraftJSON = string(draftJSON)
		project.ContentRiskSummary = fmt.Sprint(plan["content_risk_summary"])
		project.Status = NovelVideoProjectStatusAnalyzed
		project.ErrorCode = ""
		project.ErrorMessage = ""
		if err := tx.Save(&project).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ? AND work_id IS NULL", project.ID).Delete(&NovelVideoCreature{}).Error; err != nil {
			return err
		}
		for _, draft := range creatureDrafts {
			creature := NovelVideoCreature{
				ProjectID:               project.ID,
				UserID:                  project.UserID,
				Name:                    draft.Name,
				CreatureType:            draft.CreatureType,
				Appearance:              draft.Appearance,
				Abilities:               draft.Abilities,
				VisualConsistencyPrompt: draft.VisualConsistencyPrompt,
				ReviewStatus:            NovelVideoReviewStatusNeedsReview,
			}
			if err := tx.Create(&creature).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_analyze_failed", "小说解析失败")
		return
	}
	creatures, episodes, err := a.loadNovelVideoProjectChildren(project)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_project_load_failed", "小说视频项目读取失败")
		return
	}
	response := novelVideoProjectResponse(project, creatures, episodes)
	assets, _ := a.loadNovelVideoAssets(project)
	assetItems := make([]gin.H, 0, len(assets))
	for _, asset := range assets {
		assetItems = append(assetItems, novelVideoAssetResponse(asset))
	}
	response["assets"] = assetItems
	writeJSON(c, http.StatusOK, response)
}

func (a *App) handlePlanNovelVideoImages(c *gin.Context) {
	project, ok := a.loadNovelVideoProjectForUser(c)
	if !ok {
		return
	}
	var req struct {
		ShotCount int `json:"shot_count"`
	}
	_ = c.ShouldBindJSON(&req)
	shotCount := req.ShotCount
	if shotCount <= 0 {
		shotCount = 20
	}
	if shotCount > 80 {
		shotCount = 80
	}
	creatureDrafts := fallbackNovelVideoImageActors(project)
	assetDrafts := fallbackNovelVideoImageAssets(project, creatureDrafts)
	episodeDraft := fallbackNovelVideoImageEpisode(project, creatureDrafts, shotCount)
	err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("project_id = ?", project.ID).Delete(&NovelVideoShotImage{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ?", project.ID).Delete(&NovelVideoShot{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ?", project.ID).Delete(&NovelVideoEpisode{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ? AND work_id IS NULL", project.ID).Delete(&NovelVideoAsset{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ? AND work_id IS NULL", project.ID).Delete(&NovelVideoCreature{}).Error; err != nil {
			return err
		}
		for index := range creatureDrafts {
			if err := tx.Create(&creatureDrafts[index]).Error; err != nil {
				return err
			}
		}
		for index := range assetDrafts {
			if strings.TrimSpace(assetDrafts[index].Kind) == NovelVideoAssetKindActorRef && len(creatureDrafts) > 0 {
				actor := creatureDrafts[index%len(creatureDrafts)]
				assetDrafts[index].MetadataJSON = encodeJSON(map[string]any{"actor_id": actor.ID, "approved": false, "lock_level": "medium", "source": "image_plan"})
			}
			if err := tx.Create(&assetDrafts[index]).Error; err != nil {
				return err
			}
		}
		episode := NovelVideoEpisode{ProjectID: project.ID, UserID: project.UserID, Number: 1, Title: episodeDraft.Title, Summary: episodeDraft.Summary, Status: NovelVideoReviewStatusNeedsReview}
		if err := tx.Create(&episode).Error; err != nil {
			return err
		}
		for index, shotDraft := range episodeDraft.Shots {
			actor := creatureDrafts[index%len(creatureDrafts)]
			actorIDs := []uint{actor.ID}
			if index%3 == 2 && len(creatureDrafts) > 1 {
				actorIDs = []uint{actor.ID, creatureDrafts[(index+1)%len(creatureDrafts)].ID}
			}
			refs := []novelVideoAssetRef{{Type: "creature", ID: actor.ID, Name: actor.Name, Role: "lead", Weight: 1, LockLevel: "medium"}}
			if len(assetDrafts) > 0 {
				refs = append(refs, novelVideoAssetRef{Type: "asset", ID: assetDrafts[index%len(assetDrafts)].ID, Kind: assetDrafts[index%len(assetDrafts)].Kind, Name: assetDrafts[index%len(assetDrafts)].Name, Role: "visual_reference", Weight: 0.8, LockLevel: "medium"})
			}
			shot := NovelVideoShot{
				ProjectID:       project.ID,
				EpisodeID:       episode.ID,
				UserID:          project.UserID,
				Number:          shotDraft.Number,
				Title:           shotDraft.Title,
				Prompt:          shotDraft.Prompt,
				ImagePrompt:     shotDraft.Prompt,
				VideoPrompt:     "",
				VoiceoverText:   shotDraft.Title,
				DurationSeconds: 4,
				AssetRefsJSON:   encodeNovelVideoAssetRefs(refs),
				CreatureIDsJSON: encodeJSON(actorIDs),
				Status:          NovelVideoReviewStatusNeedsReview,
			}
			if err := tx.Create(&shot).Error; err != nil {
				return err
			}
		}
		project.ContentMode = NovelVideoContentModeShortFilmImage
		project.GenerationMode = NovelVideoGenerationModeImageSeries
		project.SchemaVersion = 3
		project.Status = NovelVideoProjectStatusPlanned
		return tx.Save(&project).Error
	})
	if err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_image_plan_failed", "图片镜头计划生成失败")
		return
	}
	creatures, episodes, err := a.loadNovelVideoProjectChildren(project)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_project_load_failed", "小说视频项目读取失败")
		return
	}
	response := novelVideoProjectResponse(project, creatures, episodes)
	assets, _ := a.loadNovelVideoAssets(project)
	assetItems := make([]gin.H, 0, len(assets))
	for _, asset := range assets {
		assetItems = append(assetItems, novelVideoAssetResponse(asset))
	}
	response["assets"] = assetItems
	writeJSON(c, http.StatusOK, response)
}

func (a *App) handlePatchNovelVideoCreature(c *gin.Context) {
	project, ok := a.loadNovelVideoProjectForUser(c)
	if !ok {
		return
	}
	creatureID, ok := uintParam(c, "creature_id")
	if !ok {
		writeError(c, http.StatusBadRequest, "invalid_creature_id", "生物 ID 无效")
		return
	}
	var creature NovelVideoCreature
	if err := a.db.Where("id = ? AND project_id = ? AND user_id = ?", creatureID, project.ID, project.UserID).First(&creature).Error; err != nil {
		writeError(c, http.StatusNotFound, "novel_video_creature_not_found", "生物卡不存在")
		return
	}
	var req struct {
		Name                    *string `json:"name"`
		CreatureType            *string `json:"creature_type"`
		Appearance              *string `json:"appearance"`
		Abilities               *string `json:"abilities"`
		VisualConsistencyPrompt *string `json:"visual_consistency_prompt"`
		ReviewStatus            *string `json:"review_status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	if req.Name != nil {
		creature.Name = strings.TrimSpace(*req.Name)
	}
	if req.CreatureType != nil {
		creature.CreatureType = strings.TrimSpace(*req.CreatureType)
	}
	if req.Appearance != nil {
		creature.Appearance = strings.TrimSpace(*req.Appearance)
	}
	if req.Abilities != nil {
		creature.Abilities = strings.TrimSpace(*req.Abilities)
	}
	if req.VisualConsistencyPrompt != nil {
		creature.VisualConsistencyPrompt = strings.TrimSpace(*req.VisualConsistencyPrompt)
	}
	if req.ReviewStatus != nil {
		creature.ReviewStatus = normalizeNovelVideoReviewStatus(*req.ReviewStatus)
	}
	if creature.Name == "" {
		writeError(c, http.StatusBadRequest, "creature_name_required", "生物名称不能为空")
		return
	}
	if err := a.db.Save(&creature).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_creature_save_failed", "生物卡保存失败")
		return
	}
	writeJSON(c, http.StatusOK, creature)
}

func (a *App) handlePatchNovelVideoActor(c *gin.Context) {
	project, ok := a.loadNovelVideoProjectForUser(c)
	if !ok {
		return
	}
	actorID, ok := uintParam(c, "actor_id")
	if !ok {
		writeError(c, http.StatusBadRequest, "invalid_actor_id", "演员 ID 无效")
		return
	}
	var actor NovelVideoCreature
	if err := a.db.Where("id = ? AND project_id = ? AND user_id = ?", actorID, project.ID, project.UserID).First(&actor).Error; err != nil {
		writeError(c, http.StatusNotFound, "novel_video_actor_not_found", "演员不存在")
		return
	}
	var req struct {
		Name                    *string `json:"name"`
		Appearance              *string `json:"appearance"`
		VisualConsistencyPrompt *string `json:"visual_consistency_prompt"`
		NegativeIdentityPrompt  *string `json:"negative_identity_prompt"`
		ReferenceAssetIDs       []uint  `json:"reference_asset_ids"`
		CanonicalAssetID        *uint   `json:"canonical_asset_id"`
		LockLevel               *string `json:"lock_level"`
		ApprovedVersion         *int    `json:"approved_version"`
		ReviewStatus            *string `json:"review_status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	if len(req.ReferenceAssetIDs) > 0 {
		if _, err := a.loadNovelVideoReferenceAssets(project.UserID, req.ReferenceAssetIDs, len(req.ReferenceAssetIDs), referenceAssetKindImage); err != nil {
			writeError(c, http.StatusNotFound, "reference_asset_not_found", "演员参考图不存在")
			return
		}
	}
	if req.CanonicalAssetID != nil && *req.CanonicalAssetID != 0 {
		if _, err := a.loadNovelVideoReferenceAssets(project.UserID, []uint{*req.CanonicalAssetID}, 1, referenceAssetKindImage); err != nil {
			writeError(c, http.StatusNotFound, "reference_asset_not_found", "演员主参考图不存在")
			return
		}
	}
	if req.Name != nil {
		actor.Name = strings.TrimSpace(*req.Name)
	}
	if req.Appearance != nil {
		actor.Appearance = strings.TrimSpace(*req.Appearance)
	}
	if req.VisualConsistencyPrompt != nil {
		actor.VisualConsistencyPrompt = strings.TrimSpace(*req.VisualConsistencyPrompt)
	}
	if req.ReviewStatus != nil {
		actor.ReviewStatus = normalizeNovelVideoReviewStatus(*req.ReviewStatus)
	}
	if actor.Name == "" {
		writeError(c, http.StatusBadRequest, "actor_name_required", "演员名称不能为空")
		return
	}
	lockLevel := normalizeNovelVideoActorLockLevel(pointerString(req.LockLevel))
	approvedVersion := positiveOrDefault(pointerInt(req.ApprovedVersion), 1)
	negativePrompt := strings.TrimSpace(pointerString(req.NegativeIdentityPrompt))
	canonicalID := uint(0)
	if req.CanonicalAssetID != nil {
		canonicalID = *req.CanonicalAssetID
	}
	metadata := map[string]any{
		"actor_id":                  actor.ID,
		"reference_asset_ids":       req.ReferenceAssetIDs,
		"canonical_asset_id":        canonicalID,
		"negative_identity_prompt":  negativePrompt,
		"lock_level":                lockLevel,
		"approved_version":          approvedVersion,
		"visual_consistency_prompt": actor.VisualConsistencyPrompt,
		"approved":                  actor.ReviewStatus == NovelVideoReviewStatusApproved,
	}
	err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&actor).Error; err != nil {
			return err
		}
		asset := NovelVideoAsset{ProjectID: project.ID, UserID: project.UserID, Kind: NovelVideoAssetKindActorRef, Version: 1}
		var existing []NovelVideoAsset
		if err := tx.Where("project_id = ? AND user_id = ? AND kind = ?", project.ID, project.UserID, NovelVideoAssetKindActorRef).Find(&existing).Error; err != nil {
			return err
		}
		for _, item := range existing {
			meta := decodeJSONMap(item.MetadataJSON)
			if uintFromAny(meta["actor_id"]) == actor.ID {
				asset = item
				break
			}
		}
		asset.Name = actor.Name + "参考图"
		asset.Description = "演员一致性参考资产"
		asset.Prompt = actor.VisualConsistencyPrompt
		asset.ReviewStatus = actor.ReviewStatus
		asset.MetadataJSON = encodeJSON(metadata)
		return tx.Save(&asset).Error
	})
	if err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_actor_save_failed", "演员锁定信息保存失败")
		return
	}
	response := novelVideoCreatureResponse(actor)
	response["reference_asset_ids"] = req.ReferenceAssetIDs
	response["canonical_asset_id"] = canonicalID
	response["negative_identity_prompt"] = negativePrompt
	response["lock_level"] = lockLevel
	response["approved_version"] = approvedVersion
	writeJSON(c, http.StatusOK, response)
}

func (a *App) handleGenerateNovelVideoActorLockSheet(c *gin.Context) {
	project, ok := a.loadNovelVideoProjectForUser(c)
	if !ok {
		return
	}
	actorID, ok := uintParam(c, "actor_id")
	if !ok {
		writeError(c, http.StatusBadRequest, "invalid_actor_id", "演员 ID 无效")
		return
	}
	var actor NovelVideoCreature
	if err := a.db.Where("id = ? AND project_id = ? AND user_id = ?", actorID, project.ID, project.UserID).First(&actor).Error; err != nil {
		writeError(c, http.StatusNotFound, "novel_video_actor_not_found", "演员不存在")
		return
	}
	prompt := strings.Join([]string{
		"短电影演员定妆图，正脸、半身、全身三视图，表情中性，自然光。",
		"项目：" + project.Title,
		"演员：" + actor.Name,
		"外观：" + actor.Appearance,
		"一致性要求：" + actor.VisualConsistencyPrompt,
	}, "\n")
	asset := NovelVideoAsset{
		ProjectID:    project.ID,
		UserID:       project.UserID,
		Kind:         NovelVideoAssetKindActorKeySheet,
		Name:         actor.Name + "定妆图",
		Description:  "用于锁定演员身份的一致性角色卡",
		Prompt:       prompt,
		Version:      1,
		ReviewStatus: NovelVideoReviewStatusNeedsReview,
		MetadataJSON: encodeJSON(map[string]any{"actor_id": actor.ID, "source": "lock_sheet", "lock_level": "strict"}),
	}
	if err := a.db.Create(&asset).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_actor_lock_sheet_failed", "演员定妆图生成失败")
		return
	}
	item := novelVideoAssetResponse(asset)
	writeJSON(c, http.StatusOK, gin.H{"asset": item, "item": item, "prompt": prompt})
}

func (a *App) handleGenerateNovelVideoAssets(c *gin.Context) {
	project, ok := a.loadNovelVideoProjectForUser(c)
	if !ok {
		return
	}
	var req struct {
		Kinds   []string `json:"kinds"`
		AssetID *uint    `json:"asset_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	if req.AssetID != nil && *req.AssetID != 0 {
		var asset NovelVideoAsset
		if err := a.db.Where("id = ? AND project_id = ? AND user_id = ?", *req.AssetID, project.ID, project.UserID).First(&asset).Error; err != nil {
			writeError(c, http.StatusNotFound, "novel_video_asset_not_found", "资产不存在")
			return
		}
		job := NovelVideoJob{
			ProjectID:   project.ID,
			UserID:      project.UserID,
			JobType:     NovelVideoJobTypeAssetImage,
			Status:      GenerationStatusQueued,
			AssetID:     &asset.ID,
			MaxAttempts: 3,
			PayloadJSON: encodeJSON(map[string]any{"asset_id": asset.ID, "kind": asset.Kind, "prompt": asset.Prompt, "retry": true}),
		}
		if err := a.db.Transaction(func(tx *gorm.DB) error {
			asset.ErrorCode = ""
			asset.ErrorMessage = ""
			if err := tx.Save(&asset).Error; err != nil {
				return err
			}
			return tx.Create(&job).Error
		}); err != nil {
			writeError(c, http.StatusInternalServerError, "novel_video_assets_generate_failed", "资产图片重试创建失败")
			return
		}
		go a.runNovelVideoAssetImageJobs(project, []NovelVideoJob{job})
		writeJSON(c, http.StatusOK, gin.H{"items": []NovelVideoAsset{asset}, "jobs": novelVideoJobResponses([]NovelVideoJob{job})})
		return
	}
	kinds := normalizeNovelVideoAssetKinds(req.Kinds)
	var skippedCharacterActorRefs []NovelVideoAsset
	if containsNovelVideoAssetKind(kinds, NovelVideoAssetKindCharacter) && novelVideoUsesActorRefsAsCharacters(project) {
		actorRefs, err := a.loadNovelVideoActorRefAssets(project)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "novel_video_assets_generate_failed", "资产草稿读取失败")
			return
		}
		if len(actorRefs) > 0 {
			skippedCharacterActorRefs = actorRefs
			kinds = removeNovelVideoAssetKind(kinds, NovelVideoAssetKindCharacter)
		}
	}
	if len(kinds) == 0 {
		if len(skippedCharacterActorRefs) > 0 {
			writeJSON(c, http.StatusOK, gin.H{"items": skippedCharacterActorRefs, "jobs": []gin.H{}})
			return
		}
		writeError(c, http.StatusBadRequest, "invalid_asset_kind", "不支持的资产类型")
		return
	}
	drafts := fallbackNovelVideoAssets(project, kinds)
	existingAssets, err := a.loadReusableNovelVideoAssets(project, kinds)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_assets_generate_failed", "资产草案读取失败")
		return
	}
	activeJobs, err := a.loadActiveNovelVideoAssetImageJobs(project)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_assets_generate_failed", "资产任务读取失败")
		return
	}
	activeJobByAssetID := map[uint]NovelVideoJob{}
	for _, job := range activeJobs {
		if job.AssetID != nil {
			activeJobByAssetID[*job.AssetID] = job
		}
	}
	existingByKey := map[string]NovelVideoAsset{}
	for _, asset := range existingAssets {
		key := novelVideoAssetDedupeKey(asset)
		if key == "" {
			continue
		}
		current, exists := existingByKey[key]
		if !exists || betterNovelVideoAssetForGeneration(asset, current, activeJobByAssetID) {
			existingByKey[key] = asset
		}
	}
	assets := make([]NovelVideoAsset, 0, len(skippedCharacterActorRefs)+len(drafts))
	assets = append(assets, skippedCharacterActorRefs...)
	jobs := make([]NovelVideoJob, 0, len(drafts))
	newJobs := make([]NovelVideoJob, 0, len(drafts))
	if err := a.db.Transaction(func(tx *gorm.DB) error {
		for index := range drafts {
			draft := drafts[index]
			if existing, ok := existingByKey[novelVideoAssetDedupeKey(draft)]; ok {
				assets = append(assets, existing)
				if job, active := activeJobByAssetID[existing.ID]; active {
					jobs = append(jobs, job)
				}
				continue
			}
			if err := tx.Create(&draft).Error; err != nil {
				return err
			}
			job := NovelVideoJob{
				ProjectID:   project.ID,
				UserID:      project.UserID,
				JobType:     NovelVideoJobTypeAssetImage,
				Status:      GenerationStatusQueued,
				AssetID:     &draft.ID,
				MaxAttempts: 3,
				PayloadJSON: encodeJSON(map[string]any{"asset_id": draft.ID, "kind": draft.Kind, "prompt": draft.Prompt}),
			}
			if err := tx.Create(&job).Error; err != nil {
				return err
			}
			assets = append(assets, draft)
			jobs = append(jobs, job)
			newJobs = append(newJobs, job)
		}
		return nil
	}); err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_assets_generate_failed", "资产草案生成失败")
		return
	}
	if len(newJobs) > 0 {
		go a.runNovelVideoAssetImageJobs(project, newJobs)
	}
	writeJSON(c, http.StatusOK, gin.H{"items": assets, "jobs": novelVideoJobResponses(jobs)})
}

func (a *App) loadReusableNovelVideoAssets(project NovelVideoProject, kinds []string) ([]NovelVideoAsset, error) {
	var assets []NovelVideoAsset
	err := a.db.Where("project_id = ? AND user_id = ? AND kind IN ?", project.ID, project.UserID, kinds).
		Order("kind asc, name asc, id asc").
		Find(&assets).Error
	return assets, err
}

func (a *App) loadNovelVideoActorRefAssets(project NovelVideoProject) ([]NovelVideoAsset, error) {
	var assets []NovelVideoAsset
	err := a.db.Where("project_id = ? AND user_id = ? AND kind = ?", project.ID, project.UserID, NovelVideoAssetKindActorRef).
		Order("id asc").
		Find(&assets).Error
	return assets, err
}

func (a *App) loadActiveNovelVideoAssetImageJobs(project NovelVideoProject) ([]NovelVideoJob, error) {
	var jobs []NovelVideoJob
	err := a.db.Where("project_id = ? AND user_id = ? AND job_type = ? AND status IN ? AND asset_id IS NOT NULL",
		project.ID,
		project.UserID,
		NovelVideoJobTypeAssetImage,
		[]string{GenerationStatusQueued, GenerationStatusRunning},
	).Order("id desc").Find(&jobs).Error
	return jobs, err
}

func novelVideoAssetDedupeKey(asset NovelVideoAsset) string {
	kind := strings.TrimSpace(asset.Kind)
	if kind == "" {
		return ""
	}
	meta := decodeJSONMap(asset.MetadataJSON)
	if kind == NovelVideoAssetKindActorRef {
		if actorID := uintFromAny(meta["actor_id"]); actorID > 0 {
			return kind + "\x00actor:" + strconv.FormatUint(uint64(actorID), 10)
		}
	}
	intent := normalizedNovelVideoAssetIntent(asset)
	if intent == "" {
		return ""
	}
	return kind + "\x00" + intent
}

func normalizedNovelVideoAssetIntent(asset NovelVideoAsset) string {
	name := normalizeNovelVideoAssetIntentText(asset.Name)
	if name == "" {
		name = normalizeNovelVideoAssetIntentText(asset.Description)
	}
	if name == "" {
		name = normalizeNovelVideoAssetIntentText(asset.Prompt)
	}
	if name == "" {
		return ""
	}
	switch strings.TrimSpace(asset.Kind) {
	case NovelVideoAssetKindScene:
		return stripNovelVideoAssetIntentTerms(name, []string{"主场景", "核心场景", "场景参考", "场景", "location", "scene"})
	case NovelVideoAssetKindProp:
		return stripNovelVideoAssetIntentTerms(name, []string{"关键道具", "核心物件", "道具参考", "道具", "prop", "object"})
	case NovelVideoAssetKindStyle:
		return stripNovelVideoAssetIntentTerms(name, []string{"视觉风格", "风格参考", "统一风格", "风格", "style"})
	case NovelVideoAssetKindClue:
		return stripNovelVideoAssetIntentTerms(name, []string{"悬念线索", "视觉线索", "线索参考", "线索", "clue"})
	case NovelVideoAssetKindCharacter:
		return stripNovelVideoAssetIntentTerms(name, []string{"主角视觉锚点", "角色锚点", "视觉锚点", "角色", "character"})
	default:
		return name
	}
}

func normalizeNovelVideoAssetIntentText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(
		" ", "",
		"\t", "",
		"\n", "",
		"\r", "",
		"-", "",
		"_", "",
		"·", "",
		"•", "",
		"：", "",
		":", "",
		"，", "",
		",", "",
		"。", "",
		".", "",
		"、", "",
		"(", "",
		")", "",
		"（", "",
		"）", "",
		"[", "",
		"]", "",
		"【", "",
		"】", "",
	)
	return replacer.Replace(value)
}

func stripNovelVideoAssetIntentTerms(value string, terms []string) string {
	result := value
	for _, term := range terms {
		result = strings.ReplaceAll(result, normalizeNovelVideoAssetIntentText(term), "")
	}
	if result == "" {
		return value
	}
	return result
}

func novelVideoUsesActorRefsAsCharacters(project NovelVideoProject) bool {
	return effectiveNovelVideoContentMode(project.ContentMode) == NovelVideoContentModeShortFilmImage ||
		effectiveNovelVideoGenerationMode(project.GenerationMode) == NovelVideoGenerationModeImageSeries
}

func containsNovelVideoAssetKind(kinds []string, target string) bool {
	for _, kind := range kinds {
		if kind == target {
			return true
		}
	}
	return false
}

func removeNovelVideoAssetKind(kinds []string, target string) []string {
	filtered := kinds[:0]
	for _, kind := range kinds {
		if kind != target {
			filtered = append(filtered, kind)
		}
	}
	return filtered
}

func betterNovelVideoAssetKeeper(candidate, current NovelVideoAsset) bool {
	candidateScore := novelVideoAssetKeeperScore(candidate)
	currentScore := novelVideoAssetKeeperScore(current)
	if candidateScore != currentScore {
		return candidateScore > currentScore
	}
	if !candidate.CreatedAt.Equal(current.CreatedAt) {
		return candidate.CreatedAt.Before(current.CreatedAt)
	}
	return candidate.ID < current.ID
}

func betterNovelVideoAssetForGeneration(candidate, current NovelVideoAsset, activeJobByAssetID map[uint]NovelVideoJob) bool {
	candidateActive := activeJobByAssetID[candidate.ID].ID != 0
	currentActive := activeJobByAssetID[current.ID].ID != 0
	if candidateActive != currentActive {
		return candidateActive
	}
	return betterNovelVideoAssetKeeper(candidate, current)
}

func novelVideoAssetKeeperScore(asset NovelVideoAsset) int {
	if asset.ReviewStatus == NovelVideoReviewStatusApproved {
		return 3
	}
	if strings.TrimSpace(asset.AssetURL) != "" || asset.WorkID != nil || asset.GenerationRecordID != nil {
		return 2
	}
	return 1
}

func (a *App) runNovelVideoAssetImageJobs(project NovelVideoProject, jobs []NovelVideoJob) {
	for _, job := range jobs {
		a.runNovelVideoAssetImageJob(project, job)
	}
}

func (a *App) runNovelVideoAssetImageJob(project NovelVideoProject, job NovelVideoJob) {
	startedAt := time.Now()
	claim := a.db.Model(&NovelVideoJob{}).
		Where("id = ? AND status = ?", job.ID, GenerationStatusQueued).
		Updates(map[string]any{
			"status":        GenerationStatusRunning,
			"progress":      25,
			"attempts":      gorm.Expr("attempts + ?", 1),
			"error_code":    "",
			"error_message": "",
			"started_at":    &startedAt,
			"finished_at":   nil,
		})
	if claim.Error != nil || claim.RowsAffected == 0 {
		return
	}
	if err := a.db.First(&job, job.ID).Error; err != nil {
		return
	}
	if job.AssetID == nil {
		a.failNovelVideoAssetImageJob(job, nil, "asset_image_missing", "资产图片任务缺少资产 ID", GenerationRecord{})
		return
	}

	var asset NovelVideoAsset
	if err := a.db.Where("id = ? AND project_id = ? AND user_id = ?", *job.AssetID, project.ID, project.UserID).First(&asset).Error; err != nil {
		a.failNovelVideoAssetImageJob(job, nil, "asset_image_missing", "资产不存在或已被删除", GenerationRecord{})
		return
	}

	result, record, err := a.runNovelVideoImageGeneration(project, asset.Prompt)
	if err != nil {
		a.failNovelVideoAssetImageJob(job, &asset, "asset_image_generation_failed", err.Error(), record)
		return
	}
	if record.ID == 0 {
		a.failNovelVideoAssetImageJob(job, &asset, "asset_image_generation_failed", "图片生成记录缺失", record)
		return
	}

	finishedAt := time.Now()
	asset.GenerationRecordID = &record.ID
	asset.WorkID = record.WorkID
	asset.AssetURL = record.PreviewURL
	asset.ErrorCode = ""
	asset.ErrorMessage = ""
	job.Status = GenerationStatusSucceeded
	job.Progress = 100
	job.ErrorCode = ""
	job.ErrorMessage = ""
	job.FinishedAt = &finishedAt
	job.ResultJSON = encodeJSON(map[string]any{
		"generation_record_id": record.ID,
		"work_id":              record.WorkID,
		"asset_url":            record.PreviewURL,
		"available_credits":    optionalNovelVideoGenerationAvailableCredits(result),
	})
	_ = a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&asset).Error; err != nil {
			return err
		}
		return tx.Save(&job).Error
	})
}

func (a *App) failNovelVideoAssetImageJob(job NovelVideoJob, asset *NovelVideoAsset, code, message string, record GenerationRecord) {
	finishedAt := time.Now()
	if strings.TrimSpace(message) == "" {
		message = "资产图片生成失败"
	}
	job.Status = GenerationStatusFailed
	job.Progress = 0
	job.ErrorCode = code
	job.ErrorMessage = message
	job.FinishedAt = &finishedAt
	result := map[string]any{}
	if record.ID != 0 {
		result["generation_record_id"] = record.ID
	}
	job.ResultJSON = encodeJSON(result)
	_ = a.db.Transaction(func(tx *gorm.DB) error {
		if asset != nil {
			asset.ErrorCode = code
			asset.ErrorMessage = message
			if err := tx.Save(asset).Error; err != nil {
				return err
			}
		}
		return tx.Save(&job).Error
	})
}

func optionalNovelVideoGenerationAvailableCredits(result *generationTaskResult) any {
	if result == nil {
		return nil
	}
	return result.AvailableCredits
}

func (a *App) handleDedupeNovelVideoAssets(c *gin.Context) {
	project, ok := a.loadNovelVideoProjectForUser(c)
	if !ok {
		return
	}
	result, err := a.dedupeNovelVideoAssets(project)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_assets_dedupe_failed", "重复资产清理失败")
		return
	}
	writeJSON(c, http.StatusOK, result)
}

func (a *App) dedupeNovelVideoAssets(project NovelVideoProject) (gin.H, error) {
	var assets []NovelVideoAsset
	if err := a.db.Where("project_id = ? AND user_id = ?", project.ID, project.UserID).
		Order("kind asc, name asc, id asc").
		Find(&assets).Error; err != nil {
		return nil, err
	}
	activeJobs, err := a.loadActiveNovelVideoAssetImageJobs(project)
	if err != nil {
		return nil, err
	}
	activeAssetIDs := map[uint]bool{}
	for _, job := range activeJobs {
		if job.AssetID != nil {
			activeAssetIDs[*job.AssetID] = true
		}
	}
	referencedAssetIDs, err := a.referencedNovelVideoAssetIDs(project)
	if err != nil {
		return nil, err
	}
	groups := map[string][]NovelVideoAsset{}
	for _, asset := range assets {
		key := novelVideoAssetDedupeKey(asset)
		if key == "" {
			continue
		}
		groups[key] = append(groups[key], asset)
	}
	removedIDs := make([]uint, 0)
	collapsedIDs := make([]uint, 0)
	keptIDs := make([]uint, 0)
	skippedApproved := 0
	skippedActive := 0
	skippedReferenced := 0
	if err := a.db.Transaction(func(tx *gorm.DB) error {
		for _, group := range groups {
			if len(group) < 2 {
				continue
			}
			sort.SliceStable(group, func(i, j int) bool {
				return betterNovelVideoAssetKeeper(group[i], group[j])
			})
			keep := group[0]
			keptIDs = append(keptIDs, keep.ID)
			for _, duplicate := range group[1:] {
				if duplicate.ReviewStatus == NovelVideoReviewStatusApproved {
					skippedApproved++
					collapsedIDs = append(collapsedIDs, duplicate.ID)
					continue
				}
				if referencedAssetIDs[duplicate.ID] {
					skippedReferenced++
					collapsedIDs = append(collapsedIDs, duplicate.ID)
					continue
				}
				if activeAssetIDs[duplicate.ID] {
					skippedActive++
					collapsedIDs = append(collapsedIDs, duplicate.ID)
					continue
				}
				if err := tx.Delete(&duplicate).Error; err != nil {
					return err
				}
				removedIDs = append(removedIDs, duplicate.ID)
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	refreshed, err := a.loadNovelVideoAssets(project)
	if err != nil {
		return nil, err
	}
	items := make([]gin.H, 0, len(refreshed))
	for _, asset := range refreshed {
		items = append(items, novelVideoAssetResponse(asset))
	}
	return gin.H{
		"removed":            len(removedIDs),
		"removed_ids":        removedIDs,
		"collapsed_ids":      collapsedIDs,
		"kept_ids":           keptIDs,
		"skipped_approved":   skippedApproved,
		"skipped_active":     skippedActive,
		"skipped_referenced": skippedReferenced,
		"items":              items,
	}, nil
}

func (a *App) referencedNovelVideoAssetIDs(project NovelVideoProject) (map[uint]bool, error) {
	var shots []NovelVideoShot
	if err := a.db.Where("project_id = ? AND user_id = ?", project.ID, project.UserID).Find(&shots).Error; err != nil {
		return nil, err
	}
	referenced := map[uint]bool{}
	for _, shot := range shots {
		if shot.ReferenceAssetID != nil && *shot.ReferenceAssetID != 0 {
			referenced[*shot.ReferenceAssetID] = true
		}
		for _, ref := range decodeNovelVideoAssetRefs(shot.AssetRefsJSON) {
			if ref.Type == "asset" && ref.ID != 0 {
				referenced[ref.ID] = true
			}
		}
	}
	return referenced, nil
}

func (a *App) novelVideoAssetShotReferences(project NovelVideoProject, assetID uint) ([]gin.H, error) {
	var episodes []NovelVideoEpisode
	if err := a.db.Preload("Shots").Where("project_id = ? AND user_id = ?", project.ID, project.UserID).Order("number asc, id asc").Find(&episodes).Error; err != nil {
		return nil, err
	}
	references := make([]gin.H, 0)
	for _, episode := range episodes {
		for _, shot := range episode.Shots {
			referenced := shot.ReferenceAssetID != nil && *shot.ReferenceAssetID == assetID
			if !referenced {
				for _, ref := range decodeNovelVideoAssetRefs(shot.AssetRefsJSON) {
					if ref.Type == "asset" && ref.ID == assetID {
						referenced = true
						break
					}
				}
			}
			if !referenced {
				continue
			}
			references = append(references, gin.H{
				"episode_id":     episode.ID,
				"episode_number": episode.Number,
				"episode_title":  episode.Title,
				"shot_id":        shot.ID,
				"shot_number":    shot.Number,
				"shot_title":     shot.Title,
			})
		}
	}
	return references, nil
}

func writeNovelVideoAssetConflict(c *gin.Context, code, message string, payload gin.H) {
	c.Set(requestLogErrorCodeKey, code)
	c.Set(requestLogErrorMessageKey, message)
	response := gin.H{
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	}
	for key, value := range payload {
		response[key] = value
	}
	c.JSON(http.StatusConflict, response)
}

func (a *App) handleDeleteNovelVideoAsset(c *gin.Context) {
	project, ok := a.loadNovelVideoProjectForUser(c)
	if !ok {
		return
	}
	assetID, ok := uintParam(c, "asset_id")
	if !ok {
		writeError(c, http.StatusBadRequest, "invalid_asset_id", "资产 ID 无效")
		return
	}
	var asset NovelVideoAsset
	if err := a.db.Where("id = ? AND project_id = ? AND user_id = ?", assetID, project.ID, project.UserID).First(&asset).Error; err != nil {
		writeError(c, http.StatusNotFound, "novel_video_asset_not_found", "资产不存在")
		return
	}
	activeJobs, err := a.loadActiveNovelVideoAssetImageJobs(project)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_asset_jobs_load_failed", "资产任务读取失败")
		return
	}
	queuedJobIDs := make([]uint, 0)
	for _, job := range activeJobs {
		if job.AssetID == nil || *job.AssetID != asset.ID {
			continue
		}
		if job.Status == GenerationStatusRunning {
			writeNovelVideoAssetConflict(c, "novel_video_asset_job_active", "资产图片仍在生成中，暂时不能删除", gin.H{"job": novelVideoJobResponse(job)})
			return
		}
		if job.Status == GenerationStatusQueued {
			queuedJobIDs = append(queuedJobIDs, job.ID)
		}
	}
	references, err := a.novelVideoAssetShotReferences(project, asset.ID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_asset_references_load_failed", "资产引用读取失败")
		return
	}
	if len(references) > 0 {
		writeNovelVideoAssetConflict(c, "novel_video_asset_in_use", "资产已被镜头引用，请先移除引用后再删除", gin.H{"references": references})
		return
	}
	finishedAt := time.Now()
	if err := a.db.Transaction(func(tx *gorm.DB) error {
		if len(queuedJobIDs) > 0 {
			if err := tx.Model(&NovelVideoJob{}).
				Where("id IN ? AND project_id = ? AND user_id = ? AND job_type = ? AND status = ?",
					queuedJobIDs,
					project.ID,
					project.UserID,
					NovelVideoJobTypeAssetImage,
					GenerationStatusQueued,
				).
				Updates(map[string]any{
					"status":        GenerationStatusFailed,
					"progress":      0,
					"error_code":    "user_cancelled",
					"error_message": "用户取消排队并删除资产",
					"finished_at":   &finishedAt,
				}).Error; err != nil {
				return err
			}
		}
		return tx.Delete(&asset).Error
	}); err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_asset_delete_failed", "资产删除失败")
		return
	}
	refreshedAssets, err := a.loadNovelVideoAssets(project)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_assets_load_failed", "资产列表读取失败")
		return
	}
	refreshedJobs, err := a.loadActiveNovelVideoAssetImageJobs(project)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_asset_jobs_load_failed", "资产任务读取失败")
		return
	}
	items := make([]gin.H, 0, len(refreshedAssets))
	for _, item := range refreshedAssets {
		items = append(items, novelVideoAssetResponse(item))
	}
	writeJSON(c, http.StatusOK, gin.H{
		"deleted_id": asset.ID,
		"items":      items,
		"jobs":       novelVideoJobResponses(refreshedJobs),
	})
}

func (a *App) handlePatchNovelVideoAsset(c *gin.Context) {
	project, ok := a.loadNovelVideoProjectForUser(c)
	if !ok {
		return
	}
	assetID, ok := uintParam(c, "asset_id")
	if !ok {
		writeError(c, http.StatusBadRequest, "invalid_asset_id", "资产 ID 无效")
		return
	}
	var asset NovelVideoAsset
	if err := a.db.Where("id = ? AND project_id = ? AND user_id = ?", assetID, project.ID, project.UserID).First(&asset).Error; err != nil {
		writeError(c, http.StatusNotFound, "novel_video_asset_not_found", "资产不存在")
		return
	}
	var req struct {
		Kind         *string        `json:"kind"`
		Name         *string        `json:"name"`
		Description  *string        `json:"description"`
		Prompt       *string        `json:"prompt"`
		ReferenceURL *string        `json:"reference_url"`
		AssetURL     *string        `json:"asset_url"`
		ReviewStatus *string        `json:"review_status"`
		Metadata     map[string]any `json:"metadata"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	if req.Kind != nil {
		kind := normalizeNovelVideoAssetKind(*req.Kind)
		if kind == "" {
			writeError(c, http.StatusBadRequest, "invalid_asset_kind", "不支持的资产类型")
			return
		}
		asset.Kind = kind
	}
	if req.Name != nil {
		asset.Name = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		asset.Description = strings.TrimSpace(*req.Description)
	}
	if req.Prompt != nil {
		asset.Prompt = strings.TrimSpace(*req.Prompt)
	}
	if req.ReferenceURL != nil {
		asset.ReferenceURL = strings.TrimSpace(*req.ReferenceURL)
	}
	if req.AssetURL != nil {
		asset.AssetURL = strings.TrimSpace(*req.AssetURL)
	}
	if req.ReviewStatus != nil {
		asset.ReviewStatus = normalizeNovelVideoReviewStatus(*req.ReviewStatus)
	}
	if req.Metadata != nil {
		asset.MetadataJSON = encodeJSON(req.Metadata)
	}
	if asset.Name == "" {
		writeError(c, http.StatusBadRequest, "asset_name_required", "资产名称不能为空")
		return
	}
	asset.Version = positiveOrDefault(asset.Version, 1)
	if err := a.db.Save(&asset).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_asset_save_failed", "资产保存失败")
		return
	}
	writeJSON(c, http.StatusOK, novelVideoAssetResponse(asset))
}

func (a *App) handleGenerateNovelVideoCreatureImage(c *gin.Context) {
	project, ok := a.loadNovelVideoProjectForUser(c)
	if !ok {
		return
	}
	creatureID, ok := uintParam(c, "creature_id")
	if !ok {
		writeError(c, http.StatusBadRequest, "invalid_creature_id", "生物 ID 无效")
		return
	}
	var creature NovelVideoCreature
	if err := a.db.Where("id = ? AND project_id = ? AND user_id = ?", creatureID, project.ID, project.UserID).First(&creature).Error; err != nil {
		writeError(c, http.StatusNotFound, "novel_video_creature_not_found", "生物卡不存在")
		return
	}
	prompt := strings.Join([]string{
		"小说视频生物设定细节图，单体展示，清晰外形结构，便于后续视频镜头保持一致。",
		"项目风格：" + project.StylePreset,
		"生物名称：" + creature.Name,
		"类型：" + creature.CreatureType,
		"外形：" + creature.Appearance,
		"能力与习性：" + creature.Abilities,
		"一致性提示：" + creature.VisualConsistencyPrompt,
	}, "\n")
	job, record, err := a.createNovelVideoImageGenerationRecord(project, prompt)
	if err != nil {
		writeError(c, http.StatusBadGateway, "creature_image_generation_failed", "生物设定图生成任务创建失败")
		return
	}
	creature.GenerationRecordID = &record.ID
	creature.ErrorCode = ""
	creature.ErrorMessage = ""
	creature.GenerationStatus = record.Status
	creature.LatestError = ""
	if err := a.db.Save(&creature).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_creature_save_failed", "生物卡保存失败")
		return
	}
	go a.runNovelVideoCreatureImageGeneration(project, creature.ID, record, job)
	writeJSON(c, http.StatusOK, novelVideoCreatureResponse(creature))
}

func (a *App) handlePlanNovelVideoEpisodes(c *gin.Context) {
	project, ok := a.loadNovelVideoProjectForUser(c)
	if !ok {
		return
	}
	plan := fallbackNovelVideoEpisodePlan(project)
	if deepSeekPlan, err := a.planNovelVideoEpisodesWithDeepSeek(c.Request.Context(), project); err == nil {
		plan = deepSeekPlan
	}
	err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("project_id = ?", project.ID).Delete(&NovelVideoShot{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ?", project.ID).Delete(&NovelVideoEpisode{}).Error; err != nil {
			return err
		}
		for _, episodeDraft := range plan {
			episode := NovelVideoEpisode{
				ProjectID: project.ID,
				UserID:    project.UserID,
				Number:    episodeDraft.Number,
				Title:     episodeDraft.Title,
				Summary:   episodeDraft.Summary,
				Status:    NovelVideoReviewStatusNeedsReview,
			}
			if err := tx.Create(&episode).Error; err != nil {
				return err
			}
			for _, shotDraft := range episodeDraft.Shots {
				shot := NovelVideoShot{
					ProjectID:       project.ID,
					EpisodeID:       episode.ID,
					UserID:          project.UserID,
					Number:          shotDraft.Number,
					Title:           shotDraft.Title,
					Prompt:          shotDraft.Prompt,
					VideoPrompt:     shotDraft.Prompt,
					VoiceoverText:   shotDraft.Title,
					DurationSeconds: effectiveNovelVideoShotDurationSeconds(project, NovelVideoShot{}),
					Status:          NovelVideoReviewStatusNeedsReview,
				}
				if err := tx.Create(&shot).Error; err != nil {
					return err
				}
			}
		}
		project.Status = NovelVideoProjectStatusPlanned
		return tx.Save(&project).Error
	})
	if err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_episode_plan_failed", "分集镜头规划失败")
		return
	}
	creatures, episodes, err := a.loadNovelVideoProjectChildren(project)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_project_load_failed", "小说视频项目读取失败")
		return
	}
	writeJSON(c, http.StatusOK, novelVideoProjectResponse(project, creatures, episodes))
}

func (a *App) handlePatchNovelVideoShot(c *gin.Context) {
	project, ok := a.loadNovelVideoProjectForUser(c)
	if !ok {
		return
	}
	shotID, ok := uintParam(c, "shot_id")
	if !ok {
		writeError(c, http.StatusBadRequest, "invalid_shot_id", "镜头 ID 无效")
		return
	}
	var shot NovelVideoShot
	if err := a.db.Where("id = ? AND project_id = ? AND user_id = ?", shotID, project.ID, project.UserID).First(&shot).Error; err != nil {
		writeError(c, http.StatusNotFound, "novel_video_shot_not_found", "镜头不存在")
		return
	}
	var req struct {
		Title                  *string                       `json:"title"`
		Prompt                 *string                       `json:"prompt"`
		ScriptUnitType         *string                       `json:"script_unit_type"`
		SourceExcerpt          *string                       `json:"source_excerpt"`
		DurationSeconds        *int                          `json:"duration_seconds"`
		ImagePrompt            *string                       `json:"image_prompt"`
		VideoPrompt            *string                       `json:"video_prompt"`
		VoiceoverText          *string                       `json:"voiceover_text"`
		AssetRefs              []novelVideoAssetRef          `json:"asset_refs"`
		AssetRefsSet           *bool                         `json:"asset_refs_set"`
		Status                 *string                       `json:"status"`
		ReferenceAssetID       *uint                         `json:"reference_asset_id"`
		ReferenceAssetIDs      []uint                        `json:"reference_asset_ids"`
		ReferenceVideoAssetIDs []uint                        `json:"reference_video_asset_ids"`
		ReferenceAudioAssetIDs []uint                        `json:"reference_audio_asset_ids"`
		GenerateAudio          *bool                         `json:"generate_audio"`
		GenerationSettings     *novelVideoGenerationSettings `json:"generation_settings"`
		CreatureIDs            []uint                        `json:"creature_ids"`
		CreatureIDsSet         *bool                         `json:"creature_ids_set"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	if req.Title != nil {
		shot.Title = strings.TrimSpace(*req.Title)
	}
	if req.Prompt != nil {
		shot.Prompt = strings.TrimSpace(*req.Prompt)
	}
	if req.ScriptUnitType != nil {
		shot.ScriptUnitType = strings.TrimSpace(*req.ScriptUnitType)
	}
	if req.SourceExcerpt != nil {
		shot.SourceExcerpt = strings.TrimSpace(*req.SourceExcerpt)
	}
	if req.DurationSeconds != nil {
		shot.DurationSeconds = *req.DurationSeconds
	}
	if req.ImagePrompt != nil {
		shot.ImagePrompt = strings.TrimSpace(*req.ImagePrompt)
	}
	if req.VideoPrompt != nil {
		shot.VideoPrompt = strings.TrimSpace(*req.VideoPrompt)
	}
	if req.VoiceoverText != nil {
		shot.VoiceoverText = strings.TrimSpace(*req.VoiceoverText)
	}
	if req.AssetRefsSet != nil || len(req.AssetRefs) > 0 {
		refs, err := a.validateNovelVideoShotAssetRefs(project, req.AssetRefs)
		if err != nil {
			writeError(c, http.StatusBadRequest, "invalid_asset_refs", err.Error())
			return
		}
		shot.AssetRefsJSON = encodeNovelVideoAssetRefs(refs)
	}
	if req.Status != nil {
		shot.Status = normalizeNovelVideoReviewStatus(*req.Status)
	}
	if req.ReferenceAssetID != nil {
		shot.ReferenceAssetID = req.ReferenceAssetID
	}
	settings := decodeNovelVideoGenerationSettings(shot.GenerationSettingsJSON)
	if req.GenerationSettings != nil {
		settings = normalizeNovelVideoGenerationSettings(*req.GenerationSettings)
	}
	if len(req.ReferenceAssetIDs) > 0 {
		settings.ReferenceAssetIDs = uniqueUintIDs(req.ReferenceAssetIDs)
	}
	if len(req.ReferenceVideoAssetIDs) > 0 {
		settings.ReferenceVideoAssetIDs = uniqueUintIDs(req.ReferenceVideoAssetIDs)
	}
	if len(req.ReferenceAudioAssetIDs) > 0 {
		settings.ReferenceAudioAssetIDs = uniqueUintIDs(req.ReferenceAudioAssetIDs)
	}
	if req.GenerateAudio != nil {
		settings.GenerateAudio = *req.GenerateAudio
	}
	if req.ReferenceAssetID != nil && *req.ReferenceAssetID != 0 && len(settings.ReferenceAssetIDs) == 0 {
		settings.ReferenceAssetIDs = []uint{*req.ReferenceAssetID}
	}
	settings = normalizeNovelVideoGenerationSettings(settings)
	if len(settings.ReferenceAssetIDs) > 0 || len(settings.ReferenceVideoAssetIDs) > 0 || len(settings.ReferenceAudioAssetIDs) > 0 || settings.GenerateAudio || settings.Model != "" || settings.Duration != "" || settings.Resolution != "" {
		shot.GenerationSettingsJSON = encodeNovelVideoGenerationSettings(settings)
	}
	if req.CreatureIDsSet != nil && *req.CreatureIDsSet {
		raw, _ := json.Marshal(req.CreatureIDs)
		shot.CreatureIDsJSON = string(raw)
	}
	if effectiveNovelVideoShotPrompt(shot) == "" {
		writeError(c, http.StatusBadRequest, "shot_prompt_required", "镜头提示词不能为空")
		return
	}
	if _, err := a.buildNovelVideoShotRequest(project, shot); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_generation_settings", err.Error())
		return
	}
	if err := a.db.Save(&shot).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_shot_save_failed", "镜头保存失败")
		return
	}
	writeJSON(c, http.StatusOK, novelVideoShotResponse(shot))
}

func (a *App) handleGenerateNovelVideoStoryboard(c *gin.Context) {
	project, ok := a.loadNovelVideoProjectForUser(c)
	if !ok {
		return
	}
	shotID, ok := uintParam(c, "shot_id")
	if !ok {
		writeError(c, http.StatusBadRequest, "invalid_shot_id", "镜头 ID 无效")
		return
	}
	var shot NovelVideoShot
	if err := a.db.Where("id = ? AND project_id = ? AND user_id = ?", shotID, project.ID, project.UserID).First(&shot).Error; err != nil {
		writeError(c, http.StatusNotFound, "novel_video_shot_not_found", "镜头不存在")
		return
	}
	shot.StoryboardStatus = GenerationStatusQueued
	job := NovelVideoJob{
		ProjectID:   project.ID,
		UserID:      project.UserID,
		JobType:     NovelVideoJobTypeStoryboard,
		Status:      GenerationStatusQueued,
		EpisodeID:   &shot.EpisodeID,
		ShotID:      &shot.ID,
		MaxAttempts: 3,
		PayloadJSON: encodeJSON(map[string]any{"shot_id": shot.ID, "prompt": buildNovelVideoStoryboardPrompt(project, shot)}),
	}
	if err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&shot).Error; err != nil {
			return err
		}
		return tx.Create(&job).Error
	}); err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_storyboard_queue_failed", "分镜图任务创建失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{"shot": novelVideoShotResponse(shot), "job": novelVideoJobResponse(job)})
}

func (a *App) handleGenerateNovelVideoShotImages(c *gin.Context) {
	a.handleGenerateNovelVideoShotImagesV2(c)
	return

	project, ok := a.loadNovelVideoProjectForUser(c)
	if !ok {
		return
	}
	var req struct {
		ShotIDs           []uint `json:"shot_ids"`
		CandidatesPerShot int    `json:"candidates_per_shot"`
		Mode              string `json:"mode"`
		LockLevel         string `json:"lock_level"`
		SourceWorkID      *uint  `json:"source_work_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	req.ShotIDs = uniqueUintIDs(req.ShotIDs)
	if len(req.ShotIDs) == 0 {
		writeError(c, http.StatusBadRequest, "shot_ids_required", "请选择镜头")
		return
	}
	if len(req.ShotIDs) > 20 {
		writeError(c, http.StatusBadRequest, "too_many_shots", "单次最多生成 20 个镜头")
		return
	}
	candidates := req.CandidatesPerShot
	if candidates <= 0 {
		candidates = 4
	}
	if candidates > 8 {
		candidates = 8
	}
	total := len(req.ShotIDs) * candidates
	if total > 80 {
		writeError(c, http.StatusBadRequest, "too_many_candidates", "单次最多生成 80 张候选图")
		return
	}
	mode := normalizeNovelVideoImageGenerationMode(req.Mode)
	lockLevel := normalizeNovelVideoActorLockLevel(req.LockLevel)
	var shots []NovelVideoShot
	if err := a.db.Where("project_id = ? AND user_id = ? AND id IN ?", project.ID, project.UserID, req.ShotIDs).Order("number asc, id asc").Find(&shots).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_shots_load_failed", "镜头读取失败")
		return
	}
	if len(shots) != len(req.ShotIDs) {
		writeError(c, http.StatusNotFound, "novel_video_shot_not_found", "镜头不存在")
		return
	}
	settings, err := a.loadSettings()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "settings_load_failed", "配置读取失败")
		return
	}
	candidatesForModel, _ := a.modelCenterCandidatesForGeneration(settings, ModelConfigTypeImage, 0)
	modelConfig, _ := a.modelConfigForGeneration(settings)
	user := User{ID: project.UserID}
	if err := a.db.First(&user, project.UserID).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "user_load_failed", "用户读取失败")
		return
	}
	styleStrength := 70
	referenceWeight := 85
	batchID := fmt.Sprintf("novel-image-%d-%d", project.ID, time.Now().UnixNano())
	images := make([]NovelVideoShotImage, 0, total)
	warnings := make([]string, 0)
	batchIndex := 0
	for _, shot := range shots {
		actorIDs := decodeUintList(shot.CreatureIDsJSON)
		refs, warning := a.novelVideoShotReferenceAssetIDs(project, shot, 4)
		if warning != "" {
			warnings = append(warnings, warning)
		}
		referenceAssets, err := a.loadNovelVideoReferenceAssets(project.UserID, refs, 4, referenceAssetKindImage)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "novel_video_images_generate_failed", "镜头图片候选创建失败")
			return
		}
		referenceIntent := novelVideoReferenceIntentForShot(actorIDs, refs)
		for i := 0; i < candidates; i++ {
			batchIndex++
			genReq := generationRequest{
				Prompt:            buildNovelVideoShotImagePrompt(project, shot, i+1),
				AspectRatio:       project.AspectRatio,
				Quality:           GenerationQualityHigh,
				StylePreset:       project.StylePreset,
				ToolMode:          GenerationToolModeGenerate,
				StyleStrength:     &styleStrength,
				ReferenceWeight:   &referenceWeight,
				ReferenceAssetIDs: refs,
				ReferenceIntent:   referenceIntent,
				SourceWorkID:      req.SourceWorkID,
				Num:               1,
				BatchID:           batchID,
				BatchIndex:        batchIndex,
				BatchTotal:        total,
			}
			if mode == "image_to_image" {
				genReq.ToolMode = GenerationToolModeRedraw
			}
			if genReq.AspectRatio == "" {
				genReq.AspectRatio = "16:9"
			}
			if size, ok := aspectRatioToSize(genReq.AspectRatio); ok {
				genReq.Size = size
			}
			job := &generationJob{
				User:                  user,
				Settings:              settings,
				ModelConfig:           modelConfig,
				ModelCenterCandidates: candidatesForModel,
				Request:               genReq,
				ReferenceAssets:       referenceAssets,
			}
			record, err := a.createGenerationRecord(job, GenerationStatusQueued, GenerationStageQueued)
			if err != nil {
				writeError(c, http.StatusInternalServerError, "novel_video_images_generate_failed", "镜头图片候选创建失败")
				return
			}
			image := NovelVideoShotImage{
				ProjectID:             project.ID,
				EpisodeID:             shot.EpisodeID,
				ShotID:                shot.ID,
				UserID:                project.UserID,
				GenerationRecordID:    &record.ID,
				Kind:                  NovelVideoAssetKindShotImage,
				Prompt:                genReq.Prompt,
				ReferenceAssetIDsJSON: encodeJSON(refs),
				ActorIDsJSON:          encodeJSON(actorIDs),
				ReferenceIntent:       referenceIntent,
				Mode:                  mode,
				LockLevel:             lockLevel,
				Version:               i + 1,
				ReviewStatus:          NovelVideoReviewStatusNeedsReview,
			}
			if err := a.db.Create(&image).Error; err != nil {
				writeError(c, http.StatusInternalServerError, "novel_video_images_generate_failed", "镜头图片候选创建失败")
				return
			}
			images = append(images, image)
		}
	}
	a.hydrateNovelVideoShotImages(images)
	writeJSON(c, http.StatusOK, gin.H{
		"queued":             len(images),
		"total_candidates":   total,
		"items":              novelVideoShotImageResponses(images),
		"reference_warnings": warnings,
	})
}

func (a *App) handleGenerateNovelVideoShotImagesV2(c *gin.Context) {
	project, ok := a.loadNovelVideoProjectForUser(c)
	if !ok {
		return
	}
	var req struct {
		ShotIDs           []uint `json:"shot_ids"`
		CandidatesPerShot int    `json:"candidates_per_shot"`
		Mode              string `json:"mode"`
		LockLevel         string `json:"lock_level"`
		SourceWorkID      *uint  `json:"source_work_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	req.ShotIDs = uniqueUintIDs(req.ShotIDs)
	if len(req.ShotIDs) == 0 {
		writeError(c, http.StatusBadRequest, "shot_ids_required", "请选择镜头")
		return
	}
	if len(req.ShotIDs) > 20 {
		writeError(c, http.StatusBadRequest, "too_many_shots", "单次最多生成 20 个镜头")
		return
	}
	candidates := req.CandidatesPerShot
	if candidates <= 0 {
		candidates = 4
	}
	if candidates > 8 {
		candidates = 8
	}
	total := len(req.ShotIDs) * candidates
	if total > 80 {
		writeError(c, http.StatusBadRequest, "too_many_candidates", "单次最多生成 80 张候选图")
		return
	}
	mode := normalizeNovelVideoImageGenerationMode(req.Mode)
	lockLevel := normalizeNovelVideoActorLockLevel(req.LockLevel)
	if mode == "image_to_image" && (req.SourceWorkID == nil || *req.SourceWorkID == 0) {
		writeError(c, http.StatusBadRequest, "source_work_required", "图生图重生需要选择源作品")
		return
	}

	var shots []NovelVideoShot
	if err := a.db.Where("project_id = ? AND user_id = ? AND id IN ?", project.ID, project.UserID, req.ShotIDs).Order("number asc, id asc").Find(&shots).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_shots_load_failed", "镜头读取失败")
		return
	}
	if len(shots) != len(req.ShotIDs) {
		writeError(c, http.StatusNotFound, "novel_video_shot_not_found", "镜头不存在")
		return
	}
	settings, err := a.loadSettings()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "settings_load_failed", "配置读取失败")
		return
	}
	candidatesForModel, _ := a.modelCenterCandidatesForGeneration(settings, ModelConfigTypeImage, 0)
	modelConfig, _ := a.modelConfigForGeneration(settings)
	user := User{ID: project.UserID}
	if err := a.db.First(&user, project.UserID).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "user_load_failed", "用户读取失败")
		return
	}
	var sourceWork *Work
	if req.SourceWorkID != nil && *req.SourceWorkID > 0 {
		var work Work
		if err := a.db.Where("id = ? AND user_id = ?", *req.SourceWorkID, project.UserID).First(&work).Error; err != nil {
			writeError(c, http.StatusNotFound, "source_work_not_found", "源作品不存在")
			return
		}
		if normalizeWorkCategory(work.Category) != WorkCategoryImage || strings.TrimSpace(work.AssetKey) == "" {
			writeError(c, http.StatusBadRequest, "invalid_source_work", "源作品不是可用于图生图的图片")
			return
		}
		sourceWork = &work
	}

	styleStrength := 70
	referenceWeight := 85
	batchID := fmt.Sprintf("novel-image-%d-%d", project.ID, time.Now().UnixNano())
	warnings := make([]string, 0)
	drafts := make([]novelVideoShotImageGenerationDraft, 0, total)
	batchIndex := 0
	requiredCredits := 0
	for _, shot := range shots {
		actorIDs := decodeUintList(shot.CreatureIDsJSON)
		refs, warning := a.novelVideoShotReferenceAssetIDs(project, shot, 4)
		if warning != "" {
			warnings = append(warnings, warning)
		}
		referenceAssets, err := a.loadNovelVideoReferenceAssets(project.UserID, refs, 4, referenceAssetKindImage)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "novel_video_images_generate_failed", "镜头图片候选创建失败")
			return
		}
		referenceIntent := novelVideoReferenceIntentForShot(actorIDs, refs)
		for i := 0; i < candidates; i++ {
			batchIndex++
			genReq := generationRequest{
				Prompt:            buildNovelVideoShotImagePrompt(project, shot, i+1),
				AspectRatio:       project.AspectRatio,
				Quality:           GenerationQualityHigh,
				StylePreset:       project.StylePreset,
				ToolMode:          GenerationToolModeGenerate,
				StyleStrength:     &styleStrength,
				ReferenceWeight:   &referenceWeight,
				ReferenceAssetIDs: refs,
				ReferenceIntent:   referenceIntent,
				SourceWorkID:      req.SourceWorkID,
				Num:               1,
				BatchID:           batchID,
				BatchIndex:        batchIndex,
				BatchTotal:        total,
			}
			if mode == "image_to_image" {
				genReq.ToolMode = GenerationToolModeRedraw
			}
			if genReq.AspectRatio == "" {
				genReq.AspectRatio = "16:9"
			}
			if size, ok := aspectRatioToSize(genReq.AspectRatio); ok {
				genReq.Size = size
			}
			job := &generationJob{
				User:                  user,
				Settings:              settings,
				ModelConfig:           modelConfig,
				ModelCenterCandidates: candidatesForModel,
				Request:               genReq,
				ReferenceAssets:       referenceAssets,
				SourceWork:            sourceWork,
			}
			requiredCredits += generationJobCreditCost(job)
			drafts = append(drafts, novelVideoShotImageGenerationDraft{
				Shot:                  shot,
				Request:               genReq,
				AppSettings:           settings,
				ModelConfig:           modelConfig,
				ModelCenterCandidates: candidatesForModel,
				ReferenceAssets:       referenceAssets,
				SourceWork:            sourceWork,
				ActorIDs:              actorIDs,
				ReferenceAssetIDs:     refs,
				ReferenceIntent:       referenceIntent,
				Mode:                  mode,
				LockLevel:             lockLevel,
				Version:               i + 1,
			})
		}
	}
	estimate, err := a.buildCreditEstimate(project.UserID, requiredCredits)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "balance_load_failed", "账户读取失败")
		return
	}
	if !estimate.Enough {
		writeCreditsInsufficientError(c, estimate)
		return
	}

	images := make([]NovelVideoShotImage, 0, len(drafts))
	tasks := make([]novelVideoShotImageGenerationTask, 0, len(drafts))
	for _, draft := range drafts {
		job := &generationJob{
			User:                  user,
			Settings:              draft.AppSettings,
			ModelConfig:           draft.ModelConfig,
			ModelCenterCandidates: draft.ModelCenterCandidates,
			Request:               draft.Request,
			ReferenceAssets:       draft.ReferenceAssets,
			SourceWork:            draft.SourceWork,
		}
		record, err := a.createGenerationRecord(job, GenerationStatusQueued, GenerationStageQueued)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "novel_video_images_generate_failed", "镜头图片候选创建失败")
			return
		}
		image := NovelVideoShotImage{
			ProjectID:             project.ID,
			EpisodeID:             draft.Shot.EpisodeID,
			ShotID:                draft.Shot.ID,
			UserID:                project.UserID,
			GenerationRecordID:    &record.ID,
			Kind:                  NovelVideoAssetKindShotImage,
			Prompt:                draft.Request.Prompt,
			ReferenceAssetIDsJSON: encodeJSON(draft.ReferenceAssetIDs),
			ActorIDsJSON:          encodeJSON(draft.ActorIDs),
			ReferenceIntent:       draft.ReferenceIntent,
			Mode:                  draft.Mode,
			LockLevel:             draft.LockLevel,
			Version:               draft.Version,
			ReviewStatus:          NovelVideoReviewStatusNeedsReview,
		}
		if err := a.db.Create(&image).Error; err != nil {
			writeError(c, http.StatusInternalServerError, "novel_video_images_generate_failed", "镜头图片候选创建失败")
			return
		}
		images = append(images, image)
		tasks = append(tasks, novelVideoShotImageGenerationTask{
			ImageID:  image.ID,
			Project:  project,
			Record:   record,
			Job:      job,
			SlotWait: 5 * time.Second,
		})
	}
	a.hydrateNovelVideoShotImages(images)
	writeJSON(c, http.StatusOK, gin.H{
		"queued":             len(images),
		"total_candidates":   total,
		"batch_id":           batchID,
		"items":              novelVideoShotImageResponses(images),
		"reference_warnings": warnings,
	})
	a.startNovelVideoShotImageGenerationBatch(tasks)
}

func (a *App) startNovelVideoShotImageGenerationBatch(tasks []novelVideoShotImageGenerationTask) {
	if len(tasks) == 0 {
		return
	}
	go func() {
		sem := make(chan struct{}, maxConcurrentImageGenerationsPerUser)
		for _, task := range tasks {
			task := task
			sem <- struct{}{}
			go func() {
				defer func() { <-sem }()
				a.runNovelVideoShotImageGenerationTask(task)
			}()
		}
	}()
}

func (a *App) runNovelVideoShotImageGenerationTask(task novelVideoShotImageGenerationTask) {
	record := task.Record
	defer func() {
		if recovered := recover(); recovered != nil {
			if record.LatencyMS <= 0 && !record.CreatedAt.IsZero() {
				record.LatencyMS = time.Since(record.CreatedAt).Milliseconds()
			}
			a.failGenerationRecord(&record, "shot_image_generation_panic", "镜头候选图生成异常中断，请重试")
			a.syncNovelVideoShotImageFromGenerationRecord(task, record)
		}
	}()

	slotKey, ok := a.acquireNovelVideoShotImageSlot(task.Project.UserID, task.SlotWait)
	if !ok {
		a.failGenerationRecord(&record, "generation_concurrency_limit", "并发生成任务过多，请稍后重试")
		a.syncNovelVideoShotImageFromGenerationRecord(task, record)
		return
	}
	defer a.imageGenLimiter.Release(slotKey)

	if _, providerErr, err := a.executeGenerationRecord(&record, task.Job); err != nil {
		if strings.TrimSpace(record.ErrorCode) == "" {
			a.failGenerationRecord(&record, "shot_image_generation_failed", "镜头候选图生成失败")
		}
	} else if providerErr != nil {
		// executeGenerationRecord has already persisted the public provider error.
	}
	a.syncNovelVideoShotImageFromGenerationRecord(task, record)
}

func (a *App) acquireNovelVideoShotImageSlot(userID uint, timeout time.Duration) (string, bool) {
	key := strconv.FormatUint(uint64(userID), 10)
	if a.imageGenLimiter.TryAcquire(key) {
		return key, true
	}
	if timeout <= 0 {
		return "", false
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline.C:
			return "", false
		case <-ticker.C:
			if a.imageGenLimiter.TryAcquire(key) {
				return key, true
			}
		}
	}
}

func (a *App) syncNovelVideoShotImageFromGenerationRecord(task novelVideoShotImageGenerationTask, record GenerationRecord) {
	if record.ID != 0 {
		var latest GenerationRecord
		if err := a.db.First(&latest, record.ID).Error; err == nil {
			record = latest
		}
	}
	updates := map[string]any{
		"error_code":    strings.TrimSpace(record.ErrorCode),
		"error_message": strings.TrimSpace(record.ErrorMessage),
	}
	if record.WorkID != nil {
		updates["work_id"] = record.WorkID
	}
	if record.Status == GenerationStatusSucceeded {
		updates["error_code"] = ""
		updates["error_message"] = ""
	}
	_ = a.db.Model(&NovelVideoShotImage{}).
		Where("id = ? AND project_id = ? AND user_id = ? AND generation_record_id = ?", task.ImageID, task.Project.ID, task.Project.UserID, record.ID).
		Updates(updates).Error
}

func (a *App) handleListNovelVideoShotImages(c *gin.Context) {
	project, ok := a.loadNovelVideoProjectForUser(c)
	if !ok {
		return
	}
	query := a.db.Where("project_id = ? AND user_id = ?", project.ID, project.UserID)
	if shotID, err := strconv.ParseUint(strings.TrimSpace(c.Query("shot_id")), 10, 64); err == nil && shotID > 0 {
		query = query.Where("shot_id = ?", uint(shotID))
	}
	var actorFilterID uint
	if actorID, err := strconv.ParseUint(strings.TrimSpace(c.Query("actor_id")), 10, 64); err == nil && actorID > 0 {
		actorFilterID = uint(actorID)
	}
	if status := strings.TrimSpace(c.Query("review_status")); status != "" {
		query = query.Where("review_status = ?", normalizeNovelVideoReviewStatus(status))
	}
	var images []NovelVideoShotImage
	if err := query.Order("shot_id asc, version asc, id asc").Find(&images).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_images_load_failed", "镜头图片读取失败")
		return
	}
	if actorFilterID > 0 {
		filtered := images[:0]
		for _, image := range images {
			if containsUint(decodeUintList(image.ActorIDsJSON), actorFilterID) {
				filtered = append(filtered, image)
			}
		}
		images = filtered
	}
	a.hydrateNovelVideoShotImages(images)
	writeJSON(c, http.StatusOK, gin.H{"items": novelVideoShotImageResponses(images)})
}

func (a *App) handlePatchNovelVideoShotImage(c *gin.Context) {
	project, ok := a.loadNovelVideoProjectForUser(c)
	if !ok {
		return
	}
	imageID, ok := uintParam(c, "image_id")
	if !ok {
		writeError(c, http.StatusBadRequest, "invalid_image_id", "图片候选 ID 无效")
		return
	}
	var image NovelVideoShotImage
	if err := a.db.Where("id = ? AND project_id = ? AND user_id = ?", imageID, project.ID, project.UserID).First(&image).Error; err != nil {
		writeError(c, http.StatusNotFound, "novel_video_image_not_found", "图片候选不存在")
		return
	}
	var req struct {
		Selected     *bool   `json:"selected"`
		ReviewStatus *string `json:"review_status"`
		ReviewNote   *string `json:"review_note"`
		ErrorCode    *string `json:"error_code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	if req.Selected != nil {
		image.Selected = *req.Selected
	}
	if req.ReviewStatus != nil {
		image.ReviewStatus = normalizeNovelVideoReviewStatus(*req.ReviewStatus)
	}
	if req.ReviewNote != nil {
		image.ReviewNote = strings.TrimSpace(*req.ReviewNote)
	}
	if req.ErrorCode != nil {
		image.ErrorCode = strings.TrimSpace(*req.ErrorCode)
	}
	err := a.db.Transaction(func(tx *gorm.DB) error {
		if image.Selected {
			if err := tx.Model(&NovelVideoShotImage{}).Where("project_id = ? AND shot_id = ? AND id <> ?", image.ProjectID, image.ShotID, image.ID).Update("selected", false).Error; err != nil {
				return err
			}
		}
		return tx.Save(&image).Error
	})
	if err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_image_save_failed", "图片候选保存失败")
		return
	}
	a.hydrateNovelVideoShotImages([]NovelVideoShotImage{image})
	writeJSON(c, http.StatusOK, novelVideoShotImageResponse(image))
}

func (a *App) handleGenerateNovelVideoGrids(c *gin.Context) {
	project, ok := a.loadNovelVideoProjectForUser(c)
	if !ok {
		return
	}
	var req struct {
		GridSize int `json:"grid_size"`
	}
	_ = c.ShouldBindJSON(&req)
	gridSize := normalizeNovelVideoGridSize(req.GridSize)
	if req.GridSize == 0 {
		gridSize = effectiveNovelVideoGridSize(project.GridSize)
	}
	var episodes []NovelVideoEpisode
	if err := a.db.Where("project_id = ? AND user_id = ?", project.ID, project.UserID).
		Order("number asc, id asc").
		Preload("Shots", func(db *gorm.DB) *gorm.DB {
			return db.Where("status = ?", NovelVideoReviewStatusApproved).Order("number asc, id asc")
		}).
		Find(&episodes).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_grids_load_failed", "grid shots load failed")
		return
	}
	grids := make([]NovelVideoGrid, 0)
	for _, episode := range episodes {
		for start := 0; start < len(episode.Shots); start += gridSize {
			end := start + gridSize
			if end > len(episode.Shots) {
				end = len(episode.Shots)
			}
			chunk := episode.Shots[start:end]
			shotIDs := make([]uint, 0, len(chunk))
			prompts := make([]gin.H, 0, len(chunk))
			for _, shot := range chunk {
				shotIDs = append(shotIDs, shot.ID)
				prompts = append(prompts, gin.H{
					"shot_id":        shot.ID,
					"shot_number":    shot.Number,
					"title":          shot.Title,
					"image_prompt":   shot.ImagePrompt,
					"video_prompt":   effectiveNovelVideoShotPrompt(shot),
					"voiceover_text": shot.VoiceoverText,
				})
			}
			episodeID := episode.ID
			grids = append(grids, NovelVideoGrid{
				ProjectID:   project.ID,
				UserID:      project.UserID,
				EpisodeID:   &episodeID,
				GridType:    fmt.Sprintf("grid_%d", gridSize),
				GridSize:    gridSize,
				ShotIDsJSON: encodeJSON(shotIDs),
				PromptJSON:  encodeJSON(prompts),
				Status:      NovelVideoReviewStatusNeedsReview,
			})
		}
	}
	if err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("project_id = ? AND user_id = ?", project.ID, project.UserID).Delete(&NovelVideoGrid{}).Error; err != nil {
			return err
		}
		for index := range grids {
			if err := tx.Create(&grids[index]).Error; err != nil {
				return err
			}
		}
		project.GenerationMode = NovelVideoGenerationModeGrid
		project.GridSize = gridSize
		return tx.Save(&project).Error
	}); err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_grids_generate_failed", "grid generation failed")
		return
	}
	items := make([]gin.H, 0, len(grids))
	for _, grid := range grids {
		items = append(items, novelVideoGridResponse(grid))
	}
	writeJSON(c, http.StatusOK, gin.H{"items": items, "grid_size": gridSize})
}

func (a *App) handleNovelVideoCostEstimate(c *gin.Context) {
	project, ok := a.loadNovelVideoProjectForUser(c)
	if !ok {
		return
	}
	var episodes []NovelVideoEpisode
	if err := a.db.Where("project_id = ? AND user_id = ?", project.ID, project.UserID).
		Order("number asc, id asc").
		Preload("Shots", func(db *gorm.DB) *gorm.DB {
			return db.Order("number asc, id asc")
		}).
		Find(&episodes).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_cost_estimate_failed", "cost estimate failed")
		return
	}
	var grids []NovelVideoGrid
	_ = a.db.Where("project_id = ? AND user_id = ?", project.ID, project.UserID).Order("id asc").Find(&grids).Error
	writeJSON(c, http.StatusOK, buildNovelVideoCostEstimatePayload(project, episodes, grids))
}

func (a *App) handleNovelVideoRenderPreflight(c *gin.Context) {
	project, ok := a.loadNovelVideoProjectForUser(c)
	if !ok {
		return
	}
	preflight, _, err := a.buildNovelVideoRenderPreflight(project)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_preflight_failed", "小说视频预检失败")
		return
	}
	writeJSON(c, http.StatusOK, preflight)
}

func (a *App) handleQueueNovelVideoRenderJobs(c *gin.Context) {
	project, ok := a.loadNovelVideoProjectForUser(c)
	if !ok {
		return
	}
	preflight, shots, err := a.buildNovelVideoRenderPreflight(project)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_render_preflight_failed", "镜头渲染预检失败")
		return
	}
	if preflight.Blocked > 0 || !preflight.Enough {
		writeJSON(c, http.StatusOK, preflight)
		return
	}
	jobs := make([]NovelVideoJob, 0, len(shots))
	err = a.db.Transaction(func(tx *gorm.DB) error {
		project.Status = NovelVideoProjectStatusRendering
		if err := tx.Save(&project).Error; err != nil {
			return err
		}
		for index := range shots {
			shot := shots[index]
			job := NovelVideoJob{
				ProjectID:   project.ID,
				UserID:      project.UserID,
				JobType:     NovelVideoJobTypeShotVideo,
				Status:      GenerationStatusQueued,
				EpisodeID:   &shot.EpisodeID,
				ShotID:      &shot.ID,
				MaxAttempts: 3,
				PayloadJSON: encodeJSON(map[string]any{"shot_id": shot.ID, "episode_id": shot.EpisodeID, "prompt": effectiveNovelVideoShotPrompt(shot)}),
			}
			if err := tx.Create(&job).Error; err != nil {
				return err
			}
			jobs = append(jobs, job)
		}
		return nil
	})
	if err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_render_queue_failed", "镜头渲染任务入队失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{
		"status":            GenerationStatusQueued,
		"queued":            len(jobs),
		"skipped":           preflight.Skipped,
		"required_credits":  preflight.RequiredCredits,
		"available_credits": preflight.AvailableCredits,
		"jobs":              novelVideoJobResponses(jobs),
	})
}

func (a *App) novelVideoComposeClips(project NovelVideoProject, episodes []NovelVideoEpisode) ([]novelVideoComposeClip, error) {
	clips := make([]novelVideoComposeClip, 0)
	for _, episode := range episodes {
		for _, shot := range episode.Shots {
			if shot.WorkID == nil || shot.Status != GenerationStatusSucceeded {
				continue
			}
			var work Work
			if err := a.db.Where("id = ? AND user_id = ?", *shot.WorkID, project.UserID).First(&work).Error; err != nil {
				return nil, fmt.Errorf("shot %d work not found", shot.ID)
			}
			if strings.TrimSpace(work.AssetKey) == "" {
				return nil, fmt.Errorf("shot %d rendered clip missing asset key", shot.ID)
			}
			clips = append(clips, novelVideoComposeClip{Episode: episode, Shot: shot, Work: work})
		}
	}
	if len(clips) == 0 {
		return nil, errors.New("no succeeded rendered shots")
	}
	return clips, nil
}

func (a *App) handleComposeNovelVideoProject(c *gin.Context) {
	project, ok := a.loadNovelVideoProjectForUser(c)
	if !ok {
		return
	}
	_, episodes, err := a.loadNovelVideoProjectChildren(project)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_project_load_failed", "小说视频项目读取失败")
		return
	}
	job := NovelVideoJob{
		ProjectID:   project.ID,
		UserID:      project.UserID,
		JobType:     NovelVideoJobTypeCompose,
		Status:      GenerationStatusRunning,
		MaxAttempts: 1,
		Progress:    10,
	}
	composition := NovelVideoComposition{
		ProjectID: project.ID,
		UserID:    project.UserID,
		Status:    GenerationStatusRunning,
	}
	err = a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&job).Error; err != nil {
			return err
		}
		composition.JobID = &job.ID
		return tx.Create(&composition).Error
	})
	if err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_compose_queue_failed", "合成任务创建失败")
		return
	}
	runner := a.novelVideoFFmpegRunner
	if runner == nil {
		runner = executableFFmpegRunner{}
	}
	clips, clipErr := a.novelVideoComposeClips(project, episodes)
	result, runErr := runner.ComposeNovelVideo(c.Request.Context(), project, clips, a.assetStore)
	if runErr == nil && clipErr != nil {
		runErr = clipErr
	}
	if runErr != nil {
		now := time.Now()
		job.Status = GenerationStatusFailed
		job.Progress = 0
		job.ErrorCode = novelVideoComposeErrorCode(runErr)
		job.ErrorMessage = runErr.Error()
		job.FinishedAt = &now
		composition.Status = GenerationStatusFailed
		composition.ErrorCode = job.ErrorCode
		composition.ErrorMessage = runErr.Error()
		_ = a.db.Save(&job).Error
		_ = a.db.Save(&composition).Error
		status := http.StatusInternalServerError
		if job.ErrorCode == "ffmpeg_unavailable" {
			status = http.StatusServiceUnavailable
		}
		writeError(c, status, job.ErrorCode, runErr.Error())
		return
	}
	now := time.Now()
	if len(result.OutputBytes) > 0 {
		key, mimeType, saveErr := a.assetStore.SaveBytes(result.OutputBytes, "video/mp4")
		if saveErr != nil {
			job.Status = GenerationStatusFailed
			job.Progress = 0
			job.ErrorCode = "novel_video_compose_store_failed"
			job.ErrorMessage = saveErr.Error()
			job.FinishedAt = &now
			composition.Status = GenerationStatusFailed
			composition.ErrorCode = job.ErrorCode
			composition.ErrorMessage = saveErr.Error()
			_ = a.db.Save(&job).Error
			_ = a.db.Save(&composition).Error
			writeError(c, http.StatusInternalServerError, job.ErrorCode, saveErr.Error())
			return
		}
		publicURL := a.assetStore.PublicURL(key)
		work := Work{
			UserID:      project.UserID,
			Prompt:      project.Title,
			AspectRatio: project.AspectRatio,
			Category:    WorkCategoryVideo,
			Model:       "ffmpeg",
			Status:      GenerationStatusSucceeded,
			Visibility:  WorkVisibilityPrivate,
			AssetKey:    key,
			PreviewURL:  publicURL,
			DownloadURL: publicURL,
			MIMEType:    mimeType,
		}
		if err := a.db.Create(&work).Error; err != nil {
			writeError(c, http.StatusInternalServerError, "novel_video_compose_work_create_failed", err.Error())
			return
		}
		if work.PreviewURL == "" {
			work.PreviewURL = fmt.Sprintf("/api/works/%d/file", work.ID)
			work.DownloadURL = fmt.Sprintf("/api/works/%d/download", work.ID)
			_ = a.db.Save(&work).Error
		}
		result.OutputURL = fallbackString(result.OutputURL, work.PreviewURL)
		composition.WorkID = &work.ID
	}
	if strings.TrimSpace(result.SubtitleText) != "" {
		if key, _, saveErr := a.assetStore.SaveBytes([]byte(result.SubtitleText), "text/plain"); saveErr == nil {
			composition.SubtitleURL = fallbackString(result.SubtitleURL, a.assetStore.PublicURL(key))
		}
	}
	job.Status = GenerationStatusSucceeded
	job.Progress = 100
	job.ResultJSON = encodeJSON(result)
	job.FinishedAt = &now
	composition.Status = GenerationStatusSucceeded
	composition.OutputURL = result.OutputURL
	composition.SubtitleURL = fallbackString(composition.SubtitleURL, result.SubtitleURL)
	composition.ManifestJSON = result.ManifestJSON
	_ = a.db.Save(&job).Error
	_ = a.db.Save(&composition).Error
	writeJSON(c, http.StatusOK, novelVideoCompositionResponse(composition))
}

func (a *App) handleListNovelVideoCompositions(c *gin.Context) {
	project, ok := a.loadNovelVideoProjectForUser(c)
	if !ok {
		return
	}
	var items []NovelVideoComposition
	if err := a.db.Where("project_id = ? AND user_id = ?", project.ID, project.UserID).Order("id desc").Find(&items).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_compositions_load_failed", "合成记录读取失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{"items": novelVideoCompositionResponses(items)})
}

func (a *App) handleNovelVideoProjectEvents(c *gin.Context) {
	project, ok := a.loadNovelVideoProjectForUser(c)
	if !ok {
		return
	}
	var jobs []NovelVideoJob
	_ = a.db.Where("project_id = ? AND user_id = ?", project.ID, project.UserID).Order("updated_at desc, id desc").Limit(50).Find(&jobs).Error
	writeJSON(c, http.StatusOK, gin.H{"project_id": project.ID, "items": novelVideoJobResponses(jobs)})
}

func (a *App) handleRestoreNovelVideoVersion(c *gin.Context) {
	project, ok := a.loadNovelVideoProjectForUser(c)
	if !ok {
		return
	}
	versionID, ok := uintParam(c, "version_id")
	if !ok {
		writeError(c, http.StatusBadRequest, "invalid_version_id", "版本 ID 无效")
		return
	}
	var version NovelVideoVersion
	if err := a.db.Where("id = ? AND project_id = ? AND user_id = ?", versionID, project.ID, project.UserID).First(&version).Error; err != nil {
		writeError(c, http.StatusNotFound, "novel_video_version_not_found", "版本不存在")
		return
	}
	snapshot := decodeJSONMap(version.SnapshotJSON)
	if title, ok := snapshot["title"].(string); ok && strings.TrimSpace(title) != "" {
		project.Title = strings.TrimSpace(title)
	}
	if story, ok := snapshot["story_bible"].(map[string]any); ok {
		project.StoryBibleJSON = encodeJSON(story)
	}
	project.Status = NovelVideoProjectStatusPlanned
	if err := a.db.Save(&project).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_version_restore_failed", "版本恢复失败")
		return
	}
	creatures, episodes, err := a.loadNovelVideoProjectChildren(project)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_project_load_failed", "小说视频项目读取失败")
		return
	}
	writeJSON(c, http.StatusOK, novelVideoProjectResponse(project, creatures, episodes))
}

func (a *App) handleRenderNovelVideoApprovedShots(c *gin.Context) {
	project, ok := a.loadNovelVideoProjectForUser(c)
	if !ok {
		return
	}
	preflight, shots, err := a.buildNovelVideoRenderPreflight(project)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_shots_load_failed", "镜头读取失败")
		return
	}
	requiredCredits := preflight.RequiredCredits
	approvedCount := preflight.Total
	if preflight.Blocked > 0 {
		writeJSON(c, http.StatusUnprocessableEntity, preflight)
		return
	}
	if len(shots) == 0 {
		writeJSON(c, http.StatusOK, gin.H{"status": GenerationStatusSucceeded, "queued": 0, "failed": 0, "skipped": approvedCount, "total": approvedCount})
		return
	}
	if !preflight.Enough {
		writeCreditsInsufficientError(c, creditEstimatePayload{
			RequiredCredits:  requiredCredits,
			AvailableCredits: preflight.AvailableCredits,
			MissingCredits:   preflight.MissingCredits,
			Enough:           preflight.Enough,
		})
		return
	}
	lockKey, ok := a.acquireGenerationLock(c, project.UserID)
	if !ok {
		return
	}

	attempts := make([]NovelVideoShotRenderAttempt, 0, len(shots))
	if err := a.db.Transaction(func(tx *gorm.DB) error {
		project.Status = NovelVideoProjectStatusRendering
		project.ErrorCode = ""
		project.ErrorMessage = ""
		if err := tx.Save(&project).Error; err != nil {
			return err
		}
		for index := range shots {
			shots[index].Status = GenerationStatusQueued
			shots[index].ErrorCode = ""
			shots[index].ErrorMessage = ""
			if err := tx.Save(&shots[index]).Error; err != nil {
				return err
			}
			attempts = append(attempts, NovelVideoShotRenderAttempt{
				ProjectID: project.ID,
				EpisodeID: shots[index].EpisodeID,
				ShotID:    shots[index].ID,
				UserID:    project.UserID,
				Status:    GenerationStatusQueued,
				Progress:  0,
			})
		}
		return tx.Create(&attempts).Error
	}); err != nil {
		a.concurrencyLimiter.Release(lockKey)
		writeError(c, http.StatusInternalServerError, "novel_video_render_queue_failed", "镜头渲染队列创建失败")
		return
	}

	go a.runQueuedNovelVideoShots(project, shots, attempts, lockKey)
	writeJSON(c, http.StatusOK, gin.H{
		"status":            GenerationStatusQueued,
		"queued":            len(shots),
		"skipped":           approvedCount - len(shots),
		"required_credits":  requiredCredits,
		"available_credits": preflight.AvailableCredits,
		"total":             len(shots),
	})
}

func (a *App) handleExportNovelVideoProject(c *gin.Context) {
	project, ok := a.loadNovelVideoProjectForUser(c)
	if !ok {
		return
	}
	creatures, episodes, err := a.loadNovelVideoProjectChildren(project)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "novel_video_project_load_failed", "小说视频项目读取失败")
		return
	}
	assets, _ := a.loadNovelVideoAssets(project)
	switch strings.ToLower(strings.TrimSpace(c.Query("format"))) {
	case "json":
		response := novelVideoProjectResponse(project, creatures, episodes)
		assetItems := make([]gin.H, 0, len(assets))
		for _, asset := range assets {
			assetItems = append(assetItems, novelVideoAssetResponse(asset))
		}
		response["assets"] = assetItems
		writeJSON(c, http.StatusOK, response)
		return
	case "zip":
		data, err := buildNovelVideoExportZip(project, creatures, episodes, assets)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "novel_video_export_failed", "导出包生成失败")
			return
		}
		c.Header("Content-Type", "application/zip")
		c.Header("Content-Disposition", `attachment; filename="novel-video-package.zip"`)
		c.Data(http.StatusOK, "application/zip", data)
		return
	case "jianying":
		data, err := buildNovelVideoJianyingZip(project, creatures, episodes, assets)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "novel_video_export_failed", "剪映草稿包生成失败")
			return
		}
		c.Header("Content-Type", "application/zip")
		c.Header("Content-Disposition", `attachment; filename="jianying-draft.zip"`)
		c.Data(http.StatusOK, "application/zip", data)
		return
	case "image_package":
		var images []NovelVideoShotImage
		_ = a.db.Where("project_id = ? AND user_id = ?", project.ID, project.UserID).Order("shot_id asc, version asc, id asc").Find(&images).Error
		a.hydrateNovelVideoShotImages(images)
		data, err := buildNovelVideoImagePackageZip(project, creatures, episodes, assets, images)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "novel_video_export_failed", "图片包生成失败")
			return
		}
		c.Header("Content-Type", "application/zip")
		c.Header("Content-Disposition", `attachment; filename="novel-video-image-package.zip"`)
		c.Data(http.StatusOK, "application/zip", data)
		return
	}
	c.Header("Content-Type", "text/markdown; charset=utf-8")
	c.String(http.StatusOK, buildNovelVideoExportMarkdown(project, creatures, episodes))
}

func (a *App) buildNovelVideoRenderPreflight(project NovelVideoProject) (novelVideoRenderPreflight, []NovelVideoShot, error) {
	var episodes []NovelVideoEpisode
	if err := a.db.Where("project_id = ? AND user_id = ?", project.ID, project.UserID).
		Order("number asc, id asc").
		Preload("Shots", func(db *gorm.DB) *gorm.DB {
			return db.Where("status = ?", NovelVideoReviewStatusApproved).Order("number asc, id asc")
		}).
		Find(&episodes).Error; err != nil {
		return novelVideoRenderPreflight{}, nil, err
	}

	preflight := novelVideoRenderPreflight{
		Status: GenerationStatusQueued,
		Shots:  make([]novelVideoRenderPreflightShot, 0),
	}
	renderableShots := make([]NovelVideoShot, 0)
	for _, episode := range episodes {
		for _, shot := range episode.Shots {
			preflight.Total++
			if shot.WorkID != nil && shot.Status == GenerationStatusSucceeded {
				preflight.Skipped++
				continue
			}
			item := novelVideoRenderPreflightShot{
				ShotID:        shot.ID,
				EpisodeID:     shot.EpisodeID,
				EpisodeNumber: episode.Number,
				ShotNumber:    shot.Number,
				Number:        shot.Number,
				Title:         shot.Title,
			}
			plan, err := a.prepareNovelVideoShotGenerationPlan(project, shot, false)
			if err != nil {
				item.CanRender = false
				item.BlockReasons = []string{err.Error()}
				item.BlockedReason = err.Error()
				item.EffectiveSettings = effectiveNovelVideoGenerationSettings(project, shot)
				preflight.Blocked++
				preflight.Shots = append(preflight.Shots, item)
				continue
			}
			item.Request = plan.Request
			item.CanRender = true
			item.RequiredCredits = videoCreditCost(plan.Request)
			item.EffectiveSettings = novelVideoSettingsFromRequest(plan.Request)
			preflight.RequiredCredits += item.RequiredCredits
			preflight.Renderable++
			preflight.Shots = append(preflight.Shots, item)
			renderableShots = append(renderableShots, shot)
		}
	}
	estimate, err := a.buildCreditEstimate(project.UserID, preflight.RequiredCredits)
	if err != nil {
		return novelVideoRenderPreflight{}, nil, err
	}
	preflight.AvailableCredits = estimate.AvailableCredits
	preflight.MissingCredits = estimate.MissingCredits
	preflight.Enough = estimate.Enough
	if preflight.Blocked > 0 {
		preflight.Status = "blocked"
	}
	return preflight, renderableShots, nil
}

func (a *App) runQueuedNovelVideoShots(project NovelVideoProject, shots []NovelVideoShot, attempts []NovelVideoShotRenderAttempt, lockKey string) {
	defer a.concurrencyLimiter.Release(lockKey)

	rendered := 0
	failed := 0
	for index := range shots {
		shot := shots[index]
		attempt := attempts[index]
		attempt.Status = GenerationStatusRunning
		attempt.Progress = 65
		_ = a.db.Save(&attempt).Error
		shot.Status = GenerationStatusRunning
		_ = a.db.Save(&shot).Error

		record, runErr := a.runNovelVideoShotGeneration(project, shot, attempt)
		if runErr != nil {
			failed++
			attempt.Status = GenerationStatusFailed
			attempt.Progress = 0
			attempt.GenerationRecordID = nil
			attempt.ErrorCode = "shot_video_generation_failed"
			attempt.ErrorMessage = runErr.Error()
			_ = a.db.Save(&attempt).Error
			shot.Status = GenerationStatusFailed
			shot.ErrorCode = fallbackString(shot.ErrorCode, "shot_video_generation_failed")
			shot.ErrorMessage = fallbackString(shot.ErrorMessage, "镜头视频生成失败")
			_ = a.db.Save(&shot).Error
			continue
		}

		rendered++
		attempt.Status = GenerationStatusSucceeded
		attempt.Progress = 100
		attempt.GenerationRecordID = &record.ID
		attempt.ErrorCode = ""
		attempt.ErrorMessage = ""
		_ = a.db.Save(&attempt).Error
		shot.GenerationRecordID = &record.ID
		shot.WorkID = record.WorkID
		shot.Status = GenerationStatusSucceeded
		shot.ErrorCode = ""
		shot.ErrorMessage = ""
		_ = a.db.Save(&shot).Error
	}

	if rendered > 0 && failed > 0 {
		project.Status = NovelVideoProjectStatusPartial
	} else if failed > 0 {
		project.Status = NovelVideoProjectStatusFailed
	} else {
		project.Status = NovelVideoProjectStatusSucceeded
	}
	_ = a.db.Save(&project).Error
}

func (a *App) createNovelVideoImageGenerationRecord(project NovelVideoProject, prompt string) (*generationJob, GenerationRecord, error) {
	settings, err := a.loadSettings()
	if err != nil {
		return nil, GenerationRecord{}, err
	}
	size, ok := aspectRatioToSize("1:1")
	if !ok {
		size = "1024x1024"
	}
	styleStrength := 70
	referenceWeight := 70
	req := generationRequest{
		Prompt:          strings.TrimSpace(prompt),
		AspectRatio:     "1:1",
		Quality:         GenerationQualityHigh,
		StylePreset:     project.StylePreset,
		ToolMode:        GenerationToolModeGenerate,
		StyleStrength:   &styleStrength,
		ReferenceWeight: &referenceWeight,
		Num:             1,
		Size:            size,
	}
	user := User{ID: project.UserID}
	if err := a.db.First(&user, project.UserID).Error; err != nil {
		return nil, GenerationRecord{}, err
	}
	candidates, err := a.modelCenterCandidatesForGeneration(settings, ModelConfigTypeImage, 0)
	if err != nil {
		return nil, GenerationRecord{}, err
	}
	job := &generationJob{User: user, Settings: settings, Request: req, ModelCenterCandidates: candidates}
	estimate, err := a.buildCreditEstimate(project.UserID, generationJobRequiredCredits(job))
	if err != nil {
		return nil, GenerationRecord{}, err
	}
	if !estimate.Enough {
		return nil, GenerationRecord{}, errCreditsInsufficient
	}
	record, err := a.createGenerationRecord(job, GenerationStatusQueued, GenerationStageQueued)
	if err != nil {
		return nil, GenerationRecord{}, err
	}
	return job, record, nil
}

func (a *App) runNovelVideoImageGeneration(project NovelVideoProject, prompt string) (*generationTaskResult, GenerationRecord, error) {
	job, record, err := a.createNovelVideoImageGenerationRecord(project, prompt)
	if err != nil {
		return nil, GenerationRecord{}, err
	}
	result, providerErr, err := a.executeGenerationRecord(&record, job)
	if providerErr != nil {
		return nil, record, errors.New(providerErr.Message)
	}
	return result, record, err
}

func (a *App) runNovelVideoCreatureImageGeneration(project NovelVideoProject, creatureID uint, record GenerationRecord, job *generationJob) {
	failCreature := func(code, message string) {
		code = fallbackString(strings.TrimSpace(code), "creature_image_generation_failed")
		message = fallbackString(strings.TrimSpace(message), "生物设定图生成失败")
		_ = a.db.Model(&NovelVideoCreature{}).
			Where("id = ? AND project_id = ? AND user_id = ? AND generation_record_id = ?", creatureID, project.ID, project.UserID, record.ID).
			Updates(map[string]any{
				"error_code":    code,
				"error_message": message,
			}).Error
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			if record.LatencyMS <= 0 && !record.CreatedAt.IsZero() {
				record.LatencyMS = time.Since(record.CreatedAt).Milliseconds()
			}
			a.failGenerationRecord(&record, "creature_image_generation_panic", "生物设定图生成异常中断，请重新生成")
			failCreature(record.ErrorCode, record.ErrorMessage)
		}
	}()

	result, providerErr, err := a.executeGenerationRecord(&record, job)
	if providerErr != nil {
		failCreature(record.ErrorCode, fallbackString(record.ErrorMessage, providerErr.Message))
		return
	}
	if err != nil {
		if strings.TrimSpace(record.ErrorCode) == "" {
			a.failGenerationRecord(&record, "creature_image_generation_failed", "生物设定图生成失败")
		}
		failCreature(record.ErrorCode, fallbackString(record.ErrorMessage, err.Error()))
		return
	}
	if record.ID == 0 || record.WorkID == nil || strings.TrimSpace(record.PreviewURL) == "" {
		a.failGenerationRecord(&record, "creature_image_generation_failed", "生物设定图生成结果缺失")
		failCreature(record.ErrorCode, record.ErrorMessage)
		return
	}
	if result != nil && result.AvailableCredits >= 0 {
		// Keep result referenced so future response changes can expose credits without rerunning generation.
	}
	err = a.db.Transaction(func(tx *gorm.DB) error {
		var creature NovelVideoCreature
		if err := tx.Where("id = ? AND project_id = ? AND user_id = ? AND generation_record_id = ?", creatureID, project.ID, project.UserID, record.ID).First(&creature).Error; err != nil {
			return err
		}
		if err := tx.Model(&creature).Updates(map[string]any{
			"work_id":       record.WorkID,
			"asset_url":     record.PreviewURL,
			"error_code":    "",
			"error_message": "",
		}).Error; err != nil {
			return err
		}
		creature.WorkID = record.WorkID
		creature.AssetURL = record.PreviewURL
		creature.ErrorCode = ""
		creature.ErrorMessage = ""
		return a.syncNovelVideoCreatureActorRefAsset(tx, project, creature, record)
	})
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		failCreature("creature_image_asset_sync_failed", err.Error())
	}
}

func (a *App) syncNovelVideoCreatureActorRefAsset(tx *gorm.DB, project NovelVideoProject, creature NovelVideoCreature, record GenerationRecord) error {
	if record.ID == 0 || record.WorkID == nil || strings.TrimSpace(record.PreviewURL) == "" {
		return nil
	}
	asset, found, err := a.findNovelVideoCreatureActorRefAsset(tx, project, creature)
	if err != nil {
		return err
	}
	if !found {
		asset = NovelVideoAsset{
			ProjectID:    project.ID,
			UserID:       project.UserID,
			Kind:         NovelVideoAssetKindActorRef,
			Name:         strings.TrimSpace(creature.Name) + "参考图",
			Description:  "演员身份参考图，由演员设定图生成结果同步。",
			Version:      1,
			ReviewStatus: fallbackString(strings.TrimSpace(creature.ReviewStatus), NovelVideoReviewStatusNeedsReview),
		}
	}
	if strings.TrimSpace(asset.Name) == "" {
		asset.Name = strings.TrimSpace(creature.Name) + "参考图"
	}
	if strings.TrimSpace(asset.Description) == "" {
		asset.Description = "演员身份参考图，由演员设定图生成结果同步。"
	}
	if strings.TrimSpace(asset.ReviewStatus) == "" {
		asset.ReviewStatus = fallbackString(strings.TrimSpace(creature.ReviewStatus), NovelVideoReviewStatusNeedsReview)
	}
	asset.ProjectID = project.ID
	asset.UserID = project.UserID
	asset.Kind = NovelVideoAssetKindActorRef
	asset.Prompt = record.Prompt
	asset.AssetURL = record.PreviewURL
	asset.WorkID = record.WorkID
	asset.GenerationRecordID = &record.ID
	asset.Version = positiveOrDefault(asset.Version, 1)
	asset.ErrorCode = ""
	asset.ErrorMessage = ""
	asset.MetadataJSON = encodeJSON(mergeNovelVideoCreatureActorMetadata(asset.MetadataJSON, creature, record))
	return tx.Save(&asset).Error
}

func (a *App) findNovelVideoCreatureActorRefAsset(tx *gorm.DB, project NovelVideoProject, creature NovelVideoCreature) (NovelVideoAsset, bool, error) {
	var assets []NovelVideoAsset
	if err := tx.Where("project_id = ? AND user_id = ? AND kind = ?", project.ID, project.UserID, NovelVideoAssetKindActorRef).Order("id asc").Find(&assets).Error; err != nil {
		return NovelVideoAsset{}, false, err
	}
	for _, asset := range assets {
		if uintFromAny(decodeJSONMap(asset.MetadataJSON)["actor_id"]) == creature.ID {
			return asset, true, nil
		}
	}
	creatureName := strings.TrimSpace(creature.Name)
	for _, asset := range assets {
		assetName := strings.TrimSpace(asset.Name)
		if creatureName != "" && (assetName == creatureName || strings.HasPrefix(assetName, creatureName)) {
			return asset, true, nil
		}
	}
	return NovelVideoAsset{}, false, nil
}

func mergeNovelVideoCreatureActorMetadata(raw string, creature NovelVideoCreature, record GenerationRecord) map[string]any {
	metadata := decodeJSONMap(raw)
	if metadata == nil {
		metadata = map[string]any{}
	}
	if _, ok := metadata["source"]; !ok {
		metadata["source"] = "creature_image"
	}
	metadata["actor_id"] = creature.ID
	metadata["creature_type"] = creature.CreatureType
	metadata["appearance"] = creature.Appearance
	metadata["abilities"] = creature.Abilities
	metadata["visual_consistency_prompt"] = creature.VisualConsistencyPrompt
	metadata["generation_record_id"] = record.ID
	if record.WorkID != nil {
		metadata["work_id"] = *record.WorkID
	}
	return metadata
}

func (a *App) prepareNovelVideoShotGenerationPlan(project NovelVideoProject, shot NovelVideoShot, includeModelCenter bool) (novelVideoShotGenerationPlan, error) {
	req, err := a.buildNovelVideoShotRequest(project, shot)
	if err != nil {
		return novelVideoShotGenerationPlan{}, err
	}
	appSettings, err := a.loadSettings()
	if err != nil {
		return novelVideoShotGenerationPlan{}, err
	}
	modelConfig, err := a.videoModelConfig(req.Model, appSettings)
	if err != nil {
		return novelVideoShotGenerationPlan{}, err
	}
	if modelConfig == nil {
		return novelVideoShotGenerationPlan{}, errors.New("当前视频模型不可用")
	}
	if runtimeModel := canonicalVideoRuntimeModel(modelConfigRuntime(modelConfig)); runtimeModel != "" {
		req.Model = runtimeModel
	}
	if available, disabledReason := a.videoModelAvailability(*modelConfig); !available {
		return novelVideoShotGenerationPlan{}, errors.New(fallbackString(disabledReason, "当前视频模型不可用"))
	}
	if err := a.ensureModelCenter(); err != nil {
		return novelVideoShotGenerationPlan{}, err
	}
	capability, defaultDuration, err := a.resolvedVideoModelCapability(req.Model, modelConfig)
	if err != nil {
		return novelVideoShotGenerationPlan{}, err
	}
	req.AspectRatio = normalizeVideoAspectRatioForCapabilities(req.AspectRatio, capability)
	req.Duration = normalizeVideoDurationForCapabilities(req.Duration, capability, defaultDuration)
	req.Resolution = normalizeVideoResolutionForCapabilities(req, capability)
	req.HD = videoRequestIsHD(req)
	if req.AspectRatio == "" {
		return novelVideoShotGenerationPlan{}, errors.New("不支持的视频比例")
	}
	if req.Duration == "" {
		return novelVideoShotGenerationPlan{}, errors.New("不支持的视频时长")
	}
	if isWuyinGrokImagineModel(req.Model, modelConfig) && req.Duration == "25" {
		return novelVideoShotGenerationPlan{}, errors.New("当前视频模型不支持 25 秒")
	}
	referenceAssets, err := a.loadNovelVideoReferenceAssets(project.UserID, req.ReferenceAssetIDs, capability.MaxReferenceImages, referenceAssetKindImage)
	if err != nil {
		return novelVideoShotGenerationPlan{}, err
	}
	var referenceVideoAssets []ReferenceAsset
	if len(req.ReferenceVideoAssetIDs) > 0 {
		if !capability.SupportsReferenceVideo {
			return novelVideoShotGenerationPlan{}, errors.New("当前视频模型不支持参考视频")
		}
		referenceVideoAssets, err = a.loadNovelVideoReferenceAssets(project.UserID, req.ReferenceVideoAssetIDs, capability.MaxReferenceVideos, referenceAssetKindVideo)
		if err != nil {
			return novelVideoShotGenerationPlan{}, err
		}
	}
	var referenceAudioAssets []ReferenceAsset
	if len(req.ReferenceAudioAssetIDs) > 0 {
		if !capability.SupportsReferenceAudio {
			return novelVideoShotGenerationPlan{}, errors.New("当前视频模型不支持参考音频")
		}
		referenceAudioAssets, err = a.loadNovelVideoReferenceAssets(project.UserID, req.ReferenceAudioAssetIDs, capability.MaxReferenceAudios, referenceAssetKindAudio)
		if err != nil {
			return novelVideoShotGenerationPlan{}, err
		}
		req.GenerateAudio = true
	}
	if req.GenerateAudio && !capability.SupportsGenerateAudio {
		return novelVideoShotGenerationPlan{}, errors.New("当前视频模型不支持生成音频")
	}
	plan := novelVideoShotGenerationPlan{
		Request:              req,
		AppSettings:          appSettings,
		ModelConfig:          modelConfig,
		ReferenceAssets:      referenceAssets,
		ReferenceVideoAssets: referenceVideoAssets,
		ReferenceAudioAssets: referenceAudioAssets,
	}
	if includeModelCenter {
		candidates, err := a.resolveVideoModelCenterCandidates(appSettings, modelConfig, req.Model, req.Duration)
		if err != nil {
			if errors.Is(err, errVideoProviderKeyMissing) {
				return novelVideoShotGenerationPlan{}, errors.New("Wuyin provider key is required")
			}
			return novelVideoShotGenerationPlan{}, err
		}
		plan.ModelCenterCandidates = candidates
	}
	return plan, nil
}

func (a *App) buildNovelVideoShotRequest(project NovelVideoProject, shot NovelVideoShot) (videoGenerationRequest, error) {
	settings := effectiveNovelVideoGenerationSettings(project, shot)
	private := true
	req := videoGenerationRequest{
		Prompt:                 effectiveNovelVideoShotPrompt(shot),
		AspectRatio:            fallbackString(settings.AspectRatio, project.AspectRatio),
		Duration:               fallbackString(settings.Duration, project.Duration),
		Model:                  fallbackString(settings.Model, project.VideoModel),
		Resolution:             settings.Resolution,
		VideoStylePresetID:     settings.VideoStylePresetID,
		CustomVideoStyleID:     settings.CustomVideoStyleID,
		ReferenceAssetIDs:      append([]uint(nil), settings.ReferenceAssetIDs...),
		ReferenceVideoAssetIDs: append([]uint(nil), settings.ReferenceVideoAssetIDs...),
		ReferenceAudioAssetIDs: append([]uint(nil), settings.ReferenceAudioAssetIDs...),
		GenerateAudio:          settings.GenerateAudio,
		Private:                &private,
	}
	req.Model = fallbackString(strings.TrimSpace(req.Model), wuyinGrokImagineRuntimeModel)
	req.AspectRatio = normalizeVideoAspectRatio(req.AspectRatio)
	req.Duration = normalizeNovelVideoDuration(req.Duration)
	if req.Prompt == "" {
		return videoGenerationRequest{}, errors.New("镜头提示词不能为空")
	}
	if shot.ReferenceAssetID != nil && *shot.ReferenceAssetID != 0 && len(req.ReferenceAssetIDs) == 0 {
		req.ReferenceAssetIDs = []uint{*shot.ReferenceAssetID}
	}
	for _, url := range a.novelVideoShotCreatureImageURLs(project, shot) {
		req.Images = append(req.Images, url)
	}
	return req, nil
}

func (a *App) runNovelVideoShotGeneration(project NovelVideoProject, shot NovelVideoShot, attempt NovelVideoShotRenderAttempt) (GenerationRecord, error) {
	user := User{ID: project.UserID}
	if err := a.db.First(&user, project.UserID).Error; err != nil {
		return GenerationRecord{}, err
	}
	plan, err := a.prepareNovelVideoShotGenerationPlan(project, shot, true)
	if err != nil {
		return GenerationRecord{}, err
	}
	job := &videoGenerationJob{
		User:                  user,
		Settings:              plan.AppSettings,
		ModelConfig:           plan.ModelConfig,
		ModelCenterCandidates: plan.ModelCenterCandidates,
		Request:               plan.Request,
		ReferenceAssets:       plan.ReferenceAssets,
		ReferenceVideoAssets:  plan.ReferenceVideoAssets,
		ReferenceAudioAssets:  plan.ReferenceAudioAssets,
		CreditsCost:           videoCreditCost(plan.Request),
		Source:                VideoGenerationSourceNovelShot,
		NovelVideoProjectID:   &project.ID,
		NovelVideoEpisodeID:   &shot.EpisodeID,
		NovelVideoShotID:      &shot.ID,
		NovelVideoAttemptID:   &attempt.ID,
	}
	record, err := a.createVideoGenerationRecord(job)
	if err != nil {
		return GenerationRecord{}, err
	}
	_, providerErr, err := a.executeVideoGenerationRecord(&record, job)
	if providerErr != nil {
		shot.ErrorCode = fallbackString(providerErr.Code, "provider_error")
		shot.ErrorMessage = fallbackString(providerErr.Message, "视频生成失败")
		_ = a.db.Save(&shot).Error
		return record, errors.New(shot.ErrorMessage)
	}
	return record, err
}

func (a *App) referenceAssetsForNovelVideoShot(userID uint, ids []uint) ([]ReferenceAsset, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var assets []ReferenceAsset
	if err := a.db.Where("user_id = ? AND id IN ?", userID, ids).Find(&assets).Error; err != nil {
		return nil, err
	}
	if len(assets) != len(ids) {
		return nil, errors.New("reference asset not found")
	}
	return assets, nil
}

func (a *App) loadNovelVideoReferenceAssets(userID uint, ids []uint, limit int, kind string) ([]ReferenceAsset, error) {
	ids = uniqueUintIDs(ids)
	if len(ids) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 4
	}
	if len(ids) > limit {
		return nil, fmt.Errorf("参考素材最多选择 %d 个", limit)
	}
	var assets []ReferenceAsset
	if err := a.db.Where("user_id = ? AND id IN ?", userID, ids).Find(&assets).Error; err != nil {
		return nil, err
	}
	if len(assets) != len(ids) {
		return nil, errors.New("参考素材不存在")
	}
	byID := make(map[uint]ReferenceAsset, len(assets))
	for _, asset := range assets {
		a.applyReferenceAssetPublicURL(&asset)
		if kind != referenceAssetKindAll && asset.Kind != kind {
			return nil, errors.New("参考素材类型不匹配")
		}
		byID[asset.ID] = asset
	}
	ordered := make([]ReferenceAsset, 0, len(ids))
	for _, id := range ids {
		asset, ok := byID[id]
		if !ok {
			return nil, errors.New("参考素材不存在")
		}
		ordered = append(ordered, asset)
	}
	return ordered, nil
}

func decodeNovelVideoGenerationSettings(raw string) novelVideoGenerationSettings {
	var settings novelVideoGenerationSettings
	if strings.TrimSpace(raw) == "" {
		return settings
	}
	_ = json.Unmarshal([]byte(raw), &settings)
	return normalizeNovelVideoGenerationSettings(settings)
}

func encodeNovelVideoGenerationSettings(settings novelVideoGenerationSettings) string {
	settings = normalizeNovelVideoGenerationSettings(settings)
	raw, _ := json.Marshal(settings)
	return string(raw)
}

func normalizeNovelVideoGenerationSettings(settings novelVideoGenerationSettings) novelVideoGenerationSettings {
	settings.Model = canonicalVideoRuntimeModel(strings.TrimSpace(settings.Model))
	if strings.TrimSpace(settings.AspectRatio) != "" {
		settings.AspectRatio = normalizeNovelVideoAspectRatio(settings.AspectRatio)
	}
	if strings.TrimSpace(settings.Duration) != "" {
		settings.Duration = normalizeNovelVideoDuration(settings.Duration)
	}
	settings.Resolution = strings.ToLower(strings.TrimSpace(settings.Resolution))
	settings.ReferenceAssetIDs = uniqueUintIDs(settings.ReferenceAssetIDs)
	settings.ReferenceVideoAssetIDs = uniqueUintIDs(settings.ReferenceVideoAssetIDs)
	settings.ReferenceAudioAssetIDs = uniqueUintIDs(settings.ReferenceAudioAssetIDs)
	if len(settings.ReferenceAudioAssetIDs) > 0 {
		settings.GenerateAudio = true
	}
	return settings
}

func normalizeNovelVideoDuration(value string) string {
	value = strings.TrimSpace(value)
	if value == "-1" {
		return value
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 1 && seconds <= 16 {
		return strconv.Itoa(seconds)
	}
	return normalizeVideoDuration(value)
}

func effectiveNovelVideoGenerationSettings(project NovelVideoProject, shot NovelVideoShot) novelVideoGenerationSettings {
	settings := decodeNovelVideoGenerationSettings(project.VideoSettingsJSON)
	if settings.Model == "" {
		settings.Model = strings.TrimSpace(project.VideoModel)
	}
	if settings.AspectRatio == "" {
		settings.AspectRatio = project.AspectRatio
	}
	if settings.Duration == "" {
		settings.Duration = project.Duration
	}
	shotSettings := decodeNovelVideoGenerationSettings(shot.GenerationSettingsJSON)
	if shotSettings.Model != "" {
		settings.Model = shotSettings.Model
	}
	if shotSettings.AspectRatio != "" {
		settings.AspectRatio = shotSettings.AspectRatio
	}
	if shotSettings.Duration != "" {
		settings.Duration = shotSettings.Duration
	}
	if shotSettings.Resolution != "" {
		settings.Resolution = shotSettings.Resolution
	}
	if shotSettings.VideoStylePresetID != 0 {
		settings.VideoStylePresetID = shotSettings.VideoStylePresetID
	}
	if shotSettings.CustomVideoStyleID != 0 {
		settings.CustomVideoStyleID = shotSettings.CustomVideoStyleID
	}
	if len(shotSettings.ReferenceAssetIDs) > 0 {
		settings.ReferenceAssetIDs = shotSettings.ReferenceAssetIDs
	}
	if len(shotSettings.ReferenceVideoAssetIDs) > 0 {
		settings.ReferenceVideoAssetIDs = shotSettings.ReferenceVideoAssetIDs
	}
	if len(shotSettings.ReferenceAudioAssetIDs) > 0 {
		settings.ReferenceAudioAssetIDs = shotSettings.ReferenceAudioAssetIDs
	}
	if shotSettings.GenerateAudio {
		settings.GenerateAudio = true
	}
	if shot.ReferenceAssetID != nil && *shot.ReferenceAssetID != 0 && len(settings.ReferenceAssetIDs) == 0 {
		settings.ReferenceAssetIDs = []uint{*shot.ReferenceAssetID}
	}
	return normalizeNovelVideoGenerationSettings(settings)
}

func novelVideoSettingsFromRequest(req videoGenerationRequest) novelVideoGenerationSettings {
	return normalizeNovelVideoGenerationSettings(novelVideoGenerationSettings{
		Model:                  req.Model,
		AspectRatio:            req.AspectRatio,
		Duration:               req.Duration,
		Resolution:             req.Resolution,
		VideoStylePresetID:     req.VideoStylePresetID,
		CustomVideoStyleID:     req.CustomVideoStyleID,
		ReferenceAssetIDs:      req.ReferenceAssetIDs,
		ReferenceVideoAssetIDs: req.ReferenceVideoAssetIDs,
		ReferenceAudioAssetIDs: req.ReferenceAudioAssetIDs,
		GenerateAudio:          req.GenerateAudio,
	})
}

func (a *App) novelVideoShotCreatureImageURLs(project NovelVideoProject, shot NovelVideoShot) []string {
	ids := decodeUintList(shot.CreatureIDsJSON)
	if len(ids) == 0 {
		return nil
	}
	var creatures []NovelVideoCreature
	if err := a.db.Where("project_id = ? AND user_id = ? AND id IN ?", project.ID, project.UserID, ids).Find(&creatures).Error; err != nil {
		return nil
	}
	urls := make([]string, 0, len(creatures))
	for _, creature := range creatures {
		if url := strings.TrimSpace(creature.AssetURL); url != "" {
			urls = append(urls, url)
		}
	}
	return urls
}

func (a *App) loadNovelVideoProjectForUser(c *gin.Context) (NovelVideoProject, bool) {
	user := currentUser(c)
	projectID, ok := uintParam(c, "id")
	if !ok {
		writeError(c, http.StatusBadRequest, "invalid_project_id", "项目 ID 无效")
		return NovelVideoProject{}, false
	}
	var project NovelVideoProject
	if err := a.db.Where("id = ? AND user_id = ?", projectID, user.ID).First(&project).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "novel_video_project_not_found", "小说视频项目不存在")
			return NovelVideoProject{}, false
		}
		writeError(c, http.StatusInternalServerError, "novel_video_project_load_failed", "小说视频项目读取失败")
		return NovelVideoProject{}, false
	}
	return project, true
}

func (a *App) loadNovelVideoProjectChildren(project NovelVideoProject) ([]NovelVideoCreature, []NovelVideoEpisode, error) {
	var creatures []NovelVideoCreature
	if err := a.db.Where("project_id = ?", project.ID).Order("id asc").Find(&creatures).Error; err != nil {
		return nil, nil, err
	}
	var episodes []NovelVideoEpisode
	if err := a.db.Where("project_id = ?", project.ID).Order("number asc, id asc").Preload("Shots", func(db *gorm.DB) *gorm.DB {
		return db.Order("number asc, id asc")
	}).Preload("Shots.RenderAttempts", func(db *gorm.DB) *gorm.DB {
		return db.Order("id asc")
	}).Find(&episodes).Error; err != nil {
		return nil, nil, err
	}
	a.hydrateNovelVideoGenerationMetadata(project, creatures, episodes)
	return creatures, episodes, nil
}

func (a *App) loadNovelVideoAssets(project NovelVideoProject) ([]NovelVideoAsset, error) {
	var assets []NovelVideoAsset
	err := a.db.Where("project_id = ? AND user_id = ?", project.ID, project.UserID).
		Order("kind asc, id asc").
		Find(&assets).Error
	return assets, err
}

func (a *App) hydrateNovelVideoShotImages(images []NovelVideoShotImage) {
	workIDs := make([]uint, 0)
	recordIDs := make([]uint, 0)
	for _, image := range images {
		if image.WorkID != nil {
			workIDs = append(workIDs, *image.WorkID)
		}
		if image.GenerationRecordID != nil {
			recordIDs = append(recordIDs, *image.GenerationRecordID)
		}
	}
	worksByID := map[uint]Work{}
	if len(workIDs) > 0 {
		var works []Work
		if err := a.db.Where("id IN ?", workIDs).Find(&works).Error; err == nil {
			for _, work := range works {
				worksByID[work.ID] = work
			}
		}
	}
	recordsByID := map[uint]GenerationRecord{}
	if len(recordIDs) > 0 {
		var records []GenerationRecord
		if err := a.db.Where("id IN ?", recordIDs).Find(&records).Error; err == nil {
			for _, record := range records {
				recordsByID[record.ID] = record
			}
		}
	}
	for index := range images {
		image := &images[index]
		if image.WorkID != nil {
			if work, ok := worksByID[*image.WorkID]; ok {
				image.PreviewURL = work.PreviewURL
				image.DownloadURL = work.DownloadURL
			}
		}
		if image.GenerationRecordID != nil {
			if record, ok := recordsByID[*image.GenerationRecordID]; ok {
				image.PreviewURL = fallbackString(image.PreviewURL, record.PreviewURL)
				image.DownloadURL = fallbackString(image.DownloadURL, record.DownloadURL)
				image.ErrorCode = fallbackString(image.ErrorCode, record.ErrorCode)
				image.ErrorMessage = fallbackString(image.ErrorMessage, record.ErrorMessage)
				image.GenerationStatus = normalizeGenerationStatus(record.Status)
				image.GenerationStage = normalizeGenerationStage(record.Status, record.Stage)
				image.GenerationProgress = novelVideoGenerationRecordProgress(record.Status, record.Stage)
				if image.WorkID == nil {
					image.WorkID = record.WorkID
				}
			}
		}
	}
}

func (a *App) hydrateNovelVideoGenerationMetadata(project NovelVideoProject, creatures []NovelVideoCreature, episodes []NovelVideoEpisode) {
	workIDs := make([]uint, 0)
	recordIDs := make([]uint, 0)
	for _, creature := range creatures {
		if creature.WorkID != nil {
			workIDs = append(workIDs, *creature.WorkID)
		}
		if creature.GenerationRecordID != nil {
			recordIDs = append(recordIDs, *creature.GenerationRecordID)
		}
	}
	for _, episode := range episodes {
		for _, shot := range episode.Shots {
			if shot.WorkID != nil {
				workIDs = append(workIDs, *shot.WorkID)
			}
			if shot.GenerationRecordID != nil {
				recordIDs = append(recordIDs, *shot.GenerationRecordID)
			}
			for _, attempt := range shot.RenderAttempts {
				if attempt.GenerationRecordID != nil {
					recordIDs = append(recordIDs, *attempt.GenerationRecordID)
				}
			}
		}
	}

	worksByID := map[uint]Work{}
	if len(workIDs) > 0 {
		var works []Work
		if err := a.db.Where("id IN ?", workIDs).Find(&works).Error; err == nil {
			for _, work := range works {
				worksByID[work.ID] = work
			}
		}
	}
	recordsByID := map[uint]GenerationRecord{}
	if len(recordIDs) > 0 {
		var records []GenerationRecord
		if err := a.db.Where("id IN ?", recordIDs).Find(&records).Error; err == nil {
			for _, record := range records {
				recordsByID[record.ID] = record
			}
		}
	}

	for index := range creatures {
		creature := &creatures[index]
		creature.WorkPreviewURL = strings.TrimSpace(creature.AssetURL)
		if creature.WorkID != nil {
			if work, ok := worksByID[*creature.WorkID]; ok {
				creature.WorkPreviewURL = fallbackString(creature.WorkPreviewURL, work.PreviewURL)
			}
		}
		if creature.GenerationRecordID != nil {
			if record, ok := recordsByID[*creature.GenerationRecordID]; ok {
				creature.GenerationStatus = record.Status
				creature.WorkPreviewURL = fallbackString(creature.WorkPreviewURL, record.PreviewURL)
				creature.LatestError = fallbackString(creature.ErrorMessage, record.ErrorMessage)
			}
		}
		if creature.GenerationStatus == "" && creature.WorkPreviewURL != "" {
			creature.GenerationStatus = GenerationStatusSucceeded
		}
		creature.LatestError = fallbackString(creature.LatestError, creature.ErrorMessage)
	}

	for episodeIndex := range episodes {
		for shotIndex := range episodes[episodeIndex].Shots {
			shot := &episodes[episodeIndex].Shots[shotIndex]
			shot.EstimatedCredits = videoCreditCost(videoGenerationRequest{
				Prompt:      effectiveNovelVideoShotPrompt(*shot),
				AspectRatio: project.AspectRatio,
				Duration:    strconv.Itoa(effectiveNovelVideoShotDurationSeconds(project, *shot)),
				Model:       project.VideoModel,
				Private:     boolPointer(true),
			})
			if shot.WorkID != nil {
				if work, ok := worksByID[*shot.WorkID]; ok {
					shot.WorkPreviewURL = work.PreviewURL
					shot.WorkDownloadURL = work.DownloadURL
				}
			}
			if shot.GenerationRecordID != nil {
				if record, ok := recordsByID[*shot.GenerationRecordID]; ok {
					shot.WorkPreviewURL = fallbackString(shot.WorkPreviewURL, record.PreviewURL)
					shot.WorkDownloadURL = fallbackString(shot.WorkDownloadURL, record.DownloadURL)
					shot.LatestError = fallbackString(shot.ErrorMessage, record.ErrorMessage)
					if len(shot.RenderAttempts) == 0 {
						shot.RenderAttempts = append(shot.RenderAttempts, NovelVideoShotRenderAttempt{
							ProjectID:          shot.ProjectID,
							EpisodeID:          shot.EpisodeID,
							ShotID:             shot.ID,
							UserID:             shot.UserID,
							GenerationRecordID: shot.GenerationRecordID,
							Status:             record.Status,
							Progress:           novelVideoStatusProgress(record.Status),
							ErrorCode:          record.ErrorCode,
							ErrorMessage:       record.ErrorMessage,
							CreatedAt:          record.CreatedAt,
							UpdatedAt:          record.UpdatedAt,
						})
					}
				}
			}
			shot.LatestError = fallbackString(shot.LatestError, shot.ErrorMessage)
			shot.GenerationProgress = novelVideoShotProgress(*shot)
		}
	}
}

type novelVideoCreatureDraft struct {
	Name                    string `json:"name"`
	CreatureType            string `json:"creature_type"`
	Appearance              string `json:"appearance"`
	Abilities               string `json:"abilities"`
	VisualConsistencyPrompt string `json:"visual_consistency_prompt"`
}

type novelVideoEpisodeDraft struct {
	Number  int                   `json:"number"`
	Title   string                `json:"title"`
	Summary string                `json:"summary"`
	Shots   []novelVideoShotDraft `json:"shots"`
}

type novelVideoShotDraft struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Prompt string `json:"prompt"`
}

func fallbackNovelVideoImageActors(project NovelVideoProject) []NovelVideoCreature {
	base := fallbackString(strings.TrimSpace(project.Title), "短电影")
	actors := []string{"女主角", "男主角", "关键配角"}
	items := make([]NovelVideoCreature, 0, len(actors))
	for index, role := range actors {
		name := fmt.Sprintf("%s%s", base, role)
		items = append(items, NovelVideoCreature{
			ProjectID:               project.ID,
			UserID:                  project.UserID,
			Name:                    name,
			CreatureType:            "actor",
			Appearance:              fmt.Sprintf("%s的演员定妆描述，保持五官、年龄感、发型、服装主色和标志性细节稳定。", role),
			Abilities:               "短电影演员表演资产，用于跨镜头身份一致。",
			VisualConsistencyPrompt: fmt.Sprintf("同一名%s，电影写实质感，自然光，中性表情，身份、五官、发型和服装轮廓保持一致。", role),
			ReviewStatus:            NovelVideoReviewStatusNeedsReview,
			CreatedAt:               time.Now().Add(time.Duration(index) * time.Millisecond),
		})
	}
	return items
}

func fallbackNovelVideoImageAssets(project NovelVideoProject, actors []NovelVideoCreature) []NovelVideoAsset {
	style := fallbackString(project.StylePreset, "写实短电影")
	items := make([]NovelVideoAsset, 0, len(actors)+3)
	for _, actor := range actors {
		items = append(items, NovelVideoAsset{
			ProjectID:    project.ID,
			UserID:       project.UserID,
			Kind:         NovelVideoAssetKindActorRef,
			Name:         actor.Name + "参考图",
			Description:  "演员正脸、半身、全身参考槽位，用于后续镜头身份锁定。",
			Prompt:       actor.VisualConsistencyPrompt,
			Version:      1,
			ReviewStatus: NovelVideoReviewStatusNeedsReview,
		})
	}
	items = append(items,
		NovelVideoAsset{ProjectID: project.ID, UserID: project.UserID, Kind: NovelVideoAssetKindScene, Name: project.Title + "主场景", Description: "短电影反复出现的核心场景、光线和空间结构。", Prompt: style + "，核心场景参考图，空间关系清晰。", Version: 1, ReviewStatus: NovelVideoReviewStatusNeedsReview, MetadataJSON: encodeJSON(map[string]any{"source": "image_plan"})},
		NovelVideoAsset{ProjectID: project.ID, UserID: project.UserID, Kind: NovelVideoAssetKindProp, Name: project.Title + "关键道具", Description: "推动剧情的可复用道具参考。", Prompt: style + "，关键道具特写，材质和尺寸稳定。", Version: 1, ReviewStatus: NovelVideoReviewStatusNeedsReview, MetadataJSON: encodeJSON(map[string]any{"source": "image_plan"})},
		NovelVideoAsset{ProjectID: project.ID, UserID: project.UserID, Kind: NovelVideoAssetKindStyle, Name: project.Title + "视觉风格", Description: "统一色彩、镜头语言、颗粒和景深。", Prompt: style + "，电影剧照风格，统一调色和摄影语言。", Version: 1, ReviewStatus: NovelVideoReviewStatusNeedsReview, MetadataJSON: encodeJSON(map[string]any{"source": "image_plan"})},
	)
	return items
}

func fallbackNovelVideoImageEpisode(project NovelVideoProject, actors []NovelVideoCreature, shotCount int) novelVideoEpisodeDraft {
	excerpt := fallbackString(novelVideoExcerpt(project.SourceText, 120), project.Title)
	shots := make([]novelVideoShotDraft, 0, shotCount)
	for i := 0; i < shotCount; i++ {
		actor := actors[i%len(actors)]
		shotType := "单人中景"
		if i%3 == 2 {
			shotType = "多人对手戏"
		}
		shots = append(shots, novelVideoShotDraft{
			Number: i + 1,
			Title:  fmt.Sprintf("镜头 %02d：%s", i+1, shotType),
			Prompt: strings.Join([]string{
				"短电影系列图片，电影剧照，不生成视频。",
				"故事：" + excerpt,
				"画幅：" + fallbackString(project.AspectRatio, "16:9"),
				"风格：" + fallbackString(project.StylePreset, "写实短电影"),
				"主演员：" + actor.Name,
				"镜头类型：" + shotType,
				"要求：保持演员身份一致，构图清晰，适合后续作为视频首帧或关键帧。",
			}, "\n"),
		})
	}
	return novelVideoEpisodeDraft{Number: 1, Title: "图片分镜计划", Summary: "短电影图片前期镜头计划", Shots: shots}
}

func buildNovelVideoShotImagePrompt(project NovelVideoProject, shot NovelVideoShot, candidate int) string {
	prompt := fallbackString(strings.TrimSpace(shot.ImagePrompt), effectiveNovelVideoShotPrompt(shot))
	return strings.Join([]string{
		"短电影系列图片候选图。",
		"项目：" + project.Title,
		"候选版本：" + strconv.Itoa(candidate),
		"风格：" + fallbackString(project.StylePreset, "写实短电影"),
		"镜头：" + shot.Title,
		"画面提示：" + prompt,
		"要求：演员身份、服装主轮廓、场景空间关系保持一致；输出单张高质量电影剧照。",
	}, "\n")
}

func (a *App) novelVideoShotReferenceAssetIDs(project NovelVideoProject, shot NovelVideoShot, limit int) ([]uint, string) {
	ids := make([]uint, 0)
	add := func(values ...uint) {
		for _, value := range values {
			if value == 0 || containsUint(ids, value) {
				continue
			}
			ids = append(ids, value)
		}
	}
	for _, assetID := range decodeNovelVideoActorReferenceIDsFromShot(project, shot, a.db) {
		add(assetID)
	}
	for _, ref := range decodeNovelVideoAssetRefs(shot.AssetRefsJSON) {
		if ref.Type != "asset" {
			continue
		}
		var asset NovelVideoAsset
		if err := a.db.Where("id = ? AND project_id = ? AND user_id = ?", ref.ID, project.ID, project.UserID).First(&asset).Error; err != nil {
			continue
		}
		add(referenceAssetIDsFromNovelVideoAsset(asset)...)
	}
	if shot.ReferenceAssetID != nil {
		add(*shot.ReferenceAssetID)
	}
	if limit <= 0 || len(ids) <= limit {
		return ids, ""
	}
	return ids[:limit], fmt.Sprintf("镜头 %d 引用图超过 %d 张，已按演员、场景、道具、风格优先级截断。", shot.ID, limit)
}

func decodeNovelVideoActorReferenceIDsFromShot(project NovelVideoProject, shot NovelVideoShot, db *gorm.DB) []uint {
	actorIDs := decodeUintList(shot.CreatureIDsJSON)
	ids := make([]uint, 0)
	for _, actorID := range actorIDs {
		var assets []NovelVideoAsset
		if err := db.Where("project_id = ? AND user_id = ? AND kind = ?", project.ID, project.UserID, NovelVideoAssetKindActorRef).Find(&assets).Error; err != nil {
			continue
		}
		for _, asset := range assets {
			meta := decodeJSONMap(asset.MetadataJSON)
			if uintFromAny(meta["actor_id"]) == actorID {
				ids = append(ids, referenceAssetIDsFromNovelVideoAsset(asset)...)
			}
		}
	}
	return uniqueUintIDs(ids)
}

func referenceAssetIDsFromNovelVideoAsset(asset NovelVideoAsset) []uint {
	meta := decodeJSONMap(asset.MetadataJSON)
	ids := make([]uint, 0)
	if id := uintFromAny(meta["canonical_asset_id"]); id > 0 {
		ids = append(ids, id)
	}
	if values, ok := meta["reference_asset_ids"].([]any); ok {
		for _, value := range values {
			if id := uintFromAny(value); id > 0 {
				ids = append(ids, id)
			}
		}
	}
	if id := uintFromAny(meta["reference_asset_id"]); id > 0 {
		ids = append(ids, id)
	}
	return uniqueUintIDs(ids)
}

func novelVideoReferenceIntentForShot(actorIDs, referenceAssetIDs []uint) string {
	if len(actorIDs) == 1 {
		return GenerationReferenceIntentCharacter
	}
	if len(actorIDs) > 1 || len(referenceAssetIDs) > 1 {
		return GenerationReferenceIntentCompose
	}
	return GenerationReferenceIntentCreative
}

func (a *App) planNovelVideoAnalysisWithDeepSeek(ctx context.Context, project NovelVideoProject) (map[string]any, error) {
	if strings.TrimSpace(a.cfg.DeepSeekAPIKey) == "" {
		return nil, errors.New("deepseek not configured")
	}
	userPayload, _ := json.Marshal(map[string]any{
		"title":        project.Title,
		"source_text":  novelVideoExcerpt(project.SourceText, novelVideoMaxSourceChars),
		"style_preset": project.StylePreset,
		"requirements": []string{
			"输出 story_bible、creatures、content_risk_summary 三个顶层字段。",
			"creatures 只抽取非人类或怪异生物候选，不要自动生成所有人类角色。",
			"每个 creature 必须包含 name、creature_type、appearance、abilities、visual_consistency_prompt。",
		},
	})
	system := strings.Join([]string{
		"你是小说短视频分集制作规划助手。",
		"只返回严格 JSON，不要 Markdown，不要解释。",
		"JSON schema: {\"story_bible\":{\"logline\":\"\",\"world\":\"\",\"visual_style\":\"\"},\"creatures\":[{\"name\":\"\",\"creature_type\":\"\",\"appearance\":\"\",\"abilities\":\"\",\"visual_consistency_prompt\":\"\"}],\"content_risk_summary\":\"\"}",
	}, "\n")
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		content, err := a.callNovelVideoDeepSeekJSON(ctx, system, string(userPayload))
		if err != nil {
			lastErr = err
			continue
		}
		plan, err := parseNovelVideoAnalysisPlan(content)
		if err != nil {
			lastErr = err
			continue
		}
		return plan, nil
	}
	return nil, lastErr
}

func (a *App) planNovelVideoEpisodesWithDeepSeek(ctx context.Context, project NovelVideoProject) ([]novelVideoEpisodeDraft, error) {
	if strings.TrimSpace(a.cfg.DeepSeekAPIKey) == "" {
		return nil, errors.New("deepseek not configured")
	}
	userPayload, _ := json.Marshal(map[string]any{
		"title":        project.Title,
		"source_text":  novelVideoExcerpt(project.SourceText, novelVideoMaxSourceChars),
		"story_bible":  decodeJSONMap(project.StoryBibleJSON),
		"style_preset": project.StylePreset,
		"aspect_ratio": project.AspectRatio,
		"duration":     project.Duration,
		"requirements": []string{
			"默认 3-5 集。",
			"每集 3-6 个镜头。",
			"每个镜头 prompt 要能直接用于视频生成，包含主体动作、镜头运动、场景、风格和一致性约束。",
		},
	})
	system := strings.Join([]string{
		"你是小说视频分集镜头表规划助手。",
		"只返回严格 JSON，不要 Markdown，不要解释。",
		"JSON schema: {\"episodes\":[{\"number\":1,\"title\":\"\",\"summary\":\"\",\"shots\":[{\"number\":1,\"title\":\"\",\"prompt\":\"\"}]}]}",
	}, "\n")
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		content, err := a.callNovelVideoDeepSeekJSON(ctx, system, string(userPayload))
		if err != nil {
			lastErr = err
			continue
		}
		episodes, err := parseNovelVideoEpisodePlan(content)
		if err != nil {
			lastErr = err
			continue
		}
		return episodes, nil
	}
	return nil, lastErr
}

func (a *App) callNovelVideoDeepSeekJSON(ctx context.Context, system, user string) (string, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(a.cfg.DeepSeekBaseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.deepseek.com"
	}
	model := fallbackString(strings.TrimSpace(a.cfg.DeepSeekPromptModel), "deepseek-v4")
	payload := map[string]any{
		"model":       model,
		"stream":      false,
		"temperature": 0.2,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	timeoutSeconds := fallbackPositiveInt(a.cfg.DeepSeekPromptTimeoutSeconds, 45)
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(a.cfg.DeepSeekAPIKey))
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("deepseek novel video failed: status=%d body=%s", resp.StatusCode, string(rawBody))
	}
	return deepSeekMessageContent(rawBody)
}

func parseNovelVideoAnalysisPlan(content string) (map[string]any, error) {
	var decoded struct {
		StoryBible         map[string]any            `json:"story_bible"`
		Creatures          []novelVideoCreatureDraft `json:"creatures"`
		ContentRiskSummary string                    `json:"content_risk_summary"`
	}
	if err := json.Unmarshal([]byte(extractNovelVideoJSONObject(content)), &decoded); err != nil {
		return nil, err
	}
	if len(decoded.StoryBible) == 0 {
		return nil, errors.New("story_bible is required")
	}
	validCreatures := make([]novelVideoCreatureDraft, 0, len(decoded.Creatures))
	for _, creature := range decoded.Creatures {
		creature.Name = strings.TrimSpace(creature.Name)
		creature.CreatureType = strings.TrimSpace(creature.CreatureType)
		creature.Appearance = strings.TrimSpace(creature.Appearance)
		creature.Abilities = strings.TrimSpace(creature.Abilities)
		creature.VisualConsistencyPrompt = strings.TrimSpace(creature.VisualConsistencyPrompt)
		if creature.Name == "" || creature.Appearance == "" || creature.VisualConsistencyPrompt == "" {
			continue
		}
		validCreatures = append(validCreatures, creature)
	}
	if len(validCreatures) == 0 {
		return nil, errors.New("at least one valid creature is required")
	}
	return map[string]any{
		"story_bible":          decoded.StoryBible,
		"creatures":            validCreatures,
		"content_risk_summary": strings.TrimSpace(decoded.ContentRiskSummary),
		"source":               "deepseek",
	}, nil
}

func parseNovelVideoEpisodePlan(content string) ([]novelVideoEpisodeDraft, error) {
	var decoded struct {
		Episodes []novelVideoEpisodeDraft `json:"episodes"`
	}
	if err := json.Unmarshal([]byte(extractNovelVideoJSONObject(content)), &decoded); err != nil {
		return nil, err
	}
	if len(decoded.Episodes) == 0 {
		return nil, errors.New("episodes are required")
	}
	episodes := make([]novelVideoEpisodeDraft, 0, len(decoded.Episodes))
	for episodeIndex, episode := range decoded.Episodes {
		episode.Number = positiveOrDefault(episode.Number, episodeIndex+1)
		episode.Title = fallbackString(strings.TrimSpace(episode.Title), fmt.Sprintf("第 %d 集", episode.Number))
		episode.Summary = strings.TrimSpace(episode.Summary)
		if len(episode.Shots) < 3 || len(episode.Shots) > 6 {
			return nil, errors.New("each episode must include 3 to 6 shots")
		}
		shots := make([]novelVideoShotDraft, 0, len(episode.Shots))
		for shotIndex, shot := range episode.Shots {
			shot.Number = positiveOrDefault(shot.Number, shotIndex+1)
			shot.Title = fallbackString(strings.TrimSpace(shot.Title), fmt.Sprintf("镜头 %d", shot.Number))
			shot.Prompt = strings.TrimSpace(shot.Prompt)
			if shot.Prompt == "" {
				return nil, errors.New("shot prompt is required")
			}
			shots = append(shots, shot)
		}
		episode.Shots = shots
		episodes = append(episodes, episode)
	}
	return episodes, nil
}

func extractNovelVideoJSONObject(content string) string {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(strings.TrimPrefix(content, "json"))
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end < start {
		return content
	}
	return content[start : end+1]
}

func positiveOrDefault(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func fallbackNovelVideoAnalysis(project NovelVideoProject) map[string]any {
	excerpt := novelVideoExcerpt(project.SourceText, 180)
	creatures := []novelVideoCreatureDraft{
		{
			Name:                    "核心异兽",
			CreatureType:            "小说关键生物",
			Appearance:              "从原文意象提炼外形，保留鳞片、雾气、发光眼睛、特殊肢体等可视化细节。",
			Abilities:               "根据原文冲突提炼感知、守护、追踪或环境操控能力。",
			VisualConsistencyPrompt: "同一只核心异兽，清晰轮廓，固定材质、眼睛颜色、肢体数量和标志性纹理。",
		},
	}
	return map[string]any{
		"story_bible": map[string]any{
			"logline":        fallbackString(excerpt, project.Title),
			"world":          "基于原文建立的短篇视频世界观，优先保留场景、时代、怪异规则和核心冲突。",
			"visual_style":   fallbackString(project.StylePreset, "电影感写实"),
			"episode_target": "3-5 集，每集 3-6 个镜头，先做人审草稿再生成。",
		},
		"creatures":            creatures,
		"content_risk_summary": "未发现自动草稿阶段的明确高风险内容；提交视频前仍需人工审核人物、版权和暴力尺度。",
		"source":               "fallback",
	}
}

func fallbackNovelVideoEpisodePlan(project NovelVideoProject) []novelVideoEpisodeDraft {
	excerpt := fallbackString(novelVideoExcerpt(project.SourceText, 120), project.Title)
	episodes := make([]novelVideoEpisodeDraft, 0, 3)
	for episode := 1; episode <= 3; episode++ {
		shots := make([]novelVideoShotDraft, 0, 3)
		for shot := 1; shot <= 3; shot++ {
			shots = append(shots, novelVideoShotDraft{
				Number: shot,
				Title:  fmt.Sprintf("第 %d 集镜头 %d", episode, shot),
				Prompt: strings.Join([]string{
					fmt.Sprintf("%s，第 %d 集第 %d 个短视频镜头。", project.Title, episode, shot),
					"画面基调：" + fallbackString(project.StylePreset, "电影感写实"),
					"剧情依据：" + excerpt,
					"要求：镜头运动明确，主体动作清晰，保留小说生物设定图的一致性，不加入未审核角色。",
				}, "\n"),
			})
		}
		episodes = append(episodes, novelVideoEpisodeDraft{
			Number:  episode,
			Title:   fmt.Sprintf("第 %d 集", episode),
			Summary: fmt.Sprintf("围绕“%s”推进第 %d 段冲突和反转。", excerpt, episode),
			Shots:   shots,
		})
	}
	return episodes
}

func novelVideoProjectResponse(project NovelVideoProject, creatures []NovelVideoCreature, episodes []NovelVideoEpisode) gin.H {
	videoSettings := decodeNovelVideoGenerationSettings(project.VideoSettingsJSON)
	if videoSettings.Model == "" {
		videoSettings.Model = project.VideoModel
	}
	if videoSettings.AspectRatio == "" {
		videoSettings.AspectRatio = project.AspectRatio
	}
	if videoSettings.Duration == "" {
		videoSettings.Duration = project.Duration
	}
	return gin.H{
		"id":                   project.ID,
		"user_id":              project.UserID,
		"title":                project.Title,
		"source_text":          project.SourceText,
		"source_chars":         utf8.RuneCountInString(project.SourceText),
		"content_mode":         effectiveNovelVideoContentMode(project.ContentMode),
		"schema_version":       effectiveNovelVideoSchemaVersion(project.SchemaVersion),
		"generation_mode":      effectiveNovelVideoGenerationMode(project.GenerationMode),
		"grid_size":            effectiveNovelVideoGridSize(project.GridSize),
		"style_preset":         project.StylePreset,
		"aspect_ratio":         project.AspectRatio,
		"duration":             project.Duration,
		"image_model":          project.ImageModel,
		"video_model":          project.VideoModel,
		"video_settings":       videoSettings,
		"status":               project.Status,
		"story_bible":          decodeJSONMap(project.StoryBibleJSON),
		"content_risk_summary": project.ContentRiskSummary,
		"planning_draft":       decodeJSONMap(project.PlanningDraftJSON),
		"creatures":            novelVideoCreatureResponses(creatures),
		"episodes":             novelVideoEpisodeResponses(episodes),
		"created_at":           project.CreatedAt,
		"updated_at":           project.UpdatedAt,
	}
}

func novelVideoCreatureResponses(creatures []NovelVideoCreature) []gin.H {
	items := make([]gin.H, 0, len(creatures))
	for _, creature := range creatures {
		items = append(items, novelVideoCreatureResponse(creature))
	}
	return items
}

func novelVideoCreatureResponse(creature NovelVideoCreature) gin.H {
	return gin.H{
		"id":                        creature.ID,
		"project_id":                creature.ProjectID,
		"user_id":                   creature.UserID,
		"name":                      creature.Name,
		"creature_type":             creature.CreatureType,
		"appearance":                creature.Appearance,
		"abilities":                 creature.Abilities,
		"visual_consistency_prompt": creature.VisualConsistencyPrompt,
		"review_status":             creature.ReviewStatus,
		"generation_record_id":      creature.GenerationRecordID,
		"work_id":                   creature.WorkID,
		"asset_url":                 creature.AssetURL,
		"work_preview_url":          creature.WorkPreviewURL,
		"generation_status":         creature.GenerationStatus,
		"latest_error":              creature.LatestError,
		"error_code":                creature.ErrorCode,
		"error_message":             creature.ErrorMessage,
		"created_at":                creature.CreatedAt,
		"updated_at":                creature.UpdatedAt,
	}
}

func novelVideoEpisodeResponses(episodes []NovelVideoEpisode) []gin.H {
	items := make([]gin.H, 0, len(episodes))
	for _, episode := range episodes {
		shots := make([]gin.H, 0, len(episode.Shots))
		for _, shot := range episode.Shots {
			shots = append(shots, novelVideoShotResponse(shot))
		}
		items = append(items, gin.H{
			"id":         episode.ID,
			"project_id": episode.ProjectID,
			"number":     episode.Number,
			"title":      episode.Title,
			"summary":    episode.Summary,
			"status":     episode.Status,
			"shots":      shots,
			"created_at": episode.CreatedAt,
			"updated_at": episode.UpdatedAt,
		})
	}
	return items
}

func novelVideoShotResponse(shot NovelVideoShot) gin.H {
	generationSettings := decodeNovelVideoGenerationSettings(shot.GenerationSettingsJSON)
	if shot.ReferenceAssetID != nil && *shot.ReferenceAssetID != 0 && len(generationSettings.ReferenceAssetIDs) == 0 {
		generationSettings.ReferenceAssetIDs = []uint{*shot.ReferenceAssetID}
	}
	return gin.H{
		"id":                        shot.ID,
		"project_id":                shot.ProjectID,
		"episode_id":                shot.EpisodeID,
		"number":                    shot.Number,
		"title":                     shot.Title,
		"prompt":                    shot.Prompt,
		"script_unit_type":          shot.ScriptUnitType,
		"source_excerpt":            shot.SourceExcerpt,
		"duration_seconds":          shot.DurationSeconds,
		"image_prompt":              shot.ImagePrompt,
		"video_prompt":              shot.VideoPrompt,
		"voiceover_text":            shot.VoiceoverText,
		"asset_refs":                decodeNovelVideoAssetRefs(shot.AssetRefsJSON),
		"reference_asset_id":        shot.ReferenceAssetID,
		"generation_settings":       generationSettings,
		"storyboard_url":            shot.StoryboardURL,
		"storyboard_status":         shot.StoryboardStatus,
		"subtitle_text":             shot.SubtitleText,
		"camera_plan":               decodeJSONMap(shot.CameraPlanJSON),
		"reference_asset_ids":       generationSettings.ReferenceAssetIDs,
		"reference_video_asset_ids": generationSettings.ReferenceVideoAssetIDs,
		"reference_audio_asset_ids": generationSettings.ReferenceAudioAssetIDs,
		"generate_audio":            generationSettings.GenerateAudio,
		"creature_ids":              decodeUintList(shot.CreatureIDsJSON),
		"status":                    shot.Status,
		"generation_record_id":      shot.GenerationRecordID,
		"work_id":                   shot.WorkID,
		"work_preview_url":          shot.WorkPreviewURL,
		"work_download_url":         shot.WorkDownloadURL,
		"generation_progress":       shot.GenerationProgress,
		"estimated_credits":         shot.EstimatedCredits,
		"generation_attempts":       shot.RenderAttempts,
		"latest_error":              shot.LatestError,
		"error_code":                shot.ErrorCode,
		"error_message":             shot.ErrorMessage,
		"created_at":                shot.CreatedAt,
		"updated_at":                shot.UpdatedAt,
	}
}

func novelVideoAssetResponse(asset NovelVideoAsset) gin.H {
	return gin.H{
		"id":                   asset.ID,
		"project_id":           asset.ProjectID,
		"user_id":              asset.UserID,
		"kind":                 asset.Kind,
		"name":                 asset.Name,
		"description":          asset.Description,
		"prompt":               asset.Prompt,
		"reference_url":        asset.ReferenceURL,
		"asset_url":            asset.AssetURL,
		"version":              positiveOrDefault(asset.Version, 1),
		"review_status":        asset.ReviewStatus,
		"generation_record_id": asset.GenerationRecordID,
		"work_id":              asset.WorkID,
		"metadata":             decodeJSONMap(asset.MetadataJSON),
		"error_code":           asset.ErrorCode,
		"error_message":        asset.ErrorMessage,
		"created_at":           asset.CreatedAt,
		"updated_at":           asset.UpdatedAt,
	}
}

func novelVideoShotImageResponses(images []NovelVideoShotImage) []gin.H {
	items := make([]gin.H, 0, len(images))
	for _, image := range images {
		items = append(items, novelVideoShotImageResponse(image))
	}
	return items
}

func novelVideoShotImageResponse(image NovelVideoShotImage) gin.H {
	generationRecordID := uint(0)
	if image.GenerationRecordID != nil {
		generationRecordID = *image.GenerationRecordID
	}
	workID := uint(0)
	if image.WorkID != nil {
		workID = *image.WorkID
	}
	return gin.H{
		"id":                   image.ID,
		"project_id":           image.ProjectID,
		"episode_id":           image.EpisodeID,
		"shot_id":              image.ShotID,
		"user_id":              image.UserID,
		"generation_record_id": generationRecordID,
		"work_id":              workID,
		"kind":                 image.Kind,
		"prompt":               image.Prompt,
		"negative_prompt":      image.NegativePrompt,
		"reference_asset_ids":  decodeUintList(image.ReferenceAssetIDsJSON),
		"actor_ids":            decodeUintList(image.ActorIDsJSON),
		"reference_intent":     image.ReferenceIntent,
		"mode":                 image.Mode,
		"lock_level":           image.LockLevel,
		"version":              positiveOrDefault(image.Version, 1),
		"selected":             image.Selected,
		"review_status":        image.ReviewStatus,
		"review_note":          image.ReviewNote,
		"preview_url":          image.PreviewURL,
		"download_url":         image.DownloadURL,
		"error_code":           image.ErrorCode,
		"error_message":        image.ErrorMessage,
		"generation_status":    image.GenerationStatus,
		"generation_stage":     image.GenerationStage,
		"generation_progress":  image.GenerationProgress,
		"created_at":           image.CreatedAt,
		"updated_at":           image.UpdatedAt,
	}
}

func novelVideoJobResponse(job NovelVideoJob) gin.H {
	return gin.H{
		"id":            job.ID,
		"project_id":    job.ProjectID,
		"user_id":       job.UserID,
		"type":          job.JobType,
		"status":        job.Status,
		"episode_id":    job.EpisodeID,
		"shot_id":       job.ShotID,
		"asset_id":      job.AssetID,
		"depends_on_id": job.DependsOnID,
		"attempts":      job.Attempts,
		"max_attempts":  positiveOrDefault(job.MaxAttempts, 3),
		"progress":      job.Progress,
		"payload":       decodeJSONMap(job.PayloadJSON),
		"result":        decodeJSONMap(job.ResultJSON),
		"error_code":    job.ErrorCode,
		"error_message": job.ErrorMessage,
		"started_at":    job.StartedAt,
		"finished_at":   job.FinishedAt,
		"created_at":    job.CreatedAt,
		"updated_at":    job.UpdatedAt,
	}
}

func novelVideoJobResponses(jobs []NovelVideoJob) []gin.H {
	items := make([]gin.H, 0, len(jobs))
	for _, job := range jobs {
		items = append(items, novelVideoJobResponse(job))
	}
	return items
}

func novelVideoCompositionResponse(composition NovelVideoComposition) gin.H {
	return gin.H{
		"id":            composition.ID,
		"project_id":    composition.ProjectID,
		"user_id":       composition.UserID,
		"episode_id":    composition.EpisodeID,
		"job_id":        composition.JobID,
		"work_id":       composition.WorkID,
		"output_url":    composition.OutputURL,
		"subtitle_url":  composition.SubtitleURL,
		"manifest":      decodeJSONMap(composition.ManifestJSON),
		"status":        composition.Status,
		"error_code":    composition.ErrorCode,
		"error_message": composition.ErrorMessage,
		"created_at":    composition.CreatedAt,
		"updated_at":    composition.UpdatedAt,
	}
}

func novelVideoCompositionResponses(compositions []NovelVideoComposition) []gin.H {
	items := make([]gin.H, 0, len(compositions))
	for _, composition := range compositions {
		items = append(items, novelVideoCompositionResponse(composition))
	}
	return items
}

func novelVideoGridResponse(grid NovelVideoGrid) gin.H {
	return gin.H{
		"id":         grid.ID,
		"project_id": grid.ProjectID,
		"user_id":    grid.UserID,
		"episode_id": grid.EpisodeID,
		"grid_type":  grid.GridType,
		"grid_size":  grid.GridSize,
		"shot_ids":   decodeUintList(grid.ShotIDsJSON),
		"prompt":     decodeJSONSlice(grid.PromptJSON),
		"status":     grid.Status,
		"created_at": grid.CreatedAt,
		"updated_at": grid.UpdatedAt,
	}
}

func novelVideoShotProgress(shot NovelVideoShot) int {
	if len(shot.RenderAttempts) > 0 {
		latest := shot.RenderAttempts[len(shot.RenderAttempts)-1]
		if latest.Progress > 0 || latest.Status == GenerationStatusSucceeded {
			return latest.Progress
		}
		return novelVideoStatusProgress(latest.Status)
	}
	return novelVideoStatusProgress(shot.Status)
}

func novelVideoStatusProgress(status string) int {
	switch status {
	case GenerationStatusSucceeded:
		return 100
	case GenerationStatusRunning, NovelVideoProjectStatusRendering:
		return 65
	case GenerationStatusQueued:
		return 0
	case GenerationStatusFailed:
		return 0
	default:
		return 0
	}
}

func buildNovelVideoExportMarkdown(project NovelVideoProject, creatures []NovelVideoCreature, episodes []NovelVideoEpisode) string {
	var b strings.Builder
	b.WriteString("# " + project.Title + "\n\n")
	b.WriteString("## 项目设置\n\n")
	b.WriteString(fmt.Sprintf("- 风格：%s\n- 画幅：%s\n- 单镜头时长：%s 秒\n- 图片模型：%s\n- 视频模型：%s\n\n", project.StylePreset, project.AspectRatio, project.Duration, project.ImageModel, project.VideoModel))
	if story := decodeJSONMap(project.StoryBibleJSON); len(story) > 0 {
		b.WriteString("## 故事圣经\n\n")
		for key, value := range story {
			b.WriteString(fmt.Sprintf("- %s：%v\n", key, value))
		}
		b.WriteString("\n")
	}
	if strings.TrimSpace(project.ContentRiskSummary) != "" {
		b.WriteString("## 风险提示\n\n")
		b.WriteString(project.ContentRiskSummary + "\n\n")
	}
	b.WriteString("## 生物设定\n\n")
	if len(creatures) == 0 {
		b.WriteString("暂无生物卡。\n\n")
	}
	for _, creature := range creatures {
		b.WriteString(fmt.Sprintf("### %s\n\n", creature.Name))
		b.WriteString(fmt.Sprintf("- 类型：%s\n- 外形：%s\n- 能力/习性：%s\n- 一致性提示词：%s\n- 素材 URL：%s\n\n", creature.CreatureType, creature.Appearance, creature.Abilities, creature.VisualConsistencyPrompt, creature.AssetURL))
	}
	b.WriteString("## 分集镜头包\n\n")
	for _, episode := range episodes {
		b.WriteString(fmt.Sprintf("### 第 %d 集：%s\n\n%s\n\n", episode.Number, episode.Title, episode.Summary))
		for _, shot := range episode.Shots {
			b.WriteString(fmt.Sprintf("#### 镜头 %d：%s\n\n", shot.Number, shot.Title))
			b.WriteString("```text\n" + shot.Prompt + "\n```\n\n")
			if shot.WorkID != nil {
				b.WriteString(fmt.Sprintf("- 作品 ID：%d\n", *shot.WorkID))
			}
		}
	}
	return b.String()
}

func buildNovelVideoExportZip(project NovelVideoProject, creatures []NovelVideoCreature, episodes []NovelVideoEpisode, assets []NovelVideoAsset) ([]byte, error) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	files := map[string]string{
		"project.json":       encodeJSON(novelVideoProjectExportPayload(project, creatures, episodes, assets)),
		"project.md":         buildNovelVideoExportMarkdown(project, creatures, episodes),
		"script.json":        encodeJSON(novelVideoScriptExport(episodes)),
		"grids.json":         encodeJSON([]gin.H{}),
		"cost-estimate.json": encodeJSON(buildNovelVideoCostEstimatePayload(project, episodes, nil)),
		"assets.json":        encodeJSON(assets),
		"subtitles.srt":      buildNovelVideoSRT(episodes),
		"manifest.json":      encodeJSON(map[string]any{"schema_version": 2, "format": "novel-video-package", "project_id": project.ID}),
	}
	for name, content := range files {
		if err := writeZipTextFile(writer, name, content); err != nil {
			_ = writer.Close()
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func buildNovelVideoJianyingZip(project NovelVideoProject, creatures []NovelVideoCreature, episodes []NovelVideoEpisode, assets []NovelVideoAsset) ([]byte, error) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	meta := map[string]any{
		"draft_id":       fmt.Sprintf("novel-video-%d", project.ID),
		"draft_name":     project.Title,
		"schema_version": 2,
		"source":         "image-agent",
	}
	content := map[string]any{
		"project":  novelVideoProjectExportPayload(project, creatures, episodes, assets),
		"timeline": novelVideoJianyingTimeline(episodes),
	}
	files := map[string]string{
		"draft_meta_info.json":         encodeJSON(meta),
		"draft_content.json":           encodeJSON(content),
		"materials/project.json":       encodeJSON(novelVideoProjectExportPayload(project, creatures, episodes, assets)),
		"materials/script.json":        encodeJSON(novelVideoScriptExport(episodes)),
		"materials/grids.json":         encodeJSON([]gin.H{}),
		"materials/cost-estimate.json": encodeJSON(buildNovelVideoCostEstimatePayload(project, episodes, nil)),
	}
	for name, body := range files {
		if err := writeZipTextFile(writer, name, body); err != nil {
			_ = writer.Close()
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func buildNovelVideoImagePackageZip(project NovelVideoProject, creatures []NovelVideoCreature, episodes []NovelVideoEpisode, assets []NovelVideoAsset, images []NovelVideoShotImage) ([]byte, error) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	imageItems := novelVideoShotImageResponses(images)
	selected := make([]gin.H, 0)
	for index, image := range images {
		if image.Selected {
			selected = append(selected, imageItems[index])
		}
	}
	actorCards := make([]gin.H, 0, len(creatures))
	for _, creature := range creatures {
		card := novelVideoCreatureResponse(creature)
		card["asset_refs"] = actorAssetRefsForExport(creature, assets)
		actorCards = append(actorCards, card)
	}
	prompts := make([]gin.H, 0)
	for _, episode := range episodes {
		for _, shot := range episode.Shots {
			prompts = append(prompts, gin.H{
				"episode_id":   episode.ID,
				"shot_id":      shot.ID,
				"shot_number":  shot.Number,
				"title":        shot.Title,
				"image_prompt": fallbackString(strings.TrimSpace(shot.ImagePrompt), effectiveNovelVideoShotPrompt(shot)),
				"asset_refs":   decodeNovelVideoAssetRefs(shot.AssetRefsJSON),
				"actor_ids":    decodeUintList(shot.CreatureIDsJSON),
			})
		}
	}
	files := map[string]string{
		"project.json":         encodeJSON(novelVideoProjectExportPayload(project, creatures, episodes, assets)),
		"actor-cards.json":     encodeJSON(actorCards),
		"shot-images.json":     encodeJSON(imageItems),
		"selected-images.json": encodeJSON(selected),
		"prompts.json":         encodeJSON(prompts),
		"manifest.json":        encodeJSON(map[string]any{"schema_version": 3, "format": "image_package", "project_id": project.ID, "image_count": len(images), "selected_count": len(selected)}),
	}
	for name, body := range files {
		if err := writeZipTextFile(writer, name, body); err != nil {
			_ = writer.Close()
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func actorAssetRefsForExport(creature NovelVideoCreature, assets []NovelVideoAsset) []gin.H {
	items := make([]gin.H, 0)
	for _, asset := range assets {
		meta := decodeJSONMap(asset.MetadataJSON)
		if uintFromAny(meta["actor_id"]) != creature.ID {
			continue
		}
		items = append(items, novelVideoAssetResponse(asset))
	}
	return items
}

func writeZipTextFile(writer *zip.Writer, name string, content string) error {
	file, err := writer.Create(name)
	if err != nil {
		return err
	}
	_, err = file.Write([]byte(content))
	return err
}

func novelVideoProjectExportPayload(project NovelVideoProject, creatures []NovelVideoCreature, episodes []NovelVideoEpisode, assets []NovelVideoAsset) gin.H {
	payload := novelVideoProjectResponse(project, creatures, episodes)
	assetItems := make([]gin.H, 0, len(assets))
	for _, asset := range assets {
		assetItems = append(assetItems, novelVideoAssetResponse(asset))
	}
	payload["assets"] = assetItems
	return payload
}

func novelVideoScriptExport(episodes []NovelVideoEpisode) []gin.H {
	items := make([]gin.H, 0)
	for _, episode := range episodes {
		for _, shot := range episode.Shots {
			items = append(items, gin.H{
				"episode_id":       episode.ID,
				"episode_number":   episode.Number,
				"shot_id":          shot.ID,
				"shot_number":      shot.Number,
				"title":            shot.Title,
				"script_unit_type": shot.ScriptUnitType,
				"source_excerpt":   shot.SourceExcerpt,
				"duration_seconds": effectiveNovelVideoShotDurationSeconds(NovelVideoProject{}, shot),
				"image_prompt":     shot.ImagePrompt,
				"video_prompt":     effectiveNovelVideoShotPrompt(shot),
				"voiceover_text":   fallbackString(strings.TrimSpace(shot.VoiceoverText), strings.TrimSpace(shot.SubtitleText)),
				"asset_refs":       decodeNovelVideoAssetRefs(shot.AssetRefsJSON),
			})
		}
	}
	return items
}

func buildNovelVideoCostEstimatePayload(project NovelVideoProject, episodes []NovelVideoEpisode, grids []NovelVideoGrid) gin.H {
	shotItems := make([]gin.H, 0)
	episodeItems := make([]gin.H, 0, len(episodes))
	totalShotCredits := 0
	totalGridCredits := 0
	gridCountByEpisode := map[uint]int{}
	for _, grid := range grids {
		if grid.EpisodeID != nil {
			gridCountByEpisode[*grid.EpisodeID]++
		}
	}
	for _, episode := range episodes {
		episodeShotCredits := 0
		for _, shot := range episode.Shots {
			credits := videoCreditCost(videoGenerationRequest{
				Prompt:      effectiveNovelVideoShotPrompt(shot),
				AspectRatio: project.AspectRatio,
				Duration:    strconv.Itoa(effectiveNovelVideoShotDurationSeconds(project, shot)),
				Model:       project.VideoModel,
				Private:     boolPointer(true),
			})
			episodeShotCredits += credits
			totalShotCredits += credits
			shotItems = append(shotItems, gin.H{
				"shot_id":          shot.ID,
				"episode_id":       episode.ID,
				"render_credits":   credits,
				"audio_credits":    boolToInt(decodeNovelVideoGenerationSettings(shot.GenerationSettingsJSON).GenerateAudio),
				"duration_seconds": effectiveNovelVideoShotDurationSeconds(project, shot),
			})
		}
		gridCredits := gridCountByEpisode[episode.ID]
		if gridCredits == 0 && effectiveNovelVideoGenerationMode(project.GenerationMode) == NovelVideoGenerationModeGrid && len(episode.Shots) > 0 {
			gridSize := effectiveNovelVideoGridSize(project.GridSize)
			gridCredits = (len(episode.Shots) + gridSize - 1) / gridSize
		}
		totalGridCredits += gridCredits
		episodeItems = append(episodeItems, gin.H{
			"episode_id":    episode.ID,
			"shot_credits":  episodeShotCredits,
			"grid_credits":  gridCredits,
			"total_credits": episodeShotCredits + gridCredits,
		})
	}
	return gin.H{
		"project": gin.H{
			"project_id":      project.ID,
			"generation_mode": effectiveNovelVideoGenerationMode(project.GenerationMode),
			"grid_size":       effectiveNovelVideoGridSize(project.GridSize),
			"shot_credits":    totalShotCredits,
			"grid_credits":    totalGridCredits,
			"audio_credits":   0,
			"compose_credits": 0,
			"total_credits":   totalShotCredits + totalGridCredits,
		},
		"episodes": episodeItems,
		"shots":    shotItems,
	}
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func novelVideoGenerationRecordProgress(status, stage string) int {
	switch normalizeGenerationStatus(status) {
	case GenerationStatusQueued:
		return 5
	case GenerationStatusRunning:
		if normalizeGenerationStage(status, stage) == GenerationStagePersistingResult {
			return 85
		}
		return 35
	case GenerationStatusSucceeded, GenerationStatusFailed:
		return 100
	}
	return 0
}

func buildNovelVideoSRT(episodes []NovelVideoEpisode) string {
	var b strings.Builder
	index := 1
	current := 0
	for _, episode := range episodes {
		for _, shot := range episode.Shots {
			text := strings.TrimSpace(shot.VoiceoverText)
			if text == "" {
				text = strings.TrimSpace(shot.SubtitleText)
			}
			if text == "" {
				text = strings.TrimSpace(shot.Title)
			}
			if text == "" {
				text = fmt.Sprintf("镜头 %d", shot.Number)
			}
			start := current
			end := current + effectiveNovelVideoShotDurationSeconds(NovelVideoProject{}, shot)
			b.WriteString(fmt.Sprintf("%d\n%s --> %s\n%s\n\n", index, srtTimestamp(start), srtTimestamp(end), text))
			index++
			current = end
		}
	}
	return b.String()
}

func srtTimestamp(seconds int) string {
	if seconds < 0 {
		seconds = 0
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60
	return fmt.Sprintf("%02d:%02d:%02d,000", h, m, s)
}

func buildNovelVideoSRTFromClips(clips []novelVideoComposeClip) string {
	var b strings.Builder
	index := 1
	current := 0
	for _, clip := range clips {
		text := strings.TrimSpace(clip.Shot.VoiceoverText)
		if text == "" {
			text = strings.TrimSpace(clip.Shot.SubtitleText)
		}
		if text == "" {
			text = strings.TrimSpace(clip.Shot.Title)
		}
		if text == "" {
			text = fmt.Sprintf("shot %d", clip.Shot.Number)
		}
		start := current
		end := current + effectiveNovelVideoShotDurationSeconds(NovelVideoProject{}, clip.Shot)
		b.WriteString(fmt.Sprintf("%d\n%s --> %s\n%s\n\n", index, srtTimestamp(start), srtTimestamp(end), text))
		index++
		current = end
	}
	return b.String()
}

func novelVideoJianyingTimeline(episodes []NovelVideoEpisode) []gin.H {
	items := make([]gin.H, 0)
	order := 0
	for _, episode := range episodes {
		for _, shot := range episode.Shots {
			items = append(items, gin.H{
				"order":          order,
				"episode_number": episode.Number,
				"shot_number":    shot.Number,
				"title":          shot.Title,
				"prompt":         effectiveNovelVideoShotPrompt(shot),
				"video_url":      shot.WorkDownloadURL,
				"subtitle":       fallbackString(strings.TrimSpace(shot.VoiceoverText), fallbackString(strings.TrimSpace(shot.SubtitleText), shot.Title)),
			})
			order++
		}
	}
	return items
}

func encodeJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func decodeJSONMap(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return map[string]any{"raw": raw}
	}
	return decoded
}

func decodeJSONSlice(raw string) []any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var decoded []any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil
	}
	return decoded
}

func decodeUintList(raw string) []uint {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var ids []uint
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil
	}
	return ids
}

func decodeNovelVideoAssetRefs(raw string) []novelVideoAssetRef {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var refs []novelVideoAssetRef
	if err := json.Unmarshal([]byte(raw), &refs); err != nil {
		return nil
	}
	return normalizeNovelVideoAssetRefs(refs)
}

func encodeNovelVideoAssetRefs(refs []novelVideoAssetRef) string {
	refs = normalizeNovelVideoAssetRefs(refs)
	if len(refs) == 0 {
		return ""
	}
	raw, _ := json.Marshal(refs)
	return string(raw)
}

func normalizeNovelVideoAssetRefs(refs []novelVideoAssetRef) []novelVideoAssetRef {
	seen := map[string]bool{}
	normalized := make([]novelVideoAssetRef, 0, len(refs))
	for _, ref := range refs {
		ref.Type = strings.TrimSpace(ref.Type)
		ref.Kind = strings.TrimSpace(ref.Kind)
		ref.Name = strings.TrimSpace(ref.Name)
		if ref.ID == 0 || (ref.Type != "asset" && ref.Type != "creature") {
			continue
		}
		key := fmt.Sprintf("%s:%d", ref.Type, ref.ID)
		if seen[key] {
			continue
		}
		seen[key] = true
		normalized = append(normalized, ref)
	}
	return normalized
}

func (a *App) validateNovelVideoShotAssetRefs(project NovelVideoProject, refs []novelVideoAssetRef) ([]novelVideoAssetRef, error) {
	refs = normalizeNovelVideoAssetRefs(refs)
	for index := range refs {
		ref := &refs[index]
		switch ref.Type {
		case "asset":
			var asset NovelVideoAsset
			if err := a.db.Where("id = ? AND project_id = ? AND user_id = ?", ref.ID, project.ID, project.UserID).First(&asset).Error; err != nil {
				return nil, fmt.Errorf("asset reference %d not found in project", ref.ID)
			}
			ref.Kind = fallbackString(ref.Kind, asset.Kind)
			ref.Name = fallbackString(ref.Name, asset.Name)
		case "creature":
			var creature NovelVideoCreature
			if err := a.db.Where("id = ? AND project_id = ? AND user_id = ?", ref.ID, project.ID, project.UserID).First(&creature).Error; err != nil {
				return nil, fmt.Errorf("creature reference %d not found in project", ref.ID)
			}
			ref.Name = fallbackString(ref.Name, creature.Name)
		}
	}
	return refs, nil
}

func effectiveNovelVideoShotPrompt(shot NovelVideoShot) string {
	return fallbackString(strings.TrimSpace(shot.VideoPrompt), strings.TrimSpace(shot.Prompt))
}

func effectiveNovelVideoShotDurationSeconds(project NovelVideoProject, shot NovelVideoShot) int {
	if shot.DurationSeconds > 0 {
		return shot.DurationSeconds
	}
	if seconds, err := strconv.Atoi(strings.TrimSpace(project.Duration)); err == nil && seconds > 0 {
		return seconds
	}
	return 4
}

func novelVideoExcerpt(text string, maxRunes int) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if maxRunes <= 0 || utf8.RuneCountInString(text) <= maxRunes {
		return text
	}
	runes := []rune(text)
	return string(runes[:maxRunes])
}

func normalizeNovelVideoAspectRatio(value string) string {
	value = strings.TrimSpace(value)
	switch value {
	case "9:16", "16:9":
		return value
	default:
		return "16:9"
	}
}

func normalizeNovelVideoContentMode(value string) string {
	switch strings.TrimSpace(value) {
	case "":
		return NovelVideoContentModeNarration
	case NovelVideoContentModeNarration, NovelVideoContentModeDrama, NovelVideoContentModeAd, NovelVideoContentModeShortFilmImage:
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func normalizeNovelVideoGenerationMode(value string) string {
	switch strings.TrimSpace(value) {
	case "":
		return NovelVideoGenerationModeStoryboard
	case NovelVideoGenerationModeStoryboard, NovelVideoGenerationModeGrid, NovelVideoGenerationModeReferenceVideo, NovelVideoGenerationModeImageSeries:
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func effectiveNovelVideoGenerationMode(value string) string {
	mode := normalizeNovelVideoGenerationMode(value)
	if mode == "" {
		return NovelVideoGenerationModeStoryboard
	}
	return mode
}

func normalizeNovelVideoGridSize(value int) int {
	switch value {
	case 4, 6, 9:
		return value
	default:
		return 4
	}
}

func effectiveNovelVideoGridSize(value int) int {
	return normalizeNovelVideoGridSize(value)
}

func normalizeNovelVideoAssetKinds(values []string) []string {
	if len(values) == 0 {
		values = []string{
			NovelVideoAssetKindCharacter,
			NovelVideoAssetKindScene,
			NovelVideoAssetKindProp,
			NovelVideoAssetKindClue,
			NovelVideoAssetKindStyle,
		}
	}
	seen := map[string]bool{}
	kinds := make([]string, 0, len(values))
	for _, value := range values {
		kind := normalizeNovelVideoAssetKind(value)
		if kind == "" || seen[kind] {
			continue
		}
		seen[kind] = true
		kinds = append(kinds, kind)
	}
	return kinds
}

func normalizeNovelVideoAssetKind(value string) string {
	switch strings.TrimSpace(value) {
	case NovelVideoAssetKindCharacter, NovelVideoAssetKindScene, NovelVideoAssetKindProp, NovelVideoAssetKindClue, NovelVideoAssetKindStyle, NovelVideoAssetKindActorRef, NovelVideoAssetKindActorKeySheet, NovelVideoAssetKindShotImage:
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func fallbackNovelVideoAssets(project NovelVideoProject, kinds []string) []NovelVideoAsset {
	story := decodeJSONMap(project.StoryBibleJSON)
	logline := fallbackString(fmt.Sprint(story["logline"]), novelVideoExcerpt(project.SourceText, 80))
	world := fallbackString(fmt.Sprint(story["world"]), project.Title)
	style := fallbackString(fmt.Sprint(story["visual_style"]), fallbackString(project.StylePreset, "电影感写实"))
	assets := make([]NovelVideoAsset, 0, len(kinds))
	for _, kind := range kinds {
		name, description := fallbackNovelVideoAssetName(kind, project.Title, world)
		prompt := strings.Join([]string{
			"小说视频资产设定图，保持后续镜头一致性。",
			"内容模式：" + effectiveNovelVideoContentMode(project.ContentMode),
			"项目：" + project.Title,
			"故事线索：" + logline,
			"世界观：" + world,
			"视觉风格：" + style,
			"资产类型：" + kind,
			"资产名称：" + name,
			"描述：" + description,
		}, "\n")
		assets = append(assets, NovelVideoAsset{
			ProjectID:    project.ID,
			UserID:       project.UserID,
			Kind:         kind,
			Name:         name,
			Description:  description,
			Prompt:       prompt,
			Version:      1,
			ReviewStatus: NovelVideoReviewStatusNeedsReview,
			MetadataJSON: encodeJSON(map[string]any{"source": "fallback", "content_mode": effectiveNovelVideoContentMode(project.ContentMode)}),
		})
	}
	return assets
}

func fallbackNovelVideoAssetName(kind, title, world string) (string, string) {
	base := fallbackString(strings.TrimSpace(title), "小说")
	switch kind {
	case NovelVideoAssetKindCharacter:
		return base + "主角视觉锚点", "主角或关键角色的外观、服饰、年龄感、气质和可复用镜头一致性约束。"
	case NovelVideoAssetKindScene:
		return fallbackString(strings.TrimSpace(world), base) + "核心场景", "最常出现的空间环境、时代材质、光线、天气和镜头调度参考。"
	case NovelVideoAssetKindProp:
		return base + "关键道具", "推动剧情的核心物件，包含材质、尺寸、磨损痕迹和特写识别点。"
	case NovelVideoAssetKindClue:
		return base + "悬念线索", "用于分集钩子的视觉线索，强调可反复出现但不提前剧透的细节。"
	case NovelVideoAssetKindStyle:
		return base + "风格参考", "统一色彩、摄影、景深、颗粒、构图和镜头运动语言。"
	default:
		return base + "资产", "小说视频通用资产。"
	}
}

func buildNovelVideoStoryboardPrompt(project NovelVideoProject, shot NovelVideoShot) string {
	return strings.Join([]string{
		"小说视频分镜图，单帧关键画面。",
		"项目：" + project.Title,
		"内容模式：" + effectiveNovelVideoContentMode(project.ContentMode),
		"风格：" + fallbackString(project.StylePreset, "电影感写实"),
		"镜头标题：" + shot.Title,
		"镜头提示词：" + shot.Prompt,
	}, "\n")
}

func novelVideoComposeErrorCode(err error) string {
	text := strings.ToLower(err.Error())
	if strings.Contains(text, "ffmpeg") || strings.Contains(text, "ffprobe") {
		return "ffmpeg_unavailable"
	}
	if strings.Contains(text, "clip") || strings.Contains(text, "asset key") || strings.Contains(text, "rendered") {
		return "novel_video_compose_missing_clips"
	}
	return "novel_video_compose_failed"
}

func effectiveNovelVideoContentMode(value string) string {
	mode := normalizeNovelVideoContentMode(value)
	if mode == "" {
		return NovelVideoContentModeNarration
	}
	return mode
}

func effectiveNovelVideoSchemaVersion(value int) int {
	if value <= 0 {
		return 1
	}
	return value
}

func defaultNovelVideoSchemaVersion(contentMode, generationMode string) int {
	if contentMode == NovelVideoContentModeShortFilmImage || generationMode == NovelVideoGenerationModeImageSeries {
		return 3
	}
	return 2
}

func normalizeNovelVideoImageGenerationMode(value string) string {
	switch strings.TrimSpace(value) {
	case "image_to_image":
		return "image_to_image"
	default:
		return "text_to_image"
	}
}

func normalizeNovelVideoActorLockLevel(value string) string {
	switch strings.TrimSpace(value) {
	case "loose", "medium", "strict":
		return strings.TrimSpace(value)
	default:
		return "medium"
	}
}

func normalizeNovelVideoReviewStatus(value string) string {
	switch strings.TrimSpace(value) {
	case NovelVideoReviewStatusApproved, GenerationStatusSucceeded, GenerationStatusFailed:
		return strings.TrimSpace(value)
	case NovelVideoReviewStatusDraft:
		return NovelVideoReviewStatusDraft
	default:
		return NovelVideoReviewStatusNeedsReview
	}
}

func pointerString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func pointerInt(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func containsUint(values []uint, target uint) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func uintFromAny(value any) uint {
	switch typed := value.(type) {
	case uint:
		return typed
	case int:
		if typed > 0 {
			return uint(typed)
		}
	case int64:
		if typed > 0 {
			return uint(typed)
		}
	case float64:
		if typed > 0 {
			return uint(typed)
		}
	case json.Number:
		if parsed, err := typed.Int64(); err == nil && parsed > 0 {
			return uint(parsed)
		}
	}
	return 0
}

func boolPointer(value bool) *bool {
	return &value
}
