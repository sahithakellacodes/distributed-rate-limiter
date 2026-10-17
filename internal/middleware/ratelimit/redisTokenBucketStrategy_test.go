// redisTokenBucketStrategy_test.go — Integration tests for the Redis-backed token bucket.
//
// These tests require a live Redis instance. Every test calls newTestRedisClient(t),
// which pings Redis and calls t.Skip if it's unreachable — so CI and local dev
// degrade gracefully rather than failing.
//
// What this file proves:
//   - The Lua script atomically consumes tokens and enforces the limit.
//   - Token refill works via Redis' own TIME command (no client clock dependency).
//   - The global rate limit holds when multiple independent strategy instances
//     (simulating multiple gateway pods) share the same Redis key.
//   - Concurrent access from 500 goroutines is atomically correct.
package ratelimit

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	redis "github.com/sahithakellacodes/distributed-rate-limiter/internal/redis"
)

// newTestRedisClient creates a Redis client for testing. It reads REDIS_ADDR
// from the environment (defaults to localhost:6379), pings with a 500ms timeout,
// and calls t.Skipf if Redis is unreachable. This lets Redis-dependent tests
// skip gracefully in environments without Redis.
func newTestRedisClient(t *testing.T) *redis.Client {
	t.Helper()

	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}

	client := redis.NewClient(addr)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if err := client.Ping(ctx); err != nil {
		t.Skipf("redis not available at %s: %v", addr, err)
	}

	return client
}

// TestRedisTokenBucketStrategy_AllowThenDeny verifies that a fresh Redis bucket
// with capacity N allows exactly N requests, then denies the N+1th with a
// positive RetryAfterSeconds. Uses a large window to prevent refill during the test.
func TestRedisTokenBucketStrategy_AllowThenDeny(t *testing.T) {
	client := newTestRedisClient(t)

	strategy := NewRedisTokenBucketStrategy(client)

	const limit = 5

	config := RateLimitConfig{
		MaxRequestsPerWindow: limit,
		WindowSize:           time.Hour, // large window so no refill during test
	}

	// Unique identifier per test run to avoid key collisions.
	identifier := fmt.Sprintf("allowdeny-%d", time.Now().UnixNano())
	redisKey := "ratelimit:tb:{" + identifier + "}"
	defer client.Del(context.Background(), redisKey) // clean up after test

	// First `limit` requests must all be allowed.
	for i := 1; i <= limit; i++ {
		result, err := strategy.Check(context.Background(), identifier, config)
		if err != nil {
			t.Fatalf("request %d: unexpected error: %v", i, err)
		}
		if !result.Allowed {
			t.Fatalf("request %d: expected allowed, got denied (remaining=%d)", i, result.Remaining)
		}
		t.Logf("request %d: allowed, remaining=%d", i, result.Remaining)
	}

	// The very next request must be denied.
	result, err := strategy.Check(context.Background(), identifier, config)
	if err != nil {
		t.Fatalf("request %d: unexpected error: %v", limit+1, err)
	}
	if result.Allowed {
		t.Fatalf("request %d: expected denied, but was allowed (remaining=%d)", limit+1, result.Remaining)
	}
	if result.RetryAfterSeconds <= 0 {
		t.Errorf("request %d: denied but RetryAfterSeconds=%d, expected > 0", limit+1, result.RetryAfterSeconds)
	}
	t.Logf("request %d: denied as expected, retryAfter=%ds", limit+1, result.RetryAfterSeconds)
}

// TestRedisTokenBucketStrategy_Concurrent fires 500 goroutines at a single
// Redis bucket with capacity 100. The Lua script is atomic (EVAL is single-
// threaded in Redis), so exactly 100 must be allowed and 400 denied — no
// over-grants, no errors.
func TestRedisTokenBucketStrategy_Concurrent(t *testing.T) {
	client := newTestRedisClient(t)

	strategy := NewRedisTokenBucketStrategy(client)

	const limit = 100
	const N = 500

	config := RateLimitConfig{
		MaxRequestsPerWindow: limit,
		WindowSize:           time.Hour,
	}

	identifier := fmt.Sprintf("concurrent-%d", time.Now().UnixNano())
	redisKey := "ratelimit:tb:{" + identifier + "}"
	defer client.Del(context.Background(), redisKey)

	// Per-slot results indexed by goroutine number. Each goroutine writes only
	// to its own index, so there's no shared mutation and no lock is needed.
	type outcome struct {
		result RateLimitResult
		err    error
	}
	results := make([]outcome, N)

	// Barrier: all goroutines block on <-start, close(start) releases them.
	start := make(chan struct{})
	var wg sync.WaitGroup

	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			result, err := strategy.Check(context.Background(), identifier, config)
			results[i] = outcome{result: result, err: err}
		}(i)
	}

	close(start)
	wg.Wait()

	var allowedCount, deniedCount, failedCount int
	for _, o := range results {
		switch {
		case o.err != nil:
			failedCount++
		case o.result.Allowed:
			allowedCount++
		default:
			deniedCount++
		}
	}

	t.Logf("Proving: concurrent Redis safety — allowed=%d denied=%d failed=%d (limit=%d, N=%d)", allowedCount, deniedCount, failedCount, limit, N)

	if failedCount > 0 {
		t.Fatalf("Check returned errors for %d of %d requests", failedCount, N)
	}

	if allowedCount != limit {
		t.Errorf("allowed %d requests, expected exactly %d", allowedCount, limit)
	}

	if deniedCount != N-limit {
		t.Errorf("denied %d requests, expected exactly %d", deniedCount, N-limit)
	}
}

// TestRedisTokenBucketStrategy_DistributedCorrectness is the key test for this
// project: it creates 3 independent strategy instances (simulating 3 gateway
// pods) sharing the same Redis backend, then fires 500 concurrent requests
// round-robined across them. The global limit (100) must hold — proving that
// the Lua script provides atomic distributed rate limiting.
func TestRedisTokenBucketStrategy_DistributedCorrectness(t *testing.T) {
	client := newTestRedisClient(t)

	// Three independent strategy instances — same Redis, same key.
	s1 := NewRedisTokenBucketStrategy(client)
	s2 := NewRedisTokenBucketStrategy(client)
	s3 := NewRedisTokenBucketStrategy(client)

	strategies := []*RedisTokenBucketStrategy{s1, s2, s3}

	const limit = 100
	const N = 500

	config := RateLimitConfig{
		MaxRequestsPerWindow: limit,
		WindowSize:           time.Hour,
	}

	identifier := fmt.Sprintf("distributed-%d", time.Now().UnixNano())
	redisKey := "ratelimit:tb:{" + identifier + "}"
	defer client.Del(context.Background(), redisKey)

	type outcome struct {
		result RateLimitResult
		err    error
	}
	results := make([]outcome, N)

	start := make(chan struct{})
	var wg sync.WaitGroup

	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			// Round-robin across the 3 strategy instances to simulate
			// requests arriving at different gateway pods.
			s := strategies[i%3]
			result, err := s.Check(context.Background(), identifier, config)
			results[i] = outcome{result: result, err: err}
		}(i)
	}

	close(start)
	wg.Wait()

	var allowedCount, deniedCount, failedCount int
	for _, o := range results {
		switch {
		case o.err != nil:
			failedCount++
		case o.result.Allowed:
			allowedCount++
		default:
			deniedCount++
		}
	}

	if failedCount > 0 {
		t.Fatalf("Check returned errors for %d of %d requests", failedCount, N)
	}
	if allowedCount != limit {
		t.Errorf("allowed %d requests, expected exactly %d", allowedCount, limit)
	}
	if deniedCount != N-limit {
		t.Errorf("denied %d requests, expected exactly %d", deniedCount, N-limit)
	}

	t.Logf("Proving: global rate limit holds across 3 independent strategy instances — allowed=%d denied=%d failed=%d (limit=%d, N=%d)", allowedCount, deniedCount, failedCount, limit, N)
}

// TestRedisTokenBucketStrategy_TokenRefill verifies that the Redis Lua script
// refills tokens based on Redis' own TIME command. This test must use a real
// time.Sleep because Redis controls the clock — there's no way to inject time.
//
// Setup: capacity=2, window=2s → refillRate = 1 tok/s.
// After exhausting 2 tokens and sleeping 1.2s, ~1.2 tokens should refill.
func TestRedisTokenBucketStrategy_TokenRefill(t *testing.T) {
	client := newTestRedisClient(t)

	strategy := NewRedisTokenBucketStrategy(client)

	config := RateLimitConfig{
		MaxRequestsPerWindow: 2,
		WindowSize:           2 * time.Second, // refillRate = 2/2s = 1 tok/s
	}

	identifier := fmt.Sprintf("refill-%d", time.Now().UnixNano())
	redisKey := "ratelimit:tb:{" + identifier + "}"
	defer client.Del(context.Background(), redisKey)

	// Exhaust both tokens.
	for i := 1; i <= 2; i++ {
		result, err := strategy.Check(context.Background(), identifier, config)
		if err != nil {
			t.Fatalf("request %d: unexpected error: %v", i, err)
		}
		if !result.Allowed {
			t.Fatalf("request %d: expected allowed, got denied", i)
		}
	}

	// Confirm exhausted.
	result, err := strategy.Check(context.Background(), identifier, config)
	if err != nil {
		t.Fatalf("overflow check: unexpected error: %v", err)
	}
	if result.Allowed {
		t.Fatal("expected denied after exhausting tokens")
	}

	// Sleep to let Redis-side refill happen. At 1 tok/s, 1.2s gives ~1.2
	// tokens — enough for exactly one more request.
	time.Sleep(1200 * time.Millisecond)

	result, err = strategy.Check(context.Background(), identifier, config)
	if err != nil {
		t.Fatalf("request after refill: unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Fatal("expected allowed after 1.2s refill, got denied")
	}

	t.Logf("Proving: Redis-side token refill — capacity=2, window=2s, after 1.2s sleep, request allowed=%v", result.Allowed)
}
