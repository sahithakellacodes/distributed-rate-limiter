package main

import (
	"fmt"
	"io"
	"math/rand"
	"net/http"
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
	concurrency    = 20
	totalRequests  = 100000
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
	fmt.Printf("Bucket:         %d requests / %s\n", bucketCapacity, windowSize)
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

	start := time.Now()

	// Split exactly totalRequests across the workers.
	baseRequests := totalRequests / concurrency
	remainder := totalRequests % concurrency

	for workerID := 0; workerID < concurrency; workerID++ {
		wg.Add(1)

		workerRequests := baseRequests
		if workerID < remainder {
			workerRequests++
		}

		go func(id, requestCount int) {
			defer wg.Done()

			// Independent random source for each worker.
			rng := rand.New(
				rand.NewSource(int64(id) + time.Now().UnixNano()),
			)

			for i := 0; i < requestCount; i++ {
				gateway := gatewayURLs[rng.Intn(len(gatewayURLs))]

				status := sendRequest(
					client,
					gateway,
					apiKey,
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

	elapsed := time.Since(start)

	total := s.total.Load()
	allowed := s.allowed.Load()
	denied := s.denied.Load()
	errors := s.errors.Load()

	refillRate := float64(bucketCapacity) / windowSize.Seconds()

	// Theoretical token budget over the actual duration:
	//
	//   initial capacity + refill_rate * elapsed
	//
	// Keep everything as int64 because the atomic counters return int64.
	theoreticalBudget := int64(bucketCapacity) +
		int64(elapsed.Seconds()*refillRate)

	fmt.Println("=== Results ===")
	fmt.Printf("Duration:             %s\n", elapsed)
	fmt.Printf("Total requests:       %d\n", total)
	fmt.Printf("Allowed (200):        %d\n", allowed)
	fmt.Printf("Denied (429):         %d\n", denied)
	fmt.Printf("Errors:               %d\n", errors)
	fmt.Printf(
		"Throughput:           %.0f req/sec\n",
		float64(total)/elapsed.Seconds(),
	)
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

func sendRequest(client *http.Client, gateway, apiKey string) int {
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