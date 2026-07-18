package app

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestVideoStylePresetsListReturnsOnlyActiveSorted(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "video_style_list_user", "test-password")
	setUserCredits(t, testApp, user.ID, 10)

	activeLate := VideoStylePreset{
		Slug:        "film-late",
		Title:       "Film Late",
		Category:    "movie",
		Description: "late sort",
		TagsJSON:    mustJSON(t, []string{"cinematic", "new"}),
		PreviewURL:  "https://oss.example.com/video-styles/film-late.png",
		StylePrompt: "late cinematic light",
		SortOrder:   20,
		IsActive:    true,
	}
	activeFirst := VideoStylePreset{
		Slug:        "anime-first",
		Title:       "Anime First",
		Category:    "animation",
		TagsJSON:    mustJSON(t, []string{"beginner"}),
		PreviewURL:  "https://oss.example.com/video-styles/anime-first.png",
		StylePrompt: "soft hand drawn animation",
		SortOrder:   5,
		IsActive:    true,
	}
	inactive := VideoStylePreset{
		Slug:        "hidden-style",
		Title:       "Hidden Style",
		PreviewURL:  "https://oss.example.com/video-styles/hidden.png",
		StylePrompt: "hidden prompt",
		SortOrder:   1,
		IsActive:    false,
	}
	if err := testApp.db.Create(&[]VideoStylePreset{activeLate, activeFirst, inactive}).Error; err != nil {
		t.Fatalf("seed video style presets: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/videos/style-presets", nil, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected style preset list 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Items []struct {
			ID          uint     `json:"id"`
			Slug        string   `json:"slug"`
			Title       string   `json:"title"`
			Category    string   `json:"category"`
			Tags        []string `json:"tags"`
			PreviewURL  string   `json:"preview_url"`
			StylePrompt string   `json:"style_prompt"`
			SortOrder   int      `json:"sort_order"`
			UseCount    int      `json:"use_count"`
		} `json:"items"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode style preset list: %v", err)
	}
	if len(payload.Items) < 2 || payload.Items[0].Slug != "anime-first" || payload.Items[1].Slug != "film-late" {
		t.Fatalf("expected active presets sorted by sort_order, got %+v", payload.Items)
	}
	if payload.Items[0].Title != "Anime First" || payload.Items[0].PreviewURL == "" || payload.Items[0].StylePrompt == "" ||
		len(payload.Items[0].Tags) != 1 || payload.Items[0].Tags[0] != "beginner" {
		t.Fatalf("unexpected style preset payload: %+v", payload.Items[0])
	}
	if strings.Contains(resp.Body.String(), "hidden-style") {
		t.Fatalf("inactive video style preset must be hidden: %s", resp.Body.String())
	}
}

func TestAdminVideoStylePresetsCRUD(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)

	createResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/video-style-presets", map[string]any{
		"slug":         "admin-video-style",
		"title":        "Admin Video Style",
		"category":     "电影感",
		"description":  "Created by admin",
		"tags":         []string{"cinematic", "featured"},
		"preview_url":  "https://oss.example.com/video-styles/admin.png",
		"style_prompt": "cinematic lensing, soft film grain",
		"sort_order":   15,
		"is_active":    true,
	}, adminCookies)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("expected create video style 201, got %d: %s", createResp.Code, createResp.Body.String())
	}
	var created VideoStylePreset
	if err := json.Unmarshal(createResp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created video style: %v", err)
	}
	if created.ID == 0 || created.Slug != "admin-video-style" || created.StylePrompt == "" || !created.IsActive {
		t.Fatalf("unexpected created video style: %+v", created)
	}

	listResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/video-style-presets?q=admin&page=1&page_size=5", nil, adminCookies)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected admin video style list 200, got %d: %s", listResp.Code, listResp.Body.String())
	}
	if !strings.Contains(listResp.Body.String(), "admin-video-style") {
		t.Fatalf("expected created style in admin list: %s", listResp.Body.String())
	}

	updateResp := performJSONRequest(t, testApp, http.MethodPut, "/api/admin/video-style-presets/"+itoa(created.ID), map[string]any{
		"title":        "Updated Video Style",
		"tags":         []string{"updated"},
		"style_prompt": "updated style prompt",
		"sort_order":   3,
		"is_active":    false,
	}, adminCookies)
	if updateResp.Code != http.StatusOK {
		t.Fatalf("expected update video style 200, got %d: %s", updateResp.Code, updateResp.Body.String())
	}
	var updated VideoStylePreset
	if err := json.Unmarshal(updateResp.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated video style: %v", err)
	}
	if updated.Title != "Updated Video Style" || updated.SortOrder != 3 || updated.IsActive {
		t.Fatalf("unexpected updated video style: %+v", updated)
	}
	tags := updated.Tags()
	if len(tags) != 1 || tags[0] != "updated" {
		t.Fatalf("expected updated tags, got %+v", tags)
	}

	deleteResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/admin/video-style-presets/"+itoa(created.ID), nil, adminCookies)
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("expected delete video style 200, got %d: %s", deleteResp.Code, deleteResp.Body.String())
	}
	var deleted VideoStylePreset
	if err := testApp.db.Unscoped().First(&deleted, created.ID).Error; err != nil {
		t.Fatalf("load deleted video style: %v", err)
	}
	if !deleted.DeletedAt.Valid {
		t.Fatalf("expected video style preset to be soft deleted")
	}
}

func TestUserVideoStyleTemplatesCRUDAndOwnership(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	owner, ownerCookies := createLoggedInUser(t, testApp, "video_style_template_owner", "test-password")
	_, otherCookies := createLoggedInUser(t, testApp, "video_style_template_other", "test-password")
	asset := seedReferenceAsset(t, testApp, owner.ID, "style-reference.png", "image/png", []byte("style-reference"))

	createResp := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/style-templates", map[string]any{
		"title":              "My Film Style",
		"description":        "private house style",
		"reference_asset_id": asset.ID,
		"style_prompt":       "warm private film style",
	}, ownerCookies)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("expected create user video style template 201, got %d: %s", createResp.Code, createResp.Body.String())
	}
	var created UserVideoStyleTemplate
	if err := json.Unmarshal(createResp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created template: %v", err)
	}
	if created.ID == 0 || created.UserID != owner.ID || created.ReferenceAssetID != asset.ID || created.PreviewURL == "" || !created.IsActive {
		t.Fatalf("unexpected created template: %+v", created)
	}

	listResp := performJSONRequest(t, testApp, http.MethodGet, "/api/videos/style-templates", nil, ownerCookies)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected owner template list 200, got %d: %s", listResp.Code, listResp.Body.String())
	}
	if !strings.Contains(listResp.Body.String(), "My Film Style") {
		t.Fatalf("expected owner list to include template: %s", listResp.Body.String())
	}
	otherListResp := performJSONRequest(t, testApp, http.MethodGet, "/api/videos/style-templates", nil, otherCookies)
	if otherListResp.Code != http.StatusOK || strings.Contains(otherListResp.Body.String(), "My Film Style") {
		t.Fatalf("other user must not see owner template, got %d: %s", otherListResp.Code, otherListResp.Body.String())
	}

	otherUseResp := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/generations/async", map[string]any{
		"prompt":                "try using another user style",
		"aspect_ratio":          "16:9",
		"duration":              "10",
		"model":                 "sora-2",
		"custom_video_style_id": created.ID,
		"reference_asset_ids":   []uint{},
	}, otherCookies)
	if otherUseResp.Code != http.StatusNotFound {
		t.Fatalf("expected other user custom style usage 404, got %d: %s", otherUseResp.Code, otherUseResp.Body.String())
	}

	deleteResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/videos/style-templates/"+itoa(created.ID), nil, ownerCookies)
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("expected owner delete template 200, got %d: %s", deleteResp.Code, deleteResp.Body.String())
	}
	var deleted UserVideoStyleTemplate
	if err := testApp.db.Unscoped().First(&deleted, created.ID).Error; err != nil {
		t.Fatalf("load deleted template: %v", err)
	}
	if !deleted.DeletedAt.Valid {
		t.Fatalf("expected user video style template to be soft deleted")
	}
}

func TestVideoGenerationAppliesOfficialAndCustomStylePrompts(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	videoProvider := &stubVideoProvider{
		submitResult: VideoSubmitResult{TaskID: "style-video-task"},
		pollResults: []VideoTaskResult{
			{TaskID: "style-video-task", Status: VideoTaskSucceeded, OutputBase64: base64.StdEncoding.EncodeToString([]byte("video-bytes")), MIMEType: "video/mp4", ProviderRequestID: "style-provider"},
		},
	}
	testApp.videoProvider = videoProvider
	user, cookies := createLoggedInUser(t, testApp, "video_style_generate_user", "test-password")
	setUserCredits(t, testApp, user.ID, 30)
	contentA := seedReferenceAsset(t, testApp, user.ID, "content-a.png", "image/png", []byte("content-a"))
	contentB := seedReferenceAsset(t, testApp, user.ID, "content-b.png", "image/png", []byte("content-b"))
	contentC := seedReferenceAsset(t, testApp, user.ID, "content-c.png", "image/png", []byte("content-c"))
	styleAsset := seedReferenceAsset(t, testApp, user.ID, "style-image.png", "image/png", []byte("style-image"))
	official := VideoStylePreset{
		Slug:        "official-film",
		Title:       "Official Film",
		PreviewURL:  "https://oss.example.com/video-styles/official-film.png",
		StylePrompt: "official cinematic color grading",
		SortOrder:   1,
		IsActive:    true,
	}
	if err := db.Create(&official).Error; err != nil {
		t.Fatalf("create official style: %v", err)
	}
	custom := UserVideoStyleTemplate{
		UserID:           user.ID,
		Title:            "Custom Brand Style",
		Description:      "brand style",
		ReferenceAssetID: styleAsset.ID,
		PreviewURL:       styleAsset.PreviewURL,
		StylePrompt:      "custom warm brand style",
		IsActive:         true,
	}
	if err := db.Create(&custom).Error; err != nil {
		t.Fatalf("create custom style: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/generations/async", map[string]any{
		"prompt":                "city launch video",
		"aspect_ratio":          "16:9",
		"duration":              "10",
		"model":                 "sora-2",
		"video_style_preset_id": official.ID,
		"custom_video_style_id": custom.ID,
		"reference_asset_ids":   []uint{contentA.ID, contentB.ID, contentC.ID},
	}, cookies)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected styled video create 202, got %d: %s", resp.Code, resp.Body.String())
	}
	var created struct {
		GenerationID uint `json:"generation_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode styled video response: %v", err)
	}

	waitFor(t, time.Second, func() bool {
		return len(videoProvider.submitInputs) == 1
	})
	input := videoProvider.submitInputs[0]
	if !strings.Contains(input.Prompt, "city launch video") ||
		!strings.Contains(input.Prompt, "official cinematic color grading") ||
		!strings.Contains(input.Prompt, "custom warm brand style") {
		t.Fatalf("expected provider prompt to include base and style prompts, got %q", input.Prompt)
	}
	if len(input.Images) != 4 {
		t.Fatalf("expected three content refs plus custom style image, got %d images: %+v", len(input.Images), input.Images)
	}

	var record GenerationRecord
	waitFor(t, time.Second, func() bool {
		err := db.First(&record, created.GenerationID).Error
		return err == nil && record.Status == GenerationStatusSucceeded
	})
	if record.StylePreset != "Official Film / Custom Brand Style" {
		t.Fatalf("expected generation style preset to store real style names, got %q", record.StylePreset)
	}
	var videoRecord VideoGenerationRecord
	if err := db.Where("generation_record_id = ?", created.GenerationID).First(&videoRecord).Error; err != nil {
		t.Fatalf("load video record: %v", err)
	}
	if videoRecord.StylePreset != "Official Film / Custom Brand Style" || videoRecord.DurationSeconds != 10 {
		t.Fatalf("expected video style and duration separated, got %+v", videoRecord)
	}
	var updatedOfficial VideoStylePreset
	var updatedCustom UserVideoStyleTemplate
	if err := db.First(&updatedOfficial, official.ID).Error; err != nil {
		t.Fatalf("reload official style: %v", err)
	}
	if err := db.First(&updatedCustom, custom.ID).Error; err != nil {
		t.Fatalf("reload custom style: %v", err)
	}
	if updatedOfficial.UseCount != 1 || updatedCustom.UseCount != 1 {
		t.Fatalf("expected style use counters incremented, got official=%d custom=%d", updatedOfficial.UseCount, updatedCustom.UseCount)
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return string(raw)
}
