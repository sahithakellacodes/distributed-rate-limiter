package health

import (
	"context"
	"strconv"
	"time"
	"sync"
	"os"
	"fmt"

	redis "github.com/sahithakellacodes/distributed-rate-limiter/internal/redis"
)

type Checker interface {
	IsHealthy() bool
	ReportFailure()
}

type HealthChecker struct {
	mutex              sync.Mutex
	healthy            bool // Gives information if service is healthy.
	consecutiveSuccess int  // Used to check number of consecutive success pings. This will help determine if we want to switch back to online service. Useful to prevent flapping.
}

func NewHealthChecker() *HealthChecker {
	return &HealthChecker{
		healthy: true,
	}
}

func (h *HealthChecker) StartHealthChecks(ctx context.Context, redisClient *redis.Client) {
	go h.runHealthChecks(ctx, redisClient)
}

func (h *HealthChecker) runHealthChecks(ctx context.Context, redisClient *redis.Client) error {

	healthCheckInterval := os.Getenv("HEALTH_CHECK_INTERVAL")
	interval, err := time.ParseDuration(healthCheckInterval)
	if err != nil {
		return fmt.Errorf("invalid HEALTH_CHECK_INTERVAL: %w", err)
	}
	consecutiveSuccessRequired, err := strconv.Atoi(os.Getenv("HEALTH_CHECK_CONSECUTIVE_POSITIVE"))
	if err != nil {
		return fmt.Errorf("invalid HEALTH_CHECK_CONSECUTIVE_POSITIVE: %w", err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			err := redisClient.Ping(ctx)
			h.mutex.Lock()
			if err == nil {
				h.consecutiveSuccess++
				if h.consecutiveSuccess >= consecutiveSuccessRequired {
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
