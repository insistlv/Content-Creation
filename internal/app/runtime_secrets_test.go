package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPrepareAppSecretsMigratesLegacyKeysAndPersistsJWT(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&ModelConfig{}, &ModelProvider{}); err != nil {
		t.Fatal(err)
	}
	model := ModelConfig{Name: "legacy", APIKey: "legacy-model-secret"}
	provider := ModelProvider{Name: "legacy", Provider: "legacy", APIKey: "legacy-provider-secret"}
	if err := db.Create(&model).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&provider).Error; err != nil {
		t.Fatal(err)
	}
	key := []byte("0123456789abcdef0123456789abcdef")
	cfg, store, err := prepareAppSecrets(context.Background(), db, Config{SecretsMasterKey: key, SecretsKeyVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.JWTSecret == "" {
		t.Fatal("JWT secret was not generated")
	}
	var rawModel, rawProvider string
	if err := db.Model(&ModelConfig{}).Select("api_key").Where("id = ?", model.ID).Scan(&rawModel).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&ModelProvider{}).Select("api_key").Where("id = ?", provider.ID).Scan(&rawProvider).Error; err != nil {
		t.Fatal(err)
	}
	if rawModel != "" || rawProvider != "" {
		t.Fatal("legacy plaintext API key was not cleared")
	}
	value, _, err := store.Get(context.Background(), "model_config", modelSecretOwner(model.ID), "api_key")
	if err != nil || value != "legacy-model-secret" {
		t.Fatalf("model secret = %q, %v", value, err)
	}
	value, _, err = store.Get(context.Background(), "model_provider", modelSecretOwner(provider.ID), "api_key")
	if err != nil || value != "legacy-provider-secret" {
		t.Fatalf("provider secret = %q, %v", value, err)
	}
	second, _, err := prepareAppSecrets(context.Background(), db, Config{SecretsMasterKey: key, SecretsKeyVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	if second.JWTSecret != cfg.JWTSecret {
		t.Fatal("JWT secret changed across restart")
	}
}

func TestSecretSettingsResponseNeverContainsPlaintext(t *testing.T) {
	store, db, _ := testSecretStore(t)
	plaintext := "must-never-leave-the-server"
	if err := store.Put(context.Background(), secretNamespaceRuntime, secretOwnerGlobal, "OPENAI_API_KEY", plaintext, "test"); err != nil {
		t.Fatal(err)
	}
	a := &App{db: db, secretStore: store}
	items, err := a.secretSettingsResponse()
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), plaintext) {
		t.Fatal("secret settings response leaked plaintext")
	}
	if !strings.Contains(string(payload), `"configured":true`) {
		t.Fatal("configured status missing")
	}
}
