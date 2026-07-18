package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"image-agent/internal/app/ecommerce"
)

type commerceVisionSignedStore struct{ url string }

func (s commerceVisionSignedStore) SaveBase64(string, string) (string, string, error) {
	return "", "", nil
}
func (s commerceVisionSignedStore) SaveBytes([]byte, string) (string, string, error) {
	return "", "", nil
}
func (s commerceVisionSignedStore) SaveStream(io.Reader, string) (string, string, error) {
	return "", "", nil
}
func (s commerceVisionSignedStore) Open(string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (s commerceVisionSignedStore) Read(string) ([]byte, error) { return nil, nil }
func (s commerceVisionSignedStore) ObjectMeta(string) (AssetObjectMeta, error) {
	return AssetObjectMeta{}, nil
}
func (s commerceVisionSignedStore) ReadRange(string, int64, int64) ([]byte, error) { return nil, nil }
func (s commerceVisionSignedStore) Delete(string) error                            { return nil }
func (s commerceVisionSignedStore) PublicURL(string) string                        { return "" }
func (s commerceVisionSignedStore) SignedReadURL(string, time.Duration) (string, error) {
	return s.url, nil
}

func TestModelCenterChatCommerceVisionRouting(t *testing.T) {
	if normalizeModelModality(ModelConfigTypeChat) != ModelConfigTypeChat || !validModelConfigType(ModelConfigTypeChat) {
		t.Fatal("chat modality is not supported")
	}
	testApp, _ := newTestApp(t, &stubProvider{})
	planning := ModelCatalog{Name: "Planning", Modality: ModelConfigTypeChat, Status: ModelCenterStatusOnline, CapabilityTags: []string{"commerce_planning"}}
	vision := ModelCatalog{Name: "Vision", Modality: ModelConfigTypeChat, Status: ModelCenterStatusOnline, CapabilityTags: []string{"vision", "commerce_vision"}}
	if err := testApp.db.Create(&planning).Error; err != nil {
		t.Fatal(err)
	}
	if err := testApp.db.Create(&vision).Error; err != nil {
		t.Fatal(err)
	}
	provider := ModelProvider{Name: "Vision Provider", BaseURL: "https://example.invalid", APIKey: "secret", Status: ModelCenterStatusOnline}
	testApp.db.Create(&provider)
	testApp.db.Create(&ModelChannel{ModelID: planning.ID, ProviderID: provider.ID, RuntimeModel: "planning", Endpoint: "/v1/chat/completions", Status: ModelCenterStatusOnline})
	visionChannel := ModelChannel{ModelID: vision.ID, ProviderID: provider.ID, RuntimeModel: "vision", Endpoint: "/v1/chat/completions", Status: ModelCenterStatusOnline}
	testApp.db.Create(&visionChannel)
	policy := ModelRoutingPolicy{Modality: ModelConfigTypeChat, DefaultModelID: vision.ID, RoutingEnabled: true, RoutingStrategy: ModelRoutingStrategyDefault, Source: ModelRoutingSourceModelCenter}
	testApp.db.Create(&policy)
	testApp.db.Create(&ModelRoutingEntry{PolicyID: policy.ID, ModelID: vision.ID, ChannelID: visionChannel.ID, Enabled: true, Priority: 1, Weight: 100})
	candidates, err := testApp.commerceVisionModelCandidates()
	if err != nil || len(candidates) != 1 || candidates[0].Model.ID != vision.ID {
		t.Fatalf("vision candidates=%#v err=%v", candidates, err)
	}
}

func TestCommerceVisionRoutingAuditsInvocation(t *testing.T) {
	var captured string
	responseAssetIDs := []uint{1, 2}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, fmt.Sprintf(`{"id":"provider-request-1","choices":[{"message":{"content":"{\"observed_facts\":[{\"field\":\"name\",\"value\":\"杯子\",\"confidence\":0.9,\"source_asset_ids\":[%d,%d]}],\"selling_points\":[\"便携\"],\"forbidden_changes\":[\"不得改变杯盖\"],\"brand_tone\":{\"description\":\"简洁\"},\"missing_fields\":[\"price\",\"capacity\",\"material\",\"certification\",\"efficacy\"],\"risk_notices\":[],\"suggested_sections\":[\"hero\"]}"}}]}`, responseAssetIDs[0], responseAssetIDs[1]))
	}))
	defer server.Close()
	testApp, db := newTestApp(t, &stubProvider{})
	signedURL := server.URL + "/signed-image?token=do-not-persist"
	testApp.assetStores.CommercePrivate = commerceVisionSignedStore{url: signedURL}
	model := ModelCatalog{Name: "Commerce Vision", Modality: ModelConfigTypeChat, Status: ModelCenterStatusOnline, CapabilityTags: []string{"vision", "commerce_vision"}}
	db.Create(&model)
	provider := ModelProvider{Name: "Provider", BaseURL: server.URL, APIKey: "api-key-do-not-persist", DefaultTimeoutSeconds: 5, Status: ModelCenterStatusOnline}
	db.Create(&provider)
	channel := ModelChannel{ModelID: model.ID, ProviderID: provider.ID, RuntimeModel: "vision-model", Endpoint: "/v1/chat/completions", Status: ModelCenterStatusOnline, HealthStatus: ModelChannelHealthHealthy}
	db.Create(&channel)
	policy := ModelRoutingPolicy{Modality: ModelConfigTypeChat, DefaultModelID: model.ID, RoutingEnabled: true, RoutingStrategy: ModelRoutingStrategyDefault, Source: ModelRoutingSourceModelCenter}
	db.Create(&policy)
	db.Create(&ModelRoutingEntry{PolicyID: policy.ID, ModelID: model.ID, ChannelID: channel.ID, Enabled: true, Priority: 1, Weight: 100})
	user := User{Username: "vision-audit-user", PasswordHash: "x"}
	db.Create(&user)
	product := ecommerce.CommerceProduct{UserID: user.ID, Name: "杯子", Status: "active"}
	db.Create(&product)
	project := ecommerce.CommerceProject{UserID: user.ID, ProductID: product.ID, Pipeline: "general", Status: "active"}
	db.Create(&project)
	assets := make([]ecommerce.CommerceAsset, 0, 2)
	for index, role := range []string{"product_front", "product_detail"} {
		objectKey := fmt.Sprintf("commerce/private/cup-%d.png", index)
		db.Create(&ecommerce.CommerceObjectGuard{UserID: user.ID, StorageScope: StorageScopeCommercePrivate, ObjectKey: objectKey, State: ecommerce.ObjectGuardStateActive})
		reference := ReferenceAsset{UserID: user.ID, AssetKey: objectKey, MIMEType: "image/png", StorageScope: StorageScopeCommercePrivate}
		db.Create(&reference)
		asset := ecommerce.CommerceAsset{UserID: user.ID, ProjectID: project.ID, ReferenceAssetID: reference.ID, Role: role, Lifecycle: ecommerce.AssetLifecycleProject}
		db.Create(&asset)
		assets = append(assets, asset)
		responseAssetIDs[index] = asset.ID
	}
	spec := ecommerce.CommerceCreativeSpec{UserID: user.ID, ProjectID: project.ID, Version: 1, Source: "vision", Status: "analyzing", AnalysisRequestHash: "hash"}
	db.Create(&spec)
	job := ecommerce.CommerceJob{UserID: user.ID, ProjectID: project.ID, Kind: ecommerce.CommerceJobKindProductAnalysis, SubjectType: ecommerce.CommerceSubjectCreativeSpec, SubjectID: &spec.ID, Status: ecommerce.CommerceJobRunning}
	db.Create(&job)
	analyzer := newCommerceVisionAnalyzerAdapter(testApp)
	raw, err := analyzer.AnalyzeProduct(context.Background(), ecommerce.ProductAnalysisRequest{JobID: job.ID, UserID: user.ID, ProjectID: project.ID, CreativeSpecID: spec.ID, SourceAssetIDs: []uint{assets[0].ID, assets[1].ID}})
	if err != nil || !strings.Contains(raw, `"observed_facts"`) || strings.Count(captured, signedURL) != 2 {
		t.Fatalf("analyze raw=%s err=%v request=%s", raw, err, captured)
	}
	var invocation ecommerce.CommerceAIInvocation
	if err := db.First(&invocation).Error; err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(invocation)
	for _, secret := range []string{signedURL, "do-not-persist", "api-key-do-not-persist", "signed-image"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("audit leaked %q: %s", secret, encoded)
		}
	}
	if invocation.JobID != job.ID || invocation.UserID != user.ID || invocation.ProjectID != project.ID || invocation.ChannelID != channel.ID {
		t.Fatalf("invocation=%#v", invocation)
	}
	created, err := testApp.commerceService.AnalyzeProduct(context.Background(), user.ID, project.ID, "adapter-lifecycle", ecommerce.AnalyzeProductInput{SourceAssetIDs: []uint{assets[0].ID, assets[1].ID}})
	if err != nil {
		t.Fatalf("API service analyze create: %v", err)
	}
	testApp.commerceVisionMu.Lock()
	productionAnalyzer := testApp.commerceVisionAnalyzer
	testApp.commerceVisionMu.Unlock()
	if _, err := ecommerce.NewProductAnalysisJobHandler(testApp.commerceService, productionAnalyzer).Handle(context.Background(), ecommerce.JobSnapshot{Job: created.Job}); err != nil {
		t.Fatalf("worker production adapter: %v", err)
	}
	loaded, _ := testApp.commerceService.GetCreativeSpec(context.Background(), user.ID, created.CreativeSpec.ID)
	if loaded.Status != "draft" || !strings.Contains(loaded.ObservedFactsJSON, `"field":"name"`) {
		t.Fatalf("worker lifecycle spec=%#v", loaded)
	}
}

func TestCommerceVisionStructuredOutputRequestContracts(t *testing.T) {
	for _, tc := range []struct {
		name, endpoint string
		response       string
	}{
		{name: "chat_completions", endpoint: "/v1/chat/completions", response: `{"id":"chat-1","choices":[{"message":{"content":"{}"}}]}`},
		{name: "responses", endpoint: "/v1/responses", response: `{"id":"response-1","output":[{"content":[{"text":"{}"}]}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requestErr := make(chan error, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					requestErr <- err
					http.Error(w, "bad request", http.StatusBadRequest)
					return
				}
				requestErr <- validateCommerceVisionStructuredRequest(body, strings.Contains(tc.endpoint, "responses"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tc.response)
			}))
			defer server.Close()

			candidate := modelCenterCandidate{
				Provider: ModelProvider{BaseURL: server.URL, DefaultTimeoutSeconds: 5},
				Channel:  ModelChannel{RuntimeModel: "vision-model", Endpoint: tc.endpoint},
			}
			_, _, err := callCommerceVisionChannel(context.Background(), candidate, ecommerce.ProductAnalysisRequest{SourceAssetIDs: []uint{7}}, []commerceSignedProductAsset{{ID: 7, URL: server.URL + "/asset.png"}})
			if err != nil {
				t.Fatalf("callCommerceVisionChannel: %v", err)
			}
			if err := <-requestErr; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCommerceVisionPromptOutputIsParserCompatible(t *testing.T) {
	validReport := `{"observed_facts":[{"field":"color","value":"白色","confidence":0.9,"source_asset_ids":[7]}],"selling_points":["配色简洁"],"forbidden_changes":["不得改变外形"],"brand_tone":{"description":"简洁克制"},"missing_fields":["price","capacity","material","certification","efficacy"],"risk_notices":["材质需用户确认"],"suggested_sections":["hero","detail"]}`
	materialReport := `{"observed_facts":[{"field":"material","value":"纯银","confidence":0.9,"source_asset_ids":[7]}],"selling_points":[],"forbidden_changes":[],"brand_tone":{"description":""},"missing_fields":["price","capacity","certification","efficacy"],"risk_notices":[],"suggested_sections":["hero"]}`
	for _, tc := range []struct {
		name, endpoint string
		responses      bool
	}{
		{name: "chat_completions", endpoint: "/v1/chat/completions"},
		{name: "responses", endpoint: "/v1/responses", responses: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, reportCase := range []struct {
				name, report string
				wantRejected bool
			}{
				{name: "english_field_and_section_with_chinese_value", report: validReport},
				{name: "prohibited_material_field", report: materialReport, wantRejected: true},
			} {
				t.Run(reportCase.name, func(t *testing.T) {
					requestErr := make(chan error, 1)
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						var body map[string]any
						if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
							requestErr <- err
							return
						}
						requestErr <- validateCommerceVisionStructuredRequest(body, tc.responses)
						encoded, _ := json.Marshal(reportCase.report)
						if tc.responses {
							_, _ = fmt.Fprintf(w, `{"id":"response-1","output":[{"content":[{"text":%s}]}]}`, encoded)
						} else {
							_, _ = fmt.Fprintf(w, `{"id":"chat-1","choices":[{"message":{"content":%s}}]}`, encoded)
						}
					}))
					defer server.Close()
					candidate := modelCenterCandidate{Provider: ModelProvider{BaseURL: server.URL, DefaultTimeoutSeconds: 5}, Channel: ModelChannel{RuntimeModel: "vision-model", Endpoint: tc.endpoint}}
					raw, _, err := callCommerceVisionChannel(context.Background(), candidate, ecommerce.ProductAnalysisRequest{SourceAssetIDs: []uint{7}}, []commerceSignedProductAsset{{ID: 7, URL: server.URL + "/asset.png"}})
					if err != nil {
						t.Fatalf("callCommerceVisionChannel: %v", err)
					}
					if err := <-requestErr; err != nil {
						t.Fatal(err)
					}
					report, parseErr := ecommerce.ParseProductReport(raw, []uint{7})
					if reportCase.wantRejected {
						if parseErr == nil {
							t.Fatalf("material field unexpectedly parsed: %#v", report)
						}
						return
					}
					if parseErr != nil {
						t.Fatalf("ParseProductReport: %v", parseErr)
					}
					if report.ObservedFacts[0].Field != "color" || report.ObservedFacts[0].Value != "白色" || report.SuggestedSections[0] != "hero" {
						t.Fatalf("parser-compatible report = %#v", report)
					}
				})
			}
		})
	}
}

func validateCommerceVisionStructuredRequest(body map[string]any, responses bool) error {
	var format map[string]any
	var prompt string
	if responses {
		input, _ := body["input"].([]any)
		if len(input) > 0 {
			message, _ := input[0].(map[string]any)
			content, _ := message["content"].([]any)
			if len(content) > 0 {
				text, _ := content[0].(map[string]any)
				prompt, _ = text["text"].(string)
			}
		}
		text, ok := body["text"].(map[string]any)
		if !ok {
			return fmt.Errorf("responses text wrapper missing: %#v", body)
		}
		format, _ = text["format"].(map[string]any)
		if _, nested := format["json_schema"]; nested {
			return fmt.Errorf("responses format must not use chat json_schema wrapper: %#v", format)
		}
	} else {
		messages, _ := body["messages"].([]any)
		if len(messages) > 0 {
			message, _ := messages[0].(map[string]any)
			content, _ := message["content"].([]any)
			if len(content) > 0 {
				text, _ := content[0].(map[string]any)
				prompt, _ = text["text"].(string)
			}
		}
		responseFormat, ok := body["response_format"].(map[string]any)
		if !ok || responseFormat["type"] != "json_schema" {
			return fmt.Errorf("chat response_format missing: %#v", body)
		}
		format, _ = responseFormat["json_schema"].(map[string]any)
		if format == nil {
			return fmt.Errorf("chat json_schema wrapper missing: %#v", responseFormat)
		}
	}
	for _, requirement := range []string{
		"Only observed_facts.value, selling_points, forbidden_changes, brand_tone.description, and risk_notices must use Simplified Chinese",
		"observed_facts.field, missing_fields, suggested_sections, all JSON keys, and enum values must remain the English values defined by the schema",
		"selling_points",
		"forbidden_changes",
		"brand_tone.description",
		"risk_notices",
	} {
		if !strings.Contains(prompt, requirement) {
			return fmt.Errorf("commerce vision prompt missing %q: %q", requirement, prompt)
		}
	}
	if format["name"] != "commerce_product_report" || format["strict"] != true || (!responses && body["response_format"].(map[string]any)["type"] != "json_schema") || (responses && format["type"] != "json_schema") {
		return fmt.Errorf("structured output metadata invalid: %#v", format)
	}
	schema, _ := format["schema"].(map[string]any)
	properties, _ := schema["properties"].(map[string]any)
	if schema["type"] != "object" || schema["additionalProperties"] != false || len(properties) != 9 {
		return fmt.Errorf("report schema incomplete: %#v", schema)
	}
	required, _ := schema["required"].([]any)
	if len(required) != 9 {
		return fmt.Errorf("report required fields incomplete: %#v", schema["required"])
	}
	observed, _ := properties["observed_facts"].(map[string]any)
	items, _ := observed["items"].(map[string]any)
	factProperties, _ := items["properties"].(map[string]any)
	factRequired, _ := items["required"].([]any)
	if observed["type"] != "array" || items["type"] != "object" || items["additionalProperties"] != false || len(factProperties) != 4 || len(factRequired) != 4 {
		return fmt.Errorf("observed fact schema incomplete: %#v", observed)
	}
	if factProperties["field"].(map[string]any)["type"] != "string" || factProperties["value"].(map[string]any)["type"] != "string" || factProperties["confidence"].(map[string]any)["type"] != "number" {
		return fmt.Errorf("observed fact field shapes invalid: %#v", factProperties)
	}
	sources := factProperties["source_asset_ids"].(map[string]any)
	if sources["type"] != "array" || sources["items"].(map[string]any)["type"] != "integer" {
		return fmt.Errorf("source asset shape invalid: %#v", sources)
	}
	for _, name := range []string{"selling_points", "forbidden_changes", "missing_fields", "risk_notices", "suggested_sections"} {
		field, _ := properties[name].(map[string]any)
		if field["type"] != "array" || field["items"].(map[string]any)["type"] != "string" || field["items"].(map[string]any)["minLength"] != float64(1) {
			return fmt.Errorf("%s shape invalid: %#v", name, field)
		}
	}
	brandTone, _ := properties["brand_tone"].(map[string]any)
	brandToneProperties, _ := brandTone["properties"].(map[string]any)
	if brandTone["type"] != "object" || brandTone["additionalProperties"] != false || len(brandToneProperties) != 1 || brandToneProperties["description"].(map[string]any)["type"] != "string" {
		return fmt.Errorf("brand_tone shape invalid: %#v", brandTone)
	}
	return nil
}
