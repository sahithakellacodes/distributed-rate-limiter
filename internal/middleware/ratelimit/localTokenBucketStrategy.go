package ratelimit

import (
	"math"
	"sync"
	"time"
)

type TokenBucket struct {
	tokens     float64   // Current number of tokens in the bucket
	lastRefill time.Time // Last time the bucket was refilled
}

type LocalTokenBucketStrategy struct {
	buckets map[string]*TokenBucket // Map to store token buckets for each identifier (e.g., client ID)
	mutex   sync.Mutex              // Mutex to ensure thread-safe access to the buckets map
	now     func() time.Time        // Function to get the current time, can be overridden for testing
}

func NewLocalTokenBucketStrategy() *LocalTokenBucketStrategy {
	return &LocalTokenBucketStrategy{
		buckets: make(map[string]*TokenBucket),
		now:     time.Now,
	}
}

func (s *LocalTokenBucketStrategy) Check(identifier string, config RateLimitConfig) RateLimitResult {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	now := s.now()
	bucket, exists := s.buckets[identifier]
	if !exists {
		bucket = &TokenBucket{
			tokens:     float64(config.MaxRequestsPerWindow),
			lastRefill: now,
		}
		s.buckets[identifier] = bucket
	}

	refillRate := float64(config.MaxRequestsPerWindow) / config.WindowSizeInSeconds.Seconds()
	elapsed := now.Sub(bucket.lastRefill).Seconds()
	if elapsed > 0 {
		bucket.tokens = math.Min(
			bucket.tokens+elapsed*refillRate,
			float64(config.MaxRequestsPerWindow),
		)
	}
	bucket.lastRefill = now

	result := RateLimitResult{
		ResetTimestamp: now.Add(config.WindowSizeInSeconds).Unix(),
	}
	if bucket.tokens >= 1 {
		bucket.tokens--
		result.Allowed = true
	} else {
		secondsUntilNextToken := (1 - bucket.tokens) / refillRate
		timeUntilNextToken := time.Duration(secondsUntilNextToken * float64(time.Second))

		result.ResetTimestamp = int64(math.Ceil(float64(now.Add(timeUntilNextToken).UnixNano()) / float64(time.Second))) // Epoch seconds, rounded up to avoid premature retries.
		result.RetryAfterSeconds = int(math.Ceil(secondsUntilNextToken))
	}
	result.Remaining = int(math.Floor(bucket.tokens))

	return result
}
