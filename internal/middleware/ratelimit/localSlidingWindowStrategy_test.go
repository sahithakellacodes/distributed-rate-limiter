// localSlidingWindowStrategy_test.go — Unit tests for the local (in-memory) sliding window algorithm.
//
// These tests prove that the sliding window counter math is correct without
// any external dependencies. Time is injected via the strategy's `now` field,
// so tests are instant and deterministic.
//
// What this file proves:
//   - Requests are counted within a window and denied once the limit is reached.
//   - After enough time elapses (2+ windows), the previous window count zeroes
//     out and capacity is fully restored.
//   - The window is safe under concurrent access (500 goroutines, -race clean).
package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestLocalSlidingWindow_BasicAllowThenDeny verifies the fundamental contract:
// a fresh window with capacity N allows exactly N requests in the same window,
// then the N+1th request is denied.
func TestLocalSlidingWindow_BasicAllowThenDeny(t *testing.T) {
	strategy := NewLocalSlidingWindowStrategy()

	// Freeze time so all requests land in the same window.
	fixedTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	strategy.now = func() time.Time { return fixedTime }

	config := RateLimitConfig{
		MaxRequestsPerWindow: 5,
		WindowSize:           10 * time.Second,
	}

	identifier := "client-sw-basic"

	allowed := 0
	denied := 0

	// Consume all 5 slots in the current window.
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

	// The 6th request should be denied — window is at capacity.
	result, err := strategy.Check(context.Background(), identifier, config)
	if err != nil {
		t.Fatalf("request 6: unexpected error: %v", err)
	}
	if result.Allowed {
		t.Fatal("request 6: expected denied, got allowed")
	}
	denied++

	t.Logf("Proving: sliding window allows up to capacity then denies -> allowed=%d denied=%d", allowed, denied)
}

// TestLocalSlidingWindow_WindowSlide verifies that capacity is restored after
// time advances past the window boundary. With a 1s window, advancing 2.1s
// means 2+ full windows have elapsed, which zeroes out previousWindowCount
// and resets currentWindowCount — full capacity is available again.
func TestLocalSlidingWindow_WindowSlide(t *testing.T) {
	strategy := NewLocalSlidingWindowStrategy()

	fixedTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	strategy.now = func() time.Time { return fixedTime }

	config := RateLimitConfig{
		MaxRequestsPerWindow: 5,
		WindowSize:           1 * time.Second,
	}

	identifier := "client-sw-slide"

	// Exhaust all 5 slots at t=0.
	for i := 1; i <= 5; i++ {
		result, err := strategy.Check(context.Background(), identifier, config)
		if err != nil {
			t.Fatalf("request %d: unexpected error: %v", i, err)
		}
		if !result.Allowed {
			t.Fatalf("request %d: expected allowed, got denied", i)
		}
	}

	// Confirm exhausted at t=0.
	result, err := strategy.Check(context.Background(), identifier, config)
	if err != nil {
		t.Fatalf("overflow check: unexpected error: %v", err)
	}
	if result.Allowed {
		t.Fatal("expected denied after exhaustion")
	}

	// Advance 2.1s. The sliding window algorithm checks elapsed windows:
	//   elapsedWindows = 2100ms / 1000ms = 2  (integer division)
	//   >= 2 windows elapsed → previousWindowCount = 0
	//   currentWindowCount resets to 0
	// So full capacity (5) is available again.
	slideTime := fixedTime.Add(2100 * time.Millisecond)
	strategy.now = func() time.Time { return slideTime }

	result, err = strategy.Check(context.Background(), identifier, config)
	if err != nil {
		t.Fatalf("request after slide: unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Fatal("request after slide: expected allowed after window expiry, got denied")
	}

	t.Logf("Proving: window expiry restores capacity — after 2.1s (2+ windows of 1s), request allowed=%v remaining=%d", result.Allowed, result.Remaining)
}

// TestLocalSlidingWindow_ConcurrentAccess fires 500 goroutines at a window
// with capacity 100. With frozen time, exactly 100 must be allowed and 400
// denied. Uses the same barrier + per-slot pattern as the token bucket
// concurrent test, and is clean under -race.
func TestLocalSlidingWindow_ConcurrentAccess(t *testing.T) {
	strategy := NewLocalSlidingWindowStrategy()

	// Freeze time so all requests land in the same window.
	fixedTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	strategy.now = func() time.Time { return fixedTime }

	const capacity = 100
	const N = 500

	config := RateLimitConfig{
		MaxRequestsPerWindow: capacity,
		WindowSize:           10 * time.Second,
	}

	identifier := fmt.Sprintf("concurrent-local-sw-%d", time.Now().UnixNano())

	// Per-slot results — each goroutine writes only to its own index.
	type outcome struct {
		result RateLimitResult
		err    error
	}
	results := make([]outcome, N)

	// Barrier: goroutines block on <-start, close(start) releases all at once.
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
