package ecommerce

import (
	"context"
	"reflect"
	"testing"

	"gorm.io/gorm"
)

type fakeCreditLedger struct{}

func (fakeCreditLedger) ReserveTx(context.Context, *gorm.DB, ReserveCreditsRequest) (CreditReservationSnapshot, error) {
	return CreditReservationSnapshot{}, nil
}

func (fakeCreditLedger) SettleItemTx(context.Context, *gorm.DB, SettleCreditsRequest) error {
	return nil
}

func (fakeCreditLedger) ReleaseItemTx(context.Context, *gorm.DB, ReleaseCreditsRequest) error {
	return nil
}

type fakeItemExecutor struct {
	key ExecutorKey
}

func (f fakeItemExecutor) Key() ExecutorKey {
	return f.key
}

func (fakeItemExecutor) Execute(context.Context, ItemExecutionRequest) (ExecutionResult, *ExecutionFailure) {
	return ExecutionResult{}, nil
}

var _ CreditLedger = fakeCreditLedger{}
var _ CommerceItemExecutor = fakeItemExecutor{}

func TestFoundationContracts(t *testing.T) {
	compiled := CompiledGenerationItem{
		SKUID:                   42,
		Pipeline:                "apparel",
		RecipeKey:               "listing-image",
		RecipeVersion:           3,
		SlotKey:                 "hero",
		Prompt:                  "studio product image",
		NegativePrompt:          "watermark",
		ToolMode:                "image",
		ReferenceIntent:         "product",
		BackgroundReferenceRole: "background",
		AssetIDs:                []uint{7, 9},
		AspectRatio:             "1:1",
		WorkCategory:            "product",
		PostProcessJSON:         `{"remove_background":false}`,
		PricingVersion:          "2026-07",
		PricingSnapshotID:       "snapshot-1",
		EstimatedCredits:        12,
	}
	raw, err := EncodeJSON(compiled)
	if err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}
	decoded, err := DecodeGenerationItemSnapshot(raw)
	if err != nil {
		t.Fatalf("DecodeGenerationItemSnapshot: %v", err)
	}
	if !reflect.DeepEqual(decoded, compiled) {
		t.Fatalf("decoded snapshot = %#v, want %#v", decoded, compiled)
	}
	if _, err := DecodeGenerationItemSnapshot(" " + raw); err == nil {
		t.Fatal("DecodeGenerationItemSnapshot accepted non-canonical JSON")
	}

	executors := map[ExecutorKey]CommerceItemExecutor{}
	generic := fakeItemExecutor{key: ExecutorKey{Pipeline: "generic", RecipeKey: string(CommerceJobKindGenerateItem)}}
	apparel := fakeItemExecutor{key: ExecutorKey{Pipeline: "apparel", RecipeKey: string(CommerceJobKindGenerateItem)}}
	executors[generic.Key()] = generic
	executors[apparel.Key()] = apparel
	if got := len(executors); got != 2 {
		t.Fatalf("executor registration count = %d, want 2 distinct pipeline/recipe keys", got)
	}
}
