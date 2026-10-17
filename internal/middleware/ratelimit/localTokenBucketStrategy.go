package ratelimit

import (
	"context"
	"math"
	"sync"
	"time"
)

type TokenBucket struct {
	mu         sync.Mutex
	tokens     float64   // Current number of tokens in the bucket
	lastRefill time.Time // Last time the bucket was refilled
}

type LocalTokenBucketStrategy struct {
	buckets sync.Map         // map[string]*TokenBucket — per-client token buckets
	now     func() time.Time // Function to get the current time, can be overridden for testing
}

func NewLocalTokenBucketStrategy() *LocalTokenBucketStrategy {
	return &LocalTokenBucketStrategy{
		now: time.Now,
	}
}

// TODO: Local memory might need cleanup either using a goroutine or on recovery
func (s *LocalTokenBucketStrategy) Check(_ context.Context, identifier string, config RateLimitConfig) (RateLimitResult, error) {
	now := s.now()

	// Load or lazily create the per-client bucket. LoadOrStore is atomic, so
	// two goroutines hitting a new client ID simultaneously both get the same
	// *TokenBucket back — only one allocation survives.
	val, loaded := s.buckets.Load(identifier)
	if !loaded {
		newBucket := &TokenBucket{
			tokens:     float64(config.MaxRequestsPerWindow),
			lastRefill: now,
		}
		val, _ = s.buckets.LoadOrStore(identifier, newBucket)
	}
	bucket := val.(*TokenBucket)

	// Only requests for this client contend on this lock.
	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	refillRate := float64(config.MaxRequestsPerWindow) / config.WindowSize.Seconds()
	elapsed := now.Sub(bucket.lastRefill).Seconds()
	if elapsed > 0 {
		bucket.tokens = math.Min(
			bucket.tokens+elapsed*refillRate,
			float64(config.MaxRequestsPerWindow),
		)
	}
	bucket.lastRefill = now

	result := RateLimitResult{
		Remaining: int(math.Floor(bucket.tokens)),
	}
	if bucket.tokens >= 1-tokenEpsilon {
		bucket.tokens--
		bucket.tokens = math.Max(0, bucket.tokens) // Clamp after decrement
		result.Allowed = true
	} else {
		secondsUntilNextToken := (1 - bucket.tokens) / refillRate
		result.RetryAfterSeconds = int(math.Ceil(secondsUntilNextToken))
	}

	return result, nil
}
