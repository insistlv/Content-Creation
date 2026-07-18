package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"image-agent/internal/app"
	"image-agent/internal/app/ecommerce"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const (
	e2eUsername = "workspace_e2e"
	e2ePassword = "test-password"
)

type stubImageProvider struct {
	mu                          sync.Mutex
	sellingPointsFailedBySource map[string]bool
}

type e2eCommerceCompiler struct{}

func (e2eCommerceCompiler) Definition() ecommerce.RecipeDefinition {
	return ecommerce.RecipeDefinition{Key: "workspace-e2e-recipe", Title: "Workspace E2E Recipe", Pipeline: "general", Version: 1,
		AllowedOutputCounts: []int{2}, AspectRatios: []string{"1:1"}, QualityTiers: []string{"standard"}, MaxAttempts: 1}
}

func (e2eCommerceCompiler) Compile(_ context.Context, input ecommerce.CompileInput) ([]ecommerce.CompiledGenerationItem, error) {
	items := make([]ecommerce.CompiledGenerationItem, 0, input.OutputCount)
	for index := 0; index < input.OutputCount; index++ {
		items = append(items, ecommerce.CompiledGenerationItem{SKUID: input.PrimarySKUID, Pipeline: input.Pipeline,
			RecipeKey: input.RecipeKey, RecipeVersion: input.RecipeVersion, SlotKey: fmt.Sprintf("foundation-%d", index),
			Prompt: "workspace e2e", AspectRatio: input.AspectRatio,
			PricingVersion: input.PricingSnapshot.Version, PricingSnapshotID: input.PricingSnapshot.ID, EstimatedCredits: 1})
	}
	return items, nil
}

type e2eCommerceExecutor struct {
	workID uint
}

func (e *e2eCommerceExecutor) Key() ecommerce.ExecutorKey {
	return ecommerce.ExecutorKey{Pipeline: "general", RecipeKey: "workspace-e2e-recipe"}
}
func (e *e2eCommerceExecutor) Execute(_ context.Context, request ecommerce.ItemExecutionRequest) (ecommerce.ExecutionResult, *ecommerce.ExecutionFailure) {
	if request.Compiled.SlotKey == "foundation-1" && request.Item.ParentItemID == nil {
		return ecommerce.ExecutionResult{}, &ecommerce.ExecutionFailure{Code: "e2e_expected_failure", Message: "expected fake executor failure"}
	}
	return ecommerce.ExecutionResult{WorkID: e.workID, ActualCredits: 1, MetadataJSON: `{"executor":"workspace-e2e-fake"}`}, nil
}

func (p *stubImageProvider) Generate(ctx context.Context, input app.ImageGenerationInput) (app.ImageGenerationResult, *app.ProviderError) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.sellingPointsFailedBySource == nil {
		p.sellingPointsFailedBySource = map[string]bool{}
	}
	source := "no-reference"
	if len(input.ReferenceImages) > 0 {
		source = input.ReferenceImages[0].InputURL
	}
	if strings.Contains(input.Prompt, "章节=selling_points") && !p.sellingPointsFailedBySource[source] {
		p.sellingPointsFailedBySource[source] = true
		return app.ImageGenerationResult{}, &app.ProviderError{Code: "provider_policy_rejected", Message: "E2E 预期的单项生成失败"}
	}
	return app.ImageGenerationResult{
		Base64Image:          base64.StdEncoding.EncodeToString(e2ePNGBytes()),
		MIMEType:             "image/png",
		ProviderRequestID:    "workspace-e2e-stub",
		ProviderAttemptCount: 1,
	}, nil
}

func (*stubImageProvider) CommerceVisionConfigured(context.Context) (bool, error) { return true, nil }

func (*stubImageProvider) AnalyzeProduct(_ context.Context, input ecommerce.ProductAnalysisRequest) (string, error) {
	assetIDs, err := ecommerce.EncodeJSON(input.SourceAssetIDs)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`{"observed_facts":[{"field":"name","value":"E2E 保温杯","confidence":0.99,"source_asset_ids":%s},{"field":"category","value":"家居日用","confidence":0.97,"source_asset_ids":%s},{"field":"color_scheme","value":"白色主体与深色顶部配件","confidence":0.95,"source_asset_ids":%s}],"selling_points":["白色主体与深色顶部配件的对比外观清晰","白色杯身外轮廓简洁利落"],"forbidden_changes":["不得改变白色主体与深色顶部配件的配色关系","不得改变图片中可见的商品外轮廓"],"brand_tone":{"description":"简洁克制的现代家居视觉"},"missing_fields":["material","capacity","price","certification","efficacy"],"risk_notices":["价格、材质、容量、认证与功效必须由用户补录，不得由图片推断"],"suggested_sections":["hero","selling_points","material","detail","usage","specification","closing"]}`, assetIDs, assetIDs, assetIDs), nil
}

func main() {
	go startE2EObjectStore()
	root, err := os.MkdirTemp("", "image-agent-workspace-e2e-*")
	if err != nil {
		log.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(root)

	db, err := gorm.Open(sqlite.Open(filepath.Join(root, "app.db")), &gorm.Config{})
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}

	cfg := app.Config{
		AppBaseURL:                   "http://127.0.0.1:" + listenPort(),
		OpenAIAPIKey:                 "workspace-e2e-stub-key",
		OpenAIBaseURL:                "https://stub.invalid",
		JWTSecret:                    "workspace-e2e-secret",
		AdminUsername:                "admin",
		AdminPassword:                "AdminPass123",
		DatabaseURL:                  "postgres://unused",
		AssetStoragePath:             filepath.Join(root, "assets"),
		DefaultInviteQuota:           10,
		DefaultImageModel:            "gpt-image-2",
		AllowedImageModels:           []string{"gpt-image-2"},
		RequestTimeoutSeconds:        10,
		RateLimitWindowSeconds:       60,
		RateLimitMaxRequests:         100,
		UserSessionHours:             72,
		AdminSessionHours:            12,
		FrontendDistPath:             "web/dist",
		StartupDatabaseBootstrap:     true,
		StorageType:                  "local",
		AICommerceEnabled:            true,
		AICommerceWorkerEnabled:      true,
		AICommercePrivateStorageType: "oss",
		AICommerceOSSEndpoint:        "http://127.0.0.1:8890",
		AICommerceOSSAccessKeyID:     "workspace-e2e",
		AICommerceOSSAccessKeySecret: "workspace-e2e-secret",
		AICommerceOSSBucket:          "127",
		AICommerceOSSBasePath:        "commerce/",
	}

	application, err := app.NewWithDependencies(cfg, db, &stubImageProvider{})
	if err != nil {
		log.Fatalf("boot app: %v", err)
	}
	if err := seedWorkspaceE2E(db, cfg.AssetStoragePath); err != nil {
		log.Fatalf("seed e2e data: %v", err)
	}
	if err := application.RegisterCommerceRecipe(e2eCommerceCompiler{}); err != nil {
		log.Fatalf("register commerce recipe: %v", err)
	}
	var e2eWork app.Work
	if err := db.Where("prompt = ?", "E2E 历史作品").First(&e2eWork).Error; err != nil {
		log.Fatalf("load e2e work: %v", err)
	}
	if err := application.RegisterCommerceExecutor(&e2eCommerceExecutor{workID: e2eWork.ID}); err != nil {
		log.Fatalf("register commerce executor: %v", err)
	}

	addr := "127.0.0.1:" + listenPort()
	log.Printf("workspace e2e server listening on http://%s", addr)
	if err := http.ListenAndServe(addr, application.Router()); err != nil {
		log.Fatal(err)
	}
}

var e2eObjects sync.Map

type e2eObject struct {
	content []byte
	mime    string
}

func startE2EObjectStore() {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "http://127.0.0.1:15173")
		w.Header().Set("Access-Control-Allow-Methods", "POST, PUT, DELETE, HEAD, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		log.Printf("e2e object store %s %s", r.Method, r.URL.Path)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		switch r.Method {
		case http.MethodPost:
			if err := r.ParseMultipartForm(2 << 20); err != nil {
				http.Error(w, "bad upload", 400)
				return
			}
			key := r.FormValue("key")
			file, header, err := r.FormFile("file")
			if err != nil {
				http.Error(w, "missing file", 400)
				return
			}
			defer file.Close()
			content, _ := io.ReadAll(file)
			object := e2eObject{content: content, mime: header.Header.Get("Content-Type")}
			e2eObjects.Store("/"+key, object)
			e2eObjects.Store("/127/"+key, object)
			w.WriteHeader(http.StatusCreated)
		case http.MethodPut:
			content, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "bad object", http.StatusBadRequest)
				return
			}
			e2eObjects.Store(r.URL.Path, e2eObject{content: content, mime: r.Header.Get("Content-Type")})
			w.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			e2eObjects.Delete(r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		case http.MethodHead:
			value, ok := e2eObjects.Load(r.URL.Path)
			if !ok {
				http.NotFound(w, r)
				return
			}
			object := value.(e2eObject)
			w.Header().Set("Content-Length", strconv.Itoa(len(object.content)))
			w.Header().Set("Content-Type", object.mime)
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			value, ok := e2eObjects.Load(r.URL.Path)
			if !ok {
				http.NotFound(w, r)
				return
			}
			object := value.(e2eObject)
			w.Header().Set("Content-Type", object.mime)
			_, _ = w.Write(object.content)
		default:
			http.NotFound(w, r)
		}
	})
	if err := http.ListenAndServe("127.0.0.1:8890", handler); err != nil {
		log.Printf("e2e object store stopped: %v", err)
	}
}

func seedWorkspaceE2E(db *gorm.DB, assetRoot string) error {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(e2ePassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	var role app.UserRole
	if err := db.Where("code = ?", "standard_user").First(&role).Error; err != nil {
		return fmt.Errorf("load standard role: %w", err)
	}

	now := time.Now().UTC()
	user := app.User{
		Username:                 e2eUsername,
		DisplayName:              "Workspace E2E",
		PasswordHash:             string(passwordHash),
		Status:                   app.UserStatusActive,
		UserRoleID:               &role.ID,
		LoginNotificationEnabled: true,
		RiskNotificationEnabled:  true,
		LastLoginAt:              &now,
	}
	if err := db.Create(&user).Error; err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	if err := db.Create(&app.CreditBalance{UserID: user.ID, AvailableCredits: 100}).Error; err != nil {
		return fmt.Errorf("create balance: %w", err)
	}
	if err := db.Create(&app.CreditTransaction{
		UserID:       user.ID,
		Type:         "admin_adjust_add",
		Amount:       100,
		BalanceAfter: 100,
		Reason:       "Workspace E2E seed credits",
	}).Error; err != nil {
		return fmt.Errorf("create credit transaction: %w", err)
	}

	if err := writeAsset(assetRoot, "e2e/history.png", e2ePNGBytes()); err != nil {
		return err
	}
	record := app.GenerationRecord{
		UserID:          user.ID,
		Prompt:          "E2E 历史作品",
		AspectRatio:     "1:1",
		Quality:         "medium",
		ToolMode:        "generate",
		Status:          app.GenerationStatusSucceeded,
		Stage:           "succeeded",
		AssetKey:        "e2e/history.png",
		PreviewURL:      "/api/works/1/file",
		DownloadURL:     "/api/works/1/download",
		MIMEType:        "image/png",
		CreditsCost:     1,
		CreditsDeducted: true,
	}
	if err := db.Create(&record).Error; err != nil {
		return fmt.Errorf("create generation record: %w", err)
	}
	work := app.Work{
		UserID:             user.ID,
		GenerationRecordID: record.ID,
		Prompt:             record.Prompt,
		AspectRatio:        "1:1",
		Category:           "image",
		Model:              "gpt-image-2",
		Status:             app.GenerationStatusSucceeded,
		Visibility:         app.WorkVisibilityPrivate,
		AssetKey:           "e2e/history.png",
		MIMEType:           "image/png",
		ProviderRequestID:  "workspace-e2e-history",
	}
	if err := db.Create(&work).Error; err != nil {
		return fmt.Errorf("create work: %w", err)
	}
	work.PreviewURL = fmt.Sprintf("/api/works/%d/file", work.ID)
	work.DownloadURL = fmt.Sprintf("/api/works/%d/download", work.ID)
	if err := db.Save(&work).Error; err != nil {
		return fmt.Errorf("update work urls: %w", err)
	}
	record.WorkID = &work.ID
	record.PreviewURL = work.PreviewURL
	record.DownloadURL = work.DownloadURL
	if err := db.Save(&record).Error; err != nil {
		return fmt.Errorf("update record work: %w", err)
	}

	if err := writeAsset(assetRoot, "e2e/reference.png", e2ePNGBytes()); err != nil {
		return err
	}
	reference := app.ReferenceAsset{
		UserID:           user.ID,
		AssetKey:         "e2e/reference.png",
		PreviewURL:       "/api/reference-assets/1/file",
		MIMEType:         "image/png",
		OriginalFilename: "e2e-reference.png",
	}
	if err := db.Create(&reference).Error; err != nil {
		return fmt.Errorf("create reference: %w", err)
	}
	reference.PreviewURL = fmt.Sprintf("/api/reference-assets/%d/file", reference.ID)
	if err := db.Save(&reference).Error; err != nil {
		return fmt.Errorf("update reference url: %w", err)
	}
	if err := seedCommerceFoundationE2E(db, user.ID, work.ID); err != nil {
		return err
	}

	return nil
}

func seedCommerceFoundationE2E(db *gorm.DB, userID, workID uint) error {
	now := time.Now().UTC()
	product := ecommerce.CommerceProduct{UserID: userID, Name: "Foundation E2E 商品", Category: "服饰", Status: "active"}
	if err := db.Create(&product).Error; err != nil {
		return fmt.Errorf("create commerce product: %w", err)
	}
	sku := ecommerce.CommerceSKU{UserID: userID, ProductID: product.ID, Code: "E2E-SKU", Status: "active"}
	if err := db.Create(&sku).Error; err != nil {
		return fmt.Errorf("create commerce sku: %w", err)
	}
	project := ecommerce.CommerceProject{UserID: userID, ProductID: product.ID, DefaultSKUID: &sku.ID, Title: "Foundation E2E 项目", Pipeline: "general", Status: "active"}
	if err := db.Create(&project).Error; err != nil {
		return fmt.Errorf("create commerce project: %w", err)
	}
	lockedAt := now
	spec := ecommerce.CommerceCreativeSpec{UserID: userID, ProjectID: project.ID, Version: 1, Source: "workspace-e2e", Status: "confirmed", ProductFactsJSON: "{}", SellingPointsJSON: "[]", ForbiddenChangesJSON: "[]", BrandToneJSON: "{}", ShotPlanJSON: "[]", CopyBlocksJSON: "[]", RiskNoticesJSON: "[]", SourceAssetIDsJSON: "[]", LockedAt: &lockedAt}
	if err := db.Create(&spec).Error; err != nil {
		return fmt.Errorf("create commerce creative spec: %w", err)
	}
	project.ActiveCreativeSpecID = &spec.ID
	if err := db.Save(&project).Error; err != nil {
		return err
	}
	var reference app.ReferenceAsset
	if err := db.Where("user_id = ? AND original_filename = ?", userID, "e2e-reference.png").First(&reference).Error; err != nil {
		return err
	}
	asset := ecommerce.CommerceAsset{UserID: userID, ProjectID: project.ID, ReferenceAssetID: reference.ID, SKUID: &sku.ID, Role: "product", Lifecycle: "project", MetadataJSON: `{"source":"workspace-e2e-upload"}`}
	if err := db.Create(&asset).Error; err != nil {
		return fmt.Errorf("create categorized commerce asset: %w", err)
	}
	reservation := ecommerce.CommerceCreditReservation{UserID: userID, ProjectID: project.ID, ScopeType: "batch", ScopeKey: "e2e", IdempotencyKey: "e2e-reservation", Status: "completed", TotalCredits: 2, ReservedCredits: 0, SettledCredits: 1, ReleasedCredits: 1, CompletedAt: &now}
	if err := db.Create(&reservation).Error; err != nil {
		return fmt.Errorf("create commerce reservation: %w", err)
	}
	batch := ecommerce.CommerceGenerationBatch{UserID: userID, ProjectID: project.ID, ReservationID: &reservation.ID, PrimarySKUID: sku.ID, Pipeline: "general", RecipeKey: "workspace-e2e-recipe", RecipeVersion: 1, QualityTier: "standard", Status: ecommerce.CommerceBatchFailed, IdempotencyKey: "e2e-seeded-batch", TotalItems: 2, SucceededItems: 1, FailedItems: 1, EstimatedCredits: 2, ReservedCredits: 2, SettledCredits: 1, ReleasedCredits: 1, FinishedAt: &now}
	if err := db.Create(&batch).Error; err != nil {
		return fmt.Errorf("create commerce batch: %w", err)
	}
	reservation.BatchID = &batch.ID
	if err := db.Save(&reservation).Error; err != nil {
		return err
	}
	successJSON, _ := ecommerce.EncodeJSON(ecommerce.CompiledGenerationItem{SKUID: sku.ID, Pipeline: batch.Pipeline, RecipeKey: batch.RecipeKey, RecipeVersion: 1, SlotKey: "foundation-0", AspectRatio: "1:1", EstimatedCredits: 1})
	failedJSON, _ := ecommerce.EncodeJSON(ecommerce.CompiledGenerationItem{SKUID: sku.ID, Pipeline: batch.Pipeline, RecipeKey: batch.RecipeKey, RecipeVersion: 1, SlotKey: "foundation-1", AspectRatio: "1:1", EstimatedCredits: 1})
	success := ecommerce.CommerceGenerationItem{UserID: userID, ProjectID: project.ID, BatchID: batch.ID, ReservationID: reservation.ID, SKUID: sku.ID, SlotKey: "foundation-0", Pipeline: batch.Pipeline, RecipeKey: batch.RecipeKey, RecipeVersion: 1, QualityTier: "standard", IdempotencyKey: "e2e-item-success", Status: ecommerce.CommerceItemSucceeded, OutputSpecJSON: successJSON, EstimatedCredits: 1, ReservedCredits: 1, SettledCredits: 1, WorkID: &workID, FinishedAt: &now}
	failed := ecommerce.CommerceGenerationItem{UserID: userID, ProjectID: project.ID, BatchID: batch.ID, ReservationID: reservation.ID, SKUID: sku.ID, SlotKey: "foundation-1", Pipeline: batch.Pipeline, RecipeKey: batch.RecipeKey, RecipeVersion: 1, QualityTier: "standard", IdempotencyKey: "e2e-item-failed", Status: ecommerce.CommerceItemFailed, OutputSpecJSON: failedJSON, EstimatedCredits: 1, ReservedCredits: 1, ReleasedCredits: 1, ErrorCode: "e2e_expected_failure", ErrorMessage: "expected fake executor failure", FinishedAt: &now}
	if err := db.Create(&success).Error; err != nil {
		return err
	}
	if err := db.Create(&failed).Error; err != nil {
		return err
	}
	batchID, failedID := batch.ID, failed.ID
	failedJob := ecommerce.CommerceJob{UserID: userID, ProjectID: project.ID, BatchID: &batchID, GenerationItemID: &failedID, Kind: ecommerce.CommerceJobKindGenerateItem, Pipeline: batch.Pipeline, RecipeKey: batch.RecipeKey, Status: ecommerce.CommerceJobFailed, IdempotencyKey: "e2e-job-failed", MaxAttempts: 1, ErrorCode: failed.ErrorCode, ErrorMessage: failed.ErrorMessage, FinishedAt: &now}
	if err := db.Create(&failedJob).Error; err != nil {
		return err
	}
	settlement := ecommerce.CommerceCreditSettlement{UserID: userID, ProjectID: project.ID, BatchID: batch.ID, ReservationID: reservation.ID, GenerationItemID: success.ID, IdempotencyKey: "e2e-settlement", HeldCredits: 1, ActualCredits: 1, SettledCredits: 1}
	if err := db.Create(&settlement).Error; err != nil {
		return err
	}
	return nil
}

func writeAsset(root, key string, content []byte) error {
	path := filepath.Join(root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}

func e2ePNGBytes() []byte {
	canvas := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			canvas.SetRGBA(x, y, color.RGBA{R: uint8(70 + x*2), G: uint8(110 + y), B: 160, A: 255})
		}
	}
	var output bytes.Buffer
	_ = png.Encode(&output, canvas)
	return output.Bytes()
}

func listenPort() string {
	if port := os.Getenv("PORT"); port != "" {
		return port
	}
	return "8889"
}
