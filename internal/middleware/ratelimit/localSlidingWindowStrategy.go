package ratelimit

import (
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
