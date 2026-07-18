package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"gorm.io/gorm"
	"image-agent/internal/app/ecommerce"
)

func (a *App) startCommerceWorker() error {
	if err := a.verifyCommerceWorkerSchema(); err != nil {
		return err
	}
	if a == nil || !a.cfg.AICommerceEnabled || !a.cfg.AICommerceWorkerEnabled {
		return nil
	}
	executors := ecommerce.NewExecutorRegistry()
	if _, available := a.commerceRecipes.Get("general", ecommerce.ProductDetailSetRecipeKey, ecommerce.ProductDetailSetVersion); available {
		if err := executors.Register(ecommerce.NewKeyBoundExecutor(
			ecommerce.ExecutorKey{Pipeline: "general", RecipeKey: ecommerce.ProductDetailSetRecipeKey},
			[]int{ecommerce.ProductDetailSetVersion}, &commerceGenerationBackend{app: a},
		)); err != nil {
			return fmt.Errorf("register product detail executor: %w", err)
		}
	}
	handler := &ecommerce.GenerateItemJobHandler{Executors: executors}
	workerID, err := os.Hostname()
	if err != nil || workerID == "" {
		workerID = "image-agent"
	}
	workerID = fmt.Sprintf("%s-%d", workerID, os.Getpid())
	concurrency := 4
	if a.db.Dialector.Name() == "sqlite" {
		concurrency = 1
	}
	worker := &ecommerce.Worker{
		Queue: ecommerce.NewQueue(a.db, a.commerceService, workerID),
		Handlers: map[ecommerce.JobKind]ecommerce.JobHandler{
			ecommerce.CommerceJobKindGenerateItem: handler,
		},
		Concurrency: concurrency,
		Lease:       30 * time.Second,
		Poll:        time.Second,
	}
	worker.Queue.LateResultDiscarder = a.discardLateCommerceResultTx
	worker.Queue.TerminalHooks[ecommerce.CommerceJobKindProductAnalysis] = ecommerce.NewProductAnalysisTerminalHook()
	a.commerceVisionMu.Lock()
	defer a.commerceVisionMu.Unlock()
	visionAnalyzer := a.commerceVisionAnalyzer
	if visionAnalyzer != nil {
		worker.Handlers[ecommerce.CommerceJobKindProductAnalysis] = ecommerce.NewProductAnalysisJobHandler(a.commerceService, visionAnalyzer)
	}
	if err := worker.Start(context.Background()); err != nil {
		return fmt.Errorf("start commerce worker: %w", err)
	}
	a.commerceExecutors = executors
	a.commerceWorker = worker
	a.commerceWorkerDone = make(chan struct{})
	go func() {
		<-a.cleanupStop
		worker.Stop()
		close(a.commerceWorkerDone)
	}()
	return nil
}

func (a *App) verifyCommerceWorkerSchema() error {
	if a == nil || !a.cfg.AICommerceEnabled || !a.cfg.AICommerceWorkerEnabled {
		return nil
	}
	if err := ecommerce.VerifyFoundationSchema(context.Background(), a.db); err != nil {
		return fmt.Errorf("commerce worker schema readiness: %w", err)
	}
	return nil
}

func (a *App) discardLateCommerceResultTx(ctx context.Context, tx *gorm.DB, job ecommerce.CommerceJob, result ecommerce.ExecutionResult) error {
	if result.WorkID == 0 {
		return nil
	}
	var work Work
	if err := tx.WithContext(ctx).Where("id = ?", result.WorkID).First(&work).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	if work.UserID != job.UserID || result.GenerationRecordID == 0 || work.GenerationRecordID != result.GenerationRecordID {
		return ecommerce.ErrOwnershipMismatch
	}
	var record GenerationRecord
	if err := tx.WithContext(ctx).Where("id = ? AND user_id = ? AND work_id = ?", result.GenerationRecordID, job.UserID, result.WorkID).First(&record).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return ecommerce.ErrOwnershipMismatch
	} else if err != nil {
		return err
	}
	if result.GenerationRecordID != 0 {
		if err := tx.WithContext(ctx).Model(&GenerationRecord{}).Where("id = ? AND user_id = ? AND work_id = ?", result.GenerationRecordID, job.UserID, result.WorkID).Updates(map[string]any{
			"status": GenerationStatusFailed, "work_id": nil, "asset_key": "", "preview_url": "", "download_url": "", "storage_scope": StorageScopeDefault, "error_code": imageGenerationCancelledErrorCode, "error_message": imageGenerationCancelledMessage,
		}).Error; err != nil {
			return err
		}
	}
	if err := tx.WithContext(ctx).Delete(&work).Error; err != nil {
		return err
	}
	now := time.Now().UTC()
	generationID, workID := result.GenerationRecordID, result.WorkID
	cleanup := ecommerce.CommerceObjectCleanup{UserID: work.UserID, ProjectID: job.ProjectID, GenerationRecordID: &generationID, WorkID: &workID, StorageScope: StorageScopeCommercePrivate, ObjectKey: work.AssetKey, Reason: "late_result_discarded", Status: ecommerce.CleanupStatusQueued, MaxAttempts: 8, NextAttemptAt: &now, DeleteAfter: now}
	return tx.WithContext(ctx).Create(&cleanup).Error
}

func (a *App) configureCommerceProductDetailRecipe() error {
	if a == nil || !a.cfg.AICommerceEnabled || a.db == nil || a.commerceRecipes == nil {
		return nil
	}
	if _, exists := a.commerceRecipes.Get("general", ecommerce.ProductDetailSetRecipeKey, ecommerce.ProductDetailSetVersion); exists {
		return nil
	}
	settings, err := a.loadSettings()
	if err != nil {
		return fmt.Errorf("load product detail model settings: %w", err)
	}
	candidates, err := a.modelCenterCandidatesForGeneration(settings, ModelConfigTypeImage, 0)
	if err != nil {
		return fmt.Errorf("check product detail model capability: %w", err)
	}
	available := false
	for _, candidate := range candidates {
		if candidate.Model.Status == ModelCenterStatusOnline && candidate.Model.Visibility == ModelCenterVisibilityPublic && candidate.Model.DefaultCreditsCost > 0 {
			available = true
			break
		}
	}
	if !available {
		return nil
	}
	return a.commerceRecipes.Register(ecommerce.NewProductDetailSetCompiler(ecommerce.NewSnapshotCostResolver()))
}

// ConfigureCommerceVisionAnalyzer injects the audited product-analysis boundary.
// Tests and isolated harnesses use this without starting a second worker.
func (a *App) ConfigureCommerceVisionAnalyzer(analyzer ecommerce.CommerceVisionAnalyzer) error {
	if a == nil {
		return fmt.Errorf("app is unavailable")
	}
	a.commerceVisionMu.Lock()
	defer a.commerceVisionMu.Unlock()
	if a.commerceWorker != nil {
		return fmt.Errorf("commerce vision analyzer cannot change after worker assembly")
	}
	a.commerceVisionAnalyzer = analyzer
	if a.commerceService != nil {
		a.commerceService.ConfigureVisionAnalyzer(analyzer)
	}
	return nil
}

// RegisterCommerceRecipe wires a compiler into the shared commerce foundation.
// It is primarily used by concrete pipelines and the isolated workspace E2E harness.
func (a *App) RegisterCommerceRecipe(compiler ecommerce.Compiler) error {
	if a == nil || a.commerceRecipes == nil {
		return fmt.Errorf("commerce recipe registry is unavailable")
	}
	return a.commerceRecipes.Register(compiler)
}

// RegisterCommerceExecutor wires an executor without exposing provider internals.
func (a *App) RegisterCommerceExecutor(executor ecommerce.CommerceItemExecutor) error {
	if a == nil || a.commerceExecutors == nil {
		return fmt.Errorf("commerce executor registry is unavailable")
	}
	return a.commerceExecutors.Register(executor)
}
