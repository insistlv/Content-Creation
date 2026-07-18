package ecommerce

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
)

type countingSettlementLedger struct{ settleCalls int }

func (*countingSettlementLedger) ReserveTx(context.Context, *gorm.DB, ReserveCreditsRequest) (CreditReservationSnapshot, error) {
	return CreditReservationSnapshot{}, nil
}

func (l *countingSettlementLedger) SettleItemTx(_ context.Context, tx *gorm.DB, req SettleCreditsRequest) error {
	l.settleCalls++
	return tx.Model(&CommerceGenerationItem{}).Where("id = ?", req.GenerationItemID).Updates(map[string]any{"settled_credits": req.ActualCredits, "released_credits": req.HeldCredits - req.ActualCredits}).Error
}

func (*countingSettlementLedger) ReleaseItemTx(context.Context, *gorm.DB, ReleaseCreditsRequest) error {
	return nil
}

func TestWorkerGenerationCompletionIsIdempotent(t *testing.T) {
	service, db, lease, itemID := newCancelRaceService(t, "worker-idempotent-complete")
	ledger := &countingSettlementLedger{}
	service.ConfigureBatchInfrastructure(NewRegistry(), ledger, NewGormPricingSnapshotStore(), nil)
	result := ExecutionResult{GenerationRecordID: 901, WorkID: 902, ActualCredits: 1}
	if err := service.CompleteGenerationItem(context.Background(), lease, itemID, result); err != nil {
		t.Fatalf("first CompleteGenerationItem: %v", err)
	}
	// Simulate a worker crash after the completion transaction committed but
	// before the caller observed it, then replay the same result.
	if err := service.CompleteGenerationItem(context.Background(), lease, itemID, result); err != nil {
		t.Fatalf("replayed CompleteGenerationItem: %v", err)
	}
	var item CommerceGenerationItem
	if err := db.First(&item, itemID).Error; err != nil {
		t.Fatal(err)
	}
	if ledger.settleCalls != 1 {
		t.Fatalf("settlement calls=%d want 1", ledger.settleCalls)
	}
	if item.Status != CommerceItemSucceeded || item.ProgressPercent != 100 || item.GenerationRecordID == nil || *item.GenerationRecordID != result.GenerationRecordID || item.WorkID == nil || *item.WorkID != result.WorkID || item.SettledCredits != result.ActualCredits {
		t.Fatalf("completed item=%#v", item)
	}
}

type blockingJobHandler struct {
	started chan struct{}
	done    chan struct{}
	once    sync.Once
}

type lateSuccessAfterCancelHandler struct {
	started chan struct{}
	done    chan struct{}
}

func (h *lateSuccessAfterCancelHandler) Kind() JobKind { return CommerceJobKindGenerateItem }

func (h *lateSuccessAfterCancelHandler) Handle(ctx context.Context, _ JobSnapshot) (JobResult, error) {
	close(h.started)
	<-ctx.Done()
	close(h.done)
	return JobResult{Execution: &ExecutionResult{WorkID: 99, ActualCredits: 1}}, nil
}

func (h *blockingJobHandler) Kind() JobKind { return CommerceJobKindGenerateItem }

func (h *blockingJobHandler) Handle(ctx context.Context, _ JobSnapshot) (JobResult, error) {
	h.once.Do(func() { close(h.started) })
	<-ctx.Done()
	close(h.done)
	return JobResult{}, ctx.Err()
}

func TestWorkerCloseStopsPollingAndInflightHandlers(t *testing.T) {
	db := newQueueTestDB(t)
	_, _ = seedQueueJob(t, db, "worker-close", 3)
	handler := &blockingJobHandler{started: make(chan struct{}), done: make(chan struct{})}
	queue := NewQueue(db, nil, "worker-close")
	worker := &Worker{
		Queue: queue, Handlers: map[JobKind]JobHandler{CommerceJobKindGenerateItem: handler},
		Concurrency: 1, Lease: 300 * time.Millisecond, Poll: 5 * time.Millisecond,
	}
	if err := worker.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-handler.started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start handler")
	}
	worker.Stop()
	select {
	case <-handler.done:
	case <-time.After(time.Second):
		t.Fatal("Stop did not wait for inflight handler")
	}
	if worker.Running() {
		t.Fatal("worker still reports running after Stop")
	}
}

func TestWorkerHeartbeatCancelsProviderContext(t *testing.T) {
	db := newQueueTestDB(t)
	job, _ := seedQueueJob(t, db, "worker-cancel", 3)
	handler := &blockingJobHandler{started: make(chan struct{}), done: make(chan struct{})}
	worker := &Worker{
		Queue:       NewQueue(db, nil, "worker-cancel"),
		Handlers:    map[JobKind]JobHandler{CommerceJobKindGenerateItem: handler},
		Concurrency: 1, Lease: 90 * time.Millisecond, Poll: 5 * time.Millisecond,
	}
	if err := worker.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer worker.Stop()
	select {
	case <-handler.started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start handler")
	}
	now := time.Now().UTC()
	if err := db.Model(&CommerceJob{}).Where("id = ?", job.ID).Update("cancel_requested_at", now).Error; err != nil {
		t.Fatalf("request cancel: %v", err)
	}
	select {
	case <-handler.done:
	case <-time.After(time.Second):
		t.Fatal("heartbeat did not cancel handler context")
	}
}

func TestWorkerHeartbeatFailureDiscardsHandlerResultWithoutTransition(t *testing.T) {
	db := newQueueTestDB(t)
	job, _ := seedQueueJob(t, db, "worker-heartbeat-failure", 3)
	handler := &lateSuccessAfterCancelHandler{started: make(chan struct{}), done: make(chan struct{})}
	heartbeatErr := errors.New("temporary heartbeat database error")
	worker := &Worker{
		Queue:    NewQueue(db, nil, "worker-heartbeat-failure"),
		Handlers: map[JobKind]JobHandler{CommerceJobKindGenerateItem: handler},
		Heartbeat: func(context.Context, LeaseIdentity, time.Duration) (bool, error) {
			return false, heartbeatErr
		},
		Concurrency: 1, Lease: 600 * time.Millisecond, Poll: 5 * time.Millisecond,
	}
	if err := worker.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-handler.started:
	case <-time.After(time.Second):
		worker.Stop()
		t.Fatal("worker did not start handler")
	}
	select {
	case <-handler.done:
	case <-time.After(time.Second):
		worker.Stop()
		t.Fatal("heartbeat failure did not stop handler")
	}
	worker.Stop()
	if err := db.First(&job, job.ID).Error; err != nil {
		t.Fatalf("reload job: %v", err)
	}
	if job.Status != CommerceJobRunning || job.ErrorCode != "" || job.FinishedAt != nil {
		t.Fatalf("heartbeat failure transitioned job: %#v", job)
	}
}
