package main

import (
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	gatewayURLs = []string{
		"http://127.0.0.1:8080",
		"http://127.0.0.1:8082",
		"http://127.0.0.1:8083",
	}

	apiKey         = "test-key-1"
	concurrency    = 500
	totalRequests  = 1500000
	bucketCapacity = 100
	windowSize     = 10 * time.Second
)

type stats struct {
	total   atomic.Int64
	allowed atomic.Int64
	denied  atomic.Int64
	errors  atomic.Int64
}

func main() {
	fmt.Println("=== Distributed Rate Limiter Fixed-Count Load Test ===")
	fmt.Printf("Total requests: %d\n", totalRequests)
	fmt.Printf("Concurrency:    %d\n", concurrency)
	fmt.Printf(
		"Bucket:         %d requests / %s\n",
		bucketCapacity,
		windowSize,
	)
	fmt.Printf(
		"Refill rate:    %.2f tokens/sec\n",
		float64(bucketCapacity)/windowSize.Seconds(),
	)
	fmt.Printf("Gateways:       %v\n\n", gatewayURLs)

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        concurrency * 2,
			MaxIdleConnsPerHost: concurrency * 2,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	var (
		wg sync.WaitGroup
		s  stats
	)

	// Keep latency data local to each worker.
	// This avoids lock contention during the hot path.
	perWorkerLatencies := make([][]time.Duration, concurrency)

	start := time.Now()

	// Progress reporter.
	progressDone := make(chan struct{})

	go reportProgress(&s, start, progressDone)

	// Split exactly totalRequests across workers.
	baseRequests := totalRequests / concurrency
	remainder := totalRequests % concurrency

	for workerID := 0; workerID < concurrency; workerID++ {
		wg.Add(1)

		workerRequests := baseRequests
		if workerID < remainder {
			workerRequests++
		}

		perWorkerLatencies[workerID] = make(
			[]time.Duration,
			0,
			workerRequests,
		)

		go func(id, requestCount int) {
			defer wg.Done()

			rng := rand.New(
				rand.NewSource(int64(id) + time.Now().UnixNano()),
			)

			for i := 0; i < requestCount; i++ {
				gateway := gatewayURLs[rng.Intn(len(gatewayURLs))]

				requestStart := time.Now()

				status := sendRequest(
					client,
					gateway,
					apiKey,
				)

				latency := time.Since(requestStart)

				perWorkerLatencies[id] = append(
					perWorkerLatencies[id],
					latency,
				)

				s.total.Add(1)

				switch status {
				case http.StatusOK:
					s.allowed.Add(1)

				case http.StatusTooManyRequests:
					s.denied.Add(1)

				default:
					s.errors.Add(1)
				}
			}
		}(workerID, workerRequests)
	}

	wg.Wait()

	close(progressDone)

	elapsed := time.Since(start)

	// Print one final 100% progress line.
	printProgress(&s, start, true)

	total := s.total.Load()
	allowed := s.allowed.Load()
	denied := s.denied.Load()
	errors := s.errors.Load()

	refillRate := float64(bucketCapacity) / windowSize.Seconds()

	theoreticalBudget := int64(bucketCapacity) +
		int64(elapsed.Seconds()*refillRate)

	// Merge all worker latency slices.
	latencies := make([]time.Duration, 0, totalRequests)

	for _, workerLatencies := range perWorkerLatencies {
		latencies = append(latencies, workerLatencies...)
	}

	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i] < latencies[j]
	})

	p50 := percentile(latencies, 0.50)
	p95 := percentile(latencies, 0.95)
	p99 := percentile(latencies, 0.99)

	throughput := float64(total) / elapsed.Seconds()

	fmt.Println()
	fmt.Println()
	fmt.Println("=== Results ===")

	fmt.Printf("Duration:             %s\n", elapsed)
	fmt.Printf("Total requests:       %d\n", total)
	fmt.Printf("Allowed (200):        %d\n", allowed)
	fmt.Printf("Denied (429):         %d\n", denied)
	fmt.Printf("Errors:               %d\n", errors)
	fmt.Printf("Throughput:           %.0f req/sec\n", throughput)
	fmt.Printf("P50 latency:          %s\n", p50)
	fmt.Printf("P95 latency:          %s\n", p95)
	fmt.Printf("P99 latency:          %s\n", p99)
	fmt.Printf(
		"Allow rate:           %.2f%%\n",
		float64(allowed)/float64(total)*100,
	)
	fmt.Printf(
		"Deny rate:            %.2f%%\n",
		float64(denied)/float64(total)*100,
	)

	fmt.Println()
	fmt.Println("=== Token Budget ===")
	fmt.Printf("Initial capacity:     %d\n", bucketCapacity)
	fmt.Printf("Refill rate:          %.2f tokens/sec\n", refillRate)
	fmt.Printf("Test duration:        %s\n", elapsed)
	fmt.Printf("Theoretical budget:   %d\n", theoreticalBudget)
	fmt.Printf("Actual allowed:       %d\n", allowed)

	if allowed > theoreticalBudget {
		fmt.Printf(
			"⚠ EXCEEDED by %d requests\n",
			allowed-theoreticalBudget,
		)
	} else {
		fmt.Printf(
			"✓ Within budget (headroom: %d)\n",
			theoreticalBudget-allowed,
		)
	}
}

func reportProgress(
	s *stats,
	start time.Time,
	done <-chan struct{},
) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			printProgress(s, start, false)

		case <-done:
			return
		}
	}
}

func printProgress(
	s *stats,
	start time.Time,
	final bool,
) {
	total := s.total.Load()
	allowed := s.allowed.Load()
	denied := s.denied.Load()
	errors := s.errors.Load()

	elapsed := time.Since(start)

	if total > int64(totalRequests) {
		total = int64(totalRequests)
	}

	percentage := float64(total) / float64(totalRequests)

	const barWidth = 40

	filled := int(percentage * barWidth)
	if filled > barWidth {
		filled = barWidth
	}

	bar := strings.Repeat("=", filled)

	if filled < barWidth && !final {
		bar += ">"
		filled++
	}

	if filled < barWidth {
		bar += strings.Repeat(" ", barWidth-filled)
	}

	throughput := 0.0

	if elapsed > 0 {
		throughput = float64(total) / elapsed.Seconds()
	}

	fmt.Printf(
		"\r[%s] %6.2f%% | %7d / %d | %.0f req/s | allowed %d | 429 %d | errors %d | %s",
		bar,
		percentage*100,
		total,
		totalRequests,
		throughput,
		allowed,
		denied,
		errors,
		formatDuration(elapsed),
	)

	if final {
		fmt.Print("\n")
	}
}

func formatDuration(d time.Duration) string {
	totalSeconds := int(d.Seconds())

	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60

	if hours > 0 {
		return fmt.Sprintf("%02dh %02dm %02ds", hours, minutes, seconds)
	}

	if minutes > 0 {
		return fmt.Sprintf("%02dm %02ds", minutes, seconds)
	}

	return fmt.Sprintf("%02ds", seconds)
}

func sendRequest(
	client *http.Client,
	gateway string,
	apiKey string,
) int {
	req, err := http.NewRequest(
		http.MethodGet,
		gateway+"/products",
		nil,
	)
	if err != nil {
		return -1
	}

	req.Header.Set("X-API-Key", apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return -1
	}

	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	return resp.StatusCode
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}

	if p <= 0 {
		return sorted[0]
	}

	if p >= 1 {
		return sorted[len(sorted)-1]
	}

	index := int(float64(len(sorted)-1) * p)

	return sorted[index]
}