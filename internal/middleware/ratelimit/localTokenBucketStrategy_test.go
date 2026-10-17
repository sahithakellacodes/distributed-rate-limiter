// localTokenBucketStrategy_test.go — Unit tests for the local (in-memory) token bucket algorithm.
//
// These tests prove that the token bucket math is correct without any external
// dependencies. Time is injected via the strategy's `now` field so tests run
// instantly (no time.Sleep) and are fully deterministic.
//
// What this file proves:
//   - Tokens are consumed one-per-request up to capacity, then denied.
//   - Tokens refill at the correct rate (capacity / windowSeconds).
//   - Each client identifier gets its own independent bucket.
//   - The bucket is safe under concurrent access (500 goroutines, -race clean).
package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestLocalTokenBucket_BasicAllowThenDeny verifies the fundamental contract:
// a fresh bucket with capacity N allows exactly N requests, then the N+1th
// request is denied with a positive RetryAfterSeconds.
func TestLocalTokenBucket_BasicAllowThenDeny(t *testing.T) {
	strategy := NewLocalTokenBucketStrategy()

	// Freeze time so no refill occurs between requests.
	fixedTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	strategy.now = func() time.Time { return fixedTime }

	config := RateLimitConfig{
		MaxRequestsPerWindow: 5,
		WindowSize:           time.Hour,
	}

	identifier := "client-basic"

	allowed := 0
	denied := 0

	// Consume all 5 tokens — each must be allowed.
	for i := 1; i <= 5; i++ {
		result, err := strategy.Check(context.Background(), identifier, config)
		if err != nil {
			t.Fatalf("request %d: unexpected error: %v", i, err)
		}
		if !result.Allowed {
			t.Fatalf("request %d: expected allowed, got denied", i)
		}
		allowed++
	}

	// The 6th request should be denied — bucket is empty and time hasn't moved.
	result, err := strategy.Check(context.Background(), identifier, config)
	if err != nil {
		t.Fatalf("request 6: unexpected error: %v", err)
	}
	if result.Allowed {
		t.Fatal("request 6: expected denied, got allowed")
	}
	// RetryAfterSeconds tells the caller how long to wait for a new token.
	if result.RetryAfterSeconds <= 0 {
		t.Errorf("request 6: RetryAfterSeconds=%d, expected > 0", result.RetryAfterSeconds)
	}
	denied++

	t.Logf("Proving: basic token bucket allows up to capacity then denies -> allowed=%d denied=%d", allowed, denied)
}

// TestLocalTokenBucket_TokenRefill verifies that tokens refill over time at
// the correct rate. With capacity=2 and window=1s the refill rate is 2 tok/s,
// so after 600ms roughly 1.2 tokens should be available — enough for one more
// request.
func TestLocalTokenBucket_TokenRefill(t *testing.T) {
	strategy := NewLocalTokenBucketStrategy()

	fixedTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	strategy.now = func() time.Time { return fixedTime }

	// capacity=2, WindowSize=1s => refillRate = 2 tok/s
	config := RateLimitConfig{
		MaxRequestsPerWindow: 2,
		WindowSize:           1 * time.Second,
	}

	identifier := "client-refill"

	// Exhaust both tokens at t=0.
	for i := 1; i <= 2; i++ {
		result, err := strategy.Check(context.Background(), identifier, config)
		if err != nil {
			t.Fatalf("request %d: unexpected error: %v", i, err)
		}
		if !result.Allowed {
			t.Fatalf("request %d: expected allowed, got denied", i)
		}
	}

	// Advance the clock by 600ms. The strategy recalculates:
	//   refilled = 2 tok/s * 0.6s = 1.2 tokens
	// Since 1.2 >= 1 (minus epsilon), the next request is allowed.
	refillTime := fixedTime.Add(600 * time.Millisecond)
	strategy.now = func() time.Time { return refillTime }

	result, err := strategy.Check(context.Background(), identifier, config)
	if err != nil {
		t.Fatalf("request after refill: unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Fatal("request after refill: expected allowed after 600ms refill, got denied")
	}

	t.Logf("Proving: token refill works — after 600ms at 2tok/s, ~1.2 tokens refilled, next request allowed=%v", result.Allowed)
}

// TestLocalTokenBucket_IndependentBuckets verifies per-client isolation:
// exhausting one client's bucket must not affect another client's capacity.
func TestLocalTokenBucket_IndependentBuckets(t *testing.T) {
	strategy := NewLocalTokenBucketStrategy()

	fixedTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	strategy.now = func() time.Time { return fixedTime }

	config := RateLimitConfig{
		MaxRequestsPerWindow: 5,
		WindowSize:           time.Hour,
	}

	// Exhaust client-A's entire bucket.
	for i := 1; i <= 5; i++ {
		result, err := strategy.Check(context.Background(), "client-A", config)
		if err != nil {
			t.Fatalf("client-A request %d: unexpected error: %v", i, err)
		}
		if !result.Allowed {
			t.Fatalf("client-A request %d: expected allowed", i)
		}
	}

	// Confirm client-A is exhausted.
	result, err := strategy.Check(context.Background(), "client-A", config)
	if err != nil {
		t.Fatalf("client-A overflow: unexpected error: %v", err)
	}
	if result.Allowed {
		t.Fatal("client-A: expected denied after exhaustion")
	}

	// client-B should still have its full capacity — completely independent.
	allowedB := 0
	for i := 1; i <= 5; i++ {
		result, err := strategy.Check(context.Background(), "client-B", config)
		if err != nil {
			t.Fatalf("client-B request %d: unexpected error: %v", i, err)
		}
		if !result.Allowed {
			t.Fatalf("client-B request %d: expected allowed, got denied", i)
		}
		allowedB++
	}

	t.Logf("Proving: per-client bucket isolation — client-A exhausted (denied), client-B independently allowed=%d requests", allowedB)
}

// TestLocalTokenBucket_ConcurrentAccess fires 500 goroutines at a bucket with
// capacity 100. With frozen time (no refill), exactly 100 must be allowed and
// 400 denied. The per-slot results array avoids shared mutation so the test is
// clean under -race.
func TestLocalTokenBucket_ConcurrentAccess(t *testing.T) {
	strategy := NewLocalTokenBucketStrategy()

	// Freeze time so no refill happens during the concurrent burst.
	fixedTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	strategy.now = func() time.Time { return fixedTime }

	const capacity = 100
	const N = 500

	config := RateLimitConfig{
		MaxRequestsPerWindow: capacity,
		WindowSize:           time.Hour,
	}

	identifier := fmt.Sprintf("concurrent-local-tb-%d", time.Now().UnixNano())

	// Each goroutine writes to its own slot — no shared counter, no lock needed.
	type outcome struct {
		result RateLimitResult
		err    error
	}
	results := make([]outcome, N)

	// Barrier pattern: all goroutines block on <-start, then close(start)
	// releases them simultaneously to maximize contention.
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

	close(start) // release all goroutines at once
	wg.Wait()    // wait for every goroutine to finish

	// Tally results after wg.Wait() — the happens-before edge guarantees
	// all writes to results[i] are visible.
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
	if allowedCount != capacity {
		t.Errorf("allowed %d requests, expected exactly %d", allowedCount, capacity)
	}
	if deniedCount != N-capacity {
		t.Errorf("denied %d requests, expected exactly %d", deniedCount, N-capacity)
	}

	t.Logf("Proving: concurrent safety — %d goroutines, allowed=%d denied=%d failed=%d (capacity=%d)", N, allowedCount, deniedCount, failedCount, capacity)
}
