package ratelimit

import (
	"context"
	"math"
	"sync"
	"time"
)

type SlidingWindow struct {
	currentWindowStartTime time.Time // The start time of the current window
	currentWindowCount     int       // The number of requests in the current window
	previousWindowCount    int       // The number of requests in the previous window
}

type LocalSlidingWindowStrategy struct {
	mutex   sync.Mutex                // Mutex to ensure thread-safe access to the windows map
	windows map[string]*SlidingWindow // Map to store sliding windows for each identifier (e.g., client ID)
	now     func() time.Time          // Function to get the current time, can be overridden for testing
}

func NewLocalSlidingWindowStrategy() *LocalSlidingWindowStrategy {
	return &LocalSlidingWindowStrategy{
		windows: make(map[string]*SlidingWindow),
		now:     time.Now,
	}
}

func (s *LocalSlidingWindowStrategy) Check(_ context.Context, identifier string, config RateLimitConfig) (RateLimitResult, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	now := s.now()
	window, exists := s.windows[identifier]

	// Create window if window doesn't exist
	if !exists {
		window = &SlidingWindow{
			currentWindowStartTime: now,
			currentWindowCount:     0,
			previousWindowCount:    0,
		}
		s.windows[identifier] = window
	}

	// Check if the current window has expired
	timeElapsedSinceWindowStart := now.Sub(window.currentWindowStartTime)
	if timeElapsedSinceWindowStart >= config.WindowSize {
		elapsedWindows:= timeElapsedSinceWindowStart / config.WindowSize
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
