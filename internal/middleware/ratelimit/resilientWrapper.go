package ratelimit

import (
	"context"

	"github.com/sahithakellacodes/distributed-rate-limiter/internal/health"
)

type ResilientWrapper struct {
	primary  RateLimitStrategy // redis backed
	fallback RateLimitStrategy // local
	health   health.Checker
}


func (w *ResilientWrapper) Check(
	ctx context.Context,
	identifier string,
	config RateLimitConfig,
) (RateLimitResult, error) {
	if w.health.IsHealthy() {
		res, err := w.primary.Check(ctx, identifier, config)
		if err == nil {
			return res, nil
		}
		w.health.ReportFailure()                     // accelerate failure detection
		return w.fallback.Check(ctx, identifier, config) // degrade this request to local
	}
	return w.fallback.Check(ctx, identifier, config)
}