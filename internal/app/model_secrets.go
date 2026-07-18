package app

import (
	"context"
	"errors"
	"log"
	"strings"
)

func (a *App) syncLegacyModelSecretsToProviders() error {
	if a.secretStore == nil {
		return nil
	}
	var channels []ModelChannel
	if err := a.db.Where("legacy_model_config_id <> 0 AND provider_id <> 0").Find(&channels).Error; err != nil {
		return err
	}
	ctx := context.Background()
	for _, channel := range channels {
		configured, _, err := a.secretStore.Configured(ctx, "model_provider", modelSecretOwner(channel.ProviderID), "api_key")
		if err != nil {
			return err
		}
		if configured {
			continue
		}
		value, _, err := a.secretStore.Get(ctx, "model_config", modelSecretOwner(channel.LegacyModelConfigID), "api_key")
		if errors.Is(err, ErrSecretNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		if err := a.secretStore.Put(ctx, "model_provider", modelSecretOwner(channel.ProviderID), "api_key", value, "system:model-center-sync"); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) hydrateModelConfig(model *ModelConfig) error {
	if model == nil || model.ID == 0 || a.secretStore == nil {
		return nil
	}
	value, _, err := a.secretStore.Get(context.Background(), "model_config", modelSecretOwner(model.ID), "api_key")
	if errors.Is(err, ErrSecretNotFound) {
		model.APIKey = ""
		return nil
	}
	if err != nil {
		return err
	}
	model.APIKey = value
	return nil
}

func (a *App) hydrateModelProvider(provider *ModelProvider) error {
	if provider == nil || provider.ID == 0 || a.secretStore == nil {
		return nil
	}
	value, _, err := a.secretStore.Get(context.Background(), "model_provider", modelSecretOwner(provider.ID), "api_key")
	if errors.Is(err, ErrSecretNotFound) {
		provider.APIKey = ""
		return nil
	}
	if err != nil {
		return err
	}
	provider.APIKey = value
	return nil
}

func (a *App) modelConfigAPIKeyConfigured(model ModelConfig) bool {
	if a.secretStore == nil {
		return strings.TrimSpace(model.APIKey) != ""
	}
	configured, _, err := a.secretStore.Configured(context.Background(), "model_config", modelSecretOwner(model.ID), "api_key")
	if err != nil {
		log.Printf("model secret status failed model_config_id=%d: %v", model.ID, err)
		return false
	}
	return configured
}

func (a *App) modelProviderAPIKeyConfigured(provider ModelProvider) bool {
	if a.secretStore == nil {
		return strings.TrimSpace(provider.APIKey) != ""
	}
	configured, _, err := a.secretStore.Configured(context.Background(), "model_provider", modelSecretOwner(provider.ID), "api_key")
	if err != nil {
		log.Printf("provider secret status failed provider_id=%d: %v", provider.ID, err)
		return false
	}
	return configured
}

func (a *App) saveModelConfigAPIKey(modelID uint, value string, clear bool, actor string) error {
	if a.secretStore == nil {
		return nil
	}
	if clear {
		return a.secretStore.Delete(context.Background(), "model_config", modelSecretOwner(modelID), "api_key")
	}
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return a.secretStore.Put(context.Background(), "model_config", modelSecretOwner(modelID), "api_key", strings.TrimSpace(value), actor)
}

func (a *App) saveModelProviderAPIKey(providerID uint, value string, clear bool, actor string) error {
	if a.secretStore == nil {
		return nil
	}
	if clear {
		return a.secretStore.Delete(context.Background(), "model_provider", modelSecretOwner(providerID), "api_key")
	}
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return a.secretStore.Put(context.Background(), "model_provider", modelSecretOwner(providerID), "api_key", strings.TrimSpace(value), actor)
}
