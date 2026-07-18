package app

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

type coupleAlbumTestPagePayload struct {
	ID                 uint   `json:"id"`
	PageNumber         int    `json:"page_number"`
	PageTitle          string `json:"page_title"`
	Caption            string `json:"caption"`
	Status             string `json:"status"`
	GenerationRecordID uint   `json:"generation_record_id"`
	WorkID             uint   `json:"work_id"`
	PreviewURL         string `json:"preview_url"`
	DownloadURL        string `json:"download_url"`
	ErrorCode          string `json:"error_code"`
	ErrorMessage       string `json:"error_message"`
}

type coupleAlbumTestAlbumPayload struct {
	ID                     uint                         `json:"id"`
	Title                  string                       `json:"title"`
	Location               string                       `json:"location"`
	StoryTemplate          string                       `json:"story_template"`
	Style                  string                       `json:"style"`
	Status                 string                       `json:"status"`
	ShareToken             string                       `json:"share_token"`
	ShareEnabled           bool                         `json:"share_enabled"`
	MaleReferenceAssetID   uint                         `json:"male_reference_asset_id"`
	FemaleReferenceAssetID uint                         `json:"female_reference_asset_id"`
	Pages                  []coupleAlbumTestPagePayload `json:"pages"`
}

type coupleAlbumTestPayload struct {
	Album      coupleAlbumTestAlbumPayload `json:"album"`
	ShareToken string                      `json:"share_token"`
	ShareURL   string                      `json:"share_url"`
}

func TestCoupleAlbumCreateGenerateShareAndPermission(t *testing.T) {
	provider := &stubProvider{
		results: coupleAlbumProviderResults(8, "journey"),
	}
	testApp, db := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "album_owner", "test-password")
	setUserCredits(t, testApp, user.ID, 40)
	male := seedReferenceAsset(t, testApp, user.ID, "male.png", "image/png", []byte("male"))
	female := seedReferenceAsset(t, testApp, user.ID, "female.png", "image/png", []byte("female"))

	createResp := performJSONRequest(t, testApp, http.MethodPost, "/api/couple-albums", coupleAlbumCreateBody(male.ID, female.ID), cookies)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("expected album create 201, got %d: %s", createResp.Code, createResp.Body.String())
	}
	created := decodeCoupleAlbumTestPayload(t, createResp.Body.Bytes())
	if created.Album.ID == 0 || created.Album.Title != "我们在大理的 520" || created.Album.Location != "大理" ||
		created.Album.StoryTemplate != "city_walk" || created.Album.Style != "film" ||
		created.Album.Status != "draft" || created.Album.ShareToken == "" || created.Album.ShareEnabled ||
		created.Album.MaleReferenceAssetID != male.ID || created.Album.FemaleReferenceAssetID != female.ID {
		t.Fatalf("unexpected created album payload: %+v", created.Album)
	}

	publicBeforeShare := performJSONRequest(t, testApp, http.MethodGet, "/api/public/couple-albums/"+created.Album.ShareToken, nil, nil)
	if publicBeforeShare.Code != http.StatusNotFound {
		t.Fatalf("expected disabled share token to be hidden, got %d: %s", publicBeforeShare.Code, publicBeforeShare.Body.String())
	}

	_, otherCookies := createLoggedInUser(t, testApp, "album_viewer", "test-password")
	otherGetResp := performJSONRequest(t, testApp, http.MethodGet, "/api/couple-albums/"+itoa(created.Album.ID), nil, otherCookies)
	if otherGetResp.Code != http.StatusNotFound {
		t.Fatalf("expected cross-user album detail to be isolated, got %d: %s", otherGetResp.Code, otherGetResp.Body.String())
	}

	generateResp := performJSONRequest(t, testApp, http.MethodPost, "/api/couple-albums/"+itoa(created.Album.ID)+"/generate", nil, cookies)
	if generateResp.Code != http.StatusAccepted {
		t.Fatalf("expected album generate 202, got %d: %s", generateResp.Code, generateResp.Body.String())
	}
	queued := decodeCoupleAlbumTestPayload(t, generateResp.Body.Bytes())
	if queued.Album.Status != "generating" || len(queued.Album.Pages) != 8 {
		t.Fatalf("expected 8 generating pages, got status=%s pages=%d", queued.Album.Status, len(queued.Album.Pages))
	}
	for index, page := range queued.Album.Pages {
		if page.PageNumber != index+1 || page.PageTitle == "" || page.Caption == "" ||
			page.Status != GenerationStatusQueued || page.GenerationRecordID == 0 {
			t.Fatalf("unexpected queued page %d: %+v", index, page)
		}
	}

	var album coupleAlbumTestPayload
	waitForCondition(t, 3*time.Second, func() bool {
		detailResp := performJSONRequest(t, testApp, http.MethodGet, "/api/couple-albums/"+itoa(created.Album.ID), nil, cookies)
		if detailResp.Code != http.StatusOK {
			return false
		}
		album = decodeCoupleAlbumTestPayload(t, detailResp.Body.Bytes())
		return album.Album.Status == GenerationStatusSucceeded
	})
	if len(album.Album.Pages) != 8 {
		t.Fatalf("expected 8 completed pages, got %d", len(album.Album.Pages))
	}
	for _, page := range album.Album.Pages {
		if page.Status != GenerationStatusSucceeded || page.WorkID == 0 || page.GenerationRecordID == 0 || page.PreviewURL == "" {
			t.Fatalf("expected completed page bound to work and generation, got %+v", page)
		}
	}
	var workCount int64
	if err := db.Model(&Work{}).Where("user_id = ? AND status = ?", user.ID, GenerationStatusSucceeded).Count(&workCount).Error; err != nil {
		t.Fatalf("count works: %v", err)
	}
	if workCount != 8 {
		t.Fatalf("expected 8 generated works, got %d", workCount)
	}
	if len(provider.inputs) != 8 {
		t.Fatalf("expected provider to receive 8 couple album inputs, got %d", len(provider.inputs))
	}
	for index, input := range provider.inputs {
		if input.ReferenceIntent != GenerationReferenceIntentCharacter {
			t.Fatalf("expected page %d to use character reference intent, got %+v", index+1, input.ReferenceIntent)
		}
		if len(input.ReferenceImages) != 2 {
			t.Fatalf("expected page %d to pass two reference images, got %+v", index+1, input.ReferenceImages)
		}
		if input.CompositionPlan != nil || input.BackgroundReferenceIndex != nil {
			t.Fatalf("expected page %d to avoid compose planning, got plan=%+v background=%+v", index+1, input.CompositionPlan, input.BackgroundReferenceIndex)
		}
		for _, expected := range []string{"【图1】为男主角参考", "【图2】为女主角参考", "不要换脸", "不要新增第三人"} {
			if !strings.Contains(input.Prompt, expected) {
				t.Fatalf("expected page %d prompt to contain %q, got %q", index+1, expected, input.Prompt)
			}
		}
	}

	listResp := performJSONRequest(t, testApp, http.MethodGet, "/api/couple-albums", nil, cookies)
	if listResp.Code != http.StatusOK || !strings.Contains(listResp.Body.String(), "我们在大理的 520") {
		t.Fatalf("expected owner album list to include created album, got %d: %s", listResp.Code, listResp.Body.String())
	}
	var listPayload struct {
		Albums []coupleAlbumTestAlbumPayload `json:"albums"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode couple album list: %v", err)
	}
	if len(listPayload.Albums) != 1 {
		t.Fatalf("expected one album in list, got %+v", listPayload.Albums)
	}
	if len(listPayload.Albums[0].Pages) != 8 {
		t.Fatalf("expected list album to include 8 pages, got %d", len(listPayload.Albums[0].Pages))
	}
	for _, page := range listPayload.Albums[0].Pages {
		if page.Status != GenerationStatusSucceeded || page.WorkID == 0 || page.PreviewURL == "" || page.DownloadURL == "" {
			t.Fatalf("expected list album page to expose completed work URLs, got %+v", page)
		}
	}

	shareResp := performJSONRequest(t, testApp, http.MethodPost, "/api/couple-albums/"+itoa(created.Album.ID)+"/share", nil, cookies)
	if shareResp.Code != http.StatusOK {
		t.Fatalf("expected share enable 200, got %d: %s", shareResp.Code, shareResp.Body.String())
	}
	shared := decodeCoupleAlbumTestPayload(t, shareResp.Body.Bytes())
	if shared.ShareToken == "" || !strings.Contains(shared.ShareURL, shared.ShareToken) {
		t.Fatalf("expected share token and URL, got %+v", shared)
	}

	publicResp := performJSONRequest(t, testApp, http.MethodGet, "/api/public/couple-albums/"+shared.ShareToken, nil, nil)
	if publicResp.Code != http.StatusOK {
		t.Fatalf("expected public shared album 200, got %d: %s", publicResp.Code, publicResp.Body.String())
	}
	publicBody := publicResp.Body.String()
	if !strings.Contains(publicBody, "我们在大理的 520") || !strings.Contains(publicBody, "大理") || !strings.Contains(publicBody, "pages") {
		t.Fatalf("expected public album content, got %s", publicBody)
	}
	if strings.Contains(publicBody, "album_owner") || strings.Contains(publicBody, "user_id") || strings.Contains(publicBody, "male_reference_asset_id") {
		t.Fatalf("public album must not expose owner identity or private reference metadata: %s", publicBody)
	}
	publicPayload := decodeCoupleAlbumTestPayload(t, publicResp.Body.Bytes())
	if publicPayload.Album.ShareToken != "" || publicPayload.Album.ShareEnabled {
		t.Fatalf("public album must not expose private sharing controls: %+v", publicPayload.Album)
	}
	if len(publicPayload.Album.Pages) != 8 {
		t.Fatalf("expected public album to include succeeded pages, got %+v", publicPayload.Album.Pages)
	}
	for _, page := range publicPayload.Album.Pages {
		expectedPreviewURL := "/api/public/works/" + itoa(page.WorkID) + "/file"
		if page.WorkID == 0 || page.PreviewURL != expectedPreviewURL {
			t.Fatalf("expected public preview URL %q, got page=%+v", expectedPreviewURL, page)
		}
		if strings.HasPrefix(page.PreviewURL, "/api/works/") {
			t.Fatalf("public album preview must not use private work file URL: %+v", page)
		}
	}
	previewResp := performJSONRequest(t, testApp, http.MethodGet, publicPayload.Album.Pages[0].PreviewURL, nil, nil)
	if previewResp.Code != http.StatusOK {
		t.Fatalf("expected shared album public preview file 200, got %d: %s", previewResp.Code, previewResp.Body.String())
	}
}

func TestChildhoodCareerDreamAlbumAllowsSingleChildReferenceAndBuildsCareerPrompts(t *testing.T) {
	provider := &stubProvider{
		results: coupleAlbumProviderResults(8, "childhood-dream"),
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "childhood_dream_owner", "test-password")
	setUserCredits(t, testApp, user.ID, 40)
	child := seedReferenceAsset(t, testApp, user.ID, "child.png", "image/png", []byte("child"))

	body := map[string]any{
		"title":                   "我的六一梦想相册",
		"location":                "childhood_dream_stage",
		"story_template":          "childhood_career_dream",
		"style":                   "children_storybook",
		"male_reference_asset_id": child.ID,
	}
	createResp := performJSONRequest(t, testApp, http.MethodPost, "/api/couple-albums", body, cookies)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("expected childhood dream album create 201, got %d: %s", createResp.Code, createResp.Body.String())
	}
	created := decodeCoupleAlbumTestPayload(t, createResp.Body.Bytes())
	if created.Album.Title != "我的六一梦想相册" || created.Album.Location != "childhood_dream_stage" ||
		created.Album.StoryTemplate != "childhood_career_dream" || created.Album.Style != "children_storybook" ||
		created.Album.MaleReferenceAssetID != child.ID || created.Album.FemaleReferenceAssetID != 0 {
		t.Fatalf("unexpected childhood dream album payload: %+v", created.Album)
	}

	generateResp := performJSONRequest(t, testApp, http.MethodPost, "/api/couple-albums/"+itoa(created.Album.ID)+"/generate", nil, cookies)
	if generateResp.Code != http.StatusAccepted {
		t.Fatalf("expected childhood dream album generate 202, got %d: %s", generateResp.Code, generateResp.Body.String())
	}

	var album coupleAlbumTestPayload
	waitForCondition(t, 3*time.Second, func() bool {
		detailResp := performJSONRequest(t, testApp, http.MethodGet, "/api/couple-albums/"+itoa(created.Album.ID), nil, cookies)
		if detailResp.Code != http.StatusOK {
			return false
		}
		album = decodeCoupleAlbumTestPayload(t, detailResp.Body.Bytes())
		return album.Album.Status == GenerationStatusSucceeded
	})
	if len(album.Album.Pages) != 8 {
		t.Fatalf("expected 8 childhood dream pages, got %d", len(album.Album.Pages))
	}
	expectedTitles := []string{"封面", "小小宇航员", "小小医生", "小小画家", "小小科学家", "小小厨师", "小小运动员", "梦想纪念照"}
	for index, page := range album.Album.Pages {
		if page.PageNumber != index+1 || page.PageTitle != expectedTitles[index] || page.Caption == "" {
			t.Fatalf("unexpected childhood dream page %d: %+v", index+1, page)
		}
	}
	if len(provider.inputs) != 8 {
		t.Fatalf("expected provider to receive 8 childhood dream inputs, got %d", len(provider.inputs))
	}
	for index, input := range provider.inputs {
		if len(input.ReferenceImages) != 1 {
			t.Fatalf("expected page %d to pass one child reference image, got %+v", index+1, input.ReferenceImages)
		}
		if input.ReferenceIntent != GenerationReferenceIntentCharacter {
			t.Fatalf("expected page %d to use character reference intent, got %+v", index+1, input.ReferenceIntent)
		}
		for _, expected := range []string{"以参考照片中的孩子为唯一主角", "保持孩子身份、五官、发型和年龄感一致", "不要文字", "不要水印", "不要logo", "不要多余人物", "不要畸形手"} {
			if !strings.Contains(input.Prompt, expected) {
				t.Fatalf("expected page %d prompt to contain %q, got %q", index+1, expected, input.Prompt)
			}
		}
		if strings.Contains(input.Prompt, "男女主角") || strings.Contains(input.Prompt, "【图2】为女主角参考") {
			t.Fatalf("childhood dream prompt must not contain couple role mapping, got %q", input.Prompt)
		}
		if input.AspectRatio != "3:4" || input.NegativePrompt == "" {
			t.Fatalf("expected page %d to use 3:4 and negative prompt, got aspect=%q negative=%q", index+1, input.AspectRatio, input.NegativePrompt)
		}
	}
}

func TestChildhoodCareerDreamAlbumThemeLocationShapesProviderPrompt(t *testing.T) {
	provider := &stubProvider{
		results: coupleAlbumProviderResults(8, "childhood-space"),
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "childhood_space_owner", "test-password")
	setUserCredits(t, testApp, user.ID, 40)
	child := seedReferenceAsset(t, testApp, user.ID, "child-space.png", "image/png", []byte("child-space"))

	body := map[string]any{
		"title":                     "星际梦想相册",
		"location":                  "childhood_space_adventure",
		"story_template":            "childhood_career_dream",
		"style":                     "children_storybook",
		"male_reference_asset_id":   child.ID,
		"female_reference_asset_id": 0,
	}
	createResp := performJSONRequest(t, testApp, http.MethodPost, "/api/couple-albums", body, cookies)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("expected childhood space album create 201, got %d: %s", createResp.Code, createResp.Body.String())
	}
	created := decodeCoupleAlbumTestPayload(t, createResp.Body.Bytes())
	if created.Album.Location != "childhood_space_adventure" || created.Album.StoryTemplate != "childhood_career_dream" {
		t.Fatalf("unexpected themed childhood album payload: %+v", created.Album)
	}

	generateResp := performJSONRequest(t, testApp, http.MethodPost, "/api/couple-albums/"+itoa(created.Album.ID)+"/generate", nil, cookies)
	if generateResp.Code != http.StatusAccepted {
		t.Fatalf("expected childhood space album generate 202, got %d: %s", generateResp.Code, generateResp.Body.String())
	}

	waitForCondition(t, 3*time.Second, func() bool {
		detailResp := performJSONRequest(t, testApp, http.MethodGet, "/api/couple-albums/"+itoa(created.Album.ID), nil, cookies)
		if detailResp.Code != http.StatusOK {
			return false
		}
		album := decodeCoupleAlbumTestPayload(t, detailResp.Body.Bytes())
		return album.Album.Status == GenerationStatusSucceeded
	})
	if len(provider.inputs) != 8 {
		t.Fatalf("expected provider to receive 8 childhood dream inputs, got %d", len(provider.inputs))
	}
	for index, input := range provider.inputs {
		for _, expected := range []string{"星际探索之旅", "宇宙飞船", "星球", "探索冒险"} {
			if !strings.Contains(input.Prompt, expected) {
				t.Fatalf("expected page %d prompt to contain theme phrase %q, got %q", index+1, expected, input.Prompt)
			}
		}
	}
}

func TestCoupleAlbumRetryFailedPage(t *testing.T) {
	provider := &stubProvider{
		errs: []*ProviderError{{
			Code:         "provider_asset_fetch_failed",
			Message:      "temporary fetch failed",
			FailureStage: providerFailureStageProviderAssetFetch,
		}},
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString([]byte("retry-ok")),
			MIMEType:          "image/png",
			ProviderRequestID: "req_retry_ok",
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "album_retry", "test-password")
	setUserCredits(t, testApp, user.ID, 40)
	male := seedReferenceAsset(t, testApp, user.ID, "male.png", "image/png", []byte("male"))
	female := seedReferenceAsset(t, testApp, user.ID, "female.png", "image/png", []byte("female"))

	createResp := performJSONRequest(t, testApp, http.MethodPost, "/api/couple-albums", coupleAlbumCreateBody(male.ID, female.ID), cookies)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("expected album create 201, got %d: %s", createResp.Code, createResp.Body.String())
	}
	created := decodeCoupleAlbumTestPayload(t, createResp.Body.Bytes())
	generateResp := performJSONRequest(t, testApp, http.MethodPost, "/api/couple-albums/"+itoa(created.Album.ID)+"/generate", nil, cookies)
	if generateResp.Code != http.StatusAccepted {
		t.Fatalf("expected album generate 202, got %d: %s", generateResp.Code, generateResp.Body.String())
	}

	var failedAlbum coupleAlbumTestPayload
	var failedPageID uint
	waitForCondition(t, 3*time.Second, func() bool {
		detailResp := performJSONRequest(t, testApp, http.MethodGet, "/api/couple-albums/"+itoa(created.Album.ID), nil, cookies)
		if detailResp.Code != http.StatusOK {
			return false
		}
		failedAlbum = decodeCoupleAlbumTestPayload(t, detailResp.Body.Bytes())
		failedPageID = 0
		for _, page := range failedAlbum.Album.Pages {
			if page.Status == GenerationStatusFailed {
				failedPageID = page.ID
			}
		}
		return failedPageID != 0 && failedAlbum.Album.Status == "partial_failed"
	})
	if failedPageID == 0 || failedAlbum.Album.Status != "partial_failed" {
		t.Fatalf("expected one failed page and partial_failed album, got album=%+v failed_page=%d", failedAlbum.Album, failedPageID)
	}

	retryResp := performJSONRequest(t, testApp, http.MethodPost, "/api/couple-albums/"+itoa(created.Album.ID)+"/pages/"+itoa(failedPageID)+"/retry", nil, cookies)
	if retryResp.Code != http.StatusAccepted {
		t.Fatalf("expected retry 202, got %d: %s", retryResp.Code, retryResp.Body.String())
	}

	var completed coupleAlbumTestPayload
	waitForCondition(t, 3*time.Second, func() bool {
		detailResp := performJSONRequest(t, testApp, http.MethodGet, "/api/couple-albums/"+itoa(created.Album.ID), nil, cookies)
		if detailResp.Code != http.StatusOK {
			return false
		}
		completed = decodeCoupleAlbumTestPayload(t, detailResp.Body.Bytes())
		return completed.Album.Status == GenerationStatusSucceeded
	})
	for _, page := range completed.Album.Pages {
		if page.Status != GenerationStatusSucceeded || page.WorkID == 0 {
			t.Fatalf("expected retried album page to succeed, got %+v", page)
		}
	}
	if len(provider.inputs) != 9 {
		t.Fatalf("expected initial 8 page attempts plus one retry input, got %d", len(provider.inputs))
	}
	retryInput := provider.inputs[len(provider.inputs)-1]
	if retryInput.ReferenceIntent != GenerationReferenceIntentCharacter {
		t.Fatalf("expected retry to use character reference intent, got %+v", retryInput.ReferenceIntent)
	}
	if len(retryInput.ReferenceImages) != 2 {
		t.Fatalf("expected retry to pass two reference images, got %+v", retryInput.ReferenceImages)
	}
	if retryInput.CompositionPlan != nil || retryInput.BackgroundReferenceIndex != nil {
		t.Fatalf("expected retry to avoid compose planning, got plan=%+v background=%+v", retryInput.CompositionPlan, retryInput.BackgroundReferenceIndex)
	}
}

func coupleAlbumCreateBody(maleID, femaleID uint) map[string]any {
	return map[string]any{
		"title":                     "我们在大理的 520",
		"location":                  "大理",
		"story_template":            "city_walk",
		"style":                     "film",
		"male_reference_asset_id":   maleID,
		"female_reference_asset_id": femaleID,
	}
}

func coupleAlbumProviderResults(count int, prefix string) []ImageGenerationResult {
	results := make([]ImageGenerationResult, 0, count)
	for i := 0; i < count; i++ {
		results = append(results, ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString([]byte(prefix + "-" + itoa(uint(i+1)))),
			MIMEType:          "image/png",
			ProviderRequestID: "req_" + prefix + "_" + itoa(uint(i+1)),
		})
	}
	return results
}

func decodeCoupleAlbumTestPayload(t *testing.T, body []byte) coupleAlbumTestPayload {
	t.Helper()
	var payload coupleAlbumTestPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode couple album payload: %v\n%s", err, string(body))
	}
	return payload
}
