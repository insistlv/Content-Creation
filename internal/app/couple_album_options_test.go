package app

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

type coupleAlbumOptionsTestPayload struct {
	Locations      []CoupleAlbumOption `json:"locations"`
	StoryTemplates []CoupleAlbumOption `json:"story_templates"`
	Styles         []CoupleAlbumOption `json:"styles"`
}

func TestCoupleAlbumOptionsSeedAndPublicAPI(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})

	var count int64
	if err := db.Model(&CoupleAlbumOption{}).Count(&count).Error; err != nil {
		t.Fatalf("count couple album options: %v", err)
	}
	if count < 22 {
		t.Fatalf("expected seeded couple album options, got %d", count)
	}

	_, cookies := createLoggedInUser(t, testApp, "album_options_user", "test-password")
	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/couple-album/options", nil, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected public options 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload coupleAlbumOptionsTestPayload
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode options payload: %v", err)
	}
	if len(payload.Locations) != 9 || len(payload.StoryTemplates) != 5 || len(payload.Styles) != 8 {
		t.Fatalf("unexpected options groups: %+v", payload)
	}
	for _, value := range []string{"childhood_dream_stage", "childhood_space_adventure", "childhood_fairy_tale", "childhood_nature_explorer"} {
		if !coupleAlbumOptionExists(payload.Locations, CoupleAlbumOptionTypeLocation, value) {
			t.Fatalf("expected childhood location %q, got %+v", value, payload.Locations)
		}
	}
	if !coupleAlbumOptionExists(payload.StoryTemplates, CoupleAlbumOptionTypeStoryTemplate, "childhood_career_dream") {
		t.Fatalf("expected childhood career dream story template, got %+v", payload.StoryTemplates)
	}
	for _, value := range []string{"children_storybook", "dreamy_watercolor", "animation_3d", "children_photo_poster"} {
		if !coupleAlbumOptionExists(payload.Styles, CoupleAlbumOptionTypeStyle, value) {
			t.Fatalf("expected childhood style %q in %+v", value, payload.Styles)
		}
	}
	if payload.Locations[0].Type != CoupleAlbumOptionTypeLocation || payload.Locations[0].Value != "大理" ||
		payload.Locations[0].Label != "大理洱海" || payload.Locations[0].ImageURL == "" || !payload.Locations[0].IsActive {
		t.Fatalf("unexpected first location option: %+v", payload.Locations[0])
	}
	if payload.StoryTemplates[0].Type != CoupleAlbumOptionTypeStoryTemplate || payload.StoryTemplates[0].Value != "city_walk" ||
		payload.StoryTemplates[0].IconURL == "" {
		t.Fatalf("unexpected first story template: %+v", payload.StoryTemplates[0])
	}
	if payload.Styles[0].Type != CoupleAlbumOptionTypeStyle || payload.Styles[0].Value != "film" ||
		payload.Styles[0].IconURL == "" {
		t.Fatalf("unexpected first style: %+v", payload.Styles[0])
	}
}

func TestAdminCoupleAlbumOptionsCRUDAndFiltering(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)

	listResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/couple-album-options?type=location", nil, adminCookies)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected admin options list 200, got %d: %s", listResp.Code, listResp.Body.String())
	}
	var listPayload struct {
		Items []CoupleAlbumOption `json:"items"`
		Total int64               `json:"total"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode admin list: %v", err)
	}
	if listPayload.Total != 9 || len(listPayload.Items) != 9 {
		t.Fatalf("expected nine seeded locations, got %+v", listPayload)
	}

	createResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/couple-album-options", map[string]any{
		"type":         CoupleAlbumOptionTypeLocation,
		"value":        "杭州",
		"label":        "杭州西湖",
		"description":  "湖光山色里的慢旅行",
		"image_url":    "/static/couple-album/hangzhou-westlake.png",
		"prompt_label": "杭州西湖",
		"sort_order":   60,
		"is_active":    true,
	}, adminCookies)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("expected create option 201, got %d: %s", createResp.Code, createResp.Body.String())
	}
	var created CoupleAlbumOption
	if err := json.Unmarshal(createResp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created option: %v", err)
	}
	if created.ID == 0 || created.Value != "杭州" || created.Label != "杭州西湖" || !created.IsActive {
		t.Fatalf("unexpected created option: %+v", created)
	}

	updateResp := performJSONRequest(t, testApp, http.MethodPut, "/api/admin/couple-album-options/"+itoa(created.ID), map[string]any{
		"type":         CoupleAlbumOptionTypeLocation,
		"value":        "杭州",
		"label":        "杭州西湖旅拍",
		"description":  "西湖边的电影感午后",
		"image_url":    "/static/couple-album/hangzhou-westlake.png",
		"prompt_label": "杭州西湖",
		"sort_order":   12,
		"is_active":    false,
	}, adminCookies)
	if updateResp.Code != http.StatusOK {
		t.Fatalf("expected update option 200, got %d: %s", updateResp.Code, updateResp.Body.String())
	}
	var updated CoupleAlbumOption
	if err := json.Unmarshal(updateResp.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated option: %v", err)
	}
	if updated.Label != "杭州西湖旅拍" || updated.SortOrder != 12 || updated.IsActive {
		t.Fatalf("unexpected updated option: %+v", updated)
	}

	publicResp := performJSONRequest(t, testApp, http.MethodGet, "/api/couple-album/options", nil, createUserSessionForOptionsTest(t, testApp))
	if publicResp.Code != http.StatusOK {
		t.Fatalf("expected public options after disable 200, got %d: %s", publicResp.Code, publicResp.Body.String())
	}
	if strings.Contains(publicResp.Body.String(), "杭州西湖旅拍") {
		t.Fatalf("disabled options must be hidden from public payload: %s", publicResp.Body.String())
	}

	deleteResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/admin/couple-album-options/"+itoa(created.ID), nil, adminCookies)
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("expected delete option 200, got %d: %s", deleteResp.Code, deleteResp.Body.String())
	}
	var remaining int64
	if err := db.Model(&CoupleAlbumOption{}).Where("id = ?", created.ID).Count(&remaining).Error; err != nil {
		t.Fatalf("count deleted option: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("expected option to be deleted, still found %d", remaining)
	}
}

func coupleAlbumOptionExists(options []CoupleAlbumOption, optionType, value string) bool {
	for _, option := range options {
		if option.Type == optionType && option.Value == value && option.IsActive {
			return true
		}
	}
	return false
}

func TestCoupleAlbumCreateRequiresActiveConfiguredOptions(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "album_option_validation", "test-password")
	male := seedReferenceAsset(t, testApp, user.ID, "male.png", "image/png", []byte("male"))
	female := seedReferenceAsset(t, testApp, user.ID, "female.png", "image/png", []byte("female"))

	body := coupleAlbumCreateBody(male.ID, female.ID)
	body["location"] = "不存在的地点"
	missingResp := performJSONRequest(t, testApp, http.MethodPost, "/api/couple-albums", body, cookies)
	if missingResp.Code != http.StatusUnprocessableEntity || !strings.Contains(missingResp.Body.String(), "album_location_invalid") {
		t.Fatalf("expected invalid location 422, got %d: %s", missingResp.Code, missingResp.Body.String())
	}

	if err := db.Model(&CoupleAlbumOption{}).
		Where("type = ? AND value = ?", CoupleAlbumOptionTypeStyle, "film").
		Update("is_active", false).Error; err != nil {
		t.Fatalf("disable style option: %v", err)
	}
	body = coupleAlbumCreateBody(male.ID, female.ID)
	inactiveResp := performJSONRequest(t, testApp, http.MethodPost, "/api/couple-albums", body, cookies)
	if inactiveResp.Code != http.StatusUnprocessableEntity || !strings.Contains(inactiveResp.Body.String(), "album_style_invalid") {
		t.Fatalf("expected inactive style 422, got %d: %s", inactiveResp.Code, inactiveResp.Body.String())
	}
}

func TestAdminCoupleAlbumOptionAssetUploadUsesAssetStorePublicURL(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	testApp.assetStore = &publicURLAssetStore{
		key:       "assets/2026/05/couple-location.png",
		publicURL: "https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/2026/05/couple-location.png",
	}

	adminCookies := createAdminSession(t, testApp)
	uploadResp := performMultipartRequest(t, testApp, http.MethodPost, "/api/admin/couple-album-options/assets", "file", "location.png", mustBase64Decode(t, fakePNGBase64), adminCookies)
	if uploadResp.Code != http.StatusCreated {
		t.Fatalf("expected option asset upload 201, got %d: %s", uploadResp.Code, uploadResp.Body.String())
	}

	var payload struct {
		URL      string `json:"url"`
		AssetKey string `json:"asset_key"`
		MIMEType string `json:"mime_type"`
	}
	if err := json.Unmarshal(uploadResp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode option asset upload payload: %v", err)
	}
	if payload.URL != "https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/2026/05/couple-location.png" ||
		payload.AssetKey != "assets/2026/05/couple-location.png" ||
		payload.MIMEType != "image/png" {
		t.Fatalf("unexpected option asset upload payload: %+v", payload)
	}
}

func createUserSessionForOptionsTest(t *testing.T, app *App) []*http.Cookie {
	t.Helper()
	_, cookies := createLoggedInUser(t, app, "album_options_reader", "test-password")
	return cookies
}
