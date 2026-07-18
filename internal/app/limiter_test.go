package app

import (
	"fmt"
	"testing"
	"time"
)

func TestRateLimiterEnforcesWindow(t *testing.T) {
	limiter := NewRateLimiter()
	now := time.Now()
	window := time.Minute

	if !limiter.Allow("k", now, window, 2) {
		t.Fatal("expected 1st request within limit to be allowed")
	}
	if !limiter.Allow("k", now, window, 2) {
		t.Fatal("expected 2nd request within limit to be allowed")
	}
	if limiter.Allow("k", now, window, 2) {
		t.Fatal("expected 3rd request within window to be rejected")
	}
	if !limiter.Allow("k", now.Add(2*window), window, 2) {
		t.Fatal("expected request after window to be allowed again")
	}
}

func TestRateLimiterReclaimsIdleKeys(t *testing.T) {
	limiter := NewRateLimiter()
	base := time.Now()

	for i := 0; i < 5; i++ {
		limiter.Allow(fmt.Sprintf("idle-%d", i), base, time.Minute, 3)
	}
	if got := limiter.bucketCount(); got != 5 {
		t.Fatalf("expected 5 buckets after seeding, got %d", got)
	}

	// 超过 GC 间隔与空闲 TTL 后访问新 key，应触发对历史空闲 key 的回收。
	future := base.Add(2 * time.Hour)
	if !limiter.Allow("active", future, time.Minute, 3) {
		t.Fatal("expected new active key to be allowed")
	}
	if got := limiter.bucketCount(); got != 1 {
		t.Fatalf("expected idle keys reclaimed leaving only active key, got %d", got)
	}
}

func TestRateLimiterRejectedRequestDoesNotLeaveEmptyBucket(t *testing.T) {
	limiter := NewRateLimiter()
	now := time.Now()

	// maxRequests<=0 时请求被拒绝且不应残留空 bucket。
	if limiter.Allow("zero", now, time.Minute, 0) {
		t.Fatal("expected request to be rejected when maxRequests is 0")
	}
	if got := limiter.bucketCount(); got != 0 {
		t.Fatalf("expected no bucket retained for always-rejected key, got %d", got)
	}
}

func TestConcurrencyLimiterAllowsOneActiveRequestPerKey(t *testing.T) {
	limiter := NewConcurrencyLimiter()
	key := "invite-1"

	if !limiter.TryAcquire(key) {
		t.Fatal("expected first acquire to succeed")
	}
	if limiter.TryAcquire(key) {
		t.Fatal("expected second acquire to fail while lock is held")
	}

	limiter.Release(key)

	if !limiter.TryAcquire(key) {
		t.Fatal("expected acquire after release to succeed")
	}
}

func TestUserConcurrencyLimiterAllowsUpToLimitPerKey(t *testing.T) {
	limiter := NewUserConcurrencyLimiter(3)
	key := "user-1"

	for i := 0; i < 3; i++ {
		if !limiter.TryAcquire(key) {
			t.Fatalf("expected acquire %d within limit to succeed", i+1)
		}
	}
	if limiter.TryAcquire(key) {
		t.Fatal("expected acquire beyond limit to fail")
	}

	limiter.Release(key)
	if !limiter.TryAcquire(key) {
		t.Fatal("expected acquire after one release to succeed")
	}

	if limiter.TryAcquire("user-2") != true {
		t.Fatal("expected a different key to have its own independent slots")
	}
}

func TestUserConcurrencyLimiterReleaseClearsKey(t *testing.T) {
	limiter := NewUserConcurrencyLimiter(2)
	key := "user-clear"

	if !limiter.TryAcquire(key) || !limiter.TryAcquire(key) {
		t.Fatal("expected two acquires within limit")
	}
	limiter.Release(key)
	limiter.Release(key)
	limiter.Release(key) // 多余 Release 不应使计数变为负数

	for i := 0; i < 2; i++ {
		if !limiter.TryAcquire(key) {
			t.Fatalf("expected acquire %d after full release to succeed", i+1)
		}
	}
}

func TestNewUserConcurrencyLimiterClampsInvalidLimit(t *testing.T) {
	limiter := NewUserConcurrencyLimiter(0)
	if !limiter.TryAcquire("k") {
		t.Fatal("expected at least one slot even when limit <= 0")
	}
	if limiter.TryAcquire("k") {
		t.Fatal("expected limit to be clamped to 1")
	}
}
