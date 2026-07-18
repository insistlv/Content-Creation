package app

import (
	"bufio"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"image-agent/internal/app/ecommerce"
)

type App struct {
	cfg                    Config
	db                     *gorm.DB
	provider               ImageProvider
	videoProvider          VideoProvider
	musicProvider          MusicProvider
	smsSender              SMSSender
	router                 *gin.Engine
	rateLimiter            *RateLimiter
	concurrencyLimiter     *ConcurrencyLimiter
	imageGenLimiter        *UserConcurrencyLimiter
	assetStore             AssetStore
	assetStores            ScopedAssetStores
	alipayQuerier          alipayTradeQuerier
	wechatSessionExchanger wechatSessionExchanger
	wechatPhoneResolver    wechatPhoneResolver
	wechatPayClient        wechatPayClient
	wechatVirtualPayClient wechatVirtualPayClient
	startedAt              time.Time
	modelCenterSyncMu      sync.Mutex
	imageGenerationCancels sync.Map
	novelVideoFFmpegRunner FFmpegRunner
	cleanupStop            chan struct{}
	cleanupStopOnce        sync.Once
	commerceService        *ecommerce.Service
	commerceAssets         *ecommerce.AssetService
	commerceRecipes        *ecommerce.Registry
	commerceExecutors      *ecommerce.ExecutorRegistry
	commerceWorker         *ecommerce.Worker
	commerceWorkerDone     chan struct{}
	commerceVisionAnalyzer ecommerce.CommerceVisionAnalyzer
	commerceVisionMu       sync.Mutex
	imageQueueWorkerDone   chan struct{}
	secretStore            *SecretStore
}

const (
	userPresenceOnlineWindow  = 5 * time.Minute
	userPresenceTouchInterval = 30 * time.Second
	// maxConcurrentImageGenerationsPerUser 限制单用户同时进行的图片生成任务数。
	// 取 24：合法批量上限为 16，留出余量供批量与零散单发并存；超出返回 429 防止滥用打爆 provider 配额。
	maxConcurrentImageGenerationsPerUser = 24
)

func LoadConfigFromEnv() (Config, error) {
	if err := loadDotEnv(".env"); err != nil {
		return Config{}, fmt.Errorf("load .env: %w", err)
	}

	startupDatabaseMigrations, err := startupDatabaseMigrationsModeFromEnv()
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		AppBaseURL:                        os.Getenv("APP_BASE_URL"),
		OpenAIBaseURL:                     getenv("OPENAI_BASE_URL", "https://api.openai.com"),
		DeepSeekBaseURL:                   getenv("DEEPSEEK_BASE_URL", "https://api.deepseek.com"),
		DeepSeekPromptModel:               getenv("DEEPSEEK_PROMPT_MODEL", "deepseek-v4"),
		DeepSeekPromptTimeoutSeconds:      getenvInt("DEEPSEEK_PROMPT_TIMEOUT_SECONDS", 45),
		DeepSeekComposePlanTimeoutSeconds: getenvInt("DEEPSEEK_COMPOSE_PLAN_TIMEOUT_SECONDS", 12),
		AssetStoragePath:                  getenv("ASSET_STORAGE_PATH", "data/assets"),
		AppVersion:                        getenv("APP_VERSION", "local"),
		SystemStorageCapacityBytes:        getenvInt64("SYSTEM_STORAGE_CAPACITY_BYTES", 0),
		SystemCDNTrafficBytes:             getenvInt64("SYSTEM_CDN_TRAFFIC_BYTES", 0),
		SystemCDNTrafficLimitBytes:        getenvInt64("SYSTEM_CDN_TRAFFIC_LIMIT_BYTES", 0),
		SystemDailyGenerationLimit:        getenvInt64("SYSTEM_DAILY_GENERATION_LIMIT", 0),
		DefaultInviteQuota:                getenvInt("DEFAULT_INVITE_QUOTA", 10),
		DefaultImageModel:                 getenv("DEFAULT_IMAGE_MODEL", "gpt-image-2"),
		AllowedImageModels:                splitCSV(getenv("ALLOWED_IMAGE_MODELS", "gpt-image-2,gpt-image-2-2026-04-21")),
		RequestTimeoutSeconds:             getenvInt("REQUEST_TIMEOUT_SECONDS", defaultRequestTimeoutSeconds),
		RateLimitWindowSeconds:            getenvInt("RATE_LIMIT_WINDOW_SECONDS", 60),
		RateLimitMaxRequests:              getenvInt("RATE_LIMIT_MAX_REQUESTS", defaultRateLimitMaxRequests),
		UserSessionHours:                  getenvInt("USER_SESSION_HOURS", 72),
		AdminSessionHours:                 getenvInt("ADMIN_SESSION_HOURS", 12),
		UserRememberSessionHours:          getenvInt("USER_REMEMBER_SESSION_HOURS", 720),
		AdminRememberSessionHours:         getenvInt("ADMIN_REMEMBER_SESSION_HOURS", 168),
		FrontendDistPath:                  getenv("FRONTEND_DIST_PATH", "web/dist"),
		StartupDatabaseMigrations:         startupDatabaseMigrations,
		StartupDatabaseBootstrap:          startupDatabaseMigrations == StartupDatabaseMigrationsBootstrap,

		StorageType:                          getenv("STORAGE_TYPE", "local"),
		OSSEndpoint:                          os.Getenv("OSS_ENDPOINT"),
		OSSBucket:                            os.Getenv("OSS_BUCKET"),
		OSSPublicBaseURL:                     os.Getenv("OSS_PUBLIC_BASE_URL"),
		OSSBasePath:                          getenv("OSS_BASE_PATH", "assets/"),
		ReferenceAssetUploadMaxBytes:         getenvInt64("REFERENCE_ASSET_UPLOAD_MAX_BYTES", 50*1024*1024),
		ReferenceAssetUploadPolicyTTLSeconds: getenvInt("REFERENCE_ASSET_UPLOAD_POLICY_TTL_SECONDS", 600),
		AICommerceEnabled:                    getenvBool("AI_COMMERCE_ENABLED", false),
		AICommerceWorkerEnabled:              getenvBool("AI_COMMERCE_WORKER_ENABLED", false),
		AICommercePrivateStorageType:         getenv("AI_COMMERCE_PRIVATE_STORAGE_TYPE", "local"),
		AICommercePrivateAssetPath:           getenv("AI_COMMERCE_PRIVATE_ASSET_PATH", "data/commerce-assets"),
		AICommerceOSSEndpoint:                os.Getenv("AI_COMMERCE_OSS_ENDPOINT"),
		AICommerceOSSBucket:                  os.Getenv("AI_COMMERCE_OSS_BUCKET"),
		AICommerceOSSBasePath:                getenv("AI_COMMERCE_OSS_BASE_PATH", "commerce/"),
		AICommerceSignedURLTTLSeconds:        getenvInt("AI_COMMERCE_SIGNED_URL_TTL_SECONDS", 900),
		AICommerceTempRetentionHours:         getenvInt("AI_COMMERCE_TEMP_RETENTION_HOURS", 168),
		GenerationQueueCapacity:              getenvInt("GENERATION_QUEUE_CAPACITY", 500),
		GenerationUserPendingLimit:           getenvInt("GENERATION_USER_PENDING_LIMIT", 32),
		GenerationQueueTimeoutSeconds:        getenvInt("GENERATION_QUEUE_TIMEOUT_SECONDS", 1800),
		GenerationSpoolPath:                  getenv("GENERATION_SPOOL_PATH", "/opt/image-agent/shared/generation-spool"),
		GenerationSpoolMaxBytes:              getenvInt64("GENERATION_SPOOL_MAX_BYTES", 2*1024*1024*1024),

		SMSProvider:       getenv("SMS_PROVIDER", "aliyun"),
		AliyunSMSEndpoint: getenv("ALIYUN_SMS_ENDPOINT", "dysmsapi.aliyuncs.com"),

		AlipaySandbox: getenvBool("ALIPAY_SANDBOX", false),

		WechatPayNotifyURL:  os.Getenv("WECHAT_PAY_NOTIFY_URL"),
		WechatVirtualPayEnv: getenvInt("WECHAT_VIRTUAL_PAY_ENV", 0),
	}
	databaseURL, err := readEnvOrFile("DATABASE_URL")
	if err != nil {
		return Config{}, err
	}
	masterKeyEncoded, err := readEnvOrFile("APP_SECRETS_MASTER_KEY")
	if err != nil {
		return Config{}, err
	}
	masterKey, err := DecodeSecretsMasterKey(masterKeyEncoded)
	if err != nil {
		return Config{}, err
	}
	cfg.DatabaseURL = strings.TrimSpace(databaseURL)
	cfg.SecretsMasterKey = masterKey
	cfg.SecretsKeyVersion = getenvInt("APP_SECRETS_KEY_VERSION", 1)
	cfg.AlipayGateway = strings.TrimSpace(os.Getenv("ALIPAY_GATEWAY"))
	if cfg.AlipayGateway == "" {
		cfg.AlipayGateway = defaultAlipayGateway(cfg.AlipaySandbox)
	}

	switch {
	case cfg.AppBaseURL == "":
		return Config{}, errors.New("APP_BASE_URL is required")
	case cfg.DatabaseURL == "":
		return Config{}, errors.New("DATABASE_URL is required")
	}

	return cfg, nil
}

func startupDatabaseMigrationsModeFromEnv() (StartupDatabaseMigrationsMode, error) {
	if value := strings.TrimSpace(os.Getenv("STARTUP_DATABASE_MIGRATIONS")); value != "" {
		return normalizeStartupDatabaseMigrationsMode(value)
	}
	if getenvBool("STARTUP_DATABASE_BOOTSTRAP", false) {
		return StartupDatabaseMigrationsBootstrap, nil
	}
	return StartupDatabaseMigrationsExisting, nil
}

func resolveStartupDatabaseMigrationsMode(mode StartupDatabaseMigrationsMode, legacyBootstrap bool) (StartupDatabaseMigrationsMode, error) {
	if strings.TrimSpace(string(mode)) == "" {
		if legacyBootstrap {
			return StartupDatabaseMigrationsBootstrap, nil
		}
		return StartupDatabaseMigrationsExisting, nil
	}
	return normalizeStartupDatabaseMigrationsMode(string(mode))
}

func normalizeStartupDatabaseMigrationsMode(value string) (StartupDatabaseMigrationsMode, error) {
	mode := StartupDatabaseMigrationsMode(strings.ToLower(strings.TrimSpace(value)))
	switch mode {
	case StartupDatabaseMigrationsSkip, StartupDatabaseMigrationsExisting, StartupDatabaseMigrationsBootstrap:
		return mode, nil
	default:
		return "", fmt.Errorf("STARTUP_DATABASE_MIGRATIONS must be one of skip, existing, bootstrap; got %q", value)
	}
}

func loadDotEnv(path string) error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}

	for {
		candidate := filepath.Join(dir, path)
		loaded, err := loadDotEnvFile(candidate)
		if err != nil {
			return err
		}
		if loaded {
			return nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
	}
}

func loadDotEnvFile(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		key, rawValue, ok := strings.Cut(line, "=")
		if !ok {
			return true, fmt.Errorf("%s:%d invalid line", path, lineNumber)
		}

		key = strings.TrimSpace(key)
		if key == "" {
			return true, fmt.Errorf("%s:%d missing key", path, lineNumber)
		}
		if os.Getenv(key) != "" {
			continue
		}

		value := strings.TrimSpace(rawValue)
		if len(value) >= 2 {
			switch {
			case value[0] == '"' && value[len(value)-1] == '"':
				unquoted, err := strconv.Unquote(value)
				if err != nil {
					return true, fmt.Errorf("%s:%d invalid quoted value for %s: %w", path, lineNumber, key, err)
				}
				value = unquoted
			case value[0] == '\'' && value[len(value)-1] == '\'':
				value = value[1 : len(value)-1]
			}
		}

		if err := os.Setenv(key, value); err != nil {
			return true, fmt.Errorf("set %s from %s:%d: %w", key, path, lineNumber, err)
		}
	}

	return true, scanner.Err()
}

func New(cfg Config) (*App, error) {
	if cfg.StorageType != "oss" {
		if err := os.MkdirAll(cfg.AssetStoragePath, 0o755); err != nil {
			return nil, err
		}
	}

	db, err := openDatabase(cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	cfg, secretStore, err := prepareAppSecrets(context.Background(), db, cfg)
	if err != nil {
		return nil, err
	}
	if cfg.StorageType == "oss" {
		if cfg.OSSEndpoint == "" || cfg.OSSAccessKeyID == "" || cfg.OSSAccessKeySecret == "" || cfg.OSSBucket == "" || cfg.OSSPublicBaseURL == "" {
			return nil, errors.New("OSS storage is selected but its encrypted settings are incomplete")
		}
	}
	if err := validateCommercePrivateStorageConfig(cfg); err != nil {
		return nil, err
	}
	return newWithDependencies(cfg, db, NewOpenAIProvider(cfg), secretStore)
}

func postgresDialector(dsn string) gorm.Dialector {
	return postgres.New(postgres.Config{
		DSN:                  dsn,
		PreferSimpleProtocol: true,
	})
}

func openDatabase(databaseURL string) (*gorm.DB, error) {
	if path, ok := sqliteDatabasePath(databaseURL); ok {
		return gorm.Open(sqlite.Open(sqliteDSNWithForeignKeys(path)), &gorm.Config{})
	}
	return gorm.Open(postgresDialector(databaseURL), &gorm.Config{})
}

// OpenDatabase exposes the application's SQLite/PostgreSQL connection rules to
// one-shot administrative commands without duplicating DSN parsing.
func OpenDatabase(databaseURL string) (*gorm.DB, error) {
	return openDatabase(databaseURL)
}

func sqliteDSNWithForeignKeys(dsn string) string {
	base, rawQuery, hasQuery := strings.Cut(dsn, "?")
	if !hasQuery {
		return dsn + "?_foreign_keys=on"
	}
	parts := strings.Split(rawQuery, "&")
	filtered := parts[:0]
	for _, part := range parts {
		key, _, _ := strings.Cut(part, "=")
		switch strings.ToLower(key) {
		case "_foreign_keys", "_fk":
			continue
		default:
			filtered = append(filtered, part)
		}
	}
	filtered = append(filtered, "_foreign_keys=on")
	return base + "?" + strings.Join(filtered, "&")
}

// sqliteDatabasePath detects a SQLite DATABASE_URL for local development.
// Production keeps using PostgreSQL via the default postgres:// connection string.
func sqliteDatabasePath(databaseURL string) (string, bool) {
	trimmed := strings.TrimSpace(databaseURL)
	switch {
	case strings.HasPrefix(trimmed, "sqlite://"):
		return strings.TrimPrefix(trimmed, "sqlite://"), true
	case strings.HasPrefix(trimmed, "sqlite:"):
		return strings.TrimPrefix(trimmed, "sqlite:"), true
	case strings.HasPrefix(trimmed, "file:"):
		return trimmed, true
	}
	lower := strings.ToLower(trimmed)
	if strings.HasSuffix(lower, ".db") || strings.HasSuffix(lower, ".sqlite") || strings.HasSuffix(lower, ".sqlite3") {
		return trimmed, true
	}
	return "", false
}

func NewWithDependencies(cfg Config, db *gorm.DB, provider ImageProvider) (*App, error) {
	return newWithDependencies(cfg, db, provider, nil)
}

func newWithDependencies(cfg Config, db *gorm.DB, provider ImageProvider, secretStore *SecretStore) (*App, error) {
	if len(cfg.AllowedImageModels) == 0 {
		cfg.AllowedImageModels = []string{cfg.DefaultImageModel}
	}
	startupDatabaseMigrations, err := resolveStartupDatabaseMigrationsMode(cfg.StartupDatabaseMigrations, cfg.StartupDatabaseBootstrap)
	if err != nil {
		return nil, err
	}
	cfg.StartupDatabaseMigrations = startupDatabaseMigrations
	cfg.StartupDatabaseBootstrap = startupDatabaseMigrations == StartupDatabaseMigrationsBootstrap

	var assetStore AssetStore
	switch cfg.StorageType {
	case "oss":
		store, err := NewOSSAssetStore(cfg.OSSEndpoint, cfg.OSSAccessKeyID, cfg.OSSAccessKeySecret, cfg.OSSBucket, cfg.OSSBasePath, cfg.OSSPublicBaseURL)
		if err != nil {
			return nil, fmt.Errorf("create OSS asset store: %w", err)
		}
		assetStore = store
	default:
		if cfg.AssetStoragePath == "" {
			cfg.AssetStoragePath = "data/assets"
		}
		if err := os.MkdirAll(cfg.AssetStoragePath, 0o755); err != nil {
			return nil, err
		}
		assetStore = NewLocalAssetStore(cfg.AssetStoragePath)
	}
	commercePrivateStore, err := buildCommercePrivateAssetStore(cfg)
	if err != nil {
		return nil, err
	}

	videoProvider, _ := provider.(VideoProvider)
	if videoProvider == nil {
		videoProvider = NewOpenAIProvider(cfg)
	}
	musicProvider, _ := provider.(MusicProvider)
	if musicProvider == nil {
		musicProvider = NewOpenAIProvider(cfg)
	}

	commerceRepository := ecommerce.NewRepository(db)
	commerceRecipes := ecommerce.NewRegistry()
	commerceService := ecommerce.NewService(commerceRepository, ecommerce.ReferenceAssetOwnershipResolverFunc(
		func(ctx context.Context, userID, assetID uint) (bool, error) {
			var count int64
			err := db.WithContext(ctx).Model(&ReferenceAsset{}).
				Where("id = ? AND user_id = ?", assetID, userID).
				Count(&count).Error
			return count == 1, err
		},
	))
	commerceService.ConfigureBatchInfrastructure(
		commerceRecipes,
		newCommerceCreditLedger(),
		ecommerce.NewGormPricingSnapshotStore(),
		nil,
	)
	app := &App{
		cfg:                    cfg,
		db:                     db,
		provider:               provider,
		videoProvider:          videoProvider,
		musicProvider:          musicProvider,
		smsSender:              NewAliyunSMSSender(cfg),
		rateLimiter:            NewRateLimiter(),
		concurrencyLimiter:     NewConcurrencyLimiter(),
		imageGenLimiter:        NewUserConcurrencyLimiter(maxConcurrentImageGenerationsPerUser),
		assetStore:             assetStore,
		assetStores:            ScopedAssetStores{Default: assetStore, CommercePrivate: commercePrivateStore},
		startedAt:              time.Now(),
		novelVideoFFmpegRunner: executableFFmpegRunner{},
		cleanupStop:            make(chan struct{}),
		imageQueueWorkerDone:   make(chan struct{}),
		commerceService:        commerceService,
		commerceAssets:         ecommerce.NewAssetService(commerceRepository),
		commerceRecipes:        commerceRecipes,
		secretStore:            secretStore,
	}
	commerceService.ConfigureBatchInfrastructure(
		commerceRecipes,
		newCommerceCreditLedger(),
		ecommerce.NewGormPricingSnapshotStore(),
		commercePricingSnapshotProvider{app: app},
	)
	commerceVisionAnalyzer := ecommerce.CommerceVisionAnalyzer(newCommerceVisionAnalyzerAdapter(app))
	if analyzer, ok := provider.(ecommerce.CommerceVisionAnalyzer); ok {
		commerceVisionAnalyzer = analyzer
	}
	app.commerceVisionAnalyzer = commerceVisionAnalyzer
	commerceService.ConfigureVisionAnalyzer(commerceVisionAnalyzer)
	app.alipayQuerier = httpAlipayQuerier{app: app}
	app.wechatSessionExchanger = httpWechatSessionExchanger{app: app}
	app.wechatPhoneResolver = &httpWechatPhoneResolver{app: app}
	app.wechatPayClient = httpWechatPayClient{app: app}
	app.wechatVirtualPayClient = &httpWechatVirtualPayClient{app: app}

	switch startupDatabaseMigrations {
	case StartupDatabaseMigrationsBootstrap:
		if err := app.migrateAndSeed(); err != nil {
			return nil, err
		}
	case StartupDatabaseMigrationsExisting:
		if err := app.migrateExistingSchema(); err != nil {
			return nil, err
		}
	case StartupDatabaseMigrationsSkip:
	}
	if err := app.verifyCommerceWorkerSchema(); err != nil {
		return nil, err
	}
	if err := app.configureCommerceProductDetailRecipe(); err != nil {
		return nil, err
	}
	if err := app.prepareGenerationSpool(); err != nil {
		return nil, err
	}
	if err := app.recoverInterruptedImageGenerations(); err != nil {
		return nil, err
	}
	if err := app.cleanupOldSystemRequestLogs(time.Now()); err != nil {
		return nil, err
	}
	if err := app.startCommerceWorker(); err != nil {
		return nil, err
	}
	app.startSystemRequestLogCleanupTask()
	app.startImageGenerationTimeoutCleanupTask()
	app.startImageGenerationQueueWorker()
	app.router = app.setupRouter()
	return app, nil
}

func (a *App) Router() *gin.Engine {
	return a.router
}

func (a *App) Close() error {
	if a == nil {
		return nil
	}
	if a.cleanupStop != nil {
		a.cleanupStopOnce.Do(func() {
			close(a.cleanupStop)
		})
	}
	if a.commerceWorkerDone != nil {
		<-a.commerceWorkerDone
	}
	if a.imageQueueWorkerDone != nil {
		<-a.imageQueueWorkerDone
	}
	if a.db == nil {
		return nil
	}
	sqlDB, err := a.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (a *App) migrateAndSeed() error {
	if err := a.db.AutoMigrate(
		&AppSettings{},
		&SecretRecord{},
		&ModelConfig{},
		&ModelCatalog{},
		&ModelProvider{},
		&ModelChannel{},
		&ModelRoutingPolicy{},
		&ModelRoutingEntry{},
		&Permission{},
		&Role{},
		&AdminUser{},
		&AdminSession{},
		&AdminAuditLog{},
		&SystemRequestLog{},
		&SystemAnnouncement{},
		&AnnouncementReceipt{},
		&Invite{},
		&InviteRedemption{},
		&UserRole{},
		&User{},
		&AuthVerificationCode{},
		&AuthCaptchaChallenge{},
		&UserSession{},
		&CreditBalance{},
		&CreditTransaction{},
		&Package{},
		&PromptTemplate{},
		&InspirationRecommendation{},
		&VideoStylePreset{},
		&UserVideoStyleTemplate{},
		&CoupleAlbumOption{},
		&PurchaseIntent{},
		&PurchaseIntentNote{},
		&FinanceOrder{},
		&PaymentRecord{},
		&FinanceRefund{},
		&FinanceInvoice{},
		&Work{},
		&VideoSoundtrack{},
		&CoupleAlbum{},
		&CoupleAlbumPage{},
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
		&ReferenceAsset{},
		&GenerationRecord{},
		&ImageGenerationJob{},
		&ImageExecutionLease{},
		&VideoGenerationRecord{},
		&ModelCallAttempt{},
		&GenerationEventLog{},
		&GenerationReferenceAsset{},
		&VideoConversation{},
		&VideoConversationMessage{},
		&ContentSafetyReview{},
		&ContentReport{},
		&AlgorithmDisclosure{},
		&UserConsent{},
		&AIContentMark{},
		&AlgorithmIncident{},
	); err != nil {
		return err
	}
	if err := ecommerce.ApplyFoundationMigrations(context.Background(), a.db); err != nil {
		return fmt.Errorf("apply commerce foundation migrations: %w", err)
	}
	if err := a.ensureModelConfigAPIColumns(); err != nil {
		return err
	}
	if err := a.ensurePackagePresentationColumns(); err != nil {
		return err
	}

	var count int64
	if err := a.db.Model(&AppSettings{}).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		settings := AppSettings{
			ID:                     1,
			ActiveImageModel:       a.cfg.DefaultImageModel,
			ModelRoutingEnabled:    true,
			ModelRoutingStrategy:   ModelRoutingStrategyDefault,
			ModelConcurrencyLimit:  4,
			RequestTimeoutSeconds:  a.cfg.RequestTimeoutSeconds,
			DefaultInviteQuota:     a.cfg.DefaultInviteQuota,
			RateLimitWindowSeconds: a.cfg.RateLimitWindowSeconds,
			RateLimitMaxRequests:   a.cfg.RateLimitMaxRequests,
		}
		if err := settings.SetAllowedImageModels(a.cfg.AllowedImageModels); err != nil {
			return err
		}
		a.initializeSystemSettings(&settings)
		if err := a.db.Create(&settings).Error; err != nil {
			return err
		}
	}

	var settings AppSettings
	if err := a.db.First(&settings, 1).Error; err != nil {
		return err
	}

	needsSave := false
	if settings.RequestTimeoutSeconds <= 0 || settings.RequestTimeoutSeconds == legacyRequestTimeoutSeconds {
		settings.RequestTimeoutSeconds = a.cfg.RequestTimeoutSeconds
		needsSave = true
	}
	if settings.RateLimitMaxRequests <= 0 || settings.RateLimitMaxRequests == legacyRateLimitMaxRequests {
		settings.RateLimitMaxRequests = defaultRateLimitMaxRequests
		needsSave = true
	}
	if settings.AllowedImageModelsJSON == "" {
		if err := settings.SetAllowedImageModels(a.cfg.AllowedImageModels); err != nil {
			return err
		}
		needsSave = true
	}
	if settings.ActiveImageModel == "" {
		settings.ActiveImageModel = a.cfg.DefaultImageModel
		needsSave = true
	}
	if strings.TrimSpace(settings.ModelRoutingStrategy) == "" {
		settings.ModelRoutingStrategy = ModelRoutingStrategyDefault
		needsSave = true
	}
	if !settings.SystemSettingsInitialized {
		a.initializeSystemSettings(&settings)
		needsSave = true
	}
	if needsSave {
		if err := a.db.Save(&settings).Error; err != nil {
			return err
		}
	}

	if err := a.seedUserRoles(); err != nil {
		return err
	}
	if err := a.seedPackages(); err != nil {
		return err
	}
	if err := a.seedPromptTemplates(); err != nil {
		return err
	}
	if err := a.seedInspirationRecommendations(); err != nil {
		return err
	}
	if err := a.seedCoupleAlbumOptions(); err != nil {
		return err
	}
	if err := a.seedModelConfigs(); err != nil {
		return err
	}
	if err := a.ensureModelCenter(); err != nil {
		return err
	}
	if err := a.normalizeBailinAIConcurrency(); err != nil {
		return err
	}
	if err := a.normalizePublicImageModelCreditCosts(); err != nil {
		return err
	}
	if err := a.seedRBACAndBootstrapAdmin(); err != nil {
		return err
	}
	if err := a.backfillFinanceOrders(); err != nil {
		return err
	}
	if err := a.backfillVideoGenerationRecords(); err != nil {
		log.Printf("video generation record backfill failed: %v", err)
	}
	if err := a.backfillVideoConversations(); err != nil {
		return fmt.Errorf("backfill video conversations: %w", err)
	}

	return nil
}

func (a *App) migrateExistingSchema() error {
	migrator := a.db.Migrator()

	if migrator.HasTable(&AppSettings{}) {
		if err := a.db.AutoMigrate(&AppSettings{}); err != nil {
			return err
		}
	}
	if migrator.HasTable(&ModelConfig{}) {
		if err := a.db.AutoMigrate(&ModelConfig{}); err != nil {
			return err
		}
	}
	if err := a.db.AutoMigrate(&ModelCatalog{}, &ModelProvider{}, &ModelChannel{}, &ModelRoutingPolicy{}, &ModelRoutingEntry{}); err != nil {
		return err
	}
	if err := a.normalizeBailinAIConcurrency(); err != nil {
		return err
	}
	if err := a.normalizePublicImageModelCreditCosts(); err != nil {
		return err
	}
	if migrator.HasTable(&GenerationRecord{}) {
		if err := a.migrateExistingGenerationTables(migrator); err != nil {
			return err
		}
		if err := a.db.Model(&GenerationRecord{}).
			Where("credits_deducted = ? AND (credits_cost IS NULL OR credits_cost = ?)", true, 0).
			Update("credits_cost", 1).Error; err != nil {
			return err
		}
	}
	if err := a.db.AutoMigrate(&ImageGenerationJob{}, &ImageExecutionLease{}); err != nil {
		return err
	}
	if err := a.db.AutoMigrate(&ModelCallAttempt{}); err != nil {
		return err
	}
	if err := a.db.AutoMigrate(&GenerationEventLog{}); err != nil {
		return err
	}
	if err := a.db.AutoMigrate(&SystemRequestLog{}); err != nil {
		return err
	}
	if err := a.db.AutoMigrate(&SystemAnnouncement{}, &AnnouncementReceipt{}); err != nil {
		return err
	}
	if err := a.db.AutoMigrate(&Package{}); err != nil {
		return err
	}
	if err := a.seedPackages(); err != nil {
		return err
	}
	if migrator.HasTable(&Permission{}) && migrator.HasTable(&Role{}) {
		if err := a.db.AutoMigrate(&Permission{}, &Role{}); err != nil {
			return err
		}
		if err := a.seedPermissionsAndRoles(); err != nil {
			return err
		}
	}
	if migrator.HasTable(&User{}) {
		if err := a.db.AutoMigrate(&User{}); err != nil {
			return err
		}
	}
	if err := a.db.AutoMigrate(&AuthVerificationCode{}); err != nil {
		return err
	}
	if err := a.db.AutoMigrate(&AuthCaptchaChallenge{}); err != nil {
		return err
	}
	if migrator.HasTable(&UserSession{}) {
		if err := a.db.AutoMigrate(&UserSession{}); err != nil {
			return err
		}
	}
	if migrator.HasTable(&CreditBalance{}) {
		if err := a.db.AutoMigrate(&CreditBalance{}); err != nil {
			return err
		}
	}
	if migrator.HasTable(&CreditTransaction{}) {
		if err := a.db.AutoMigrate(&CreditTransaction{}); err != nil {
			return err
		}
	}
	if migrator.HasTable(&ReferenceAsset{}) {
		// Production reference_assets has a legacy trigger on storage_scope.
		// No columns from this release are added to this table, and PostgreSQL
		// rejects GORM's equivalent varchar normalization while the trigger exists.
		if a.db.Dialector.Name() != "postgres" {
			if err := a.db.AutoMigrate(&ReferenceAsset{}); err != nil {
				return err
			}
		}
	}
	if migrator.HasTable(&Work{}) {
		// works uses the same legacy storage_scope trigger as reference_assets.
		// Keep PostgreSQL existing-schema migrations additive for this release.
		if a.db.Dialector.Name() != "postgres" {
			if err := a.db.AutoMigrate(&Work{}); err != nil {
				return err
			}
		}
	}
	if err := a.db.AutoMigrate(&VideoSoundtrack{}); err != nil {
		return err
	}
	if migrator.HasTable(&GenerationReferenceAsset{}) {
		if err := a.db.AutoMigrate(&GenerationReferenceAsset{}); err != nil {
			return err
		}
	}
	if err := a.db.AutoMigrate(&VideoConversation{}, &VideoConversationMessage{}); err != nil {
		return err
	}
	if err := a.db.AutoMigrate(&CoupleAlbum{}, &CoupleAlbumPage{}); err != nil {
		return err
	}
	if err := a.db.AutoMigrate(&NovelVideoProject{}, &NovelVideoAsset{}, &NovelVideoCreature{}, &NovelVideoEpisode{}, &NovelVideoShot{}, &NovelVideoShotRenderAttempt{}, &NovelVideoShotImage{}, &NovelVideoComposition{}, &NovelVideoGrid{}, &NovelVideoVersion{}, &NovelVideoJob{}); err != nil {
		return err
	}
	if err := a.db.AutoMigrate(&PromptTemplate{}); err != nil {
		return err
	}
	if err := a.seedPromptTemplates(); err != nil {
		return err
	}
	if err := a.db.AutoMigrate(&InspirationRecommendation{}); err != nil {
		return err
	}
	if err := a.seedInspirationRecommendations(); err != nil {
		return err
	}
	if err := a.db.AutoMigrate(&VideoStylePreset{}, &UserVideoStyleTemplate{}); err != nil {
		return err
	}
	if err := a.seedVideoStylePresets(); err != nil {
		return err
	}
	if err := a.db.AutoMigrate(&CoupleAlbumOption{}); err != nil {
		return err
	}
	if err := a.seedCoupleAlbumOptions(); err != nil {
		return err
	}
	if migrator.HasTable(&FinanceOrder{}) {
		if err := a.db.AutoMigrate(&FinanceOrder{}); err != nil {
			return err
		}
	}
	if err := a.db.AutoMigrate(&PaymentRecord{}); err != nil {
		return err
	}
	if err := a.db.AutoMigrate(&ContentSafetyReview{}, &ContentReport{}, &AlgorithmDisclosure{}, &UserConsent{}, &AIContentMark{}, &AlgorithmIncident{}); err != nil {
		return err
	}
	if err := ecommerce.ApplyFoundationMigrations(context.Background(), a.db); err != nil {
		return fmt.Errorf("apply commerce foundation migrations: %w", err)
	}

	if migrator.HasTable(&AppSettings{}) && migrator.HasTable(&ModelConfig{}) {
		var settingsCount int64
		if err := a.db.Model(&AppSettings{}).Count(&settingsCount).Error; err != nil {
			return err
		}
		if settingsCount > 0 {
			if err := a.seedModelConfigs(); err != nil {
				return err
			}
			if err := a.ensureModelCenter(); err != nil {
				return err
			}
		}
	}
	if err := a.backfillVideoGenerationRecords(); err != nil {
		log.Printf("video generation record backfill failed: %v", err)
	}
	if err := a.backfillVideoConversations(); err != nil {
		return fmt.Errorf("backfill video conversations: %w", err)
	}
	return nil
}

func (a *App) migrateExistingGenerationTables(migrator gorm.Migrator) error {
	// PostgreSQL rejects ALTER TYPE when a legacy trigger references the column,
	// even when GORM is only trying to normalize an equivalent varchar type.
	// Existing production databases only need the additive conversation-workspace
	// columns here, so avoid rewriting unrelated legacy generation columns.
	if a.db.Dialector.Name() != "postgres" {
		return a.db.AutoMigrate(&GenerationRecord{}, &VideoGenerationRecord{})
	}

	if err := addMissingColumns(migrator, &GenerationRecord{}, "Progress", "RequestFingerprint"); err != nil {
		return err
	}
	if !migrator.HasIndex(&GenerationRecord{}, "RequestFingerprint") {
		if err := migrator.CreateIndex(&GenerationRecord{}, "RequestFingerprint"); err != nil {
			return err
		}
	}

	if !migrator.HasTable(&VideoGenerationRecord{}) {
		return a.db.AutoMigrate(&VideoGenerationRecord{})
	}
	if err := addMissingColumns(migrator, &VideoGenerationRecord{}, "ConversationID", "Progress"); err != nil {
		return err
	}
	if !migrator.HasIndex(&VideoGenerationRecord{}, "ConversationID") {
		if err := migrator.CreateIndex(&VideoGenerationRecord{}, "ConversationID"); err != nil {
			return err
		}
	}
	return nil
}

func addMissingColumns(migrator gorm.Migrator, model any, fields ...string) error {
	for _, field := range fields {
		if migrator.HasColumn(model, field) {
			continue
		}
		if err := migrator.AddColumn(model, field); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) normalizePublicImageModelCreditCosts() error {
	return a.db.Model(&ModelCatalog{}).
		Where("modality = ? AND visibility = ? AND default_credits_cost > ?", ModelConfigTypeImage, ModelCenterVisibilityPublic, 1).
		Update("default_credits_cost", 1).Error
}

func (a *App) seedUserRoles() error {
	roles := []UserRole{
		{Code: "standard_user", Name: "普通用户", Description: "默认应用用户", Color: "blue"},
		{Code: "standard_admin", Name: "普通管理员", Description: "运营侧管理员标签", Color: "violet"},
		{Code: "operations_admin", Name: "运营管理员", Description: "负责用户运营和点数处理", Color: "emerald"},
		{Code: "content_reviewer", Name: "内容审核", Description: "负责内容审核标记", Color: "amber"},
		{Code: "super_admin", Name: "超级管理员", Description: "最高权限运营标签", Color: "rose"},
	}
	return a.db.Transaction(func(tx *gorm.DB) error {
		for _, seed := range roles {
			role := UserRole{Code: seed.Code}
			if err := tx.Where("code = ?", seed.Code).FirstOrCreate(&role, seed).Error; err != nil {
				return err
			}
			updates := map[string]any{}
			if role.Name == "" {
				updates["name"] = seed.Name
			}
			if role.Description == "" && seed.Description != "" {
				updates["description"] = seed.Description
			}
			if role.Color == "" && seed.Color != "" {
				updates["color"] = seed.Color
			}
			if len(updates) > 0 {
				if err := tx.Model(&role).Updates(updates).Error; err != nil {
					return err
				}
			}
		}

		var standard UserRole
		if err := tx.Where("code = ?", "standard_user").First(&standard).Error; err != nil {
			return err
		}
		return tx.Model(&User{}).Where("user_role_id IS NULL").Update("user_role_id", standard.ID).Error
	})
}

func (a *App) seedPackages() error {
	defaults := []Package{
		{Name: "体验包", Description: "适合新用户快速体验图片与视频创作", PriceLabel: "10 元", PriceCents: 1000, Credits: 50, ValidDays: 30, Audience: "新手体验", Tags: []string{"体验", "入门"}, Icon: "sparkles", Theme: "blue", Badge: "体验", WechatVirtualProductID: "pointspack10", SortOrder: 10, IsActive: true},
		{Name: "入门包", Description: "适合个人轻量创作和短周期内容制作", PriceLabel: "30 元", PriceCents: 3000, Credits: 188, ValidDays: 90, Audience: "个人创作者", Tags: []string{"入门", "常备"}, Icon: "leaf", Theme: "green", Badge: "入门", WechatVirtualProductID: "pointspack30", SortOrder: 20, IsActive: true},
		{Name: "常用包", Description: "适合日常高频创作和多项目内容排期", PriceLabel: "100 元", PriceCents: 10000, Credits: 688, ValidDays: 180, Audience: "高频创作者", Tags: []string{"常用", "高频"}, Icon: "zap", Theme: "orange", Badge: "常用", WechatVirtualProductID: "pointspack100", SortOrder: 30, IsActive: true},
		{Name: "进阶包", Description: "适合进阶商业创作和稳定批量生成", PriceLabel: "198 元", PriceCents: 19800, Credits: 1488, ValidDays: 365, Audience: "商业创作者", Tags: []string{"进阶", "商用"}, Icon: "rocket", Theme: "violet", Badge: "进阶", WechatVirtualProductID: "pointspack198", SortOrder: 40, IsActive: true},
		{Name: "专业包", Description: "适合专业创作者持续产出和多场景交付", PriceLabel: "298 元", PriceCents: 29800, Credits: 2588, ValidDays: 365, Audience: "专业创作者", Tags: []string{"专业", "推荐"}, Icon: "badge-check", Theme: "rose", Badge: "推荐", Recommended: true, WechatVirtualProductID: "pointspack298", SortOrder: 50, IsActive: true},
		{Name: "旗舰包", Description: "适合长期储备、大批量生成和团队内容生产", PriceLabel: "648 元", PriceCents: 64800, Credits: 6188, ValidDays: 365, Audience: "团队 / 工作室", Tags: []string{"旗舰", "最划算"}, Icon: "crown", Theme: "gold", Badge: "最划算", WechatVirtualProductID: "pointspack648", SortOrder: 60, IsActive: true},
	}
	return a.db.Transaction(func(tx *gorm.DB) error {
		for i := range defaults {
			defaults[i].Features = defaultPackageFeatures(defaults[i], i)
			defaults[i].Benefits = defaultPackageBenefits(defaults[i], i)
			defaults[i].NormalizeTags()
			defaults[i].NormalizePresentation()
			var existing Package
			err := tx.Unscoped().Where("name = ?", defaults[i].Name).First(&existing).Error
			if err == nil {
				if existing.DeletedAt.Valid {
					continue
				}
				updates := defaultPackageBackfill(existing, defaults[i])
				if len(updates) > 0 {
					if err := tx.Model(&existing).Updates(updates).Error; err != nil {
						return err
					}
				}
				continue
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if legacy, ok, err := findLegacyPackageForDefault(tx, defaults[i]); err != nil {
				return err
			} else if ok {
				if legacy.DeletedAt.Valid {
					continue
				}
				if err := tx.Model(&legacy).Updates(defaultPackageUpgrade(legacy, defaults[i])).Error; err != nil {
					return err
				}
				continue
			}
			if err := tx.Create(&defaults[i]).Error; err != nil {
				return err
			}
		}
		return deactivateLegacyTeamPackage(tx)
	})
}

func defaultPackageFeatures(pkg Package, index int) []string {
	features := []string{
		"支持图片生成",
		"支持视频生成",
		"支持参考图 / 图生视频",
		"作品入库与历史管理",
		"失败任务不扣点，以生成页实时提示为准",
	}
	name := pkg.Name
	credits := pkg.Credits
	if nameContainsPackageTier(name, "旗舰") || credits >= 5000 || index >= 5 {
		return append(features, "商用授权", "长期储备", "最高优先")
	}
	if nameContainsPackageTier(name, "专业") || credits >= 2500 || index == 4 {
		return append(features, "商用授权", "专业交付", "更高优先")
	}
	if nameContainsPackageTier(name, "进阶") || credits >= 1400 || index == 3 {
		return append(features, "商用授权", "批量生成")
	}
	if nameContainsPackageTier(name, "常用") || credits >= 600 || index == 2 {
		return append(features, "商用授权")
	}
	if nameContainsPackageTier(name, "入门") || credits >= 188 || index == 1 {
		return append(features, "优先队列")
	}
	return append(features, "基础排队")
}

func defaultPackageBenefits(pkg Package, index int) []PackageBenefit {
	commercial := "—"
	if nameContainsPackageTier(pkg.Name, "常用") || nameContainsPackageTier(pkg.Name, "进阶") ||
		nameContainsPackageTier(pkg.Name, "专业") || nameContainsPackageTier(pkg.Name, "旗舰") ||
		pkg.Credits >= 600 || index >= 2 {
		commercial = "✓"
	}
	return []PackageBenefit{
		{Label: "点数", Value: strconv.Itoa(pkg.Credits) + " 点"},
		{Label: "图片生成", Value: "✓"},
		{Label: "视频生成", Value: "✓"},
		{Label: "图生视频 / 参考图能力", Value: "✓"},
		{Label: "高清下载", Value: "✓"},
		{Label: "私有作品库", Value: "✓"},
		{Label: "队列优先级", Value: defaultPackageQueuePriority(pkg, index)},
		{Label: "商用授权", Value: commercial},
		{Label: "适合人群", Value: pkg.Audience},
	}
}

func defaultPackageQueuePriority(pkg Package, index int) string {
	name := pkg.Name
	credits := pkg.Credits
	if nameContainsPackageTier(name, "旗舰") || credits >= 5000 || index >= 5 {
		return "最高优先"
	}
	if nameContainsPackageTier(name, "专业") || nameContainsPackageTier(name, "进阶") || credits >= 1400 || index >= 3 {
		return "更高优先"
	}
	if nameContainsPackageTier(name, "入门") || nameContainsPackageTier(name, "常用") || credits >= 188 || index >= 1 {
		return "优先"
	}
	return "普通"
}

func nameContainsPackageTier(name, tier string) bool {
	return strings.Contains(strings.TrimSpace(name), tier)
}

func defaultPackageBackfill(existing Package, seed Package) map[string]any {
	updates := map[string]any{}
	if strings.TrimSpace(existing.Description) == "" && strings.TrimSpace(seed.Description) != "" {
		updates["description"] = seed.Description
	}
	if strings.TrimSpace(existing.PriceLabel) == "" && strings.TrimSpace(seed.PriceLabel) != "" {
		updates["price_label"] = seed.PriceLabel
	}
	if existing.PriceCents <= 0 {
		if cents, ok := parsePriceCents(existing.PriceLabel); ok {
			updates["price_cents"] = cents
		} else {
			updates["price_cents"] = seed.PriceCents
		}
	}
	if existing.ValidDays <= 0 {
		updates["valid_days"] = seed.ValidDays
	}
	if strings.TrimSpace(existing.Audience) == "" && strings.TrimSpace(seed.Audience) != "" {
		updates["audience"] = seed.Audience
	}
	if strings.TrimSpace(existing.TagsJSON) == "" && strings.TrimSpace(seed.TagsJSON) != "" {
		updates["tags_json"] = seed.TagsJSON
	}
	if strings.TrimSpace(existing.Icon) == "" && strings.TrimSpace(seed.Icon) != "" {
		updates["icon"] = seed.Icon
	}
	if strings.TrimSpace(existing.Theme) == "" && strings.TrimSpace(seed.Theme) != "" {
		updates["theme"] = seed.Theme
	}
	if strings.TrimSpace(existing.Badge) == "" && strings.TrimSpace(seed.Badge) != "" {
		updates["badge"] = seed.Badge
	}
	if existing.SortOrder == 0 {
		updates["sort_order"] = seed.SortOrder
	}
	if strings.TrimSpace(existing.WechatVirtualProductID) == "" && strings.TrimSpace(seed.WechatVirtualProductID) != "" {
		updates["wechat_virtual_product_id"] = seed.WechatVirtualProductID
	}
	if len(existing.Features) == 0 && strings.TrimSpace(seed.FeaturesJSON) != "" {
		updates["features_json"] = seed.FeaturesJSON
	}
	if len(existing.Benefits) == 0 && strings.TrimSpace(seed.BenefitsJSON) != "" {
		updates["benefits_json"] = seed.BenefitsJSON
	}
	return updates
}

func findLegacyPackageForDefault(tx *gorm.DB, seed Package) (Package, bool, error) {
	if !isLegacyUpgradeableDefault(seed) {
		return Package{}, false, nil
	}
	var legacy Package
	err := tx.Unscoped().
		Where("price_cents = ? AND credits = ? AND name IN ?", seed.PriceCents, seed.Credits, []string{"灵感包", "创作包", "高频包"}).
		First(&legacy).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Package{}, false, nil
	}
	if err != nil {
		return Package{}, false, err
	}
	return legacy, true, nil
}

func isLegacyUpgradeableDefault(seed Package) bool {
	return (seed.Name == "体验包" && seed.PriceCents == 1000 && seed.Credits == 50) ||
		(seed.Name == "入门包" && seed.PriceCents == 3000 && seed.Credits == 188) ||
		(seed.Name == "常用包" && seed.PriceCents == 10000 && seed.Credits == 688)
}

func defaultPackageUpgrade(existing Package, seed Package) map[string]any {
	updates := map[string]any{
		"name":                      seed.Name,
		"description":               seed.Description,
		"price_label":               seed.PriceLabel,
		"price_cents":               seed.PriceCents,
		"credits":                   seed.Credits,
		"valid_days":                seed.ValidDays,
		"audience":                  seed.Audience,
		"tags_json":                 seed.TagsJSON,
		"icon":                      seed.Icon,
		"theme":                     seed.Theme,
		"badge":                     seed.Badge,
		"recommended":               seed.Recommended,
		"sort_order":                seed.SortOrder,
		"is_active":                 seed.IsActive,
		"wechat_virtual_product_id": seed.WechatVirtualProductID,
		"features_json":             seed.FeaturesJSON,
		"benefits_json":             seed.BenefitsJSON,
	}
	if existing.ID == 0 {
		return updates
	}
	return updates
}

func deactivateLegacyTeamPackage(tx *gorm.DB) error {
	return tx.Model(&Package{}).
		Where("name = ? AND price_cents = ? AND credits = ? AND is_active = ?", "团队包", 39900, 320, true).
		Update("is_active", false).Error
}

func (a *App) setupRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(a.requestLogMiddleware())

	api := router.Group("/api")
	{
		api.POST("/auth/register", a.requireSameOrigin(), a.handleRegister)
		api.POST("/auth/sms-code", a.requireSameOrigin(), a.handleSendSMSCode)
		api.POST("/auth/register-phone", a.requireSameOrigin(), a.handleRegisterPhone)
		api.POST("/auth/reset-password", a.requireSameOrigin(), a.handleResetPassword)
		api.GET("/auth/csrf-token", a.handleGetCSRFToken)
		api.GET("/auth/captcha", a.handleGetAuthCaptcha)
		api.POST("/auth/login", a.requireSameOrigin(), a.handleLogin)
		api.POST("/auth/wechat-login", a.requireSameOrigin(), a.handleWechatLogin)
		api.POST("/auth/wechat-phone-login", a.requireSameOrigin(), a.handleWechatPhoneLogin)
		api.POST("/auth/wechat-bind", a.requireSameOrigin(), a.requireUser(), a.handleWechatBind)
		api.POST("/auth/logout", a.requireSameOrigin(), a.requireUser(), a.handleLogout)
		api.GET("/me", a.requireUser(), a.handleMe)

		commerceAPI := api.Group("/ecommerce")
		commerceAPI.GET("/capabilities", a.requireUser(), a.handleCommerceCapabilities)
		commerceAPI.Use(a.requireCommerceEnabled())
		commerceAPI.GET("/recipes", a.requireUser(), a.handleListCommerceRecipes)
		commerceAPI.GET("/categories", a.requireUser(), a.handleListCommerceCategories)
		commerceAPI.POST("/categories/custom", a.requireSameOrigin(), a.requireUser(), a.handleCreateCommerceCustomCategory)
		commerceAPI.PATCH("/categories/custom/:id", a.requireSameOrigin(), a.requireUser(), a.handlePatchCommerceCustomCategory)
		commerceAPI.GET("/brands", a.requireUser(), a.handleListCommerceBrands)
		commerceAPI.POST("/brands", a.requireSameOrigin(), a.requireUser(), a.handleCreateCommerceBrand)
		commerceAPI.GET("/brands/:id", a.requireUser(), a.handleGetCommerceBrand)
		commerceAPI.PATCH("/brands/:id", a.requireSameOrigin(), a.requireUser(), a.handlePatchCommerceBrand)
		commerceAPI.GET("/products", a.requireUser(), a.handleListCommerceProducts)
		commerceAPI.POST("/products", a.requireSameOrigin(), a.requireUser(), a.handleCreateCommerceProduct)
		commerceAPI.GET("/products/:id", a.requireUser(), a.handleGetCommerceProduct)
		commerceAPI.PATCH("/products/:id", a.requireSameOrigin(), a.requireUser(), a.handlePatchCommerceProduct)
		commerceAPI.GET("/products/:id/skus", a.requireUser(), a.handleListCommerceSKUs)
		commerceAPI.GET("/products/:id/sku-config", a.requireUser(), a.handleGetCommerceSKUConfig)
		commerceAPI.POST("/products/:id/sku-matrix/preview", a.requireSameOrigin(), a.requireUser(), a.handlePreviewCommerceSKUMatrix)
		commerceAPI.PUT("/products/:id/sku-matrix", a.requireSameOrigin(), a.requireUser(), a.handleApplyCommerceSKUMatrix)
		commerceAPI.POST("/products/:id/skus", a.requireSameOrigin(), a.requireUser(), a.handleCreateCommerceSKU)
		commerceAPI.PATCH("/skus/:id", a.requireSameOrigin(), a.requireUser(), a.handlePatchCommerceSKU)
		commerceAPI.GET("/projects", a.requireUser(), a.handleListCommerceProjects)
		commerceAPI.POST("/projects", a.requireSameOrigin(), a.requireUser(), a.handleCreateCommerceProject)
		commerceAPI.POST("/projects/bootstrap", a.requireSameOrigin(), a.requireUser(), a.handleBootstrapCommerceProject)
		commerceAPI.GET("/projects/:id", a.requireUser(), a.handleGetCommerceProject)
		commerceAPI.PATCH("/projects/:id", a.requireSameOrigin(), a.requireUser(), a.handlePatchCommerceProject)
		commerceAPI.DELETE("/projects/:id", a.requireSameOrigin(), a.requireUser(), a.handleDeleteCommerceProject)
		commerceAPI.POST("/projects/:id/creative-specs", a.requireSameOrigin(), a.requireUser(), a.handleCreateManualCommerceCreativeSpec)
		commerceAPI.POST("/projects/:id/creative-specs/analyze", a.requireSameOrigin(), a.requireUser(), a.handleAnalyzeCommerceProduct)
		commerceAPI.GET("/projects/:id/creative-specs/latest", a.requireUser(), a.handleGetLatestCommerceCreativeSpec)
		commerceAPI.POST("/projects/:id/assets/upload-policy", a.requireSameOrigin(), a.requireUser(), a.handleCreateCommerceAssetUploadPolicy)
		commerceAPI.POST("/projects/:id/assets/complete-upload", a.requireSameOrigin(), a.requireUser(), a.handleCompleteCommerceAssetUpload)
		commerceAPI.GET("/projects/:id/assets", a.requireUser(), a.handleListCommerceAssets)
		commerceAPI.DELETE("/projects/:id/assets/:asset_id", a.requireSameOrigin(), a.requireUser(), a.handleDeleteCommerceAsset)
		commerceAPI.GET("/assets/:id/file", a.requireUser(), a.handleServeCommerceAssetFile)
		commerceAPI.GET("/creative-specs/:id", a.requireUser(), a.handleGetCommerceCreativeSpec)
		commerceAPI.PATCH("/creative-specs/:id", a.requireSameOrigin(), a.requireUser(), a.handlePatchCommerceCreativeSpec)
		commerceAPI.POST("/creative-specs/:id/confirm", a.requireSameOrigin(), a.requireUser(), a.handleConfirmCommerceCreativeSpec)
		commerceAPI.POST("/projects/:id/batches/estimate", a.requireSameOrigin(), a.requireUser(), a.handleEstimateCommerceBatch)
		commerceAPI.POST("/projects/:id/batches", a.requireSameOrigin(), a.requireUser(), a.handleSubmitCommerceBatch)
		commerceAPI.GET("/batches/:id", a.requireUser(), a.handleGetCommerceBatch)
		commerceAPI.GET("/batches/:id/events", a.requireUser(), a.handleListCommerceBatchEvents)
		commerceAPI.GET("/projects/:id/batches", a.requireUser(), a.handleListCommerceBatches)
		commerceAPI.POST("/batches/:id/cancel", a.requireSameOrigin(), a.requireUser(), a.handleCancelCommerceBatch)
		commerceAPI.POST("/items/:id/cancel", a.requireSameOrigin(), a.requireUser(), a.handleCancelCommerceItem)
		commerceAPI.POST("/items/:id/retry", a.requireSameOrigin(), a.requireUser(), a.handleRetryCommerceItem)
		api.GET("/announcements/popup", a.requireUser(), a.handleListPopupAnnouncements)
		api.POST("/announcements/:id/dismiss", a.requireSameOrigin(), a.requireUser(), a.handleDismissAnnouncement)

		api.GET("/packages", a.handleListPackages)
		api.GET("/customer-service", a.handleGetCustomerService)
		api.POST("/content-reports", a.requireSameOrigin(), a.requireUser(), a.handleCreateContentReport)
		api.POST("/purchase-intents", a.requireSameOrigin(), a.requireUser(), a.handlePurchaseIntentsDisabled)
		api.POST("/payments/alipay/orders", a.requireSameOrigin(), a.requireUser(), a.handleCreateAlipayOrder)
		api.GET("/payments/alipay/orders/:order_number", a.requireUser(), a.handleGetAlipayOrder)
		api.POST("/payments/alipay/orders/:order_number/pay", a.requireSameOrigin(), a.requireUser(), a.handlePayAlipayOrder)
		api.POST("/payments/alipay/orders/:order_number/query", a.requireSameOrigin(), a.requireUser(), a.handleQueryAlipayOrder)
		api.POST("/payments/alipay/notify", a.handleAlipayNotify)
		api.POST("/payments/wechat/orders", a.requireSameOrigin(), a.requireUser(), a.handleCreateWechatPayOrder)
		api.POST("/payments/wechat/orders/:order_number/query", a.requireSameOrigin(), a.requireUser(), a.handleQueryWechatPayOrder)
		api.POST("/payments/wechat/notify", a.handleWechatPayNotify)
		api.POST("/payments/wechat/virtual-orders", a.requireSameOrigin(), a.requireUser(), a.handleCreateWechatVirtualPayOrder)
		api.POST("/payments/wechat/virtual-orders/:order_number/confirm", a.requireSameOrigin(), a.requireUser(), a.handleConfirmWechatVirtualPayOrder)

		api.GET("/account/credits", a.requireUser(), a.handleGetCredits)
		api.GET("/account/credit-transactions", a.requireUser(), a.handleGetCreditTransactions)
		api.POST("/account/phone", a.requireSameOrigin(), a.requireUser(), a.handleBindPhone)
		api.DELETE("/account/phone", a.requireSameOrigin(), a.requireUser(), a.handleUnbindPhone)
		api.POST("/account/wechat-phone", a.requireSameOrigin(), a.requireUser(), a.handleBindWechatPhone)
		api.PATCH("/account/profile", a.requireSameOrigin(), a.requireUser(), a.handleUpdateProfile)
		api.PATCH("/account/email", a.requireSameOrigin(), a.requireUser(), a.handleUpdateEmail)
		api.PATCH("/account/preferences", a.requireSameOrigin(), a.requireUser(), a.handleUpdatePreferences)
		api.POST("/account/password", a.requireSameOrigin(), a.requireUser(), a.handleChangePassword)
		api.POST("/account/payment-password", a.requireSameOrigin(), a.requireUser(), a.handleSetPaymentPassword)
		api.DELETE("/account/payment-password", a.requireSameOrigin(), a.requireUser(), a.handleClearPaymentPassword)
		api.GET("/account/presence", a.requireUser(), a.handleAccountPresence)
		api.GET("/account/sessions", a.requireUser(), a.handleListUserSessions)
		api.DELETE("/account/sessions/:id", a.requireSameOrigin(), a.requireUser(), a.handleDeleteUserSession)

		api.GET("/works", a.requireUser(), a.handleListWorks)
		api.GET("/works/:id", a.requireUser(), a.handleGetWork)
		api.PATCH("/works/:id", a.requireSameOrigin(), a.requireUser(), a.handleUpdateWork)
		api.DELETE("/works/:id", a.requireSameOrigin(), a.requireUser(), a.handleDeleteWork)
		api.POST("/works/:id/reuse", a.requireSameOrigin(), a.requireUser(), a.handleReuseWork)
		api.GET("/works/:id/file", a.requireUser(), a.handleServeWorkPreview)
		api.GET("/works/:id/download", a.requireUser(), a.handleServeWorkDownload)
		api.GET("/public/works", a.handleListPublicWorks)
		api.GET("/public/works/:id/file", a.handleServePublicWorkPreview)
		api.GET("/public/prompt-templates/:id/preview", a.handleServePromptTemplatePreview)

		api.POST("/reference-assets", a.requireSameOrigin(), a.requireUser(), a.handleUploadReferenceAsset)
		api.POST("/reference-assets/upload-policy", a.requireSameOrigin(), a.requireUser(), a.handleCreateReferenceAssetUploadPolicy)
		api.POST("/reference-assets/complete-upload", a.requireSameOrigin(), a.requireUser(), a.handleCompleteReferenceAssetUpload)
		api.GET("/reference-assets", a.requireUser(), a.handleListReferenceAssets)
		api.PATCH("/reference-assets/:id", a.requireSameOrigin(), a.requireUser(), a.handleUpdateReferenceAsset)
		api.DELETE("/reference-assets/:id", a.requireSameOrigin(), a.requireUser(), a.handleDeleteReferenceAsset)
		api.GET("/reference-assets/:id/file", a.requireUser(), a.handleServeReferenceAssetPreview)

		api.POST("/images/generations/estimate", a.requireSameOrigin(), a.requireUser(), a.handleEstimateImageGeneration)
		api.POST("/images/generations", a.requireSameOrigin(), a.requireUser(), a.handleGenerateImage)
		api.POST("/images/generations/async", a.requireSameOrigin(), a.requireUser(), a.handleCreateAsyncGeneration)
		api.GET("/images/generations/:id", a.requireUser(), a.handleGetGeneration)
		api.POST("/images/generations/:id/cancel", a.requireSameOrigin(), a.requireUser(), a.handleCancelImageGeneration)
		api.POST("/virtual-try-on/generations/estimate", a.requireSameOrigin(), a.requireUser(), a.handleEstimateVirtualTryOn)
		api.POST("/virtual-try-on/generations/async", a.requireSameOrigin(), a.requireUser(), a.handleCreateAsyncVirtualTryOn)
		api.POST("/marketing/moments/plan", a.requireSameOrigin(), a.requireUser(), a.handlePlanMomentsMarketing)
		api.POST("/marketing/article-images/plan", a.requireSameOrigin(), a.requireUser(), a.handlePlanArticleImages)
		api.POST("/agent/image-plan", a.requireSameOrigin(), a.requireUser(), a.handlePlanImageAgent)
		api.POST("/prompts/optimize", a.requireSameOrigin(), a.requireUser(), a.handleOptimizePrompt)
		api.GET("/workspace/discovery", a.handleWorkspaceDiscovery)
		api.POST("/workspace/inspiration-recommendations/:id/use", a.requireSameOrigin(), a.handleUseInspirationRecommendation)
		api.GET("/prompt-templates", a.requireUser(), a.handleListPromptTemplates)
		api.POST("/prompt-templates/:id/use", a.requireSameOrigin(), a.requireUser(), a.handleUsePromptTemplate)
		api.GET("/videos/style-presets", a.requireUser(), a.handleListVideoStylePresets)
		api.GET("/videos/style-templates", a.requireUser(), a.handleListUserVideoStyleTemplates)
		api.POST("/videos/style-templates", a.requireSameOrigin(), a.requireUser(), a.handleCreateUserVideoStyleTemplate)
		api.DELETE("/videos/style-templates/:id", a.requireSameOrigin(), a.requireUser(), a.handleDeleteUserVideoStyleTemplate)
		api.GET("/videos/models", a.handleListVideoModels)
		api.POST("/videos/generations/estimate", a.requireSameOrigin(), a.requireUser(), a.handleEstimateVideoGeneration)
		api.POST("/videos/generations/async", a.requireSameOrigin(), a.requireUser(), a.handleCreateAsyncVideoGeneration)
		api.POST("/videos/conversations", a.requireSameOrigin(), a.requireUser(), a.handleCreateVideoConversation)
		api.GET("/videos/conversations", a.requireUser(), a.handleListVideoConversations)
		api.GET("/videos/conversations/:id", a.requireUser(), a.handleGetVideoConversation)
		api.PATCH("/videos/conversations/:id", a.requireSameOrigin(), a.requireUser(), a.handlePatchVideoConversation)
		api.POST("/videos/conversations/:id/messages", a.requireSameOrigin(), a.requireUser(), a.handleCreateVideoConversationMessage)
		api.GET("/videos/generations", a.requireUser(), a.handleListUserVideoGenerations)
		api.GET("/videos/generations/:id", a.requireUser(), a.handleGetVideoGeneration)
		api.GET("/videos/:work_id/soundtracks", a.requireUser(), a.handleListVideoSoundtracks)
		api.POST("/videos/:work_id/soundtracks/generate", a.requireSameOrigin(), a.requireUser(), a.handleGenerateVideoSoundtrack)
		api.POST("/videos/:work_id/soundtracks/upload", a.requireSameOrigin(), a.requireUser(), a.handleUploadVideoSoundtrack)

		api.POST("/novel-video-projects", a.requireSameOrigin(), a.requireUser(), a.handleCreateNovelVideoProject)
		api.GET("/novel-video-projects", a.requireUser(), a.handleListNovelVideoProjects)
		api.GET("/novel-video-projects/:id", a.requireUser(), a.handleGetNovelVideoProject)
		api.PATCH("/novel-video-projects/:id", a.requireSameOrigin(), a.requireUser(), a.handlePatchNovelVideoProject)
		api.POST("/novel-video-projects/:id/analyze", a.requireSameOrigin(), a.requireUser(), a.handleAnalyzeNovelVideoProject)
		api.POST("/novel-video-projects/:id/image-plan", a.requireSameOrigin(), a.requireUser(), a.handlePlanNovelVideoImages)
		api.POST("/novel-video-projects/:id/assets/generate", a.requireSameOrigin(), a.requireUser(), a.handleGenerateNovelVideoAssets)
		api.POST("/novel-video-projects/:id/assets/dedupe", a.requireSameOrigin(), a.requireUser(), a.handleDedupeNovelVideoAssets)
		api.PATCH("/novel-video-projects/:id/assets/:asset_id", a.requireSameOrigin(), a.requireUser(), a.handlePatchNovelVideoAsset)
		api.DELETE("/novel-video-projects/:id/assets/:asset_id", a.requireSameOrigin(), a.requireUser(), a.handleDeleteNovelVideoAsset)
		api.PATCH("/novel-video-projects/:id/actors/:actor_id", a.requireSameOrigin(), a.requireUser(), a.handlePatchNovelVideoActor)
		api.POST("/novel-video-projects/:id/actors/:actor_id/generate-lock-sheet", a.requireSameOrigin(), a.requireUser(), a.handleGenerateNovelVideoActorLockSheet)
		api.PATCH("/novel-video-projects/:id/creatures/:creature_id", a.requireSameOrigin(), a.requireUser(), a.handlePatchNovelVideoCreature)
		api.POST("/novel-video-projects/:id/creatures/:creature_id/generate-image", a.requireSameOrigin(), a.requireUser(), a.handleGenerateNovelVideoCreatureImage)
		api.POST("/novel-video-projects/:id/episodes/plan", a.requireSameOrigin(), a.requireUser(), a.handlePlanNovelVideoEpisodes)
		api.PATCH("/novel-video-projects/:id/shots/:shot_id", a.requireSameOrigin(), a.requireUser(), a.handlePatchNovelVideoShot)
		api.POST("/novel-video-projects/:id/shots/:shot_id/storyboard", a.requireSameOrigin(), a.requireUser(), a.handleGenerateNovelVideoStoryboard)
		api.POST("/novel-video-projects/:id/images/generate", a.requireSameOrigin(), a.requireUser(), a.handleGenerateNovelVideoShotImages)
		api.GET("/novel-video-projects/:id/images", a.requireUser(), a.handleListNovelVideoShotImages)
		api.PATCH("/novel-video-projects/:id/images/:image_id", a.requireSameOrigin(), a.requireUser(), a.handlePatchNovelVideoShotImage)
		api.POST("/novel-video-projects/:id/grids/generate", a.requireSameOrigin(), a.requireUser(), a.handleGenerateNovelVideoGrids)
		api.GET("/novel-video-projects/:id/cost-estimate", a.requireUser(), a.handleNovelVideoCostEstimate)
		api.POST("/novel-video-projects/:id/render-preflight", a.requireSameOrigin(), a.requireUser(), a.handleNovelVideoRenderPreflight)
		api.POST("/novel-video-projects/:id/render-approved-shots", a.requireSameOrigin(), a.requireUser(), a.handleRenderNovelVideoApprovedShots)
		api.POST("/novel-video-projects/:id/render", a.requireSameOrigin(), a.requireUser(), a.handleQueueNovelVideoRenderJobs)
		api.POST("/novel-video-projects/:id/compose", a.requireSameOrigin(), a.requireUser(), a.handleComposeNovelVideoProject)
		api.GET("/novel-video-projects/:id/compositions", a.requireUser(), a.handleListNovelVideoCompositions)
		api.GET("/novel-video-projects/:id/events", a.requireUser(), a.handleNovelVideoProjectEvents)
		api.POST("/novel-video-projects/:id/versions/:version_id/restore", a.requireSameOrigin(), a.requireUser(), a.handleRestoreNovelVideoVersion)
		api.GET("/novel-video-projects/:id/export", a.requireUser(), a.handleExportNovelVideoProject)

		api.GET("/couple-album/options", a.requireUser(), a.handleListCoupleAlbumOptions)
		api.POST("/couple-albums/estimate", a.requireSameOrigin(), a.requireUser(), a.handleEstimateCoupleAlbum)
		api.POST("/couple-albums", a.requireSameOrigin(), a.requireUser(), a.handleCreateCoupleAlbum)
		api.GET("/couple-albums", a.requireUser(), a.handleListCoupleAlbums)
		api.GET("/couple-albums/:id", a.requireUser(), a.handleGetCoupleAlbum)
		api.POST("/couple-albums/:id/generate", a.requireSameOrigin(), a.requireUser(), a.handleGenerateCoupleAlbum)
		api.POST("/couple-albums/:id/pages/:page_id/retry", a.requireSameOrigin(), a.requireUser(), a.handleRetryCoupleAlbumPage)
		api.POST("/couple-albums/:id/share", a.requireSameOrigin(), a.requireUser(), a.handleShareCoupleAlbum)
		api.GET("/public/couple-albums/:share_token", a.handleGetPublicCoupleAlbum)
	}

	admin := api.Group("/admin")
	{
		admin.POST("/login", a.requireSameOrigin(), a.handleAdminLogin)
		admin.POST("/logout", a.requireSameOrigin(), a.requireAdmin(), a.handleAdminLogout)
		admin.POST("/password", a.requireSameOrigin(), a.requireAdmin(), a.handleChangeAdminPassword)
		admin.GET("/me", a.requireAdmin(), a.handleAdminMe)
		admin.GET("/search", a.requireAdmin(), a.handleAdminSearch)
		admin.GET("/dashboard", a.requireAdminPermission("dashboard.read"), a.handleAdminDashboard)
		admin.GET("/settings/image", a.requireAdminPermission("settings.image.read"), a.handleGetImageSettings)
		admin.PUT("/settings/image", a.requireSameOrigin(), a.requireAdminPermission("settings.image.update"), a.handlePutImageSettings)
		admin.GET("/system-settings", a.requireAdminPermission("system_settings.read"), a.handleGetSystemSettings)
		admin.PATCH("/system-settings", a.requireSameOrigin(), a.requireAdminPermission("system_settings.update"), a.handlePatchSystemSettings)
		admin.GET("/ecommerce/categories", a.requireAdminPermission("system_settings.read"), a.handleListAdminCommerceCategories)
		admin.POST("/ecommerce/categories", a.requireSameOrigin(), a.requireAdminPermission("system_settings.update"), a.handleCreateAdminCommerceCategory)
		admin.PATCH("/ecommerce/categories/:id", a.requireSameOrigin(), a.requireAdminPermission("system_settings.update"), a.handlePatchAdminCommerceCategory)
		admin.GET("/system-settings/export", a.requireAdminPermission("system_settings.read"), a.handleExportSystemSettings)
		admin.GET("/system-logs", a.requireStrictAdminPermission("system_logs.read"), a.handleListSystemLogs)
		admin.GET("/system-logs/export", a.requireStrictAdminPermission("system_logs.read"), a.handleExportSystemLogs)
		admin.GET("/system-resources", a.requireAdminPermission("system_resources.read"), a.handleGetSystemResources)
		admin.GET("/customer-service", a.requireAdminPermission("customer_service.read"), a.handleGetAdminCustomerService)
		admin.PATCH("/customer-service", a.requireSameOrigin(), a.requireAdminPermission("customer_service.update"), a.handlePatchAdminCustomerService)
		admin.POST("/customer-service/qrcode", a.requireSameOrigin(), a.requireAdminPermission("customer_service.update"), a.handleUploadAdminCustomerServiceQRCode)
		admin.GET("/models", a.requireAdminPermission("settings.image.read"), a.handleListAdminModels)
		admin.GET("/secret-settings", a.requireAdminPermission("system_settings.read"), a.handleGetSecretSettings)
		admin.PATCH("/secret-settings", a.requireSameOrigin(), a.requireAdminPermission("system_settings.update"), a.handlePatchSecretSettings)
		admin.POST("/models", a.requireSameOrigin(), a.requireAdminPermission("settings.image.update"), a.handleCreateAdminModel)
		admin.GET("/models/:id", a.requireAdminPermission("settings.image.read"), a.handleGetAdminModel)
		admin.PUT("/models/:id", a.requireSameOrigin(), a.requireAdminPermission("settings.image.update"), a.handleUpdateAdminModel)
		admin.PATCH("/models/:id/video-readiness", a.requireSameOrigin(), a.requireAdminPermission("settings.image.update"), a.handlePatchAdminModelVideoReadiness)
		admin.DELETE("/models/:id", a.requireSameOrigin(), a.requireAdminPermission("settings.image.update"), a.handleDeleteAdminModel)
		admin.GET("/model-center/overview", a.requireAdminPermission("settings.image.read"), a.handleGetModelCenterOverview)
		admin.GET("/model-center/models", a.requireAdminPermission("settings.image.read"), a.handleListModelCenterModels)
		admin.POST("/model-center/models", a.requireSameOrigin(), a.requireAdminPermission("settings.image.update"), a.handleCreateModelCenterModel)
		admin.PUT("/model-center/models/:id", a.requireSameOrigin(), a.requireAdminPermission("settings.image.update"), a.handleUpdateModelCenterModel)
		admin.DELETE("/model-center/models/:id", a.requireSameOrigin(), a.requireAdminPermission("settings.image.update"), a.handleDeleteModelCenterModel)
		admin.GET("/model-center/providers", a.requireAdminPermission("settings.image.read"), a.handleListModelCenterProviders)
		admin.POST("/model-center/providers", a.requireSameOrigin(), a.requireAdminPermission("settings.image.update"), a.handleCreateModelCenterProvider)
		admin.PUT("/model-center/providers/:id", a.requireSameOrigin(), a.requireAdminPermission("settings.image.update"), a.handleUpdateModelCenterProvider)
		admin.DELETE("/model-center/providers/:id", a.requireSameOrigin(), a.requireAdminPermission("settings.image.update"), a.handleDeleteModelCenterProvider)
		admin.GET("/model-center/channels", a.requireAdminPermission("settings.image.read"), a.handleListModelCenterChannels)
		admin.GET("/model-center/channels/:id/call-attempts", a.requireAdminPermission("settings.image.read"), a.handleListModelCenterChannelCallAttempts)
		admin.POST("/model-center/channels", a.requireSameOrigin(), a.requireAdminPermission("settings.image.update"), a.handleCreateModelCenterChannel)
		admin.PUT("/model-center/channels/:id", a.requireSameOrigin(), a.requireAdminPermission("settings.image.update"), a.handleUpdateModelCenterChannel)
		admin.DELETE("/model-center/channels/:id", a.requireSameOrigin(), a.requireAdminPermission("settings.image.update"), a.handleDeleteModelCenterChannel)
		admin.GET("/model-center/routing", a.requireAdminPermission("settings.image.read"), a.handleGetModelCenterRouting)
		admin.PUT("/model-center/routing", a.requireSameOrigin(), a.requireAdminPermission("settings.image.update"), a.handlePutModelCenterRouting)
		admin.GET("/model-center/audit-logs", a.requireAdminPermission("settings.image.read"), a.handleListModelCenterAuditLogs)
		admin.GET("/prompt-templates", a.requireAdminPermission("prompt_templates.read"), a.handleListAdminPromptTemplates)
		admin.POST("/prompt-templates", a.requireSameOrigin(), a.requireAdminPermission("prompt_templates.update"), a.handleCreateAdminPromptTemplate)
		admin.PUT("/prompt-templates/:id", a.requireSameOrigin(), a.requireAdminPermission("prompt_templates.update"), a.handleUpdateAdminPromptTemplate)
		admin.DELETE("/prompt-templates/:id", a.requireSameOrigin(), a.requireAdminPermission("prompt_templates.update"), a.handleDeleteAdminPromptTemplate)
		admin.POST("/prompt-templates/:id/preview", a.requireSameOrigin(), a.requireAdminPermission("prompt_templates.update"), a.handleGenerateAdminPromptTemplatePreview)
		admin.POST("/prompt-templates/previews/generate", a.requireSameOrigin(), a.requireAdminPermission("prompt_templates.update"), a.handleBatchGenerateAdminPromptTemplatePreviews)
		admin.GET("/inspiration-recommendations", a.requireAdminPermission("inspiration_recommendations.read"), a.handleListAdminInspirationRecommendations)
		admin.POST("/inspiration-recommendations", a.requireSameOrigin(), a.requireAdminPermission("inspiration_recommendations.update"), a.handleCreateAdminInspirationRecommendation)
		admin.PUT("/inspiration-recommendations/:id", a.requireSameOrigin(), a.requireAdminPermission("inspiration_recommendations.update"), a.handleUpdateAdminInspirationRecommendation)
		admin.DELETE("/inspiration-recommendations/:id", a.requireSameOrigin(), a.requireAdminPermission("inspiration_recommendations.update"), a.handleDeleteAdminInspirationRecommendation)
		admin.GET("/video-style-presets", a.requireAdminPermission("video_style_presets.read"), a.handleListAdminVideoStylePresets)
		admin.POST("/video-style-presets", a.requireSameOrigin(), a.requireAdminPermission("video_style_presets.update"), a.handleCreateAdminVideoStylePreset)
		admin.PUT("/video-style-presets/:id", a.requireSameOrigin(), a.requireAdminPermission("video_style_presets.update"), a.handleUpdateAdminVideoStylePreset)
		admin.DELETE("/video-style-presets/:id", a.requireSameOrigin(), a.requireAdminPermission("video_style_presets.update"), a.handleDeleteAdminVideoStylePreset)
		admin.GET("/couple-album-options", a.requireAdminPermission("couple_album_options.read"), a.handleListAdminCoupleAlbumOptions)
		admin.POST("/couple-album-options", a.requireSameOrigin(), a.requireAdminPermission("couple_album_options.update"), a.handleCreateAdminCoupleAlbumOption)
		admin.PUT("/couple-album-options/:id", a.requireSameOrigin(), a.requireAdminPermission("couple_album_options.update"), a.handleUpdateAdminCoupleAlbumOption)
		admin.DELETE("/couple-album-options/:id", a.requireSameOrigin(), a.requireAdminPermission("couple_album_options.update"), a.handleDeleteAdminCoupleAlbumOption)
		admin.POST("/couple-album-options/assets", a.requireSameOrigin(), a.requireAdminPermission("couple_album_options.update"), a.handleUploadAdminCoupleAlbumOptionAsset)
		admin.GET("/model-routing", a.requireAdminPermission("settings.image.read"), a.handleGetModelRouting)
		admin.PUT("/model-routing", a.requireSameOrigin(), a.requireAdminPermission("settings.image.update"), a.handlePutModelRouting)
		admin.GET("/invites", a.requireAdminPermission("invites.read"), a.handleListInvites)
		admin.GET("/invites/export", a.requireAdminPermission("invites.read"), a.handleExportInvites)
		admin.POST("/invites", a.requireSameOrigin(), a.requireAdminPermission("invites.create"), a.handleCreateInvite)
		admin.POST("/invites/batch", a.requireSameOrigin(), a.requireAdminPermission("invites.create"), a.handleBatchCreateInvites)
		admin.PUT("/invites/:id", a.requireSameOrigin(), a.requireAdminPermission("invites.update"), a.handleUpdateInvite)
		admin.GET("/invite-redemptions", a.requireAdminPermission("invites.read"), a.handleListInviteRedemptions)
		admin.GET("/invite-redemptions/export", a.requireAdminPermission("invites.read"), a.handleExportInviteRedemptions)
		admin.GET("/generations", a.requireAdminPermission("generations.read"), a.handleListGenerations)
		admin.GET("/generations/export", a.requireAdminPermission("generations.read"), a.handleExportGenerations)
		admin.GET("/generations/:id", a.requireAdminPermission("generations.read"), a.handleGetAdminGeneration)
		admin.GET("/video-generations", a.requireAdminPermission("generations.read"), a.handleListVideoGenerations)
		admin.GET("/video-generations/export", a.requireAdminPermission("generations.read"), a.handleExportVideoGenerations)
		admin.GET("/video-generations/:id", a.requireAdminPermission("generations.read"), a.handleGetAdminVideoGeneration)
		admin.GET("/content-reviews", a.requireAdminPermission("content_reviews.read"), a.handleListAdminContentReviews)
		admin.PATCH("/content-reviews/:id", a.requireSameOrigin(), a.requireAdminPermission("content_reviews.update"), a.handlePatchAdminContentReview)
		admin.GET("/content-reports", a.requireAdminPermission("content_reports.read"), a.handleListAdminContentReports)
		admin.GET("/algorithm-disclosure", a.requireAdminPermission("algorithm_compliance.read"), a.handleGetAdminAlgorithmDisclosure)
		admin.PATCH("/algorithm-disclosure", a.requireSameOrigin(), a.requireAdminPermission("algorithm_compliance.update"), a.handlePatchAdminAlgorithmDisclosure)
		admin.GET("/algorithm-compliance/export", a.requireAdminPermission("algorithm_compliance.read"), a.handleExportAdminAlgorithmCompliance)
		admin.GET("/incidents", a.requireAdminPermission("algorithm_incidents.read"), a.handleListAdminAlgorithmIncidents)
		admin.POST("/incidents", a.requireSameOrigin(), a.requireAdminPermission("algorithm_incidents.update"), a.handleCreateAdminAlgorithmIncident)
		admin.GET("/users", a.requireAdminPermission("users.read"), a.handleAdminListUsers)
		admin.POST("/users/batch-delete", a.requireSameOrigin(), a.requireAdminPermission("users.delete"), a.handleAdminBatchDeleteUsers)
		admin.DELETE("/users/:id", a.requireSameOrigin(), a.requireAdminPermission("users.delete"), a.handleAdminDeleteUser)
		admin.POST("/users/:id/reset-password", a.requireSameOrigin(), a.requireAdminPermission("users.password.reset"), a.handleAdminResetUserPassword)
		admin.GET("/credit-transactions", a.requireAdminPermission("users.read"), a.handleAdminListCreditTransactions)
		admin.POST("/users/:id/credits", a.requireSameOrigin(), a.requireAdminPermission("users.credits.add"), a.handleAdminAddCredits)
		admin.POST("/users/:id/credit-adjustments", a.requireSameOrigin(), a.requireAdminPermission("users.credits.add"), a.handleAdminAdjustCredits)
		admin.PATCH("/users/:id/wechat-binding", a.requireSameOrigin(), a.requireAdminPermission("users.update"), a.handleAdminUpdateUserWechatBinding)
		admin.DELETE("/users/:id/wechat-binding", a.requireSameOrigin(), a.requireAdminPermission("users.update"), a.handleAdminUnbindUserWechat)
		admin.DELETE("/users/:id/phone-binding", a.requireSameOrigin(), a.requireAdminPermission("users.update"), a.handleAdminUnbindUserPhone)
		admin.GET("/packages", a.requireAdminPermission("packages.read"), a.handleAdminListPackages)
		admin.POST("/packages", a.requireSameOrigin(), a.requireAdminPermission("packages.create"), a.handleAdminCreatePackage)
		admin.PUT("/packages/:id", a.requireSameOrigin(), a.requireAdminPermission("packages.update"), a.handleAdminUpdatePackage)
		admin.DELETE("/packages/:id", a.requireSameOrigin(), a.requireAdminPermission("packages.delete"), a.handleAdminDeletePackage)
		admin.GET("/purchase-intents", a.requireAdmin(), a.handlePurchaseIntentsDisabled)
		admin.PUT("/purchase-intents/:id", a.requireSameOrigin(), a.requireAdmin(), a.handlePurchaseIntentsDisabled)
		admin.GET("/finance-orders", a.requireAdminPermission("finance_orders.read"), a.handleAdminListFinanceOrders)
		admin.GET("/finance-orders/export", a.requireAdminPermission("finance_orders.read"), a.handleExportFinanceOrders)
		admin.GET("/finance-orders/:id", a.requireAdminPermission("finance_orders.read"), a.handleGetFinanceOrder)
		admin.POST("/finance-orders/:id/sync-payment", a.requireSameOrigin(), a.requireAdminPermission("finance_orders.update"), a.handleAdminSyncFinanceOrderPayment)
		admin.PATCH("/finance-refunds/:id", a.requireSameOrigin(), a.requireAdminPermission("finance_orders.update"), a.handleUpdateFinanceRefund)
		admin.PATCH("/finance-invoices/:id", a.requireSameOrigin(), a.requireAdminPermission("finance_orders.update"), a.handleUpdateFinanceInvoice)
		admin.GET("/announcements", a.requireAdminPermission("announcements.read"), a.handleListAdminAnnouncements)
		admin.POST("/announcements", a.requireSameOrigin(), a.requireAdminPermission("announcements.create"), a.handleCreateAnnouncement)
		admin.PUT("/announcements/:id", a.requireSameOrigin(), a.requireAdminPermission("announcements.update"), a.handleUpdateAnnouncement)
		admin.PATCH("/announcements/:id/status", a.requireSameOrigin(), a.requireAdminPermission("announcements.update"), a.handleUpdateAnnouncementStatus)
		admin.GET("/admin-users", a.requireAdminPermission("admin_users.read"), a.handleListAdminUsers)
		admin.POST("/admin-users", a.requireSameOrigin(), a.requireAdminPermission("admin_users.create"), a.handleCreateAdminUser)
		admin.PATCH("/admin-users/:id", a.requireSameOrigin(), a.requireAdminPermission("admin_users.update"), a.handleUpdateAdminUser)
		admin.PUT("/admin-users/:id/roles", a.requireSameOrigin(), a.requireAdminPermission("admin_users.roles.update"), a.handlePutAdminUserRoles)
		admin.POST("/admin-users/:id/reset-password", a.requireSameOrigin(), a.requireAdminPermission("admin_users.password.reset"), a.handleResetAdminUserPassword)
		admin.GET("/roles", a.requireAdminPermission("roles.read"), a.handleListRoles)
		admin.POST("/roles", a.requireSameOrigin(), a.requireAdminPermission("roles.create"), a.handleCreateRole)
		admin.PATCH("/roles/:id", a.requireSameOrigin(), a.requireAdminPermission("roles.update"), a.handleUpdateRole)
		admin.PUT("/roles/:id/permissions", a.requireSameOrigin(), a.requireAdminPermission("roles.permissions.update"), a.handlePutRolePermissions)
	}

	if _, err := os.Stat(a.cfg.FrontendDistPath); err == nil {
		a.registerFrontendAssetRoutes(router)
		router.NoRoute(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/api/") {
				writeError(c, http.StatusNotFound, "not_found", "API route not found")
				return
			}
			if isProtectedAdminSPAPath(c.Request.URL.Path) && !a.hasAdminPageSession(c.Request) {
				c.Redirect(http.StatusFound, "/admin/login")
				return
			}
			c.File(filepath.Join(a.cfg.FrontendDistPath, "index.html"))
		})
	}

	return router
}

func (a *App) registerFrontendAssetRoutes(router *gin.Engine) {
	router.GET("/build-info.json", a.handleFrontendBuildInfo)
	router.GET("/assets", a.handleFrontendAsset)
	router.GET("/assets/*filepath", a.handleFrontendAsset)
	router.HEAD("/assets/*filepath", a.handleFrontendAsset)
	router.GET("/app-assets/*filepath", a.handleFrontendAppAsset)
	router.HEAD("/app-assets/*filepath", a.handleFrontendAppAsset)
}

func (a *App) handleFrontendBuildInfo(c *gin.Context) {
	buildInfoPath := filepath.Join(a.cfg.FrontendDistPath, "build-info.json")
	info, err := os.Stat(buildInfoPath)
	if err != nil || info.IsDir() {
		writeError(c, http.StatusNotFound, "not_found", "build info not found")
		return
	}

	c.Header("Cache-Control", "no-store")
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.File(buildInfoPath)
}

func (a *App) handleFrontendAsset(c *gin.Context) {
	a.serveFrontendAsset(c, "assets")
}

func (a *App) handleFrontendAppAsset(c *gin.Context) {
	a.serveFrontendAsset(c, "app-assets")
}

func (a *App) serveFrontendAsset(c *gin.Context, assetDir string) {
	requestedPath := strings.TrimPrefix(c.Param("filepath"), "/")
	if requestedPath == "" {
		c.File(filepath.Join(a.cfg.FrontendDistPath, "index.html"))
		return
	}

	cleanPath := filepath.Clean(requestedPath)
	if cleanPath == "." || cleanPath == ".." || strings.HasPrefix(cleanPath, "../") || filepath.IsAbs(cleanPath) {
		writeError(c, http.StatusNotFound, "not_found", "frontend asset not found")
		return
	}

	assetPath := filepath.Join(a.cfg.FrontendDistPath, assetDir, cleanPath)
	info, err := os.Stat(assetPath)
	if err != nil || info.IsDir() {
		writeError(c, http.StatusNotFound, "not_found", "frontend asset not found")
		return
	}

	setFrontendAssetCacheHeaders(c)
	if requestAcceptsGzip(c.Request) && isCompressibleFrontendAsset(assetPath) {
		serveGzipFrontendAsset(c, assetPath, info)
		return
	}
	c.File(assetPath)
}

func setFrontendAssetCacheHeaders(c *gin.Context) {
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
}

func requestAcceptsGzip(req *http.Request) bool {
	for _, item := range strings.Split(req.Header.Get("Accept-Encoding"), ",") {
		encoding := strings.TrimSpace(strings.SplitN(item, ";", 2)[0])
		if strings.EqualFold(encoding, "gzip") {
			return true
		}
	}
	return false
}

func isCompressibleFrontendAsset(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".css", ".js", ".mjs", ".json", ".map", ".svg", ".txt":
		return true
	default:
		return false
	}
}

func serveGzipFrontendAsset(c *gin.Context, assetPath string, info os.FileInfo) {
	file, err := os.Open(assetPath)
	if err != nil {
		writeError(c, http.StatusNotFound, "not_found", "frontend asset not found")
		return
	}
	defer file.Close()

	if contentType := mime.TypeByExtension(filepath.Ext(assetPath)); contentType != "" {
		c.Header("Content-Type", contentType)
	}
	c.Header("Content-Encoding", "gzip")
	c.Header("Vary", "Accept-Encoding")
	c.Header("Last-Modified", info.ModTime().UTC().Format(http.TimeFormat))
	c.Status(http.StatusOK)
	if c.Request.Method == http.MethodHead {
		return
	}

	writer := gzip.NewWriter(c.Writer)
	if _, err := io.Copy(writer, file); err != nil {
		_ = c.Error(err)
	}
	if err := writer.Close(); err != nil {
		_ = c.Error(err)
	}
}

func isProtectedAdminSPAPath(path string) bool {
	return path == "/admin" || (strings.HasPrefix(path, "/admin/") && path != "/admin/login")
}

func (a *App) hasAdminPageSession(req *http.Request) bool {
	_, _, _, err := a.authenticateAdmin(req)
	return err == nil
}

func (a *App) requireSameOrigin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead || c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		// Allow WeChat Mini Program requests with X-Image-Agent-Client header
		if c.GetHeader("X-Image-Agent-Client") == "mp-weixin" {
			c.Next()
			return
		}

		origin := c.GetHeader("Origin")
		if origin == "" {
			writeError(c, http.StatusForbidden, "origin_required", "Origin header is required")
			c.Abort()
			return
		}
		if origin != a.cfg.AppBaseURL && origin != requestOrigin(c.Request) {
			writeError(c, http.StatusForbidden, "cross_origin_blocked", "Origin mismatch")
			c.Abort()
			return
		}
		if code, ok := validateCSRFToken(c.Request); !ok {
			message := "CSRF Token 缺失"
			if code == "csrf_invalid" {
				message = "CSRF Token 无效"
			}
			writeError(c, http.StatusForbidden, code, message)
			c.Abort()
			return
		}
		c.Next()
	}
}

func requestOrigin(req *http.Request) string {
	scheme := "http"
	if req.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + req.Host
}

func (a *App) requireUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		now := time.Now()
		claims, err := a.parseSessionCookie(c.Request, userSessionCookie)
		if err != nil {
			claims, err = a.parseBearerSession(c.Request)
		}
		if err != nil || claims.Role != "user" || claims.UserID == 0 || strings.TrimSpace(claims.SessionID) == "" {
			writeError(c, http.StatusUnauthorized, "unauthorized", "Login required")
			c.Abort()
			return
		}

		var session UserSession
		if err := a.db.Where("token_id = ? AND user_id = ?", claims.SessionID, claims.UserID).First(&session).Error; err != nil {
			writeError(c, http.StatusUnauthorized, "unauthorized", "Login required")
			c.Abort()
			return
		}
		if now.After(session.ExpiresAt) {
			_ = a.db.Delete(&session).Error
			writeError(c, http.StatusUnauthorized, "session_expired", "登录已过期")
			c.Abort()
			return
		}

		var user User
		if err := a.db.First(&user, claims.UserID).Error; err != nil || user.Status != UserStatusActive {
			writeError(c, http.StatusUnauthorized, "unauthorized", "Login required")
			c.Abort()
			return
		}
		if err := a.touchUserSessionLastSeen(&session, now); err != nil {
			writeError(c, http.StatusInternalServerError, "session_update_failed", "会话更新失败")
			c.Abort()
			return
		}

		c.Set("claims", claims)
		c.Set("currentUser", &user)
		c.Set("currentUserSession", &session)
		c.Next()
	}
}

func (a *App) touchUserSessionLastSeen(session *UserSession, now time.Time) error {
	if session == nil {
		return nil
	}
	if session.LastSeenAt != nil && now.Sub(*session.LastSeenAt) < userPresenceTouchInterval {
		return nil
	}
	if err := a.db.Model(&UserSession{}).Where("id = ?", session.ID).Update("last_seen_at", now).Error; err != nil {
		return err
	}
	session.LastSeenAt = &now
	return nil
}

func (a *App) requireAdmin() gin.HandlerFunc {
	return a.requireAdminPermission("")
}

func (a *App) requireAdminPermission(permissionCode string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, admin, permissions, err := a.authenticateAdmin(c.Request)
		if err != nil {
			writeError(c, http.StatusUnauthorized, "unauthorized", "Admin login required")
			c.Abort()
			return
		}
		c.Set("claims", claims)
		c.Set("currentAdmin", &admin)
		c.Set("adminPermissions", permissions)
		if strings.TrimSpace(permissionCode) != "" && !permissions[permissionCode] {
			writeError(c, http.StatusForbidden, "forbidden", "权限不足")
			c.Abort()
			return
		}
		c.Next()
	}
}

// requireStrictAdminPermission 用于敏感端点（如系统日志）：在常规权限校验之外，
// 额外要求调用者持有 super_admin 角色，使该能力不可委派给普通自定义角色。
func (a *App) requireStrictAdminPermission(permissionCode string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, admin, permissions, err := a.authenticateAdmin(c.Request)
		if err != nil {
			writeError(c, http.StatusUnauthorized, "unauthorized", "Admin login required")
			c.Abort()
			return
		}
		c.Set("claims", claims)
		c.Set("currentAdmin", &admin)
		c.Set("adminPermissions", permissions)
		if strings.TrimSpace(permissionCode) != "" && !permissions[permissionCode] {
			writeError(c, http.StatusForbidden, "forbidden", "权限不足")
			c.Abort()
			return
		}
		if !adminHasSuperAdminRole(admin) {
			writeError(c, http.StatusForbidden, "forbidden", "仅超级管理员可访问")
			c.Abort()
			return
		}
		c.Next()
	}
}

// adminHasSuperAdminRole 判断管理员是否持有处于启用状态的 super_admin 角色。
func adminHasSuperAdminRole(admin AdminUser) bool {
	for _, role := range admin.Roles {
		if role.Code == "super_admin" && role.Status == RoleStatusActive {
			return true
		}
	}
	return false
}

func (a *App) loadSettings() (AppSettings, error) {
	var settings AppSettings
	err := a.db.First(&settings, 1).Error
	if err != nil {
		return settings, err
	}
	if settings.RequestTimeoutSeconds <= 0 {
		settings.RequestTimeoutSeconds = a.cfg.RequestTimeoutSeconds
	}
	settings.ModelRoutingStrategy = normalizeModelRoutingStrategy(settings.ModelRoutingStrategy)
	return settings, err
}

func (a *App) recoverInterruptedImageGenerations() error {
	if a.db.Migrator().HasTable(&ImageGenerationJob{}) {
		return a.recoverGenerationQueue(time.Now().UTC())
	}
	return a.expireStaleImageGenerations(time.Now())
}

type sqlStateError interface {
	SQLState() string
}

func isMissingDatabaseObjectError(err error) bool {
	if err == nil {
		return false
	}
	var stateErr sqlStateError
	if errors.As(err, &stateErr) {
		switch stateErr.SQLState() {
		case "42P01":
			return true
		}
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "no such table") {
		return true
	}
	return strings.Contains(message, "relation ") && strings.Contains(message, " does not exist")
}

func aspectRatioToSize(value string) (string, bool) {
	switch value {
	case "", "1:1":
		return "1024x1024", true
	case "2:3", "3:4", "9:16", "9:21":
		return "1024x1536", true
	case "3:2", "4:3", "16:9", "21:9":
		return "1536x1024", true
	default:
		return "", false
	}
}

func mapProviderError(err *ProviderError) (int, string, string) {
	code, message, _ := publicProviderFailure(err)
	status := http.StatusBadGateway
	if code == "provider_policy_rejected" {
		status = http.StatusUnprocessableEntity
	} else if err != nil && err.HTTPStatus == http.StatusTooManyRequests {
		status = http.StatusServiceUnavailable
	}
	return status, code, message
}

func publicProviderFailure(err *ProviderError) (string, string, bool) {
	if err == nil {
		return "provider_error", "图片生成失败，请稍后再试。", false
	}
	if strings.TrimSpace(err.FailureStage) == providerFailureStageProviderAssetFetch {
		return "provider_asset_fetch_failed", providerVisibleMessage(err, "模型已返回图片结果，但平台下载图片失败，系统已自动重试，请稍后重新生成。"), true
	}
	if isProviderPolicyRejection(err) {
		return "provider_policy_rejected", providerVisibleMessage(err, "提示词可能触发平台安全策略，请调整后重试。"), false
	}
	switch {
	case err.HTTPStatus == http.StatusUnauthorized || err.HTTPStatus == http.StatusForbidden:
		return "provider_auth_failed", providerVisibleMessage(err, "图片服务鉴权失败，请联系管理员检查模型配置。"), false
	case err.HTTPStatus == http.StatusTooManyRequests:
		return "provider_rate_limited", providerVisibleMessage(err, "图片服务当前繁忙，请稍后重新生成。"), true
	case err.HTTPStatus == http.StatusInternalServerError || err.HTTPStatus == http.StatusBadGateway ||
		err.HTTPStatus == http.StatusServiceUnavailable || err.HTTPStatus == http.StatusGatewayTimeout:
		return "provider_unavailable", providerVisibleMessage(err, "图片服务暂时不可用，请稍后重新生成。"), true
	}
	switch strings.TrimSpace(err.Code) {
	case "provider_timeout":
		return "provider_timeout", providerVisibleMessage(err, "网络超时，生成失败，请点击重试。"), true
	case "provider_request_failed":
		return "provider_request_failed", providerVisibleMessage(err, "图片服务请求失败，可能是网络波动或供应商暂不可用，请稍后重新生成。"), true
	case "provider_decode_failed":
		return "provider_decode_failed", providerVisibleMessage(err, "图片服务返回异常，请稍后重新生成。"), true
	case "provider_empty_image":
		return "provider_empty_image", providerVisibleMessage(err, "图片服务未返回可识别的图片结果，请稍后重新生成。"), true
	case "provider_rate_limited":
		return "provider_rate_limited", providerVisibleMessage(err, "图片服务当前繁忙，请稍后重新生成。"), true
	default:
		return fallbackString(err.Code, "provider_error"), providerVisibleMessage(err, "图片生成失败，请稍后再试。"), isRetryableProviderError(err)
	}
}

func providerVisibleMessage(err *ProviderError, fallback string) string {
	if err != nil {
		if message := strings.TrimSpace(err.Message); message != "" {
			if isTechnicalProviderMessage(message) {
				return fallback
			}
			return message
		}
	}
	return fallback
}

func isTechnicalProviderMessage(message string) bool {
	text := strings.ToLower(strings.TrimSpace(message))
	if text == "" {
		return false
	}
	for _, marker := range []string{
		"http://",
		"https://",
		"context deadline exceeded",
		"client.timeout",
		"connection refused",
		"connection reset",
		"no such host",
		"tls handshake",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	for _, method := range []string{"post", "get", "put", "patch", "delete"} {
		if strings.Contains(text, method+" \"") {
			return true
		}
	}
	return false
}

func isProviderPolicyRejection(err *ProviderError) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(err.Code + " " + err.Message))
	for _, marker := range []string{
		"moderation_blocked",
		"safety_violations",
		"rejected by the safety system",
		"policy",
		"违反平台政策",
		"安全策略",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func isRetryableProviderError(err *ProviderError) bool {
	if err == nil {
		return false
	}
	if strings.TrimSpace(err.FailureStage) == providerFailureStageProviderAssetFetch {
		return false
	}
	switch strings.TrimSpace(err.Code) {
	case "provider_timeout", "provider_request_failed", "provider_decode_failed", "provider_rate_limited":
		return true
	}
	switch err.HTTPStatus {
	case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func writeJSON(c *gin.Context, status int, payload any) {
	c.JSON(status, payload)
}

func writeError(c *gin.Context, status int, code, message string) {
	c.Set(requestLogErrorCodeKey, code)
	c.Set(requestLogErrorMessageKey, message)
	c.JSON(status, gin.H{
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	})
}

func writeErrorWithLogDetail(c *gin.Context, status int, code, message, detail string) {
	c.Set(requestLogErrorDetailKey, sanitizeRequestLogDetail(detail))
	writeError(c, status, code, message)
}

func sanitizeRequestLogDetail(detail string) string {
	text := strings.TrimSpace(detail)
	if text == "" {
		return ""
	}
	text = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return ' '
		}
		return r
	}, text)
	text = strings.Join(strings.Fields(text), " ")
	const maxRequestLogDetailLength = 800
	runes := []rune(text)
	if len(runes) > maxRequestLogDetailLength {
		return string(runes[:maxRequestLogDetailLength]) + "..."
	}
	return text
}

func clientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		return strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	return r.RemoteAddr
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func fallbackString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func getQueryInt(c *gin.Context, key string, fallback int) int {
	value := c.Query(key)
	if value == "" {
		return fallback
	}
	number, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return number
}

func queryBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	number, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return number
}

func getenvInt64(key string, fallback int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	number, err := strconv.ParseInt(value, 10, 64)
	if err != nil || number < 0 {
		return fallback
	}
	return number
}

func getenvBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func splitCSV(value string) []string {
	raw := strings.Split(value, ",")
	items := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item != "" {
			items = append(items, item)
		}
	}
	return items
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
