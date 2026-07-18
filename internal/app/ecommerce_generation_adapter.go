package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"gorm.io/gorm"
	"image-agent/internal/app/ecommerce"
)

// commerceGenerationBackend is deliberately keyless. Recipe-specific executors
// bind keys around it; this backend is never registered directly.
type commerceGenerationBackend struct{ app *App }

func (b *commerceGenerationBackend) Execute(ctx context.Context, request ecommerce.ItemExecutionRequest) (ecommerce.ExecutionResult, *ecommerce.ExecutionFailure) {
	if err := ctx.Err(); err != nil {
		return ecommerce.ExecutionResult{}, commerceContextFailure(err)
	}
	if b == nil || b.app == nil {
		return ecommerce.ExecutionResult{}, &ecommerce.ExecutionFailure{Code: "executor_unavailable", Message: "commerce generation backend is unavailable", Retryable: true}
	}
	a := b.app
	executionKey := fmt.Sprintf("commerce:item:%d", request.Item.ID)
	if request.Compiled.RecipeKey == ecommerce.ProductDetailSetRecipeKey {
		if _, err := replayProductDetailOutputSnapshot(request.Compiled); err != nil {
			return ecommerce.ExecutionResult{}, &ecommerce.ExecutionFailure{Code: "layout_invalid", Message: err.Error()}
		}
	}
	metadataJSON := ""
	options := generationExecutionOptions{
		Context: ctx, BillingMode: generationBillingExternalReservation,
		ResultStorageScope: StorageScopeCommercePrivate, ResultWorkCategory: fallbackString(request.Compiled.WorkCategory, WorkCategoryImage),
		IdempotencyKey: executionKey, CommerceProjectID: request.Item.ProjectID,
	}
	if request.Compiled.RecipeKey == ecommerce.ProductDetailSetRecipeKey && strings.TrimSpace(request.Compiled.PostProcessJSON) != "" {
		options.TransformResult = func(image, mimeType string) (string, string, error) {
			var err error
			image, mimeType, metadataJSON, err = transformCommerceProductDetailResult(image, mimeType, request.Compiled.PostProcessJSON)
			return image, mimeType, err
		}
	}

	record, existing, failure := a.bindCommerceGenerationRecord(ctx, request, executionKey)
	if failure != nil {
		return ecommerce.ExecutionResult{}, failure
	}
	a.advanceCommerceItemProgress(ctx, request.Item, 25)
	if existing && record.Status == GenerationStatusSucceeded && record.WorkID != nil {
		metadataJSON, err := replayProductDetailOutputSnapshot(request.Compiled)
		if err != nil {
			return ecommerce.ExecutionResult{}, &ecommerce.ExecutionFailure{Code: "layout_invalid", Message: err.Error()}
		}
		return ecommerce.ExecutionResult{GenerationRecordID: record.ID, WorkID: *record.WorkID, ActualCredits: generationRecordCreditCost(record), MetadataJSON: metadataJSON}, nil
	}
	job, err := a.commerceGenerationJob(ctx, request)
	if err != nil {
		if errors.Is(err, ecommerce.ErrRecipeReferenceLimitExceeded) {
			return ecommerce.ExecutionResult{}, &ecommerce.ExecutionFailure{Code: "recipe_reference_limit_exceeded", Message: err.Error()}
		}
		if errors.Is(err, ecommerce.ErrRecipeModelUnavailable) {
			return ecommerce.ExecutionResult{}, &ecommerce.ExecutionFailure{Code: "recipe_model_unavailable", Message: err.Error()}
		}
		return ecommerce.ExecutionResult{}, &ecommerce.ExecutionFailure{Code: "generation_input_invalid", Message: err.Error()}
	}
	if existing && !commerceGenerationReplaySafe(record, job) {
		return ecommerce.ExecutionResult{}, &ecommerce.ExecutionFailure{Code: "provider_result_unknown", Message: "provider result is unknown and cannot be replayed safely", ResultUnknown: true}
	}
	a.advanceCommerceItemProgress(ctx, request.Item, 60)
	result, providerErr, err := a.executeGenerationRecordWithOptions(&record, job, options)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return ecommerce.ExecutionResult{}, commerceContextFailure(err)
		}
		if errors.Is(err, ecommerce.ErrLayoutTextOverflow) || errors.Is(err, ecommerce.ErrLayoutInvalid) {
			return ecommerce.ExecutionResult{}, &ecommerce.ExecutionFailure{Code: errorCodeForCommerceLayout(err), Message: err.Error()}
		}
		return ecommerce.ExecutionResult{}, &ecommerce.ExecutionFailure{Code: "generation_failed", Message: err.Error(), Retryable: true}
	}
	if providerErr != nil {
		if providerErr.Code == "provider_result_unknown" {
			return ecommerce.ExecutionResult{}, &ecommerce.ExecutionFailure{Code: providerErr.Code, Message: providerErr.Message, ResultUnknown: true}
		}
		return ecommerce.ExecutionResult{}, &ecommerce.ExecutionFailure{Code: fallbackString(providerErr.Code, "provider_error"), Message: providerErr.Message, Retryable: providerErrorTriggersSameChannelRetry(providerErr)}
	}
	a.advanceCommerceItemProgress(ctx, request.Item, 90)
	return ecommerce.ExecutionResult{GenerationRecordID: result.Record.ID, WorkID: *result.Record.WorkID, ActualCredits: generationRecordCreditCost(result.Record), MetadataJSON: metadataJSON}, nil
}

func (a *App) advanceCommerceItemProgress(ctx context.Context, item ecommerce.CommerceGenerationItem, percent int) {
	if a == nil || a.db == nil || item.ID == 0 || percent <= 0 {
		return
	}
	_ = a.db.WithContext(ctx).Model(&ecommerce.CommerceGenerationItem{}).
		Where("id = ? AND user_id = ? AND status = ? AND progress_percent < ?", item.ID, item.UserID, ecommerce.CommerceItemRunning, percent).
		Update("progress_percent", percent).Error
}

func replayProductDetailOutputSnapshot(compiled ecommerce.CompiledGenerationItem) (string, error) {
	if compiled.RecipeKey != ecommerce.ProductDetailSetRecipeKey {
		return "", nil
	}
	var document ecommerce.LayoutDocument
	decoder := json.NewDecoder(strings.NewReader(compiled.LayoutDocumentJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return "", fmt.Errorf("%w: replay layout: %v", ecommerce.ErrLayoutInvalid, err)
	}
	if err := ecommerce.ValidateLayoutDocument(document); err != nil {
		return "", err
	}
	sha := compiled.LayoutDocumentSHA256
	raw, _ := ecommerce.EncodeJSON(document)
	sum := sha256.Sum256([]byte(raw))
	actualSHA := hex.EncodeToString(sum[:])
	if sha != "" && sha != actualSHA {
		return "", fmt.Errorf("%w: layout document SHA mismatch", ecommerce.ErrLayoutInvalid)
	}
	sha = actualSHA
	sourceSize := map[string]string{"1:1": "1024x1024", "3:4": "1024x1536", "4:5": "1024x1536", "9:16": "1024x1536"}[compiled.AspectRatio]
	metadata := ecommerce.LayoutRenderMetadata{LayoutVersion: document.Version, LayoutSHA256: sha, SourceSize: sourceSize, OutputSize: fmt.Sprintf("%dx%d", document.Canvas.Width, document.Canvas.Height), CropMode: "center_cover"}
	return ecommerce.EncodeJSON(metadata)
}

func errorCodeForCommerceLayout(err error) string {
	if errors.Is(err, ecommerce.ErrLayoutTextOverflow) {
		return "layout_text_overflow"
	}
	return "layout_invalid"
}

func transformCommerceProductDetailResult(encoded, mimeType, documentJSON string) (string, string, string, error) {
	var document ecommerce.LayoutDocument
	decoder := json.NewDecoder(strings.NewReader(documentJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return "", "", "", fmt.Errorf("%w: decode layout document: %v", ecommerce.ErrLayoutInvalid, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return "", "", "", fmt.Errorf("%w: layout document trailing data", ecommerce.ErrLayoutInvalid)
	}
	source, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", "", "", fmt.Errorf("decode provider image: %w", err)
	}
	output, metadata, err := ecommerce.RenderLayout(source, mimeType, document)
	if err != nil {
		return "", "", "", err
	}
	metadataJSON, err := ecommerce.EncodeJSON(metadata)
	if err != nil {
		return "", "", "", err
	}
	return base64.StdEncoding.EncodeToString(output), "image/png", metadataJSON, nil
}

func (a *App) bindCommerceGenerationRecord(ctx context.Context, request ecommerce.ItemExecutionRequest, executionKey string) (GenerationRecord, bool, *ecommerce.ExecutionFailure) {
	var record GenerationRecord
	existing := false
	err := a.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Where("execution_key = ?", executionKey).First(&record).Error
		if err == nil {
			existing = true
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		} else {
			styleStrength, referenceWeight := 65, 75
			job := &generationJob{User: User{ID: request.Item.UserID}, Request: generationRequest{Prompt: request.Compiled.Prompt, NegativePrompt: request.Compiled.NegativePrompt, AspectRatio: request.Compiled.AspectRatio, Quality: request.Item.QualityTier, ToolMode: fallbackString(request.Compiled.ToolMode, GenerationToolModeGenerate), StyleStrength: &styleStrength, ReferenceWeight: &referenceWeight, ReferenceIntent: request.Compiled.ReferenceIntent, Num: 1}}
			record = GenerationRecord{UserID: request.Item.UserID, Prompt: job.Request.Prompt, NegativePrompt: job.Request.NegativePrompt, AspectRatio: job.Request.AspectRatio, Quality: job.Request.Quality, ToolMode: job.Request.ToolMode, StyleStrength: styleStrength, ReferenceWeight: referenceWeight, Status: GenerationStatusQueued, Stage: GenerationStageQueued, CreditsCost: fallbackPositive(request.Compiled.EstimatedCredits, 1), StorageScope: StorageScopeDefault, ExecutionKey: &executionKey}
			if err := tx.Create(&record).Error; err != nil {
				return err
			}
		}
		if request.Job.ID != 0 {
			var leased ecommerce.CommerceJob
			if err := tx.Where("id = ? AND lease_owner = ? AND lease_token = ?", request.Lease.JobID, request.Lease.LeaseOwner, request.Lease.LeaseToken).First(&leased).Error; err != nil {
				return err
			}
		}
		return tx.Model(&ecommerce.CommerceGenerationItem{}).Where("id = ? AND user_id = ?", request.Item.ID, request.Item.UserID).Update("generation_record_id", record.ID).Error
	})
	if err != nil {
		return GenerationRecord{}, false, &ecommerce.ExecutionFailure{Code: "generation_bind_failed", Message: err.Error(), Retryable: true}
	}
	return record, existing, nil
}

func (a *App) commerceGenerationJob(ctx context.Context, request ecommerce.ItemExecutionRequest) (*generationJob, error) {
	settings, err := a.loadSettings()
	if err != nil {
		return nil, err
	}
	var user User
	if err := a.db.WithContext(ctx).First(&user, request.Item.UserID).Error; err != nil {
		return nil, err
	}
	refs := make([]ReferenceAsset, 0, 4)
	if request.Compiled.RecipeKey == ecommerce.ProductDetailSetRecipeKey {
		frozen, err := ecommerce.DecodeExecutionReferenceSetSnapshot(request.Compiled.ExecutionReferenceSnapshotJSON)
		if err != nil {
			return nil, err
		}
		for _, frozenReference := range frozen.References {
			var boundary ecommerce.CommerceAsset
			if err := a.db.WithContext(ctx).Where("id = ? AND user_id = ? AND project_id = ? AND role = ?", frozenReference.CommerceAssetID, request.Item.UserID, request.Item.ProjectID, frozenReference.Role).First(&boundary).Error; err != nil {
				return nil, ecommerce.ErrOwnershipMismatch
			}
			var ref ReferenceAsset
			if err := a.db.WithContext(ctx).Where("id = ? AND user_id = ? AND storage_scope = ?", frozenReference.ReferenceAssetID, request.Item.UserID, StorageScopeCommercePrivate).First(&ref).Error; err != nil {
				return nil, ecommerce.ErrOwnershipMismatch
			}
			if a.commerceAssets == nil {
				return nil, ecommerce.ErrOwnershipMismatch
			}
			if err := a.commerceAssets.EnsureObjectGuard(ctx, request.Item.UserID, StorageScopeCommercePrivate, ref.AssetKey); err != nil {
				return nil, ecommerce.ErrOwnershipMismatch
			}
			refs = append(refs, ref)
		}
	} else {
		if len(request.Compiled.AssetIDs) > 4 {
			return nil, ecommerce.ErrRecipeReferenceLimitExceeded
		}
		for _, commerceAssetID := range request.Compiled.AssetIDs {
			var binding ecommerce.CommerceAsset
			if err := a.db.WithContext(ctx).Where("id = ? AND user_id = ?", commerceAssetID, request.Item.UserID).First(&binding).Error; err != nil {
				return nil, err
			}
			var ref ReferenceAsset
			if err := a.db.WithContext(ctx).Where("id = ? AND user_id = ?", binding.ReferenceAssetID, request.Item.UserID).First(&ref).Error; err != nil {
				return nil, err
			}
			refs = append(refs, ref)
		}
	}
	styleStrength, referenceWeight := 65, 75
	size, ok := aspectRatioToSize(request.Compiled.AspectRatio)
	if request.Compiled.AspectRatio == "4:5" {
		size, ok = "1024x1536", true
	}
	if !ok {
		return nil, fmt.Errorf("unsupported aspect ratio %q", request.Compiled.AspectRatio)
	}
	job := &generationJob{User: user, Settings: settings, ReferenceAssets: refs, Request: generationRequest{Prompt: request.Compiled.Prompt, NegativePrompt: request.Compiled.NegativePrompt, AspectRatio: request.Compiled.AspectRatio, Quality: request.Item.QualityTier, ToolMode: fallbackString(request.Compiled.ToolMode, GenerationToolModeGenerate), StyleStrength: &styleStrength, ReferenceWeight: &referenceWeight, ReferenceIntent: request.Compiled.ReferenceIntent, Size: size, Num: 1}}
	if request.Compiled.RecipeKey == ecommerce.ProductDetailSetRecipeKey && strings.TrimSpace(request.Compiled.ModelSnapshotJSON) != "" {
		var frozen struct {
			Candidates []struct {
				ModelID      uint   `json:"model_id"`
				ChannelID    uint   `json:"channel_id"`
				ProviderID   uint   `json:"provider_id"`
				RuntimeModel string `json:"runtime_model"`
				Endpoint     string `json:"endpoint"`
			} `json:"candidates"`
		}
		if err := json.Unmarshal([]byte(request.Compiled.ModelSnapshotJSON), &frozen); err != nil {
			return nil, fmt.Errorf("decode frozen model snapshot: %w", err)
		}
		for _, frozenCandidate := range frozen.Candidates {
			if frozenCandidate.ModelID == 0 || frozenCandidate.ChannelID == 0 || frozenCandidate.ProviderID == 0 {
				continue
			}
			var channel ModelChannel
			if err := a.db.WithContext(ctx).Preload("Model").Preload("Provider").First(&channel, frozenCandidate.ChannelID).Error; err != nil {
				continue
			}
			if channel.ModelID != frozenCandidate.ModelID || channel.ProviderID != frozenCandidate.ProviderID {
				continue
			}
			channel.RuntimeModel, channel.Endpoint = frozenCandidate.RuntimeModel, frozenCandidate.Endpoint
			job.ModelCenterCandidates = append(job.ModelCenterCandidates, modelCenterCandidate{Model: channel.Model, Channel: channel, Provider: channel.Provider})
		}
		if len(job.ModelCenterCandidates) == 0 {
			return nil, ecommerce.ErrRecipeModelUnavailable
		}
	} else {
		job.ModelCenterCandidates, err = a.modelCenterCandidatesForGeneration(settings, ModelConfigTypeImage, 0)
		if err != nil {
			return nil, err
		}
	}
	if len(job.ModelCenterCandidates) > 0 {
		job.ModelCenterModel, job.ModelCenterChannel = &job.ModelCenterCandidates[0].Model, &job.ModelCenterCandidates[0].Channel
	} else {
		job.ModelCandidates, err = a.modelConfigCandidatesForGeneration(settings)
		if err != nil {
			return nil, err
		}
		if len(job.ModelCandidates) > 0 {
			job.ModelConfig = &job.ModelCandidates[0]
		} else {
			job.ModelConfig, err = a.modelConfigForGeneration(settings)
			if err != nil {
				return nil, err
			}
		}
	}
	return job, nil
}

func commerceGenerationReplaySafe(record GenerationRecord, job *generationJob) bool {
	if record.Status == GenerationStatusSucceeded && record.WorkID != nil {
		return true
	}
	if !record.ProviderRequestStarted {
		return true
	}
	if !record.ProviderIdempotencySupported || record.ChannelID == 0 || job == nil {
		return false
	}
	for _, candidate := range job.ModelCenterCandidates {
		if candidate.Channel.ID != record.ChannelID {
			continue
		}
		candidate.Model.CapabilityTags = append(candidate.Model.CapabilityTags, "idempotency_key")
		job.ModelCenterCandidates = []modelCenterCandidate{candidate}
		job.ModelCenterModel, job.ModelCenterChannel = &job.ModelCenterCandidates[0].Model, &job.ModelCenterCandidates[0].Channel
		return true
	}
	return false
}

func commerceContextFailure(err error) *ecommerce.ExecutionFailure {
	code := imageGenerationCancelledErrorCode
	if errors.Is(err, context.DeadlineExceeded) {
		code = imageGenerationTimeoutErrorCode
	}
	return &ecommerce.ExecutionFailure{Code: code, Message: err.Error()}
}

func fallbackPositive(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func commerceGenerationTraceID(key string, recordID uint) string {
	mac := hmac.New(sha256.New, []byte("image-agent-commerce-generation-trace-v1"))
	_, _ = fmt.Fprintf(mac, "%s:%d", strings.TrimSpace(key), recordID)
	return hex.EncodeToString(mac.Sum(nil))
}
