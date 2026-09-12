package main

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/sahithakellacodes/distributed-rate-limiter/internal/middleware/ratelimit"
)

// ─── Tunable constants ───────────────────────────────────────────────
// Change these to match your docker-compose / gateway configuration.

var (
	gatewayURLs          = []string{"http://127.0.0.1:8080", "http://127.0.0.1:8082", "http://127.0.0.1:8083"}
	apiKey               = "test-key-1"
	concurrency          = 20       // number of worker goroutines
	numWindows           = 3        // how many windows to run for
	maxRequestsPerWindow = 100        // matches compose X_RATELIMIT_LIMIT
	windowSize           = 10 * time.Second // matches gateway config
)

// clientID must match what InMemoryAuthStore resolves for apiKey.
// test-key-1 → client-1
const clientID = "client-1"

// ─── Per-request result ──────────────────────────────────────────────

type requestResult struct {
	timestamp    time.Time // when the request was fired
	gateway      string    // which gateway was hit
	localAllowed bool      // what the in-process algorithm said
	httpStatus   int       // what the distributed system returned (200 or 429)
}

// ─── Main ────────────────────────────────────────────────────────────

func main() {
	totalDuration := time.Duration(numWindows) * windowSize
	deadline := time.Now().Add(totalDuration)
	testStart := time.Now()

	fmt.Println("=== Shared-State Load Test ===")
	fmt.Printf("Config: maxRequests=%d, window=%s, concurrency=%d, windows=%d\n",
		maxRequestsPerWindow, windowSize, concurrency, numWindows)
	fmt.Printf("Gateways: %v\n", gatewayURLs)
	fmt.Printf("Total duration: %s\n\n", totalDuration)

	// The local strategy is a single instance shared across all goroutines.
	// It is mutex-guarded internally so it is safe under concurrency.
	// It represents "what one perfect single-process rate limiter would do".
	//
	// NOTE: The local strategy starts with a full bucket, same as Redis on a
	// fresh key. If Redis already has state from a previous run (compose wasn't
	// restarted), the distributed results will reflect that stale state while
	// the local oracle starts fresh. For a clean comparison, restart compose or
	// use a unique API key / client per run.
	localStrategy := ratelimit.NewLocalTokenBucketStrategy()

	config := ratelimit.RateLimitConfig{
		MaxRequestsPerWindow: maxRequestsPerWindow,
		WindowSize:           windowSize,
	}

	httpClient := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{
		MaxIdleConns: 100,
		MaxIdleConnsPerHost: 50,
		IdleConnTimeout: 90 * time.Second,
	}}

	// Each goroutine collects results in its own slice to avoid lock
	// contention during the hot loop. Slices are merged after completion.
	var wg sync.WaitGroup
	perWorker := make([][]requestResult, concurrency)

	for i := range concurrency {
		wg.Add(1)
		perWorker[i] = make([]requestResult, 0, 1024)

		go func(workerID int) {
			defer wg.Done()

			for time.Now().Before(deadline) {
				// Pick a random gateway per request.
				gw := gatewayURLs[rand.Intn(len(gatewayURLs))]

				// ── Local oracle check ──
				localResult, _ := localStrategy.Check(context.Background(), clientID, config)

				// ── Distributed check (HTTP) ──
				ts := time.Now()
				status := sendRequest(httpClient, gw, apiKey)

				perWorker[workerID] = append(perWorker[workerID], requestResult{
					timestamp:    ts,
					gateway:      gw,
					localAllowed: localResult.Allowed,
					httpStatus:   status,
				})
				// No sleep — continuous pressure.
			}
		}(i)
	}

	wg.Wait()

	// ── Merge all per-worker slices ──
	var results []requestResult
	for _, ws := range perWorker {
		results = append(results, ws...)
	}

	// ── Bucket into windows and print histograms ──
	printHistograms(results, testStart)
}

// ─── HTTP helper ─────────────────────────────────────────────────────

// sendRequest fires a GET to gateway/products with the API key header.
// Returns the HTTP status code, or -1 on error/timeout.
func sendRequest(client *http.Client, gateway, key string) int {
	req, err := http.NewRequest(http.MethodGet, gateway+"/products", nil)
	if err != nil {
		return -1
	}
	req.Header.Set("X-API-Key", key)

	resp, err := client.Do(req)
	if err != nil {
		return -1
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode
}

// ─── Histogram / reporting ───────────────────────────────────────────

type windowStats struct {
	sent             int
	algoAllowed      int
	algoDenied       int
	distAllowed      int
	distDenied       int
	distErrors       int
}

func printHistograms(results []requestResult, testStart time.Time) {
	windows := make([]windowStats, numWindows)

	for _, r := range results {
		elapsed := r.timestamp.Sub(testStart)
		idx := int(elapsed / windowSize)
		if idx < 0 {
			idx = 0
		}
		if idx >= numWindows {
			idx = numWindows - 1
		}

		windows[idx].sent++

		if r.localAllowed {
			windows[idx].algoAllowed++
		} else {
			windows[idx].algoDenied++
		}

		switch {
		case r.httpStatus == http.StatusOK:
			windows[idx].distAllowed++
		case r.httpStatus == http.StatusTooManyRequests:
			windows[idx].distDenied++
		default:
			// Timeouts, connection errors, unexpected codes.
			windows[idx].distErrors++
		}
	}

	// ── Per-window output ──
	fmt.Printf("\nConfig: maxRequests=%d, window=%s, concurrency=%d, windows=%d\n\n",
		maxRequestsPerWindow, windowSize, concurrency, numWindows)

	var totals windowStats

	for i, w := range windows {
		lo := time.Duration(i) * windowSize
		hi := lo + windowSize

		fmt.Printf("Window %d (%s–%s):\n", i+1, lo, hi)
		fmt.Printf("  Sent: %d\n", w.sent)
		fmt.Printf("  Algorithm:   allowed=%-6d denied=%d\n", w.algoAllowed, w.algoDenied)
		fmt.Printf("  Distributed: allowed=%-6d denied=%d\n", w.distAllowed, w.distDenied)
		if w.distErrors > 0 {
			fmt.Printf("  Errors:      %d\n", w.distErrors)
		}
		fmt.Println()

		totals.sent += w.sent
		totals.algoAllowed += w.algoAllowed
		totals.algoDenied += w.algoDenied
		totals.distAllowed += w.distAllowed
		totals.distDenied += w.distDenied
		totals.distErrors += w.distErrors
	}

	// ── Totals ──
	variance := totals.distAllowed - totals.algoAllowed

	fmt.Println("Totals:")
	fmt.Printf("  Sent: %d\n", totals.sent)
	fmt.Printf("  Algorithm:   allowed=%-6d denied=%d\n", totals.algoAllowed, totals.algoDenied)
	fmt.Printf("  Distributed: allowed=%-6d denied=%d\n", totals.distAllowed, totals.distDenied)
	if totals.distErrors > 0 {
		fmt.Printf("  Errors:      %d\n", totals.distErrors)
	}

	sign := "+"
	if variance < 0 {
		sign = ""
	}
	fmt.Printf("  Variance:    %s%d\n", sign, variance)
}
