// resilientWrapper_test.go — Resilience tests for the ResilientWrapper.
//
// The ResilientWrapper sits between the HTTP middleware and the rate-limiting
// strategies. It routes requests to the primary (Redis) strategy when healthy,
// falls back to the local strategy on error, and switches back when health
// recovers. These tests verify that routing logic using mock strategies and
// a mock health checker — no Redis needed.
//
// What this file proves:
//   - Healthy primary receives requests; fallback is never called.
//   - Primary error triggers fallback AND reports a health failure.
//   - Unhealthy state skips primary entirely (no wasted round-trip).
//   - Recovery (health restored) switches back to primary.
package ratelimit

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// --- mocks ---

// mockStrategy is a configurable RateLimitStrategy that tracks call counts
// atomically. Set failOn=true to simulate a strategy that returns errors
// (e.g., Redis connection failure).
type mockStrategy struct {
	calls  atomic.Int32
	failOn bool // when true, Check returns an error
}

func (m *mockStrategy) Check(_ context.Context, _ string, _ RateLimitConfig) (RateLimitResult, error) {
	m.calls.Add(1)
	if m.failOn {
		return RateLimitResult{}, errors.New("mock primary failure")
	}
	return RateLimitResult{Allowed: true, Remaining: 42}, nil
}

// mockHealthChecker simulates the health.Checker interface. healthy controls
// whether IsHealthy() returns true; failureReports counts how many times
// ReportFailure() was called.
type mockHealthChecker struct {
	healthy        atomic.Bool
	failureReports atomic.Int32
}

func (h *mockHealthChecker) IsHealthy() bool        { return h.healthy.Load() }
func (h *mockHealthChecker) ReportFailure()          { h.failureReports.Add(1); h.healthy.Store(false) }

// noopLogger discards all log output — keeps test output clean.
type noopLogger struct{}

func (noopLogger) Info(_ string, _ map[string]any)  {}
func (noopLogger) Error(_ string, _ map[string]any) {}

// --- helpers ---

var testConfig = RateLimitConfig{
	MaxRequestsPerWindow: 10,
	WindowSize:           time.Hour,
}

func newWrapper(primary, fallback RateLimitStrategy, hc *mockHealthChecker) *ResilientWrapper {
	return NewResilientWrapper(primary, fallback, hc, noopLogger{})
}

// --- tests ---

// TestResilientWrapper_HealthyPrimaryHandlesRequest verifies the happy path:
// when the health checker says "healthy", the wrapper routes to primary and
// never touches the fallback.
func TestResilientWrapper_HealthyPrimaryHandlesRequest(t *testing.T) {
	t.Log("Proving: when primary (Redis) is healthy, requests go to primary and fallback is never called")
	primary := &mockStrategy{}
	fallback := &mockStrategy{}
	hc := &mockHealthChecker{}
	hc.healthy.Store(true)

	w := newWrapper(primary, fallback, hc)

	result, err := w.Check(context.Background(), "client-1", testConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Fatal("expected allowed from primary")
	}
	if primary.calls.Load() != 1 {
		t.Errorf("primary call count = %d, want 1", primary.calls.Load())
	}
	if fallback.calls.Load() != 0 {
		t.Errorf("fallback call count = %d, want 0", fallback.calls.Load())
	}
}

// TestResilientWrapper_PrimaryErrorFallsBackToLocal verifies the degradation
// path: when the primary strategy returns an error, the wrapper must:
//   1. Fall back to the local strategy (request still succeeds).
//   2. Report a health failure (so subsequent requests skip primary).
func TestResilientWrapper_PrimaryErrorFallsBackToLocal(t *testing.T) {
	t.Log("Proving: when primary returns an error, wrapper falls back to local strategy and reports health failure")
	primary := &mockStrategy{failOn: true}
	fallback := NewLocalTokenBucketStrategy()
	hc := &mockHealthChecker{}
	hc.healthy.Store(true)

	w := newWrapper(primary, fallback, hc)

	result, err := w.Check(context.Background(), "client-2", testConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Fatal("expected fallback to allow first request")
	}
	if primary.calls.Load() != 1 {
		t.Errorf("primary call count = %d, want 1", primary.calls.Load())
	}
	// ReportFailure must have been called exactly once to mark health as down.
	if hc.failureReports.Load() != 1 {
		t.Errorf("ReportFailure called %d times, want 1", hc.failureReports.Load())
	}
	if hc.IsHealthy() {
		t.Error("health should be unhealthy after ReportFailure")
	}
}

// TestResilientWrapper_UnhealthySkipsPrimary verifies that when the health
// checker reports unhealthy, the wrapper doesn't waste a round-trip to the
// primary — it routes directly to the fallback.
func TestResilientWrapper_UnhealthySkipsPrimary(t *testing.T) {
	t.Log("Proving: when health checker reports unhealthy, primary is skipped entirely and fallback handles the request")
	primary := &mockStrategy{}
	fallback := NewLocalTokenBucketStrategy()
	hc := &mockHealthChecker{}
	hc.healthy.Store(false) // start in unhealthy state

	w := newWrapper(primary, fallback, hc)

	result, err := w.Check(context.Background(), "client-3", testConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Fatal("expected fallback to allow first request")
	}
	// Primary must never have been called.
	if primary.calls.Load() != 0 {
		t.Errorf("primary should not be called when unhealthy, got %d calls", primary.calls.Load())
	}
}

// TestResilientWrapper_RecoverySwitchesBackToPrimary verifies the recovery
// path: after the health checker transitions from unhealthy back to healthy,
// the wrapper must route subsequent requests back to the primary strategy.
func TestResilientWrapper_RecoverySwitchesBackToPrimary(t *testing.T) {
	t.Log("Proving: after recovery (health restored), wrapper switches back to primary strategy")
	primary := &mockStrategy{}
	fallback := &mockStrategy{}
	hc := &mockHealthChecker{}
	hc.healthy.Store(false) // start unhealthy

	w := newWrapper(primary, fallback, hc)

	// While unhealthy — fallback handles it.
	_, _ = w.Check(context.Background(), "client-4", testConfig)
	if primary.calls.Load() != 0 {
		t.Fatalf("primary called while unhealthy")
	}
	if fallback.calls.Load() != 1 {
		t.Fatalf("fallback call count = %d, want 1", fallback.calls.Load())
	}

	// Simulate recovery — external health checker marks primary as healthy again.
	hc.healthy.Store(true)

	// Now primary should handle the request, not fallback.
	_, _ = w.Check(context.Background(), "client-4", testConfig)
	if primary.calls.Load() != 1 {
		t.Errorf("primary call count after recovery = %d, want 1", primary.calls.Load())
	}
	if fallback.calls.Load() != 1 {
		t.Errorf("fallback call count after recovery = %d, want 1 (no new calls)", fallback.calls.Load())
	}
}
