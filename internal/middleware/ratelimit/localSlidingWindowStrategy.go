package ratelimit

import (
	"context"
	"math"
	"sync"
	"time"
)

type SlidingWindow struct {
	mu                     sync.Mutex
	currentWindowStartTime time.Time // The start time of the current window
	currentWindowCount     int       // The number of requests in the current window
	previousWindowCount    int       // The number of requests in the previous window
}

type LocalSlidingWindowStrategy struct {
	windows sync.Map         // map[string]*SlidingWindow — per-client sliding windows
	now     func() time.Time // Function to get the current time, can be overridden for testing
}

func NewLocalSlidingWindowStrategy() *LocalSlidingWindowStrategy {
	return &LocalSlidingWindowStrategy{
		now: time.Now,
	}
}

func (s *LocalSlidingWindowStrategy) Check(_ context.Context, identifier string, config RateLimitConfig) (RateLimitResult, error) {
	now := s.now()

	// Load or lazily create the per-client window. LoadOrStore is atomic, so
	// two goroutines hitting a new client ID simultaneously both get the same
	// *SlidingWindow back — only one allocation survives.
	val, loaded := s.windows.Load(identifier)
	if !loaded {
		newWindow := &SlidingWindow{
			currentWindowStartTime: now,
			currentWindowCount:     0,
			previousWindowCount:    0,
		}
		val, _ = s.windows.LoadOrStore(identifier, newWindow)
	}
	window := val.(*SlidingWindow)

	// Only requests for this client contend on this lock.
	window.mu.Lock()
	defer window.mu.Unlock()

	// Check if the current window has expired
	timeElapsedSinceWindowStart := now.Sub(window.currentWindowStartTime)
	if timeElapsedSinceWindowStart >= config.WindowSize {
		elapsedWindows := timeElapsedSinceWindowStart / config.WindowSize
		if elapsedWindows >= 2 {
			window.previousWindowCount = 0
		} else {
			window.previousWindowCount = window.currentWindowCount
		}
		window.currentWindowCount = 0
		window.currentWindowStartTime = window.currentWindowStartTime.Add(
			time.Duration(elapsedWindows) * config.WindowSize,
		)
	}

	fractionOfPreviousWindow := 1.0 - (float64(now.Sub(window.currentWindowStartTime)) / float64(config.WindowSize))
	consumedCount := float64(window.previousWindowCount)*fractionOfPreviousWindow + float64(window.currentWindowCount)
	remainingRequestsCount := float64(config.MaxRequestsPerWindow) - consumedCount

	if remainingRequestsCount < 1-tokenEpsilon {
		retryAfterSeconds := 0

		if window.previousWindowCount > 0 {
			previousWindowSeconds := config.WindowSize.Seconds() * fractionOfPreviousWindow
			retryAfterSeconds = int(math.Ceil(previousWindowSeconds / float64(window.previousWindowCount)))
		} else {
			retryAfterSeconds = int(math.Ceil(
				window.currentWindowStartTime.Add(config.WindowSize).Sub(now).Seconds(),
			))
		}

		return RateLimitResult{
			Allowed:           false,
			Remaining:         0,
			RetryAfterSeconds: retryAfterSeconds,
		}, nil
	}

	window.currentWindowCount += 1
	return RateLimitResult{
		Allowed:           true,
		Remaining:         int(math.Floor(remainingRequestsCount + tokenEpsilon)) - 1,
		RetryAfterSeconds: 0,
	}, nil
}
