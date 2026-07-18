package ecommerce

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"gorm.io/gorm"
)

type JobKind string

const CommerceJobKindGenerateItem JobKind = "generate_item"

type LeaseIdentity struct {
	JobID      uint
	LeaseOwner string
	LeaseToken string
}

type ExecutionResult struct {
	GenerationRecordID uint
	WorkID             uint
	ActualCredits      int
	MetadataJSON       string
}

type ExecutionFailure struct {
	Code, Message string
	Retryable     bool
	ResultUnknown bool
}

// CompiledGenerationItem is the immutable, JSON-serializable execution
// payload. Task 4 persists the canonical JSON produced by Task 2 in
// Item.OutputSpecJSON.
type CompiledGenerationItem struct {
	SKUID                                      uint
	Pipeline, RecipeKey                        string
	RecipeVersion                              int
	SlotKey, Prompt, NegativePrompt            string
	ToolMode, ReferenceIntent                  string
	BackgroundReferenceRole                    string
	AssetIDs                                   []uint
	AspectRatio, WorkCategory, PostProcessJSON string
	PricingVersion, PricingSnapshotID          string
	EstimatedCredits                           int
	Section                                    string
	Scope                                      string
	SKUCode, SpecificationPath                 string
	AssetSnapshotJSON, SKUSnapshotJSON         string
	InheritedSharedAssets                      bool
	CreativeSpecSnapshot                       CreativeSpecSnapshot
	ModelSnapshotJSON                          string
	LayoutDocumentJSON, LayoutDocumentSHA256   string
	ExecutionReferenceSnapshotJSON             string
}

// DecodeGenerationItemSnapshot accepts only canonical OutputSpecJSON and is
// called once by GenerateItemJobHandler before executor dispatch.
func DecodeGenerationItemSnapshot(raw string) (CompiledGenerationItem, error) {
	if raw == "" {
		return CompiledGenerationItem{}, fmt.Errorf("compiled generation item snapshot is empty")
	}

	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var compiled CompiledGenerationItem
	if err := decoder.Decode(&compiled); err != nil {
		return CompiledGenerationItem{}, fmt.Errorf("decode compiled generation item snapshot: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return CompiledGenerationItem{}, fmt.Errorf("decode compiled generation item snapshot: multiple JSON values")
		}
		return CompiledGenerationItem{}, fmt.Errorf("decode compiled generation item snapshot trailing data: %w", err)
	}

	canonical, err := EncodeJSON(compiled)
	if err != nil {
		return CompiledGenerationItem{}, fmt.Errorf("encode canonical compiled generation item snapshot: %w", err)
	}
	if raw != canonical {
		return CompiledGenerationItem{}, fmt.Errorf("compiled generation item snapshot is not canonical JSON")
	}
	return compiled, nil
}

type ReserveCreditsRequest struct {
	UserID, ProjectID   uint
	BatchID             *uint
	ScopeType, ScopeKey string
	Amount              int
	IdempotencyKey      string
}

type CreditReservationSnapshot struct {
	ReservationID, UserID             uint
	BatchID                           *uint
	ScopeType, ScopeKey               string
	ReservedCredits, SettledCredits   int
	ReleasedCredits, AvailableCredits int
}

type SettleCreditsRequest struct {
	UserID, ProjectID, BatchID      uint
	ReservationID, GenerationItemID uint
	HeldCredits, ActualCredits      int
	IdempotencyKey                  string
}

type ReleaseCreditsRequest struct {
	UserID, ProjectID, BatchID      uint
	ReservationID, GenerationItemID uint
	HeldCredits                     int
	Reason, IdempotencyKey          string
}

type JobSnapshot struct {
	Job  CommerceJob
	Item *CommerceGenerationItem
}

func (s JobSnapshot) Lease() LeaseIdentity {
	return LeaseIdentity{JobID: s.Job.ID, LeaseOwner: s.Job.LeaseOwner, LeaseToken: s.Job.LeaseToken}
}

type JobResult struct {
	Execution    *ExecutionResult
	MetadataJSON string
}

type ItemExecutionRequest struct {
	Lease          LeaseIdentity
	Job            CommerceJob
	Item           CommerceGenerationItem
	Compiled       CompiledGenerationItem
	IdempotencyKey string
}

type ExecutorKey struct {
	Pipeline, RecipeKey string
}

type CommerceItemExecutor interface {
	Key() ExecutorKey
	Execute(context.Context, ItemExecutionRequest) (ExecutionResult, *ExecutionFailure)
}

type CreditLedger interface {
	ReserveTx(context.Context, *gorm.DB, ReserveCreditsRequest) (CreditReservationSnapshot, error)
	SettleItemTx(context.Context, *gorm.DB, SettleCreditsRequest) error
	ReleaseItemTx(context.Context, *gorm.DB, ReleaseCreditsRequest) error
}

type PricingSnapshot struct {
	ID, Version, RequestDigest, SnapshotHash, Status string
	UserID, ProjectID                                uint
	Entries                                          []PricingSnapshotEntry
	CreatedAt, ExpiresAt                             time.Time
}

type PricingSnapshotEntry struct {
	Pipeline, RecipeKey, QualityTier string
	ModelID                          uint
	ModelName                        string
	ChannelID, ProviderID            uint
	RuntimeModel, Endpoint           string
	RouteOrder                       int
	RequiredCapabilities             []string
	Credits                          int
}

type PricingSnapshotStore interface {
	IssueTx(context.Context, *gorm.DB, PricingSnapshot) (PricingSnapshot, error)
	ResolveForSubmitTx(
		ctx context.Context,
		db *gorm.DB,
		userID, projectID uint,
		snapshotID, requestDigest string,
		now time.Time,
	) (PricingSnapshot, error)
}

type CreativeSpecSnapshot struct {
	ID, Version           uint
	Status, ContentSHA256 string
	ProductFactsJSON      string
	CommonFactsJSON       string
	SKUOverridesJSON      string
	SKUContextSHA256      string
	SellingPointsJSON     string
	ForbiddenChangesJSON  string
	BrandToneJSON         string
	ShotPlanJSON          string
	CopyBlocksJSON        string
	RiskNoticesJSON       string
	SourceAssetIDsJSON    string
}

type ExecutionReferenceSnapshot struct {
	CommerceAssetID  uint   `json:"commerce_asset_id"`
	ReferenceAssetID uint   `json:"reference_asset_id"`
	Role             string `json:"role"`
	Order            int    `json:"order"`
}

type ExecutionReferenceSetSnapshot struct {
	References []ExecutionReferenceSnapshot `json:"references"`
}
