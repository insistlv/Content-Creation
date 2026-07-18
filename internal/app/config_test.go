package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testSecretsMasterKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

func setTestSecretsMasterKey(t *testing.T) {
	t.Helper()
	t.Setenv("APP_SECRETS_MASTER_KEY", testSecretsMasterKey)
	t.Setenv("APP_SECRETS_MASTER_KEY_FILE", "")
}

func TestLoadConfigFromEnvReadsDotEnvFile(t *testing.T) {
	setTestSecretsMasterKey(t)
	t.Setenv("APP_BASE_URL", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("JWT_SECRET", "")
	t.Setenv("ADMIN_USERNAME", "")
	t.Setenv("ADMIN_PASSWORD", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DEFAULT_IMAGE_MODEL", "")
	t.Setenv("ALLOWED_IMAGE_MODELS", "")
	t.Setenv("REQUEST_TIMEOUT_SECONDS", "")
	t.Setenv("RATE_LIMIT_WINDOW_SECONDS", "")
	t.Setenv("RATE_LIMIT_MAX_REQUESTS", "")
	t.Setenv("DEFAULT_INVITE_QUOTA", "")
	t.Setenv("FRONTEND_DIST_PATH", "")

	tempDir := t.TempDir()
	dotenvPath := filepath.Join(tempDir, ".env")
	dotenv := []byte("APP_BASE_URL=http://localhost:8080\n" +
		"OPENAI_API_KEY=test-key\n" +
		"JWT_SECRET=test-secret\n" +
		"ADMIN_USERNAME=admin\n" +
		"ADMIN_PASSWORD=change-me\n" +
		"DATABASE_URL=postgres://user:pass@localhost:5432/image_agent?sslmode=disable\n")
	if err := os.WriteFile(dotenvPath, dotenv, 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restore wd: %v", err)
		}
	})

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv() error = %v", err)
	}

	if cfg.AppBaseURL != "http://localhost:8080" {
		t.Fatalf("expected APP_BASE_URL from .env, got %q", cfg.AppBaseURL)
	}
	if cfg.OpenAIAPIKey != "" || cfg.JWTSecret != "" || cfg.AdminUsername != "" || cfg.AdminPassword != "" {
		t.Fatalf("business secrets must not be loaded before the database is connected")
	}
	if cfg.DatabaseURL != "postgres://user:pass@localhost:5432/image_agent?sslmode=disable" {
		t.Fatalf("expected DATABASE_URL from .env, got %q", cfg.DatabaseURL)
	}
	if cfg.RequestTimeoutSeconds != 600 {
		t.Fatalf("expected default REQUEST_TIMEOUT_SECONDS 600, got %d", cfg.RequestTimeoutSeconds)
	}
	if cfg.RateLimitMaxRequests != 20 {
		t.Fatalf("expected default RATE_LIMIT_MAX_REQUESTS 20, got %d", cfg.RateLimitMaxRequests)
	}
}

func TestLoadConfigFromEnvReadsAlipayConfig(t *testing.T) {
	setTestSecretsMasterKey(t)
	t.Setenv("APP_BASE_URL", "https://example.com")
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/image_agent?sslmode=disable")
	t.Setenv("ALIPAY_APP_ID", "2026000000000001")
	t.Setenv("ALIPAY_PRIVATE_KEY", "private-key")
	t.Setenv("ALIPAY_PUBLIC_KEY", "public-key")
	t.Setenv("ALIPAY_GATEWAY", " ")
	t.Setenv("ALIPAY_SANDBOX", "1")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv() error = %v", err)
	}

	if cfg.AlipayAppID != "" || cfg.AlipayPrivateKey != "" || cfg.AlipayPublicKey != "" || !cfg.AlipaySandbox {
		t.Fatalf("unexpected alipay config: %+v", cfg)
	}
	if cfg.AlipayGateway != "https://openapi-sandbox.dl.alipaydev.com/gateway.do" {
		t.Fatalf("expected sandbox gateway default, got %q", cfg.AlipayGateway)
	}
}

func TestLoadConfigFromEnvFindsDotEnvInParentDirectory(t *testing.T) {
	setTestSecretsMasterKey(t)
	t.Setenv("APP_BASE_URL", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("JWT_SECRET", "")
	t.Setenv("ADMIN_USERNAME", "")
	t.Setenv("ADMIN_PASSWORD", "")
	t.Setenv("DATABASE_URL", "")

	tempDir := t.TempDir()
	dotenvPath := filepath.Join(tempDir, ".env")
	dotenv := []byte("APP_BASE_URL=http://localhost:8080\n" +
		"OPENAI_API_KEY=test-key\n" +
		"JWT_SECRET=test-secret\n" +
		"ADMIN_USERNAME=admin\n" +
		"ADMIN_PASSWORD=change-me\n" +
		"DATABASE_URL=postgres://user:pass@localhost:5432/image_agent?sslmode=disable\n")
	if err := os.WriteFile(dotenvPath, dotenv, 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	childDir := filepath.Join(tempDir, "cmd", "server")
	if err := os.MkdirAll(childDir, 0o755); err != nil {
		t.Fatalf("mkdir child dir: %v", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(childDir); err != nil {
		t.Fatalf("chdir child dir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restore wd: %v", err)
		}
	})

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv() error = %v", err)
	}

	if cfg.AppBaseURL != "http://localhost:8080" {
		t.Fatalf("expected APP_BASE_URL from parent .env, got %q", cfg.AppBaseURL)
	}
}

func TestLoadConfigFromEnvReadsSystemStatusSettings(t *testing.T) {
	setTestSecretsMasterKey(t)
	t.Setenv("APP_BASE_URL", "http://localhost:8080")
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/image_agent?sslmode=disable")
	t.Setenv("APP_VERSION", "2026.05-test")
	t.Setenv("SYSTEM_STORAGE_CAPACITY_BYTES", "1048576")
	t.Setenv("SYSTEM_CDN_TRAFFIC_BYTES", "2048")
	t.Setenv("SYSTEM_CDN_TRAFFIC_LIMIT_BYTES", "4096")
	t.Setenv("SYSTEM_DAILY_GENERATION_LIMIT", "300")

	tempDir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restore wd: %v", err)
		}
	})

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv() error = %v", err)
	}
	if cfg.AppVersion != "2026.05-test" {
		t.Fatalf("expected APP_VERSION from env, got %q", cfg.AppVersion)
	}
	if cfg.SystemStorageCapacityBytes != 1048576 ||
		cfg.SystemCDNTrafficBytes != 2048 ||
		cfg.SystemCDNTrafficLimitBytes != 4096 ||
		cfg.SystemDailyGenerationLimit != 300 {
		t.Fatalf("unexpected system status config: %+v", cfg)
	}
}

func TestLoadConfigFromEnvReadsStartupDatabaseBootstrapFlag(t *testing.T) {
	setTestSecretsMasterKey(t)
	t.Setenv("APP_BASE_URL", "http://localhost:8080")
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/image_agent?sslmode=disable")
	t.Setenv("STARTUP_DATABASE_MIGRATIONS", "")
	t.Setenv("STARTUP_DATABASE_BOOTSTRAP", "1")

	tempDir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restore wd: %v", err)
		}
	})

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv() error = %v", err)
	}
	if !cfg.StartupDatabaseBootstrap {
		t.Fatal("expected STARTUP_DATABASE_BOOTSTRAP=1 to enable startup database bootstrap")
	}
	if cfg.StartupDatabaseMigrations != StartupDatabaseMigrationsBootstrap {
		t.Fatalf("expected legacy bootstrap flag to resolve bootstrap migrations, got %q", cfg.StartupDatabaseMigrations)
	}
}

func TestLoadConfigFromEnvReadsStartupDatabaseMigrationsSkip(t *testing.T) {
	setTestSecretsMasterKey(t)
	t.Setenv("APP_BASE_URL", "http://localhost:8080")
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/image_agent?sslmode=disable")
	t.Setenv("STARTUP_DATABASE_BOOTSTRAP", "1")
	t.Setenv("STARTUP_DATABASE_MIGRATIONS", "skip")

	tempDir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restore wd: %v", err)
		}
	})

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv() error = %v", err)
	}
	if cfg.StartupDatabaseMigrations != StartupDatabaseMigrationsSkip {
		t.Fatalf("expected STARTUP_DATABASE_MIGRATIONS=skip, got %q", cfg.StartupDatabaseMigrations)
	}
	if cfg.StartupDatabaseBootstrap {
		t.Fatal("expected STARTUP_DATABASE_MIGRATIONS to take priority over legacy bootstrap flag")
	}
}

func TestLoadConfigFromEnvRejectsInvalidStartupDatabaseMigrations(t *testing.T) {
	setTestSecretsMasterKey(t)
	t.Setenv("APP_BASE_URL", "http://localhost:8080")
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/image_agent?sslmode=disable")
	t.Setenv("STARTUP_DATABASE_MIGRATIONS", "fast")

	tempDir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restore wd: %v", err)
		}
	})

	_, err = LoadConfigFromEnv()
	if err == nil {
		t.Fatal("expected invalid STARTUP_DATABASE_MIGRATIONS to fail")
	}
	if !strings.Contains(err.Error(), "STARTUP_DATABASE_MIGRATIONS") {
		t.Fatalf("expected error to mention STARTUP_DATABASE_MIGRATIONS, got %v", err)
	}
}

func TestLoadConfigFromEnvRequiresDatabaseURL(t *testing.T) {
	setTestSecretsMasterKey(t)
	t.Setenv("APP_BASE_URL", "http://localhost:8080")
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("DATABASE_URL", "")

	tempDir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restore wd: %v", err)
		}
	})

	_, err = LoadConfigFromEnv()
	if err == nil {
		t.Fatal("expected missing DATABASE_URL to fail")
	}
}

func TestLoadConfigFromEnvDefersOSSSecretValidationUntilAfterDatabaseLoad(t *testing.T) {
	setTestSecretsMasterKey(t)
	t.Setenv("APP_BASE_URL", "http://localhost:8080")
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/image_agent?sslmode=disable")
	t.Setenv("STORAGE_TYPE", "oss")
	t.Setenv("OSS_ENDPOINT", "https://oss-cn-shenzhen.aliyuncs.com")
	t.Setenv("OSS_ACCESS_KEY_ID", "access-key")
	t.Setenv("OSS_ACCESS_KEY_SECRET", "secret-key")
	t.Setenv("OSS_BUCKET", "example-assets")
	t.Setenv("OSS_PUBLIC_BASE_URL", "")

	tempDir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restore wd: %v", err)
		}
	})

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv() error = %v", err)
	}
	if cfg.OSSAccessKeyID != "" || cfg.OSSAccessKeySecret != "" {
		t.Fatal("OSS credentials must not be loaded from the process environment")
	}
}

func TestLoadConfigFromEnvReadsOSSConfig(t *testing.T) {
	setTestSecretsMasterKey(t)
	t.Setenv("APP_BASE_URL", "http://localhost:8080")
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/image_agent?sslmode=disable")
	t.Setenv("STORAGE_TYPE", "oss")
	t.Setenv("OSS_ENDPOINT", "https://oss-cn-shenzhen.aliyuncs.com")
	t.Setenv("OSS_ACCESS_KEY_ID", "access-key")
	t.Setenv("OSS_ACCESS_KEY_SECRET", "secret-key")
	t.Setenv("OSS_BUCKET", "example-assets")
	t.Setenv("OSS_PUBLIC_BASE_URL", "https://example-assets.oss-cn-shenzhen.aliyuncs.com")
	t.Setenv("OSS_BASE_PATH", "assets/")

	tempDir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restore wd: %v", err)
		}
	})

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv() error = %v", err)
	}
	if cfg.StorageType != "oss" ||
		cfg.OSSEndpoint != "https://oss-cn-shenzhen.aliyuncs.com" ||
		cfg.OSSBucket != "example-assets" ||
		cfg.OSSPublicBaseURL != "https://example-assets.oss-cn-shenzhen.aliyuncs.com" ||
		cfg.OSSBasePath != "assets/" {
		t.Fatalf("unexpected OSS config: %+v", cfg)
	}
}
