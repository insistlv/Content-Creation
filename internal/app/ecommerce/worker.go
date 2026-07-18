package ecommerce

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type Worker struct {
	Queue       *Queue
	Handlers    map[JobKind]JobHandler
	Concurrency int
	Lease       time.Duration
	Poll        time.Duration
	Heartbeat   func(context.Context, LeaseIdentity, time.Duration) (bool, error)

	mu      sync.Mutex
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	running bool
	active  atomic.Int32
}

func (w *Worker) Start(parent context.Context) error {
	if w == nil || w.Queue == nil {
		return fmt.Errorf("commerce worker queue is required")
	}
	if w.Concurrency <= 0 {
		w.Concurrency = 1
	}
	if w.Lease <= 0 {
		w.Lease = 30 * time.Second
	}
	if w.Poll <= 0 {
		w.Poll = time.Second
	}
	for kind, handler := range w.Handlers {
		if handler == nil {
			return fmt.Errorf("commerce worker handler %s is nil", kind)
		}
		if handler.Kind() != kind {
			return fmt.Errorf("commerce worker handler key %s does not match kind %s", kind, handler.Kind())
		}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.running {
		return nil
	}
	ctx, cancel := context.WithCancel(parent)
	w.cancel = cancel
	w.running = true
	w.wg.Add(1)
	go w.poll(ctx)
	return nil
}

func (w *Worker) Stop() {
	if w == nil {
		return
	}
	w.mu.Lock()
	cancel := w.cancel
	w.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	w.wg.Wait()
	w.mu.Lock()
	w.running = false
	w.cancel = nil
	w.mu.Unlock()
}

func (w *Worker) Running() bool {
	if w == nil {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.running
}

func (w *Worker) poll(ctx context.Context) {
	defer w.wg.Done()
	ticker := time.NewTicker(w.Poll)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		available := w.Concurrency - int(w.active.Load())
		if available > 0 {
			jobs, err := w.Queue.Claim(ctx, available, w.Lease)
			if err == nil {
				for _, snapshot := range jobs {
					if ctx.Err() != nil {
						return
					}
					w.active.Add(1)
					w.wg.Add(1)
					go w.handle(ctx, snapshot)
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *Worker) handle(workerCtx context.Context, snapshot JobSnapshot) {
	defer w.wg.Done()
	defer w.active.Add(-1)
	handler, ok := w.Handlers[snapshot.Job.Kind]
	if !ok {
		_ = w.Queue.Fail(workerCtx, snapshot.Lease(), ExecutionFailure{Code: "handler_unavailable", Message: "job handler is unavailable", Retryable: true})
		return
	}
	jobCtx, cancel := context.WithCancel(workerCtx)
	defer cancel()
	heartbeatDone := make(chan struct{})
	var cancellationRequested atomic.Bool
	var heartbeatFailed atomic.Bool
	go func() {
		defer close(heartbeatDone)
		interval := w.Lease / 3
		if interval <= 0 {
			interval = time.Millisecond
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-jobCtx.Done():
				return
			case <-ticker.C:
				heartbeat := w.Heartbeat
				if heartbeat == nil {
					heartbeat = w.Queue.Heartbeat
				}
				requested, err := heartbeat(jobCtx, snapshot.Lease(), w.Lease)
				if err != nil {
					heartbeatFailed.Store(true)
					cancel()
					return
				}
				if requested {
					cancellationRequested.Store(true)
					cancel()
					return
				}
			}
		}
	}()

	result, handleErr := handler.Handle(jobCtx, snapshot)
	cancel()
	<-heartbeatDone
	if workerCtx.Err() != nil {
		return
	}
	if heartbeatFailed.Load() {
		return
	}
	transitionCtx, transitionCancel := context.WithTimeout(context.Background(), w.Lease)
	defer transitionCancel()
	if cancellationRequested.Load() {
		_ = w.Queue.Cancel(transitionCtx, snapshot.Lease(), "cancel_requested")
		return
	}
	if handleErr != nil {
		_ = w.Queue.Fail(transitionCtx, snapshot.Lease(), ExecutionFailure{
			Code: JobErrorCode(handleErr), Message: handleErr.Error(), Retryable: IsRetryableJobError(handleErr),
		})
		return
	}
	_ = w.Queue.Complete(transitionCtx, snapshot.Lease(), result)
}
