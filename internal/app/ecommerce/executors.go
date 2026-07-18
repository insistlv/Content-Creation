package ecommerce

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

var (
	ErrExecutorAlreadyRegistered = errors.New("commerce executor already registered")
	ErrGenerationItemRequired    = errors.New("generate_item job requires generation item")
)

type JobHandler interface {
	Kind() JobKind
	Handle(context.Context, JobSnapshot) (JobResult, error)
}

type JobError struct {
	Code      string
	Message   string
	Retryable bool
}

func (e *JobError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Message) == "" {
		return e.Code
	}
	return e.Message
}

func NewJobError(code, message string, retryable bool) error {
	return &JobError{Code: strings.TrimSpace(code), Message: strings.TrimSpace(message), Retryable: retryable}
}

func IsRetryableJobError(err error) bool {
	var jobErr *JobError
	return errors.As(err, &jobErr) && jobErr.Retryable
}

func JobErrorCode(err error) string {
	var jobErr *JobError
	if errors.As(err, &jobErr) {
		return jobErr.Code
	}
	return "job_handler_failed"
}

type ExecutorRegistry struct {
	mu        sync.RWMutex
	executors map[ExecutorKey]CommerceItemExecutor
}

func NewExecutorRegistry() *ExecutorRegistry {
	return &ExecutorRegistry{executors: make(map[ExecutorKey]CommerceItemExecutor)}
}

func (r *ExecutorRegistry) Register(executor CommerceItemExecutor) error {
	if executor == nil {
		return fmt.Errorf("register commerce executor: nil executor")
	}
	key := normalizedExecutorKey(executor.Key())
	if key.Pipeline == "" || key.RecipeKey == "" {
		return fmt.Errorf("register commerce executor: pipeline and recipe key are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.executors == nil {
		r.executors = make(map[ExecutorKey]CommerceItemExecutor)
	}
	if _, exists := r.executors[key]; exists {
		return fmt.Errorf("%w: %s/%s", ErrExecutorAlreadyRegistered, key.Pipeline, key.RecipeKey)
	}
	r.executors[key] = executor
	return nil
}

func (r *ExecutorRegistry) Get(key ExecutorKey) (CommerceItemExecutor, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	executor, ok := r.executors[normalizedExecutorKey(key)]
	return executor, ok
}

func normalizedExecutorKey(key ExecutorKey) ExecutorKey {
	return ExecutorKey{Pipeline: strings.TrimSpace(key.Pipeline), RecipeKey: strings.TrimSpace(key.RecipeKey)}
}

type keyBoundExecutor struct {
	key               ExecutorKey
	supportedVersions map[int]struct{}
	backend           CommerceExecutionBackend
}

func NewKeyBoundExecutor(key ExecutorKey, supportedVersions []int, backend CommerceExecutionBackend) CommerceItemExecutor {
	versions := make(map[int]struct{}, len(supportedVersions))
	for _, version := range supportedVersions {
		if version > 0 {
			versions[version] = struct{}{}
		}
	}
	return &keyBoundExecutor{key: normalizedExecutorKey(key), supportedVersions: versions, backend: backend}
}

func (e *keyBoundExecutor) Key() ExecutorKey { return e.key }

func (e *keyBoundExecutor) Execute(ctx context.Context, request ItemExecutionRequest) (ExecutionResult, *ExecutionFailure) {
	compiledKey := normalizedExecutorKey(ExecutorKey{Pipeline: request.Compiled.Pipeline, RecipeKey: request.Compiled.RecipeKey})
	if compiledKey != e.key {
		return ExecutionResult{}, &ExecutionFailure{Code: "executor_identity_mismatch", Message: "compiled item does not match executor key"}
	}
	if _, supported := e.supportedVersions[request.Compiled.RecipeVersion]; !supported {
		return ExecutionResult{}, &ExecutionFailure{Code: "executor_version_unsupported", Message: "compiled recipe version is not supported"}
	}
	if e.backend == nil {
		return ExecutionResult{}, &ExecutionFailure{Code: "executor_unavailable", Message: "commerce execution backend is unavailable", Retryable: true}
	}
	return e.backend.Execute(ctx, request)
}

type GenerateItemJobHandler struct {
	Executors *ExecutorRegistry
}

func (*GenerateItemJobHandler) Kind() JobKind { return CommerceJobKindGenerateItem }

func (h *GenerateItemJobHandler) Handle(ctx context.Context, snapshot JobSnapshot) (JobResult, error) {
	if snapshot.Item == nil {
		return JobResult{}, ErrGenerationItemRequired
	}
	item := *snapshot.Item
	compiled, err := DecodeGenerationItemSnapshot(item.OutputSpecJSON)
	if err != nil {
		return JobResult{}, NewJobError("invalid_item_snapshot", err.Error(), false)
	}
	if compiled.Pipeline != item.Pipeline || compiled.RecipeKey != item.RecipeKey || compiled.RecipeVersion != item.RecipeVersion {
		return JobResult{}, fmt.Errorf("%w: decoded=%s/%s@%d item=%s/%s@%d", ErrCompiledItemIdentityMismatch,
			compiled.Pipeline, compiled.RecipeKey, compiled.RecipeVersion, item.Pipeline, item.RecipeKey, item.RecipeVersion)
	}
	key := ExecutorKey{Pipeline: item.Pipeline, RecipeKey: item.RecipeKey}
	executor, ok := h.Executors.Get(key)
	if !ok {
		return JobResult{}, NewJobError("executor_unavailable", fmt.Sprintf("executor %s/%s is unavailable", key.Pipeline, key.RecipeKey), true)
	}
	result, failure := executor.Execute(ctx, ItemExecutionRequest{
		Lease: snapshot.Lease(), Job: snapshot.Job, Item: item, Compiled: compiled, IdempotencyKey: item.IdempotencyKey,
	})
	if failure != nil {
		return JobResult{}, NewJobError(failure.Code, failure.Message, failure.Retryable)
	}
	return JobResult{Execution: &result, MetadataJSON: result.MetadataJSON}, nil
}
