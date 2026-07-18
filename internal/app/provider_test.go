package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOpenAIProviderGenerateUsesImageModelForPlainImageGeneration(t *testing.T) {
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Fatalf("expected request path /v1/images/generations, got %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer model-key" {
			t.Fatalf("expected model-specific bearer token, got %q", auth)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("decode request payload: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-request-id", "req-image-2")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"ZmFrZQ=="}]}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{
		OpenAIBaseURL: "https://wrong.example",
		OpenAIAPIKey:  "global-key",
	})
	provider.client = server.Client()

	result, providerErr := provider.Generate(context.Background(), ImageGenerationInput{
		Model:               "gpt-image-2",
		Prompt:              "tiny blue bird",
		AspectRatio:         "21:9",
		Size:                "1536x1024",
		ProviderBaseURL:     server.URL,
		ProviderAPIKey:      "model-key",
		ProviderAPIEndpoint: "/v1/images/generations",
	})
	if providerErr != nil {
		t.Fatalf("Generate() provider error = %+v", providerErr)
	}
	if result.Base64Image != "ZmFrZQ==" || result.ProviderRequestID != "req-image-2" {
		t.Fatalf("unexpected image generation result: %+v", result)
	}
	if payload["model"] != "gpt-image-2" {
		t.Fatalf("expected model gpt-image-2, got %#v", payload["model"])
	}
	if payload["prompt"] != "tiny blue bird" {
		t.Fatalf("expected prompt in images payload, got %#v", payload["prompt"])
	}
	if payload["aspect_ratio"] != "21:9" {
		t.Fatalf("expected aspect_ratio 21:9, got %#v", payload["aspect_ratio"])
	}
	if _, exists := payload["image"]; exists {
		t.Fatalf("did not expect reference image payload for plain generation, got %#v", payload["image"])
	}
}

func TestNewOpenAIProviderUsesIsolatedHTTPTransport(t *testing.T) {
	provider := NewOpenAIProvider(Config{
		OpenAIBaseURL: "https://provider.example",
		OpenAIAPIKey:  "test-key",
	})
	transport, ok := provider.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected explicit HTTP transport, got %#v", provider.client.Transport)
	}
	if !transport.DisableKeepAlives {
		t.Fatalf("expected provider HTTP client to disable keep-alive connection reuse")
	}
}

func TestOpenAIProviderGenerateUsesResponsesPayloadForReferenceImages(t *testing.T) {
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("expected request path /v1/responses, got %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			t.Fatalf("expected bearer token, got %q", auth)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("decode request payload: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":[{"type":"image_generation_call","result":"ZmFrZQ=="}]}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{
		OpenAIBaseURL: server.URL,
		OpenAIAPIKey:  "test-key",
	})
	provider.client = server.Client()

	result, providerErr := provider.Generate(context.Background(), ImageGenerationInput{
		Model:               "gpt-image-2",
		Prompt:              "tiny blue bird",
		Size:                "1024x1024",
		ProviderAPIEndpoint: "responses",
		ReferenceImages: []ReferenceImageInput{
			{MIMEType: "image/png", Base64Data: "aW1hZ2UtMQ=="},
			{MIMEType: "image/jpeg", Base64Data: "aW1hZ2UtMg=="},
		},
	})
	if providerErr != nil {
		t.Fatalf("Generate() provider error = %+v", providerErr)
	}
	if result.Base64Image != "ZmFrZQ==" {
		t.Fatalf("expected base64 image from provider response, got %q", result.Base64Image)
	}

	if payload["model"] != "gpt-image-2" {
		t.Fatalf("expected selected image model gpt-image-2, got %#v", payload["model"])
	}
	if payload["tool_choice"] == nil {
		t.Fatalf("expected tool_choice to force image_generation")
	}

	tools, ok := payload["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("expected one tool payload, got %#v", payload["tools"])
	}
	tool, ok := tools[0].(map[string]any)
	if !ok {
		t.Fatalf("expected tool object, got %#v", tools[0])
	}
	if tool["type"] != "image_generation" {
		t.Fatalf("expected image_generation tool, got %#v", tool["type"])
	}
	if tool["size"] != "1024x1024" {
		t.Fatalf("expected tool size 1024x1024, got %#v", tool["size"])
	}
	if tool["quality"] != "medium" {
		t.Fatalf("expected quality medium, got %#v", tool["quality"])
	}
	if tool["output_format"] != "png" {
		t.Fatalf("expected output_format png, got %#v", tool["output_format"])
	}
	if tool["action"] != "generate" {
		t.Fatalf("expected action generate, got %#v", tool["action"])
	}

	inputs, ok := payload["input"].([]any)
	if !ok || len(inputs) != 1 {
		t.Fatalf("expected single input message, got %#v", payload["input"])
	}
	message, ok := inputs[0].(map[string]any)
	if !ok {
		t.Fatalf("expected input message object, got %#v", inputs[0])
	}
	content, ok := message["content"].([]any)
	if !ok || len(content) != 3 {
		t.Fatalf("expected prompt plus two images, got %#v", message["content"])
	}
	first, ok := content[0].(map[string]any)
	firstText, _ := first["text"].(string)
	if !ok || first["type"] != "input_text" || !strings.Contains(firstText, "tiny blue bird") || !strings.Contains(firstText, "上传图片是本次生成的主要视觉参考") {
		t.Fatalf("expected input_text prompt, got %#v", content[0])
	}
	second, ok := content[1].(map[string]any)
	if !ok || second["type"] != "input_image" || second["image_url"] != "data:image/png;base64,aW1hZ2UtMQ==" {
		t.Fatalf("expected first input_image data URL, got %#v", content[1])
	}
	third, ok := content[2].(map[string]any)
	if !ok || third["type"] != "input_image" || third["image_url"] != "data:image/jpeg;base64,aW1hZ2UtMg==" {
		t.Fatalf("expected second input_image data URL, got %#v", content[2])
	}
}

func TestOpenAIProviderGenerateUsesResponsesPayloadForComposeReferenceImages(t *testing.T) {
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("expected compose multi-reference generation to use responses endpoint, got %s", r.URL.Path)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("decode request payload: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":[{"type":"image_generation_call","result":"ZmFrZQ=="}]}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{
		OpenAIBaseURL: server.URL,
		OpenAIAPIKey:  "test-key",
	})
	provider.client = server.Client()

	_, providerErr := provider.Generate(context.Background(), ImageGenerationInput{
		Model:               "gpt-image-2",
		Prompt:              "把两位人物合成到第二张图的场景中",
		NegativePrompt:      "不要文字、不要水印",
		StylePreset:         "电影感",
		ToolMode:            GenerationToolModeGenerate,
		StyleStrength:       80,
		ReferenceWeight:     90,
		Seed:                "seed-compose",
		VariationMode:       "balanced",
		VariationPrompt:     "改成热闹街景并增加路人",
		ProviderAPIEndpoint: "responses",
		ReferenceIntent:     GenerationReferenceIntentCompose,
		ReferenceImages: []ReferenceImageInput{
			{MIMEType: "image/png", Base64Data: "aW1hZ2UtMQ=="},
			{MIMEType: "image/jpeg", Base64Data: "aW1hZ2UtMg=="},
		},
	})
	if providerErr != nil {
		t.Fatalf("Generate() provider error = %+v", providerErr)
	}

	inputs, ok := payload["input"].([]any)
	if !ok || len(inputs) != 1 {
		t.Fatalf("expected one responses input item, got %#v", payload["input"])
	}
	message := inputs[0].(map[string]any)
	content, ok := message["content"].([]any)
	if !ok || len(content) != 3 {
		t.Fatalf("expected prompt plus two independent reference images, got %#v", message["content"])
	}
	promptItem := content[0].(map[string]any)
	promptText, ok := promptItem["text"].(string)
	if !ok {
		t.Fatalf("expected compose prompt text, got %#v", promptItem)
	}
	for _, expected := range []string{"把两位人物合成到第二张图的场景中", "反向提示词：不要文字、不要水印", "【图1】", "【图2】", "背景/场景严格取【图2】", "只保留参考图中已经出现的人物", "不要新增人物"} {
		if !strings.Contains(promptText, expected) {
			t.Fatalf("expected compose prompt to contain %q, got %q", expected, promptText)
		}
	}
	for _, unexpected := range []string{"风格预设", "风格强度", "相似度/参考权重", "变化模式", "本张变化方向", "seed-compose", "改成热闹街景并增加路人"} {
		if strings.Contains(promptText, unexpected) {
			t.Fatalf("did not expect creative metadata %q in compose prompt: %q", unexpected, promptText)
		}
	}
	firstImage := content[1].(map[string]any)
	secondImage := content[2].(map[string]any)
	if firstImage["image_url"] != "data:image/png;base64,aW1hZ2UtMQ==" || secondImage["image_url"] != "data:image/jpeg;base64,aW1hZ2UtMg==" {
		t.Fatalf("expected independent reference images in request order, got %#v", content)
	}
}

func TestOpenAIProviderGenerateUsesResponsesPayloadForCharacterReferenceImages(t *testing.T) {
	firstReference := testPNGBase64(t, color.RGBA{R: 240, A: 255})
	secondReference := testPNGBase64(t, color.RGBA{G: 220, A: 255})

	var requestedPath string
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("decode request payload: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/responses" {
			_, _ = w.Write([]byte(`{"output":[{"type":"image_generation_call","result":"ZmFrZQ=="}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"ZmFrZQ=="}]}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{
		OpenAIBaseURL: server.URL,
		OpenAIAPIKey:  "test-key",
	})
	provider.client = server.Client()

	_, providerErr := provider.Generate(context.Background(), ImageGenerationInput{
		Model:               "gpt-image-2",
		Prompt:              "生成情侣旅行相册页",
		Size:                "1024x1536",
		ProviderAPIEndpoint: "responses",
		ReferenceIntent:     GenerationReferenceIntentCharacter,
		ReferenceImages: []ReferenceImageInput{
			{MIMEType: "image/png", Base64Data: firstReference},
			{MIMEType: "image/png", Base64Data: secondReference},
		},
	})
	if providerErr != nil {
		t.Fatalf("Generate() provider error = %+v", providerErr)
	}
	if requestedPath != "/v1/responses" {
		t.Fatalf("expected character multi-reference generation to use responses endpoint, got %s", requestedPath)
	}

	inputs, ok := payload["input"].([]any)
	if !ok || len(inputs) != 1 {
		t.Fatalf("expected one responses input item, got %#v", payload["input"])
	}
	message := inputs[0].(map[string]any)
	content, ok := message["content"].([]any)
	if !ok || len(content) != 3 {
		t.Fatalf("expected prompt plus two independent reference images, got %#v", message["content"])
	}
	promptItem := content[0].(map[string]any)
	promptText, ok := promptItem["text"].(string)
	if !ok {
		t.Fatalf("expected character prompt text, got %#v", promptItem)
	}
	for _, expected := range []string{"【图1】", "【图2】", "人物身份", "五官", "发型", "气质", "不要新增人物", "不要互换角色"} {
		if !strings.Contains(promptText, expected) {
			t.Fatalf("expected character prompt to contain %q, got %q", expected, promptText)
		}
	}
	if strings.Contains(promptText, "背景/场景严格取【图2】") {
		t.Fatalf("did not expect character prompt to force a reference background, got %q", promptText)
	}
	firstImage := content[1].(map[string]any)
	secondImage := content[2].(map[string]any)
	if firstImage["type"] != "input_image" || secondImage["type"] != "input_image" {
		t.Fatalf("expected independent input_image payloads, got %#v", content)
	}
}

func TestOpenAIProviderGenerateUsesConfiguredImagesEndpointWhenReferenceImagesArePresent(t *testing.T) {
	var formValues map[string][]string
	var imageParts []string
	var imageBodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/edits" {
			t.Fatalf("expected configured images endpoint to switch to edits, got %s", r.URL.Path)
		}
		formValues, imageParts, imageBodies = readMultipartProviderRequest(t, r)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"ZmFrZQ=="}]}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{
		OpenAIBaseURL: server.URL,
		OpenAIAPIKey:  "test-key",
	})
	provider.client = server.Client()

	_, providerErr := provider.Generate(context.Background(), ImageGenerationInput{
		Model:               "gpt-image-2",
		Prompt:              "tiny blue bird",
		AspectRatio:         "1:1",
		Size:                "1024x1024",
		ProviderAPIEndpoint: "/v1/images/generations",
		ReferenceImages: []ReferenceImageInput{
			{MIMEType: "image/png", Base64Data: "aW1hZ2UtMQ=="},
		},
	})
	if providerErr != nil {
		t.Fatalf("Generate() provider error = %+v", providerErr)
	}
	if formValues["model"][0] != "gpt-image-2" {
		t.Fatalf("expected selected image model gpt-image-2, got %#v", formValues["model"])
	}
	if _, exists := formValues["aspect_ratio"]; exists {
		t.Fatalf("did not expect aspect_ratio in edits multipart payload, got %#v", formValues["aspect_ratio"])
	}
	if imageParts == nil || len(imageParts) != 1 || imageParts[0] != "image" {
		t.Fatalf("expected one image file part, got names=%#v bodies=%#v", imageParts, imageBodies)
	}
	if imageBodies[0] != "image-1" {
		t.Fatalf("expected decoded reference image bytes, got %#v", imageBodies[0])
	}
}

func TestOpenAIProviderGenerateUsesEditsMultipartForMultipleImagesEndpoint(t *testing.T) {
	sourceImage := testPNGBase64(t, color.RGBA{R: 240, A: 255})
	firstReference := testPNGBase64(t, color.RGBA{G: 220, A: 255})
	secondReference := testPNGBase64(t, color.RGBA{B: 230, A: 255})

	var formValues map[string][]string
	var imageParts []string
	var imageBodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/edits" {
			t.Fatalf("expected configured images endpoint to switch to edits, got %s", r.URL.Path)
		}
		formValues, imageParts, imageBodies = readMultipartProviderRequest(t, r)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"ZmFrZQ=="}]}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{
		OpenAIBaseURL: server.URL,
		OpenAIAPIKey:  "test-key",
	})
	provider.client = server.Client()

	_, providerErr := provider.Generate(context.Background(), ImageGenerationInput{
		Model:               "gpt-image-2",
		Prompt:              "create using every reference",
		AspectRatio:         "1:1",
		Size:                "1024x1024",
		ProviderAPIEndpoint: "/v1/images/generations",
		SourceImage: &ReferenceImageInput{
			MIMEType:   "image/png",
			Base64Data: sourceImage,
		},
		ReferenceImages: []ReferenceImageInput{
			{MIMEType: "image/png", Base64Data: firstReference},
			{MIMEType: "image/png", Base64Data: secondReference},
		},
	})
	if providerErr != nil {
		t.Fatalf("Generate() provider error = %+v", providerErr)
	}

	if formValues["response_format"][0] != "b64_json" {
		t.Fatalf("expected b64_json response format, got %#v", formValues["response_format"])
	}
	if len(imageParts) != 3 {
		t.Fatalf("expected three independent image file parts, got names=%#v bodies=%#v", imageParts, imageBodies)
	}
	if imageParts[0] != "image[]" || imageParts[1] != "image[]" || imageParts[2] != "image[]" {
		t.Fatalf("expected multi-image edits parts to use image[], got %#v", imageParts)
	}
	for index, body := range imageBodies {
		if strings.TrimSpace(body) == "" {
			t.Fatalf("expected image part %d to contain bytes", index)
		}
	}
	promptText := formValues["prompt"][0]
	if strings.Contains(promptText, imageReferenceSheetPromptInstruction) {
		t.Fatalf("did not expect reference sheet prompt instruction for edits multipart, got %#v", promptText)
	}
}

func TestOpenAIProviderGenerateSendsMaskImageAsMultipartMask(t *testing.T) {
	sourceImage := testPNGBase64(t, color.RGBA{R: 240, A: 255})
	referenceImage := testPNGBase64(t, color.RGBA{G: 220, A: 255})
	maskImage := testPNGBase64(t, color.RGBA{A: 255})

	var imageParts []string
	var imageBodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/edits" {
			t.Fatalf("expected configured images endpoint to switch to edits, got %s", r.URL.Path)
		}
		_, imageParts, imageBodies = readMultipartProviderRequest(t, r)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"ZmFrZQ=="}]}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{
		OpenAIBaseURL: server.URL,
		OpenAIAPIKey:  "test-key",
	})
	provider.client = server.Client()

	_, providerErr := provider.Generate(context.Background(), ImageGenerationInput{
		Model:               "gpt-image-2",
		Prompt:              "edit masked area",
		Size:                "1024x1024",
		ProviderAPIEndpoint: "/v1/images/generations",
		ToolMode:            GenerationToolModePrecisionEdit,
		SourceImage: &ReferenceImageInput{
			MIMEType:   "image/png",
			Base64Data: sourceImage,
		},
		MaskImage: &ReferenceImageInput{
			MIMEType:   "image/png",
			Base64Data: maskImage,
		},
		ReferenceImages: []ReferenceImageInput{
			{MIMEType: "image/png", Base64Data: referenceImage},
		},
	})
	if providerErr != nil {
		t.Fatalf("Generate() provider error = %+v", providerErr)
	}

	if len(imageParts) != 3 {
		t.Fatalf("expected source, reference and mask file parts, got names=%#v bodies=%d", imageParts, len(imageBodies))
	}
	if imageParts[0] != "image[]" || imageParts[1] != "image[]" || imageParts[2] != "mask" {
		t.Fatalf("expected mask image to use multipart mask field after regular images, got %#v", imageParts)
	}
	if strings.TrimSpace(imageBodies[2]) == "" {
		t.Fatalf("expected mask multipart body to contain decoded PNG bytes")
	}
}

func TestOpenAIProviderGenerateWithImagesAPIKeepsComposePromptForEditsMultipart(t *testing.T) {
	firstReference := testPNGBase64(t, color.RGBA{R: 240, A: 255})
	secondReference := testPNGBase64(t, color.RGBA{G: 220, A: 255})

	var formValues map[string][]string
	var imageParts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/edits" {
			t.Fatalf("expected configured images endpoint to switch to edits, got %s", r.URL.Path)
		}
		var imageBodies []string
		formValues, imageParts, imageBodies = readMultipartProviderRequest(t, r)
		if len(imageBodies) != 2 {
			t.Fatalf("expected two image bodies, got %#v", imageBodies)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"ZmFrZQ=="}]}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{
		OpenAIBaseURL: server.URL,
		OpenAIAPIKey:  "test-key",
	})
	provider.client = server.Client()

	_, providerErr := provider.generateWithImagesAPI(context.Background(), ImageGenerationInput{
		Model:               "gpt-image-2",
		Prompt:              "合成两位人物",
		Size:                "1024x1024",
		ProviderAPIEndpoint: "/v1/images/generations",
		ReferenceIntent:     GenerationReferenceIntentCompose,
		ReferenceImages: []ReferenceImageInput{
			{MIMEType: "image/png", Base64Data: firstReference},
			{MIMEType: "image/png", Base64Data: secondReference},
		},
	})
	if providerErr != nil {
		t.Fatalf("Generate() provider error = %+v", providerErr)
	}

	if len(imageParts) != 2 || imageParts[0] != "image[]" || imageParts[1] != "image[]" {
		t.Fatalf("expected two independent image[] parts, got %#v", imageParts)
	}
	promptText := formValues["prompt"][0]
	if strings.Contains(promptText, "参考图已合成为一张带编号标签的多图网格") || !strings.Contains(promptText, "背景/场景严格取【图2】") {
		t.Fatalf("expected compose prompt without reference sheet instructions, got %#v", promptText)
	}
}

func TestOpenAIProviderGenerateDoesNotAddReferenceSheetInstructionForEditsMultipart(t *testing.T) {
	tinyImage := testPNGBase64(t, color.RGBA{R: 128, G: 64, B: 32, A: 255})
	var prompts []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data;") {
			values, _, _ := readMultipartProviderRequest(t, r)
			prompts = append(prompts, values["prompt"][0])
		} else {
			var payload map[string]any
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatalf("decode request payload: %v", err)
			}
			inputs := payload["input"].([]any)
			message := inputs[0].(map[string]any)
			content := message["content"].([]any)
			first := content[0].(map[string]any)
			prompts = append(prompts, first["text"].(string))
		}

		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/responses" {
			_, _ = w.Write([]byte(`{"output":[{"type":"image_generation_call","result":"ZmFrZQ=="}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"ZmFrZQ=="}]}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{
		OpenAIBaseURL: server.URL,
		OpenAIAPIKey:  "test-key",
	})
	provider.client = server.Client()

	cases := []ImageGenerationInput{
		{
			Model:               "gpt-image-2",
			Prompt:              "single reference",
			ProviderAPIEndpoint: "/v1/images/generations",
			ReferenceImages: []ReferenceImageInput{
				{MIMEType: "image/png", Base64Data: tinyImage},
			},
		},
		{
			Model:               "gpt-image-2",
			Prompt:              "responses references",
			ProviderAPIEndpoint: "responses",
			ReferenceImages: []ReferenceImageInput{
				{MIMEType: "image/png", Base64Data: "aW1hZ2UtMQ=="},
				{MIMEType: "image/png", Base64Data: "aW1hZ2UtMg=="},
			},
		},
		{
			Model:               "gpt-image-2",
			Prompt:              "multi reference",
			ProviderAPIEndpoint: "/v1/images/generations",
			ReferenceImages: []ReferenceImageInput{
				{MIMEType: "image/png", Base64Data: tinyImage},
				{MIMEType: "image/png", Base64Data: tinyImage},
			},
		},
	}
	for _, input := range cases {
		if _, providerErr := provider.Generate(context.Background(), input); providerErr != nil {
			t.Fatalf("Generate(%q) provider error = %+v", input.Prompt, providerErr)
		}
	}

	if len(prompts) != 3 {
		t.Fatalf("expected three captured prompts, got %#v", prompts)
	}
	if strings.Contains(prompts[0], imageReferenceSheetPromptInstruction) {
		t.Fatalf("did not expect sheet instruction for single image endpoint prompt: %q", prompts[0])
	}
	if strings.Contains(prompts[1], imageReferenceSheetPromptInstruction) {
		t.Fatalf("did not expect sheet instruction for responses endpoint prompt: %q", prompts[1])
	}
	if strings.Contains(prompts[2], imageReferenceSheetPromptInstruction) {
		t.Fatalf("did not expect sheet instruction for multi image edits prompt: %q", prompts[2])
	}
}

func TestOpenAIProviderGenerateDefaultsToImagesEndpointWhenReferenceImagesArePresent(t *testing.T) {
	var formValues map[string][]string
	var imageParts []string
	var imageBodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/edits" {
			t.Fatalf("expected default images endpoint to switch to edits, got %s", r.URL.Path)
		}
		formValues, imageParts, imageBodies = readMultipartProviderRequest(t, r)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"ZmFrZQ=="}]}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{
		OpenAIBaseURL: server.URL,
		OpenAIAPIKey:  "test-key",
	})
	provider.client = server.Client()

	_, providerErr := provider.Generate(context.Background(), ImageGenerationInput{
		Model:       "gpt-image-2",
		Prompt:      "tiny blue bird",
		AspectRatio: "9:21",
		Size:        "1024x1536",
		ReferenceImages: []ReferenceImageInput{
			{MIMEType: "image/png", Base64Data: "aW1hZ2UtMQ=="},
		},
	})
	if providerErr != nil {
		t.Fatalf("Generate() provider error = %+v", providerErr)
	}
	if _, exists := formValues["aspect_ratio"]; exists {
		t.Fatalf("did not expect aspect_ratio in edits multipart payload, got %#v", formValues["aspect_ratio"])
	}
	if len(imageParts) != 1 || imageParts[0] != "image" {
		t.Fatalf("expected one reference image file part, got names=%#v bodies=%#v", imageParts, imageBodies)
	}
	if imageBodies[0] != "image-1" {
		t.Fatalf("expected decoded reference image bytes, got %#v", imageBodies[0])
	}
}

func TestOpenAIProviderGenerateUsesPublicReferenceImageURLForImagesEndpoint(t *testing.T) {
	const referenceURL = "https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/reference.png"

	var imageBodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/edits" {
			t.Fatalf("expected configured images endpoint to switch to edits, got %s", r.URL.Path)
		}
		_, _, imageBodies = readMultipartProviderRequest(t, r)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"ZmFrZQ=="}]}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{
		OpenAIBaseURL: server.URL,
		OpenAIAPIKey:  "test-key",
	})
	provider.client = server.Client()

	_, providerErr := provider.Generate(context.Background(), ImageGenerationInput{
		Model:               "gpt-image-2",
		Prompt:              "tiny blue bird",
		AspectRatio:         "1:1",
		Size:                "1024x1024",
		ProviderAPIEndpoint: "/v1/images/generations",
		ReferenceImages: []ReferenceImageInput{
			{MIMEType: "image/png", Base64Data: "aW1hZ2UtMQ==", InputURL: referenceURL},
		},
	})
	if providerErr != nil {
		t.Fatalf("Generate() provider error = %+v", providerErr)
	}

	if len(imageBodies) != 1 || imageBodies[0] != "image-1" {
		t.Fatalf("expected edits endpoint to upload decoded inline reference image bytes despite public URL %q, got %#v", referenceURL, imageBodies)
	}
}

func TestOpenAIProviderGenerateFetchesImageWhenProviderReturnsURL(t *testing.T) {
	const imageBody = "fake-png-binary"

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/images/generations":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"url":"` + server.URL + `/generated.png"}]}`))
		case "/generated.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte(imageBody))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{
		OpenAIBaseURL: server.URL,
		OpenAIAPIKey:  "test-key",
	})
	provider.client = server.Client()

	result, providerErr := provider.Generate(context.Background(), ImageGenerationInput{
		Model:  "gpt-image-2",
		Prompt: "tiny blue bird",
		Size:   "1024x1024",
	})
	if providerErr != nil {
		t.Fatalf("Generate() provider error = %+v", providerErr)
	}

	expected := base64.StdEncoding.EncodeToString([]byte(imageBody))
	if result.Base64Image != expected {
		t.Fatalf("expected fetched image URL result, got %q", result.Base64Image)
	}
	if result.MIMEType != "image/png" {
		t.Fatalf("expected MIME type image/png, got %q", result.MIMEType)
	}
}

func TestOpenAIProviderGenerateAcceptsDataURLB64JSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Fatalf("expected request path /v1/images/generations, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"data:image/png;base64,ZmFrZS1wbmc="}]}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{
		OpenAIBaseURL: server.URL,
		OpenAIAPIKey:  "test-key",
	})
	provider.client = server.Client()

	result, providerErr := provider.Generate(context.Background(), ImageGenerationInput{
		Model:  "gpt-image-2",
		Prompt: "tiny blue bird",
		Size:   "1024x1024",
	})
	if providerErr != nil {
		t.Fatalf("Generate() provider error = %+v", providerErr)
	}
	if result.Base64Image != "ZmFrZS1wbmc=" {
		t.Fatalf("expected base64 image without data URL prefix, got %q", result.Base64Image)
	}
	if result.MIMEType != "image/png" {
		t.Fatalf("expected MIME type image/png, got %q", result.MIMEType)
	}
}

func TestOpenAIProviderGenerateUsesChatCompletionsPayloadForTextToImage(t *testing.T) {
	const imageBody = "fake-chat-png"
	var payload map[string]any

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST chat completions request, got %s", r.Method)
			}
			if auth := r.Header.Get("Authorization"); auth != "Bearer chat-key" {
				t.Fatalf("expected model-specific bearer token, got %q", auth)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatalf("decode request payload: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("x-request-id", "chat-req-1")
			_, _ = w.Write([]byte(fmt.Sprintf(`{"choices":[{"message":{"content":"![result](%s/generated.png)"}}]}`, server.URL)))
		case "/generated.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte(imageBody))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{
		OpenAIBaseURL: "https://wrong.example",
		OpenAIAPIKey:  "global-key",
	})
	provider.client = server.Client()

	result, providerErr := provider.Generate(context.Background(), ImageGenerationInput{
		Model:               "gpt-best-image",
		Prompt:              "tiny blue bird",
		ProviderBaseURL:     server.URL,
		ProviderAPIKey:      "chat-key",
		ProviderAPIEndpoint: "/v1/chat/completions",
	})
	if providerErr != nil {
		t.Fatalf("Generate() provider error = %+v", providerErr)
	}
	if result.Base64Image != base64.StdEncoding.EncodeToString([]byte(imageBody)) ||
		result.MIMEType != "image/png" ||
		result.ProviderRequestID != "chat-req-1" {
		t.Fatalf("unexpected chat generation result: %+v", result)
	}
	if payload["model"] != "gpt-best-image" {
		t.Fatalf("expected chat model gpt-best-image, got %#v", payload["model"])
	}
	if payload["stream"] != false {
		t.Fatalf("expected stream=false, got %#v", payload["stream"])
	}
	messages := payload["messages"].([]any)
	message := messages[0].(map[string]any)
	if message["role"] != "user" || message["content"] != "tiny blue bird" {
		t.Fatalf("expected plain text user message, got %#v", message)
	}
}

func TestOpenAIProviderGenerateUsesChatCompletionsPayloadForImageEdit(t *testing.T) {
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("expected request path /v1/chat/completions, got %s", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("decode request payload: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"data:image/png;base64,ZmFrZS1jaGF0"}}]}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{
		OpenAIBaseURL: server.URL,
		OpenAIAPIKey:  "test-key",
	})
	provider.client = server.Client()

	result, providerErr := provider.Generate(context.Background(), ImageGenerationInput{
		Model:               "gpt-best-image",
		Prompt:              "change background",
		ProviderAPIEndpoint: "chat",
		SourceImage: &ReferenceImageInput{
			MIMEType:   "image/png",
			Base64Data: "c291cmNl",
		},
		ReferenceImages: []ReferenceImageInput{
			{MIMEType: "image/jpeg", Base64Data: "cmVm"},
		},
	})
	if providerErr != nil {
		t.Fatalf("Generate() provider error = %+v", providerErr)
	}
	if result.Base64Image != "ZmFrZS1jaGF0" || result.MIMEType != "image/png" {
		t.Fatalf("unexpected chat edit result: %+v", result)
	}

	messages := payload["messages"].([]any)
	message := messages[0].(map[string]any)
	if message["role"] != "user" {
		t.Fatalf("expected user role, got %#v", message["role"])
	}
	content, ok := message["content"].([]any)
	if !ok || len(content) != 3 {
		t.Fatalf("expected text, source image, and one reference image, got %#v", message["content"])
	}
	text := content[0].(map[string]any)
	textPrompt, _ := text["text"].(string)
	if text["type"] != "text" || !strings.Contains(textPrompt, "change background") || !strings.Contains(textPrompt, "上传图片是本次生成的主要视觉参考") {
		t.Fatalf("expected first content item to be text prompt, got %#v", text)
	}
	source := content[1].(map[string]any)
	sourceURL := source["image_url"].(map[string]any)["url"]
	if source["type"] != "image_url" || sourceURL != "data:image/png;base64,c291cmNl" {
		t.Fatalf("expected source image data URL, got %#v", source)
	}
	reference := content[2].(map[string]any)
	referenceURL := reference["image_url"].(map[string]any)["url"]
	if reference["type"] != "image_url" || referenceURL != "data:image/jpeg;base64,cmVm" {
		t.Fatalf("expected reference image data URL, got %#v", reference)
	}
}

func TestOpenAIProviderGenerateParsesChatJSONImageURL(t *testing.T) {
	const imageBody = "fake-json-chat-png"

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(`{"choices":[{"message":{"content":%q}}]}`, fmt.Sprintf(`{"images":[{"url":"%s/image.png"}]}`, server.URL))))
		case "/image.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte(imageBody))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{
		OpenAIBaseURL: server.URL,
		OpenAIAPIKey:  "test-key",
	})
	provider.client = server.Client()

	result, providerErr := provider.Generate(context.Background(), ImageGenerationInput{
		Model:               "gpt-best-image",
		Prompt:              "json image",
		ProviderAPIEndpoint: "/v1/chat/completions",
	})
	if providerErr != nil {
		t.Fatalf("Generate() provider error = %+v", providerErr)
	}
	if result.Base64Image != base64.StdEncoding.EncodeToString([]byte(imageBody)) {
		t.Fatalf("expected fetched JSON image URL, got %+v", result)
	}
}

func TestOpenAIProviderGenerateReturnsEmptyImageForChatWithoutImage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("expected request path /v1/chat/completions, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-request-id", "chat-empty")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"done, but no image"}}]}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{
		OpenAIBaseURL: server.URL,
		OpenAIAPIKey:  "test-key",
	})
	provider.client = server.Client()

	_, providerErr := provider.Generate(context.Background(), ImageGenerationInput{
		Model:               "gpt-best-image",
		Prompt:              "no image",
		ProviderAPIEndpoint: "/v1/chat/completions",
	})
	if providerErr == nil {
		t.Fatal("expected provider_empty_image")
	}
	if providerErr.Code != "provider_empty_image" ||
		providerErr.ProviderRequestID != "chat-empty" ||
		providerErr.FailureStage != providerFailureStageImageGenerationRequest {
		t.Fatalf("unexpected empty chat image error: %+v", providerErr)
	}
}

func TestOpenAIProviderRetriesChatImageURLFetchWithoutResubmittingGeneration(t *testing.T) {
	const imageBody = "eventual-chat-image"
	var chatCalls int
	var assetCalls int

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			chatCalls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(`{"choices":[{"message":{"content":"%s/image.png"}}]}`, server.URL)))
		case "/image.png":
			assetCalls++
			if assetCalls <= 2 {
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte("asset warming up"))
				return
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte(imageBody))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{
		OpenAIBaseURL: server.URL,
		OpenAIAPIKey:  "test-key",
	})
	provider.client = server.Client()

	result, providerErr := provider.Generate(context.Background(), ImageGenerationInput{
		Model:               "gpt-best-image",
		Prompt:              "retry asset fetch",
		ProviderAPIEndpoint: "/v1/chat/completions",
	})
	if providerErr != nil {
		t.Fatalf("Generate() provider error = %+v", providerErr)
	}
	if chatCalls != 1 {
		t.Fatalf("expected one chat generation request, got %d", chatCalls)
	}
	if assetCalls != 3 {
		t.Fatalf("expected three asset fetch attempts, got %d", assetCalls)
	}
	if result.Base64Image != base64.StdEncoding.EncodeToString([]byte(imageBody)) {
		t.Fatalf("unexpected image result: %+v", result)
	}
}

func TestOpenAIProviderReportsAssetFetchFailureAfterURLRetries(t *testing.T) {
	var assetCalls int
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(`{"choices":[{"message":{"content":"%s/image.png"}}]}`, server.URL)))
		case "/image.png":
			assetCalls++
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("asset gateway failed"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{
		OpenAIBaseURL: server.URL,
		OpenAIAPIKey:  "test-key",
	})
	provider.client = server.Client()

	_, providerErr := provider.Generate(context.Background(), ImageGenerationInput{
		Model:               "gpt-best-image",
		Prompt:              "asset failure",
		ProviderAPIEndpoint: "/v1/chat/completions",
	})
	if providerErr == nil {
		t.Fatal("expected asset fetch provider error")
	}
	if assetCalls != 3 {
		t.Fatalf("expected three asset fetch attempts, got %d", assetCalls)
	}
	if providerErr.HTTPStatus != http.StatusBadGateway ||
		providerErr.Code != "provider_asset_http_502" ||
		providerErr.Message != "asset gateway failed" ||
		providerErr.FailureStage != providerFailureStageProviderAssetFetch ||
		providerErr.AttemptCount != 3 {
		t.Fatalf("unexpected asset fetch provider error: %+v", providerErr)
	}
}

func TestOpenAIProviderSubmitsAndPollsSoraVideoTask(t *testing.T) {
	const videoBody = "fake-mp4-binary"
	var submitPayload map[string]any

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/videos/generations":
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST submit, got %s", r.Method)
			}
			if auth := r.Header.Get("Authorization"); auth != "Bearer video-key" {
				t.Fatalf("expected model-specific video key, got %q", auth)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read video request body: %v", err)
			}
			if err := json.Unmarshal(body, &submitPayload); err != nil {
				t.Fatalf("decode video request payload: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("x-request-id", "submit-req")
			_, _ = w.Write([]byte(`{"task_id":"sora-task-1"}`))
		case "/v2/videos/generations/sora-task-1":
			if r.Method != http.MethodGet {
				t.Fatalf("expected GET poll, got %s", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("x-request-id", "poll-req")
			_, _ = w.Write([]byte(`{"task_id":"sora-task-1","status":"SUCCESS","progress":"100%","data":{"output":"` + server.URL + `/video.mp4"}}`))
		case "/video.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte(videoBody))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{
		OpenAIBaseURL: "https://wrong.example",
		OpenAIAPIKey:  "global-key",
	})
	provider.client = server.Client()

	input := VideoGenerationInput{
		Model:               "sora-2-pro",
		Prompt:              "产品主图动起来",
		AspectRatio:         "9:16",
		Duration:            "15",
		HD:                  true,
		Private:             true,
		Images:              []string{"https://assets.example/ref.png"},
		ProviderBaseURL:     server.URL,
		ProviderAPIKey:      "video-key",
		ProviderAPIEndpoint: "/v2/videos/generations",
	}
	submit, providerErr := provider.SubmitVideo(context.Background(), input)
	if providerErr != nil {
		t.Fatalf("SubmitVideo() provider error = %+v", providerErr)
	}
	if submit.TaskID != "sora-task-1" || submit.ProviderRequestID != "submit-req" {
		t.Fatalf("unexpected submit result: %+v", submit)
	}

	poll, providerErr := provider.PollVideo(context.Background(), submit.TaskID, input)
	if providerErr != nil {
		t.Fatalf("PollVideo() provider error = %+v", providerErr)
	}
	if poll.Status != VideoTaskSucceeded || poll.OutputBase64 != base64.StdEncoding.EncodeToString([]byte(videoBody)) || poll.MIMEType != "video/mp4" {
		t.Fatalf("unexpected poll result: %+v", poll)
	}
	if submitPayload["model"] != "sora-2-pro" || submitPayload["prompt"] != "产品主图动起来" || submitPayload["aspect_ratio"] != "9:16" || submitPayload["duration"] != "15" {
		t.Fatalf("unexpected video submit payload: %#v", submitPayload)
	}
	if images, ok := submitPayload["images"].([]any); !ok || len(images) != 1 || images[0] != "https://assets.example/ref.png" {
		t.Fatalf("expected reference image in video payload, got %#v", submitPayload["images"])
	}
}

func TestOpenAIProviderSubmitsAndPollsArkSeedanceVideoTask(t *testing.T) {
	const videoBody = "fake-ark-seedance-mp4"
	var submitPayload map[string]any

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/contents/generations/tasks":
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST Ark submit, got %s", r.Method)
			}
			if auth := r.Header.Get("Authorization"); auth != "Bearer ark-key" {
				t.Fatalf("expected Ark bearer key, got %q", auth)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read Ark request body: %v", err)
			}
			if err := json.Unmarshal(body, &submitPayload); err != nil {
				t.Fatalf("decode Ark request payload: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("x-request-id", "ark-submit-req")
			_, _ = w.Write([]byte(`{"id":"cgt-ark-1"}`))
		case "/api/v3/contents/generations/tasks/cgt-ark-1":
			if r.Method != http.MethodGet {
				t.Fatalf("expected GET Ark poll, got %s", r.Method)
			}
			if auth := r.Header.Get("Authorization"); auth != "Bearer ark-key" {
				t.Fatalf("expected Ark bearer key on poll, got %q", auth)
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("x-request-id", "ark-poll-req")
			_, _ = w.Write([]byte(`{"id":"cgt-ark-1","status":"succeeded","content":{"video_url":"` + server.URL + `/ark.mp4"},"usage":{"total_tokens":1234}}`))
		case "/ark.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte(videoBody))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{
		OpenAIBaseURL: "https://wrong.example",
		OpenAIAPIKey:  "global-key",
	})
	provider.client = server.Client()

	input := VideoGenerationInput{
		Model:               "doubao-seed-2-0-mini-260428",
		Prompt:              "make the product image move",
		AspectRatio:         "21:9",
		Duration:            "-1",
		HD:                  true,
		Watermark:           true,
		Private:             true,
		Images:              []string{"https://assets.example/ref-a.png", "https://assets.example/ref-b.png"},
		ProviderBaseURL:     server.URL + "/api/v3",
		ProviderAPIKey:      "ark-key",
		ProviderAPIEndpoint: "/contents/generations/tasks",
	}
	submit, providerErr := provider.SubmitVideo(context.Background(), input)
	if providerErr != nil {
		t.Fatalf("SubmitVideo() provider error = %+v", providerErr)
	}
	if submit.TaskID != "cgt-ark-1" || submit.ProviderRequestID != "ark-submit-req" {
		t.Fatalf("unexpected Ark submit result: %+v", submit)
	}

	poll, providerErr := provider.PollVideo(context.Background(), submit.TaskID, input)
	if providerErr != nil {
		t.Fatalf("PollVideo() provider error = %+v", providerErr)
	}
	if poll.TaskID != "cgt-ark-1" || poll.Status != VideoTaskSucceeded || poll.OutputURL != server.URL+"/ark.mp4" {
		t.Fatalf("unexpected Ark poll result: %+v", poll)
	}
	if poll.OutputBase64 != base64.StdEncoding.EncodeToString([]byte(videoBody)) || poll.MIMEType != "video/mp4" {
		t.Fatalf("unexpected Ark downloaded video: %+v", poll)
	}
	if submitPayload["model"] != "doubao-seed-2-0-mini-260428" ||
		submitPayload["ratio"] != "21:9" ||
		submitPayload["duration"] != float64(-1) ||
		submitPayload["resolution"] != "720p" ||
		submitPayload["watermark"] != true ||
		submitPayload["generate_audio"] != false {
		t.Fatalf("unexpected Ark submit payload: %#v", submitPayload)
	}
	content, ok := submitPayload["content"].([]any)
	if !ok || len(content) != 3 {
		t.Fatalf("expected text plus two reference images, got %#v", submitPayload["content"])
	}
	textItem := content[0].(map[string]any)
	if textItem["type"] != "text" || textItem["text"] != "make the product image move" {
		t.Fatalf("unexpected Ark text content: %#v", textItem)
	}
	for i, raw := range content[1:] {
		item := raw.(map[string]any)
		imageURL := item["image_url"].(map[string]any)
		if item["type"] != "image_url" || item["role"] != "reference_image" || imageURL["url"] == "" {
			t.Fatalf("unexpected Ark image content %d: %#v", i, item)
		}
	}
}

func TestOpenAIProviderCanonicalizesArkSeedanceMiniAlias(t *testing.T) {
	var submitPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/contents/generations/tasks" {
			t.Fatalf("expected Ark submit path, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST Ark submit, got %s", r.Method)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read Ark request body: %v", err)
		}
		if err := json.Unmarshal(body, &submitPayload); err != nil {
			t.Fatalf("decode Ark request payload: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"cgt-ark-alias"}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{})
	provider.client = server.Client()

	submit, providerErr := provider.SubmitVideo(context.Background(), VideoGenerationInput{
		Model:               "doubao-seed-2-0-mini",
		Prompt:              "make a tiny video",
		AspectRatio:         "16:9",
		Duration:            "4",
		ProviderBaseURL:     server.URL + "/api/v3",
		ProviderAPIKey:      "ark-key",
		ProviderAPIEndpoint: "/contents/generations/tasks",
	})
	if providerErr != nil {
		t.Fatalf("SubmitVideo() provider error = %+v", providerErr)
	}
	if submit.TaskID != "cgt-ark-alias" {
		t.Fatalf("unexpected submit result: %+v", submit)
	}
	if submitPayload["model"] != arkSeedanceMiniRuntimeModel {
		t.Fatalf("expected canonical Ark model %q, got payload %#v", arkSeedanceMiniRuntimeModel, submitPayload)
	}
}

func TestOpenAIProviderSubmitsArkSeedance2RuntimeAndResolution(t *testing.T) {
	var submitPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/contents/generations/tasks" {
			t.Fatalf("expected Ark submit path, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST Ark submit, got %s", r.Method)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read Ark request body: %v", err)
		}
		if err := json.Unmarshal(body, &submitPayload); err != nil {
			t.Fatalf("decode Ark request payload: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"cgt-ark-seedance2"}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{})
	provider.client = server.Client()

	submit, providerErr := provider.SubmitVideo(context.Background(), VideoGenerationInput{
		Model:               arkSeedance2RuntimeModel,
		Prompt:              "make a 1080p seedance 2.0 video",
		AspectRatio:         "16:9",
		Duration:            "10",
		Resolution:          "1080p",
		ProviderBaseURL:     server.URL + "/api/v3",
		ProviderAPIKey:      "ark-key",
		ProviderAPIEndpoint: "/contents/generations/tasks",
	})
	if providerErr != nil {
		t.Fatalf("SubmitVideo() provider error = %+v", providerErr)
	}
	if submit.TaskID != "cgt-ark-seedance2" {
		t.Fatalf("unexpected submit result: %+v", submit)
	}
	if submitPayload["model"] != arkSeedance2RuntimeModel || submitPayload["resolution"] != "1080p" {
		t.Fatalf("expected Ark Seedance 2.0 runtime and 1080p resolution, got payload %#v", submitPayload)
	}
}

func TestOpenAIProviderSubmitsArkSeedance2MultimodalReferences(t *testing.T) {
	var submitPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/contents/generations/tasks" {
			t.Fatalf("expected Ark submit path, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST Ark submit, got %s", r.Method)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read Ark request body: %v", err)
		}
		if err := json.Unmarshal(body, &submitPayload); err != nil {
			t.Fatalf("decode Ark request payload: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"cgt-ark-multimodal"}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{})
	provider.client = server.Client()

	submit, providerErr := provider.SubmitVideo(context.Background(), VideoGenerationInput{
		Model:               arkSeedance2RuntimeModel,
		Prompt:              "make a product video from all references",
		AspectRatio:         "16:9",
		Duration:            "11",
		Resolution:          "1080p",
		Watermark:           true,
		GenerateAudio:       true,
		Images:              []string{"https://assets.example/ref-1.png", "https://assets.example/ref-2.jpg"},
		ReferenceVideos:     []string{"https://assets.example/ref-video.mp4"},
		ReferenceAudios:     []string{"https://assets.example/ref-audio.mp3"},
		ProviderBaseURL:     server.URL + "/api/v3",
		ProviderAPIKey:      "ark-key",
		ProviderAPIEndpoint: "/contents/generations/tasks",
	})
	if providerErr != nil {
		t.Fatalf("SubmitVideo() provider error = %+v", providerErr)
	}
	if submit.TaskID != "cgt-ark-multimodal" {
		t.Fatalf("unexpected submit result: %+v", submit)
	}
	if submitPayload["model"] != arkSeedance2RuntimeModel ||
		submitPayload["duration"] != float64(11) ||
		submitPayload["resolution"] != "1080p" ||
		submitPayload["watermark"] != true ||
		submitPayload["generate_audio"] != true {
		t.Fatalf("unexpected Ark multimodal payload: %#v", submitPayload)
	}
	content, ok := submitPayload["content"].([]any)
	if !ok || len(content) != 5 {
		t.Fatalf("expected text, two images, one video, one audio, got %#v", submitPayload["content"])
	}
	expected := []struct {
		kind string
		role string
		url  string
	}{
		{kind: "image_url", role: "reference_image", url: "https://assets.example/ref-1.png"},
		{kind: "image_url", role: "reference_image", url: "https://assets.example/ref-2.jpg"},
		{kind: "video_url", role: "reference_video", url: "https://assets.example/ref-video.mp4"},
		{kind: "audio_url", role: "reference_audio", url: "https://assets.example/ref-audio.mp3"},
	}
	text := content[0].(map[string]any)
	if text["type"] != "text" || text["text"] != "make a product video from all references" {
		t.Fatalf("unexpected text content item: %#v", text)
	}
	for index, want := range expected {
		item := content[index+1].(map[string]any)
		urlObject := item[want.kind].(map[string]any)
		if item["type"] != want.kind || item["role"] != want.role || urlObject["url"] != want.url {
			t.Fatalf("unexpected content item %d: got %#v want %+v", index+1, item, want)
		}
	}
}

func TestOpenAIProviderPollsArkSeedanceStatusMapping(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus string
		wantReason string
	}{
		{name: "queued", body: `{"id":"cgt-ark-1","status":"queued"}`, wantStatus: VideoTaskInProgress},
		{name: "running", body: `{"id":"cgt-ark-1","status":"running"}`, wantStatus: VideoTaskInProgress},
		{name: "failed", body: `{"id":"cgt-ark-1","status":"failed","error":{"code":"SensitiveContent","message":"input blocked"}}`, wantStatus: VideoTaskFailed, wantReason: "SensitiveContent: input blocked"},
		{name: "expired", body: `{"id":"cgt-ark-1","status":"expired","error":{"message":"task expired"}}`, wantStatus: VideoTaskFailed, wantReason: "task expired"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v3/contents/generations/tasks/cgt-ark-1" {
					t.Fatalf("expected Ark poll path, got %s", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			provider := NewOpenAIProvider(Config{
				OpenAIBaseURL: server.URL + "/api/v3",
				OpenAIAPIKey:  "ark-key",
			})
			provider.client = server.Client()

			poll, providerErr := provider.PollVideo(context.Background(), "cgt-ark-1", VideoGenerationInput{
				Model:               "doubao-seed-2-0-mini-260428",
				ProviderBaseURL:     server.URL + "/api/v3",
				ProviderAPIEndpoint: "/contents/generations/tasks",
				ProviderAPIKey:      "ark-key",
			})
			if providerErr != nil {
				t.Fatalf("PollVideo() provider error = %+v", providerErr)
			}
			if poll.Status != tt.wantStatus || poll.FailReason != tt.wantReason {
				t.Fatalf("unexpected Ark status mapping: %+v", poll)
			}
		})
	}
}

func TestOpenAIProviderSubmitsWuyinGrokImagineVideoTask(t *testing.T) {
	var submitPayload map[string]any
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/api/async/video_grok_imagine" {
			t.Fatalf("expected Wuyin submit path, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST submit, got %s", r.Method)
		}
		if key := r.URL.Query().Get("key"); key != "wuyin-key" {
			t.Fatalf("expected Wuyin submit query key, got %q", key)
		}
		if auth := r.Header.Get("Authorization"); auth != "wuyin-key" {
			t.Fatalf("expected raw Wuyin key, got %q", auth)
		}
		if contentType := r.Header.Get("Content-Type"); !strings.Contains(contentType, "application/json") {
			t.Fatalf("expected JSON content type, got %q", contentType)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read Wuyin request body: %v", err)
		}
		if err := json.Unmarshal(body, &submitPayload); err != nil {
			t.Fatalf("decode Wuyin request payload: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"msg":"ok","data":{"id":"video_wuyin_1"}}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{
		OpenAIBaseURL: "https://wrong.example",
		OpenAIAPIKey:  "global-key",
	})
	provider.client = server.Client()

	submit, providerErr := provider.SubmitVideo(context.Background(), VideoGenerationInput{
		Model:               "grok-imagine-video-1.5-preview",
		Prompt:              "a cinematic product launch",
		AspectRatio:         "9:16",
		Duration:            "15",
		HD:                  true,
		Private:             true,
		Watermark:           true,
		Images:              []string{"https://assets.example/ref.png"},
		ProviderBaseURL:     server.URL,
		ProviderAPIKey:      "wuyin-key",
		ProviderAPIEndpoint: "/api/async/video_grok_imagine",
	})
	if providerErr != nil {
		t.Fatalf("SubmitVideo() provider error = %+v", providerErr)
	}
	if requests != 1 {
		t.Fatalf("expected one Wuyin submit request, got %d", requests)
	}
	if submit.TaskID != "video_wuyin_1" || submit.ProviderRequestID != "video_wuyin_1" {
		t.Fatalf("unexpected Wuyin submit result: %+v", submit)
	}
	wantKeys := []string{"prompt", "duration", "aspect_ratio", "image_urls"}
	if len(submitPayload) != len(wantKeys) {
		t.Fatalf("expected only Wuyin payload keys %v, got %#v", wantKeys, submitPayload)
	}
	if submitPayload["prompt"] != "a cinematic product launch" || submitPayload["duration"] != "15" || submitPayload["aspect_ratio"] != "9:16" {
		t.Fatalf("unexpected Wuyin submit payload: %#v", submitPayload)
	}
	if urls, ok := submitPayload["image_urls"].([]any); !ok || len(urls) != 1 || urls[0] != "https://assets.example/ref.png" {
		t.Fatalf("expected image_urls in Wuyin payload, got %#v", submitPayload["image_urls"])
	}
	if _, ok := submitPayload["model"]; ok {
		t.Fatalf("Wuyin payload must not include model: %#v", submitPayload)
	}
	if _, ok := submitPayload["hd"]; ok {
		t.Fatalf("Wuyin payload must not include hd: %#v", submitPayload)
	}
	if _, ok := submitPayload["private"]; ok {
		t.Fatalf("Wuyin payload must not include private: %#v", submitPayload)
	}
	if _, ok := submitPayload["watermark"]; ok {
		t.Fatalf("Wuyin payload must not include watermark: %#v", submitPayload)
	}
}

func TestOpenAIProviderRejectsWuyinInlineReferenceImages(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.NotFound(w, r)
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{
		OpenAIBaseURL: server.URL,
		OpenAIAPIKey:  "wuyin-key",
	})
	provider.client = server.Client()

	_, providerErr := provider.SubmitVideo(context.Background(), VideoGenerationInput{
		Model:               "grok-imagine-video-1.5-preview",
		Prompt:              "animate this image",
		AspectRatio:         "16:9",
		Duration:            "6",
		Images:              []string{"data:image/png;base64,ZmFrZQ=="},
		ProviderBaseURL:     server.URL,
		ProviderAPIKey:      "wuyin-key",
		ProviderAPIEndpoint: "/api/async/video_grok_imagine",
	})
	if providerErr == nil {
		t.Fatalf("expected provider error for inline Wuyin image")
	}
	if providerErr.Code != "provider_reference_url_required" {
		t.Fatalf("expected provider_reference_url_required, got %+v", providerErr)
	}
	if requests != 0 {
		t.Fatalf("expected inline reference validation before HTTP call, got %d requests", requests)
	}
}

func TestOpenAIProviderPollsWuyinGrokImagineVideoTaskAndFetchesResult(t *testing.T) {
	const videoBody = "fake-wuyin-mp4"
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/async/detail":
			if r.Method != http.MethodGet {
				t.Fatalf("expected GET detail, got %s", r.Method)
			}
			if auth := r.Header.Get("Authorization"); auth != "wuyin-key" {
				t.Fatalf("expected raw Wuyin key, got %q", auth)
			}
			if got := r.URL.Query().Get("id"); got != "video_wuyin_1" {
				t.Fatalf("expected detail id video_wuyin_1, got %q", got)
			}
			if key := r.URL.Query().Get("key"); key != "wuyin-key" {
				t.Fatalf("expected Wuyin detail query key, got %q", key)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":200,"msg":"ok","data":{"status":2,"message":"","video_url":"` + server.URL + `/video.mp4"}}`))
		case "/video.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte(videoBody))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{
		OpenAIBaseURL: "https://wrong.example",
		OpenAIAPIKey:  "global-key",
	})
	provider.client = server.Client()

	poll, providerErr := provider.PollVideo(context.Background(), "video_wuyin_1", VideoGenerationInput{
		Model:               "grok-imagine-video-1.5-preview",
		ProviderBaseURL:     server.URL,
		ProviderAPIKey:      "wuyin-key",
		ProviderAPIEndpoint: "/api/async/video_grok_imagine",
	})
	if providerErr != nil {
		t.Fatalf("PollVideo() provider error = %+v", providerErr)
	}
	if poll.TaskID != "video_wuyin_1" || poll.Status != VideoTaskSucceeded || poll.OutputURL != server.URL+"/video.mp4" {
		t.Fatalf("unexpected Wuyin poll result: %+v", poll)
	}
	if poll.OutputBase64 != base64.StdEncoding.EncodeToString([]byte(videoBody)) || poll.MIMEType != "video/mp4" {
		t.Fatalf("unexpected Wuyin downloaded video: %+v", poll)
	}
	if poll.ProviderRequestID != "video_wuyin_1" {
		t.Fatalf("expected task id as provider request id fallback, got %q", poll.ProviderRequestID)
	}
}

func TestOpenAIProviderPollsWuyinGrokImagineStatusMapping(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus string
		wantReason string
	}{
		{name: "init", body: `{"code":200,"msg":"ok","data":{"status":0,"message":"queued"}}`, wantStatus: VideoTaskInProgress},
		{name: "running", body: `{"code":200,"msg":"ok","data":{"status":1,"message":"running"}}`, wantStatus: VideoTaskInProgress},
		{name: "failed", body: `{"code":200,"msg":"ok","data":{"status":3,"message":"审核失败"}}`, wantStatus: VideoTaskFailed, wantReason: "审核失败"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/async/detail" {
					t.Fatalf("expected detail path, got %s", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			provider := NewOpenAIProvider(Config{
				OpenAIBaseURL: server.URL,
				OpenAIAPIKey:  "wuyin-key",
			})
			provider.client = server.Client()

			poll, providerErr := provider.PollVideo(context.Background(), "video_wuyin_1", VideoGenerationInput{
				Model:               "grok-imagine-video-1.5-preview",
				ProviderBaseURL:     server.URL,
				ProviderAPIKey:      "wuyin-key",
				ProviderAPIEndpoint: "/api/async/video_grok_imagine",
			})
			if providerErr != nil {
				t.Fatalf("PollVideo() provider error = %+v", providerErr)
			}
			if poll.Status != tt.wantStatus || poll.FailReason != tt.wantReason {
				t.Fatalf("unexpected status mapping: %+v", poll)
			}
		})
	}
}

func TestOpenAIProviderSubmitsZZVideoTask(t *testing.T) {
	var submitPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/videos" {
			t.Fatalf("expected ZZ submit path, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST ZZ submit, got %s", r.Method)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer zz-key" {
			t.Fatalf("expected ZZ bearer key, got %q", auth)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read ZZ submit body: %v", err)
		}
		if err := json.Unmarshal(body, &submitPayload); err != nil {
			t.Fatalf("decode ZZ submit payload: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"zz-task-1"}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{OpenAIBaseURL: "https://wrong.example"})
	provider.client = server.Client()

	submit, providerErr := provider.SubmitVideo(context.Background(), VideoGenerationInput{
		Model:               "video-ds-2.0-fast",
		Prompt:              "make a ZZ video",
		AspectRatio:         "9:16",
		Duration:            "15",
		Resolution:          "720p",
		Images:              []string{"https://assets.example/ref.png"},
		ReferenceVideos:     []string{"https://assets.example/ref.mp4"},
		ReferenceAudios:     []string{"https://assets.example/ref.mp3"},
		ProviderBaseURL:     server.URL,
		ProviderAPIKey:      "zz-key",
		ProviderAPIEndpoint: "/v1/videos",
	})
	if providerErr != nil {
		t.Fatalf("SubmitVideo() provider error = %+v", providerErr)
	}
	if submit.TaskID != "zz-task-1" || submit.ProviderRequestID != "zz-task-1" {
		t.Fatalf("unexpected ZZ submit result: %+v", submit)
	}
	if submitPayload["model"] != "video-ds-2.0-fast" ||
		submitPayload["prompt"] != "make a ZZ video" ||
		submitPayload["aspect_ratio"] != "9:16" ||
		submitPayload["resolution"] != "720p" ||
		submitPayload["seconds"] != float64(15) {
		t.Fatalf("unexpected ZZ submit payload: %#v", submitPayload)
	}
	for key, want := range map[string]string{
		"images": "https://assets.example/ref.png",
		"videos": "https://assets.example/ref.mp4",
		"audios": "https://assets.example/ref.mp3",
	} {
		values, ok := submitPayload[key].([]any)
		if !ok || len(values) != 1 || values[0] != want {
			t.Fatalf("expected ZZ %s payload %q, got %#v", key, want, submitPayload[key])
		}
	}
}

func TestOpenAIProviderPollsZZVideoTaskAndDownloadsContent(t *testing.T) {
	const videoBody = "fake-zz-mp4"
	var contentAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/videos/zz-task-1":
			if r.Method != http.MethodGet {
				t.Fatalf("expected GET ZZ poll, got %s", r.Method)
			}
			if auth := r.Header.Get("Authorization"); auth != "Bearer zz-key" {
				t.Fatalf("expected ZZ poll bearer key, got %q", auth)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"task_id":"zz-task-1","status":"completed"}`))
		case "/v1/videos/zz-task-1/content":
			contentAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte(videoBody))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{OpenAIBaseURL: "https://wrong.example"})
	provider.client = server.Client()

	poll, providerErr := provider.PollVideo(context.Background(), "zz-task-1", VideoGenerationInput{
		Model:               "video-ds-2.0-fast",
		ProviderBaseURL:     server.URL,
		ProviderAPIKey:      "zz-key",
		ProviderAPIEndpoint: "/v1/videos",
	})
	if providerErr != nil {
		t.Fatalf("PollVideo() provider error = %+v", providerErr)
	}
	if poll.TaskID != "zz-task-1" || poll.Status != VideoTaskSucceeded || poll.ProviderRequestID != "zz-task-1" {
		t.Fatalf("unexpected ZZ poll result: %+v", poll)
	}
	if poll.OutputBase64 != base64.StdEncoding.EncodeToString([]byte(videoBody)) || poll.MIMEType != "video/mp4" {
		t.Fatalf("unexpected ZZ downloaded video: %+v", poll)
	}
	if contentAuth != "Bearer zz-key" {
		t.Fatalf("expected ZZ content bearer key, got %q", contentAuth)
	}
}

func TestOpenAIProviderPollsZZStatusMapping(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus string
		wantReason string
	}{
		{name: "succeeded", body: `{"id":"zz-task-1","status":"succeeded"}`, wantStatus: VideoTaskSucceeded},
		{name: "in progress", body: `{"task_id":"zz-task-1","status":"in_progress"}`, wantStatus: VideoTaskInProgress},
		{name: "failed string", body: `{"task_id":"zz-task-1","status":"failed","error":"policy rejected"}`, wantStatus: VideoTaskFailed, wantReason: "policy rejected"},
		{name: "failed object", body: `{"task_id":"zz-task-1","status":"failed","error":{"message":"quota exceeded"}}`, wantStatus: VideoTaskFailed, wantReason: "quota exceeded"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/videos/zz-task-1/content" {
					w.Header().Set("Content-Type", "video/mp4")
					_, _ = w.Write([]byte("zz-status-video"))
					return
				}
				if r.URL.Path != "/v1/videos/zz-task-1" {
					t.Fatalf("expected ZZ poll path, got %s", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			provider := NewOpenAIProvider(Config{})
			provider.client = server.Client()

			poll, providerErr := provider.PollVideo(context.Background(), "zz-task-1", VideoGenerationInput{
				Model:               "video-ds-2.0-fast",
				ProviderBaseURL:     server.URL,
				ProviderAPIKey:      "zz-key",
				ProviderAPIEndpoint: "/v1/videos",
			})
			if providerErr != nil {
				t.Fatalf("PollVideo() provider error = %+v", providerErr)
			}
			if poll.Status != tt.wantStatus || poll.FailReason != tt.wantReason {
				t.Fatalf("unexpected ZZ status mapping: %+v", poll)
			}
		})
	}
}

func TestOpenAIProviderRejectsZZVideoWhenKeyMissing(t *testing.T) {
	provider := NewOpenAIProvider(Config{})
	_, providerErr := provider.SubmitVideo(context.Background(), VideoGenerationInput{
		Model:               "video-ds-2.0-fast",
		Prompt:              "missing key",
		ProviderAPIEndpoint: "/v1/videos",
	})
	if providerErr == nil {
		t.Fatal("expected provider error")
	}
	if providerErr.Code != "provider_api_key_missing" || providerErr.FailureStage != providerFailureStageVideoSubmitRequest {
		t.Fatalf("unexpected ZZ missing key error: %+v", providerErr)
	}
}

func TestOpenAIProviderGenerateMapsAdvancedParametersToPayload(t *testing.T) {
	var formValues map[string][]string
	var imageParts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/edits" {
			t.Fatalf("expected default images endpoint to switch to edits, got %s", r.URL.Path)
		}
		var imageBodies []string
		formValues, imageParts, imageBodies = readMultipartProviderRequest(t, r)
		if len(imageBodies) != 2 {
			t.Fatalf("expected source and reference image file bodies, got %#v", imageBodies)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"ZmFrZQ=="}]}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{
		OpenAIBaseURL: server.URL,
		OpenAIAPIKey:  "test-key",
	})
	provider.client = server.Client()

	_, providerErr := provider.Generate(context.Background(), ImageGenerationInput{
		Model:           "gpt-image-2",
		Prompt:          "repair this campus night photo",
		NegativePrompt:  "no text, no watermark",
		Size:            "1024x1024",
		Quality:         "high",
		StylePreset:     "写实",
		ToolMode:        "upscale",
		StyleStrength:   65,
		ReferenceWeight: 75,
		Seed:            "seed-42",
		SourceImage: &ReferenceImageInput{
			MIMEType:   "image/png",
			Base64Data: testPNGBase64(t, color.RGBA{R: 250, A: 255}),
		},
		ReferenceImages: []ReferenceImageInput{
			{MIMEType: "image/png", Base64Data: testPNGBase64(t, color.RGBA{B: 250, A: 255})},
		},
	})
	if providerErr != nil {
		t.Fatalf("Generate() provider error = %+v", providerErr)
	}

	if formValues["quality"][0] != "high" {
		t.Fatalf("expected quality high, got %#v", formValues["quality"])
	}
	promptText := formValues["prompt"][0]
	for _, expected := range []string{"repair this campus night photo", "no text, no watermark", "upscale", "写实", "65", "75", "seed-42"} {
		if !strings.Contains(promptText, expected) {
			t.Fatalf("expected composed prompt to contain %q, got %q", expected, promptText)
		}
	}
	if len(imageParts) != 2 || imageParts[0] != "image[]" || imageParts[1] != "image[]" {
		t.Fatalf("expected source and reference images to be sent as independent image[] parts, got %#v", imageParts)
	}
}

func TestComposeImagePromptAddsDefaultReferenceConstraint(t *testing.T) {
	prompt := composeImagePrompt(ImageGenerationInput{
		Prompt: "生成一张新海报",
		ReferenceImages: []ReferenceImageInput{
			{MIMEType: "image/png", Base64Data: "aW1hZ2U="},
		},
	})

	for _, expected := range []string{"上传图片是本次生成的主要视觉参考", "保留其中的主体、构图、风格或用户指定元素"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("expected default reference prompt to contain %q, got %q", expected, prompt)
		}
	}
}

func readMultipartProviderRequest(t *testing.T, r *http.Request) (map[string][]string, []string, []string) {
	t.Helper()

	if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data;") {
		t.Fatalf("expected multipart/form-data content type, got %q", r.Header.Get("Content-Type"))
	}
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		t.Fatalf("parse multipart provider request: %v", err)
	}

	partNames := []string{}
	partBodies := []string{}
	for _, name := range []string{"image", "image[]", "mask"} {
		files := r.MultipartForm.File[name]
		for _, fileHeader := range files {
			file, err := fileHeader.Open()
			if err != nil {
				t.Fatalf("open multipart file %q: %v", name, err)
			}
			body, err := io.ReadAll(file)
			_ = file.Close()
			if err != nil {
				t.Fatalf("read multipart file %q: %v", name, err)
			}
			partNames = append(partNames, name)
			partBodies = append(partBodies, string(body))
		}
	}
	return r.MultipartForm.Value, partNames, partBodies
}

func testPNGBase64(t *testing.T, fill color.RGBA) string {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: fill}, image.Point{}, draw.Src)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode test PNG: %v", err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func decodeTestDataURLImage(t *testing.T, value string) (image.Image, string) {
	t.Helper()

	prefix, data, ok := strings.Cut(value, ",")
	if !ok || !strings.HasPrefix(prefix, "data:image/") {
		t.Fatalf("expected image data URL, got %q", value)
	}
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		t.Fatalf("decode data URL image: %v", err)
	}
	img, format, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("decode image: %v", err)
	}
	return img, format
}

func testImageHasDarkPixel(img image.Image, rect image.Rectangle) bool {
	bounds := img.Bounds()
	rect = rect.Intersect(bounds)
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if a > 0 && r < 0x5000 && g < 0x5000 && b < 0x5000 {
				return true
			}
		}
	}
	return false
}

func TestProviderHTTPErrorReadsTopLevelProviderMessage(t *testing.T) {
	providerErr := providerHTTPError(http.StatusBadRequest, []byte(`{"code":"invalid_model","message":"模型未开通"}`), "req-1", providerFailureStageImageGenerationRequest)
	if providerErr.Code != "invalid_model" || providerErr.Message != "模型未开通" || providerErr.ProviderRequestID != "req-1" || providerErr.FailureStage != providerFailureStageImageGenerationRequest {
		t.Fatalf("unexpected provider error: %+v", providerErr)
	}
}

func TestProviderHTTPErrorNormalizesNestedTokenInvalidatedMessage(t *testing.T) {
	rawBody := []byte(`{"error":{"message":"{\n  \"error\": {\n    \"message\": \"Your authentication token has been invalidated. Please try signing in again.\",\n    \"type\": \"invalid_request_error\",\n    \"code\": \"token_invalidated\",\n    \"param\": null\n  },\n  \"status\": 401\n}（traceid: auth-1）","code":"<nil>"}}`)

	providerErr := providerHTTPError(http.StatusUnauthorized, rawBody, "req-auth", providerFailureStageImageGenerationRequest)

	if providerErr.Code != "token_invalidated" {
		t.Fatalf("expected token_invalidated code, got %+v", providerErr)
	}
	if providerErr.Message != "模型通道认证已失效，请检查 API Key 或切换可用通道（traceid: auth-1）" {
		t.Fatalf("expected actionable auth message, got %q", providerErr.Message)
	}
}

func TestOpenAIProviderGenerateCapturesImageRequestHTTPDiagnostics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream gateway failed"))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{
		OpenAIBaseURL: server.URL,
		OpenAIAPIKey:  "test-key",
	})
	provider.client = server.Client()

	_, providerErr := provider.Generate(context.Background(), ImageGenerationInput{
		Model: "gpt-image-2",
		Size:  "1024x1024",
	})
	if providerErr == nil {
		t.Fatal("expected provider error")
	}
	if providerErr.HTTPStatus != http.StatusBadGateway ||
		providerErr.Code != "provider_http_502" ||
		providerErr.Message != "upstream gateway failed" ||
		providerErr.FailureStage != providerFailureStageImageGenerationRequest {
		t.Fatalf("unexpected HTTP diagnostics: %+v", providerErr)
	}
}

func TestOpenAIProviderGenerateCapturesAssetFetchHTTPDiagnostics(t *testing.T) {
	assetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("asset gateway failed"))
	}))
	defer assetServer.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`{"data":[{"url":%q}]}`, assetServer.URL+"/image.png")))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{
		OpenAIBaseURL: server.URL,
		OpenAIAPIKey:  "test-key",
	})
	provider.client = server.Client()

	_, providerErr := provider.Generate(context.Background(), ImageGenerationInput{
		Model: "gpt-image-2",
		Size:  "1024x1024",
	})
	if providerErr == nil {
		t.Fatal("expected provider error")
	}
	if providerErr.HTTPStatus != http.StatusBadGateway ||
		providerErr.Code != "provider_asset_http_502" ||
		providerErr.Message != "asset gateway failed" ||
		providerErr.FailureStage != providerFailureStageProviderAssetFetch {
		t.Fatalf("unexpected asset diagnostics: %+v", providerErr)
	}
}

func TestOpenAIProviderGenerateReportsEmptyProviderJSONBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-request-id", "req-empty-json")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{
		OpenAIBaseURL: server.URL,
		OpenAIAPIKey:  "test-key",
	})
	provider.client = server.Client()

	_, providerErr := provider.Generate(context.Background(), ImageGenerationInput{
		Model: "gpt-image-2",
		Size:  "1024x1024",
	})
	if providerErr == nil {
		t.Fatal("expected provider error")
	}
	if providerErr.Code != "provider_decode_failed" || providerErr.Message != "provider returned empty response body" || providerErr.ProviderRequestID != "req-empty-json" {
		t.Fatalf("unexpected empty body provider error: %+v", providerErr)
	}
}

func TestOpenAIProviderGenerateReportsInvalidProviderJSONBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-request-id", "req-invalid-json")
		_, _ = w.Write([]byte(`{"data":[`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{
		OpenAIBaseURL: server.URL,
		OpenAIAPIKey:  "test-key",
	})
	provider.client = server.Client()

	_, providerErr := provider.Generate(context.Background(), ImageGenerationInput{
		Model: "gpt-image-2",
		Size:  "1024x1024",
	})
	if providerErr == nil {
		t.Fatal("expected provider error")
	}
	if providerErr.Code != "provider_decode_failed" || providerErr.Message != "provider returned invalid JSON" || providerErr.ProviderRequestID != "req-invalid-json" {
		t.Fatalf("unexpected invalid JSON provider error: %+v", providerErr)
	}
}

func TestOpenAIProviderGenerateClassifiesContextDeadlineExceededAsTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"ZmFrZQ=="}]}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(Config{
		OpenAIBaseURL: server.URL,
		OpenAIAPIKey:  "test-key",
	})
	provider.client = server.Client()

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	_, providerErr := provider.Generate(ctx, ImageGenerationInput{
		Model: "gpt-image-2",
		Size:  "1024x1024",
	})
	if providerErr == nil {
		t.Fatal("expected provider error")
	}
	if providerErr.Code != "provider_timeout" {
		t.Fatalf("expected provider_timeout, got %+v", providerErr)
	}
}
