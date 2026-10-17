// benchmark_test.go — Performance benchmarks for all rate-limiting strategies.
//
// These benchmarks measure per-request overhead (ns/op) and memory allocations
// (B/op, allocs/op) for each strategy. They answer the question: "Is this
// middleware fast enough for the hot path?"
//
// Run with:  go test -bench=. -benchmem -count=1 -run='^$' ./internal/middleware/ratelimit/...
//
// Notes:
//   - Local benchmarks use a unique identifier per iteration to measure
//     first-request cost (bucket/window creation + allow). This is the
//     worst-case path; steady-state is cheaper.
//   - The Redis benchmark uses a single identifier, so it measures a mix
//     of allow + deny paths against a real Redis instance.
//   - The Redis benchmark skips gracefully if Redis is unreachable.
package ratelimit

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	redis "github.com/sahithakellacodes/distributed-rate-limiter/internal/redis"
)

// newBenchRedisClient mirrors newTestRedisClient but accepts *testing.B.
// Go's testing package doesn't share a common interface for Helper()/Skipf()
// across T and B, so this small duplication is unavoidable.
func newBenchRedisClient(b *testing.B) *redis.Client {
	b.Helper()

	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}

	client := redis.NewClient(addr)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if err := client.Ping(ctx); err != nil {
		b.Skipf("redis not available at %s: %v", addr, err)
	}

	return client
}

// BenchmarkLocalTokenBucket measures per-request cost of the local token bucket.
// Uses a unique identifier per iteration, so this measures bucket creation +
// first-request (the most expensive path).
func BenchmarkLocalTokenBucket(b *testing.B) {
	strategy := NewLocalTokenBucketStrategy()

	config := RateLimitConfig{
		MaxRequestsPerWindow: 100,
		WindowSize:           time.Hour,
	}

	ctx := context.Background()

	b.ReportAllocs() // include allocation stats in output
	b.ResetTimer()   // exclude setup from timing

	for i := 0; i < b.N; i++ {
		strategy.Check(ctx, fmt.Sprintf("bench-tb-%d", i), config)
	}
}

// BenchmarkLocalSlidingWindow measures per-request cost of the local sliding
// window. Same unique-identifier pattern as the token bucket benchmark.
func BenchmarkLocalSlidingWindow(b *testing.B) {
	strategy := NewLocalSlidingWindowStrategy()

	config := RateLimitConfig{
		MaxRequestsPerWindow: 100,
		WindowSize:           time.Hour,
	}

	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		strategy.Check(ctx, fmt.Sprintf("bench-sw-%d", i), config)
	}
}

// BenchmarkRedisTokenBucket measures per-request cost against a real Redis
// instance. Uses a single identifier — after capacity runs out, the benchmark
// measures the denial path (still a full Redis round-trip via EVALSHA).
func BenchmarkRedisTokenBucket(b *testing.B) {
	client := newBenchRedisClient(b)

	strategy := NewRedisTokenBucketStrategy(client)

	config := RateLimitConfig{
		MaxRequestsPerWindow: 100,
		WindowSize:           time.Hour,
	}

	identifier := fmt.Sprintf("bench-redis-%d", time.Now().UnixNano())
	redisKey := "ratelimit:tb:{" + identifier + "}"
	defer client.Del(context.Background(), redisKey) // clean up the Redis key

	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		strategy.Check(ctx, identifier, config)
	}
}
