package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestNovelVideoProjectCreateEnforcesTextLimitAndUserIsolation(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	_, ownerCookies := createLoggedInUser(t, testApp, "novel_owner", "test-password")
	_, otherCookies := createLoggedInUser(t, testApp, "novel_other", "test-password")

	createResp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects", map[string]any{
		"title":        "雾潮之兽",
		"source_text":  "第一章，海雾里有会发光的鳞片。",
		"style_preset": "东方奇幻电影感",
		"aspect_ratio": "16:9",
		"duration":     "10",
	}, ownerCookies)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("expected create 201, got %d: %s", createResp.Code, createResp.Body.String())
	}

	var created struct {
		ID          uint   `json:"id"`
		Title       string `json:"title"`
		Status      string `json:"status"`
		SourceChars int    `json:"source_chars"`
		VideoModel  string `json:"video_model"`
	}
	if err := json.Unmarshal(createResp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.ID == 0 || created.Title != "雾潮之兽" || created.Status != NovelVideoProjectStatusDraft || created.SourceChars == 0 || created.VideoModel != wuyinGrokImagineRuntimeModel {
		t.Fatalf("unexpected create payload: %+v", created)
	}

	otherResp := performJSONRequest(t, testApp, http.MethodGet, "/api/novel-video-projects/"+itoa(created.ID), nil, otherCookies)
	if otherResp.Code != http.StatusNotFound {
		t.Fatalf("expected other user 404, got %d: %s", otherResp.Code, otherResp.Body.String())
	}

	tooLongResp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects", map[string]any{
		"title":       "太长文本",
		"source_text": strings.Repeat("字", novelVideoMaxSourceChars+1),
	}, ownerCookies)
	if tooLongResp.Code != http.StatusBadRequest {
		t.Fatalf("expected too long 400, got %d: %s", tooLongResp.Code, tooLongResp.Body.String())
	}
}

func TestNovelVideoProjectCreateReportsMissingSchemaAndLogsDetail(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	_, cookies := createLoggedInUser(t, testApp, "novel_missing_schema", "test-password")
	if err := db.Migrator().DropTable(&NovelVideoProject{}); err != nil {
		t.Fatalf("drop novel video projects table: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects", map[string]any{
		"title":       "缺表项目",
		"source_text": "第一章，数据库表尚未创建。",
	}, cookies)
	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected missing schema 500, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "小说视频项目数据表未初始化，请联系管理员执行数据库迁移后重试") {
		t.Fatalf("expected safe missing schema message, got: %s", resp.Body.String())
	}
	if strings.Contains(strings.ToLower(resp.Body.String()), "no such table") {
		t.Fatalf("response leaked raw database detail: %s", resp.Body.String())
	}

	var logItem SystemRequestLog
	if err := db.Where("path = ? AND error_code = ?", "/api/novel-video-projects", "novel_video_project_create_failed").
		Order("created_at desc").
		First(&logItem).Error; err != nil {
		t.Fatalf("load request log: %v", err)
	}
	if !strings.Contains(strings.ToLower(logItem.ErrorDetail), "no such table") ||
		!strings.Contains(logItem.ErrorDetail, "novel_video_projects") {
		t.Fatalf("expected raw db detail in request log, got: %q", logItem.ErrorDetail)
	}
}

func TestNovelVideoProjectExportIncludesStoryCreatureAndShotPrompts(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "novel_exporter", "test-password")

	project := NovelVideoProject{
		UserID:         user.ID,
		Title:          "灰塔兽群",
		SourceText:     "灰塔里有三种守门兽。",
		StylePreset:    "冷峻写实",
		AspectRatio:    "16:9",
		Duration:       "10",
		VideoModel:     "sora-2",
		Status:         NovelVideoProjectStatusPlanned,
		StoryBibleJSON: `{"logline":"守塔人穿过兽群抵达塔顶","world":"潮湿石塔"}`,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	creature := NovelVideoCreature{
		ProjectID:               project.ID,
		UserID:                  user.ID,
		Name:                    "灰鳞门兽",
		CreatureType:            "守门生物",
		Appearance:              "背部有岩片，眼睛像低温火焰",
		Abilities:               "可听见石墙里的脚步声",
		VisualConsistencyPrompt: "同一只灰鳞门兽，岩片背脊，蓝白眼焰",
		ReviewStatus:            NovelVideoReviewStatusApproved,
		AssetURL:                "https://cdn.example.test/creature.png",
	}
	if err := db.Create(&creature).Error; err != nil {
		t.Fatalf("create creature: %v", err)
	}
	episode := NovelVideoEpisode{
		ProjectID: project.ID,
		UserID:    user.ID,
		Number:    1,
		Title:     "进入灰塔",
		Summary:   "主角发现兽群不是敌人。",
		Status:    NovelVideoReviewStatusApproved,
	}
	if err := db.Create(&episode).Error; err != nil {
		t.Fatalf("create episode: %v", err)
	}
	shot := NovelVideoShot{
		ProjectID:       project.ID,
		EpisodeID:       episode.ID,
		UserID:          user.ID,
		Number:          1,
		Title:           "门兽抬头",
		Prompt:          "低机位，灰鳞门兽从雾气中抬头，蓝白眼焰照亮石门。",
		CreatureIDsJSON: `[1]`,
		Status:          NovelVideoReviewStatusApproved,
	}
	if err := db.Create(&shot).Error; err != nil {
		t.Fatalf("create shot: %v", err)
	}

	exportResp := performJSONRequest(t, testApp, http.MethodGet, "/api/novel-video-projects/"+itoa(project.ID)+"/export", nil, cookies)
	if exportResp.Code != http.StatusOK {
		t.Fatalf("expected export 200, got %d: %s", exportResp.Code, exportResp.Body.String())
	}
	body := exportResp.Body.String()
	for _, expected := range []string{"# 灰塔兽群", "守塔人穿过兽群抵达塔顶", "灰鳞门兽", "进入灰塔", "门兽抬头", "低机位"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected export to contain %q, got:\n%s", expected, body)
		}
	}
}

func TestNovelVideoProjectPatchUpdatesEditableFieldsAndRejectsOtherUsers(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	owner, ownerCookies := createLoggedInUser(t, testApp, "novel_patch_owner", "test-password")
	_, otherCookies := createLoggedInUser(t, testApp, "novel_patch_other", "test-password")

	project := NovelVideoProject{
		UserID:         owner.ID,
		Title:          "旧标题",
		SourceText:     "旧文本",
		StylePreset:    "旧风格",
		AspectRatio:    "16:9",
		Duration:       "10",
		VideoModel:     "sora-2",
		Status:         NovelVideoProjectStatusAnalyzed,
		StoryBibleJSON: `{"logline":"旧一句话"}`,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}

	patchResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/novel-video-projects/"+itoa(project.ID), map[string]any{
		"title":                "新标题",
		"style_preset":         "水墨电影感",
		"aspect_ratio":         "9:16",
		"duration":             "15",
		"video_model":          "sora-2-pro",
		"content_risk_summary": "低风险",
		"story_bible": map[string]any{
			"logline":         "新一句话",
			"world":           "潮湿石塔",
			"conflict":        "守塔人与兽群互相试探",
			"visual_style":    "冷峻写实",
			"risk_highlight":  "无明显风险",
			"ignored_unknown": "保留 JSON 但不影响响应",
		},
	}, ownerCookies)
	if patchResp.Code != http.StatusOK {
		t.Fatalf("expected patch 200, got %d: %s", patchResp.Code, patchResp.Body.String())
	}
	var payload struct {
		Title              string         `json:"title"`
		StylePreset        string         `json:"style_preset"`
		AspectRatio        string         `json:"aspect_ratio"`
		Duration           string         `json:"duration"`
		VideoModel         string         `json:"video_model"`
		ContentRiskSummary string         `json:"content_risk_summary"`
		StoryBible         map[string]any `json:"story_bible"`
	}
	if err := json.Unmarshal(patchResp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode patch response: %v", err)
	}
	if payload.Title != "新标题" || payload.AspectRatio != "9:16" || payload.Duration != "15" || payload.VideoModel != "sora-2-pro" {
		t.Fatalf("unexpected patched settings: %+v", payload)
	}
	if payload.StoryBible["logline"] != "新一句话" || payload.ContentRiskSummary != "低风险" {
		t.Fatalf("unexpected patched story bible: %+v", payload)
	}

	otherResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/novel-video-projects/"+itoa(project.ID), map[string]any{
		"title": "越权修改",
	}, otherCookies)
	if otherResp.Code != http.StatusNotFound {
		t.Fatalf("expected other user 404, got %d: %s", otherResp.Code, otherResp.Body.String())
	}
}

func TestNovelVideoProjectDetailIncludesGenerationMetadataAndJSONExport(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "novel_detail_metadata", "test-password")

	project := NovelVideoProject{UserID: user.ID, Title: "灰塔兽群", SourceText: "灰塔里有兽群。", AspectRatio: "16:9", Duration: "10", VideoModel: "sora-2", Status: NovelVideoProjectStatusPlanned}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	work := Work{UserID: user.ID, PreviewURL: "/api/works/99/file", DownloadURL: "/api/works/99/download", Status: GenerationStatusSucceeded}
	if err := db.Create(&work).Error; err != nil {
		t.Fatalf("create work: %v", err)
	}
	record := GenerationRecord{UserID: user.ID, WorkID: &work.ID, Status: GenerationStatusSucceeded, PreviewURL: work.PreviewURL, DownloadURL: work.DownloadURL, CreditsCost: 3}
	if err := db.Create(&record).Error; err != nil {
		t.Fatalf("create record: %v", err)
	}
	work.GenerationRecordID = record.ID
	if err := db.Save(&work).Error; err != nil {
		t.Fatalf("save work: %v", err)
	}
	creature := NovelVideoCreature{
		ProjectID: project.ID,
		UserID:    user.ID,
		Name:      "灰鳞门兽",
		WorkID:    &work.ID,
		AssetURL:  work.PreviewURL,
	}
	if err := db.Create(&creature).Error; err != nil {
		t.Fatalf("create creature: %v", err)
	}
	episode := NovelVideoEpisode{ProjectID: project.ID, UserID: user.ID, Number: 1, Title: "第一集", Status: NovelVideoReviewStatusApproved}
	if err := db.Create(&episode).Error; err != nil {
		t.Fatalf("create episode: %v", err)
	}
	shot := NovelVideoShot{ProjectID: project.ID, EpisodeID: episode.ID, UserID: user.ID, Number: 1, Title: "门兽抬头", Prompt: "低机位", Status: GenerationStatusSucceeded, GenerationRecordID: &record.ID, WorkID: &work.ID}
	if err := db.Create(&shot).Error; err != nil {
		t.Fatalf("create shot: %v", err)
	}
	attempt := NovelVideoShotRenderAttempt{ProjectID: project.ID, EpisodeID: episode.ID, ShotID: shot.ID, UserID: user.ID, GenerationRecordID: &record.ID, Status: GenerationStatusSucceeded, Progress: 100}
	if err := db.Create(&attempt).Error; err != nil {
		t.Fatalf("create attempt: %v", err)
	}

	detailResp := performJSONRequest(t, testApp, http.MethodGet, "/api/novel-video-projects/"+itoa(project.ID), nil, cookies)
	if detailResp.Code != http.StatusOK {
		t.Fatalf("expected detail 200, got %d: %s", detailResp.Code, detailResp.Body.String())
	}
	body := detailResp.Body.String()
	for _, expected := range []string{`"work_preview_url":"/api/works/99/file"`, `"work_download_url":"/api/works/99/download"`, `"generation_progress":100`, `"generation_attempts":[`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected detail to contain %s, got: %s", expected, body)
		}
	}

	jsonResp := performJSONRequest(t, testApp, http.MethodGet, "/api/novel-video-projects/"+itoa(project.ID)+"/export?format=json", nil, cookies)
	if jsonResp.Code != http.StatusOK {
		t.Fatalf("expected json export 200, got %d: %s", jsonResp.Code, jsonResp.Body.String())
	}
	if !strings.Contains(jsonResp.Body.String(), `"episodes"`) || !strings.Contains(jsonResp.Body.String(), `"灰鳞门兽"`) {
		t.Fatalf("unexpected json export: %s", jsonResp.Body.String())
	}
}

func TestNovelVideoCreatureImageGenerationReturnsActiveRecordBeforeProviderCompletes(t *testing.T) {
	provider := newBlockingImageProvider()
	testApp, db := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "novel_creature_async", "test-password")
	setUserCredits(t, testApp, user.ID, 100)

	project := NovelVideoProject{
		UserID:      user.ID,
		Title:       "Creature async",
		SourceText:  "A creature waits for its portrait.",
		StylePreset: "cinematic",
		AspectRatio: "16:9",
		Duration:    "10",
		ImageModel:  "gpt-image-2",
		VideoModel:  "sora-2",
		Status:      NovelVideoProjectStatusPlanned,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	creature := NovelVideoCreature{
		ProjectID:               project.ID,
		UserID:                  user.ID,
		Name:                    "Ash Keeper",
		CreatureType:            "guardian",
		Appearance:              "charcoal scales",
		Abilities:               "keeps embers alive",
		VisualConsistencyPrompt: "same charcoal guardian",
		ReviewStatus:            NovelVideoReviewStatusNeedsReview,
		ErrorCode:               "old_error",
		ErrorMessage:            "old failed message",
	}
	if err := db.Create(&creature).Error; err != nil {
		t.Fatalf("create creature: %v", err)
	}

	released := false
	releaseProvider := func() {
		if !released {
			close(provider.release)
			released = true
		}
	}
	defer releaseProvider()

	respCh := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		respCh <- performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects/"+itoa(project.ID)+"/creatures/"+itoa(creature.ID)+"/generate-image", map[string]any{}, cookies)
	}()

	var resp *httptest.ResponseRecorder
	select {
	case resp = <-respCh:
	case <-time.After(150 * time.Millisecond):
		releaseProvider()
		<-respCh
		t.Fatalf("expected creature image endpoint to return before provider generation completes")
	}
	if resp.Code != http.StatusOK {
		t.Fatalf("expected generate creature image 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		ID                 uint   `json:"id"`
		GenerationRecordID *uint  `json:"generation_record_id"`
		GenerationStatus   string `json:"generation_status"`
		ErrorCode          string `json:"error_code"`
		ErrorMessage       string `json:"error_message"`
		LatestError        string `json:"latest_error"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode creature response: %v", err)
	}
	if payload.ID != creature.ID || payload.GenerationRecordID == nil || *payload.GenerationRecordID == 0 {
		t.Fatalf("expected active generation record in response, got %+v", payload)
	}
	if payload.GenerationStatus != GenerationStatusQueued && payload.GenerationStatus != GenerationStatusRunning {
		t.Fatalf("expected queued/running response status, got %+v", payload)
	}
	if payload.ErrorCode != "" || payload.ErrorMessage != "" || payload.LatestError != "" {
		t.Fatalf("expected old creature errors to be cleared while active, got %+v", payload)
	}

	waitForCondition(t, time.Second, func() bool {
		return provider.callCount() == 1
	})

	var persisted NovelVideoCreature
	if err := db.First(&persisted, creature.ID).Error; err != nil {
		t.Fatalf("reload creature: %v", err)
	}
	if persisted.GenerationRecordID == nil || *persisted.GenerationRecordID != *payload.GenerationRecordID || persisted.ErrorCode != "" || persisted.ErrorMessage != "" {
		t.Fatalf("expected persisted creature to point at active record with cleared errors, got %+v", persisted)
	}

	detailResp := performJSONRequest(t, testApp, http.MethodGet, "/api/novel-video-projects/"+itoa(project.ID), nil, cookies)
	if detailResp.Code != http.StatusOK {
		t.Fatalf("expected project detail 200, got %d: %s", detailResp.Code, detailResp.Body.String())
	}
	var detail struct {
		Creatures []struct {
			ID               uint   `json:"id"`
			GenerationStatus string `json:"generation_status"`
			LatestError      string `json:"latest_error"`
			ErrorMessage     string `json:"error_message"`
		} `json:"creatures"`
	}
	if err := json.Unmarshal(detailResp.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode detail response: %v", err)
	}
	if len(detail.Creatures) != 1 || (detail.Creatures[0].GenerationStatus != GenerationStatusQueued && detail.Creatures[0].GenerationStatus != GenerationStatusRunning) || detail.Creatures[0].LatestError != "" || detail.Creatures[0].ErrorMessage != "" {
		t.Fatalf("expected project detail to expose active generation without stale failure, got %+v", detail.Creatures)
	}
}

func TestNovelVideoCreatureImageGenerationRecordsTerminalFailureOnlyAfterProviderFails(t *testing.T) {
	provider := &promptRoutingImageProvider{}
	testApp, db := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "novel_creature_failure", "test-password")
	setUserCredits(t, testApp, user.ID, 100)

	project := NovelVideoProject{UserID: user.ID, Title: "Creature failure", SourceText: "source", StylePreset: "cinematic", AspectRatio: "16:9", Duration: "10", ImageModel: "gpt-image-2", VideoModel: "sora-2", Status: NovelVideoProjectStatusPlanned}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	creature := NovelVideoCreature{ProjectID: project.ID, UserID: user.ID, Name: "fail creature", CreatureType: "guardian", Appearance: "dark", Abilities: "fail signal", VisualConsistencyPrompt: "same", ReviewStatus: NovelVideoReviewStatusNeedsReview, ErrorMessage: "old failed message"}
	if err := db.Create(&creature).Error; err != nil {
		t.Fatalf("create creature: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects/"+itoa(project.ID)+"/creatures/"+itoa(creature.ID)+"/generate-image", map[string]any{}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected generate creature image 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		GenerationRecordID *uint  `json:"generation_record_id"`
		GenerationStatus   string `json:"generation_status"`
		ErrorMessage       string `json:"error_message"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.GenerationRecordID == nil || payload.ErrorMessage != "" || (payload.GenerationStatus != GenerationStatusQueued && payload.GenerationStatus != GenerationStatusRunning) {
		t.Fatalf("expected active response without stale error, got %+v", payload)
	}

	waitForCondition(t, 3*time.Second, func() bool {
		var refreshed NovelVideoCreature
		if err := db.First(&refreshed, creature.ID).Error; err != nil {
			return false
		}
		return refreshed.ErrorCode != "" && refreshed.ErrorMessage != ""
	})
	var failed NovelVideoCreature
	if err := db.First(&failed, creature.ID).Error; err != nil {
		t.Fatalf("reload failed creature: %v", err)
	}
	if failed.GenerationRecordID == nil || *failed.GenerationRecordID != *payload.GenerationRecordID || failed.ErrorCode == "" || failed.ErrorMessage == "old failed message" {
		t.Fatalf("expected provider terminal failure on current record, got %+v", failed)
	}

	detailResp := performJSONRequest(t, testApp, http.MethodGet, "/api/novel-video-projects/"+itoa(project.ID), nil, cookies)
	if detailResp.Code != http.StatusOK {
		t.Fatalf("expected detail 200, got %d: %s", detailResp.Code, detailResp.Body.String())
	}
	var detail struct {
		Creatures []struct {
			GenerationStatus string `json:"generation_status"`
			LatestError      string `json:"latest_error"`
		} `json:"creatures"`
	}
	if err := json.Unmarshal(detailResp.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if len(detail.Creatures) != 1 || detail.Creatures[0].GenerationStatus != GenerationStatusFailed || detail.Creatures[0].LatestError == "" {
		t.Fatalf("expected detail to show terminal failed state, got %+v", detail.Creatures)
	}
}

func TestNovelVideoCreatureImageGenerationOldRecordCannotOverwriteNewerRetry(t *testing.T) {
	provider := newDelayedSuccessImageProvider()
	testApp, db := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "novel_creature_retry_guard", "test-password")
	setUserCredits(t, testApp, user.ID, 100)

	project := NovelVideoProject{UserID: user.ID, Title: "Creature retry guard", SourceText: "source", StylePreset: "cinematic", AspectRatio: "16:9", Duration: "10", ImageModel: "gpt-image-2", VideoModel: "sora-2", Status: NovelVideoProjectStatusPlanned}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	creature := NovelVideoCreature{ProjectID: project.ID, UserID: user.ID, Name: "Ash Keeper", CreatureType: "guardian", Appearance: "charcoal", Abilities: "embers", VisualConsistencyPrompt: "same", ReviewStatus: NovelVideoReviewStatusNeedsReview}
	if err := db.Create(&creature).Error; err != nil {
		t.Fatalf("create creature: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects/"+itoa(project.ID)+"/creatures/"+itoa(creature.ID)+"/generate-image", map[string]any{}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected generate creature image 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		GenerationRecordID *uint `json:"generation_record_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.GenerationRecordID == nil {
		t.Fatalf("expected first active record, got %+v", payload)
	}
	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatalf("provider did not start")
	}

	newerRecord := GenerationRecord{UserID: user.ID, Prompt: "newer retry", Status: GenerationStatusQueued, Stage: GenerationStageQueued}
	if err := db.Create(&newerRecord).Error; err != nil {
		t.Fatalf("create newer record: %v", err)
	}
	if err := db.Model(&NovelVideoCreature{}).Where("id = ?", creature.ID).Updates(map[string]any{
		"generation_record_id": newerRecord.ID,
		"error_code":           "",
		"error_message":        "",
	}).Error; err != nil {
		t.Fatalf("point creature at newer record: %v", err)
	}
	close(provider.release)

	waitForCondition(t, 3*time.Second, func() bool {
		var oldRecord GenerationRecord
		if err := db.First(&oldRecord, *payload.GenerationRecordID).Error; err != nil {
			return false
		}
		return oldRecord.Status == GenerationStatusSucceeded
	})
	var refreshed NovelVideoCreature
	if err := db.First(&refreshed, creature.ID).Error; err != nil {
		t.Fatalf("reload creature: %v", err)
	}
	if refreshed.GenerationRecordID == nil || *refreshed.GenerationRecordID != newerRecord.ID || refreshed.WorkID != nil || refreshed.AssetURL != "" {
		t.Fatalf("expected stale first record not to overwrite newer retry, got %+v", refreshed)
	}
}

type novelVideoCreatureImageAssetDetail struct {
	Creatures []struct {
		ID                 uint   `json:"id"`
		AssetURL           string `json:"asset_url"`
		GenerationRecordID *uint  `json:"generation_record_id"`
		WorkID             *uint  `json:"work_id"`
	} `json:"creatures"`
	Assets []novelVideoCreatureImageAssetItem `json:"assets"`
}

type novelVideoCreatureImageAssetItem struct {
	ID                 uint           `json:"id"`
	Kind               string         `json:"kind"`
	Name               string         `json:"name"`
	AssetURL           string         `json:"asset_url"`
	Prompt             string         `json:"prompt"`
	ReviewStatus       string         `json:"review_status"`
	GenerationRecordID *uint          `json:"generation_record_id"`
	WorkID             *uint          `json:"work_id"`
	Metadata           map[string]any `json:"metadata"`
	ErrorCode          string         `json:"error_code"`
	ErrorMessage       string         `json:"error_message"`
}

func loadNovelVideoCreatureImageAssetDetail(t *testing.T, app *App, projectID uint, cookies []*http.Cookie) novelVideoCreatureImageAssetDetail {
	t.Helper()
	resp := performJSONRequest(t, app, http.MethodGet, "/api/novel-video-projects/"+itoa(projectID), nil, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected project detail 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var detail novelVideoCreatureImageAssetDetail
	if err := json.Unmarshal(resp.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode project detail: %v", err)
	}
	return detail
}

func findActorRefAssetForCreature(t *testing.T, detail novelVideoCreatureImageAssetDetail, creatureID uint) []novelVideoCreatureImageAssetItem {
	t.Helper()
	matches := make([]novelVideoCreatureImageAssetItem, 0)
	for _, asset := range detail.Assets {
		if asset.Kind == NovelVideoAssetKindActorRef && uintFromAny(asset.Metadata["actor_id"]) == creatureID {
			matches = append(matches, asset)
		}
	}
	return matches
}

func TestNovelVideoCreatureImageGenerationSyncsActorRefAsset(t *testing.T) {
	provider := &promptRoutingImageProvider{}
	testApp, db := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "novel_creature_asset_sync", "test-password")
	setUserCredits(t, testApp, user.ID, 100)

	project := NovelVideoProject{UserID: user.ID, Title: "Creature asset sync", SourceText: "source", StylePreset: "cinematic", AspectRatio: "16:9", Duration: "10", ImageModel: "gpt-image-2", VideoModel: "sora-2", Status: NovelVideoProjectStatusPlanned}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	creature := NovelVideoCreature{ProjectID: project.ID, UserID: user.ID, Name: "Ash Keeper", CreatureType: "guardian", Appearance: "charcoal", Abilities: "embers", VisualConsistencyPrompt: "same ash keeper", ReviewStatus: NovelVideoReviewStatusApproved}
	if err := db.Create(&creature).Error; err != nil {
		t.Fatalf("create creature: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects/"+itoa(project.ID)+"/creatures/"+itoa(creature.ID)+"/generate-image", map[string]any{}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected generate creature image 200, got %d: %s", resp.Code, resp.Body.String())
	}

	waitForCondition(t, 3*time.Second, func() bool {
		var refreshed NovelVideoCreature
		if err := db.First(&refreshed, creature.ID).Error; err != nil {
			return false
		}
		return refreshed.AssetURL != "" && refreshed.WorkID != nil && refreshed.GenerationRecordID != nil
	})
	detail := loadNovelVideoCreatureImageAssetDetail(t, testApp, project.ID, cookies)
	if len(detail.Creatures) != 1 || detail.Creatures[0].AssetURL == "" || detail.Creatures[0].GenerationRecordID == nil || detail.Creatures[0].WorkID == nil {
		t.Fatalf("expected detail creature to expose generated image, got %+v", detail.Creatures)
	}
	matches := findActorRefAssetForCreature(t, detail, creature.ID)
	if len(matches) != 1 {
		t.Fatalf("expected exactly one actor_ref asset for creature, got %+v", detail.Assets)
	}
	asset := matches[0]
	if asset.AssetURL != detail.Creatures[0].AssetURL || asset.GenerationRecordID == nil || *asset.GenerationRecordID != *detail.Creatures[0].GenerationRecordID || asset.WorkID == nil || *asset.WorkID != *detail.Creatures[0].WorkID {
		t.Fatalf("expected actor_ref asset to share creature generation output, creature=%+v asset=%+v", detail.Creatures[0], asset)
	}
	if asset.Prompt == "" || asset.ReviewStatus != NovelVideoReviewStatusApproved || asset.ErrorCode != "" || asset.ErrorMessage != "" {
		t.Fatalf("expected actor_ref metadata/status/errors to be synchronized, got %+v", asset)
	}
}

func TestNovelVideoCreatureImageGenerationFillsExistingActorRefAsset(t *testing.T) {
	provider := &promptRoutingImageProvider{}
	testApp, db := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "novel_creature_asset_reuse", "test-password")
	setUserCredits(t, testApp, user.ID, 100)

	project := NovelVideoProject{UserID: user.ID, Title: "Creature asset reuse", SourceText: "source", StylePreset: "cinematic", AspectRatio: "16:9", Duration: "10", ImageModel: "gpt-image-2", VideoModel: "sora-2", Status: NovelVideoProjectStatusPlanned}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	creature := NovelVideoCreature{ProjectID: project.ID, UserID: user.ID, Name: "Ash Keeper", CreatureType: "guardian", Appearance: "charcoal", Abilities: "embers", VisualConsistencyPrompt: "same ash keeper", ReviewStatus: NovelVideoReviewStatusNeedsReview}
	if err := db.Create(&creature).Error; err != nil {
		t.Fatalf("create creature: %v", err)
	}
	existing := NovelVideoAsset{ProjectID: project.ID, UserID: user.ID, Kind: NovelVideoAssetKindActorRef, Name: "Ash Keeper reference", Prompt: "old prompt", Version: 1, ReviewStatus: NovelVideoReviewStatusApproved, MetadataJSON: encodeJSON(map[string]any{"actor_id": creature.ID, "source": "image_plan"}), ErrorCode: "old_error", ErrorMessage: "old message"}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("create existing asset: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects/"+itoa(project.ID)+"/creatures/"+itoa(creature.ID)+"/generate-image", map[string]any{}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected generate creature image 200, got %d: %s", resp.Code, resp.Body.String())
	}
	waitForCondition(t, 3*time.Second, func() bool {
		var refreshed NovelVideoAsset
		if err := db.First(&refreshed, existing.ID).Error; err != nil {
			return false
		}
		return refreshed.AssetURL != "" && refreshed.GenerationRecordID != nil && refreshed.WorkID != nil
	})

	detail := loadNovelVideoCreatureImageAssetDetail(t, testApp, project.ID, cookies)
	matches := findActorRefAssetForCreature(t, detail, creature.ID)
	if len(matches) != 1 || matches[0].ID != existing.ID {
		t.Fatalf("expected existing actor_ref asset to be filled without duplicate, matches=%+v assets=%+v", matches, detail.Assets)
	}
	if matches[0].ReviewStatus != NovelVideoReviewStatusApproved || matches[0].ErrorCode != "" || matches[0].ErrorMessage != "" {
		t.Fatalf("expected existing review status preserved and errors cleared, got %+v", matches[0])
	}
}

func TestNovelVideoCreatureImageGenerationCreatesActorRefAssetWhenMissing(t *testing.T) {
	provider := &promptRoutingImageProvider{}
	testApp, db := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "novel_creature_asset_create", "test-password")
	setUserCredits(t, testApp, user.ID, 100)

	project := NovelVideoProject{UserID: user.ID, Title: "Creature asset create", SourceText: "source", StylePreset: "cinematic", AspectRatio: "16:9", Duration: "10", ImageModel: "gpt-image-2", VideoModel: "sora-2", Status: NovelVideoProjectStatusPlanned}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	creature := NovelVideoCreature{ProjectID: project.ID, UserID: user.ID, Name: "Silver Warden", CreatureType: "guardian", Appearance: "silver", Abilities: "watch", VisualConsistencyPrompt: "same silver warden", ReviewStatus: ""}
	if err := db.Create(&creature).Error; err != nil {
		t.Fatalf("create creature: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects/"+itoa(project.ID)+"/creatures/"+itoa(creature.ID)+"/generate-image", map[string]any{}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected generate creature image 200, got %d: %s", resp.Code, resp.Body.String())
	}
	waitForCondition(t, 3*time.Second, func() bool {
		var count int64
		if err := db.Model(&NovelVideoAsset{}).Where("project_id = ? AND kind = ?", project.ID, NovelVideoAssetKindActorRef).Count(&count).Error; err != nil {
			return false
		}
		return count == 1
	})

	detail := loadNovelVideoCreatureImageAssetDetail(t, testApp, project.ID, cookies)
	matches := findActorRefAssetForCreature(t, detail, creature.ID)
	if len(matches) != 1 || matches[0].AssetURL == "" || matches[0].ReviewStatus != NovelVideoReviewStatusNeedsReview {
		t.Fatalf("expected new needs_review actor_ref with generated image, matches=%+v assets=%+v", matches, detail.Assets)
	}
}

func TestNovelVideoCreatureImageGenerationFailureDoesNotSyncActorRefAsset(t *testing.T) {
	provider := &promptRoutingImageProvider{}
	testApp, db := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "novel_creature_asset_failure", "test-password")
	setUserCredits(t, testApp, user.ID, 100)

	project := NovelVideoProject{UserID: user.ID, Title: "Creature asset failure", SourceText: "source", StylePreset: "cinematic", AspectRatio: "16:9", Duration: "10", ImageModel: "gpt-image-2", VideoModel: "sora-2", Status: NovelVideoProjectStatusPlanned}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	creature := NovelVideoCreature{ProjectID: project.ID, UserID: user.ID, Name: "fail creature", CreatureType: "guardian", Appearance: "dark", Abilities: "fail signal", VisualConsistencyPrompt: "same", ReviewStatus: NovelVideoReviewStatusNeedsReview}
	if err := db.Create(&creature).Error; err != nil {
		t.Fatalf("create creature: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects/"+itoa(project.ID)+"/creatures/"+itoa(creature.ID)+"/generate-image", map[string]any{}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected generate creature image 200, got %d: %s", resp.Code, resp.Body.String())
	}
	waitForCondition(t, 3*time.Second, func() bool {
		var refreshed NovelVideoCreature
		if err := db.First(&refreshed, creature.ID).Error; err != nil {
			return false
		}
		return refreshed.ErrorCode != "" && refreshed.ErrorMessage != ""
	})

	var count int64
	if err := db.Model(&NovelVideoAsset{}).Where("project_id = ?", project.ID).Count(&count).Error; err != nil {
		t.Fatalf("count assets: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected failed creature generation not to create assets, got %d", count)
	}
}

func TestNovelVideoCreatureImageGenerationStaleRecordDoesNotSyncActorRefAsset(t *testing.T) {
	provider := newDelayedSuccessImageProvider()
	testApp, db := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "novel_creature_asset_stale", "test-password")
	setUserCredits(t, testApp, user.ID, 100)

	project := NovelVideoProject{UserID: user.ID, Title: "Creature asset stale", SourceText: "source", StylePreset: "cinematic", AspectRatio: "16:9", Duration: "10", ImageModel: "gpt-image-2", VideoModel: "sora-2", Status: NovelVideoProjectStatusPlanned}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	creature := NovelVideoCreature{ProjectID: project.ID, UserID: user.ID, Name: "Ash Keeper", CreatureType: "guardian", Appearance: "charcoal", Abilities: "embers", VisualConsistencyPrompt: "same", ReviewStatus: NovelVideoReviewStatusNeedsReview}
	if err := db.Create(&creature).Error; err != nil {
		t.Fatalf("create creature: %v", err)
	}
	existing := NovelVideoAsset{ProjectID: project.ID, UserID: user.ID, Kind: NovelVideoAssetKindActorRef, Name: "Ash Keeper reference", Version: 1, ReviewStatus: NovelVideoReviewStatusNeedsReview, MetadataJSON: encodeJSON(map[string]any{"actor_id": creature.ID})}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("create existing asset: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects/"+itoa(project.ID)+"/creatures/"+itoa(creature.ID)+"/generate-image", map[string]any{}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected generate creature image 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		GenerationRecordID *uint `json:"generation_record_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.GenerationRecordID == nil {
		t.Fatalf("expected first active record, got %+v", payload)
	}
	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatalf("provider did not start")
	}
	newerRecord := GenerationRecord{UserID: user.ID, Prompt: "newer retry", Status: GenerationStatusQueued, Stage: GenerationStageQueued}
	if err := db.Create(&newerRecord).Error; err != nil {
		t.Fatalf("create newer record: %v", err)
	}
	if err := db.Model(&NovelVideoCreature{}).Where("id = ?", creature.ID).Update("generation_record_id", newerRecord.ID).Error; err != nil {
		t.Fatalf("point creature at newer record: %v", err)
	}
	close(provider.release)

	waitForCondition(t, 3*time.Second, func() bool {
		var oldRecord GenerationRecord
		if err := db.First(&oldRecord, *payload.GenerationRecordID).Error; err != nil {
			return false
		}
		return oldRecord.Status == GenerationStatusSucceeded
	})
	var refreshed NovelVideoAsset
	if err := db.First(&refreshed, existing.ID).Error; err != nil {
		t.Fatalf("reload existing asset: %v", err)
	}
	if refreshed.AssetURL != "" || refreshed.GenerationRecordID != nil || refreshed.WorkID != nil {
		t.Fatalf("expected stale first record not to update actor_ref asset, got %+v", refreshed)
	}
}

func TestNovelVideoRenderApprovedShotsQueuesAttemptsAndRunsInEpisodeOrder(t *testing.T) {
	provider := &stubProvider{}
	testApp, db := newTestApp(t, provider)
	videoProvider := &stubVideoProvider{submitResult: VideoSubmitResult{TaskID: "task-1"}}
	testApp.videoProvider = videoProvider

	user, cookies := createLoggedInUser(t, testApp, "novel_renderer", "test-password")
	setUserCredits(t, testApp, user.ID, 100)
	project := NovelVideoProject{
		UserID:      user.ID,
		Title:       "镜头顺序测试",
		SourceText:  "从门口到塔顶。",
		AspectRatio: "16:9",
		Duration:    "10",
		VideoModel:  "sora-2",
		Status:      NovelVideoProjectStatusPlanned,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	episodeOne := NovelVideoEpisode{ProjectID: project.ID, UserID: user.ID, Number: 1, Title: "第一集", Status: NovelVideoReviewStatusApproved}
	episodeTwo := NovelVideoEpisode{ProjectID: project.ID, UserID: user.ID, Number: 2, Title: "第二集", Status: NovelVideoReviewStatusApproved}
	if err := db.Create(&episodeOne).Error; err != nil {
		t.Fatalf("create episode one: %v", err)
	}
	if err := db.Create(&episodeTwo).Error; err != nil {
		t.Fatalf("create episode two: %v", err)
	}
	shots := []NovelVideoShot{
		{ProjectID: project.ID, EpisodeID: episodeTwo.ID, UserID: user.ID, Number: 1, Title: "后生成但排序靠后", Prompt: "第二集镜头", Status: NovelVideoReviewStatusApproved},
		{ProjectID: project.ID, EpisodeID: episodeOne.ID, UserID: user.ID, Number: 2, Title: "同集第二镜头", Prompt: "第一集第二镜头", Status: NovelVideoReviewStatusApproved},
		{ProjectID: project.ID, EpisodeID: episodeOne.ID, UserID: user.ID, Number: 1, Title: "同集第一镜头", Prompt: "第一集第一镜头", Status: NovelVideoReviewStatusApproved},
	}
	if err := db.Create(&shots).Error; err != nil {
		t.Fatalf("create shots: %v", err)
	}

	renderResp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects/"+itoa(project.ID)+"/render-approved-shots", map[string]any{}, cookies)
	if renderResp.Code != http.StatusOK {
		t.Fatalf("expected render 200, got %d: %s", renderResp.Code, renderResp.Body.String())
	}
	var renderPayload struct {
		Status           string `json:"status"`
		Queued           int    `json:"queued"`
		Skipped          int    `json:"skipped"`
		RequiredCredits  int    `json:"required_credits"`
		AvailableCredits int    `json:"available_credits"`
		Total            int    `json:"total"`
	}
	if err := json.Unmarshal(renderResp.Body.Bytes(), &renderPayload); err != nil {
		t.Fatalf("decode render response: %v", err)
	}
	if renderPayload.Status != GenerationStatusQueued || renderPayload.Queued != 3 || renderPayload.Total != 3 || renderPayload.RequiredCredits <= 0 || renderPayload.AvailableCredits <= 0 {
		t.Fatalf("unexpected queued render payload: %+v", renderPayload)
	}

	waitForCondition(t, time.Second, func() bool {
		return len(videoProvider.submitInputs) == 3
	})
	gotPrompts := make([]string, 0, len(videoProvider.submitInputs))
	for _, input := range videoProvider.submitInputs {
		gotPrompts = append(gotPrompts, input.Prompt)
	}
	wantPrompts := []string{"第一集第一镜头", "第一集第二镜头", "第二集镜头"}
	if strings.Join(gotPrompts, "|") != strings.Join(wantPrompts, "|") {
		t.Fatalf("unexpected render order: got %#v want %#v", gotPrompts, wantPrompts)
	}

	waitForCondition(t, 3*time.Second, func() bool {
		var completedAttempts int64
		if err := db.Model(&NovelVideoShotRenderAttempt{}).
			Where("project_id = ? AND status = ? AND progress = ? AND generation_record_id IS NOT NULL", project.ID, GenerationStatusSucceeded, 100).
			Count(&completedAttempts).Error; err != nil {
			return false
		}
		var completedShots int64
		if err := db.Model(&NovelVideoShot{}).
			Where("project_id = ? AND status = ? AND generation_record_id IS NOT NULL AND work_id IS NOT NULL", project.ID, GenerationStatusSucceeded).
			Count(&completedShots).Error; err != nil {
			return false
		}
		return completedAttempts == 3 && completedShots == 3
	})

	var attempts []NovelVideoShotRenderAttempt
	if err := db.Where("project_id = ?", project.ID).Order("id asc").Find(&attempts).Error; err != nil {
		t.Fatalf("load attempts: %v", err)
	}
	if len(attempts) != 3 {
		t.Fatalf("expected 3 attempts, got %d: %+v", len(attempts), attempts)
	}
	for _, attempt := range attempts {
		if attempt.Status != GenerationStatusSucceeded || attempt.Progress != 100 || attempt.GenerationRecordID == nil {
			t.Fatalf("expected succeeded attempt, got %+v", attempt)
		}
	}

	var updated []NovelVideoShot
	if err := db.Where("project_id = ?", project.ID).Order("id asc").Find(&updated).Error; err != nil {
		t.Fatalf("load updated shots: %v", err)
	}
	for _, shot := range updated {
		if shot.Status != GenerationStatusSucceeded || shot.GenerationRecordID == nil || shot.WorkID == nil {
			t.Fatalf("expected shot rendered with linked work, got %+v", shot)
		}
	}
}

func TestNovelVideoShotGenerationUsesWuyinModelCenterProviderKey(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	videoProvider := &stubVideoProvider{
		submitResult: VideoSubmitResult{TaskID: "novel-grok-task"},
		pollResults: []VideoTaskResult{
			{TaskID: "novel-grok-task", Status: VideoTaskSucceeded, OutputBase64: base64.StdEncoding.EncodeToString([]byte("novel-grok-video")), MIMEType: "video/mp4"},
		},
	}
	testApp.videoProvider = videoProvider

	user, _ := createLoggedInUser(t, testApp, "novel_wuyin_key", "test-password")
	setUserCredits(t, testApp, user.ID, 20)
	var grok ModelConfig
	if err := db.Where("runtime_model = ?", wuyinGrokImagineRuntimeModel).First(&grok).Error; err != nil {
		t.Fatalf("load Grok model config: %v", err)
	}
	setWuyinModelCenterProviderKey(t, testApp, grok.ID, "novel-model-center-wuyin-key")

	project := NovelVideoProject{
		UserID:      user.ID,
		Title:       "Grok 分镜",
		SourceText:  "测试文本",
		AspectRatio: "9:16",
		Duration:    "6",
		VideoModel:  wuyinGrokImagineRuntimeModel,
		Status:      NovelVideoProjectStatusPlanned,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	episode := NovelVideoEpisode{ProjectID: project.ID, UserID: user.ID, Number: 1, Title: "第一集", Status: NovelVideoReviewStatusApproved}
	if err := db.Create(&episode).Error; err != nil {
		t.Fatalf("create episode: %v", err)
	}
	shot := NovelVideoShot{ProjectID: project.ID, EpisodeID: episode.ID, UserID: user.ID, Number: 1, Title: "第一镜", Prompt: "雨夜街角", Status: NovelVideoReviewStatusApproved}
	if err := db.Create(&shot).Error; err != nil {
		t.Fatalf("create shot: %v", err)
	}
	attempt := NovelVideoShotRenderAttempt{ProjectID: project.ID, EpisodeID: episode.ID, ShotID: shot.ID, UserID: user.ID, Status: GenerationStatusQueued}
	if err := db.Create(&attempt).Error; err != nil {
		t.Fatalf("create attempt: %v", err)
	}

	record, err := testApp.runNovelVideoShotGeneration(project, shot, attempt)
	if err != nil {
		t.Fatalf("run novel video shot generation: %v", err)
	}
	if len(videoProvider.submitInputs) != 1 {
		t.Fatalf("expected one video provider call, got %d", len(videoProvider.submitInputs))
	}
	input := videoProvider.submitInputs[0]
	if input.ProviderAPIKey != "novel-model-center-wuyin-key" {
		t.Fatalf("expected novel video Wuyin key from model center, got %+v", input)
	}
	if record.ModelID == 0 || record.ChannelID == 0 || record.RuntimeModel != wuyinGrokImagineRuntimeModel {
		t.Fatalf("expected novel video generation record model center fields, got %+v", record)
	}
}

func TestNovelVideoProjectVideoSettingsShotReferencesAndRenderPreflight(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	videoProvider := &stubVideoProvider{submitResult: VideoSubmitResult{TaskID: "preflight-must-not-submit"}}
	testApp.videoProvider = videoProvider

	now := time.Now()
	if err := db.Model(&ModelConfig{}).
		Where("runtime_model = ?", arkSeedance2RuntimeModel).
		Updates(map[string]any{
			"permission":                 ModelConfigPermissionPublic,
			"api_key":                    "ark-seedance-2-key",
			"video_readiness_status":     arkVideoReadinessStatusPassed,
			"video_readiness_reason":     "",
			"video_readiness_checked_at": &now,
		}).Error; err != nil {
		t.Fatalf("publish Seedance 2.0 model: %v", err)
	}

	user, cookies := createLoggedInUser(t, testApp, "novel_video_settings", "test-password")
	setUserCredits(t, testApp, user.ID, 1000)
	image := seedReferenceAsset(t, testApp, user.ID, "keyframe.png", "image/png", []byte("image-ref"))
	video := seedReferenceAsset(t, testApp, user.ID, "motion.mp4", "video/mp4", []byte("video-ref"))
	audio := seedReferenceAsset(t, testApp, user.ID, "music.mp3", "audio/mpeg", []byte("audio-ref"))

	createResp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects", map[string]any{
		"title":       "多模态短片",
		"source_text": "第一章，角色在雨夜穿过霓虹街区。",
		"video_settings": map[string]any{
			"model":          arkSeedance2RuntimeModel,
			"aspect_ratio":   "16:9",
			"duration":       "11",
			"resolution":     "1080p",
			"generate_audio": true,
		},
	}, cookies)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("expected create 201, got %d: %s", createResp.Code, createResp.Body.String())
	}
	var created struct {
		ID            uint `json:"id"`
		VideoSettings struct {
			Model      string `json:"model"`
			Duration   string `json:"duration"`
			Resolution string `json:"resolution"`
		} `json:"video_settings"`
	}
	if err := json.Unmarshal(createResp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.VideoSettings.Model != arkSeedance2RuntimeModel || created.VideoSettings.Duration != "11" || created.VideoSettings.Resolution != "1080p" {
		t.Fatalf("expected Seedance 2.0 video settings in create response, got %+v", created.VideoSettings)
	}

	episode := NovelVideoEpisode{ProjectID: created.ID, UserID: user.ID, Number: 1, Title: "第一集", Status: NovelVideoReviewStatusApproved}
	if err := db.Create(&episode).Error; err != nil {
		t.Fatalf("create episode: %v", err)
	}
	shot := NovelVideoShot{ProjectID: created.ID, EpisodeID: episode.ID, UserID: user.ID, Number: 1, Title: "雨夜街区", Prompt: "角色走过雨夜霓虹街区，镜头平滑跟随。", Status: NovelVideoReviewStatusApproved}
	if err := db.Create(&shot).Error; err != nil {
		t.Fatalf("create shot: %v", err)
	}

	patchResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/novel-video-projects/"+itoa(created.ID)+"/shots/"+itoa(shot.ID), map[string]any{
		"prompt":                    shot.Prompt,
		"reference_asset_ids":       []uint{image.ID},
		"reference_video_asset_ids": []uint{video.ID},
		"reference_audio_asset_ids": []uint{audio.ID},
		"generate_audio":            true,
	}, cookies)
	if patchResp.Code != http.StatusOK {
		t.Fatalf("expected shot patch 200, got %d: %s", patchResp.Code, patchResp.Body.String())
	}
	if !strings.Contains(patchResp.Body.String(), `"reference_video_asset_ids":[`) ||
		!strings.Contains(patchResp.Body.String(), `"reference_audio_asset_ids":[`) ||
		!strings.Contains(patchResp.Body.String(), `"generate_audio":true`) {
		t.Fatalf("expected typed generation settings in shot response, got: %s", patchResp.Body.String())
	}

	preflightResp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects/"+itoa(created.ID)+"/render-preflight", map[string]any{}, cookies)
	if preflightResp.Code != http.StatusOK {
		t.Fatalf("expected preflight 200, got %d: %s", preflightResp.Code, preflightResp.Body.String())
	}
	var preflight struct {
		Renderable        int  `json:"renderable"`
		Blocked           int  `json:"blocked"`
		RequiredCredits   int  `json:"required_credits"`
		Enough            bool `json:"enough"`
		ProviderSubmitted int
		Shots             []struct {
			ShotID            uint     `json:"shot_id"`
			CanRender         bool     `json:"can_render"`
			RequiredCredits   int      `json:"required_credits"`
			BlockReasons      []string `json:"block_reasons"`
			EffectiveSettings struct {
				Model                  string `json:"model"`
				Resolution             string `json:"resolution"`
				GenerateAudio          bool   `json:"generate_audio"`
				ReferenceAssetIDs      []uint `json:"reference_asset_ids"`
				ReferenceVideoAssetIDs []uint `json:"reference_video_asset_ids"`
				ReferenceAudioAssetIDs []uint `json:"reference_audio_asset_ids"`
			} `json:"effective_settings"`
		} `json:"shots"`
	}
	if err := json.Unmarshal(preflightResp.Body.Bytes(), &preflight); err != nil {
		t.Fatalf("decode preflight response: %v", err)
	}
	if preflight.Renderable != 1 || preflight.Blocked != 0 || !preflight.Enough || preflight.RequiredCredits != 550 || len(preflight.Shots) != 1 {
		t.Fatalf("unexpected preflight summary: %+v", preflight)
	}
	shotPreflight := preflight.Shots[0]
	if !shotPreflight.CanRender || shotPreflight.RequiredCredits != 550 || len(shotPreflight.BlockReasons) != 0 {
		t.Fatalf("unexpected shot preflight: %+v", shotPreflight)
	}
	if shotPreflight.EffectiveSettings.Model != arkSeedance2RuntimeModel ||
		shotPreflight.EffectiveSettings.Resolution != "1080p" ||
		!shotPreflight.EffectiveSettings.GenerateAudio ||
		len(shotPreflight.EffectiveSettings.ReferenceAssetIDs) != 1 ||
		len(shotPreflight.EffectiveSettings.ReferenceVideoAssetIDs) != 1 ||
		len(shotPreflight.EffectiveSettings.ReferenceAudioAssetIDs) != 1 {
		t.Fatalf("unexpected effective settings: %+v", shotPreflight.EffectiveSettings)
	}
	if len(videoProvider.submitInputs) != 0 {
		t.Fatalf("preflight must not submit provider task, got %+v", videoProvider.submitInputs)
	}
	var attempts int64
	if err := db.Model(&NovelVideoShotRenderAttempt{}).Where("project_id = ?", created.ID).Count(&attempts).Error; err != nil {
		t.Fatalf("count attempts: %v", err)
	}
	if attempts != 0 {
		t.Fatalf("preflight must not create attempts, got %d", attempts)
	}
}

func TestNovelVideoShotGenerationPassesSeedance2ReferenceMedia(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	videoProvider := &stubVideoProvider{
		submitResult: VideoSubmitResult{TaskID: "novel-seedance2-task"},
		pollResults: []VideoTaskResult{
			{TaskID: "novel-seedance2-task", Status: VideoTaskSucceeded, OutputBase64: base64.StdEncoding.EncodeToString([]byte("novel-seedance2-video")), MIMEType: "video/mp4"},
		},
	}
	testApp.videoProvider = videoProvider

	now := time.Now()
	if err := db.Model(&ModelConfig{}).
		Where("runtime_model = ?", arkSeedance2RuntimeModel).
		Updates(map[string]any{
			"permission":                 ModelConfigPermissionPublic,
			"api_key":                    "ark-seedance-2-key",
			"video_readiness_status":     arkVideoReadinessStatusPassed,
			"video_readiness_reason":     "",
			"video_readiness_checked_at": &now,
		}).Error; err != nil {
		t.Fatalf("publish Seedance 2.0 model: %v", err)
	}

	user, _ := createLoggedInUser(t, testApp, "novel_seedance2_refs", "test-password")
	setUserCredits(t, testApp, user.ID, 1000)
	image := seedReferenceAsset(t, testApp, user.ID, "keyframe.png", "image/png", []byte("image-ref"))
	video := seedReferenceAsset(t, testApp, user.ID, "motion.mp4", "video/mp4", []byte("video-ref"))
	audio := seedReferenceAsset(t, testApp, user.ID, "music.mp3", "audio/mpeg", []byte("audio-ref"))

	project := NovelVideoProject{
		UserID:            user.ID,
		Title:             "Seedance 小说镜头",
		SourceText:        "雨夜镜头",
		AspectRatio:       "16:9",
		Duration:          "11",
		VideoModel:        arkSeedance2RuntimeModel,
		VideoSettingsJSON: `{"model":"doubao-seedance-2-0-260128","aspect_ratio":"16:9","duration":"11","resolution":"1080p"}`,
		Status:            NovelVideoProjectStatusPlanned,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	episode := NovelVideoEpisode{ProjectID: project.ID, UserID: user.ID, Number: 1, Title: "第一集", Status: NovelVideoReviewStatusApproved}
	if err := db.Create(&episode).Error; err != nil {
		t.Fatalf("create episode: %v", err)
	}
	settings, _ := json.Marshal(novelVideoGenerationSettings{
		ReferenceAssetIDs:      []uint{image.ID},
		ReferenceVideoAssetIDs: []uint{video.ID},
		ReferenceAudioAssetIDs: []uint{audio.ID},
		GenerateAudio:          true,
	})
	shot := NovelVideoShot{
		ProjectID:              project.ID,
		EpisodeID:              episode.ID,
		UserID:                 user.ID,
		Number:                 1,
		Title:                  "雨夜街区",
		Prompt:                 "角色走过雨夜霓虹街区，镜头平滑跟随。",
		Status:                 NovelVideoReviewStatusApproved,
		GenerationSettingsJSON: string(settings),
	}
	if err := db.Create(&shot).Error; err != nil {
		t.Fatalf("create shot: %v", err)
	}
	attempt := NovelVideoShotRenderAttempt{ProjectID: project.ID, EpisodeID: episode.ID, ShotID: shot.ID, UserID: user.ID, Status: GenerationStatusQueued}
	if err := db.Create(&attempt).Error; err != nil {
		t.Fatalf("create attempt: %v", err)
	}

	if _, err := testApp.runNovelVideoShotGeneration(project, shot, attempt); err != nil {
		t.Fatalf("run novel Seedance 2.0 shot generation: %v", err)
	}
	if len(videoProvider.submitInputs) != 1 {
		t.Fatalf("expected one provider call, got %d", len(videoProvider.submitInputs))
	}
	input := videoProvider.submitInputs[0]
	if input.Model != arkSeedance2RuntimeModel || input.Duration != "11" || input.Resolution != "1080p" || !input.GenerateAudio {
		t.Fatalf("unexpected Seedance 2.0 request settings: %+v", input)
	}
	if len(input.Images) != 1 || len(input.ReferenceVideos) != 1 || len(input.ReferenceAudios) != 1 {
		t.Fatalf("expected image/video/audio references, got %+v", input)
	}
}

func TestParseNovelVideoAnalysisPlanExtractsStrictJSON(t *testing.T) {
	content := "```json\n{\"story_bible\":{\"logline\":\"守塔人穿过兽群抵达塔顶\"},\"creatures\":[{\"name\":\"灰鳞门兽\",\"creature_type\":\"守门生物\",\"appearance\":\"岩片背脊\",\"abilities\":\"听见石墙脚步\",\"visual_consistency_prompt\":\"同一只灰鳞门兽\"}],\"content_risk_summary\":\"低风险\"}\n```"

	plan, err := parseNovelVideoAnalysisPlan(content)
	if err != nil {
		t.Fatalf("parse analysis plan: %v", err)
	}
	if plan["content_risk_summary"] != "低风险" {
		t.Fatalf("unexpected risk summary: %+v", plan)
	}
	creatures, ok := plan["creatures"].([]novelVideoCreatureDraft)
	if !ok || len(creatures) != 1 || creatures[0].Name != "灰鳞门兽" {
		t.Fatalf("unexpected creatures: %#v", plan["creatures"])
	}
}

func TestParseNovelVideoEpisodePlanRequiresUsableShots(t *testing.T) {
	_, err := parseNovelVideoEpisodePlan(`{"episodes":[{"number":1,"title":"第一集","summary":"缺少镜头","shots":[]}]}`)
	if err == nil {
		t.Fatal("expected empty shots to be rejected")
	}

	episodes, err := parseNovelVideoEpisodePlan(`{"episodes":[{"number":1,"title":"第一集","summary":"进入灰塔","shots":[{"number":1,"title":"门兽抬头","prompt":"低机位，门兽抬头。"},{"number":2,"title":"石门开启","prompt":"石门缓慢开启。"},{"number":3,"title":"雾气涌入","prompt":"雾气吞没走廊。"}]}]}`)
	if err != nil {
		t.Fatalf("parse episode plan: %v", err)
	}
	if len(episodes) != 1 || len(episodes[0].Shots) != 3 || episodes[0].Shots[0].Prompt == "" {
		t.Fatalf("unexpected episodes: %+v", episodes)
	}
}

func TestNovelVideoProjectCreatePersistsContentModeAndSchemaVersion(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	_, cookies := createLoggedInUser(t, testApp, "novel_mode_owner", "test-password")

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects", map[string]any{
		"title":        "短剧改编",
		"source_text":  "第一章，主角在雨夜收到一封旧信。",
		"content_mode": "drama",
	}, cookies)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected create 201, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		ContentMode   string `json:"content_mode"`
		SchemaVersion int    `json:"schema_version"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if payload.ContentMode != "drama" || payload.SchemaVersion != 2 {
		t.Fatalf("expected drama schema v2, got %+v", payload)
	}

	invalidResp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects", map[string]any{
		"title":        "非法模式",
		"source_text":  "正文",
		"content_mode": "documentary",
	}, cookies)
	if invalidResp.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid mode 400, got %d: %s", invalidResp.Code, invalidResp.Body.String())
	}
}

func TestNovelVideoProjectGenerationModeAndGridSizeRoundTrip(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	_, cookies := createLoggedInUser(t, testApp, "novel_mode_grid_owner", "test-password")

	createResp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects", map[string]any{
		"title":           "宫格短片",
		"source_text":     "第一章，主角进入雨夜街区。",
		"generation_mode": "grid",
		"grid_size":       6,
	}, cookies)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("expected create 201, got %d: %s", createResp.Code, createResp.Body.String())
	}
	var created struct {
		ID             uint   `json:"id"`
		GenerationMode string `json:"generation_mode"`
		GridSize       int    `json:"grid_size"`
	}
	if err := json.Unmarshal(createResp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.GenerationMode != "grid" || created.GridSize != 6 {
		t.Fatalf("unexpected generation mode create payload: %+v", created)
	}

	patchResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/novel-video-projects/"+itoa(created.ID), map[string]any{
		"generation_mode": "reference_video",
		"grid_size":       9,
	}, cookies)
	if patchResp.Code != http.StatusOK {
		t.Fatalf("expected patch 200, got %d: %s", patchResp.Code, patchResp.Body.String())
	}
	var patched struct {
		GenerationMode string `json:"generation_mode"`
		GridSize       int    `json:"grid_size"`
	}
	if err := json.Unmarshal(patchResp.Body.Bytes(), &patched); err != nil {
		t.Fatalf("decode patch response: %v", err)
	}
	if patched.GenerationMode != "reference_video" || patched.GridSize != 9 {
		t.Fatalf("unexpected generation mode patch payload: %+v", patched)
	}
}

func TestNovelVideoShortFilmImageProjectActorLockAndImagePlan(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	owner, ownerCookies := createLoggedInUser(t, testApp, "novel_image_owner", "test-password")
	other, otherCookies := createLoggedInUser(t, testApp, "novel_image_other", "test-password")
	ownerRef := seedReferenceAsset(t, testApp, owner.ID, "actor-front.png", "image/png", []byte("actor-front"))
	otherRef := seedReferenceAsset(t, testApp, other.ID, "actor-other.png", "image/png", []byte("actor-other"))

	createResp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects", map[string]any{
		"title":           "夜航短电影",
		"source_text":     "三名演员在暴雨夜航站寻找失踪胶片，最终发现旧放映厅里的线索。",
		"content_mode":    "short_film_image",
		"generation_mode": "image_series",
		"aspect_ratio":    "16:9",
		"style_preset":    "写实短电影",
	}, ownerCookies)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("expected image project create 201, got %d: %s", createResp.Code, createResp.Body.String())
	}
	var created struct {
		ID             uint   `json:"id"`
		ContentMode    string `json:"content_mode"`
		GenerationMode string `json:"generation_mode"`
		SchemaVersion  int    `json:"schema_version"`
	}
	if err := json.Unmarshal(createResp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode image project: %v", err)
	}
	if created.ContentMode != "short_film_image" || created.GenerationMode != "image_series" || created.SchemaVersion != 3 {
		t.Fatalf("unexpected image project defaults: %+v", created)
	}

	planResp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects/"+itoa(created.ID)+"/image-plan", map[string]any{
		"shot_count": 6,
	}, ownerCookies)
	if planResp.Code != http.StatusOK {
		t.Fatalf("expected image-plan 200, got %d: %s", planResp.Code, planResp.Body.String())
	}
	var planned struct {
		SchemaVersion int `json:"schema_version"`
		Creatures     []struct {
			ID uint `json:"id"`
		} `json:"creatures"`
		Episodes []struct {
			Shots []struct {
				ID                uint   `json:"id"`
				ImagePrompt       string `json:"image_prompt"`
				ReferenceAssetIDs []uint `json:"reference_asset_ids"`
				CreatureIDs       []uint `json:"creature_ids"`
			} `json:"shots"`
		} `json:"episodes"`
		Assets []struct {
			Kind string `json:"kind"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(planResp.Body.Bytes(), &planned); err != nil {
		t.Fatalf("decode image plan: %v", err)
	}
	if planned.SchemaVersion != 3 || len(planned.Creatures) < 3 || len(planned.Episodes) == 0 || len(planned.Episodes[0].Shots) == 0 {
		t.Fatalf("unexpected image plan payload: %+v", planned)
	}
	if planned.Episodes[0].Shots[0].ImagePrompt == "" || len(planned.Episodes[0].Shots[0].CreatureIDs) == 0 {
		t.Fatalf("expected image prompt and actor ids on planned shot: %+v", planned.Episodes[0].Shots[0])
	}
	hasActorAsset := false
	for _, asset := range planned.Assets {
		if asset.Kind == "actor_ref" || asset.Kind == "actor_key_sheet" {
			hasActorAsset = true
		}
	}
	if !hasActorAsset {
		t.Fatalf("expected actor assets in image plan, got %+v", planned.Assets)
	}

	actorID := planned.Creatures[0].ID
	otherRefResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/novel-video-projects/"+itoa(created.ID)+"/actors/"+itoa(actorID), map[string]any{
		"reference_asset_ids": []uint{otherRef.ID},
	}, ownerCookies)
	if otherRefResp.Code != http.StatusNotFound {
		t.Fatalf("expected cross-user reference 404, got %d: %s", otherRefResp.Code, otherRefResp.Body.String())
	}

	patchResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/novel-video-projects/"+itoa(created.ID)+"/actors/"+itoa(actorID), map[string]any{
		"reference_asset_ids":       []uint{ownerRef.ID},
		"canonical_asset_id":        ownerRef.ID,
		"visual_consistency_prompt": "同一名短发女演员，左眼下有小痣，雨衣造型保持一致",
		"negative_identity_prompt":  "不要改变五官、年龄、发型、痣的位置",
		"lock_level":                "strict",
		"approved_version":          2,
		"review_status":             NovelVideoReviewStatusApproved,
	}, ownerCookies)
	if patchResp.Code != http.StatusOK {
		t.Fatalf("expected actor patch 200, got %d: %s", patchResp.Code, patchResp.Body.String())
	}
	if !strings.Contains(patchResp.Body.String(), `"lock_level":"strict"`) || !strings.Contains(patchResp.Body.String(), `"approved_version":2`) {
		t.Fatalf("expected actor lock metadata, got %s", patchResp.Body.String())
	}

	otherUserResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/novel-video-projects/"+itoa(created.ID)+"/actors/"+itoa(actorID), map[string]any{
		"lock_level": "strict",
	}, otherCookies)
	if otherUserResp.Code != http.StatusNotFound {
		t.Fatalf("expected other user actor patch 404, got %d: %s", otherUserResp.Code, otherUserResp.Body.String())
	}

	var actorAsset NovelVideoAsset
	if err := db.Where("project_id = ? AND kind = ? AND user_id = ?", created.ID, "actor_ref", owner.ID).First(&actorAsset).Error; err != nil {
		t.Fatalf("expected actor_ref asset: %v", err)
	}
}

func TestNovelVideoShotStructuredScriptAssetRefsAndVideoPromptPriority(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "novel_structured_shot", "test-password")
	project := NovelVideoProject{UserID: user.ID, Title: "结构化短片", SourceText: "雨夜街区出现线索。", AspectRatio: "16:9", Duration: "10", VideoModel: "sora-2", Status: NovelVideoProjectStatusPlanned}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	asset := NovelVideoAsset{ProjectID: project.ID, UserID: user.ID, Kind: NovelVideoAssetKindClue, Name: "蓝色车票", ReviewStatus: NovelVideoReviewStatusApproved}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatalf("create asset: %v", err)
	}
	creature := NovelVideoCreature{ProjectID: project.ID, UserID: user.ID, Name: "灰鳞门兽", Appearance: "岩片背脊", VisualConsistencyPrompt: "同一只灰鳞门兽", ReviewStatus: NovelVideoReviewStatusApproved}
	if err := db.Create(&creature).Error; err != nil {
		t.Fatalf("create creature: %v", err)
	}
	episode := NovelVideoEpisode{ProjectID: project.ID, UserID: user.ID, Number: 1, Title: "第一集", Status: NovelVideoReviewStatusApproved}
	if err := db.Create(&episode).Error; err != nil {
		t.Fatalf("create episode: %v", err)
	}
	shot := NovelVideoShot{ProjectID: project.ID, EpisodeID: episode.ID, UserID: user.ID, Number: 1, Title: "车票特写", Prompt: "旧提示词", Status: NovelVideoReviewStatusNeedsReview}
	if err := db.Create(&shot).Error; err != nil {
		t.Fatalf("create shot: %v", err)
	}

	patchResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/novel-video-projects/"+itoa(project.ID)+"/shots/"+itoa(shot.ID), map[string]any{
		"script_unit_type": "action",
		"source_excerpt":   "她在雨水里捡起蓝色车票。",
		"duration_seconds": 7,
		"image_prompt":     "雨夜街区，蓝色车票特写，冷色灯光",
		"video_prompt":     "镜头从水面推近蓝色车票，灰鳞门兽倒影一闪而过",
		"voiceover_text":   "她终于看清车票背面的名字。",
		"asset_refs": []map[string]any{
			{"type": "asset", "id": asset.ID},
			{"type": "creature", "id": creature.ID},
		},
		"creature_ids":     []uint{creature.ID},
		"creature_ids_set": true,
		"status":           NovelVideoReviewStatusApproved,
	}, cookies)
	if patchResp.Code != http.StatusOK {
		t.Fatalf("expected structured shot patch 200, got %d: %s", patchResp.Code, patchResp.Body.String())
	}
	var payload struct {
		Prompt          string                       `json:"prompt"`
		ScriptUnitType  string                       `json:"script_unit_type"`
		SourceExcerpt   string                       `json:"source_excerpt"`
		DurationSeconds int                          `json:"duration_seconds"`
		ImagePrompt     string                       `json:"image_prompt"`
		VideoPrompt     string                       `json:"video_prompt"`
		VoiceoverText   string                       `json:"voiceover_text"`
		AssetRefs       []map[string]any             `json:"asset_refs"`
		Generation      novelVideoGenerationSettings `json:"generation_settings"`
	}
	if err := json.Unmarshal(patchResp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode structured shot response: %v", err)
	}
	if payload.Prompt != "旧提示词" || payload.VideoPrompt == "" || payload.DurationSeconds != 7 || len(payload.AssetRefs) != 2 {
		t.Fatalf("unexpected structured shot payload: %+v", payload)
	}
	var saved NovelVideoShot
	if err := db.First(&saved, shot.ID).Error; err != nil {
		t.Fatalf("load saved shot: %v", err)
	}
	req, err := testApp.buildNovelVideoShotRequest(project, saved)
	if err != nil {
		t.Fatalf("build shot request: %v", err)
	}
	if req.Prompt != "镜头从水面推近蓝色车票，灰鳞门兽倒影一闪而过" {
		t.Fatalf("expected video_prompt to take render priority, got %q", req.Prompt)
	}

	otherAsset := NovelVideoAsset{ProjectID: project.ID + 999, UserID: user.ID, Kind: NovelVideoAssetKindProp, Name: "越权道具"}
	if err := db.Create(&otherAsset).Error; err != nil {
		t.Fatalf("create other asset: %v", err)
	}
	badResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/novel-video-projects/"+itoa(project.ID)+"/shots/"+itoa(shot.ID), map[string]any{
		"video_prompt": "仍然有效的提示词",
		"asset_refs":   []map[string]any{{"type": "asset", "id": otherAsset.ID}},
	}, cookies)
	if badResp.Code != http.StatusBadRequest {
		t.Fatalf("expected cross-project asset ref 400, got %d: %s", badResp.Code, badResp.Body.String())
	}
}

func TestNovelVideoGenerateGridsAndCostEstimate(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "novel_grid_cost", "test-password")
	setUserCredits(t, testApp, user.ID, 100)
	project := NovelVideoProject{UserID: user.ID, Title: "宫格成本", SourceText: "正文", AspectRatio: "16:9", Duration: "10", VideoModel: "sora-2", GenerationMode: "grid", GridSize: 4, Status: NovelVideoProjectStatusPlanned}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	episode := NovelVideoEpisode{ProjectID: project.ID, UserID: user.ID, Number: 1, Title: "第一集", Status: NovelVideoReviewStatusApproved}
	if err := db.Create(&episode).Error; err != nil {
		t.Fatalf("create episode: %v", err)
	}
	for i := 1; i <= 5; i++ {
		shot := NovelVideoShot{ProjectID: project.ID, EpisodeID: episode.ID, UserID: user.ID, Number: i, Title: "镜头" + itoa(uint(i)), Prompt: "视频提示词", VideoPrompt: "视频提示词", DurationSeconds: 6, Status: NovelVideoReviewStatusApproved}
		if err := db.Create(&shot).Error; err != nil {
			t.Fatalf("create shot %d: %v", i, err)
		}
	}

	gridResp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects/"+itoa(project.ID)+"/grids/generate", map[string]any{}, cookies)
	if gridResp.Code != http.StatusOK {
		t.Fatalf("expected grid generate 200, got %d: %s", gridResp.Code, gridResp.Body.String())
	}
	var gridPayload struct {
		Items []struct {
			GridType string `json:"grid_type"`
			GridSize int    `json:"grid_size"`
			ShotIDs  []uint `json:"shot_ids"`
		} `json:"items"`
	}
	if err := json.Unmarshal(gridResp.Body.Bytes(), &gridPayload); err != nil {
		t.Fatalf("decode grid payload: %v", err)
	}
	if len(gridPayload.Items) != 2 || gridPayload.Items[0].GridType != "grid_4" || len(gridPayload.Items[0].ShotIDs) != 4 || len(gridPayload.Items[1].ShotIDs) != 1 {
		t.Fatalf("unexpected grid payload: %+v", gridPayload)
	}

	costResp := performJSONRequest(t, testApp, http.MethodGet, "/api/novel-video-projects/"+itoa(project.ID)+"/cost-estimate", nil, cookies)
	if costResp.Code != http.StatusOK {
		t.Fatalf("expected cost estimate 200, got %d: %s", costResp.Code, costResp.Body.String())
	}
	var cost struct {
		Project struct {
			TotalCredits int `json:"total_credits"`
		} `json:"project"`
		Episodes []struct {
			ShotCredits int `json:"shot_credits"`
			GridCredits int `json:"grid_credits"`
		} `json:"episodes"`
		Shots []struct {
			ShotID        uint `json:"shot_id"`
			RenderCredits int  `json:"render_credits"`
		} `json:"shots"`
	}
	if err := json.Unmarshal(costResp.Body.Bytes(), &cost); err != nil {
		t.Fatalf("decode cost estimate: %v", err)
	}
	if cost.Project.TotalCredits <= 0 || len(cost.Episodes) != 1 || cost.Episodes[0].GridCredits <= 0 || len(cost.Shots) != 5 {
		t.Fatalf("unexpected cost estimate: %+v", cost)
	}
}

func TestNovelVideoComposeSuccessCreatesPrivateWorkAndLinksComposition(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	store := &publicURLAssetStore{key: "novel/composed.mp4", mimeType: "video/mp4", publicURL: "https://oss.example.com/novel/composed.mp4"}
	testApp.assetStore = store
	testApp.novelVideoFFmpegRunner = successfulNovelVideoComposeRunner{
		result: novelVideoCompositionResult{
			OutputBytes:  []byte("composed-mp4"),
			SubtitleText: "1\n00:00:00,000 --> 00:00:05,000\n旁白\n\n",
			ManifestJSON: `{"schema_version":2}`,
		},
	}
	user, cookies := createLoggedInUser(t, testApp, "novel_compose_success", "test-password")
	project := NovelVideoProject{UserID: user.ID, Title: "合成成功", SourceText: "正文", AspectRatio: "16:9", Duration: "10", VideoModel: "sora-2", Status: NovelVideoProjectStatusSucceeded}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	episode := NovelVideoEpisode{ProjectID: project.ID, UserID: user.ID, Number: 1, Title: "第一集", Status: NovelVideoReviewStatusApproved}
	if err := db.Create(&episode).Error; err != nil {
		t.Fatalf("create episode: %v", err)
	}
	work := Work{UserID: user.ID, Category: WorkCategoryVideo, Status: GenerationStatusSucceeded, AssetKey: "shot-1.mp4", MIMEType: "video/mp4"}
	if err := db.Create(&work).Error; err != nil {
		t.Fatalf("create shot work: %v", err)
	}
	shot := NovelVideoShot{ProjectID: project.ID, EpisodeID: episode.ID, UserID: user.ID, Number: 1, Title: "镜头", Prompt: "提示词", VoiceoverText: "旁白", DurationSeconds: 5, Status: GenerationStatusSucceeded, WorkID: &work.ID}
	if err := db.Create(&shot).Error; err != nil {
		t.Fatalf("create shot: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects/"+itoa(project.ID)+"/compose", map[string]any{}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected compose 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Status    string `json:"status"`
		WorkID    *uint  `json:"work_id"`
		OutputURL string `json:"output_url"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode compose payload: %v", err)
	}
	if payload.Status != GenerationStatusSucceeded || payload.WorkID == nil || payload.OutputURL != store.publicURL {
		t.Fatalf("unexpected compose payload: %+v", payload)
	}
	var composed Work
	if err := db.First(&composed, *payload.WorkID).Error; err != nil {
		t.Fatalf("load composed work: %v", err)
	}
	if composed.Category != WorkCategoryVideo || composed.Visibility != WorkVisibilityPrivate || composed.AssetKey != store.key {
		t.Fatalf("unexpected composed work: %+v", composed)
	}
}

type successfulNovelVideoComposeRunner struct {
	result novelVideoCompositionResult
}

func (r successfulNovelVideoComposeRunner) ComposeNovelVideo(_ context.Context, _ NovelVideoProject, _ []novelVideoComposeClip, _ AssetStore) (novelVideoCompositionResult, error) {
	return r.result, nil
}

func TestNovelVideoAssetGeneratePatchAndIsolation(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	owner, ownerCookies := createLoggedInUser(t, testApp, "novel_asset_owner", "test-password")
	_, otherCookies := createLoggedInUser(t, testApp, "novel_asset_other", "test-password")
	project := NovelVideoProject{
		UserID:         owner.ID,
		Title:          "灰塔兽群",
		SourceText:     "灰塔里有门兽、石门和潮湿走廊。",
		StylePreset:    "冷色写实",
		AspectRatio:    "16:9",
		Duration:       "10",
		VideoModel:     "sora-2",
		Status:         NovelVideoProjectStatusAnalyzed,
		StoryBibleJSON: `{"logline":"守塔人穿过兽群抵达塔顶","world":"潮湿石塔","visual_style":"冷色写实"}`,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}

	generateResp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects/"+itoa(project.ID)+"/assets/generate", map[string]any{
		"kinds": []string{"character", "scene", "prop", "clue"},
	}, ownerCookies)
	if generateResp.Code != http.StatusOK {
		t.Fatalf("expected generate assets 200, got %d: %s", generateResp.Code, generateResp.Body.String())
	}
	var generated struct {
		Items []NovelVideoAsset `json:"items"`
	}
	if err := json.Unmarshal(generateResp.Body.Bytes(), &generated); err != nil {
		t.Fatalf("decode generated assets: %v", err)
	}
	if len(generated.Items) < 4 {
		t.Fatalf("expected at least four generated assets, got %+v", generated.Items)
	}
	if generated.Items[0].ProjectID != project.ID || generated.Items[0].UserID != owner.ID || generated.Items[0].Version != 1 {
		t.Fatalf("unexpected generated asset: %+v", generated.Items[0])
	}

	patchResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/novel-video-projects/"+itoa(project.ID)+"/assets/"+itoa(generated.Items[0].ID), map[string]any{
		"name":          "灰塔门兽",
		"review_status": NovelVideoReviewStatusApproved,
		"prompt":        "同一只灰塔门兽，岩片背脊，蓝白眼焰。",
	}, ownerCookies)
	if patchResp.Code != http.StatusOK {
		t.Fatalf("expected patch asset 200, got %d: %s", patchResp.Code, patchResp.Body.String())
	}
	if !strings.Contains(patchResp.Body.String(), `"name":"灰塔门兽"`) || !strings.Contains(patchResp.Body.String(), `"review_status":"approved"`) {
		t.Fatalf("unexpected patch asset body: %s", patchResp.Body.String())
	}

	otherResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/novel-video-projects/"+itoa(project.ID)+"/assets/"+itoa(generated.Items[0].ID), map[string]any{
		"name": "越权",
	}, otherCookies)
	if otherResp.Code != http.StatusNotFound {
		t.Fatalf("expected other user 404, got %d: %s", otherResp.Code, otherResp.Body.String())
	}
}

func TestNovelVideoAssetGenerateQueuesAndCompletesImageJobs(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString([]byte("novel-asset-image")),
			MIMEType:          "image/png",
			ProviderRequestID: "asset-success-1",
		},
	}
	testApp, db := newTestApp(t, provider)
	testApp.assetStore = &publicURLAssetStore{key: "novel/assets/prop.png", mimeType: "image/png", publicURL: "https://oss.example.test/novel/assets/prop.png"}
	owner, ownerCookies := createLoggedInUser(t, testApp, "novel_asset_job_owner", "test-password")
	setUserCredits(t, testApp, owner.ID, 20)
	project := NovelVideoProject{
		UserID:      owner.ID,
		Title:       "雾塔道具",
		SourceText:  "雾塔里有一枚会发光的钥匙。",
		StylePreset: "冷色写实",
		AspectRatio: "16:9",
		Duration:    "10",
		Status:      NovelVideoProjectStatusAnalyzed,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}

	generateResp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects/"+itoa(project.ID)+"/assets/generate", map[string]any{
		"kinds": []string{"prop"},
	}, ownerCookies)
	if generateResp.Code != http.StatusOK {
		t.Fatalf("expected generate assets 200, got %d: %s", generateResp.Code, generateResp.Body.String())
	}
	var generated struct {
		Items []NovelVideoAsset `json:"items"`
		Jobs  []NovelVideoJob   `json:"jobs"`
	}
	if err := json.Unmarshal(generateResp.Body.Bytes(), &generated); err != nil {
		t.Fatalf("decode generated assets: %v", err)
	}
	if len(generated.Items) != 1 || len(generated.Jobs) != 1 {
		t.Fatalf("expected one asset and one job, got %+v", generated)
	}
	if generated.Jobs[0].Status != GenerationStatusQueued || generated.Jobs[0].AssetID == nil || *generated.Jobs[0].AssetID != generated.Items[0].ID {
		t.Fatalf("unexpected queued job: %+v", generated.Jobs[0])
	}

	waitForCondition(t, 3*time.Second, func() bool {
		var asset NovelVideoAsset
		if err := db.First(&asset, generated.Items[0].ID).Error; err != nil {
			return false
		}
		var job NovelVideoJob
		if err := db.First(&job, generated.Jobs[0].ID).Error; err != nil {
			return false
		}
		return asset.AssetURL == "https://oss.example.test/novel/assets/prop.png" &&
			asset.WorkID != nil &&
			asset.GenerationRecordID != nil &&
			job.Status == GenerationStatusSucceeded &&
			job.Progress == 100 &&
			job.ErrorMessage == ""
	})
	if provider.calls != 1 {
		t.Fatalf("expected one provider call, got %d", provider.calls)
	}
}

func TestNovelVideoAssetGenerateReusesExistingDrafts(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	owner, ownerCookies := createLoggedInUser(t, testApp, "novel_asset_idempotent_owner", "test-password")
	project := NovelVideoProject{
		UserID:      owner.ID,
		Title:       "雾塔草案",
		SourceText:  "雾塔里有一枚发光钥匙。",
		StylePreset: "冷色写实",
		AspectRatio: "16:9",
		Duration:    "10",
		Status:      NovelVideoProjectStatusAnalyzed,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}

	firstResp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects/"+itoa(project.ID)+"/assets/generate", map[string]any{
		"kinds": []string{"prop"},
	}, ownerCookies)
	if firstResp.Code != http.StatusOK {
		t.Fatalf("expected first generate assets 200, got %d: %s", firstResp.Code, firstResp.Body.String())
	}
	var first struct {
		Items []NovelVideoAsset `json:"items"`
		Jobs  []NovelVideoJob   `json:"jobs"`
	}
	if err := json.Unmarshal(firstResp.Body.Bytes(), &first); err != nil {
		t.Fatalf("decode first generated assets: %v", err)
	}
	if len(first.Items) != 1 || len(first.Jobs) != 1 {
		t.Fatalf("expected one first asset and job, got %+v", first)
	}

	secondResp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects/"+itoa(project.ID)+"/assets/generate", map[string]any{
		"kinds": []string{"prop"},
	}, ownerCookies)
	if secondResp.Code != http.StatusOK {
		t.Fatalf("expected second generate assets 200, got %d: %s", secondResp.Code, secondResp.Body.String())
	}
	var second struct {
		Items []NovelVideoAsset `json:"items"`
		Jobs  []NovelVideoJob   `json:"jobs"`
	}
	if err := json.Unmarshal(secondResp.Body.Bytes(), &second); err != nil {
		t.Fatalf("decode second generated assets: %v", err)
	}
	if len(second.Items) != 1 || second.Items[0].ID != first.Items[0].ID {
		t.Fatalf("expected second call to reuse first asset, got %+v", second)
	}
	var assetCount int64
	if err := db.Model(&NovelVideoAsset{}).Where("project_id = ?", project.ID).Count(&assetCount).Error; err != nil {
		t.Fatalf("count assets: %v", err)
	}
	var jobCount int64
	if err := db.Model(&NovelVideoJob{}).Where("project_id = ? AND job_type = ?", project.ID, NovelVideoJobTypeAssetImage).Count(&jobCount).Error; err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if assetCount != 1 || jobCount != 1 {
		t.Fatalf("expected idempotent generate to keep one asset and one job, got assets=%d jobs=%d", assetCount, jobCount)
	}
}

func TestNovelVideoAssetGenerateReusesActiveAssetImageJob(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	owner, ownerCookies := createLoggedInUser(t, testApp, "novel_asset_active_job_owner", "test-password")
	project := NovelVideoProject{
		UserID:      owner.ID,
		Title:       "雾塔任务",
		SourceText:  "雾塔里有一枚发光钥匙。",
		StylePreset: "冷色写实",
		AspectRatio: "16:9",
		Duration:    "10",
		Status:      NovelVideoProjectStatusAnalyzed,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	draft := fallbackNovelVideoAssets(project, []string{NovelVideoAssetKindProp})[0]
	if err := db.Create(&draft).Error; err != nil {
		t.Fatalf("create asset draft: %v", err)
	}
	job := NovelVideoJob{
		ProjectID:   project.ID,
		UserID:      owner.ID,
		JobType:     NovelVideoJobTypeAssetImage,
		Status:      GenerationStatusQueued,
		AssetID:     &draft.ID,
		MaxAttempts: 3,
		PayloadJSON: encodeJSON(map[string]any{"asset_id": draft.ID, "kind": draft.Kind, "prompt": draft.Prompt}),
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatalf("create active job: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects/"+itoa(project.ID)+"/assets/generate", map[string]any{
		"kinds": []string{"prop"},
	}, ownerCookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected generate assets 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Items []NovelVideoAsset `json:"items"`
		Jobs  []NovelVideoJob   `json:"jobs"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode generated assets: %v", err)
	}
	if len(payload.Items) != 1 || payload.Items[0].ID != draft.ID || len(payload.Jobs) != 1 || payload.Jobs[0].ID != job.ID {
		t.Fatalf("expected existing asset and active job to be returned, got %+v", payload)
	}
	var assetCount int64
	if err := db.Model(&NovelVideoAsset{}).Where("project_id = ?", project.ID).Count(&assetCount).Error; err != nil {
		t.Fatalf("count assets: %v", err)
	}
	var jobCount int64
	if err := db.Model(&NovelVideoJob{}).Where("project_id = ? AND job_type = ?", project.ID, NovelVideoJobTypeAssetImage).Count(&jobCount).Error; err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if assetCount != 1 || jobCount != 1 {
		t.Fatalf("expected no duplicate asset or job, got assets=%d jobs=%d", assetCount, jobCount)
	}
}

func TestNovelVideoAssetGenerateSkipsCharacterWhenImageSeriesHasActorRefs(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	owner, ownerCookies := createLoggedInUser(t, testApp, "novel_asset_image_series_actor_owner", "test-password")
	project := NovelVideoProject{
		UserID:         owner.ID,
		Title:          "Image Series",
		SourceText:     "source",
		ContentMode:    NovelVideoContentModeShortFilmImage,
		GenerationMode: NovelVideoGenerationModeImageSeries,
		StylePreset:    "cinematic",
		AspectRatio:    "16:9",
		Duration:       "10",
		Status:         NovelVideoProjectStatusPlanned,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	actorRef := NovelVideoAsset{
		ProjectID:    project.ID,
		UserID:       owner.ID,
		Kind:         NovelVideoAssetKindActorRef,
		Name:         "Lead actor reference",
		Prompt:       "consistent lead actor",
		Version:      1,
		ReviewStatus: NovelVideoReviewStatusApproved,
		MetadataJSON: encodeJSON(map[string]any{"actor_id": uint(21), "source": "image_plan"}),
	}
	if err := db.Create(&actorRef).Error; err != nil {
		t.Fatalf("create actor ref: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects/"+itoa(project.ID)+"/assets/generate", map[string]any{
		"kinds": []string{"character", "scene", "prop", "clue", "style"},
	}, ownerCookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected generate assets 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Items []NovelVideoAsset `json:"items"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode generated assets: %v", err)
	}
	for _, asset := range payload.Items {
		if asset.Kind == NovelVideoAssetKindCharacter {
			t.Fatalf("expected image-series actor_ref to replace character draft, got %+v", payload.Items)
		}
	}
	var characterCount int64
	if err := db.Model(&NovelVideoAsset{}).Where("project_id = ? AND kind = ?", project.ID, NovelVideoAssetKindCharacter).Count(&characterCount).Error; err != nil {
		t.Fatalf("count character assets: %v", err)
	}
	if characterCount != 0 {
		t.Fatalf("expected no character assets, got %d", characterCount)
	}
}

func TestNovelVideoAssetGenerateReusesSemanticDraftAcrossSources(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	owner, ownerCookies := createLoggedInUser(t, testApp, "novel_asset_source_reuse_owner", "test-password")
	project := NovelVideoProject{
		UserID:      owner.ID,
		Title:       "Semantic Draft",
		SourceText:  "source",
		StylePreset: "cinematic",
		AspectRatio: "16:9",
		Duration:    "10",
		Status:      NovelVideoProjectStatusAnalyzed,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	existing := fallbackNovelVideoAssets(project, []string{NovelVideoAssetKindScene})[0]
	existing.MetadataJSON = encodeJSON(map[string]any{"source": "image_plan"})
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("create existing scene draft: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects/"+itoa(project.ID)+"/assets/generate", map[string]any{
		"kinds": []string{"scene"},
	}, ownerCookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected generate assets 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Items []NovelVideoAsset `json:"items"`
		Jobs  []NovelVideoJob   `json:"jobs"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode generated assets: %v", err)
	}
	if len(payload.Items) != 1 || payload.Items[0].ID != existing.ID {
		t.Fatalf("expected existing semantic draft to be reused, got %+v", payload)
	}
	var assetCount int64
	if err := db.Model(&NovelVideoAsset{}).Where("project_id = ?", project.ID).Count(&assetCount).Error; err != nil {
		t.Fatalf("count assets: %v", err)
	}
	var jobCount int64
	if err := db.Model(&NovelVideoJob{}).Where("project_id = ? AND job_type = ?", project.ID, NovelVideoJobTypeAssetImage).Count(&jobCount).Error; err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if assetCount != 1 || jobCount != 0 {
		t.Fatalf("expected source-agnostic reuse to avoid new asset/job, got assets=%d jobs=%d", assetCount, jobCount)
	}
}

func TestNovelVideoAssetGeneratePersistsJobFailure(t *testing.T) {
	provider := &stubProvider{err: &ProviderError{Code: "provider_down", Message: "provider unavailable"}}
	testApp, db := newTestApp(t, provider)
	owner, ownerCookies := createLoggedInUser(t, testApp, "novel_asset_job_fail_owner", "test-password")
	setUserCredits(t, testApp, owner.ID, 20)
	project := NovelVideoProject{
		UserID:      owner.ID,
		Title:       "雾塔失败",
		SourceText:  "雾塔里有一枚会发光的钥匙。",
		StylePreset: "冷色写实",
		AspectRatio: "16:9",
		Duration:    "10",
		Status:      NovelVideoProjectStatusAnalyzed,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}

	generateResp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects/"+itoa(project.ID)+"/assets/generate", map[string]any{
		"kinds": []string{"prop"},
	}, ownerCookies)
	if generateResp.Code != http.StatusOK {
		t.Fatalf("expected generate assets 200, got %d: %s", generateResp.Code, generateResp.Body.String())
	}
	var generated struct {
		Items []NovelVideoAsset `json:"items"`
		Jobs  []NovelVideoJob   `json:"jobs"`
	}
	if err := json.Unmarshal(generateResp.Body.Bytes(), &generated); err != nil {
		t.Fatalf("decode generated assets: %v", err)
	}
	if len(generated.Items) != 1 || len(generated.Jobs) != 1 {
		t.Fatalf("expected one asset and one job, got %+v", generated)
	}

	waitForCondition(t, 3*time.Second, func() bool {
		var asset NovelVideoAsset
		if err := db.First(&asset, generated.Items[0].ID).Error; err != nil {
			return false
		}
		var job NovelVideoJob
		if err := db.First(&job, generated.Jobs[0].ID).Error; err != nil {
			return false
		}
		return asset.AssetURL == "" &&
			asset.WorkID == nil &&
			asset.GenerationRecordID == nil &&
			asset.ErrorCode == "asset_image_generation_failed" &&
			strings.Contains(asset.ErrorMessage, "provider unavailable") &&
			job.Status == GenerationStatusFailed &&
			job.Progress == 0 &&
			strings.Contains(job.ErrorMessage, "provider unavailable")
	})
}

func TestNovelVideoAssetDedupeRemovesOnlySafeDuplicates(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	owner, ownerCookies := createLoggedInUser(t, testApp, "novel_asset_dedupe_owner", "test-password")
	project := NovelVideoProject{
		UserID:      owner.ID,
		Title:       "雾塔清理",
		SourceText:  "雾塔里有一枚发光钥匙。",
		StylePreset: "冷色写实",
		AspectRatio: "16:9",
		Duration:    "10",
		Status:      NovelVideoProjectStatusAnalyzed,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	base := fallbackNovelVideoAssets(project, []string{NovelVideoAssetKindProp})[0]
	approved := base
	approved.ReviewStatus = NovelVideoReviewStatusApproved
	approved.MetadataJSON = encodeJSON(map[string]any{"source": "fallback"})
	if err := db.Create(&approved).Error; err != nil {
		t.Fatalf("create approved asset: %v", err)
	}
	safeDuplicate := base
	safeDuplicate.ReviewStatus = NovelVideoReviewStatusNeedsReview
	safeDuplicate.MetadataJSON = encodeJSON(map[string]any{"source": "image_plan"})
	if err := db.Create(&safeDuplicate).Error; err != nil {
		t.Fatalf("create safe duplicate: %v", err)
	}
	referencedDuplicate := base
	referencedDuplicate.ReviewStatus = NovelVideoReviewStatusNeedsReview
	referencedDuplicate.MetadataJSON = encodeJSON(map[string]any{"source": "manual"})
	if err := db.Create(&referencedDuplicate).Error; err != nil {
		t.Fatalf("create referenced duplicate: %v", err)
	}
	activeDuplicate := base
	activeDuplicate.ReviewStatus = NovelVideoReviewStatusNeedsReview
	activeDuplicate.MetadataJSON = encodeJSON(map[string]any{"source": "retry"})
	if err := db.Create(&activeDuplicate).Error; err != nil {
		t.Fatalf("create active duplicate: %v", err)
	}
	episode := NovelVideoEpisode{ProjectID: project.ID, UserID: owner.ID, Number: 1, Title: "第一集", Status: NovelVideoReviewStatusNeedsReview}
	if err := db.Create(&episode).Error; err != nil {
		t.Fatalf("create episode: %v", err)
	}
	shot := NovelVideoShot{
		ProjectID:     project.ID,
		EpisodeID:     episode.ID,
		UserID:        owner.ID,
		Number:        1,
		Title:         "钥匙特写",
		Prompt:        "钥匙出现",
		AssetRefsJSON: encodeNovelVideoAssetRefs([]novelVideoAssetRef{{Type: "asset", ID: referencedDuplicate.ID}}),
		Status:        NovelVideoReviewStatusNeedsReview,
	}
	if err := db.Create(&shot).Error; err != nil {
		t.Fatalf("create shot: %v", err)
	}
	activeJob := NovelVideoJob{
		ProjectID: project.ID,
		UserID:    owner.ID,
		JobType:   NovelVideoJobTypeAssetImage,
		Status:    GenerationStatusRunning,
		AssetID:   &activeDuplicate.ID,
	}
	if err := db.Create(&activeJob).Error; err != nil {
		t.Fatalf("create active job: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects/"+itoa(project.ID)+"/assets/dedupe", map[string]any{}, ownerCookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected dedupe 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Removed       int    `json:"removed"`
		CollapsedIDs  []uint `json:"collapsed_ids"`
		SkippedActive int    `json:"skipped_active"`
		SkippedRefs   int    `json:"skipped_referenced"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode dedupe payload: %v", err)
	}
	if payload.Removed != 1 || payload.SkippedActive != 1 || payload.SkippedRefs != 1 {
		t.Fatalf("unexpected dedupe payload: %+v", payload)
	}
	for _, collapsedID := range []uint{referencedDuplicate.ID, activeDuplicate.ID} {
		if !containsUint(payload.CollapsedIDs, collapsedID) {
			t.Fatalf("expected collapsed_ids to include unsafe duplicate %d, got %+v", collapsedID, payload.CollapsedIDs)
		}
	}
	var removed NovelVideoAsset
	if err := db.Unscoped().First(&removed, safeDuplicate.ID).Error; err != nil {
		t.Fatalf("load removed duplicate: %v", err)
	}
	if !removed.DeletedAt.Valid {
		t.Fatalf("expected safe duplicate to be soft deleted")
	}
	for _, keptID := range []uint{approved.ID, referencedDuplicate.ID, activeDuplicate.ID} {
		var kept NovelVideoAsset
		if err := db.First(&kept, keptID).Error; err != nil {
			t.Fatalf("expected asset %d to remain: %v", keptID, err)
		}
	}
}

func TestNovelVideoAssetDeleteSoftDeletesSafeAssetAndRefreshesLists(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	owner, ownerCookies := createLoggedInUser(t, testApp, "novel_asset_delete_owner", "test-password")
	project := NovelVideoProject{
		UserID:      owner.ID,
		Title:       "Asset delete",
		SourceText:  "source",
		StylePreset: "cinematic",
		AspectRatio: "16:9",
		Duration:    "10",
		Status:      NovelVideoProjectStatusAnalyzed,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	asset := NovelVideoAsset{
		ProjectID:    project.ID,
		UserID:       owner.ID,
		Kind:         NovelVideoAssetKindProp,
		Name:         "Safe prop",
		Prompt:       "safe prop prompt",
		Version:      1,
		ReviewStatus: NovelVideoReviewStatusApproved,
	}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatalf("create asset: %v", err)
	}
	finishedJob := NovelVideoJob{ProjectID: project.ID, UserID: owner.ID, JobType: NovelVideoJobTypeAssetImage, Status: GenerationStatusSucceeded, AssetID: &asset.ID}
	if err := db.Create(&finishedJob).Error; err != nil {
		t.Fatalf("create finished job: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodDelete, "/api/novel-video-projects/"+itoa(project.ID)+"/assets/"+itoa(asset.ID), map[string]any{}, ownerCookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected delete asset 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		DeletedID uint              `json:"deleted_id"`
		Items     []NovelVideoAsset `json:"items"`
		Jobs      []NovelVideoJob   `json:"jobs"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode delete payload: %v", err)
	}
	if payload.DeletedID != asset.ID || len(payload.Items) != 0 || len(payload.Jobs) != 0 {
		t.Fatalf("unexpected delete payload: %+v", payload)
	}
	var deleted NovelVideoAsset
	if err := db.Unscoped().First(&deleted, asset.ID).Error; err != nil {
		t.Fatalf("load deleted asset: %v", err)
	}
	if !deleted.DeletedAt.Valid {
		t.Fatalf("expected asset to be soft deleted")
	}
	detailResp := performJSONRequest(t, testApp, http.MethodGet, "/api/novel-video-projects/"+itoa(project.ID), nil, ownerCookies)
	if detailResp.Code != http.StatusOK {
		t.Fatalf("expected detail 200, got %d: %s", detailResp.Code, detailResp.Body.String())
	}
	var detail struct {
		Assets []NovelVideoAsset `json:"assets"`
	}
	if err := json.Unmarshal(detailResp.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode detail payload: %v", err)
	}
	if len(detail.Assets) != 0 {
		t.Fatalf("expected deleted asset to be absent from detail assets: %+v", detail.Assets)
	}
}

func TestNovelVideoAssetDeleteCancelsQueuedJobAndSoftDeletesAsset(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	owner, ownerCookies := createLoggedInUser(t, testApp, "novel_asset_delete_queued_owner", "test-password")
	project := NovelVideoProject{
		UserID:      owner.ID,
		Title:       "Queued asset delete",
		SourceText:  "source",
		StylePreset: "cinematic",
		AspectRatio: "16:9",
		Duration:    "10",
		Status:      NovelVideoProjectStatusAnalyzed,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	asset := NovelVideoAsset{
		ProjectID:    project.ID,
		UserID:       owner.ID,
		Kind:         NovelVideoAssetKindProp,
		Name:         "Queued prop",
		Prompt:       "queued prop prompt",
		Version:      1,
		ReviewStatus: NovelVideoReviewStatusNeedsReview,
	}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatalf("create asset: %v", err)
	}
	queuedJob := NovelVideoJob{ProjectID: project.ID, UserID: owner.ID, JobType: NovelVideoJobTypeAssetImage, Status: GenerationStatusQueued, AssetID: &asset.ID}
	if err := db.Create(&queuedJob).Error; err != nil {
		t.Fatalf("create queued job: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodDelete, "/api/novel-video-projects/"+itoa(project.ID)+"/assets/"+itoa(asset.ID), map[string]any{}, ownerCookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected queued asset delete 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		DeletedID uint              `json:"deleted_id"`
		Items     []NovelVideoAsset `json:"items"`
		Jobs      []NovelVideoJob   `json:"jobs"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode delete payload: %v", err)
	}
	if payload.DeletedID != asset.ID || len(payload.Items) != 0 || len(payload.Jobs) != 0 {
		t.Fatalf("expected deleted asset and no active jobs in response, got %+v", payload)
	}
	var deleted NovelVideoAsset
	if err := db.Unscoped().First(&deleted, asset.ID).Error; err != nil {
		t.Fatalf("load deleted asset: %v", err)
	}
	if !deleted.DeletedAt.Valid {
		t.Fatalf("expected queued asset to be soft deleted")
	}
	var cancelled NovelVideoJob
	if err := db.First(&cancelled, queuedJob.ID).Error; err != nil {
		t.Fatalf("load cancelled job: %v", err)
	}
	if cancelled.Status != GenerationStatusFailed || cancelled.ErrorCode != "user_cancelled" || cancelled.ErrorMessage != "用户取消排队并删除资产" || cancelled.FinishedAt == nil {
		t.Fatalf("expected queued job to be marked user_cancelled, got %+v", cancelled)
	}
	detailResp := performJSONRequest(t, testApp, http.MethodGet, "/api/novel-video-projects/"+itoa(project.ID), nil, ownerCookies)
	if detailResp.Code != http.StatusOK {
		t.Fatalf("expected detail 200, got %d: %s", detailResp.Code, detailResp.Body.String())
	}
	var detail struct {
		Assets []NovelVideoAsset `json:"assets"`
	}
	if err := json.Unmarshal(detailResp.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode detail payload: %v", err)
	}
	if len(detail.Assets) != 0 {
		t.Fatalf("expected deleted queued asset to be absent from detail assets: %+v", detail.Assets)
	}
}

func TestNovelVideoAssetDeleteRejectsActiveJobReferencedAssetAndOtherUser(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	owner, ownerCookies := createLoggedInUser(t, testApp, "novel_asset_delete_guard_owner", "test-password")
	_, otherCookies := createLoggedInUser(t, testApp, "novel_asset_delete_guard_other", "test-password")
	project := NovelVideoProject{
		UserID:      owner.ID,
		Title:       "Asset delete guards",
		SourceText:  "source",
		StylePreset: "cinematic",
		AspectRatio: "16:9",
		Duration:    "10",
		Status:      NovelVideoProjectStatusAnalyzed,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	activeAsset := NovelVideoAsset{ProjectID: project.ID, UserID: owner.ID, Kind: NovelVideoAssetKindProp, Name: "Active prop", Prompt: "active", Version: 1, ReviewStatus: NovelVideoReviewStatusNeedsReview}
	referencedAsset := NovelVideoAsset{ProjectID: project.ID, UserID: owner.ID, Kind: NovelVideoAssetKindScene, Name: "Referenced scene", Prompt: "scene", Version: 1, ReviewStatus: NovelVideoReviewStatusApproved}
	otherSafeAsset := NovelVideoAsset{ProjectID: project.ID, UserID: owner.ID, Kind: NovelVideoAssetKindClue, Name: "Other safe", Prompt: "safe", Version: 1, ReviewStatus: NovelVideoReviewStatusNeedsReview}
	if err := db.Create(&activeAsset).Error; err != nil {
		t.Fatalf("create active asset: %v", err)
	}
	if err := db.Create(&referencedAsset).Error; err != nil {
		t.Fatalf("create referenced asset: %v", err)
	}
	if err := db.Create(&otherSafeAsset).Error; err != nil {
		t.Fatalf("create other safe asset: %v", err)
	}
	activeJob := NovelVideoJob{ProjectID: project.ID, UserID: owner.ID, JobType: NovelVideoJobTypeAssetImage, Status: GenerationStatusRunning, AssetID: &activeAsset.ID}
	if err := db.Create(&activeJob).Error; err != nil {
		t.Fatalf("create active job: %v", err)
	}
	referencedQueuedJob := NovelVideoJob{ProjectID: project.ID, UserID: owner.ID, JobType: NovelVideoJobTypeAssetImage, Status: GenerationStatusQueued, AssetID: &referencedAsset.ID}
	if err := db.Create(&referencedQueuedJob).Error; err != nil {
		t.Fatalf("create referenced queued job: %v", err)
	}
	episode := NovelVideoEpisode{ProjectID: project.ID, UserID: owner.ID, Number: 1, Title: "Episode", Status: NovelVideoReviewStatusNeedsReview}
	if err := db.Create(&episode).Error; err != nil {
		t.Fatalf("create episode: %v", err)
	}
	shot := NovelVideoShot{
		ProjectID:     project.ID,
		EpisodeID:     episode.ID,
		UserID:        owner.ID,
		Number:        1,
		Title:         "Scene shot",
		Prompt:        "scene prompt",
		AssetRefsJSON: encodeNovelVideoAssetRefs([]novelVideoAssetRef{{Type: "asset", ID: referencedAsset.ID, Name: referencedAsset.Name}}),
		Status:        NovelVideoReviewStatusNeedsReview,
	}
	if err := db.Create(&shot).Error; err != nil {
		t.Fatalf("create shot: %v", err)
	}

	activeResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/novel-video-projects/"+itoa(project.ID)+"/assets/"+itoa(activeAsset.ID), map[string]any{}, ownerCookies)
	if activeResp.Code != http.StatusConflict || !strings.Contains(activeResp.Body.String(), "novel_video_asset_job_active") {
		t.Fatalf("expected active job 409, got %d: %s", activeResp.Code, activeResp.Body.String())
	}

	referencedResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/novel-video-projects/"+itoa(project.ID)+"/assets/"+itoa(referencedAsset.ID), map[string]any{}, ownerCookies)
	if referencedResp.Code != http.StatusConflict || !strings.Contains(referencedResp.Body.String(), "novel_video_asset_in_use") || !strings.Contains(referencedResp.Body.String(), `"shot_id":`+itoa(shot.ID)) {
		t.Fatalf("expected referenced asset 409 with shot summary, got %d: %s", referencedResp.Code, referencedResp.Body.String())
	}
	var stillQueued NovelVideoJob
	if err := db.First(&stillQueued, referencedQueuedJob.ID).Error; err != nil {
		t.Fatalf("load referenced queued job: %v", err)
	}
	if stillQueued.Status != GenerationStatusQueued || stillQueued.ErrorCode != "" || stillQueued.FinishedAt != nil {
		t.Fatalf("expected referenced queued job to remain untouched, got %+v", stillQueued)
	}

	otherResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/novel-video-projects/"+itoa(project.ID)+"/assets/"+itoa(otherSafeAsset.ID), map[string]any{}, otherCookies)
	if otherResp.Code != http.StatusNotFound {
		t.Fatalf("expected other user 404, got %d: %s", otherResp.Code, otherResp.Body.String())
	}
	for _, keptID := range []uint{activeAsset.ID, referencedAsset.ID, otherSafeAsset.ID} {
		var kept NovelVideoAsset
		if err := db.First(&kept, keptID).Error; err != nil {
			t.Fatalf("expected asset %d to remain: %v", keptID, err)
		}
	}
}

func TestNovelVideoAssetImageRunnerSkipsCancelledQueuedJob(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString([]byte("cancelled-asset-image")),
			MIMEType:          "image/png",
			ProviderRequestID: "cancelled-asset",
		},
	}
	testApp, db := newTestApp(t, provider)
	testApp.assetStore = &publicURLAssetStore{key: "novel/assets/cancelled.png", mimeType: "image/png", publicURL: "https://oss.example.test/novel/assets/cancelled.png"}
	owner, _ := createLoggedInUser(t, testApp, "novel_asset_cancelled_runner", "test-password")
	setUserCredits(t, testApp, owner.ID, 20)
	project := NovelVideoProject{
		UserID:      owner.ID,
		Title:       "Cancelled runner",
		SourceText:  "source",
		StylePreset: "cinematic",
		AspectRatio: "16:9",
		Duration:    "10",
		Status:      NovelVideoProjectStatusAnalyzed,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	asset := NovelVideoAsset{ProjectID: project.ID, UserID: owner.ID, Kind: NovelVideoAssetKindProp, Name: "Cancelled prop", Prompt: "cancelled prompt", Version: 1, ReviewStatus: NovelVideoReviewStatusNeedsReview}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatalf("create asset: %v", err)
	}
	finishedAt := time.Now()
	job := NovelVideoJob{
		ProjectID:    project.ID,
		UserID:       owner.ID,
		JobType:      NovelVideoJobTypeAssetImage,
		Status:       GenerationStatusFailed,
		AssetID:      &asset.ID,
		ErrorCode:    "user_cancelled",
		ErrorMessage: "用户取消排队并删除资产",
		FinishedAt:   &finishedAt,
		MaxAttempts:  3,
		PayloadJSON:  encodeJSON(map[string]any{"asset_id": asset.ID, "prompt": asset.Prompt}),
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatalf("create cancelled job: %v", err)
	}

	testApp.runNovelVideoAssetImageJob(project, job)

	if provider.calls != 0 {
		t.Fatalf("expected cancelled queued job runner to skip provider, got %d calls", provider.calls)
	}
	var refreshedJob NovelVideoJob
	if err := db.First(&refreshedJob, job.ID).Error; err != nil {
		t.Fatalf("reload cancelled job: %v", err)
	}
	if refreshedJob.Status != GenerationStatusFailed || refreshedJob.ErrorCode != "user_cancelled" || refreshedJob.FinishedAt == nil {
		t.Fatalf("expected cancelled job to remain failed/user_cancelled, got %+v", refreshedJob)
	}
	var refreshedAsset NovelVideoAsset
	if err := db.First(&refreshedAsset, asset.ID).Error; err != nil {
		t.Fatalf("reload asset: %v", err)
	}
	if refreshedAsset.AssetURL != "" || refreshedAsset.GenerationRecordID != nil {
		t.Fatalf("expected cancelled runner not to update asset, got %+v", refreshedAsset)
	}
}

func TestNovelVideoAssetGenerateRetriesExistingAsset(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString([]byte("novel-asset-retry")),
			MIMEType:          "image/png",
			ProviderRequestID: "asset-retry-1",
		},
	}
	testApp, db := newTestApp(t, provider)
	testApp.assetStore = &publicURLAssetStore{key: "novel/assets/retry.png", mimeType: "image/png", publicURL: "https://oss.example.test/novel/assets/retry.png"}
	owner, ownerCookies := createLoggedInUser(t, testApp, "novel_asset_retry_owner", "test-password")
	setUserCredits(t, testApp, owner.ID, 20)
	project := NovelVideoProject{
		UserID:      owner.ID,
		Title:       "雾塔重试",
		SourceText:  "雾塔里有一枚会发光的钥匙。",
		StylePreset: "冷色写实",
		AspectRatio: "16:9",
		Duration:    "10",
		Status:      NovelVideoProjectStatusAnalyzed,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	asset := NovelVideoAsset{
		ProjectID:    project.ID,
		UserID:       owner.ID,
		Kind:         NovelVideoAssetKindProp,
		Name:         "发光钥匙",
		Description:  "关键道具",
		Prompt:       "冷色写实发光钥匙",
		Version:      1,
		ReviewStatus: NovelVideoReviewStatusNeedsReview,
		ErrorCode:    "asset_image_generation_failed",
		ErrorMessage: "provider unavailable",
		MetadataJSON: encodeJSON(map[string]any{"source": "test"}),
	}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatalf("create asset: %v", err)
	}

	retryResp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects/"+itoa(project.ID)+"/assets/generate", map[string]any{
		"asset_id": asset.ID,
	}, ownerCookies)
	if retryResp.Code != http.StatusOK {
		t.Fatalf("expected retry asset 200, got %d: %s", retryResp.Code, retryResp.Body.String())
	}
	var retried struct {
		Items []NovelVideoAsset `json:"items"`
		Jobs  []NovelVideoJob   `json:"jobs"`
	}
	if err := json.Unmarshal(retryResp.Body.Bytes(), &retried); err != nil {
		t.Fatalf("decode retried asset: %v", err)
	}
	if len(retried.Items) != 1 || retried.Items[0].ID != asset.ID || len(retried.Jobs) != 1 {
		t.Fatalf("expected same asset and one retry job, got %+v", retried)
	}
	var assetCount int64
	if err := db.Model(&NovelVideoAsset{}).Where("project_id = ?", project.ID).Count(&assetCount).Error; err != nil {
		t.Fatalf("count assets: %v", err)
	}
	if assetCount != 1 {
		t.Fatalf("expected retry to reuse existing asset, got %d assets", assetCount)
	}

	waitForCondition(t, 3*time.Second, func() bool {
		var refreshed NovelVideoAsset
		if err := db.First(&refreshed, asset.ID).Error; err != nil {
			return false
		}
		return refreshed.AssetURL == "https://oss.example.test/novel/assets/retry.png" &&
			refreshed.ErrorMessage == "" &&
			refreshed.GenerationRecordID != nil
	})
}

func TestNovelVideoActorLockSheetReturnsAssetAndItem(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	owner, ownerCookies := createLoggedInUser(t, testApp, "novel_actor_lock_contract", "test-password")
	project := NovelVideoProject{
		UserID:      owner.ID,
		Title:       "演员契约",
		SourceText:  "两名演员在雨夜航站楼行动。",
		StylePreset: "写实短电影",
		AspectRatio: "16:9",
		Duration:    "10",
		Status:      NovelVideoProjectStatusAnalyzed,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	actor := NovelVideoCreature{ProjectID: project.ID, UserID: owner.ID, Name: "林岚", Appearance: "短发女演员", VisualConsistencyPrompt: "短发女演员，左眼下小痣", ReviewStatus: NovelVideoReviewStatusApproved}
	if err := db.Create(&actor).Error; err != nil {
		t.Fatalf("create actor: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects/"+itoa(project.ID)+"/actors/"+itoa(actor.ID)+"/generate-lock-sheet", map[string]any{}, ownerCookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected actor lock sheet 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Asset map[string]any `json:"asset"`
		Item  map[string]any `json:"item"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode actor lock sheet: %v", err)
	}
	if payload.Asset["id"] == nil || payload.Item["id"] == nil || payload.Asset["id"] != payload.Item["id"] {
		t.Fatalf("expected compatible asset and item fields, got %s", resp.Body.String())
	}
}

func TestNovelVideoImageBatchGenerateReviewAndImagePackageExport(t *testing.T) {
	waitCh := make(chan struct{})
	defer close(waitCh)
	testApp, db := newTestApp(t, &stubProvider{
		waitCh: waitCh,
		result: ImageGenerationResult{
			Base64Image:       "c2hvdC1pbWFnZQ==",
			MIMEType:          "image/png",
			ProviderRequestID: "req_novel_image_batch",
		},
	})
	owner, ownerCookies := createLoggedInUser(t, testApp, "novel_image_batch", "test-password")
	setUserCredits(t, testApp, owner.ID, 200)
	actorRef := seedReferenceAsset(t, testApp, owner.ID, "actor.png", "image/png", []byte("actor-ref"))
	sceneRef := seedReferenceAsset(t, testApp, owner.ID, "station.png", "image/png", []byte("scene-ref"))
	project := NovelVideoProject{
		UserID:         owner.ID,
		Title:          "夜航候选图",
		SourceText:     "三名演员在航站楼行动。",
		ContentMode:    "short_film_image",
		SchemaVersion:  3,
		GenerationMode: "image_series",
		StylePreset:    "写实短电影",
		AspectRatio:    "16:9",
		ImageModel:     "gpt-image-2",
		Status:         NovelVideoProjectStatusPlanned,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	actorA := NovelVideoCreature{ProjectID: project.ID, UserID: owner.ID, Name: "林岚", VisualConsistencyPrompt: "短发女演员，左眼下小痣", ReviewStatus: NovelVideoReviewStatusApproved}
	actorB := NovelVideoCreature{ProjectID: project.ID, UserID: owner.ID, Name: "周河", VisualConsistencyPrompt: "中年男演员，灰色风衣", ReviewStatus: NovelVideoReviewStatusApproved}
	if err := db.Create(&actorA).Error; err != nil {
		t.Fatalf("create actor a: %v", err)
	}
	if err := db.Create(&actorB).Error; err != nil {
		t.Fatalf("create actor b: %v", err)
	}
	actorAsset := NovelVideoAsset{ProjectID: project.ID, UserID: owner.ID, Kind: "actor_ref", Name: "林岚正脸", ReferenceURL: actorRef.PreviewURL, MetadataJSON: encodeJSON(map[string]any{"actor_id": actorA.ID, "reference_asset_ids": []uint{actorRef.ID}, "approved": true}), ReviewStatus: NovelVideoReviewStatusApproved}
	sceneAsset := NovelVideoAsset{ProjectID: project.ID, UserID: owner.ID, Kind: "scene", Name: "雨夜航站楼", ReferenceURL: sceneRef.PreviewURL, MetadataJSON: encodeJSON(map[string]any{"reference_asset_ids": []uint{sceneRef.ID}}), ReviewStatus: NovelVideoReviewStatusApproved}
	if err := db.Create(&actorAsset).Error; err != nil {
		t.Fatalf("create actor asset: %v", err)
	}
	if err := db.Create(&sceneAsset).Error; err != nil {
		t.Fatalf("create scene asset: %v", err)
	}
	episode := NovelVideoEpisode{ProjectID: project.ID, UserID: owner.ID, Number: 1, Title: "雨夜", Status: NovelVideoReviewStatusApproved}
	if err := db.Create(&episode).Error; err != nil {
		t.Fatalf("create episode: %v", err)
	}
	shots := []NovelVideoShot{
		{ProjectID: project.ID, EpisodeID: episode.ID, UserID: owner.ID, Number: 1, Title: "单人特写", Prompt: "林岚在雨夜航站楼回头", ImagePrompt: "电影剧照，林岚单人近景", CreatureIDsJSON: encodeJSON([]uint{actorA.ID}), AssetRefsJSON: encodeNovelVideoAssetRefs([]novelVideoAssetRef{{Type: "creature", ID: actorA.ID}, {Type: "asset", ID: actorAsset.ID}}), Status: NovelVideoReviewStatusApproved},
		{ProjectID: project.ID, EpisodeID: episode.ID, UserID: owner.ID, Number: 2, Title: "双人对峙", Prompt: "林岚与周河在闸机前对峙", ImagePrompt: "电影剧照，双人中景，雨夜航站楼", CreatureIDsJSON: encodeJSON([]uint{actorA.ID, actorB.ID}), AssetRefsJSON: encodeNovelVideoAssetRefs([]novelVideoAssetRef{{Type: "creature", ID: actorA.ID}, {Type: "creature", ID: actorB.ID}, {Type: "asset", ID: sceneAsset.ID}}), Status: NovelVideoReviewStatusApproved},
	}
	if err := db.Create(&shots).Error; err != nil {
		t.Fatalf("create shots: %v", err)
	}

	generateResp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects/"+itoa(project.ID)+"/images/generate", map[string]any{
		"shot_ids":            []uint{shots[0].ID, shots[1].ID},
		"candidates_per_shot": 2,
		"mode":                "text_to_image",
		"lock_level":          "strict",
	}, ownerCookies)
	if generateResp.Code != http.StatusOK {
		t.Fatalf("expected images generate 200, got %d: %s", generateResp.Code, generateResp.Body.String())
	}
	var generated struct {
		Queued          int `json:"queued"`
		TotalCandidates int `json:"total_candidates"`
		Items           []struct {
			ID                 uint   `json:"id"`
			ShotID             uint   `json:"shot_id"`
			GenerationRecordID uint   `json:"generation_record_id"`
			ReferenceIntent    string `json:"reference_intent"`
			ReviewStatus       string `json:"review_status"`
			GenerationStatus   string `json:"generation_status"`
			GenerationStage    string `json:"generation_stage"`
			GenerationProgress int    `json:"generation_progress"`
		} `json:"items"`
	}
	if err := json.Unmarshal(generateResp.Body.Bytes(), &generated); err != nil {
		t.Fatalf("decode generated images: %v", err)
	}
	if generated.Queued != 4 || generated.TotalCandidates != 4 || len(generated.Items) != 4 {
		t.Fatalf("unexpected generated payload: %+v", generated)
	}
	if generated.Items[0].ReferenceIntent != GenerationReferenceIntentCharacter || generated.Items[2].ReferenceIntent != GenerationReferenceIntentCompose {
		t.Fatalf("expected character then compose reference intents, got %+v", generated.Items)
	}
	for _, item := range generated.Items {
		if item.GenerationStatus != GenerationStatusQueued || item.GenerationStage != GenerationStageQueued || item.GenerationProgress != 5 {
			t.Fatalf("expected generated image to expose queued generation state, got %+v", item)
		}
	}
	var records []GenerationRecord
	if err := db.Order("id asc").Find(&records).Error; err != nil {
		t.Fatalf("load generation records: %v", err)
	}
	if len(records) != 4 || records[0].ReferenceWeight == 0 || records[0].ToolMode != GenerationToolModeGenerate || records[0].BatchTotal != 4 {
		t.Fatalf("unexpected generation records: %+v", records)
	}
	if err := db.Model(&GenerationRecord{}).Where("id = ?", generated.Items[0].GenerationRecordID).Updates(map[string]any{
		"status": GenerationStatusRunning,
		"stage":  GenerationStageRequestingProvider,
	}).Error; err != nil {
		t.Fatalf("mark first record running: %v", err)
	}
	if err := db.Model(&GenerationRecord{}).Where("id = ?", generated.Items[1].GenerationRecordID).Updates(map[string]any{
		"status":        GenerationStatusFailed,
		"stage":         GenerationStageFailed,
		"error_message": "provider timeout",
	}).Error; err != nil {
		t.Fatalf("mark second record failed: %v", err)
	}

	imagesResp := performJSONRequest(t, testApp, http.MethodGet, "/api/novel-video-projects/"+itoa(project.ID)+"/images?shot_id="+itoa(shots[0].ID), nil, ownerCookies)
	if imagesResp.Code != http.StatusOK {
		t.Fatalf("expected images list 200, got %d: %s", imagesResp.Code, imagesResp.Body.String())
	}
	if !strings.Contains(imagesResp.Body.String(), `"shot_id":`+itoa(shots[0].ID)) || strings.Contains(imagesResp.Body.String(), `"shot_id":`+itoa(shots[1].ID)) {
		t.Fatalf("expected shot-filtered image list, got %s", imagesResp.Body.String())
	}
	if !strings.Contains(imagesResp.Body.String(), `"generation_status":"running"`) ||
		!strings.Contains(imagesResp.Body.String(), `"generation_stage":"requesting_provider"`) ||
		!strings.Contains(imagesResp.Body.String(), `"generation_progress":35`) ||
		!strings.Contains(imagesResp.Body.String(), `"generation_status":"failed"`) ||
		!strings.Contains(imagesResp.Body.String(), `"generation_progress":100`) ||
		!strings.Contains(imagesResp.Body.String(), `"error_message":"provider timeout"`) {
		t.Fatalf("expected image list to hydrate generation state, got %s", imagesResp.Body.String())
	}
	prefixCollision := NovelVideoShotImage{ProjectID: project.ID, EpisodeID: episode.ID, ShotID: shots[0].ID, UserID: owner.ID, Kind: NovelVideoAssetKindShotImage, ActorIDsJSON: encodeJSON([]uint{actorA.ID * 10}), ReviewStatus: NovelVideoReviewStatusNeedsReview}
	if err := db.Create(&prefixCollision).Error; err != nil {
		t.Fatalf("create prefix collision image: %v", err)
	}
	actorImagesResp := performJSONRequest(t, testApp, http.MethodGet, "/api/novel-video-projects/"+itoa(project.ID)+"/images?actor_id="+itoa(actorA.ID), nil, ownerCookies)
	if actorImagesResp.Code != http.StatusOK {
		t.Fatalf("expected actor image list 200, got %d: %s", actorImagesResp.Code, actorImagesResp.Body.String())
	}
	if strings.Contains(actorImagesResp.Body.String(), `"id":`+itoa(prefixCollision.ID)) {
		t.Fatalf("expected exact actor id filtering, got %s", actorImagesResp.Body.String())
	}

	patchResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/novel-video-projects/"+itoa(project.ID)+"/images/"+itoa(generated.Items[0].ID), map[string]any{
		"selected":      true,
		"review_status": "approved",
		"review_note":   "定稿",
	}, ownerCookies)
	if patchResp.Code != http.StatusOK {
		t.Fatalf("expected image patch 200, got %d: %s", patchResp.Code, patchResp.Body.String())
	}
	if !strings.Contains(patchResp.Body.String(), `"selected":true`) {
		t.Fatalf("expected selected image response, got %s", patchResp.Body.String())
	}

	exportResp := performJSONRequest(t, testApp, http.MethodGet, "/api/novel-video-projects/"+itoa(project.ID)+"/export?format=image_package", nil, ownerCookies)
	if exportResp.Code != http.StatusOK {
		t.Fatalf("expected image package export 200, got %d: %s", exportResp.Code, exportResp.Body.String())
	}
	assertZipContains(t, exportResp.Body.Bytes(), []string{"project.json", "actor-cards.json", "shot-images.json", "selected-images.json", "prompts.json", "manifest.json"})
}

func TestNovelVideoShotImagesGenerateExecutesCandidates(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       "c2hvdC1pbWFnZQ==",
			MIMEType:          "image/png",
			ProviderRequestID: "req_novel_shot_image",
		},
	}
	testApp, db := newTestApp(t, provider)
	owner, ownerCookies := createLoggedInUser(t, testApp, "novel_shot_image_exec", "test-password")
	setUserCredits(t, testApp, owner.ID, 20)
	project, episode, shot := seedNovelVideoShotImageProject(t, db, owner)

	generateResp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects/"+itoa(project.ID)+"/images/generate", map[string]any{
		"shot_ids":            []uint{shot.ID},
		"candidates_per_shot": 1,
		"mode":                "text_to_image",
	}, ownerCookies)
	if generateResp.Code != http.StatusOK {
		t.Fatalf("expected images generate 200, got %d: %s", generateResp.Code, generateResp.Body.String())
	}

	waitForCondition(t, 3*time.Second, func() bool {
		var image NovelVideoShotImage
		if err := db.Where("project_id = ? AND episode_id = ? AND shot_id = ?", project.ID, episode.ID, shot.ID).First(&image).Error; err != nil {
			return false
		}
		if image.GenerationRecordID == nil {
			return false
		}
		var record GenerationRecord
		if err := db.First(&record, *image.GenerationRecordID).Error; err != nil {
			return false
		}
		return record.Status == GenerationStatusSucceeded && record.WorkID != nil && strings.TrimSpace(record.PreviewURL) != ""
	})

	imagesResp := performJSONRequest(t, testApp, http.MethodGet, "/api/novel-video-projects/"+itoa(project.ID)+"/images", nil, ownerCookies)
	if imagesResp.Code != http.StatusOK {
		t.Fatalf("expected images list 200, got %d: %s", imagesResp.Code, imagesResp.Body.String())
	}
	if !strings.Contains(imagesResp.Body.String(), `"generation_status":"succeeded"`) ||
		!strings.Contains(imagesResp.Body.String(), `"preview_url":"`) ||
		!strings.Contains(imagesResp.Body.String(), `"work_id":`) {
		t.Fatalf("expected image list to expose generated work, got %s", imagesResp.Body.String())
	}
	if provider.calls != 1 {
		t.Fatalf("expected provider called once, got %d", provider.calls)
	}
}

func TestNovelVideoShotImagesGenerateMarksProviderFailure(t *testing.T) {
	provider := &stubProvider{err: &ProviderError{Code: "provider_down", Message: "provider unavailable"}}
	testApp, db := newTestApp(t, provider)
	owner, ownerCookies := createLoggedInUser(t, testApp, "novel_shot_image_fail", "test-password")
	setUserCredits(t, testApp, owner.ID, 20)
	project, _, shot := seedNovelVideoShotImageProject(t, db, owner)

	generateResp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects/"+itoa(project.ID)+"/images/generate", map[string]any{
		"shot_ids":            []uint{shot.ID},
		"candidates_per_shot": 1,
		"mode":                "text_to_image",
	}, ownerCookies)
	if generateResp.Code != http.StatusOK {
		t.Fatalf("expected images generate 200, got %d: %s", generateResp.Code, generateResp.Body.String())
	}

	waitForCondition(t, 3*time.Second, func() bool {
		var image NovelVideoShotImage
		if err := db.Where("project_id = ? AND shot_id = ?", project.ID, shot.ID).First(&image).Error; err != nil {
			return false
		}
		return image.ErrorMessage != ""
	})

	imagesResp := performJSONRequest(t, testApp, http.MethodGet, "/api/novel-video-projects/"+itoa(project.ID)+"/images", nil, ownerCookies)
	if imagesResp.Code != http.StatusOK {
		t.Fatalf("expected images list 200, got %d: %s", imagesResp.Code, imagesResp.Body.String())
	}
	if !strings.Contains(imagesResp.Body.String(), `"generation_status":"failed"`) ||
		!strings.Contains(imagesResp.Body.String(), `"error_message":"`) {
		t.Fatalf("expected image list to expose provider failure, got %s", imagesResp.Body.String())
	}
}

func TestNovelVideoShotImagesGenerateRejectsInsufficientCreditsWithoutRecords(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	owner, ownerCookies := createLoggedInUser(t, testApp, "novel_shot_image_low_balance", "test-password")
	setUserCredits(t, testApp, owner.ID, 1)
	project, _, shot := seedNovelVideoShotImageProject(t, db, owner)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects/"+itoa(project.ID)+"/images/generate", map[string]any{
		"shot_ids":            []uint{shot.ID},
		"candidates_per_shot": 2,
		"mode":                "text_to_image",
	}, ownerCookies)
	if resp.Code != http.StatusConflict || !strings.Contains(resp.Body.String(), `"code":"credits_insufficient"`) {
		t.Fatalf("expected credits_insufficient 409, got %d: %s", resp.Code, resp.Body.String())
	}
	var recordCount int64
	if err := db.Model(&GenerationRecord{}).Where("user_id = ?", owner.ID).Count(&recordCount).Error; err != nil {
		t.Fatalf("count generation records: %v", err)
	}
	if recordCount != 0 {
		t.Fatalf("expected no generation records when credits are insufficient, got %d", recordCount)
	}
	var imageCount int64
	if err := db.Model(&NovelVideoShotImage{}).Where("project_id = ?", project.ID).Count(&imageCount).Error; err != nil {
		t.Fatalf("count shot images: %v", err)
	}
	if imageCount != 0 {
		t.Fatalf("expected no shot images when credits are insufficient, got %d", imageCount)
	}
}

func seedNovelVideoShotImageProject(t *testing.T, db *gorm.DB, owner User) (NovelVideoProject, NovelVideoEpisode, NovelVideoShot) {
	t.Helper()
	project := NovelVideoProject{
		UserID:         owner.ID,
		Title:          "shot image project",
		SourceText:     "source text",
		ContentMode:    "short_film_image",
		SchemaVersion:  3,
		GenerationMode: "image_series",
		StylePreset:    "cinematic",
		AspectRatio:    "16:9",
		ImageModel:     "gpt-image-2",
		Status:         NovelVideoProjectStatusPlanned,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	episode := NovelVideoEpisode{ProjectID: project.ID, UserID: owner.ID, Number: 1, Title: "episode", Status: NovelVideoReviewStatusApproved}
	if err := db.Create(&episode).Error; err != nil {
		t.Fatalf("create episode: %v", err)
	}
	shot := NovelVideoShot{
		ProjectID: project.ID,
		EpisodeID: episode.ID,
		UserID:    owner.ID,
		Number:    1,
		Title:     "shot",
		Prompt:    "cinematic shot",
		Status:    NovelVideoReviewStatusApproved,
	}
	if err := db.Create(&shot).Error; err != nil {
		t.Fatalf("create shot: %v", err)
	}
	return project, episode, shot
}

func TestNovelVideoRenderEndpointCreatesRecoverableJobsWithoutProviderSubmit(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	videoProvider := &stubVideoProvider{submitResult: VideoSubmitResult{TaskID: "render-route-must-not-submit"}}
	testApp.videoProvider = videoProvider
	user, cookies := createLoggedInUser(t, testApp, "novel_job_owner", "test-password")
	setUserCredits(t, testApp, user.ID, 100)
	project := NovelVideoProject{UserID: user.ID, Title: "任务队列", SourceText: "正文", AspectRatio: "16:9", Duration: "10", VideoModel: "sora-2", Status: NovelVideoProjectStatusPlanned}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	episode := NovelVideoEpisode{ProjectID: project.ID, UserID: user.ID, Number: 1, Title: "第一集", Status: NovelVideoReviewStatusApproved}
	if err := db.Create(&episode).Error; err != nil {
		t.Fatalf("create episode: %v", err)
	}
	shots := []NovelVideoShot{
		{ProjectID: project.ID, EpisodeID: episode.ID, UserID: user.ID, Number: 1, Title: "镜头一", Prompt: "雨夜推镜", Status: NovelVideoReviewStatusApproved},
		{ProjectID: project.ID, EpisodeID: episode.ID, UserID: user.ID, Number: 2, Title: "镜头二", Prompt: "石门开启", Status: NovelVideoReviewStatusApproved},
	}
	if err := db.Create(&shots).Error; err != nil {
		t.Fatalf("create shots: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects/"+itoa(project.ID)+"/render", map[string]any{}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected render 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Status string `json:"status"`
		Queued int    `json:"queued"`
		Jobs   []struct {
			ID     uint   `json:"id"`
			Type   string `json:"type"`
			Status string `json:"status"`
			ShotID uint   `json:"shot_id"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode render payload: %v", err)
	}
	if payload.Status != GenerationStatusQueued || payload.Queued != 2 || len(payload.Jobs) != 2 {
		t.Fatalf("unexpected render payload: %+v", payload)
	}
	if len(videoProvider.submitInputs) != 0 {
		t.Fatalf("new DB queue render endpoint must not submit provider synchronously, got %+v", videoProvider.submitInputs)
	}
	var jobs []NovelVideoJob
	if err := db.Where("project_id = ?", project.ID).Order("id asc").Find(&jobs).Error; err != nil {
		t.Fatalf("load jobs: %v", err)
	}
	if len(jobs) != 2 || jobs[0].Status != GenerationStatusQueued || jobs[0].JobType != "shot_video" || jobs[0].ShotID == nil {
		t.Fatalf("unexpected jobs: %+v", jobs)
	}
}

func TestNovelVideoComposeReportsMissingFFmpegAndStoresComposition(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "novel_compose_owner", "test-password")
	project := NovelVideoProject{UserID: user.ID, Title: "合成测试", SourceText: "正文", AspectRatio: "16:9", Duration: "10", VideoModel: "sora-2", Status: NovelVideoProjectStatusSucceeded}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/novel-video-projects/"+itoa(project.ID)+"/compose", map[string]any{}, cookies)
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected missing ffmpeg 503, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "ffmpeg") {
		t.Fatalf("expected ffmpeg error, got: %s", resp.Body.String())
	}
	var compositions []NovelVideoComposition
	if err := db.Where("project_id = ?", project.ID).Find(&compositions).Error; err != nil {
		t.Fatalf("load compositions: %v", err)
	}
	if len(compositions) != 1 || compositions[0].Status != GenerationStatusFailed || compositions[0].ErrorCode != "ffmpeg_unavailable" {
		t.Fatalf("unexpected failed composition: %+v", compositions)
	}
}

func TestNovelVideoExportZipAndJianyingPackages(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "novel_zip_owner", "test-password")
	project := NovelVideoProject{UserID: user.ID, Title: "灰塔兽群", SourceText: "灰塔里有兽群。", AspectRatio: "16:9", Duration: "10", VideoModel: "sora-2", Status: NovelVideoProjectStatusPlanned}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	asset := NovelVideoAsset{ProjectID: project.ID, UserID: user.ID, Kind: "scene", Name: "灰塔走廊", Prompt: "潮湿石塔走廊", Version: 1, ReviewStatus: NovelVideoReviewStatusApproved}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatalf("create asset: %v", err)
	}
	episode := NovelVideoEpisode{ProjectID: project.ID, UserID: user.ID, Number: 1, Title: "第一集", Summary: "进入灰塔", Status: NovelVideoReviewStatusApproved}
	if err := db.Create(&episode).Error; err != nil {
		t.Fatalf("create episode: %v", err)
	}
	shot := NovelVideoShot{ProjectID: project.ID, EpisodeID: episode.ID, UserID: user.ID, Number: 1, Title: "门兽抬头", Prompt: "低机位，门兽抬头。", Status: NovelVideoReviewStatusApproved}
	if err := db.Create(&shot).Error; err != nil {
		t.Fatalf("create shot: %v", err)
	}

	jsonResp := performJSONRequest(t, testApp, http.MethodGet, "/api/novel-video-projects/"+itoa(project.ID)+"/export?format=json", nil, cookies)
	if jsonResp.Code != http.StatusOK {
		t.Fatalf("expected json export 200, got %d: %s", jsonResp.Code, jsonResp.Body.String())
	}
	var exported struct {
		Assets []struct {
			ID   uint   `json:"id"`
			Kind string `json:"kind"`
			Name string `json:"name"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(jsonResp.Body.Bytes(), &exported); err != nil {
		t.Fatalf("decode json export: %v", err)
	}
	if len(exported.Assets) != 1 || exported.Assets[0].ID != asset.ID || exported.Assets[0].Kind != asset.Kind {
		t.Fatalf("expected json export assets, got %+v", exported.Assets)
	}

	zipResp := performJSONRequest(t, testApp, http.MethodGet, "/api/novel-video-projects/"+itoa(project.ID)+"/export?format=zip", nil, cookies)
	if zipResp.Code != http.StatusOK {
		t.Fatalf("expected zip export 200, got %d: %s", zipResp.Code, zipResp.Body.String())
	}
	assertZipContains(t, zipResp.Body.Bytes(), []string{"project.json", "project.md", "assets.json", "subtitles.srt"})

	jianyingResp := performJSONRequest(t, testApp, http.MethodGet, "/api/novel-video-projects/"+itoa(project.ID)+"/export?format=jianying", nil, cookies)
	if jianyingResp.Code != http.StatusOK {
		t.Fatalf("expected jianying export 200, got %d: %s", jianyingResp.Code, jianyingResp.Body.String())
	}
	assertZipContains(t, jianyingResp.Body.Bytes(), []string{"draft_meta_info.json", "draft_content.json", "materials/project.json"})
}

func assertZipContains(t *testing.T, data []byte, expected []string) {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	names := map[string]bool{}
	for _, file := range reader.File {
		names[file.Name] = true
	}
	for _, name := range expected {
		if !names[name] {
			t.Fatalf("expected zip to contain %q, got %v", name, names)
		}
	}
}
