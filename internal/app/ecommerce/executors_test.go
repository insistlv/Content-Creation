package ecommerce

import (
	"context"
	"errors"
	"testing"
)

type recordingExecutionBackend struct {
	requests []ItemExecutionRequest
	result   ExecutionResult
}

func (b *recordingExecutionBackend) Execute(_ context.Context, request ItemExecutionRequest) (ExecutionResult, *ExecutionFailure) {
	b.requests = append(b.requests, request)
	return b.result, nil
}

func TestExecutorRegistryRejectsDuplicateKey(t *testing.T) {
	registry := NewExecutorRegistry()
	backend := &recordingExecutionBackend{}
	first := NewKeyBoundExecutor(ExecutorKey{Pipeline: "general", RecipeKey: "poster"}, []int{1}, backend)
	if err := registry.Register(first); err != nil {
		t.Fatalf("Register first: %v", err)
	}
	if err := registry.Register(first); !errors.Is(err, ErrExecutorAlreadyRegistered) {
		t.Fatalf("duplicate error = %v", err)
	}
}

func TestGenerateItemJobHandlerDecodesAndDispatchesFrozenSnapshot(t *testing.T) {
	registry := NewExecutorRegistry()
	backend := &recordingExecutionBackend{result: ExecutionResult{WorkID: 77, ActualCredits: 2}}
	if err := registry.Register(NewKeyBoundExecutor(ExecutorKey{Pipeline: "general", RecipeKey: "poster"}, []int{1}, backend)); err != nil {
		t.Fatalf("Register: %v", err)
	}
	raw, err := EncodeJSON(CompiledGenerationItem{SKUID: 9, Pipeline: "general", RecipeKey: "poster", RecipeVersion: 1, SlotKey: "hero"})
	if err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}
	item := CommerceGenerationItem{ID: 4, SKUID: 9, Pipeline: "general", RecipeKey: "poster", RecipeVersion: 1, OutputSpecJSON: raw, IdempotencyKey: "item-key"}
	job := CommerceJob{ID: 3, GenerationItemID: &item.ID, Kind: CommerceJobKindGenerateItem, Pipeline: "general", RecipeKey: "poster", LeaseOwner: "worker", LeaseToken: "token"}
	handler := &GenerateItemJobHandler{Executors: registry}

	result, err := handler.Handle(context.Background(), JobSnapshot{Job: job, Item: &item})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.Execution == nil || result.Execution.WorkID != 77 || len(backend.requests) != 1 {
		t.Fatalf("result=%#v requests=%#v", result, backend.requests)
	}
	request := backend.requests[0]
	if request.Compiled.SlotKey != "hero" || request.IdempotencyKey != "item-key" || request.Lease.LeaseToken != "token" {
		t.Fatalf("request = %#v", request)
	}
}

func TestGenerateItemJobHandlerRejectsMismatchAndUnavailableExecutor(t *testing.T) {
	raw, _ := EncodeJSON(CompiledGenerationItem{Pipeline: "fashion", RecipeKey: "poster", RecipeVersion: 1})
	item := CommerceGenerationItem{ID: 1, Pipeline: "general", RecipeKey: "poster", RecipeVersion: 1, OutputSpecJSON: raw}
	handler := &GenerateItemJobHandler{Executors: NewExecutorRegistry()}
	if _, err := handler.Handle(context.Background(), JobSnapshot{Job: CommerceJob{Kind: CommerceJobKindGenerateItem}, Item: &item}); !errors.Is(err, ErrCompiledItemIdentityMismatch) {
		t.Fatalf("mismatch error = %v", err)
	}

	item.Pipeline = "fashion"
	if _, err := handler.Handle(context.Background(), JobSnapshot{Job: CommerceJob{Kind: CommerceJobKindGenerateItem}, Item: &item}); !IsRetryableJobError(err) || JobErrorCode(err) != "executor_unavailable" {
		t.Fatalf("unavailable error = %v", err)
	}
}

func TestGenerateItemJobHandlerRequiresItemOnlyForGenerateKind(t *testing.T) {
	handler := &GenerateItemJobHandler{Executors: NewExecutorRegistry()}
	if _, err := handler.Handle(context.Background(), JobSnapshot{Job: CommerceJob{Kind: CommerceJobKindGenerateItem}}); !errors.Is(err, ErrGenerationItemRequired) {
		t.Fatalf("missing item error = %v", err)
	}
}
