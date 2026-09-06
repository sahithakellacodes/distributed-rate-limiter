package health

import (
	"context"
	"strconv"
	"time"
	"sync"
	"math"

	redis "github.com/sahithakellacodes/distributed-rate-limiter/internal/redis"
)

// Health checker follows a fast-down, slow-up mechanism  

type Checker interface {
	IsHealthy() bool
	ReportFailure()
}

type HealthChecker struct {
	mutex              sync.Mutex
	healthy            bool // Gives information if service is healthy.
	consecutiveSuccess int  // Used to check number of consecutive success pings. This will help determine if we want to switch back to online service. Useful to prevent flapping.
	interval		   time.Duration
	consecutiveSuccessRequired int
}

func NewHealthChecker(interval time.Duration, consecutiveSuccessRequired int) *HealthChecker {
	return &HealthChecker{
		healthy: true,
		interval: interval,
		consecutiveSuccessRequired: consecutiveSuccessRequired,
	}
}

func (h *HealthChecker) StartHealthChecks(ctx context.Context, redisClient *redis.Client) {
	go h.runHealthChecks(ctx, redisClient)
}

// Might need to return func(...)
func (h *HealthChecker) runHealthChecks(ctx context.Context, redisClient *redis.Client) error {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			err := redisClient.Ping(ctx)
			h.mutex.Lock()
			if err == nil {
				h.consecutiveSuccess = math.min(h.consecutiveSuccess + 1, h.consecutiveSuccessRequired)
				if h.consecutiveSuccess >= h.consecutiveSuccessRequired {
					h.healthy = true
				}
			} else {
				h.consecutiveSuccess = 0
				h.healthy = false
			}
			h.mutex.Unlock()
		case <-ctx.Done():
			return nil
		}
	}
}

func (h *HealthChecker) IsHealthy() bool {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	return h.healthy
}

func (h *HealthChecker) ReportFailure() {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.consecutiveSuccess = 0
	h.healthy = false
}
