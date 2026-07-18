package app

import (
	"sync"
	"time"
)

const (
	// rateLimiterGCInterval 控制空闲 key 回收的最小触发间隔，避免每次请求都做全量扫描。
	rateLimiterGCInterval = 10 * time.Minute
	// rateLimiterIdleTTL 远大于任何业务限流窗口（秒级到分钟级），
	// 因此只回收确实不再活跃的 key，不会误删窗口内仍在计数的 key。
	rateLimiterIdleTTL = time.Hour
)

type RateLimiter struct {
	mu         sync.Mutex
	buckets    map[string][]time.Time
	lastGC     time.Time
	gcInterval time.Duration
	idleTTL    time.Duration
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		buckets:    make(map[string][]time.Time),
		gcInterval: rateLimiterGCInterval,
		idleTTL:    rateLimiterIdleTTL,
	}
}

func (l *RateLimiter) Allow(key string, now time.Time, window time.Duration, maxRequests int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := now.Add(-window)
	timestamps := l.buckets[key][:0]
	for _, ts := range l.buckets[key] {
		if ts.After(cutoff) {
			timestamps = append(timestamps, ts)
		}
	}

	allowed := len(timestamps) < maxRequests
	if allowed {
		timestamps = append(timestamps, now)
	}

	// 空 bucket 直接删除，避免被拒绝/过期后仍残留空 key 造成 map 只增不减。
	if len(timestamps) == 0 {
		delete(l.buckets, key)
	} else {
		l.buckets[key] = timestamps
	}

	l.collectIdleLocked(now)
	return allowed
}

// collectIdleLocked 在持有 l.mu 时按节流间隔回收长时间不活跃的 key。
// 判定依据是 bucket 中最后一次（即最近一次）时间戳，早于 now-idleTTL 即视为空闲。
func (l *RateLimiter) collectIdleLocked(now time.Time) {
	if l.gcInterval <= 0 {
		return
	}
	if !l.lastGC.IsZero() && now.Sub(l.lastGC) < l.gcInterval {
		return
	}
	l.lastGC = now

	idleCutoff := now.Add(-l.idleTTL)
	for key, timestamps := range l.buckets {
		if len(timestamps) == 0 || timestamps[len(timestamps)-1].Before(idleCutoff) {
			delete(l.buckets, key)
		}
	}
}

// bucketCount 返回当前活跃 key 数量，供测试与可观测使用。
func (l *RateLimiter) bucketCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}

type ConcurrencyLimiter struct {
	mu    sync.Mutex
	locks map[string]struct{}
}

func NewConcurrencyLimiter() *ConcurrencyLimiter {
	return &ConcurrencyLimiter{locks: make(map[string]struct{})}
}

func (l *ConcurrencyLimiter) TryAcquire(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, exists := l.locks[key]; exists {
		return false
	}
	l.locks[key] = struct{}{}
	return true
}

func (l *ConcurrencyLimiter) Release(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.locks, key)
}

// UserConcurrencyLimiter 是按 key 计数的并发信号量，允许每个 key 同时持有最多 limit 个槽位。
// 用于图片生成：既要支持同一用户的批量并行（batch_total 上限 16），又要防止单用户瞬间发起
// 上千个并行任务打爆 provider 配额与服务资源。
type UserConcurrencyLimiter struct {
	mu     sync.Mutex
	counts map[string]int
	limit  int
}

func NewUserConcurrencyLimiter(limit int) *UserConcurrencyLimiter {
	if limit < 1 {
		limit = 1
	}
	return &UserConcurrencyLimiter{counts: make(map[string]int), limit: limit}
}

func (l *UserConcurrencyLimiter) TryAcquire(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.counts[key] >= l.limit {
		return false
	}
	l.counts[key]++
	return true
}

func (l *UserConcurrencyLimiter) Release(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.counts[key] <= 1 {
		delete(l.counts, key)
		return
	}
	l.counts[key]--
}
