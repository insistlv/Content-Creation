package app

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"

	"gorm.io/gorm"
)

var runtimeSecretNames = []string{
	"OPENAI_API_KEY", "DEEPSEEK_API_KEY", "JWT_SECRET",
	"OSS_ACCESS_KEY_ID", "OSS_ACCESS_KEY_SECRET",
	"AI_COMMERCE_OSS_ACCESS_KEY_ID", "AI_COMMERCE_OSS_ACCESS_KEY_SECRET",
	"ALIYUN_SMS_ACCESS_KEY_ID", "ALIYUN_SMS_ACCESS_KEY_SECRET", "ALIYUN_SMS_SIGN_NAME",
	"ALIYUN_SMS_REGISTER_TEMPLATE_CODE", "ALIYUN_SMS_RESET_TEMPLATE_CODE",
	"ALIPAY_APP_ID", "ALIPAY_PRIVATE_KEY", "ALIPAY_PUBLIC_KEY",
	"WECHAT_PAY_APP_ID", "WECHAT_PAY_MCH_ID", "WECHAT_PAY_MCH_CERT_SERIAL_NO",
	"WECHAT_PAY_MCH_PRIVATE_KEY", "WECHAT_PAY_API_V3_KEY", "WECHAT_PAY_PLATFORM_PUBLIC_KEY",
	"WECHAT_APP_SECRET", "WECHAT_VIRTUAL_PAY_OFFER_ID", "WECHAT_VIRTUAL_PAY_APP_KEY",
	"WECHAT_VIRTUAL_PAY_SANDBOX_APP_KEY", "ARK_API_KEY", "ZZ_API_KEY",
}

func readEnvOrFile(name string) (string, error) {
	if path := strings.TrimSpace(os.Getenv(name + "_FILE")); path != "" {
		value, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read %s_FILE: %w", name, err)
		}
		return strings.TrimRight(string(value), "\r\n"), nil
	}
	return os.Getenv(name), nil
}

func LoadSecretsBootstrapFromEnv() (string, []byte, int, error) {
	if err := loadDotEnv(".env"); err != nil {
		return "", nil, 0, fmt.Errorf("load .env: %w", err)
	}
	databaseURL, err := readEnvOrFile("DATABASE_URL")
	if err != nil {
		return "", nil, 0, err
	}
	if strings.TrimSpace(databaseURL) == "" {
		return "", nil, 0, errors.New("DATABASE_URL(_FILE) is required")
	}
	encoded, err := readEnvOrFile("APP_SECRETS_MASTER_KEY")
	if err != nil {
		return "", nil, 0, err
	}
	key, err := DecodeSecretsMasterKey(encoded)
	if err != nil {
		return "", nil, 0, err
	}
	return strings.TrimSpace(databaseURL), key, getenvInt("APP_SECRETS_KEY_VERSION", 1), nil
}

func ReadSecretValueFromEnv(name string) (string, error) { return readEnvOrFile(name) }

func prepareAppSecrets(ctx context.Context, db *gorm.DB, cfg Config) (Config, *SecretStore, error) {
	store, err := NewSecretStore(db, cfg.SecretsMasterKey, cfg.SecretsKeyVersion)
	if err != nil {
		return Config{}, nil, err
	}
	if err := store.Migrate(ctx); err != nil {
		return Config{}, nil, fmt.Errorf("migrate secret records: %w", err)
	}
	if err := migrateLegacyModelAPIKeys(ctx, db, store); err != nil {
		return Config{}, nil, fmt.Errorf("migrate legacy model API keys: %w", err)
	}
	for _, name := range runtimeSecretNames {
		value, _, err := store.Get(ctx, secretNamespaceRuntime, secretOwnerGlobal, name)
		if errors.Is(err, ErrSecretNotFound) {
			continue
		}
		if err != nil {
			return Config{}, nil, fmt.Errorf("load runtime secret %s: %w", name, err)
		}
		setRuntimeSecret(&cfg, name, value)
	}
	if strings.TrimSpace(cfg.JWTSecret) == "" {
		generated := make([]byte, 32)
		if _, err := rand.Read(generated); err != nil {
			return Config{}, nil, fmt.Errorf("generate JWT secret: %w", err)
		}
		cfg.JWTSecret = base64.RawURLEncoding.EncodeToString(generated)
		if err := store.Put(ctx, secretNamespaceRuntime, secretOwnerGlobal, "JWT_SECRET", cfg.JWTSecret, "system:first-start"); err != nil {
			return Config{}, nil, fmt.Errorf("store generated JWT secret: %w", err)
		}
	}
	return cfg, store, nil
}

func setRuntimeSecret(cfg *Config, name, value string) {
	switch name {
	case "OPENAI_API_KEY":
		cfg.OpenAIAPIKey = value
	case "DEEPSEEK_API_KEY":
		cfg.DeepSeekAPIKey = value
	case "JWT_SECRET":
		cfg.JWTSecret = value
	case "OSS_ACCESS_KEY_ID":
		cfg.OSSAccessKeyID = value
	case "OSS_ACCESS_KEY_SECRET":
		cfg.OSSAccessKeySecret = value
	case "AI_COMMERCE_OSS_ACCESS_KEY_ID":
		cfg.AICommerceOSSAccessKeyID = value
	case "AI_COMMERCE_OSS_ACCESS_KEY_SECRET":
		cfg.AICommerceOSSAccessKeySecret = value
	case "ALIYUN_SMS_ACCESS_KEY_ID":
		cfg.AliyunSMSAccessKeyID = value
	case "ALIYUN_SMS_ACCESS_KEY_SECRET":
		cfg.AliyunSMSAccessKeySecret = value
	case "ALIYUN_SMS_SIGN_NAME":
		cfg.AliyunSMSSignName = value
	case "ALIYUN_SMS_REGISTER_TEMPLATE_CODE":
		cfg.AliyunSMSRegisterTemplateCode = value
	case "ALIYUN_SMS_RESET_TEMPLATE_CODE":
		cfg.AliyunSMSResetTemplateCode = value
	case "ALIPAY_APP_ID":
		cfg.AlipayAppID = value
	case "ALIPAY_PRIVATE_KEY":
		cfg.AlipayPrivateKey = value
	case "ALIPAY_PUBLIC_KEY":
		cfg.AlipayPublicKey = value
	case "WECHAT_PAY_APP_ID":
		cfg.WechatPayAppID = value
	case "WECHAT_PAY_MCH_ID":
		cfg.WechatPayMchID = value
	case "WECHAT_PAY_MCH_CERT_SERIAL_NO":
		cfg.WechatPayMchCertSerialNo = value
	case "WECHAT_PAY_MCH_PRIVATE_KEY":
		cfg.WechatPayMchPrivateKey = value
	case "WECHAT_PAY_API_V3_KEY":
		cfg.WechatPayAPIv3Key = value
	case "WECHAT_PAY_PLATFORM_PUBLIC_KEY":
		cfg.WechatPayPlatformPublicKey = value
	case "WECHAT_APP_SECRET":
		cfg.WechatAppSecret = value
	case "WECHAT_VIRTUAL_PAY_OFFER_ID":
		cfg.WechatVirtualPayOfferID = value
	case "WECHAT_VIRTUAL_PAY_APP_KEY":
		cfg.WechatVirtualPayAppKey = value
	case "WECHAT_VIRTUAL_PAY_SANDBOX_APP_KEY":
		cfg.WechatVirtualPaySandboxAppKey = value
	case "ARK_API_KEY":
		cfg.ArkAPIKey = value
	case "ZZ_API_KEY":
		cfg.ZZAPIKey = value
	}
}

type legacyModelConfigSecret struct {
	ID     uint
	APIKey string `gorm:"column:api_key"`
}

func (legacyModelConfigSecret) TableName() string { return "model_configs" }

type legacyModelProviderSecret struct {
	ID     uint
	APIKey string `gorm:"column:api_key"`
}

func (legacyModelProviderSecret) TableName() string { return "model_providers" }

func migrateLegacyModelAPIKeys(ctx context.Context, db *gorm.DB, store *SecretStore) error {
	migrator := db.Migrator()
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if migrator.HasTable(&ModelConfig{}) && migrator.HasColumn(&ModelConfig{}, "APIKey") {
			var rows []legacyModelConfigSecret
			if err := tx.Where("TRIM(COALESCE(api_key, '')) <> ''").Find(&rows).Error; err != nil {
				return err
			}
			for _, row := range rows {
				var count int64
				if err := tx.Model(&SecretRecord{}).Where("namespace = ? AND owner_id = ? AND name = ?", "model_config", modelSecretOwner(row.ID), "api_key").Count(&count).Error; err != nil {
					return err
				}
				if count == 0 {
					if err := store.putDB(tx, "model_config", modelSecretOwner(row.ID), "api_key", row.APIKey, "system:legacy-import"); err != nil {
						return err
					}
				}
			}
			if len(rows) > 0 {
				if err := tx.Model(&legacyModelConfigSecret{}).Where("TRIM(COALESCE(api_key, '')) <> ''").Update("api_key", "").Error; err != nil {
					return err
				}
			}
		}
		if migrator.HasTable(&ModelProvider{}) && migrator.HasColumn(&ModelProvider{}, "APIKey") {
			var rows []legacyModelProviderSecret
			if err := tx.Where("TRIM(COALESCE(api_key, '')) <> ''").Find(&rows).Error; err != nil {
				return err
			}
			for _, row := range rows {
				var count int64
				if err := tx.Model(&SecretRecord{}).Where("namespace = ? AND owner_id = ? AND name = ?", "model_provider", modelSecretOwner(row.ID), "api_key").Count(&count).Error; err != nil {
					return err
				}
				if count == 0 {
					if err := store.putDB(tx, "model_provider", modelSecretOwner(row.ID), "api_key", row.APIKey, "system:legacy-import"); err != nil {
						return err
					}
				}
			}
			if len(rows) > 0 {
				if err := tx.Model(&legacyModelProviderSecret{}).Where("TRIM(COALESCE(api_key, '')) <> ''").Update("api_key", "").Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func ImportRuntimeSecretsFromEnv(ctx context.Context, store *SecretStore, actor string) (int, error) {
	count := 0
	for _, name := range runtimeSecretNames {
		value, err := readEnvOrFile(name)
		if err != nil {
			return count, err
		}
		if strings.TrimSpace(value) == "" {
			continue
		}
		if err := store.Put(ctx, secretNamespaceRuntime, secretOwnerGlobal, name, value, actor); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
