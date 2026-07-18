package ecommerce

type CommerceItemStatus string
type CommerceJobStatus string
type CommerceBatchStatus string

const (
	CommerceItemQueued    CommerceItemStatus = "queued"
	CommerceItemRunning   CommerceItemStatus = "running"
	CommerceItemRetrying  CommerceItemStatus = "retrying"
	CommerceItemSucceeded CommerceItemStatus = "succeeded"
	CommerceItemFailed    CommerceItemStatus = "failed"
	CommerceItemCanceled  CommerceItemStatus = "canceled"

	CommerceJobQueued    CommerceJobStatus = "queued"
	CommerceJobRunning   CommerceJobStatus = "running"
	CommerceJobRetrying  CommerceJobStatus = "retrying"
	CommerceJobSucceeded CommerceJobStatus = "succeeded"
	CommerceJobFailed    CommerceJobStatus = "failed"
	CommerceJobCanceled  CommerceJobStatus = "canceled"

	CommerceBatchQueued           CommerceBatchStatus = "queued"
	CommerceBatchRunning          CommerceBatchStatus = "running"
	CommerceBatchPartialSucceeded CommerceBatchStatus = "partial_succeeded"
	CommerceBatchSucceeded        CommerceBatchStatus = "succeeded"
	CommerceBatchFailed           CommerceBatchStatus = "failed"
	CommerceBatchCanceled         CommerceBatchStatus = "canceled"
)

func CanTransitionCommerceItem(from, to CommerceItemStatus) bool {
	switch from {
	case CommerceItemQueued:
		return to == CommerceItemRunning || to == CommerceItemCanceled
	case CommerceItemRunning:
		return to == CommerceItemRetrying || to == CommerceItemSucceeded || to == CommerceItemFailed || to == CommerceItemCanceled
	case CommerceItemRetrying:
		return to == CommerceItemRunning || to == CommerceItemCanceled
	default:
		return false
	}
}

func CanTransitionCommerceJob(from, to CommerceJobStatus) bool {
	switch from {
	case CommerceJobQueued:
		return to == CommerceJobRunning || to == CommerceJobCanceled
	case CommerceJobRunning:
		return to == CommerceJobRetrying || to == CommerceJobSucceeded || to == CommerceJobFailed || to == CommerceJobCanceled
	case CommerceJobRetrying:
		return to == CommerceJobRunning || to == CommerceJobCanceled
	default:
		return false
	}
}
