package app

import (
	"image-agent/internal/app/ecommerce"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenDatabaseSQLiteEnforcesForeignKeys(t *testing.T) {
	db, err := openDatabase("sqlite:" + filepath.Join(t.TempDir(), "foreign-keys.db"))
	if err != nil {
		t.Fatalf("openDatabase() error = %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB() error = %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	var enabled int
	if err := db.Raw("PRAGMA foreign_keys").Scan(&enabled).Error; err != nil {
		t.Fatalf("read PRAGMA foreign_keys: %v", err)
	}
	if enabled != 1 {
		t.Fatalf("PRAGMA foreign_keys = %d, want 1", enabled)
	}
}

func TestNewWithDependenciesSkipsStartupDatabaseBootstrap(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "app.db")
	db := openTestSQLiteDB(t, dbPath)

	cfg := Config{
		AppBaseURL:               "http://localhost:3000",
		OpenAIAPIKey:             "test-key",
		OpenAIBaseURL:            "https://api.openai.com",
		JWTSecret:                "test-secret",
		AdminUsername:            "admin",
		AdminPassword:            "admin-pass",
		DatabaseURL:              "postgres://test:test@localhost:5432/test?sslmode=disable",
		AssetStoragePath:         filepath.Join(t.TempDir(), "assets"),
		DefaultInviteQuota:       10,
		DefaultImageModel:        "gpt-image-2",
		AllowedImageModels:       []string{"gpt-image-2", "gpt-image-2-2026-04-21"},
		RequestTimeoutSeconds:    600,
		RateLimitWindowSeconds:   60,
		RateLimitMaxRequests:     20,
		UserSessionHours:         72,
		AdminSessionHours:        12,
		FrontendDistPath:         "web/dist",
		StartupDatabaseBootstrap: false,
	}

	application, err := NewWithDependencies(cfg, db, &stubProvider{})
	if err != nil {
		t.Fatalf("NewWithDependencies() error = %v", err)
	}
	if application.Router() == nil {
		t.Fatal("expected router to be initialized")
	}
	if db.Migrator().HasTable(&AppSettings{}) {
		t.Fatal("expected startup database bootstrap to skip AutoMigrate")
	}
	if db.Migrator().HasTable(&ecommerce.CommerceGenerationItem{}) {
		t.Fatal("expected startup database bootstrap skip to avoid Commerce migrations")
	}
}

func TestNewWithDependenciesSkipsStartupDatabaseMigrations(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "app.db")
	db := openTestSQLiteDB(t, dbPath)

	cfg := Config{
		AppBaseURL:                "http://localhost:3000",
		OpenAIAPIKey:              "test-key",
		OpenAIBaseURL:             "https://api.openai.com",
		JWTSecret:                 "test-secret",
		AdminUsername:             "admin",
		AdminPassword:             "admin-pass",
		DatabaseURL:               "postgres://test:test@localhost:5432/test?sslmode=disable",
		AssetStoragePath:          filepath.Join(t.TempDir(), "assets"),
		DefaultInviteQuota:        10,
		DefaultImageModel:         "gpt-image-2",
		AllowedImageModels:        []string{"gpt-image-2", "gpt-image-2-2026-04-21"},
		RequestTimeoutSeconds:     600,
		RateLimitWindowSeconds:    60,
		RateLimitMaxRequests:      20,
		UserSessionHours:          72,
		AdminSessionHours:         12,
		FrontendDistPath:          "web/dist",
		StartupDatabaseMigrations: StartupDatabaseMigrationsSkip,
		StartupDatabaseBootstrap:  true,
	}

	application, err := NewWithDependencies(cfg, db, &stubProvider{})
	if err != nil {
		t.Fatalf("NewWithDependencies() error = %v", err)
	}
	if application.Router() == nil {
		t.Fatal("expected router to be initialized")
	}
	for _, model := range []any{&AppSettings{}, &ModelCatalog{}, &SystemRequestLog{}, &GenerationRecord{}, &ecommerce.CommerceGenerationItem{}} {
		if db.Migrator().HasTable(model) {
			t.Fatalf("expected STARTUP_DATABASE_MIGRATIONS=skip to avoid creating %T", model)
		}
	}
}

func TestNewWithDependenciesBootstrapsCommerceSchema(t *testing.T) {
	db := openTestSQLiteDB(t, filepath.Join(t.TempDir(), "app.db"))
	cfg := testConfig(t)
	cfg.StartupDatabaseMigrations = StartupDatabaseMigrationsBootstrap
	cfg.StartupDatabaseBootstrap = true

	if _, err := NewWithDependencies(cfg, db, &stubProvider{}); err != nil {
		t.Fatalf("NewWithDependencies() error = %v", err)
	}
	for _, model := range []any{
		&ecommerce.CommerceGenerationBatch{},
		&ecommerce.CommerceGenerationItem{},
		&ecommerce.CommerceJob{},
	} {
		if !db.Migrator().HasTable(model) {
			t.Fatalf("expected bootstrap to create %T", model)
		}
	}
	for _, check := range []struct {
		model any
		name  string
	}{
		{&CreditBalance{}, "ReservedCredits"},
		{&CreditTransaction{}, "IdempotencyKey"},
		{&CreditTransaction{}, "ReservedAfter"},
		{&ReferenceAsset{}, "StorageScope"},
		{&Work{}, "StorageScope"},
		{&GenerationRecord{}, "StorageScope"},
		{&GenerationRecord{}, "ExecutionKey"},
		{&GenerationRecord{}, "ProviderRequestStarted"},
		{&GenerationRecord{}, "ProviderIdempotencySupported"},
	} {
		if !db.Migrator().HasColumn(check.model, check.name) {
			t.Fatalf("expected bootstrap to create %T.%s", check.model, check.name)
		}
	}
}

func TestNormalizeZZVideoDS20DataSkipsAmbiguousLegacyGenerationRecords(t *testing.T) {
	db := openTestSQLiteDB(t, filepath.Join(t.TempDir(), "app.db"))
	if err := db.AutoMigrate(&ModelConfig{}); err != nil {
		t.Fatalf("create model config table: %v", err)
	}
	if err := db.Create(&ModelConfig{
		Name:         "ZZ API Video DS 2.0 Fast",
		Type:         ModelConfigTypeVideo,
		RuntimeModel: zzVideoDSFastRuntimeModel,
	}).Error; err != nil {
		t.Fatalf("seed legacy ZZ model: %v", err)
	}
	if err := db.Exec(`CREATE TABLE generation_records (
		id integer primary key autoincrement,
		model_name text
	)`).Error; err != nil {
		t.Fatalf("create sparse legacy generation_records table: %v", err)
	}
	if err := db.Exec(`INSERT INTO generation_records (model_name) VALUES (?)`, "Video DS 2.0").Error; err != nil {
		t.Fatalf("seed ambiguous legacy generation record: %v", err)
	}

	if err := normalizeZZVideoDS20Data(db); err != nil {
		t.Fatalf("normalize sparse legacy records: %v", err)
	}

	var model ModelConfig
	if err := db.Where("runtime_model = ?", zzVideoDSFastRuntimeModel).First(&model).Error; err != nil {
		t.Fatalf("load normalized model: %v", err)
	}
	if model.Name != "DS 2.0" {
		t.Fatalf("expected target model name normalized, got %q", model.Name)
	}
	var generationName string
	if err := db.Raw("SELECT model_name FROM generation_records WHERE id = 1").Scan(&generationName).Error; err != nil {
		t.Fatalf("read ambiguous legacy generation name: %v", err)
	}
	if generationName != "Video DS 2.0" {
		t.Fatalf("expected ambiguous legacy generation name to stay unchanged, got %q", generationName)
	}
}

func TestNewWithDependenciesMigratesExistingSchemaWithoutFullBootstrap(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "app.db")
	db := openTestSQLiteDB(t, dbPath)
	if err := db.AutoMigrate(&AppSettings{}, &ModelConfig{}, &ModelCatalog{}, &ModelProvider{}, &ModelChannel{}); err != nil {
		t.Fatalf("create existing settings and model tables: %v", err)
	}
	settings := AppSettings{ID: 1, ActiveImageModel: "gpt-image-2"}
	if err := settings.SetAllowedImageModels([]string{"gpt-image-2"}); err != nil {
		t.Fatalf("set allowed models: %v", err)
	}
	if err := db.Create(&settings).Error; err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	legacyZZ := ModelConfig{
		Name:         "ZZ API Video DS 2.0 Fast",
		Type:         ModelConfigTypeVideo,
		Provider:     zzVideoProviderName,
		Status:       ModelConfigStatusOnline,
		Priority:     1,
		CostLabel:    "18-24 点/秒",
		Permission:   ModelConfigPermissionInternal,
		SortOrder:    39,
		RuntimeModel: zzVideoDSFastRuntimeModel,
		APIBaseURL:   zzVideoProviderBaseURL,
		APIEndpoint:  zzVideoEndpoint,
	}
	if err := db.Create(&legacyZZ).Error; err != nil {
		t.Fatalf("seed legacy ZZ video model: %v", err)
	}
	legacyCatalog := ModelCatalog{
		Name:       "ZZ API Video DS 2.0 Fast",
		Modality:   ModelConfigTypeVideo,
		Status:     ModelCenterStatusOnline,
		Visibility: ModelCenterVisibilityInternal,
		SortOrder:  39,
	}
	if err := db.Create(&legacyCatalog).Error; err != nil {
		t.Fatalf("seed legacy ZZ model catalog: %v", err)
	}
	legacyProvider := ModelProvider{
		Name:     zzVideoProviderName,
		Provider: zzVideoProviderCode,
		BaseURL:  zzVideoProviderBaseURL,
		Status:   ModelCenterStatusOnline,
	}
	if err := db.Create(&legacyProvider).Error; err != nil {
		t.Fatalf("seed legacy ZZ provider: %v", err)
	}
	legacyChannel := ModelChannel{
		ModelID:             legacyCatalog.ID,
		ProviderID:          legacyProvider.ID,
		LegacyModelConfigID: legacyZZ.ID,
		Name:                "Video DS 2.0",
		RuntimeModel:        zzVideoDSFastRuntimeModel,
		Endpoint:            zzVideoEndpoint,
		Status:              ModelCenterStatusOnline,
		HealthStatus:        ModelChannelHealthHealthy,
		Priority:            1,
	}
	if err := db.Create(&legacyChannel).Error; err != nil {
		t.Fatalf("seed legacy ZZ channel: %v", err)
	}
	if err := db.Exec(`CREATE TABLE generation_records (
		id integer primary key autoincrement,
		model_config_id integer,
		model_name text,
		runtime_model text,
		model text,
		credits_deducted numeric
	)`).Error; err != nil {
		t.Fatalf("create legacy generation_records table: %v", err)
	}
	if err := db.Exec(
		`INSERT INTO generation_records (model_config_id, model_name, runtime_model, model, credits_deducted) VALUES (?, ?, ?, ?, ?)`,
		legacyZZ.ID,
		"Video DS 2.0",
		zzVideoDSFastRuntimeModel,
		zzVideoDSFastRuntimeModel,
		true,
	).Error; err != nil {
		t.Fatalf("seed legacy generation record: %v", err)
	}
	if err := db.AutoMigrate(&VideoGenerationRecord{}); err != nil {
		t.Fatalf("create legacy video generation records table: %v", err)
	}
	if err := db.Create(&VideoGenerationRecord{
		ModelConfigID: legacyZZ.ID,
		ModelName:     "ZZ API Video DS 2.0 Fast",
		RuntimeModel:  zzVideoDSFastRuntimeModel,
		Status:        GenerationStatusSucceeded,
	}).Error; err != nil {
		t.Fatalf("seed legacy video generation record: %v", err)
	}

	cfg := Config{
		AppBaseURL:               "http://localhost:3000",
		OpenAIAPIKey:             "test-key",
		OpenAIBaseURL:            "https://api.openai.com",
		JWTSecret:                "test-secret",
		AdminUsername:            "admin",
		AdminPassword:            "admin-pass",
		DatabaseURL:              "postgres://test:test@localhost:5432/test?sslmode=disable",
		AssetStoragePath:         filepath.Join(t.TempDir(), "assets"),
		DefaultInviteQuota:       10,
		DefaultImageModel:        "gpt-image-2",
		AllowedImageModels:       []string{"gpt-image-2"},
		RequestTimeoutSeconds:    600,
		RateLimitWindowSeconds:   60,
		RateLimitMaxRequests:     20,
		UserSessionHours:         72,
		AdminSessionHours:        12,
		FrontendDistPath:         "web/dist",
		StartupDatabaseBootstrap: false,
	}

	if _, err := NewWithDependencies(cfg, db, &stubProvider{}); err != nil {
		t.Fatalf("NewWithDependencies() error = %v", err)
	}
	for _, model := range []any{
		&ecommerce.CommerceGenerationBatch{},
		&ecommerce.CommerceGenerationItem{},
		&ecommerce.CommerceJob{},
		&NovelVideoProject{},
		&NovelVideoAsset{},
		&NovelVideoCreature{},
		&NovelVideoEpisode{},
		&NovelVideoShot{},
		&NovelVideoShotRenderAttempt{},
		&NovelVideoShotImage{},
		&NovelVideoComposition{},
		&NovelVideoGrid{},
		&NovelVideoVersion{},
		&NovelVideoJob{},
	} {
		if !db.Migrator().HasTable(model) {
			t.Fatalf("expected existing startup migrations to create %T", model)
		}
	}
	for _, column := range []struct {
		model any
		name  string
	}{
		{&NovelVideoProject{}, "ContentMode"},
		{&NovelVideoProject{}, "SchemaVersion"},
		{&NovelVideoProject{}, "GenerationMode"},
		{&NovelVideoProject{}, "GridSize"},
		{&NovelVideoProject{}, "PlanningDraftJSON"},
		{&NovelVideoShot{}, "ScriptUnitType"},
		{&NovelVideoShot{}, "AssetRefsJSON"},
		{&NovelVideoShot{}, "GenerationSettingsJSON"},
		{&NovelVideoShotImage{}, "ReferenceAssetIDsJSON"},
		{&NovelVideoShotImage{}, "ActorIDsJSON"},
		{&NovelVideoShotImage{}, "LockLevel"},
		{&NovelVideoJob{}, "JobType"},
	} {
		if !db.Migrator().HasColumn(column.model, column.name) {
			t.Fatalf("expected existing startup migrations to create %T.%s", column.model, column.name)
		}
	}
	if !db.Migrator().HasColumn(&GenerationRecord{}, "CreditsCost") {
		t.Fatal("expected existing generation_records table to receive credits_cost")
	}
	for _, column := range []struct {
		model any
		name  string
	}{
		{&GenerationRecord{}, "StorageScope"},
		{&GenerationRecord{}, "ExecutionKey"},
		{&GenerationRecord{}, "ProviderRequestStarted"},
		{&GenerationRecord{}, "ProviderIdempotencySupported"},
	} {
		if !db.Migrator().HasColumn(column.model, column.name) {
			t.Fatalf("expected existing startup migrations to create %T.%s", column.model, column.name)
		}
	}
	var creditsCost int
	if err := db.Raw("SELECT credits_cost FROM generation_records WHERE id = 1").Scan(&creditsCost).Error; err != nil {
		t.Fatalf("read backfilled credits_cost: %v", err)
	}
	if creditsCost != 1 {
		t.Fatalf("expected deducted legacy record to backfill credits_cost=1, got %d", creditsCost)
	}
	var packageCount int64
	if err := db.Model(&Package{}).Where("is_active = ?", true).Count(&packageCount).Error; err != nil {
		t.Fatalf("count seeded packages: %v", err)
	}
	if packageCount != 6 {
		t.Fatalf("expected existing startup migrations to seed six active packages, got %d", packageCount)
	}
	var professional Package
	if err := db.Where("name = ?", "专业包").First(&professional).Error; err != nil {
		t.Fatalf("expected professional package seed in existing schema: %v", err)
	}
	if professional.PriceCents != 29800 || professional.Credits != 2588 || !professional.Recommended || professional.WechatVirtualProductID != "pointspack298" {
		t.Fatalf("unexpected professional package: %+v", professional)
	}
	var sora ModelConfig
	if err := db.Where("name = ?", "Sora2").First(&sora).Error; err != nil {
		t.Fatalf("expected Sora2 seed in existing model_configs table: %v", err)
	}
	if sora.RuntimeModel != "sora-2" || sora.APIEndpoint != "/v2/videos/generations" {
		t.Fatalf("unexpected Sora2 config: %+v", sora)
	}
	var grok ModelConfig
	if err := db.Where("name = ?", "Grok Imagine").First(&grok).Error; err != nil {
		t.Fatalf("expected Grok Imagine seed in existing model_configs table: %v", err)
	}
	if grok.RuntimeModel != wuyinGrokImagineRuntimeModel || grok.APIBaseURL != wuyinGrokImagineProviderBaseURL || grok.APIEndpoint != wuyinGrokImagineSubmitEndpoint || strings.TrimSpace(grok.APIKey) != "" {
		t.Fatalf("unexpected Grok Imagine config: %+v", grok)
	}
	var doubao ModelConfig
	if err := db.Where("name = ?", "Doubao Seedance 2.0 Mini").First(&doubao).Error; err != nil {
		t.Fatalf("expected Doubao Seedance 2.0 Mini seed in existing model_configs table: %v", err)
	}
	if doubao.Provider != "Volcengine Ark" ||
		doubao.RuntimeModel != "doubao-seed-2-0-mini-260428" ||
		doubao.APIBaseURL != "https://ark.cn-beijing.volces.com/api/v3" ||
		doubao.APIEndpoint != "/contents/generations/tasks" ||
		doubao.Permission != ModelConfigPermissionInternal ||
		doubao.Status != ModelConfigStatusOnline ||
		strings.TrimSpace(doubao.APIKey) != "" {
		t.Fatalf("unexpected Doubao Seedance config: %+v", doubao)
	}
	var zz ModelConfig
	if err := db.Where("name = ?", "DS 2.0").First(&zz).Error; err != nil {
		t.Fatalf("expected ZZ video seed in existing model_configs table: %v", err)
	}
	if zz.Provider != "ZZ API" ||
		zz.RuntimeModel != zzVideoDSFastRuntimeModel ||
		zz.APIBaseURL != zzVideoProviderBaseURL ||
		zz.APIEndpoint != zzVideoEndpoint ||
		zz.Permission != ModelConfigPermissionInternal ||
		zz.Status != ModelConfigStatusOnline ||
		strings.TrimSpace(zz.APIKey) != "" {
		t.Fatalf("unexpected ZZ video config: %+v", zz)
	}
	var oldZZCount int64
	if err := db.Model(&ModelConfig{}).
		Where("runtime_model = ? AND name IN ?", zzVideoDSFastRuntimeModel, []string{"ZZ API Video DS 2.0 Fast", "Video DS 2.0"}).
		Count(&oldZZCount).Error; err != nil {
		t.Fatalf("count old ZZ model names: %v", err)
	}
	if oldZZCount != 0 {
		t.Fatalf("expected old ZZ model names to be normalized, got %d", oldZZCount)
	}
	var zzChannel ModelChannel
	if err := db.Preload("Model").Preload("Provider").Where("legacy_model_config_id = ?", zz.ID).First(&zzChannel).Error; err != nil {
		t.Fatalf("expected ZZ video model center channel: %v", err)
	}
	if zzChannel.Name != "DS 2.0" ||
		zzChannel.Model.Name != "DS 2.0" ||
		zzChannel.RuntimeModel != zzVideoDSFastRuntimeModel ||
		zzChannel.Endpoint != zzVideoEndpoint ||
		zzChannel.Provider.Name != "ZZ API" ||
		zzChannel.Provider.Provider != "zz" ||
		zzChannel.Provider.BaseURL != zzVideoProviderBaseURL {
		t.Fatalf("unexpected ZZ model center channel/provider: %+v provider=%+v", zzChannel, zzChannel.Provider)
	}
	var generationName string
	if err := db.Raw("SELECT model_name FROM generation_records WHERE id = 1").Scan(&generationName).Error; err != nil {
		t.Fatalf("read normalized generation model name: %v", err)
	}
	if generationName != "DS 2.0" {
		t.Fatalf("expected generation record model_name normalized to DS 2.0, got %q", generationName)
	}
	var videoRecord VideoGenerationRecord
	if err := db.First(&videoRecord).Error; err != nil {
		t.Fatalf("load normalized video generation record: %v", err)
	}
	if videoRecord.ModelName != "DS 2.0" {
		t.Fatalf("expected video generation record model_name normalized to DS 2.0, got %+v", videoRecord)
	}
	var loadedSettings AppSettings
	if err := db.First(&loadedSettings, 1).Error; err != nil {
		t.Fatalf("load settings after seed: %v", err)
	}
	if loadedSettings.DefaultVideoModelID == nil || *loadedSettings.DefaultVideoModelID != grok.ID {
		t.Fatalf("expected Grok Imagine as default video model, got settings %+v and model %+v", loadedSettings, grok)
	}
}

func TestNewWithDependenciesMigratesLegacyWorksStorageScope(t *testing.T) {
	db := openTestSQLiteDB(t, filepath.Join(t.TempDir(), "app.db"))
	if err := db.Exec(`CREATE TABLE works (
		id integer primary key autoincrement,
		user_id integer,
		asset_key text
	)`).Error; err != nil {
		t.Fatalf("create legacy works table: %v", err)
	}
	cfg := testConfig(t)
	cfg.StartupDatabaseMigrations = StartupDatabaseMigrationsExisting
	cfg.StartupDatabaseBootstrap = false

	if _, err := NewWithDependencies(cfg, db, &stubProvider{}); err != nil {
		t.Fatalf("NewWithDependencies() error = %v", err)
	}
	if !db.Migrator().HasColumn(&Work{}, "StorageScope") {
		t.Fatal("expected existing works table to receive storage_scope")
	}
}

func TestNewWithDependenciesBackfillsRBACSeedsForExistingSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "app.db")
	db := openTestSQLiteDB(t, dbPath)
	if err := db.AutoMigrate(&Permission{}, &Role{}); err != nil {
		t.Fatalf("create existing rbac tables: %v", err)
	}
	cfg := testConfig(t)
	cfg.StartupDatabaseBootstrap = false

	if _, err := NewWithDependencies(cfg, db, &stubProvider{}); err != nil {
		t.Fatalf("NewWithDependencies() error = %v", err)
	}

	var permission Permission
	if err := db.Where("code = ?", "system_logs.read").First(&permission).Error; err != nil {
		t.Fatalf("expected system_logs.read permission to be backfilled: %v", err)
	}
	var auditor Role
	if err := db.Preload("Permissions").Where("code = ?", "auditor").First(&auditor).Error; err != nil {
		t.Fatalf("expected auditor role to be backfilled: %v", err)
	}
	if !roleHasPermission(auditor, "dashboard.read") {
		t.Fatalf("expected auditor role to be backfilled with read permissions, got %+v", auditor.Permissions)
	}
	// system_logs.read 为敏感能力，仅 super_admin 可用，不应下放给 auditor。
	if roleHasPermission(auditor, "system_logs.read") {
		t.Fatalf("expected auditor role NOT to include system_logs.read, got %+v", auditor.Permissions)
	}
}

func roleHasPermission(role Role, code string) bool {
	for _, permission := range role.Permissions {
		if permission.Code == code {
			return true
		}
	}
	return false
}

func TestDefaultModelConfigBackfillPreservesExplicitImageEndpoints(t *testing.T) {
	seed := ModelConfig{
		Name:         "DALL-E 3",
		Type:         ModelConfigTypeImage,
		RuntimeModel: "gpt-image-2",
		APIEndpoint:  "/v1/images/generations",
	}
	explicitEndpoints := []string{"chat", "/v1/chat/completions", "responses", "/v1/responses"}
	for _, endpoint := range explicitEndpoints {
		updates := defaultModelConfigBackfill(ModelConfig{
			Name:         "DALL-E 3",
			Type:         ModelConfigTypeImage,
			RuntimeModel: "gpt-image-2",
			APIEndpoint:  endpoint,
		}, seed)
		if _, ok := updates["api_endpoint"]; ok {
			t.Fatalf("expected explicit endpoint %q to be preserved, got updates %#v", endpoint, updates)
		}
	}
}

func TestDefaultModelConfigBackfillFillsEmptyImageEndpoint(t *testing.T) {
	seed := ModelConfig{
		Name:         "DALL-E 3",
		Type:         ModelConfigTypeImage,
		RuntimeModel: "gpt-image-2",
		APIEndpoint:  "/v1/images/generations",
	}
	updates := defaultModelConfigBackfill(ModelConfig{
		Name:         "DALL-E 3",
		Type:         ModelConfigTypeImage,
		RuntimeModel: "gpt-image-2",
	}, seed)
	if updates["api_endpoint"] != "/v1/images/generations" {
		t.Fatalf("expected empty endpoint to backfill images generation endpoint, got %#v", updates["api_endpoint"])
	}
}

func TestDefaultModelConfigBackfillFillsEmptyProviderBaseURL(t *testing.T) {
	seed := ModelConfig{
		Name:        "Grok Imagine",
		Type:        ModelConfigTypeVideo,
		APIBaseURL:  wuyinGrokImagineProviderBaseURL,
		APIEndpoint: wuyinGrokImagineSubmitEndpoint,
	}
	updates := defaultModelConfigBackfill(ModelConfig{
		Name: "Grok Imagine",
		Type: ModelConfigTypeVideo,
	}, seed)
	if updates["api_base_url"] != wuyinGrokImagineProviderBaseURL || updates["api_endpoint"] != wuyinGrokImagineSubmitEndpoint {
		t.Fatalf("expected empty Wuyin provider config to backfill, got %#v", updates)
	}
}

func TestStartupModelRoutingSurvivesSoftDeletedPreferredImageModel(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "app.db")
	db := openTestSQLiteDB(t, dbPath)
	if err := db.AutoMigrate(&AppSettings{}, &ModelConfig{}); err != nil {
		t.Fatalf("create existing settings and model tables: %v", err)
	}
	settings := AppSettings{ID: 1, ActiveImageModel: "gpt-image-2"}
	if err := settings.SetAllowedImageModels([]string{"gpt-image-2"}); err != nil {
		t.Fatalf("set allowed models: %v", err)
	}
	if err := db.Create(&settings).Error; err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	softDeletedDalle := ModelConfig{
		Name:         "DALL-E 3",
		Type:         ModelConfigTypeImage,
		Provider:     "OpenAI",
		Status:       ModelConfigStatusOnline,
		RuntimeModel: "gpt-image-2",
		APIEndpoint:  "/v1/images/generations",
	}
	if err := db.Create(&softDeletedDalle).Error; err != nil {
		t.Fatalf("seed preferred model: %v", err)
	}
	if err := db.Delete(&softDeletedDalle).Error; err != nil {
		t.Fatalf("soft delete preferred model: %v", err)
	}

	cfg := testConfig(t)
	cfg.DatabaseURL = "postgres://test:test@localhost:5432/test?sslmode=disable"
	cfg.AssetStoragePath = filepath.Join(t.TempDir(), "assets")

	if _, err := NewWithDependencies(cfg, db, &stubProvider{}); err != nil {
		t.Fatalf("NewWithDependencies() should not fail when DALL-E 3 is soft-deleted: %v", err)
	}
	var loaded AppSettings
	if err := db.First(&loaded, 1).Error; err != nil {
		t.Fatalf("load settings: %v", err)
	}
	if loaded.DefaultImageModelID == nil {
		t.Fatal("expected startup to choose an available replacement image model")
	}
	var replacement ModelConfig
	if err := db.First(&replacement, *loaded.DefaultImageModelID).Error; err != nil {
		t.Fatalf("load replacement model: %v", err)
	}
	if replacement.Name == "DALL-E 3" {
		t.Fatalf("expected routing replacement to avoid soft-deleted DALL-E 3, got %+v", replacement)
	}
}

func TestAspectRatioToSizeSupportsGPTBestRatios(t *testing.T) {
	cases := map[string]string{
		"21:9": "1536x1024",
		"16:9": "1536x1024",
		"4:3":  "1536x1024",
		"3:2":  "1536x1024",
		"1:1":  "1024x1024",
		"2:3":  "1024x1536",
		"3:4":  "1024x1536",
		"9:16": "1024x1536",
		"9:21": "1024x1536",
	}
	for ratio, expectedSize := range cases {
		size, ok := aspectRatioToSize(ratio)
		if !ok || size != expectedSize {
			t.Fatalf("expected %s to map to %s, got size=%q ok=%v", ratio, expectedSize, size, ok)
		}
	}
	if _, ok := aspectRatioToSize("5:4"); ok {
		t.Fatal("expected legacy 5:4 to be rejected")
	}
}
