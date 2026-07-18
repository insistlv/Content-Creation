package ecommerce

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

func EncodeJSON(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func DecodeJSON(raw string, target any) error {
	return json.Unmarshal([]byte(raw), target)
}

type CommerceBrand struct {
	ID, UserID           uint
	LogoReferenceAssetID *uint
	Name                 string `gorm:"size:160"`
	ColorPaletteJSON     string `gorm:"type:text"`
	FontsJSON            string `gorm:"type:text"`
	ForbiddenTermsJSON   string `gorm:"type:text"`
	VisualRulesJSON      string `gorm:"type:text"`
	CreatedAt, UpdatedAt time.Time
	DeletedAt            gorm.DeletedAt `gorm:"index"`
}

type CommerceProduct struct {
	ID, UserID                     uint
	BrandID                        *uint
	CategoryID                     *uint
	Name, Category, CategorySource string
	CategoryPath, SPUCode          string
	SellingPointsJSON              string `gorm:"type:text"`
	TargetChannelsJSON             string `gorm:"type:text"`
	Status                         string `gorm:"size:32;index"`
	SKUVersion                     int    `gorm:"not null;default:0"`
	CreatedAt, UpdatedAt           time.Time
	DeletedAt                      gorm.DeletedAt `gorm:"index"`
}

type CommerceSystemCategory struct {
	ID                               uint
	ParentID                         *uint
	Parent                           *CommerceSystemCategory `gorm:"foreignKey:ParentID"`
	Level                            int
	Name, SeedKey, SearchAliasesJSON string
	SortOrder                        int
	Status, CatalogVersion           string
	CreatedAt, UpdatedAt             time.Time
}

type CommerceUserCategory struct {
	ID, UserID, ParentID    uint
	Name, SearchAliasesJSON string
	Status                  string
	CreatedAt, UpdatedAt    time.Time
}

type CommerceSKU struct {
	ID, UserID, ProductID    uint
	Code, Color, Style, Size string
	AttributesJSON           string `gorm:"type:text"`
	Status                   string `gorm:"size:32;index"`
	CreatedAt, UpdatedAt     time.Time
	DeletedAt                gorm.DeletedAt     `gorm:"index"`
	Specification            []SKUSpecification `gorm:"-" json:"specification"`
	IsDefault                bool               `gorm:"-" json:"is_default"`
	AssetCount               int64              `gorm:"-" json:"asset_count"`
}

type CommerceSKUDimension struct {
	ID, UserID, ProductID uint
	Name                  string `gorm:"size:80;not null"`
	Version, SortOrder    int
	Status                string `gorm:"size:32;index"`
	CreatedAt, UpdatedAt  time.Time
	Product               CommerceProduct `gorm:"foreignKey:ProductID;constraint:OnDelete:RESTRICT" json:"-"`
}

type CommerceSKUValue struct {
	ID, UserID, ProductID, DimensionID uint
	Name                               string `gorm:"size:80;not null"`
	SortOrder                          int
	Status                             string `gorm:"size:32;index"`
	CreatedAt, UpdatedAt               time.Time
	Product                            CommerceProduct      `gorm:"foreignKey:ProductID;constraint:OnDelete:RESTRICT" json:"-"`
	Dimension                          CommerceSKUDimension `gorm:"foreignKey:DimensionID;constraint:OnDelete:RESTRICT" json:"-"`
}

type CommerceSKUValueLink struct {
	ID, UserID, ProductID uint
	SKUID                 uint `gorm:"column:sku_id"`
	ValueID               uint `gorm:"column:value_id"`
	CreatedAt             time.Time
	Product               CommerceProduct  `gorm:"foreignKey:ProductID;constraint:OnDelete:RESTRICT" json:"-"`
	SKU                   CommerceSKU      `gorm:"foreignKey:SKUID;constraint:OnDelete:RESTRICT" json:"-"`
	Value                 CommerceSKUValue `gorm:"foreignKey:ValueID;constraint:OnDelete:RESTRICT" json:"-"`
}

type CommerceSKUMatrixRequest struct {
	ID, UserID, ProductID uint
	IdempotencyKey        string `gorm:"size:160;not null"`
	RequestDigest         string `gorm:"size:64;not null"`
	ResponseJSON          string `gorm:"type:text;not null"`
	CreatedAt             time.Time
	Product               CommerceProduct `gorm:"foreignKey:ProductID;constraint:OnDelete:RESTRICT" json:"-"`
}

type SKUSpecification struct {
	DimensionID uint   `json:"dimension_id"`
	ValueID     uint   `json:"value_id"`
	Dimension   string `json:"dimension"`
	Value       string `json:"value"`
}

type CommerceProject struct {
	ID, UserID, ProductID   uint
	BrandID                 *uint
	DefaultSKUID            *uint `gorm:"column:default_sku_id"`
	ActiveCreativeSpecID    *uint
	Title, Pipeline, Status string
	DefaultChannelProfile   string
	DeletionRequestedAt     *time.Time
	CreatedAt, UpdatedAt    time.Time
	DeletedAt               gorm.DeletedAt `gorm:"index"`
}

type CommerceAsset struct {
	ID, UserID, ProjectID, ReferenceAssetID uint
	SKUID                                   *uint `gorm:"column:sku_id"`
	Role, Lifecycle                         string
	SortOrder                               int
	MetadataJSON                            string `gorm:"type:text"`
	RetainUntil, ObjectDeletedAt            *time.Time
	CreatedAt, UpdatedAt                    time.Time
	DeletedAt                               gorm.DeletedAt `gorm:"index"`
}

type CommerceCreativeSpec struct {
	ID, UserID, ProjectID uint
	Version               int
	Source, Status        string
	ProductFactsJSON      string `gorm:"type:text"`
	CommonFactsJSON       string `gorm:"type:text"`
	SKUOverridesJSON      string `gorm:"type:text"`
	SKUContextSHA256      string `gorm:"size:64;index"`
	SellingPointsJSON     string `gorm:"type:text"`
	ForbiddenChangesJSON  string `gorm:"type:text"`
	BrandToneJSON         string `gorm:"type:text"`
	ShotPlanJSON          string `gorm:"type:text"`
	CopyBlocksJSON        string `gorm:"type:text"`
	RiskNoticesJSON       string `gorm:"type:text"`
	SourceAssetIDsJSON    string `gorm:"type:text"`
	ObservedFactsJSON     string `gorm:"type:text"`
	UserOverridesJSON     string `gorm:"type:text"`
	MissingFieldsJSON     string `gorm:"type:text"`
	SuggestedSectionsJSON string `gorm:"type:text"`
	AnalysisError         string `gorm:"type:text"`
	AnalysisRequestHash   string `gorm:"size:64;index"`
	LockedAt              *time.Time
	CreatedAt, UpdatedAt  time.Time
}

type CommerceIdempotencyRecord struct {
	ID                    uint
	UserID                uint   `gorm:"uniqueIndex:ux_commerce_idempotency_scope_key,priority:1"`
	Scope                 string `gorm:"size:96;uniqueIndex:ux_commerce_idempotency_scope_key,priority:2"`
	IdempotencyKey        string `gorm:"size:160;uniqueIndex:ux_commerce_idempotency_scope_key,priority:3"`
	RequestDigest         string `gorm:"size:64"`
	ProductID, ProjectID  *uint
	SKUID                 *uint `gorm:"column:sku_id"`
	CreativeSpecID, JobID *uint
	CreatedAt, UpdatedAt  time.Time
}

type CommerceAIInvocation struct {
	ID                       uint
	JobID, UserID, ProjectID uint
	Purpose                  string `gorm:"size:96;index"`
	ModelID, ChannelID       uint
	Status                   string `gorm:"size:32;index"`
	LatencyMS                int64
	ProviderRequestID        string `gorm:"size:160"`
	RequestAssetIDsJSON      string `gorm:"type:text"`
	ResponseSchemaVersion    int
	ErrorCode                string `gorm:"size:128"`
	ErrorMessage             string `gorm:"type:text"`
	CreatedAt                time.Time
}

type CommerceGenerationBatch struct {
	ID, UserID, ProjectID uint
	CreativeSpecID        *uint
	ParentBatchID         *uint
	ReservationID         *uint
	PrimarySKUID          uint `gorm:"column:primary_sku_id"`
	Pipeline, RecipeKey   string
	RecipeVersion         int
	QualityTier           string
	Status                CommerceBatchStatus `gorm:"size:32;index"`
	IdempotencyKey        string              `gorm:"size:160"`
	RequestDigest         string              `gorm:"size:64"`
	RequestSnapshotJSON   string              `gorm:"type:text"`
	PricingVersion        string              `gorm:"size:64"`
	PricingSnapshotID     string              `gorm:"size:160"`
	PricingSnapshotJSON   string              `gorm:"type:text"`
	TotalItems            int
	QueuedItems           int
	RunningItems          int
	RetryingItems         int
	SucceededItems        int
	FailedItems           int
	CanceledItems         int
	EstimatedCredits      int
	ReservedCredits       int
	SettledCredits        int
	ReleasedCredits       int
	ETASeconds            int
	CancelRequestedAt     *time.Time
	StartedAt, FinishedAt *time.Time
	CreatedAt, UpdatedAt  time.Time
}

type CommerceJob struct {
	ID, UserID, ProjectID       uint
	BatchID, GenerationItemID   *uint
	GenerationItem              *CommerceGenerationItem `gorm:"foreignKey:GenerationItemID;references:ID;constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT"`
	SubjectID                   *uint
	SubjectType                 string `gorm:"size:64"`
	Kind                        JobKind
	Pipeline, RecipeKey         string
	Status                      CommerceJobStatus `gorm:"size:32;index"`
	IdempotencyKey              string            `gorm:"size:160"`
	Priority                    int
	AttemptCount, MaxAttempts   int
	NextAttemptAt               *time.Time
	LeaseOwner                  string `gorm:"size:160"`
	LeaseToken                  string `gorm:"size:160"`
	LeaseExpiresAt, HeartbeatAt *time.Time
	CancelRequestedAt           *time.Time
	PayloadJSON, ResultJSON     string `gorm:"type:text"`
	ErrorCode                   string `gorm:"size:128"`
	ErrorMessage                string `gorm:"type:text"`
	StartedAt, FinishedAt       *time.Time
	DeadLetteredAt              *time.Time
	CreatedAt, UpdatedAt        time.Time
}

type CommerceGenerationItem struct {
	ID, UserID, ProjectID, BatchID uint
	ParentItemID                   *uint
	ReservationID                  uint
	SKUID                          uint   `gorm:"column:sku_id"`
	Scope                          string `gorm:"size:16;not null;default:sku"`
	SlotKey                        string `gorm:"size:160"`
	CandidateIndex                 int
	Pipeline, RecipeKey            string
	RecipeVersion                  int
	QualityTier                    string
	PricingVersion                 string
	PricingSnapshotID              string
	IdempotencyKey                 string             `gorm:"size:160"`
	Status                         CommerceItemStatus `gorm:"size:32;index"`
	ProgressPercent                int                `gorm:"not null;default:0"`
	InputSnapshotJSON              string             `gorm:"type:text"`
	OutputSpecJSON                 string             `gorm:"type:text"`
	OutputSnapshotJSON             string             `gorm:"-"`
	EstimatedCredits               int
	ReservedCredits                int
	SettledCredits                 int
	ReleasedCredits                int
	GenerationRecordID             *uint
	WorkID                         *uint
	ErrorCode                      string `gorm:"size:128"`
	ErrorMessage                   string `gorm:"type:text"`
	CancelRequestedAt              *time.Time
	StartedAt, FinishedAt          *time.Time
	CreatedAt, UpdatedAt           time.Time
}

type CommerceCreditReservation struct {
	ID, UserID, ProjectID uint
	BatchID               *uint
	ScopeType, ScopeKey   string `gorm:"size:64"`
	IdempotencyKey        string `gorm:"size:160"`
	Status                string `gorm:"size:32;index"`
	TotalCredits          int
	ReservedCredits       int
	SettledCredits        int
	ReleasedCredits       int
	CreatedAt, UpdatedAt  time.Time
	CompletedAt           *time.Time
}

type CommercePricingSnapshot struct {
	ID                            string `gorm:"primaryKey;size:160"`
	UserID, ProjectID             uint
	RequestDigest                 string `gorm:"size:64"`
	SnapshotJSON                  string `gorm:"type:text"`
	SnapshotHash, Version, Status string `gorm:"size:160"`
	ExpiresAt                     time.Time
	ConsumedAt                    *time.Time
	CreatedAt                     time.Time
}

type CommerceCreditSettlement struct {
	ID, UserID, ProjectID, BatchID uint
	ReservationID                  uint
	GenerationItemID               uint
	IdempotencyKey                 string `gorm:"size:160"`
	HeldCredits                    int
	ActualCredits                  int
	SettledCredits                 int
	ReleasedCredits                int
	AnomalyCode                    string `gorm:"size:128"`
	CreatedAt                      time.Time
}

type CommerceEvent struct {
	ID, UserID, ProjectID uint
	BatchID, JobID        *uint
	EntityType            string `gorm:"size:64"`
	EntityID              uint
	Pipeline, RecipeKey   string
	EventType             string `gorm:"size:96;index"`
	MetadataJSON          string `gorm:"type:text"`
	CreatedAt             time.Time
}

type CommerceObjectCleanup struct {
	ID, UserID, ProjectID uint
	CommerceAssetID       *uint
	ReferenceAssetID      *uint
	GenerationRecordID    *uint
	WorkID                *uint
	StorageScope          string `gorm:"size:32"`
	ObjectKey             string `gorm:"size:512"`
	Reason, Status        string
	AttemptCount          int
	MaxAttempts           int
	NextAttemptAt         *time.Time
	DeleteAfter           time.Time
	LastError             string `gorm:"type:text"`
	ObjectDeletedAt       *time.Time
	CreatedAt, UpdatedAt  time.Time
}

type CommerceObjectGuard struct {
	ID           uint   `gorm:"primaryKey"`
	UserID       uint   `gorm:"uniqueIndex:ux_commerce_object_guards_identity,priority:1"`
	StorageScope string `gorm:"size:32;uniqueIndex:ux_commerce_object_guards_identity,priority:2"`
	ObjectKey    string `gorm:"size:512;uniqueIndex:ux_commerce_object_guards_identity,priority:3"`
	State        string `gorm:"size:32;index"`
	DeleteToken  string `gorm:"size:64"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
