package app

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"testing"
)

func TestWorkspaceDiscoveryExposesExpandPercentSchema(t *testing.T) {
	tools := workspaceDiscoveryTools()
	var expand *workspaceTool
	for index := range tools {
		if tools[index].Mode == GenerationToolModeExpand {
			expand = &tools[index]
			break
		}
	}
	if expand == nil {
		t.Fatalf("expected expand tool in workspace discovery")
	}
	if !expand.RequiresSource || expand.SourceLimit != 1 {
		t.Fatalf("expected expand to declare one required source, got requires_source=%v source_limit=%d", expand.RequiresSource, expand.SourceLimit)
	}
	for _, field := range expand.FormSchema {
		if field.Type != "number" {
			t.Fatalf("expected numeric expand field, got %+v", field)
		}
		if field.Default != 20 || field.Min == nil || *field.Min != 0 || field.Max == nil || *field.Max != 100 || field.Step != 5 {
			t.Fatalf("expected percent expand field defaults/range, got %+v", field)
		}
	}
}

func TestExpandGenerationBuildsDerivedTransparentCanvasInput(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString(testPNGBytes(t, 15, 9, color.RGBA{R: 20, G: 180, B: 70, A: 255})),
			MIMEType:          "image/png",
			ProviderRequestID: "req_expand",
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_expand_canvas", "test-password")
	setUserCredits(t, testApp, user.ID, 4)
	source := seedReferenceAsset(t, testApp, user.ID, "expand-source.png", "image/png", testPNGBytes(t, 10, 6, color.RGBA{R: 220, G: 48, B: 48, A: 255}))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations", map[string]any{
		"prompt":              "自然延展森林边界",
		"aspect_ratio":        "1:1",
		"tool_mode":           GenerationToolModeExpand,
		"tool_options":        map[string]any{"unit": "percent", "top": 50, "bottom": 0, "left": 20, "right": 30},
		"reference_asset_ids": []uint{source.ID},
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected expand generation 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if provider.calls != 1 || len(provider.inputs) != 1 {
		t.Fatalf("expected provider called once, got calls=%d inputs=%d", provider.calls, len(provider.inputs))
	}
	input := provider.inputs[0]
	if input.ToolMode != GenerationToolModeExpand || input.SourceImage == nil || input.SourceImage.MIMEType != "image/png" {
		t.Fatalf("expected expand source image input, got %+v", input)
	}
	if input.MaskImage == nil || input.MaskImage.MIMEType != "image/png" {
		t.Fatalf("expected expand mask image input, got %+v", input.MaskImage)
	}
	if len(input.ReferenceImages) != 0 {
		t.Fatalf("expected promoted reference source to be consumed, got %d reference images", len(input.ReferenceImages))
	}
	derived := decodePNGFromBase64(t, input.SourceImage.Base64Data)
	if got, want := derived.Bounds().Dx(), 15; got != want {
		t.Fatalf("expected expanded width %d, got %d", want, got)
	}
	if got, want := derived.Bounds().Dy(), 9; got != want {
		t.Fatalf("expected expanded height %d, got %d", want, got)
	}
	if _, _, _, alpha := derived.At(0, 0).RGBA(); alpha != 0 {
		t.Fatalf("expected outer expanded pixel to be transparent")
	}
	r, g, b, a := derived.At(2, 3).RGBA()
	if r>>8 != 220 || g>>8 != 48 || b>>8 != 48 || a>>8 != 255 {
		t.Fatalf("expected original image copied at offset, got rgba=(%d,%d,%d,%d)", r>>8, g>>8, b>>8, a>>8)
	}
	mask := decodePNGFromBase64(t, input.MaskImage.Base64Data)
	if got, want := mask.Bounds().Dx(), 15; got != want {
		t.Fatalf("expected mask width %d, got %d", want, got)
	}
	if got, want := mask.Bounds().Dy(), 9; got != want {
		t.Fatalf("expected mask height %d, got %d", want, got)
	}
	if _, _, _, alpha := mask.At(0, 0).RGBA(); alpha != 0 {
		t.Fatalf("expected expand mask outer pixel to be transparent")
	}
	if _, _, _, alpha := mask.At(2, 3).RGBA(); alpha>>8 != 255 {
		t.Fatalf("expected expand mask original area to be opaque, got alpha=%d", alpha>>8)
	}
	if input.Size != "1536x1024" {
		t.Fatalf("expected provider size from expanded canvas ratio, got %q", input.Size)
	}
	if !bytes.Contains([]byte(input.Prompt), []byte("透明外扩区域")) || !bytes.Contains([]byte(input.Prompt), []byte("不要裁切、缩放或重绘原图主体")) {
		t.Fatalf("expected expand prompt constraints, got %q", input.Prompt)
	}

	var payload struct {
		Parameters struct {
			ToolOptions map[string]any `json:"tool_options"`
		} `json:"parameters"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Parameters.ToolOptions["unit"] != "percent" || int(payload.Parameters.ToolOptions["top"].(float64)) != 50 {
		t.Fatalf("expected normalized expand options in response, got %+v", payload.Parameters.ToolOptions)
	}
}

func TestExpandGenerationPreservesOriginalPixelsInPersistedResult(t *testing.T) {
	providerResult := image.NewRGBA(image.Rect(0, 0, 30, 18))
	for y := 0; y < providerResult.Bounds().Dy(); y++ {
		for x := 0; x < providerResult.Bounds().Dx(); x++ {
			providerResult.SetRGBA(x, y, color.RGBA{R: 20, G: 180, B: 70, A: 255})
		}
	}
	for y := 6; y < 18; y++ {
		for x := 4; x < 24; x++ {
			providerResult.SetRGBA(x, y, color.RGBA{R: 250, G: 220, B: 30, A: 255})
		}
	}
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString(encodePNGImage(t, providerResult)),
			MIMEType:          "image/png",
			ProviderRequestID: "req_expand_preserve",
		},
	}
	testApp, db := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_expand_preserve", "test-password")
	setUserCredits(t, testApp, user.ID, 4)
	source := seedReferenceAsset(t, testApp, user.ID, "expand-source.png", "image/png", testPNGBytes(t, 10, 6, color.RGBA{R: 220, G: 48, B: 48, A: 255}))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations", map[string]any{
		"prompt":              "自然延展森林边界",
		"aspect_ratio":        "1:1",
		"tool_mode":           GenerationToolModeExpand,
		"tool_options":        map[string]any{"unit": "percent", "top": 50, "bottom": 0, "left": 20, "right": 30},
		"reference_asset_ids": []uint{source.ID},
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected expand generation 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var work Work
	if err := db.Where("user_id = ? AND status = ?", user.ID, GenerationStatusSucceeded).Order("id DESC").First(&work).Error; err != nil {
		t.Fatalf("load generated work: %v", err)
	}
	content, err := testApp.assetStore.Read(work.AssetKey)
	if err != nil {
		t.Fatalf("read persisted work asset: %v", err)
	}
	persisted, err := png.Decode(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("decode persisted PNG: %v", err)
	}

	if got, want := persisted.Bounds().Dx(), 30; got != want {
		t.Fatalf("expected provider result width %d, got %d", want, got)
	}
	if got, want := persisted.Bounds().Dy(), 18; got != want {
		t.Fatalf("expected provider result height %d, got %d", want, got)
	}
	assertRGBAAt(t, persisted, 12, 10, color.RGBA{R: 220, G: 48, B: 48, A: 255})
	assertRGBAAt(t, persisted, 1, 1, color.RGBA{R: 20, G: 180, B: 70, A: 255})
}

func TestExpandGenerationFailsWhenResultCannotBePreserved(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString([]byte("not-a-png")),
			MIMEType:          "image/png",
			ProviderRequestID: "req_expand_invalid_result",
		},
	}
	testApp, db := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_expand_invalid_result", "test-password")
	setUserCredits(t, testApp, user.ID, 4)
	source := seedReferenceAsset(t, testApp, user.ID, "expand-source.png", "image/png", testPNGBytes(t, 10, 6, color.RGBA{R: 220, G: 48, B: 48, A: 255}))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations", map[string]any{
		"prompt":              "自然延展森林边界",
		"aspect_ratio":        "1:1",
		"tool_mode":           GenerationToolModeExpand,
		"tool_options":        map[string]any{"unit": "percent", "top": 50, "bottom": 0, "left": 20, "right": 30},
		"reference_asset_ids": []uint{source.ID},
	}, cookies)
	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected expand generation failure, got %d: %s", resp.Code, resp.Body.String())
	}

	var record GenerationRecord
	if err := db.Where("user_id = ?", user.ID).Order("id DESC").First(&record).Error; err != nil {
		t.Fatalf("load failed generation record: %v", err)
	}
	if record.Status != GenerationStatusFailed || record.ErrorCode != "expand_result_preserve_failed" {
		t.Fatalf("expected expand_result_preserve_failed record, got status=%q code=%q", record.Status, record.ErrorCode)
	}
}

func TestExpandGenerationRejectsInvalidSourcesAndEdges(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "creator_expand_invalid", "test-password")
	setUserCredits(t, testApp, user.ID, 10)
	first := seedReferenceAsset(t, testApp, user.ID, "expand-first.png", "image/png", testPNGBytes(t, 4, 4, color.RGBA{R: 1, G: 2, B: 3, A: 255}))
	second := seedReferenceAsset(t, testApp, user.ID, "expand-second.png", "image/png", testPNGBytes(t, 4, 4, color.RGBA{R: 4, G: 5, B: 6, A: 255}))

	cases := []struct {
		name string
		body map[string]any
	}{
		{
			name: "all zero edges",
			body: map[string]any{
				"prompt":              "扩图",
				"aspect_ratio":        "1:1",
				"tool_mode":           GenerationToolModeExpand,
				"tool_options":        map[string]any{"unit": "percent", "top": 0, "bottom": 0, "left": 0, "right": 0},
				"reference_asset_ids": []uint{first.ID},
			},
		},
		{
			name: "edge over percent range",
			body: map[string]any{
				"prompt":              "扩图",
				"aspect_ratio":        "1:1",
				"tool_mode":           GenerationToolModeExpand,
				"tool_options":        map[string]any{"unit": "percent", "top": 101, "bottom": 0, "left": 0, "right": 0},
				"reference_asset_ids": []uint{first.ID},
			},
		},
		{
			name: "missing source",
			body: map[string]any{
				"prompt":       "扩图",
				"aspect_ratio": "1:1",
				"tool_mode":    GenerationToolModeExpand,
				"tool_options": map[string]any{"unit": "percent", "top": 20, "bottom": 0, "left": 0, "right": 0},
			},
		},
		{
			name: "multiple reference sources",
			body: map[string]any{
				"prompt":              "扩图",
				"aspect_ratio":        "1:1",
				"tool_mode":           GenerationToolModeExpand,
				"tool_options":        map[string]any{"unit": "percent", "top": 20, "bottom": 0, "left": 0, "right": 0},
				"reference_asset_ids": []uint{first.ID, second.ID},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", tc.body, cookies)
			if resp.Code != http.StatusBadRequest {
				t.Fatalf("expected bad request, got %d: %s", resp.Code, resp.Body.String())
			}
			if !bytes.Contains(resp.Body.Bytes(), []byte(`"code":"invalid_generation_parameter"`)) {
				t.Fatalf("expected invalid_generation_parameter payload, got %s", resp.Body.String())
			}
		})
	}
}

func testPNGBytes(t *testing.T, width, height int, fill color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, fill)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func decodePNGFromBase64(t *testing.T, value string) image.Image {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		t.Fatalf("decode base64: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	return img
}

func encodePNGImage(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func assertRGBAAt(t *testing.T, img image.Image, x, y int, want color.RGBA) {
	t.Helper()
	r, g, b, a := img.At(x, y).RGBA()
	got := color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
	if got != want {
		t.Fatalf("expected pixel at (%d,%d) to be %#v, got %#v", x, y, want, got)
	}
}
