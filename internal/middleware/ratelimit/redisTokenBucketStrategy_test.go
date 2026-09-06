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

func TestRedisTokenBucketStrategy_Concurrent(t *testing.T) {
	client := newTestRedisClient(t)

	strategy := NewRedisTokenBucketStrategy(client)

	const limit = 100
	const N = 500

	config := RateLimitConfig{
		MaxRequestsPerWindow: limit,
		WindowSize:           time.Hour,
	}

	// Unique identifier so this test starts from a fresh bucket regardless of
	// leftover state from other runs.
	identifier := fmt.Sprintf("concurrent-%d", time.Now().UnixNano())

	// Per-goroutine result slots indexed by goroutine number. Each goroutine
	// writes only to its own index, so there's no shared mutation and no lock
	// is needed. Reads happen after wg.Wait() establishes a happens-before edge.
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

			<-start // block until all goroutines are ready

			result, err := strategy.Check(context.Background(), identifier, config)
			results[i] = outcome{result: result, err: err}
		}(i)
	}

	// Release all N goroutines simultaneously.
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
}
