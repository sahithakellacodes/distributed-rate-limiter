package health

import (
	"sync"
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
