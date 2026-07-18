package ecommerce

import "testing"

func TestCommerceItemAndJobStateTransitions(t *testing.T) {
	itemCases := []struct {
		from, to CommerceItemStatus
		allowed  bool
	}{
		{CommerceItemQueued, CommerceItemRunning, true},
		{CommerceItemQueued, CommerceItemCanceled, true},
		{CommerceItemRunning, CommerceItemRetrying, true},
		{CommerceItemRunning, CommerceItemSucceeded, true},
		{CommerceItemRunning, CommerceItemFailed, true},
		{CommerceItemRunning, CommerceItemCanceled, true},
		{CommerceItemRetrying, CommerceItemRunning, true},
		{CommerceItemRetrying, CommerceItemCanceled, true},
		{CommerceItemSucceeded, CommerceItemRunning, false},
		{CommerceItemFailed, CommerceItemRetrying, false},
		{CommerceItemCanceled, CommerceItemRunning, false},
	}
	for _, tc := range itemCases {
		if got := CanTransitionCommerceItem(tc.from, tc.to); got != tc.allowed {
			t.Fatalf("CanTransitionCommerceItem(%q,%q)=%v, want %v", tc.from, tc.to, got, tc.allowed)
		}
	}

	jobCases := []struct {
		from, to CommerceJobStatus
		allowed  bool
	}{
		{CommerceJobQueued, CommerceJobRunning, true},
		{CommerceJobQueued, CommerceJobCanceled, true},
		{CommerceJobRunning, CommerceJobRetrying, true},
		{CommerceJobRunning, CommerceJobSucceeded, true},
		{CommerceJobRunning, CommerceJobFailed, true},
		{CommerceJobRunning, CommerceJobCanceled, true},
		{CommerceJobRetrying, CommerceJobRunning, true},
		{CommerceJobRetrying, CommerceJobCanceled, true},
		{CommerceJobSucceeded, CommerceJobRunning, false},
		{CommerceJobFailed, CommerceJobRetrying, false},
		{CommerceJobCanceled, CommerceJobRunning, false},
	}
	for _, tc := range jobCases {
		if got := CanTransitionCommerceJob(tc.from, tc.to); got != tc.allowed {
			t.Fatalf("CanTransitionCommerceJob(%q,%q)=%v, want %v", tc.from, tc.to, got, tc.allowed)
		}
	}

	batchStatuses := []CommerceBatchStatus{
		CommerceBatchQueued, CommerceBatchRunning, CommerceBatchPartialSucceeded,
		CommerceBatchSucceeded, CommerceBatchFailed, CommerceBatchCanceled,
	}
	if got := len(batchStatuses); got != 6 {
		t.Fatalf("batch status count=%d, want 6", got)
	}
}
