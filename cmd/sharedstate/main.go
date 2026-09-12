package main

import (
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

// ─── Tunable constants ───────────────────────────────────────────────

var (
	gatewayURLs          = []string{"http://127.0.0.1:8080", "http://127.0.0.1:8082", "http://127.0.0.1:8083"}
	apiKey               = "test-key-1"
	concurrency          = 20
	numWindows           = 3
	maxRequestsPerWindow = 100
	windowSize           = 10 * time.Second
)

// ─── Per-request result ──────────────────────────────────────────────

type requestResult struct {
	timestamp  time.Time
	gateway    string
	httpStatus int
}

// ─── Main ────────────────────────────────────────────────────────────

func main() {
	totalDuration := time.Duration(numWindows) * windowSize
	deadline := time.Now().Add(totalDuration)
	testStart := time.Now()

	refillRate := float64(maxRequestsPerWindow) / windowSize.Seconds()

	fmt.Println("=== Shared-State Correctness Test ===")
	fmt.Printf("Config: maxRequests=%d, window=%s, concurrency=%d, windows=%d\n",
		maxRequestsPerWindow, windowSize, concurrency, numWindows)
	fmt.Printf("Gateways: %v\n", gatewayURLs)
	fmt.Printf("Refill rate: %.2f tokens/sec\n", refillRate)
	fmt.Printf("Total duration: %s\n\n", totalDuration)

	httpClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 50,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	var wg sync.WaitGroup
	perWorker := make([][]requestResult, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		perWorker[i] = make([]requestResult, 0, 1024)

		go func(workerID int) {
			defer wg.Done()

			for time.Now().Before(deadline) {
				gw := gatewayURLs[rand.Intn(len(gatewayURLs))]
				ts := time.Now()
				status := sendRequest(httpClient, gw, apiKey)

				perWorker[workerID] = append(perWorker[workerID], requestResult{
					timestamp:  ts,
					gateway:    gw,
					httpStatus: status,
				})
			}
		}(i)
	}

	wg.Wait()

	// Merge
	var results []requestResult
	for _, ws := range perWorker {
		results = append(results, ws...)
	}

	printReport(results, testStart, refillRate)
}

// ─── HTTP helper ─────────────────────────────────────────────────────

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

// ─── Reporting ───────────────────────────────────────────────────────

type windowStats struct {
	sent     int
	allowed  int
	denied   int
	errors   int
}

func printReport(results []requestResult, testStart time.Time, refillRate float64) {
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

		switch {
		case r.httpStatus == http.StatusOK:
			windows[idx].allowed++
		case r.httpStatus == http.StatusTooManyRequests:
			windows[idx].denied++
		default:
			windows[idx].errors++
		}
	}

	fmt.Println()

	var totals windowStats

	for i, w := range windows {
		lo := time.Duration(i) * windowSize
		hi := lo + windowSize
		windowDuration := windowSize.Seconds()

		// Window 1 starts from a full bucket; subsequent windows only get refill
		var theoreticalMax int
		if i == 0 {
			theoreticalMax = maxRequestsPerWindow + int(windowDuration*refillRate)
		} else {
			theoreticalMax = int(windowDuration * refillRate)
		}

		fmt.Printf("Window %d (%s–%s):\n", i+1, lo, hi)
		fmt.Printf("  Sent:            %d\n", w.sent)
		fmt.Printf("  Allowed:         %d\n", w.allowed)
		fmt.Printf("  Denied:          %d\n", w.denied)
		fmt.Printf("  Theoretical max: %d\n", theoreticalMax)

		if w.allowed > theoreticalMax {
			fmt.Printf("  ⚠ EXCEEDED theoretical max by %d\n", w.allowed-theoreticalMax)
		} else {
			fmt.Printf("  ✓ Within bounds (headroom: %d)\n", theoreticalMax-w.allowed)
		}

		if w.errors > 0 {
			fmt.Printf("  Errors:          %d\n", w.errors)
		}
		fmt.Println()

		totals.sent += w.sent
		totals.allowed += w.allowed
		totals.denied += w.denied
		totals.errors += w.errors
	}

	totalDuration := time.Duration(numWindows) * windowSize
	totalTheoreticalMax := maxRequestsPerWindow + int(totalDuration.Seconds()*refillRate)

	fmt.Println("Totals:")
	fmt.Printf("  Sent:            %d\n", totals.sent)
	fmt.Printf("  Allowed:         %d\n", totals.allowed)
	fmt.Printf("  Denied:          %d\n", totals.denied)
	fmt.Printf("  Theoretical max: %d\n", totalTheoreticalMax)

	if totals.allowed > totalTheoreticalMax {
		fmt.Printf("  ⚠ EXCEEDED theoretical max by %d\n", totals.allowed-totalTheoreticalMax)
	} else {
		fmt.Printf("  ✓ Within bounds (headroom: %d)\n", totalTheoreticalMax-totals.allowed)
	}

	if totals.errors > 0 {
		fmt.Printf("  Errors:          %d\n", totals.errors)
	}
}
